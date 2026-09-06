package ui

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type installPlanAuthorityProjection interface {
	installAuthority(toolID string) (health.Presence, string, bool)
}

type installPlanSnapshotAuthorityProjection interface {
	installSnapshotAuthority() (int, uint64, string, string, string)
	installRecipeAuthority(toolID string) (string, bool)
}

type mutableInstallRecipeTool struct {
	tools.Tool
	recipe operation.InstallRecipe
}

func (t *mutableInstallRecipeTool) InstallRecipe(environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
	return operation.CloneInstallRecipe(t.recipe), nil
}

func plannerSupportedObservation(t *testing.T, runtime toolInstallRuntime, id string, packageFacet health.PackageFacet, directFacet health.DirectFacet) health.InstallationObservation {
	t.Helper()
	tool, ok := runtime.lookupTool(id)
	if !ok {
		t.Fatalf("registry tool %q not found", id)
	}
	recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatalf("describe %s install: %v", id, err)
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID:              id,
		Installability:      health.InstallabilitySupported,
		InstallRecipeDigest: installRecipeDigest(recipe),
		Package:             packageFacet,
		Direct:              directFacet,
	})
	if err != nil {
		t.Fatalf("build %s custom observation: %v", id, err)
	}
	return observation
}

func plannerDisagreementPartialObservation(t *testing.T, runtime toolInstallRuntime, id string) health.InstallationObservation {
	t.Helper()
	tool, ok := runtime.lookupTool(id)
	if !ok {
		t.Fatalf("registry tool %q not found", id)
	}
	receipts := slices.Clone(tools.PackagesForPlatform(tool.Packages(), pkg.PlatformMacOS))
	sort.Strings(receipts)
	if len(receipts) == 0 {
		t.Fatalf("registry tool %q has no macOS receipts", id)
	}
	return plannerSupportedObservation(t, runtime, id,
		health.PackageFacet{State: health.PackagePresent, Provider: "brew", ExpectedReceipts: receipts, ObservedReceipts: slices.Clone(receipts), Authoritative: true, Complete: true},
		health.DirectFacet{State: health.ComponentMissing, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{id}, State: health.ComponentMissing}}},
	)
}

func plannerReceiptPositivePartialObservation(t *testing.T, runtime toolInstallRuntime, id string) health.InstallationObservation {
	t.Helper()
	tool, ok := runtime.lookupTool(id)
	if !ok {
		t.Fatalf("registry tool %q not found", id)
	}
	receipts := slices.Clone(tools.PackagesForPlatform(tool.Packages(), pkg.PlatformMacOS))
	sort.Strings(receipts)
	if len(receipts) == 0 {
		t.Fatalf("registry tool %q has no macOS receipts", id)
	}
	return plannerSupportedObservation(t, runtime, id,
		health.PackageFacet{State: health.PackagePresent, Provider: "brew", ExpectedReceipts: receipts, ObservedReceipts: slices.Clone(receipts), Authoritative: false, Complete: true},
		health.DirectFacet{State: health.ComponentNotApplicable},
	)
}

func plannerSnapshotObservation(t *testing.T, runtime toolInstallRuntime, id string, presence health.Presence, installability health.Installability) health.InstallationObservation {
	t.Helper()
	tool, ok := runtime.lookupTool(id)
	if !ok {
		t.Fatalf("registry tool %q not found", id)
	}
	receipts := slices.Clone(tools.PackagesForPlatform(tool.Packages(), pkg.PlatformMacOS))
	sort.Strings(receipts)
	if len(receipts) == 0 {
		t.Fatalf("registry tool %q has no macOS receipts", id)
	}
	spec := health.InstallationObservationSpec{
		ToolID:         id,
		Installability: installability,
		Direct:         health.DirectFacet{State: health.ComponentNotApplicable},
	}
	if installability == health.InstallabilitySupported {
		recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
		if err != nil {
			t.Fatalf("describe %s install: %v", id, err)
		}
		detectorReceipts := slices.Clone(recipe.Detector.Values)
		sort.Strings(detectorReceipts)
		if recipe.Detector.Kind != operation.InstallDetectorPackageReceipt || !slices.Equal(detectorReceipts, receipts) {
			t.Fatalf("%s recipe detector = (%s,%v), want package receipts %v", id, recipe.Detector.Kind, detectorReceipts, receipts)
		}
		spec.InstallRecipeDigest = installRecipeDigest(recipe)
	}
	switch presence {
	case health.PresencePresent:
		spec.Package = health.PackageFacet{State: health.PackagePresent, Provider: "brew", ExpectedReceipts: receipts, ObservedReceipts: slices.Clone(receipts), Authoritative: true, Complete: true}
	case health.PresencePartial:
		if len(receipts) < 2 {
			t.Fatalf("registry tool %q needs at least two receipts for a partial fixture", id)
		}
		spec.Package = health.PackageFacet{State: health.PackagePartial, Provider: "brew", ExpectedReceipts: receipts, ObservedReceipts: slices.Clone(receipts[:len(receipts)-1]), MissingReceipts: slices.Clone(receipts[len(receipts)-1:]), Authoritative: true, Complete: true}
	case health.PresenceMissing:
		spec.Package = health.PackageFacet{State: health.PackageMissing, Provider: "brew", ExpectedReceipts: receipts, MissingReceipts: slices.Clone(receipts), Authoritative: true, Complete: true}
	case health.PresenceUnknown:
		spec.Package = health.PackageFacet{State: health.PackageUnknown, Provider: "brew", ExpectedReceipts: receipts, UnresolvedReceipts: slices.Clone(receipts), Authoritative: true, Complete: false}
	default:
		t.Fatalf("unsupported presence %q", presence)
	}
	observation, err := health.NewInstallationObservation(spec)
	if err != nil {
		t.Fatalf("build %s observation: %v", id, err)
	}
	return observation
}

func TestInstallSnapshotPlannerDecisionMatrix(t *testing.T) {
	const targetID = "zsh"
	tests := []struct {
		name           string
		presence       health.Presence
		installability health.Installability
		omit           bool
		legacyValue    bool
		wantApply      bool
		wantRepair     bool
		wantDetected   bool
		wantIntent     string
		wantErr        bool
		wantFiltered   bool
		partialFixture func(*testing.T, toolInstallRuntime, string) health.InstallationObservation
	}{
		{name: "present skips install", presence: health.PresencePresent, installability: health.InstallabilitySupported, legacyValue: false, wantIntent: "none"},
		{name: "present and unsupported still skips install", presence: health.PresencePresent, installability: health.InstallabilityUnsupported, legacyValue: false, wantIntent: "none"},
		{name: "present and unknown installability still skips install", presence: health.PresencePresent, installability: health.InstallabilityUnknown, legacyValue: false, wantIntent: "none"},
		{name: "missing and supported applies", presence: health.PresenceMissing, installability: health.InstallabilitySupported, legacyValue: true, wantApply: true, wantIntent: "install"},
		{name: "receipt partial and supported repairs", presence: health.PresencePartial, installability: health.InstallabilitySupported, legacyValue: true, wantApply: true, wantRepair: true, wantIntent: "repair"},
		{name: "authoritative package and direct disagreement repairs", presence: health.PresencePartial, installability: health.InstallabilitySupported, legacyValue: true, wantApply: true, wantRepair: true, wantIntent: "repair", wantDetected: true, partialFixture: plannerDisagreementPartialObservation},
		{name: "nonauthoritative receipt positive evidence is unresolved", presence: health.PresencePartial, installability: health.InstallabilitySupported, legacyValue: true, wantErr: true, partialFixture: plannerReceiptPositivePartialObservation},
		{name: "unknown is filtered without install actions", presence: health.PresenceUnknown, installability: health.InstallabilitySupported, legacyValue: false, wantFiltered: true},
		{name: "missing and unsupported is filtered without actions", presence: health.PresenceMissing, installability: health.InstallabilityUnsupported, legacyValue: false, wantFiltered: true},
		{name: "missing and unknown installability is filtered without actions", presence: health.PresenceMissing, installability: health.InstallabilityUnknown, legacyValue: false, wantFiltered: true},
		{name: "partial and unsupported is filtered without actions", presence: health.PresencePartial, installability: health.InstallabilityUnsupported, legacyValue: false, wantFiltered: true},
		{name: "partial and unknown installability is filtered without actions", presence: health.PresencePartial, installability: health.InstallabilityUnknown, legacyValue: false, wantFiltered: true},
		{name: "absent observation is filtered without actions", omit: true, legacyValue: false, wantFiltered: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, runtime := newPlanTestApp(t)
			app.deepDiveConfig.CLITools = map[string]bool{}
			app.deepDiveConfig.GUIApps = map[string]bool{}
			app.deepDiveConfig.CLIUtilities = map[string]bool{}
			app.deepDiveConfig.MacApps = map[string]bool{}
			app.manageInstalled = make(map[string]bool, len(alwaysConfiguredToolIDs))
			observations := make([]health.InstallationObservation, 0, len(alwaysConfiguredToolIDs))
			for _, id := range alwaysConfiguredToolIDs {
				app.manageInstalled[id] = true
				if id == targetID {
					app.manageInstalled[id] = tt.legacyValue
					if tt.omit {
						continue
					}
					if tt.partialFixture != nil {
						observation := tt.partialFixture(t, runtime, id)
						if observation.Presence() != tt.presence {
							t.Fatalf("custom observation presence = %s, want %s", observation.Presence(), tt.presence)
						}
						observations = append(observations, observation)
					} else {
						observations = append(observations, plannerSnapshotObservation(t, runtime, id, tt.presence, tt.installability))
					}
					continue
				}
				observations = append(observations, plannerSnapshotObservation(t, runtime, id, health.PresencePresent, health.InstallabilitySupported))
			}
			snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: observations})
			if err != nil {
				t.Fatal(err)
			}
			app.installationSnapshotGeneration = 1
			app.installationSnapshotTerminal = true
			app.installationSnapshot = snapshot
			app.installationSnapshotManagerIdentity = stableUIManagerIdentity()
			app.installationSnapshotReady = true
			app.installationSnapshotLoading = false
			app.installationSnapshotStale = false
			app.installationSnapshotError = ""

			plan, planErr := buildInstallPlan(app, runtime, time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC))
			if tt.wantErr {
				if planErr == nil || plan != nil {
					actionCount := 0
					if plan != nil {
						actionCount = len(plan.actions())
					}
					t.Fatalf("plan nonnil=%v, action count=%d, error=%v; want nil plan and error", plan != nil, actionCount, planErr)
				}
				return
			}
			if planErr != nil {
				t.Fatal(planErr)
			}
			var targetActions []operation.Action
			for _, action := range plan.actions() {
				if action.Kind == operation.KindInstallTool && action.ToolID == targetID {
					targetActions = append(targetActions, action)
				}
			}
			if tt.wantFiltered {
				if len(targetActions) != 0 {
					t.Fatalf("filtered tool produced install actions: %+v", targetActions)
				}
				if _, _, found := plan.installAuthority(targetID); found {
					t.Fatal("filtered tool retained install authority")
				}
				return
			}
			projection, ok := any(plan).(installPlanAuthorityProjection)
			if !ok {
				t.Fatal("install plan exposes no typed installation authority projection")
			}
			if gotPresence, gotIntent, found := projection.installAuthority(targetID); !found || gotPresence != tt.presence || gotIntent != tt.wantIntent {
				t.Fatalf("install authority = (%s,%q,%v), want (%s,%q,true)", gotPresence, gotIntent, found, tt.presence, tt.wantIntent)
			}
			if !tt.wantApply {
				if len(targetActions) != 0 {
					t.Fatalf("present tool produced install actions: %+v", targetActions)
				}
				return
			}
			if len(targetActions) != 1 || targetActions[0].Disposition != operation.DispositionApply {
				t.Fatalf("install actions = %+v, want one Apply action", targetActions)
			}
			if targetActions[0].InstallDetected == nil || *targetActions[0].InstallDetected != tt.wantDetected {
				t.Fatalf("accepted detector=%v, want %v", targetActions[0].InstallDetected, tt.wantDetected)
			}
			if tt.wantRepair && !strings.Contains(strings.ToLower(targetActions[0].Description), "repair") {
				t.Fatalf("partial install action description = %q, want explicit repair semantics", targetActions[0].Description)
			}
			tool, _ := runtime.lookupTool(targetID)
			recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := targetActions[0].DesiredDigest, installRecipeDigest(recipe); got != want {
				t.Fatalf("install action recipe digest = %q, want %q", got, want)
			}
		})
	}
}

type installPlannerProbeCounts struct {
	lookup, platform, manager, runtimeInstalled, toolInstalled, packages, direct int
}

type installPlannerProbeTool struct {
	tools.Tool
	counts *installPlannerProbeCounts
}

func (t installPlannerProbeTool) Packages() map[pkg.Platform][]string {
	t.counts.packages++
	return t.Tool.Packages()
}

func (t installPlannerProbeTool) IsInstalled() bool {
	t.counts.toolInstalled++
	return t.Tool.IsInstalled()
}

func (t installPlannerProbeTool) IsInstalledOutsidePackageManager(observation tools.DirectInstallationObservation) bool {
	t.counts.direct++
	detector, ok := t.Tool.(tools.DirectInstallationDetector)
	return ok && detector.IsInstalledOutsidePackageManager(observation)
}

func TestInstallSnapshotPlannerRequiresLatestBoundEnvironment(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*App)
	}{
		{name: "loading", mutate: func(app *App) { app.installationSnapshotLoading = true; app.installationSnapshotReady = false }},
		{name: "stale", mutate: func(app *App) { app.installationSnapshotStale = true; app.installationSnapshotReady = false }},
		{name: "generic error", mutate: func(app *App) {
			app.installationSnapshotError = "installation status unavailable"
			app.installationSnapshotReady = false
		}},
		{name: "not ready", mutate: func(app *App) { app.installationSnapshotReady = false }},
		{name: "generation mismatch", mutate: func(app *App) { app.installationSnapshotGeneration++ }},
		{name: "zero snapshot", mutate: func(app *App) { app.installationSnapshot = health.InstallationSnapshot{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, fixtureRuntime := newPlanTestApp(t)
			app.deepDiveConfig.CLITools = map[string]bool{}
			app.deepDiveConfig.GUIApps = map[string]bool{}
			app.deepDiveConfig.CLIUtilities = map[string]bool{}
			app.deepDiveConfig.MacApps = map[string]bool{}
			observations := make([]health.InstallationObservation, 0, len(alwaysConfiguredToolIDs))
			for _, id := range alwaysConfiguredToolIDs {
				observations = append(observations, plannerSnapshotObservation(t, fixtureRuntime, id, health.PresencePresent, health.InstallabilitySupported))
			}
			snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: observations})
			if err != nil {
				t.Fatal(err)
			}
			app.installationSnapshotGeneration = 1
			app.installationSnapshotTerminal = true
			app.installationSnapshot = snapshot
			app.installationSnapshotManagerIdentity = stableUIManagerIdentity()
			app.installationSnapshotReady = true
			app.installationSnapshotLoading = false
			app.installationSnapshotStale = false
			app.installationSnapshotError = ""
			tt.mutate(app)

			counts := &installPlannerProbeCounts{}
			registry := tools.NewRegistry()
			probeRuntime := registryRuntime(pkg.PlatformArch, map[string]bool{})
			probeRuntime.lookupTool = func(id string) (tools.Tool, bool) {
				counts.lookup++
				tool, ok := registry.Get(id)
				if !ok {
					return nil, false
				}
				return installPlannerProbeTool{Tool: tool, counts: counts}, true
			}
			probeRuntime.detectPlatform = func() pkg.Platform { counts.platform++; return pkg.PlatformArch }
			probeRuntime.detectManager = func() pkg.PackageManager { counts.manager++; return pkg.NewMockPackageManager() }
			probeRuntime.isToolInstalled = func(tool tools.Tool) bool { counts.runtimeInstalled++; return tool.IsInstalled() }

			plan, planErr := buildInstallPlan(app, probeRuntime, time.Date(2026, 7, 12, 12, 30, 0, 0, time.UTC))
			if plan != nil || planErr == nil {
				t.Fatalf("plan nonnil=%v error=%v, want nil plan and error", plan != nil, planErr)
			}
			if *counts != (installPlannerProbeCounts{}) {
				t.Fatalf("invalid snapshot reached legacy/runtime/tool probes: %+v", *counts)
			}
		})
	}
}

func TestInstallSnapshotPlannerRejectsRecipeDriftAndPinsIdentity(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	app.deepDiveConfig.CLITools = map[string]bool{}
	app.deepDiveConfig.GUIApps = map[string]bool{}
	app.deepDiveConfig.CLIUtilities = map[string]bool{}
	app.deepDiveConfig.MacApps = map[string]bool{}
	registry := tools.NewRegistry()
	realZsh, ok := registry.Get("zsh")
	if !ok {
		t.Fatal("zsh missing from registry")
	}
	recipeA, err := tools.DescribeInstall(realZsh, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	recipeB := operation.CloneInstallRecipe(recipeA)
	recipeB.Risk += "; changed after observation"
	if digestA, digestB := installRecipeDigest(recipeA), installRecipeDigest(recipeB); digestA == "" || digestA == digestB {
		t.Fatalf("mutable recipes do not have distinct valid digests: %q %q", digestA, digestB)
	}
	mutableZsh := &mutableInstallRecipeTool{Tool: realZsh, recipe: recipeA}
	runtime.lookupTool = func(id string) (tools.Tool, bool) {
		if id == "zsh" {
			return mutableZsh, true
		}
		return registry.Get(id)
	}
	app.manageInstalled = make(map[string]bool, len(alwaysConfiguredToolIDs))
	observations := make([]health.InstallationObservation, 0, len(alwaysConfiguredToolIDs))
	for _, id := range alwaysConfiguredToolIDs {
		app.manageInstalled[id] = true
		presence := health.PresencePresent
		if id == "zsh" {
			app.manageInstalled[id] = false
			presence = health.PresenceMissing
		}
		observations = append(observations, plannerSnapshotObservation(t, runtime, id, presence, health.InstallabilitySupported))
	}
	setSnapshot := func(generation uint64) health.InstallationSnapshot {
		t.Helper()
		snapshot, snapshotErr := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: generation, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: observations})
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		app.installationSnapshotGeneration = generation
		app.installationSnapshotTerminal = true
		app.installationSnapshot = snapshot
		app.installationSnapshotManagerIdentity = stableUIManagerIdentity()
		app.installationSnapshotReady = true
		app.installationSnapshotLoading = false
		app.installationSnapshotStale = false
		app.installationSnapshotError = ""
		return snapshot
	}
	snapshotA := setSnapshot(1)
	now := time.Date(2026, 7, 12, 13, 0, 0, 0, time.UTC)

	mutableZsh.recipe = recipeB
	drifted, driftErr := buildInstallPlan(app, runtime, now)
	if driftErr == nil || drifted != nil {
		actionCount := 0
		if drifted != nil {
			actionCount = len(drifted.actions())
		}
		t.Fatalf("recipe drift plan nonnil=%v actions=%d error=%v, want nil plan and error", drifted != nil, actionCount, driftErr)
	}

	mutableZsh.recipe = recipeA
	accepted, err := buildInstallPlan(app, runtime, now)
	if err != nil || accepted == nil {
		t.Fatalf("matching recipe plan nonnil=%v error=%v", accepted != nil, err)
	}
	projection, ok := any(accepted).(installPlanSnapshotAuthorityProjection)
	if !ok {
		t.Fatal("install plan exposes no typed snapshot/recipe authority projection")
	}
	schema, generation, platform, manager, snapshotDigest := projection.installSnapshotAuthority()
	if schema != snapshotA.SchemaVersion() || generation != snapshotA.Generation() || platform != snapshotA.Platform() || manager != snapshotA.Manager() || snapshotDigest != snapshotA.Digest() {
		t.Fatalf("snapshot authority=(%d,%d,%q,%q,%q), want snapshot identity", schema, generation, platform, manager, snapshotDigest)
	}
	wantRecipeDigest := installRecipeDigest(recipeA)
	if got, found := projection.installRecipeAuthority("zsh"); !found || got != wantRecipeDigest {
		t.Fatalf("zsh recipe authority=(%q,%v), want (%q,true)", got, found, wantRecipeDigest)
	}
	originalHash := accepted.hash()
	originalActions := accepted.actions()
	originalAuthority := []any{schema, generation, platform, manager, snapshotDigest, wantRecipeDigest}

	snapshotB := setSnapshot(2)
	schema, generation, platform, manager, snapshotDigest = projection.installSnapshotAuthority()
	if got := []any{schema, generation, platform, manager, snapshotDigest, wantRecipeDigest}; !reflect.DeepEqual(got, originalAuthority) || accepted.hash() != originalHash || !reflect.DeepEqual(accepted.actions(), originalActions) {
		t.Fatal("published plan changed after App installation snapshot replacement")
	}
	if snapshotB.Digest() == snapshotA.Digest() {
		t.Fatal("generation change did not change snapshot digest")
	}
	second, err := buildInstallPlan(app, runtime, now)
	if err != nil || second == nil {
		t.Fatalf("generation-two matching plan nonnil=%v error=%v", second != nil, err)
	}
	if second.hash() == originalHash {
		t.Fatal("plan hash does not bind accepted installation snapshot generation/digest")
	}
}

type snapshotEnvironmentRecipeTool struct {
	tools.Tool
	recipe operation.InstallRecipe
	counts *installPlannerProbeCounts
}

func (t snapshotEnvironmentRecipeTool) Packages() map[pkg.Platform][]string {
	t.counts.packages++
	return t.Tool.Packages()
}

func (t snapshotEnvironmentRecipeTool) IsInstalled() bool {
	t.counts.toolInstalled++
	return t.Tool.IsInstalled()
}

func (t snapshotEnvironmentRecipeTool) IsInstalledOutsidePackageManager(observation tools.DirectInstallationObservation) bool {
	t.counts.direct++
	detector, ok := t.Tool.(tools.DirectInstallationDetector)
	return ok && detector.IsInstalledOutsidePackageManager(observation)
}

func (t snapshotEnvironmentRecipeTool) InstallRecipe(environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
	return operation.CloneInstallRecipe(t.recipe), nil
}

func TestInstallSnapshotPlannerUsesReadySnapshotEnvironmentWithoutLiveProbes(t *testing.T) {
	app, _, fixtureRuntime := newPlanTestApp(t)
	app.deepDiveConfig.CLITools = map[string]bool{}
	app.deepDiveConfig.GUIApps = map[string]bool{}
	app.deepDiveConfig.CLIUtilities = map[string]bool{}
	app.deepDiveConfig.MacApps = map[string]bool{}
	registry := tools.NewRegistry()
	observations := make([]health.InstallationObservation, 0, len(alwaysConfiguredToolIDs))
	for _, id := range alwaysConfiguredToolIDs {
		presence := health.PresencePresent
		if id == "zsh" {
			presence = health.PresenceMissing
		}
		observations = append(observations, plannerSnapshotObservation(t, fixtureRuntime, id, presence, health.InstallabilitySupported))
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 41, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: observations})
	if err != nil {
		t.Fatal(err)
	}
	app.installationSnapshotGeneration = snapshot.Generation()
	app.installationSnapshotTerminal = true
	app.installationSnapshot = snapshot
	app.installationSnapshotManagerIdentity = stableUIManagerIdentity()
	app.installationSnapshotReady = true
	app.installationSnapshotLoading = false
	app.installationSnapshotStale = false
	app.installationSnapshotError = ""
	app.manageInstalled = map[string]bool{"zsh": true}

	counts := &installPlannerProbeCounts{}
	recipes := make(map[string]operation.InstallRecipe, len(alwaysConfiguredToolIDs))
	for _, id := range alwaysConfiguredToolIDs {
		tool, ok := registry.Get(id)
		if !ok {
			t.Fatalf("registry tool %q missing", id)
		}
		recipe, recipeErr := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
		if recipeErr != nil {
			t.Fatalf("describe %s: %v", id, recipeErr)
		}
		recipes[id] = recipe
	}
	probeRuntime := registryRuntime(pkg.PlatformArch, map[string]bool{})
	probeRuntime.lookupTool = func(id string) (tools.Tool, bool) {
		counts.lookup++
		tool, ok := registry.Get(id)
		if !ok {
			return nil, false
		}
		return snapshotEnvironmentRecipeTool{Tool: tool, recipe: recipes[id], counts: counts}, true
	}
	probeRuntime.detectPlatform = func() pkg.Platform { counts.platform++; return pkg.PlatformArch }
	probeRuntime.detectManager = func() pkg.PackageManager {
		counts.manager++
		manager := pkg.NewMockPackageManager()
		manager.ManagerName = "apt"
		return manager
	}
	probeRuntime.isToolInstalled = func(tool tools.Tool) bool { counts.runtimeInstalled++; return tool.IsInstalled() }

	plan, err := buildInstallPlan(app, probeRuntime, time.Date(2026, 7, 12, 14, 0, 0, 0, time.UTC))
	if err != nil || plan == nil {
		t.Fatalf("ready snapshot plan nonnil=%v error=%v", plan != nil, err)
	}
	if counts.platform != 0 || counts.manager != 0 || counts.runtimeInstalled != 0 || counts.toolInstalled != 0 || counts.direct != 0 {
		t.Fatalf("ready snapshot planning performed live probes: %+v", *counts)
	}
	authority, ok := any(plan).(installPlanSnapshotAuthorityProjection)
	if !ok {
		t.Fatal("plan exposes no typed snapshot authority")
	}
	schema, generation, platform, manager, digest := authority.installSnapshotAuthority()
	if schema != snapshot.SchemaVersion() || generation != snapshot.Generation() || platform != snapshot.Platform() || manager != snapshot.Manager() || digest != snapshot.Digest() {
		t.Fatalf("snapshot authority=(%d,%d,%q,%q,%q), want exact ready snapshot", schema, generation, platform, manager, digest)
	}
	zshRecipe := recipes["zsh"]
	if zshRecipe.Platform != string(pkg.PlatformMacOS) || zshRecipe.Manager != "brew" {
		t.Fatalf("zsh recipe environment=(%q,%q), want macos/brew", zshRecipe.Platform, zshRecipe.Manager)
	}
	wantDigest := installRecipeDigest(zshRecipe)
	if got, found := authority.installRecipeAuthority("zsh"); !found || got != wantDigest {
		t.Fatalf("zsh recipe authority=(%q,%v), want (%q,true)", got, found, wantDigest)
	}
	var actions []operation.Action
	for _, action := range plan.actions() {
		if action.Kind == operation.KindInstallTool && action.ToolID == "zsh" {
			actions = append(actions, action)
		}
	}
	if len(actions) != 1 || actions[0].Disposition != operation.DispositionApply || actions[0].DesiredDigest != wantDigest || actions[0].InstallRecipe == nil || actions[0].InstallRecipe.Platform != string(pkg.PlatformMacOS) || actions[0].InstallRecipe.Manager != "brew" {
		t.Fatalf("zsh install actions=%+v, want one pinned macos/brew Apply action", actions)
	}
}
