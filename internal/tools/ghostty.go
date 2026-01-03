package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// GhosttyTool represents the Ghostty terminal emulator
type GhosttyTool struct {
	BaseTool
}

// NewGhosttyTool creates a new Ghostty tool
func NewGhosttyTool() *GhosttyTool {
	home, _ := os.UserHomeDir()
	return &GhosttyTool{
		BaseTool: BaseTool{
			id:          "ghostty",
			name:        "Ghostty",
			description: "GPU-accelerated terminal emulator",
			icon:        "",
			category:    CategoryTerminal,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"ghostty"},
				pkg.PlatformArch:  {"ghostty"},
				// Debian: not yet in repos, manual install
			},
			configPaths: []string{
				filepath.Join(home, ".config", "ghostty", "config"),
			},
		},
	}
}
