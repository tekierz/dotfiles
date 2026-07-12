package tools

import (
	"os/exec"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// MoonlightTool represents the Moonlight game streaming client
type MoonlightTool struct {
	BaseTool
}

// NewMoonlightTool creates a new Moonlight tool
func NewMoonlightTool() *MoonlightTool {
	return &MoonlightTool{
		BaseTool: BaseTool{
			id:          "moonlight",
			name:        "Moonlight",
			description: "Open-source game streaming client",
			icon:        "🌙",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"moonlight"},
				pkg.PlatformArch:   {"moonlight-qt"},
				pkg.PlatformDebian: {"moonlight-qt"},
			},
			configPaths: []string{},
			// UI metadata
			uiGroup:        UIGroupGUIApps,
			configScreen:   0, // Part of GUI Apps group screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if Moonlight is available (command, app bundle, or package).
// Moonlight is a Homebrew cask / GUI app, so detect out-of-band installs the same
// way the other GUI apps do before falling back to the package manager.
func (t *MoonlightTool) IsInstalled() bool {
	return directInstallationDetected(t) || t.BaseTool.IsInstalled()
}

func (t *MoonlightTool) IsInstalledOutsidePackageManager(DirectInstallationObservation) bool {
	if _, err := exec.LookPath("moonlight"); err == nil {
		return true
	}
	if _, err := exec.LookPath("moonlight-qt"); err == nil {
		return true
	}
	// Check desktop entry (Linux)
	if hasDesktopEntry("moonlight", "moonlight-qt", "Moonlight") {
		return true
	}
	// Check macOS app bundle
	if hasMacOSApp("Moonlight") {
		return true
	}
	return false
}
