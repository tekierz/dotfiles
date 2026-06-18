package ui

import (
	"errors"
	"strings"
	"testing"
)

// newGoldenContext builds a deterministic ScreenContext for characterization
// tests: fixed size, animations off, fixed theme, and connected to a real App
// so handlers that reach through ScreenContext.app behave as in production.
func newGoldenContext(t *testing.T) *ScreenContext {
	t.Helper()

	app := NewApp(true)

	ctx := &ScreenContext{
		Deps:              NewTestDependencies(),
		app:               app,
		Theme:             "neon-seapunk",
		NavStyle:          "emacs",
		AnimationsEnabled: false,
		Width:             80,
		Height:            24,
	}
	return ctx
}

// TestErrorScreenGolden is a regression guard for the migrated ErrorScreen.
// It must render the actual error text (finding #40) deterministically.
func TestErrorScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)

	const errText = "permission denied while installing tmux"
	screen := NewErrorScreen(ctx, errors.New(errText))

	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("ErrorScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Error Occurred",
		errText,
		"[R] Retry",
		"[S] Skip",
		"[Q] Quit",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("ErrorScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}
}

// TestErrorScreenGoldenNilError verifies the fallback message when no error is set.
func TestErrorScreenGoldenNilError(t *testing.T) {
	ctx := newGoldenContext(t)
	screen := NewErrorScreen(ctx, nil)

	out := screen.View(ctx.Width, ctx.Height)
	if !strings.Contains(out, "Unknown error") {
		t.Errorf("ErrorScreen.View() with nil error should show %q\n---\n%s\n---", "Unknown error", out)
	}
}

// TestSummaryScreenGolden is a regression guard for the migrated SummaryScreen.
// It renders the key summary labels deterministically and reflects the theme.
func TestSummaryScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	screen := NewSummaryScreen(ctx)

	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("SummaryScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Installation Complete",
		"Theme:",
		"Navigation:",
		"Next steps:",
		"source ~/.zshrc",
		"[ENTER] Exit",
		// Theme/nav values come from the context.
		"neon-seapunk",
		"emacs",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("SummaryScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}
}

// TestMigratedScreensReachableViaManager verifies the foundation wiring: an App
// built with the ScreenManager enters managed mode on NavigateTo(ScreenError)
// and renders the real error through the factory (fixes finding #40 end-to-end).
func TestMigratedScreensReachableViaManager(t *testing.T) {
	app := NewApp(true, WithScreenFactory())
	if app.screenMgr == nil {
		t.Fatal("WithScreenFactory should initialize screenMgr")
	}
	if app.screenFactory == nil {
		t.Fatal("WithScreenFactory should initialize screenFactory")
	}
	if app.screenMgr.Context().app != app {
		t.Fatal("ScreenContext.app should be wired to the App")
	}

	app.screenMgr.SetSize(80, 24)

	const errText = "disk full"
	cmd := app.showError(errors.New(errText))
	if cmd == nil {
		t.Fatal("showError should return a NavigateTo command when manager is active")
	}

	// Execute the navigation command and feed the resulting message to the manager.
	msg := cmd()
	if _, handled := app.screenMgr.Update(msg); !handled {
		t.Fatal("manager should handle the NavigateMsg")
	}
	if app.screenMgr.IsLegacyMode() {
		t.Fatal("manager should be in managed mode after navigating to ScreenError")
	}

	view := app.screenMgr.View()
	if !strings.Contains(view, errText) {
		t.Errorf("managed ErrorScreen should render the real error %q\n---\n%s\n---", errText, view)
	}

	// Summary should also be reachable through the manager.
	scmd := app.showSummary()
	if scmd == nil {
		t.Fatal("showSummary should return a NavigateTo command when manager is active")
	}
	if _, handled := app.screenMgr.Update(scmd()); !handled {
		t.Fatal("manager should handle the summary NavigateMsg")
	}
	sview := app.screenMgr.View()
	if !strings.Contains(sview, "Installation Complete") {
		t.Errorf("managed SummaryScreen should render summary\n---\n%s\n---", sview)
	}
}
