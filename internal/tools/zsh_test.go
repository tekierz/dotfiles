package tools

import (
	"os"
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

func TestMergeZshManagedSectionRefusesMultipleOrReversedSections(t *testing.T) {
	managed := []byte(wrapZshManagedSection("new managed\n"))
	section := wrapZshManagedSection("old managed\n")
	cases := map[string][]byte{
		"two complete sections": []byte(section + "user middle\n" + section),
		"two starts":            []byte(zshManagedStart + "\n" + zshManagedStart + "\n" + zshManagedEnd + "\n"),
		"end before start":      []byte(zshManagedEnd + "\nuser\n" + zshManagedStart + "\n"),
	}

	for name, existing := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := mergeZshManagedSection(existing, managed); err == nil {
				t.Fatal("mergeZshManagedSection returned nil error, want ambiguity refusal")
			}
		})
	}
}

func TestMergeZshManagedSectionIgnoresMarkerTextInsideUserLines(t *testing.T) {
	managed := []byte(wrapZshManagedSection("managed\n"))
	existing := []byte("print -r -- '" + zshManagedStart + "'\n" +
		"alias keep='true'\n" +
		"print -r -- '" + zshManagedEnd + "'\n")

	got, err := mergeZshManagedSection(existing, managed)
	if err != nil {
		t.Fatalf("mergeZshManagedSection returned error: %v", err)
	}
	out := string(got)
	for _, want := range []string{string(existing), "alias keep='true'", string(managed)} {
		if !strings.Contains(out, want) {
			t.Fatalf("merged config dropped %q:\n%s", want, out)
		}
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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	managed := []byte(wrapZshManagedSection("new managed\n"))
	for _, style := range []string{"p10k", "starship", "pure", "minimal"} {
		t.Run(style, func(t *testing.T) {
			existing := []byte(GenerateZshConfig(ZshConfig{PromptStyle: style}, "catppuccin-mocha"))
			got, err := mergeZshManagedSection(existing, managed)
			if err != nil {
				t.Fatalf("mergeZshManagedSection returned error: %v", err)
			}
			if string(got) != string(managed) {
				t.Fatalf("legacy generated config was not replaced:\n%s", string(got))
			}
		})
	}
}

func TestMergeZshManagedSectionPreservesLegacyAppendedUserContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	managed := []byte(wrapZshManagedSection("new managed\n"))
	legacy := GenerateZshConfig(ZshConfig{PromptStyle: "minimal"}, "catppuccin-mocha")
	userTail := "\n# Added by the user\nexport WORKSPACE=~/src\nalias mine='git status'\n"

	got, err := mergeZshManagedSection([]byte(legacy+userTail), managed)
	if err != nil {
		t.Fatalf("mergeZshManagedSection returned error: %v", err)
	}
	if !strings.HasPrefix(string(got), string(managed)) {
		t.Fatalf("legacy config did not migrate to managed section:\n%s", got)
	}
	if !strings.HasSuffix(string(got), userTail) {
		t.Fatalf("legacy user tail was not preserved byte-for-byte:\n%s", got)
	}
	if strings.Contains(string(got), "# Theme: catppuccin-mocha") {
		t.Fatalf("legacy generated body survived migration:\n%s", got)
	}
}

func TestMergeZshManagedSectionPreservesCRLFLegacyTail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	managed := []byte(wrapZshManagedSection("new managed\n"))
	legacy := GenerateZshConfig(ZshConfig{PromptStyle: "pure"}, "catppuccin-mocha")
	legacy = strings.ReplaceAll(legacy, "\n", "\r\n")
	userTail := "# user tail\r\nalias mine='true'\r\n"

	got, err := mergeZshManagedSection([]byte(legacy+userTail), managed)
	if err != nil {
		t.Fatalf("mergeZshManagedSection returned error: %v", err)
	}
	if !strings.HasSuffix(string(got), userTail) {
		t.Fatalf("CRLF legacy user tail was not preserved byte-for-byte:\n%q", got)
	}
}

func TestMergeZshManagedSectionRefusesUnknownLegacyShape(t *testing.T) {
	managed := []byte(wrapZshManagedSection("new managed\n"))
	existing := []byte("# Generated by dotfiles TUI\n# Theme: old\n\n# Prompt\nsource ~/.unknown-prompt.zsh\n# user data that must not be guessed about\n")

	if _, err := mergeZshManagedSection(existing, managed); err == nil {
		t.Fatal("mergeZshManagedSection accepted an unrecognized legacy shape")
	}
}

func TestWriteZshConfigPreservesLegacyAppendedUserContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".zshrc")
	legacy := GenerateZshConfig(ZshConfig{PromptStyle: "minimal", HistorySize: 1000}, "catppuccin-mocha")
	userTail := "\n# personal tail\nexport EDITOR=nvim\nalias work='cd ~/src'\n"
	if err := os.WriteFile(path, []byte(legacy+userTail), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteZshConfig(ZshConfig{PromptStyle: "pure", HistorySize: 5000}, "dracula")
	if err != nil {
		t.Fatalf("WriteZshConfig returned error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(got), userTail) {
		t.Fatalf("WriteZshConfig dropped legacy user tail:\n%s", got)
	}
	if !strings.Contains(string(got), zshManagedStart) || !strings.Contains(string(got), "HISTSIZE=5000") {
		t.Fatalf("WriteZshConfig did not install the new managed section:\n%s", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("migrated .zshrc mode = %04o, want 0600", info.Mode().Perm())
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
