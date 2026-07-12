package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBtopConfigRejectsInjectedOrUnsupportedValuesBeforeMutation(t *testing.T) {
	for name, mutate := range map[string]func(*BtopConfig){
		"shown box injection": func(cfg *BtopConfig) { cfg.ShownBoxes = "cpu\nupdate_check = true" },
		"duplicate box":       func(cfg *BtopConfig) { cfg.ShownBoxes = "cpu cpu" },
		"temperature":         func(cfg *BtopConfig) { cfg.TempScale = `celsius"\nvim_keys = false` },
		"graph":               func(cfg *BtopConfig) { cfg.GraphType = "unknown" },
		"theme traversal":     func(cfg *BtopConfig) { cfg.Theme = "../../victim" },
		"update interval":     func(cfg *BtopConfig) { cfg.UpdateMs = 10 },
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			cfg := BtopConfig{Theme: "auto", UpdateMs: 2000, GraphType: "braille", TempScale: "celsius", ShownBoxes: "cpu mem net proc"}
			mutate(&cfg)
			if err := WriteBtopConfig(cfg, "nord"); err == nil {
				t.Fatal("invalid btop config was accepted")
			}
			if _, err := os.Lstat(filepath.Join(home, ".config", "btop")); !os.IsNotExist(err) {
				t.Fatalf("invalid btop config mutated filesystem: %v", err)
			}
		})
	}
}

func TestWriteBtopConfigCreatesReferencedOverrideTheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := BtopConfig{Theme: "gruvbox", UpdateMs: 2000, GraphType: "braille", TempScale: "celsius", ShownBoxes: "cpu mem"}
	if err := WriteBtopConfig(cfg, "nord"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "btop", "themes", "gruvbox.theme")); err != nil {
		t.Fatalf("referenced btop override theme was not created: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(home, ".config", "btop", "btop.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(string(content), `color_theme = "gruvbox"`) {
		t.Fatalf("btop config does not reference created override:\n%s", content)
	}
}

func TestWriteGlowConfigRejectsInjectedOrUnsupportedValuesBeforeMutation(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg   GlowConfig
		theme string
	}{
		"style injection": {GlowConfig{Style: "dark\nlocal: false", Pager: "auto", Width: 100}, "nord"},
		"unknown pager":   {GlowConfig{Style: "dark", Pager: "shell", Width: 100}, "nord"},
		"negative width":  {GlowConfig{Style: "dark", Pager: "auto", Width: -1}, "nord"},
		"theme injection": {GlowConfig{Style: "dark", Pager: "auto", Width: 100}, "nord\nlocal: false"},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			if err := WriteGlowConfig(tc.cfg, tc.theme); err == nil {
				t.Fatal("invalid Glow config was accepted")
			}
			for _, path := range NewGlowTool().ConfigPaths() {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("invalid Glow config mutated %s: %v", path, err)
				}
			}
		})
	}
}

func TestGenerateGlowConfigTreatsNoneAsPagerDisabled(t *testing.T) {
	if got := GenerateGlowConfig(GlowConfig{Style: "auto", Pager: "none", Width: 100}, "nord"); !containsLine(got, "pager: false") {
		t.Fatalf("Glow none pager remained enabled:\n%s", got)
	}
}

func containsLine(content, line string) bool {
	for _, candidate := range strings.Split(content, "\n") {
		if candidate == line {
			return true
		}
	}
	return false
}
