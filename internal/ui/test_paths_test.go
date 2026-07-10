package ui

import "runtime"

// glowTestRelPath mirrors Glow's platform-specific user-scope config lookup.
// macOS uses go-app-paths' ~/Library/Preferences; other platforms use XDG.
func glowTestRelPath() string {
	if runtime.GOOS == "darwin" {
		return "Library/Preferences/glow/glow.yml"
	}
	return ".config/glow/glow.yml"
}
