package pkg

import (
	"errors"
	"reflect"
	"testing"
)

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
	if pkgs[0].Name != "linux" {
		t.Errorf("pkgs[0].Name = %q, want %q", pkgs[0].Name, "linux")
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

func TestPacmanUpdateArgsUseFullSyncUpgrade(t *testing.T) {
	args := pacmanUpdateArgs(nil, []string{"vim"})
	want := []string{"-Syu", "--noconfirm", "vim"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("pacmanUpdateArgs() = %v, want %v", args, want)
	}

	paruArgs := pacmanUpdateArgs([]string{"--skipreview", "--noprovides"}, []string{"paru"})
	paruWant := []string{"-Syu", "--noconfirm", "--skipreview", "--noprovides", "paru"}
	if !reflect.DeepEqual(paruArgs, paruWant) {
		t.Fatalf("pacmanUpdateArgs() with paru flags = %v, want %v", paruArgs, paruWant)
	}
}

func TestClassifyPacmanQuResult_NoUpdatesIgnoresBenignStderr(t *testing.T) {
	stderr := `warning: config file /etc/pacman.conf, line 42: directive 'UseSyslog' in section 'options' not recognized.
warning: ignoring package upgrade (linux: 6.9.1.arch1-1 => 6.9.2.arch1-1)
warning: foo: local (1.0-2) is newer than core (1.0-1)
`

	out, err := classifyPacmanQuResult(1, "", stderr, errors.New("exit status 1"))
	if err != nil {
		t.Fatalf("classifyPacmanQuResult returned error for no-updates stderr: %v", err)
	}
	if out != "" {
		t.Fatalf("classifyPacmanQuResult output = %q, want empty", out)
	}
}

func TestClassifyPacmanQuResult_DatabaseErrors(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stdout   string
		stderr   string
	}{
		{
			name:     "db lock on non one exit",
			exitCode: 2,
			stderr: `error: failed to init transaction (unable to lock database)
error: could not lock database: File exists
`,
		},
		{
			name:     "sync db error with stdout",
			exitCode: 1,
			stdout:   "linux 6.9.1.arch1-1 -> 6.9.2.arch1-1\n",
			stderr:   "error: failed to synchronize all databases (invalid or corrupted database (PGP signature))\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := classifyPacmanQuResult(tt.exitCode, tt.stdout, tt.stderr, errors.New("exit status"))
			if err == nil {
				t.Fatalf("classifyPacmanQuResult returned nil error with output %q", out)
			}
		})
	}
}
