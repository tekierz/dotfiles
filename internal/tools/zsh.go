package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// ZshTool represents the Zsh shell
type ZshTool struct {
	BaseTool
}

// NewZshTool creates a new Zsh tool
func NewZshTool() *ZshTool {
	home, _ := os.UserHomeDir()
	return &ZshTool{
		BaseTool: BaseTool{
			id:          "zsh",
			name:        "Zsh",
			description: "Z shell with plugins and customization",
			icon:        "",
			category:    CategoryShell,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions"},
				pkg.PlatformArch:   {"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions"},
				pkg.PlatformDebian: {"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting"},
			},
			configPaths: []string{
				filepath.Join(home, ".zshrc"),
				filepath.Join(home, ".zshenv"),
			},
		},
	}
}
