package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// handleManageNavigation handles common navigation for management config screens
func (a *App) handleManageNavigation(key string, maxFields int, backScreen Screen) {
	switch key {
	case "up", "k":
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
	case "down", "j":
		if a.configFieldIndex < maxFields {
			a.configFieldIndex++
		}
	case "esc":
		a.configFieldIndex = 0
		a.screen = backScreen
	}
}

// handleTabNavigation handles number key shortcuts for tab navigation
// Returns (handled, command) - command may be nil even if handled
func (a *App) handleTabNavigationWithCmd(key string) (bool, tea.Cmd) {
	tabs := GetManagementTabs()
	var targetScreen Screen
	switch key {
	case "1":
		if len(tabs) > 0 {
			targetScreen = tabs[0].Screen
		}
	case "2":
		if len(tabs) > 1 {
			targetScreen = tabs[1].Screen
		}
	case "3":
		if len(tabs) > 2 {
			targetScreen = tabs[2].Screen
		}
	case "4":
		if len(tabs) > 3 {
			targetScreen = tabs[3].Screen
		}
	default:
		return false, nil
	}

	if targetScreen == 0 {
		return false, nil
	}

	a.screen = targetScreen

	// Start async operations when switching to certain screens
	if targetScreen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
		a.updateChecking = true
		return true, checkUpdatesCmd()
	}

	if targetScreen == ScreenManage {
		if cmd := a.startInstallCacheLoad(); cmd != nil {
			return true, cmd
		}
	}

	if targetScreen == ScreenUsers && !a.usersLoaded {
		a.usersLoaded = true
		return true, loadUsersCmd()
	}

	if targetScreen == ScreenBackups && !a.backupsLoading && !a.backupsLoaded {
		a.backupsLoading = true
		return true, loadBackupsCmd()
	}

	return true, nil
}

// handleTabNavigation handles number key shortcuts for tab navigation
// Returns true if the key was handled
func (a *App) handleTabNavigation(key string) bool {
	handled, _ := a.handleTabNavigationWithCmd(key)
	return handled
}

// cycleOption cycles through options forward or backward
func cycleOption(opts []string, current string, forward bool) string {
	for i, o := range opts {
		if o == current {
			if forward {
				return opts[(i+1)%len(opts)]
			}
			return opts[(i-1+len(opts))%len(opts)]
		}
	}
	return opts[0]
}

// atoi converts a string to int with a default value
func atoi(s string, defaultVal int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return defaultVal
	}
	return n
}

// getDeepDiveItemStatus returns the install status for a deep dive menu item
// Returns "installed" (blue) if all installed, "partial" (yellow) if partially installed, "pending" (grey) if not
func (a *App) getDeepDiveItemStatus(item DeepDiveMenuItem) string {
	toolIDs, ok := ScreenToolIDs[item.Screen]
	if !ok || len(toolIDs) == 0 {
		// No tool mapping (e.g., utilities) - show as pending
		return "pending"
	}

	installedCount := 0
	for _, id := range toolIDs {
		if a.manageInstalled[id] {
			installedCount++
		}
	}

	if installedCount == len(toolIDs) {
		return "installed" // All installed (blue)
	} else if installedCount > 0 {
		return "partial" // Partially installed (yellow)
	}
	return "pending" // None installed (grey)
}

// getConfigScreenMaxFields returns the number of fields for the current config screen
func (a *App) getConfigScreenMaxFields() int {
	switch a.screen {
	case ScreenConfigGhostty:
		return 7
	case ScreenConfigTmux:
		if a.deepDiveConfig.TmuxTPMEnabled {
			return 13
		}
		return 8
	case ScreenConfigZsh:
		return 13
	case ScreenConfigNeovim:
		return 14
	case ScreenConfigGit:
		return 5
	case ScreenConfigYazi:
		return 3
	case ScreenConfigFzf:
		return 3
	case ScreenConfigMacApps:
		return len(a.deepDiveConfig.MacApps)
	case ScreenConfigGUIApps:
		return 4
	case ScreenConfigCLITools:
		return 5
	case ScreenConfigCLIUtilities:
		return 7
	case ScreenConfigUtilities:
		return 3
	default:
		return 10
	}
}

// togglePlugin adds or removes a plugin from the list
func togglePlugin(plugins *[]string, plugin string) {
	for i, p := range *plugins {
		if p == plugin {
			*plugins = append((*plugins)[:i], (*plugins)[i+1:]...)
			return
		}
	}
	*plugins = append(*plugins, plugin)
}
