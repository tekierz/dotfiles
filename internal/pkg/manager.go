package pkg

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/tekierz/dotfiles/internal/runner"
)

// Cached platform and manager detection using sync.Once for thread-safe lazy initialization.
// This avoids redundant file I/O and exec.LookPath() calls when DetectPlatform() and
// DetectManager() are called multiple times (e.g., once per tool during registry operations).
var (
	cachedPlatform     Platform
	cachedPlatformOnce sync.Once

	cachedManager     PackageManager
	cachedManagerOnce sync.Once
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

	// NeedsSudo returns true if package operations require elevated privileges
	NeedsSudo() bool

	// InstallStreaming installs packages with real-time output streaming
	InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error)

	// UpdateStreaming updates packages with real-time output streaming
	UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error)

	// UpdateAllStreaming updates all packages with real-time output streaming
	UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error)
}

// Platform represents the current operating system
type Platform string

const (
	PlatformMacOS   Platform = "macos"
	PlatformArch    Platform = "arch"
	PlatformDebian  Platform = "debian"
	PlatformPi      Platform = "pi" // Raspberry Pi (uses Debian packages)
	PlatformUnknown Platform = "unknown"
)

// DetectPlatform detects the current platform (cached after first call)
func DetectPlatform() Platform {
	cachedPlatformOnce.Do(func() {
		cachedPlatform = detectPlatformImpl()
	})
	return cachedPlatform
}

// detectPlatformImpl performs the actual platform detection
func detectPlatformImpl() Platform {
	switch runtime.GOOS {
	case "darwin":
		return PlatformMacOS
	case "linux":
		// Check for Raspberry Pi first (it also has /etc/debian_version)
		if isRaspberryPi() {
			return PlatformPi
		}
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

// DetectManager returns the appropriate package manager for the current system (cached after first call)
func DetectManager() PackageManager {
	cachedManagerOnce.Do(func() {
		cachedManager = detectManagerImpl()
	})
	return cachedManager
}

// detectManagerImpl performs the actual package manager detection
func detectManagerImpl() PackageManager {
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
	case PlatformDebian, PlatformPi:
		// Raspberry Pi uses apt like Debian
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
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// isRaspberryPi detects if running on a Raspberry Pi by checking device tree model
func isRaspberryPi() bool {
	// Check device tree model (most reliable method)
	if data, err := os.ReadFile("/sys/firmware/devicetree/base/model"); err == nil {
		model := strings.ToLower(string(data))
		if strings.Contains(model, "raspberry pi") {
			return true
		}
	}

	// Fallback: check /proc/cpuinfo for Raspberry Pi
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.ToLower(scanner.Text())
			if strings.Contains(line, "raspberry pi") {
				return true
			}
			// Check for BCM2835/BCM2836/BCM2837/BCM2711/BCM2712 (Pi SoCs)
			if strings.HasPrefix(line, "hardware") && strings.Contains(line, "bcm2") {
				return true
			}
		}
	}

	return false
}

// Cached memory detection
var (
	cachedTotalMemoryMB     int
	cachedTotalMemoryMBOnce sync.Once
)

// GetTotalMemoryMB returns the total system memory in MB (cached after first call)
func GetTotalMemoryMB() int {
	cachedTotalMemoryMBOnce.Do(func() {
		cachedTotalMemoryMB = getTotalMemoryMBImpl()
	})
	return cachedTotalMemoryMB
}

// getTotalMemoryMBImpl reads total memory from /proc/meminfo
func getTotalMemoryMBImpl() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			// Format: "MemTotal:       16384000 kB"
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.Atoi(fields[1])
				if err == nil {
					return kb / 1024 // Convert to MB
				}
			}
			break
		}
	}

	return 0
}

// IsLowMemorySystem returns true if system has less than threshold MB of RAM.
// Default threshold is 1024 MB (1 GB). This is useful for Raspberry Pi Zero 2 (512MB)
// and similar constrained devices.
func IsLowMemorySystem(thresholdMB int) bool {
	if thresholdMB <= 0 {
		thresholdMB = 1024 // Default 1GB threshold
	}
	return GetTotalMemoryMB() > 0 && GetTotalMemoryMB() < thresholdMB
}
