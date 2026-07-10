package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestRestoreAppliesRecordedModeWhenOverwritingExistingFile(t *testing.T) {
	home := t.TempDir()
	relPath := filepath.Join(".config", "tool", "settings.json")
	dstPath := filepath.Join(home, relPath)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
		t.Fatalf("mkdir destination: %v", err)
	}
	if err := os.WriteFile(dstPath, []byte("current"), 0o644); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}
	if err := os.Chmod(dstPath, 0o644); err != nil {
		t.Fatalf("chmod existing destination: %v", err)
	}

	backupDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("restored"), 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 1 || len(result.Skipped) != 0 {
		t.Fatalf("Restore result = restored %v skipped %v, want one restored and none skipped", result.Restored, result.Skipped)
	}
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("stat restored destination: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("restored destination mode = %o, want 600", got)
	}
	if content, err := os.ReadFile(dstPath); err != nil || string(content) != "restored" {
		t.Errorf("restored content = %q, err %v, want restored", content, err)
	}
}

func TestRestoreSkipsFinalComponentSymlinkWithoutChangingTarget(t *testing.T) {
	home := t.TempDir()
	relPath := filepath.Join(".config", "tool", "settings.json")
	dstPath := filepath.Join(home, relPath)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
		t.Fatalf("mkdir destination: %v", err)
	}

	outside := t.TempDir()
	target := filepath.Join(outside, "settings.json")
	if err := os.WriteFile(target, []byte("protected"), 0o644); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatalf("chmod symlink target: %v", err)
	}
	if err := os.Symlink(target, dstPath); err != nil {
		t.Fatalf("create destination symlink: %v", err)
	}

	backupDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("restored"), 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 0 {
		t.Fatalf("restored through final symlink: %v", result.Restored)
	}
	if _, ok := result.Skipped[relPath]; !ok {
		t.Fatalf("final symlink not recorded as skipped: %v", result.Skipped)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read protected target: %v", err)
	}
	if string(content) != "protected" {
		t.Errorf("protected target content = %q, want protected", content)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat protected target: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("protected target mode = %o, want 644", got)
	}
}

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

func TestRestoreRegularFileRefusesIntermediateSymlinkInsideHome(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(home, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("mkdir outside sentinel dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".config", "linked")); err != nil {
		t.Fatalf("create intermediate symlink: %v", err)
	}

	relPath := filepath.Join(".config", "linked", "settings.json")
	backupDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("restored"), 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 0 || result.Skipped[relPath] == "" {
		t.Fatalf("intermediate symlink restore result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(outside, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("restore wrote through intermediate symlink, stat err=%v", err)
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

func TestRestoreBashDirectoryEntriesFailClosedWithoutTouchingLivePaths(t *testing.T) {
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
	if result.Count() != 0 || len(result.Removed) != 0 || len(result.Skipped) != 2 {
		t.Fatalf("Restore result = restored %v removed %v skipped %v, want both directory operations skipped",
			result.Restored, result.Removed, result.Skipped)
	}

	gotInit, err := os.ReadFile(filepath.Join(originalDir, "init.lua"))
	if err != nil {
		t.Fatalf("read restored init: %v", err)
	}
	if string(gotInit) != "modified\n" {
		t.Errorf("live init = %q, want untouched modified content", gotInit)
	}
	if _, err := os.Stat(filepath.Join(originalDir, "lua", "plugin.lua")); !os.IsNotExist(err) {
		t.Fatalf("disabled directory restore copied a nested file: %v", err)
	}
	if info, err := os.Stat(createdDir); err != nil || !info.IsDir() {
		t.Errorf("created directory was removed despite fail-closed gate: info=%v err=%v", info, err)
	}
	for _, rel := range []string{filepath.Join(".config", "nvim"), filepath.Join(".config", "generated")} {
		if !strings.Contains(result.Skipped[rel], "unavailable") {
			t.Errorf("skip reason for %s is not actionable: %q", rel, result.Skipped[rel])
		}
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

func TestRestoreBashManifestRelocatedDirectoryAlsoFailsClosed(t *testing.T) {
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
	if result.Count() != 0 || len(result.Skipped) != 1 {
		t.Fatalf("Restore result = restored %v skipped %v, want directory skipped", result.Restored, result.Skipped)
	}
	if _, err := os.Stat(filepath.Join(home, relFile)); !os.IsNotExist(err) {
		t.Fatalf("disabled directory restore created live content: %v", err)
	}
}

func TestRestoreNotExistedFileFailsClosedWithoutRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	backupDir := t.TempDir()
	relPath := filepath.Join(".config", "generated.conf")
	livePath := filepath.Join(home, relPath)
	if err := os.MkdirAll(filepath.Dir(livePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(livePath, []byte("created after backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := livePath + "||no|file\n"
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Count() != 0 || len(result.Removed) != 0 || !strings.Contains(result.Skipped[relPath], "manual review") {
		t.Fatalf("fail-closed removal result = %+v", result)
	}
	content, err := os.ReadFile(livePath)
	if err != nil || string(content) != "created after backup\n" {
		t.Fatalf("created file changed: content=%q err=%v", content, err)
	}
}

func TestRestoreRefusesSymlinkedBackupFileSource(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()
	relPath := ".zshrc"
	outside := filepath.Join(t.TempDir(), "outside-source")
	if err := os.WriteFile(outside, []byte("outside secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(backupDir, EncodeName(relPath))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Count() != 0 || result.Skipped[relPath] == "" {
		t.Fatalf("symlink source result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, relPath)); !os.IsNotExist(err) {
		t.Fatalf("outside source was restored: %v", err)
	}
}

func TestRestoreRefusesIntermediateSymlinkInBackupSource(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()
	relPath := filepath.Join(".config", "tool", "settings.json")
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(backupDir, ".config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "tool"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "tool", "settings.json"), []byte("outside secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(backupDir, ".config", "tool")); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(home, relPath)
	manifest := original + "|ignored-original-backup-path|yes|file\n"
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Count() != 0 || result.Skipped[relPath] == "" {
		t.Fatalf("intermediate source symlink result = %+v", result)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("outside source was restored: %v", err)
	}
}

func TestRestoreRefusesSymlinkedManifest(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()
	outsideManifest := filepath.Join(t.TempDir(), "manifest")
	if err := os.WriteFile(outsideManifest, []byte(ManifestLine(".zshrc", 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideManifest, filepath.Join(backupDir, ManifestName)); err != nil {
		t.Fatal(err)
	}

	if result, err := Restore(backupDir, home); err == nil || result.Count() != 0 {
		t.Fatalf("symlinked manifest was accepted: result=%+v err=%v", result, err)
	}
}

func TestRestoreRefusesSymlinkedBackupDirectory(t *testing.T) {
	home := t.TempDir()
	realBackup := filepath.Join(t.TempDir(), "real-backup")
	if err := os.MkdirAll(realBackup, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realBackup, ".zshrc"), []byte("outside backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realBackup, ManifestName), []byte(ManifestLine(".zshrc", 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}
	linkParent := t.TempDir()
	backupLink := filepath.Join(linkParent, "selected-backup")
	if err := os.Symlink(realBackup, backupLink); err != nil {
		t.Fatal(err)
	}

	if result, err := Restore(backupLink, home); err == nil || result.Count() != 0 {
		t.Fatalf("symlinked backup directory was trusted: result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("symlinked backup directory restored data: %v", err)
	}
}

func TestRestoreRefusesSymlinkedBackupRoot(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	realBackupRoot := t.TempDir()
	selected := filepath.Join(realBackupRoot, "selected")
	if err := os.MkdirAll(selected, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, ".zshrc"), []byte("outside backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, ManifestName), []byte(ManifestLine(".zshrc", 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}
	backupRootLink := filepath.Join(workspace, "backups")
	if err := os.Symlink(realBackupRoot, backupRootLink); err != nil {
		t.Fatal(err)
	}

	if result, err := Restore(filepath.Join(backupRootLink, "selected"), home); err == nil || result.Count() != 0 {
		t.Fatalf("symlinked backup root was trusted: result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("symlinked backup root restored data: %v", err)
	}
}

func TestReadBackupDescendantRejectsDegenerateLayouts(t *testing.T) {
	for _, backupDir := range []string{"relative/selected", filepath.Join(string(os.PathSeparator), "selected")} {
		if _, _, err := readBackupDescendant(backupDir, ManifestName); err == nil {
			t.Errorf("readBackupDescendant(%q) accepted a degenerate layout", backupDir)
		}
	}
}

func TestRestoreCommittedErrorIsRestoredWithWarning(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()
	relPath := ".zshrc"
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("restored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := restoreWithReplace(backupDir, home, func(root, rel string, data []byte, mode os.FileMode) error {
		if err := safefile.ReplaceWithin(root, rel, data, mode); err != nil {
			return err
		}
		return &safefile.CommittedError{Operation: "injected parent fsync", Err: errors.New("injected durability failure")}
	})
	if err != nil {
		t.Fatalf("restoreWithReplace: %v", err)
	}
	if result.Count() != 1 || len(result.Skipped) != 0 || len(result.Warnings) != 1 {
		t.Fatalf("committed result was not reported truthfully: %+v", result)
	}
	if content, err := os.ReadFile(filepath.Join(home, relPath)); err != nil || string(content) != "restored\n" {
		t.Fatalf("committed destination content=%q err=%v", content, err)
	}
}

func TestRestorePrecommitFailurePreservesExistingDestination(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()
	relPath := ".zshrc"
	destination := filepath.Join(home, relPath)
	if err := os.WriteFile(destination, []byte("live content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("backup content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(ManifestLine(relPath, 0o600)), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := restoreWithReplace(backupDir, home, func(string, string, []byte, os.FileMode) error {
		return errors.New("injected precommit failure")
	})
	if err != nil {
		t.Fatalf("restoreWithReplace: %v", err)
	}
	if result.Count() != 0 || len(result.Warnings) != 0 || result.Skipped[relPath] == "" {
		t.Fatalf("precommit failure result = %+v", result)
	}
	if content, err := os.ReadFile(destination); err != nil || string(content) != "live content\n" {
		t.Fatalf("precommit failure changed destination: content=%q err=%v", content, err)
	}
}

func TestExplicitManifestModesAreStrictAndSkippedByRestore(t *testing.T) {
	invalidModes := []string{"", "600junk", "1000", "888", "-1", " 600", "600 "}
	for _, invalidMode := range invalidModes {
		t.Run(fmt.Sprintf("mode_%q", invalidMode), func(t *testing.T) {
			home := t.TempDir()
			backupDir := t.TempDir()
			relPath := ".zshrc"
			destination := filepath.Join(home, relPath)
			if err := os.WriteFile(destination, []byte("live\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("backup\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			manifest := relPath + "\t" + invalidMode
			if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}

			if entries, err := ReadManifest(backupDir); err == nil {
				t.Fatalf("ReadManifest accepted mode %q: entries=%+v", invalidMode, entries)
			}
			result, err := Restore(backupDir, home)
			if err != nil {
				t.Fatalf("Restore: %v", err)
			}
			if result.Count() != 0 || result.Skipped[relPath] == "" {
				t.Fatalf("Restore accepted mode %q: %+v", invalidMode, result)
			}
			if content, err := os.ReadFile(destination); err != nil || string(content) != "live\n" {
				t.Fatalf("invalid mode %q changed destination: content=%q err=%v", invalidMode, content, err)
			}
		})
	}
}

func TestLegacyManifestWithoutModeUses0600(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()
	relPath := ".zshrc"
	if err := os.WriteFile(filepath.Join(backupDir, EncodeName(relPath)), []byte("legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(relPath), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadManifest(backupDir)
	if err != nil || len(entries) != 1 || entries[0].Mode.Perm() != 0o600 {
		t.Fatalf("legacy ReadManifest entries=%+v err=%v", entries, err)
	}
	result, err := Restore(backupDir, home)
	if err != nil || result.Count() != 1 {
		t.Fatalf("legacy Restore result=%+v err=%v", result, err)
	}
	if info, err := os.Stat(filepath.Join(home, relPath)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("legacy restored mode info=%v err=%v", info, err)
	}
}

func TestBashManifestGrammarRejectsUnknownAndExcessFields(t *testing.T) {
	home := t.TempDir()
	relPath := ".zshrc"
	original := filepath.Join(home, relPath)
	invalidLines := []string{
		original + "|backup|yes|socket",
		original + "|backup|yes|file|extra",
		original + "|backup",
	}
	for _, line := range invalidLines {
		t.Run(strings.ReplaceAll(line, string(os.PathSeparator), "_"), func(t *testing.T) {
			backupDir := t.TempDir()
			if err := os.WriteFile(original, []byte("live\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(line), 0o600); err != nil {
				t.Fatal(err)
			}

			if entries, err := ReadManifest(backupDir); err == nil {
				t.Fatalf("ReadManifest accepted malformed Bash line: entries=%+v", entries)
			}
			result, err := Restore(backupDir, home)
			if err != nil {
				t.Fatalf("Restore: %v", err)
			}
			if result.Count() != 0 || len(result.Skipped) != 1 {
				t.Fatalf("Restore accepted malformed Bash line: %+v", result)
			}
			if content, err := os.ReadFile(original); err != nil || string(content) != "live\n" {
				t.Fatalf("malformed Bash line mutated destination: content=%q err=%v", content, err)
			}
		})
	}
}

func TestBashManifestExactThreeFieldFileRemainsCompatible(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()
	relPath := ".zshrc"
	original := filepath.Join(home, relPath)
	backupPath := filepath.Join(backupDir, relPath)
	if err := os.WriteFile(backupPath, []byte("legacy bash\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ManifestName), []byte(original+"|"+backupPath+"|yes"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Restore(backupDir, home)
	if err != nil || result.Count() != 1 || len(result.Skipped) != 0 {
		t.Fatalf("three-field Bash compatibility result=%+v err=%v", result, err)
	}
	if content, err := os.ReadFile(original); err != nil || string(content) != "legacy bash\n" {
		t.Fatalf("three-field Bash restore content=%q err=%v", content, err)
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
