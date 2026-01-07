package tools

import (
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
	ConfigPaths() []string              // Config file paths (e.g., ~/.config/ghostty/config)
	HasConfig() bool                    // Whether this tool has configurable options
	GenerateConfig(theme string) string // Generate config content for a theme
	ApplyConfig(theme string) error     // Write config to disk
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

func (t *BaseTool) IsInstalled() bool {
	platform := pkg.DetectPlatform()
	mgr := pkg.DetectManager()
	if mgr == nil {
		return false
	}

	pkgs := t.packages[platform]
	if len(pkgs) == 0 {
		// Try "all" platform
		pkgs = t.packages["all"]
	}
	if len(pkgs) == 0 {
		return false
	}

	// Check if primary package is installed
	return mgr.IsInstalled(pkgs[0])
}

func (t *BaseTool) Install(mgr pkg.PackageManager) error {
	platform := pkg.DetectPlatform()
	pkgs := t.packages[platform]
	if len(pkgs) == 0 {
		pkgs = t.packages["all"]
	}
	if len(pkgs) == 0 {
		return nil // No packages to install for this platform
	}
	return mgr.Install(pkgs...)
}

// GenerateConfig default implementation (override in specific tools)
func (t *BaseTool) GenerateConfig(theme string) string {
	return ""
}

// ApplyConfig default implementation (override in specific tools)
func (t *BaseTool) ApplyConfig(theme string) error {
	return nil
}
