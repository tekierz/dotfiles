package tools

import (
	"os/exec"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// TailscaleTool represents the Tailscale mesh VPN.
type TailscaleTool struct {
	BaseTool
}

// NewTailscaleTool creates a new Tailscale tool.
func NewTailscaleTool() *TailscaleTool {
	return &TailscaleTool{
		BaseTool: BaseTool{
			id:          "tailscale",
			name:        "Tailscale",
			description: "Mesh VPN for secure networking",
			icon:        "󰖂",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"tailscale"},
				pkg.PlatformArch:   {"tailscale"},
				pkg.PlatformDebian: {"tailscale"},
			},
			configPaths: []string{},
			// UI metadata
			uiGroup:        UIGroupCLIUtilities,
			configScreen:   0, // Part of CLI Utilities group screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if Tailscale is available. The `tailscale` CLI may be
// installed out-of-band (official installer, macOS app bundle) where the
// package-manager check would miss it, so probe the CLI first.
func (t *TailscaleTool) IsInstalled() bool {
	if _, err := exec.LookPath("tailscale"); err == nil {
		return true
	}
	// Fall back to package manager check
	return t.BaseTool.IsInstalled()
}
