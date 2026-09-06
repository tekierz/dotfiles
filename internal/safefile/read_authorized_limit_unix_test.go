//go:build darwin || linux

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadWithinAuthorizedLimitExactOversizeAndMissing(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	mustWrite(t, filepath.Join(parent, "exact"), "value", 0o600)
	mustWrite(t, filepath.Join(parent, "over"), "values", 0o600)

	for _, target := range []string{"parent/exact", "parent/over", "parent/missing"} {
		parents, err := CaptureParentChainWithin(root, target)
		if err != nil {
			t.Fatalf("capture %s parents: %v", target, err)
		}
		switch target {
		case "parent/exact":
			data, revision, err := ReadWithinAuthorizedLimit(root, target, parents, int64(len("value")))
			if err != nil || string(data) != "value" || !revision.Exists() || revision.Permissions() != 0o600 {
				t.Fatalf("exact authorized read = %q %+v %v", data, revision, err)
			}
		case "parent/over":
			data, revision, err := ReadWithinAuthorizedLimit(root, target, parents, int64(len("value")))
			if !errors.Is(err, ErrSizeLimit) || data != nil || revision.Tracked() {
				t.Fatalf("oversize authorized read = %v %+v %v", data, revision, err)
			}
		case "parent/missing":
			data, revision, err := ReadWithinAuthorizedLimit(root, target, parents, 1)
			if err != nil || data != nil || !revision.Tracked() || revision.Exists() {
				t.Fatalf("missing authorized read = %v %+v %v", data, revision, err)
			}
		}
	}
}

func TestReadWithinAuthorizedLimitRejectsNegativeLimitBeforeValidationOrOpen(t *testing.T) {
	for _, test := range []struct {
		name    string
		root    string
		rel     string
		parents *ParentChain
	}{
		{
			name:    "invalid authority",
			root:    filepath.Join(t.TempDir(), "missing-root"),
			rel:     "parent/config",
			parents: &ParentChain{},
		},
		{
			name:    "invalid path",
			root:    filepath.Join(t.TempDir(), "missing-root"),
			rel:     "../escape",
			parents: &ParentChain{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			authorizedOpenCalls := 0
			readOpenCalls := 0
			setReplaceHooks(t, func(hooks *replaceHooks) {
				hooks.afterAuthorizedParentOpen = func(_ int, _ int, _ string) error {
					authorizedOpenCalls++
					return nil
				}
				hooks.afterReadOpen = func(_ int, _ int, _ string) error {
					readOpenCalls++
					return nil
				}
			})

			data, revision, err := ReadWithinAuthorizedLimit(test.root, test.rel, test.parents, -1)
			if !errors.Is(err, ErrSizeLimit) || data != nil || revision.Tracked() {
				t.Fatalf("negative-limit read = %v %+v %v", data, revision, err)
			}
			if authorizedOpenCalls != 0 || readOpenCalls != 0 {
				t.Fatalf("negative-limit hooks = authorized:%d read:%d, want zero", authorizedOpenCalls, readOpenCalls)
			}
		})
	}
}

func TestReadWithinAuthorizedUsesHeldParentAcrossSwapReadRestore(t *testing.T) {
	for _, test := range []struct {
		name string
		read func(string, string, *ParentChain) ([]byte, Revision, error)
	}{
		{
			name: "limited",
			read: func(root, rel string, parents *ParentChain) ([]byte, Revision, error) {
				return ReadWithinAuthorizedLimit(root, rel, parents, 1024)
			},
		},
		{
			name: "unlimited",
			read: ReadWithinAuthorized,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "parent")
			acceptedParent := filepath.Join(root, "accepted-parent")
			mustMkdir(t, parent, 0o700)
			mustWrite(t, filepath.Join(parent, "config"), "accepted", 0o600)
			parents, err := CaptureParentChainWithin(root, "parent/config")
			if err != nil {
				t.Fatal(err)
			}

			authorizedOpenCalls := 0
			readOpenCalls := 0
			setReplaceHooks(t, func(hooks *replaceHooks) {
				hooks.afterAuthorizedParentOpen = func(_ int, _ int, _ string) error {
					authorizedOpenCalls++
					if err := os.Rename(parent, acceptedParent); err != nil {
						return err
					}
					if err := os.Mkdir(parent, 0o700); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(parent, "config"), []byte("replacement"), 0o600)
				}
				hooks.afterReadOpen = func(_ int, _ int, _ string) error {
					readOpenCalls++
					if err := os.Remove(filepath.Join(parent, "config")); err != nil {
						return err
					}
					if err := os.Remove(parent); err != nil {
						return err
					}
					return os.Rename(acceptedParent, parent)
				}
			})

			data, revision, err := test.read(root, "parent/config", parents)
			if err != nil || string(data) != "accepted" || !revision.Exists() {
				t.Fatalf("authorized ABA read = %q %+v %v", data, revision, err)
			}
			if string(data) == "replacement" {
				t.Fatal("authorized ABA read accepted replacement bytes")
			}
			if authorizedOpenCalls != 1 || readOpenCalls != 1 {
				t.Fatalf("authorized ABA hooks = authorized:%d read:%d, want one each", authorizedOpenCalls, readOpenCalls)
			}
			assertContent(t, filepath.Join(parent, "config"), "accepted")
		})
	}
}

func TestReadWithinAuthorizedLimitRejectsParentDriftBeforeRead(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	mustWrite(t, filepath.Join(parent, "config"), "accepted", 0o600)
	parents, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(parent, filepath.Join(root, "former-parent")); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, parent, 0o700)
	mustWrite(t, filepath.Join(parent, "config"), "replacement", 0o600)

	data, revision, err := ReadWithinAuthorizedLimit(root, "parent/config", parents, 1024)
	if !errors.Is(err, ErrParentChanged) || data != nil || revision.Tracked() {
		t.Fatalf("pre-read parent drift = %v %+v %v", data, revision, err)
	}
}

func TestReadWithinAuthorizedLimitDiscardsEvidenceAfterPostReadParentDrift(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	mustWrite(t, filepath.Join(parent, "config"), "accepted", 0o600)
	parents, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterReadOpen = func(_ int, _ int, _ string) error {
			if err := os.Rename(parent, filepath.Join(root, "former-parent")); err != nil {
				return err
			}
			return os.Mkdir(parent, 0o700)
		}
	})

	data, revision, err := ReadWithinAuthorizedLimit(root, "parent/config", parents, 1024)
	if !errors.Is(err, ErrParentChanged) || data != nil || revision.Tracked() {
		t.Fatalf("post-read parent drift = %v %+v %v", data, revision, err)
	}
}

func TestReadWithinAuthorizedLimitRejectsSymlinkAndNonRegularLeaves(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	mustWrite(t, filepath.Join(parent, "target"), "value", 0o600)
	if err := os.Symlink("target", filepath.Join(parent, "link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(parent, "fifo"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		want error
	}{
		{name: "link", want: ErrSymlink},
		{name: "fifo", want: ErrNonRegular},
	} {
		t.Run(test.name, func(t *testing.T) {
			rel := "parent/" + test.name
			parents, err := CaptureParentChainWithin(root, rel)
			if err != nil {
				t.Fatal(err)
			}
			data, revision, err := ReadWithinAuthorizedLimit(root, rel, parents, 1024)
			if !errors.Is(err, test.want) || data != nil || revision.Tracked() {
				t.Fatalf("%s authorized read = %v %+v %v", test.name, data, revision, err)
			}
		})
	}
}

func TestReadWithinAuthorizedLimitRejectsDescriptorRevisionRace(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	mustMkdir(t, parent, 0o700)
	mustWrite(t, filepath.Join(parent, "config"), "value", 0o600)
	parents, err := CaptureParentChainWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	setReplaceHooks(t, func(hooks *replaceHooks) {
		hooks.afterReadOpen = func(parentFD int, _ int, target string) error {
			writeFD, err := unix.Openat(parentFD, target, unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			if _, err := unix.Pwrite(writeFD, []byte("!"), int64(len("value"))); err != nil {
				_ = unix.Close(writeFD)
				return err
			}
			return unix.Close(writeFD)
		}
	})

	data, revision, err := ReadWithinAuthorizedLimit(root, "parent/config", parents, 1024)
	if !errors.Is(err, ErrRevisionChanged) || data != nil || revision.Tracked() {
		t.Fatalf("descriptor revision race = %v %+v %v", data, revision, err)
	}
}
