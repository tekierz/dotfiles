package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg builds a deterministic tea.KeyMsg for the named keys used by these
// tests. Its String() must match what the handlers switch on (e.g. "down",
// "tab").
func keyMsg(name string) tea.KeyMsg {
	switch name {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		// Single-rune keys (e.g. "j", "k", "q").
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
}

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

// TestWelcomeScreenGolden is a regression guard for the migrated welcomeScreen.
func TestWelcomeScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	screen := NewWelcomeScreen(ctx)

	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("welcomeScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"SYSTEM READY",
		"QUICK SETUP",
		"DEEP DIVE",
		"enter continue",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("welcomeScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenWelcome {
		t.Errorf("welcomeScreen.ID() = %v, want ScreenWelcome", screen.ID())
	}
}

// TestThemePickerScreenGolden is a regression guard for the migrated
// themePickerScreen. It must list the available themes and mark the selection.
func TestThemePickerScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	// Deterministic selection: first theme.
	ctx.app.themeIndex = 0
	ctx.app.theme = themes[0].name

	screen := NewThemePickerScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("themePickerScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Select Theme",
		themes[0].name, // first theme listed
		"neon-seapunk", // last theme is always in the list
		"Navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("themePickerScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenThemePicker {
		t.Errorf("themePickerScreen.ID() = %v, want ScreenThemePicker", screen.ID())
	}
}

// TestThemePickerLivePreview verifies that changing the selection applies the
// theme to App state and the shared context (live preview wiring).
func TestThemePickerLivePreview(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.themeIndex = 0
	ctx.app.theme = themes[0].name
	ctx.Theme = themes[0].name

	screen := NewThemePickerScreen(ctx)

	// Press down once: selection should advance and theme should be applied to
	// both the App and the context.
	next, _ := screen.Update(keyMsg("down"))
	if next != screen {
		t.Fatalf("themePickerScreen should remain current after 'down'")
	}
	if ctx.app.themeIndex != 1 {
		t.Errorf("themeIndex = %d, want 1 after 'down'", ctx.app.themeIndex)
	}
	if ctx.app.theme != themes[1].name {
		t.Errorf("app.theme = %q, want %q", ctx.app.theme, themes[1].name)
	}
	if ctx.Theme != themes[1].name {
		t.Errorf("ctx.Theme = %q, want %q (live preview must update context)", ctx.Theme, themes[1].name)
	}
}

// TestNavPickerScreenGolden is a regression guard for the migrated
// navPickerScreen.
func TestNavPickerScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.navStyle = "emacs"

	screen := NewNavPickerScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("navPickerScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Select Navigation Style",
		"EMACS",
		"VIM STYLE",
		"Continue",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("navPickerScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	// 'tab' should toggle the nav style on both App and context.
	if _, _ = screen.Update(keyMsg("tab")); ctx.app.navStyle != "vim" {
		t.Errorf("navStyle = %q, want vim after 'tab'", ctx.app.navStyle)
	}
	if ctx.NavStyle != "vim" {
		t.Errorf("ctx.NavStyle = %q, want vim (toggle must update context)", ctx.NavStyle)
	}
}

// TestFileTreeScreenGolden is a regression guard for the migrated fileTreeScreen.
// The install cache is pre-populated so the render is deterministic and does not
// depend on the host's package manager.
func TestFileTreeScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)

	// Pre-populate the install cache so ensureInstallCache() returns early and
	// the output is deterministic regardless of the host environment.
	ctx.app.manageInstalled = map[string]bool{}
	ctx.app.manageInstalledReady = true

	screen := NewFileTreeScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("fileTreeScreen.View() returned empty output")
	}

	// The "Files to be Modified" section is static and always rendered.
	wantSubstrings := []string{
		"Installation Summary",
		"Files to be Modified",
		"ghostty/",
		"yazi/",
		".zshrc",
		"Start Installation",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("fileTreeScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenFileTree {
		t.Errorf("fileTreeScreen.ID() = %v, want ScreenFileTree", screen.ID())
	}
}

// TestMainMenuScreenGolden is a regression guard for the migrated mainMenuScreen.
func TestMainMenuScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.mainMenuIndex = 0

	screen := NewMainMenuScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("mainMenuScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Dotfiles Management",
		"Install",
		"Manage",
		"Update",
		"Theme",
		"Hotkeys",
		"Backups",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("mainMenuScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenMainMenu {
		t.Errorf("mainMenuScreen.ID() = %v, want ScreenMainMenu", screen.ID())
	}
}

// TestWelcomeReachableViaManager verifies the navigation backbone: an App built
// with the ScreenManager enters managed mode on NavigateTo(ScreenWelcome) and
// renders the welcome content through the factory.
func TestWelcomeReachableViaManager(t *testing.T) {
	app := NewApp(true, WithScreenFactory())
	if app.screenMgr == nil {
		t.Fatal("WithScreenFactory should initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)

	cmd := NavigateTo(ScreenWelcome)
	if _, handled := app.screenMgr.Update(cmd()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenWelcome")
	}
	if app.screenMgr.IsLegacyMode() {
		t.Fatal("manager should be in managed mode after navigating to ScreenWelcome")
	}

	view := app.screenMgr.View()
	if !strings.Contains(view, "SYSTEM READY") {
		t.Errorf("managed welcomeScreen should render welcome content\n---\n%s\n---", view)
	}

	// MainMenu should likewise be reachable through the manager.
	if _, handled := app.screenMgr.Update(NavigateTo(ScreenMainMenu)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenMainMenu")
	}
	if app.screenMgr.IsLegacyMode() {
		t.Fatal("manager should be in managed mode after navigating to ScreenMainMenu")
	}
	if mv := app.screenMgr.View(); !strings.Contains(mv, "Dotfiles Management") {
		t.Errorf("managed mainMenuScreen should render menu\n---\n%s\n---", mv)
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
