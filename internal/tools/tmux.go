package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// TmuxTool represents tmux terminal multiplexer
type TmuxTool struct {
	BaseTool
}

// NewTmuxTool creates a new Tmux tool
func NewTmuxTool() *TmuxTool {
	home, _ := os.UserHomeDir()
	return &TmuxTool{
		BaseTool: BaseTool{
			id:          "tmux",
			name:        "Tmux",
			description: "Terminal multiplexer",
			icon:        "",
			category:    CategoryTerminal,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"tmux"},
				pkg.PlatformArch:   {"tmux"},
				pkg.PlatformDebian: {"tmux"},
			},
			configPaths: []string{
				filepath.Join(home, ".tmux.conf"),
			},
		},
	}
}
