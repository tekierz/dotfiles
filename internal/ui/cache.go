package ui

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

// checkUpdatesCmd starts an async update check.
func checkUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		updates, err := pkg.CheckDotfilesUpdates()
		return updateCheckDoneMsg{updates: updates, err: err}
	}
}

// caskLister is implemented by package managers that distinguish casks (Homebrew).
// Used to batch cask enumeration in a single subprocess call.
type caskLister interface {
	ListInstalledCasks() ([]string, error)
}

// batchInstalledPackages returns a lookup set of every installed package name
// using batched subprocess calls (one for formulae, one for casks on brew). This
// avoids per-tool shell-outs to IsInstalled() during cache build. Returns nil if
// no manager is available or the batch query fails. Cask enumeration only applies
// to package managers that support it (Homebrew); apt/pacman are unaffected.
func batchInstalledPackages(mgr pkg.PackageManager) map[string]bool {
	if mgr == nil {
		return nil
	}

	pkgList, err := mgr.ListInstalled()
	if err != nil {
		return nil
	}

	installedPkgs := make(map[string]bool, len(pkgList))
	for _, p := range pkgList {
		installedPkgs[p.Name] = true
	}

	// Merge in casks (Homebrew only) so cask-backed tools resolve from the
	// batched result instead of each shelling out via IsInstalled().
	if cl, ok := mgr.(caskLister); ok {
		if casks, err := cl.ListInstalledCasks(); err == nil {
			for _, token := range casks {
				installedPkgs[token] = true
			}
		}
	}

	return installedPkgs
}

// allPackagesInBatch reports whether every package in pkgs is present in the
// batched installed-set. Mirrors tools.allPackagesInstalled for the batch path
// so a partially-installed multi-package tool is not reported installed (C6).
// An empty pkgs list is never considered installed.
func allPackagesInBatch(installedPkgs map[string]bool, pkgs []string) bool {
	if len(pkgs) == 0 {
		return false
	}
	for _, p := range pkgs {
		if !installedPkgs[p] {
			return false
		}
	}
	return true
}

// loadInstallCacheCmd loads installation status for all tools asynchronously
// This uses batch checking where supported (brew list --versions) for better performance.
func loadInstallCacheCmd() tea.Cmd {
	return func() tea.Msg {
		reg := tools.GetRegistry()
		all := reg.All()
		installed := make(map[string]bool, len(all)+3)

		// Try batch checking first (much faster than individual checks)
		mgr := pkg.DetectManager()
		platform := pkg.DetectPlatform()

		installedPkgs := batchInstalledPackages(mgr)

		// Check each tool
		for _, t := range all {
			found := false
			if installedPkgs != nil {
				// Use batch result - resolve packages via PackagesForPlatform so
				// that Raspberry Pi (PlatformPi) falls back to Debian package names
				// and benefits from the single batched dpkg-query rather than
				// falling through to per-tool shell-outs.
				pkgs := tools.PackagesForPlatform(t.Packages(), platform)
				if len(pkgs) > 0 {
					if allPackagesInBatch(installedPkgs, pkgs) {
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

// appendInstallLog adds a line to the install log buffer (max 500 lines).
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

// clearInstallLogs clears the log buffer and resets scroll.
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

	installedPkgs := batchInstalledPackages(mgr)

	for _, t := range all {
		found := false
		if installedPkgs != nil {
			// Use PackagesForPlatform so Raspberry Pi resolves to the Debian
			// package set and hits the batched dpkg-query path.
			pkgs := tools.PackagesForPlatform(t.Packages(), platform)
			if len(pkgs) > 0 {
				if allPackagesInBatch(installedPkgs, pkgs) {
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
