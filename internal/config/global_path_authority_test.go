package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestSaveGlobalConfigAtPathBoundAuthorityTrackedUsesFrozenPathAfterXDGDrift(t *testing.T) {
	home := t.TempDir()
	configA := filepath.Join(home, "config-a")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	pathA := filepath.Join(ConfigDir(), "global.json")
	if err := os.MkdirAll(filepath.Dir(pathA), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	accepted, tracked := GlobalConfigRevision(cfg)
	if !tracked || accepted.Exists() {
		t.Fatalf("loaded missing global revision=%+v tracked=%t", accepted, tracked)
	}
	root, rel, err := anchoredFilePath(pathA)
	if err != nil {
		t.Fatal(err)
	}
	_, observed, parents, err := safefile.ObserveFileWithin(root, rel)
	if err != nil || observed != accepted {
		t.Fatalf("observed global revision=%+v accepted=%+v err=%v", observed, accepted, err)
	}
	cfg.Theme = "nord"
	normalized, err := normalizeGlobalConfigForSave(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want, err := marshalGlobalConfig(&normalized)
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
	committed, err := SaveGlobalConfigAtPathBoundAuthorityTracked(pathA, cfg, accepted, parents, locker)
	if err != nil || !committed.Tracked() || !committed.Exists() {
		t.Fatalf("frozen global commit=%+v err=%v", committed, err)
	}
	if lockedPath != pathA {
		t.Fatalf("global lock target=%q, want frozen %q", lockedPath, pathA)
	}
	got, err := os.ReadFile(pathA)
	if err != nil || string(got) != string(want) {
		t.Fatalf("frozen global bytes=%q err=%v want=%q", got, err, want)
	}
	pathB := filepath.Join(configB, "dotfiles", "global.json")
	for _, path := range []string{pathB, configB} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("XDG drift created %s: %v", path, statErr)
		}
	}
}

func TestSaveGlobalConfigAtPathBoundAuthorityTrackedRejectsInvalidPathsBeforeLock(t *testing.T) {
	home := t.TempDir()
	configA := filepath.Join(home, "config-a")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	validPath := filepath.Join(ConfigDir(), "global.json")
	if err := os.MkdirAll(filepath.Dir(validPath), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	accepted, _ := GlobalConfigRevision(cfg)
	root, rel, err := anchoredFilePath(validPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, parents, err := safefile.ObserveFileWithin(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-global.json")
	for _, path := range []string{filepath.Join("relative", "global.json"), outside} {
		lockCalled := false
		locker := operation.Locker(func(string, string) (func() error, error) {
			lockCalled = true
			return func() error { return nil }, nil
		})
		if _, err := SaveGlobalConfigAtPathBoundAuthorityTracked(path, cfg, accepted, parents, locker); err == nil {
			t.Fatalf("invalid explicit global path %q was accepted", path)
		}
		if lockCalled {
			t.Fatalf("invalid explicit global path %q acquired lock", path)
		}
		if filepath.IsAbs(path) {
			if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid explicit global path %q was mutated: %v", path, statErr)
			}
		}
	}
}
