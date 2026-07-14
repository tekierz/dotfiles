//go:build darwin || linux

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func observeRestoreParentTest(t *testing.T, root, rel string) *ParentChain {
	t.Helper()
	_, parents, err := ObserveDirectoryStateWithin(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	return parents
}

func newRestoreSessionTest(t *testing.T, root string) *RestoreSession {
	t.Helper()
	session, err := NewRestoreSession(root)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func assertRestoreDirTest(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != mode.Perm() {
		t.Fatalf("%s mode = %v, want directory %04o", path, info.Mode(), mode.Perm())
	}
}

func assertRestoreMissingTest(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists or is unreadable: %v", path, err)
	}
}

func TestObserveDirectoryStateTracksAbsenceAndExactDirectory(t *testing.T) {
	root := t.TempDir()
	absent, parents, err := ObserveDirectoryStateWithin(root, "missing/leaf")
	if err != nil || absent.Exists() || (DirectoryState{}).Exists() || !parents.Tracked() {
		t.Fatalf("absent/zero state exists=%v/%v parents=%#v err=%v", absent.Exists(), (DirectoryState{}).Exists(), parents, err)
	}
	mustMkdir(t, filepath.Join(root, "existing"), 0o750)
	existing, parents, err := ObserveDirectoryStateWithin(root, "existing")
	if err != nil || !existing.Exists() || !parents.Tracked() {
		t.Fatalf("existing state exists=%v parents=%#v err=%v", existing.Exists(), parents, err)
	}
}

func TestRestoreSessionBindsOneRealRoot(t *testing.T) {
	real := t.TempDir()
	linked := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(real, linked); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing-root")
	regular := filepath.Join(t.TempDir(), "regular-root")
	mustWrite(t, regular, "not a directory", 0o600)
	for _, invalid := range []string{".", linked, missing, regular} {
		if _, err := NewRestoreSession(invalid); err == nil {
			t.Fatalf("invalid restore root %q was accepted", invalid)
		}
	}
	root := t.TempDir()
	other := t.TempDir()
	session := newRestoreSessionTest(t, root)
	foreign := observeRestoreParentTest(t, other, "parent/leaf")
	if _, err := session.bindRestoreParents("parent/leaf", foreign, true); err == nil {
		t.Fatal("session accepted parent authority captured under another root")
	}
	assertRestoreMissingTest(t, filepath.Join(root, "parent"))
	sameRoot := observeRestoreParentTest(t, root, "authorized/nested/leaf")
	if _, err := session.bindRestoreParents("other/nested/leaf", sameRoot, true); err == nil {
		t.Fatal("session accepted same-root parent authority for another path")
	}
	assertRestoreMissingTest(t, filepath.Join(root, "other"))
	base := t.TempDir()
	replaceable := filepath.Join(base, "root")
	mustMkdir(t, replaceable, 0o700)
	accepted := observeRestoreParentTest(t, replaceable, "parent/leaf")
	replacedSession := newRestoreSessionTest(t, replaceable)
	if err := os.Rename(replaceable, filepath.Join(base, "old-root")); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, replaceable, 0o700)
	if _, err := replacedSession.bindRestoreParents("parent/leaf", accepted, true); err == nil {
		t.Fatal("session accepted an identical replacement root")
	}
	assertRestoreMissingTest(t, filepath.Join(replaceable, "parent"))
}

func TestRestoreSessionBindsExistingParentsWithoutMutation(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "parent"), 0o750)
	mustMkdir(t, filepath.Join(root, "parent", "nested"), 0o700)
	beforeParent, _, err := CaptureDirectoryRootWithin(root, "parent")
	if err != nil {
		t.Fatal(err)
	}
	beforeNested, _, err := CaptureDirectoryRootWithin(root, "parent/nested")
	if err != nil {
		t.Fatal(err)
	}
	accepted := observeRestoreParentTest(t, root, "parent/nested/leaf")
	session := newRestoreSessionTest(t, root)
	binding, err := session.bindRestoreParents("parent/nested/leaf", accepted, true)
	if err != nil {
		t.Fatal(err)
	}
	assertRestoreDirTest(t, filepath.Join(root, "parent"), 0o750)
	assertRestoreDirTest(t, filepath.Join(root, "parent", "nested"), 0o700)
	afterParent, _, err := CaptureDirectoryRootWithin(root, "parent")
	if err != nil || !SameDirectoryRootState(beforeParent, afterParent) {
		t.Fatalf("existing parent identity changed: %v", err)
	}
	afterNested, _, err := CaptureDirectoryRootWithin(root, "parent/nested")
	if err != nil || !SameDirectoryRootState(beforeNested, afterNested) {
		t.Fatalf("existing nested parent identity changed: %v", err)
	}
	assertRestoreMissingTest(t, filepath.Join(root, "parent", "nested", "leaf"))
	if err := session.rollbackRestoreParents(binding); err != nil {
		t.Fatal(err)
	}
	assertRestoreDirTest(t, filepath.Join(root, "parent", "nested"), 0o700)
}

func TestRestoreSessionMaterializesMissingTailsAndRollsBackInReverse(t *testing.T) {
	for _, rel := range []string{"one/leaf", "deep/one/two/leaf"} {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			accepted := observeRestoreParentTest(t, root, rel)
			session := newRestoreSessionTest(t, root)
			binding, err := session.bindRestoreParents(rel, accepted, true)
			if err != nil {
				t.Fatal(err)
			}
			parent := filepath.Dir(filepath.FromSlash(rel))
			for current := parent; current != "."; current = filepath.Dir(current) {
				assertRestoreDirTest(t, filepath.Join(root, current), 0o700)
			}
			assertRestoreMissingTest(t, filepath.Join(root, filepath.FromSlash(rel)))
			if err := session.rollbackRestoreParents(binding); err != nil {
				t.Fatal(err)
			}
			assertRestoreMissingTest(t, filepath.Join(root, strings.Split(rel, "/")[0]))
		})
	}
}

func TestRestoreSessionReusesSiblingEvidence(t *testing.T) {
	root := t.TempDir()
	firstAccepted := observeRestoreParentTest(t, root, "shared/first/leaf")
	secondAccepted := observeRestoreParentTest(t, root, "shared/second/leaf")
	session := newRestoreSessionTest(t, root)
	first, err := session.bindRestoreParents("shared/first/leaf", firstAccepted, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.bindRestoreParents("shared/second/leaf", secondAccepted, true)
	if err != nil {
		t.Fatal(err)
	}
	assertRestoreDirTest(t, filepath.Join(root, "shared", "first"), 0o700)
	assertRestoreDirTest(t, filepath.Join(root, "shared", "second"), 0o700)
	if err := session.rollbackRestoreParents(second); err != nil {
		t.Fatal(err)
	}
	assertRestoreDirTest(t, filepath.Join(root, "shared", "first"), 0o700)
	assertRestoreMissingTest(t, filepath.Join(root, "shared", "second"))
	if err := session.rollbackRestoreParents(first); err != nil {
		t.Fatal(err)
	}
	assertRestoreMissingTest(t, filepath.Join(root, "shared"))
}

func TestRestoreSessionValidationOnlyUsesSameSessionEvidence(t *testing.T) {
	root := t.TempDir()
	accepted := observeRestoreParentTest(t, root, "parent/nested/leaf")
	session := newRestoreSessionTest(t, root)
	if _, err := session.bindRestoreParents("parent/nested/leaf", accepted, false); err == nil {
		t.Fatal("validation-only bind created or accepted missing parents")
	}
	assertRestoreMissingTest(t, filepath.Join(root, "parent"))
	created, err := session.bindRestoreParents("parent/nested/leaf", accepted, true)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := session.bindRestoreParents("parent/nested/leaf", accepted, false)
	if err != nil {
		t.Fatalf("same-session evidence rejected: %v", err)
	}
	if err := session.rollbackRestoreParents(validated); err != nil {
		t.Fatal(err)
	}
	assertRestoreDirTest(t, filepath.Join(root, "parent", "nested"), 0o700)
	if err := session.rollbackRestoreParents(created); err != nil {
		t.Fatal(err)
	}
	assertRestoreMissingTest(t, filepath.Join(root, "parent"))
}

func TestRestoreSessionRejectsUnacceptedParentAppearances(t *testing.T) {
	tests := []struct {
		name        string
		alter       func(t *testing.T, root string)
		assertAfter func(t *testing.T, root string)
	}{
		{name: "directory", alter: func(t *testing.T, root string) { mustMkdir(t, filepath.Join(root, "parent"), 0o700) }, assertAfter: func(t *testing.T, root string) {
			assertRestoreMissingTest(t, filepath.Join(root, "parent", "nested"))
		}},
		{name: "symlink", alter: func(t *testing.T, root string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "parent")); err != nil {
				t.Fatal(err)
			}
		}, assertAfter: func(t *testing.T, root string) {
			info, err := os.Lstat(filepath.Join(root, "parent"))
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("external symlink was changed: info=%v err=%v", info, err)
			}
		}},
		{name: "non-directory", alter: func(t *testing.T, root string) { mustWrite(t, filepath.Join(root, "parent"), "external", 0o600) }, assertAfter: func(t *testing.T, root string) {
			assertContent(t, filepath.Join(root, "parent"), "external")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			accepted := observeRestoreParentTest(t, root, "parent/nested/leaf")
			session := newRestoreSessionTest(t, root)
			tt.alter(t, root)
			if _, err := session.bindRestoreParents("parent/nested/leaf", accepted, true); err == nil {
				t.Fatal("unaccepted parent appearance was trusted")
			}
			tt.assertAfter(t, root)
		})
	}
}

func TestRestoreSessionRejectsAcceptedAncestorSwap(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "ancestor", "parent"), 0o700)
	accepted := observeRestoreParentTest(t, root, "ancestor/parent/leaf")
	session := newRestoreSessionTest(t, root)
	if err := os.Rename(filepath.Join(root, "ancestor"), filepath.Join(root, "old-ancestor")); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "ancestor", "parent"), 0o700)
	if _, err := session.bindRestoreParents("ancestor/parent/leaf", accepted, true); err == nil {
		t.Fatal("identical ancestor replacement was trusted")
	}
	assertRestoreMissingTest(t, filepath.Join(root, "ancestor", "parent", "leaf"))
}

func TestRestoreSessionRollbackPreservesNonemptyAndReplacedParents(t *testing.T) {
	t.Run("nonempty", func(t *testing.T) {
		root := t.TempDir()
		accepted := observeRestoreParentTest(t, root, "parent/nested/leaf")
		session := newRestoreSessionTest(t, root)
		binding, err := session.bindRestoreParents("parent/nested/leaf", accepted, true)
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(root, "parent", "nested", "external"), "preserve", 0o600)
		err = session.rollbackRestoreParents(binding)
		var recovery *RecoveryError
		if !errors.As(err, &recovery) || !errors.Is(err, ErrDirectoryChanged) {
			t.Fatalf("rollback error = %v, want RecoveryError/ErrDirectoryChanged", err)
		}
		assertContent(t, filepath.Join(root, "parent", "nested", "external"), "preserve")
	})

	t.Run("replaced", func(t *testing.T) {
		root := t.TempDir()
		accepted := observeRestoreParentTest(t, root, "parent/leaf")
		session := newRestoreSessionTest(t, root)
		binding, err := session.bindRestoreParents("parent/leaf", accepted, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(root, "parent"), filepath.Join(root, "moved")); err != nil {
			t.Fatal(err)
		}
		mustMkdir(t, filepath.Join(root, "parent"), 0o700)
		err = session.rollbackRestoreParents(binding)
		var recovery *RecoveryError
		if !errors.As(err, &recovery) || !errors.Is(err, ErrDirectoryChanged) {
			t.Fatalf("rollback error = %v, want RecoveryError/ErrDirectoryChanged", err)
		}
		assertRestoreDirTest(t, filepath.Join(root, "parent"), 0o700)
		assertRestoreDirTest(t, filepath.Join(root, "moved"), 0o700)
	})
}

func TestRestoreSessionRejectsNilZeroAndCrossSessionAuthority(t *testing.T) {
	root := t.TempDir()
	accepted := observeRestoreParentTest(t, root, "parent/leaf")
	var nilSession *RestoreSession
	if _, err := nilSession.bindRestoreParents("parent/leaf", accepted, true); err == nil {
		t.Fatal("nil session bind succeeded")
	}
	if err := nilSession.rollbackRestoreParents(restoreParentBinding{}); err == nil {
		t.Fatal("nil session rollback succeeded")
	}
	zeroSession := &RestoreSession{}
	if _, err := zeroSession.bindRestoreParents("parent/leaf", accepted, true); err == nil {
		t.Fatal("zero session bind succeeded")
	}
	valid := newRestoreSessionTest(t, root)
	if _, err := valid.bindRestoreParents("parent/leaf", nil, true); err == nil {
		t.Fatal("nil parent authority succeeded")
	}
	if _, err := valid.bindRestoreParents("parent/leaf", &ParentChain{}, true); err == nil {
		t.Fatal("zero parent authority succeeded")
	}
	if err := valid.rollbackRestoreParents(restoreParentBinding{}); err == nil {
		t.Fatal("zero binding rollback succeeded")
	}
	binding, err := valid.bindRestoreParents("parent/leaf", accepted, true)
	if err != nil {
		t.Fatal(err)
	}
	other := newRestoreSessionTest(t, root)
	if err := other.rollbackRestoreParents(binding); err == nil {
		t.Fatal("cross-session rollback authority succeeded")
	}
	assertRestoreDirTest(t, filepath.Join(root, "parent"), 0o700)
	if err := valid.rollbackRestoreParents(binding); err != nil {
		t.Fatal(err)
	}
}
