package tools

import (
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// YaziTool represents the Yazi file manager
type YaziTool struct {
	BaseTool
}

// NewYaziTool creates a new Yazi tool
func NewYaziTool() *YaziTool {
	home, _ := os.UserHomeDir()
	return &YaziTool{
		BaseTool: BaseTool{
			id:          "yazi",
			name:        "Yazi",
			description: "Blazing fast terminal file manager",
			icon:        "󰉋",
			category:    CategoryFile,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"yazi", "ffmpegthumbnailer", "unar", "jq", "poppler", "fd", "ripgrep", "fzf", "zoxide", "imagemagick"},
				pkg.PlatformArch:  {"yazi", "ffmpegthumbnailer", "unarchiver", "jq", "poppler", "fd", "ripgrep", "fzf", "zoxide", "imagemagick"},
			},
			configPaths: []string{
				filepath.Join(home, ".config", "yazi", "yazi.toml"),
				filepath.Join(home, ".config", "yazi", "keymap.toml"),
				filepath.Join(home, ".config", "yazi", "theme.toml"),
			},
		},
	}
}
