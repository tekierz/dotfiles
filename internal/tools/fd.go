package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// FdTool represents fd find replacement
type FdTool struct {
	BaseTool
}

// NewFdTool creates a new fd tool
func NewFdTool() *FdTool {
	return &FdTool{
		BaseTool: BaseTool{
			id:          "fd",
			name:        "fd",
			description: "Simple, fast find alternative",
			icon:        "󰈞",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"fd"},
				pkg.PlatformArch:   {"fd"},
				pkg.PlatformDebian: {"fd-find"},
			},
			configPaths: []string{},
			// UI metadata
			uiGroup:        UIGroupCLIUtilities,
			configScreen:   0, // Part of CLI Utilities group screen
			defaultEnabled: true,
		},
	}
}
