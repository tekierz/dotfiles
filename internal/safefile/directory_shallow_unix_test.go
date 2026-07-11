//go:build darwin || linux

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestEnsureShallowDirectoryCreatesOnlyAcceptedLeafAndReturnsEvidence(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	evidence, err := EnsureShallowDirectoryWithinSnapshotTracked(root, "parent/leaf", nil, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil || evidence.Permissions() != 0o700 {
		t.Fatalf("evidence = %#v mode %04o, want tracked private snapshot", evidence, evidence.Permissions())
	}
	if err := VerifyDirectoryWithinSnapshot(root, "parent/leaf", evidence); err != nil {
		t.Fatalf("verify returned evidence: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "parent", "leaf"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("created directory entries = %v err=%v", entries, err)
	}
}

func TestEnsureShallowDirectoryRefusesMissingParent(t *testing.T) {
	root := t.TempDir()
	if _, err := EnsureShallowDirectoryWithinSnapshotTracked(root, "missing/leaf", nil, 0o700); err == nil {
		t.Fatal("missing parent was created")
	}
	if _, err := os.Lstat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing parent changed: %v", err)
	}
}

func TestEnsureShallowDirectoryExistingRequiresExactIdentity(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "leaf")
	mustMkdir(t, target, 0o700)
	accepted, err := SnapshotDirectoryWithin(root, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "moved")
	if err := os.Rename(target, moved); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, target, 0o700)
	if _, err := EnsureShallowDirectoryWithinSnapshotTracked(root, "leaf", accepted, 0o700); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("identical replacement error = %v, want ErrDirectoryChanged", err)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("replacement was not preserved: info=%v err=%v", info, err)
	}
}

func TestEnsureShallowDirectoryPreservesConcurrentChildWithoutEvidence(t *testing.T) {
	root := t.TempDir()
	directoryTestHooks.afterShallowMkdir = func(_ int, directoryFD int, _ string) error {
		fd, err := unix.Openat(directoryFD, "external", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
		if err != nil {
			return err
		}
		return unix.Close(fd)
	}
	t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
	evidence, err := EnsureShallowDirectoryWithinSnapshotTracked(root, "leaf", nil, 0o700)
	var committed *CommittedError
	if evidence != nil || !errors.As(err, &committed) || !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("create result = evidence %#v err %v, want committed change without evidence", evidence, err)
	}
	assertContent(t, filepath.Join(root, "leaf", "external"), "")
}

func TestEnsureShallowDirectoryRejectsParentReplacementDuringEvidenceCapture(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	moved := filepath.Join(root, "moved")
	mustMkdir(t, parent, 0o700)
	parents, err := CaptureParentChainWithin(root, "parent/leaf")
	if err != nil {
		t.Fatal(err)
	}
	directoryTestHooks.afterSnapshotOpen = func(_ int, _ int, target string) error {
		if target != "leaf" {
			return nil
		}
		if err := os.Rename(parent, moved); err != nil {
			return err
		}
		return os.Mkdir(parent, 0o700)
	}
	t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
	evidence, err := EnsureShallowDirectoryWithinParentChainTracked(root, "parent/leaf", nil, parents, 0o700)
	var committed *CommittedError
	if evidence != nil || !errors.As(err, &committed) || !errors.Is(err, ErrParentChanged) {
		t.Fatalf("create result = evidence %#v err %v, want committed parent change without evidence", evidence, err)
	}
	if info, err := os.Stat(filepath.Join(moved, "leaf")); err != nil || !info.IsDir() {
		t.Fatalf("committed leaf was not preserved in original parent: info=%v err=%v", info, err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "leaf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement parent received leaf: %v", err)
	}
}

func TestRemoveEmptyDirectoryWithinSnapshotSucceeds(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "leaf"), 0o700)
	expected, err := SnapshotDirectoryWithin(root, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveEmptyDirectoryWithinSnapshot(root, "leaf", expected); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "leaf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed directory remains: %v", err)
	}
}

func TestRemoveEmptyDirectoryPreservesConcurrentChild(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "leaf"), 0o700)
	expected, err := SnapshotDirectoryWithin(root, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	directoryTestHooks.beforeRemoveEmpty = func(_ int, directoryFD int, _ string) error {
		fd, err := unix.Openat(directoryFD, "external", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
		if err != nil {
			return err
		}
		return unix.Close(fd)
	}
	t.Cleanup(func() { directoryTestHooks = directoryHooks{} })
	if err := RemoveEmptyDirectoryWithinSnapshot(root, "leaf", expected); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("concurrent child error = %v, want ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(root, "leaf", "external"), "")
}

func TestRemoveEmptyDirectoryRefusesNonemptyAuthorityWithoutRecursion(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "leaf")
	mustMkdir(t, target, 0o700)
	mustWrite(t, filepath.Join(target, "preserve"), "value", 0o600)
	expected, err := SnapshotDirectoryWithin(root, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveEmptyDirectoryWithinSnapshot(root, "leaf", expected); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("nonempty removal error = %v, want ErrDirectoryChanged", err)
	}
	assertContent(t, filepath.Join(target, "preserve"), "value")
}

func TestRemoveEmptyDirectoryRefusesIdenticalReplacement(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "leaf")
	mustMkdir(t, target, 0o700)
	expected, err := SnapshotDirectoryWithin(root, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	mustMkdir(t, target, 0o700)
	if err := RemoveEmptyDirectoryWithinSnapshot(root, "leaf", expected); !errors.Is(err, ErrDirectoryChanged) {
		t.Fatalf("replacement removal error = %v, want ErrDirectoryChanged", err)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("replacement was not preserved: info=%v err=%v", info, err)
	}
}

func TestReplaceWithinRevisionNoCreateTrackedRefusesMissingParent(t *testing.T) {
	root := t.TempDir()
	_, missing, err := ReadWithin(root, "missing/config")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaceWithinRevisionNoCreateTracked(root, "missing/config", missing, []byte("value"), 0o600); err == nil {
		t.Fatal("no-create replacement created a missing parent")
	}
	if _, err := os.Lstat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing parent changed: %v", err)
	}
}

func TestReplaceWithinRevisionNoCreateTrackedReturnsExactEvidence(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	_, missing, err := ReadWithin(root, "parent/config")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := ReplaceWithinRevisionNoCreateTracked(root, "parent/config", missing, []byte("value"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	data, current, err := ReadWithin(root, "parent/config")
	if err != nil || string(data) != "value" || current != evidence {
		t.Fatalf("committed result = data %q current %+v evidence %+v err %v", data, current, evidence, err)
	}
}

func TestRestoreDirectoryWithinSnapshotNoCreateTrackedRefusesMissingParent(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "value", 0o600)
	source, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreDirectoryWithinSnapshotNoCreateTracked(root, "missing/live", source, nil); err == nil {
		t.Fatal("no-create directory restore created a missing parent")
	}
	if _, err := os.Lstat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing parent changed: %v", err)
	}
}

func TestRestoreDirectoryWithinSnapshotNoCreateTrackedReturnsExactEvidence(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "source"), 0o700)
	mustWrite(t, filepath.Join(root, "source", "config"), "value", 0o600)
	mustMkdir(t, filepath.Join(root, "parent"), 0o700)
	source, err := SnapshotDirectoryWithin(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := RestoreDirectoryWithinSnapshotNoCreateTracked(root, "parent/live", source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil {
		t.Fatal("restore returned nil evidence")
	}
	if err := VerifyDirectoryWithinSnapshot(root, "parent/live", evidence); err != nil {
		t.Fatalf("verify restore evidence: %v", err)
	}
	assertContent(t, filepath.Join(root, "parent", "live", "config"), "value")
}
