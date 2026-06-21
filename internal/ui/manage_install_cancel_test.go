package ui

import (
	"testing"
)

// TestManageInstallRegistersCancel guards FIX 3: starting a manage single-tool
// install must register a cancelable handle (a.streamCancel) on the main loop so
// teardownStream can cancel the (often `sudo apt/pacman install ...`) subprocess
// on Ctrl+C / q instead of orphaning it to init. The previous code used
// context.Background() with no registered cancel, so teardownStream was a no-op
// for this path.
//
// The test only exercises the handle wiring; it deliberately does NOT run the
// returned Cmd (which would invoke the real package manager / sudo).
func TestManageInstallRegistersCancel(t *testing.T) {
	ctx := newGoldenContext(t)
	a := ctx.app

	if a.streamCancel != nil {
		t.Fatal("precondition: streamCancel should be nil before install starts")
	}

	// Start the install (returns a Cmd we intentionally do not execute).
	_ = a.handleManageStartInstallMsg(manageStartInstallMsg{toolID: "tmux"})

	if a.streamCancel == nil {
		t.Fatal("handleManageStartInstallMsg did not register a.streamCancel; teardownStream cannot cancel the install (FIX 3)")
	}
	if !a.manageInstalling {
		t.Error("manageInstalling should be true after start")
	}

	// Prove teardownStream actually invokes the registered cancel and clears it.
	cancelled := false
	a.streamCancel = func() { cancelled = true }
	a.teardownStream()

	if !cancelled {
		t.Error("teardownStream did not invoke the registered streamCancel (FIX 3)")
	}
	if a.streamCancel != nil || a.streamCmd != nil {
		t.Errorf("teardownStream did not clear handles: streamCancel=%v streamCmd=%v", a.streamCancel, a.streamCmd)
	}
}

// TestManageInstallCompletionClearsHandles verifies that finalizing a manage
// install tears down the stream so no stale cancel/keep-alive lingers (FIX 3).
func TestManageInstallCompletionClearsHandles(t *testing.T) {
	ctx := newGoldenContext(t)
	a := ctx.app

	// Simulate an in-flight install with a registered cancel handle.
	cancelled := false
	a.manageInstalling = true
	a.manageInstallID = "tmux"
	a.streamCancel = func() { cancelled = true }

	// Completion (success path) must tear down the stream.
	_ = a.handleManageInstallWithLogsMsg(manageInstallWithLogsMsg{toolID: "tmux"})

	if !cancelled {
		t.Error("completion did not invoke teardownStream's cancel")
	}
	if a.streamCancel != nil || a.streamCmd != nil {
		t.Errorf("completion left handles set: streamCancel=%v streamCmd=%v", a.streamCancel, a.streamCmd)
	}
	if a.manageInstalling {
		t.Error("manageInstalling should be false after completion")
	}
}
