package backup

import (
	"os"
	"path/filepath"
	"strings"
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

func TestRestoreBashDirectoryFormatBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	backupRoot := t.TempDir()
	backupDir := filepath.Join(backupRoot, "20260704_120000")

	originalDir := filepath.Join(home, ".config", "nvim")
	backupNvim := filepath.Join(backupDir, ".config", "nvim")
	if err := os.MkdirAll(filepath.Join(backupNvim, "lua"), 0o700); err != nil {
		t.Fatalf("mkdir backup nvim: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupNvim, "init.lua"), []byte("original init\n"), 0o600); err != nil {
		t.Fatalf("write backup init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupNvim, "lua", "plugin.lua"), []byte("original plugin\n"), 0o640); err != nil {
		t.Fatalf("write backup nested file: %v", err)
	}

	if err := os.MkdirAll(originalDir, 0o700); err != nil {
		t.Fatalf("mkdir current nvim: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalDir, "init.lua"), []byte("modified\n"), 0o600); err != nil {
		t.Fatalf("write current init: %v", err)
	}

	createdDir := filepath.Join(home, ".config", "generated")
	if err := os.MkdirAll(createdDir, 0o700); err != nil {
		t.Fatalf("mkdir generated dir: %v", err)
	}

	manifest := strings.Join([]string{
		"# Dotfiles Backup Manifest",
		"# Format: original_path|backup_path|existed_before",
		originalDir + "|" + backupNvim + "|yes|directory",
		createdDir + "||no|directory",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write bash manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 1 || len(result.Removed) != 1 || len(result.Skipped) != 0 {
		t.Fatalf("Restore result = restored %v removed %v skipped %v, want 1 restored dir, 1 removed dir, 0 skipped",
			result.Restored, result.Removed, result.Skipped)
	}

	gotInit, err := os.ReadFile(filepath.Join(originalDir, "init.lua"))
	if err != nil {
		t.Fatalf("read restored init: %v", err)
	}
	if string(gotInit) != "original init\n" {
		t.Errorf("restored init = %q, want original backup content", gotInit)
	}
	gotNested, err := os.ReadFile(filepath.Join(originalDir, "lua", "plugin.lua"))
	if err != nil {
		t.Fatalf("read restored nested file: %v", err)
	}
	if string(gotNested) != "original plugin\n" {
		t.Errorf("restored nested file = %q, want original backup content", gotNested)
	}
	if _, err := os.Stat(createdDir); !os.IsNotExist(err) {
		t.Errorf("generated dir still exists after restore, stat err=%v", err)
	}
	info, err := os.Stat(filepath.Join(originalDir, "lua", "plugin.lua"))
	if err != nil {
		t.Fatalf("stat restored nested file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("restored nested file mode = %o, want 0640", got)
	}

	entries, err := ReadManifest(backupDir)
	if err != nil {
		t.Fatalf("ReadManifest bash format: %v", err)
	}
	if len(entries) != 1 || entries[0].RelPath != filepath.Join(".config", "nvim") {
		t.Fatalf("ReadManifest entries = %#v, want one .config/nvim entry", entries)
	}
}

func TestRestoreBashDirectoryMissingSourceKeepsLiveDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	backupDir := t.TempDir()

	originalDir := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(originalDir, 0o700); err != nil {
		t.Fatalf("mkdir live nvim: %v", err)
	}
	liveFile := filepath.Join(originalDir, "init.lua")
	if err := os.WriteFile(liveFile, []byte("do not delete\n"), 0o600); err != nil {
		t.Fatalf("write live init: %v", err)
	}

	missingBackupDir := filepath.Join(backupDir, ".config", "nvim")
	manifest := originalDir + "|" + missingBackupDir + "|yes|directory\n"
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write bash manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 0 {
		t.Fatalf("expected no restored entries, got restored=%v", result.Restored)
	}
	if _, ok := result.Skipped[filepath.Join(".config", "nvim")]; !ok {
		t.Fatalf("expected missing backup directory to be skipped, skipped=%v", result.Skipped)
	}
	got, err := os.ReadFile(liveFile)
	if err != nil {
		t.Fatalf("live file was removed or became unreadable: %v", err)
	}
	if string(got) != "do not delete\n" {
		t.Fatalf("live file content = %q, want unchanged", got)
	}
}

func TestRestoreSkipsMalformedBashManifestLineAndContinues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	backupDir := t.TempDir()

	relPath := ".zshrc"
	originalPath := filepath.Join(home, relPath)
	backupPath := filepath.Join(backupDir, relPath)
	if err := os.WriteFile(backupPath, []byte("restored zshrc\n"), 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}

	badLine := originalPath + "|" + backupPath + "|maybe"
	manifest := strings.Join([]string{
		badLine,
		originalPath + "|" + backupPath + "|yes|file",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write bash manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 1 {
		t.Fatalf("expected one valid entry restored, got restored=%v skipped=%v", result.Restored, result.Skipped)
	}
	if _, ok := result.Skipped[badLine]; !ok {
		t.Fatalf("expected malformed line to be skipped, skipped=%v", result.Skipped)
	}
	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != "restored zshrc\n" {
		t.Fatalf("restored file content = %q, want backup content", got)
	}
}

func TestRestoreBashManifestRelocatedBackupDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldBackupDir := filepath.Join(t.TempDir(), "old-backup")
	backupDir := filepath.Join(t.TempDir(), "relocated-backup")

	originalDir := filepath.Join(home, ".config", "nvim")
	relDir := filepath.Join(".config", "nvim")
	relFile := filepath.Join(relDir, "init.lua")
	actualBackupDir := filepath.Join(backupDir, relDir)
	if err := os.MkdirAll(actualBackupDir, 0o700); err != nil {
		t.Fatalf("mkdir relocated backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, relFile), []byte("relocated backup\n"), 0o600); err != nil {
		t.Fatalf("write relocated backup file: %v", err)
	}

	oldBackupPath := filepath.Join(oldBackupDir, relDir)
	manifest := originalDir + "|" + oldBackupPath + "|yes|directory\n"
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write bash manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 1 || len(result.Skipped) != 0 {
		t.Fatalf("Restore result = restored %v skipped %v, want one restored and no skipped", result.Restored, result.Skipped)
	}
	got, err := os.ReadFile(filepath.Join(home, relFile))
	if err != nil {
		t.Fatalf("read restored relocated file: %v", err)
	}
	if string(got) != "relocated backup\n" {
		t.Fatalf("restored relocated file content = %q, want backup content", got)
	}
}

func TestRestoreRefusesManifestlessBackupWithoutLossyDecode(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()

	flatName := ".zsh_history"
	if err := os.WriteFile(filepath.Join(backupDir, flatName), []byte("history\n"), 0o600); err != nil {
		t.Fatalf("write manifestless backup file: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err == nil {
		t.Fatalf("Restore returned nil error for manifestless backup; result=%+v", result)
	}
	if !strings.Contains(err.Error(), "refusing lossy underscore path decode") {
		t.Fatalf("Restore error = %q, want explicit lossy decode refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".zsh", "history")); !os.IsNotExist(statErr) {
		t.Fatalf("manifestless fallback created lossy decoded path, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(home, flatName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifestless fallback restored file despite refusal, stat err=%v", statErr)
	}
}
