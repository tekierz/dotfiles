//go:build !darwin && !linux

package config

import (
	"errors"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestGlobalConfigLockUnsupportedIsTyped(t *testing.T) {
	unsupported := func(_, _ string) (func() error, error) {
		return nil, safefile.ErrUnsupported
	}
	_, err := acquireGlobalConfigStateLock(unsupported, "/global.json")
	if !errors.Is(err, ErrGlobalConfigLockUnsupported) {
		t.Fatalf("lock error = %v, want ErrGlobalConfigLockUnsupported", err)
	}
	if !errors.Is(err, safefile.ErrUnsupported) {
		t.Fatalf("lock error = %v, want safefile.ErrUnsupported", err)
	}
}
