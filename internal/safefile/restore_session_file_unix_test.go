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

func restoreFileSourceTest(t *testing.T) (string, *DirectorySnapshot) {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "tree"), 0o700)
	mustMkdir(t, filepath.Join(root, "tree", "nested"), 0o710)
	mustWrite(t, filepath.Join(root, "tree", "config"), "captured\n", 0o640)
	mustWrite(t, filepath.Join(root, "tree", "nested", "value"), "nested\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "tree")
	wantRestoreErrorTest(t, err, nil)
	return root, snapshot
}
func fileRevisionTest(t *testing.T, root, rel string) Revision {
	t.Helper()
	_, revision, err := ReadWithin(root, rel)
	wantRestoreErrorTest(t, err, nil)
	return revision
}
func fileParentChainTest(t *testing.T, root, rel string) *ParentChain {
	t.Helper()
	parents, err := CaptureParentChainWithin(root, rel)
	wantRestoreErrorTest(t, err, nil)
	return parents
}
func wantFileRevisionTest(t *testing.T, root, rel string, want Revision) {
	t.Helper()
	if got := fileRevisionTest(t, root, rel); got != want {
		t.Fatalf("file %q revision changed: got=%#v want=%#v", rel, got, want)
	}
}
func wantRestoreErrorTest(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
func directoryRootStateTest(t *testing.T, root, rel string) *DirectorySnapshot {
	t.Helper()
	state, _, err := CaptureDirectoryRootWithin(root, rel)
	wantRestoreErrorTest(t, err, nil)
	return state
}
func wantDirectoryRootStateTest(t *testing.T, root, rel string, want *DirectorySnapshot) {
	t.Helper()
	got := directoryRootStateTest(t, root, rel)
	if !SameDirectoryIdentity(want, got) || !SameDirectoryRootState(want, got) {
		t.Fatalf("directory %q identity/root state changed", rel)
	}
}

type symlinkStateTest struct {
	info os.FileInfo
	dest string
}

func symlinkLstatTest(t *testing.T, path string) symlinkStateTest {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	return symlinkStateTest{info: info, dest: dest}
}
func wantSymlinkLstatTest(t *testing.T, path string, want symlinkStateTest) {
	t.Helper()
	got := symlinkLstatTest(t, path)
	if want.dest != got.dest || want.info.Name() != got.info.Name() || want.info.Mode() != got.info.Mode() ||
		want.info.Size() != got.info.Size() || !want.info.ModTime().Equal(got.info.ModTime()) || !os.SameFile(want.info, got.info) {
		t.Fatalf("symlink Lstat/Readlink changed: got=%#v -> %q want=%#v -> %q", got.info, got.dest, want.info, want.dest)
	}
}
func TestRestoreSessionRestoreFileUsesImmutableSourceAndReturnsExactRevision(t *testing.T) {
	for _, tt := range []struct {
		name, rel        string
		parent, existing bool
	}{{"missing-parents", "new/parent/config", false, false}, {"missing-file", "parent/config", true, false}, {"existing-file", "parent/config", true, true}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.parent {
				mustMkdir(t, filepath.Join(root, "parent"), 0o750)
			}
			if tt.existing {
				mustWrite(t, filepath.Join(root, filepath.FromSlash(tt.rel)), "old\n", 0o600)
			}
			expected := missingRevision()
			if tt.parent {
				expected = fileRevisionTest(t, root, tt.rel)
			}
			parents := fileParentChainTest(t, root, tt.rel)
			sourceRoot, source := restoreFileSourceTest(t)
			mustWrite(t, filepath.Join(sourceRoot, "tree", "config"), "poison\n", 0o600)
			if err := os.Chmod(filepath.Join(sourceRoot, "tree", "config"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := newRestoreSessionTest(t, root).RestoreFile(tt.rel, parents, expected, source, "config")
			if err != nil {
				t.Fatal(err)
			}
			data, observed, err := ReadWithin(root, tt.rel)
			if err != nil || string(data) != "captured\n" || got != observed || !got.Exists() || got.Permissions() != 0o640 {
				t.Fatalf("installed data=%q revision=%#v observed=%#v err=%v", data, got, observed, err)
			}
			if !tt.parent {
				assertRestoreDirTest(t, filepath.Join(root, "new"), 0o700)
				assertRestoreDirTest(t, filepath.Join(root, "new", "parent"), 0o700)
			}
		})
	}
}
func TestRestoreSessionRestoreFileRejectsEveryInvalidSourceBeforeMutation(t *testing.T) {
	sourceRoot, valid := restoreFileSourceTest(t)
	rootOnly, _, err := CaptureDirectoryRootWithin(sourceRoot, "tree")
	if err != nil {
		t.Fatal(err)
	}
	corrupt := *valid
	corrupt.digest[0] ^= 0xff
	cases := []struct {
		name, rel string
		source    *DirectorySnapshot
		want      error
	}{{"nil", "config", nil, ErrInvalidAuthority}, {"untracked", "config", &DirectorySnapshot{}, ErrInvalidAuthority},
		{"root-only", "config", rootOnly, ErrInvalidAuthority}, {"corrupt", "config", &corrupt, ErrInvalidAuthority},
		{"missing", "missing", valid, fs.ErrNotExist}, {"directory", "nested", valid, ErrInvalidPath}, {"cross-file", "config/child", valid, ErrInvalidPath}}
	for index, rel := range []string{"", ".", "./config", "config/", "nested//value", "nested/./value", "../config", "nested/../config", "config\x00bad", filepath.Join(sourceRoot, "absolute")} {
		cases = append(cases, struct {
			name, rel string
			source    *DirectorySnapshot
			want      error
		}{fmt.Sprintf("path-%02d", index), rel, valid, ErrInvalidPath})
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root, rel := t.TempDir(), "new/nested/config"
			parents := fileParentChainTest(t, root, rel)
			hooks := 0
			setDirectoryHooks(t, func(h *directoryHooks) {
				h.beforeShallowMkdir = func(_ int, _ string) error { hooks++; return nil }
				h.afterShallowMkdir = func(_, _ int, _ string) error { hooks++; return nil }
			})
			setReplaceHooks(t, func(h *replaceHooks) {
				h.beforeCommit = func(_, _ int, _, _ string) error { hooks++; return nil }
			})
			_, err := newRestoreSessionTest(t, root).RestoreFile(rel, parents, missingRevision(), tt.source, tt.rel)
			wantRestoreErrorTest(t, err, tt.want)
			if hooks != 0 {
				t.Fatalf("invalid source reached %d mutation hooks", hooks)
			}
			assertDirectoryEntries(t, root, []string{})
		})
	}
}
func TestRestoreSessionFileAuthorityInputsAreExact(t *testing.T) {
	root, rel := t.TempDir(), "parent/config"
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	mustWrite(t, filepath.Join(root, "parent", "config"), "old\n", 0o600)
	parents, expected := fileParentChainTest(t, root, rel), fileRevisionTest(t, root, rel)
	_, source := restoreFileSourceTest(t)
	session := newRestoreSessionTest(t, root)
	check := func(name string, candidate *RestoreSession, path string, parent *ParentChain, revision Revision, want error) {
		t.Run(name, func(t *testing.T) {
			_, err := candidate.RestoreFile(path, parent, revision, source, "config")
			wantRestoreErrorTest(t, err, want)
			wantRestoreErrorTest(t, candidate.RemoveFile(path, parent, revision), want)
		})
	}
	check("nil-session", nil, rel, parents, expected, ErrInvalidAuthority)
	check("zero-session", &RestoreSession{}, rel, parents, expected, ErrInvalidAuthority)
	for index, path := range []string{"", ".", "/absolute", "..", "../outside", "nested/../outside", "nested//config"} {
		check(fmt.Sprintf("path-%02d", index), session, path, parents, expected, ErrInvalidPath)
	}
	check("nil-parent", session, rel, nil, expected, ErrParentChanged)
	check("zero-parent", session, rel, &ParentChain{}, expected, ErrParentChanged)
	other := t.TempDir()
	mustMkdir(t, filepath.Join(other, "parent"), 0o700)
	check("foreign-parent", session, rel, fileParentChainTest(t, other, rel), expected, ErrParentChanged)
	check("wrong-path-parent", session, rel, fileParentChainTest(t, root, "other/config"), expected, ErrParentChanged)
	check("zero-revision", session, rel, parents, Revision{}, ErrRevisionChanged)
	wantFileRevisionTest(t, root, rel, expected)
}
func TestRestoreSessionFileRejectsRootAndParentSwaps(t *testing.T) {
	_, source := restoreFileSourceTest(t)
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
			mustWrite(t, filepath.Join(root, "parent", "config"), "old\n", 0o600)
			parents, expected := fileParentChainTest(t, root, "parent/config"), fileRevisionTest(t, root, "parent/config")
			session := newRestoreSessionTest(t, root)
			old := filepath.Join(base, "old-"+kind)
			moved := root
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
			mustWrite(t, filepath.Join(root, "parent", "config"), "replacement\n", 0o600)
			rootState, parentState := directoryRootStateTest(t, filepath.Dir(root), filepath.Base(root)), directoryRootStateTest(t, root, "parent")
			replacement := fileRevisionTest(t, root, "parent/config")
			_, err := session.RestoreFile("parent/config", parents, expected, source, "config")
			want := ErrParentChanged
			if kind == "root" {
				want = ErrDirectoryChanged
			}
			wantRestoreErrorTest(t, err, want)
			wantRestoreErrorTest(t, session.RemoveFile("parent/config", parents, expected), want)
			wantDirectoryRootStateTest(t, filepath.Dir(root), filepath.Base(root), rootState)
			wantDirectoryRootStateTest(t, root, "parent", parentState)
			wantFileRevisionTest(t, root, "parent/config", replacement)
			wantFileRevisionTest(t, old, []string{"parent/config", "config"}[index], expected)
		})
	}
}
func TestRestoreSessionRestoreFilePreservesRejectedTargetDrift(t *testing.T) {
	for _, kind := range []string{"disappearance", "appearance", "same-inode", "identity", "type", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, rel := t.TempDir(), "parent/config"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			target := filepath.Join(root, filepath.FromSlash(rel))
			if kind != "appearance" {
				mustWrite(t, target, "old\n", 0o600)
			}
			expected, parents := fileRevisionTest(t, root, rel), fileParentChainTest(t, root, rel)
			_, source := restoreFileSourceTest(t)
			var preserved Revision
			var directory *DirectorySnapshot
			var link symlinkStateTest
			switch kind {
			case "disappearance":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			case "appearance":
				mustWrite(t, target, "appeared\n", 0o604)
				preserved = fileRevisionTest(t, root, rel)
			case "same-inode":
				mustWrite(t, target, "edited-in-place\n", 0o600)
				preserved = fileRevisionTest(t, root, rel)
			case "identity":
				if err := os.Rename(target, filepath.Join(root, "parent", "accepted-old")); err != nil {
					t.Fatal(err)
				}
				mustWrite(t, target, "old\n", 0o600)
				preserved = fileRevisionTest(t, root, rel)
			case "type":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				mustMkdir(t, target, 0o700)
				directory = directoryRootStateTest(t, filepath.Dir(target), filepath.Base(target))
			case "symlink":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
				link = symlinkLstatTest(t, target)
			}
			_, err := newRestoreSessionTest(t, root).RestoreFile(rel, parents, expected, source, "config")
			want := ErrRevisionChanged
			if kind == "type" {
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
				wantFileRevisionTest(t, root, rel, preserved)
				if kind == "identity" {
					wantFileRevisionTest(t, root, "parent/accepted-old", expected)
				}
			case "type":
				wantDirectoryRootStateTest(t, filepath.Dir(target), filepath.Base(target), directory)
			case "symlink":
				wantSymlinkLstatTest(t, target, link)
			}
		})
	}
}
func TestRestoreSessionRestoreFileRollbackAndCommittedErrors(t *testing.T) {
	for _, kind := range []string{"rollback", "recovery", "committed"} {
		t.Run(kind, func(t *testing.T) {
			root, rel := t.TempDir(), "new/nested/config"
			parents, injected := fileParentChainTest(t, root, rel), errors.New("injected "+kind)
			_, source := restoreFileSourceTest(t)
			setReplaceHooks(t, func(h *replaceHooks) {
				if kind == "committed" {
					h.afterCommit = func(_, _ int, _ string) error { return injected }
					return
				}
				h.beforeCommit = func(_, _ int, _, _ string) error {
					if kind == "recovery" {
						mustWrite(t, filepath.Join(root, "new", "nested", "external"), "preserve\n", 0o600)
					}
					return injected
				}
			})
			got, err := newRestoreSessionTest(t, root).RestoreFile(rel, parents, missingRevision(), source, "config")
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
				if !errors.As(err, &recovery) || errors.As(err, &committed) || !errors.Is(err, ErrDirectoryChanged) {
					t.Fatalf("error = %v, want uncommitted RecoveryError", err)
				}
				assertRestoreMissingTest(t, filepath.Join(root, filepath.FromSlash(rel)))
				assertContent(t, filepath.Join(root, "new", "nested", "external"), "preserve\n")
			case "committed":
				if !errors.As(err, &committed) || got.Tracked() {
					t.Fatalf("error/revision = %v/%#v, want CommittedError/no evidence", err, got)
				}
				assertContent(t, filepath.Join(root, filepath.FromSlash(rel)), "captured\n")
			}
		})
	}
}
func TestRestoreSessionRemoveFileExactStaleAndCommitBoundaries(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		root, rel := t.TempDir(), "parent/config"
		mustMkdir(t, filepath.Join(root, "parent"), 0o700)
		mustWrite(t, filepath.Join(root, filepath.FromSlash(rel)), "old\n", 0o600)
		err := newRestoreSessionTest(t, root).RemoveFile(rel, fileParentChainTest(t, root, rel), fileRevisionTest(t, root, rel))
		if err != nil {
			t.Fatal(err)
		}
		assertRestoreMissingTest(t, filepath.Join(root, filepath.FromSlash(rel)))
	})
	t.Run("missing-parent", func(t *testing.T) {
		root, rel := t.TempDir(), "missing/nested/config"
		wantRestoreErrorTest(t, newRestoreSessionTest(t, root).RemoveFile(rel, fileParentChainTest(t, root, rel), missingRevision()), ErrRevisionChanged)
		assertDirectoryEntries(t, root, []string{})
	})
	for _, kind := range []string{"same-inode", "replacement", "symlink"} {
		t.Run("stale-"+kind, func(t *testing.T) {
			root, rel := t.TempDir(), "parent/config"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			target := filepath.Join(root, filepath.FromSlash(rel))
			mustWrite(t, target, "accepted\n", 0o600)
			parents, accepted := fileParentChainTest(t, root, rel), fileRevisionTest(t, root, rel)
			var preserved Revision
			var link symlinkStateTest
			switch kind {
			case "same-inode":
				mustWrite(t, target, "edited\n", 0o600)
				if err := os.Chmod(target, 0o640); err != nil {
					t.Fatal(err)
				}
				preserved = fileRevisionTest(t, root, rel)
			case "replacement":
				if err := os.Rename(target, filepath.Join(root, "parent", "accepted-old")); err != nil {
					t.Fatal(err)
				}
				mustWrite(t, target, "accepted\n", 0o600)
				preserved = fileRevisionTest(t, root, rel)
			case "symlink":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
				link = symlinkLstatTest(t, target)
			}
			want := ErrRevisionChanged
			if kind == "symlink" {
				want = ErrSymlink
			}
			wantRestoreErrorTest(t, newRestoreSessionTest(t, root).RemoveFile(rel, parents, accepted), want)
			if kind == "symlink" {
				wantSymlinkLstatTest(t, target, link)
			} else {
				wantFileRevisionTest(t, root, rel, preserved)
			}
			if kind == "replacement" {
				wantFileRevisionTest(t, root, "parent/accepted-old", accepted)
			}
		})
	}
	for _, kind := range []string{"pre-error", "pre-drift", "post-error", "post-appearance"} {
		t.Run(kind, func(t *testing.T) {
			root, rel := t.TempDir(), "parent/config"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			target := filepath.Join(root, filepath.FromSlash(rel))
			mustWrite(t, target, "accepted\n", 0o600)
			parents, accepted, injected := fileParentChainTest(t, root, rel), fileRevisionTest(t, root, rel), errors.New("injected "+kind)
			var appeared Revision
			setReplaceHooks(t, func(h *replaceHooks) {
				switch kind {
				case "pre-error":
					h.beforeRemove = func(_, _ int, _ string) error { return injected }
				case "pre-drift":
					h.beforeRemove = func(_, _ int, _ string) error {
						if err := os.Rename(target, filepath.Join(root, "parent", "accepted-old")); err != nil {
							return err
						}
						if err := os.WriteFile(target, []byte("replacement\n"), 0o604); err != nil {
							return err
						}
						appeared = fileRevisionTest(t, root, rel)
						return nil
					}
				case "post-error":
					h.afterRemove = func(_, _ int, _ string) error { return injected }
				case "post-appearance":
					h.afterRemove = func(_, _ int, _ string) error {
						if err := os.WriteFile(target, []byte("replacement\n"), 0o604); err != nil {
							return err
						}
						appeared = fileRevisionTest(t, root, rel)
						return nil
					}
				}
			})
			err := newRestoreSessionTest(t, root).RemoveFile(rel, parents, accepted)
			var committed *CommittedError
			switch kind {
			case "pre-error":
				if !errors.Is(err, injected) || errors.As(err, &committed) {
					t.Fatalf("error = %v, want uncommitted injected", err)
				}
				wantFileRevisionTest(t, root, rel, accepted)
			case "pre-drift":
				wantRestoreErrorTest(t, err, ErrTargetChanged)
				if errors.As(err, &committed) {
					t.Fatalf("pre-remove drift committed: %v", err)
				}
				wantFileRevisionTest(t, root, "parent/accepted-old", accepted)
				wantFileRevisionTest(t, root, rel, appeared)
			case "post-error":
				if !errors.As(err, &committed) || !errors.Is(err, injected) {
					t.Fatalf("error = %v, want CommittedError/injected", err)
				}
				assertRestoreMissingTest(t, target)
			case "post-appearance":
				if !errors.As(err, &committed) || !errors.Is(err, ErrTargetChanged) {
					t.Fatalf("error = %v, want CommittedError/ErrTargetChanged", err)
				}
				wantFileRevisionTest(t, root, rel, appeared)
			}
		})
	}
}
