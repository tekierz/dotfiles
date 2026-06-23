package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/hotkeys"
)

// neovimNavigateStableID returns the stable item ID for neovim "Navigate", used
// to seed an already-migrated favorites file.
func neovimNavigateStableID(t *testing.T) string {
	t.Helper()
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					return it.ID
				}
			}
		}
	}
	t.Fatal("could not determine stable ID for neovim/Navigate")
	return ""
}

// TestNewApp_NoOpMigrationDoesNotRewriteHotkeys proves the C3 fix: constructing
// the App when the hotkeys config is ALREADY migrated must not re-persist it, so
// no disk write happens on every launch. We seed an already-migrated file, record
// its bytes + mtime, build the App, and assert both are unchanged.
func TestNewApp_NoOpMigrationDoesNotRewriteHotkeys(t *testing.T) {
	withTempHome(t)

	id := neovimNavigateStableID(t)

	// Seed an already-migrated hotkeys.json (favorites keyed by stable ID).
	dir := filepath.Join(os.Getenv("HOME"), ".config", "dotfiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := filepath.Join(dir, "hotkeys.json")
	cfg := &config.HotkeysConfig{
		Users: map[string]*config.UserHotkeys{
			"default": {Favorites: map[string][]string{"neovim": {id}}},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed before: %v", err)
	}
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat seed before: %v", err)
	}

	// Constructing the App runs MigrateLegacyFavorites; with an already-migrated
	// config it must NOT call SaveHotkeysConfig.
	_ = NewApp(true)

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed after: %v", err)
	}
	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat seed after: %v", err)
	}

	if string(before) != string(after) {
		t.Errorf("no-op migration rewrote hotkeys.json content\nbefore: %s\nafter:  %s", before, after)
	}
	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Errorf("no-op migration touched hotkeys.json mtime: %v -> %v", infoBefore.ModTime(), infoAfter.ModTime())
	}
}

// TestNewApp_LegacyMigrationRewritesHotkeys is the positive counterpart: a
// legacy-keyed favorites file IS migrated and persisted on construction, so the
// one-time migration still happens.
func TestNewApp_LegacyMigrationRewritesHotkeys(t *testing.T) {
	withTempHome(t)

	id := neovimNavigateStableID(t)

	dir := filepath.Join(os.Getenv("HOME"), ".config", "dotfiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := filepath.Join(dir, "hotkeys.json")
	cfg := &config.HotkeysConfig{
		Users: map[string]*config.UserHotkeys{
			"default": {Favorites: map[string][]string{"neovim": {"Arrow keys"}}}, // legacy key
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	_ = NewApp(true)

	// Reload and confirm the favorite was rewritten to the stable ID.
	out, err := config.LoadHotkeysConfig()
	if err != nil {
		t.Fatalf("reload hotkeys: %v", err)
	}
	favs := out.Users["default"].Favorites["neovim"]
	if len(favs) != 1 || favs[0] != id {
		t.Errorf("legacy migration not persisted: got %v, want [%q]", favs, id)
	}
}
