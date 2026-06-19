package backup

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateZeroFiles verifies that a backup of a home directory containing
// none of the requested files returns an error (and writes no manifest),
// instead of silently reporting success (C4/C5). A "successful" empty backup
// would let the pre-install auto-backup claim a rollback point exists when it
// does not.
func TestCreateZeroFiles(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")

	files := []string{".zshrc", ".gitconfig"}

	count, err := Create(home, backupDir, files)
	if err == nil {
		t.Fatalf("Create with no capturable files = nil error, want error")
	}
	if count != 0 {
		t.Errorf("Create count = %d, want 0", count)
	}

	// No manifest should have been written for an empty backup.
	if _, statErr := os.Stat(filepath.Join(backupDir, ManifestName)); statErr == nil {
		t.Errorf("manifest was written for an empty backup; want none")
	}
}

// TestCreateCapturesFilesAndManifest verifies a normal backup writes each
// present file, records a manifest, and returns the real success count.
func TestCreateCapturesFilesAndManifest(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "backup")

	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export A=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// .gitconfig intentionally absent; should be skipped without failing.

	count, err := Create(home, backupDir, []string{".zshrc", ".gitconfig"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if count != 1 {
		t.Errorf("Create count = %d, want 1", count)
	}

	// Manifest must exist and round-trip through ReadManifest to the one file.
	entries, err := ReadManifest(backupDir)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if len(entries) != 1 || entries[0].RelPath != ".zshrc" {
		t.Errorf("manifest entries = %+v, want single .zshrc", entries)
	}
}
