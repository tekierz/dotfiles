package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// FswatchTool represents fswatch file watcher
type FswatchTool struct {
	BaseTool
}

// NewFswatchTool creates a new fswatch tool
func NewFswatchTool() *FswatchTool {
	return &FswatchTool{
		BaseTool: BaseTool{
			id:          "fswatch",
			name:        "fswatch",
			description: "Cross-platform file change monitor",
			icon:        "󱄄",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"fswatch"},
				pkg.PlatformArch:   {"fswatch"},
				pkg.PlatformDebian: {"fswatch"},
			},
			configPaths: []string{},
		},
	}
}
