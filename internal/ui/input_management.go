package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// handleManagementKey handles key events for management screens:
// ScreenMainMenu, ScreenManage, ScreenUpdate, ScreenHotkeys, ScreenBackups, ScreenUsers,
// and ScreenManage* screens (ScreenManageGhostty, ScreenManageTmux, etc.)
func (a *App) handleManagementKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.screen {
	// Main menu navigation
	case ScreenMainMenu:
		items := GetMainMenuItems()
		switch key {
		case "up", "k":
			if a.mainMenuIndex > 0 {
				a.mainMenuIndex--
			}
		case "down", "j":
			if a.mainMenuIndex < len(items)-1 {
				a.mainMenuIndex++
			}
		case "enter":
			targetScreen := items[a.mainMenuIndex].Screen
			a.screen = targetScreen
			// Start async operations for screens that need it
			switch targetScreen {
			case ScreenManage:
				if cmd := a.startInstallCacheLoad(); cmd != nil {
					return a, cmd
				}
			case ScreenBackups:
				if !a.backupsLoading && !a.backupsLoaded {
					a.backupsLoading = true
					return a, loadBackupsCmd()
				}
			case ScreenUpdate:
				if !a.updateChecking && !a.updateCheckDone {
					a.updateChecking = true
					return a, checkUpdatesCmd()
				}
			case ScreenUsers:
				if !a.usersLoaded {
					a.usersLoaded = true
					return a, loadUsersCmd()
				}
			}
		}

	// Update screen navigation
	case ScreenUpdate:
		// Start async update check if not already running or done
		if !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return a, checkUpdatesCmd()
		}
		// Don't allow actions while update is running
		if a.updateRunning {
			return a, nil
		}
		// Handle tab navigation first
		if handled, cmd := a.handleTabNavigationWithCmd(key); handled {
			return a, cmd
		}
		switch key {
		case "up", "k":
			if a.updateIndex > 0 {
				a.updateIndex--
			}
		case "down", "j":
			a.updateIndex++
		case " ": // Toggle selection for batch update
			if len(a.updateResults) > 0 && a.updateIndex < len(a.updateResults) {
				if a.updateSelected[a.updateIndex] {
					delete(a.updateSelected, a.updateIndex)
				} else {
					a.updateSelected[a.updateIndex] = true
				}
			}
		case "enter": // Update selected or current package
			if len(a.updateResults) > 0 && !a.updateChecking && !a.updateRunning {
				var packagesToUpdate []pkg.Package
				if len(a.updateSelected) > 0 {
					// Update selected packages
					for idx := range a.updateSelected {
						if idx < len(a.updateResults) {
							packagesToUpdate = append(packagesToUpdate, a.updateResults[idx])
						}
					}
				} else if a.updateIndex < len(a.updateResults) {
					// Update current package
					packagesToUpdate = append(packagesToUpdate, a.updateResults[a.updateIndex])
				}
				if len(packagesToUpdate) > 0 {
					a.clearInstallLogs()
					a.updateStatus = fmt.Sprintf("Updating %d package(s)...", len(packagesToUpdate))
					return a, checkSudoAndUpdateCmd(packagesToUpdate, false)
				}
			}
		case "a": // Update all packages
			if len(a.updateResults) > 0 && !a.updateChecking && !a.updateRunning {
				a.clearInstallLogs()
				a.updateStatus = "Updating all packages..."
				return a, checkSudoAndUpdateCmd(nil, true)
			}
		case "r": // Refresh updates
			a.updateCheckDone = false
			a.updateChecking = true
			a.updateResults = nil
			a.updateError = nil
			a.updateStatus = ""
			a.updateSelected = make(map[int]bool)
			a.clearInstallLogs()
			return a, checkUpdatesCmd()
		case "c", "C": // Clear logs
			if !a.updateRunning && len(a.installLogs) > 0 {
				a.clearInstallLogs()
				a.updateStatus = "Logs cleared"
			}
		case "pgup", "ctrl+u": // Scroll logs up
			if len(a.installLogs) > 0 {
				a.installLogScroll += 10
				maxScroll := CalculateMaxLogScroll(len(a.installLogs), a.height-14)
				if a.installLogScroll > maxScroll {
					a.installLogScroll = maxScroll
				}
				a.installLogAutoScroll = false
			}
		case "pgdown", "ctrl+d": // Scroll logs down
			if len(a.installLogs) > 0 {
				a.installLogScroll -= 10
				if a.installLogScroll < 0 {
					a.installLogScroll = 0
				}
			}
		case "esc":
			a.screen = ScreenMainMenu
		}

	// Hotkeys screen navigation - delegates to existing handler
	case ScreenHotkeys:
		return a.handleHotkeysKey(msg)

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

	// Backups screen navigation
	case ScreenBackups:
		// Start async backup loading if not already running or done
		if !a.backupsLoading && !a.backupsLoaded {
			a.backupsLoading = true
			return a, loadBackupsCmd()
		}
		// Don't allow actions while backup operation is running
		if a.backupRunning {
			return a, nil
		}
		// Handle confirmation mode
		if a.backupConfirmMode {
			switch key {
			case "y", "Y":
				if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
					a.backupRunning = true
					backup := a.backups[a.backupIndex]
					if a.backupConfirmType == "restore" {
						return a, restoreBackupCmd(backup)
					} else if a.backupConfirmType == "delete" {
						return a, deleteBackupCmd(backup)
					}
				}
				a.backupConfirmMode = false
			case "n", "N", "esc":
				a.backupConfirmMode = false
				a.backupStatus = ""
			}
			return a, nil
		}
		// Handle tab navigation first
		if handled, cmd := a.handleTabNavigationWithCmd(key); handled {
			return a, cmd
		}
		switch key {
		case "up", "k":
			if a.backupIndex > 0 {
				a.backupIndex--
			}
		case "down", "j":
			if len(a.backups) > 0 && a.backupIndex < len(a.backups)-1 {
				a.backupIndex++
			}
		case "enter": // Restore selected backup
			if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
				a.backupConfirmMode = true
				a.backupConfirmType = "restore"
				a.backupStatus = fmt.Sprintf("Restore backup '%s'? (y/n)", a.backups[a.backupIndex].Name)
			}
		case "d", "D": // Delete selected backup
			if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
				a.backupConfirmMode = true
				a.backupConfirmType = "delete"
				a.backupStatus = fmt.Sprintf("Delete backup '%s'? (y/n)", a.backups[a.backupIndex].Name)
			}
		case "n", "N": // Create new backup
			a.backupRunning = true
			a.backupStatus = "Creating backup..."
			return a, createBackupCmd()
		case "r", "R": // Refresh backup list
			a.backupsLoaded = false
			a.backupsLoading = true
			a.backupStatus = ""
			a.backupError = nil
			return a, loadBackupsCmd()
		case "esc":
			a.screen = ScreenMainMenu
		}
	}

	return a, nil
}
