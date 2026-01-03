package tools

import (
	"github.com/tekierz/dotfiles/internal/pkg"
)

// ZenBrowserTool represents Zen Browser
type ZenBrowserTool struct {
	BaseTool
}

// NewZenBrowserTool creates a new Zen Browser tool
func NewZenBrowserTool() *ZenBrowserTool {
	return &ZenBrowserTool{
		BaseTool: BaseTool{
			id:          "zen-browser",
			name:        "Zen Browser",
			description: "Privacy-focused browser based on Firefox",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"zen-browser"},
				pkg.PlatformArch:  {"zen-browser-bin"},
			},
			configPaths: []string{},
		},
	}
}

// CursorTool represents Cursor IDE
type CursorTool struct {
	BaseTool
}

// NewCursorTool creates a new Cursor tool
func NewCursorTool() *CursorTool {
	return &CursorTool{
		BaseTool: BaseTool{
			id:          "cursor",
			name:        "Cursor",
			description: "AI-first code editor",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"cursor"},
				pkg.PlatformArch:  {"cursor-bin"},
			},
			configPaths: []string{},
		},
	}
}

// LMStudioTool represents LM Studio
type LMStudioTool struct {
	BaseTool
}

// NewLMStudioTool creates a new LM Studio tool
func NewLMStudioTool() *LMStudioTool {
	return &LMStudioTool{
		BaseTool: BaseTool{
			id:          "lm-studio",
			name:        "LM Studio",
			description: "Local LLM runner",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"lm-studio"},
				pkg.PlatformArch:  {"lm-studio"},
			},
			configPaths: []string{},
		},
	}
}

// OBSTool represents OBS Studio
type OBSTool struct {
	BaseTool
}

// NewOBSTool creates a new OBS tool
func NewOBSTool() *OBSTool {
	return &OBSTool{
		BaseTool: BaseTool{
			id:          "obs",
			name:        "OBS Studio",
			description: "Streaming and recording software",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS:  {"obs"},
				pkg.PlatformArch:   {"obs-studio"},
				pkg.PlatformDebian: {"obs-studio"},
			},
			configPaths: []string{},
		},
	}
}

// RectangleTool represents Rectangle window manager
type RectangleTool struct {
	BaseTool
}

// NewRectangleTool creates a new Rectangle tool
func NewRectangleTool() *RectangleTool {
	return &RectangleTool{
		BaseTool: BaseTool{
			id:          "rectangle",
			name:        "Rectangle",
			description: "Window management for macOS",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"rectangle"},
			},
			configPaths: []string{},
		},
	}
}

// RaycastTool represents Raycast launcher
type RaycastTool struct {
	BaseTool
}

// NewRaycastTool creates a new Raycast tool
func NewRaycastTool() *RaycastTool {
	return &RaycastTool{
		BaseTool: BaseTool{
			id:          "raycast",
			name:        "Raycast",
			description: "Productivity launcher for macOS",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"raycast"},
			},
			configPaths: []string{},
		},
	}
}

// IINATool represents IINA media player
type IINATool struct {
	BaseTool
}

// NewIINATool creates a new IINA tool
func NewIINATool() *IINATool {
	return &IINATool{
		BaseTool: BaseTool{
			id:          "iina",
			name:        "IINA",
			description: "Modern media player for macOS",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"iina"},
			},
			configPaths: []string{},
		},
	}
}

// AppCleanerTool represents AppCleaner
type AppCleanerTool struct {
	BaseTool
}

// NewAppCleanerTool creates a new AppCleaner tool
func NewAppCleanerTool() *AppCleanerTool {
	return &AppCleanerTool{
		BaseTool: BaseTool{
			id:          "appcleaner",
			name:        "AppCleaner",
			description: "Thoroughly uninstall macOS apps",
			icon:        "",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"appcleaner"},
			},
			configPaths: []string{},
		},
	}
}
