package tools

import (
	"os"
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

func TestSetupNeovimPresetCloneFailureLeavesExistingConfigUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installFakeGit(t, true)

	nvimDir := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(nvimDir, 0755); err != nil {
		t.Fatalf("mkdir nvim: %v", err)
	}
	initVim := filepath.Join(nvimDir, "init.vim")
	original := []byte("\" existing vim config\n")
	if err := os.WriteFile(initVim, original, 0600); err != nil {
		t.Fatalf("write init.vim: %v", err)
	}

	cfg := NeovimConfig{ConfigPreset: "kickstart", TabWidth: 4, LineNumbers: "absolute"}
	if err := setupNeovimPreset(cfg, "catppuccin-mocha", nvimDir); err == nil {
		t.Fatal("setupNeovimPreset succeeded with failing git")
	}

	got, err := os.ReadFile(initVim)
	if err != nil {
		t.Fatalf("existing init.vim missing after failed clone: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("existing init.vim changed after failed clone:\n%s", got)
	}
	if _, err := os.Stat(nvimDir); err != nil {
		t.Fatalf("existing nvim dir missing after failed clone: %v", err)
	}
	backups, err := filepath.Glob(nvimDir + ".backup*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("failed clone created backup(s): %v", backups)
	}
}

func TestSetupNeovimPresetUsesDistinctTimestampedBackups(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installFakeGit(t, false)

	nvimDir := filepath.Join(home, ".config", "nvim")
	cfg := NeovimConfig{ConfigPreset: "kickstart", TabWidth: 4, LineNumbers: "absolute"}

	for i := 0; i < 2; i++ {
		if err := os.RemoveAll(nvimDir); err != nil {
			t.Fatalf("remove nvim: %v", err)
		}
		if err := os.MkdirAll(nvimDir, 0755); err != nil {
			t.Fatalf("mkdir nvim: %v", err)
		}
		content := []byte(strings.Repeat("\" legacy config\n", i+1))
		if err := os.WriteFile(filepath.Join(nvimDir, "init.vim"), content, 0600); err != nil {
			t.Fatalf("write init.vim: %v", err)
		}

		if err := setupNeovimPreset(cfg, "catppuccin-mocha", nvimDir); err != nil {
			t.Fatalf("setupNeovimPreset run %d: %v", i+1, err)
		}
	}

	backups, err := filepath.Glob(nvimDir + ".backup.*")
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("backup count = %d, want 2: %v", len(backups), backups)
	}
	if backups[0] == backups[1] {
		t.Fatalf("backup paths are not distinct: %v", backups)
	}
	for _, backup := range backups {
		if _, err := os.Stat(filepath.Join(backup, "init.vim")); err != nil {
			t.Fatalf("backup %s missing init.vim: %v", backup, err)
		}
	}
}

func installFakeGit(t *testing.T, fail bool) {
	t.Helper()

	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	script := `#!/bin/sh
if [ "$FAKE_GIT_FAIL" = "1" ]; then
	exit 42
fi
dest="$5"
mkdir -p "$dest/.git"
printf '%s\n' '-- cloned preset' > "$dest/init.lua"
`
	if err := os.WriteFile(gitPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if fail {
		t.Setenv("FAKE_GIT_FAIL", "1")
	} else {
		t.Setenv("FAKE_GIT_FAIL", "0")
	}
}
