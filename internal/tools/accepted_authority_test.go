package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestAcceptedWholeFileWritersRejectIdenticalNamespaceReplacement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	theme := "dracula"
	tests := []struct {
		name  string
		path  func() (string, error)
		seed  func() error
		apply func(safefile.Revision) error
	}{
		{
			name: "fzf",
			path: func() (string, error) { return filepath.Join(home, ".config", "fzf", "fzf.zsh"), nil },
			seed: func() error { return WriteFzfConfig(FzfConfig{Layout: "reverse", Height: 40}, theme) },
			apply: func(accepted safefile.Revision) error {
				_, err := WriteFzfConfigAtRevisionTracked(FzfConfig{Layout: "reverse", Height: 40}, theme, accepted)
				return err
			},
		},
		{
			name: "lazygit",
			path: func() (string, error) { return filepath.Join(home, ".config", "lazygit", "config.yml"), nil },
			seed: func() error {
				return WriteLazyGitConfig(LazyGitConfig{SideBySide: true, Theme: "dark", Paging: "never"}, theme)
			},
			apply: func(accepted safefile.Revision) error {
				_, err := WriteLazyGitConfigAtRevisionTracked(LazyGitConfig{SideBySide: true, Theme: "dark", Paging: "never"}, theme, accepted)
				return err
			},
		},
		{
			name: "glow",
			path: glowConfigPath,
			seed: func() error { return WriteGlowConfig(GlowConfig{Style: "auto", Width: 100}, theme) },
			apply: func(accepted safefile.Revision) error {
				_, err := WriteGlowConfigAtRevisionTracked(GlowConfig{Style: "auto", Width: 100}, theme, accepted)
				return err
			},
		},
		{
			name: "tmux",
			path: func() (string, error) { return filepath.Join(home, ".tmux.conf"), nil },
			seed: func() error { return WriteTmuxConfig(TmuxConfig{Prefix: "ctrl-a"}, theme) },
			apply: func(accepted safefile.Revision) error {
				_, err := WriteTmuxConfigAtRevisionTracked(TmuxConfig{Prefix: "ctrl-a"}, theme, accepted)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.seed(); err != nil {
				t.Fatalf("seed config: %v", err)
			}
			path, err := test.path()
			if err != nil {
				t.Fatalf("resolve path: %v", err)
			}
			accepted := readAcceptedFileRevision(t, path)
			replaceFileWithIdenticalNewIdentity(t, path)
			if err := test.apply(accepted); !errors.Is(err, safefile.ErrRevisionChanged) {
				t.Fatalf("accepted writer error = %v, want ErrRevisionChanged", err)
			}
		})
	}
}

func TestAcceptedFragmentWritersCheckIdentityBeforeSameContentNoop(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	theme := "dracula"

	ghosttyPath := ghosttyConfigCandidates(home)[0]
	ghosttyCfg := GhosttyConfig{FontSize: 14, FontFamily: "JetBrains Mono"}
	if err := WriteGhosttyConfigAt(ghosttyPath, ghosttyCfg, theme); err != nil {
		t.Fatal(err)
	}
	ghosttyAccepted := readAcceptedFileRevision(t, ghosttyPath)
	replaceFileWithIdenticalNewIdentity(t, ghosttyPath)
	if _, err := WriteGhosttyConfigAtRevisionTracked(ghosttyPath, ghosttyCfg, theme, ghosttyAccepted); !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("Ghostty error = %v, want ErrRevisionChanged", err)
	}

	zshCfg := ZshConfig{PromptStyle: "minimal", HistorySize: 1000}
	if err := WriteZshConfig(zshCfg, theme); err != nil {
		t.Fatal(err)
	}
	zshPath := filepath.Join(home, ".zshrc")
	zshAccepted := readAcceptedFileRevision(t, zshPath)
	replaceFileWithIdenticalNewIdentity(t, zshPath)
	if _, err := WriteZshConfigAtRevisionTracked(zshCfg, theme, zshAccepted); !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("Zsh error = %v, want ErrRevisionChanged", err)
	}
}

func TestAcceptedYaziPreflightsCompleteSetBeforeMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	initial := YaziConfig{Keymap: "vim", ShowHidden: false}
	if err := WriteYaziConfig(initial, "dracula"); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(home, ".config", "yazi")
	yaziPath := filepath.Join(base, "yazi.toml")
	keymapPath := filepath.Join(base, "keymap.toml")
	themePath := filepath.Join(base, "theme.toml")
	yaziAccepted := readAcceptedFileRevision(t, yaziPath)
	keymapAccepted := readAcceptedFileRevision(t, keymapPath)
	themeAccepted := readAcceptedFileRevision(t, themePath)
	yaziBefore, err := os.ReadFile(yaziPath)
	if err != nil {
		t.Fatal(err)
	}
	replaceFileWithIdenticalNewIdentity(t, keymapPath)
	_, err = WriteYaziConfigAtRevisionsTracked(YaziConfig{Keymap: "emacs", ShowHidden: true}, "nord", yaziAccepted, keymapAccepted, themeAccepted)
	if !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("Yazi error = %v, want ErrRevisionChanged", err)
	}
	yaziAfter, err := os.ReadFile(yaziPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(yaziAfter) != string(yaziBefore) {
		t.Fatal("Yazi mutated an earlier target before complete accepted-set preflight")
	}
}

func TestAcceptedBtopPreflightsCompleteSetBeforeMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	cfg := BtopConfig{UpdateMs: 2000, GraphType: "braille"}
	if err := WriteBtopConfig(cfg, "dracula"); err != nil {
		t.Fatal(err)
	}
	themePath := filepath.Join(home, ".config", "btop", "themes", "dracula.theme")
	configPath := filepath.Join(home, ".config", "btop", "btop.conf")
	themeAccepted := readAcceptedFileRevision(t, themePath)
	configAccepted := readAcceptedFileRevision(t, configPath)
	themeBefore, err := os.ReadFile(themePath)
	if err != nil {
		t.Fatal(err)
	}
	replaceFileWithIdenticalNewIdentity(t, configPath)
	cfg.UpdateMs = 500
	_, err = WriteBtopConfigAtRevisionsTracked(cfg, "dracula", themeAccepted, configAccepted)
	if !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("btop error = %v, want ErrRevisionChanged", err)
	}
	themeAfter, err := os.ReadFile(themePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(themeAfter) != string(themeBefore) {
		t.Fatal("btop mutated the first target before complete accepted-set preflight")
	}
}

func TestAcceptedGitPreflightsCompleteSetBeforeMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := GitConfig{DefaultBranch: "main"}
	if err := WriteGitConfig(cfg, "dracula"); err != nil {
		t.Fatal(err)
	}
	rootPath := filepath.Join(home, ".gitconfig")
	managedPath := filepath.Join(home, filepath.FromSlash(gitManagedConfigRel))
	rootAccepted := readAcceptedFileRevision(t, rootPath)
	managedAccepted := readAcceptedFileRevision(t, managedPath)
	managedBefore, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatal(err)
	}
	replaceFileWithIdenticalNewIdentity(t, rootPath)
	cfg.DefaultBranch = "trunk"
	_, err = WriteGitConfigAtRevisionsTracked(cfg, "nord", rootAccepted, managedAccepted)
	if !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("Git error = %v, want ErrRevisionChanged", err)
	}
	managedAfter, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(managedAfter) != string(managedBefore) {
		t.Fatal("Git mutated the managed include before complete accepted-set preflight")
	}
}

func TestAcceptedFragmentAndMultiFileWritersCreateNoBackupSidecars(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	ghosttyPath := ghosttyConfigCandidates(home)[0]
	if err := WriteGhosttyConfigAt(ghosttyPath, GhosttyConfig{FontSize: 12}, "dracula"); err != nil {
		t.Fatal(err)
	}
	ghosttyAccepted := readAcceptedFileRevision(t, ghosttyPath)
	if _, err := WriteGhosttyConfigAtRevisionTracked(ghosttyPath, GhosttyConfig{FontSize: 15}, "nord", ghosttyAccepted); err != nil {
		t.Fatal(err)
	}

	gitCfg := GitConfig{DefaultBranch: "main"}
	if err := WriteGitConfig(gitCfg, "dracula"); err != nil {
		t.Fatal(err)
	}
	rootAccepted := readAcceptedFileRevision(t, filepath.Join(home, ".gitconfig"))
	managedAccepted := readAcceptedFileRevision(t, filepath.Join(home, filepath.FromSlash(gitManagedConfigRel)))
	gitCfg.DefaultBranch = "trunk"
	if _, err := WriteGitConfigAtRevisionsTracked(gitCfg, "nord", rootAccepted, managedAccepted); err != nil {
		t.Fatal(err)
	}

	yaziCfg := YaziConfig{Keymap: "vim"}
	if err := WriteYaziConfig(yaziCfg, "dracula"); err != nil {
		t.Fatal(err)
	}
	yaziBase := filepath.Join(home, ".config", "yazi")
	yaziAccepted := readAcceptedFileRevision(t, filepath.Join(yaziBase, "yazi.toml"))
	keymapAccepted := readAcceptedFileRevision(t, filepath.Join(yaziBase, "keymap.toml"))
	themeAccepted := readAcceptedFileRevision(t, filepath.Join(yaziBase, "theme.toml"))
	yaziCfg.ShowHidden = true
	if _, err := WriteYaziConfigAtRevisionsTracked(yaziCfg, "nord", yaziAccepted, keymapAccepted, themeAccepted); err != nil {
		t.Fatal(err)
	}

	err := filepath.Walk(home, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".bak" || filepath.Base(path) == ".dotfiles.bak" {
			t.Errorf("accepted writer created unplanned backup sidecar %s", path)
		}
		if strings.HasSuffix(path, ".dotfiles.lock") {
			t.Errorf("writer created adjacent config lock sidecar %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAcceptedClaudeWriterRejectsIdenticalReplacement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tool := NewClaudeCodeTool()
	if err := tool.ApplyConfigWithMCPs(map[string]bool{"context7": true}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude.json")
	accepted := readAcceptedFileRevision(t, path)
	replaceFileWithIdenticalNewIdentity(t, path)
	if _, err := tool.ApplyConfigWithMCPsAtRevisionTracked(map[string]bool{"context7": true}, accepted); !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("Claude error = %v, want ErrRevisionChanged", err)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("accepted Claude writer created backup sidecar: %v", err)
	}
}

func TestAcceptedDirectoryWritersRejectIdenticalReplacementBeforeMutation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	nvimPath := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(nvimPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nvimPath, "init.lua"), []byte("-- same\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nvimAccepted, err := safefile.SnapshotDirectoryWithin(home, filepath.ToSlash(filepath.Join(".config", "nvim")))
	if err != nil {
		t.Fatal(err)
	}
	replaceDirectoryWithIdenticalNewIdentity(t, nvimPath)
	if _, err := WriteNeovimConfigAtSnapshotTracked(NeovimConfig{ConfigPreset: "kickstart"}, "dracula", nvimAccepted); !errors.Is(err, safefile.ErrDirectoryChanged) {
		t.Fatalf("Neovim error = %v, want ErrDirectoryChanged", err)
	}

	tmuxCfg := TmuxConfig{Prefix: "ctrl-a", TPMEnabled: true}
	if err := WriteTmuxConfig(tmuxCfg, "dracula"); err != nil {
		t.Fatal(err)
	}
	tmuxAccepted := readAcceptedFileRevision(t, filepath.Join(home, ".tmux.conf"))
	tpmPath := filepath.Join(home, ".tmux", "plugins", "tpm")
	if err := os.MkdirAll(tpmPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tpmPath, "tpm"), []byte("same\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	tpmAccepted, err := safefile.SnapshotDirectoryWithin(home, filepath.ToSlash(filepath.Join(".tmux", "plugins", "tpm")))
	if err != nil {
		t.Fatal(err)
	}
	replaceDirectoryWithIdenticalNewIdentity(t, tpmPath)
	if _, err := SetupTPMAtAuthorityTracked(tmuxCfg, "dracula", tmuxAccepted, tpmAccepted); !errors.Is(err, safefile.ErrDirectoryChanged) {
		t.Fatalf("TPM error = %v, want ErrDirectoryChanged", err)
	}
}

func readAcceptedFileRevision(t *testing.T, path string) safefile.Revision {
	t.Helper()
	root, rel, _, err := generatedConfigDestination(path)
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := safefile.ReadWithin(root, rel)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func replaceFileWithIdenticalNewIdentity(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, content, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
}

func replaceDirectoryWithIdenticalNewIdentity(t *testing.T, path string) {
	t.Helper()
	name := filepath.Base(path)
	parent := filepath.Dir(path)
	replacement := filepath.Join(parent, name+"-replacement")
	if err := copyTestDirectory(path, replacement); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(parent, name+"-old")
	if err := os.Rename(path, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
}

func copyTestDirectory(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
}
