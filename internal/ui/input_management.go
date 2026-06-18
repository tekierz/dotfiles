package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleManagementKey handles key events for the still-legacy management
// screens: ScreenManage, ScreenUsers, and the ScreenManage* config screens
// (ScreenManageGhostty, ScreenManageTmux, etc.).
//
// Note: ScreenMainMenu, ScreenUpdate, ScreenHotkeys, and ScreenBackups are
// migrated to ScreenHandlers and driven by the ScreenManager, so they are no
// longer handled here.
func (a *App) handleManagementKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.screen {
	// Manage screen navigation - delegates to existing handler
	case ScreenManage:
		return a.handleManageKey(msg)

	// Management config screens
	case ScreenManageGhostty:
		maxFields := 7
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageTmux:
		maxFields := 7
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageZsh:
		maxFields := 6
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageNeovim:
		maxFields := 7
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageGit:
		maxFields := 6
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageYazi:
		maxFields := 4
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageFzf:
		maxFields := 4
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageLazyGit:
		maxFields := 3
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageLazyDocker:
		maxFields := 1
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageBtop:
		maxFields := 5
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageGlow:
		maxFields := 3
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageClaudeCode:
		maxFields := 7 // Number of MCP toggles
		a.handleManageNavigation(key, maxFields, ScreenManage)

	// Users screen navigation - delegates to existing handler
	case ScreenUsers:
		return a.handleUsersKey(msg)
	}

	return a, nil
}
