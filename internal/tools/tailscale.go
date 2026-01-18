package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// TailscaleTool represents the Tailscale mesh VPN
type TailscaleTool struct {
	BaseTool
}

// NewTailscaleTool creates a new Tailscale tool
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
		},
	}
}
