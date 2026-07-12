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

func TestSaveToolConfigAtPathBoundAuthorityTrackedUsesFrozenPathAfterXDGDrift(t *testing.T) {
	home := t.TempDir()
	configA := filepath.Join(home, "config-a")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	pathA := filepath.Join(ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(pathA), 0o700); err != nil {
		t.Fatal(err)
	}
	root, rel, err := anchoredFilePath(pathA)
	if err != nil {
		t.Fatal(err)
	}
	_, accepted, parents, err := safefile.ObserveFileWithin(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	configB := filepath.Join(home, "config-b")
	t.Setenv("XDG_CONFIG_HOME", configB)
	lockedPath := ""
	locker := operation.Locker(func(_ string, target string) (func() error, error) {
		lockedPath = target
		return func() error { return nil }, nil
	})
	committed, err := SaveToolConfigAtPathBoundAuthorityTracked(pathA, &reviewedToolState{Value: "frozen"}, accepted, parents, locker)
	if err != nil || !committed.Tracked() || !committed.Exists() {
		t.Fatalf("frozen tool-state commit=%+v err=%v", committed, err)
	}
	if lockedPath != pathA {
		t.Fatalf("tool-state lock target=%q, want frozen %q", lockedPath, pathA)
	}
	got, err := os.ReadFile(pathA)
	want := []byte("{\n  \"value\": \"frozen\"\n}")
	if err != nil || string(got) != string(want) {
		t.Fatalf("frozen tool-state bytes=%q err=%v want=%q", got, err, want)
	}
	pathB := filepath.Join(configB, "dotfiles", "tools", "manage.json")
	for _, path := range []string{pathB, configB} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("XDG drift created %s: %v", path, statErr)
		}
	}
}

func TestSaveToolConfigAtPathBoundAuthorityTrackedRejectsInvalidPathsBeforeLock(t *testing.T) {
	home := t.TempDir()
	configA := filepath.Join(home, "config-a")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	validPath := filepath.Join(ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(validPath), 0o700); err != nil {
		t.Fatal(err)
	}
	root, rel, err := anchoredFilePath(validPath)
	if err != nil {
		t.Fatal(err)
	}
	_, accepted, parents, err := safefile.ObserveFileWithin(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-manage.json")
	for _, test := range []struct {
		name string
		path string
	}{
		{"relative", filepath.Join("relative", "manage.json")},
		{"outside home and xdg", outside},
	} {
		t.Run(test.name, func(t *testing.T) {
			lockCalled := false
			locker := operation.Locker(func(string, string) (func() error, error) {
				lockCalled = true
				return func() error { return nil }, nil
			})
			if _, err := SaveToolConfigAtPathBoundAuthorityTracked(test.path, &reviewedToolState{Value: "blocked"}, accepted, parents, locker); err == nil {
				t.Fatalf("invalid explicit path %q was accepted", test.path)
			}
			if lockCalled {
				t.Fatalf("invalid explicit path %q acquired a lock", test.path)
			}
			if filepath.IsAbs(test.path) {
				if _, statErr := os.Lstat(test.path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("invalid explicit path %q was mutated: %v", test.path, statErr)
				}
			}
			if _, statErr := os.Lstat(validPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid explicit path attempt mutated valid target: %v", statErr)
			}
		})
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
