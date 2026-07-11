package tools

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestWriteGitConfigPreservesNativeConfigThroughManagedInclude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".gitconfig")
	original := []byte("# personal Git configuration\n" +
		"[user]\n\tname = Local User\n\temail = local@example.test\n" +
		"[core]\n\teditor = helix\n" +
		"[core]\n\tunknownFutureSetting = keep-me\n" +
		"# marker text in prose: " + gitManagedIncludeStart + "\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := GitConfig{
		DefaultBranch:    "develop",
		Aliases:          []string{"st", "lg"},
		PullRebase:       true,
		AutoSetupRemote:  true,
		DiffTool:         "delta",
		MergeTool:        "nvimdiff",
		CredentialHelper: "osxkeychain",
	}
	if err := WriteGitConfig(cfg, "catppuccin-mocha"); err != nil {
		t.Fatalf("WriteGitConfig: %v", err)
	}

	rootConfig := mustReadOwnershipFile(t, path)
	if !bytes.HasPrefix(rootConfig, original) {
		t.Fatalf("native Git config was not preserved byte-for-byte:\n%s", rootConfig)
	}
	if got := len(exactManagedMarkerLines(string(rootConfig), gitManagedIncludeStart)); got != 1 {
		t.Fatalf("managed Git include start count = %d, want 1:\n%s", got, rootConfig)
	}
	for _, want := range []string{gitManagedIncludeEnd, "[include]", "path = " + gitManagedIncludePath} {
		if !bytes.Contains(rootConfig, []byte(want)) {
			t.Fatalf("native Git config missing %q:\n%s", want, rootConfig)
		}
	}
	if bytes.Contains(rootConfig, []byte("defaultBranch = develop")) {
		t.Fatalf("generated Git settings leaked into the user-owned root file:\n%s", rootConfig)
	}

	managedPath := filepath.Join(home, filepath.FromSlash(gitManagedConfigRel))
	managed := mustReadOwnershipFile(t, managedPath)
	if want := []byte(GenerateGitConfig(cfg, "catppuccin-mocha")); !bytes.Equal(managed, want) {
		t.Fatalf("managed Git config differs from generated content:\ngot:\n%s\nwant:\n%s", managed, want)
	}
	assertOwnershipFileMode(t, managedPath, 0o600)
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")

	// Verify the include with Git itself when available, rather than only
	// asserting that the emitted text looks plausible.
	if gitPath, err := exec.LookPath("git"); err == nil {
		cmd := exec.Command(gitPath, "config", "--includes", "--file", path, "--get", "init.defaultBranch")
		cmd.Env = append(os.Environ(), "HOME="+home)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("Git could not resolve managed include: %v", err)
		}
		if got := strings.TrimSpace(string(output)); got != "develop" {
			t.Fatalf("resolved init.defaultBranch = %q, want develop", got)
		}
	}

	rootBefore := append([]byte(nil), rootConfig...)
	managedBefore := append([]byte(nil), managed...)
	if err := WriteGitConfig(cfg, "catppuccin-mocha"); err != nil {
		t.Fatalf("idempotent WriteGitConfig: %v", err)
	}
	if got := mustReadOwnershipFile(t, path); !bytes.Equal(got, rootBefore) {
		t.Fatalf("idempotent write changed native Git config:\n%s", got)
	}
	if got := mustReadOwnershipFile(t, managedPath); !bytes.Equal(got, managedBefore) {
		t.Fatalf("idempotent write changed managed Git bytes:\n%s", got)
	}
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")

	cfg.DefaultBranch = "main"
	if err := WriteGitConfig(cfg, "dracula"); err != nil {
		t.Fatalf("update managed Git config: %v", err)
	}
	if got := mustReadOwnershipFile(t, path); !bytes.Equal(got, rootBefore) {
		t.Fatalf("managed setting update changed native Git config:\n%s", got)
	}
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")
	if got := mustReadOwnershipFile(t, managedPath); !bytes.Contains(got, []byte("defaultBranch = main")) {
		t.Fatalf("managed Git config was not updated:\n%s", got)
	}
}

func TestWriteGitConfigRefusesMalformedManagedIncludes(t *testing.T) {
	cases := map[string]string{
		"start only":         gitManagedIncludeStart + "\n[include]\n",
		"end only":           gitManagedIncludeEnd + "\n",
		"duplicate sections": string(wrapManagedConfigSection(gitManagedIncludeStart, gitManagedIncludeEnd, "[include]\n\tpath = old")) + string(wrapManagedConfigSection(gitManagedIncludeStart, gitManagedIncludeEnd, "[include]\n\tpath = other")),
		"two starts":         gitManagedIncludeStart + "\n" + gitManagedIncludeStart + "\n" + gitManagedIncludeEnd + "\n",
		"reversed":           gitManagedIncludeEnd + "\nuser\n" + gitManagedIncludeStart + "\n",
	}

	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			path := filepath.Join(home, ".gitconfig")
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := WriteGitConfig(GitConfig{DefaultBranch: "main"}, "dracula"); err == nil {
				t.Fatal("WriteGitConfig accepted malformed managed include")
			}
			if got := mustReadOwnershipFile(t, path); string(got) != original {
				t.Fatalf("malformed native Git config changed:\n%s", got)
			}
			assertOwnershipPathAbsent(t, path+".dotfiles.bak")
			assertOwnershipPathAbsent(t, filepath.Join(home, filepath.FromSlash(gitManagedConfigRel)))
		})
	}
}

func TestWriteGitConfigReplacesOnlyExistingManagedInclude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".gitconfig")
	before := "# before\n[user]\n\tname = Keep\n\n"
	after := "\n# after\n[custom]\n\tkey = keep\n"
	original := before + string(wrapManagedConfigSection(gitManagedIncludeStart, gitManagedIncludeEnd, "[include]\n\tpath = old")) + after
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteGitConfig(GitConfig{DefaultBranch: "main"}, "dracula"); err != nil {
		t.Fatalf("WriteGitConfig: %v", err)
	}
	got := string(mustReadOwnershipFile(t, path))
	if !strings.HasPrefix(got, before) || !strings.HasSuffix(got, after) {
		t.Fatalf("bytes outside existing managed include changed:\n%s", got)
	}
	if strings.Contains(got, "path = old") || !strings.Contains(got, "path = "+gitManagedIncludePath) {
		t.Fatalf("existing managed include was not replaced:\n%s", got)
	}
	assertOwnershipPathAbsent(t, path+".dotfiles.bak")
}

func TestWriteGitConfigLeavesUnownedBackupSidecarUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".gitconfig")
	original := []byte("[user]\n\tname = Keep\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".dotfiles.bak", []byte("occupied\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := WriteGitConfig(GitConfig{DefaultBranch: "main"}, "dracula"); err != nil {
		t.Fatalf("WriteGitConfig: %v", err)
	}
	if got := mustReadOwnershipFile(t, path); !bytes.HasPrefix(got, original) || !bytes.Contains(got, []byte(gitManagedIncludeStart)) {
		t.Fatalf("native Git config was not preserved while adopting:\n%s", got)
	}
	if got := mustReadOwnershipFile(t, path+".dotfiles.bak"); string(got) != "occupied\n" {
		t.Fatalf("unowned sidecar changed: %q", got)
	}
}

func TestWriteGitConfigRefusesUnownedManagedIncludeFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	rootPath := filepath.Join(home, ".gitconfig")
	originalRoot := []byte("[user]\n\tname = Keep\n")
	if err := os.WriteFile(rootPath, originalRoot, 0o600); err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(home, filepath.FromSlash(gitManagedConfigRel))
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	originalManaged := []byte("# user-owned file in a colliding path\n[custom]\n\tkeep = true\n")
	if err := os.WriteFile(managedPath, originalManaged, 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteGitConfig(GitConfig{DefaultBranch: "main"}, "dracula")
	if !errors.Is(err, ErrUnmanagedConfig) {
		t.Fatalf("WriteGitConfig error = %v, want ErrUnmanagedConfig", err)
	}
	if got := mustReadOwnershipFile(t, rootPath); !bytes.Equal(got, originalRoot) {
		t.Fatalf("native Git config changed after managed-file ownership refusal:\n%s", got)
	}
	if got := mustReadOwnershipFile(t, managedPath); !bytes.Equal(got, originalManaged) {
		t.Fatalf("colliding user-owned managed path changed:\n%s", got)
	}
	assertOwnershipPathAbsent(t, rootPath+".dotfiles.bak")
}

func TestWriteGitConfigRefusesSymlinkedOwnershipPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink ownership checks are Unix-specific")
	}

	t.Run("native config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		victim := filepath.Join(home, "victim")
		if err := os.WriteFile(victim, []byte("do not change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victim, filepath.Join(home, ".gitconfig")); err != nil {
			t.Fatal(err)
		}
		err := WriteGitConfig(GitConfig{DefaultBranch: "main"}, "dracula")
		if !errors.Is(err, safefile.ErrSymlink) {
			t.Fatalf("WriteGitConfig error = %v, want safefile.ErrSymlink", err)
		}
		if got := mustReadOwnershipFile(t, victim); string(got) != "do not change\n" {
			t.Fatalf("symlink victim changed: %q", got)
		}
	})

	t.Run("managed include target", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		rootPath := filepath.Join(home, ".gitconfig")
		original := []byte("[user]\n\tname = Keep\n")
		if err := os.WriteFile(rootPath, original, 0o600); err != nil {
			t.Fatal(err)
		}
		managedPath := filepath.Join(home, filepath.FromSlash(gitManagedConfigRel))
		if err := os.MkdirAll(filepath.Dir(managedPath), 0o700); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(home, "victim")
		if err := os.WriteFile(victim, []byte("do not change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victim, managedPath); err != nil {
			t.Fatal(err)
		}

		err := WriteGitConfig(GitConfig{DefaultBranch: "main"}, "dracula")
		if !errors.Is(err, safefile.ErrSymlink) {
			t.Fatalf("WriteGitConfig error = %v, want safefile.ErrSymlink", err)
		}
		if got := mustReadOwnershipFile(t, rootPath); !bytes.Equal(got, original) {
			t.Fatalf("native Git config changed after managed symlink refusal:\n%s", got)
		}
		if got := mustReadOwnershipFile(t, victim); string(got) != "do not change\n" {
			t.Fatalf("managed symlink victim changed: %q", got)
		}
		assertOwnershipPathAbsent(t, rootPath+".dotfiles.bak")
	})

	t.Run("unowned backup sidecar", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		rootPath := filepath.Join(home, ".gitconfig")
		original := []byte("[user]\n\tname = Keep\n")
		if err := os.WriteFile(rootPath, original, 0o600); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(home, "victim")
		if err := os.WriteFile(victim, []byte("do not change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victim, rootPath+".dotfiles.bak"); err != nil {
			t.Fatal(err)
		}

		err := WriteGitConfig(GitConfig{DefaultBranch: "main"}, "dracula")
		if err != nil {
			t.Fatalf("WriteGitConfig: %v", err)
		}
		if got := mustReadOwnershipFile(t, rootPath); !bytes.HasPrefix(got, original) || !bytes.Contains(got, []byte(gitManagedIncludeStart)) {
			t.Fatalf("native Git config was not preserved while adopting:\n%s", got)
		}
		if got := mustReadOwnershipFile(t, victim); string(got) != "do not change\n" {
			t.Fatalf("backup symlink victim changed: %q", got)
		}
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(gitManagedConfigRel))); err != nil {
			t.Fatalf("managed Git config was not written: %v", err)
		}
	})
}

func TestGitToolTracksNativeAndManagedConfigPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	paths := NewGitTool().ConfigPaths()
	want := []string{
		filepath.Join(home, ".gitconfig"),
		filepath.Join(home, filepath.FromSlash(gitManagedConfigRel)),
	}
	if len(paths) != len(want) {
		t.Fatalf("ConfigPaths() = %#v, want %#v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("ConfigPaths()[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestGenerateGitConfigRejectsDirectiveInjectionFromPersistedValues(t *testing.T) {
	got := GenerateGitConfig(GitConfig{
		DefaultBranch:    "main\n[include]\npath = /tmp/branch-attacker",
		CredentialHelper: "cache\n[core]",
		MergeTool:        "meld\n[include]",
	}, "dracula\n[include]\npath = /tmp/theme-attacker")
	if !strings.Contains(got, "defaultBranch = main\n") {
		t.Fatalf("invalid branch did not fall back safely:\n%s", got)
	}
	for _, forbidden := range []string{"/tmp/branch-attacker", "helper = cache", "tool = meld"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("generated Git config contains injected value %q:\n%s", forbidden, got)
		}
	}
	if strings.Contains(got, "\n[include]\n") {
		t.Fatalf("generated Git config contains an injected include section:\n%s", got)
	}
}

func mustReadOwnershipFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func assertOwnershipPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to remain absent, got %v", path, err)
	}
}

func assertOwnershipFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want.Perm() {
		t.Fatalf("mode of %s = %04o, want %04o", path, got, want.Perm())
	}
}
