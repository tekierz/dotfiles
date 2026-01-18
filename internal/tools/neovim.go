package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// NeovimTool represents the Neovim editor
type NeovimTool struct {
	BaseTool
}

// NewNeovimTool creates a new Neovim tool
func NewNeovimTool() *NeovimTool {
	home, _ := os.UserHomeDir()
	return &NeovimTool{
		BaseTool: BaseTool{
			id:          "neovim",
			name:        "Neovim",
			description: "Hyperextensible Vim-based text editor",
			icon:        "",
			category:    CategoryEditor,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"neovim"},
				pkg.PlatformArch:   {"neovim"},
				pkg.PlatformDebian: {"neovim"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "nvim", "init.lua"),
			},
		},
	}
}
