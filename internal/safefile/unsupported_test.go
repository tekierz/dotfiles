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
	if _, err := CaptureParentChainWithin(".", "directory/file"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("CaptureParentChainWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := BindParentChainWithin(".", "directory/file", &ParentChain{}, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("BindParentChainWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := BindParentChainPrefixWithin(".", "directory/file", "directory", &ParentChain{}, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("BindParentChainPrefixWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := ValidateParentChainWithin(".", "directory/file", &ParentChain{}, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ValidateParentChainWithin error = %v, want ErrUnsupported", err)
	}
	if _, _, err := CaptureDirectoryRootWithin(".", "directory"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("CaptureDirectoryRootWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := OpenDirectoryWithinAuthorized(".", "directory", &ParentChain{}, &DirectorySnapshot{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("OpenDirectoryWithinAuthorized error = %v, want ErrUnsupported", err)
	}
	if _, err := ExtendParentChainWithinDirectory(".", "directory/file", "directory", &ParentChain{}, &DirectorySnapshot{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ExtendParentChainWithinDirectory error = %v, want ErrUnsupported", err)
	}
	if _, _, _, err := ObserveFileWithin(".", "file"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ObserveFileWithin error = %v, want ErrUnsupported", err)
	}
	if _, _, err := ObserveDirectoryWithin(".", "directory"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ObserveDirectoryWithin error = %v, want ErrUnsupported", err)
	}
	if _, _, err := ReadWithinAuthorized(".", "file", &ParentChain{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReadWithinAuthorized error = %v, want ErrUnsupported", err)
	}
	if _, _, err := ReadWithinAuthorizedLimit(".", "file", &ParentChain{}, 1); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReadWithinAuthorizedLimit error = %v, want ErrUnsupported", err)
	}
	if _, _, err := ReadWithinAuthorizedLimit(".", "../file", &ParentChain{}, -1); !errors.Is(err, ErrSizeLimit) {
		t.Fatalf("negative ReadWithinAuthorizedLimit error = %v, want ErrSizeLimit", err)
	}
	if err := RestoreDirectoryWithin(".", "directory", &DirectorySnapshot{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RestoreDirectoryWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := RestoreDirectoryWithinSnapshotNoCreateTracked(".", "directory", &DirectorySnapshot{}, nil); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RestoreDirectoryWithinSnapshotNoCreateTracked error = %v, want ErrUnsupported", err)
	}
	if _, err := RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(".", "directory", &DirectorySnapshot{}, nil, &ParentChain{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked error = %v, want ErrUnsupported", err)
	}
	if _, err := EnsureShallowDirectoryWithinSnapshotTracked(".", "directory", nil, 0o700); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("EnsureShallowDirectoryWithinSnapshotTracked error = %v, want ErrUnsupported", err)
	}
	if _, err := EnsureShallowDirectoryWithinParentChainTracked(".", "directory", nil, &ParentChain{}, 0o700); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("EnsureShallowDirectoryWithinParentChainTracked error = %v, want ErrUnsupported", err)
	}
	if err := RemoveEmptyDirectoryWithinSnapshot(".", "directory", &DirectorySnapshot{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveEmptyDirectoryWithinSnapshot error = %v, want ErrUnsupported", err)
	}
	if err := RemoveEmptyDirectoryWithinSnapshotAuthorized(".", "directory", &DirectorySnapshot{}, &ParentChain{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveEmptyDirectoryWithinSnapshotAuthorized error = %v, want ErrUnsupported", err)
	}
	if err := RemoveDirectoryWithin(".", "directory"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveDirectoryWithin error = %v, want ErrUnsupported", err)
	}
	if err := RemoveDirectoryWithinSnapshotAuthorized(".", "directory", &DirectorySnapshot{}, &ParentChain{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveDirectoryWithinSnapshotAuthorized error = %v, want ErrUnsupported", err)
	}
	if err := ReplaceWithin(".", "file", nil, 0o600); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReplaceWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := ReplaceWithinRevisionNoCreateTracked(".", "file", Revision{}, nil, 0o600); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReplaceWithinRevisionNoCreateTracked error = %v, want ErrUnsupported", err)
	}
	if _, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(".", "file", Revision{}, &ParentChain{}, nil, 0o600); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReplaceWithinRevisionNoCreateAuthorizedTracked error = %v, want ErrUnsupported", err)
	}
	if _, _, err := ReadWithin(".", "file"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReadWithin error = %v, want ErrUnsupported", err)
	}
	if _, _, err := ReadWithinLimit(".", "file", 1); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ReadWithinLimit error = %v, want ErrUnsupported", err)
	}
	if _, err := AcquireLockWithin(".", "lock", 0o600); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AcquireLockWithin error = %v, want ErrUnsupported", err)
	}
	if _, err := AcquireLockWithinAuthorized(".", "lock", 0o600, &ParentChain{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AcquireLockWithinAuthorized error = %v, want ErrUnsupported", err)
	}
	if err := RemoveWithin(".", "file"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveWithin error = %v, want ErrUnsupported", err)
	}
	if err := RemoveWithinRevisionAuthorized(".", "file", Revision{}, &ParentChain{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveWithinRevisionAuthorized error = %v, want ErrUnsupported", err)
	}
}
