//go:build darwin || linux

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthorizedReplaceRejectsImmediateParentReplacement(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	_, missing, err := ReadWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	parents, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "parent"), filepath.Join(root, "old-parent")); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	if _, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(root, "parent/config", missing, parents, []byte("value"), 0o600); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("replacement error = %v, want ErrParentChanged", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "parent", "config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement parent changed: %v", err)
	}
}

func TestAuthorizedLockReleaseRejectsUnlinkedRecreatedLeaf(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "locks"), 0o700)
	parents, err := CaptureParentChainWithin(root, "locks/config.lock")
	if err != nil {
		t.Fatal(err)
	}
	release, err := AcquireLockWithinAuthorized(root, "locks/config.lock", 0o600, parents)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, "locks", "config.lock")
	if err := os.Rename(lockPath, lockPath+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := release(); !errors.Is(err, ErrLockChanged) {
		t.Fatalf("release error = %v, want ErrLockChanged", err)
	}
}

func TestValidateParentChainPermitsOnlyExactAcceptedCreationEvidence(t *testing.T) {
	root := t.TempDir()
	accepted, err := CaptureParentChainWithin(root, "parent/child/file")
	if err != nil {
		t.Fatal(err)
	}
	rootParents, err := BindParentChainPrefixWithin(root, "parent/child/file", "parent", accepted, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := EnsureShallowDirectoryWithinParentChainTracked(root, "parent", nil, rootParents, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateParentChainWithin(root, "parent/child/file", accepted, map[string]*DirectorySnapshot{"parent": parent}); err != nil {
		t.Fatalf("exact creation evidence rejected: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "parent", "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateParentChainWithin(root, "parent/child/file", accepted, map[string]*DirectorySnapshot{"parent": parent}); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("unaccepted deeper appearance error = %v, want ErrParentChanged", err)
	}
}

func TestAuthorizedReplaceRejectsParentModeChange(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	_, missing, err := ReadWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	parents, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(root, "parent/config", missing, parents, []byte("value"), 0o600); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("mode change error = %v, want ErrParentChanged", err)
	}
}

func TestCaptureParentChainRefusesWritableDescendantParent(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	if err := os.Chmod(parent, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureParentChainWithin(root, "parent/config"); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("writable parent error = %v, want ErrParentChanged", err)
	}
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureParentChainWithin(root, "parent/config"); err != nil {
		t.Fatalf("0755 parent rejected: %v", err)
	}
}

func TestCaptureParentChainRefusesWritableTrustedRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := CaptureParentChainWithin(root, "config"); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("writable root error = %v, want ErrParentChanged", err)
	}
}

func TestFileObservationRejectsForeignOwnerWhenChownIsAvailable(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "value", 0o600)
	if !changeToForeignOwnerForTest(target) {
		t.Skip("cannot create a foreign-owned test file as this user")
	}
	if _, _, err := ReadWithin(root, "config"); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("foreign file owner error = %v, want ErrRevisionChanged", err)
	}
}

func TestParentChainRejectsForeignOwnerWhenChownIsAvailable(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	if !changeToForeignOwnerForTest(parent) {
		t.Skip("cannot create a foreign-owned test directory as this user")
	}
	if _, err := CaptureParentChainWithin(root, "parent/config"); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("foreign parent owner error = %v, want ErrParentChanged", err)
	}
}

func changeToForeignOwnerForTest(path string) bool {
	if os.Geteuid() == 0 {
		uid := 1
		if os.Geteuid() == uid {
			uid = 2
		}
		return os.Chown(path, uid, os.Getegid()) == nil
	}
	groups, err := os.Getgroups()
	if err != nil {
		return false
	}
	for _, gid := range groups {
		if gid != os.Getegid() && os.Chown(path, -1, gid) == nil {
			return true
		}
	}
	return false
}

func TestAuthorizedReplaceRejectsAncestorGraftWithOriginalImmediateParent(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "ancestor"), 0o700)
	mustMkdir(t, filepath.Join(root, "ancestor", "parent"), 0o700)
	_, missing, err := ReadWithin(root, "ancestor/parent/config")
	if err != nil {
		t.Fatal(err)
	}
	parents, err := CaptureParentChainWithin(root, "ancestor/parent/config")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "ancestor"), filepath.Join(root, "old-ancestor")); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "ancestor"), 0o700)
	if err := os.Rename(filepath.Join(root, "old-ancestor", "parent"), filepath.Join(root, "ancestor", "parent")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(root, "ancestor/parent/config", missing, parents, []byte("value"), 0o600); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("graft error = %v, want ErrParentChanged", err)
	}
}

func TestBindParentChainRequiresExactCreatedParentEvidence(t *testing.T) {
	root := t.TempDir()
	accepted, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	parentAuthority, err := CaptureParentChainWithin(root, "parent")
	if err != nil {
		t.Fatal(err)
	}
	created, err := EnsureShallowDirectoryWithinParentChainTracked(root, "parent", nil, parentAuthority, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindParentChainWithin(root, "parent/config", accepted, nil); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("unproven bind error = %v, want ErrParentChanged", err)
	}
	bound, err := BindParentChainWithin(root, "parent/config", accepted, map[string]*DirectorySnapshot{"parent": created})
	if err != nil {
		t.Fatal(err)
	}
	_, missing, err := ReadWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(root, "parent/config", missing, bound, []byte("value"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertContent(t, filepath.Join(root, "parent", "config"), "value")
}

func TestCreatedParentReplacementIsRejectedBeforeAndAfterBind(t *testing.T) {
	for _, replaceBeforeBind := range []bool{true, false} {
		t.Run(map[bool]string{true: "before bind", false: "after bind"}[replaceBeforeBind], func(t *testing.T) {
			root := t.TempDir()
			accepted, err := CaptureParentChainWithin(root, "parent/config")
			if err != nil {
				t.Fatal(err)
			}
			parentAuthority, err := CaptureParentChainWithin(root, "parent")
			if err != nil {
				t.Fatal(err)
			}
			created, err := EnsureShallowDirectoryWithinParentChainTracked(root, "parent", nil, parentAuthority, 0o700)
			if err != nil {
				t.Fatal(err)
			}
			var bound *ParentChain
			if !replaceBeforeBind {
				bound, err = BindParentChainWithin(root, "parent/config", accepted, map[string]*DirectorySnapshot{"parent": created})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Rename(filepath.Join(root, "parent"), filepath.Join(root, "old-parent")); err != nil {
				t.Fatal(err)
			}
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			if replaceBeforeBind {
				if _, err := BindParentChainWithin(root, "parent/config", accepted, map[string]*DirectorySnapshot{"parent": created}); !errors.Is(err, ErrParentChanged) {
					t.Fatalf("bind replacement error = %v, want ErrParentChanged", err)
				}
				return
			}
			_, missing, err := ReadWithin(root, "parent/config")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(root, "parent/config", missing, bound, []byte("value"), 0o600); !errors.Is(err, ErrParentChanged) {
				t.Fatalf("commit replacement error = %v, want ErrParentChanged", err)
			}
			if _, err := os.Lstat(filepath.Join(root, "parent", "config")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("replacement received write: %v", err)
			}
		})
	}
}

func TestAuthorizedReplaceRejectsRootReplacementBeforeCommit(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, filepath.Join(root, "config"), 0o700)
	_, missing, err := ReadWithin(root, "config/value")
	if err != nil {
		t.Fatal(err)
	}
	parents, err := CaptureParentChainWithin(root, "config/value")
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	replaceTestHooks.beforeCommit = func(_, _ int, _, _ string) error {
		if err := os.Rename(root, moved); err != nil {
			return err
		}
		return os.Mkdir(root, 0o700)
	}
	t.Cleanup(func() { replaceTestHooks = replaceHooks{} })
	if _, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(root, "config/value", missing, parents, []byte("value"), 0o600); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("root replacement error = %v, want ErrParentChanged", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "config")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement root changed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(moved, "config", "value")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("detached original root received write: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(moved, "config"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("precommit refusal left staged entries: entries=%v err=%v", entries, err)
	}
}

func TestAuthorizedReplaceRootReplacementAfterCommitReturnsNoEvidence(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, filepath.Join(root, "config"), 0o700)
	_, missing, err := ReadWithin(root, "config/value")
	if err != nil {
		t.Fatal(err)
	}
	parents, err := CaptureParentChainWithin(root, "config/value")
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	replaceTestHooks.afterCommit = func(_, _ int, _ string) error {
		if err := os.Rename(root, moved); err != nil {
			return err
		}
		return os.Mkdir(root, 0o700)
	}
	t.Cleanup(func() { replaceTestHooks = replaceHooks{} })
	evidence, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(root, "config/value", missing, parents, []byte("value"), 0o600)
	var committed *CommittedError
	if evidence.Tracked() || !errors.As(err, &committed) || !errors.Is(err, ErrParentChanged) {
		t.Fatalf("result = evidence %+v err %v, want committed parent change without evidence", evidence, err)
	}
	assertContent(t, filepath.Join(moved, "config", "value"), "value")
}

func TestAuthorizedDirectoryRemovalRefusesMissingParentAndLeaf(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	mustMkdir(t, filepath.Join(root, "parent", "target"), 0o700)
	expected, err := SnapshotDirectoryWithin(root, "parent/target")
	if err != nil {
		t.Fatal(err)
	}
	parents, err := CaptureParentChainWithin(root, "parent/target")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "parent", "target")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDirectoryWithinSnapshotAuthorized(root, "parent/target", expected, parents); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("missing leaf error = %v, want ErrDirectoryChanged", err)
	}
	if err := os.Remove(filepath.Join(root, "parent")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDirectoryWithinSnapshotAuthorized(root, "parent/target", expected, parents); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("missing parent error = %v, want ErrParentChanged", err)
	}
}

func TestAuthorizedDirectoryRestoreRejectsAncestorGraftBeforeCommit(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "value", 0o600)
	source, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "ancestor"), 0o700)
	mustMkdir(t, filepath.Join(root, "ancestor", "parent"), 0o700)
	parents, err := CaptureParentChainWithin(root, "ancestor/parent/live")
	if err != nil {
		t.Fatal(err)
	}
	directoryTestHooks.beforeInstall = func(_ int, _ int, _, _ string) error {
		if err := os.Rename(filepath.Join(root, "ancestor"), filepath.Join(root, "old-ancestor")); err != nil {
			return err
		}
		if err := os.Mkdir(filepath.Join(root, "ancestor"), 0o700); err != nil {
			return err
		}
		return os.Rename(filepath.Join(root, "old-ancestor", "parent"), filepath.Join(root, "ancestor", "parent"))
	}
	t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
	evidence, err := RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(root, "ancestor/parent/live", source, nil, parents)
	if evidence != nil || !errors.Is(err, ErrParentChanged) {
		t.Fatalf("restore result = evidence %#v err %v, want precommit ErrParentChanged", evidence, err)
	}
	if _, err := os.Lstat(filepath.Join(root, "ancestor", "parent", "live")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("grafted parent received directory: %v", err)
	}
}

func TestAuthorizedDirectoryRestoreEvidenceRejectsCommittedAncestorGraft(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "value", 0o600)
	source, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "ancestor"), 0o700)
	mustMkdir(t, filepath.Join(root, "ancestor", "parent"), 0o700)
	parents, err := CaptureParentChainWithin(root, "ancestor/parent/live")
	if err != nil {
		t.Fatal(err)
	}
	directoryTestHooks.beforeRestoreEvidence = func(_ int, _ string) error {
		if err := os.Rename(filepath.Join(root, "ancestor"), filepath.Join(root, "old-ancestor")); err != nil {
			return err
		}
		if err := os.Mkdir(filepath.Join(root, "ancestor"), 0o700); err != nil {
			return err
		}
		return os.Rename(filepath.Join(root, "old-ancestor", "parent"), filepath.Join(root, "ancestor", "parent"))
	}
	t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
	evidence, err := RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(root, "ancestor/parent/live", source, nil, parents)
	var committed *CommittedError
	if evidence != nil || !errors.As(err, &committed) || !errors.Is(err, ErrParentChanged) {
		t.Fatalf("restore result = evidence %#v err %v, want committed ErrParentChanged without evidence", evidence, err)
	}
	assertContent(t, filepath.Join(root, "ancestor", "parent", "live", "config"), "value")
}

func TestAuthorizedFileAndEmptyDirectoryRemovalRejectAncestorGraft(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		root := t.TempDir()
		mustMkdir(t, filepath.Join(root, "ancestor"), 0o700)
		mustMkdir(t, filepath.Join(root, "ancestor", "parent"), 0o700)
		mustWrite(t, filepath.Join(root, "ancestor", "parent", "config"), "value", 0o600)
		_, revision, parents, err := ObserveFileWithin(root, "ancestor/parent/config")
		if err != nil {
			t.Fatal(err)
		}
		replaceTestHooks.beforeRemove = func(_, _ int, _ string) error {
			if err := os.Rename(filepath.Join(root, "ancestor"), filepath.Join(root, "old-ancestor")); err != nil {
				return err
			}
			if err := os.Mkdir(filepath.Join(root, "ancestor"), 0o700); err != nil {
				return err
			}
			return os.Rename(filepath.Join(root, "old-ancestor", "parent"), filepath.Join(root, "ancestor", "parent"))
		}
		t.Cleanup(func() { replaceTestHooks = replaceHooks{} })
		if err := RemoveWithinRevisionAuthorized(root, "ancestor/parent/config", revision, parents); !errors.Is(err, ErrParentChanged) {
			t.Fatalf("file removal error = %v, want ErrParentChanged", err)
		}
		assertContent(t, filepath.Join(root, "ancestor", "parent", "config"), "value")
	})
	t.Run("empty directory", func(t *testing.T) {
		root := t.TempDir()
		mustMkdir(t, filepath.Join(root, "ancestor"), 0o700)
		mustMkdir(t, filepath.Join(root, "ancestor", "parent"), 0o700)
		mustMkdir(t, filepath.Join(root, "ancestor", "parent", "empty"), 0o700)
		expected, parents, err := ObserveDirectoryWithin(root, "ancestor/parent/empty")
		if err != nil {
			t.Fatal(err)
		}
		directoryTestHooks.beforeRemoveEmpty = func(_, _ int, _ string) error {
			if err := os.Rename(filepath.Join(root, "ancestor"), filepath.Join(root, "old-ancestor")); err != nil {
				return err
			}
			if err := os.Mkdir(filepath.Join(root, "ancestor"), 0o700); err != nil {
				return err
			}
			return os.Rename(filepath.Join(root, "old-ancestor", "parent"), filepath.Join(root, "ancestor", "parent"))
		}
		t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
		if err := RemoveEmptyDirectoryWithinSnapshotAuthorized(root, "ancestor/parent/empty", expected, parents); !errors.Is(err, ErrParentChanged) {
			t.Fatalf("empty removal error = %v, want ErrParentChanged", err)
		}
		if info, err := os.Stat(filepath.Join(root, "ancestor", "parent", "empty")); err != nil || !info.IsDir() {
			t.Fatalf("empty directory was not preserved: info=%v err=%v", info, err)
		}
	})
}

func TestAuthorizedDirectoryRemovalDetectsRootReplacementAfterCleanup(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, filepath.Join(root, "target"), 0o700)
	mustWrite(t, filepath.Join(root, "target", "config"), "value", 0o600)
	expected, parents, err := ObserveDirectoryWithin(root, "target")
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	directoryTestHooks.afterRemoveCleanup = func(_ int) error {
		if err := os.Rename(root, moved); err != nil {
			return err
		}
		return os.Mkdir(root, 0o700)
	}
	t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
	err = RemoveDirectoryWithinSnapshotAuthorized(root, "target", expected, parents)
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, ErrParentChanged) {
		t.Fatalf("removal error = %v, want committed ErrParentChanged", err)
	}
	if _, err := os.Lstat(filepath.Join(moved, "target")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed tree remains in original root: %v", err)
	}
}

func TestRootOnlyDirectoryAuthorityCannotAuthorizeRecursiveOperations(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "target"), 0o700)
	mustWrite(t, filepath.Join(root, "target", "config"), "value", 0o600)
	rootOnly, parents, err := CaptureDirectoryRootWithin(root, "target")
	if err != nil {
		t.Fatal(err)
	}
	if rootOnly.Digest() != ([32]byte{}) {
		t.Fatal("root-only authority exposed a recursive digest")
	}
	opened, err := OpenDirectoryWithinAuthorized(root, "target", parents, rootOnly)
	if err != nil {
		t.Fatalf("root-only authority should open its exact directory: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}

	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "replacement", 0o600)
	source, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	assertChanged := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrDirectoryChanged) {
			t.Fatalf("%s error = %v, want ErrDirectoryChanged", name, err)
		}
	}
	assertChanged("verify", VerifyDirectoryWithinSnapshot(root, "target", rootOnly))
	assertChanged("restore source", RestoreDirectoryWithin(root, "target", rootOnly))
	assertChanged("restore expected", RestoreDirectoryWithinSnapshot(root, "target", source, rootOnly))
	_, err = RestoreDirectoryWithinSnapshotNoCreateTracked(root, "target", source, rootOnly)
	assertChanged("restore no-create expected", err)
	_, err = EnsureShallowDirectoryWithinSnapshotTracked(root, "target", rootOnly, 0o700)
	assertChanged("shallow expected", err)
	assertChanged("empty removal", RemoveEmptyDirectoryWithinSnapshot(root, "target", rootOnly))
	assertChanged("recursive removal", RemoveDirectoryWithinSnapshot(root, "target", rootOnly))
	assertContent(t, filepath.Join(root, "target", "config"), "value")
}

func TestRootOnlyDirectoryAuthorityCannotBindCreatedParent(t *testing.T) {
	root := t.TempDir()
	accepted, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	rootOnly, _, err := CaptureDirectoryRootWithin(root, "parent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindParentChainWithin(root, "parent/config", accepted, map[string]*DirectorySnapshot{"parent": rootOnly}); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("bind error = %v, want ErrParentChanged", err)
	}
}
