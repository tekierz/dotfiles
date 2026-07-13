package installplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestMain(main *testing.M) {
	home, err := os.MkdirTemp("", "dotfiles-installplan-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if err := os.Unsetenv("XDG_STATE_HOME"); err != nil {
		panic(err)
	}
	code := main.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func TestAcceptedHashBindsPrivateStateAuthorityAndPublishesOnlyItsFingerprint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	firstState, err := operation.CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".local"), 0o700); err != nil {
		t.Fatal(err)
	}
	secondState, err := operation.CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 61, mustObservation(t, "git", health.PackageMissing, recipe))
	request := Request{Intent: mustIntent(t, "git"), Snapshot: snapshot, Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 61}}
	build := func(state *operation.StatePlan) Result {
		t.Helper()
		result, buildErr := Build(request, Dependencies{
			LookupTool: tools.GetRegistry().Get,
			DescribeInstall: func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
				return operation.CloneInstallRecipe(recipe), nil
			},
			CaptureStatePlan: func() (*operation.StatePlan, error) { return state, nil },
			Now:              func() time.Time { return time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC) },
		})
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		return result
	}
	first, second := build(firstState), build(secondState)
	firstAccepted, firstOK := first.Accepted()
	secondAccepted, secondOK := second.Accepted()
	if !firstOK || !secondOK || firstAccepted.Hash() == secondAccepted.Hash() {
		t.Fatalf("private authority hashes = %q/%q accepted=%v/%v", firstAccepted.Hash(), secondAccepted.Hash(), firstOK, secondOK)
	}
	firstPublic, err := planpublic.MarshalDocument(first.Public())
	if err != nil {
		t.Fatal(err)
	}
	secondPublic, err := planpublic.MarshalDocument(second.Public())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstPublic, secondPublic) {
		t.Fatal("private state authority did not change published plan authority")
	}
	var firstRedacted, secondRedacted map[string]any
	if err := json.Unmarshal(firstPublic, &firstRedacted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(secondPublic, &secondRedacted); err != nil {
		t.Fatal(err)
	}
	delete(firstRedacted, "authority")
	delete(secondRedacted, "authority")
	if !reflect.DeepEqual(firstRedacted, secondRedacted) {
		t.Fatalf("private state authority changed redacted public projection:\n%s\n%s", firstPublic, secondPublic)
	}
	forged := firstAccepted
	forged.intent.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	forgedHash, err := acceptedPlanHash(forged)
	if err != nil || forgedHash == firstAccepted.Hash() {
		t.Fatalf("intent digest was not bound: forged=%q accepted=%q err=%v", forgedHash, firstAccepted.Hash(), err)
	}
}

func TestBuildRejectsMalformedStateAuthority(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 62, mustObservation(t, "git", health.PackageMissing, recipe))
	result, err := Build(Request{
		Intent: mustIntent(t, "git"), Snapshot: snapshot,
		Environment: Environment{Platform: pkg.PlatformMacOS, Manager: "brew", ExpectedGeneration: 62},
	}, Dependencies{
		LookupTool: tools.GetRegistry().Get,
		DescribeInstall: func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
			return operation.CloneInstallRecipe(recipe), nil
		},
		CaptureStatePlan: func() (*operation.StatePlan, error) { return &operation.StatePlan{}, nil },
		Now:              func() time.Time { return time.Now() },
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("malformed state error=%v, want ErrInvalidRequest", err)
	}
	if _, ok := result.Accepted(); ok {
		t.Fatal("malformed state authority produced accepted plan")
	}
}
