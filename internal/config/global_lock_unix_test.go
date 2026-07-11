//go:build darwin || linux

package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func setupGlobalConfigLockTest(t *testing.T) (*GlobalConfig, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ConfigDir(), "global.json")
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, root, globalConfigLockRel(rel)
}

func TestGlobalConfigLockRejectsSymlink(t *testing.T) {
	cfg, root, lockRel := setupGlobalConfigLockTest(t)
	victim := filepath.Join(t.TempDir(), "victim")
	original := []byte("do not touch")
	if err := os.WriteFile(victim, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(root, lockRel)); err != nil {
		t.Fatal(err)
	}

	err := SaveGlobalConfig(cfg)
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("SaveGlobalConfig error = %v, want safefile.ErrSymlink", err)
	}
	got, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("symlink victim changed: got %q, want %q", got, original)
	}
	info, statErr := os.Stat(victim)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink victim mode = %04o, want 0644", info.Mode().Perm())
	}
}

func TestGlobalConfigLockRejectsHardlinkBeforeChmod(t *testing.T) {
	cfg, root, lockRel := setupGlobalConfigLockTest(t)
	victim := filepath.Join(t.TempDir(), "victim")
	original := []byte("do not touch")
	if err := os.WriteFile(victim, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(victim, filepath.Join(root, lockRel)); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}

	err := SaveGlobalConfig(cfg)
	if !errors.Is(err, safefile.ErrHardlink) {
		t.Fatalf("SaveGlobalConfig error = %v, want safefile.ErrHardlink", err)
	}
	got, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("hardlink victim changed: got %q, want %q", got, original)
	}
	info, statErr := os.Stat(victim)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("hardlink victim mode = %04o, want 0644", info.Mode().Perm())
	}
}

func TestGlobalConfigLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_GLOBAL_CONFIG_LOCK_HELPER") != "1" {
		return
	}
	lockRoot := os.Getenv("GO_GLOBAL_CONFIG_LOCK_ROOT")
	lockRel := os.Getenv("GO_GLOBAL_CONFIG_LOCK_REL")
	startedPath := os.Getenv("GO_GLOBAL_CONFIG_LOCK_STARTED")
	acquiredPath := os.Getenv("GO_GLOBAL_CONFIG_LOCK_ACQUIRED")
	if err := os.WriteFile(startedPath, []byte("started"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := safefile.AcquireLockWithin(lockRoot, lockRel, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	if err := os.WriteFile(acquiredPath, []byte("acquired"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGlobalConfigLockSerializesCooperatingProcesses(t *testing.T) {
	dir := t.TempDir()
	lockRel := ".global.json.lock"
	startedPath := filepath.Join(dir, "helper-started")
	acquiredPath := filepath.Join(dir, "helper-acquired")

	release, err := safefile.AcquireLockWithin(dir, lockRel, 0o600)
	if err != nil {
		t.Fatalf("acquire parent lock: %v", err)
	}
	released := false
	defer func() {
		if !released {
			_ = release()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// #nosec G204 -- os.Args[0] re-runs this test binary with a fixed helper selector.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGlobalConfigLockHelperProcess$")
	cmd.Env = append(os.Environ(),
		"GO_GLOBAL_CONFIG_LOCK_HELPER=1",
		"GO_GLOBAL_CONFIG_LOCK_ROOT="+dir,
		"GO_GLOBAL_CONFIG_LOCK_REL="+lockRel,
		"GO_GLOBAL_CONFIG_LOCK_STARTED="+startedPath,
		"GO_GLOBAL_CONFIG_LOCK_ACQUIRED="+acquiredPath,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForPath(t, startedPath, 2*time.Second)
	// The helper announces immediately before attempting flock. Give it time to
	// enter the syscall, then prove it cannot pass while the parent holds it.
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(acquiredPath); !os.IsNotExist(err) {
		t.Fatalf("helper acquired lock before release, stat error = %v", err)
	}

	_ = release()
	released = true
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper process: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("helper timed out: %v", ctx.Err())
	}
	waitForPath(t, acquiredPath, time.Second)
}

func TestGlobalConfigFirstCreationKeepsExternalXDGLockAnchorStable(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	externalXDG := filepath.Join(workspace, "external-xdg")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", externalXDG)

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	callbackEntered := make(chan struct{})
	callbackRelease := make(chan struct{})
	callbackReleased := false
	defer func() {
		if !callbackReleased {
			close(callbackRelease)
		}
	}()
	saveDone := make(chan error, 1)
	go func() {
		saveDone <- SaveGlobalConfigWithReservedRevision(cfg, func() error {
			// Model a dependent-state callback such as ManageSettings.Save. Under
			// the old implementation this created XDG_CONFIG_HOME only after the
			// global lock had been acquired at its existing ancestor.
			if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
				return err
			}
			close(callbackEntered)
			<-callbackRelease
			return nil
		})
	}()
	select {
	case <-callbackEntered:
	case err := <-saveDone:
		t.Fatalf("save returned before callback: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reserved callback")
	}

	path := filepath.Join(ConfigDir(), "global.json")
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(root) != filepath.Clean(externalXDG) {
		t.Fatalf("post-creation root = %q, want XDG root %q", root, externalXDG)
	}
	lockRel := globalConfigLockRel(rel)
	startedPath := filepath.Join(workspace, "helper-started")
	acquiredPath := filepath.Join(workspace, "helper-acquired")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// #nosec G204 -- os.Args[0] re-runs this test binary with a fixed helper selector.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGlobalConfigLockHelperProcess$")
	cmd.Env = append(os.Environ(),
		"GO_GLOBAL_CONFIG_LOCK_HELPER=1",
		"GO_GLOBAL_CONFIG_LOCK_ROOT="+root,
		"GO_GLOBAL_CONFIG_LOCK_REL="+lockRel,
		"GO_GLOBAL_CONFIG_LOCK_STARTED="+startedPath,
		"GO_GLOBAL_CONFIG_LOCK_ACQUIRED="+acquiredPath,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForPath(t, startedPath, 2*time.Second)
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(acquiredPath); !os.IsNotExist(err) {
		t.Fatalf("helper acquired post-creation lock before first writer released it, stat error = %v", err)
	}

	close(callbackRelease)
	callbackReleased = true
	if err := <-saveDone; err != nil {
		t.Fatalf("save global config: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper process: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("helper timed out: %v", ctx.Err())
	}
	waitForPath(t, acquiredPath, time.Second)
}

func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
