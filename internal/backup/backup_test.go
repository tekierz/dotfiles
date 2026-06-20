package backup

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreSkipsSymlinkedParent verifies the restore write path refuses to
// write through a symlinked PARENT directory that resolves outside home. The
// final-component noFollowWrite check alone does not cover this: the leaf does
// not exist yet, so only resolving the parent catches the escape. The malicious
// file must be recorded in Skipped, not written through the symlink.
func TestRestoreSkipsSymlinkedParent(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir() // simulates a location OUTSIDE home

	// Create a symlink inside home (.config/evil) that points at the outside dir.
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o700); err != nil {
		t.Fatalf("mkdir .config: %v", err)
	}
	evilLink := filepath.Join(home, ".config", "evil")
	if err := os.Symlink(outside, evilLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// A backup whose relPath traverses through the symlinked parent. safeJoin's
	// lexical check passes (no ".." component, stays under home textually), so the
	// only defense is resolving the symlinked parent at write time.
	relPath := ".config/evil/secret"
	backupDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("pwned"), 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}

	// Nothing must have been written through the symlink to the outside dir.
	if _, statErr := os.Stat(filepath.Join(outside, "secret")); statErr == nil {
		t.Fatalf("file was written THROUGH the symlinked parent into %q", outside)
	}

	if result.Count() != 0 {
		t.Errorf("expected 0 restored files, got %d", result.Count())
	}
	if _, ok := result.Skipped[relPath]; !ok {
		t.Errorf("expected %q to be recorded as skipped, skipped=%v", relPath, result.Skipped)
	}
}

// TestRestoreAllowsNewDirsUnderHome verifies the symlinked-parent guard does NOT
// reject legitimate restores into directories that do not exist yet under the
// real home. EvalSymlinks must resolve the deepest existing ancestor, not the
// (absent) leaf or its (absent) parent.
func TestRestoreAllowsNewDirsUnderHome(t *testing.T) {
	home := t.TempDir()

	// Deeply nested path; none of these directories exist yet.
	relPath := ".config/brand/new/tool/config.toml"
	content := []byte("ok")
	backupDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), content, 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 1 {
		t.Fatalf("expected 1 restored file into new dirs under home, got %d (skipped=%v)", result.Count(), result.Skipped)
	}
	got, err := os.ReadFile(filepath.Join(home, relPath))
	if err != nil {
		t.Fatalf("restored file not found: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("restored content = %q, want %q", got, content)
	}
}
