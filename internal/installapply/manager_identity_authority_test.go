package installapply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func applyManagerIdentity(t *testing.T, name, body string) (pkg.ExecutableIdentity, string) {
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

func TestExecuteRecipeRejectsManagerIdentityMismatchBeforeMutation(t *testing.T) {
	accepted, acceptedPath := applyManagerIdentity(t, "accepted-brew", "exit 0")
	current, currentPath := applyManagerIdentity(t, "current-brew", "exit 0")
	manager := &recipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	if err := manager.SetExecutableIdentity(current); err != nil {
		t.Fatal(err)
	}
	recipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"git"}})
	if err := ExecuteRecipe(context.Background(), recipe, manager, accepted, nil); !errors.Is(err, ErrManagerIdentityChanged) {
		t.Fatalf("mismatched manager identity error=%v", err)
	} else if strings.Contains(err.Error(), accepted.Digest()) || strings.Contains(err.Error(), acceptedPath) || strings.Contains(err.Error(), currentPath) {
		t.Fatalf("manager identity error leaked private authority: %v", err)
	}
	if len(manager.order) != 0 {
		t.Fatalf("mismatched manager identity mutated packages: %v", manager.order)
	}
}

func TestExecuteRecipeRevalidatesAcceptedManagerImmediatelyBeforeMutation(t *testing.T) {
	accepted, path := applyManagerIdentity(t, "drifting-brew", "exit 0")
	manager := &recipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	if err := manager.SetExecutableIdentity(accepted); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 9\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	recipe := reviewedExecutionRecipe(operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"git"}})
	if err := ExecuteRecipe(context.Background(), recipe, manager, accepted, nil); !errors.Is(err, ErrManagerIdentityChanged) {
		t.Fatalf("drifted accepted manager identity error=%v", err)
	}
	if len(manager.order) != 0 {
		t.Fatalf("drifted accepted manager identity mutated packages: %v", manager.order)
	}
}

func TestExecuteRecipeStopsBetweenManagerBackedStepsOnIdentityDrift(t *testing.T) {
	accepted, path := applyManagerIdentity(t, "multi-step-brew", "exit 0")
	manager := &recipeManager{MockPackageManager: pkg.NewMockPackageManager()}
	manager.ManagerName = "brew"
	if err := manager.SetExecutableIdentity(accepted); err != nil {
		t.Fatal(err)
	}
	manager.onInstall = func() {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
			t.Errorf("mutate manager executable: %v", err)
		}
	}
	recipe := reviewedExecutionRecipe(
		operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}},
		operation.InstallStep{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"t3-code"}},
	)
	err := ExecuteRecipe(context.Background(), recipe, manager, accepted, nil)
	wrapped := fmt.Errorf("outer execution boundary: %w", err)
	if !errors.Is(err, ErrManagerIdentityChanged) || !errors.Is(wrapped, ErrManagerIdentityChanged) {
		t.Fatalf("between-step drift error=%v wrapped=%v", err, wrapped)
	}
	if strings.Contains(wrapped.Error(), path) || strings.Contains(wrapped.Error(), accepted.Digest()) {
		t.Fatalf("wrapped identity error leaked private authority: %v", wrapped)
	}
	if !reflect.DeepEqual(manager.order, []string{"package:node"}) || len(manager.casks) != 0 {
		t.Fatalf("between-step drift reached later mutation: order=%v casks=%v", manager.order, manager.casks)
	}
}

func TestApplyRejectsCurrentManagerIdentityMismatchBeforeBootstrap(t *testing.T) {
	session, managerValue, hash := applyFreshSession(t, health.PresenceMissing)
	manager, ok := managerValue.(*pkg.MockPackageManager)
	if !ok {
		t.Fatalf("fixture manager type=%T", managerValue)
	}
	replacement, _ := applyManagerIdentity(t, "replacement-brew", "exit 0")
	if err := manager.SetExecutableIdentity(replacement); err != nil {
		t.Fatal(err)
	}
	dependencies := rejectingMutationDependencies(t, func(context.Context, []string) (installplan.FreshSession, error) {
		return session, nil
	})
	if _, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, dependencies); !errors.Is(err, ErrApplyFailed) || !errors.Is(err, ErrManagerIdentityChanged) {
		t.Fatalf("manager identity mismatch error=%v, want ErrApplyFailed + ErrManagerIdentityChanged", err)
	}
}
