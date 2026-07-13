package ui

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	headless "github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestAdoptHeadlessInstallPlanPreservesPackageAuthorityWithoutConfigLeakage(t *testing.T) {
	recipe := adapterPackageRecipe("zsh")
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: "zsh", Installability: health.InstallabilitySupported, InstallRecipeDigest: digest,
		Package: health.PackageFacet{
			State: health.PackageMissing, Provider: "brew", ExpectedReceipts: []string{"zsh"}, MissingReceipts: []string{"zsh"}, Authoritative: true, Complete: true,
			Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: health.PackageMissing, ExpectedReceipts: []string{"zsh"}, MissingReceipts: []string{"zsh"}, Complete: true}},
		},
		Direct: health.DirectFacet{State: health.ComponentNotApplicable},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{
		Generation: 61, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: []health.InstallationObservation{observation},
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := planpublic.NormalizeExplicitTools([]string{"zsh"}, []string{"zsh"})
	if err != nil {
		t.Fatal(err)
	}
	statePlan := &operation.StatePlan{}
	result, err := headless.Build(headless.Request{
		Intent: intent, Snapshot: snapshot,
		Environment: headless.Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 61},
	}, headless.Dependencies{
		LookupTool: func(id string) (tools.Tool, bool) {
			if id != "zsh" {
				return nil, false
			}
			return tools.NewZshTool(), true
		},
		DescribeInstall: func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
			return operation.CloneInstallRecipe(recipe), nil
		},
		CaptureStatePlan: func() (*operation.StatePlan, error) { return statePlan, nil },
		Now:              func() time.Time { return time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := result.Accepted()
	if !ok {
		t.Fatal("neutral missing-Zsh plan omitted accepted authority")
	}

	app := &App{
		theme: "nord", navStyle: "vim", animationsEnabled: false,
		deepDiveConfig: NewDeepDiveConfig(),
	}
	// Seed unrelated installer defaults to prove the pure adapter consumes only
	// the accepted headless authority.
	app.deepDiveConfig.CLITools["codex"] = true
	app.deepDiveConfig.GUIApps["cursor"] = true
	plan, err := adoptHeadlessInstallPlan(app, accepted)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil {
		t.Fatal("adapter returned nil plan")
	}
	if plan.hash() != accepted.Hash() || !reflect.DeepEqual(plan.actions(), accepted.Operation().Actions()) {
		t.Fatalf("adapted document/hash drifted: hash=%q actions=%+v", plan.hash(), plan.actions())
	}
	authority := accepted.SnapshotAuthority()
	schema, generation, platform, manager, snapshotDigest := plan.installSnapshotAuthority()
	if schema != authority.SchemaVersion || generation != authority.Generation || platform != authority.Platform || manager != authority.Manager || snapshotDigest != authority.Digest {
		t.Fatalf("adapted snapshot=%d/%d/%s/%s/%s, want %+v", schema, generation, platform, manager, snapshotDigest, authority)
	}
	presence, installIntent, found := plan.installAuthority("zsh")
	wantTool, wantFound := accepted.ToolAuthority("zsh")
	if !found || !wantFound || presence != wantTool.Presence || installIntent != wantTool.Intent || plan.installTools["zsh"].recipeDigest != wantTool.RecipeDigest {
		t.Fatalf("adapted tool authority=%q/%q/%+v, want %+v", presence, installIntent, plan.installTools["zsh"], wantTool)
	}
	if got, ok := plan.installRecipes["zsh"]; !ok || !reflect.DeepEqual(got, accepted.Recipes()["zsh"]) {
		t.Fatalf("adapted recipe=%+v present=%v", got, ok)
	}
	if plan.statePlan != statePlan {
		t.Fatal("adapter replaced opaque StatePlan identity")
	}
	if !slices.Equal(plan.selectedToolIDs(), []string{"zsh"}) || plan.selectedTools != nil {
		t.Fatalf("adapted selected tools derived=%v legacy=%v", plan.selectedToolIDs(), plan.selectedTools)
	}
	execution, err := plan.installExecutionSnapshot()
	if err != nil || execution.platform != pkg.PlatformMacOS || execution.manager != "brew" || !reflect.DeepEqual(execution.recipes["zsh"], recipe) || execution.detected["zsh"] || execution.digests["zsh"] != digest {
		t.Fatalf("adapted execution snapshot=%+v err=%v", execution, err)
	}
	if plan.theme != "nord" || plan.navStyle != "vim" || plan.animations || !reflect.DeepEqual(plan.config, DeepDiveConfig{}) {
		t.Fatalf("execution compatibility/preferences leaked config: theme=%q nav=%q animations=%v config=%+v", plan.theme, plan.navStyle, plan.animations, plan.config)
	}
	if plan.configTools != nil || plan.globalConfig != nil || len(plan.authority) != 0 || plan.authority == nil || plan.parentDirs != nil || plan.yaziConfigPaths != (tools.YaziConfigPaths{}) || plan.ghosttyConfigTarget != "" || len(plan.backupTargets()) != 0 {
		t.Fatalf("adapter leaked config/target/backup authority: configTools=%v global=%v authority=%v parents=%v yazi=%+v ghostty=%q backups=%v", plan.configTools, plan.globalConfig, plan.authority, plan.parentDirs, plan.yaziConfigPaths, plan.ghosttyConfigTarget, plan.backupTargets())
	}
}

func TestAdoptHeadlessInstallPlanRejectsNilAndZeroWithoutMutationOrPanic(t *testing.T) {
	accepted, _ := adapterAcceptedFixture(t, false)
	for _, test := range []struct {
		name     string
		app      *App
		accepted headless.AcceptedPlan
	}{
		{name: "nil app", accepted: accepted},
		{name: "zero accepted", app: &App{theme: "nord", navStyle: "vim", deepDiveConfig: NewDeepDiveConfig()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var plan *installPlan
			var err error
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("adapter panicked: %v", recovered)
					}
				}()
				plan, err = adoptHeadlessInstallPlan(test.app, test.accepted)
			}()
			if err == nil || plan != nil {
				t.Fatalf("invalid adoption=(plan=%#v,err=%v), want nil/error", plan, err)
			}
		})
	}
}

func TestAdoptHeadlessInstallPlanIsReadOnlyAndDeeplyFrozen(t *testing.T) {
	accepted, statePlan := adapterAcceptedFixture(t, false)
	app := &App{theme: "nord", navStyle: "vim", animationsEnabled: true, deepDiveConfig: NewDeepDiveConfig()}
	app.deepDiveConfig.CLITools["codex"] = true
	beforeConfig := snapshotDeepDiveConfig(app.deepDiveConfig)
	beforeConfigPointer := app.deepDiveConfig
	beforeTheme, beforeNav, beforeAnimations := app.theme, app.navStyle, app.animationsEnabled

	// Mutating defensive accessors before adoption must not poison the accepted
	// authority that the adapter reads.
	accessorRecipes := accepted.Recipes()
	accessorRecipes["zsh"].Steps[0].Packages[0] = "pre-adoption-mutation"
	accessorAuthorities := accepted.ToolAuthorities()
	authority := accessorAuthorities["zsh"]
	authority.Intent = "pre-adoption-mutation"
	accessorAuthorities["zsh"] = authority

	plan, err := adoptHeadlessInstallPlan(app, accepted)
	if err != nil {
		t.Fatal(err)
	}
	if app.deepDiveConfig != beforeConfigPointer || !reflect.DeepEqual(snapshotDeepDiveConfig(app.deepDiveConfig), beforeConfig) || app.theme != beforeTheme || app.navStyle != beforeNav || app.animationsEnabled != beforeAnimations {
		t.Fatal("adapter mutated App preferences or deep-dive config")
	}
	if plan.statePlan != statePlan || plan.installRecipes["zsh"].Steps[0].Packages[0] != "zsh" || plan.installTools["zsh"].intent != "install" {
		t.Fatalf("pre-adoption accessor mutation contaminated plan: recipes=%+v tools=%+v", plan.installRecipes, plan.installTools)
	}
	frozenHash := plan.hash()
	frozenActions := plan.actions()

	app.theme, app.navStyle, app.animationsEnabled = "dracula", "emacs", false
	app.deepDiveConfig.CLITools["pi"] = true
	plan.installRecipes["zsh"].Steps[0].Packages[0] = "plan-mutation"
	plan.installRecipes["zsh"].Detector.Values[0] = "plan-detector-mutation"
	mutatedTool := plan.installTools["zsh"]
	mutatedTool.intent = "plan-authority-mutation"
	plan.installTools["zsh"] = mutatedTool

	if accepted.Hash() != frozenHash || !reflect.DeepEqual(accepted.Operation().Actions(), frozenActions) {
		t.Fatal("mutating App/adapted plan changed neutral Accepted authority")
	}
	if got := accepted.Recipes()["zsh"]; got.Steps[0].Packages[0] != "zsh" || got.Detector.Values[0] != "zsh" {
		t.Fatalf("adapted recipe mutation escaped into Accepted: %+v", got)
	}
	if got, _ := accepted.ToolAuthority("zsh"); got.Intent != "install" {
		t.Fatalf("adapted tool-authority mutation escaped into Accepted: %+v", got)
	}
	if plan.theme != "nord" || plan.navStyle != "vim" || !plan.animations {
		t.Fatalf("adapted preferences were not frozen: %q/%q/%v", plan.theme, plan.navStyle, plan.animations)
	}
}

func TestAdoptHeadlessMixedAuthorityKeepsPresentTruthButExecutesOnlyMissing(t *testing.T) {
	accepted, _ := adapterAcceptedFixture(t, true)
	app := &App{theme: "nord", navStyle: "vim", animationsEnabled: true, deepDiveConfig: NewDeepDiveConfig()}
	plan, err := adoptHeadlessInstallPlan(app, accepted)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.installTools) != 2 || plan.installTools["git"].presence != health.PresencePresent || plan.installTools["git"].intent != "none" || plan.installTools["zsh"].intent != "install" {
		t.Fatalf("mixed adapted tool authorities=%+v", plan.installTools)
	}
	if len(plan.actions()) != 1 || plan.actions()[0].ToolID != "zsh" || len(plan.installRecipes) != 1 || plan.installRecipes["zsh"].ToolID != "zsh" || !slices.Equal(plan.selectedToolIDs(), []string{"zsh"}) {
		t.Fatalf("mixed adapted execution scope actions=%+v recipes=%+v selected=%v", plan.actions(), plan.installRecipes, plan.selectedToolIDs())
	}
	execution, err := plan.installExecutionSnapshot()
	if err != nil || len(execution.recipes) != 1 || execution.recipes["zsh"].ToolID != "zsh" {
		t.Fatalf("mixed execution snapshot=%+v err=%v", execution, err)
	}
}

func adapterPackageRecipe(id string) operation.InstallRecipe {
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        id, Platform: string(pkg.PlatformMacOS), Manager: "brew",
		Steps:    []operation.InstallStep{{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{id}}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{id}},
		Risk:     "installs packages from the configured system package manager",
	}
}

func adapterAcceptedFixture(t *testing.T, mixed bool) (headless.AcceptedPlan, *operation.StatePlan) {
	t.Helper()
	zshRecipe := adapterPackageRecipe("zsh")
	observations := []health.InstallationObservation{adapterObservation(t, "zsh", health.PackageMissing, zshRecipe)}
	ids := []string{"zsh"}
	if mixed {
		gitRecipe := adapterPackageRecipe("git")
		observations = append(observations, adapterObservation(t, "git", health.PackagePresent, gitRecipe))
		ids = append(ids, "git")
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 71, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: observations})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := planpublic.NormalizeExplicitTools(ids, []string{"git", "zsh"})
	if err != nil {
		t.Fatal(err)
	}
	statePlan := &operation.StatePlan{}
	result, err := headless.Build(headless.Request{Intent: intent, Snapshot: snapshot, Environment: headless.Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 71}}, headless.Dependencies{
		LookupTool: func(id string) (tools.Tool, bool) {
			if id == "zsh" {
				return tools.NewZshTool(), true
			}
			return tools.NewGitTool(), id == "git"
		},
		DescribeInstall: func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
			return adapterPackageRecipe(tool.ID()), nil
		},
		CaptureStatePlan: func() (*operation.StatePlan, error) { return statePlan, nil },
		Now:              func() time.Time { return time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := result.Accepted()
	if !ok {
		t.Fatal("fixture omitted Accepted authority")
	}
	return accepted, statePlan
}

func adapterObservation(t *testing.T, id string, state health.PackageState, recipe operation.InstallRecipe) health.InstallationObservation {
	t.Helper()
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	facet := health.PackageFacet{State: state, Provider: "brew", ExpectedReceipts: []string{id}, Authoritative: true, Complete: true, Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: state, ExpectedReceipts: []string{id}, Complete: true}}}
	if state == health.PackagePresent {
		facet.ObservedReceipts = []string{id}
		facet.Namespaces[0].ObservedReceipts = []string{id}
	} else {
		facet.MissingReceipts = []string{id}
		facet.Namespaces[0].MissingReceipts = []string{id}
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{ToolID: id, Installability: health.InstallabilitySupported, InstallRecipeDigest: digest, Package: facet, Direct: health.DirectFacet{State: health.ComponentNotApplicable}})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
