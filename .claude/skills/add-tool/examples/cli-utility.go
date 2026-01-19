// Example: Simple CLI utility tool
// Copy this template for tools like bat, eza, ripgrep, etc.

package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// ExampleCLITool represents a cross-platform CLI utility
type ExampleCLITool struct {
	BaseTool
}

// NewExampleCLITool creates a new ExampleCLI tool
func NewExampleCLITool() *ExampleCLITool {
	home, _ := os.UserHomeDir()
	return &ExampleCLITool{
		BaseTool: BaseTool{
			// Identity
			id:          "example-cli", // Used in configs and registry
			name:        "ExampleCLI",  // Display name in TUI
			description: "Example CLI utility for demonstration",
			icon:        "󰊠", // Nerd font icon (find at nerdfonts.com)
			category:    CategoryUtility,

			// Package names per platform
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"example-cli"}, // Homebrew
				pkg.PlatformArch:   {"example-cli"}, // pacman/paru
				pkg.PlatformDebian: {"example-cli"}, // apt
				// Use "all" for platform-agnostic package name:
				// "all": {"example-cli"},
			},

			// Config paths (empty if no config)
			configPaths: []string{
				filepath.Join(home, ".config", "example-cli", "config"),
			},

			// UI metadata
			uiGroup:        UIGroupCLIUtilities, // Groups with bat, eza, etc.
			configScreen:   0,                   // 0 = part of group screen
			defaultEnabled: true,                // Selected by default
			platformFilter: "",                  // Empty = all platforms

			// Resource requirements (optional)
			heavyTool: false, // Set true to skip on Pi Zero 2 (<1GB RAM)
		},
	}
}

// Optional: Override ApplyConfig if tool needs config generation
// func (t *ExampleCLITool) ApplyConfig(theme string) error {
//     // Generate and write config file
//     return nil
// }

// Optional: Override GenerateConfig for theme-aware config
// func (t *ExampleCLITool) GenerateConfig(theme string) string {
//     return "# ExampleCLI config\n"
// }
