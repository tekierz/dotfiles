package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestManageReviewedPackageOnlyPlanExecutesWithoutFilesystemBackup(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	setManageTruthSnapshot(t, app, 61, pkg.PlatformMacOS, "brew",
		plannerSnapshotObservation(t, runtime, "zsh", health.PresenceMissing, health.InstallabilitySupported),
	)

	plan, err := buildInstallPlanForTools(app, runtime, time.Now(), []string{"zsh"})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.backupTargets(); len(got) != 0 {
		t.Fatalf("package-only backup targets = %v, want none", got)
	}
	specs, err := plan.backupTargetSpecs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 0 {
		t.Fatalf("package-only backup specs = %v, want none", specs)
	}
	if got := plan.selectedToolIDs(); !slices.Equal(got, []string{"zsh"}) {
		t.Fatalf("selected tool IDs = %v, want [zsh]", got)
	}

	zsh, ok := runtime.lookupTool("zsh")
	if !ok {
		t.Fatal("zsh missing from registry")
	}
	wantRecipe, err := tools.DescribeInstall(zsh, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	manager := &pinnedRecipeExecutionManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	runtime.detectManager = func() pkg.PackageManager { return manager }

	events := make(chan installEventMsg, 128)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err != nil {
		t.Fatalf("package-only reviewed execution failed: %v\n%s", terminal.err, terminal.context)
	}
	if len(wantRecipe.Steps) != 1 || !reflect.DeepEqual(manager.installCalls, [][]string{wantRecipe.Steps[0].Packages}) {
		t.Fatalf("install calls = %v, want exact recipe %v", manager.installCalls, wantRecipe.Steps)
	}

	journal, err := operation.DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	record, err := journal.Read(terminal.operationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != operation.StatusSucceeded || record.PlanHash != plan.hash() {
		t.Fatalf("terminal journal = status %s plan %q, want succeeded plan %q", record.Status, record.PlanHash, plan.hash())
	}
}

// TestManageInstallOnlyPlanningIgnoresUnrelatedConfigPreflight keeps the
// one-tool Manage install/repair path independent from configuration products
// that it will not mutate. A hostile Yazi override and imported-config errors
// must not prevent review of a package-only zsh action or leak unrelated
// state/config/helper actions into that review.
func TestManageInstallOnlyPlanningIgnoresUnrelatedConfigPreflight(t *testing.T) {
	for _, presence := range []health.Presence{health.PresenceMissing, health.PresencePartial} {
		t.Run(string(presence), func(t *testing.T) {
			ctx := newGoldenContext(t)
			app := ctx.app
			app.deepDiveConfig.CLITools = map[string]bool{}
			app.deepDiveConfig.GUIApps = map[string]bool{}
			app.deepDiveConfig.CLIUtilities = map[string]bool{}
			app.deepDiveConfig.MacApps = map[string]bool{}
			app.deepDiveConfig.Utilities = map[string]bool{"hk": true}
			app.nativeConfigState = NativeManageConfigState{
				PreferenceError: "unrelated invalid manage preferences",
				GhosttyError:    "unrelated invalid Ghostty config",
				YaziError:       "unrelated invalid Yazi config",
			}
			t.Setenv("YAZI_CONFIG_HOME", "relative/unsafe")
			if _, err := tools.ResolveYaziConfigPaths(); err == nil {
				t.Fatal("hostile YAZI_CONFIG_HOME fixture unexpectedly passed validation")
			}
			xdg := filepath.Join(t.TempDir(), "xdg")
			globalDir := filepath.Join(xdg, "dotfiles")
			if err := os.MkdirAll(globalDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(globalDir, "global.json"), []byte(`{"theme":`), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_CONFIG_HOME", xdg)
			if _, err := config.LoadGlobalConfig(); err == nil {
				t.Fatal("malformed unrelated global config fixture unexpectedly passed validation")
			}

			planRuntime := registryRuntime(pkg.PlatformMacOS, map[string]bool{})
			observation := plannerSnapshotObservation(t, planRuntime, "zsh", presence, health.InstallabilitySupported)
			setManageTruthSnapshot(t, app, 71, pkg.PlatformMacOS, "brew", observation)

			assertPackageOnly := func(label string, plan *installPlan, err error) {
				t.Helper()
				if err != nil || plan == nil {
					t.Fatalf("%s plan nonnil=%v error=%v", label, plan != nil, err)
				}
				actions := plan.actions()
				if len(actions) != 1 {
					t.Fatalf("%s actions=%+v, want exactly one package action", label, actions)
				}
				action := actions[0]
				if action.ID != "install:zsh" || action.Kind != operation.KindInstallTool || action.ToolID != "zsh" || action.Disposition != operation.DispositionApply {
					t.Fatalf("%s action=%+v, want one applicable zsh install action", label, action)
				}
				for _, action := range actions {
					if action.Kind == operation.KindUpdateState || action.Kind == operation.KindWriteConfig || action.Kind == operation.KindInstallFile {
						t.Fatalf("%s leaked unrelated state/config/helper action: %+v", label, action)
					}
				}
			}

			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			helperPath := filepath.Join(home, ".local", "bin", "hk")
			if err := os.MkdirAll(filepath.Dir(helperPath), 0o700); err != nil {
				t.Fatal(err)
			}
			userHelper := []byte("#!/bin/sh\necho user-owned\n")
			if err := os.WriteFile(helperPath, userHelper, 0o700); err != nil {
				t.Fatal(err)
			}
			plan, err := buildInstallPlanForTools(app, planRuntime, time.Now(), []string{"zsh"})
			assertPackageOnly("direct", plan, err)
			manager := &pinnedRecipeExecutionManager{MockPackageManager: pkg.NewMockPackageManager()}
			manager.ManagerName = "brew"
			executionRuntime := planRuntime
			executionRuntime.detectManager = func() pkg.PackageManager { return manager }
			events := make(chan installEventMsg, 128)
			go runInstallPlanWorker(context.Background(), events, plan, executionRuntime)
			var terminal installEventMsg
			for event := range events {
				if event.done {
					terminal = event
				}
			}
			if terminal.err != nil {
				t.Fatalf("install-only execution inspected unrelated configuration: %v\n%s", terminal.err, terminal.context)
			}
			gotHelper, err := os.ReadFile(helperPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotHelper, userHelper) {
				t.Fatalf("install-only execution mutated unrelated hk helper: got %q want %q", gotHelper, userHelper)
			}

			selectManageTruthItem(t, app, "zsh")
			app.managePane = managePaneSettings
			app.pendingInstallPlan = nil
			app.installPlanError = nil
			cmd := NewManageScreen(ctx).handleKey(keyMsg("i"))
			assertPackageOnly("Manage I", app.pendingInstallPlan, app.installPlanError)
			if cmd == nil {
				t.Fatal("Manage I returned no review navigation command")
			}
			msg, ok := cmd().(NavigateMsg)
			if !ok || msg.To != ScreenFileTree {
				t.Fatalf("Manage I navigation=%#v, want FileTree review", msg)
			}
		})
	}
}

func TestReviewedFilesystemMutationWithEmptyBackupScopeFailsClosed(t *testing.T) {
	_, _, installRuntime := newPlanTestApp(t)
	document, err := operation.NewPlan(time.Now(), []operation.Action{{
		ID:            "helper:sentinel",
		Kind:          operation.KindInstallFile,
		ToolID:        "sentinel",
		Target:        ".local/bin/sentinel",
		Description:   "install sentinel managed helper",
		Disposition:   operation.DispositionApply,
		DesiredDigest: strings.Repeat("a", 64),
		Ownership:     operation.OwnershipManagedFile,
		Reversibility: operation.ReversibilityManual,
		Observation:   operation.Observation{Source: ".local/bin/sentinel"},
		Observations:  []operation.Observation{{Source: ".local/bin/sentinel"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	statePlan, err := operation.CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	plan := &installPlan{
		document:      document,
		selectedTools: []string{},
		config:        DeepDiveConfig{},
		authority:     map[string]map[string]acceptedTarget{},
		statePlan:     statePlan,
	}
	if got := plan.backupTargets(); len(got) != 0 {
		t.Fatalf("unsafe fixture backup targets = %v, want none", got)
	}

	events := make(chan installEventMsg, 128)
	go runInstallPlanWorker(context.Background(), events, plan, installRuntime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err == nil || !strings.Contains(terminal.err.Error(), "mandatory rollback point was not created") {
		t.Fatalf("terminal error = %v, want fail-closed empty rollback scope", terminal.err)
	}
}

func TestProductionManageInstallUsesOnlyReviewedPlanRoute(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	entries, err := os.ReadDir(filepath.Dir(currentFile))
	if err != nil {
		t.Fatal(err)
	}
	obsolete := []string{
		"manageInstallWithLogsMsg",
		"manageInstallDoneMsg",
		"installToolWithRuntimeCmd",
		"streamingInstallToolCmdWithRuntime",
		"handleManageInstallWithLogsMsg",
		"handleManageInstallDoneMsg",
	}
	var found []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(filepath.Dir(currentFile), name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, token := range obsolete {
			if strings.Contains(string(data), token) {
				found = append(found, fmt.Sprintf("%s:%s", name, token))
			}
		}
	}
	if len(found) != 0 {
		t.Fatalf("obsolete direct manage-install route remains in production sources: %v", found)
	}
}
