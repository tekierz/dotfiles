package ui

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

// TestWizardInstallResolvesPiPackages guards FIX 2: the wizard install loop must
// resolve packages via tools.PackagesForPlatform (the documented single source of
// truth, which applies the Raspberry Pi -> Debian fallback), NOT the raw
// t.Packages()[platform] / ["all"] lookup. With the raw lookup, a standard tool
// that defines only MacOS/Arch/Debian keys resolved to nothing on a Pi and the
// loop silently skipped it (no install, no failure, no Error screen).
func TestWizardInstallResolvesPiPackages(t *testing.T) {
	reg := tools.GetRegistry()

	// tmux is a standard tool: it defines MacOS/Arch/Debian packages and NO Pi or
	// "all" key. On a Pi the raw lookup is empty; PackagesForPlatform falls back to
	// the Debian packages.
	tmux, ok := reg.Get("tmux")
	if !ok {
		t.Fatal("tmux not in registry")
	}

	// What the BUGGY loop computed (raw lookup) — empty on Pi.
	raw := tmux.Packages()[pkg.PlatformPi]
	if len(raw) == 0 {
		raw = tmux.Packages()["all"]
	}
	if len(raw) != 0 {
		t.Skipf("tmux now defines a Pi/all package (%v); test premise no longer holds", raw)
	}

	// What the FIXED loop computes — must be non-empty (Debian fallback).
	resolved := tools.PackagesForPlatform(tmux.Packages(), pkg.PlatformPi)
	if len(resolved) == 0 {
		t.Errorf("PackagesForPlatform(tmux, Pi) = empty; wizard would silently skip tmux on a Pi (FIX 2)")
	}

	// A genuinely-unsupported package set resolves empty under PackagesForPlatform
	// too — that is the case the loop now turns into a noteFailure (not a silent
	// skip).
	empty := tools.PackagesForPlatform(map[pkg.Platform][]string{
		pkg.PlatformMacOS: {"mac-only"},
	}, pkg.PlatformPi)
	if len(empty) != 0 {
		t.Errorf("PackagesForPlatform(macOS-only, Pi) = %v; want empty (the noteFailure path)", empty)
	}
}
