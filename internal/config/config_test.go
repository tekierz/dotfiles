package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()

	if cfg.Theme != themeCatppuccinMocha {
		t.Errorf("Theme = %q, want %q", cfg.Theme, themeCatppuccinMocha)
	}
	if cfg.NavStyle != "emacs" {
		t.Errorf("NavStyle = %q, want %q", cfg.NavStyle, "emacs")
	}
	if cfg.ActiveUser != "" {
		t.Errorf("ActiveUser = %q, want empty", cfg.ActiveUser)
	}
	if cfg.DisableAnimations {
		t.Error("DisableAnimations should be false by default")
	}
}

func TestConfigDir(t *testing.T) {
	// Test with XDG_CONFIG_HOME set
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir := ConfigDir()
	expected := "/custom/config/dotfiles"
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}

	// Test without XDG_CONFIG_HOME (uses HOME)
	os.Unsetenv("XDG_CONFIG_HOME")
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/home/testuser")
	defer os.Setenv("HOME", origHome)

	dir = ConfigDir()
	expected = "/home/testuser/.config/dotfiles"
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}
}

func TestConfigDirIgnoresRelativeEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("HOME", home)

	expected := filepath.Join(home, ".config", "dotfiles")
	if dir := ConfigDir(); dir != expected {
		t.Fatalf("ConfigDir() = %q, want %q", dir, expected)
	}

	t.Setenv("HOME", "relative-home")
	if dir := ConfigDir(); dir != "" {
		t.Fatalf("ConfigDir() with relative XDG_CONFIG_HOME and HOME = %q, want empty", dir)
	}
}

func TestSafeRemoveAllUnderAllowsChild(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "backups", "old")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "manifest.tsv"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	if err := SafeRemoveAllUnder(base, target); err != nil {
		t.Fatalf("SafeRemoveAllUnder() returned error: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists after remove, stat err=%v", err)
	}
}

func TestSafeRemoveAllUnderRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "victim")
	if err := os.WriteFile(outsideFile, []byte("do not remove"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	linkPath := filepath.Join(base, "link")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := SafeRemoveAllUnder(base, filepath.Join(linkPath, "victim"))
	if err == nil {
		t.Fatal("SafeRemoveAllUnder() succeeded through symlink escape, want error")
	}
	if _, statErr := os.Stat(outsideFile); statErr != nil {
		t.Fatalf("outside file was affected, stat err=%v", statErr)
	}
}

func TestToolsDir(t *testing.T) {
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir := ToolsDir()
	expected := "/custom/config/dotfiles/tools"
	if dir != expected {
		t.Errorf("ToolsDir() = %q, want %q", dir, expected)
	}
}

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() failed: %v", err)
	}

	// Verify directories exist
	expectedDirs := []string{
		filepath.Join(dir, ".config", "dotfiles"),
		filepath.Join(dir, ".config", "dotfiles", "tools"),
		filepath.Join(dir, ".config", "dotfiles", "users"),
		filepath.Join(dir, ".config", "dotfiles", "backups"),
	}

	for _, d := range expectedDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %q should exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q should be a directory", d)
		}
	}
}

func TestGlobalConfigCRUD(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Load when file doesn't exist - should return defaults
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if cfg.Theme != themeCatppuccinMocha {
		t.Errorf("Theme = %q, want default", cfg.Theme)
	}

	// Save modified config
	cfg.Theme = themeDracula
	cfg.NavStyle = navStyleVim
	cfg.DisableAnimations = true

	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("SaveGlobalConfig failed: %v", err)
	}

	// Reload and verify
	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if loaded.Theme != themeDracula {
		t.Errorf("Theme = %q, want %q", loaded.Theme, themeDracula)
	}
	if loaded.NavStyle != navStyleVim {
		t.Errorf("NavStyle = %q, want %q", loaded.NavStyle, navStyleVim)
	}
	if !loaded.DisableAnimations {
		t.Error("DisableAnimations should be true")
	}
}

func TestIsValidTheme(t *testing.T) {
	tests := []struct {
		theme string
		valid bool
	}{
		{themeCatppuccinMocha, true},
		{themeDracula, true},
		{"nord", true},
		{"neon-seapunk", true},
		{"invalid-theme", false},
		{"", false},
		{"DRACULA", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			got := IsValidTheme(tt.theme)
			if got != tt.valid {
				t.Errorf("IsValidTheme(%q) = %v, want %v", tt.theme, got, tt.valid)
			}
		})
	}
}

func TestAvailableThemes(t *testing.T) {
	// Ensure we have a reasonable number of themes
	if len(AvailableThemes) < 10 {
		t.Errorf("expected at least 10 themes, got %d", len(AvailableThemes))
	}

	// Ensure no duplicates
	seen := make(map[string]bool)
	for _, theme := range AvailableThemes {
		if seen[theme] {
			t.Errorf("duplicate theme: %s", theme)
		}
		seen[theme] = true
	}
}

// TestToolConfig is a simple config for testing.
type TestToolConfig struct {
	Option1 string `json:"option1"`
	Option2 int    `json:"option2"`
}

func DefaultTestToolConfig() *TestToolConfig {
	return &TestToolConfig{
		Option1: "default",
		Option2: 42,
	}
}

func TestToolConfigCRUD(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Load when file doesn't exist - should return defaults
	cfg, err := LoadToolConfig("testtool", DefaultTestToolConfig)
	if err != nil {
		t.Fatalf("LoadToolConfig failed: %v", err)
	}
	if cfg.Option1 != "default" {
		t.Errorf("Option1 = %q, want %q", cfg.Option1, "default")
	}
	if cfg.Option2 != 42 {
		t.Errorf("Option2 = %d, want %d", cfg.Option2, 42)
	}

	// Save modified config
	cfg.Option1 = "modified"
	cfg.Option2 = 100

	if err := SaveToolConfig("testtool", cfg); err != nil {
		t.Fatalf("SaveToolConfig failed: %v", err)
	}

	// Reload and verify
	loaded, err := LoadToolConfig("testtool", DefaultTestToolConfig)
	if err != nil {
		t.Fatalf("LoadToolConfig failed: %v", err)
	}
	if loaded.Option1 != "modified" {
		t.Errorf("Option1 = %q, want %q", loaded.Option1, "modified")
	}
	if loaded.Option2 != 100 {
		t.Errorf("Option2 = %d, want %d", loaded.Option2, 100)
	}
}
