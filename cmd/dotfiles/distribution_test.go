package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLegacyInstallerIsNotDistributed prevents the retired Bash product from
// quietly returning through source, local packaging, CI, or primary install docs.
// Diagnostic and migration references to the old executable name remain valid.
func TestLegacyInstallerIsNotDistributed(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	legacyPath := filepath.Join(repoRoot, "bin", "dotfiles-setup")
	if _, err := os.Stat(legacyPath); err == nil {
		t.Fatalf("retired installer must not be distributed at %s", legacyPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect retired installer path: %v", err)
	}

	for _, check := range []struct {
		path   string
		forbid []string
	}{
		{
			path:   "Makefile",
			forbid: []string{"dotfiles-setup"},
		},
		{
			path: "README.md",
			forbid: []string{
				"raw.githubusercontent.com/tekierz/dotfiles/main/bin/dotfiles-setup",
				"bin/dotfiles-setup | bash",
			},
		},
		{
			path:   filepath.Join(".github", "workflows", "ci.yml"),
			forbid: []string{"bin/dotfiles-setup"},
		},
		{
			path:   filepath.Join(".github", "workflows", "release.yml"),
			forbid: []string{"bin/dotfiles-setup"},
		},
		{
			path:   ".goreleaser.yml",
			forbid: []string{"dotfiles-setup"},
		},
		{
			path:   filepath.Join(".claude", "skills", "pre-pr-tests", "SKILL.md"),
			forbid: []string{"bin/dotfiles-setup"},
		},
	} {
		data, err := os.ReadFile(filepath.Join(repoRoot, check.path))
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		for _, forbidden := range check.forbid {
			if strings.Contains(string(data), forbidden) {
				t.Errorf("%s still advertises or packages retired installer via %q", check.path, forbidden)
			}
		}
	}
}
