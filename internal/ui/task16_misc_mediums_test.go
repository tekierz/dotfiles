package ui

import (
	"errors"
	"testing"
)

// TestUsersLoadedResetOnError is the regression guard for the audit finding:
// usersLoaded must be reset to false when a userLoadedMsg arrives with an
// error, so that re-entering the screen retries the load instead of
// permanently stranding an empty list.
func TestUsersLoadedResetOnError(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width = 80
	ctx.app.height = 24

	s := NewUsersScreen(ctx)

	// Simulate Init setting usersLoaded = true (as if load was kicked).
	ctx.app.usersLoaded = true

	// Feed a userLoadedMsg with an error — the load failed.
	loadErr := errors.New("disk read error")
	s.Update(userLoadedMsg{err: loadErr})

	// usersLoaded must be false so re-entry will retry the load.
	if ctx.app.usersLoaded {
		t.Error("usersLoaded must be false after a failed load so re-entry retries; got true")
	}

	// Status should reflect the failure.
	if ctx.app.usersStatus == "" {
		t.Error("usersStatus should be non-empty after a failed load")
	}
}

// TestUsersLoadedRemainsOnSuccess verifies the success path is unaffected:
// usersLoaded stays true and usersItems are populated when load succeeds.
func TestUsersLoadedRemainsOnSuccess(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width = 80
	ctx.app.height = 24

	s := NewUsersScreen(ctx)
	ctx.app.usersLoaded = true

	users := []userItem{{name: "alice", theme: "dracula", navStyle: "emacs", keyboard: "linux"}}
	s.Update(userLoadedMsg{users: users})

	if !ctx.app.usersLoaded {
		t.Error("usersLoaded must remain true after a successful load")
	}
	if len(ctx.app.usersItems) != 1 {
		t.Errorf("usersItems should have 1 item after success, got %d", len(ctx.app.usersItems))
	}
}

// TestConfigFieldIndexClampOnInit is the regression guard for the audit
// finding: a stale out-of-range configFieldIndex must be clamped to a valid
// range when a config screen is initialised, decoupling correctness from the
// back() reset path.
func TestConfigFieldIndexClampOnInit(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.width = 80
	ctx.app.height = 50

	s := NewConfigGhosttyScreen(ctx)

	// Set configFieldIndex well beyond the screen's maxField (6 for Ghostty).
	ctx.app.configFieldIndex = 999

	// Init must clamp/reset the index.
	s.Init()

	maxField := s.maxField(ctx.app)
	if ctx.app.configFieldIndex > maxField {
		t.Errorf("configFieldIndex = %d after Init(), want <= %d (maxField)",
			ctx.app.configFieldIndex, maxField)
	}
}

// TestConfigFieldIndexClampNegativeOnInit verifies Init also corrects a
// negative (underflow) configFieldIndex.
func TestConfigFieldIndexClampNegativeOnInit(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.width = 80
	ctx.app.height = 50

	s := NewConfigGhosttyScreen(ctx)
	ctx.app.configFieldIndex = -5

	s.Init()

	if ctx.app.configFieldIndex < 0 {
		t.Errorf("configFieldIndex = %d after Init(), want >= 0", ctx.app.configFieldIndex)
	}
}
