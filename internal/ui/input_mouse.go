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
