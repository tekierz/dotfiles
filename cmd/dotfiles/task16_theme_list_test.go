package main

import (
	"io"
	"os"
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

func TestThemeListFlagRuns(t *testing.T) {
	if themeCmd.Flags().Lookup("list") == nil {
		t.Fatal("theme --list flag is not registered")
	}

	originalStdout := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	defer devNull.Close()
	os.Stdout = devNull
	defer func() {
		os.Stdout = originalStdout
		if err := themeCmd.Flags().Set("list", "false"); err != nil {
			t.Fatalf("reset theme list flag: %v", err)
		}
	}()

	themeCmd.SetOut(io.Discard)
	themeCmd.SetErr(io.Discard)
	if err := themeCmd.Flags().Set("list", "true"); err != nil {
		t.Fatalf("set theme list flag: %v", err)
	}
	themeCmd.Run(themeCmd, nil)
}
