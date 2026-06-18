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

// newDeepDiveContext builds a deterministic ScreenContext for the deep-dive
// screens. It pre-populates the install cache (so DeepDiveMenu renders the menu
// instead of the loading spinner, and the install-aware list screens render
// deterministically regardless of the host environment) and ensures a fresh
// NewDeepDiveConfig() is present.
func newDeepDiveContext(t *testing.T) *ScreenContext {
	t.Helper()
	ctx := newGoldenContext(t)
	// NewApp already sets a NewDeepDiveConfig(); assert it for clarity.
	if ctx.app.deepDiveConfig == nil {
		ctx.app.deepDiveConfig = NewDeepDiveConfig()
	}
	ctx.app.manageInstalled = map[string]bool{}
	ctx.app.manageInstalledReady = true
	ctx.app.installCacheLoading = false
	return ctx
}

// TestDeepDiveMenuScreenGolden is a regression guard for the migrated
// deepDiveMenuScreen. With the install cache pre-populated it renders the tool
// menu (not the loading spinner).
func TestDeepDiveMenuScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.deepDiveMenuIndex = 0

	screen := NewDeepDiveMenuScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("deepDiveMenuScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"DEEP DIVE CONFIGURATION",
		"Ghostty",
		"Tmux",
		"Zsh",
		"Neovim",
		"Continue to Installation",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("deepDiveMenuScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenDeepDiveMenu {
		t.Errorf("deepDiveMenuScreen.ID() = %v, want ScreenDeepDiveMenu", screen.ID())
	}
}

// TestDeepDiveMenuLoadingGolden verifies the loading spinner is shown while the
// install cache is still populating.
func TestDeepDiveMenuLoadingGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installCacheLoading = true

	screen := NewDeepDiveMenuScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if !strings.Contains(out, "Loading installation status") {
		t.Errorf("deepDiveMenuScreen.View() should show loading state\n---\n%s\n---", out)
	}
}

// TestDeepDiveMenuNavigatesToConfig verifies that pressing enter on the first
// menu item produces a NavigateMsg to that item's config screen.
func TestDeepDiveMenuNavigatesToConfig(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.deepDiveMenuIndex = 0

	screen := NewDeepDiveMenuScreen(ctx)
	_, cmd := screen.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter on a config item should return a navigation command")
	}
	msg := cmd()
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	want := GetFilteredDeepDiveMenuItems()[0].Screen
	if nav.To != want {
		t.Errorf("NavigateMsg.To = %v, want %v", nav.To, want)
	}
}

// TestConfigGhosttyScreenGolden is a regression guard for the migrated Ghostty
// config screen.
func TestConfigGhosttyScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigGhosttyScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configGhosttyScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Ghostty",
		"Font Family",
		"Font Size",
		"Background Opacity",
		"Cursor Style",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configGhosttyScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigGhostty {
		t.Errorf("configGhosttyScreen.ID() = %v, want ScreenConfigGhostty", screen.ID())
	}
}

// TestConfigGhosttyAdjustAndBack verifies the shared field navigation: right
// adjusts the focused field and esc navigates back to the deep-dive menu.
func TestConfigGhosttyAdjustAndBack(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.configFieldIndex = 1 // Font Size
	before := ctx.app.deepDiveConfig.GhosttyFontSize

	screen := NewConfigGhosttyScreen(ctx)
	if _, _ = screen.Update(keyMsg("right")); ctx.app.deepDiveConfig.GhosttyFontSize != before+1 {
		t.Errorf("GhosttyFontSize = %d, want %d after 'right'", ctx.app.deepDiveConfig.GhosttyFontSize, before+1)
	}

	_, cmd := screen.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("esc should return a navigation command")
	}
	nav, ok := cmd().(NavigateMsg)
	if !ok || nav.To != ScreenDeepDiveMenu {
		t.Errorf("esc should NavigateTo(ScreenDeepDiveMenu), got %#v", cmd())
	}
	if ctx.app.configFieldIndex != 0 {
		t.Errorf("configFieldIndex = %d, want 0 reset on back", ctx.app.configFieldIndex)
	}
}

// TestConfigZshScreenGolden is a regression guard for the migrated Zsh config
// screen.
func TestConfigZshScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigZshScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configZshScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Zsh",
		"Prompt Style",
		"Powerlevel10k",
		"Shell Options",
		"Plugins",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configZshScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigZsh {
		t.Errorf("configZshScreen.ID() = %v, want ScreenConfigZsh", screen.ID())
	}
}

// TestConfigNeovimScreenGolden is a regression guard for the migrated Neovim
// config screen.
func TestConfigNeovimScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigNeovimScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configNeovimScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Neovim",
		"Configuration",
		"Kickstart.nvim",
		"Editor Settings",
		"LSP Servers",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configNeovimScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigNeovim {
		t.Errorf("configNeovimScreen.ID() = %v, want ScreenConfigNeovim", screen.ID())
	}
}

// TestConfigMacAppsScreenGolden is a regression guard for the migrated macOS
// apps selection screen. The install cache is pre-populated so the render does
// not depend on the host's package manager.
func TestConfigMacAppsScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.macAppIndex = 0
	screen := NewConfigMacAppsScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configMacAppsScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"macOS Apps",
		"Rectangle",
		"Raycast",
		"AppCleaner",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configMacAppsScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigMacApps {
		t.Errorf("configMacAppsScreen.ID() = %v, want ScreenConfigMacApps", screen.ID())
	}
}

// TestConfigMacAppsToggleAndBack verifies list navigation: space toggles the
// focused (not-installed) app, and esc resets the index and navigates back.
func TestConfigMacAppsToggleAndBack(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.macAppIndex = 0
	firstID := macAppItems[0].id
	before := ctx.app.deepDiveConfig.MacApps[firstID]

	screen := NewConfigMacAppsScreen(ctx)
	if _, _ = screen.Update(keyMsg(" ")); ctx.app.deepDiveConfig.MacApps[firstID] == before {
		t.Errorf("space should toggle MacApps[%q] from %v", firstID, before)
	}

	ctx.app.macAppIndex = 3
	_, cmd := screen.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("esc should return a navigation command")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenDeepDiveMenu {
		t.Errorf("esc should NavigateTo(ScreenDeepDiveMenu), got %#v", cmd())
	}
	if ctx.app.macAppIndex != 0 {
		t.Errorf("macAppIndex = %d, want 0 reset on back", ctx.app.macAppIndex)
	}
}

// TestConfigGhosttyReachableViaManager verifies the navigation backbone: an App
// built with the ScreenManager enters managed mode on
// NavigateTo(ScreenConfigGhostty) and renders the Ghostty config through the
// factory.
func TestConfigGhosttyReachableViaManager(t *testing.T) {
	app := NewApp(true, WithScreenFactory())
	if app.screenMgr == nil {
		t.Fatal("WithScreenFactory should initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	// Pre-populate cache so any install-aware screens render deterministically.
	app.manageInstalled = map[string]bool{}
	app.manageInstalledReady = true

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenConfigGhostty)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenConfigGhostty")
	}
	if app.screenMgr.IsLegacyMode() {
		t.Fatal("manager should be in managed mode after navigating to ScreenConfigGhostty")
	}

	view := app.screenMgr.View()
	if !strings.Contains(view, "Font Family") {
		t.Errorf("managed configGhosttyScreen should render Ghostty config\n---\n%s\n---", view)
	}

	// DeepDiveMenu should likewise be reachable through the manager.
	if _, handled := app.screenMgr.Update(NavigateTo(ScreenDeepDiveMenu)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenDeepDiveMenu")
	}
	if mv := app.screenMgr.View(); !strings.Contains(mv, "DEEP DIVE CONFIGURATION") {
		t.Errorf("managed deepDiveMenuScreen should render menu\n---\n%s\n---", mv)
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
