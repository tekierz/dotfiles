//go:build darwin || linux

package safefile

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestEnsureDirectoryWithinCreatesPrivateTreeAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := EnsureDirectoryWithin(root, "one/two/three", 0o700); err != nil {
		t.Fatalf("EnsureDirectoryWithin create: %v", err)
	}
	for _, rel := range []string{"one", "one/two", "one/two/three"} {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode/type = %v, want private directory", rel, info.Mode())
		}
	}
	if err := os.Chmod(filepath.Join(root, "one/two"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDirectoryWithin(root, "one/two/three", 0o700); err != nil {
		t.Fatalf("EnsureDirectoryWithin existing: %v", err)
	}
	assertMode(t, filepath.Join(root, "one/two"), 0o750)
}

func TestEnsureDirectoryWithinRefusesSymlinkAndInvalidMode(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, outside, 0o700)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDirectoryWithin(root, "linked/private", 0o700); !errors.Is(err, ErrSymlink) {
		t.Fatalf("intermediate symlink error = %v, want ErrSymlink", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "private")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("outside directory was touched: %v", err)
	}
	if err := EnsureDirectoryWithin(root, "invalid", 0o755); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("invalid mode error = %v, want ErrInvalidMode", err)
	}
	if err := EnsureDirectoryWithin(root, "invalid-bits", fs.ModeDir|0o700); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("invalid type-bit mode error = %v, want ErrInvalidMode", err)
	}
}

func TestEnsureDirectoryWithinAllowsTrustedRootSymlinkAndRefusesFileLeaf(t *testing.T) {
	workspace := t.TempDir()
	realRoot := filepath.Join(workspace, "real")
	mustMkdir(t, realRoot, 0o700)
	rootLink := filepath.Join(workspace, "trusted")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDirectoryWithin(rootLink, "private/nested", 0o700); err != nil {
		t.Fatalf("trusted root symlink: %v", err)
	}
	mustWrite(t, filepath.Join(realRoot, "file"), "value", 0o600)
	if err := EnsureDirectoryWithin(rootLink, "file", 0o700); err == nil {
		t.Fatal("file leaf was accepted as a directory")
	}
}

func TestReadWithinAbsentAndComparableRevision(t *testing.T) {
	root := t.TempDir()
	data, missing, err := ReadWithin(root, "missing/parents/config")
	if err != nil {
		t.Fatalf("ReadWithin missing: %v", err)
	}
	if data != nil || !missing.Tracked() || missing.Exists() || missing.Permissions() != 0 {
		t.Fatalf("missing read = data %v revision %+v", data, missing)
	}

	mustWrite(t, filepath.Join(root, "config"), "value", 0o640)
	firstData, first, err := ReadWithin(root, "config")
	if err != nil {
		t.Fatalf("ReadWithin existing: %v", err)
	}
	secondData, second, err := ReadWithin(root, "config")
	if err != nil {
		t.Fatalf("second ReadWithin: %v", err)
	}
	if string(firstData) != "value" || string(secondData) != "value" {
		t.Fatalf("read data = %q/%q", firstData, secondData)
	}
	if !first.Tracked() || !first.Exists() || first != second {
		t.Fatalf("stable reads produced revisions %+v and %+v", first, second)
	}
	if first.Permissions() != 0o640 {
		t.Fatalf("Revision.Permissions() = %04o, want 0640", first.Permissions())
	}
	if first == missing {
		t.Fatal("existing and missing revisions compare equal")
	}
}

func TestReadWithinAllowsTrustedRootSymlink(t *testing.T) {
	workspace := t.TempDir()
	realRoot := filepath.Join(workspace, "real")
	mustMkdir(t, realRoot, 0o700)
	mustWrite(t, filepath.Join(realRoot, "config"), "value", 0o600)
	rootLink := filepath.Join(workspace, "trusted")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Fatal(err)
	}

	data, revision, err := ReadWithin(rootLink, "config")
	if err != nil {
		t.Fatalf("ReadWithin trusted root symlink: %v", err)
	}
	if string(data) != "value" || !revision.Exists() {
		t.Fatalf("read = %q revision %+v", data, revision)
	}
}

func TestReadWithinRefusesSymlinksAndNonRegularLeaves(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, outside, 0o700)
	mustWrite(t, filepath.Join(outside, "config"), "outside", 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadWithin(root, "linked/config"); !errors.Is(err, ErrSymlink) {
		t.Fatalf("intermediate symlink error = %v, want ErrSymlink", err)
	}
	if err := os.Symlink(filepath.Join(outside, "config"), filepath.Join(root, "leaf")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadWithin(root, "leaf"); !errors.Is(err, ErrSymlink) {
		t.Fatalf("leaf symlink error = %v, want ErrSymlink", err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadWithin(root, "fifo"); !errors.Is(err, ErrNonRegular) {
		t.Fatalf("FIFO error = %v, want ErrNonRegular", err)
	}
	assertContent(t, filepath.Join(outside, "config"), "outside")
}

func TestReadWithinReplacementKeepsDescriptorBytesAndIdentity(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	moved := filepath.Join(root, "opened-source")
	mustWrite(t, target, "old", 0o600)
	replaced := false
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterReadOpen = func(_ int, _ int, _ string) error {
			if replaced {
				return nil
			}
			replaced = true
			if err := os.Rename(target, moved); err != nil {
				return err
			}
			return os.WriteFile(target, []byte("new"), 0o600)
		}
	})

	oldData, oldRevision, err := ReadWithin(root, "config")
	if err != nil {
		t.Fatalf("ReadWithin replaced path: %v", err)
	}
	newData, newRevision, err := ReadWithin(root, "config")
	if err != nil {
		t.Fatalf("ReadWithin replacement: %v", err)
	}
	if string(oldData) != "old" || string(newData) != "new" {
		t.Fatalf("descriptor/path reads = %q/%q, want old/new", oldData, newData)
	}
	if oldRevision == newRevision {
		t.Fatal("replacement inode produced equal revisions")
	}
}

func TestReadWithinDetectsInPlaceChangeAndClosesFD(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "old", 0o600)
	openedFD := -1
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterReadOpen = func(parentFD, fileFD int, name string) error {
			openedFD = fileFD
			writer, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_TRUNC|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			if _, err := unix.Write(writer, []byte("changed-and-longer")); err != nil {
				_ = unix.Close(writer)
				return err
			}
			return unix.Close(writer)
		}
	})

	if _, _, err := ReadWithin(root, "config"); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("ReadWithin changed file error = %v, want ErrRevisionChanged", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(openedFD, &stat); !errors.Is(err, unix.EBADF) {
		t.Fatalf("read descriptor still open after return: %v", err)
	}
}

func TestAcquireLockWithinRequiresExistingParents(t *testing.T) {
	root := t.TempDir()
	if _, err := AcquireLockWithin(root, "missing/lock", 0o600); !errors.Is(err, unix.ENOENT) {
		t.Fatalf("AcquireLockWithin missing parent error = %v, want ENOENT", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("AcquireLockWithin created descendant parent: %v", err)
	}
}

func TestAcquireLockWithinRefusesSymlinksAndNonRegular(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, outside, 0o700)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLockWithin(root, "linked/lock", 0o600); !errors.Is(err, ErrSymlink) {
		t.Fatalf("intermediate symlink error = %v, want ErrSymlink", err)
	}
	outsideFile := filepath.Join(outside, "file")
	mustWrite(t, outsideFile, "outside", 0o644)
	if err := os.Symlink(outsideFile, filepath.Join(root, "lock-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLockWithin(root, "lock-link", 0o600); !errors.Is(err, ErrSymlink) {
		t.Fatalf("lock symlink error = %v, want ErrSymlink", err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLockWithin(root, "fifo", 0o600); !errors.Is(err, ErrNonRegular) {
		t.Fatalf("lock FIFO error = %v, want ErrNonRegular", err)
	}
	assertContent(t, outsideFile, "outside")
	assertMode(t, outsideFile, 0o644)
}

func TestAcquireLockWithinRefusesHardlinkBeforeChmod(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	mustMkdir(t, root, 0o700)
	outside := filepath.Join(workspace, "outside")
	mustWrite(t, outside, "outside", 0o644)
	if err := os.Chmod(outside, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(root, "lock")); err != nil {
		t.Fatal(err)
	}

	if _, err := AcquireLockWithin(root, "lock", 0o600); !errors.Is(err, ErrHardlink) {
		t.Fatalf("AcquireLockWithin hardlink error = %v, want ErrHardlink", err)
	}
	assertMode(t, outside, 0o644)
	assertContent(t, outside, "outside")
}

func TestAcquireLockWithinSetsExactModeAndValidatesMode(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, "lock")
	mustWrite(t, lockPath, "", 0o644)
	if err := os.Chmod(lockPath, 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireLockWithin(root, "lock", 0o600)
	if err != nil {
		t.Fatalf("AcquireLockWithin existing: %v", err)
	}
	assertMode(t, lockPath, 0o600)
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := AcquireLockWithin(root, "invalid", fs.ModeDir|0o600); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("invalid mode error = %v, want ErrInvalidMode", err)
	}
}

func TestAcquireLockWithinDetectsNamespaceReplacement(t *testing.T) {
	root := t.TempDir()
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterLock = func(parentFD, _ int, name string) error {
			if err := unix.Unlinkat(parentFD, name, 0); err != nil {
				return err
			}
			fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
			if err != nil {
				return err
			}
			if _, err := unix.Write(fd, []byte("replacement")); err != nil {
				_ = unix.Close(fd)
				return err
			}
			return unix.Close(fd)
		}
	})

	if _, err := AcquireLockWithin(root, "lock", 0o600); !errors.Is(err, ErrLockChanged) {
		t.Fatalf("namespace replacement error = %v, want ErrLockChanged", err)
	}
	assertContent(t, filepath.Join(root, "lock"), "replacement")
}

func TestAcquireLockWithinDetectsParentReplacementWithoutOutsideTouch(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	parent := filepath.Join(root, "parent")
	moved := filepath.Join(root, "moved")
	outside := filepath.Join(workspace, "outside")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, parent, 0o700)
	mustMkdir(t, outside, 0o700)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterLock = func(_, _ int, _ string) error {
			if err := os.Rename(parent, moved); err != nil {
				return err
			}
			return os.Symlink(outside, parent)
		}
	})

	if _, err := AcquireLockWithin(root, "parent/lock", 0o600); !errors.Is(err, ErrParentChanged) {
		t.Fatalf("parent replacement error = %v, want ErrParentChanged", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "lock")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("outside lock changed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(moved, "lock")); err != nil {
		t.Fatalf("held-parent lock missing: %v", err)
	}
}

func TestAcquireLockWithinReleaseIsConcurrentIdempotentAndClosesFD(t *testing.T) {
	root := t.TempDir()
	lockFD := -1
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterLock = func(_, fd int, _ string) error {
			lockFD = fd
			return nil
		}
	})
	release, err := AcquireLockWithin(root, "lock", 0o600)
	if err != nil {
		t.Fatalf("AcquireLockWithin: %v", err)
	}

	const callers = 16
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- release()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("idempotent release error: %v", err)
		}
	}
	var stat unix.Stat_t
	if err := unix.Fstat(lockFD, &stat); !errors.Is(err, unix.EBADF) {
		t.Fatalf("lock descriptor still open after release: %v", err)
	}
}

func TestAcquireLockWithinSerializesGoroutines(t *testing.T) {
	root := t.TempDir()
	const workers = 12
	var active int32
	var overlap int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release, err := AcquireLockWithin(root, "lock", 0o600)
			if err != nil {
				errs <- err
				return
			}
			if atomic.AddInt32(&active, 1) != 1 {
				atomic.StoreInt32(&overlap, 1)
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&active, -1)
			errs <- release()
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent lock error: %v", err)
		}
	}
	if atomic.LoadInt32(&overlap) != 0 {
		t.Fatal("lock allowed overlapping goroutine critical sections")
	}
}

func TestAcquireLockWithinHelperProcess(t *testing.T) {
	if os.Getenv("SAFEFILE_LOCK_HELPER") != "1" {
		return
	}
	root := os.Getenv("SAFEFILE_LOCK_ROOT")
	started := os.Getenv("SAFEFILE_LOCK_STARTED")
	acquired := os.Getenv("SAFEFILE_LOCK_ACQUIRED")
	if err := os.WriteFile(started, []byte("started"), 0o600); err != nil {
		t.Fatal(err)
	}
	release, err := AcquireLockWithin(root, "lock", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer release() //nolint:errcheck
	if err := os.WriteFile(acquired, []byte("acquired"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireLockWithinSerializesChildProcess(t *testing.T) {
	root := t.TempDir()
	release, err := AcquireLockWithin(root, "lock", 0o600)
	if err != nil {
		t.Fatalf("parent acquire: %v", err)
	}
	released := false
	defer func() {
		if !released {
			_ = release()
		}
	}()
	started := filepath.Join(root, "started")
	acquired := filepath.Join(root, "acquired")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAcquireLockWithinHelperProcess$")
	cmd.Env = append(os.Environ(),
		"SAFEFILE_LOCK_HELPER=1",
		"SAFEFILE_LOCK_ROOT="+root,
		"SAFEFILE_LOCK_STARTED="+started,
		"SAFEFILE_LOCK_ACQUIRED="+acquired,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitForSafefilePath(t, started, 2*time.Second)
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(acquired); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("child acquired before release: %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("parent release: %v", err)
	}
	released = true
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child process: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("child timed out: %v", ctx.Err())
	}
	waitForSafefilePath(t, acquired, time.Second)
}

func waitForSafefilePath(t *testing.T, path string, timeout time.Duration) {
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

func TestRevisionIsComparable(t *testing.T) {
	// This compile-time-style check keeps Revision value-only/comparable.
	var a, b Revision
	_ = a == b
}

func TestRemoveWithinDeletesOnlyNamedRegularLinkAndClosesDescriptor(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "nested", "profile.json")
	alias := filepath.Join(root, "alias.json")
	if err := os.Mkdir(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, target, "profile", 0o600)
	if err := os.Link(target, alias); err != nil {
		t.Fatal(err)
	}
	openedFD := -1
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.beforeRemove = func(_, fd int, _ string) error {
			openedFD = fd
			return nil
		}
	})

	if err := RemoveWithin(root, "nested/profile.json"); err != nil {
		t.Fatalf("RemoveWithin: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target still exists: %v", err)
	}
	assertContent(t, alias, "profile")
	var stat unix.Stat_t
	if err := unix.Fstat(openedFD, &stat); !errors.Is(err, unix.EBADF) {
		t.Fatalf("removed descriptor still open: %v", err)
	}
}

func TestRemoveWithinRejectsInvalidSymlinkAndNonRegularTargets(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, outside, 0o700)
	mustWrite(t, filepath.Join(outside, "victim"), "outside", 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "victim"), filepath.Join(root, "leaf")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}

	for name, rel := range map[string]string{
		"invalid":              "../outside/victim",
		"intermediate symlink": "linked/victim",
		"leaf symlink":         "leaf",
		"fifo":                 "fifo",
		"directory":            "directory",
	} {
		t.Run(name, func(t *testing.T) {
			err := RemoveWithin(root, rel)
			switch name {
			case "invalid":
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("error = %v, want ErrInvalidPath", err)
				}
			case "intermediate symlink", "leaf symlink":
				if !errors.Is(err, ErrSymlink) {
					t.Fatalf("error = %v, want ErrSymlink", err)
				}
			default:
				if !errors.Is(err, ErrNonRegular) {
					t.Fatalf("error = %v, want ErrNonRegular", err)
				}
			}
		})
	}
	assertContent(t, filepath.Join(outside, "victim"), "outside")
}

func TestRemoveWithinDetectsEntrySubstitutionBeforeUnlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "profile.json")
	mustWrite(t, target, "original", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.beforeRemove = func(parentFD, _ int, name string) error {
			if err := unix.Unlinkat(parentFD, name, 0); err != nil {
				return err
			}
			fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
			if err != nil {
				return err
			}
			if _, err := unix.Write(fd, []byte("replacement")); err != nil {
				_ = unix.Close(fd)
				return err
			}
			return unix.Close(fd)
		}
	})

	err := RemoveWithin(root, "profile.json")
	if !errors.Is(err, ErrTargetChanged) {
		t.Fatalf("RemoveWithin error = %v, want ErrTargetChanged", err)
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		t.Fatalf("pre-unlink substitution reported committed: %v", err)
	}
	assertContent(t, target, "replacement")
}

func TestRemoveWithinDetectsParentSwapBeforeUnlinkWithoutOutsideTouch(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	parent := filepath.Join(root, "profiles")
	moved := filepath.Join(root, "moved")
	outside := filepath.Join(workspace, "outside")
	for _, dir := range []string{root, parent, outside} {
		mustMkdir(t, dir, 0o700)
	}
	mustWrite(t, filepath.Join(parent, "alice.json"), "inside", 0o600)
	mustWrite(t, filepath.Join(outside, "alice.json"), "outside", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.beforeRemove = func(_, _ int, _ string) error {
			if err := os.Rename(parent, moved); err != nil {
				return err
			}
			return os.Symlink(outside, parent)
		}
	})

	err := RemoveWithin(root, "profiles/alice.json")
	if !errors.Is(err, ErrParentChanged) {
		t.Fatalf("RemoveWithin error = %v, want ErrParentChanged", err)
	}
	assertContent(t, filepath.Join(moved, "alice.json"), "inside")
	assertContent(t, filepath.Join(outside, "alice.json"), "outside")
}

func TestRemoveWithinPostRemoveFailureReportsCommittedState(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "profile.json")
	mustWrite(t, target, "original", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterRemove = func(parentFD, _ int, name string) error {
			fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
			if err != nil {
				return err
			}
			if _, err := unix.Write(fd, []byte("new actor")); err != nil {
				_ = unix.Close(fd)
				return err
			}
			return unix.Close(fd)
		}
	})

	err := RemoveWithin(root, "profile.json")
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, ErrTargetChanged) {
		t.Fatalf("RemoveWithin error = %v, want committed ErrTargetChanged", err)
	}
	assertContent(t, target, "new actor")
}

func TestRemoveWithinParentFsyncFailureReportsCommittedState(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "profile.json")
	mustWrite(t, target, "original", 0o600)
	wantErr := errors.New("injected remove parent fsync failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.fsyncDir = func(_ int, operation string) error {
			if operation == "remove parent" {
				return wantErr
			}
			return nil
		}
	})

	err := RemoveWithin(root, "profile.json")
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, wantErr) {
		t.Fatalf("RemoveWithin error = %v, want committed fsync error", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target exists after committed removal: %v", err)
	}
}

func TestRemoveWithinCloseFailureReportsCommittedAndDescriptorClosed(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "profile.json")
	mustWrite(t, target, "original", 0o600)
	wantErr := errors.New("injected removed-target close failure")
	openedFD := -1
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.beforeRemove = func(_, fd int, _ string) error {
			openedFD = fd
			return nil
		}
		hooks.closeRemoved = func(fd int) error {
			if err := unix.Close(fd); err != nil {
				return err
			}
			return wantErr
		}
	})

	err := RemoveWithin(root, "profile.json")
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, wantErr) {
		t.Fatalf("RemoveWithin error = %v, want committed close error", err)
	}
	if !strings.Contains(err.Error(), "safe file operation committed") {
		t.Fatalf("committed removal error text is not operation-neutral: %v", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(openedFD, &stat); !errors.Is(err, unix.EBADF) {
		t.Fatalf("removed descriptor remains open after close error: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target exists after committed removal: %v", err)
	}
}

func TestRemoveWithinMissingTargetsAreNonCommittedNotExist(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"missing.json", "missing/parent/profile.json"} {
		t.Run(rel, func(t *testing.T) {
			err := RemoveWithin(root, rel)
			if !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("RemoveWithin error = %v, want fs.ErrNotExist", err)
			}
			var committed *CommittedError
			if errors.As(err, &committed) {
				t.Fatalf("missing target reported committed: %v", err)
			}
		})
	}
}

func TestRemoveWithinRemovesUnreadableRegularFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "mode-zero.json")
	mustWrite(t, target, "private", 0o600)
	if err := os.Chmod(target, 0); err != nil {
		t.Fatal(err)
	}

	if err := RemoveWithin(root, "mode-zero.json"); err != nil {
		t.Fatalf("RemoveWithin unreadable regular file: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unreadable target remains after removal: %v", err)
	}
}

func TestRemoveWithinRefusesUnixSocketBeforeRemoval(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "sf-rm-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	target := filepath.Join(root, "service.sock")
	listener, err := new(net.ListenConfig).Listen(context.Background(), "unix", target)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			t.Skipf("sandbox does not permit Unix sockets: %v", err)
		}
		t.Fatal(err)
	}
	defer listener.Close()

	err = RemoveWithin(root, "service.sock")
	if !errors.Is(err, ErrNonRegular) {
		t.Fatalf("RemoveWithin socket error = %v, want ErrNonRegular", err)
	}
	info, statErr := os.Lstat(target)
	if statErr != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket target changed: info=%v err=%v", info, statErr)
	}
}

func TestRemoveWithinPermissionDeniedIsNonCommittedAndPreservesTarget(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "profiles")
	mustMkdir(t, parent, 0o700)
	target := filepath.Join(parent, "alice.json")
	mustWrite(t, target, "alice", 0o600)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	err := RemoveWithin(root, "profiles/alice.json")
	if err == nil {
		t.Skip("filesystem/user allowed unlink without parent write permission")
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		t.Fatalf("permission-denied unlink reported committed: %v", err)
	}
	assertContent(t, target, "alice")
}

func TestRemoveWithinClosesDescriptorOnPreRemoveFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "profile.json")
	mustWrite(t, target, "profile", 0o600)
	wantErr := errors.New("injected pre-remove failure")
	openedFD := -1
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.beforeRemove = func(_, fd int, _ string) error {
			openedFD = fd
			return wantErr
		}
	})

	err := RemoveWithin(root, "profile.json")
	if !errors.Is(err, wantErr) {
		t.Fatalf("RemoveWithin error = %v, want injected error", err)
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		t.Fatalf("pre-remove hook failure reported committed: %v", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(openedFD, &stat); !errors.Is(err, unix.EBADF) {
		t.Fatalf("target descriptor remains open after pre-remove failure: %v", err)
	}
	assertContent(t, target, "profile")
}

func TestRemoveWithinParentSwapAfterUnlinkIsCommittedAndOutsideUntouched(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	parent := filepath.Join(root, "profiles")
	moved := filepath.Join(root, "moved")
	outside := filepath.Join(workspace, "outside")
	for _, dir := range []string{root, parent, outside} {
		mustMkdir(t, dir, 0o700)
	}
	mustWrite(t, filepath.Join(parent, "alice.json"), "inside", 0o600)
	mustWrite(t, filepath.Join(outside, "alice.json"), "outside", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterRemove = func(_, _ int, _ string) error {
			if err := os.Rename(parent, moved); err != nil {
				return err
			}
			return os.Symlink(outside, parent)
		}
	})

	err := RemoveWithin(root, "profiles/alice.json")
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, ErrParentChanged) {
		t.Fatalf("RemoveWithin error = %v, want committed ErrParentChanged", err)
	}
	if _, err := os.Lstat(filepath.Join(moved, "alice.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("original target remains in moved parent: %v", err)
	}
	assertContent(t, filepath.Join(outside, "alice.json"), "outside")
}

func TestRemoveWithinConcurrentCallsLeaveTargetAbsent(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "profile.json")
	mustWrite(t, target, "profile", 0o600)

	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- RemoveWithin(root, "profile.json")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, ErrTargetChanged):
		default:
			t.Fatalf("concurrent RemoveWithin error = %v", err)
		}
	}
	// Darwin may report success to more than one racing unlinkat caller for the
	// same directory entry. The portable contract is therefore at least one
	// success, only expected losing errors, and an absent final name.
	if successes == 0 {
		t.Fatal("no concurrent RemoveWithin call succeeded")
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target remains after concurrent removals: %v", err)
	}
}
