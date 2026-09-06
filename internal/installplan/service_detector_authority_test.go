package installplan

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestBuildBindsActualDetectorObservationForPartialRepairs(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	for _, test := range []struct {
		name       string
		packageSet health.PackageFacet
		direct     health.DirectFacet
		want       bool
	}{
		{
			name: "missing package receipt is detector false",
			packageSet: health.PackageFacet{State: health.PackagePartial, Provider: "brew", ExpectedReceipts: []string{"git", "git-extra"}, ObservedReceipts: []string{"git-extra"}, MissingReceipts: []string{"git"}, Authoritative: true, Complete: true,
				Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: health.PackagePartial, ExpectedReceipts: []string{"git", "git-extra"}, ObservedReceipts: []string{"git-extra"}, MissingReceipts: []string{"git"}, Complete: true}}},
			direct: health.DirectFacet{State: health.ComponentNotApplicable},
		},
		{
			name: "present package receipt with missing direct facet is detector true",
			packageSet: health.PackageFacet{State: health.PackagePresent, Provider: "brew", ExpectedReceipts: []string{"git"}, ObservedReceipts: []string{"git"}, Authoritative: true, Complete: true,
				Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: health.PackagePresent, ExpectedReceipts: []string{"git"}, ObservedReceipts: []string{"git"}, Complete: true}}},
			direct: health.DirectFacet{State: health.ComponentMissing, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"git"}, State: health.ComponentMissing}}},
			want:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			digest, err := operation.InstallRecipeDigest(recipe)
			if err != nil {
				t.Fatal(err)
			}
			observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{ToolID: "git", Installability: health.InstallabilitySupported, InstallRecipeDigest: digest, Package: test.packageSet, Direct: test.direct})
			if err != nil {
				t.Fatal(err)
			}
			if observation.Presence() != health.PresencePartial {
				t.Fatalf("presence=%q", observation.Presence())
			}
			snapshot := mustSnapshot(t, 73, observation)
			result, err := Build(Request{Intent: mustIntent(t, "git"), Snapshot: snapshot, Environment: managerTestEnvironment(t, pkg.PlatformMacOS, "brew", 73)}, countingDependencies(&dependencyCounts{}, recipe))
			if err != nil {
				t.Fatal(err)
			}
			accepted, ok := result.Accepted()
			if !ok {
				t.Fatal("partial repair omitted accepted authority")
			}
			actions := accepted.Operation().Actions()
			if len(actions) != 1 || actions[0].InstallDetected == nil || *actions[0].InstallDetected != test.want {
				t.Fatalf("detector authority actions=%+v want=%v", actions, test.want)
			}
		})
	}
}

func TestDetectorObservationChangesAcceptedAuthorityHash(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	build := func(t *testing.T, detectorPresent bool) AcceptedPlan {
		t.Helper()
		packageFacet := health.PackageFacet{
			State: health.PackagePartial, Provider: "brew", ExpectedReceipts: []string{"git", "git-repair"},
			ObservedReceipts: []string{"git-repair"}, MissingReceipts: []string{"git"}, Authoritative: true, Complete: true,
		}
		directFacet := health.DirectFacet{State: health.ComponentNotApplicable}
		if detectorPresent {
			packageFacet = health.PackageFacet{
				State: health.PackagePresent, Provider: "brew", ExpectedReceipts: []string{"git"},
				ObservedReceipts: []string{"git"}, Authoritative: true, Complete: true,
			}
			directFacet = health.DirectFacet{State: health.ComponentMissing, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"repair-signal"}, State: health.ComponentMissing}}}
		}
		observation := mustDetectorAuthorityObservation(t, "git", recipe,
			packageFacet,
			directFacet,
		)
		result, err := Build(
			Request{Intent: mustIntent(t, "git"), Snapshot: mustSnapshot(t, 74, observation), Environment: managerTestEnvironment(t, pkg.PlatformMacOS, "brew", 74)},
			countingDependencies(&dependencyCounts{}, recipe),
		)
		if err != nil {
			t.Fatal(err)
		}
		accepted, ok := result.Accepted()
		if !ok {
			t.Fatal("partial repair omitted accepted authority")
		}
		return accepted
	}

	detectorMissing := build(t, false)
	detectorPresent := build(t, true)
	missingActions := detectorMissing.Operation().Actions()
	presentActions := detectorPresent.Operation().Actions()
	if len(missingActions) != 1 || missingActions[0].InstallDetected == nil || *missingActions[0].InstallDetected {
		t.Fatalf("missing detector authority=%+v, want false", missingActions)
	}
	if len(presentActions) != 1 || presentActions[0].InstallDetected == nil || !*presentActions[0].InstallDetected {
		t.Fatalf("present detector authority=%+v, want true", presentActions)
	}
	if detectorMissing.Hash() == detectorPresent.Hash() {
		t.Fatalf("detector boolean did not change accepted hash %q", detectorMissing.Hash())
	}
}

func TestUnknownDetectorEvidenceBlocksBeforeStateCaptureAndClock(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	for _, test := range []struct {
		name         string
		recipe       operation.InstallRecipe
		packageFacet health.PackageFacet
		directFacet  health.DirectFacet
	}{
		{
			name: "unresolved binary detector",
			recipe: func() operation.InstallRecipe {
				result := operation.CloneInstallRecipe(recipe)
				result.Detector = operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{"git-bin"}}
				return result
			}(),
			packageFacet: health.PackageFacet{State: health.PackagePresent, Provider: "brew", ExpectedReceipts: []string{"git"}, ObservedReceipts: []string{"git"}, Authoritative: true, Complete: true},
			directFacet:  health.DirectFacet{State: health.ComponentUnknown, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"git-bin"}, State: health.ComponentUnknown}}},
		},
		{
			name:         "unauthoritative detector facet",
			recipe:       recipe,
			packageFacet: health.PackageFacet{State: health.PackagePresent, Provider: "brew", ExpectedReceipts: []string{"git"}, ObservedReceipts: []string{"git"}, Complete: true},
			directFacet:  health.DirectFacet{State: health.ComponentMissing, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"repair-signal"}, State: health.ComponentMissing}}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			observation := mustDetectorAuthorityObservation(t, "git", test.recipe, test.packageFacet, test.directFacet)
			if observation.Presence() != health.PresencePartial {
				t.Fatalf("fixture presence=%q, want partial", observation.Presence())
			}
			counts := &dependencyCounts{}
			result, err := Build(
				Request{Intent: mustIntent(t, "git"), Snapshot: mustSnapshot(t, 75, observation), Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 75}},
				countingDependencies(counts, test.recipe),
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Public().Status() != "blocked" {
				t.Fatalf("status=%q, want blocked", result.Public().Status())
			}
			if _, ok := result.Accepted(); ok {
				t.Fatal("unknown detector evidence exposed accepted authority")
			}
			if counts.lookup != 1 || counts.describe != 1 || counts.capture != 0 || counts.clock != 0 {
				t.Fatalf("dependency calls=%+v, want lookup/describe only", *counts)
			}
		})
	}
}

func TestMultiValuePackageDetectorRequiresEveryReceipt(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	recipe.Steps[0].Packages = []string{"git", "git-runtime"}
	recipe.Detector.Values = []string{"git", "git-runtime"}
	for _, test := range []struct {
		name     string
		observed []string
		missing  []string
		want     bool
	}{
		{name: "all present", observed: []string{"git", "git-runtime"}, want: true},
		{name: "one missing", observed: []string{"git"}, missing: []string{"git-runtime"}},
		{name: "first missing", observed: []string{"git-runtime"}, missing: []string{"git"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := health.PackagePresent
			if len(test.missing) != 0 {
				state = health.PackagePartial
			}
			observation := mustDetectorAuthorityObservation(t, "git", recipe,
				health.PackageFacet{State: state, Provider: "brew", ExpectedReceipts: []string{"git", "git-runtime"}, ObservedReceipts: test.observed, MissingReceipts: test.missing, Authoritative: true, Complete: true},
				health.DirectFacet{State: health.ComponentNotApplicable},
			)
			got, known := ObservedInstallDetector(observation, recipe.Detector)
			if !known || got != test.want {
				t.Fatalf("detector=(%v,%v), want (%v,true)", got, known, test.want)
			}
		})
	}
}

func TestDirectDetectorEvidenceMapsBinaryAndAppBundleBooleans(t *testing.T) {
	for _, test := range []struct {
		name   string
		kind   operation.InstallDetectorKind
		source health.DirectSourceKind
		state  health.ComponentState
		want   bool
	}{
		{name: "binary present", kind: operation.InstallDetectorBinary, source: health.DirectSourceBinary, state: health.ComponentPresent, want: true},
		{name: "binary missing", kind: operation.InstallDetectorBinary, source: health.DirectSourceBinary, state: health.ComponentMissing},
		{name: "app present", kind: operation.InstallDetectorAppBundle, source: health.DirectSourceAppBundle, state: health.ComponentPresent, want: true},
		{name: "app missing", kind: operation.InstallDetectorAppBundle, source: health.DirectSourceAppBundle, state: health.ComponentMissing},
	} {
		t.Run(test.name, func(t *testing.T) {
			recipe := reviewedPackageRecipe("candidate")
			recipe.Detector = operation.InstallDetector{Kind: test.kind, Values: []string{"candidate-detector"}}
			observation := mustDetectorAuthorityObservation(t, "candidate", recipe,
				health.PackageFacet{State: health.PackageNotApplicable},
				health.DirectFacet{State: test.state, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: test.source, Identifiers: []string{"candidate-detector"}, State: test.state}}},
			)
			got, known := ObservedInstallDetector(observation, recipe.Detector)
			if !known || got != test.want {
				t.Fatalf("detector=(%v,%v), want (%v,true)", got, known, test.want)
			}
		})
	}
}

func mustDetectorAuthorityObservation(t *testing.T, id string, recipe operation.InstallRecipe, packageFacet health.PackageFacet, directFacet health.DirectFacet) health.InstallationObservation {
	t.Helper()
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: id, Installability: health.InstallabilitySupported, InstallRecipeDigest: digest,
		Package: packageFacet, Direct: directFacet,
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
