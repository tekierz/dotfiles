package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempHome points HOME at a fresh temp dir and clears XDG config/state
// overrides so configuration and private operation-state fixtures cannot read
// or write the host environment. It restores the previous environment on
// cleanup.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	return dir
}

// TestApplyManageConfigWritesGeneratedFiles is the key round-trip test: a value
// set in the Manage UI's ManageConfig must reach the generated tool config file
// on disk via the shared apply path.
func TestApplyManageConfigWritesGeneratedFiles(t *testing.T) {
	home := withTempHome(t)

	mc := NewManageConfig()
	// Change values across several tools.
	mc.GhosttyFontSize = 21
	mc.GhosttyFontFamily = "Fira Code"
	mc.GitDefaultBranch = "develop"
	mc.TmuxPrefix = "C-b" // Manage vocabulary; generator expects ctrl-b -> "C-b"

	dd := manageConfigToDeepDive(mc)
	// Write each tool via the scoped config-apply path (the same path the Manage
	// save and standalone `dotfiles config <tool>` use). This proves a Manage value
	// reaches the generated file on disk.
	for _, id := range []string{"ghostty", "git", "tmux"} {
		if errs := applyOneToolConfig(id, dd, "catppuccin-mocha"); len(errs) > 0 {
			t.Fatalf("applyOneToolConfig(%s) returned errors: %v", id, errs)
		}
	}

	// Ghostty
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	ghostty := readFileOrFail(t, ghosttyPath)
	if !strings.Contains(ghostty, "font-size = 21") {
		t.Errorf("ghostty config missing font-size 21:\n%s", ghostty)
	}
	if !strings.Contains(ghostty, "Fira Code") {
		t.Errorf("ghostty config missing font family Fira Code:\n%s", ghostty)
	}

	// Git
	gitPath := filepath.Join(home, ".config", "dotfiles", "git", "config")
	git := readFileOrFail(t, gitPath)
	if !strings.Contains(git, "defaultBranch = develop") && !strings.Contains(git, "develop") {
		t.Errorf("gitconfig missing develop default branch:\n%s", git)
	}

	// Tmux prefix: Manage "C-b" must translate to a generator value that yields
	// "C-b" in the output (regression guard for the vocabulary mismatch).
	tmuxPath := filepath.Join(home, ".tmux.conf")
	tmux := readFileOrFail(t, tmuxPath)
	if !strings.Contains(tmux, "C-b") {
		t.Errorf("tmux.conf missing prefix C-b:\n%s", tmux)
	}
}

func readFileOrFail(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
