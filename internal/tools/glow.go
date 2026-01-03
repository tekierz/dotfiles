package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// GlowTool represents glow markdown viewer
type GlowTool struct {
	BaseTool
}

// NewGlowTool creates a new glow tool
func NewGlowTool() *GlowTool {
	return &GlowTool{
		BaseTool: BaseTool{
			id:          "glow",
			name:        "Glow",
			description: "Render markdown on the CLI",
			icon:        "",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"glow"},
				pkg.PlatformArch:   {"glow"},
				pkg.PlatformDebian: {"glow"},
			},
			configPaths: []string{},
		},
	}
}
