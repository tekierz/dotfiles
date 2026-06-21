package ui

import (
	"os"
	"testing"
)

// TestMain makes the entire ui test binary hermetic: it points HOME (and clears
// XDG_CONFIG_HOME) at a throwaway temp dir for the whole package BEFORE any test
// runs. This is the package-wide safety net behind the per-test withTempHome /
// newGoldenContext isolation (P1-C): even tests that call NewApp directly — which
// performs a best-effort hotkeys-migration save at startup — write into the temp
// dir, never the developer's real ~/.config/dotfiles. It also makes the suite
// pass under a sandbox with no writable real HOME.
//
// Per-test helpers (withTempHome) still override HOME to their own t.TempDir() and
// restore it afterward; because the pre-test value is THIS temp dir, the restore
// can never expose the real HOME mid-run. t.Setenv is intentionally not used here
// (it is illegal before tests start and incompatible with t.Parallel()).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dotfiles-ui-test-home-")
	if err != nil {
		panic("TestMain: create temp HOME: " + err.Error())
	}

	os.Setenv("HOME", dir)
	os.Unsetenv("XDG_CONFIG_HOME")

	code := m.Run()

	_ = os.RemoveAll(dir)
	os.Exit(code)
}
