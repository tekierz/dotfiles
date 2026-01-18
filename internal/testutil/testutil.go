// Package testutil provides common testing utilities for the dotfiles project.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempConfigDir creates a temporary config directory for testing.
// It returns the path and sets XDG_CONFIG_HOME to point to it.
// The directory is automatically cleaned up when the test completes.
func TempConfigDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "dotfiles")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create temp config dir: %v", err)
	}

	// Store original value
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	origHome := os.Getenv("HOME")

	// Set env vars for test
	configHome := filepath.Join(dir, ".config")
	os.Setenv("XDG_CONFIG_HOME", configHome)
	os.Setenv("HOME", dir)

	// Cleanup after test
	t.Cleanup(func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
		if origHome != "" {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
	})

	return configDir
}

// CreateTempFile creates a temporary file with the given content.
// Returns the full path to the created file.
func CreateTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	parentDir := filepath.Dir(path)

	if err := os.MkdirAll(parentDir, 0700); err != nil {
		t.Fatalf("failed to create parent dir for %s: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file %s: %v", path, err)
	}

	return path
}

// MustReadFile reads a file or fails the test.
func MustReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	return string(data)
}

// FileExists returns true if the file exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DirExists returns true if the directory exists.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// RequireEqual fails the test if expected != actual.
func RequireEqual(t *testing.T, expected, actual interface{}, msg string) {
	t.Helper()
	if expected != actual {
		t.Fatalf("%s: expected %v, got %v", msg, expected, actual)
	}
}

// RequireNoError fails the test if err != nil.
func RequireNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", msg, err)
	}
}

// RequireError fails the test if err == nil.
func RequireError(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error but got nil", msg)
	}
}

// RequireContains fails if substring is not in s.
func RequireContains(t *testing.T, s, substring, msg string) {
	t.Helper()
	if len(substring) == 0 {
		return
	}
	for i := 0; i <= len(s)-len(substring); i++ {
		if s[i:i+len(substring)] == substring {
			return
		}
	}
	t.Fatalf("%s: %q does not contain %q", msg, s, substring)
}
