package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteYaziConfigWritesAllFiles(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := YaziConfig{
		Keymap:      "emacs",
		ShowHidden:  true,
		PreviewMode: "auto",
		SortBy:      "natural",
		LineMode:    "size",
		ScrollOff:   5,
	}
	if err := WriteYaziConfig(cfg, "dracula"); err != nil {
		t.Fatalf("WriteYaziConfig returned error: %v", err)
	}

	configDir := filepath.Join(tmpHome, ".config", "yazi")
	for _, name := range []string{"yazi.toml", "keymap.toml", "theme.toml"} {
		if _, err := os.Stat(filepath.Join(configDir, name)); err != nil {
			t.Fatalf("expected %s to be written: %v", name, err)
		}
	}
}

func TestGenerateYaziTheme(t *testing.T) {
	out := GenerateYaziTheme("dracula")

	assertContainsText(t, out, "#ff79c6")
	// yazi 26.x renamed [manager]->[mgr] and [select]->[pick]; the old names are
	// silently ignored, so the generator (and this test) must use the new schema.
	for _, header := range []string{
		"[mgr]",
		"[tabs]",
		"[status]",
		"[input]",
		"[pick]",
		"[tasks]",
		"[which]",
		"[help]",
		"[filetype]",
	} {
		assertContainsText(t, out, header)
	}
	assertNotContainsText(t, out, "[manager]")
	assertNotContainsText(t, out, "[select]")
}

func TestGenerateYaziThemeUnknownFallsBackToMocha(t *testing.T) {
	out := GenerateYaziTheme("unknown-theme")

	assertContainsText(t, out, "#89b4fa")
	assertNotContainsText(t, out, `fg = ""`)
	assertNotContainsText(t, out, `bg = ""`)
}

func TestGenerateYaziConfigPreviewModeAffectsOutput(t *testing.T) {
	cfg := YaziConfig{
		SortBy:    "natural",
		LineMode:  "size",
		ScrollOff: 5,
	}

	auto := GenerateYaziConfig(cfg, "catppuccin-mocha")
	cfg.PreviewMode = "never"
	never := GenerateYaziConfig(cfg, "catppuccin-mocha")

	if auto == never {
		t.Fatal("PreviewMode=never produced the same yazi.toml as auto")
	}
	assertContainsText(t, auto, "image_delay = 30")
	assertNotContainsText(t, auto, "previewers = []")
	assertContainsText(t, never, "PreviewMode=never")
	assertContainsText(t, never, "[plugin]")
	assertContainsText(t, never, "previewers = []")
}
