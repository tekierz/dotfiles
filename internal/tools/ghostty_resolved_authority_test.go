package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestWriteGhosttyConfigAtResolvedAuthorityTrackedUsesFrozenPathAfterXDGDrift(t *testing.T) {
	home := t.TempDir()
	configA := filepath.Join(home, "config-a")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	pathA, err := GhosttyConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pathA), 0o700); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(home, pathA)
	if err != nil {
		t.Fatal(err)
	}
	_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
	if err != nil {
		t.Fatal(err)
	}
	configB := filepath.Join(home, "config-b")
	t.Setenv("XDG_CONFIG_HOME", configB)
	cfg := GhosttyConfig{FontFamily: "JetBrains Mono", FontSize: 15, Opacity: 91, BlurRadius: 12, CursorStyle: "bar", ScrollbackLines: 123456, WindowDecorations: true, ConfirmClose: true}
	if _, err := WriteGhosttyConfigAtBoundAuthorityTracked(pathA, cfg, "dracula", accepted, parents, operation.DefaultLocker); err == nil || !strings.Contains(err.Error(), "unsupported Ghostty config target") {
		t.Fatalf("discovery-bound writer stale-path error=%v, want unsupported target refusal", err)
	}
	if _, statErr := os.Lstat(pathA); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("discovery-bound stale-path refusal mutated A: %v", statErr)
	}
	lockedPath := ""
	locker := operation.Locker(func(_ string, target string) (func() error, error) {
		lockedPath = target
		return func() error { return nil }, nil
	})
	evidence, err := WriteGhosttyConfigAtResolvedAuthorityTracked(pathA, cfg, "dracula", accepted, parents, locker)
	if err != nil {
		t.Fatal(err)
	}
	assertExactMutationEvidence(t, pathA, evidence)
	if evidence.Parents != parents || lockedPath != pathA {
		t.Fatalf("frozen Ghostty evidence parents=%p want=%p lock=%q want=%q", evidence.Parents, parents, lockedPath, pathA)
	}
	got, err := os.ReadFile(pathA)
	want := wrapManagedConfigSection(ghosttyManagedStart, ghosttyManagedEnd, GenerateGhosttyConfig(cfg, "dracula"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("frozen Ghostty bytes=%q err=%v want=%q", got, err, want)
	}
	for _, path := range append([]string{configB}, ghosttyConfigCandidates(home)...) {
		for _, candidate := range []string{path, filepath.Dir(path)} {
			if candidate == home {
				continue
			}
			if _, statErr := os.Lstat(candidate); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("XDG drift created Ghostty path %s: %v", candidate, statErr)
			}
		}
	}
}

func TestWriteGhosttyConfigAtResolvedAuthorityTrackedRejectsInvalidPathsBeforeLock(t *testing.T) {
	home := t.TempDir()
	configA := filepath.Join(home, "config-a")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configA)
	validPath, err := GhosttyConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(validPath), 0o700); err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(home, validPath)
	_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "config.ghostty")
	for _, path := range []string{
		filepath.Join("relative", "config.ghostty"),
		outside,
		filepath.Join(filepath.Dir(validPath), "bad\nconfig.ghostty"),
		filepath.Join(filepath.Dir(validPath), "bad\u202econfig.ghostty"),
	} {
		lockCalled := false
		locker := operation.Locker(func(string, string) (func() error, error) {
			lockCalled = true
			return func() error { return nil }, nil
		})
		if _, err := WriteGhosttyConfigAtResolvedAuthorityTracked(path, GhosttyConfig{}, "dracula", accepted, parents, locker); err == nil {
			t.Fatalf("invalid frozen Ghostty path %q was accepted", path)
		}
		if lockCalled {
			t.Fatalf("invalid frozen Ghostty path %q acquired lock", path)
		}
		if filepath.IsAbs(path) {
			if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid frozen Ghostty path %q was mutated: %v", path, statErr)
			}
		}
		if _, statErr := os.Lstat(validPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("invalid frozen Ghostty attempt mutated valid target: %v", statErr)
		}
	}
}

func TestWriteGhosttyConfigAtResolvedAuthorityTrackedRejectsUnsafeHomeBeforeLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	validPath, err := GhosttyConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(validPath), 0o700); err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(home, validPath)
	_, accepted, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
	if err != nil {
		t.Fatal(err)
	}
	for _, unsafeHome := range []string{"relative-home", filepath.Join(home, "unsafe\nhome")} {
		t.Setenv("HOME", unsafeHome)
		lockCalled := false
		locker := operation.Locker(func(string, string) (func() error, error) {
			lockCalled = true
			return func() error { return nil }, nil
		})
		if _, err := WriteGhosttyConfigAtResolvedAuthorityTracked(validPath, GhosttyConfig{}, "dracula", accepted, parents, locker); err == nil {
			t.Fatalf("unsafe HOME %q was accepted", unsafeHome)
		}
		if lockCalled {
			t.Fatalf("unsafe HOME %q acquired lock", unsafeHome)
		}
		if _, statErr := os.Lstat(validPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("unsafe HOME attempt mutated valid target: %v", statErr)
		}
	}
}
