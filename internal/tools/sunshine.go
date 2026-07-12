package tools

import (
	"os/exec"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// SunshineTool represents the Sunshine game streaming server
type SunshineTool struct {
	BaseTool
}

// NewSunshineTool creates a new Sunshine tool
func NewSunshineTool() *SunshineTool {
	return &SunshineTool{
		BaseTool: BaseTool{
			id:          "sunshine",
			name:        "Sunshine",
			description: "Self-hosted game streaming server",
			icon:        "☀",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"sunshine"},
				pkg.PlatformArch:   {"sunshine"},
				pkg.PlatformDebian: {"sunshine"},
			},
			configPaths: []string{},
			// UI metadata
			uiGroup:        UIGroupGUIApps,
			configScreen:   0, // Part of GUI Apps group screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if Sunshine is available (command, app bundle, or package).
// Sunshine is a Homebrew cask / GUI app, so detect out-of-band installs the same
// way the other GUI apps do before falling back to the package manager.
func (t *SunshineTool) IsInstalled() bool {
	return directInstallationDetected(t) || t.BaseTool.IsInstalled()
}

func (t *SunshineTool) IsInstalledOutsidePackageManager(DirectInstallationObservation) bool {
	if _, err := exec.LookPath("sunshine"); err == nil {
		return true
	}
	// Check desktop entry (Linux)
	if hasDesktopEntry("sunshine", "Sunshine") {
		return true
	}
	// Check macOS app bundle
	if hasMacOSApp("Sunshine") {
		return true
	}
	return false
}
