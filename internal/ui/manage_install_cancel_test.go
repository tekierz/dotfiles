package ui

import (
	"errors"
	"testing"
)

func TestWizardPartialFailureInvalidatesAndReloadsInstallCache(t *testing.T) {
	ctx := newGoldenContext(t)
	a := ctx.app
	a.manageInstalledReady = true
	a.installCacheLoading = false
	screen := NewProgressScreen(ctx)

	_, cmd := screen.Update(installDoneMsg{err: errors.New("later tool failed")})
	if cmd == nil || a.manageInstalledReady || !a.installCacheLoading {
		t.Fatalf("partial wizard failure did not reload cache: cmd=%v ready=%v loading=%v", cmd != nil, a.manageInstalledReady, a.installCacheLoading)
	}
}
