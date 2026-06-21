package main

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
)

// TestListThemesWithConfigNilNoPanic is the regression guard for the
// theme-list nil-deref finding: listThemesWithConfig must not panic when
// called with a nil config (the error path where LoadGlobalConfig fails).
func TestListThemesWithConfigNilNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("listThemesWithConfig(nil) panicked: %v", r)
		}
	}()

	// nil represents a failed LoadGlobalConfig where DefaultGlobalConfig
	// is substituted; the function must still iterate AvailableThemes.
	listThemesWithConfig(nil)
}

// TestListThemesWithConfigNonNilNoPanic verifies the non-nil path is also safe.
func TestListThemesWithConfigNonNilNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("listThemesWithConfig(cfg) panicked: %v", r)
		}
	}()

	cfg := config.DefaultGlobalConfig()
	listThemesWithConfig(cfg)
}
