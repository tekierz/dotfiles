package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNeovimNumbersDrivesBothOpts proves the C2 reconciliation: the single
// LineNumbers control drives BOTH vim.opt.number and vim.opt.relativenumber, with
// no separate relative-number toggle. Each value must produce the exact pair:
//
//	absolute -> number=true,  relativenumber=false
//	relative -> number=true,  relativenumber=true
//	none     -> number=false, relativenumber=false
//
// The assertions are differential across values (changing LineNumbers changes the
// generated output), which is what the Manage round-trip guardrail relies on.
func TestNeovimNumbersDrivesBothOpts(t *testing.T) {
	cases := []struct {
		lineNumbers     string
		wantNumber      string
		wantRelativeNum string
	}{
		{"absolute", "vim.opt.number = true", "vim.opt.relativenumber = false"},
		{"relative", "vim.opt.number = true", "vim.opt.relativenumber = true"},
		{"none", "vim.opt.number = false", "vim.opt.relativenumber = false"},
	}

	for _, tc := range cases {
		t.Run(tc.lineNumbers, func(t *testing.T) {
			out := GenerateNeovimConfig(NeovimConfig{LineNumbers: tc.lineNumbers}, "catppuccin-mocha")
			if !strings.Contains(out, tc.wantNumber) {
				t.Errorf("LineNumbers=%q: missing %q\n%s", tc.lineNumbers, tc.wantNumber, out)
			}
			if !strings.Contains(out, tc.wantRelativeNum) {
				t.Errorf("LineNumbers=%q: missing %q\n%s", tc.lineNumbers, tc.wantRelativeNum, out)
			}
		})
	}
}

// TestSetupNeovimPresetFailedCloneIsNonDestructive guards the data-loss fix in
// setupNeovimPreset: a failed `git clone` must leave the user's existing
// ~/.config/nvim intact. The old code renamed the real config to .backup BEFORE
// cloning, so a clone failure left ~/.config/nvim missing (or a half-cloned
// tree). The fix clones into a temp dir and only swaps into place on success, so
// the original config is never touched when the clone fails.
//
// We force the clone to fail by pointing the preset at an unreachable file:// URL
// (restored via defer), seed ~/.config/nvim with a sentinel file (and no
// init.lua, so the clone path — not the prefs-update path — runs), then assert
// that WriteNeovimConfig returns an error AND the sentinel survives untouched.
func TestSetupNeovimPresetFailedCloneIsNonDestructive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; skipping clone-failure regression test")
	}

	const preset = "kickstart"

	// Force `git clone` to fail with a bogus local URL. file:// guarantees no
	// network access (hermetic) while still being a path git will reject.
	bogusURL := "file://" + filepath.Join(t.TempDir(), "nonexistent-repo.git")
	orig := neovimConfigRepos[preset]
	neovimConfigRepos[preset] = bogusURL
	defer func() { neovimConfigRepos[preset] = orig }()

	// Redirect HOME so the real config is never touched.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Seed a pre-existing ~/.config/nvim WITHOUT init.lua (so setupNeovimPreset
	// takes the clone branch) but WITH a sentinel file representing the user's
	// real config that must survive a failed install.
	nvimDir := filepath.Join(tmpHome, ".config", "nvim")
	if err := os.MkdirAll(nvimDir, 0o755); err != nil {
		t.Fatalf("seed nvimDir: %v", err)
	}
	sentinel := filepath.Join(nvimDir, "my-precious-config.lua")
	const sentinelBody = "-- user config, must not be lost\n"
	if err := os.WriteFile(sentinel, []byte(sentinelBody), 0o600); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	cfg := NeovimConfig{ConfigPreset: preset, TabWidth: 4}
	err := WriteNeovimConfig(cfg, "catppuccin-mocha")

	// (a) A failed clone must surface as an error, not silent success.
	if err == nil {
		t.Fatalf("WriteNeovimConfig(%s) with unreachable repo returned nil; expected clone failure error", preset)
	}

	// (b) The user's config must be fully intact: the sentinel still exists with
	// its original contents at the original path.
	got, readErr := os.ReadFile(sentinel)
	if readErr != nil {
		t.Fatalf("sentinel %s was destroyed by failed clone: %v", sentinel, readErr)
	}
	if string(got) != sentinelBody {
		t.Errorf("sentinel contents changed after failed clone: got %q want %q", got, sentinelBody)
	}

	// nvimDir must remain the user's directory, not a half-cloned tree. The only
	// entry should be the sentinel we wrote — no clone artifacts leaked in.
	entries, readDirErr := os.ReadDir(nvimDir)
	if readDirErr != nil {
		t.Fatalf("read nvimDir after failed clone: %v", readDirErr)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(sentinel) {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("nvimDir is not the original config after failed clone; entries = %v", names)
	}
}
