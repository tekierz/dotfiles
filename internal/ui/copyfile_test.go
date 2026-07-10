package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCopyFilePermsNeverGroupOrOtherWritable verifies copyFile creates the
// destination with restrictive permissions and that the file is never group- or
// other-writable at any point. The destination is created with explicit perms
// (not umask-default 0666 minus umask) so it cannot be momentarily broad.
func TestCopyFilePermsNeverGroupOrOtherWritable(t *testing.T) {
	dir := t.TempDir()

	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("binary contents"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dst := filepath.Join(dir, "dst")
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	mode := info.Mode().Perm()

	// Group-writable (0o020) or other-writable (0o002) must never be set.
	if mode&0o022 != 0 {
		t.Errorf("copyFile produced group/other-writable file: mode = %04o", mode)
	}
	// The destination is created with explicit owner-only perms (0700), so no
	// group/other bits at all — never momentarily broad before the caller's
	// final chmod. (os.Create's umask-default 0666 would leave group/other read
	// bits, which this asserts against.)
	if mode&0o077 != 0 {
		t.Errorf("copyFile created file with group/other bits: mode = %04o, want owner-only", mode)
	}
	// Contents must round-trip.
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "binary contents" {
		t.Errorf("copied contents = %q, want %q", got, "binary contents")
	}
}

func TestInstallBinaryCopyFailureLeavesExistingBinaryUntouched(t *testing.T) {
	dir := t.TempDir()

	dest := filepath.Join(dir, "dotfiles")
	original := []byte("original binary")
	if err := os.WriteFile(dest, original, 0o755); err != nil {
		t.Fatalf("write dest: %v", err)
	}

	err := installBinary(filepath.Join(dir, "missing-source"), dest)
	if err == nil {
		t.Fatal("installBinary succeeded with missing source")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("dest contents = %q, want %q", got, original)
	}

	matches, err := filepath.Glob(filepath.Join(dir, ".dotfiles-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

// TestInstallBinarySelfReplace covers the original audit scenario: the running
// binary replacing itself (execPath == destPath). The staged-temp-plus-rename
// flow must leave the binary intact and executable with no temp litter.
func TestInstallBinarySelfReplace(t *testing.T) {
	dir := t.TempDir()

	dest := filepath.Join(dir, "dotfiles")
	contents := []byte("running binary")
	if err := os.WriteFile(dest, contents, 0o700); err != nil {
		t.Fatalf("write dest: %v", err)
	}

	if err := installBinary(dest, dest); err != nil {
		t.Fatalf("installBinary self-replace: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(contents) {
		t.Errorf("dest contents = %q, want %q", got, contents)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("dest mode = %v, want 0700", info.Mode().Perm())
	}

	matches, err := filepath.Glob(filepath.Join(dir, ".dotfiles-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}
}

func TestInstallUtilitiesLeavesPackageManagerAndLegacyBinariesUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinels := map[string]string{
		"dotfiles":       "package-manager-owned",
		"dotfiles-tui":   "unverified-legacy-name",
		"dotfiles-setup": "unverified-legacy-name",
	}
	for name, content := range sentinels {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if err := installUtilities(map[string]bool{}); err != nil {
		t.Fatalf("installUtilities: %v", err)
	}
	for name, want := range sentinels {
		got, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s changed: content=%q err=%v", name, got, err)
		}
	}
}

func TestInstallScriptFileRefusesSymlinkedDescendant(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".local")); err != nil {
		t.Fatal(err)
	}

	err := installScriptFile(home, "hk", []byte("#!/bin/sh\n"))
	if err == nil {
		t.Fatal("installScriptFile followed symlinked .local directory")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "bin", "hk")); !os.IsNotExist(statErr) {
		t.Fatalf("helper escaped trusted HOME: %v", statErr)
	}
}
