package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// LazyGitTool represents LazyGit TUI for Git
type LazyGitTool struct {
	BaseTool
}

// NewLazyGitTool creates a new LazyGit tool
func NewLazyGitTool() *LazyGitTool {
	home, _ := os.UserHomeDir()
	return &LazyGitTool{
		BaseTool: BaseTool{
			id:          "lazygit",
			name:        "LazyGit",
			description: "Simple terminal UI for Git commands",
			icon:        "",
			category:    CategoryGit,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"lazygit"},
				pkg.PlatformArch:   {"lazygit"},
				pkg.PlatformDebian: {"lazygit"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "lazygit", "config.yml"),
			},
		},
	}
}
