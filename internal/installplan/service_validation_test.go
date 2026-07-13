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

type overrideIDTool struct {
	tools.Tool
	id string
}

func (tool overrideIDTool) ID() string { return tool.id }

func TestBuildSeparatesFatalSnapshotShapeFromRepresentableStaleness(t *testing.T) {
	intent := mustIntent(t, "git")
	recipe := reviewedPackageRecipe("git")
	valid := mustSnapshot(t, 11, mustObservation(t, "git", health.PackageMissing, recipe))

	counts := &dependencyCounts{}
	result, err := Build(Request{Intent: intent, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 11}}, countingDependencies(counts, recipe))
	assertFatalWithoutPublic(t, result, err)
	assertDependencyCounts(t, counts, 0, 0, 0, 0)

	for _, test := range []struct {
		name        string
		snapshot    health.InstallationSnapshot
		environment Environment
		code        string
	}{
		{name: "generation stale", snapshot: valid, environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 12}, code: "stale"},
		{name: "platform changed", snapshot: valid, environment: Environment{Platform: pkg.PlatformArch, Manager: "brew", ExpectedGeneration: 11}, code: "environment_mismatch"},
		{name: "manager changed", snapshot: valid, environment: Environment{Platform: pkg.PlatformMacOS, Manager: "apt", ExpectedGeneration: 11}, code: "environment_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := &dependencyCounts{}
			result, err := Build(Request{Intent: intent, Snapshot: test.snapshot, Environment: test.environment}, countingDependencies(counts, recipe))
			assertBlockedWithoutAccepted(t, result, err, test.code)
			assertDependencyCounts(t, counts, 0, 0, 0, 0)
		})
	}

	missingSelected := mustSnapshot(t, 11, mustObservation(t, "codex", health.PackageMissing, reviewedPackageRecipe("codex")))
	for _, test := range []struct {
		name      string
		code      string
		configure func(*Dependencies)
	}{
		{name: "known canonical tool is stale", code: "stale"},
		{name: "unknown tool identity is unknown", code: "unknown", configure: func(deps *Dependencies) {
			original := deps.LookupTool
			deps.LookupTool = func(id string) (tools.Tool, bool) { _, _ = original(id); return nil, false }
		}},
		{name: "typed nil tool identity is unknown", code: "unknown", configure: func(deps *Dependencies) {
			original := deps.LookupTool
			deps.LookupTool = func(id string) (tools.Tool, bool) {
				_, _ = original(id)
				var missing *tools.GitTool
				return missing, true
			}
		}},
		{name: "mismatched tool identity is recipe drift", code: "recipe_drift", configure: func(deps *Dependencies) {
			original := deps.LookupTool
			deps.LookupTool = func(id string) (tools.Tool, bool) {
				_, _ = original(id)
				return overrideIDTool{Tool: tools.NewGitTool(), id: "other"}, true
			}
		}},
	} {
		t.Run("missing selected observation/"+test.name, func(t *testing.T) {
			counts := &dependencyCounts{}
			deps := countingDependencies(counts, recipe)
			if test.configure != nil {
				test.configure(&deps)
			}
			result, err := Build(Request{Intent: intent, Snapshot: missingSelected, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 11}}, deps)
			assertBlockedWithoutAccepted(t, result, err, test.code)
			if !reflect.DeepEqual(counts.sequence, []string{"lookup:git"}) {
				t.Fatalf("missing observation dependency sequence=%v", counts.sequence)
			}
			assertDependencyCounts(t, counts, 1, 0, 0, 0)
		})
	}
}

func TestBuildReturnsBlockedDomainOutcomesWithoutPrivateAuthority(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	digest := mustRecipeDigest(t, recipe)
	unknown := mustTypedObservation(t, "git", health.InstallabilitySupported, digest, health.ComponentUnknown)
	unsupported := mustTypedObservation(t, "git", health.InstallabilityUnsupported, "", health.ComponentMissing)
	validMissing := mustObservation(t, "git", health.PackageMissing, recipe)

	for _, test := range []struct {
		name         string
		observation  health.InstallationObservation
		code         string
		configure    func(*Dependencies)
		wantSequence []string
	}{
		{name: "unknown presence", observation: unknown, code: "unknown"},
		{name: "unsupported installability", observation: unsupported, code: "unsupported"},
		{name: "lookup absent", observation: validMissing, code: "recipe_drift", wantSequence: []string{"lookup:git"}, configure: func(deps *Dependencies) {
			original := deps.LookupTool
			deps.LookupTool = func(id string) (tools.Tool, bool) { _, _ = original(id); return nil, false }
		}},
		{name: "lookup identity mismatch", observation: validMissing, code: "recipe_drift", wantSequence: []string{"lookup:git"}, configure: func(deps *Dependencies) {
			original := deps.LookupTool
			deps.LookupTool = func(id string) (tools.Tool, bool) {
				_, _ = original(id)
				return overrideIDTool{Tool: tools.NewGitTool(), id: "other"}, true
			}
		}},
		{name: "recipe tool drift", observation: validMissing, code: "recipe_drift", wantSequence: []string{"lookup:git", "describe:git"}, configure: mutateRecipeDependency(func(value *operation.InstallRecipe) { value.ToolID = "other" })},
		{name: "recipe platform drift", observation: validMissing, code: "recipe_drift", wantSequence: []string{"lookup:git", "describe:git"}, configure: mutateRecipeDependency(func(value *operation.InstallRecipe) { value.Platform = "arch" })},
		{name: "recipe manager drift", observation: validMissing, code: "recipe_drift", wantSequence: []string{"lookup:git", "describe:git"}, configure: mutateRecipeDependency(func(value *operation.InstallRecipe) { value.Manager = "apt"; value.Steps[0].Provider = "apt" })},
		{name: "recipe digest drift", observation: validMissing, code: "recipe_drift", wantSequence: []string{"lookup:git", "describe:git"}, configure: mutateRecipeDependency(func(value *operation.InstallRecipe) { value.Risk = "different reviewed risk" })},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := &dependencyCounts{}
			deps := countingDependencies(counts, recipe)
			if test.configure != nil {
				test.configure(&deps)
			}
			snapshot := mustSnapshot(t, 13, test.observation)
			result, err := Build(Request{Intent: mustIntent(t, "git"), Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 13}}, deps)
			assertBlockedWithoutAccepted(t, result, err, test.code)
			if !reflect.DeepEqual(counts.sequence, test.wantSequence) {
				t.Fatalf("dependency sequence=%v, want %v", counts.sequence, test.wantSequence)
			}
			lookup, describe, capture, clock := dependencySequenceCounts(test.wantSequence)
			assertDependencyCounts(t, counts, lookup, describe, capture, clock)
		})
	}
}

func TestBuildFatalDependencyDescribeCaptureAndClockFailuresExposeNoPublicDocument(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	request := Request{Intent: mustIntent(t, "git"), Snapshot: mustSnapshot(t, 17, mustObservation(t, "git", health.PackageMissing, recipe)), Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 17}}
	for _, missing := range []string{"lookup", "describe", "capture", "clock"} {
		counts := &dependencyCounts{}
		deps := countingDependencies(counts, recipe)
		switch missing {
		case "lookup":
			deps.LookupTool = nil
		case "describe":
			deps.DescribeInstall = nil
		case "capture":
			deps.CaptureStatePlan = nil
		case "clock":
			deps.Now = nil
		}
		result, err := Build(request, deps)
		assertFatalWithoutPublic(t, result, err)
		assertDependencyCounts(t, counts, 0, 0, 0, 0)
		if len(counts.sequence) != 0 || counts.statePlan != nil {
			t.Fatalf("missing %s dependency invoked a sentinel before full dependency validation: %+v", missing, *counts)
		}
	}

	for _, test := range []struct {
		name         string
		configure    func(*Dependencies, *dependencyCounts)
		wantSequence []string
	}{
		{name: "describe error", wantSequence: []string{"lookup:git", "describe:git"}, configure: func(deps *Dependencies, counts *dependencyCounts) {
			original := deps.DescribeInstall
			deps.DescribeInstall = func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
				_, _ = original(tool, environment)
				return operation.InstallRecipe{}, errors.New("describe failed")
			}
		}},
		{name: "capture error", wantSequence: []string{"lookup:git", "describe:git", "capture"}, configure: func(deps *Dependencies, counts *dependencyCounts) {
			deps.CaptureStatePlan = func() (*operation.StatePlan, error) {
				counts.capture++
				counts.sequence = append(counts.sequence, "capture")
				return nil, errors.New("capture failed")
			}
		}},
		{name: "nil capture", wantSequence: []string{"lookup:git", "describe:git", "capture"}, configure: func(deps *Dependencies, counts *dependencyCounts) {
			deps.CaptureStatePlan = func() (*operation.StatePlan, error) {
				counts.capture++
				counts.sequence = append(counts.sequence, "capture")
				return nil, nil
			}
		}},
		{name: "zero clock", wantSequence: []string{"lookup:git", "describe:git", "capture", "clock"}, configure: func(deps *Dependencies, counts *dependencyCounts) {
			deps.Now = func() time.Time {
				counts.clock++
				counts.sequence = append(counts.sequence, "clock")
				return time.Time{}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := &dependencyCounts{}
			deps := countingDependencies(counts, recipe)
			test.configure(&deps, counts)
			result, err := Build(request, deps)
			assertFatalWithoutPublic(t, result, err)
			if !reflect.DeepEqual(counts.sequence, test.wantSequence) {
				t.Fatalf("fatal dependency sequence=%v, want %v", counts.sequence, test.wantSequence)
			}
			lookup, describe, capture, clock := dependencySequenceCounts(test.wantSequence)
			assertDependencyCounts(t, counts, lookup, describe, capture, clock)
		})
	}
}

func TestBuildMultiToolFailureStopsAtExactPrefixAndPublishesNoSubset(t *testing.T) {
	gitRecipe := reviewedPackageRecipe("git")
	piRecipe := reviewedPackageRecipe("pi")
	snapshot := mustSnapshot(t, 19, mustObservation(t, "git", health.PackageMissing, gitRecipe), mustObservation(t, "pi", health.PackageMissing, piRecipe))
	request := Request{Intent: mustIntent(t, "pi", "git"), Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 19}}

	counts := &dependencyCounts{}
	deps := countingDependencies(counts, gitRecipe)
	deps.DescribeInstall = func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
		counts.describe++
		counts.sequence = append(counts.sequence, "describe:"+tool.ID())
		if tool.ID() == "pi" {
			return operation.InstallRecipe{}, errors.New("pi description failed")
		}
		return operation.CloneInstallRecipe(gitRecipe), nil
	}
	result, err := Build(request, deps)
	assertFatalWithoutPublic(t, result, err)
	want := []string{"lookup:git", "describe:git", "lookup:pi", "describe:pi"}
	if !reflect.DeepEqual(counts.sequence, want) || counts.capture != 0 || counts.clock != 0 {
		t.Fatalf("multi-tool fatal sequence=%v, want %v", counts.sequence, want)
	}

	counts = &dependencyCounts{}
	deps = countingDependencies(counts, gitRecipe)
	deps.DescribeInstall = func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
		counts.describe++
		counts.sequence = append(counts.sequence, "describe:"+tool.ID())
		recipe := operation.CloneInstallRecipe(gitRecipe)
		recipe.ToolID = "other"
		return recipe, nil
	}
	result, err = Build(request, deps)
	assertBlockedWithoutAccepted(t, result, err, "recipe_drift")
	want = []string{"lookup:git", "describe:git"}
	if !reflect.DeepEqual(counts.sequence, want) {
		t.Fatalf("first blocked tool did not stop later validation: %v", counts.sequence)
	}
}

func mutateRecipeDependency(mutate func(*operation.InstallRecipe)) func(*Dependencies) {
	return func(deps *Dependencies) {
		original := deps.DescribeInstall
		deps.DescribeInstall = func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
			recipe, err := original(tool, environment)
			if err == nil {
				mutate(&recipe)
			}
			return recipe, err
		}
	}
}

func mustTypedObservation(t *testing.T, id string, installability health.Installability, digest string, direct health.ComponentState) health.InstallationObservation {
	t.Helper()
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: id, Installability: installability, InstallRecipeDigest: digest,
		Package: health.PackageFacet{State: health.PackageNotApplicable},
		Direct:  health.DirectFacet{State: direct, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{id}, State: direct}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func assertBlockedWithoutAccepted(t *testing.T, result Result, err error, code string) {
	t.Helper()
	if err != nil {
		t.Fatalf("blocked domain outcome returned fatal error: %v", err)
	}
	if _, ok := result.Accepted(); ok {
		t.Fatal("blocked outcome exposed accepted private authority")
	}
	actions := result.Public().Actions()
	if len(actions) == 0 {
		t.Fatal("blocked outcome omitted public decision action")
	}
	for _, action := range actions {
		if action.Disposition != "blocked" || action.ReasonCode != code {
			t.Fatalf("blocked action=%+v, want code %q", action, code)
		}
	}
}

func assertFatalWithoutPublic(t *testing.T, result Result, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("fatal build unexpectedly succeeded")
	}
	if _, ok := result.Accepted(); ok {
		t.Fatal("fatal build exposed accepted authority")
	}
	if encoded, marshalErr := planpublic.MarshalDocument(result.Public()); marshalErr == nil {
		t.Fatalf("fatal build exposed public bytes: %s", encoded)
	}
}

func assertDependencyCounts(t *testing.T, counts *dependencyCounts, lookup, describe, capture, clock int) {
	t.Helper()
	if counts.lookup != lookup || counts.describe != describe || counts.capture != capture || counts.clock != clock {
		t.Fatalf("dependency counts=%+v, want %d/%d/%d/%d", *counts, lookup, describe, capture, clock)
	}
}

func dependencySequenceCounts(sequence []string) (lookup, describe, capture, clock int) {
	for _, entry := range sequence {
		switch {
		case strings.HasPrefix(entry, "lookup:"):
			lookup++
		case strings.HasPrefix(entry, "describe:"):
			describe++
		case entry == "capture":
			capture++
		case entry == "clock":
			clock++
		}
	}
	return lookup, describe, capture, clock
}
