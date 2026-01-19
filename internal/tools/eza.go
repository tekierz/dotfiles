package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// EzaTool represents the eza ls replacement
type EzaTool struct {
	BaseTool
}

// NewEzaTool creates a new eza tool
func NewEzaTool() *EzaTool {
	return &EzaTool{
		BaseTool: BaseTool{
			id:          "eza",
			name:        "eza",
			description: "Modern replacement for ls",
			icon:        "󰙅",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"eza"},
				pkg.PlatformArch:   {"eza"},
				pkg.PlatformDebian: {"eza"},
			},
			configPaths: []string{},
			// UI metadata
			uiGroup:        UIGroupCLIUtilities,
			configScreen:   0, // Part of CLI Utilities group screen
			defaultEnabled: true,
		},
	}
}
