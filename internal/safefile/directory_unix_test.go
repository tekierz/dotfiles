//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDirectorySnapshotRestoreReplacesTreeWithExactModes(t *testing.T) {
	workspace := t.TempDir()
	sourceRoot := filepath.Join(workspace, "backups")
	source := filepath.Join(sourceRoot, "selected", "nvim")
	mustMkdir(t, sourceRoot, 0o700)
	mustMkdir(t, filepath.Join(sourceRoot, "selected"), 0o700)
	mustMkdir(t, source, 0o750)
	mustMkdir(t, filepath.Join(source, "lua"), 0o710)
	mustWrite(t, filepath.Join(source, "init.lua"), "original init\n", 0o640)
	mustWrite(t, filepath.Join(source, "lua", "plugin.lua"), "original plugin\n", 0o600)

	snapshot, err := SnapshotDirectoryWithin(sourceRoot, "selected/nvim")
	if err != nil {
		t.Fatalf("SnapshotDirectoryWithin: %v", err)
	}
	home := filepath.Join(workspace, "home")
	mustMkdir(t, home, 0o700)
	mustMkdir(t, filepath.Join(home, ".config"), 0o700)
	live := filepath.Join(home, ".config", "nvim")
	mustMkdir(t, live, 0o755)
	mustWrite(t, filepath.Join(live, "stale.lua"), "stale\n", 0o644)

	if err := RestoreDirectoryWithin(home, ".config/nvim", snapshot); err != nil {
		t.Fatalf("RestoreDirectoryWithin: %v", err)
	}
	assertDirectoryEntries(t, live, []string{"init.lua", "lua"})
	assertDirectoryEntries(t, filepath.Join(live, "lua"), []string{"plugin.lua"})
	assertContent(t, filepath.Join(live, "init.lua"), "original init\n")
	assertContent(t, filepath.Join(live, "lua", "plugin.lua"), "original plugin\n")
	assertMode(t, live, 0o750)
	assertMode(t, filepath.Join(live, "lua"), 0o710)
	assertMode(t, filepath.Join(live, "init.lua"), 0o640)
	assertMode(t, filepath.Join(live, "lua", "plugin.lua"), 0o600)
	assertNoSafefileDirectoryArtifacts(t, filepath.Join(home, ".config"))
}

func TestDirectorySnapshotDigestIsStableAndObservesRecursiveChanges(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mustMkdir(t, tree, 0o750)
	mustMkdir(t, filepath.Join(tree, "nested"), 0o710)
	target := filepath.Join(tree, "nested", "config")
	mustWrite(t, target, "value-a\n", 0o640)

	first, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	second, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() == ([32]byte{}) || first.Digest() != second.Digest() {
		t.Fatalf("stable snapshots produced zero/different digests: %x / %x", first.Digest(), second.Digest())
	}
	if first.Permissions() != 0o750 {
		t.Fatalf("snapshot root permissions = %04o, want 0750", first.Permissions())
	}

	digests := map[[32]byte]string{first.Digest(): "original"}
	observeChange := func(label string) {
		t.Helper()
		snapshot, err := SnapshotDirectoryWithin(root, "tree")
		if err != nil {
			t.Fatal(err)
		}
		if previous, duplicate := digests[snapshot.Digest()]; duplicate {
			t.Fatalf("%s change reused %s digest %x", label, previous, snapshot.Digest())
		}
		digests[snapshot.Digest()] = label
	}
	if err := os.WriteFile(target, []byte("value-b\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	observeChange("content")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	observeChange("file-mode")
	if err := os.Chmod(tree, 0o700); err != nil {
		t.Fatal(err)
	}
	observeChange("root-mode")
	if err := os.Rename(target, filepath.Join(tree, "nested", "renamed")); err != nil {
		t.Fatal(err)
	}
	observeChange("name")

	if first.Digest() != second.Digest() || first.Permissions() != 0o750 {
		t.Fatal("previously returned immutable snapshot observation changed after live mutations")
	}
}

func TestDirectorySnapshotExtractsImmutableFilesAndSubtrees(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "tree"), 0o750)
	mustWrite(t, filepath.Join(root, "tree", "config"), "original\n", 0o640)
	mustMkdir(t, filepath.Join(root, "tree", "nested"), 0o710)
	mustWrite(t, filepath.Join(root, "tree", "nested", "value"), "nested\n", 0o604)
	mustMkdir(t, filepath.Join(root, "tree", "nested", "empty"), 0o730)
	if err := os.Chmod(filepath.Join(root, "tree", "nested", "empty"), 0o730); err != nil {
		t.Fatal(err)
	}
	snapshot, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "tree", "config"), "live edit\n", 0o600)
	mustWrite(t, filepath.Join(root, "tree", "nested", "value"), "live nested edit\n", 0o600)
	for _, path := range []string{filepath.Join(root, "tree", "config"), filepath.Join(root, "tree", "nested", "value")} {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(root, "tree", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "tree", "nested", "empty")); err != nil {
		t.Fatal(err)
	}

	data, mode, err := ReadDirectorySnapshotFile(snapshot, "config")
	if err != nil || string(data) != "original\n" || mode != 0o640 {
		t.Fatalf("snapshot file = %q mode=%04o err=%v", data, mode, err)
	}
	data[0] = 'X'
	again, _, err := ReadDirectorySnapshotFile(snapshot, "config")
	if err != nil || string(again) != "original\n" {
		t.Fatalf("snapshot file changed through returned bytes: %q err=%v", again, err)
	}
	nested, nestedMode, err := ReadDirectorySnapshotFile(snapshot, "nested/value")
	if err != nil || string(nested) != "nested\n" || nestedMode != 0o604 {
		t.Fatalf("nested snapshot file = %q mode=%04o err=%v", nested, nestedMode, err)
	}

	first, err := SubdirectorySnapshot(snapshot, "nested")
	if err != nil || first.Permissions() != 0o710 {
		t.Fatalf("first subtree mode = %04o, err=%v", first.Permissions(), err)
	}
	wantDigest := first.Digest()
	first.root.mode = 0o700
	first.root.entries[0].dir.mode = 0o700
	first.root.entries[0].name = "poison"
	first.root.entries[1].mode = 0o600
	first.root.entries[1].data[0] = 'X'
	first.root.entries = first.root.entries[:1]
	second, err := SubdirectorySnapshot(snapshot, "nested")
	if err != nil || second.Permissions() != 0o710 || second.Digest() != wantDigest || len(second.root.entries) != 2 ||
		second.root.entries[0].name != "empty" || second.root.entries[1].name != "value" {
		t.Fatalf("re-extracted subtree was changed through first copy: %+v err=%v", second, err)
	}
	if _, err := DirectorySnapshotAuthorityDigest(second); !errors.Is(err, ErrInvalidAuthority) {
		t.Fatalf("extracted subtree carried namespace authority: %v", err)
	}
	if err := RestoreDirectoryWithin(root, "restored", second); err != nil {
		t.Fatal(err)
	}
	assertContent(t, filepath.Join(root, "restored", "value"), "nested\n")
	assertMode(t, filepath.Join(root, "restored"), 0o710)
	assertMode(t, filepath.Join(root, "restored", "value"), 0o604)
	assertMode(t, filepath.Join(root, "restored", "empty"), 0o730)
}

func TestDirectorySnapshotExtractionRejectsInvalidDescendants(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "tree"), 0o700)
	mustMkdir(t, filepath.Join(root, "tree", "nested"), 0o700)
	mustWrite(t, filepath.Join(root, "tree", "config"), "value\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	malformed := []string{"", ".", "./config", "config/", "nested//value", "nested/./value", "../config", "nested/../config", "config\x00bad", filepath.Join(root, "absolute")}
	for _, rel := range malformed {
		if _, _, err := ReadDirectorySnapshotFile(snapshot, rel); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("ReadDirectorySnapshotFile(%q) error = %v, want ErrInvalidPath", rel, err)
		}
		if _, err := SubdirectorySnapshot(snapshot, rel); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("SubdirectorySnapshot(%q) error = %v, want ErrInvalidPath", rel, err)
		}
	}
	if _, _, err := ReadDirectorySnapshotFile(snapshot, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file error = %v, want fs.ErrNotExist", err)
	}
	if _, err := SubdirectorySnapshot(snapshot, "missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing directory error = %v, want fs.ErrNotExist", err)
	}
	if _, _, err := ReadDirectorySnapshotFile(snapshot, "nested"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("directory-as-file error = %v, want ErrInvalidPath", err)
	}
	if _, err := SubdirectorySnapshot(snapshot, "config"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("file-as-directory error = %v, want ErrInvalidPath", err)
	}

	rootOnly, _, err := CaptureDirectoryRootWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	corrupt, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	corrupt.digest[0] ^= 0xff
	for name, invalid := range map[string]*DirectorySnapshot{"nil": nil, "untracked": {}, "root-only": rootOnly, "corrupt": corrupt} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ReadDirectorySnapshotFile(invalid, "config"); !errors.Is(err, ErrInvalidAuthority) {
				t.Errorf("file extraction error = %v, want ErrInvalidAuthority", err)
			}
			if _, err := SubdirectorySnapshot(invalid, "nested"); !errors.Is(err, ErrInvalidAuthority) {
				t.Errorf("subtree extraction error = %v, want ErrInvalidAuthority", err)
			}
		})
	}
}

func TestSnapshotDirectoryWithinRefusesNestedSymlinkAndNonRegular(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(t *testing.T, root string)
		want error
	}{
		{
			name: "symlink",
			make: func(t *testing.T, root string) {
				outside := t.TempDir()
				if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
					t.Fatal(err)
				}
			},
			want: ErrSymlink,
		},
		{
			name: "fifo",
			make: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "regular"), []byte("ok"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: ErrNonRegular,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			tree := filepath.Join(root, "tree")
			mustMkdir(t, tree, 0o700)
			test.make(t, tree)
			if _, err := SnapshotDirectoryWithin(root, "tree"); !errors.Is(err, test.want) {
				t.Fatalf("SnapshotDirectoryWithin error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSnapshotDirectoryWithinDetectsRootNamespaceReplacement(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	moved := filepath.Join(root, "moved")
	mustMkdir(t, tree, 0o700)
	mustWrite(t, filepath.Join(tree, "config"), "original\n", 0o600)
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterSnapshotOpen = func(_ int, _ int, _ string) error {
			if err := os.Rename(tree, moved); err != nil {
				return err
			}
			return os.Mkdir(tree, 0o700)
		}
	})
	if _, err := SnapshotDirectoryWithin(root, "tree"); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("namespace replacement error = %v, want ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(moved, "config"), "original\n")
}

func TestVerifyDirectoryWithinSnapshotRejectsIdenticalReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tree")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config"), []byte("same\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := SnapshotDirectoryWithin(root, "tree")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config"), []byte("same\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDirectoryWithinSnapshot(root, "tree", expected); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("VerifyDirectoryWithinSnapshot error = %v, want ErrDirectoryChanged", err)
	}
}

func TestVerifyDirectoryWithinSnapshotTracksAcceptedAbsence(t *testing.T) {
	root := t.TempDir()
	if err := VerifyDirectoryWithinSnapshot(root, "missing/leaf", nil); err != nil {
		t.Fatalf("accepted absence rejected: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "missing", "leaf"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDirectoryWithinSnapshot(root, "missing/leaf", nil); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("created directory error = %v, want ErrDirectoryChanged", err)
	}
}

func TestSnapshotDirectoryWithinDetectsFileReplacementDuringRead(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	target := filepath.Join(tree, "config")
	moved := filepath.Join(tree, "opened-config")
	mustMkdir(t, tree, 0o700)
	mustWrite(t, target, "original\n", 0o600)
	replaced := false
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterSnapshotFileOpen = func(_ int, _ int, _ string) error {
			if replaced {
				return nil
			}
			replaced = true
			if err := os.Rename(target, moved); err != nil {
				return err
			}
			return os.WriteFile(target, []byte("replacement\n"), 0o600)
		}
	})
	if _, err := SnapshotDirectoryWithin(root, "tree"); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("file replacement error = %v, want ErrDirectoryChanged", err)
	}
	assertContent(t, moved, "original\n")
	assertContent(t, target, "replacement\n")
}

func TestRestoreDirectoryWithinRefusesSymlinkInLiveTreeBeforeMutation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	live := filepath.Join(root, "live")
	mustMkdir(t, source, 0o700)
	mustWrite(t, filepath.Join(source, "new"), "new\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, live, 0o700)
	out := filepath.Join(t.TempDir(), "outside")
	mustWrite(t, out, "outside\n", 0o600)
	if err := os.Symlink(out, filepath.Join(live, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := RestoreDirectoryWithin(root, "live", snapshot); !errors.Is(err, ErrSymlink) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want ErrSymlink", err)
	}
	assertContent(t, out, "outside\n")
	if _, err := os.Lstat(filepath.Join(live, "linked")); err != nil {
		t.Fatalf("live symlink was changed: %v", err)
	}
	assertNoSafefileDirectoryArtifacts(t, root)
}

func TestRestoreDirectoryWithinRollsBackMoveAsideFailure(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "backup\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o750)
	mustWrite(t, filepath.Join(root, "live", "config"), "live\n", 0o640)
	wantErr := errors.New("injected preinstall failure")
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterMoveAside = func(_ int, _, _ string) error { return wantErr }
	})
	err = RestoreDirectoryWithin(root, "live", snapshot)
	if !errors.Is(err, wantErr) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want injected error", err)
	}
	var committed *CommittedError
	var recovery *RecoveryError
	if errors.As(err, &committed) || errors.As(err, &recovery) {
		t.Fatalf("successful rollback reported committed/recovery failure: %v", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "live\n")
	assertMode(t, filepath.Join(root, "live"), 0o750)
	assertMode(t, filepath.Join(root, "live", "config"), 0o640)
	assertNoSafefileDirectoryArtifacts(t, root)
}

func TestRestoreDirectoryWithinSnapshotRefusesExternalEditBeforeRollback(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "backup"), 0o700)
	mustWrite(t, filepath.Join(root, "backup", "config"), "backup\n", 0o600)
	backupSnapshot, err := SnapshotDirectoryWithin(root, "backup")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "operation post-state\n", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "live")
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "live", "config"), "external edit\n", 0o600)
	err = RestoreDirectoryWithinSnapshot(root, "live", backupSnapshot, expected)
	if !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RestoreDirectoryWithinSnapshot error = %v, want ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "external edit\n")
}

func TestRestoreDirectoryWithinSnapshotRefusesIdenticalReplacementDirectory(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "backup"), 0o700)
	mustWrite(t, filepath.Join(root, "backup", "config"), "backup\n", 0o600)
	backupSnapshot, err := SnapshotDirectoryWithin(root, "backup")
	if err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(root, "live")
	moved := filepath.Join(root, "original-live")
	mustMkdir(t, live, 0o700)
	mustWrite(t, filepath.Join(live, "config"), "operation post-state\n", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "live")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(live, moved); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, live, 0o700)
	mustWrite(t, filepath.Join(live, "config"), "operation post-state\n", 0o600)
	replacement, err := SnapshotDirectoryWithin(root, "live")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Digest() != expected.Digest() {
		t.Fatal("test replacement does not have identical recursive content and modes")
	}

	err = RestoreDirectoryWithinSnapshot(root, "live", backupSnapshot, expected)
	if !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RestoreDirectoryWithinSnapshot error = %v, want identity-bound ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(live, "config"), "operation post-state\n")
}

func TestRestoreDirectoryWithinSnapshotTrackedRefusesLateIdenticalReplacement(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "installed\n", 0o600)
	source, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.beforeRestoreEvidence = func(_ int, target string) error {
			live := filepath.Join(root, target)
			if err := os.Rename(live, filepath.Join(root, "superseded-live")); err != nil {
				return err
			}
			if err := os.Mkdir(live, 0o700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(live, "config"), []byte("installed\n"), 0o600)
		}
	})

	evidence, err := RestoreDirectoryWithinSnapshotTracked(root, "live", source, nil)
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RestoreDirectoryWithinSnapshotTracked error = %v, want committed identity drift", err)
	}
	if evidence != nil {
		t.Fatalf("superseded directory returned rollback evidence: %+v", evidence)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "installed\n")
}

func TestRemoveDirectoryWithinSnapshotRefusesIdenticalReplacementDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "created")
	moved := filepath.Join(root, "original-created")
	mustMkdir(t, target, 0o700)
	mustWrite(t, filepath.Join(target, "config"), "operation post-state\n", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "created")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target, moved); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, target, 0o700)
	mustWrite(t, filepath.Join(target, "config"), "operation post-state\n", 0o600)
	replacement, err := SnapshotDirectoryWithin(root, "created")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Digest() != expected.Digest() {
		t.Fatal("test replacement does not have identical recursive content and modes")
	}

	err = RemoveDirectoryWithinSnapshot(root, "created", expected)
	if !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RemoveDirectoryWithinSnapshot error = %v, want identity-bound ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(target, "config"), "operation post-state\n")
}

func TestRestoreDirectoryWithinDetectsExternalEditAfterMoveAside(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "backup"), 0o700)
	mustWrite(t, filepath.Join(root, "backup", "config"), "backup\n", 0o600)
	backupSnapshot, err := SnapshotDirectoryWithin(root, "backup")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "operation post-state\n", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "live")
	if err != nil {
		t.Fatal(err)
	}
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterMoveAside = func(_ int, _, recovery string) error {
			return os.WriteFile(filepath.Join(root, recovery, "config"), []byte("external edit after staging\n"), 0o600)
		}
	})
	err = RestoreDirectoryWithinSnapshot(root, "live", backupSnapshot, expected)
	if !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RestoreDirectoryWithinSnapshot error = %v, want ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "external edit after staging\n")
}

func TestRemoveDirectoryWithinSnapshotDetectsEditAfterMoveAside(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "created"), 0o700)
	mustWrite(t, filepath.Join(root, "created", "config"), "operation post-state\n", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "created")
	if err != nil {
		t.Fatal(err)
	}
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterRemoveMove = func(_ int, _, recovery string) error {
			return os.WriteFile(filepath.Join(root, recovery, "config"), []byte("external edit after staging\n"), 0o600)
		}
	})
	err = RemoveDirectoryWithinSnapshot(root, "created", expected)
	if !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RemoveDirectoryWithinSnapshot error = %v, want ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(root, "created", "config"), "external edit after staging\n")
}

func TestRestoreDirectoryWithinPreservesRecoveryEditedAfterInstall(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "backup"), 0o700)
	mustWrite(t, filepath.Join(root, "backup", "config"), "backup\n", 0o600)
	backupSnapshot, err := SnapshotDirectoryWithin(root, "backup")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "operation post-state\n", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "live")
	if err != nil {
		t.Fatal(err)
	}
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterInstall = func(_ int, _ int, _ string) error {
			recoveries := safefileArtifacts(t, root, ".safefile-recovery-")
			if len(recoveries) != 1 {
				return fmt.Errorf("recovery artifacts = %v", recoveries)
			}
			return os.WriteFile(filepath.Join(root, recoveries[0], "config"), []byte("external recovery edit\n"), 0o600)
		}
	})
	err = RestoreDirectoryWithinSnapshot(root, "live", backupSnapshot, expected)
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RestoreDirectoryWithinSnapshot error = %v, want committed directory drift", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "backup\n")
	recoveries := safefileArtifacts(t, root, ".safefile-recovery-")
	if len(recoveries) != 1 {
		t.Fatalf("edited recovery artifacts = %v, want preserved recovery", recoveries)
	}
	assertContent(t, filepath.Join(root, recoveries[0], "config"), "external recovery edit\n")
}

func TestRemoveDirectoryWithinPreservesRecoveryEditedAfterCommit(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "created"), 0o700)
	mustWrite(t, filepath.Join(root, "created", "config"), "operation post-state\n", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "created")
	if err != nil {
		t.Fatal(err)
	}
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterRemoveCommit = func(_ int, recovery string) error {
			return os.WriteFile(filepath.Join(root, recovery, "config"), []byte("external recovery edit\n"), 0o600)
		}
	})
	err = RemoveDirectoryWithinSnapshot(root, "created", expected)
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RemoveDirectoryWithinSnapshot error = %v, want committed directory drift", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "created")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed removal target exists: %v", err)
	}
	recoveries := safefileArtifacts(t, root, ".safefile-removed-")
	if len(recoveries) != 1 {
		t.Fatalf("edited removed artifacts = %v, want preserved recovery", recoveries)
	}
	assertContent(t, filepath.Join(root, recoveries[0], "config"), "external recovery edit\n")
}

func TestRestoreDirectoryWithinRefusesRecreatedTargetAndPreservesRecovery(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "backup\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "original live\n", 0o600)
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.beforeInstall = func(_ int, _ int, _, _ string) error {
			if err := os.Mkdir(filepath.Join(root, "live"), 0o700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, "live", "attacker"), []byte("do not overwrite\n"), 0o600)
		}
	})
	err = RestoreDirectoryWithin(root, "live", snapshot)
	var recovery *RecoveryError
	if !errors.As(err, &recovery) || !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want RecoveryError/ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(root, "live", "attacker"), "do not overwrite\n")
	recoveries := safefileArtifacts(t, root, ".safefile-recovery-")
	if len(recoveries) != 1 {
		t.Fatalf("recovery artifacts = %v, want preserved original", recoveries)
	}
	assertContent(t, filepath.Join(root, recoveries[0], "config"), "original live\n")
}

func TestRestoreDirectoryWithinDetectsStagedTreeMutationAndRollsBack(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "backup\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "original live\n", 0o600)
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.beforeInstall = func(_ int, _ int, staged, _ string) error {
			return os.WriteFile(filepath.Join(root, staged, "config"), []byte("tampered\n"), 0o600)
		}
	})
	err = RestoreDirectoryWithin(root, "live", snapshot)
	if !errors.Is(err, ErrStagedChanged) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want ErrStagedChanged", err)
	}
	var recovery *RecoveryError
	var committed *CommittedError
	if errors.As(err, &recovery) || errors.As(err, &committed) {
		t.Fatalf("staged mutation with successful rollback reported uncertain/committed: %v", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "original live\n")
	assertNoSafefileDirectoryArtifacts(t, root)
}

func TestRestoreDirectoryWithinPostCommitFailurePreservesRecoveryTree(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "backup\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "live\n", 0o600)
	wantErr := errors.New("injected postinstall failure")
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterInstall = func(_ int, _ int, _ string) error { return wantErr }
	})
	err = RestoreDirectoryWithin(root, "live", snapshot)
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, wantErr) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want committed injected error", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "backup\n")
	recoveries := safefileArtifacts(t, root, ".safefile-recovery-")
	if len(recoveries) != 1 {
		t.Fatalf("recovery artifacts = %v, want one preserved original", recoveries)
	}
	assertContent(t, filepath.Join(root, recoveries[0], "config"), "live\n")
}

func TestRestoreDirectoryWithinParentFsyncFailureIsCommittedAndRecoverable(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "backup\n", 0o600)
	snapshot, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "live\n", 0o600)
	wantErr := errors.New("injected restore parent fsync failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.fsyncDir = func(_ int, operation string) error {
			if operation == "directory restore parent" {
				return wantErr
			}
			return nil
		}
	})
	err = RestoreDirectoryWithin(root, "live", snapshot)
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, wantErr) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want committed fsync error", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "backup\n")
	recoveries := safefileArtifacts(t, root, ".safefile-recovery-")
	if len(recoveries) != 1 {
		t.Fatalf("recovery artifacts = %v, want preserved original", recoveries)
	}
	assertContent(t, filepath.Join(root, recoveries[0], "config"), "live\n")
}

func TestRestoreDirectoryWithinStagingFailurePreservesLiveTree(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "backup\n", 0o600)
	if err := os.Chmod(filepath.Join(root, "source"), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "source"), 0o700) })
	snapshot, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, filepath.Join(root, "live"), 0o700)
	mustWrite(t, filepath.Join(root, "live", "config"), "live\n", 0o600)
	wantErr := errors.New("injected staged directory fsync failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.fsyncDir = func(_ int, operation string) error {
			if operation == "staged directory" {
				return wantErr
			}
			return nil
		}
	})
	err = RestoreDirectoryWithin(root, "live", snapshot)
	if !errors.Is(err, wantErr) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want injected error", err)
	}
	assertContent(t, filepath.Join(root, "live", "config"), "live\n")
	assertMode(t, filepath.Join(root, "source"), 0o555)
	assertNoSafefileDirectoryArtifacts(t, root)
}

func TestRemoveDirectoryWithinRecursesAndRollsBackPrecommitFailure(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mustMkdir(t, tree, 0o700)
	mustMkdir(t, filepath.Join(tree, "nested"), 0o700)
	mustWrite(t, filepath.Join(tree, "nested", "config"), "live\n", 0o600)
	wantErr := errors.New("injected removal move failure")
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.afterRemoveMove = func(_ int, _, _ string) error { return wantErr }
	})
	if err := RemoveDirectoryWithin(root, "tree"); !errors.Is(err, wantErr) {
		t.Fatalf("RemoveDirectoryWithin error = %v, want injected error", err)
	}
	assertContent(t, filepath.Join(tree, "nested", "config"), "live\n")
	assertNoSafefileDirectoryArtifacts(t, root)

	directoryTestHooks = directoryHooks{}
	if err := RemoveDirectoryWithin(root, "tree"); err != nil {
		t.Fatalf("RemoveDirectoryWithin retry: %v", err)
	}
	if _, err := os.Lstat(tree); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("tree still exists after removal: %v", err)
	}
}

func TestRemoveDirectoryWithinPostCommitCleanupFailureIsTruthful(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mustMkdir(t, tree, 0o700)
	mustWrite(t, filepath.Join(tree, "config"), "live\n", 0o600)
	wantErr := errors.New("injected cleanup failure")
	setDirectoryHooks(t, func(hooks *directoryHooks) {
		hooks.beforeRemoveEntry = func(_ int, _ string) error { return wantErr }
	})
	err := RemoveDirectoryWithin(root, "tree")
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, wantErr) {
		t.Fatalf("RemoveDirectoryWithin error = %v, want committed cleanup error", err)
	}
	if _, err := os.Lstat(tree); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("committed removal target still exists: %v", err)
	}
	recoveries := safefileArtifacts(t, root, ".safefile-removed-")
	if len(recoveries) != 1 {
		t.Fatalf("removal recovery artifacts = %v, want one", recoveries)
	}
	assertContent(t, filepath.Join(root, recoveries[0], "config"), "live\n")
}

func TestRemoveDirectoryWithinParentFsyncFailureRollsBack(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mustMkdir(t, tree, 0o700)
	mustWrite(t, filepath.Join(tree, "config"), "live\n", 0o600)
	wantErr := errors.New("injected removal parent fsync failure")
	failed := false
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.fsyncDir = func(_ int, operation string) error {
			if operation == "directory removal parent" && !failed {
				failed = true
				return wantErr
			}
			return nil
		}
	})
	err := RemoveDirectoryWithin(root, "tree")
	if !errors.Is(err, wantErr) {
		t.Fatalf("RemoveDirectoryWithin error = %v, want injected fsync error", err)
	}
	var committed *CommittedError
	var recovery *RecoveryError
	if errors.As(err, &committed) || errors.As(err, &recovery) {
		t.Fatalf("successful removal rollback reported committed/recovery failure: %v", err)
	}
	assertContent(t, filepath.Join(tree, "config"), "live\n")
	assertNoSafefileDirectoryArtifacts(t, root)
}

func TestRemoveDirectoryWithinRefusesNestedSymlinkBeforeMutation(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mustMkdir(t, tree, 0o700)
	out := filepath.Join(t.TempDir(), "outside")
	mustWrite(t, out, "outside\n", 0o600)
	if err := os.Symlink(out, filepath.Join(tree, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveDirectoryWithin(root, "tree"); !errors.Is(err, ErrSymlink) {
		t.Fatalf("RemoveDirectoryWithin error = %v, want ErrSymlink", err)
	}
	assertContent(t, out, "outside\n")
	if _, err := os.Lstat(tree); err != nil {
		t.Fatalf("tree was mutated: %v", err)
	}
	assertNoSafefileDirectoryArtifacts(t, root)
}

func setDirectoryHooks(t *testing.T, configure func(*directoryHooks)) {
	t.Helper()
	directoryTestHooks = directoryHooks{}
	configure(&directoryTestHooks)
	t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
}

func assertNoSafefileDirectoryArtifacts(t *testing.T, parent string) {
	t.Helper()
	if got := safefileArtifacts(t, parent, ".safefile-"); len(got) != 0 {
		t.Fatalf("unexpected safefile directory artifacts in %s: %v", parent, got)
	}
}

func safefileArtifacts(t *testing.T, parent, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			got = append(got, entry.Name())
		}
	}
	return got
}
