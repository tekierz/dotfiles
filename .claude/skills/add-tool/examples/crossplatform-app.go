// Example: Cross-platform GUI app with comprehensive IsInstalled
// Copy this template for apps like Cursor, OBS, Zen Browser, LM Studio

package tools

import (
	"os/exec"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// ExampleAppTool represents a cross-platform GUI application
type ExampleAppTool struct {
	BaseTool
}

// NewExampleAppTool creates a new ExampleApp tool
func NewExampleAppTool() *ExampleAppTool {
	return &ExampleAppTool{
		BaseTool: BaseTool{
			// Identity
			id:          "example-app",
			name:        "ExampleApp",
			description: "Cross-platform GUI application",
			icon:        "󰊯", // Nerd font icon
			category:    CategoryApp,

			// Package names per platform
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"example-app"},     // Homebrew Cask
				pkg.PlatformArch:  {"example-app-bin"}, // AUR (often -bin suffix)
				// Debian: Often not in apt, installed via AppImage/Flatpak
			},

			// No config paths for most GUI apps
			configPaths: []string{},

			// UI metadata
			uiGroup:        UIGroupGUIApps, // Cross-platform apps group
			configScreen:   0,
			defaultEnabled: false, // Opt-in for GUI apps
			platformFilter: "",    // Empty = available on all platforms
		},
	}
}

// IsInstalled checks if ExampleApp is available via any installation method
// Cross-platform apps have many possible installation sources
func (t *ExampleAppTool) IsInstalled() bool {
	// 1. Check if command is in PATH
	if _, err := exec.LookPath("example-app"); err == nil {
		return true
	}
	// Alternative command names
	if _, err := exec.LookPath("exampleapp"); err == nil {
		return true
	}

	// 2. Check Flatpak (Linux)
	// Find IDs with: flatpak search example-app
	if isFlatpakInstalled("com.example.app", "io.example.app") {
		return true
	}

	// 3. Check AppImage (Linux)
	// AppImages are often in ~/Applications or ~/.local/bin
	if hasAppImage("example", "ExampleApp", "example-app") {
		return true
	}

	// 4. Check desktop entry (Linux)
	// This also catches AppImage integrations
	if hasDesktopEntry("example-app", "exampleapp", "ExampleApp") {
		return true
	}

	// 5. Check macOS app bundle
	if hasMacOSApp("ExampleApp", "Example App") {
		return true
	}

	// 6. Fall back to package manager check
	return t.BaseTool.IsInstalled()
}
