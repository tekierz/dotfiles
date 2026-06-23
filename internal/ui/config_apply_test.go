package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sample config values reused across the config-apply round-trip tests.
const (
	testFontFamily = "Fira Code"
	testGitBranch  = "develop"
	testTmuxPrefix = "C-b"
)

// withTempHome points HOME at a fresh temp dir (and clears XDG_CONFIG_HOME) so
// both config.ConfigDir() and the tools.Write*Config generators (which use
// os.UserHomeDir / $HOME) land their output inside the temp dir. It restores
// the previous environment on cleanup.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	origHome, hadHome := os.LookupEnv("HOME")
	origXDG, hadXDG := os.LookupEnv("XDG_CONFIG_HOME")

	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	_ = os.Unsetenv("XDG_CONFIG_HOME")

	t.Cleanup(func() {
		if hadHome {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
		if hadXDG {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	})

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
	mc.GhosttyFontFamily = testFontFamily
	mc.GitDefaultBranch = testGitBranch
	mc.TmuxPrefix = testTmuxPrefix // Manage vocabulary; generator expects ctrl-b -> "C-b"

	dd := manageConfigToDeepDive(mc)
	// Write each tool via the scoped config-apply path (the same path the Manage
	// save and standalone `dotfiles config <tool>` use). This proves a Manage value
	// reaches the generated file on disk.
	for _, id := range []string{toolGhostty, "git", "tmux"} {
		if errs := applyOneToolConfig(id, dd, defaultTheme); len(errs) > 0 {
			t.Fatalf("applyOneToolConfig(%s) returned errors: %v", id, errs)
		}
	}

	// Ghostty
	ghosttyPath := filepath.Join(home, ".config", toolGhostty, "config")
	ghostty := readFileOrFail(t, ghosttyPath)
	if !strings.Contains(ghostty, "font-size = 21") {
		t.Errorf("ghostty config missing font-size 21:\n%s", ghostty)
	}
	if !strings.Contains(ghostty, testFontFamily) {
		t.Errorf("ghostty config missing font family Fira Code:\n%s", ghostty)
	}

	// Git
	gitPath := filepath.Join(home, ".gitconfig")
	git := readFileOrFail(t, gitPath)
	if !strings.Contains(git, "defaultBranch = "+testGitBranch) && !strings.Contains(git, testGitBranch) {
		t.Errorf("gitconfig missing develop default branch:\n%s", git)
	}

	// Tmux prefix: Manage "C-b" must translate to a generator value that yields
	// "C-b" in the output (regression guard for the vocabulary mismatch).
	tmuxPath := filepath.Join(home, ".tmux.conf")
	tmux := readFileOrFail(t, tmuxPath)
	if !strings.Contains(tmux, testTmuxPrefix) {
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
