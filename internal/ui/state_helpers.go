package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

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
// on-enter trigger used by the migrated screen handlers' tab navigation, so every
// entry path kicks the same load.
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
