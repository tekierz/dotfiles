package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// LazyDockerTool represents LazyDocker TUI for Docker
type LazyDockerTool struct {
	BaseTool
}

// NewLazyDockerTool creates a new LazyDocker tool
func NewLazyDockerTool() *LazyDockerTool {
	home, _ := os.UserHomeDir()
	return &LazyDockerTool{
		BaseTool: BaseTool{
			id:          "lazydocker",
			name:        "LazyDocker",
			description: "Simple terminal UI for Docker",
			icon:        "",
			category:    CategoryContainer,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"lazydocker"},
				pkg.PlatformArch:   {"lazydocker"},
				pkg.PlatformDebian: {"lazydocker"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "lazydocker", "config.yml"),
			},
		},
	}
}
