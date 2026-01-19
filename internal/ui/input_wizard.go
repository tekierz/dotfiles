package ui

import tea "github.com/charmbracelet/bubbletea"

// handleWizardKey handles key events for wizard screens:
// ScreenAnimation, ScreenWelcome, ScreenThemePicker, ScreenNavPicker,
// ScreenFileTree, ScreenProgress, ScreenSummary, ScreenError
func (a *App) handleWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.screen {
	case ScreenAnimation:
		// Any key skips animation
		a.animationDone = true
		a.screen = a.postIntroScreen
		// Trigger update check if transitioning to Update screen
		if a.postIntroScreen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return a, checkUpdatesCmd()
		}
		// Trigger install cache load if transitioning to Manage or Hotkeys screen
		if a.postIntroScreen == ScreenManage || a.postIntroScreen == ScreenHotkeys {
			if cmd := a.startInstallCacheLoad(); cmd != nil {
				return a, cmd
			}
		}
		return a, nil

	case ScreenWelcome:
		switch key {
		case "enter":
			if a.deepDive {
				a.screen = ScreenDeepDiveMenu
				// Start async install cache load for deep dive menu
				if cmd := a.startInstallCacheLoad(); cmd != nil {
					return a, cmd
				}
			} else {
				a.screen = ScreenThemePicker
			}
		case "tab", "left", "right", "h", "l":
			a.deepDive = !a.deepDive
		}

	case ScreenThemePicker:
		switch key {
		case "up", "k":
			if a.themeIndex > 0 {
				a.themeIndex--
				a.theme = themes[a.themeIndex].name
				SetTheme(a.theme) // Apply theme immediately for live preview
			}
		case "down", "j":
			if a.themeIndex < len(themes)-1 {
				a.themeIndex++
				a.theme = themes[a.themeIndex].name
				SetTheme(a.theme) // Apply theme immediately for live preview
			}
		case "enter":
			a.screen = ScreenNavPicker
		case "esc":
			a.screen = ScreenWelcome
		}

	case ScreenNavPicker:
		switch key {
		case "left", "right", "h", "l", "tab":
			if a.navStyle == "emacs" {
				a.navStyle = "vim"
			} else {
				a.navStyle = "emacs"
			}
		case "enter":
			a.screen = ScreenFileTree
		case "esc":
			a.screen = ScreenThemePicker
		}

	case ScreenFileTree:
		switch key {
		case "enter":
			a.screen = ScreenProgress
			return a, func() tea.Msg { return installStartMsg{} }
		case "esc":
			a.screen = ScreenNavPicker
		}

	case ScreenProgress:
		switch key {
		case "enter":
			// Only advance if installation is complete
			if !a.installRunning {
				a.screen = ScreenSummary
			}
		}

	case ScreenSummary:
		switch key {
		case "enter", "q":
			return a, tea.Quit
		}

	case ScreenError:
		switch key {
		case "r":
			// Retry - go back to progress
			a.screen = ScreenProgress
		case "s":
			// Skip - continue to summary
			a.screen = ScreenSummary
		case "q":
			return a, tea.Quit
		case "esc":
			a.screen = ScreenFileTree
		}
	}

	return a, nil
}
