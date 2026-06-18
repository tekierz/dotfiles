package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// handleManageNavigation handles common navigation for management config screens.
//
// It returns a tea.Cmd for the back-navigation: when the back-screen is a
// migrated ScreenHandler (e.g. ScreenManage, the live dual-pane), esc routes
// through the ScreenManager via NavigateTo so the manager enters managed mode
// (setting a.screen directly would leave the manager in legacy mode and the
// migrated screen would not render). For still-legacy back-screens it sets
// a.screen directly and returns nil.
func (a *App) handleManageNavigation(key string, maxFields int, backScreen Screen) tea.Cmd {
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
		if isManagedScreen(backScreen) || backScreen == ScreenManage {
			// Migrated destination: route through the ScreenManager.
			return NavigateTo(backScreen)
		}
		a.screen = backScreen
	}
	return nil
}

// tabNavigationTarget maps a number key ("1".."4") to the corresponding
// management tab's destination screen. It returns (0, false) for any other key
// or an out-of-range index. Migrated screen handlers use this to drive tab
// switches through the ScreenManager (via NavigateTo) instead of poking a.screen.
func tabNavigationTarget(key string) (Screen, bool) {
	tabs := GetManagementTabs()
	var idx int
	switch key {
	case "1":
		idx = 0
	case "2":
		idx = 1
	case "3":
		idx = 2
	case "4":
		idx = 3
	default:
		return 0, false
	}
	if idx >= len(tabs) {
		return 0, false
	}
	target := tabs[idx].Screen
	if target == 0 {
		return 0, false
	}
	return target, true
}

// startTabTargetLoad returns the on-enter async load a management tab destination
// needs (install cache for Manage, update check for Update, user list for Users,
// backup list for Backups), or nil if none / already loaded. It is the shared
// on-enter trigger used by both the legacy handleTabNavigationWithCmd and the
// migrated screen handlers' tab navigation, so the two paths can never diverge.
func startTabTargetLoad(a *App, target Screen) tea.Cmd {
	switch target {
	case ScreenUpdate:
		if !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return checkUpdatesCmd()
		}
	case ScreenManage:
		return a.startInstallCacheLoad()
	case ScreenUsers:
		if !a.usersLoaded {
			a.usersLoaded = true
			return loadUsersCmd()
		}
	case ScreenBackups:
		if !a.backupsLoading && !a.backupsLoaded {
			a.backupsLoading = true
			return loadBackupsCmd()
		}
	}
	return nil
}

// handleTabNavigation handles number key shortcuts for tab navigation
// Returns (handled, command) - command may be nil even if handled.
//
// This is the LEGACY-screen path (Manage, Users): for migrated destinations
// (Hotkeys, Update, Backups) it routes through the ScreenManager via NavigateTo
// so the manager enters managed mode; for the still-legacy destinations it sets
// a.screen directly. Either way it kicks the destination's on-enter load.
func (a *App) handleTabNavigationWithCmd(key string) (bool, tea.Cmd) {
	targetScreen, ok := tabNavigationTarget(key)
	if !ok {
		return false, nil
	}

	load := startTabTargetLoad(a, targetScreen)

	if isManagedScreen(targetScreen) {
		// Route through the manager so it switches to managed mode for the
		// migrated destination; a.screen is synced by the manager dispatch.
		return true, tea.Batch(NavigateTo(targetScreen), load)
	}

	// Legacy destination: switch the screen field directly.
	a.screen = targetScreen
	return true, load
}

// isManagedScreen reports whether the given screen is handled by a migrated
// ScreenHandler (and therefore must be entered through the ScreenManager rather
// than by setting a.screen). Kept narrow on purpose: it lists only the
// management-tab destinations that have been migrated.
func isManagedScreen(s Screen) bool {
	switch s {
	case ScreenUsers, ScreenHotkeys, ScreenUpdate, ScreenBackups:
		return true
	default:
		return false
	}
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
