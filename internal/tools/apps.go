package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
			icon:        "󰖟",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"zen-browser"},
				pkg.PlatformArch:  {"zen-browser-bin"},
			},
			configPaths: []string{},
		},
	}
}

// IsInstalled checks if Zen Browser is available (package, flatpak, app bundle, or command)
func (t *ZenBrowserTool) IsInstalled() bool {
	// Check command
	if _, err := exec.LookPath("zen-browser"); err == nil {
		return true
	}
	// Check flatpak (Linux)
	if isFlatpakInstalled("io.github.nicothin.zen_browser", "zen") {
		return true
	}
	// Check desktop entry (Linux)
	if hasDesktopEntry("zen-browser", "zen") {
		return true
	}
	// Check macOS app bundle
	if hasMacOSApp("Zen Browser", "Zen") {
		return true
	}
	// Fall back to package manager check
	return t.BaseTool.IsInstalled()
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
			icon:        "󰦨",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"cursor"},
				pkg.PlatformArch:  {"cursor-bin"},
			},
			configPaths: []string{},
		},
	}
}

// IsInstalled checks if Cursor is available
func (t *CursorTool) IsInstalled() bool {
	if _, err := exec.LookPath("cursor"); err == nil {
		return true
	}
	// Check desktop entry (Linux)
	if hasDesktopEntry("cursor", "Cursor") {
		return true
	}
	// Check macOS app bundle
	if hasMacOSApp("Cursor") {
		return true
	}
	return t.BaseTool.IsInstalled()
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
			icon:        "󰚩",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"lm-studio"},
				pkg.PlatformArch:  {"lm-studio"},
			},
			configPaths: []string{},
		},
	}
}

// IsInstalled checks if LM Studio is available
func (t *LMStudioTool) IsInstalled() bool {
	if _, err := exec.LookPath("lm-studio"); err == nil {
		return true
	}
	if _, err := exec.LookPath("lmstudio"); err == nil {
		return true
	}
	// Check desktop entry (Linux)
	if hasDesktopEntry("lmstudio", "lm-studio", "LM Studio", "LM-Studio") {
		return true
	}
	// Check macOS app bundle
	if hasMacOSApp("LM Studio", "LMStudio") {
		return true
	}
	return t.BaseTool.IsInstalled()
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
			icon:        "",
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

// IsInstalled checks if OBS Studio is available
func (t *OBSTool) IsInstalled() bool {
	if _, err := exec.LookPath("obs"); err == nil {
		return true
	}
	// Check desktop entry (Linux)
	if hasDesktopEntry("obs", "obs-studio", "OBS") {
		return true
	}
	// Check macOS app bundle
	if hasMacOSApp("OBS", "OBS Studio") {
		return true
	}
	return t.BaseTool.IsInstalled()
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
			icon:        "󰍹",
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
			icon:        "󰈸",
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
			icon:        "󰕼",
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
			icon:        "󰃢",
			category:    CategoryApp,
			packages: map[pkg.Platform][]string{
				pkg.PlatformMacOS: {"appcleaner"},
			},
			configPaths: []string{},
		},
	}
}

// Helper functions for detecting installed apps

// isFlatpakInstalled checks if a flatpak app is installed (Linux only)
func isFlatpakInstalled(appIDs ...string) bool {
	flatpak, err := exec.LookPath("flatpak")
	if err != nil {
		return false
	}

	for _, appID := range appIDs {
		cmd := exec.Command(flatpak, "info", appID)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

// hasDesktopEntry checks if a .desktop file exists for the app (Linux only)
func hasDesktopEntry(names ...string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// Common locations for .desktop files
	searchPaths := []string{
		filepath.Join(home, ".local", "share", "applications"),
		"/usr/share/applications",
		"/usr/local/share/applications",
	}

	for _, searchPath := range searchPaths {
		entries, err := os.ReadDir(searchPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".desktop") {
				entryLower := strings.ToLower(entry.Name())
				for _, name := range names {
					if strings.Contains(entryLower, strings.ToLower(name)) {
						return true
					}
				}
			}
		}
	}
	return false
}

// hasMacOSApp checks if a .app bundle exists in /Applications (macOS only)
func hasMacOSApp(names ...string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// Check both system and user Applications folders
	searchPaths := []string{
		"/Applications",
		filepath.Join(home, "Applications"),
	}

	for _, searchPath := range searchPaths {
		entries, err := os.ReadDir(searchPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() && strings.HasSuffix(entry.Name(), ".app") {
				entryLower := strings.ToLower(entry.Name())
				for _, name := range names {
					if strings.Contains(entryLower, strings.ToLower(name)) {
						return true
					}
				}
			}
		}
	}
	return false
}
