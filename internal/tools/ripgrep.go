package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// RipgrepTool represents ripgrep search tool
type RipgrepTool struct {
	BaseTool
}

// NewRipgrepTool creates a new ripgrep tool
func NewRipgrepTool() *RipgrepTool {
	home, _ := os.UserHomeDir()
	return &RipgrepTool{
		BaseTool: BaseTool{
			id:          "ripgrep",
			name:        "ripgrep",
			description: "Fast recursive grep alternative",
			icon:        "󰑐",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"ripgrep"},
				pkg.PlatformArch:   {"ripgrep"},
				pkg.PlatformDebian: {"ripgrep"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "ripgrep", "config"),
			},
			// UI metadata
			uiGroup:        UIGroupCLIUtilities,
			configScreen:   0, // Part of CLI Utilities group screen
			defaultEnabled: true,
		},
	}
}
