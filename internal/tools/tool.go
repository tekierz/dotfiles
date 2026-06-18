package tools

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// Category represents a tool category
type Category string

const (
	CategoryShell     Category = "shell"
	CategoryTerminal  Category = "terminal"
	CategoryEditor    Category = "editor"
	CategoryFile      Category = "file"
	CategoryGit       Category = "git"
	CategoryContainer Category = "container"
	CategoryUtility   Category = "utility"
	CategoryApp       Category = "app"
)

// UIGroup represents which installer section a tool belongs to
type UIGroup string

const (
	UIGroupNone         UIGroup = "" // Has dedicated config screen
	UIGroupCLITools     UIGroup = "cli-tools"
	UIGroupCLIUtilities UIGroup = "cli-utilities"
	UIGroupGUIApps      UIGroup = "gui-apps"
	UIGroupMacApps      UIGroup = "macos-apps"
	UIGroupUtilities    UIGroup = "utilities" // Shell scripts (hk, caff, sshh)
)

// Tool defines the interface for all managed tools
type Tool interface {
	// Identity
	ID() string          // Unique identifier (e.g., "ghostty", "lazygit")
	Name() string        // Display name (e.g., "Ghostty", "LazyGit")
	Description() string // Short description
	Icon() string        // Nerd font icon
	Category() Category  // Tool category

	// Package management
	Packages() map[pkg.Platform][]string // Platform-specific package names
	IsInstalled() bool                   // Check if tool is installed
	Install(mgr pkg.PackageManager) error

	// Configuration
	ConfigPaths() []string // Config file paths (e.g., ~/.config/ghostty/config)
	HasConfig() bool       // Whether this tool has configurable options

	// Resource requirements
	IsHeavy() bool // Whether this tool requires significant resources (skip on low-memory systems)

	// UI metadata for installer screens
	UIGroup() UIGroup             // Which installer group (empty = dedicated screen)
	ConfigScreen() int            // Which config screen constant (0 = part of group screen)
	DefaultEnabled() bool         // Default state in installer
	PlatformFilter() pkg.Platform // Empty for all platforms, or specific platform
}

// BaseTool provides common functionality for tools
type BaseTool struct {
	id          string
	name        string
	description string
	icon        string
	category    Category
	packages    map[pkg.Platform][]string
	configPaths []string
	heavyTool   bool // If true, tool is skipped on low-memory systems (e.g., Pi Zero 2)

	// UI metadata
	uiGroup        UIGroup
	configScreen   int // Screen constant as int to avoid import cycle
	defaultEnabled bool
	platformFilter pkg.Platform
}

func (t *BaseTool) ID() string          { return t.id }
func (t *BaseTool) Name() string        { return t.name }
func (t *BaseTool) Description() string { return t.description }
func (t *BaseTool) Icon() string        { return t.icon }
func (t *BaseTool) Category() Category  { return t.category }

func (t *BaseTool) Packages() map[pkg.Platform][]string {
	return t.packages
}

func (t *BaseTool) ConfigPaths() []string {
	return t.configPaths
}

func (t *BaseTool) HasConfig() bool {
	return len(t.configPaths) > 0
}

func (t *BaseTool) IsHeavy() bool {
	return t.heavyTool
}

func (t *BaseTool) UIGroup() UIGroup             { return t.uiGroup }
func (t *BaseTool) ConfigScreen() int            { return t.configScreen }
func (t *BaseTool) DefaultEnabled() bool         { return t.defaultEnabled }
func (t *BaseTool) PlatformFilter() pkg.Platform { return t.platformFilter }

// PackagesForPlatform resolves the package names for a given platform from a
// tool's package map. Resolution order is: exact platform, then (for a
// Raspberry Pi, which uses Debian packages) the Debian entry, then the "all"
// fallback key. This is the single source of truth for package resolution and
// must be used everywhere packages are looked up so that documented platforms
// like the Raspberry Pi are never silently skipped.
func PackagesForPlatform(packages map[pkg.Platform][]string, platform pkg.Platform) []string {
	if pkgs := packages[platform]; len(pkgs) > 0 {
		return pkgs
	}
	// Raspberry Pi reuses Debian packages (same apt manager).
	if platform == pkg.PlatformPi {
		if pkgs := packages[pkg.PlatformDebian]; len(pkgs) > 0 {
			return pkgs
		}
	}
	return packages["all"]
}

func (t *BaseTool) IsInstalled() bool {
	mgr := pkg.DetectManager()
	if mgr == nil {
		return false
	}

	pkgs := PackagesForPlatform(t.packages, pkg.DetectPlatform())
	if len(pkgs) == 0 {
		return false
	}

	// Check if primary package is installed
	return mgr.IsInstalled(pkgs[0])
}

func (t *BaseTool) Install(mgr pkg.PackageManager) error {
	pkgs := PackagesForPlatform(t.packages, pkg.DetectPlatform())
	if len(pkgs) == 0 {
		return nil // No packages to install for this platform
	}
	return mgr.Install(pkgs...)
}

// writeToolConfig writes a generated tool config file to disk, creating its
// parent directory if needed. It is the single source of truth for the
// "create dir 0700, write file 0600" pattern shared by every WriteXConfig
// function. Permissions are deliberately restrictive: 0700 on the config
// directory and 0600 on the file (see the security notes in CLAUDE.md). For
// configs that live directly in $HOME (e.g. ~/.zshrc, ~/.gitconfig) the parent
// directory already exists, so MkdirAll is a no-op and does not alter $HOME.
func writeToolConfig(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}
	return nil
}
