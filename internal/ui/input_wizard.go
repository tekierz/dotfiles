package ui

import tea "github.com/charmbracelet/bubbletea"

// handleWizardKey handles key events for the still-legacy wizard screens:
// ScreenSummary and ScreenError.
//
// ScreenWelcome, ScreenThemePicker, ScreenNavPicker, ScreenFileTree,
// ScreenAnimation and ScreenProgress have been migrated to ScreenHandler
// implementations (screen_welcome.go, screen_animation.go, screen_progress.go,
// etc.) and are driven by the ScreenManager, so they are no longer handled here.
func (a *App) handleWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.screen {
	case ScreenSummary:
		switch key {
		case "enter", "q":
			return a, tea.Quit
		}

	case ScreenError:
		// Note: when the ScreenManager is active, the migrated ErrorScreen
		// handles these keys; this legacy block only runs as a fallback when
		// the manager is not wired (e.g. tests constructing App without it).
		switch key {
		case "r":
			// Retry - go back to progress
			a.screen = ScreenProgress
		case "s":
			// Skip - continue to summary
			return a, a.showSummary()
		case "q":
			return a, tea.Quit
		case "esc":
			a.screen = ScreenFileTree
		}
	}

	return a, nil
}
