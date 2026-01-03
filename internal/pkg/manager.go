package pkg

import (
	"os/exec"
	"runtime"
)

// Package represents a package with version info
type Package struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	Outdated       bool   `json:"outdated"`
	InstalledBy    string `json:"installed_by"` // brew, pacman, apt, manual
	Description    string `json:"description,omitempty"`
}

// PackageManager defines the interface for package management operations
type PackageManager interface {
	// Name returns the package manager name (brew, pacman, apt)
	Name() string

	// IsAvailable checks if this package manager is available on the system
	IsAvailable() bool

	// Install installs one or more packages
	Install(packages ...string) error

	// Uninstall removes one or more packages
	Uninstall(packages ...string) error

	// IsInstalled checks if a specific package is installed
	IsInstalled(pkg string) bool

	// GetVersion returns the installed version of a package
	GetVersion(pkg string) (string, error)

	// CheckOutdated returns a list of outdated packages
	CheckOutdated() ([]Package, error)

	// Update updates specific packages
	Update(packages ...string) error

	// UpdateAll updates all outdated packages
	UpdateAll() error

	// Search searches for packages matching a query
	Search(query string) ([]Package, error)

	// ListInstalled returns all installed packages
	ListInstalled() ([]Package, error)
}

// Platform represents the current operating system
type Platform string

const (
	PlatformMacOS   Platform = "macos"
	PlatformArch    Platform = "arch"
	PlatformDebian  Platform = "debian"
	PlatformUnknown Platform = "unknown"
)

// DetectPlatform detects the current platform
func DetectPlatform() Platform {
	switch runtime.GOOS {
	case "darwin":
		return PlatformMacOS
	case "linux":
		// Check for Arch
		if fileExists("/etc/arch-release") || fileExists("/etc/cachyos-release") {
			return PlatformArch
		}
		// Check for Debian/Ubuntu
		if fileExists("/etc/debian_version") {
			return PlatformDebian
		}
	}
	return PlatformUnknown
}

// DetectManager returns the appropriate package manager for the current system
func DetectManager() PackageManager {
	platform := DetectPlatform()

	switch platform {
	case PlatformMacOS:
		if brew := NewBrewManager(); brew.IsAvailable() {
			return brew
		}
	case PlatformArch:
		// Prefer paru over pacman for AUR support
		if paru := NewPacmanManager(true); paru.IsAvailable() {
			return paru
		}
		if pacman := NewPacmanManager(false); pacman.IsAvailable() {
			return pacman
		}
	case PlatformDebian:
		if apt := NewAptManager(); apt.IsAvailable() {
			return apt
		}
	}

	return nil
}

// AllManagers returns all available package managers on the system
func AllManagers() []PackageManager {
	var managers []PackageManager

	if brew := NewBrewManager(); brew.IsAvailable() {
		managers = append(managers, brew)
	}
	if paru := NewPacmanManager(true); paru.IsAvailable() {
		managers = append(managers, paru)
	}
	if pacman := NewPacmanManager(false); pacman.IsAvailable() {
		managers = append(managers, pacman)
	}
	if apt := NewAptManager(); apt.IsAvailable() {
		managers = append(managers, apt)
	}

	return managers
}

// Helper functions

func fileExists(path string) bool {
	_, err := exec.LookPath(path)
	if err == nil {
		return true
	}
	// Also check as a regular file
	cmd := exec.Command("test", "-f", path)
	return cmd.Run() == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
