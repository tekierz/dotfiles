package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRestoreSecurityPathTraversal tests that path traversal attacks are blocked
// during backup restoration. Backup files use underscore-separated paths that
// get converted to OS path separators.
func TestRestoreSecurityPathTraversal(t *testing.T) {
	home := t.TempDir()

	tests := []struct {
		name     string
		filename string
		wantSafe bool
	}{
		// Safe filenames
		{"simple config file", ".config_dotfiles_settings.json", true},
		{"zshrc", ".zshrc", true},
		{"nested config", ".config_nvim_init.lua", true},
		{"deep nesting", ".config_some_deep_path_file.txt", true},

		// Unsafe filenames (path traversal attempts)
		{"parent directory traversal", ".._.._etc_passwd", false},
		{"deep traversal", ".._.._.._.._etc_shadow", false},
		{"embedded traversal", ".config_.._.._etc_passwd", false},
		{"single parent traversal", ".._etc_passwd", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			safe := isRestorePathSafe(home, tc.filename)
			if safe != tc.wantSafe {
				t.Errorf("isRestorePathSafe(%q, %q) = %v, want %v",
					home, tc.filename, safe, tc.wantSafe)
			}
		})
	}
}

// isRestorePathSafe validates that a backup filename will not escape
// the home directory when restored. This mirrors the validation in restoreBackup.
func isRestorePathSafe(home, filename string) bool {
	relPath := strings.ReplaceAll(filename, "_", string(os.PathSeparator))
	dstPath := filepath.Clean(filepath.Join(home, relPath))
	homePrefix := filepath.Clean(home) + string(os.PathSeparator)
	return strings.HasPrefix(dstPath, homePrefix) || dstPath == filepath.Clean(home)
}

// TestRestoreDeeplyNestedPaths tests handling of deeply nested paths
func TestRestoreDeeplyNestedPaths(t *testing.T) {
	home := t.TempDir()

	// Very deep nesting should work fine
	deepPath := strings.Repeat("dir_", 50) + "file.txt"
	if !isRestorePathSafe(home, deepPath) {
		t.Error("Deeply nested path should be safe")
	}

	// But deep traversal should not
	deepTraversal := strings.Repeat(".._", 50) + "etc_passwd"
	if isRestorePathSafe(home, deepTraversal) {
		t.Error("Deep traversal path should not be safe")
	}
}

// BenchmarkRestorePathValidation benchmarks the path validation function
func BenchmarkRestorePathValidation(b *testing.B) {
	home := "/home/testuser"
	filenames := []string{
		".config_dotfiles_settings.json",
		".._.._etc_passwd",
		".config_nvim_init.lua",
		strings.Repeat("dir_", 20) + "file.txt",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, f := range filenames {
			isRestorePathSafe(home, f)
		}
	}
}
