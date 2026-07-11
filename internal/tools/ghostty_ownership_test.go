package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestWriteGhosttyConfigPreservesNativeSettingsInManagedBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("# personal Ghostty settings\n" +
		"font-family = Berkeley Mono\n" +
		"font-family = Symbols Nerd Font\n" +
		"unreleased-setting = preserve-me\n" +
		"# marker text in prose: " + ghosttyManagedStart + "\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := ownershipGhosttyConfig()
	if err := WriteGhosttyConfig(cfg, "catppuccin-mocha"); err != nil {
		t.Fatalf("WriteGhosttyConfig: %v", err)
	}

	got := mustReadOwnershipFile(t, path)
	if !bytes.HasPrefix(got, original) {
		t.Fatalf("native Ghostty settings were not preserved byte-for-byte:\n%s", got)
	}
	if count := len(exactManagedMarkerLines(string(got), ghosttyManagedStart)); count != 1 {
		t.Fatalf("managed Ghostty start count = %d, want 1:\n%s", count, got)
	}
	for _, want := range []string{ghosttyManagedEnd, "font-size = 15", "scrollback-limit = 25000", "unreleased-setting = preserve-me"} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("Ghostty config missing %q:\n%s", want, got)
		}
	}
	assertOwnershipFileMode(t, path, 0o600)
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")

	first := append([]byte(nil), got...)
	if err := WriteGhosttyConfig(cfg, "catppuccin-mocha"); err != nil {
		t.Fatalf("idempotent WriteGhosttyConfig: %v", err)
	}
	if got := mustReadOwnershipFile(t, path); !bytes.Equal(got, first) {
		t.Fatalf("idempotent write changed Ghostty config:\n%s", got)
	}
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")

	cfg.FontSize = 19
	if err := WriteGhosttyConfig(cfg, "dracula"); err != nil {
		t.Fatalf("update managed Ghostty config: %v", err)
	}
	updated := mustReadOwnershipFile(t, path)
	if !bytes.HasPrefix(updated, original) || !bytes.Contains(updated, []byte("font-size = 19")) || bytes.Contains(updated, []byte("font-size = 15")) {
		t.Fatalf("managed Ghostty update did not preserve the native prefix and replace its section:\n%s", updated)
	}
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")
}

func TestWriteGhosttyConfigCreatesMissingPrivateParents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config.ghostty")

	if err := WriteGhosttyConfig(ownershipGhosttyConfig(), "dracula"); err != nil {
		t.Fatalf("WriteGhosttyConfig: %v", err)
	}
	got := mustReadOwnershipFile(t, path)
	if !bytes.HasPrefix(got, []byte(ghosttyManagedStart+"\n")) || !bytes.Contains(got, []byte(ghosttyManagedEnd)) {
		t.Fatalf("new Ghostty config is not an owned managed section:\n%s", got)
	}
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")
	assertOwnershipFileMode(t, filepath.Join(home, ".config"), 0o700)
	assertOwnershipFileMode(t, filepath.Dir(path), 0o700)
}

func TestWriteGhosttyConfigRefusesMalformedManagedSections(t *testing.T) {
	cases := map[string]string{
		"start only":         ghosttyManagedStart + "\nfont-size = 10\n",
		"end only":           ghosttyManagedEnd + "\n",
		"duplicate sections": string(wrapManagedConfigSection(ghosttyManagedStart, ghosttyManagedEnd, "font-size = 10")) + string(wrapManagedConfigSection(ghosttyManagedStart, ghosttyManagedEnd, "font-size = 11")),
		"two starts":         ghosttyManagedStart + "\n" + ghosttyManagedStart + "\n" + ghosttyManagedEnd + "\n",
		"reversed":           ghosttyManagedEnd + "\nuser-setting = keep\n" + ghosttyManagedStart + "\n",
	}

	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			path := filepath.Join(home, ".config", "ghostty", "config")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := WriteGhosttyConfig(ownershipGhosttyConfig(), "dracula"); err == nil {
				t.Fatal("WriteGhosttyConfig accepted malformed managed section")
			}
			if got := mustReadOwnershipFile(t, path); string(got) != original {
				t.Fatalf("malformed Ghostty config changed:\n%s", got)
			}
			assertOwnershipPathAbsent(t, path+".dotfiles.bak")
		})
	}
}

func TestWriteGhosttyConfigReplacesOnlyExistingManagedSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	before := "# before\nfont-family = Keep Before\n\n"
	after := "\n# after\nunreleased-setting = keep-after\n"
	original := before + string(wrapManagedConfigSection(ghosttyManagedStart, ghosttyManagedEnd, "font-size = 9")) + after
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteGhosttyConfig(ownershipGhosttyConfig(), "dracula"); err != nil {
		t.Fatalf("WriteGhosttyConfig: %v", err)
	}
	got := string(mustReadOwnershipFile(t, path))
	if !strings.HasPrefix(got, before) || !strings.HasSuffix(got, after) {
		t.Fatalf("bytes outside existing Ghostty managed section changed:\n%s", got)
	}
	if strings.Contains(got, "font-size = 9") || !strings.Contains(got, "font-size = 15") {
		t.Fatalf("existing Ghostty managed section was not replaced:\n%s", got)
	}
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")
}

func TestWriteGhosttyConfigLeavesUnownedBackupSidecarUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("unreleased-setting = keep\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".dotfiles.bak", []byte("occupied\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteGhosttyConfig(ownershipGhosttyConfig(), "dracula"); err != nil {
		t.Fatalf("WriteGhosttyConfig: %v", err)
	}
	if got := mustReadOwnershipFile(t, path); !bytes.HasPrefix(got, original) || !bytes.Contains(got, []byte(ghosttyManagedStart)) {
		t.Fatalf("Ghostty config did not preserve native bytes while adopting:\n%s", got)
	}
	if got := mustReadOwnershipFile(t, path+".dotfiles.bak"); string(got) != "occupied\n" {
		t.Fatalf("unowned sidecar changed: %q", got)
	}
}

func TestWriteGhosttyConfigRefusesSymlinkedOwnershipPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink ownership checks are Unix-specific")
	}

	t.Run("native config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		path := filepath.Join(home, ".config", "ghostty", "config")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(home, "victim")
		if err := os.WriteFile(victim, []byte("do not change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victim, path); err != nil {
			t.Fatal(err)
		}

		err := WriteGhosttyConfig(ownershipGhosttyConfig(), "dracula")
		if !errors.Is(err, safefile.ErrSymlink) {
			t.Fatalf("WriteGhosttyConfig error = %v, want safefile.ErrSymlink", err)
		}
		if got := mustReadOwnershipFile(t, victim); string(got) != "do not change\n" {
			t.Fatalf("native config symlink victim changed: %q", got)
		}
	})

	t.Run("parent directory", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		if err := os.Mkdir(filepath.Join(home, ".config"), 0o700); err != nil {
			t.Fatal(err)
		}
		victimDir := filepath.Join(home, "victim-dir")
		if err := os.Mkdir(victimDir, 0o700); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(victimDir, "config")
		if err := os.WriteFile(victim, []byte("do not change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victimDir, filepath.Join(home, ".config", "ghostty")); err != nil {
			t.Fatal(err)
		}

		err := WriteGhosttyConfig(ownershipGhosttyConfig(), "dracula")
		if !errors.Is(err, safefile.ErrSymlink) {
			t.Fatalf("WriteGhosttyConfig error = %v, want safefile.ErrSymlink", err)
		}
		if got := mustReadOwnershipFile(t, victim); string(got) != "do not change\n" {
			t.Fatalf("parent symlink victim changed: %q", got)
		}
	})

	t.Run("unowned backup sidecar", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		path := filepath.Join(home, ".config", "ghostty", "config")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		original := []byte("unreleased-setting = keep\n")
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(home, "victim")
		if err := os.WriteFile(victim, []byte("do not change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victim, path+".dotfiles.bak"); err != nil {
			t.Fatal(err)
		}

		err := WriteGhosttyConfig(ownershipGhosttyConfig(), "dracula")
		if err != nil {
			t.Fatalf("WriteGhosttyConfig: %v", err)
		}
		if got := mustReadOwnershipFile(t, path); !bytes.HasPrefix(got, original) || !bytes.Contains(got, []byte(ghosttyManagedStart)) {
			t.Fatalf("Ghostty config did not adopt native config:\n%s", got)
		}
		if got := mustReadOwnershipFile(t, victim); string(got) != "do not change\n" {
			t.Fatalf("backup symlink victim changed: %q", got)
		}
	})
}

func ownershipGhosttyConfig() GhosttyConfig {
	return GhosttyConfig{
		FontSize:          15,
		FontFamily:        "JetBrainsMono Nerd Font",
		Opacity:           90,
		BlurRadius:        12,
		TabBindings:       "super",
		ScrollbackLines:   25000,
		CursorStyle:       "block",
		WindowDecorations: true,
		ConfirmClose:      true,
	}
}

func TestGenerateGhosttyConfigRejectsManagedMarkerInjection(t *testing.T) {
	got := GenerateGhosttyConfig(GhosttyConfig{
		FontFamily:      "Safe Font\n" + ghosttyManagedEnd + "\nfont-family = Attacker",
		FontSize:        -10,
		Opacity:         101,
		BlurRadius:      101,
		TabBindings:     "invalid\nkeybind = super+x=close_all_windows",
		ScrollbackLines: -1,
		CursorStyle:     "invalid\ncursor-style = bar",
	}, "dracula")
	if len(exactManagedMarkerLines(got, ghosttyManagedEnd)) != 0 || strings.Count(got, "\nfont-family = ") != 1 || strings.Contains(got, "super+x") {
		t.Fatalf("generated Ghostty config contains injected directive or marker:\n%s", got)
	}
	for _, want := range []string{"font-size = 14", "cursor-style = block", "scrollback-limit = 10000", "keybind = super+t=new_tab"} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized Ghostty config missing %q:\n%s", want, got)
		}
	}
}
