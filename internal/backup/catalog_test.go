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

func TestRestoreCatalogEntryPinsLaterPayloadAcrossEarlierWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "backups")
	// The first accepted target is the second item's live backup source. This
	// deterministically changes that source during actual restore, without hooks.
	first := "backups/selected/.zshrc"
	dir := writeCatalogManifest(t, root, "selected", first+"\t600\n.zshrc\t640\n", []byte("accepted later payload"))
	if err := os.WriteFile(filepath.Join(dir, EncodeName(first)), []byte("changed during restore"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := ListCatalog(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("catalog count=%d error=%v", len(entries), err)
	}
	entries[0].Name, entries[0].Path = "unreviewed", filepath.Join(home, "unreviewed")
	result, err := RestoreCatalogEntry(entries[0], home)
	if err != nil || result.Count() != 2 || len(result.Skipped) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("restore result=%+v error=%v", result, err)
	}
	for path, want := range map[string]string{
		filepath.Join(dir, ".zshrc"):  "changed during restore",
		filepath.Join(home, ".zshrc"): "accepted later payload",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("restored %s=%q error=%v, want %q", path, data, err, want)
		}
	}
	if info, err := os.Stat(filepath.Join(home, ".zshrc")); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("restored mode info=%v error=%v", info, err)
	}
}

func TestCatalogRejectsChangedAuthorityBeforeRestoreOrDelete(t *testing.T) {
	for _, change := range []string{"content", "manifest", "mode", "symlink", "root", "parent"} {
		t.Run(change, func(t *testing.T) {
			home, root, dir := createCatalogFixture(t)
			entries, err := ListCatalog(root)
			if err != nil || len(entries) != 1 {
				t.Fatalf("catalog count=%d error=%v", len(entries), err)
			}
			payload := filepath.Join(dir, EncodeName(".zshrc"))
			switch change {
			case "content":
				err = os.WriteFile(payload, []byte("changed"), 0o600)
			case "manifest":
				err = os.WriteFile(filepath.Join(dir, ManifestName), []byte(".other\t600\n"), 0o600)
			case "mode":
				err = os.Chmod(payload, 0o640)
			case "symlink":
				if err = os.Remove(payload); err == nil {
					err = os.Symlink(filepath.Join(home, ".zshrc"), payload)
				}
			case "root":
				if err = os.Rename(dir, dir+"-old"); err == nil {
					_, err = Create(home, dir, []string{".zshrc"})
				}
			case "parent":
				if err = os.Rename(root, root+"-old"); err == nil {
					if err = os.Mkdir(root, 0o700); err == nil {
						// Keep the selected root inode while replacing its parent.
						err = os.Rename(filepath.Join(root+"-old", "valid"), dir)
					}
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("live unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			if result, err := RestoreCatalogEntry(entries[0], home); err == nil || result.Count() != 0 {
				t.Fatalf("changed source restored: result=%+v error=%v", result, err)
			}
			if err := RemoveCatalogEntry(entries[0]); err == nil {
				t.Fatal("changed source deleted")
			}
			data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
			if err != nil || string(data) != "live unchanged" {
				t.Fatalf("failed restore changed destination: %q error=%v", data, err)
			}
			if _, err := os.Stat(filepath.Join(dir, ManifestName)); err != nil {
				t.Fatalf("failed deletion changed backup: %v", err)
			}
		})
	}
}

func TestRestoreCatalogEntryDirectoryAndAcceptedAbsence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "backups")
	dirTarget := filepath.Join(home, ".config", "test")
	absentTarget := filepath.Join(home, ".obsolete")
	dir := writeCatalogManifest(t, root, "selected", dirTarget+"|ignored|yes|directory\n"+absentTarget+"||no|file\n", nil)
	sourceDir := filepath.Join(dir, ".config", "test")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "config"), []byte("accepted directory"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absentTarget, []byte("remove me"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := ListCatalog(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("catalog count=%d error=%v", len(entries), err)
	}
	result, err := RestoreCatalogEntry(entries[0], home)
	if err != nil || result.Count() != 1 || len(result.Removed) != 1 || len(result.Skipped) != 0 {
		t.Fatalf("restore result=%+v error=%v", result, err)
	}
	if data, err := os.ReadFile(filepath.Join(dirTarget, "config")); err != nil || string(data) != "accepted directory" {
		t.Fatalf("directory payload=%q error=%v", data, err)
	}
	if _, err := os.Lstat(absentTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("accepted absent target remains: %v", err)
	}
	entries[0].Path, entries[0].Name = home, "unreviewed"
	if err := RemoveCatalogEntry(entries[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dirTarget, "config")); err != nil {
		t.Fatalf("display metadata redirected deletion: %v", err)
	}
}
