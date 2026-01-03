package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// FzfTool represents the fzf fuzzy finder
type FzfTool struct {
	BaseTool
}

// NewFzfTool creates a new fzf tool
func NewFzfTool() *FzfTool {
	return &FzfTool{
		BaseTool: BaseTool{
			id:          "fzf",
			name:        "fzf",
			description: "Command-line fuzzy finder",
			icon:        "",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"fzf"},
				pkg.PlatformArch:   {"fzf"},
				pkg.PlatformDebian: {"fzf"},
			},
			configPaths: []string{},
		},
	}
}
