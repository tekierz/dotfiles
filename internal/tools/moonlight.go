package tools

import (
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
		},
	}
}
