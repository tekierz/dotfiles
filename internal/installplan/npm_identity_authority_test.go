package installplan

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func observedNPMExecutionIdentity(t *testing.T, name string) (pkg.NPMExecutionIdentity, []string) {
	t.Helper()
	dir := t.TempDir()
	npmPath := filepath.Join(dir, "private-npm-"+name)
	nodePath := filepath.Join(dir, "private-node-"+name)
	if err := os.WriteFile(npmPath, []byte("#!/usr/bin/env node\n// private npm fixture\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	native, err := os.ReadFile("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nodePath, native, 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
	if err != nil {
		t.Fatal(err)
	}
	return identity, []string{npmPath, nodePath, filepath.Base(npmPath), filepath.Base(nodePath)}
}

func pureNPMRecipe(id string, steps int) operation.InstallRecipe {
	recipe := operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        id, Platform: string(pkg.PlatformMacOS), Manager: "brew",
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{id}},
		Risk:     "downloads and executes npm package lifecycle code",
	}
	for index := 0; index < steps; index++ {
		recipe.Steps = append(recipe.Steps, operation.InstallStep{
			Kind: operation.InstallStepNPMGlobal, Provider: "npm",
			Args: []string{"install", "-g", fmt.Sprintf("private-%s-package-%d@1.2.3", id, index)},
		})
	}
	return recipe
}

func mappedNPMDependencies(counts *dependencyCounts, recipes map[string]operation.InstallRecipe) Dependencies {
	deps := countingDependencies(counts, operation.InstallRecipe{})
	deps.DescribeInstall = func(tool tools.Tool, _ tools.InstallEnvironment) (operation.InstallRecipe, error) {
		counts.describe++
		counts.sequence = append(counts.sequence, "describe:"+tool.ID())
		recipe, ok := recipes[tool.ID()]
		if !ok {
			return operation.InstallRecipe{}, errors.New("missing test recipe")
		}
		return operation.CloneInstallRecipe(recipe), nil
	}
	return deps
}

func npmRequest(t *testing.T, generation uint64, identity pkg.NPMExecutionIdentity, recipes map[string]operation.InstallRecipe, ids ...string) Request {
	t.Helper()
	observations := make([]health.InstallationObservation, 0, len(ids))
	for _, id := range ids {
		observations = append(observations, mustObservation(t, id, health.PackageMissing, recipes[id]))
	}
	return Request{
		Intent: mustIntent(t, ids...), Snapshot: mustSnapshot(t, generation, observations...),
		Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", NPMIdentity: identity, ExpectedGeneration: generation},
	}
}

func TestPureNPMPlanRejectsMissingExecutionIdentityBeforeCapture(t *testing.T) {
	recipe := pureNPMRecipe("codex", 1)
	privatePATH := filepath.Join(t.TempDir(), "private-untrusted-npm-path")
	t.Setenv("PATH", privatePATH)
	counts := &dependencyCounts{}
	result, err := Build(
		npmRequest(t, 101, pkg.NPMExecutionIdentity{}, map[string]operation.InstallRecipe{"codex": recipe}, "codex"),
		mappedNPMDependencies(counts, map[string]operation.InstallRecipe{"codex": recipe}),
	)
	if !errors.Is(err, ErrInvalidRequest) || !errors.Is(err, ErrNPMExecutionAuthorityUnavailable) {
		t.Fatalf("missing npm identity error=%v", err)
	}
	if !reflect.DeepEqual(result, Result{}) || counts.capture != 0 || counts.clock != 0 {
		t.Fatalf("missing npm identity result=%#v counts=%+v", result, *counts)
	}
	assertNPMErrorOmits(t, err, "private-codex-package-0@1.2.3", privatePATH)
}

func TestNPMMixedPhaseOrdersFailBeforeStateCapture(t *testing.T) {
	npmIdentity, npmPrivate := observedNPMExecutionIdentity(t, "mixed-phases")
	managerThenNPM := pureNPMRecipe("codex", 1)
	managerThenNPM.Steps = append(reviewedPackageRecipe("codex").Steps, managerThenNPM.Steps...)
	npmThenManager := operation.CloneInstallRecipe(managerThenNPM)
	npmThenManager.Steps[0], npmThenManager.Steps[1] = npmThenManager.Steps[1], npmThenManager.Steps[0]
	caskThenNPM := pureNPMRecipe("codex", 1)
	caskThenNPM.Steps = append(caskOnlyRecipe("codex").Steps, caskThenNPM.Steps...)
	npmThenCask := operation.CloneInstallRecipe(caskThenNPM)
	npmThenCask.Steps[0], npmThenCask.Steps[1] = npmThenCask.Steps[1], npmThenCask.Steps[0]

	actualClaude := mustDescribedRecipe(t, "claude-code")
	actualCodex := mustDescribedRecipe(t, "codex")
	actualPi := mustDescribedRecipe(t, "pi")
	for _, test := range []struct {
		name              string
		ids               []string
		recipes           map[string]operation.InstallRecipe
		withoutIdentities bool
	}{
		{name: "same-cask-then-npm", ids: []string{"codex"}, recipes: map[string]operation.InstallRecipe{"codex": caskThenNPM}},
		{name: "same-npm-then-cask", ids: []string{"codex"}, recipes: map[string]operation.InstallRecipe{"codex": npmThenCask}},
		{name: "cross-manager-then-npm", ids: []string{"codex", "pi"}, recipes: map[string]operation.InstallRecipe{"codex": reviewedPackageRecipe("codex"), "pi": pureNPMRecipe("pi", 1)}},
		{name: "cross-npm-then-manager", ids: []string{"codex", "pi"}, recipes: map[string]operation.InstallRecipe{"codex": pureNPMRecipe("codex", 1), "pi": reviewedPackageRecipe("pi")}},
		{name: "cross-cask-then-npm", ids: []string{"codex", "pi"}, recipes: map[string]operation.InstallRecipe{"codex": caskOnlyRecipe("codex"), "pi": pureNPMRecipe("pi", 1)}},
		{name: "cross-npm-then-cask", ids: []string{"codex", "pi"}, recipes: map[string]operation.InstallRecipe{"codex": pureNPMRecipe("codex", 1), "pi": caskOnlyRecipe("pi")}},
		{name: "actual-claude", ids: []string{"claude-code"}, recipes: map[string]operation.InstallRecipe{"claude-code": actualClaude}},
		{name: "actual-codex", ids: []string{"codex"}, recipes: map[string]operation.InstallRecipe{"codex": actualCodex}},
		{name: "actual-pi", ids: []string{"pi"}, recipes: map[string]operation.InstallRecipe{"pi": actualPi}},
		{name: "same-manager-then-npm", ids: []string{"codex"}, recipes: map[string]operation.InstallRecipe{"codex": managerThenNPM}},
		{name: "same-npm-then-manager-no-identities", ids: []string{"codex"}, recipes: map[string]operation.InstallRecipe{"codex": npmThenManager}, withoutIdentities: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			counts := &dependencyCounts{}
			identity := npmIdentity
			if test.withoutIdentities {
				identity = pkg.NPMExecutionIdentity{}
			}
			request := npmRequest(t, 103, identity, test.recipes, test.ids...)
			managerIdentity, managerPath := observedManagerIdentity(t, "mixed-phase-brew", "exit 0")
			if !test.withoutIdentities {
				request.Environment.ManagerIdentity = managerIdentity
			}
			result, err := Build(request, mappedNPMDependencies(counts, test.recipes))
			if !errors.Is(err, ErrInvalidRequest) || !errors.Is(err, ErrNPMPhaseBoundaryRequired) {
				t.Fatalf("phase error=%v", err)
			}
			if !reflect.DeepEqual(result, Result{}) || counts.capture != 0 || counts.clock != 0 {
				t.Fatalf("phase result=%#v counts=%+v", result, *counts)
			}
			assertNPMErrorOmits(t, err, append(npmPrivate, npmIdentity.Digest(), managerIdentity.Digest(), managerPath, filepath.Base(managerPath), "private-codex@1.2.3", "private-codex-package-0@1.2.3", "private-pi-package-0@1.2.3")...)
		})
	}
}

func assertNPMErrorOmits(t *testing.T, err error, privateValues ...string) {
	t.Helper()
	formatted := err.Error() + fmt.Sprintf(" %#v", err)
	for _, private := range privateValues {
		if private != "" && strings.Contains(formatted, private) {
			t.Fatalf("npm authority error leaks %q: %q", private, formatted)
		}
	}
}

func TestPureNPMReadyPlansBindOnlyTheAcceptedExecutionIdentity(t *testing.T) {
	for _, test := range []struct {
		name    string
		ids     []string
		recipes map[string]operation.InstallRecipe
	}{
		{name: "single-step", ids: []string{"codex"}, recipes: map[string]operation.InstallRecipe{"codex": pureNPMRecipe("codex", 1)}},
		{name: "multi-step", ids: []string{"codex"}, recipes: map[string]operation.InstallRecipe{"codex": pureNPMRecipe("codex", 2)}},
		{name: "multi-tool", ids: []string{"codex", "pi"}, recipes: map[string]operation.InstallRecipe{"codex": pureNPMRecipe("codex", 1), "pi": pureNPMRecipe("pi", 2)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			identityA, privateA := observedNPMExecutionIdentity(t, test.name+"-a")
			identityB, privateB := observedNPMExecutionIdentity(t, test.name+"-b")
			build := func(identity pkg.NPMExecutionIdentity) Result {
				t.Helper()
				counts := &dependencyCounts{}
				result, err := Build(npmRequest(t, 104, identity, test.recipes, test.ids...), mappedNPMDependencies(counts, test.recipes))
				if err != nil {
					t.Fatal(err)
				}
				if counts.capture != 1 || counts.clock != 1 {
					t.Fatalf("pure npm dependencies=%+v, want one capture and clock", *counts)
				}
				return result
			}
			first, repeat, drifted := build(identityA), build(identityA), build(identityB)
			accepted, ok := first.Accepted()
			if !ok || first.Public().Status() != "ready" {
				t.Fatalf("pure npm result accepted=%v status=%q", ok, first.Public().Status())
			}
			bound, boundOK := accepted.NPMExecutionIdentity()
			if !boundOK || bound.SchemaVersion() != pkg.CurrentNPMExecutionIdentitySchemaVersion || bound.Digest() != identityA.Digest() {
				t.Fatalf("bound npm identity=(%d,%q,%v)", bound.SchemaVersion(), bound.Digest(), boundOK)
			}
			repeatedAccepted, repeatedOK := repeat.Accepted()
			driftedAccepted, driftedOK := drifted.Accepted()
			if !repeatedOK || !driftedOK || accepted.Hash() != repeatedAccepted.Hash() || accepted.Hash() == driftedAccepted.Hash() {
				t.Fatalf("npm hash binding first=%q repeat=%q drift=%q", accepted.Hash(), repeatedAccepted.Hash(), driftedAccepted.Hash())
			}
			if !reflect.DeepEqual(mustPublicJSON(t, first.Public()), mustPublicJSON(t, repeat.Public())) {
				t.Fatal("identical npm authority produced nondeterministic public bytes")
			}
			operationJSON, err := json.Marshal(accepted.Operation())
			if err != nil {
				t.Fatal(err)
			}
			projections := [][]byte{mustPublicJSON(t, first.Public()), operationJSON}
			for _, projection := range projections {
				for _, private := range append(append([]string{identityA.Digest(), identityB.Digest()}, privateA...), privateB...) {
					if private != "" && strings.Contains(string(projection), private) {
						t.Fatalf("public/operation projection leaked npm authority %q", private)
					}
				}
			}
		})
	}
}

func TestPresentPackageDoesNotContaminateMissingPureNPMApplyingSet(t *testing.T) {
	packageRecipe, npmRecipe := reviewedPackageRecipe("git"), pureNPMRecipe("codex", 1)
	npmIdentity, _ := observedNPMExecutionIdentity(t, "present-package")
	counts := &dependencyCounts{}
	result, err := Build(Request{
		Intent: mustIntent(t, "git", "codex"),
		Snapshot: mustSnapshot(t, 105,
			mustObservation(t, "git", health.PackagePresent, packageRecipe),
			mustObservation(t, "codex", health.PackageMissing, npmRecipe)),
		Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", NPMIdentity: npmIdentity, ExpectedGeneration: 105},
	}, mappedNPMDependencies(counts, map[string]operation.InstallRecipe{"git": packageRecipe, "codex": npmRecipe}))
	accepted, ok := result.Accepted()
	if err != nil || !ok || result.Public().Status() != "ready" {
		t.Fatalf("mixed selection status=%q accepted=%v error=%v", result.Public().Status(), ok, err)
	}
	if _, retained := accepted.ManagerExecutableIdentity(); retained {
		t.Fatal("present package contaminated pure npm applying authority")
	}
	if bound, retained := accepted.NPMExecutionIdentity(); !retained || bound.Digest() != npmIdentity.Digest() {
		t.Fatal("pure npm applying set omitted npm identity")
	}
	actions := accepted.Operation().Actions()
	if len(actions) != 1 || actions[0].ToolID != "codex" || counts.capture != 1 || counts.clock != 1 {
		t.Fatalf("mixed selection actions=%+v counts=%+v", actions, *counts)
	}
}

func TestAcceptedPlanHashRejectsForgedNPMPhaseAuthority(t *testing.T) {
	npmIdentity, _ := observedNPMExecutionIdentity(t, "forged")
	pure := pureNPMRecipe("codex", 1)
	result, err := Build(npmRequest(t, 106, npmIdentity, map[string]operation.InstallRecipe{"codex": pure}, "codex"), mappedNPMDependencies(&dependencyCounts{}, map[string]operation.InstallRecipe{"codex": pure}))
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := result.Accepted()
	if !ok {
		t.Fatal("pure npm fixture omitted accepted authority")
	}

	missing := cloneAccepted(accepted)
	missing.npmIdentity = pkg.NPMExecutionIdentity{}
	if hash, hashErr := acceptedPlanHash(missing); hash != "" || !errors.Is(hashErr, ErrInvalidRequest) || !errors.Is(hashErr, ErrNPMExecutionAuthorityUnavailable) {
		t.Fatalf("missing npm forgery hash=%q error=%v", hash, hashErr)
	}

	packageRecipe := reviewedPackageRecipe("git")
	packageResult, err := Build(Request{
		Intent: mustIntent(t, "git"), Snapshot: mustSnapshot(t, 107, mustObservation(t, "git", health.PackageMissing, packageRecipe)),
		Environment: managerTestEnvironment(t, pkg.PlatformMacOS, "brew", 107),
	}, countingDependencies(&dependencyCounts{}, packageRecipe))
	if err != nil {
		t.Fatal(err)
	}
	packageAccepted, ok := packageResult.Accepted()
	if !ok {
		t.Fatal("package fixture omitted accepted authority")
	}
	unexpected := cloneAccepted(packageAccepted)
	unexpected.npmIdentity = npmIdentity
	if hash, hashErr := acceptedPlanHash(unexpected); hash != "" || !errors.Is(hashErr, ErrInvalidRequest) {
		t.Fatalf("unexpected npm forgery hash=%q error=%v", hash, hashErr)
	}

	phase := cloneAccepted(accepted)
	phaseRecipe := operation.CloneInstallRecipe(pure)
	phaseRecipe.Steps = append(reviewedPackageRecipe("codex").Steps, phaseRecipe.Steps...)
	phase.recipes["codex"] = phaseRecipe
	phase.managerIdentity, _ = observedManagerIdentity(t, "forged-phase-brew", "exit 0")
	if hash, hashErr := acceptedPlanHash(phase); hash != "" || !errors.Is(hashErr, ErrInvalidRequest) || !errors.Is(hashErr, ErrNPMPhaseBoundaryRequired) {
		t.Fatalf("phase forgery hash=%q error=%v", hash, hashErr)
	}
}

func TestNPMAuthorityChecksFollowRecipeAndDetectorValidation(t *testing.T) {
	t.Run("recipe-drift", func(t *testing.T) {
		recipe := pureNPMRecipe("codex", 1)
		counts := &dependencyCounts{}
		deps := mappedNPMDependencies(counts, map[string]operation.InstallRecipe{"codex": recipe})
		original := deps.DescribeInstall
		deps.DescribeInstall = func(tool tools.Tool, environment tools.InstallEnvironment) (operation.InstallRecipe, error) {
			drifted, err := original(tool, environment)
			drifted.Risk = "different reviewed risk"
			return drifted, err
		}
		result, err := Build(npmRequest(t, 108, pkg.NPMExecutionIdentity{}, map[string]operation.InstallRecipe{"codex": recipe}, "codex"), deps)
		assertBlockedWithoutAccepted(t, result, err, "recipe_drift")
		if counts.capture != 0 || counts.clock != 0 {
			t.Fatalf("recipe drift reached capture/clock: %+v", *counts)
		}
	})

	t.Run("detector-unknown", func(t *testing.T) {
		recipe := mustDescribedRecipe(t, "codex")
		observation := mustDetectorAuthorityObservation(t, "codex", recipe,
			completePackageFacet(health.PackagePresent, recipePackageReceipts(recipe)),
			health.DirectFacet{State: health.ComponentUnknown, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"codex"}, State: health.ComponentUnknown}}},
		)
		counts := &dependencyCounts{}
		result, err := Build(Request{
			Intent: mustIntent(t, "codex"), Snapshot: mustSnapshot(t, 109, observation),
			Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 109},
		}, mappedNPMDependencies(counts, map[string]operation.InstallRecipe{"codex": recipe}))
		assertBlockedWithoutAccepted(t, result, err, "unknown")
		if errors.Is(err, ErrNPMPhaseBoundaryRequired) || errors.Is(err, ErrNPMExecutionAuthorityUnavailable) || counts.capture != 0 || counts.clock != 0 {
			t.Fatalf("detector precedence error=%v counts=%+v", err, *counts)
		}
	})
}

func caskOnlyRecipe(id string) operation.InstallRecipe {
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        id, Platform: string(pkg.PlatformMacOS), Manager: "brew",
		Steps:    []operation.InstallStep{{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{id + "-app"}}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorAppBundle, Values: []string{id + ".app"}},
		Risk:     "installs the current t3-code Homebrew cask; artifact content is not pinned",
	}
}

func mustDescribedRecipe(t *testing.T, id string) operation.InstallRecipe {
	t.Helper()
	tool, ok := tools.GetRegistry().Get(id)
	if !ok {
		t.Fatalf("missing tool %q", id)
	}
	recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	return recipe
}
