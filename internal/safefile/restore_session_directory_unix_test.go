//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func makeRestoreDirectoryTreeTest(t *testing.T, path, value string) {
	t.Helper()
	mustMkdir(t, path, 0o750)
	mustMkdir(t, filepath.Join(path, "nested"), 0o710)
	mustMkdir(t, filepath.Join(path, "empty"), 0o730)
	if err := os.Chmod(filepath.Join(path, "empty"), 0o730); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(path, "config"), value+" config\n", 0o640)
	mustWrite(t, filepath.Join(path, "nested", "value"), value+" nested\n", 0o604)
}

func restoreDirectorySourceTest(t *testing.T) (string, *DirectorySnapshot) {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "backup"), 0o700)
	mustMkdir(t, filepath.Join(root, "backup", "selected"), 0o700)
	makeRestoreDirectoryTreeTest(t, filepath.Join(root, "backup", "selected", "app"), "captured")
	mustWrite(t, filepath.Join(root, "backup", "plain"), "not a directory\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "backup")
	wantRestoreErrorTest(t, err, nil)
	return root, snapshot
}

func observeRestoreDirectoryTest(t *testing.T, root, rel string) (*DirectorySnapshot, *ParentChain) {
	t.Helper()
	snapshot, parents, err := ObserveDirectoryWithin(root, rel)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if (snapshot == nil) != errors.Is(err, os.ErrNotExist) || !parents.Tracked() {
		t.Fatalf("directory observation snapshot=%#v parents=%#v err=%v", snapshot, parents, err)
	}
	return snapshot, parents
}

func wantRestoreDirectoryTreeTest(t *testing.T, root, rel string, want *DirectorySnapshot) {
	t.Helper()
	got, _, err := ObserveDirectoryWithin(root, rel)
	if err != nil || !SameDirectoryIdentity(got, want) || !sameDirectorySnapshotTree(got, want) {
		t.Fatalf("directory %q changed: got=%#v want=%#v err=%v", rel, got, want, err)
	}
}

func TestRestoreSessionRestoreDirectoryReturnsExactRecursiveEvidence(t *testing.T) {
	for _, tt := range []struct {
		name, rel        string
		parent, existing bool
	}{{"missing-parents", "new/parent/app", false, false}, {"missing-directory", "parent/app", true, false}, {"existing-directory", "parent/app", true, true}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.parent {
				mustMkdir(t, filepath.Join(root, "parent"), 0o755)
			}
			if tt.existing {
				makeRestoreDirectoryTreeTest(t, filepath.Join(root, "parent", "app"), "old")
			}
			expected, parents := observeRestoreDirectoryTest(t, root, tt.rel)
			sourceRoot, source := restoreDirectorySourceTest(t)
			mustWrite(t, filepath.Join(sourceRoot, "backup", "selected", "app", "config"), "poison\n", 0o600)
			if err := os.Chmod(filepath.Join(sourceRoot, "backup", "selected", "app", "nested"), 0o700); err != nil {
				t.Fatal(err)
			}
			evidence, err := newRestoreSessionTest(t, root).RestoreDirectory(tt.rel, parents, expected, source, "selected/app")
			if err != nil {
				t.Fatal(err)
			}
			installed, _, err := ObserveDirectoryWithin(root, tt.rel)
			if err != nil || evidence == nil || !SameDirectoryIdentity(evidence, installed) || !sameDirectorySnapshotTree(evidence, installed) {
				t.Fatalf("installed/evidence mismatch: evidence=%#v installed=%#v err=%v", evidence, installed, err)
			}
			assertContent(t, filepath.Join(root, filepath.FromSlash(tt.rel), "config"), "captured config\n")
			assertContent(t, filepath.Join(root, filepath.FromSlash(tt.rel), "nested", "value"), "captured nested\n")
			assertMode(t, filepath.Join(root, filepath.FromSlash(tt.rel)), 0o750)
			assertMode(t, filepath.Join(root, filepath.FromSlash(tt.rel), "nested"), 0o710)
			assertMode(t, filepath.Join(root, filepath.FromSlash(tt.rel), "empty"), 0o730)
			if !tt.parent {
				assertRestoreDirTest(t, filepath.Join(root, "new"), 0o700)
				assertRestoreDirTest(t, filepath.Join(root, "new", "parent"), 0o700)
			}
		})
	}
}

func TestRestoreSessionRestoreDirectoryRejectsInvalidSourceBeforeMutation(t *testing.T) {
	sourceRoot, valid := restoreDirectorySourceTest(t)
	rootOnly, _, err := CaptureDirectoryRootWithin(sourceRoot, "backup")
	wantRestoreErrorTest(t, err, nil)
	corrupt := *valid
	corrupt.digest[0] ^= 0xff
	cases := []struct {
		name, rel string
		source    *DirectorySnapshot
		want      error
	}{{"nil", "selected/app", nil, ErrInvalidAuthority}, {"untracked", "selected/app", &DirectorySnapshot{}, ErrInvalidAuthority},
		{"root-only", "selected/app", rootOnly, ErrInvalidAuthority}, {"corrupt", "selected/app", &corrupt, ErrInvalidAuthority},
		{"missing", "missing", valid, fs.ErrNotExist}, {"file", "plain", valid, ErrInvalidPath}, {"cross-file", "plain/child", valid, ErrInvalidPath}}
	for index, rel := range []string{"", ".", "./selected", "selected/", "selected//app", "selected/./app", "../app", "selected/../app", "app\x00bad", filepath.Join(sourceRoot, "absolute")} {
		cases = append(cases, struct {
			name, rel string
			source    *DirectorySnapshot
			want      error
		}{fmt.Sprintf("path-%02d", index), rel, valid, ErrInvalidPath})
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root, rel := t.TempDir(), "new/nested/app"
			_, parents := observeRestoreDirectoryTest(t, root, rel)
			hooks := 0
			setDirectoryHooks(t, func(h *directoryHooks) {
				h.beforeShallowMkdir = func(_ int, _ string) error { hooks++; return nil }
				h.beforeInstall = func(_, _ int, _, _ string) error { hooks++; return nil }
			})
			_, err := newRestoreSessionTest(t, root).RestoreDirectory(rel, parents, nil, tt.source, tt.rel)
			wantRestoreErrorTest(t, err, tt.want)
			if hooks != 0 {
				t.Fatalf("invalid source reached %d mutation hooks", hooks)
			}
			assertDirectoryEntries(t, root, []string{})
		})
	}
}

func TestRestoreSessionDirectoryAuthorityInputsAreExact(t *testing.T) {
	root, rel := t.TempDir(), "parent/app"
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	makeRestoreDirectoryTreeTest(t, filepath.Join(root, "parent", "app"), "accepted")
	expected, parents := observeRestoreDirectoryTest(t, root, rel)
	_, source := restoreDirectorySourceTest(t)
	session := newRestoreSessionTest(t, root)
	check := func(name string, candidate *RestoreSession, path string, parent *ParentChain, state *DirectorySnapshot, want error) {
		t.Run(name, func(t *testing.T) {
			_, err := candidate.RestoreDirectory(path, parent, state, source, "selected/app")
			wantRestoreErrorTest(t, err, want)
			wantRestoreErrorTest(t, candidate.RemoveDirectory(path, parent, state), want)
		})
	}
	check("nil-session", nil, rel, parents, expected, ErrInvalidAuthority)
	check("zero-session", &RestoreSession{}, rel, parents, expected, ErrInvalidAuthority)
	for index, path := range []string{"", ".", "/absolute", "..", "../outside", "nested/../outside", "nested//app"} {
		check(fmt.Sprintf("path-%02d", index), session, path, parents, expected, ErrInvalidPath)
	}
	check("nil-parent", session, rel, nil, expected, ErrParentChanged)
	check("zero-parent", session, rel, &ParentChain{}, expected, ErrParentChanged)
	other := t.TempDir()
	mustMkdir(t, filepath.Join(other, "parent"), 0o700)
	check("foreign-parent", session, rel, fileParentChainTest(t, other, rel), expected, ErrParentChanged)
	check("wrong-path-parent", session, rel, fileParentChainTest(t, root, "other/app"), expected, ErrParentChanged)
	check("zero-expected", session, rel, parents, &DirectorySnapshot{}, ErrDirectoryChanged)
	rootOnly, _, err := CaptureDirectoryRootWithin(root, rel)
	wantRestoreErrorTest(t, err, nil)
	check("root-only-expected", session, rel, parents, rootOnly, ErrDirectoryChanged)
	wantRestoreErrorTest(t, session.RemoveDirectory(rel, parents, nil), ErrDirectoryChanged)
	wantRestoreDirectoryTreeTest(t, root, rel, expected)
}

func TestRestoreSessionDirectoryRejectsRootAndParentSwaps(t *testing.T) {
	_, source := restoreDirectorySourceTest(t)
	for index, kind := range []string{"root", "parent"} {
		t.Run(kind, func(t *testing.T) {
			base, root := t.TempDir(), ""
			if kind == "root" {
				root = filepath.Join(base, "root")
				mustMkdir(t, root, 0o700)
			} else {
				root = base
			}
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			makeRestoreDirectoryTreeTest(t, filepath.Join(root, "parent", "app"), "accepted")
			expected, parents := observeRestoreDirectoryTest(t, root, "parent/app")
			session := newRestoreSessionTest(t, root)
			old, moved := filepath.Join(base, "old-"+kind), root
			if kind == "parent" {
				moved = filepath.Join(root, "parent")
			}
			if err := os.Rename(moved, old); err != nil {
				t.Fatal(err)
			}
			if kind == "root" {
				mustMkdir(t, root, 0o700)
			}
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			makeRestoreDirectoryTreeTest(t, filepath.Join(root, "parent", "app"), "replacement")
			replacement, _ := observeRestoreDirectoryTest(t, root, "parent/app")
			_, err := session.RestoreDirectory("parent/app", parents, expected, source, "selected/app")
			want := ErrParentChanged
			if kind == "root" {
				want = ErrDirectoryChanged
			}
			wantRestoreErrorTest(t, err, want)
			wantRestoreErrorTest(t, session.RemoveDirectory("parent/app", parents, expected), want)
			wantRestoreDirectoryTreeTest(t, root, "parent/app", replacement)
			wantRestoreDirectoryTreeTest(t, old, []string{"parent/app", "app"}[index], expected)
		})
	}
}

func TestRestoreSessionRestoreDirectoryPreservesRejectedDrift(t *testing.T) {
	for _, kind := range []string{"disappearance", "appearance", "same-inode", "identity", "file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, rel := t.TempDir(), "parent/app"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			if kind != "appearance" {
				makeRestoreDirectoryTreeTest(t, filepath.Join(root, "parent", "app"), "accepted")
			}
			expected, parents := observeRestoreDirectoryTest(t, root, rel)
			_, source := restoreDirectorySourceTest(t)
			target := filepath.Join(root, filepath.FromSlash(rel))
			var preserved *DirectorySnapshot
			var revision Revision
			var link symlinkStateTest
			switch kind {
			case "disappearance":
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
			case "appearance":
				makeRestoreDirectoryTreeTest(t, target, "appeared")
				preserved, _ = observeRestoreDirectoryTest(t, root, rel)
			case "same-inode":
				mustWrite(t, filepath.Join(target, "nested", "value"), "edited\n", 0o600)
				if err := os.Chmod(target, 0o700); err != nil {
					t.Fatal(err)
				}
				preserved, _ = observeRestoreDirectoryTest(t, root, rel)
			case "identity":
				if err := os.Rename(target, filepath.Join(root, "parent", "accepted-old")); err != nil {
					t.Fatal(err)
				}
				makeRestoreDirectoryTreeTest(t, target, "accepted")
				preserved, _ = observeRestoreDirectoryTest(t, root, rel)
			case "file":
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				mustWrite(t, target, "preserve file\n", 0o604)
				revision = fileRevisionTest(t, root, rel)
			case "symlink":
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
				link = symlinkLstatTest(t, target)
			}
			_, err := newRestoreSessionTest(t, root).RestoreDirectory(rel, parents, expected, source, "selected/app")
			want := ErrDirectoryChanged
			if kind == "file" {
				want = ErrNonRegular
			}
			if kind == "symlink" {
				want = ErrSymlink
			}
			wantRestoreErrorTest(t, err, want)
			switch kind {
			case "disappearance":
				assertRestoreMissingTest(t, target)
			case "appearance", "same-inode", "identity":
				wantRestoreDirectoryTreeTest(t, root, rel, preserved)
				if kind == "identity" {
					wantRestoreDirectoryTreeTest(t, root, "parent/accepted-old", expected)
				}
			case "file":
				wantFileRevisionTest(t, root, rel, revision)
			case "symlink":
				wantSymlinkLstatTest(t, target, link)
			}
		})
	}
}

func TestRestoreSessionRestoreDirectoryRollbackAndCommitErrors(t *testing.T) {
	for _, kind := range []string{"rollback", "recovery", "committed", "evidence", "existing-rollback"} {
		t.Run(kind, func(t *testing.T) {
			root, rel := t.TempDir(), "new/nested/app"
			if kind == "existing-rollback" {
				rel = "parent/app"
				mustMkdir(t, filepath.Join(root, "parent"), 0o700)
				makeRestoreDirectoryTreeTest(t, filepath.Join(root, "parent", "app"), "accepted")
			}
			expected, parents := observeRestoreDirectoryTest(t, root, rel)
			_, source := restoreDirectorySourceTest(t)
			injected := errors.New("injected " + kind)
			setDirectoryHooks(t, func(h *directoryHooks) {
				switch kind {
				case "rollback", "recovery":
					h.beforeInstall = func(_, _ int, _, _ string) error {
						if kind == "recovery" {
							mustWrite(t, filepath.Join(root, "new", "nested", "external"), "preserve\n", 0o600)
						}
						return injected
					}
				case "committed":
					h.afterInstall = func(_, _ int, _ string) error { return injected }
				case "evidence":
					h.beforeRestoreEvidence = func(_ int, _ string) error { return injected }
				case "existing-rollback":
					h.afterMoveAside = func(_ int, _, _ string) error { return injected }
				}
			})
			evidence, err := newRestoreSessionTest(t, root).RestoreDirectory(rel, parents, expected, source, "selected/app")
			wantRestoreErrorTest(t, err, injected)
			var recovery *RecoveryError
			var committed *CommittedError
			switch kind {
			case "rollback":
				if errors.As(err, &recovery) || errors.As(err, &committed) {
					t.Fatalf("precommit error wrapped: %v", err)
				}
				assertRestoreMissingTest(t, filepath.Join(root, "new"))
			case "recovery":
				if !errors.As(err, &recovery) || errors.As(err, &committed) {
					t.Fatalf("error = %v, want RecoveryError", err)
				}
				assertRestoreMissingTest(t, filepath.Join(root, filepath.FromSlash(rel)))
				assertContent(t, filepath.Join(root, "new", "nested", "external"), "preserve\n")
			case "committed", "evidence":
				if !errors.As(err, &committed) || evidence != nil {
					t.Fatalf("error/evidence = %v/%#v, want CommittedError/nil", err, evidence)
				}
				installed, _ := observeRestoreDirectoryTest(t, root, rel)
				if installed == nil {
					t.Fatal("committed restore missing")
				}
			case "existing-rollback":
				if errors.As(err, &committed) {
					t.Fatalf("rollback reported committed: %v", err)
				}
				wantRestoreDirectoryTreeTest(t, root, rel, expected)
			}
		})
	}
}

func TestRestoreSessionRemoveDirectoryExactStaleAndBoundaries(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		root, rel := t.TempDir(), "parent/app"
		mustMkdir(t, filepath.Join(root, "parent"), 0o700)
		makeRestoreDirectoryTreeTest(t, filepath.Join(root, "parent", "app"), "accepted")
		expected, parents := observeRestoreDirectoryTest(t, root, rel)
		if err := newRestoreSessionTest(t, root).RemoveDirectory(rel, parents, expected); err != nil {
			t.Fatal(err)
		}
		assertRestoreMissingTest(t, filepath.Join(root, filepath.FromSlash(rel)))
	})
	t.Run("missing-parent", func(t *testing.T) {
		root, rel := t.TempDir(), "missing/nested/app"
		_, parents := observeRestoreDirectoryTest(t, root, rel)
		wantRestoreErrorTest(t, newRestoreSessionTest(t, root).RemoveDirectory(rel, parents, nil), ErrDirectoryChanged)
		assertDirectoryEntries(t, root, []string{})
	})
	for _, kind := range []string{"same-inode", "identity", "file", "symlink"} {
		t.Run("stale-"+kind, func(t *testing.T) {
			root, rel := t.TempDir(), "parent/app"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			target := filepath.Join(root, filepath.FromSlash(rel))
			makeRestoreDirectoryTreeTest(t, target, "accepted")
			expected, parents := observeRestoreDirectoryTest(t, root, rel)
			var preserved *DirectorySnapshot
			var revision Revision
			var link symlinkStateTest
			switch kind {
			case "same-inode":
				mustWrite(t, filepath.Join(target, "config"), "edited\n", 0o600)
				preserved, _ = observeRestoreDirectoryTest(t, root, rel)
			case "identity":
				if err := os.Rename(target, filepath.Join(root, "parent", "accepted-old")); err != nil {
					t.Fatal(err)
				}
				makeRestoreDirectoryTreeTest(t, target, "accepted")
				preserved, _ = observeRestoreDirectoryTest(t, root, rel)
			case "file":
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				mustWrite(t, target, "preserve\n", 0o604)
				revision = fileRevisionTest(t, root, rel)
			case "symlink":
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
				link = symlinkLstatTest(t, target)
			}
			want := ErrDirectoryChanged
			switch kind {
			case "file":
				want = ErrNonRegular
			case "symlink":
				want = ErrSymlink
			}
			wantRestoreErrorTest(t, newRestoreSessionTest(t, root).RemoveDirectory(rel, parents, expected), want)
			switch kind {
			case "file":
				wantFileRevisionTest(t, root, rel, revision)
			case "symlink":
				wantSymlinkLstatTest(t, target, link)
			default:
				wantRestoreDirectoryTreeTest(t, root, rel, preserved)
			}
			if kind == "identity" {
				wantRestoreDirectoryTreeTest(t, root, "parent/accepted-old", expected)
			}
		})
	}
	for _, kind := range []string{"pre-error", "move-rollback", "post-commit", "post-cleanup", "post-appearance"} {
		t.Run(kind, func(t *testing.T) {
			root, rel := t.TempDir(), "parent/app"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			target := filepath.Join(root, filepath.FromSlash(rel))
			makeRestoreDirectoryTreeTest(t, target, "accepted")
			expected, parents := observeRestoreDirectoryTest(t, root, rel)
			injected := errors.New("injected " + kind)
			var appeared *DirectorySnapshot
			setDirectoryHooks(t, func(h *directoryHooks) {
				switch kind {
				case "pre-error":
					h.beforeRemove = func(_ int, _ string) error { return injected }
				case "move-rollback":
					h.afterRemoveMove = func(_ int, _, _ string) error { return injected }
				case "post-commit":
					h.afterRemoveCommit = func(_ int, _ string) error { return injected }
				case "post-cleanup":
					h.afterRemoveCleanup = func(_ int) error { return injected }
				case "post-appearance":
					h.afterRemoveCleanup = func(_ int) error {
						makeRestoreDirectoryTreeTest(t, target, "appeared")
						appeared, _ = observeRestoreDirectoryTest(t, root, rel)
						return nil
					}
				}
			})
			err := newRestoreSessionTest(t, root).RemoveDirectory(rel, parents, expected)
			var committed *CommittedError
			switch kind {
			case "pre-error", "move-rollback":
				wantRestoreErrorTest(t, err, injected)
				if errors.As(err, &committed) {
					t.Fatalf("precommit removal committed: %v", err)
				}
				wantRestoreDirectoryTreeTest(t, root, rel, expected)
			case "post-commit":
				if !errors.As(err, &committed) || !errors.Is(err, injected) {
					t.Fatalf("error = %v, want committed injected", err)
				}
				assertRestoreMissingTest(t, target)
				if len(safefileArtifacts(t, filepath.Dir(target), ".safefile-removed-")) != 1 {
					t.Fatal("recovery artifact not preserved")
				}
			case "post-cleanup":
				if !errors.As(err, &committed) || !errors.Is(err, injected) {
					t.Fatalf("error = %v, want committed injected", err)
				}
				assertRestoreMissingTest(t, target)
			case "post-appearance":
				if !errors.As(err, &committed) || !errors.Is(err, ErrDirectoryChanged) {
					t.Fatalf("error = %v, want committed appearance", err)
				}
				wantRestoreDirectoryTreeTest(t, root, rel, appeared)
			}
		})
	}
}
