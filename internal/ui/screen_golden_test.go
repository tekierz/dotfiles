package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
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

	// Hermetic by construction: point HOME at a temp dir BEFORE NewApp so the
	// app's startup config load (and any persistTheme / SaveToolConfig a handler
	// under test triggers) reads and writes the temp dir, never the developer's
	// real ~/.config/dotfiles. This makes every golden/handler test that goes
	// through newGoldenContext safe to run with no writable real HOME (P1-C).
	withTempHome(t)

	app := NewApp(true)

	ctx := &ScreenContext{
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
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	// Pre-populate cache so any install-aware screens render deterministically.
	app.manageInstalled = map[string]bool{}
	app.manageInstalledReady = true

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenConfigGhostty)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenConfigGhostty")
	}
	if app.screenMgr.Current() == nil {
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
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	app.manageInstalled = map[string]bool{}
	app.manageInstalledReady = true

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenConfigClaudeCode)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenConfigClaudeCode")
	}
	if app.screenMgr.Current() == nil {
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
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)

	cmd := NavigateTo(ScreenWelcome)
	if _, handled := app.screenMgr.Update(cmd()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenWelcome")
	}
	if app.screenMgr.Current() == nil {
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
	if app.screenMgr.Current() == nil {
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
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	if app.screenFactory == nil {
		t.Fatal("NewApp should always initialize screenFactory")
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
	if app.screenMgr.Current() == nil {
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

// TestUpdateScreenStreamLineLive proves the live-streaming wiring: a streamed
// update line is appended to installLogs immediately (so it renders during the
// run, not all at once at the end) and the global handler re-arms the listen
// Cmd to keep the stream flowing. The streaming messages are now handled in
// App.Update (so they survive navigation), so the test drives App.Update.
func TestUpdateScreenStreamLineLive(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateRunning = true
	ctx.app.updateStream = make(chan updateStreamMsg, 1)

	_, cmd := ctx.app.Update(updateStreamMsg{line: "==> Downloading foo"})
	if len(ctx.app.installLogs) != 1 || ctx.app.installLogs[0] != "==> Downloading foo" {
		t.Errorf("streamed line should be appended to installLogs immediately, got %v", ctx.app.installLogs)
	}
	if !ctx.app.updateRunning {
		t.Error("a non-terminal streamed line must not end the run")
	}
	if cmd == nil {
		t.Fatal("a non-terminal streamed line should re-arm the listen Cmd")
	}
}

// TestUpdateScreenStreamDoneFinalizes verifies a terminal streamed event ends the
// run, clears the stream channel, and sets the status from the results. Handled
// globally in App.Update.
func TestUpdateScreenStreamDoneFinalizes(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.updateRunning = true
	ctx.app.updateStream = make(chan updateStreamMsg, 1)

	results := []pkg.UpdateResult{{Success: true}}
	ctx.app.Update(updateStreamMsg{done: true, results: results})
	if ctx.app.updateRunning {
		t.Error("a terminal streamed event should clear updateRunning")
	}
	if ctx.app.updateStream != nil {
		t.Error("a terminal streamed event should clear the update stream channel")
	}
	if !strings.Contains(ctx.app.updateStatus, "Updated 1") {
		t.Errorf("updateStatus should report the success count, got %q", ctx.app.updateStatus)
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

// ============================================================================
// Migrated management screen: Manage (live dual-pane)
// ============================================================================

// newManageContext builds a deterministic ScreenContext for the Manage dual-pane
// screen: a fresh NewManageConfig() (provided by NewApp), a seeded
// manageInstalled map, the cache marked ready (so the View renders the panes,
// not the loading spinner), and a non-zero window size on the App (the dual-pane
// layout reads a.width/a.height). Animations are off via newGoldenContext.
func newManageContext(t *testing.T) *ScreenContext {
	t.Helper()
	ctx := newGoldenContext(t)
	// NewApp sets a NewManageConfig(); assert it for clarity.
	if ctx.app.manageConfig == nil {
		ctx.app.manageConfig = NewManageConfig()
	}
	// Seeded install-status cache (deterministic; no package-manager calls).
	ctx.app.manageInstalled = map[string]bool{
		"ghostty": true,
		"tmux":    true,
	}
	ctx.app.manageInstalledReady = true
	ctx.app.installCacheLoading = false
	// The dual-pane layout/render reads a.width/a.height directly.
	ctx.app.width = ctx.Width
	ctx.app.height = ctx.Height
	return ctx
}

// TestManageScreenGolden is a regression guard for the migrated manageScreen.
// It renders the dual-pane editor: the tools pane (TOOLS header + Global entry)
// and the settings pane (SETTINGS header + the global Theme/Navigation/Animations
// fields, since the Global entry is selected by default at index 0).
func TestManageScreenGolden(t *testing.T) {
	ctx := newManageContext(t)
	ctx.app.manageIndex = 0 // Global entry selected -> global fields shown.
	ctx.app.managePane = managePaneSettings

	screen := NewManageScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("manageScreen.View() returned empty output")
	}

	wantSubstrings := []string{
		"TOOLS",      // left pane header
		"Global",     // left pane: the global entry
		"SETTINGS",   // right pane header
		"Theme",      // global field
		"Navigation", // global field
		"Animations", // global field
		"Manage",     // tab bar
		"Esc back",   // footer hint
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("manageScreen.View() missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenManage {
		t.Errorf("manageScreen.ID() = %v, want ScreenManage", screen.ID())
	}
}

// TestManageScreenLoadingGolden verifies the loading spinner is shown while the
// install-status cache is still populating.
func TestManageScreenLoadingGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width = ctx.Width
	ctx.app.height = ctx.Height
	ctx.app.installCacheLoading = true

	screen := NewManageScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if !strings.Contains(out, "Loading installation status") {
		t.Errorf("manageScreen.View() should show loading state\n---\n%s\n---", out)
	}
}

// TestManageScreenInstalledBadge verifies the seeded manageInstalled map drives
// the settings-pane render: selecting an installed tool (ghostty) shows its
// INSTALLED badge and its fields. Uses a wide window so the settings header
// (name + description + badge) is not truncated by the narrow-pane layout.
func TestManageScreenInstalledBadge(t *testing.T) {
	ctx := newManageContext(t)
	// Wide window so the badge fits; the dual-pane layout reads a.width/a.height.
	const w, h = 140, 40
	ctx.Width, ctx.Height = w, h
	ctx.app.width, ctx.app.height = w, h

	screen := NewManageScreen(ctx)

	// Navigate the tools cursor to the ghostty entry (seeded installed). Items are
	// category-sorted with Global first; find ghostty's index from manageItems.
	items := ctx.app.manageItems()
	ghosttyIdx := -1
	for i, it := range items {
		if it.id == "ghostty" {
			ghosttyIdx = i
			break
		}
	}
	if ghosttyIdx < 0 {
		t.Skip("ghostty not present on this platform's tool list; skipping badge check")
	}
	ctx.app.manageIndex = ghosttyIdx
	ctx.app.managePane = managePaneSettings

	out := screen.View(w, h)
	if !strings.Contains(out, "INSTALLED") {
		t.Errorf("manageScreen.View() should show INSTALLED badge for seeded-installed ghostty\n---\n%s\n---", out)
	}
	// Ghostty exposes a Font Family field in the manager.
	if !strings.Contains(out, "Font Family") {
		t.Errorf("manageScreen.View() should render ghostty fields\n---\n%s\n---", out)
	}
}

// TestManageScreenReachableViaManager verifies the navigation backbone: an App
// built with the ScreenManager enters managed mode on NavigateTo(ScreenManage)
// and renders the dual-pane through the factory.
func TestManageScreenReachableViaManager(t *testing.T) {
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	app.width, app.height = 80, 24
	// Seed the cache so the render shows the panes, not the loading spinner.
	app.manageInstalled = map[string]bool{"ghostty": true}
	app.manageInstalledReady = true
	app.installCacheLoading = false

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenManage)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenManage")
	}
	if app.screenMgr.Current() == nil {
		t.Fatal("manager should be in managed mode after navigating to ScreenManage")
	}
	if app.screenMgr.Current().ID() != ScreenManage {
		t.Fatalf("manager current screen ID = %v, want ScreenManage", app.screenMgr.Current().ID())
	}

	view := app.screenMgr.View()
	for _, want := range []string{"TOOLS", "SETTINGS", "Global"} {
		if !strings.Contains(view, want) {
			t.Errorf("managed manageScreen should render %q\n---\n%s\n---", want, view)
		}
	}
}

// TestManageScreenInstallDoneAsyncInHandler proves the global-handler wiring for
// the manage install completion: feeding a manageInstallDoneMsg to App.Update
// updates App state and requests an install-status cache reload (Phase B + C10
// fix). The streaming/terminal install messages are handled globally in
// App.Update so they survive navigation, so the test drives App.Update.
func TestManageScreenInstallDoneAsyncInHandler(t *testing.T) {
	ctx := newManageContext(t)
	// Simulate an install in progress with a ready cache (so the reload is the
	// one triggered by the completion, not a pre-existing load).
	ctx.app.manageInstalling = true
	ctx.app.manageInstallID = "ghostty"
	ctx.app.manageInstalledReady = true
	ctx.app.installCacheLoading = false

	_, cmd := ctx.app.Update(manageInstallDoneMsg{toolID: "ghostty", err: nil})

	if ctx.app.manageInstalling {
		t.Error("manageInstallDoneMsg should clear manageInstalling")
	}
	if ctx.app.manageInstallID != "" {
		t.Errorf("manageInstallDoneMsg should clear manageInstallID, got %q", ctx.app.manageInstallID)
	}
	if ctx.app.manageStatus != "Installed ✓" {
		t.Errorf("manageStatus = %q, want \"Installed ✓\"", ctx.app.manageStatus)
	}
	// Phase B fix: completion must invalidate the cache and re-issue the reload.
	if ctx.app.manageInstalledReady {
		t.Error("manageInstallDoneMsg should set manageInstalledReady=false to force a refresh")
	}
	if cmd == nil {
		t.Fatal("manageInstallDoneMsg should return a cache-reload command (startInstallCacheLoad)")
	}
	if !ctx.app.installCacheLoading {
		t.Error("manageInstallDoneMsg should kick the install-cache reload (installCacheLoading=true)")
	}
}

// TestManageScreenInstallWithLogsAsyncInHandler proves the streaming-install
// terminal message is handled globally in App.Update: manageInstallWithLogsMsg
// appends the collected logs, clears the installing flag, and (on success)
// requests a cache reload. Handled in App.Update so it survives navigation.
func TestManageScreenInstallWithLogsAsyncInHandler(t *testing.T) {
	ctx := newManageContext(t)
	ctx.app.manageInstalling = true
	ctx.app.manageInstallID = "ghostty"
	ctx.app.manageInstalledReady = true
	ctx.app.installCacheLoading = false
	ctx.app.clearInstallLogs()

	_, cmd := ctx.app.Update(manageInstallWithLogsMsg{
		toolID: "ghostty",
		logs:   []string{"Installing ghostty...", "done"},
		err:    nil,
	})
	if ctx.app.manageInstalling {
		t.Error("manageInstallWithLogsMsg should clear manageInstalling")
	}
	if len(ctx.app.installLogs) != 2 || ctx.app.installLogs[0] != "Installing ghostty..." {
		t.Errorf("manageInstallWithLogsMsg should append collected logs, got %v", ctx.app.installLogs)
	}
	if !strings.Contains(ctx.app.manageStatus, "Installed successfully") {
		t.Errorf("manageStatus = %q, want a success message", ctx.app.manageStatus)
	}
	if ctx.app.manageInstalledReady {
		t.Error("manageInstallWithLogsMsg success should set manageInstalledReady=false")
	}
	if cmd == nil {
		t.Fatal("manageInstallWithLogsMsg success should return a cache-reload command")
	}
}

// TestManageScreenEscNavigatesToMainMenu verifies esc routes back through the
// ScreenManager to the main menu and resets the pane/status.
func TestManageScreenEscNavigatesToMainMenu(t *testing.T) {
	ctx := newManageContext(t)
	ctx.app.managePane = managePaneSettings
	ctx.app.manageStatus = "something"

	screen := NewManageScreen(ctx)
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
	if ctx.app.managePane != managePaneTools {
		t.Errorf("esc should reset managePane to tools, got %d", ctx.app.managePane)
	}
	if ctx.app.manageStatus != "" {
		t.Errorf("esc should clear manageStatus, got %q", ctx.app.manageStatus)
	}
}

// TestManageScreenInitLoadsCache verifies Init() kicks the install-status cache
// load when not ready, and is idempotent when already ready/loading.
func TestManageScreenInitLoadsCache(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.manageInstalledReady = false
	ctx.app.installCacheLoading = false

	screen := NewManageScreen(ctx)
	if cmd := screen.Init(); cmd == nil {
		t.Fatal("manageScreen.Init() should return startInstallCacheLoad when cache not ready")
	}
	if !ctx.app.installCacheLoading {
		t.Error("manageScreen.Init() should set installCacheLoading")
	}
	// Idempotent: already loading -> no new command.
	if cmd := screen.Init(); cmd != nil {
		t.Error("manageScreen.Init() should be a no-op while the cache is already loading")
	}
}

// TestManagementScreensReachableViaManager verifies the navigation backbone for
// the three migrated management screens: an App built with the ScreenManager
// enters managed mode on NavigateTo and renders each through the factory.
func TestManagementScreensReachableViaManager(t *testing.T) {
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
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
			if app.screenMgr.Current() == nil {
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

// ============================================================================
// Migrated wizard screens: Animation, Progress
// ============================================================================

// TestAnimationScreenGolden is a regression guard for the migrated
// animationScreen. With a window size set on the App it renders the intro card
// deterministically (animations off in the context is irrelevant here: the intro
// frame is driven by App.animFrame, which we fix). It must render the banner and
// skip hint, and never panic.
func TestAnimationScreenGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	// The intro layout reads a.width/a.height directly.
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.animFrame = 36 // a fixed mid-animation frame for determinism

	screen := NewAnimationScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("animationScreen.View() returned empty output")
	}
	// The skip hint is always rendered; assert it so we know the card drew.
	if !strings.Contains(out, "skip") {
		t.Errorf("animationScreen.View() should render the skip hint\n---\n%s\n---", out)
	}

	if screen.ID() != ScreenAnimation {
		t.Errorf("animationScreen.ID() = %v, want ScreenAnimation", screen.ID())
	}
}

// TestAnimationScreenNoSizeLoading verifies the "no window size yet" fallback
// renders the loading placeholder rather than dividing by a zero-size layout.
func TestAnimationScreenNoSizeLoading(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = 0, 0

	screen := NewAnimationScreen(ctx)
	if out := screen.View(ctx.Width, ctx.Height); !strings.Contains(out, "Loading") {
		t.Errorf("animationScreen.View() with no size should show loading placeholder, got %q", out)
	}
}

// TestAnimationScreenTickAdvancesFrame proves the intro tick logic now lives in
// the handler: a tickMsg advances animFrame and re-issues the tick (a non-nil
// continuation) until the final frame, where it transitions instead.
func TestAnimationScreenTickAdvancesFrame(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.animFrame = 0

	screen := NewAnimationScreen(ctx)

	// A mid-animation tick advances the frame and re-arms the tick.
	next, cmd := screen.Update(tickMsg(time.Now()))
	if next != screen {
		t.Fatalf("animationScreen should remain current on a mid-animation tick")
	}
	if ctx.app.animFrame != 1 {
		t.Errorf("animFrame = %d, want 1 after one tickMsg", ctx.app.animFrame)
	}
	if cmd == nil {
		t.Fatal("mid-animation tickMsg should return a non-nil continuation (tickAnimation)")
	}

	// At the final frame, the tick transitions via postIntroTransition (which sets
	// animationDone and routes to the post-intro screen through the ScreenManager).
	// Assert the state transition (animationDone) rather than the returned command.
	ctx.app.animFrame = introAnimationFrames
	ctx.app.animationDone = false
	screen.Update(tickMsg(time.Now()))
	if !ctx.app.animationDone {
		t.Error("final-frame tickMsg should trigger the post-intro transition (animationDone=true)")
	}
}

// TestAnimationScreenTickNoSizeHolds verifies the guard: a tickMsg before a
// window size is known does NOT advance the frame (prevents fast-forward).
func TestAnimationScreenTickNoSizeHolds(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = 0, 0
	ctx.app.animFrame = 0

	screen := NewAnimationScreen(ctx)
	_, cmd := screen.Update(tickMsg(time.Now()))
	if ctx.app.animFrame != 0 {
		t.Errorf("animFrame = %d, want 0 (must not advance before a window size)", ctx.app.animFrame)
	}
	if cmd == nil {
		t.Error("tickMsg with no size should still re-arm the tick")
	}
}

// TestAnimationScreenInit verifies Init() issues the intro tick.
func TestAnimationScreenInit(t *testing.T) {
	ctx := newGoldenContext(t)
	screen := NewAnimationScreen(ctx)
	if cmd := screen.Init(); cmd == nil {
		t.Fatal("animationScreen.Init() should return the intro tick command")
	}
}

// TestAnimationScreenReachableViaManager verifies an App built with the manager
// enters managed mode on NavigateTo(ScreenAnimation) and renders the intro.
func TestAnimationScreenReachableViaManager(t *testing.T) {
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	app.width, app.height = 80, 24
	app.animFrame = 10

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenAnimation)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenAnimation")
	}
	if app.screenMgr.Current() == nil {
		t.Fatal("manager should be in managed mode after navigating to ScreenAnimation")
	}
	if app.screenMgr.Current().ID() != ScreenAnimation {
		t.Fatalf("manager current screen ID = %v, want ScreenAnimation", app.screenMgr.Current().ID())
	}
	if view := app.screenMgr.View(); !strings.Contains(view, "skip") {
		t.Errorf("managed animationScreen should render the intro card\n---\n%s\n---", view)
	}
}

// TestProgressScreenNotStartedGolden is a regression guard for the migrated
// progressScreen before the install begins: it shows the step list and the
// "Press ENTER to start" prompt.
func TestProgressScreenNotStartedGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.installRunning = false
	ctx.app.installComplete = false
	ctx.app.installOutput = nil

	screen := NewProgressScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("progressScreen.View() returned empty output")
	}
	wantSubstrings := []string{
		"Installing...",
		"Installing packages",
		"Configuring zsh",
		"Press ENTER to start",
		"[ENTER] Start",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("progressScreen.View() (not started) missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenProgress {
		t.Errorf("progressScreen.ID() = %v, want ScreenProgress", screen.ID())
	}
}

// TestProgressScreenRunningGolden verifies the running state renders the live
// output lines and the in-progress footer.
func TestProgressScreenRunningGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.installRunning = true
	ctx.app.installComplete = false
	ctx.app.installStep = 2
	ctx.app.installOutput = []string{"▶ Installing tmux...", "  ✓ tmux installed successfully"}

	screen := NewProgressScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	wantSubstrings := []string{
		"Installing...",
		"Installing tmux",          // live output line
		"installed successfully",   // live output line
		"Installation in progress", // running footer
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("progressScreen.View() (running) missing %q\n---\n%s\n---", want, out)
		}
	}
}

// TestProgressScreenCompleteGolden verifies the complete state renders the
// success title and the continue footer.
func TestProgressScreenCompleteGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.installRunning = false
	ctx.app.installComplete = true

	screen := NewProgressScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	wantSubstrings := []string{
		"Installation Complete",
		"[ENTER] Continue",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("progressScreen.View() (complete) missing %q\n---\n%s\n---", want, out)
		}
	}
}

// TestProgressScreenInstallEventStreams is the key streaming proof: an
// installEventMsg applied to the Progress handler updates App state AND returns a
// non-nil continuation (the listen cmd) so the event stream keeps flowing.
func TestProgressScreenInstallEventStreams(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installRunning = true
	ctx.app.installComplete = false
	ctx.app.installStep = 0
	ctx.app.installOutput = nil
	// A live channel so the re-issued listen cmd has something to read from.
	ctx.app.installEvents = make(chan installEventMsg, 1)

	screen := NewProgressScreen(ctx)

	// A line+step event updates state and re-arms the listen cmd.
	next, cmd := screen.Update(installEventMsg{line: "▶ Installing ghostty...", stepInc: true})
	if next != screen {
		t.Fatalf("progressScreen should remain current after installEventMsg")
	}
	if len(ctx.app.installOutput) != 1 || ctx.app.installOutput[0] != "▶ Installing ghostty..." {
		t.Errorf("installEventMsg should append the output line, got %v", ctx.app.installOutput)
	}
	if ctx.app.installStep != 1 {
		t.Errorf("installStep = %d, want 1 after a stepInc event", ctx.app.installStep)
	}
	if cmd == nil {
		t.Fatal("non-done installEventMsg must return a non-nil continuation (listenInstallEventsCmd) to keep the stream flowing")
	}

	// The 20-line display cap is preserved.
	ctx.app.installOutput = nil
	for i := 0; i < 25; i++ {
		screen.Update(installEventMsg{line: "line"})
	}
	if len(ctx.app.installOutput) != 20 {
		t.Errorf("installOutput should be capped at 20 lines, got %d", len(ctx.app.installOutput))
	}
}

// TestProgressScreenInstallDoneSuccess verifies a done event clears the channel,
// marks complete, and does not error on success.
func TestProgressScreenInstallDoneSuccess(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installRunning = true
	ctx.app.installEvents = make(chan installEventMsg, 1)

	screen := NewProgressScreen(ctx)
	next, _ := screen.Update(installEventMsg{done: true, err: nil})
	if next != screen {
		t.Fatalf("progressScreen should remain current after a done installEventMsg")
	}
	if ctx.app.installRunning {
		t.Error("done event should clear installRunning")
	}
	if !ctx.app.installComplete {
		t.Error("done event should set installComplete")
	}
	if ctx.app.installEvents != nil {
		t.Error("done event should nil out installEvents")
	}
}

// TestProgressScreenInstallDoneError verifies a failing done event records the
// error (with context) and returns a navigation command to the error screen.
func TestProgressScreenInstallDoneError(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installRunning = true

	screen := NewProgressScreen(ctx)
	_, cmd := screen.Update(installEventMsg{done: true, err: errors.New("boom"), context: "last lines"})
	if ctx.app.lastError == nil || !strings.Contains(ctx.app.lastError.Error(), "boom") {
		t.Errorf("done error event should record lastError, got %v", ctx.app.lastError)
	}
	if !strings.Contains(ctx.app.lastError.Error(), "last lines") {
		t.Errorf("done error event should include the output context, got %v", ctx.app.lastError)
	}
	// showError now always routes through the ScreenManager: it sets the error on
	// the factory and returns a NavigateTo(ScreenError) command. Assert the error
	// state and that the returned command navigates to the error screen.
	if cmd == nil {
		t.Fatal("done error event should return a NavigateTo(ScreenError) command")
	}
	nav, ok := cmd().(NavigateMsg)
	if !ok || nav.To != ScreenError {
		t.Errorf("done error event should navigate to ScreenError, got %#v", cmd())
	}
}

// TestProgressScreenReachableViaManager verifies an App built with the manager
// enters managed mode on NavigateTo(ScreenProgress) and renders the progress
// screen through the factory. (Init triggers the install; on a non-Linux host
// startInstallation runs without a sudo prompt, so the render is still safe.)
func TestProgressScreenReachableViaManager(t *testing.T) {
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	app.width, app.height = 80, 24
	// Pre-mark as running so Init() no-ops (it guards on installRunning) and the
	// render is deterministic without kicking off a real package install.
	app.installRunning = true

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenProgress)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenProgress")
	}
	if app.screenMgr.Current() == nil {
		t.Fatal("manager should be in managed mode after navigating to ScreenProgress")
	}
	if app.screenMgr.Current().ID() != ScreenProgress {
		t.Fatalf("manager current screen ID = %v, want ScreenProgress", app.screenMgr.Current().ID())
	}
	if view := app.screenMgr.View(); !strings.Contains(view, "Installing") {
		t.Errorf("managed progressScreen should render the progress screen\n---\n%s\n---", view)
	}
}

// TestProgressScreenInitNoOpWhileRunning verifies Init() does not re-trigger the
// install when one is already running (idempotent guard).
func TestProgressScreenInitNoOpWhileRunning(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installRunning = true

	screen := NewProgressScreen(ctx)
	if cmd := screen.Init(); cmd != nil {
		t.Error("progressScreen.Init() should be a no-op while an install is already running")
	}
}

// ============================================================================
// Migrated management screen: Users (dual-pane)
// ============================================================================

// TestUsersScreenEmptyGolden is a regression guard for the migrated usersScreen
// with an empty (loaded) user list.
func TestUsersScreenEmptyGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.usersLoaded = true
	ctx.app.usersItems = nil

	screen := NewUsersScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	if strings.TrimSpace(out) == "" {
		t.Fatal("usersScreen.View() returned empty output")
	}
	wantSubstrings := []string{
		"User Profiles",     // left pane header
		"No user profiles.", // empty-state message
		"create one",        // empty-state hint
		"n:new",             // status bar help
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("usersScreen.View() (empty) missing %q\n---\n%s\n---", want, out)
		}
	}

	if screen.ID() != ScreenUsers {
		t.Errorf("usersScreen.ID() = %v, want ScreenUsers", screen.ID())
	}
}

// TestUsersScreenPopulatedGolden verifies the populated list renders the user
// rows, the active indicator, and the settings pane fields.
func TestUsersScreenPopulatedGolden(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.usersLoaded = true
	ctx.app.usersIndex = 0
	ctx.app.usersPane = usersPaneSettings
	ctx.app.usersItems = []userItem{
		{name: "alice", theme: "dracula", navStyle: "vim", keyboard: "macos", isActive: true},
		{name: "bob", theme: "nord", navStyle: "emacs", keyboard: "linux", isActive: false},
	}

	screen := NewUsersScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)

	wantSubstrings := []string{
		"alice",
		"bob",
		"Settings: alice", // settings pane header for the selected user
		"Theme",
		"Navigation",
		"Keyboard",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("usersScreen.View() (populated) missing %q\n---\n%s\n---", want, out)
		}
	}
}

// TestUsersScreenAsyncInHandler proves the async-in-handler wiring: feeding a
// userLoadedMsg to the handler's Update updates App state and the rendered list,
// with no App.Update involvement.
func TestUsersScreenAsyncInHandler(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = ctx.Width, ctx.Height
	ctx.app.usersItems = nil

	screen := NewUsersScreen(ctx)

	loaded := userLoadedMsg{users: []userItem{
		{name: "async-user", theme: "nord", navStyle: "emacs", keyboard: "linux"},
	}}
	next, _ := screen.Update(loaded)
	if next != screen {
		t.Fatalf("usersScreen should remain current after userLoadedMsg")
	}
	if len(ctx.app.usersItems) != 1 || ctx.app.usersItems[0].name != "async-user" {
		t.Fatalf("userLoadedMsg should populate usersItems, got %+v", ctx.app.usersItems)
	}
	if out := screen.View(ctx.Width, ctx.Height); !strings.Contains(out, "async-user") {
		t.Errorf("usersScreen.View() should render the async-loaded user\n---\n%s\n---", out)
	}
}

// TestUsersScreenDeleteClearsActive proves the Phase B fix path is preserved:
// after a userDeletedMsg, the handler reloads the list (returns loadUsersCmd) and
// decrements the index when appropriate.
func TestUsersScreenDeleteAdjustsIndexAndReloads(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.usersIndex = 1

	screen := NewUsersScreen(ctx)
	_, cmd := screen.Update(userDeletedMsg{name: "bob"})
	if ctx.app.usersIndex != 0 {
		t.Errorf("usersIndex = %d, want 0 after deleting at index 1", ctx.app.usersIndex)
	}
	if cmd == nil {
		t.Fatal("userDeletedMsg should return loadUsersCmd to refresh the list")
	}
	if !strings.Contains(ctx.app.usersStatus, "Deleted bob") {
		t.Errorf("usersStatus = %q, want a 'Deleted bob' message", ctx.app.usersStatus)
	}
}

// TestUsersScreenEscNavigatesToMainMenu verifies esc routes back through the
// ScreenManager to the main menu.
func TestUsersScreenEscNavigatesToMainMenu(t *testing.T) {
	ctx := newGoldenContext(t)
	screen := NewUsersScreen(ctx)

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

// TestUsersScreenInitLoads verifies Init() kicks the user-list load when not yet
// loaded, and is idempotent when already loaded.
func TestUsersScreenInitLoads(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.usersLoaded = false

	screen := NewUsersScreen(ctx)
	if cmd := screen.Init(); cmd == nil {
		t.Fatal("usersScreen.Init() should return loadUsersCmd when not loaded")
	}
	if !ctx.app.usersLoaded {
		t.Error("usersScreen.Init() should set usersLoaded")
	}
	// Idempotent: already loaded -> no new command.
	if cmd := screen.Init(); cmd != nil {
		t.Error("usersScreen.Init() should be a no-op when already loaded")
	}
}

// TestUsersScreenReachableViaManager verifies an App built with the manager
// enters managed mode on NavigateTo(ScreenUsers) and renders the dual-pane
// through the factory.
func TestUsersScreenReachableViaManager(t *testing.T) {
	app := NewApp(true)
	if app.screenMgr == nil {
		t.Fatal("NewApp should always initialize screenMgr")
	}
	app.screenMgr.SetSize(80, 24)
	app.width, app.height = 80, 24
	// Mark loaded with an empty list so the render is deterministic (no disk).
	app.usersLoaded = true
	app.usersItems = nil

	if _, handled := app.screenMgr.Update(NavigateTo(ScreenUsers)()); !handled {
		t.Fatal("manager should handle the NavigateMsg to ScreenUsers")
	}
	if app.screenMgr.Current() == nil {
		t.Fatal("manager should be in managed mode after navigating to ScreenUsers")
	}
	if app.screenMgr.Current().ID() != ScreenUsers {
		t.Fatalf("manager current screen ID = %v, want ScreenUsers", app.screenMgr.Current().ID())
	}
	if view := app.screenMgr.View(); !strings.Contains(view, "User Profiles") {
		t.Errorf("managed usersScreen should render the user list\n---\n%s\n---", view)
	}
}
