//go:build !darwin && !linux

package config

import (
	"errors"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestGlobalConfigLockUnsupportedIsTyped(t *testing.T) {
	_, err := acquireGlobalConfigLock(".", ".global.json.lock")
	if !errors.Is(err, ErrGlobalConfigLockUnsupported) {
		t.Fatalf("lock error = %v, want ErrGlobalConfigLockUnsupported", err)
	}
	if !errors.Is(err, safefile.ErrUnsupported) {
		t.Fatalf("lock error = %v, want safefile.ErrUnsupported", err)
	}
}
