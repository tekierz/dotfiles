package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestTrackedWholeFileWritersReturnExactPrivateRevision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	tests := []struct {
		name  string
		path  func() (string, error)
		write func(theme string) (MutationEvidence, error)
	}{
		{
			name: "fzf",
			path: func() (string, error) {
				return filepath.Join(home, ".config", "fzf", "fzf.zsh"), nil
			},
			write: func(theme string) (MutationEvidence, error) {
				return WriteFzfConfigTracked(FzfConfig{Layout: "reverse", Height: 40}, theme)
			},
		},
		{
			name: "lazygit",
			path: func() (string, error) {
				return filepath.Join(home, ".config", "lazygit", "config.yml"), nil
			},
			write: func(theme string) (MutationEvidence, error) {
				return WriteLazyGitConfigTracked(LazyGitConfig{SideBySide: true, Theme: "dark", Paging: "never"}, theme)
			},
		},
		{
			name: "glow",
			path: glowConfigPath,
			write: func(theme string) (MutationEvidence, error) {
				return WriteGlowConfigTracked(GlowConfig{Style: "auto", Width: 100, Mouse: true}, theme)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, err := test.path()
			if err != nil {
				t.Fatalf("resolve config path: %v", err)
			}
			first, err := test.write("dracula")
			if err != nil {
				t.Fatalf("first tracked write: %v", err)
			}
			assertExactMutationEvidence(t, path, first)

			second, err := test.write("nord")
			if err != nil {
				t.Fatalf("second tracked write: %v", err)
			}
			assertExactMutationEvidence(t, path, second)
			if first.Revision == second.Revision {
				t.Fatal("tracked revision did not change after content replacement")
			}
		})
	}
}

func assertExactMutationEvidence(t *testing.T, path string, evidence MutationEvidence) {
	t.Helper()
	if evidence.Path != path {
		t.Fatalf("evidence path = %q, want %q", evidence.Path, path)
	}
	if evidence.Directory != nil {
		t.Fatal("whole-file evidence unexpectedly contains a directory snapshot")
	}
	if !evidence.Revision.Tracked() || !evidence.Revision.Exists() {
		t.Fatal("evidence does not identify an existing tracked revision")
	}
	if evidence.Revision.Permissions() != 0o600 {
		t.Fatalf("evidence mode = %04o, want 0600", evidence.Revision.Permissions())
	}

	root, rel, _, err := generatedConfigDestination(path)
	if err != nil {
		t.Fatalf("resolve generated config destination: %v", err)
	}
	_, current, err := safefile.ReadWithin(root, rel)
	if err != nil {
		t.Fatalf("read committed config: %v", err)
	}
	if current != evidence.Revision {
		t.Fatalf("evidence revision does not match committed revision")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat committed config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("on-disk mode = %04o, want 0600", info.Mode().Perm())
	}
}
