//go:build darwin || linux

package safefile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func stagedModeTest(t *testing.T, fd int) fs.FileMode {
	t.Helper()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		t.Fatal(err)
	}
	return fs.FileMode(stat.Mode).Perm()
}

func TestRestoreSessionRestoreFileWithModeIsOneAtomicExactRevision(t *testing.T) {
	for _, tt := range []struct {
		name, rel        string
		parent, existing bool
		desired          fs.FileMode
	}{
		{"missing-parents", "new/parent/config", false, false, 0o600},
		{"missing-file", "parent/config", true, false, 0o644},
		{"existing-file", "parent/config", true, true, 0o755},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.parent {
				mustMkdir(t, filepath.Join(root, "parent"), 0o750)
			}
			target := filepath.Join(root, filepath.FromSlash(tt.rel))
			if tt.existing {
				mustWrite(t, target, "old\n", 0o600)
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

			before, after := 0, 0
			setReplaceHooks(t, func(h *replaceHooks) {
				h.beforeCommit = func(_, stagedFD int, _, _ string) error {
					before++
					if got := stagedModeTest(t, stagedFD); got != tt.desired {
						t.Fatalf("staged mode before rename = %04o, want %04o", got, tt.desired)
					}
					if tt.existing {
						assertContent(t, target, "old\n")
						assertMode(t, target, 0o600)
					} else {
						assertRestoreMissingTest(t, target)
					}
					return nil
				}
				h.afterCommit = func(_, stagedFD int, _ string) error {
					after++
					if got := stagedModeTest(t, stagedFD); got != tt.desired {
						t.Fatalf("committed descriptor mode = %04o, want %04o", got, tt.desired)
					}
					assertContent(t, target, "captured\n")
					assertMode(t, target, tt.desired)
					return nil
				}
			})
			got, err := newRestoreSessionTest(t, root).RestoreFileWithMode(tt.rel, parents, expected, source, "config", tt.desired)
			if err != nil {
				t.Fatal(err)
			}
			data, observed, err := ReadWithin(root, tt.rel)
			if err != nil || string(data) != "captured\n" || got != observed || !got.Exists() || got.Permissions() != tt.desired {
				t.Fatalf("installed data=%q revision=%#v observed=%#v err=%v", data, got, observed, err)
			}
			if before != 1 || after != 1 {
				t.Fatalf("atomic boundaries before/after = %d/%d, want 1/1", before, after)
			}
			if !tt.parent {
				assertRestoreDirTest(t, filepath.Join(root, "new"), 0o700)
				assertRestoreDirTest(t, filepath.Join(root, "new", "parent"), 0o700)
			}
		})
	}
}

func TestRestoreSessionRestoreFileWithModeRejectsInvalidInputsBeforeMutation(t *testing.T) {
	_, valid := restoreFileSourceTest(t)
	invalidModes := []fs.FileMode{
		0o1000,
		fs.ModeDir | 0o600,
		fs.ModeSymlink | 0o600,
		fs.ModeSetuid | 0o600,
		fs.ModeSetgid | 0o600,
		fs.ModeSticky | 0o600,
		fs.ModeType | 0o600,
	}
	for _, mode := range invalidModes {
		t.Run(mode.String(), func(t *testing.T) {
			root, rel := t.TempDir(), "new/nested/config"
			parents := fileParentChainTest(t, root, rel)
			hooks := 0
			setDirectoryHooks(t, func(h *directoryHooks) {
				h.beforeShallowMkdir = func(_ int, _ string) error { hooks++; return nil }
			})
			setReplaceHooks(t, func(h *replaceHooks) {
				h.beforeCommit = func(_, _ int, _, _ string) error { hooks++; return nil }
			})
			got, err := newRestoreSessionTest(t, root).RestoreFileWithMode(rel, parents, missingRevision(), valid, "config", mode)
			wantRestoreErrorTest(t, err, ErrInvalidMode)
			if got.Tracked() || hooks != 0 {
				t.Fatalf("invalid mode returned evidence/reached mutation: %#v hooks=%d", got, hooks)
			}
			assertDirectoryEntries(t, root, []string{})
		})
	}

	for _, tt := range []struct {
		name     string
		source   *DirectorySnapshot
		expected Revision
		mode     fs.FileMode
		want     error
	}{
		{"source-before-mode", nil, missingRevision(), fs.ModeSetuid | 0o600, ErrInvalidAuthority},
		{"mode-before-revision", valid, Revision{}, fs.ModeSetuid | 0o600, ErrInvalidMode},
		{"revision-after-mode", valid, Revision{}, 0o600, ErrRevisionChanged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, rel := t.TempDir(), "new/nested/config"
			got, err := newRestoreSessionTest(t, root).RestoreFileWithMode(rel, fileParentChainTest(t, root, rel), tt.expected, tt.source, "config", tt.mode)
			wantRestoreErrorTest(t, err, tt.want)
			if got.Tracked() {
				t.Fatalf("rejected input returned evidence: %#v", got)
			}
			assertDirectoryEntries(t, root, []string{})
		})
	}
}

func TestRestoreSessionRestoreFileWithModeAcceptsOrdinaryModeBoundaries(t *testing.T) {
	for _, mode := range []fs.FileMode{0o000, 0o777} {
		t.Run(mode.String(), func(t *testing.T) {
			root, rel := t.TempDir(), "parent/config"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			parents := fileParentChainTest(t, root, rel)
			_, source := restoreFileSourceTest(t)
			got, err := newRestoreSessionTest(t, root).RestoreFileWithMode(rel, parents, missingRevision(), source, "config", mode)
			if errors.Is(err, ErrInvalidMode) {
				t.Fatalf("ordinary mode %04o rejected: %v", mode, err)
			}
			if mode != 0 && err != nil {
				t.Fatalf("readable ordinary mode %04o failed: %v", mode, err)
			}
			if err == nil {
				if !got.Tracked() || !got.Exists() || got.Permissions() != mode {
					t.Fatalf("successful boundary mode evidence = %#v, want %04o", got, mode)
				}
			} else {
				var committed *CommittedError
				if !errors.As(err, &committed) || got.Tracked() {
					t.Fatalf("boundary mode result = %#v/%v, want committed uncertainty without evidence", got, err)
				}
			}
			assertMode(t, filepath.Join(root, filepath.FromSlash(rel)), mode)
		})
	}
}

func TestRestoreSessionRestoreFileWithModePreservesExactAuthority(t *testing.T) {
	_, source := restoreFileSourceTest(t)
	root, rel := t.TempDir(), "parent/config"
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	mustWrite(t, filepath.Join(root, filepath.FromSlash(rel)), "accepted\n", 0o600)
	parents, expected := fileParentChainTest(t, root, rel), fileRevisionTest(t, root, rel)
	session := newRestoreSessionTest(t, root)

	for _, tt := range []struct {
		name      string
		candidate *RestoreSession
		parents   *ParentChain
		expected  Revision
		want      error
	}{
		{"nil-session", nil, parents, expected, ErrInvalidAuthority},
		{"zero-session", &RestoreSession{}, parents, expected, ErrInvalidAuthority},
		{"nil-parent", session, nil, expected, ErrParentChanged},
		{"zero-parent", session, &ParentChain{}, expected, ErrParentChanged},
		{"zero-revision", session, parents, Revision{}, ErrRevisionChanged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.candidate.RestoreFileWithMode(rel, tt.parents, tt.expected, source, "config", 0o644)
			wantRestoreErrorTest(t, err, tt.want)
			if got.Tracked() {
				t.Fatalf("rejected authority returned evidence: %#v", got)
			}
			wantFileRevisionTest(t, root, rel, expected)
		})
	}

	t.Run("same-inode-drift", func(t *testing.T) {
		mustWrite(t, filepath.Join(root, filepath.FromSlash(rel)), "edited\n", 0o600)
		preserved := fileRevisionTest(t, root, rel)
		got, err := session.RestoreFileWithMode(rel, parents, expected, source, "config", 0o644)
		wantRestoreErrorTest(t, err, ErrRevisionChanged)
		if got.Tracked() {
			t.Fatalf("drift returned evidence: %#v", got)
		}
		wantFileRevisionTest(t, root, rel, preserved)
	})

	t.Run("foreign-parent", func(t *testing.T) {
		other := t.TempDir()
		mustMkdir(t, filepath.Join(other, "parent"), 0o700)
		preserved := fileRevisionTest(t, root, rel)
		got, err := session.RestoreFileWithMode(rel, fileParentChainTest(t, other, rel), preserved, source, "config", 0o644)
		wantRestoreErrorTest(t, err, ErrParentChanged)
		if got.Tracked() {
			t.Fatalf("foreign parent returned evidence: %#v", got)
		}
		wantFileRevisionTest(t, root, rel, preserved)
	})
}

func TestRestoreSessionRestoreFileWithModeRejectsRootAndParentSwaps(t *testing.T) {
	_, source := restoreFileSourceTest(t)
	for _, kind := range []string{"root", "parent"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			root := base
			if kind == "root" {
				root = filepath.Join(base, "root")
				mustMkdir(t, root, 0o700)
			}
			rel := "parent/config"
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			mustWrite(t, filepath.Join(root, filepath.FromSlash(rel)), "accepted\n", 0o600)
			parents, accepted := fileParentChainTest(t, root, rel), fileRevisionTest(t, root, rel)
			session := newRestoreSessionTest(t, root)

			moved := root
			oldRel := rel
			if kind == "parent" {
				moved = filepath.Join(root, "parent")
				oldRel = "config"
			}
			old := filepath.Join(base, "accepted-old-"+kind)
			if err := os.Rename(moved, old); err != nil {
				t.Fatal(err)
			}
			if kind == "root" {
				mustMkdir(t, root, 0o700)
			}
			mustMkdir(t, filepath.Join(root, "parent"), 0o700)
			mustWrite(t, filepath.Join(root, filepath.FromSlash(rel)), "replacement\n", 0o600)
			replacement := fileRevisionTest(t, root, rel)

			got, err := session.RestoreFileWithMode(rel, parents, accepted, source, "config", 0o644)
			want := ErrParentChanged
			if kind == "root" {
				want = ErrDirectoryChanged
			}
			wantRestoreErrorTest(t, err, want)
			if got.Tracked() {
				t.Fatalf("swapped %s returned evidence: %#v", kind, got)
			}
			wantFileRevisionTest(t, root, rel, replacement)
			wantFileRevisionTest(t, old, oldRel, accepted)
		})
	}
}

func TestRestoreSessionRestoreFileWithModeRollbackAndCommitSemantics(t *testing.T) {
	for _, kind := range []string{"rollback", "recovery", "committed"} {
		t.Run(kind, func(t *testing.T) {
			root, rel := t.TempDir(), "new/nested/config"
			parents := fileParentChainTest(t, root, rel)
			_, source := restoreFileSourceTest(t)
			injected := errors.New("injected " + kind)
			setReplaceHooks(t, func(h *replaceHooks) {
				if kind == "committed" {
					h.afterCommit = func(_, stagedFD int, _ string) error {
						if got := stagedModeTest(t, stagedFD); got != 0o755 {
							t.Fatalf("committed mode = %04o, want 0755", got)
						}
						return injected
					}
					return
				}
				h.beforeCommit = func(_, stagedFD int, _, _ string) error {
					if got := stagedModeTest(t, stagedFD); got != 0o755 {
						t.Fatalf("precommit mode = %04o, want 0755", got)
					}
					if kind == "recovery" {
						mustWrite(t, filepath.Join(root, "new", "nested", "external"), "preserve\n", 0o600)
					}
					return injected
				}
			})
			got, err := newRestoreSessionTest(t, root).RestoreFileWithMode(rel, parents, missingRevision(), source, "config", 0o755)
			wantRestoreErrorTest(t, err, injected)
			var recovery *RecoveryError
			var committed *CommittedError
			switch kind {
			case "rollback":
				if got.Tracked() || errors.As(err, &recovery) || errors.As(err, &committed) {
					t.Fatalf("rollback result = %#v/%v", got, err)
				}
				assertRestoreMissingTest(t, filepath.Join(root, "new"))
			case "recovery":
				if got.Tracked() || !errors.As(err, &recovery) || errors.As(err, &committed) || !errors.Is(err, ErrDirectoryChanged) {
					t.Fatalf("recovery result = %#v/%v", got, err)
				}
				assertRestoreMissingTest(t, filepath.Join(root, filepath.FromSlash(rel)))
				assertContent(t, filepath.Join(root, "new", "nested", "external"), "preserve\n")
			case "committed":
				if got.Tracked() || !errors.As(err, &committed) {
					t.Fatalf("committed result = %#v/%v", got, err)
				}
				assertContent(t, filepath.Join(root, filepath.FromSlash(rel)), "captured\n")
				assertMode(t, filepath.Join(root, filepath.FromSlash(rel)), 0o755)
			}
		})
	}
}
