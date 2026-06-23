package config

import (
	"os"
	"path/filepath"
	"testing"
)

// sampleToolConfig is a small struct used to exercise LoadToolConfig's
// default-then-overlay behavior independently of any production config type.
type sampleToolConfig struct {
	FontSize int    `json:"fontSize"`
	Theme    string `json:"theme"`
	Enabled  bool   `json:"enabled"`
}

func sampleDefaults() *sampleToolConfig {
	return &sampleToolConfig{
		FontSize: 14,
		Theme:    themeCatppuccinMocha,
		Enabled:  true,
	}
}

// TestLoadToolConfigKeepsDefaultsForAbsentKeys verifies that a partial JSON file
// (older config missing newer keys) loads with the missing keys at their
// intended defaults rather than the Go zero value (config-medium). Only keys
// actually present in the file should override the defaults.
func TestLoadToolConfigKeepsDefaultsForAbsentKeys(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	// Write a partial file that only sets FontSize. Theme and Enabled are absent.
	path := filepath.Join(ToolsDir(), "sample.json")
	if err := os.WriteFile(path, []byte(`{"fontSize": 22}`), 0600); err != nil {
		t.Fatalf("write partial config: %v", err)
	}

	cfg, err := LoadToolConfig("sample", sampleDefaults)
	if err != nil {
		t.Fatalf("LoadToolConfig: %v", err)
	}

	// Present key overrides the default.
	if cfg.FontSize != 22 {
		t.Errorf("FontSize = %d, want 22 (present in file)", cfg.FontSize)
	}
	// Absent keys keep their defaults (not "" / false).
	if cfg.Theme != themeCatppuccinMocha {
		t.Errorf("Theme = %q, want default %q for absent key", cfg.Theme, themeCatppuccinMocha)
	}
	if !cfg.Enabled {
		t.Error("Enabled = false, want default true for absent key")
	}
}

// TestLoadToolConfigMissingFileReturnsDefaults guards the existing
// not-found-returns-defaults path still holds after the change.
func TestLoadToolConfigMissingFileReturnsDefaults(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg, err := LoadToolConfig("does-not-exist", sampleDefaults)
	if err != nil {
		t.Fatalf("LoadToolConfig: %v", err)
	}
	if *cfg != *sampleDefaults() {
		t.Errorf("LoadToolConfig returned %+v, want defaults %+v", *cfg, *sampleDefaults())
	}
}
