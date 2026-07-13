package installplan

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func observedManagerIdentity(t *testing.T, name, body string) (pkg.ExecutableIdentity, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := pkg.ObserveExecutableIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	return identity, path
}

func managerTestEnvironment(t *testing.T, platform pkg.Platform, manager string, generation uint64) Environment {
	t.Helper()
	identity, err := pkg.ObserveExecutableIdentity("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	return Environment{
		Platform:           platform,
		Manager:            manager,
		ManagerIdentity:    identity,
		ExpectedGeneration: generation,
	}
}

func TestAcceptedPlanBindsManagerExecutableIdentityIntoPrivateHash(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", "")
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 91, mustObservation(t, "git", health.PackageMissing, recipe))
	identityA, pathA := observedManagerIdentity(t, "private-brew-a", "exit 0")
	identityB, pathB := observedManagerIdentity(t, "private-brew-b", "exit 1")

	build := func(identity pkg.ExecutableIdentity) (Result, error) {
		return Build(Request{
			Intent:   mustIntent(t, "git"),
			Snapshot: snapshot,
			Environment: Environment{
				Platform:           pkg.PlatformMacOS,
				Manager:            "brew",
				ManagerIdentity:    identity,
				ExpectedGeneration: 91,
			},
		}, countingDependencies(&dependencyCounts{}, recipe))
	}

	first, err := build(identityA)
	if err != nil {
		t.Fatal(err)
	}
	second, err := build(identityB)
	if err != nil {
		t.Fatal(err)
	}
	acceptedA, ok := first.Accepted()
	if !ok {
		t.Fatal("ready plan omitted accepted authority")
	}
	acceptedB, ok := second.Accepted()
	if !ok {
		t.Fatal("second ready plan omitted accepted authority")
	}
	bound, ok := acceptedA.ManagerExecutableIdentity()
	if !ok || bound.Digest() != identityA.Digest() {
		t.Fatalf("accepted manager identity=(%q,%v), want (%q,true)", bound.Digest(), ok, identityA.Digest())
	}
	if acceptedA.Hash() == acceptedB.Hash() {
		t.Fatal("manager executable identity drift did not change the accepted plan hash")
	}

	publicJSON := mustPublicJSON(t, first.Public())
	operationJSON, err := json.Marshal(acceptedA.Operation())
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range [][]byte{publicJSON, operationJSON} {
		for _, forbidden := range []string{identityA.Digest(), pathA, pathB, filepath.Base(pathA), filepath.Base(pathB)} {
			if forbidden != "" && strings.Contains(string(projection), forbidden) {
				t.Fatalf("public/operation projection exposed private manager identity material %q", forbidden)
			}
		}
	}
}

func TestReadyPlanRejectsMissingManagerExecutableIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", "")
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 92, mustObservation(t, "git", health.PackageMissing, recipe))
	result, err := Build(Request{
		Intent:   mustIntent(t, "git"),
		Snapshot: snapshot,
		Environment: Environment{
			Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 92,
		},
	}, countingDependencies(&dependencyCounts{}, recipe))
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("missing manager identity error=%v, want ErrInvalidRequest", err)
	}
	if _, ok := result.Accepted(); ok {
		t.Fatal("missing manager identity produced executable accepted authority")
	}
}

func TestNPMOnlyReadyPlanOmitsManagerIdentityEvenWhenEnvironmentSuppliesOne(t *testing.T) {
	recipe := operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        "pi",
		Platform:      string(pkg.PlatformMacOS),
		Manager:       "brew",
		Steps: []operation.InstallStep{{
			Kind:     operation.InstallStepNPMGlobal,
			Provider: "npm",
			Args:     []string{"install", "-g", "--ignore-scripts", "@earendil-works/pi-coding-agent@0.80.3"},
		}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{"pi"}},
		Risk:     "installs an npm package with lifecycle scripts disabled; package code runs when Pi is launched",
	}
	snapshot := mustSnapshot(t, 93, mustObservation(t, "pi", health.PackageMissing, recipe))
	build := func(environment Environment) AcceptedPlan {
		t.Helper()
		result, err := Build(Request{Intent: mustIntent(t, "pi"), Snapshot: snapshot, Environment: environment}, countingDependencies(&dependencyCounts{}, recipe))
		if err != nil {
			t.Fatal(err)
		}
		accepted, ok := result.Accepted()
		if !ok {
			t.Fatal("npm-only ready plan omitted accepted authority")
		}
		return accepted
	}

	withoutIdentity := build(Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 93})
	withIdentity := build(managerTestEnvironment(t, pkg.PlatformMacOS, "brew", 93))
	if _, ok := withoutIdentity.ManagerExecutableIdentity(); ok {
		t.Fatal("npm-only plan unexpectedly bound a manager executable identity")
	}
	if _, ok := withIdentity.ManagerExecutableIdentity(); ok {
		t.Fatal("npm-only plan retained an unrequired manager executable identity")
	}
	if withoutIdentity.Hash() != withIdentity.Hash() {
		t.Fatal("unrequired manager identity changed npm-only accepted authority")
	}
}
