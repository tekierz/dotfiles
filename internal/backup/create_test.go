package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
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

func TestCreatePlanRecordsAbsentTargetsAndRollbackRemovesThem(t *testing.T) {
	home := t.TempDir()
	backupDir := filepath.Join(t.TempDir(), "nested", "sessions", "plan")
	targets := []Target{
		{RelPath: ".config/tool/config", Kind: TargetFile},
		{RelPath: ".config/new-tree", Kind: TargetDirectory},
	}
	count, err := CreatePlan(home, backupDir, targets)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("captured count = %d, want 0 existing targets", count)
	}
	manifest, err := os.ReadFile(filepath.Join(backupDir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "|no|file") || !strings.Contains(string(manifest), "|no|directory") {
		t.Fatalf("absence manifest = %s", manifest)
	}
	filePath := filepath.Join(home, ".config", "tool", "config")
	dirPath := filepath.Join(home, ".config", "new-tree")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("created"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "child"), []byte("created"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 0 || len(result.Removed) != 2 {
		t.Fatalf("absence rollback result = %+v", result)
	}
	for _, path := range []string{filePath, dirPath} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("created target survived rollback %s: %v", path, err)
		}
	}
}

func TestCreatePlanRestoresExistingFileAndDirectoryExactly(t *testing.T) {
	home := t.TempDir()
	fileRel := ".toolrc"
	dirRel := ".config/tool-tree"
	filePath := filepath.Join(home, fileRel)
	dirPath := filepath.Join(home, filepath.FromSlash(dirRel))
	if err := os.WriteFile(filePath, []byte("original file\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dirPath, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dirPath, 0o710); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "nested", "value"), []byte("original tree\n"), 0o604); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(t.TempDir(), "plan")
	count, err := CreatePlan(home, backupDir, []Target{{fileRel, TargetFile}, {dirRel, TargetDirectory}})
	if err != nil || count != 2 {
		t.Fatalf("CreatePlan count=%d err=%v", count, err)
	}
	if err := os.WriteFile(filePath, []byte("mutated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dirPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "other"), []byte("mutated"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Restore(backupDir, home)
	if err != nil || len(result.Skipped) != 0 {
		t.Fatalf("Restore result=%+v err=%v", result, err)
	}
	data, _ := os.ReadFile(filePath)
	if string(data) != "original file\n" {
		t.Fatalf("restored file = %q", data)
	}
	assertCreateMode(t, filePath, 0o640)
	data, _ = os.ReadFile(filepath.Join(dirPath, "nested", "value"))
	if string(data) != "original tree\n" {
		t.Fatalf("restored directory content = %q", data)
	}
	assertCreateMode(t, dirPath, 0o710)
	assertCreateMode(t, filepath.Join(dirPath, "nested", "value"), 0o604)
}

func TestCreatePlanRefusesSymlinkedSourceAndUnsafeScope(t *testing.T) {
	home := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, ".toolrc")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := CreatePlan(home, filepath.Join(t.TempDir(), "symlink"), []Target{{".toolrc", TargetFile}}); !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("symlink CreatePlan error = %v, want ErrSymlink", err)
	}
	if _, err := CreatePlan(home, filepath.Join(t.TempDir(), "traversal"), []Target{{"../escape", TargetFile}}); err == nil {
		t.Fatal("CreatePlan accepted traversal target")
	}
}

func TestCreatePlanRefusesSymlinkedBackupAncestorBelowHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "plan")
	_, err := CreatePlan(home, backupDir, []Target{{RelPath: ".toolrc", Kind: TargetFile}})
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("CreatePlan error = %v, want ErrSymlink", err)
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Fatalf("symlink target was changed: entries=%v err=%v", entries, readErr)
	}
}

func assertCreateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
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
