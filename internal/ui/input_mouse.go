package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// handleTabBarMouse handles tab-bar clicks for the still-legacy screens that use
// the management tab bar (currently the Users screen). For migrated tab
// destinations (Hotkeys, Update, Backups) it routes through the ScreenManager
// via NavigateTo so the manager enters managed mode; for legacy destinations it
// switches a.screen directly. Either way it kicks the destination's on-enter
// load.
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
	if screen, _ := a.detectTabClick(m.X); screen != 0 && screen != a.screen {
		load := startTabTargetLoad(a, screen)
		if isManagedScreen(screen) {
			return a, tea.Batch(NavigateTo(screen), load)
		}
		a.screen = screen
		return a, load
	}

	return a, nil
}

// detectTabClick determines which management tab was clicked based on the X
// position, returning the target screen (or 0 if no tab was clicked).
//
// It is pure: it does not mutate App state and does not start any async work.
// Callers decide how to navigate (legacy a.screen vs. managed NavigateTo) and
// kick any on-enter load via startTabTargetLoad. The "already on this screen"
// suppression is the caller's responsibility, since a migrated handler knows its
// own active screen while a.screen may be stale in managed mode.
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
			return tab.Screen, nil
		}

		// Move past tab width + separator (1 char)
		currentX = endX + 1
	}

	return 0, nil
}
