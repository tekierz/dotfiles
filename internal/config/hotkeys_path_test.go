package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHotkeysConfigUsesXDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	xdgConfigHome := filepath.Join(tmp, "xdg")
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("HOME", home)

	cfg := &HotkeysConfig{
		Users: map[string]*UserHotkeys{
			"alice": {
				Favorites: map[string][]string{"git": {"status"}},
				Aliases:   map[string]string{"gs": "git status --short"},
			},
		},
	}
	if err := SaveHotkeysConfig(cfg); err != nil {
		t.Fatalf("SaveHotkeysConfig returned error: %v", err)
	}

	expectedPath := filepath.Join(xdgConfigHome, "dotfiles", "hotkeys.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected hotkeys config at %s: %v", expectedPath, err)
	}

	legacyPath := filepath.Join(home, ".config", "dotfiles", "hotkeys.json")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy HOME config path should not exist, stat error = %v", err)
	}

	loaded, err := LoadHotkeysConfig()
	if err != nil {
		t.Fatalf("LoadHotkeysConfig returned error: %v", err)
	}
	got := loaded.GetUserHotkeys("alice").Aliases["gs"]
	if got != "git status --short" {
		t.Fatalf("loaded alias = %q, want %q", got, "git status --short")
	}
}

func TestLoadHotkeysConfigFallsBackToLegacyPath(t *testing.T) {
	tmp := t.TempDir()
	xdgConfigHome := filepath.Join(tmp, "xdg")
	home := filepath.Join(tmp, "home")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("HOME", home)

	// Simulate a config written by an old build to the literal ~/.config path
	// while XDG_CONFIG_HOME points elsewhere.
	legacyDir := filepath.Join(home, ".config", "dotfiles")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	legacy := `{"users":{"alice":{"favorites":{"git":["status"]},"aliases":{"gs":"git status --short"}}}}`
	if err := os.WriteFile(filepath.Join(legacyDir, "hotkeys.json"), []byte(legacy), 0600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	loaded, err := LoadHotkeysConfig()
	if err != nil {
		t.Fatalf("LoadHotkeysConfig returned error: %v", err)
	}
	if got := loaded.GetUserHotkeys("alice").Aliases["gs"]; got != "git status --short" {
		t.Fatalf("legacy alias = %q, want %q", got, "git status --short")
	}

	// A file at the XDG location must win over the legacy one.
	if err := os.MkdirAll(filepath.Join(xdgConfigHome, "dotfiles"), 0700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	current := `{"users":{"alice":{"aliases":{"gs":"git status"}}}}`
	if err := os.WriteFile(filepath.Join(xdgConfigHome, "dotfiles", "hotkeys.json"), []byte(current), 0600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	loaded, err = LoadHotkeysConfig()
	if err != nil {
		t.Fatalf("LoadHotkeysConfig returned error: %v", err)
	}
	if got := loaded.GetUserHotkeys("alice").Aliases["gs"]; got != "git status" {
		t.Fatalf("alias after XDG file exists = %q, want %q", got, "git status")
	}
}
