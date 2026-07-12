//go:build darwin || linux

package safefile

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadWithinLimitRejectsNegativeLimit(t *testing.T) {
	data, revision, err := ReadWithinLimit(t.TempDir(), "config", -1)
	if !errors.Is(err, ErrSizeLimit) || data != nil || revision.Tracked() {
		t.Fatalf("negative limit = data %v revision %+v err %v, want ErrSizeLimit and zero revision", data, revision, err)
	}
}

func TestReadWithinLimitZeroAndExactBoundaries(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "empty"), "", 0o600)
	mustWrite(t, filepath.Join(root, "exact"), "value", 0o640)
	mustWrite(t, filepath.Join(root, "over"), "values", 0o600)

	data, empty, err := ReadWithinLimit(root, "empty", 0)
	if err != nil || len(data) != 0 || !empty.Tracked() || !empty.Exists() {
		t.Fatalf("empty zero-limit read = %q %+v %v", data, empty, err)
	}
	data, exact, err := ReadWithinLimit(root, "exact", int64(len("value")))
	if err != nil || string(data) != "value" || !exact.Exists() || exact.Permissions() != 0o640 {
		t.Fatalf("exact-limit read = %q %+v %v", data, exact, err)
	}
	for name, limit := range map[string]int64{"exact at N-1": int64(len("value") - 1), "nonempty at zero": 0} {
		t.Run(name, func(t *testing.T) {
			path := "exact"
			if limit == 0 {
				path = "over"
			}
			data, revision, err := ReadWithinLimit(root, path, limit)
			if !errors.Is(err, ErrSizeLimit) || data != nil || revision.Tracked() {
				t.Fatalf("over-limit read = data %v revision %+v err %v", data, revision, err)
			}
		})
	}
}

func TestReadWithinLimitRejectsSparseOversizedFileWithoutRevision(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sparse")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(1 << 30); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, revision, err := ReadWithinLimit(root, "sparse", 1024)
	if !errors.Is(err, ErrSizeLimit) || data != nil || revision.Tracked() {
		t.Fatalf("sparse oversized read = data length %d revision %+v err %v", len(data), revision, err)
	}
}

func TestReadWithinLimitMissingIsTrackedMissing(t *testing.T) {
	data, revision, err := ReadWithinLimit(t.TempDir(), "missing/parents/config", 1)
	if err != nil || data != nil || !revision.Tracked() || revision.Exists() {
		t.Fatalf("missing read = data %v revision %+v err %v", data, revision, err)
	}
}

func TestReadWithinLimitPreservesSymlinkAndNonRegularErrors(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "root")
	outside := filepath.Join(workspace, "outside")
	mustMkdir(t, root, 0o700)
	mustMkdir(t, outside, 0o700)
	mustWrite(t, filepath.Join(outside, "config"), "outside", 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadWithinLimit(root, "linked/config", 1024); !errors.Is(err, ErrSymlink) {
		t.Fatalf("ancestor symlink error = %v, want ErrSymlink", err)
	}
	if err := os.Symlink(filepath.Join(outside, "config"), filepath.Join(root, "leaf")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadWithinLimit(root, "leaf", 1024); !errors.Is(err, ErrSymlink) {
		t.Fatalf("leaf symlink error = %v, want ErrSymlink", err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadWithinLimit(root, "fifo", 1024); !errors.Is(err, ErrNonRegular) {
		t.Fatalf("FIFO error = %v, want ErrNonRegular", err)
	}
}

func TestReadWithinLimitRetainsHardlinkCount(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "source"), "value", 0o600)
	if err := os.Link(filepath.Join(root, "source"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	data, revision, err := ReadWithinLimit(root, "alias", 5)
	if err != nil || string(data) != "value" || revision.LinkCount() != 2 {
		t.Fatalf("hardlink read = %q %+v %v", data, revision, err)
	}
}

func TestReadWithinLimitMatchesReadWithinAtSufficientLimit(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "config"), "stable value", 0o640)
	wantData, wantRevision, err := ReadWithin(root, "config")
	if err != nil {
		t.Fatal(err)
	}
	gotData, gotRevision, err := ReadWithinLimit(root, "config", int64(len(wantData)))
	if err != nil || string(gotData) != string(wantData) || gotRevision != wantRevision {
		t.Fatalf("limited parity = %q %+v %v, want %q %+v", gotData, gotRevision, err, wantData, wantRevision)
	}
}

func TestReadWithinLimitMaxInt64MatchesLegacyRead(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "config"), "small", 0o600)
	wantData, wantRevision, err := ReadWithin(root, "config")
	if err != nil {
		t.Fatal(err)
	}
	gotData, gotRevision, err := ReadWithinLimit(root, "config", math.MaxInt64)
	if err != nil || string(gotData) != string(wantData) || gotRevision != wantRevision {
		t.Fatalf("MaxInt64 limited read = %q %+v %v, want %q %+v", gotData, gotRevision, err, wantData, wantRevision)
	}
}

func TestReadWithinLimitClosesDescriptorAfterStableSizeFailure(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "config"), "value", 0o600)
	openedFD := -1
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterReadOpen = func(_ int, fileFD int, _ string) error {
			openedFD = fileFD
			return nil
		}
	})
	data, revision, err := ReadWithinLimit(root, "config", 4)
	if !errors.Is(err, ErrSizeLimit) || data != nil || revision.Tracked() {
		t.Fatalf("stable oversized read = data %v revision %+v err %v", data, revision, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(openedFD, &stat); !errors.Is(err, unix.EBADF) {
		t.Fatalf("limited-read descriptor still open after size failure: %v", err)
	}
}

func TestReadWithinLimitPrioritizesRevisionChangeOverSizeLimit(t *testing.T) {
	for _, test := range []struct {
		name    string
		initial string
		limit   int64
		mutate  func(int, string) error
	}{
		{"append beyond limit", "value", 5, func(parentFD int, name string) error {
			fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			if _, err := unix.Pwrite(fd, []byte("!"), 5); err != nil {
				_ = unix.Close(fd)
				return err
			}
			return unix.Close(fd)
		}},
		{"truncate oversized", "values", 5, func(parentFD int, name string) error {
			fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			if err := unix.Ftruncate(fd, 2); err != nil {
				_ = unix.Close(fd)
				return err
			}
			return unix.Close(fd)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mustWrite(t, filepath.Join(root, "config"), test.initial, 0o600)
			setReplaceHooks(t, func(hooks *replaceHooks) {
				hooks.afterReadOpen = func(parentFD int, _ int, name string) error { return test.mutate(parentFD, name) }
			})
			data, revision, err := ReadWithinLimit(root, "config", test.limit)
			if !errors.Is(err, ErrRevisionChanged) || errors.Is(err, ErrSizeLimit) || data != nil || revision.Tracked() {
				t.Fatalf("racing limited read = data %v revision %+v err %v, want ErrRevisionChanged", data, revision, err)
			}
		})
	}
}

func TestReadWithinLimitReplacementKeepsOpenedDescriptorParity(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "config")
	moved := filepath.Join(root, "opened-source")
	mustWrite(t, target, "old", 0o600)
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterReadOpen = func(_ int, _ int, _ string) error {
			if err := os.Rename(target, moved); err != nil {
				return err
			}
			return os.WriteFile(target, []byte("replacement"), 0o600)
		}
	})
	data, revision, err := ReadWithinLimit(root, "config", 3)
	if err != nil || string(data) != "old" || !revision.Exists() {
		t.Fatalf("replacement descriptor read = %q %+v %v", data, revision, err)
	}
}
