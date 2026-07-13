package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type planTestWriter struct {
	bytes.Buffer
	writes int
	short  bool
	err    error
}

func (writer *planTestWriter) Write(value []byte) (int, error) {
	writer.writes++
	if writer.err != nil {
		return 0, writer.err
	}
	if writer.short && len(value) != 0 {
		_, _ = writer.Buffer.Write(value[:len(value)-1])
		return len(value) - 1, nil
	}
	return writer.Buffer.Write(value)
}

func TestPlanCommandRequiresJSONNoArgsAndRepeatedLocalTool(t *testing.T) {
	command := newPlanCommand(planJSONRuntime{})
	if command.Use != "plan" || command.Args == nil {
		t.Fatalf("plan command identity=%q args=%v", command.Use, command.Args)
	}
	jsonFlag, toolFlag := command.Flags().Lookup("json"), command.Flags().Lookup("tool")
	if jsonFlag == nil || jsonFlag.DefValue != "false" || toolFlag == nil || toolFlag.DefValue != "[]" {
		t.Fatalf("plan flags json=%#v tool=%#v", jsonFlag, toolFlag)
	}
	if command.InheritedFlags().Lookup("tool") != nil {
		t.Fatal("--tool escaped the local plan command")
	}
}

func TestPlanJSONEmptyIntentWritesOnceAndCallsNoRuntimeDependency(t *testing.T) {
	calls := 0
	panicCall := func() { calls++; panic("empty intent called runtime") }
	runtime := planJSONRuntime{
		registry:       func() []tools.Tool { panicCall(); return nil },
		detectPlatform: func() pkg.Platform { panicCall(); return pkg.PlatformUnknown },
		detectManager:  func() pkg.PackageManager { panicCall(); return nil },
		collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			panicCall()
			return health.InstallationSnapshot{}, nil
		},
		describe: func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
			panicCall()
			return operation.InstallRecipe{}, nil
		},
		capture: func() (*operation.StatePlan, error) { panicCall(); return nil, nil },
		now:     func() time.Time { panicCall(); return time.Time{} },
	}
	for _, arguments := range [][]string{{"--json"}, {"--json", "--tool", "   "}, {"--json", "--tool", "\t"}} {
		command := newPlanCommand(runtime)
		writer := &planTestWriter{}
		command.SetOut(writer)
		command.SetErr(io.Discard)
		command.SetArgs(arguments)
		err := command.Execute()
		var exit *commandExitError
		if !errors.As(err, &exit) || exit.code != 2 || !exit.silent {
			t.Fatalf("empty intent %q error=%#v", arguments, err)
		}
		if calls != 0 || writer.writes != 1 || !bytes.Contains(writer.Bytes(), []byte(`"status":"intent_required"`)) {
			t.Fatalf("empty intent %q calls=%d writes=%d output=%s", arguments, calls, writer.writes, writer.Bytes())
		}
	}
}

func TestPlanJSONShortOrFailedWriteIsGenericFatal(t *testing.T) {
	for _, writer := range []*planTestWriter{{short: true}, {err: errors.New("SECRET writer failure")}} {
		command := newPlanCommand(planJSONRuntime{})
		command.SetOut(writer)
		command.SetErr(io.Discard)
		command.SetArgs([]string{"--json"})
		err := command.Execute()
		if !errors.Is(err, errPlanCollection) || writer.writes != 1 {
			t.Fatalf("writer failure error=%v writes=%d", err, writer.writes)
		}
	}
}

type planRuntimeCalls struct {
	registry, platform, manager, collect, describe, capture, clock int
	generation                                                     uint64
	collectedPlatform                                              pkg.Platform
	collectedManager                                               string
}

func TestPlanJSONReadyNoChangesAndUnknownOutcomes(t *testing.T) {
	for _, test := range []struct {
		name         string
		presence     health.Presence
		selection    string
		wantStatus   string
		wantApply    string
		wantExit     int
		wantDescribe int
		wantCapture  int
	}{
		{name: "ready", presence: health.PresenceMissing, selection: "zsh", wantStatus: "ready", wantApply: "hash_required", wantDescribe: 1, wantCapture: 1},
		{name: "no changes", presence: health.PresencePresent, selection: "zsh", wantStatus: "no_changes", wantApply: "not_available"},
		{name: "unknown", presence: health.PresencePresent, selection: "unknown-tool", wantStatus: "blocked", wantApply: "not_available", wantExit: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, calls := planRuntimeFixture(t, test.presence)
			writer := &planTestWriter{}
			command := newPlanCommand(runtime)
			command.SetOut(writer)
			command.SetErr(io.Discard)
			command.SetArgs([]string{"--json", "--tool", test.selection})
			err := command.Execute()
			if test.wantExit == 0 {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var exit *commandExitError
				if !errors.As(err, &exit) || exit.code != test.wantExit || !exit.silent {
					t.Fatalf("domain error=%#v", err)
				}
			}
			if writer.writes != 1 || !strings.Contains(writer.String(), `"status":"`+test.wantStatus+`"`) {
				t.Fatalf("writes=%d output=%s", writer.writes, writer.String())
			}
			if !strings.Contains(writer.String(), `"apply":"`+test.wantApply+`"`) {
				t.Fatalf("status %q apply capability mismatch: %s", test.wantStatus, writer.String())
			}
			if test.wantStatus == "ready" && !strings.Contains(writer.String(), `"plan_hash":"`) {
				t.Fatalf("ready output omitted complete plan hash: %s", writer.String())
			}
			if test.wantStatus != "ready" && strings.Contains(writer.String(), `"plan_hash"`) {
				t.Fatalf("non-ready output published plan hash: %s", writer.String())
			}
			if calls.registry != 1 || calls.platform != 1 || calls.manager != 1 || calls.collect != 1 ||
				calls.describe != test.wantDescribe || calls.capture != test.wantCapture || calls.clock != test.wantCapture ||
				calls.generation != 1 || calls.collectedPlatform != pkg.PlatformMacOS || calls.collectedManager != "brew" {
				t.Fatalf("runtime calls=%+v", *calls)
			}
		})
	}
}

func TestPlanJSONDuplicateIntentIsCanonicalAndDeterministic(t *testing.T) {
	runtime, _ := planRuntimeFixture(t, health.PresenceMissing)
	run := func(arguments ...string) string {
		t.Helper()
		writer := &planTestWriter{}
		command := newPlanCommand(runtime)
		command.SetOut(writer)
		command.SetErr(io.Discard)
		command.SetArgs(arguments)
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		return writer.String()
	}
	first := run("--json", "--tool", "zsh", "--tool", "zsh")
	second := run("--json", "--tool", "zsh")
	if first != second || !strings.Contains(first, `"tools":["zsh"]`) {
		t.Fatalf("canonical output changed:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestPlanJSONRecipeDriftBlocksAndSnapshotMismatchIsFatal(t *testing.T) {
	runtime, _ := planRuntimeFixture(t, health.PresenceMissing)
	describe := runtime.describe
	runtime.describe = func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
		recipe, err := describe(tool, environment)
		recipe.Manager = "apt"
		return recipe, err
	}
	writer := &planTestWriter{}
	command := newPlanCommand(runtime)
	command.SetOut(writer)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--json", "--tool", "zsh"})
	var exit *commandExitError
	if err := command.Execute(); !errors.As(err, &exit) || exit.code != 2 || !strings.Contains(writer.String(), `"reason_code":"recipe_drift"`) {
		t.Fatalf("recipe drift error=%v output=%s", err, writer.String())
	}

	runtime, _ = planRuntimeFixture(t, health.PresenceMissing)
	collect := runtime.collect
	runtime.collect = func(ctx context.Context, registry []tools.Tool, manager pkg.PackageManager, platform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
		snapshot, err := collect(ctx, registry, manager, platform, generation)
		if err != nil {
			return health.InstallationSnapshot{}, err
		}
		observation, _ := snapshot.Tool("zsh")
		return health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 2, Platform: snapshot.Platform(), Manager: snapshot.Manager(), Tools: []health.InstallationObservation{observation}})
	}
	writer = &planTestWriter{}
	command = newPlanCommand(runtime)
	command.SetOut(writer)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--json", "--tool", "zsh"})
	if err := command.Execute(); !errors.Is(err, errPlanCollection) || writer.Len() != 0 {
		t.Fatalf("snapshot mismatch error=%v output=%q", err, writer.String())
	}
}

func TestPlanJSONRejectsCollectorClaimForUnregisteredTool(t *testing.T) {
	runtime, calls := planRuntimeFixture(t, health.PresencePresent)
	collect := runtime.collect
	runtime.collect = func(ctx context.Context, registry []tools.Tool, manager pkg.PackageManager, platform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
		base, err := collect(ctx, registry, manager, platform, generation)
		if err != nil {
			return health.InstallationSnapshot{}, err
		}
		zsh, _ := base.Tool("zsh")
		unknown, err := presentPlanObservation("unknown-tool", "brew")
		if err != nil {
			return health.InstallationSnapshot{}, err
		}
		return health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: []health.InstallationObservation{zsh, unknown}})
	}
	writer := &planTestWriter{}
	command := newPlanCommand(runtime)
	command.SetOut(writer)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--json", "--tool", "unknown-tool"})
	if err := command.Execute(); !errors.Is(err, errPlanCollection) {
		t.Fatalf("unregistered collector claim error=%#v", err)
	}
	if writer.Len() != 0 || calls.describe != 0 || calls.capture != 0 || calls.clock != 0 {
		t.Fatalf("unregistered collector claim output=%s calls=%+v", writer.String(), *calls)
	}
}

func presentPlanObservation(id, manager string) (health.InstallationObservation, error) {
	recipe := operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion, ToolID: id, Platform: string(pkg.PlatformMacOS), Manager: manager,
		Steps:    []operation.InstallStep{{Kind: operation.InstallStepPackageManager, Provider: manager, Packages: []string{id}}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{id}},
		Risk:     "installs packages from the configured system package manager",
	}
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		return health.InstallationObservation{}, err
	}
	return health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: id, Installability: health.InstallabilitySupported, InstallRecipeDigest: digest,
		Package: health.PackageFacet{State: health.PackagePresent, Provider: manager, ExpectedReceipts: []string{id}, ObservedReceipts: []string{id}, Authoritative: true, Complete: true,
			Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: health.PackagePresent, ExpectedReceipts: []string{id}, ObservedReceipts: []string{id}, Complete: true}}},
		Direct: health.DirectFacet{State: health.ComponentNotApplicable},
	})
}

func planRuntimeFixture(t *testing.T, presence health.Presence) (planJSONRuntime, *planRuntimeCalls) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", "")
	tool := tools.NewZshTool()
	recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	packageState := health.PackageMissing
	packageFacet := health.PackageFacet{State: packageState, Provider: "brew", ExpectedReceipts: []string{"zsh"}, MissingReceipts: []string{"zsh"}, Authoritative: true, Complete: true,
		Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: packageState, ExpectedReceipts: []string{"zsh"}, MissingReceipts: []string{"zsh"}, Complete: true}}}
	if presence == health.PresencePresent {
		packageState = health.PackagePresent
		packageFacet.State = packageState
		packageFacet.MissingReceipts = nil
		packageFacet.ObservedReceipts = []string{"zsh"}
		packageFacet.Namespaces[0].State = packageState
		packageFacet.Namespaces[0].MissingReceipts = nil
		packageFacet.Namespaces[0].ObservedReceipts = []string{"zsh"}
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{ToolID: "zsh", Installability: health.InstallabilitySupported, InstallRecipeDigest: digest, Package: packageFacet, Direct: health.DirectFacet{State: health.ComponentNotApplicable}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: []health.InstallationObservation{observation}})
	if err != nil {
		t.Fatal(err)
	}
	statePlan, err := operation.CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	calls := &planRuntimeCalls{}
	runtime := planJSONRuntime{
		registry:       func() []tools.Tool { calls.registry++; return []tools.Tool{tool} },
		detectPlatform: func() pkg.Platform { calls.platform++; return pkg.PlatformMacOS },
		detectManager: func() pkg.PackageManager {
			calls.manager++
			manager := pkg.NewMockPackageManager()
			manager.ManagerName = "brew"
			setCommandManagerIdentity(t, manager)
			return manager
		},
		collect: func(_ context.Context, _ []tools.Tool, manager pkg.PackageManager, platform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
			calls.collect++
			calls.generation, calls.collectedPlatform = generation, platform
			if manager != nil {
				calls.collectedManager = manager.Name()
			}
			return snapshot, nil
		},
		describe: func(_ tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
			calls.describe++
			return operation.CloneInstallRecipe(recipe), nil
		},
		capture: func() (*operation.StatePlan, error) { calls.capture++; return statePlan, nil },
		now:     func() time.Time { calls.clock++; return time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC) },
	}
	return runtime, calls
}
