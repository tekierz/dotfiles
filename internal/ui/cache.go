package ui

import (
	"context"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

// checkUpdatesCmd starts an async update check
func checkUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		managedPackages := tools.GetRegistry().ManagedPackagesForPlatform(pkg.DetectPlatform())
		updates, err := pkg.CheckManagedUpdates(managedPackages)
		return updateCheckDoneMsg{updates: updates, err: err}
	}
}

// appendInstallLog adds a line to the install log buffer (max 500 lines)
func (a *App) appendInstallLog(line string) {
	const maxLogLines = 500
	a.installLogs = append(a.installLogs, sanitizeLogLine(line))
	// Use copy to avoid memory leak from reslicing
	if len(a.installLogs) > maxLogLines {
		copy(a.installLogs, a.installLogs[len(a.installLogs)-maxLogLines:])
		a.installLogs = a.installLogs[:maxLogLines]
	}
	// Auto-scroll to bottom if enabled
	if a.installLogAutoScroll {
		a.installLogScroll = 0 // 0 = bottom in our scroll model
	}
}

// clearInstallLogs clears the log buffer and resets scroll
func (a *App) clearInstallLogs() {
	a.installLogs = make([]string, 0, 500)
	a.installLogScroll = 0
	a.installLogAutoScroll = true
}

// ensureInstallCache populates the install status cache if not already done.
// This is kept for synchronous contexts (like collectSelectedTools before install).
// For UI rendering, use startInstallCacheLoad() and check installCacheLoading.
func (a *App) ensureInstallCache() {
	if a.manageInstalledReady {
		return
	}

	reg := tools.GetRegistry()
	all := reg.All()

	if a.manageInstalled == nil {
		a.manageInstalled = make(map[string]bool, len(all)+3) // +3 for utilities
	}

	// Try batch checking first (much faster)
	mgr := pkg.DetectManager()
	platform := pkg.DetectPlatform()

	a.manageInstalled = tools.ObserveInstallations(context.Background(), all, mgr, platform)

	// Check utility scripts in ~/.local/bin
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home != "" {
		binDir := filepath.Join(home, ".local", "bin")
		for _, util := range []string{"hk", "caff", "sshh"} {
			_, err := os.Stat(filepath.Join(binDir, util))
			a.manageInstalled[util] = err == nil
		}
	}

	a.manageInstalledReady = true
}

// startInstallCacheLoad begins async cache loading if not already loading or ready.
// Returns a command to start loading, or nil if cache is ready/loading.
func (a *App) startInstallCacheLoad() tea.Cmd {
	if a.installationSnapshotLoading || (a.installationSnapshotReady && a.manageInstalledReady) {
		return nil
	}
	return a.beginInstallationSnapshotLoad(defaultInstallationSnapshotCacheRuntime())
}
