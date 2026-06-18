package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleManagementKey handles key events for the still-legacy management
// screens: the ScreenManage* config detail screens (ScreenManageGhostty,
// ScreenManageTmux, etc.).
//
// Note: ScreenMainMenu, ScreenManage (live dual-pane), ScreenUsers, ScreenUpdate,
// ScreenHotkeys, and ScreenBackups are migrated to ScreenHandlers and driven by
// the ScreenManager, so they are no longer handled here.
func (a *App) handleManagementKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.screen {
	// Management config screens. handleManageNavigation routes esc back to
	// ScreenManage (now a migrated ScreenHandler) through the ScreenManager.
	case ScreenManageGhostty:
		return a, a.handleManageNavigation(key, 7, ScreenManage)

	case ScreenManageTmux:
		return a, a.handleManageNavigation(key, 7, ScreenManage)

	case ScreenManageZsh:
		return a, a.handleManageNavigation(key, 6, ScreenManage)

	case ScreenManageNeovim:
		return a, a.handleManageNavigation(key, 7, ScreenManage)

	case ScreenManageGit:
		return a, a.handleManageNavigation(key, 6, ScreenManage)

	case ScreenManageYazi:
		return a, a.handleManageNavigation(key, 4, ScreenManage)

	case ScreenManageFzf:
		return a, a.handleManageNavigation(key, 4, ScreenManage)

	case ScreenManageLazyGit:
		return a, a.handleManageNavigation(key, 3, ScreenManage)

	case ScreenManageLazyDocker:
		return a, a.handleManageNavigation(key, 1, ScreenManage)

	case ScreenManageBtop:
		return a, a.handleManageNavigation(key, 5, ScreenManage)

	case ScreenManageGlow:
		return a, a.handleManageNavigation(key, 3, ScreenManage)

	case ScreenManageClaudeCode:
		return a, a.handleManageNavigation(key, 7, ScreenManage) // 7 MCP toggles
	}

	return a, nil
}
