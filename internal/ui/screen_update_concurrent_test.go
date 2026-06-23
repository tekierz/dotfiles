package ui

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// sampleUpdateResults returns a deterministic, non-empty outdated-package list
// so the update screen's action gates (which require len(updateResults) > 0)
// are satisfied without touching the host's package manager.
func sampleUpdateResults() []pkg.Package {
	return []pkg.Package{
		{Name: "tmux", CurrentVersion: "3.3a", LatestVersion: "3.4", Outdated: true, InstalledBy: "brew"},
		{Name: "neovim", CurrentVersion: "0.9.5", LatestVersion: "0.10.0", Outdated: true, InstalledBy: "brew"},
	}
}

// TestUpdateScreenEnterSetsRunGuardSynchronously pins down the concurrent-update
// fix in updateHandleEnter: the run guard (a.updateRunning) must be set
// SYNCHRONOUSLY at dispatch, before the async checkSudoAndUpdateCmd starts. The
// first Enter must return a non-nil command AND flip updateRunning=true in the
// same call; a SECOND Enter (delivered before any async start) must be a no-op
// (nil command) so no concurrent update is dispatched.
//
// On the buggy code, updateRunning was only set later inside handleUpdateStartMsg,
// so the second Enter would pass the top-of-handleKey guard and return a second
// non-nil checkSudoAndUpdateCmd — starting a CONCURRENT update and orphaning the
// first stream. The wantSecondNil assertion below would fail on that code.
func TestUpdateScreenEnterSetsRunGuardSynchronously(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateChecking = false
	ctx.app.updateCheckDone = true
	ctx.app.updateRunning = false
	ctx.app.updateIndex = 0
	ctx.app.updateResults = sampleUpdateResults()

	screen := NewUpdateScreen(ctx)

	// First Enter: dispatches the update AND sets the run guard synchronously.
	next, cmd := screen.Update(keyMsg("enter"))
	if next != screen {
		t.Fatalf("updateScreen should remain current after first Enter")
	}
	if cmd == nil {
		t.Fatal("first Enter should dispatch an update (non-nil command), got nil")
	}
	if !ctx.app.updateRunning {
		t.Fatal("first Enter must set updateRunning=true SYNCHRONOUSLY at dispatch")
	}

	// Second Enter before any async start: must be a no-op (the top-of-handleKey
	// guard `if a.updateRunning { return nil }` rejects it).
	next2, cmd2 := screen.Update(keyMsg("enter"))
	if next2 != screen {
		t.Fatalf("updateScreen should remain current after second Enter")
	}
	if cmd2 != nil {
		t.Error("second Enter must be a no-op (nil command); a non-nil command means a concurrent update was dispatched")
	}
}

// TestUpdateScreenUpdateAllSetsRunGuardSynchronously is the analogous guard for
// the 'a' (update-all) path. The first 'a' must dispatch (non-nil command) and
// set updateRunning=true synchronously; the second 'a' must be a no-op so no
// concurrent update-all is started.
//
// On the buggy code the 'a' case did not set updateRunning before returning
// checkSudoAndUpdateCmd, so the second 'a' would pass the guard and dispatch a
// second concurrent update — failing the wantSecondNil assertion below.
func TestUpdateScreenUpdateAllSetsRunGuardSynchronously(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateChecking = false
	ctx.app.updateCheckDone = true
	ctx.app.updateRunning = false
	ctx.app.updateIndex = 0
	ctx.app.updateResults = sampleUpdateResults()

	screen := NewUpdateScreen(ctx)

	// First 'a': dispatches update-all AND sets the run guard synchronously.
	next, cmd := screen.Update(keyMsg("a"))
	if next != screen {
		t.Fatalf("updateScreen should remain current after first 'a'")
	}
	if cmd == nil {
		t.Fatal("first 'a' should dispatch an update-all (non-nil command), got nil")
	}
	if !ctx.app.updateRunning {
		t.Fatal("first 'a' must set updateRunning=true SYNCHRONOUSLY at dispatch")
	}

	// Second 'a' before any async start: must be a no-op.
	next2, cmd2 := screen.Update(keyMsg("a"))
	if next2 != screen {
		t.Fatalf("updateScreen should remain current after second 'a'")
	}
	if cmd2 != nil {
		t.Error("second 'a' must be a no-op (nil command); a non-nil command means a concurrent update-all was dispatched")
	}
}
