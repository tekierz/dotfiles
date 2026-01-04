package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// BtopTool represents btop system monitor
type BtopTool struct {
	BaseTool
}

// NewBtopTool creates a new btop tool
func NewBtopTool() *BtopTool {
	home, _ := os.UserHomeDir()
	return &BtopTool{
		BaseTool: BaseTool{
			id:          "btop",
			name:        "btop",
			description: "Resource monitor with TUI",
			icon:        "󰄨",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"btop"},
				pkg.PlatformArch:   {"btop"},
				pkg.PlatformDebian: {"btop"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "btop", "btop.conf"),
			},
		},
	}
}
