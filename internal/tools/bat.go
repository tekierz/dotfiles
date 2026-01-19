package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// BatTool represents the bat cat clone
type BatTool struct {
	BaseTool
}

// NewBatTool creates a new bat tool
func NewBatTool() *BatTool {
	home, _ := os.UserHomeDir()
	return &BatTool{
		BaseTool: BaseTool{
			id:          "bat",
			name:        "bat",
			description: "A cat clone with syntax highlighting",
			icon:        "󰭟",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"bat"},
				pkg.PlatformArch:   {"bat"},
				pkg.PlatformDebian: {"bat"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "bat", "config"),
			},
			// UI metadata
			uiGroup:        UIGroupCLIUtilities,
			configScreen:   0, // Part of CLI Utilities group screen
			defaultEnabled: true,
		},
	}
}
