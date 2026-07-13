package installplan

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

type dependencyCounts struct {
	lookup, describe, capture, clock int
	sequence                         []string
	statePlan                        *operation.StatePlan
}

func TestBuildEmptyOrInvalidIntentCallsNoDependencies(t *testing.T) {
	for _, test := range []struct {
		name    string
		intent  planpublic.Intent
		wantErr error
		status  string
	}{
		{name: "empty", intent: planpublic.Intent{Source: "explicit_tools", Tools: []string{}}, status: `"status":"intent_required"`},
		{name: "invalid digest", intent: planpublic.Intent{Source: "explicit_tools", Tools: []string{"git"}, Digest: strings.Repeat("f", 64)}, wantErr: planpublic.ErrInvalidIntent},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := &dependencyCounts{}
			result, err := Build(Request{Intent: test.intent}, countingDependencies(counts, operation.InstallRecipe{}))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Build error=%v, want %v", err, test.wantErr)
			}
			if counts.lookup != 0 || counts.describe != 0 || counts.capture != 0 || counts.clock != 0 || len(counts.sequence) != 0 || counts.statePlan != nil {
				t.Fatalf("dependencies called for invalid intent: %+v", *counts)
			}
			if test.status != "" {
				encoded, marshalErr := planpublic.MarshalDocument(result.Public())
				if marshalErr != nil || !strings.Contains(string(encoded), test.status) {
					t.Fatalf("public outcome=%s err=%v, want %s", encoded, marshalErr, test.status)
				}
			}
		})
	}
}

func TestBuildPresentOnlyReturnsNoChangesWithoutCaptureOrClock(t *testing.T) {
	intent := mustIntent(t, "git")
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 7, mustObservation(t, "git", health.PackagePresent, recipe))
	counts := &dependencyCounts{}

	result, err := Build(Request{
		Intent: intent, Snapshot: snapshot,
		Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 7},
	}, countingDependencies(counts, recipe))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := planpublic.MarshalDocument(result.Public())
	if err != nil || !strings.Contains(string(encoded), `"status":"no_changes"`) {
		t.Fatalf("public outcome=%s err=%v", encoded, err)
	}
	if _, ok := result.Accepted(); ok {
		t.Fatal("present-only result exposed private mutation authority")
	}
	if counts.lookup != 0 || counts.describe != 0 || counts.capture != 0 || counts.clock != 0 {
		t.Fatalf("present-only dependency calls=%+v, want all zero", *counts)
	}
}

func TestBuildOneMissingReviewedToolReturnsReadyPrivateAndPublicPlan(t *testing.T) {
	intent := mustIntent(t, "git")
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 9, mustObservation(t, "git", health.PackageMissing, recipe))
	counts := &dependencyCounts{}

	result, err := Build(Request{
		Intent: intent, Snapshot: snapshot,
		Environment: managerTestEnvironment(t, pkg.PlatformMacOS, "brew", 9),
	}, countingDependencies(counts, recipe))
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := result.Accepted()
	if !ok {
		t.Fatal("ready result omitted private accepted plan")
	}
	actions := accepted.Operation().Actions()
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 {
		t.Fatalf("private actions=%+v", actions)
	}
	action := actions[0]
	if action.ID != "install:git" || action.Kind != operation.KindInstallTool || action.ToolID != "git" || action.Target != "git" || action.Description != "install git" ||
		action.Disposition != operation.DispositionApply || action.Reason != "" || action.DesiredDigest != digest || action.Ownership != operation.OwnershipPackageManager ||
		action.Reversibility != operation.ReversibilityManual || action.InstallRecipe == nil || action.InstallDetected == nil || *action.InstallDetected {
		t.Fatalf("private action contract=%+v", action)
	}
	if !reflect.DeepEqual(*action.InstallRecipe, recipe) {
		t.Fatalf("private recipe=%+v, want %+v", *action.InstallRecipe, recipe)
	}
	if accepted.StatePlan() != counts.statePlan {
		t.Fatal("Accepted did not preserve the opaque captured state-plan identity")
	}
	wantAuthority := SnapshotAuthority{
		SchemaVersion: snapshot.SchemaVersion(), Generation: snapshot.Generation(), Platform: snapshot.Platform(),
		Manager: snapshot.Manager(), Digest: snapshot.Digest(),
	}
	if got := accepted.SnapshotAuthority(); got != wantAuthority {
		t.Fatalf("snapshot authority=%+v, want %+v", got, wantAuthority)
	}
	if len(accepted.Hash()) != 64 {
		t.Fatalf("private accepted hash=%q", accepted.Hash())
	}
	publicActions := result.Public().Actions()
	if len(publicActions) != 1 || publicActions[0].ActionID != "install:git" || publicActions[0].Install == nil {
		t.Fatalf("public actions=%+v", publicActions)
	}
	public := publicActions[0]
	if public.Observation == nil || public.Observation.Exists || public.Observation.Managed || public.Install.RecipeDigest != digest ||
		public.Install.Authentication != "none" || public.Install.Risk != "package_manager_install" ||
		public.Install.Detector.Kind != "package_receipt" || !reflect.DeepEqual(public.Install.Detector.Values, []string{"git"}) || len(public.Install.Steps) != 1 {
		t.Fatalf("public install projection=%+v", public)
	}
	step := public.Install.Steps[0]
	if step.Kind != "package_manager" || step.Provider != "brew" || !reflect.DeepEqual(step.Packages, []string{"git"}) || len(step.Casks) != 0 || len(step.Arguments) != 0 {
		t.Fatalf("public package step=%+v", step)
	}
	encoded, err := planpublic.MarshalDocument(result.Public())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"status":"ready"`, `"installation":"planned"`, `"config":"not_planned"`, `"service":"not_collected"`, `"auth":"not_collected"`, `"apply":"hash_required"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("public ready document omitted %s: %s", want, encoded)
		}
	}
	if counts.lookup != 1 || counts.describe != 1 || counts.capture != 1 || counts.clock != 1 {
		t.Fatalf("dependency calls=%+v", *counts)
	}
	wantSequence := []string{"lookup:git", "describe:git", "capture", "clock"}
	if !reflect.DeepEqual(counts.sequence, wantSequence) {
		t.Fatalf("dependency sequence=%v, want %v", counts.sequence, wantSequence)
	}

	secondCounts := &dependencyCounts{}
	secondDependencies := countingDependencies(secondCounts, recipe)
	secondDependencies.Now = func() time.Time {
		secondCounts.clock++
		secondCounts.sequence = append(secondCounts.sequence, "clock")
		return time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	secondResult, err := Build(Request{
		Intent: intent, Snapshot: snapshot,
		Environment: managerTestEnvironment(t, pkg.PlatformMacOS, "brew", 9),
	}, secondDependencies)
	if err != nil {
		t.Fatal(err)
	}
	secondAccepted, ok := secondResult.Accepted()
	if !ok || secondAccepted.Hash() != accepted.Hash() {
		t.Fatalf("clock-only drift changed private hash: first=%q second=%q", accepted.Hash(), secondAccepted.Hash())
	}
	secondEncoded, err := planpublic.MarshalDocument(secondResult.Public())
	if err != nil || !reflect.DeepEqual(encoded, secondEncoded) {
		t.Fatalf("clock-only drift changed public bytes:\n%s\n%s err=%v", encoded, secondEncoded, err)
	}

	// Every accessor must be defensive: neither source nor returned slices may
	// rewrite the accepted recipe or its hash-bound operation action.
	recipe.Steps[0].Packages[0] = "mutated-source"
	recipes := accepted.Recipes()
	recipes["git"].Steps[0].Packages[0] = "mutated-accessor"
	actions[0].InstallRecipe.Steps[0].Packages[0] = "mutated-action"
	publicActions[0].Install.Steps[0].Packages[0] = "mutated-public"
	if got := accepted.Recipes()["git"].Steps[0].Packages[0]; got != "git" {
		t.Fatalf("accepted recipe aliases mutable data: %q", got)
	}
	if got := accepted.Operation().Actions()[0].InstallRecipe.Steps[0].Packages[0]; got != "git" {
		t.Fatalf("accepted operation aliases mutable data: %q", got)
	}
	if got := result.Public().Actions()[0].Install.Steps[0].Packages[0]; got != "git" {
		t.Fatalf("public document aliases mutable data: %q", got)
	}
}

func countingDependencies(counts *dependencyCounts, recipe operation.InstallRecipe) Dependencies {
	return Dependencies{
		LookupTool: func(id string) (tools.Tool, bool) {
			counts.lookup++
			counts.sequence = append(counts.sequence, "lookup:"+id)
			return tools.GetRegistry().Get(id)
		},
		DescribeInstall: func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
			counts.describe++
			counts.sequence = append(counts.sequence, "describe:"+tool.ID())
			return operation.CloneInstallRecipe(recipe), nil
		},
		CaptureStatePlan: func() (*operation.StatePlan, error) {
			counts.capture++
			counts.sequence = append(counts.sequence, "capture")
			var err error
			counts.statePlan, err = operation.CaptureStatePlan()
			return counts.statePlan, err
		},
		Now: func() time.Time {
			counts.clock++
			counts.sequence = append(counts.sequence, "clock")
			return time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
		},
	}
}

func mustIntent(t *testing.T, ids ...string) planpublic.Intent {
	t.Helper()
	intent, err := planpublic.NormalizeExplicitTools(ids, ids)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func reviewedPackageRecipe(toolID string) operation.InstallRecipe {
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        toolID, Platform: string(pkg.PlatformMacOS), Manager: "brew",
		Steps:    []operation.InstallStep{{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{toolID}}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{toolID}},
		Risk:     "installs packages from the configured system package manager",
	}
}

func mustObservation(t *testing.T, toolID string, state health.PackageState, recipe operation.InstallRecipe) health.InstallationObservation {
	t.Helper()
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	receipts := recipePackageReceipts(recipe)
	facet := completePackageFacet(state, receipts)
	direct := directDetectorFacet(recipe.Detector, componentStateForPackageState(state))
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: toolID, Installability: health.InstallabilitySupported, InstallRecipeDigest: digest,
		Package: facet, Direct: direct,
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func recipePackageReceipts(recipe operation.InstallRecipe) []string {
	seen := make(map[string]struct{})
	var receipts []string
	for _, step := range recipe.Steps {
		var values []string
		switch step.Kind {
		case operation.InstallStepPackageManager:
			values = step.Packages
		case operation.InstallStepNPMGlobal:
			// NPM globals are detected by their binary, not a package receipt.
		case operation.InstallStepHomebrewCask:
			values = step.Casks
		}
		for _, value := range values {
			if _, duplicate := seen[value]; duplicate {
				continue
			}
			seen[value] = struct{}{}
			receipts = append(receipts, value)
		}
	}
	return receipts
}

func completePackageFacet(state health.PackageState, receipts []string) health.PackageFacet {
	if len(receipts) == 0 {
		return health.PackageFacet{State: health.PackageNotApplicable}
	}
	facet := health.PackageFacet{State: state, Provider: "brew", ExpectedReceipts: append([]string(nil), receipts...), Authoritative: true, Complete: true}
	switch state {
	case health.PackagePresent:
		facet.ObservedReceipts = append([]string(nil), receipts...)
	case health.PackageMissing:
		facet.MissingReceipts = append([]string(nil), receipts...)
	case health.PackagePartial, health.PackageUnknown, health.PackageNotApplicable:
		// These states are not used by this complete-fixture helper.
	}
	return facet
}

func componentStateForPackageState(state health.PackageState) health.ComponentState {
	if state == health.PackagePresent {
		return health.ComponentPresent
	}
	return health.ComponentMissing
}

func directDetectorFacet(detector operation.InstallDetector, state health.ComponentState) health.DirectFacet {
	var kind health.DirectSourceKind
	switch detector.Kind {
	case operation.InstallDetectorBinary:
		kind = health.DirectSourceBinary
	case operation.InstallDetectorAppBundle:
		kind = health.DirectSourceAppBundle
	case operation.InstallDetectorPackageReceipt:
		return health.DirectFacet{State: health.ComponentNotApplicable}
	default:
		return health.DirectFacet{State: health.ComponentNotApplicable}
	}
	return health.DirectFacet{
		State: state, Authoritative: true,
		Alternatives: []health.DirectAlternative{{Kind: kind, Identifiers: append([]string(nil), detector.Values...), State: state}},
	}
}

func mustSnapshot(t *testing.T, generation uint64, observations ...health.InstallationObservation) health.InstallationSnapshot {
	t.Helper()
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{
		Generation: generation, Platform: string(pkg.PlatformMacOS), Manager: "brew", Tools: observations,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
