// Example: macOS-only app with IsInstalled override
// Copy this template for apps like Raycast, Rectangle, IINA, etc.

package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// ExampleMacAppTool represents a macOS-only application
type ExampleMacAppTool struct {
	BaseTool
}

// NewExampleMacAppTool creates a new ExampleMacApp tool
func NewExampleMacAppTool() *ExampleMacAppTool {
	return &ExampleMacAppTool{
		BaseTool: BaseTool{
			// Identity
			id:          "example-mac-app",
			name:        "ExampleMacApp",
			description: "Example macOS application",
			icon:        "󰀵", // Nerd font icon
			category:    CategoryApp,

			// Package names - macOS only via Homebrew Cask
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"example-mac-app"},
			},

			// No config paths for most apps
			configPaths: []string{},

			// UI metadata - IMPORTANT for macOS apps
			uiGroup:        UIGroupMacApps,    // Groups with Rectangle, Raycast
			configScreen:   0,                 // Part of group screen
			defaultEnabled: true,              // Selected by default
			platformFilter: pkg.PlatformMacOS, // CRITICAL: Only show on macOS
		},
	}
}

// IsInstalled checks if ExampleMacApp is available
// Override because app may be installed manually (not via Homebrew)
func (t *ExampleMacAppTool) IsInstalled() bool {
	// Check macOS app bundle first (most common installation method)
	// Use multiple names if app bundle name varies
	if hasMacOSApp("ExampleMacApp", "Example Mac App") {
		return true
	}
	// Fall back to Homebrew check
	return t.BaseTool.IsInstalled()
}
