//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReplaceWithinAllowsTrustedRootSymlink(t *testing.T) {
	workspace := t.TempDir()
	realRoot := filepath.Join(workspace, "real-root")
	mustMkdir(t, realRoot, 0o700)
	rootLink := filepath.Join(workspace, "trusted-root")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceWithin(rootLink, "nested/config", []byte("new"), 0o640); err != nil {
		t.Fatalf("ReplaceWithin() error = %v", err)
	}
	assertContent(t, filepath.Join(realRoot, "nested", "config"), "new")
}

func TestReplaceWithinRejectsInvalidRelativePaths(t *testing.T) {
	root := t.TempDir()
	paths := []string{"", "/absolute", "..", "../outside", "nested/../outside", ".", "nested//file"}
	for _, target := range paths {
		t.Run(strings.ReplaceAll(target, "/", "_"), func(t *testing.T) {
			err := ReplaceWithin(root, target, []byte("data"), 0o600)
			if !errors.Is(err, ErrInvalidPath) {
				t.Fatalf("ReplaceWithin(%q) error = %v, want ErrInvalidPath", target, err)
			}
		})
	}
}

func TestReplaceWithinRejectsInvalidModeBeforeFilesystemMutation(t *testing.T) {
	root := t.TempDir()
	err := ReplaceWithin(root, "new/child/config", []byte("unsafe"), fs.ModeDir|0o600)
	if !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("ReplaceWithin() error = %v, want ErrInvalidMode", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatalf("ReadDir(root): %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid mode mutated filesystem: %v", entries)
	}
}

func TestReplaceWithinRefusesIntermediateSymlink(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, outside, 0o700)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	err := ReplaceWithin(root, "linked/config", []byte("unsafe"), 0o600)
	if !errors.Is(err, ErrSymlink) {
		t.Fatalf("ReplaceWithin() error = %v, want ErrSymlink", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "config")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("outside target changed: %v", err)
	}
}

func TestReplaceWithinRefusesFinalSymlink(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	mustMkdir(t, root, 0o700)
	outside := filepath.Join(workspace, "outside")
	mustWrite(t, outside, "outside", 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}

	err := ReplaceWithin(root, "config", []byte("unsafe"), 0o600)
	if !errors.Is(err, ErrSymlink) {
		t.Fatalf("ReplaceWithin() error = %v, want ErrSymlink", err)
	}
	assertContent(t, outside, "outside")
}

func TestReplaceWithinRefusesNonRegularLeaves(t *testing.T) {
	tests := []struct {
		name   string
		create func(*testing.T, string) func()
	}{
		{
			name: "directory",
			create: func(t *testing.T, path string) func() {
				mustMkdir(t, path, 0o700)
				return func() {}
			},
		},
		{
			name: "fifo",
			create: func(t *testing.T, path string) func() {
				if err := unix.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
				return func() {}
			},
		},
		{
			name: "unix socket",
			create: func(t *testing.T, path string) func() {
				listener, err := net.Listen("unix", path)
				if err != nil {
					if errors.Is(err, os.ErrPermission) || errors.Is(err, unix.EPERM) {
						t.Skipf("sandbox does not permit Unix socket creation: %v", err)
					}
					t.Fatal(err)
				}
				return func() { _ = listener.Close() }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.name == "unix socket" {
				// Darwin's sockaddr_un path is short; the default testing temp
				// path can exceed it before the leaf name is appended.
				var err error
				root, err = os.MkdirTemp("/tmp", "safefile-socket-")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.RemoveAll(root) })
			}
			target := filepath.Join(root, "config")
			cleanup := tt.create(t, target)
			defer cleanup()

			err := ReplaceWithin(root, "config", []byte("unsafe"), 0o600)
			if !errors.Is(err, ErrNonRegular) {
				t.Fatalf("ReplaceWithin() error = %v, want ErrNonRegular", err)
			}
			if _, statErr := os.Lstat(target); statErr != nil {
				t.Fatalf("non-regular target was removed: %v", statErr)
			}
		})
	}
}

func TestReplaceWithinUsesExactModeAndReplacesInode(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	alias := filepath.Join(root, "old-hardlink")
	mustWrite(t, target, "old", 0o666)
	if err := os.Link(target, alias); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceWithin(root, "config", []byte("new"), 0o640); err != nil {
		t.Fatalf("ReplaceWithin() error = %v", err)
	}
	assertContent(t, target, "new")
	assertContent(t, alias, "old")
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("target mode = %o, want 640", got)
	}
}

func TestReplaceWithinDirectoryModes(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	mustMkdir(t, existing, 0o750)
	if err := os.Chmod(existing, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceWithin(root, "existing/created/deeper/config", []byte("new"), 0o600); err != nil {
		t.Fatalf("ReplaceWithin() error = %v", err)
	}
	assertMode(t, existing, 0o750)
	assertMode(t, filepath.Join(existing, "created"), 0o700)
	assertMode(t, filepath.Join(existing, "created", "deeper"), 0o700)
}

func TestReplaceWithinPrecommitFailurePreservesOldAndCleansTemporary(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "old", 0o600)
	wantErr := errors.New("injected precommit failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.randomSuffix = func() (string, error) { return "deterministic", nil }
		hooks.beforeCommit = func(_, _ int, _, _ string) error { return wantErr }
	})

	err := ReplaceWithin(root, "config", []byte("new"), 0o600)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceWithin() error = %v, want injected error", err)
	}
	assertContent(t, target, "old")
	assertDirectoryEntries(t, root, []string{"config"})
}

func TestReplaceWithinDetectsStagedNamespaceSubstitution(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "old", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.randomSuffix = func() (string, error) { return "deterministic", nil }
		hooks.beforeCommit = func(parentFD, _ int, temporary, _ string) error {
			if err := unix.Unlinkat(parentFD, temporary, 0); err != nil {
				return err
			}
			fd, err := unix.Openat(parentFD, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
			if err != nil {
				return err
			}
			if _, err := unix.Write(fd, []byte("substituted")); err != nil {
				_ = unix.Close(fd)
				return err
			}
			return unix.Close(fd)
		}
	})

	err := ReplaceWithin(root, "config", []byte("new"), 0o600)
	if !errors.Is(err, ErrStagedChanged) {
		t.Fatalf("ReplaceWithin() error = %v, want ErrStagedChanged", err)
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		t.Fatalf("staged substitution reported committed: %v", err)
	}
	assertContent(t, target, "old")
	assertDirectoryEntries(t, root, []string{"config"})
}

func TestReplaceWithinKeepsStagedDescriptorOpenAcrossRename(t *testing.T) {
	root := t.TempDir()
	var beforeIdentity, afterIdentity fileIdentity
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.beforeCommit = func(_, stagedFD int, _, _ string) error {
			var err error
			beforeIdentity, err = identityOf(stagedFD)
			return err
		}
		hooks.afterCommit = func(_, stagedFD int, _ string) error {
			var err error
			afterIdentity, err = identityOf(stagedFD)
			return err
		}
	})

	if err := ReplaceWithin(root, "config", []byte("new"), 0o600); err != nil {
		t.Fatalf("ReplaceWithin() error = %v", err)
	}
	if beforeIdentity != afterIdentity || beforeIdentity == (fileIdentity{}) {
		t.Fatalf("staged descriptor identity changed across rename: before=%+v after=%+v", beforeIdentity, afterIdentity)
	}
}

func TestReplaceWithinCreatedDirectorySwapIsDetectedWithoutChmod(t *testing.T) {
	root := t.TempDir()
	created := filepath.Join(root, "created")
	moved := filepath.Join(root, "created-by-safefile")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterMkdir = func(_ int, name string) error {
			if name != "created" {
				return nil
			}
			if err := os.Rename(created, moved); err != nil {
				return err
			}
			if err := os.Mkdir(created, 0o755); err != nil {
				return err
			}
			return os.Chmod(created, 0o755)
		}
	})

	err := ReplaceWithin(root, "created/config", []byte("new"), 0o600)
	if !errors.Is(err, ErrParentChanged) {
		t.Fatalf("ReplaceWithin() error = %v, want ErrParentChanged", err)
	}
	assertMode(t, created, 0o755)
	assertMode(t, moved, 0o700)
	if _, err := os.Lstat(filepath.Join(created, "config")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("replacement directory was modified: %v", err)
	}
}

func TestReplaceWithinFsyncsCreatedDirectoryAndParent(t *testing.T) {
	root := t.TempDir()
	var operations []string
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.fsyncDir = func(fd int, operation string) error {
			operations = append(operations, operation)
			return unix.Fsync(fd)
		}
	})

	if err := ReplaceWithin(root, "created/config", []byte("new"), 0o600); err != nil {
		t.Fatalf("ReplaceWithin() error = %v", err)
	}
	want := []string{"created directory", "created directory parent", "commit parent"}
	if !slices.Equal(operations, want) {
		t.Fatalf("directory fsync operations = %v, want %v", operations, want)
	}
}

func TestReplaceWithinCreatedDirectoryFsyncFailureIsTruthful(t *testing.T) {
	root := t.TempDir()
	wantErr := errors.New("injected created-directory parent fsync failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.fsyncDir = func(_ int, operation string) error {
			if operation == "created directory parent" {
				return wantErr
			}
			return nil
		}
	})

	err := ReplaceWithin(root, "created/config", []byte("new"), 0o600)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceWithin() error = %v, want injected error", err)
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		t.Fatalf("directory creation failure reported file commit: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, "created", "config")); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("file was created after directory fsync failure: %v", statErr)
	}
}

func TestReplaceWithinStagedFsyncFailurePreservesOldAndCleansTemporary(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "old", 0o600)
	wantErr := errors.New("injected staged fsync failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.randomSuffix = func() (string, error) { return "deterministic", nil }
		hooks.fsyncStaged = func(_ int) error { return wantErr }
	})

	err := ReplaceWithin(root, "config", []byte("new"), 0o600)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceWithin() error = %v, want injected error", err)
	}
	assertContent(t, target, "old")
	assertDirectoryEntries(t, root, []string{"config"})
}

func TestReplaceWithinCleanupFailureIsJoined(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "old", 0o600)
	primaryErr := errors.New("injected precommit failure")
	cleanupErr := errors.New("injected cleanup failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.randomSuffix = func() (string, error) { return "deterministic", nil }
		hooks.beforeCommit = func(_, _ int, _, _ string) error { return primaryErr }
		hooks.unlinkTemp = func(_ int, _ string) error { return cleanupErr }
	})

	err := ReplaceWithin(root, "config", []byte("new"), 0o600)
	if !errors.Is(err, primaryErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("ReplaceWithin() error = %v, want both primary and cleanup failures", err)
	}
	assertContent(t, target, "old")
	assertDirectoryEntries(t, root, []string{".safefile-deterministic", "config"})
}

func TestReplaceWithinCloseFailureReportsCommittedState(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "old", 0o600)
	wantErr := errors.New("injected close failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.closeStaged = func(fd int) error {
			if err := unix.Close(fd); err != nil {
				return err
			}
			return wantErr
		}
	})

	err := ReplaceWithin(root, "config", []byte("new"), 0o600)
	var committed *CommittedError
	if !errors.As(err, &committed) || !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceWithin() error = %v, want committed injected close failure", err)
	}
	assertContent(t, target, "new")
}

func TestReplaceWithinParentSwapDoesNotTouchOutsideAndCleansTemporary(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	parent := filepath.Join(root, "parent")
	moved := filepath.Join(root, "moved")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, parent, 0o700)
	mustMkdir(t, outside, 0o700)
	mustWrite(t, filepath.Join(parent, "config"), "old", 0o600)
	mustWrite(t, filepath.Join(outside, "config"), "outside", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.randomSuffix = func() (string, error) { return "deterministic", nil }
		hooks.beforeCommit = func(_, _ int, _, _ string) error {
			if err := os.Rename(parent, moved); err != nil {
				return err
			}
			return os.Symlink(outside, parent)
		}
	})

	err := ReplaceWithin(root, "parent/config", []byte("new"), 0o600)
	if !errors.Is(err, ErrParentChanged) {
		t.Fatalf("ReplaceWithin() error = %v, want ErrParentChanged", err)
	}
	var committed *CommittedError
	if errors.As(err, &committed) {
		t.Fatalf("precommit parent swap reported committed: %v", err)
	}
	assertContent(t, filepath.Join(outside, "config"), "outside")
	assertContent(t, filepath.Join(moved, "config"), "old")
	assertDirectoryEntries(t, moved, []string{"config"})
}

func TestReplaceWithinParentFsyncFailureReportsCommittedState(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	mustWrite(t, target, "old", 0o600)
	wantErr := errors.New("injected parent fsync failure")
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.fsyncDir = func(_ int, operation string) error {
			if operation == "commit parent" {
				return wantErr
			}
			return nil
		}
	})

	err := ReplaceWithin(root, "config", []byte("new"), 0o600)
	var committed *CommittedError
	if !errors.As(err, &committed) || !committed.Committed() {
		t.Fatalf("ReplaceWithin() error = %v, want CommittedError", err)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("ReplaceWithin() error = %v, want injected cause", err)
	}
	if !strings.Contains(committed.Operation, "parent fsync") {
		t.Fatalf("CommittedError.Operation = %q, want parent fsync", committed.Operation)
	}
	assertContent(t, target, "new")
}

func TestReplaceWithinPostcommitParentSwapReportsCommittedState(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	parent := filepath.Join(root, "parent")
	moved := filepath.Join(root, "moved")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, parent, 0o700)
	mustMkdir(t, outside, 0o700)
	mustWrite(t, filepath.Join(parent, "config"), "old", 0o600)
	mustWrite(t, filepath.Join(outside, "config"), "outside", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterCommit = func(_, _ int, _ string) error {
			if err := os.Rename(parent, moved); err != nil {
				return err
			}
			return os.Symlink(outside, parent)
		}
	})

	err := ReplaceWithin(root, "parent/config", []byte("new"), 0o600)
	var committed *CommittedError
	if !errors.As(err, &committed) {
		t.Fatalf("ReplaceWithin() error = %v, want CommittedError", err)
	}
	if !errors.Is(err, ErrParentChanged) {
		t.Fatalf("ReplaceWithin() error = %v, want ErrParentChanged", err)
	}
	assertContent(t, filepath.Join(moved, "config"), "new")
	assertContent(t, filepath.Join(outside, "config"), "outside")
}

func TestReplaceWithinConcurrentSameTargetUsesWholeLastWriter(t *testing.T) {
	root := t.TempDir()
	const writers = 24
	payloads := make(map[string]struct{}, writers)
	for i := 0; i < writers; i++ {
		payloads[fmt.Sprintf("writer-%02d:%s", i, strings.Repeat(string(rune('a'+i%26)), 4096))] = struct{}{}
	}

	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for payload := range payloads {
		payload := payload
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- ReplaceWithin(root, "config", []byte(payload), 0o600)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		var committed *CommittedError
		if !errors.As(err, &committed) || !errors.Is(err, ErrStagedChanged) {
			t.Fatalf("concurrent ReplaceWithin() error = %v, want nil or superseded CommittedError", err)
		}
	}
	if successes == 0 {
		t.Fatal("no concurrent writer completed without being superseded")
	}
	data, err := os.ReadFile(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := payloads[string(data)]; !ok {
		t.Fatalf("final file is not one complete writer payload (length %d)", len(data))
	}
	assertMode(t, filepath.Join(root, "config"), 0o600)
}

func setReplaceHooks(t *testing.T, configure func(*replaceHooks)) {
	t.Helper()
	replaceTestHooks = replaceHooks{}
	configure(&replaceTestHooks)
	t.Cleanup(func() {
		replaceTestHooks = replaceHooks{}
	})
}

func mustMkdir(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

func assertDirectoryEntries(t *testing.T, path string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s entries = %v, want %v", path, got, want)
	}
}
