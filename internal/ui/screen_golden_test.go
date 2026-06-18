package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

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

// TestConfigCLIToolsScreenGolden is a regression guard for the migrated CLI
// tools selection screen. The install cache is pre-populated so the render does
// not depend on the host's package manager.
func TestConfigCLIToolsScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.cliToolIndex = 0
	screen := NewConfigCLIToolsScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configCLIToolsScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"CLI Tools",
		"LazyGit",
		"LazyDocker",
		"btop",
		"Glow",
		"Claude Code", // context row, rendered but not navigable
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configCLIToolsScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigCLITools {
		t.Errorf("configCLIToolsScreen.ID() = %v, want ScreenConfigCLITools", screen.ID())
	}
}

// TestConfigCLIToolsToggleAndBack verifies list navigation: space toggles the
// focused (not-installed) tool, the cursor cannot reach the claude-code context
// row, and esc resets the index and navigates back.
func TestConfigCLIToolsToggleAndBack(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.cliToolIndex = 0
	firstID := cliToolItems[0].id
	before := ctx.app.deepDiveConfig.CLITools[firstID]

	screen := NewConfigCLIToolsScreen(ctx)
	if _, _ = screen.Update(keyMsg(" ")); ctx.app.deepDiveConfig.CLITools[firstID] == before {
		t.Errorf("space should toggle CLITools[%q] from %v", firstID, before)
	}

	// Down should stop at the last navigable tool (index navigableCLIToolCount-1),
	// never reaching the claude-code context row.
	for i := 0; i < len(cliToolItems)+2; i++ {
		_, _ = screen.Update(keyMsg("down"))
	}
	if ctx.app.cliToolIndex != navigableCLIToolCount-1 {
		t.Errorf("cliToolIndex = %d, want %d (cursor must not reach claude-code row)",
			ctx.app.cliToolIndex, navigableCLIToolCount-1)
	}

	_, cmd := screen.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("esc should return a navigation command")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenDeepDiveMenu {
		t.Errorf("esc should NavigateTo(ScreenDeepDiveMenu), got %#v", cmd())
	}
	if ctx.app.cliToolIndex != 0 {
		t.Errorf("cliToolIndex = %d, want 0 reset on back", ctx.app.cliToolIndex)
	}
}

// TestConfigCLIUtilitiesScreenGolden is a regression guard for the migrated CLI
// utilities checkbox-list screen.
func TestConfigCLIUtilitiesScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.cliUtilityIndex = 0
	screen := NewConfigCLIUtilitiesScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configCLIUtilitiesScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"CLI Utilities",
		"bat",
		"eza",
		"zoxide",
		"ripgrep",
		"fswatch",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configCLIUtilitiesScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigCLIUtilities {
		t.Errorf("configCLIUtilitiesScreen.ID() = %v, want ScreenConfigCLIUtilities", screen.ID())
	}
}

// TestConfigGUIAppsScreenGolden is a regression guard for the migrated GUI apps
// checkbox-list screen.
func TestConfigGUIAppsScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.guiAppIndex = 0
	screen := NewConfigGUIAppsScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configGUIAppsScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"GUI Apps",
		"Zen Browser",
		"Cursor",
		"OBS Studio",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configGUIAppsScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigGUIApps {
		t.Errorf("configGUIAppsScreen.ID() = %v, want ScreenConfigGUIApps", screen.ID())
	}
}

// TestConfigLazyGitScreenGolden is a regression guard for the migrated LazyGit
// field-based config screen.
func TestConfigLazyGitScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigLazyGitScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configLazyGitScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"LazyGit",
		"Side-by-Side Diff",
		"Mouse Mode",
		"Theme",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configLazyGitScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigLazyGit {
		t.Errorf("configLazyGitScreen.ID() = %v, want ScreenConfigLazyGit", screen.ID())
	}
}

// TestConfigLazyGitToggleAndBack verifies the shared field navigation: space
// toggles the focused boolean field and esc navigates back, resetting the index.
func TestConfigLazyGitToggleAndBack(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.configFieldIndex = 0 // Side-by-Side Diff
	before := ctx.app.deepDiveConfig.LazyGitSideBySide

	screen := NewConfigLazyGitScreen(ctx)
	if _, _ = screen.Update(keyMsg(" ")); ctx.app.deepDiveConfig.LazyGitSideBySide == before {
		t.Errorf("space should toggle LazyGitSideBySide from %v", before)
	}

	// Move to the Theme field and adjust it with 'right'.
	ctx.app.configFieldIndex = 2
	themeBefore := ctx.app.deepDiveConfig.LazyGitTheme
	if _, _ = screen.Update(keyMsg("right")); ctx.app.deepDiveConfig.LazyGitTheme == themeBefore {
		t.Errorf("right should cycle LazyGitTheme from %q", themeBefore)
	}

	_, cmd := screen.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("esc should return a navigation command")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenDeepDiveMenu {
		t.Errorf("esc should NavigateTo(ScreenDeepDiveMenu), got %#v", cmd())
	}
	if ctx.app.configFieldIndex != 0 {
		t.Errorf("configFieldIndex = %d, want 0 reset on back", ctx.app.configFieldIndex)
	}
}

// TestConfigBtopScreenGolden is a regression guard for the migrated Btop
// field-based config screen.
func TestConfigBtopScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigBtopScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configBtopScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Btop",
		"Theme",
		"Update Interval",
		"Show CPU Temp",
		"Graph Type",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configBtopScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigBtop {
		t.Errorf("configBtopScreen.ID() = %v, want ScreenConfigBtop", screen.ID())
	}
}

// TestConfigGlowScreenGolden is a regression guard for the migrated Glow
// field-based config screen.
func TestConfigGlowScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigGlowScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configGlowScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Glow",
		"Style",
		"Pager",
		"Width",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configGlowScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigGlow {
		t.Errorf("configGlowScreen.ID() = %v, want ScreenConfigGlow", screen.ID())
	}
}

// TestConfigClaudeCodeScreenGolden is a regression guard for the migrated Claude
// Code MCP configuration screen.
func TestConfigClaudeCodeScreenGolden(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.configFieldIndex = -1 // install toggle focused
	screen := NewConfigClaudeCodeScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("configClaudeCodeScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Claude Code",
		"Install Claude Code",
		"MCP Servers",
		"Context7",
		"(recommended)",
		"Task Master",
		"Sequential Thinking",
		"navigate",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("configClaudeCodeScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenConfigClaudeCode {
		t.Errorf("configClaudeCodeScreen.ID() = %v, want ScreenConfigClaudeCode", screen.ID())
	}
}

// TestConfigClaudeCodeToggleAndBack verifies the custom navigation: focus index
// -1 toggles the install flag, MCP indices toggle their server, navigation stops
// at -1 (up) and len-1 (down), and esc resets the index and navigates back.
func TestConfigClaudeCodeToggleAndBack(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigClaudeCodeScreen(ctx)

	// Index -1: space toggles the Claude Code install flag.
	ctx.app.configFieldIndex = -1
	installBefore := ctx.app.deepDiveConfig.CLITools["claude-code"]
	if _, _ = screen.Update(keyMsg(" ")); ctx.app.deepDiveConfig.CLITools["claude-code"] == installBefore {
		t.Errorf("space at index -1 should toggle CLITools[claude-code] from %v", installBefore)
	}

	// 'up' should not go below -1.
	if _, _ = screen.Update(keyMsg("up")); ctx.app.configFieldIndex != -1 {
		t.Errorf("up at index -1 should stay at -1, got %d", ctx.app.configFieldIndex)
	}

	// Index 0: space toggles the first MCP server (context7).
	ctx.app.configFieldIndex = 0
	mcpID := claudeCodeMCPItems[0].id
	mcpBefore := ctx.app.deepDiveConfig.ClaudeCodeMCPs[mcpID]
	if _, _ = screen.Update(keyMsg(" ")); ctx.app.deepDiveConfig.ClaudeCodeMCPs[mcpID] == mcpBefore {
		t.Errorf("space at index 0 should toggle ClaudeCodeMCPs[%q] from %v", mcpID, mcpBefore)
	}

	// 'down' should not exceed the last MCP index.
	for i := 0; i < len(claudeCodeMCPItems)+3; i++ {
		_, _ = screen.Update(keyMsg("down"))
	}
	if ctx.app.configFieldIndex != len(claudeCodeMCPItems)-1 {
		t.Errorf("down should cap at %d, got %d", len(claudeCodeMCPItems)-1, ctx.app.configFieldIndex)
	}

	_, cmd := screen.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("esc should return a navigation command")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenDeepDiveMenu {
		t.Errorf("esc should NavigateTo(ScreenDeepDiveMenu), got %#v", cmd())
	}
	if ctx.app.configFieldIndex != 0 {
		t.Errorf("configFieldIndex = %d, want 0 reset on back", ctx.app.configFieldIndex)
	}
}

// TestConfigClaudeCodeReachableViaManager verifies the navigation backbone: an
// App built with the ScreenManager enters managed mode on
// NavigateTo(ScreenConfigClaudeCode) and renders the Claude Code MCP config
// through the factory.
func TestConfigClaudeCodeReachableViaManager(t *testing.T) {
	app := NewApp(true, WithScreenFactory())
	if app.screenMgr == nil {
		t.Fatal("WithScreenFactory should initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	app.manageInstalled = map[string]bool{}
	app.manageInstalledReady = true

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenConfigClaudeCode)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenConfigClaudeCode")
	}
	if app.screenMgr.IsLegacyMode() {
		t.Fatal("manager should be in managed mode after navigating to ScreenConfigClaudeCode")
	}

	view := app.screenMgr.View()
	for _, want := range []string{"Claude Code", "MCP Servers", "Context7"} {
		if !strings.Contains(view, want) {
			t.Errorf("managed configClaudeCodeScreen should render %q\n---\n%s\n---", want, view)
		}
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

// ============================================================================
// Migrated management screens: Hotkeys, Backups, Update
// ============================================================================

// TestHotkeysScreenGolden is a regression guard for the migrated hotkeysScreen.
// It renders the dual-pane viewer (categories + items + footer help) and matches
// the screen ID.
func TestHotkeysScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)

	screen := NewHotkeysScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("hotkeysScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"CATEGORIES",
		"ITEMS",
		"favorite", // footer help: "f favorite"
		"Esc back",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("hotkeysScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenHotkeys {
		t.Errorf("hotkeysScreen.ID() = %v, want ScreenHotkeys", screen.ID())
	}

	// Hotkeys has no async work on entry.
	if cmd := screen.Init(); cmd != nil {
		t.Errorf("hotkeysScreen.Init() should be nil (favorites load in NewApp)")
	}
}

// TestHotkeysScreenEscNavigatesBack verifies esc routes back through the
// ScreenManager to the recorded return screen (the main menu by default).
func TestHotkeysScreenEscNavigatesBack(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.hotkeysReturn = ScreenMainMenu

	screen := NewHotkeysScreen(ctx)
	_, cmd := screen.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("esc should return a navigation command")
	}
	nav, ok := cmd().(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg from esc, got %T", cmd())
	}
	if nav.To != ScreenMainMenu {
		t.Errorf("esc should NavigateTo(ScreenMainMenu), got %v", nav.To)
	}
}

// TestBackupsScreenEmptyGolden is a regression guard for the migrated
// backupsScreen with an empty (loaded) backup list.
func TestBackupsScreenEmptyGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	// Loaded with zero backups (deterministic; no disk access).
	ctx.app.backupsLoading = false
	ctx.app.backupsLoaded = true
	ctx.app.backups = []BackupEntry{}

	screen := NewBackupsScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("backupsScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Backups",
		"0 backup(s) available",
		"No backups found.",
		"new backup",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("backupsScreen.View() (empty) missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenBackups {
		t.Errorf("backupsScreen.ID() = %v, want ScreenBackups", screen.ID())
	}
}

// TestBackupsScreenPopulatedGolden verifies the populated list renders the
// backup rows, the count, and the details panel.
func TestBackupsScreenPopulatedGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.backupsLoading = false
	ctx.app.backupsLoaded = true
	ctx.app.backupIndex = 0
	ctx.app.backups = []BackupEntry{
		{Name: "2026-06-18_alpha", Timestamp: time.Date(2026, 6, 18, 10, 30, 0, 0, time.UTC), FileCount: 5, Size: 2048, Path: "/tmp/backups/alpha"},
		{Name: "2026-06-17_bravo", Timestamp: time.Date(2026, 6, 17, 9, 15, 0, 0, time.UTC), FileCount: 3, Size: 1024, Path: "/tmp/backups/bravo"},
	}

	screen := NewBackupsScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	wantSubstrings := []string{
		"2 backup(s) available",
		"2026-06-18_alpha",
		"2026-06-17_bravo",
		"DETAILS",
		"Files:",
		"enter restore",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("backupsScreen.View() (populated) missing %q\n---\n%s\n---", want, out)
		}
	}
}

// TestBackupsScreenAsyncInHandler proves the async-in-handler wiring: feeding a
// backupsLoadedMsg to the handler's Update updates App state and the rendered
// list, with no App.Update involvement.
func TestBackupsScreenAsyncInHandler(t *testing.T) {
	ctx := newGoldenContext(t)
	// Start in the loading state (as Init() would leave it).
	ctx.app.backupsLoading = true
	ctx.app.backupsLoaded = false
	ctx.app.backups = nil

	screen := NewBackupsScreen(ctx)

	// Before the async result lands, the view shows the loading spinner.
	if before := screen.View(ctx.Width, ctx.Height); !strings.Contains(before, "Loading backups") {
		t.Errorf("backupsScreen.View() should show loading state before backupsLoadedMsg\n---\n%s\n---", before)
	}

	// Deliver the async result directly to the handler.
	loaded := backupsLoadedMsg{backups: []BackupEntry{
		{Name: "async-backup", Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), FileCount: 1, Size: 512, Path: "/tmp/backups/async"},
	}}
	next, _ := screen.Update(loaded)
	if next != screen {
		t.Fatalf("backupsScreen should remain current after backupsLoadedMsg")
	}

	if ctx.app.backupsLoading {
		t.Error("backupsLoadedMsg should clear backupsLoading")
	}
	if !ctx.app.backupsLoaded {
		t.Error("backupsLoadedMsg should set backupsLoaded")
	}
	if len(ctx.app.backups) != 1 || ctx.app.backups[0].Name != "async-backup" {
		t.Fatalf("backupsLoadedMsg should populate backups, got %+v", ctx.app.backups)
	}

	// The rendered list now reflects the async result.
	out := screen.View(ctx.Width, ctx.Height)
	if !strings.Contains(out, "async-backup") {
		t.Errorf("backupsScreen.View() should render the async-loaded backup\n---\n%s\n---", out)
	}
	if !strings.Contains(out, "1 backup(s) available") {
		t.Errorf("backupsScreen.View() should show the updated count\n---\n%s\n---", out)
	}
}

// TestBackupsScreenInitLoads verifies Init() kicks the backup load when not yet
// loaded, and is idempotent when already loading/loaded.
func TestBackupsScreenInitLoads(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.backupsLoading = false
	ctx.app.backupsLoaded = false

	screen := NewBackupsScreen(ctx)
	if cmd := screen.Init(); cmd == nil {
		t.Fatal("backupsScreen.Init() should return loadBackupsCmd when not loaded")
	}
	if !ctx.app.backupsLoading {
		t.Error("backupsScreen.Init() should set backupsLoading")
	}

	// Idempotent: already loading -> no new command.
	if cmd := screen.Init(); cmd != nil {
		t.Error("backupsScreen.Init() should be a no-op while already loading")
	}
}

// TestUpdateScreenCheckingGolden is a regression guard for the migrated
// updateScreen in its (host-independent) checking state.
func TestUpdateScreenCheckingGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateChecking = true
	ctx.app.updateCheckDone = false

	screen := NewUpdateScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("updateScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"Package Updates",
		"Checking for updates",
		"switch tabs",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("updateScreen.View() (checking) missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenUpdate {
		t.Errorf("updateScreen.ID() = %v, want ScreenUpdate", screen.ID())
	}
}

// TestUpdateScreenNoUpdatesGolden verifies the no-updates / no-results state.
// The body depends on whether the host has a package manager, so accept either
// the "up to date" message (manager present) or the "no package manager" message
// (manager absent) — both are valid deterministic renders of an empty result set.
func TestUpdateScreenNoUpdatesGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateChecking = false
	ctx.app.updateCheckDone = true
	ctx.app.updateError = nil
	ctx.app.updateResults = nil

	screen := NewUpdateScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if !strings.Contains(out, "Package Updates") {
		t.Errorf("updateScreen.View() should render the title\n---\n%s\n---", out)
	}
	if !strings.Contains(out, "All packages are up to date!") &&
		!strings.Contains(out, "No package manager detected") {
		t.Errorf("updateScreen.View() (no updates) should show the up-to-date or no-manager body\n---\n%s\n---", out)
	}
}

// TestUpdateScreenCheckDoneAsyncInHandler proves the async-in-handler wiring for
// the update check: feeding an updateCheckDoneMsg updates App state directly.
func TestUpdateScreenCheckDoneAsyncInHandler(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateChecking = true
	ctx.app.updateCheckDone = false

	screen := NewUpdateScreen(ctx)
	next, _ := screen.Update(updateCheckDoneMsg{updates: nil, err: nil})
	if next != screen {
		t.Fatalf("updateScreen should remain current after updateCheckDoneMsg")
	}
	if ctx.app.updateChecking {
		t.Error("updateCheckDoneMsg should clear updateChecking")
	}
	if !ctx.app.updateCheckDone {
		t.Error("updateCheckDoneMsg should set updateCheckDone")
	}
}

// TestUpdateScreenInitChecks verifies Init() kicks the update check when not yet
// done, and is idempotent when already checking/done.
func TestUpdateScreenInitChecks(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateChecking = false
	ctx.app.updateCheckDone = false

	screen := NewUpdateScreen(ctx)
	if cmd := screen.Init(); cmd == nil {
		t.Fatal("updateScreen.Init() should return checkUpdatesCmd when not done")
	}
	if !ctx.app.updateChecking {
		t.Error("updateScreen.Init() should set updateChecking")
	}

	// Idempotent: already checking -> no new command.
	if cmd := screen.Init(); cmd != nil {
		t.Error("updateScreen.Init() should be a no-op while already checking")
	}
}

// TestManagementScreensReachableViaManager verifies the navigation backbone for
// the three migrated management screens: an App built with the ScreenManager
// enters managed mode on NavigateTo and renders each through the factory.
func TestManagementScreensReachableViaManager(t *testing.T) {
	app := NewApp(true, WithScreenFactory())
	if app.screenMgr == nil {
		t.Fatal("WithScreenFactory should initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)

	cases := []struct {
		name   string
		target Screen
		// substr must appear in the managed render; for screens whose populated
		// body depends on async loads, we use a header/tab substring that is
		// always present.
		substr string
		setup  func()
	}{
		{
			name:   "Hotkeys",
			target: ScreenHotkeys,
			substr: "CATEGORIES",
		},
		{
			name:   "Backups",
			target: ScreenBackups,
			substr: "Backups",
			setup: func() {
				// Mark as loaded with an empty list so the render is deterministic
				// (skips the loading spinner) and host-independent.
				app.backupsLoading = false
				app.backupsLoaded = true
				app.backups = []BackupEntry{}
			},
		},
		{
			name:   "Update",
			target: ScreenUpdate,
			substr: "Package Updates",
			setup: func() {
				// Force the checking state so the render does not depend on the
				// host's package manager.
				app.updateChecking = true
				app.updateCheckDone = false
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup()
			}
			if _, handled := app.screenMgr.Update(NavigateTo(tc.target)()); !handled {
				t.Fatalf("manager should handle NavigateMsg to %v", tc.target)
			}
			if app.screenMgr.IsLegacyMode() {
				t.Fatalf("manager should be in managed mode after navigating to %v", tc.target)
			}
			if app.screenMgr.Current().ID() != tc.target {
				t.Fatalf("manager current screen ID = %v, want %v", app.screenMgr.Current().ID(), tc.target)
			}
			view := app.screenMgr.View()
			if !strings.Contains(view, tc.substr) {
				t.Errorf("managed %s screen should render %q\n---\n%s\n---", tc.name, tc.substr, view)
			}
		})
	}
}
