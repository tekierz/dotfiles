package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// ZoxideTool represents the zoxide directory jumper
type ZoxideTool struct {
	BaseTool
}

// NewZoxideTool creates a new zoxide tool
func NewZoxideTool() *ZoxideTool {
	return &ZoxideTool{
		BaseTool: BaseTool{
			id:          "zoxide",
			name:        "zoxide",
			description: "Smarter cd command with learning",
			icon:        "󰄛",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"zoxide"},
				pkg.PlatformArch:   {"zoxide"},
				pkg.PlatformDebian: {"zoxide"},
			},
			configPaths: []string{},
		},
	}
}
