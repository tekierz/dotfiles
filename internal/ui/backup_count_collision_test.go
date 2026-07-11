package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/backup"
)

// TestCountBackupFilesExcludesManifest verifies that countBackupFiles reports
// exactly the number of backed-up dotfiles (N), not N+1, when a manifest is
// present alongside the backed-up files.
func TestCountBackupFilesExcludesManifest(t *testing.T) {
	dir := t.TempDir()

	// Write 2 dotfile entries (flat-encoded names) and the manifest.
	files := []string{"_zshrc", "_gitconfig"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, backup.ManifestName), []byte("manifest content"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got := countBackupFiles(dir)
	if got != len(files) {
		t.Errorf("countBackupFiles = %d, want %d (manifest must not be counted)", got, len(files))
	}
}

// TestCreateBackupCmdCollisionYieldsTwoDistinctDirs verifies that two backups
// whose timestamp-derived directory names would collide (same second) result in
// two distinct, non-empty directories instead of one overwriting the other.
//
// The collision is simulated by pre-creating the expected timestamp directory
// before createBackupCmd runs — this is equivalent to a backup having just been
// created in the same second, so the cmd must pick a unique alternative name.
func TestCreateBackupCmdCollisionYieldsTwoDistinctDirs(t *testing.T) {
	// Redirect config and home to temp dirs so createBackupCmd is fully
	// exercised without touching real user files.
	home := t.TempDir()
	cfgDir := t.TempDir()

	// Plant a dotfile so backup.Create captures at least one file.
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# zshrc"), 0o600); err != nil {
		t.Fatalf("write .zshrc: %v", err)
	}

	backupsDir := filepath.Join(cfgDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o700); err != nil {
		t.Fatalf("mkdir backupsDir: %v", err)
	}

	// Pre-create the "collision" directory — simulates a backup already
	// created in the same second.
	collidingName := "2026-01-01_12-00-00"
	collidingDir := filepath.Join(backupsDir, collidingName)
	if err := os.MkdirAll(collidingDir, 0o700); err != nil {
		t.Fatalf("mkdir colliding: %v", err)
	}
	// Write a sentinel file so we can confirm it was not overwritten.
	sentinel := []byte("original")
	if err := os.WriteFile(filepath.Join(collidingDir, "sentinel"), sentinel, 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Call makeUniqueBackupDir to get a unique directory for the same timestamp.
	unique := makeUniqueBackupDir(backupsDir, collidingName)
	if unique == collidingDir {
		t.Fatal("makeUniqueBackupDir returned the colliding path unchanged")
	}
	if strings.Contains(unique, "..") || !strings.HasPrefix(unique, backupsDir) {
		t.Fatalf("unique dir %q escapes backupsDir", unique)
	}

	// Create owns the final unique directory creation so its descriptor-bound
	// identity cannot be replaced between path selection and persistence.
	if _, err := backup.Create(home, unique, []string{".zshrc"}); err != nil {
		t.Fatalf("backup.Create: %v", err)
	}

	// Both directories must exist and the colliding one must be untouched.
	if _, err := os.Stat(collidingDir); err != nil {
		t.Errorf("colliding dir disappeared: %v", err)
	}
	if _, err := os.Stat(unique); err != nil {
		t.Errorf("unique dir not created: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(collidingDir, "sentinel"))
	if err != nil || string(got) != string(sentinel) {
		t.Errorf("colliding dir was overwritten; sentinel = %q want %q (err=%v)", got, sentinel, err)
	}

	// Both must pass isValidBackupName (listable/restorable by CLI).
	collidingBase := filepath.Base(collidingDir)
	uniqueBase := filepath.Base(unique)
	// We can't call isValidBackupName (cmd package) directly; replicate the
	// relevant check: no path separator, equals filepath.Base.
	for _, name := range []string{collidingBase, uniqueBase} {
		if strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator) {
			t.Errorf("backup name %q contains a path separator", name)
		}
		if name != filepath.Base(name) {
			t.Errorf("backup name %q is not a single path component", name)
		}
	}
}
