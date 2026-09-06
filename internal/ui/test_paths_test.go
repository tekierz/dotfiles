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

// lazyGitTestRelPath mirrors LazyGit's platform default when tests do not set
// CONFIG_DIR or XDG_CONFIG_HOME explicitly.
func lazyGitTestRelPath() string {
	if runtime.GOOS == "darwin" {
		return "Library/Application Support/lazygit/config.yml"
	}
	return ".config/lazygit/config.yml"
}
