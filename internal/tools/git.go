package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// GitTool represents Git version control
type GitTool struct {
	BaseTool
}

// NewGitTool creates a new Git tool
func NewGitTool() *GitTool {
	home, _ := os.UserHomeDir()
	return &GitTool{
		BaseTool: BaseTool{
			id:          "git",
			name:        "Git",
			description: "Distributed version control system",
			icon:        "",
			category:    CategoryGit,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"git"},
				pkg.PlatformArch:   {"git"},
				pkg.PlatformDebian: {"git"},
			},
			configPaths: []string{
				filepath.Join(home, ".gitconfig"),
			},
		},
	}
}
