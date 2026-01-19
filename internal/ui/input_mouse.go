package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// handleTabBarMouse handles mouse clicks on the tab bar for screens that use it
func (a *App) handleTabBarMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Only handle left clicks
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Tab bar is at Y=0 (first line)
	if m.Y != 0 {
		return a, nil
	}

	// Check if click is on a tab
	if screen, cmd := a.detectTabClick(m.X); screen != 0 {
		a.screen = screen
		return a, cmd
	}

	return a, nil
}

// detectTabClick determines which tab was clicked based on X position
// Returns the target screen and any command to run, or (0, nil) if no tab clicked
func (a *App) detectTabClick(x int) (Screen, tea.Cmd) {
	tabs := GetManagementTabs()
	if len(tabs) == 0 {
		return 0, nil
	}

	// All screens now use unified RenderTabBar format: "N 󰒓 Name" with Padding(0,1)
	var tabWidths []int
	for i, tab := range tabs {
		content := fmt.Sprintf("%d %s %s", i+1, tab.Icon, tab.Name)
		// Padding(0, 1) = 1 space each side = 2 total
		width := lipgloss.Width(content) + 2
		tabWidths = append(tabWidths, width)
	}

	// Tab bar is left-aligned (Width renders left-aligned by default)
	// Account for 1-char separator " " between tabs
	currentX := 0
	for i, tab := range tabs {
		endX := currentX + tabWidths[i]

		if x >= currentX && x < endX {
			// Don't switch if already on this screen
			if tab.Screen == a.screen {
				return 0, nil
			}

			// Start async update check when switching to Update screen
			if tab.Screen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
				a.updateChecking = true
				return tab.Screen, checkUpdatesCmd()
			}

			return tab.Screen, nil
		}

		// Move past tab width + separator (1 char)
		currentX = endX + 1
	}

	return 0, nil
}

// handleWelcomeMouse handles mouse clicks on the welcome screen
func (a *App) handleWelcomeMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle left click to toggle deep dive option or proceed
	if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		// The welcome screen has two boxes side by side at the bottom
		// Left box: Quick Install, Right box: Deep Dive
		centerY := a.height / 2
		centerX := a.width / 2

		// If clicking in lower half where the options are
		if m.Y > centerY {
			if m.X < centerX {
				a.deepDive = false
			} else {
				a.deepDive = true
			}
		}
	}

	return a, nil
}

// handleThemePickerMouse handles mouse clicks on the theme picker screen
func (a *App) handleThemePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle scroll wheel for navigation (works anywhere on screen)
	switch m.Button {
	case tea.MouseButtonWheelUp:
		if a.themeIndex > 0 {
			a.themeIndex--
			a.theme = themes[a.themeIndex].name
			SetTheme(a.theme)
		}
		return a, nil
	case tea.MouseButtonWheelDown:
		if a.themeIndex < len(themes)-1 {
			a.themeIndex++
			a.theme = themes[a.themeIndex].name
			SetTheme(a.theme)
		}
		return a, nil
	}

	// Handle left clicks
	if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		// The content is centered using PlaceWithBackground + ContainerStyle
		// Container has Padding(1, 2) and Border, then title + empty line + themes
		// Calculate approximate content area
		containerH := len(themes) + 6 // title + empty + themes + empty + help + padding
		containerW := 60              // approximate
		startY := (a.height - containerH) / 2
		startX := (a.width - containerW) / 2

		// Theme list starts after: container border (1) + padding (1) + title (1) + empty (1)
		listStartY := startY + 4

		// Check if click is in theme list area
		if m.Y >= listStartY && m.Y < listStartY+len(themes) && m.X >= startX {
			themeIdx := m.Y - listStartY
			if themeIdx >= 0 && themeIdx < len(themes) {
				a.themeIndex = themeIdx
				a.theme = themes[themeIdx].name
				SetTheme(a.theme)
				return a, nil
			}
		}
	}

	return a, nil
}

// handleNavPickerMouse handles mouse clicks on the nav picker screen
func (a *App) handleNavPickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Nav picker has two boxes side by side (or stacked on narrow terminals)
	// Calculate approximate positions
	centerX := a.width / 2
	centerY := a.height / 2

	if a.width >= 78 {
		// Side by side layout
		// Emacs box is on the left, Vim box is on the right
		if m.X < centerX {
			a.navStyle = "emacs"
		} else {
			a.navStyle = "vim"
		}
	} else {
		// Stacked layout - Emacs on top, Vim on bottom
		if m.Y < centerY {
			a.navStyle = "emacs"
		} else {
			a.navStyle = "vim"
		}
	}

	return a, nil
}

// handleDeepDiveMenuMouse handles mouse clicks on the deep dive menu
func (a *App) handleDeepDiveMenuMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle scroll wheel
	if m.Button == tea.MouseButtonWheelUp {
		if a.deepDiveMenuIndex > 0 {
			a.deepDiveMenuIndex--
		}
		return a, nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		items := GetDeepDiveMenuItems()
		if a.deepDiveMenuIndex < len(items)-1 {
			a.deepDiveMenuIndex++
		}
		return a, nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Menu items are in a centered container
	items := GetDeepDiveMenuItems()
	contentHeight := len(items) + 10 // items + headers + padding
	startY := (a.height - contentHeight) / 2
	listStartY := startY + 4 // After title and instructions

	// Account for category headers (they take up a line but aren't clickable)
	clickableY := listStartY
	for i, item := range items {
		if item.Category != "" {
			clickableY++ // Category header takes a line
		}
		if m.Y == clickableY {
			a.deepDiveMenuIndex = i
			// Double-click or single click to enter
			a.configFieldIndex = 0
			a.screen = item.Screen
			return a, nil
		}
		clickableY++
	}

	return a, nil
}

// handleConfigScreenMouse handles mouse clicks on config screens
func (a *App) handleConfigScreenMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle scroll wheel for field navigation
	if m.Button == tea.MouseButtonWheelUp {
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
		return a, nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		// Get max fields for current screen
		maxFields := a.getConfigScreenMaxFields()
		if a.configFieldIndex < maxFields-1 {
			a.configFieldIndex++
		}
		return a, nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Config screens have fields listed vertically
	// Approximate click detection based on Y position
	contentHeight := a.getConfigScreenMaxFields() + 8
	startY := (a.height - contentHeight) / 2
	fieldStartY := startY + 4 // After title

	if m.Y >= fieldStartY {
		fieldIdx := m.Y - fieldStartY
		maxFields := a.getConfigScreenMaxFields()
		if fieldIdx >= 0 && fieldIdx < maxFields {
			a.configFieldIndex = fieldIdx
		}
	}

	return a, nil
}

// handleSummaryMouse handles mouse clicks on the summary screen
func (a *App) handleSummaryMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Summary screen doesn't need special mouse handling yet
	// Left click could be used to trigger install in the future
	return a, nil
}
