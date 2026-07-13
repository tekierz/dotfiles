package ui

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type installplanWiringCalls struct {
	registered int
	lookup     []string
	describe   []string
	capture    int
}

func installplanWiringRuntime(t *testing.T, base toolInstallRuntime, calls *installplanWiringCalls) toolInstallRuntime {
	t.Helper()
	registry := tools.NewRegistry()
	base.registeredToolIDs = func() []string {
		calls.registered++
		all := registry.All()
		ids := make([]string, 0, len(all))
		for _, tool := range all {
			ids = append(ids, tool.ID())
		}
		return ids
	}
	base.lookupTool = func(id string) (tools.Tool, bool) {
		calls.lookup = append(calls.lookup, id)
		return registry.Get(id)
	}
	base.describeInstall = func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
		calls.describe = append(calls.describe, tool.ID())
		return tools.DescribeInstall(tool, environment)
	}
	base.captureStatePlan = func() (*operation.StatePlan, error) {
		calls.capture++
		return operation.CaptureStatePlan()
	}
	base.isToolInstalled = func(tools.Tool) bool {
		t.Fatal("explicit planning consulted a poisoned live installation probe")
		return false
	}
	return base
}

func TestInstallplanServiceWiringCanonicalizesOnceAndAdoptsReady(t *testing.T) {
	app, _, base := newPlanTestApp(t)
	setManageTruthSnapshot(t, app, 141, pkg.PlatformMacOS, "brew",
		manageTruthObservation(t, "zsh", health.PresenceMissing),
		manageTruthObservation(t, "pi", health.PresenceMissing),
	)
	calls := &installplanWiringCalls{}
	runtime := installplanWiringRuntime(t, base, calls)
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)

	plan, err := buildInstallPlanForTools(app, runtime, now, []string{"zsh", "pi", "zsh"})
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || plan.hash() == "" || plan.statePlan == nil {
		t.Fatalf("adopted plan = %+v", plan)
	}
	if got := plan.actions(); len(got) != 2 || got[0].ID != "install:pi" || got[1].ID != "install:zsh" {
		t.Fatalf("canonical adopted actions = %+v", got)
	}
	if plan.selectedTools != nil || plan.configTools != nil || len(plan.authority) != 0 || len(plan.parentDirs) != 0 {
		t.Fatalf("package-only plan leaked wizard authority: selected=%v config=%v authority=%v parents=%v", plan.selectedTools, plan.configTools, plan.authority, plan.parentDirs)
	}
	if calls.registered != 1 || !slices.Equal(calls.lookup, []string{"pi", "zsh"}) || !slices.Equal(calls.describe, []string{"pi", "zsh"}) || calls.capture != 1 {
		t.Fatalf("service dependency calls = %+v", *calls)
	}
}

func TestInstallplanServiceWiringRejectsNeutralAndInvalidIntentWithoutAuthority(t *testing.T) {
	tests := []struct {
		name         string
		selection    []string
		observations []health.InstallationObservation
		wantError    string
	}{
		{name: "explicit empty", selection: []string{}, wantError: "explicit tool intent required"},
		{name: "unknown id", selection: []string{"not-registered"}, wantError: installationSnapshotUnavailable},
		{name: "noncanonical id", selection: []string{"Zsh"}, wantError: installationSnapshotUnavailable},
		{name: "missing observation", selection: []string{"zsh"}, wantError: installationSnapshotUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, _, base := newPlanTestApp(t)
			setManageTruthSnapshot(t, app, 142, pkg.PlatformMacOS, "brew", test.observations...)
			calls := &installplanWiringCalls{}
			runtime := installplanWiringRuntime(t, base, calls)
			plan, err := buildInstallPlanForTools(app, runtime, time.Now(), test.selection)
			if plan != nil || err == nil || err.Error() != test.wantError {
				t.Fatalf("fail-closed result = plan %v error %v", plan, err)
			}
			if len(calls.lookup) != 0 || len(calls.describe) != 0 || calls.capture != 0 {
				t.Fatalf("invalid/neutral intent acquired authority: %+v", *calls)
			}
		})
	}
}

func TestInstallplanServiceWiringExplicitEmptyPrecedesAppSnapshotReadiness(t *testing.T) {
	for _, test := range []struct {
		name string
		app  *App
	}{
		{name: "unready", app: &App{}},
		{name: "loading", app: &App{installationSnapshotLoading: true}},
		{name: "stale", app: &App{installationSnapshotStale: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := &installplanWiringCalls{}
			runtime := toolInstallRuntime{
				registeredToolIDs: func() []string { calls.registered++; return []string{"zsh"} },
				lookupTool: func(id string) (tools.Tool, bool) {
					calls.lookup = append(calls.lookup, id)
					return nil, false
				},
				describeInstall: func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
					calls.describe = append(calls.describe, tool.ID())
					return operation.InstallRecipe{}, nil
				},
				captureStatePlan: func() (*operation.StatePlan, error) { calls.capture++; return &operation.StatePlan{}, nil },
			}
			plan, err := buildInstallPlanForTools(test.app, runtime, time.Now(), []string{})
			if plan != nil || err == nil || err.Error() != "explicit tool intent required" {
				t.Fatalf("explicit-empty result = plan %v error %v", plan, err)
			}
			if calls.registered != 0 || len(calls.lookup) != 0 || len(calls.describe) != 0 || calls.capture != 0 {
				t.Fatalf("explicit-empty consulted dependencies: %+v", *calls)
			}
		})
	}
}

func TestInstallplanServiceWiringRejectsPresentUnknownAndRecipeDrift(t *testing.T) {
	tests := []struct {
		name     string
		presence health.Presence
		drift    bool
		want     string
	}{
		{name: "present only", presence: health.PresencePresent, want: "no installation changes required"},
		{name: "unknown health", presence: health.PresenceUnknown, want: installationSnapshotUnavailable},
		{name: "recipe drift", presence: health.PresenceMissing, drift: true, want: installationSnapshotUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, _, base := newPlanTestApp(t)
			observation := plannerSnapshotObservation(t, base, "zsh", test.presence, health.InstallabilitySupported)
			setManageTruthSnapshot(t, app, 143, pkg.PlatformMacOS, "brew", observation)
			calls := &installplanWiringCalls{}
			runtime := installplanWiringRuntime(t, base, calls)
			if test.drift {
				describe := runtime.describeInstall
				runtime.describeInstall = func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
					recipe, err := describe(tool, environment)
					recipe.Manager = "apt"
					return recipe, err
				}
			}
			plan, err := buildInstallPlanForTools(app, runtime, time.Now(), []string{"zsh"})
			if plan != nil || err == nil || err.Error() != test.want {
				t.Fatalf("fail-closed result = plan %v error %v", plan, err)
			}
			if calls.capture != 0 {
				t.Fatalf("neutral/blocked result captured state authority: %+v", *calls)
			}
		})
	}
}

func TestInstallplanServiceWiringPreservesReviewedPiAndT3Recipes(t *testing.T) {
	for _, id := range []string{"pi", "t3-code"} {
		t.Run(id, func(t *testing.T) {
			app, _, base := newPlanTestApp(t)
			setManageTruthSnapshot(t, app, 144, pkg.PlatformMacOS, "brew",
				manageTruthObservation(t, id, health.PresenceMissing),
			)
			calls := &installplanWiringCalls{}
			runtime := installplanWiringRuntime(t, base, calls)
			plan, err := buildInstallPlanForTools(app, runtime, time.Now(), []string{id})
			if err != nil {
				t.Fatal(err)
			}
			tool, _ := tools.NewRegistry().Get(id)
			want, describeErr := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
			if describeErr != nil {
				t.Fatal(describeErr)
			}
			actions := plan.actions()
			if len(actions) != 1 || actions[0].InstallRecipe == nil || !reflect.DeepEqual(*actions[0].InstallRecipe, want) {
				t.Fatalf("%s adopted recipe = %+v, want %+v", id, actions, want)
			}
		})
	}
}

func TestInstallplanServiceWiringMixedIntentUsesSnapshotAndIgnoresWizardState(t *testing.T) {
	app, _, base := newPlanTestApp(t)
	setManageTruthSnapshot(t, app, 146, pkg.PlatformMacOS, "brew",
		plannerSnapshotObservation(t, base, "git", health.PresencePresent, health.InstallabilitySupported),
		plannerSnapshotObservation(t, base, "zsh", health.PresenceMissing, health.InstallabilitySupported),
	)
	app.deepDiveConfig = nil
	calls := &installplanWiringCalls{}
	runtime := installplanWiringRuntime(t, base, calls)
	plan, err := buildInstallPlanForTools(app, runtime, time.Now(), []string{"zsh", "git"})
	if err != nil {
		t.Fatal(err)
	}
	if actions := plan.actions(); len(actions) != 1 || actions[0].ID != "install:zsh" {
		t.Fatalf("mixed intent actions = %+v", actions)
	}
	if presence, intent, ok := plan.installAuthority("git"); !ok || presence != health.PresencePresent || intent != "none" {
		t.Fatalf("present authority = (%s,%q,%v)", presence, intent, ok)
	}
	if !slices.Equal(calls.lookup, []string{"zsh"}) || !slices.Equal(calls.describe, []string{"zsh"}) || calls.capture != 1 {
		t.Fatalf("mixed dependency calls = %+v", *calls)
	}
}

func TestManageInstallClearsStalePendingPlanWhenNeutralPlannerBlocks(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "zsh", Installability: health.InstallabilitySupported, InstallRecipeDigest: strings.Repeat("a", 64),
		Package: health.PackageFacet{State: health.PackageMissing, Provider: "brew", ExpectedReceipts: []string{"zsh"}, MissingReceipts: []string{"zsh"}, Authoritative: true, Complete: true},
		Direct:  health.DirectFacet{State: health.ComponentNotApplicable},
	})
	if err != nil {
		t.Fatal(err)
	}
	setManageTruthSnapshot(t, app, 147, pkg.PlatformMacOS, "brew", observation)
	selectManageTruthItem(t, app, "zsh")
	app.pendingInstallPlan = &installPlan{}
	app.installPlanError = nil
	if cmd := NewManageScreen(ctx).handleKey(keyMsg("i")); cmd != nil {
		t.Fatal("blocked Manage install navigated to review")
	}
	if app.pendingInstallPlan != nil || app.installPlanError == nil || app.installPlanError.Error() != installationSnapshotUnavailable || app.manageStatus != installationSnapshotUnavailable {
		t.Fatalf("blocked Manage state = pending %v error %v status %q", app.pendingInstallPlan, app.installPlanError, app.manageStatus)
	}
}

func TestInstallplanServiceWiringBoundsDependencyFailureWithoutPlan(t *testing.T) {
	app, _, base := newPlanTestApp(t)
	setManageTruthSnapshot(t, app, 145, pkg.PlatformMacOS, "brew",
		plannerSnapshotObservation(t, base, "zsh", health.PresenceMissing, health.InstallabilitySupported),
	)
	calls := &installplanWiringCalls{}
	runtime := installplanWiringRuntime(t, base, calls)
	want := errors.New("describe failed: /Users/private/.config token=SECRET")
	runtime.describeInstall = func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
		return operation.InstallRecipe{}, want
	}
	plan, err := buildInstallPlanForTools(app, runtime, time.Now(), []string{"zsh"})
	if plan != nil || err == nil || err.Error() != installationSnapshotUnavailable || errors.Is(err, want) || calls.capture != 0 {
		t.Fatalf("dependency failure = plan %v error %v calls %+v", plan, err, *calls)
	}
}
