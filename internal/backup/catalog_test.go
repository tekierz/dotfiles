//go:build darwin || linux

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func writeCatalogTestFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func makeCatalogTestDir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(path, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func snapshotCatalogTestBackup(t *testing.T, root, name string) *safefile.DirectorySnapshot {
	t.Helper()
	snapshot, err := safefile.SnapshotDirectoryWithin(root, name)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func assertCatalogTestFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != contents {
		t.Fatalf("%s contents = %q, want %q", path, data, contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != mode.Perm() {
		t.Fatalf("%s mode = %o, want %o", path, got, mode.Perm())
	}
}

func assertCatalogTestDirMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
	if got := info.Mode().Perm(); got != mode.Perm() {
		t.Fatalf("%s mode = %o, want %o", path, got, mode.Perm())
	}
}

func createCatalogFixture(t *testing.T) (home, root, path string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export TEST=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(home, ".config", "dotfiles", "backups")
	path = filepath.Join(root, "valid")
	if _, err := Create(home, path, []string{".zshrc"}); err != nil {
		t.Fatal(err)
	}
	return home, root, path
}

func TestListCatalogFiltersPartialAndSymlinkedManifestBackups(t *testing.T) {
	_, root, _ := createCatalogFixture(t)
	partial := filepath.Join(root, "partial")
	if err := os.Mkdir(partial, 0o700); err != nil {
		t.Fatal(err)
	}
	symlinked := filepath.Join(root, "symlinked")
	if err := os.Mkdir(symlinked, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "manifest")
	if err := os.WriteFile(outside, []byte(".zshrc\t600\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(symlinked, ManifestName)); err != nil {
		t.Fatal(err)
	}

	entries, err := ListCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "valid" || entries[0].FileCount != 1 || entries[0].Size == 0 {
		t.Fatalf("catalog entries = %+v, want only valid manifest-backed backup", entries)
	}
}

func TestRemoveCatalogEntryRejectsIdenticalDirectoryReplacement(t *testing.T) {
	home, root, path := createCatalogFixture(t)
	entries, err := ListCatalog(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("catalog = %+v, err = %v", entries, err)
	}
	old := filepath.Join(root, "old")
	if err := os.Rename(path, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(home, path, []string{".zshrc"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCatalogEntry(entries[0]); !errors.Is(err, safefile.ErrDirectoryChanged) {
		t.Fatalf("remove replacement error = %v, want ErrDirectoryChanged", err)
	}
	if _, err := os.Stat(filepath.Join(path, ManifestName)); err != nil {
		t.Fatalf("replacement backup was removed: %v", err)
	}
}

func TestRemoveCatalogEntryDeletesExactValidatedTree(t *testing.T) {
	_, root, path := createCatalogFixture(t)
	entries, err := ListCatalog(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("catalog = %+v, err = %v", entries, err)
	}
	if err := RemoveCatalogEntry(entries[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("exact backup remains after delete: %v", err)
	}
}

func TestRestoreCatalogSnapshotUsesImmutableSourcesAndExplicitHome(t *testing.T) {
	home := t.TempDir()
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)

	backupRoot := t.TempDir()
	backupName := "selected"
	backupDir := filepath.Join(backupRoot, backupName)
	makeCatalogTestDir(t, backupDir, 0o700)

	writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName(".zshrc")), "captured zshrc\n", 0o600)
	nvimSource := filepath.Join(backupDir, ".config", "nvim")
	makeCatalogTestDir(t, nvimSource, 0o710)
	writeCatalogTestFile(t, filepath.Join(nvimSource, "init.lua"), "captured init\n", 0o644)
	makeCatalogTestDir(t, filepath.Join(nvimSource, "lua"), 0o730)
	writeCatalogTestFile(t, filepath.Join(nvimSource, "lua", "plugin.lua"), "captured plugin\n", 0o604)
	makeCatalogTestDir(t, filepath.Join(nvimSource, "empty"), 0o750)

	liveNvim := filepath.Join(home, ".config", "nvim")
	createdDir := filepath.Join(home, ".config", "generated")
	manifest := strings.Join([]string{
		ManifestLine(".zshrc", 0o640),
		liveNvim + "|" + nvimSource + "|yes|directory|710",
		createdDir + "||no|directory|700",
		"",
	}, "\n")
	writeCatalogTestFile(t, filepath.Join(backupDir, ManifestName), manifest, 0o600)

	writeCatalogTestFile(t, filepath.Join(home, ".zshrc"), "live zshrc\n", 0o600)
	writeCatalogTestFile(t, filepath.Join(liveNvim, "old.lua"), "live nvim\n", 0o600)
	writeCatalogTestFile(t, filepath.Join(createdDir, "nested", "remove-me"), "generated\n", 0o600)
	writeCatalogTestFile(t, filepath.Join(ambientHome, ".zshrc"), "ambient decoy\n", 0o600)

	snapshot := snapshotCatalogTestBackup(t, backupRoot, backupName)
	if err := os.Rename(backupDir, filepath.Join(backupRoot, "captured-replaced")); err != nil {
		t.Fatal(err)
	}
	makeCatalogTestDir(t, backupDir, 0o700)
	writeCatalogTestFile(t, filepath.Join(backupDir, ManifestName), manifest, 0o600)
	writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName(".zshrc")), "poison zshrc\n", 0o600)
	writeCatalogTestFile(t, filepath.Join(backupDir, ".config", "nvim", "init.lua"), "poison init\n", 0o600)

	result, err := restoreCatalogSnapshot(snapshot, home)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count() != 2 || len(result.Restored) != 2 {
		t.Fatalf("restore result = %+v, want two restored entries", result)
	}
	if len(result.Removed) != 1 || result.Removed[0] != ".config/generated" {
		t.Fatalf("removed = %v, want [.config/generated]", result.Removed)
	}
	if len(result.Skipped) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("unexpected non-clean restore result: %+v", result)
	}

	assertCatalogTestFile(t, filepath.Join(home, ".zshrc"), "captured zshrc\n", 0o640)
	assertCatalogTestDirMode(t, liveNvim, 0o710)
	assertCatalogTestFile(t, filepath.Join(liveNvim, "init.lua"), "captured init\n", 0o644)
	assertCatalogTestDirMode(t, filepath.Join(liveNvim, "lua"), 0o730)
	assertCatalogTestFile(t, filepath.Join(liveNvim, "lua", "plugin.lua"), "captured plugin\n", 0o604)
	assertCatalogTestDirMode(t, filepath.Join(liveNvim, "empty"), 0o750)
	if _, err := os.Lstat(filepath.Join(liveNvim, "old.lua")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale live directory entry remains after exact replacement: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(liveNvim, "empty"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("restored empty directory entries = %v, err = %v", entries, err)
	}
	if _, err := os.Lstat(createdDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created directory remains after restore: %v", err)
	}
	assertCatalogTestFile(t, filepath.Join(ambientHome, ".zshrc"), "ambient decoy\n", 0o600)
}

func TestRestoreCatalogSnapshotPreflightsEveryRecordBeforeMutation(t *testing.T) {
	tests := []struct {
		name        string
		prepare     func(t *testing.T, home, backupDir string) string
		assertAfter func(t *testing.T, home string)
	}{
		{
			name: "malformed record",
			prepare: func(_ *testing.T, _, _ string) string {
				return "malformed|record"
			},
		},
		{
			name: "traversal",
			prepare: func(t *testing.T, _, backupDir string) string {
				writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName("../escape")), "captured traversal\n", 0o600)
				return ManifestLine("../escape", 0o600)
			},
		},
		{
			name: "duplicate destination",
			prepare: func(_ *testing.T, _, _ string) string {
				return ManifestLine(".first", 0o600)
			},
		},
		{
			name: "missing source",
			prepare: func(_ *testing.T, _, _ string) string {
				return ManifestLine(".missing", 0o600)
			},
		},
		{
			name: "source type mismatch",
			prepare: func(t *testing.T, _, backupDir string) string {
				makeCatalogTestDir(t, filepath.Join(backupDir, EncodeName(".wrong")), 0o700)
				return ManifestLine(".wrong", 0o600)
			},
		},
		{
			name: "symlinked destination ancestor",
			prepare: func(t *testing.T, home, backupDir string) string {
				writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName(".linked/config")), "captured linked\n", 0o600)
				outside := t.TempDir()
				writeCatalogTestFile(t, filepath.Join(outside, "config"), "outside sentinel\n", 0o640)
				if err := os.Symlink(outside, filepath.Join(home, ".linked")); err != nil {
					t.Fatal(err)
				}
				return ManifestLine(".linked/config", 0o600)
			},
			assertAfter: func(t *testing.T, home string) {
				assertCatalogTestFile(t, filepath.Join(home, ".linked", "config"), "outside sentinel\n", 0o640)
			},
		},
		{
			name: "symlinked destination leaf",
			prepare: func(t *testing.T, home, backupDir string) string {
				writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName(".leaf")), "captured leaf\n", 0o600)
				outside := filepath.Join(t.TempDir(), "outside-leaf")
				writeCatalogTestFile(t, outside, "outside leaf sentinel\n", 0o640)
				if err := os.Symlink(outside, filepath.Join(home, ".leaf")); err != nil {
					t.Fatal(err)
				}
				return ManifestLine(".leaf", 0o600)
			},
			assertAfter: func(t *testing.T, home string) {
				assertCatalogTestFile(t, filepath.Join(home, ".leaf"), "outside leaf sentinel\n", 0o640)
				info, err := os.Lstat(filepath.Join(home, ".leaf"))
				if err != nil {
					t.Fatalf("destination leaf symlink was changed: %v", err)
				}
				if info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("destination leaf mode = %v, want symlink", info.Mode())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", t.TempDir())
			backupRoot := t.TempDir()
			backupName := "selected"
			backupDir := filepath.Join(backupRoot, backupName)
			makeCatalogTestDir(t, backupDir, 0o700)

			writeCatalogTestFile(t, filepath.Join(home, ".first"), "live sentinel\n", 0o640)
			writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName(".first")), "captured first\n", 0o600)
			laterRecord := tt.prepare(t, home, backupDir)
			manifest := strings.Join([]string{
				ManifestLine(".first", 0o600),
				laterRecord,
				"",
			}, "\n")
			writeCatalogTestFile(t, filepath.Join(backupDir, ManifestName), manifest, 0o600)

			snapshot := snapshotCatalogTestBackup(t, backupRoot, backupName)
			result, err := restoreCatalogSnapshot(snapshot, home)
			if err == nil {
				t.Fatalf("restore result = %+v, want fatal preflight error", result)
			}
			if result.Count() != 0 || len(result.Restored) != 0 || len(result.Removed) != 0 || len(result.Skipped) != 0 || len(result.Warnings) != 0 {
				t.Fatalf("fatal preflight result = %+v, want empty result", result)
			}
			assertCatalogTestFile(t, filepath.Join(home, ".first"), "live sentinel\n", 0o640)
			if tt.assertAfter != nil {
				tt.assertAfter(t, home)
			}
		})
	}
}

func TestRestoreCatalogSnapshotRejectsUntrustedHomeAuthority(t *testing.T) {
	tests := []struct {
		name    string
		homeArg func(t *testing.T, realHome string) string
	}{
		{
			name: "relative home",
			homeArg: func(t *testing.T, realHome string) string {
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				rel, err := filepath.Rel(cwd, realHome)
				if err != nil {
					t.Fatal(err)
				}
				return rel
			},
		},
		{
			name: "symlinked home",
			homeArg: func(t *testing.T, realHome string) string {
				link := filepath.Join(t.TempDir(), "home-link")
				if err := os.Symlink(realHome, link); err != nil {
					t.Fatal(err)
				}
				return link
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realHome := t.TempDir()
			t.Setenv("HOME", t.TempDir())
			writeCatalogTestFile(t, filepath.Join(realHome, ".first"), "live sentinel\n", 0o640)

			backupRoot := t.TempDir()
			backupName := "selected"
			backupDir := filepath.Join(backupRoot, backupName)
			makeCatalogTestDir(t, backupDir, 0o700)
			writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName(".first")), "captured first\n", 0o600)
			writeCatalogTestFile(t, filepath.Join(backupDir, ManifestName), ManifestLine(".first", 0o600)+"\n", 0o600)
			snapshot := snapshotCatalogTestBackup(t, backupRoot, backupName)

			result, err := restoreCatalogSnapshot(snapshot, tt.homeArg(t, realHome))
			if err == nil {
				t.Fatalf("restore result = %+v, want fatal home authority error", result)
			}
			if result.Count() != 0 || len(result.Restored) != 0 || len(result.Removed) != 0 || len(result.Skipped) != 0 || len(result.Warnings) != 0 {
				t.Fatalf("fatal home authority result = %+v, want empty result", result)
			}
			assertCatalogTestFile(t, filepath.Join(realHome, ".first"), "live sentinel\n", 0o640)
		})
	}
}

func TestBA1ASnapshotCaptureRejectsSymlinkedCatalogSourceBeforeRestore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	backupRoot := t.TempDir()
	backupName := "selected"
	backupDir := filepath.Join(backupRoot, backupName)
	makeCatalogTestDir(t, backupDir, 0o700)

	writeCatalogTestFile(t, filepath.Join(home, ".first"), "live sentinel\n", 0o640)
	writeCatalogTestFile(t, filepath.Join(backupDir, EncodeName(".first")), "captured first\n", 0o600)
	outside := filepath.Join(t.TempDir(), "outside-source")
	writeCatalogTestFile(t, outside, "outside bytes\n", 0o600)
	if err := os.Symlink(outside, filepath.Join(backupDir, EncodeName(".linked-source"))); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Join([]string{
		ManifestLine(".first", 0o600),
		ManifestLine(".linked-source", 0o600),
		"",
	}, "\n")
	writeCatalogTestFile(t, filepath.Join(backupDir, ManifestName), manifest, 0o600)

	if _, err := safefile.SnapshotDirectoryWithin(backupRoot, backupName); !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("snapshot symlinked manifest source error = %v, want ErrSymlink", err)
	}
	assertCatalogTestFile(t, filepath.Join(home, ".first"), "live sentinel\n", 0o640)
	assertCatalogTestFile(t, outside, "outside bytes\n", 0o600)
}
