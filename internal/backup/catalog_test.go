//go:build darwin || linux

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

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
