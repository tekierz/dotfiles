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
