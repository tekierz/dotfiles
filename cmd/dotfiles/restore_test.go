package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/backup"
)

// TestRestoreSecurityPathTraversal verifies the shared restore guard blocks
// path-traversal attempts while allowing legitimate in-home paths. The CLI
// restore (restoreBackup) drives the same backup.Restore -> safeJoin guard, so
// testing the exported predicate exercises the real code path rather than a
// re-implementation.
func TestRestoreSecurityPathTraversal(t *testing.T) {
	home := t.TempDir()

	tests := []struct {
		name     string
		relPath  string
		wantSafe bool
	}{
		// Safe relative paths.
		{"simple config file", ".config/dotfiles/settings.json", true},
		{"zshrc", ".zshrc", true},
		{"nested config", ".config/nvim/init.lua", true},
		{"underscore in component", ".config/some_tool/config", true},

		// Unsafe paths (traversal / absolute escape).
		{"parent directory traversal", "../etc/passwd", false},
		{"deep traversal", "../../../../etc/shadow", false},
		{"embedded traversal", ".config/../../etc/passwd", false},
		{"absolute path", "/etc/passwd", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := backup.IsRestorePathSafe(home, tc.relPath); got != tc.wantSafe {
				t.Errorf("IsRestorePathSafe(%q, %q) = %v, want %v",
					home, tc.relPath, got, tc.wantSafe)
			}
		})
	}
}

// TestIsValidBackupName verifies the CLI restore name guard: a backup name must
// be a single path component (== filepath.Base(name)), never empty, absolute, or
// containing ".." or path separators. This prevents path traversal of the
// restore SOURCE directory (config/backups/<name>).
func TestIsValidBackupName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		// Legitimate timestamp-style names.
		{"timestamp", "2026-06-20_15-04-05", true},
		{"plain name", "mybackup", true},

		// Rejected.
		{"empty", "", false},
		{"dot dot", "..", false},
		{"parent traversal", "../../etc", false},
		{"leading traversal", "../backup", false},
		{"absolute path", "/abs/path", false},
		{"separator", "a/b", false},
		{"dot", ".", false},
		{"embedded traversal", "good/../../../etc", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidBackupName(tc.input); got != tc.valid {
				t.Errorf("isValidBackupName(%q) = %v, want %v", tc.input, got, tc.valid)
			}
		})
	}
}

// TestRestoreFilenameRoundTrip is the regression test for cmd-1: a path whose
// component contains a literal underscore must round-trip back to its exact
// original location, not be split into extra directory levels. The manifest is
// the authoritative source for the original path, so a backup written with the
// manifest restores correctly even though the flat filename encoding is lossy.
func TestRestoreFilenameRoundTrip(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()

	// A real-world path with an underscore in a directory component. The old
	// ReplaceAll("_","/") decode would mangle this to ".config/some/tool/config".
	relPath := ".config/some_tool/config"
	content := []byte("user config contents")

	// Write the flat backup file plus a manifest recording the true path/mode.
	if err := os.WriteFile(filepath.Join(backupDir, backup.EncodeName(relPath)), content, 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}
	manifest := backup.ManifestLine(relPath, 0o600)
	if err := os.WriteFile(filepath.Join(backupDir, backup.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := backup.Restore(backupDir, home)
	if err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}
	if result.Count() != 1 {
		t.Fatalf("expected 1 restored file, got %d (skipped: %v)", result.Count(), result.Skipped)
	}

	// The file must land at the exact original relative path under home.
	dst := filepath.Join(home, relPath)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("restored file not at expected path %q: %v", dst, err)
	}
	if string(got) != string(content) {
		t.Errorf("restored content = %q, want %q", got, content)
	}

	// The old lossy decode would have created this wrong path; it must not exist.
	wrong := filepath.Join(home, ".config", "some", "tool", "config")
	if _, err := os.Stat(wrong); err == nil {
		t.Errorf("file restored to corrupted path %q (underscore split into directories)", wrong)
	}
}

// TestRestorePreservesMode is the regression test for cmd-5: restore must honor
// the mode recorded in the manifest (e.g. 0600 for a credential-bearing file)
// rather than hardcoding a world-readable 0644.
func TestRestorePreservesMode(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()

	relPath := ".gitconfig"
	if err := os.WriteFile(filepath.Join(backupDir, backup.EncodeName(relPath)), []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write backup file: %v", err)
	}
	manifest := backup.ManifestLine(relPath, 0o600)
	if err := os.WriteFile(filepath.Join(backupDir, backup.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := backup.Restore(backupDir, home); err != nil {
		t.Fatalf("Restore returned fatal error: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, relPath))
	if err != nil {
		t.Fatalf("stat restored file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("restored file mode = %o, want 0600 (must not be hardcoded 0644)", got)
	}
}

// TestRestoreRefusesManifestlessBackupEndToEnd verifies that a legacy
// manifest-less backup is refused loudly instead of decoding flat filenames
// with the old lossy "_" -> "/" mapping.
func TestRestoreRefusesManifestlessBackupEndToEnd(t *testing.T) {
	home := t.TempDir()
	backupDir := t.TempDir()

	malicious := ".._.._etc_passwd"
	if err := os.WriteFile(filepath.Join(backupDir, malicious), []byte("pwned"), 0o600); err != nil {
		t.Fatalf("write malicious backup file: %v", err)
	}

	result, err := backup.Restore(backupDir, home)
	if err == nil {
		t.Fatalf("Restore returned nil error for manifestless backup; result=%+v", result)
	}
	if !strings.Contains(err.Error(), "refusing lossy underscore path decode") {
		t.Fatalf("Restore error = %q, want explicit lossy decode refusal", err)
	}
}

func TestUninstallPreservesBackupWhenRestoreIsNotClean(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	configDir := filepath.Join(xdg, "dotfiles")
	backupName := "2026-07-04_12-00-00"
	backupDir := filepath.Join(configDir, "backups", backupName)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, backup.ManifestName), []byte(backup.ManifestLine(".zshrc", 0o600)), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	runUninstall(false, true, false, true)

	if _, err := os.Stat(backupDir); err != nil {
		t.Fatalf("backup dir was not preserved after failed restore: %v", err)
	}
	if _, err := os.Stat(configDir); err != nil {
		t.Fatalf("config dir was not preserved after failed restore: %v", err)
	}
}

func TestUninstallPreservesBackupWhenRestoreIsPartial(t *testing.T) {
	home := t.TempDir()
	xdg := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	configDir := filepath.Join(xdg, "dotfiles")
	backupName := "2026-07-04_12-30-00"
	backupDir := filepath.Join(configDir, "backups", backupName)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}

	validRel := ".zshrc"
	if err := os.WriteFile(filepath.Join(backupDir, backup.EncodeName(validRel)), []byte("original zshrc\n"), 0o600); err != nil {
		t.Fatalf("write valid backup file: %v", err)
	}
	missingRel := ".tmux.conf"
	manifest := strings.Join([]string{
		backup.ManifestLine(validRel, 0o600),
		backup.ManifestLine(missingRel, 0o600),
	}, "\n")
	if err := os.WriteFile(filepath.Join(backupDir, backup.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	runUninstall(false, true, false, true)

	if _, err := os.Stat(backupDir); err != nil {
		t.Fatalf("backup dir was not preserved after partial restore: %v", err)
	}
	if _, err := os.Stat(configDir); err != nil {
		t.Fatalf("config dir was not preserved after partial restore: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(home, validRel))
	if err != nil {
		t.Fatalf("valid file was not restored before partial failure accounting: %v", err)
	}
	if string(got) != "original zshrc\n" {
		t.Fatalf("restored valid file = %q, want original zshrc", got)
	}
}

func TestRestoreOutcomeErrorIncludesSkippedAndCommittedWarnings(t *testing.T) {
	result := backup.RestoreResult{
		Skipped: map[string]string{
			".tmux.conf": "source missing",
			".gitconfig": "invalid mode",
		},
		Warnings: map[string]string{
			".zshrc": "committed but parent fsync failed",
		},
	}

	err := restoreOutcomeError(result)
	if err == nil {
		t.Fatal("combined partial restore returned no outcome error")
	}
	message := err.Error()
	if !strings.Contains(message, "2 backup entries could not be restored") ||
		!strings.Contains(message, "1 restored backup entries have durability/verification warnings") {
		t.Fatalf("combined outcome error omitted a category: %q", message)
	}
}
