package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// DeltaTool represents the Delta diff viewer
type DeltaTool struct {
	BaseTool
}

// NewDeltaTool creates a new Delta tool
func NewDeltaTool() *DeltaTool {
	return &DeltaTool{
		BaseTool: BaseTool{
			id:          "delta",
			name:        "Delta",
			description: "Syntax-highlighting pager for git diffs",
			icon:        "󰘧",
			category:    CategoryGit,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"git-delta"},
				pkg.PlatformArch:   {"git-delta"},
				pkg.PlatformDebian: {"git-delta"},
			},
			configPaths: []string{},
		},
	}
}
