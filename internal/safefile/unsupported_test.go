//go:build !darwin && !linux

package safefile

import (
	"errors"
	"testing"
)

func TestDescriptorAnchoredOperationsAreTypedUnsupported(t *testing.T) {
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
