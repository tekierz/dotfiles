package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/health"
)

// tabNavigationTarget maps a number key ("1".."9") to the corresponding
// management tab's destination screen (key N -> tab index N-1). It returns
// (0, false) for any non-digit key or an out-of-range index, so it automatically
// covers every tab in GetManagementTabs() regardless of count (e.g. the 5th tab,
// Backups). Migrated screen handlers use this to drive tab switches through the
// ScreenManager (via NavigateTo) instead of poking a.screen.
func tabNavigationTarget(key string) (Screen, bool) {
	tabs := GetManagementTabs()
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	idx := int(key[0] - '1')
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
	switch target { //nolint:exhaustive // Only management tabs have on-enter loaders.
	case ScreenUpdate:
		if !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return a.startAsync(asyncUpdates, checkUpdatesCmd())
		}
	case ScreenManage:
		return a.startInstallCacheLoad()
	case ScreenUsers:
		if !a.usersLoaded {
			a.usersLoaded = true
			return a.startAsync(asyncUsers, loadUsersCmd())
		}
	case ScreenBackups:
		if !a.backupsLoading && !a.backupsLoaded {
			a.backupsLoading = true
			return a.startAsync(asyncBackups, loadBackupsCmd())
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

	cache := a.installationSnapshotCacheView()
	fresh, _ := cliToolSnapshotFreshness(cache)
	if !fresh {
		return "pending"
	}
	installedCount := 0
	partial := false
	for _, id := range toolIDs {
		observation, observed := cache.Snapshot.Tool(id)
		if !observed {
			continue
		}
		switch observation.Presence() {
		case health.PresencePresent:
			installedCount++
		case health.PresencePartial:
			partial = true
		case health.PresenceMissing, health.PresenceUnknown:
		}
	}

	if installedCount == len(toolIDs) {
		return "installed" // All installed (blue)
	} else if installedCount > 0 || partial {
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
