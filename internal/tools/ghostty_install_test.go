package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGhosttyAppBundleIsDetectedWithoutCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, "Applications", "Ghostty.app"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !ghosttyAppBundleInstalled() {
		t.Fatal("Ghostty.app was not detected in the user Applications directory")
	}
	if !NewGhosttyTool().IsInstalled() {
		t.Fatal("Ghostty tool identity ignored an installed app bundle")
	}
}
