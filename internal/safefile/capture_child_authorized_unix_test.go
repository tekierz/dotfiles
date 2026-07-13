//go:build darwin || linux

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCaptureChildDirectoryWithinAuthorizedPresentAndMissing(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		root := t.TempDir()
		state := filepath.Join(root, "state")
		operations := filepath.Join(state, "operations")
		mustMkdir(t, state, 0o700)
		mustMkdir(t, operations, 0o700)
		stateSnapshot, stateParents, err := CaptureDirectoryRootWithin(root, "state")
		if err != nil {
			t.Fatal(err)
		}

		child, childParents, err := CaptureChildDirectoryWithinAuthorized(root, "state", "operations", stateParents, stateSnapshot)
		if err != nil || child == nil || child.Permissions() != 0o700 || !childParents.Tracked() {
			t.Fatalf("present child = %+v %+v %v", child, childParents, err)
		}
		wantParents, err := CaptureParentChainWithin(root, "state/operations")
		if err != nil || !SameParentChain(childParents, wantParents) {
			t.Fatalf("present child parents mismatch: %v", err)
		}
		opened, err := OpenDirectoryWithinAuthorized(root, "state/operations", childParents, child)
		if err != nil {
			t.Fatal(err)
		}
		if err := opened.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		root := t.TempDir()
		state := filepath.Join(root, "state")
		mustMkdir(t, state, 0o700)
		stateSnapshot, stateParents, err := CaptureDirectoryRootWithin(root, "state")
		if err != nil {
			t.Fatal(err)
		}

		child, childParents, err := CaptureChildDirectoryWithinAuthorized(root, "state", "operations", stateParents, stateSnapshot)
		if !errors.Is(err, os.ErrNotExist) || child != nil || !childParents.Tracked() {
			t.Fatalf("missing child = %+v %+v %v", child, childParents, err)
		}
		if _, statErr := os.Lstat(filepath.Join(state, "operations")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("missing capture created child: %v", statErr)
		}
	})
}

func TestCaptureChildDirectoryWithinAuthorizedUsesAcceptedDirectoryAcrossSwapMissingRestore(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	acceptedState := filepath.Join(root, "accepted-state")
	operations := filepath.Join(state, "operations")
	mustMkdir(t, state, 0o700)
	mustMkdir(t, operations, 0o700)
	stateSnapshot, stateParents, err := CaptureDirectoryRootWithin(root, "state")
	if err != nil {
		t.Fatal(err)
	}
	openCalls := 0
	observeCalls := 0
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterAuthorizedDirectoryOpen = func(_ int, child string) error {
			openCalls++
			if child != "operations" {
				t.Fatalf("opened child = %q", child)
			}
			if err := os.Rename(state, acceptedState); err != nil {
				return err
			}
			return os.Mkdir(state, 0o700)
		}
		hooks.afterAuthorizedChildObserve = func(_ int, _ int, child string, exists bool) error {
			observeCalls++
			if child != "operations" || !exists {
				t.Fatalf("held accepted observation = child:%q exists:%t", child, exists)
			}
			if err := os.Remove(state); err != nil {
				return err
			}
			return os.Rename(acceptedState, state)
		}
	})

	child, childParents, err := CaptureChildDirectoryWithinAuthorized(root, "state", "operations", stateParents, stateSnapshot)
	if err != nil || child == nil || !childParents.Tracked() || openCalls != 1 || observeCalls != 1 {
		t.Fatalf("ABA child = %+v %+v open:%d observe:%d %v", child, childParents, openCalls, observeCalls, err)
	}
	if info, err := os.Lstat(filepath.Join(state, "operations")); err != nil || !info.IsDir() {
		t.Fatalf("accepted child not restored: %v %v", info, err)
	}
}

func TestCaptureChildDirectoryWithinAuthorizedRejectsSymlinkNonDirectoryAndInvalidAuthority(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
		want  error
	}{
		{name: "symlink", setup: func(t *testing.T, state string) {
			if err := os.Symlink("target", filepath.Join(state, "operations")); err != nil {
				t.Fatal(err)
			}
		}, want: ErrSymlink},
		{name: "regular file", setup: func(t *testing.T, state string) {
			mustWrite(t, filepath.Join(state, "operations"), "value", 0o600)
		}, want: ErrNonRegular},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			state := filepath.Join(root, "state")
			mustMkdir(t, state, 0o700)
			stateSnapshot, stateParents, err := CaptureDirectoryRootWithin(root, "state")
			if err != nil {
				t.Fatal(err)
			}
			test.setup(t, state)
			if _, _, err := CaptureChildDirectoryWithinAuthorized(root, "state", "operations", stateParents, stateSnapshot); !errors.Is(err, test.want) {
				t.Fatalf("child type error = %v, want %v", err, test.want)
			}
		})
	}

	root := t.TempDir()
	state := filepath.Join(root, "state")
	mustMkdir(t, state, 0o700)
	stateSnapshot, stateParents, err := CaptureDirectoryRootWithin(root, "state")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CaptureChildDirectoryWithinAuthorized(root, "state", "../operations", stateParents, stateSnapshot); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("invalid child error = %v", err)
	}
	if _, _, err := CaptureChildDirectoryWithinAuthorized(root, "state", "operations", &ParentChain{}, stateSnapshot); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("invalid parent authority error = %v", err)
	}
	if _, _, err := CaptureChildDirectoryWithinAuthorized(root, "state", "operations", stateParents, &DirectorySnapshot{}); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("invalid directory authority error = %v", err)
	}
}

func TestCaptureChildDirectoryWithinAuthorizedRejectsSpecialModeOnAcceptedDirectory(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	mustMkdir(t, state, 0o700)
	mustMkdir(t, filepath.Join(state, "operations"), 0o700)
	stateSnapshot, stateParents, err := CaptureDirectoryRootWithin(root, "state")
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Chmod(state, 0o1700); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(state); err != nil || info.Mode()&os.ModeSticky == 0 {
		t.Fatalf("special-mode fixture = %v, %v", info, err)
	}

	if _, _, err := CaptureChildDirectoryWithinAuthorized(root, "state", "operations", stateParents, stateSnapshot); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("special-mode accepted directory error = %v", err)
	}
}
