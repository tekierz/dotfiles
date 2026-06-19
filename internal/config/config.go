package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNoConfigDir is returned when the config directory cannot be determined
// (e.g. neither XDG_CONFIG_HOME nor HOME is set and os.UserHomeDir fails).
// It guards against silently building relative paths from an empty base.
var ErrNoConfigDir = errors.New("cannot determine config directory: HOME and XDG_CONFIG_HOME are unset")

// writeFileAtomic writes data to path atomically by writing to a temporary
// file in the same directory and renaming it over the destination. This
// prevents a truncated/half-written file if the process is interrupted
// mid-write (rename is atomic on the same filesystem).
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we fail before the rename.
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// GlobalConfig holds global dotfiles settings
type GlobalConfig struct {
	Theme             string `json:"theme"`
	NavStyle          string `json:"nav_style"`
	ActiveUser        string `json:"active_user,omitempty"`
	DisableAnimations bool   `json:"disable_animations,omitempty"`

	// Backup settings
	AutoBackup       bool `json:"auto_backup"`         // Create backup before install/config changes
	BackupMaxCount   int  `json:"backup_max_count"`    // Max number of backups to keep (0 = unlimited)
	BackupMaxAgeDays int  `json:"backup_max_age_days"` // Delete backups older than this (0 = keep forever)
}

// DefaultGlobalConfig returns default global settings
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Theme:            "catppuccin-mocha",
		NavStyle:         "emacs",
		AutoBackup:       true, // Auto-backup enabled by default
		BackupMaxCount:   10,   // Keep last 10 backups
		BackupMaxAgeDays: 30,   // Delete backups older than 30 days
	}
}

// ConfigDir returns the dotfiles config directory path
// Returns empty string if HOME is not set and XDG_CONFIG_HOME is not available
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Clean(filepath.Join(xdg, "dotfiles"))
	}
	home := os.Getenv("HOME")
	if home == "" {
		// Fallback: try to get home directory from os.UserHomeDir
		var err error
		home, err = os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
	}
	return filepath.Clean(filepath.Join(home, ".config", "dotfiles"))
}

// ToolsDir returns the per-tool config directory path
func ToolsDir() string {
	return filepath.Join(ConfigDir(), "tools")
}

// EnsureDirs creates config directories if they don't exist
func EnsureDirs() error {
	if ConfigDir() == "" {
		return ErrNoConfigDir
	}
	dirs := []string{
		ConfigDir(),
		ToolsDir(),
		filepath.Join(ConfigDir(), "users"),
		filepath.Join(ConfigDir(), "backups"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	return nil
}

// LoadToolConfig loads a tool config from JSON file, returning defaults if not found
func LoadToolConfig[T any](toolName string, defaultFn func() *T) (*T, error) {
	if ConfigDir() == "" {
		return nil, ErrNoConfigDir
	}
	path := filepath.Join(ToolsDir(), toolName+".json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Return defaults if file doesn't exist
		return defaultFn(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Start from the intended defaults so keys absent from an older/partial JSON
	// file keep their default value instead of decoding to the Go zero value.
	// json.Unmarshal only overwrites keys that are actually present in the file.
	cfg := defaultFn()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	return cfg, nil
}

// SaveToolConfig saves a tool config to JSON file
func SaveToolConfig[T any](toolName string, cfg *T) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	path := filepath.Join(ToolsDir(), toolName+".json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := writeFileAtomic(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	return nil
}

// LoadGlobalConfig loads global config from settings file
func LoadGlobalConfig() (*GlobalConfig, error) {
	if ConfigDir() == "" {
		return nil, ErrNoConfigDir
	}
	path := filepath.Join(ConfigDir(), "global.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultGlobalConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	return &cfg, nil
}

// SaveGlobalConfig saves global config to settings file
func SaveGlobalConfig(cfg *GlobalConfig) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	path := filepath.Join(ConfigDir(), "global.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal global config: %w", err)
	}

	if err := writeFileAtomic(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write global config: %w", err)
	}

	return nil
}

// AllToolConfigs holds all tool configurations
type AllToolConfigs struct {
	Ghostty   *GhosttyConfig   `json:"ghostty"`
	Tmux      *TmuxConfig      `json:"tmux"`
	Zsh       *ZshConfig       `json:"zsh"`
	Neovim    *NeovimConfig    `json:"neovim"`
	Git       *GitConfig       `json:"git"`
	Yazi      *YaziConfig      `json:"yazi"`
	Fzf       *FzfConfig       `json:"fzf"`
	Apps      *AppsConfig      `json:"apps"`
	Utilities *UtilitiesConfig `json:"utilities"`
}

// LoadAllToolConfigs loads all tool configs with defaults
func LoadAllToolConfigs() (*AllToolConfigs, error) {
	ghostty, err := LoadToolConfig("ghostty", DefaultGhosttyConfig)
	if err != nil {
		return nil, err
	}

	tmux, err := LoadToolConfig("tmux", DefaultTmuxConfig)
	if err != nil {
		return nil, err
	}

	zsh, err := LoadToolConfig("zsh", DefaultZshConfig)
	if err != nil {
		return nil, err
	}

	neovim, err := LoadToolConfig("neovim", DefaultNeovimConfig)
	if err != nil {
		return nil, err
	}

	git, err := LoadToolConfig("git", DefaultGitConfig)
	if err != nil {
		return nil, err
	}

	yazi, err := LoadToolConfig("yazi", DefaultYaziConfig)
	if err != nil {
		return nil, err
	}

	fzf, err := LoadToolConfig("fzf", DefaultFzfConfig)
	if err != nil {
		return nil, err
	}

	apps, err := LoadToolConfig("apps", DefaultAppsConfig)
	if err != nil {
		return nil, err
	}

	utilities, err := LoadToolConfig("utilities", DefaultUtilitiesConfig)
	if err != nil {
		return nil, err
	}

	return &AllToolConfigs{
		Ghostty:   ghostty,
		Tmux:      tmux,
		Zsh:       zsh,
		Neovim:    neovim,
		Git:       git,
		Yazi:      yazi,
		Fzf:       fzf,
		Apps:      apps,
		Utilities: utilities,
	}, nil
}

// SaveAllToolConfigs saves all tool configs
func SaveAllToolConfigs(cfgs *AllToolConfigs) error {
	if err := SaveToolConfig("ghostty", cfgs.Ghostty); err != nil {
		return err
	}
	if err := SaveToolConfig("tmux", cfgs.Tmux); err != nil {
		return err
	}
	if err := SaveToolConfig("zsh", cfgs.Zsh); err != nil {
		return err
	}
	if err := SaveToolConfig("neovim", cfgs.Neovim); err != nil {
		return err
	}
	if err := SaveToolConfig("git", cfgs.Git); err != nil {
		return err
	}
	if err := SaveToolConfig("yazi", cfgs.Yazi); err != nil {
		return err
	}
	if err := SaveToolConfig("fzf", cfgs.Fzf); err != nil {
		return err
	}
	if err := SaveToolConfig("apps", cfgs.Apps); err != nil {
		return err
	}
	if err := SaveToolConfig("utilities", cfgs.Utilities); err != nil {
		return err
	}
	return nil
}

// AvailableThemes returns the list of available themes
var AvailableThemes = []string{
	"catppuccin-mocha",
	"catppuccin-latte",
	"catppuccin-frappe",
	"catppuccin-macchiato",
	"dracula",
	"gruvbox-dark",
	"gruvbox-light",
	"nord",
	"tokyo-night",
	"solarized-dark",
	"solarized-light",
	"monokai",
	"rose-pine",
	"everforest",
	"one-dark",
	"neon-seapunk",
}

// IsValidTheme checks if a theme name is valid
func IsValidTheme(theme string) bool {
	for _, t := range AvailableThemes {
		if t == theme {
			return true
		}
	}
	return false
}
