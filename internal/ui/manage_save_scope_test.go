package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
)

// TestManageSaveScopedToChangedTool is the data-loss regression guard for P1-A2:
// saving the Manage editor after changing ONE tool's setting must rewrite ONLY
// that tool's config file and leave every other tool's config file byte-for-byte
// intact (mirrors the standalone scoping test but via the Manage save path).
func TestManageSaveScopedToChangedTool(t *testing.T) {
	home := withTempHome(t)

	// Pre-seed sentinel content in OTHER tools' config files. A correct scoped
	// Manage save of a ghostty-only change must leave all of these untouched.
	zshrc := filepath.Join(home, ".zshrc")
	gitconfig := filepath.Join(home, ".gitconfig")
	tmuxconf := filepath.Join(home, ".tmux.conf")
	const sentinel = "### USER SENTINEL — do not overwrite ###\n"
	for _, p := range []string{zshrc, gitconfig, tmuxconf} {
		if err := os.WriteFile(p, []byte(sentinel), 0o644); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}

	// Baseline (what was loaded) vs current (after a single ghostty edit).
	baseline := NewManageConfig()
	current := NewManageConfig()
	current.GhosttyFontSize = baseline.GhosttyFontSize + 3

	changed := changedManageTools(baseline, current, "catppuccin-mocha", "catppuccin-mocha")
	if len(changed) != 1 || changed[0] != "ghostty" {
		t.Fatalf("changedManageTools = %v, want [ghostty]", changed)
	}

	if errs := applyChangedManageTools(changed, manageConfigToDeepDive(current), "catppuccin-mocha"); len(errs) > 0 {
		t.Fatalf("applyChangedManageTools returned errors: %v", errs)
	}

	// Ghostty's own config MUST have been written.
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	if _, err := os.Stat(ghosttyPath); err != nil {
		t.Errorf("ghostty config not written by scoped Manage save: %v", err)
	}

	// Every OTHER tool's config file must be untouched (sentinel intact).
	for _, p := range []string{zshrc, gitconfig, tmuxconf} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if string(data) != sentinel {
			t.Errorf("scoped Manage save clobbered %s:\n got: %q\nwant: %q", p, string(data), sentinel)
		}
	}
}

func TestManageSaveDoesNotPersistPreferenceWhenNativeApplyFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "relative/unsafe")
	app := NewApp(true)
	app.manageConfig.GitDefaultBranch = "develop"

	msg := app.saveManageConfigCmd()()
	saved, ok := msg.(manageSavedMsg)
	if !ok || saved.err == nil {
		t.Fatalf("save result = %#v, want native apply error", msg)
	}
	if _, err := os.Lstat(filepath.Join(config.ToolsDir(), "manage.json")); !os.IsNotExist(err) {
		t.Fatalf("failed native apply persisted manage preference: %v", err)
	}
}

// TestManageSaveNoChangeWritesNothing verifies that saving with no tool-field
// change (and no theme change) writes no tool config files at all.
func TestManageSaveNoChangeWritesNothing(t *testing.T) {
	baseline := NewManageConfig()
	current := NewManageConfig()

	changed := changedManageTools(baseline, current, "catppuccin-mocha", "catppuccin-mocha")
	if len(changed) != 0 {
		t.Fatalf("changedManageTools with no change = %v, want []", changed)
	}
}

// TestManageSaveThemeChangeReappliesAll verifies the documented cross-cutting
// rule: a theme change re-applies every tool (generated colors depend on theme),
// even if no per-tool field changed.
func TestManageSaveThemeChangeReappliesAll(t *testing.T) {
	baseline := NewManageConfig()
	current := NewManageConfig()

	changed := changedManageTools(baseline, current, "catppuccin-mocha", "nord")
	// All generator tools (and claude-code) should be in the set on a theme change.
	if len(changed) < len(manageGeneratorToolOrder) {
		t.Fatalf("theme change should re-apply all tools; got %v", changed)
	}
}

// TestManageSaveTmuxNeovimNoClone proves config-apply for tmux/neovim does NOT
// clone (no network / no destructive move). After P1-A, changing the tmux prefix
// writes ~/.tmux.conf via WriteTmuxConfig (no TPM install) and the neovim path
// writes options.lua only when ~/.config/nvim already exists (never clones).
func TestManageSaveTmuxNeovimNoClone(t *testing.T) {
	home := withTempHome(t)

	baseline := NewManageConfig()
	current := NewManageConfig()
	current.TmuxPrefix = "C-b"
	current.NeovimTabWidth = baseline.NeovimTabWidth + 1

	changed := changedManageTools(baseline, current, "catppuccin-mocha", "catppuccin-mocha")
	if errs := applyChangedManageTools(changed, manageConfigToDeepDive(current), "catppuccin-mocha"); len(errs) > 0 {
		t.Fatalf("applyChangedManageTools returned errors (cloned?): %v", errs)
	}

	// tmux.conf written with the new prefix, no TPM clone required.
	tmuxconf := readFileOrFail(t, filepath.Join(home, ".tmux.conf"))
	if want := "C-b"; !strings.Contains(tmuxconf, want) {
		t.Errorf("tmux.conf missing prefix %q after scoped save:\n%s", want, tmuxconf)
	}
	// TPM must NOT have been cloned by a config write.
	tpmDir := filepath.Join(home, ".tmux", "plugins", "tpm")
	if _, err := os.Stat(tpmDir); err == nil {
		t.Errorf("config-apply cloned TPM into %s (must clone at install only)", tpmDir)
	}

	// neovim: no ~/.config/nvim was created (not installed) -> no clone, no-op.
	nvimDir := filepath.Join(home, ".config", "nvim")
	if _, err := os.Stat(nvimDir); err == nil {
		t.Errorf("config-apply created %s (must not clone preset at config time)", nvimDir)
	}
}

// TestNeovimUserPrefsWritesWhenInstalled verifies the pure neovim writer DOES
// overlay options.lua when ~/.config/nvim already exists (simulating an installed
// preset), without cloning or moving anything.
func TestNeovimUserPrefsWritesWhenInstalled(t *testing.T) {
	home := withTempHome(t)

	nvimDir := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(nvimDir, 0o755); err != nil {
		t.Fatalf("mkdir nvim: %v", err)
	}
	// Pre-seed an init.lua so the writer appends its require without cloning.
	initPath := filepath.Join(nvimDir, "init.lua")
	const initSentinel = "-- existing user init\n"
	if err := os.WriteFile(initPath, []byte(initSentinel), 0o644); err != nil {
		t.Fatalf("seed init.lua: %v", err)
	}

	baseline := NewManageConfig()
	current := NewManageConfig()
	current.NeovimTabWidth = baseline.NeovimTabWidth + 1

	changed := changedManageTools(baseline, current, "catppuccin-mocha", "catppuccin-mocha")
	if errs := applyChangedManageTools(changed, manageConfigToDeepDive(current), "catppuccin-mocha"); len(errs) > 0 {
		t.Fatalf("applyChangedManageTools returned errors: %v", errs)
	}

	optionsPath := filepath.Join(nvimDir, "lua", "custom", "options.lua")
	if _, err := os.Stat(optionsPath); err != nil {
		t.Errorf("neovim options.lua not written when nvim installed: %v", err)
	}
	// init.lua must still contain the user's original content (appended, not clobbered).
	got := readFileOrFail(t, initPath)
	if !strings.Contains(got, "existing user init") {
		t.Errorf("neovim writer clobbered existing init.lua:\n%s", got)
	}
}
