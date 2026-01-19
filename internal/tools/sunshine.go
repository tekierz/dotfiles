package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// SunshineTool represents the Sunshine game streaming server
type SunshineTool struct {
	BaseTool
}

// NewSunshineTool creates a new Sunshine tool
func NewSunshineTool() *SunshineTool {
	return &SunshineTool{
		BaseTool: BaseTool{
			id:          "sunshine",
			name:        "Sunshine",
			description: "Self-hosted game streaming server",
			icon:        "☀",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"sunshine"},
				pkg.PlatformArch:   {"sunshine"},
				pkg.PlatformDebian: {"sunshine"},
			},
			configPaths: []string{},
			// UI metadata
			uiGroup:        UIGroupGUIApps,
			configScreen:   0, // Part of GUI Apps group screen
			defaultEnabled: false,
		},
	}
}
