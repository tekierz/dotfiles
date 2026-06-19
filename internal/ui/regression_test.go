package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tekierz/dotfiles/internal/tools"
)

// navTarget executes a NavigateTo command and returns its destination screen.
func navTarget(t *testing.T, cmd tea.Cmd) Screen {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a navigation command, got nil")
	}
	msg := cmd()
	nav, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	return nav.To
}

// TestThemePickerReturnContext pins down the fix for the reported bug where
// selecting a theme from the main menu dumped the user into the install wizard.
// The theme picker must branch on themeReturn: standalone (from the main menu)
// applies-and-returns; wizard advances to the nav picker.
func TestThemePickerReturnContext(t *testing.T) {
	t.Run("standalone from main menu returns to main menu", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.themeReturn = ScreenMainMenu
		screen := NewThemePickerScreen(ctx)

		_, enterCmd := screen.Update(keyMsg("enter"))
		if got := navTarget(t, enterCmd); got != ScreenMainMenu {
			t.Errorf("Enter (standalone): navigated to %v, want ScreenMainMenu", got)
		}

		_, escCmd := screen.Update(keyMsg("esc"))
		if got := navTarget(t, escCmd); got != ScreenMainMenu {
			t.Errorf("Esc (standalone): navigated to %v, want ScreenMainMenu", got)
		}
	})

	t.Run("wizard advances through the install flow", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.themeReturn = ScreenWelcome
		screen := NewThemePickerScreen(ctx)

		_, enterCmd := screen.Update(keyMsg("enter"))
		if got := navTarget(t, enterCmd); got != ScreenNavPicker {
			t.Errorf("Enter (wizard): navigated to %v, want ScreenNavPicker", got)
		}

		_, escCmd := screen.Update(keyMsg("esc"))
		if got := navTarget(t, escCmd); got != ScreenWelcome {
			t.Errorf("Esc (wizard): navigated to %v, want ScreenWelcome", got)
		}
	})
}

// TestTabNavigationTargetCoversAllTabs pins down the fix for Backups being
// unreachable via the keyboard tab bar: the digit handler must cover every tab
// in GetManagementTabs(), including the 5th (Backups).
func TestTabNavigationTargetCoversAllTabs(t *testing.T) {
	tabs := GetManagementTabs()
	for i, tab := range tabs {
		key := string(rune('1' + i))
		got, ok := tabNavigationTarget(key)
		if !ok {
			t.Errorf("key %q: tabNavigationTarget returned ok=false, want tab %q", key, tab.Name)
			continue
		}
		if got != tab.Screen {
			t.Errorf("key %q: got screen %v, want %v (%s)", key, got, tab.Screen, tab.Name)
		}
	}

	// The reported bug: "5" must reach Backups (the 5th tab).
	if got, ok := tabNavigationTarget("5"); !ok || got != ScreenBackups {
		t.Errorf(`tabNavigationTarget("5") = (%v, %v), want (ScreenBackups, true)`, got, ok)
	}

	// Out-of-range / non-digit keys are rejected.
	for _, bad := range []string{"0", "6", "9", "x", "", "12"} {
		if _, ok := tabNavigationTarget(bad); ok {
			t.Errorf("tabNavigationTarget(%q) returned ok=true, want false", bad)
		}
	}
}

// TestGhosttyTabBindingOptionsAllGenerate pins down the fix for the Ghostty
// "New Tab Keybinding" field: every option the UI offers must produce a real
// new_tab keybinding (the old "alt" option matched no generator case and emitted
// nothing). The UI options must match the generator's supported values.
func TestGhosttyTabBindingOptionsAllGenerate(t *testing.T) {
	uiOptions := []string{"super", "ctrl", "ctrl-shift"} // must match screen_config_ghostty.go
	for _, opt := range uiOptions {
		cfg := tools.GhosttyConfig{TabBindings: opt}
		out := tools.GenerateGhosttyConfig(cfg, "catppuccin-mocha")
		if !strings.Contains(out, "=new_tab") {
			t.Errorf("TabBindings=%q generated no new_tab keybinding:\n%s", opt, out)
		}
		// The generated binding uses the T key (correct Ghostty convention), which
		// is what the labels now advertise (⌘/Super+T, Ctrl+T, Ctrl+Shift+T).
		if !strings.Contains(out, "+t=new_tab") {
			t.Errorf("TabBindings=%q does not bind new_tab to the T key:\n%s", opt, out)
		}
	}
}

// TestNavBlockedWhileStreaming pins down the fix for the streaming-navigation
// wedge: switching tabs (keyboard or mouse) while an install/update streams
// would drop the terminal message, strand the running flag, and orphan the
// subprocess. Navigation must be a no-op while the op is running.
func TestNavBlockedWhileStreaming(t *testing.T) {
	t.Run("manage keyboard tab-nav is a no-op while installing", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.manageInstalling = true
		s := NewManageScreen(ctx)
		if _, cmd := s.Update(keyMsg("2")); cmd != nil {
			t.Error("manage tab-nav while installing returned a command; want nil (no navigation)")
		}
	})
	t.Run("update mouse tab-click is a no-op while running", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.updateRunning = true
		s := NewUpdateScreen(ctx)
		click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 0}
		if _, cmd := s.Update(click); cmd != nil {
			t.Error("update tab-click while running returned a command; want nil (no navigation)")
		}
	})
}
