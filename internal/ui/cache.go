package ui

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

// checkUpdatesCmd starts an async update check
func checkUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		updates, err := pkg.CheckDotfilesUpdates()
		return updateCheckDoneMsg{updates: updates, err: err}
	}
}

// loadInstallCacheCmd loads installation status for all tools asynchronously
// This uses batch checking where supported (brew list --versions) for better performance
func loadInstallCacheCmd() tea.Cmd {
	return func() tea.Msg {
		reg := tools.GetRegistry()
		all := reg.All()
		installed := make(map[string]bool, len(all)+3)

		// Try batch checking first (much faster than individual checks)
		mgr := pkg.DetectManager()
		platform := pkg.DetectPlatform()

		var installedPkgs map[string]bool
		if mgr != nil {
			// Get all installed packages in one call
			pkgList, err := mgr.ListInstalled()
			if err == nil {
				installedPkgs = make(map[string]bool, len(pkgList))
				for _, p := range pkgList {
					installedPkgs[p.Name] = true
				}
			}
		}

		// Check each tool
		for _, t := range all {
			found := false
			if installedPkgs != nil {
				// Use batch result - check if primary package is installed
				pkgs := t.Packages()[platform]
				if len(pkgs) == 0 {
					pkgs = t.Packages()["all"]
				}
				if len(pkgs) > 0 {
					if installedPkgs[pkgs[0]] {
						installed[t.ID()] = true
						found = true
					}
				}
			}
			// Fall back to IsInstalled() for tools not found via package manager
			// (e.g., flatpaks, AppImages, direct binaries)
			if !found {
				installed[t.ID()] = t.IsInstalled()
			}
		}

		// Check utility scripts in ~/.local/bin (always fast - just file existence)
		home := os.Getenv("HOME")
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		if home != "" {
			binDir := filepath.Join(home, ".local", "bin")
			for _, util := range []string{"hk", "caff", "sshh"} {
				_, err := os.Stat(filepath.Join(binDir, util))
				installed[util] = err == nil
			}
		}

		return installCacheDoneMsg{installed: installed}
	}
}

// appendInstallLog adds a line to the install log buffer (max 500 lines)
func (a *App) appendInstallLog(line string) {
	const maxLogLines = 500
	a.installLogs = append(a.installLogs, line)
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

	var installedPkgs map[string]bool
	if mgr != nil {
		pkgList, err := mgr.ListInstalled()
		if err == nil {
			installedPkgs = make(map[string]bool, len(pkgList))
			for _, p := range pkgList {
				installedPkgs[p.Name] = true
			}
		}
	}

	for _, t := range all {
		found := false
		if installedPkgs != nil {
			pkgs := t.Packages()[platform]
			if len(pkgs) == 0 {
				pkgs = t.Packages()["all"]
			}
			if len(pkgs) > 0 {
				if installedPkgs[pkgs[0]] {
					a.manageInstalled[t.ID()] = true
					found = true
				}
			}
		}
		// Fall back to IsInstalled() for tools not found via package manager
		// (e.g., flatpaks, AppImages, direct binaries)
		if !found {
			a.manageInstalled[t.ID()] = t.IsInstalled()
		}
	}

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
	if a.manageInstalledReady || a.installCacheLoading {
		return nil
	}
	a.installCacheLoading = true
	return loadInstallCacheCmd()
}

// isInstallCacheReady returns true if the cache is ready, false if loading or not started
func (a *App) isInstallCacheReady() bool {
	return a.manageInstalledReady
}
