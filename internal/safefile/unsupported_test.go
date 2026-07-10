//go:build !darwin && !linux

package safefile

import (
	"errors"
	"testing"
)

func TestDescriptorAnchoredOperationsAreTypedUnsupported(t *testing.T) {
	if _, err := SnapshotDirectoryWithin(".", "directory"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("SnapshotDirectoryWithin error = %v, want ErrUnsupported", err)
	}
	if err := RestoreDirectoryWithin(".", "directory", &DirectorySnapshot{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want ErrUnsupported", err)
	}
	if err := RemoveDirectoryWithin(".", "directory"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveDirectoryWithin error = %v, want ErrUnsupported", err)
	}
	if err := ReplaceWithin(".", "file", nil, 0o600); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReplaceWithin error = %v, want ErrUnsupported", err)
	}
	if _, _, err := ReadWithin(".", "file"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReadWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := AcquireLockWithin(".", "lock", 0o600); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AcquireLockWithin error = %v, want ErrUnsupported", err)
	}
	if err := RemoveWithin(".", "file"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveWithin error = %v, want ErrUnsupported", err)
	}
}
