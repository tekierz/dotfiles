package tools

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

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
			// UI metadata
			uiGroup:        UIGroupGUIApps,
			configScreen:   0, // Part of GUI Apps group screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if Zen Browser is available (package, flatpak, app bundle, or command)
func (t *ZenBrowserTool) IsInstalled() bool {
	// Check command
	if _, err := exec.LookPath("zen-browser"); err == nil {
		return true
	}
	if _, err := exec.LookPath("zen"); err == nil {
		return true
	}
	// Check flatpak (Linux) - multiple possible IDs
	if isFlatpakInstalled("io.github.nicothin.zen_browser", "io.github.nicothined.zen_browser", "app.zen_browser.zen", "zen") {
		return true
	}
	// Check AppImage (Linux)
	if hasAppImage("zen", "zen-browser", "ZenBrowser") {
		return true
	}
	// Check desktop entry (Linux) - also checks Exec= field
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
			// UI metadata
			uiGroup:        UIGroupGUIApps,
			configScreen:   0, // Part of GUI Apps group screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if Cursor is available
func (t *CursorTool) IsInstalled() bool {
	if _, err := exec.LookPath("cursor"); err == nil {
		return true
	}
	// Check AppImage (Linux) - Cursor distributes as AppImage
	if hasAppImage("cursor", "Cursor") {
		return true
	}
	// Check desktop entry (Linux) - also checks Exec= field for AppImage entries
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
				// lm-studio is AUR-only on Arch; use the actual AUR package name so
				// paru can locate and build it. Plain pacman will not find this in
				// official repos, but the installer already guards AUR installs behind
				// the paru availability check.
				pkg.PlatformArch: {"lmstudio-bin"},
			},
			configPaths: []string{},
			// UI metadata
			uiGroup:        UIGroupGUIApps,
			configScreen:   0, // Part of GUI Apps group screen
			defaultEnabled: false,
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
	// Check common install locations (Linux)
	commonPaths := []string{
		"/usr/bin/lmstudio",
		"/opt/lm-studio/lm-studio",
		"/opt/LM Studio/lm-studio",
	}
	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	// Check AppImage (Linux)
	if hasAppImage("lmstudio", "lm-studio", "LM-Studio", "LM Studio") {
		return true
	}
	// Check desktop entry (Linux) - also checks Exec= field
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
			// UI metadata
			uiGroup:        UIGroupGUIApps,
			configScreen:   0, // Part of GUI Apps group screen
			defaultEnabled: false,
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
			// UI metadata
			uiGroup:        UIGroupMacApps,
			configScreen:   0, // Part of macOS Apps group screen
			defaultEnabled: true,
			platformFilter: pkg.PlatformMacOS,
		},
	}
}

// IsInstalled checks if Rectangle is available (Homebrew or app bundle)
func (t *RectangleTool) IsInstalled() bool {
	// Check macOS app bundle first
	if hasMacOSApp("Rectangle") {
		return true
	}
	// Fall back to package manager check
	return t.BaseTool.IsInstalled()
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
			// UI metadata
			uiGroup:        UIGroupMacApps,
			configScreen:   0, // Part of macOS Apps group screen
			defaultEnabled: true,
			platformFilter: pkg.PlatformMacOS,
		},
	}
}

// IsInstalled checks if Raycast is available (Homebrew or app bundle)
func (t *RaycastTool) IsInstalled() bool {
	// Check macOS app bundle first
	if hasMacOSApp("Raycast") {
		return true
	}
	// Fall back to package manager check
	return t.BaseTool.IsInstalled()
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
			// UI metadata
			uiGroup:        UIGroupMacApps,
			configScreen:   0, // Part of macOS Apps group screen
			defaultEnabled: false,
			platformFilter: pkg.PlatformMacOS,
		},
	}
}

// IsInstalled checks if IINA is available (Homebrew or app bundle)
func (t *IINATool) IsInstalled() bool {
	// Check macOS app bundle first
	if hasMacOSApp("IINA") {
		return true
	}
	// Fall back to package manager check
	return t.BaseTool.IsInstalled()
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
			// UI metadata
			uiGroup:        UIGroupMacApps,
			configScreen:   0, // Part of macOS Apps group screen
			defaultEnabled: true,
			platformFilter: pkg.PlatformMacOS,
		},
	}
}

// IsInstalled checks if AppCleaner is available (Homebrew or app bundle)
func (t *AppCleanerTool) IsInstalled() bool {
	// Check macOS app bundle first
	if hasMacOSApp("AppCleaner") {
		return true
	}
	// Fall back to package manager check
	return t.BaseTool.IsInstalled()
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

// hasDesktopEntry checks if a .desktop file exists for the app (Linux only).
// It checks both the filename AND the Exec= field content for AppImage entries
// like "appimagekit_xxx-Cursor.desktop".
func hasDesktopEntry(names ...string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// Common locations for .desktop files, including flatpak exports
	searchPaths := []string{
		filepath.Join(home, ".local", "share", "applications"),
		filepath.Join(home, ".local", "share", "flatpak", "exports", "share", "applications"),
		"/usr/share/applications",
		"/usr/local/share/applications",
		"/var/lib/flatpak/exports/share/applications",
	}

	return hasDesktopEntryInDirs(searchPaths, names...)
}

func hasDesktopEntryInDirs(searchPaths []string, names ...string) bool {
	for _, searchPath := range searchPaths {
		entries, err := os.ReadDir(searchPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}

			entryName := strings.TrimSuffix(entry.Name(), ".desktop")

			// First check: filename match (e.g., "cursor.desktop", "zen-browser.desktop")
			for _, name := range names {
				if matchesToken(entryName, name) {
					return true
				}
			}

			// Second check: read Exec= field for AppImage entries
			// AppImage desktop entries often have names like "appimagekit_xxx-Cursor.desktop"
			// but the Exec= field contains the actual AppImage path
			if strings.Contains(strings.ToLower(entryName), "appimage") {
				desktopPath := filepath.Join(searchPath, entry.Name())
				if hasDesktopEntryExec(desktopPath, names...) {
					return true
				}
			}
		}
	}
	return false
}

// hasDesktopEntryExec reads a .desktop file and checks if its Exec= line
// contains any of the given names.
func hasDesktopEntryExec(path string, names ...string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Exec=") {
			execName := execBasename(line[5:])
			for _, name := range names {
				if matchesToken(execName, name) {
					return true
				}
			}
			// Only one Exec= line matters
			return false
		}
	}
	return false
}

// hasAppImage checks if an AppImage exists in common locations (Linux only)
func hasAppImage(patterns ...string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// Common locations for AppImages
	searchPaths := []string{
		filepath.Join(home, "Applications"),
		filepath.Join(home, ".local", "bin"),
		"/opt",
		"/usr/local/bin",
	}

	return hasAppImageInDirs(searchPaths, patterns...)
}

func hasAppImageInDirs(searchPaths []string, patterns ...string) bool {
	for _, searchPath := range searchPaths {
		entries, err := os.ReadDir(searchPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			entryName := entry.Name()
			entryLower := strings.ToLower(entryName)

			// Check if it's an AppImage file matching any pattern
			for _, pattern := range patterns {
				// Match AppImage files (e.g., "Cursor-0.45.11-x86_64.AppImage")
				if (strings.HasSuffix(entryLower, ".appimage") ||
					strings.Contains(entryLower, "appimage")) &&
					matchesToken(entryName, pattern) {
					return true
				}
			}
		}
	}
	return false
}

func matchesToken(fieldName, wanted string) bool {
	fieldName = normalizeAppTokenField(fieldName)
	wanted = normalizeAppTokenField(wanted)
	if fieldName == "" || wanted == "" {
		return false
	}
	if fieldName == wanted {
		return true
	}
	fieldTokens := splitAppTokens(fieldName)
	wantedTokens := splitAppTokens(wanted)
	if len(wantedTokens) == 0 {
		return false
	}
	for i := 0; i <= len(fieldTokens)-len(wantedTokens); i++ {
		matched := true
		for j, token := range wantedTokens {
			if fieldTokens[i+j] != token {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func normalizeAppTokenField(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'`))
	value = filepath.Base(value)
	lower := strings.ToLower(value)
	for _, suffix := range []string{".desktop", ".appimage", ".app"} {
		lower = strings.TrimSuffix(lower, suffix)
	}
	return lower
}

func splitAppTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func execBasename(execValue string) string {
	execValue = strings.TrimSpace(execValue)
	if execValue == "" {
		return ""
	}

	if execValue[0] == '"' || execValue[0] == '\'' {
		quote := execValue[0]
		if end := strings.IndexByte(execValue[1:], quote); end >= 0 {
			return execValue[1 : end+1]
		}
	}

	fields := strings.Fields(execValue)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
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
