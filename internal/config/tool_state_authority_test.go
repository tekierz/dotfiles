package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

type reviewedToolState struct {
	Value string `json:"value"`
}

func noOpConfigLocker(string, string) (func() error, error) {
	return func() error { return nil }, nil
}

func reviewedToolStateAuthority(t *testing.T) (string, safefile.Revision, *safefile.ParentChain) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(ToolsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ToolsDir(), "manage.json")
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		t.Fatal(err)
	}
	_, revision, parents, err := safefile.ObserveFileWithin(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	return path, revision, parents
}

func TestSaveToolConfigAtBoundAuthorityTrackedModeAndStaleRevision(t *testing.T) {
	path, accepted, parents := reviewedToolStateAuthority(t)
	committed, err := SaveToolConfigAtBoundAuthorityTracked("manage", &reviewedToolState{Value: "first"}, accepted, parents, noOpConfigLocker)
	if err != nil || !committed.Exists() {
		t.Fatalf("save committed=%v err=%v", committed.Exists(), err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v, want 0600", info, err)
	}
	if err := os.WriteFile(path, []byte(`{"value":"external"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveToolConfigAtBoundAuthorityTracked("manage", &reviewedToolState{Value: "stale"}, committed, parents, noOpConfigLocker); !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("stale save error=%v, want ErrRevisionChanged", err)
	}
}

func TestSaveToolConfigAtBoundAuthorityTrackedRejectsParentReplacementAndSymlink(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		name := "replacement"
		if symlink {
			name = "symlink"
		}
		t.Run(name, func(t *testing.T) {
			path, accepted, parents := reviewedToolStateAuthority(t)
			toolsDir := filepath.Dir(path)
			moved := toolsDir + "-original"
			if err := os.Rename(toolsDir, moved); err != nil {
				t.Fatal(err)
			}
			if symlink {
				if err := os.Symlink(t.TempDir(), toolsDir); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			} else if err := os.Mkdir(toolsDir, 0o700); err != nil {
				t.Fatal(err)
			}
			_, err := SaveToolConfigAtBoundAuthorityTracked("manage", &reviewedToolState{Value: "must not write"}, accepted, parents, noOpConfigLocker)
			if !errors.Is(err, safefile.ErrParentChanged) && !errors.Is(err, safefile.ErrSymlink) {
				t.Fatalf("parent replacement error=%v", err)
			}
		})
	}
}

func TestSaveToolConfigAtBoundAuthorityTrackedReportsPostCommitFailure(t *testing.T) {
	path, accepted, parents := reviewedToolStateAuthority(t)
	hookErr := errors.New("injected postcommit validation failure")
	toolConfigAfterWriteHook = func(string) error { return hookErr }
	t.Cleanup(func() { toolConfigAfterWriteHook = nil })
	committed, err := SaveToolConfigAtBoundAuthorityTracked("manage", &reviewedToolState{Value: "committed"}, accepted, parents, operation.Locker(noOpConfigLocker))
	var committedErr interface{ Committed() bool }
	if !errors.Is(err, hookErr) || !errors.As(err, &committedErr) || !committedErr.Committed() || !committed.Exists() {
		t.Fatalf("postcommit revision=%v error=%v", committed.Exists(), err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("committed state missing after postcommit failure: %v", statErr)
	}
}
