package operation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCanonicalLockTargetStableAcrossMissingParentCreation(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "real-home")
	linkedHome := filepath.Join(root, "linked-home")
	if err := os.Mkdir(realHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realHome, linkedHome); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", linkedHome)
	t.Setenv("XDG_STATE_HOME", "")
	target := filepath.Join(linkedHome, ".config", "fzf", "fzf.zsh")
	before, err := canonicalLockTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	firstRelease, err := AcquireStateLock("tool-config", target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(realHome, ".config", "fzf", "fzf.zsh")), 0o700); err != nil {
		t.Fatal(err)
	}
	after, err := canonicalLockTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("canonical lock identity changed after parent creation: before=%q after=%q", before, after)
	}
	acquired := make(chan func() error, 1)
	errCh := make(chan error, 1)
	go func() {
		release, err := AcquireStateLock("tool-config", target)
		if err != nil {
			errCh <- err
			return
		}
		acquired <- release
	}()
	select {
	case release := <-acquired:
		_ = release()
		t.Fatal("second spelling acquired a split lock identity while the first was held")
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := firstRelease(); err != nil {
		t.Fatal(err)
	}
	select {
	case release := <-acquired:
		if err := release(); err != nil {
			t.Fatal(err)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("second lock did not acquire after the first was released")
	}
	entries, err := os.ReadDir(filepath.Join(realHome, ".local", "state", "dotfiles", "locks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("same target used %d operation lock identities, want one", len(entries))
	}
}

func TestAcquireStateLockUsesPrivateOperationalNamespace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	target := filepath.Join(home, ".config", "fzf", "fzf.zsh")
	release, err := AcquireStateLock("tool-config", target)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target + ".dotfiles.lock"); !os.IsNotExist(err) {
		t.Fatalf("adjacent user-config lock exists: %v", err)
	}
	locks := filepath.Join(home, ".local", "state", "dotfiles", "locks")
	info, err := os.Stat(locks)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("lock directory mode = %04o, want 0700", info.Mode().Perm())
	}
	entries, err := os.ReadDir(locks)
	if err != nil || len(entries) != 1 {
		t.Fatalf("operation lock entries = %v err=%v", entries, err)
	}
	lockInfo, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if lockInfo.Mode().Perm() != 0o600 {
		t.Fatalf("operation lock mode = %04o, want 0600", lockInfo.Mode().Perm())
	}
}
