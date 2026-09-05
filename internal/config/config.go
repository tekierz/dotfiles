package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoConfigDir is returned when the config directory cannot be determined
// (e.g. neither XDG_CONFIG_HOME nor HOME is set and os.UserHomeDir fails).
// It guards against silently building relative paths from an empty base.
var ErrNoConfigDir = errors.New("cannot determine config directory: HOME and XDG_CONFIG_HOME are unset")

// writeFileAtomic writes data to path atomically by writing to a temporary
// file in the same directory and renaming it over the destination. This
// prevents a truncated/half-written file if the process is interrupted
// mid-write (rename is atomic on the same filesystem).
func writeFileAtomic(path string, data []byte) error {
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
	// Config files are owner-only (0600); all callers wrote them this way.
	if err := tmp.Chmod(0o600); err != nil {
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

// GlobalConfig holds global dotfiles settings.
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

// DefaultGlobalConfig returns default global settings.
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Theme:            "catppuccin-mocha",
		NavStyle:         "emacs",
		AutoBackup:       true, // Auto-backup enabled by default
		BackupMaxCount:   10,   // Keep last 10 backups
		BackupMaxAgeDays: 30,   // Delete backups older than 30 days
	}
}

// ConfigDir returns the absolute dotfiles config directory path. Relative
// XDG_CONFIG_HOME/HOME values are ignored rather than being used for config
// writes or destructive cleanup paths.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if filepath.IsAbs(xdg) {
			return filepath.Clean(filepath.Join(xdg, "dotfiles"))
		}
	}
	home := os.Getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		// Fallback: try to get home directory from os.UserHomeDir
		var err error
		home, err = os.UserHomeDir()
		if err != nil || home == "" || !filepath.IsAbs(home) {
			return ""
		}
	}
	return filepath.Clean(filepath.Join(home, ".config", "dotfiles"))
}

// SafeRemoveAllUnder removes target only when target resolves under base. It is
// intended for config-owned destructive deletes such as backup cleanup and
// uninstall. Symlinked intermediate directories are resolved through the deepest
// existing ancestor, so paths like <base>/link/child cannot delete outside base.
func SafeRemoveAllUnder(base, target string) error {
	if base == "" || target == "" {
		return ErrNoConfigDir
	}
	if !filepath.IsAbs(base) || !filepath.IsAbs(target) {
		return fmt.Errorf("refusing to remove relative path: base=%q target=%q", base, target)
	}

	cleanBase := filepath.Clean(base)
	cleanTarget := filepath.Clean(target)
	if cleanTarget == string(os.PathSeparator) {
		return fmt.Errorf("refusing to remove filesystem root")
	}

	realBase, err := filepath.EvalSymlinks(cleanBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("resolve base: %w", err)
	}
	realBase = filepath.Clean(realBase)

	resolvedTarget, exists, err := resolveDeepestExisting(cleanTarget)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}
	if exists {
		if !pathWithin(resolvedTarget, realBase) {
			return fmt.Errorf("refusing to remove %s outside %s", cleanTarget, cleanBase)
		}
	} else {
		// No target ancestor exists. This is a no-op for RemoveAll, but still
		// require the lexical target to be under the requested base.
		if !pathWithin(cleanTarget, cleanBase) {
			return fmt.Errorf("refusing to remove %s outside %s", cleanTarget, cleanBase)
		}
	}

	return os.RemoveAll(cleanTarget)
}

func resolveDeepestExisting(path string) (string, bool, error) {
	probe := filepath.Clean(path)
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			return filepath.Clean(resolved), true, nil
		}
		if !os.IsNotExist(err) {
			return "", false, err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", false, nil
		}
		probe = parent
	}
}

func pathWithin(path, base string) bool {
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(base)
	return cleanPath == cleanBase || strings.HasPrefix(cleanPath, cleanBase+string(os.PathSeparator))
}

// ToolsDir returns the per-tool config directory path.
func ToolsDir() string {
	return filepath.Join(ConfigDir(), "tools")
}

// EnsureDirs creates config directories if they don't exist.
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

// LoadToolConfig loads a tool config from JSON file, returning defaults if not found.
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

// SaveToolConfig saves a tool config to JSON file.
func SaveToolConfig[T any](toolName string, cfg *T) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	path := filepath.Join(ToolsDir(), toolName+".json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	return nil
}

// LoadGlobalConfig loads global config from settings file.
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

// SaveGlobalConfig saves global config to settings file.
func SaveGlobalConfig(cfg *GlobalConfig) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	path := filepath.Join(ConfigDir(), "global.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal global config: %w", err)
	}

	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("failed to write global config: %w", err)
	}

	return nil
}

// AvailableThemes returns the list of available themes.
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

// IsValidTheme checks if a theme name is valid.
func IsValidTheme(theme string) bool {
	for _, t := range AvailableThemes {
		if t == theme {
			return true
		}
	}
	return false
}
