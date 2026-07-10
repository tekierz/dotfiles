package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStandaloneConfigScopedToOpenedTool is the data-loss regression guard for
// FIX 1: exiting a `dotfiles config <tool>` session must write ONLY that tool's
// config file and must not clobber any other tool's config file with defaults.
func TestStandaloneConfigScopedToOpenedTool(t *testing.T) {
	// newGoldenContext establishes the temp HOME (it calls withTempHome before
	// NewApp so startup config load is also hermetic); read it back so the seeding
	// and assertions below target the SAME home the apply writes to.
	ctx := newGoldenContext(t)
	home := os.Getenv("HOME")

	// Pre-seed sentinel content in several OTHER tools' config files. A correct
	// scoped apply for `glow` must leave all of these byte-for-byte intact.
	zshrc := filepath.Join(home, ".zshrc")
	gitconfig := filepath.Join(home, ".gitconfig")
	tmuxconf := filepath.Join(home, ".tmux.conf")
	const sentinel = "### USER SENTINEL — do not overwrite ###\n"
	for _, p := range []string{zshrc, gitconfig, tmuxconf} {
		if err := os.WriteFile(p, []byte(sentinel), 0o644); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}

	a := ctx.app
	// Simulate `dotfiles config glow`: standalone mode, started at the glow
	// config screen.
	a.configStandalone = true
	a.startScreen = ScreenConfigGlow
	a.theme = "catppuccin-mocha"

	if errs := a.applyStandaloneConfig(); len(errs) > 0 {
		t.Fatalf("applyStandaloneConfig returned errors: %v", errs)
	}

	// Glow's own config MUST have been written.
	glowPath := filepath.Join(home, filepath.FromSlash(glowTestRelPath()))
	if _, err := os.Stat(glowPath); err != nil {
		t.Errorf("glow config not written by standalone apply: %v", err)
	}

	// Every OTHER tool's config file must be untouched (sentinel intact).
	for _, p := range []string{zshrc, gitconfig, tmuxconf} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if string(data) != sentinel {
			t.Errorf("standalone `config glow` clobbered %s:\n got: %q\nwant: %q", p, string(data), sentinel)
		}
	}
}
