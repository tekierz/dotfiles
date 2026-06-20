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
