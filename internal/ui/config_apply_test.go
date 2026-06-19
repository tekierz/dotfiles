package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	mc.GhosttyFontFamily = "Fira Code"
	mc.GitDefaultBranch = "develop"
	mc.TmuxPrefix = "C-b" // Manage vocabulary; generator expects ctrl-b -> "C-b"

	dd := manageConfigToDeepDive(mc)
	if errs := applyDeepDiveConfig(dd, "catppuccin-mocha"); len(errs) > 0 {
		t.Fatalf("applyDeepDiveConfig returned errors: %v", errs)
	}

	// Ghostty
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config")
	ghostty := readFileOrFail(t, ghosttyPath)
	if !strings.Contains(ghostty, "font-size = 21") {
		t.Errorf("ghostty config missing font-size 21:\n%s", ghostty)
	}
	if !strings.Contains(ghostty, "Fira Code") {
		t.Errorf("ghostty config missing font family Fira Code:\n%s", ghostty)
	}

	// Git
	gitPath := filepath.Join(home, ".gitconfig")
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
