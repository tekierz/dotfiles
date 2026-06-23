package pkg

import (
	"os/exec"
	"strconv"
	"testing"
)

// runExit runs `sh -c "exit N"` and returns the resulting error so tests can
// exercise classifyCheckupdatesErr with real *exec.ExitError values (exit code
// preserved by the OS) instead of synthesizing one. The exit code is a fixed
// integer from the test table, never user input.
func runExit(t *testing.T, code int) error {
	t.Helper()
	return exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
}

// TestClassifyCheckupdatesErr guards the pacman checkupdates exit-code
// handling. The regression silently treated *any* nonzero exit as "no updates
// available" (returning nil), masking real failures such as a stale/locked
// temp DB or mirror error. The fix narrows the "no updates" case to exit code 2
// only; every other nonzero exit must surface as an error.
//
// Against the buggy code these would FAIL: exit 1 and exit 3 returned nil but
// the assertions below require a non-nil error for them.
func TestClassifyCheckupdatesErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{name: "nil means updates found", err: nil, wantErr: false},
		{name: "exit 2 means no updates", err: runExit(t, 2), wantErr: false},
		{name: "exit 1 is a real failure", err: runExit(t, 1), wantErr: true},
		{name: "exit 3 is a real failure", err: runExit(t, 3), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyCheckupdatesErr(tt.err)
			if (got != nil) != tt.wantErr {
				t.Errorf("classifyCheckupdatesErr(%v) error = %v, wantErr = %v", tt.err, got, tt.wantErr)
			}
		})
	}
}

// TestParsePacmanSearch_MultiLine verifies that pacman -Ss output is parsed
// correctly regardless of multi-line descriptions or leading blank output.
// pacman -Ss uses a header line ("repo/name version [installed]") followed
// by one or more indented description lines; the old stride-by-2 parser
// misaligns whenever output doesn't follow an exact two-line pattern.
func TestParsePacmanSearch_MultiLine(t *testing.T) {
	// Sample pacman -Ss output: two packages, first has a multi-word description
	// that still fits on one line (normal case), second has the "[installed]" tag.
	raw := `core/linux 6.9.0-1
    The Linux kernel and modules
extra/vim 9.1.0-1 [installed]
    Vi Improved, a highly configurable, improved version of the Vi text editor
extra/vim-runtime 9.1.0-1
    Runtime support files for vim
`

	pkgs := parsePacmanSearch(raw)
	if len(pkgs) != 3 {
		t.Fatalf("parsePacmanSearch returned %d packages, want 3: %+v", len(pkgs), pkgs)
	}

	// First package
	if pkgs[0].Name != pkgNameLinux {
		t.Errorf("pkgs[0].Name = %q, want %q", pkgs[0].Name, pkgNameLinux)
	}
	if pkgs[0].Description != "The Linux kernel and modules" {
		t.Errorf("pkgs[0].Description = %q", pkgs[0].Description)
	}

	// Second package (with [installed] tag stripped from version field)
	if pkgs[1].Name != "vim" {
		t.Errorf("pkgs[1].Name = %q, want %q", pkgs[1].Name, "vim")
	}
	if pkgs[1].Description != "Vi Improved, a highly configurable, improved version of the Vi text editor" {
		t.Errorf("pkgs[1].Description = %q", pkgs[1].Description)
	}

	// Third package
	if pkgs[2].Name != "vim-runtime" {
		t.Errorf("pkgs[2].Name = %q, want %q", pkgs[2].Name, "vim-runtime")
	}

	// All must be tagged pacman
	for i, p := range pkgs {
		if p.InstalledBy != "pacman" {
			t.Errorf("pkgs[%d].InstalledBy = %q, want %q", i, p.InstalledBy, "pacman")
		}
	}
}

// TestParsePacmanSearch_LeadingBlankLines verifies that leading blank lines
// (which some paru versions emit before the first result) don't produce
// phantom empty Package entries.
func TestParsePacmanSearch_LeadingBlankLines(t *testing.T) {
	raw := `
extra/ripgrep 14.1.0-1
    A search tool that combines the usability of ag with the raw speed of grep
`
	pkgs := parsePacmanSearch(raw)
	if len(pkgs) != 1 {
		t.Fatalf("parsePacmanSearch returned %d packages, want 1: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "ripgrep" {
		t.Errorf("pkgs[0].Name = %q, want %q", pkgs[0].Name, "ripgrep")
	}
}

// TestParsePacmanSearch_Empty verifies empty input produces no packages.
func TestParsePacmanSearch_Empty(t *testing.T) {
	pkgs := parsePacmanSearch("")
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %d: %+v", len(pkgs), pkgs)
	}
}
