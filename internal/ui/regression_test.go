package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tekierz/dotfiles/internal/pkg"
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
// The theme picker must branch on themeStandalone: standalone (from the main menu
// or CLI) applies-and-exits; wizard advances to the nav picker.
func TestThemePickerReturnContext(t *testing.T) {
	t.Run("standalone from main menu returns to main menu", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.themeStandalone = true
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
		ctx.app.themeStandalone = false
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

// TestThemePickerStandaloneExplicitMode covers the two entry paths called out
// in the audit (C7, C8) that were previously un-tested:
//
//  1. CLI standalone (`dotfiles theme`): Enter must persist and quit (not go to
//     NavPicker). SetStartScreen(ScreenThemePicker) sets themeStandalone=true, so
//     the picker quits instead of advancing through the install wizard.
//
//  2. Deep-dive "Continue to Installation": Enter must advance to NavPicker (not
//     bounce back to MainMenu). After a previous main-menu theme visit
//     (themeReturn=ScreenMainMenu), the deep-dive continue path must reset
//     themeStandalone=false so the wizard step takes over.
func TestThemePickerStandaloneExplicitMode(t *testing.T) {
	t.Run("CLI standalone Enter quits, not NavPicker", func(t *testing.T) {
		// Simulate SetStartScreen(ScreenThemePicker): themeStandalone=true,
		// themeReturn=ScreenWelcome (constructor default).
		ctx := newGoldenContext(t)
		ctx.app.themeStandalone = true
		ctx.app.themeReturn = ScreenWelcome // stale default from constructor
		screen := NewThemePickerScreen(ctx)

		_, enterCmd := screen.Update(keyMsg("enter"))
		// Enter in CLI standalone must quit (tea.Quit), NOT navigate to NavPicker.
		if enterCmd == nil {
			t.Fatal("Enter in standalone CLI: expected tea.Quit command, got nil")
		}
		msg := enterCmd()
		if _, isNav := msg.(NavigateMsg); isNav {
			nav := msg.(NavigateMsg)
			t.Errorf("Enter in standalone CLI: got NavigateTo(%v), want tea.Quit (should not enter install wizard)", nav.To)
		}
		// The command should be tea.Quit (its message is tea.QuitMsg).
		if _, isQuit := msg.(tea.QuitMsg); !isQuit {
			t.Errorf("Enter in standalone CLI: got message type %T, want tea.QuitMsg", msg)
		}
		// Persist must have been attempted: a successful persist clears themeStatus.
		// If persistTheme() were removed before tea.Quit, themeStatus could remain
		// non-empty from a prior error — but its initial value is "", so the more
		// important invariant is that no persist error was recorded.
		if ctx.app.themeStatus != "" {
			t.Errorf("Enter in standalone CLI: themeStatus = %q, want \"\" (persist must not error)", ctx.app.themeStatus)
		}
	})

	t.Run("CLI standalone Esc quits, not welcome screen", func(t *testing.T) {
		// Regression: before the fix, Esc in CLI standalone navigated to
		// a.themeReturn (ScreenWelcome, the constructor default), which dumped
		// the user into the install wizard welcome screen (C7 cancel case).
		ctx := newGoldenContext(t)
		ctx.app.themeStandalone = true
		ctx.app.themeReturn = ScreenWelcome // constructor default for CLI path
		screen := NewThemePickerScreen(ctx)

		_, escCmd := screen.Update(keyMsg("esc"))
		if escCmd == nil {
			t.Fatal("Esc in standalone CLI: expected a command, got nil")
		}
		msg := escCmd()
		if nav, isNav := msg.(NavigateMsg); isNav {
			t.Errorf("Esc in standalone CLI: got NavigateTo(%v), want tea.QuitMsg (must not enter install wizard)", nav.To)
		}
		if _, isQuit := msg.(tea.QuitMsg); !isQuit {
			t.Errorf("Esc in standalone CLI: got message type %T, want tea.QuitMsg", msg)
		}
	})

	t.Run("deep-dive continue Enter advances to NavPicker, not MainMenu", func(t *testing.T) {
		// Simulate: user went MainMenu -> Theme (themeReturn=ScreenMainMenu, themeStandalone=true),
		// then Esc -> Install -> DeepDive -> Continue. The Continue path must reset
		// themeStandalone=false before navigating to the theme picker.
		ctx := newGoldenContext(t)
		// After the explicit-mode fix, deep-dive continue resets themeStandalone.
		ctx.app.themeStandalone = false
		ctx.app.themeReturn = ScreenDeepDiveMenu
		screen := NewThemePickerScreen(ctx)

		_, enterCmd := screen.Update(keyMsg("enter"))
		if got := navTarget(t, enterCmd); got != ScreenNavPicker {
			t.Errorf("Enter (deep-dive continue): navigated to %v, want ScreenNavPicker (wizard must advance)", got)
		}

		// Esc from the deep-dive wizard entry must return to ScreenDeepDiveMenu,
		// not the hardcoded ScreenWelcome that existed before the fix.
		_, escCmd := screen.Update(keyMsg("esc"))
		if got := navTarget(t, escCmd); got != ScreenDeepDiveMenu {
			t.Errorf("Esc (deep-dive continue): navigated to %v, want ScreenDeepDiveMenu (must respect themeReturn)", got)
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
		out := tools.GenerateGhosttyConfig(cfg, defaultTheme)
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

// drainCmd runs a tea.Cmd (and any batch it expands into) to completion,
// returning every message produced. Used to assert that a global handler
// re-armed a listen Cmd or kicked a cache refresh.
func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, drainCmd(c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

// TestStreamingMsgSurvivesNavigation pins down the durable fix for cluster A:
// the terminal/streaming install & update async messages must be fully
// processed (running flags reset, cache refresh requested) even when a
// DIFFERENT screen is active when the message arrives. Before the fix these
// messages were handled ONLY by the originating screen's Update, so navigating
// away dropped them, stranded the running flag, and orphaned the worker.
func TestStreamingMsgSurvivesNavigation(t *testing.T) {
	t.Run("manageInstallWithLogsMsg resets flag from another screen", func(t *testing.T) {
		app := NewApp(true)
		// Simulate an install started on Manage, then the user navigated to the
		// main menu (a different screen) before the terminal message arrives.
		app.screenMgr.Navigate(ScreenMainMenu)
		app.manageInstalling = true
		app.manageInstallID = "btop"
		app.manageInstalledReady = true

		_, cmd := app.Update(manageInstallWithLogsMsg{toolID: "btop", logs: []string{"done"}})

		if app.manageInstalling {
			t.Error("manageInstalling still true after terminal msg delivered to another screen")
		}
		if app.manageInstalledReady {
			t.Error("manageInstalledReady not reset; install-status cache would stay stale")
		}
		// A successful install must kick a fresh cache load.
		if cmd == nil {
			t.Fatal("expected a cache-refresh command after successful install, got nil")
		}
		if !app.installCacheLoading {
			t.Error("expected installCacheLoading=true (cache refresh kicked)")
		}
	})

	t.Run("manageInstallDoneMsg resets flag from another screen", func(t *testing.T) {
		app := NewApp(true)
		app.screenMgr.Navigate(ScreenMainMenu)
		app.manageInstalling = true
		app.manageInstallID = "btop"
		app.manageInstalledReady = true

		app.Update(manageInstallDoneMsg{toolID: "btop"})

		if app.manageInstalling {
			t.Error("manageInstalling still true after manageInstallDoneMsg on another screen")
		}
		if app.manageInstallID != "" {
			t.Error("manageInstallID not cleared")
		}
		if app.manageInstalledReady {
			t.Error("manageInstalledReady not reset")
		}
	})

	t.Run("updateStreamMsg done resets updateRunning from another screen", func(t *testing.T) {
		app := NewApp(true)
		app.screenMgr.Navigate(ScreenMainMenu)
		app.updateRunning = true

		app.Update(updateStreamMsg{done: true})

		if app.updateRunning {
			t.Error("updateRunning still true after streaming done delivered to another screen")
		}
	})

	t.Run("updateStreamMsg line re-arms the listen cmd globally", func(t *testing.T) {
		app := NewApp(true)
		app.screenMgr.Navigate(ScreenMainMenu)
		app.updateRunning = true
		// A live (non-done) line must re-issue listenUpdateStreamCmd so the stream
		// keeps flowing regardless of the active screen. Closed channel yields a
		// done event from the listen cmd.
		ch := make(chan updateStreamMsg)
		close(ch)
		app.updateStream = ch

		_, cmd := app.Update(updateStreamMsg{line: "Downloading..."})
		if cmd == nil {
			t.Fatal("expected listenUpdateStreamCmd to be re-armed, got nil")
		}
		msgs := drainCmd(cmd)
		found := false
		for _, m := range msgs {
			if _, ok := m.(updateStreamMsg); ok {
				found = true
			}
		}
		if !found {
			t.Errorf("re-armed command did not produce an updateStreamMsg; got %v", msgs)
		}
	})
}

// TestLoadResultSurvivesNavigation pins down the durable fix for the on-enter
// LOAD results of the management tabs (Update check, Backups list, Users list).
// These async results must be fully applied by App.Update even when a DIFFERENT
// screen is active when they arrive. Before the fix they were handled ONLY by the
// owning screen's Update, so navigating away while the load was in flight dropped
// the result, stranded the in-flight guard flag set (updateChecking /
// backupsLoading / usersLoaded), and wedged the screen on "Checking/Loading..."
// forever (re-entry sees the guard set and never re-kicks). Each subtask
// navigates to a screen OTHER than the one that owns the message, then feeds the
// result to App.Update; on the buggy code the assertions below would fail because
// the global cases in App.Update did not exist.
func TestLoadResultSurvivesNavigation(t *testing.T) {
	t.Run("updateCheckDoneMsg applied from another screen", func(t *testing.T) {
		withTempHome(t)
		app := NewApp(true)
		// Update check kicked on entry to ScreenUpdate, then the user navigated to
		// the main menu before the check completed.
		app.screenMgr.Navigate(ScreenMainMenu)
		app.updateChecking = true

		app.Update(updateCheckDoneMsg{updates: []pkg.Package{}})

		if app.updateChecking {
			t.Error("updateChecking still true after updateCheckDoneMsg on another screen; screen would wedge on \"Checking...\"")
		}
		if !app.updateCheckDone {
			t.Error("updateCheckDone not set; re-entry would re-kick or show stale state")
		}
	})

	t.Run("backupsLoadedMsg applied from another screen", func(t *testing.T) {
		withTempHome(t)
		app := NewApp(true)
		app.screenMgr.Navigate(ScreenMainMenu)
		app.backupsLoading = true

		app.Update(backupsLoadedMsg{backups: []BackupEntry{}})

		if app.backupsLoading {
			t.Error("backupsLoading still true after backupsLoadedMsg on another screen; screen would wedge on \"Loading...\"")
		}
		if !app.backupsLoaded {
			t.Error("backupsLoaded not set; re-entry would re-kick or show stale state")
		}
	})

	t.Run("userLoadedMsg success populates the list from another screen", func(t *testing.T) {
		withTempHome(t)
		app := NewApp(true)
		app.screenMgr.Navigate(ScreenMainMenu)
		app.usersLoaded = true // guard set on entry to ScreenUsers

		const wantUser = "alice"
		app.Update(userLoadedMsg{users: []userItem{{name: wantUser}}})

		if len(app.usersItems) != 1 || app.usersItems[0].name != wantUser {
			t.Errorf("usersItems = %v after userLoadedMsg on another screen; want one entry %q", app.usersItems, wantUser)
		}
	})

	t.Run("userLoadedMsg error resets guard so re-entry retries", func(t *testing.T) {
		withTempHome(t)
		app := NewApp(true)
		app.screenMgr.Navigate(ScreenMainMenu)
		app.usersLoaded = true // guard set on entry to ScreenUsers

		app.Update(userLoadedMsg{err: errors.New("disk gone")})

		if app.usersLoaded {
			t.Error("usersLoaded still true after a load error; re-entering Users would never retry the load")
		}
	})
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
