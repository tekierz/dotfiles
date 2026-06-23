package main

import "testing"

func TestUninstallBinaryNamesIncludeInstalledUtilities(t *testing.T) {
	got := map[string]bool{}
	for _, name := range uninstallBinaryNames() {
		got[name] = true
	}

	for _, want := range []string{"dotfiles", "dotfiles-tui", "dotfiles-setup", "hk", "caff", "sshh"} {
		if !got[want] {
			t.Fatalf("uninstallBinaryNames() missing %q", want)
		}
	}
	if got["y"] {
		t.Fatal("uninstallBinaryNames() still removes legacy y helper instead of sshh")
	}
}
