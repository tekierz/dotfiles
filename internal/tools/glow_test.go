package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlowConfigPathUsesUserConfigDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir returned error: %v", err)
	}
	expectedPath := filepath.Join(userConfigDir, "glow", "glow.yml")

	cfg := GlowConfig{Pager: "auto", Style: "dark", Mouse: true}
	if err := WriteGlowConfig(cfg, "catppuccin-mocha"); err != nil {
		t.Fatalf("WriteGlowConfig returned error: %v", err)
	}
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected glow config at %s: %v", expectedPath, err)
	}

	paths := NewGlowTool().ConfigPaths()
	if len(paths) != 1 || paths[0] != expectedPath {
		t.Fatalf("NewGlowTool configPaths = %v, want [%s]", paths, expectedPath)
	}
}

func TestGenerateGlowConfigPager(t *testing.T) {
	never := GenerateGlowConfig(GlowConfig{Pager: "never"}, "catppuccin-mocha")
	if !strings.Contains(never, "pager: false\n") {
		t.Fatalf("Pager=never output missing pager false:\n%s", never)
	}

	auto := GenerateGlowConfig(GlowConfig{Pager: "auto"}, "catppuccin-mocha")
	if !strings.Contains(auto, "pager: true\n") {
		t.Fatalf("Pager=auto output missing pager true:\n%s", auto)
	}
}
