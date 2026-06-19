package tools

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// TestAllPackagesInstalled verifies that a multi-package tool is only reported
// installed when EVERY platform package is present. A tool whose first package
// is installed but whose secondary package is missing must NOT be considered
// installed (C6): otherwise the install-skip guard permanently excludes a
// partially-installed tool from re-install.
func TestAllPackagesInstalled(t *testing.T) {
	tests := []struct {
		name      string
		installed []string
		pkgs      []string
		want      bool
	}{
		{
			name:      "all packages present",
			installed: []string{"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting"},
			pkgs:      []string{"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting"},
			want:      true,
		},
		{
			name:      "secondary package missing",
			installed: []string{"zsh"},
			pkgs:      []string{"zsh", "zsh-autosuggestions", "zsh-syntax-highlighting"},
			want:      false,
		},
		{
			name:      "primary package missing",
			installed: []string{"zsh-autosuggestions"},
			pkgs:      []string{"zsh", "zsh-autosuggestions"},
			want:      false,
		},
		{
			name:      "single package present",
			installed: []string{"ghostty"},
			pkgs:      []string{"ghostty"},
			want:      true,
		},
		{
			name:      "single package missing",
			installed: []string{},
			pkgs:      []string{"ghostty"},
			want:      false,
		},
		{
			name:      "no packages defined",
			installed: []string{"anything"},
			pkgs:      []string{},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := pkg.NewMockPackageManager()
			for _, p := range tt.installed {
				mgr.SetInstalled(p, "1.0.0")
			}
			if got := allPackagesInstalled(mgr, tt.pkgs); got != tt.want {
				t.Errorf("allPackagesInstalled(%v) = %v, want %v", tt.pkgs, got, tt.want)
			}
		})
	}
}
