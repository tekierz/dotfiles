package tools

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestZshPackagesIncludePowerlevel10k(t *testing.T) {
	packages := NewZshTool().Packages()

	assertContains(t, packages[pkg.PlatformMacOS], "powerlevel10k")
	assertContains(t, packages[pkg.PlatformArch], "zsh-theme-powerlevel10k")
}

func TestGenerateZshConfigPluginGating(t *testing.T) {
	t.Run("booleans enable supported plugins even with legacy non-empty plugin list", func(t *testing.T) {
		isolateHome(t)
		out := GenerateZshConfig(ZshConfig{
			Plugins:         []string{"zsh-completions"},
			Autosuggestions: true,
			SyntaxHighlight: true,
		}, "catppuccin-mocha")

		assertContainsText(t, out, "${HOMEBREW_PREFIX:-$(brew --prefix 2>/dev/null)}/share/zsh-autosuggestions/zsh-autosuggestions.zsh")
		assertContainsText(t, out, "${HOMEBREW_PREFIX:-$(brew --prefix 2>/dev/null)}/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh")
		assertContainsText(t, out, "$(brew --prefix)/share/zsh-completions:$FPATH")
	})

	t.Run("booleans disable supported plugins even when legacy list contains them", func(t *testing.T) {
		isolateHome(t)
		out := GenerateZshConfig(ZshConfig{
			Plugins:         []string{"zsh-autosuggestions", "zsh-syntax-highlighting"},
			Autosuggestions: false,
			SyntaxHighlight: false,
		}, "catppuccin-mocha")

		assertNotContainsText(t, out, "share/zsh-autosuggestions/zsh-autosuggestions.zsh")
		assertNotContainsText(t, out, "share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh")
	})
}

func TestGenerateZshConfigSourcesFzfConfig(t *testing.T) {
	isolateHome(t)
	out := GenerateZshConfig(ZshConfig{}, "catppuccin-mocha")

	assertContainsText(t, out, `[[ -r "$HOME/.config/fzf/fzf.zsh" ]] && source "$HOME/.config/fzf/fzf.zsh"`)
}

func TestGenerateZshConfigP10kSourceAndFallback(t *testing.T) {
	isolateHome(t)
	out := GenerateZshConfig(ZshConfig{PromptStyle: "p10k"}, "catppuccin-mocha")

	assertContainsText(t, out, "${HOMEBREW_PREFIX:-$(brew --prefix 2>/dev/null)}/share/powerlevel10k/powerlevel10k.zsh-theme")
	assertContainsText(t, out, "/usr/share/zsh-theme-powerlevel10k/powerlevel10k.zsh-theme")
	assertContainsText(t, out, `[[ -f "$HOME/.p10k.zsh" ]] && source "$HOME/.p10k.zsh"`)
	assertContainsText(t, out, "PROMPT='%~ > '")
}

func TestGenerateZshConfigEmitsSavedHotkeyAliases(t *testing.T) {
	isolateHome(t)

	if err := config.SaveGlobalConfig(&config.GlobalConfig{ActiveUser: "alice"}); err != nil {
		t.Fatalf("SaveGlobalConfig returned error: %v", err)
	}
	hotkeysCfg := &config.HotkeysConfig{Users: map[string]*config.UserHotkeys{
		"alice": {
			Aliases: map[string]string{
				"quote": "printf 'ship it'",
				"gs":    "git status --short",
			},
		},
		"bob": {
			Aliases: map[string]string{"wrong": "echo wrong user"},
		},
	}}
	if err := config.SaveHotkeysConfig(hotkeysCfg); err != nil {
		t.Fatalf("SaveHotkeysConfig returned error: %v", err)
	}

	out := GenerateZshConfig(ZshConfig{}, "catppuccin-mocha")

	assertContainsText(t, out, "alias -- 'gs=git status --short'")
	assertContainsText(t, out, "alias -- 'quote=printf '\\''ship it'\\'''")
	assertNotContainsText(t, out, "wrong user")
}

func TestWriteSavedAliasesQuotesAndFiltersUnsafeNames(t *testing.T) {
	var sb strings.Builder
	writeSavedAliases(&sb, map[string]string{
		"ok":        "echo ok",
		"quote":     "printf 'ship it'",
		"-bad":      "echo nope",
		"bad name":  "echo nope",
		"bad;name":  "echo nope",
		"bad=name":  "echo nope",
		"multiline": "echo nope\nrm -rf /",
	})

	out := sb.String()
	assertContainsText(t, out, "alias -- 'ok=echo ok'")
	assertContainsText(t, out, "alias -- 'quote=printf '\\''ship it'\\'''")
	assertNotContainsText(t, out, "-bad")
	assertNotContainsText(t, out, "bad name")
	assertNotContainsText(t, out, "bad;name")
	assertNotContainsText(t, out, "bad=name")
	assertNotContainsText(t, out, "multiline")
}

func TestMergeZshManagedSectionReplacesExistingSection(t *testing.T) {
	oldManaged := []byte(wrapZshManagedSection("old managed\n"))
	newManaged := []byte(wrapZshManagedSection("new managed\n"))
	existing := []byte("user before\n\n" + string(oldManaged) + "\nuser after\n")

	got, err := mergeZshManagedSection(existing, newManaged)
	if err != nil {
		t.Fatalf("mergeZshManagedSection returned error: %v", err)
	}

	out := string(got)
	assertContainsText(t, out, "user before")
	assertContainsText(t, out, "new managed")
	assertContainsText(t, out, "user after")
	assertNotContainsText(t, out, "old managed")

	again, err := mergeZshManagedSection(got, newManaged)
	if err != nil {
		t.Fatalf("second mergeZshManagedSection returned error: %v", err)
	}
	if string(again) != out {
		t.Fatalf("merge is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, string(again))
	}
}

func TestMergeZshManagedSectionRefusesIncompleteMarkers(t *testing.T) {
	managed := []byte(wrapZshManagedSection("new managed\n"))
	cases := map[string][]byte{
		"start only": []byte("user\n" + zshManagedStart + "\nold\n"),
		"end only":   []byte("user\n" + zshManagedEnd + "\n"),
	}

	for name, existing := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := mergeZshManagedSection(existing, managed); err == nil {
				t.Fatal("mergeZshManagedSection returned nil error, want refusal")
			}
		})
	}
}

func TestMergeZshManagedSectionAppendsToUserConfig(t *testing.T) {
	managed := []byte(wrapZshManagedSection("managed\n"))
	existing := []byte("# user config\nalias mine='true'\n")

	got, err := mergeZshManagedSection(existing, managed)
	if err != nil {
		t.Fatalf("mergeZshManagedSection returned error: %v", err)
	}

	out := string(got)
	assertContainsText(t, out, "# user config")
	assertContainsText(t, out, "alias mine='true'")
	assertContainsText(t, out, zshManagedStart)
	assertContainsText(t, out, "managed")
}

func TestMergeZshManagedSectionReplacesLegacyGeneratedConfig(t *testing.T) {
	managed := []byte(wrapZshManagedSection("new managed\n"))
	existing := []byte("# Generated by dotfiles TUI\n# Theme: old\n\nHISTSIZE=1000\n# Prompt\nPROMPT='old'\n")

	got, err := mergeZshManagedSection(existing, managed)
	if err != nil {
		t.Fatalf("mergeZshManagedSection returned error: %v", err)
	}

	if string(got) != string(managed) {
		t.Fatalf("legacy generated config was not replaced:\n%s", string(got))
	}
}

func TestFzfToolTracksGeneratedConfigPath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	paths := NewFzfTool().ConfigPaths()
	want := filepath.Join(tmpHome, ".config", "fzf", "fzf.zsh")
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("ConfigPaths() = %#v, want [%q]", paths, want)
	}
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, got := range values {
		if got == want {
			return
		}
	}
	t.Fatalf("%q not found in %#v", want, values)
}

func assertContainsText(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q:\n%s", want, got)
	}
}

func assertNotContainsText(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Fatalf("expected output not to contain %q:\n%s", want, got)
	}
}

func isolateHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	return tmpHome
}
