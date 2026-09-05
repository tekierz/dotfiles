package installapply

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestApplyCancellationRetainsOperationLockUntilRecipeTerminal(t *testing.T) {
	session, plannedManager, hash := applyFreshSession(t, health.PresenceMissing)
	mock, ok := plannedManager.(*pkg.MockPackageManager)
	if !ok {
		t.Fatalf("unexpected planned manager %T", plannedManager)
	}
	manager := newGatedRecipeManager(mock, false)
	defer manager.complete()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sequence []string
	journal := &applyJournal{sequence: &sequence}
	deps := successfulApplyDependencies(t, session, plannedManager, journal, &sequence)
	var state *operation.StateAuthority
	var scope, target string
	releases, detects := 0, 0
	deps.AcquireLock = func(authority *operation.StateAuthority, lockScope, lockTarget string) (func() error, error) {
		state, scope, target = authority, lockScope, lockTarget
		release, err := operation.AcquireStateLockWithAuthority(state, scope, target)
		if err != nil {
			return nil, err
		}
		return func() error { releases++; return release() }, nil
	}
	deps.ExecuteRecipe = func(ctx context.Context, recipe operation.InstallRecipe, _ pkg.PackageManager, identity pkg.ExecutableIdentity, emit func(string)) error {
		return ExecuteRecipe(ctx, recipe, manager, identity, emit)
	}
	deps.DetectRecipe = func(operation.InstallRecipe, pkg.PackageManager) (bool, error) { detects++; return false, nil }
	type outcome struct {
		result Result
		err    error
	}
	returned := make(chan outcome, 1)
	go func() {
		result, err := Apply(ctx, Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps)
		returned <- outcome{result, err}
	}()
	select {
	case <-manager.started:
	case result := <-returned:
		t.Fatalf("apply returned before recipe started: %+v", result)
	case <-time.After(2 * time.Second):
		t.Fatal("apply did not start recipe")
	}
	cancel()
	<-manager.cancelled
	select {
	case result := <-returned:
		t.Fatalf("apply returned before recipe terminal completion: %+v", result)
	case <-time.After(50 * time.Millisecond):
	}
	acquired := make(chan error, 1)
	go func() {
		release, err := operation.AcquireStateLockWithAuthority(state, scope, target)
		if err == nil {
			err = release()
		}
		acquired <- err
	}()
	select {
	case err := <-acquired:
		t.Fatalf("competing operation stopped waiting before terminal completion: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	manager.complete()
	select {
	case result := <-returned:
		if !errors.Is(result.err, context.Canceled) || !errors.Is(result.err, ErrApplyFailed) || result.result.Status != operation.StatusCancelled {
			t.Fatalf("apply result=%+v error=%v", result.result, result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("apply did not return after terminal completion")
	}
	if releases != 1 || detects != 2 || len(journal.records) != 2 || journal.records[1].Status != operation.StatusCancelled {
		t.Fatalf("releases=%d detectors=%d journal=%+v", releases, detects, journal.records)
	}
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("competing operation failed after terminal completion: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("operation lock still held after terminal completion")
	}
}
