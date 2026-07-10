package tools

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func TestWriteGeneratedConfigReplacesContentAndCorrectsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes are not enforced on Windows")
	}

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("old\n"), 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}
	oldLink := filepath.Join(dir, "old-inode")
	if err := os.Link(path, oldLink); err != nil {
		t.Fatalf("link old config inode: %v", err)
	}

	if err := writeGeneratedConfig(path, []byte("new\n"), 0600); err != nil {
		t.Fatalf("writeGeneratedConfig: %v", err)
	}

	assertFileContent(t, path, "new\n")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat replacement: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("replacement mode = %04o, want 0600", got)
	}
	// A truncate-in-place writer would change oldLink too. Keeping the old
	// inode's content proves the destination name was replaced instead.
	assertFileContent(t, oldLink, "old\n")
	assertNoSafeFileStaging(t, dir)
}

func TestWriteGeneratedConfigRefusesFinalSymlinkWithoutTouchingTarget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "config")
	if err := os.WriteFile(target, []byte("untouched\n"), 0600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := writeGeneratedConfig(link, []byte("replacement\n"), 0600)
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("writeGeneratedConfig error = %v, want safefile.ErrSymlink", err)
	}
	assertFileContent(t, target, "untouched\n")
	info, statErr := os.Lstat(link)
	if statErr != nil {
		t.Fatalf("lstat symlink: %v", statErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("destination symlink was replaced")
	}
	assertNoSafeFileStaging(t, dir)
}

func TestWriteGeneratedConfigFailurePreservesDestinationAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	destination := filepath.Join(dir, "config")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatalf("mkdir destination: %v", err)
	}
	sentinel := filepath.Join(destination, "sentinel")
	if err := os.WriteFile(sentinel, []byte("old remains valid\n"), 0600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	if err := writeGeneratedConfig(destination, []byte("new\n"), 0600); err == nil {
		t.Fatal("writeGeneratedConfig unexpectedly replaced a directory")
	}
	assertFileContent(t, sentinel, "old remains valid\n")
	assertNoSafeFileStaging(t, dir)
}

func TestWriteGeneratedConfigRefusesIntermediateSymlink(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	outside := filepath.Join(workspace, "outside")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	// Even though XDG names the symlink itself, HOME must win as the trusted
	// anchor so .config remains an untrusted descendant and is refused.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := writeGeneratedConfig(filepath.Join(home, ".config", "ghostty", "config"), []byte("unsafe"), 0600)
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("writeGeneratedConfig error = %v, want safefile.ErrSymlink", err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "ghostty", "config")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("intermediate symlink target was modified: %v", statErr)
	}
}

func TestWriteGeneratedConfigSupportsExternalXDGRoot(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	xdg := filepath.Join(workspace, "external-xdg")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	path := filepath.Join(xdg, "glow", "glow.yml")

	if err := writeGeneratedConfig(path, []byte("style: dark\n"), 0600); err != nil {
		t.Fatalf("writeGeneratedConfig external XDG: %v", err)
	}
	assertFileContent(t, path, "style: dark\n")
	assertMode(t, path, 0600)
	assertMode(t, xdg, 0700)
	assertNoSafeFileStaging(t, filepath.Dir(path))
}

func TestWriteGeneratedConfigAllowsTrustedHomeRootSymlink(t *testing.T) {
	workspace := t.TempDir()
	realHome := filepath.Join(workspace, "real-home")
	homeLink := filepath.Join(workspace, "home")
	if err := os.MkdirAll(realHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realHome, homeLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", homeLink)
	t.Setenv("XDG_CONFIG_HOME", "")

	if err := writeGeneratedConfig(filepath.Join(homeLink, ".gitconfig"), []byte("[core]\n"), 0600); err != nil {
		t.Fatalf("writeGeneratedConfig trusted HOME symlink: %v", err)
	}
	assertFileContent(t, filepath.Join(realHome, ".gitconfig"), "[core]\n")
}

func TestWriteGeneratedConfigResolvedHomeStillWinsOverXDGAlias(t *testing.T) {
	workspace := t.TempDir()
	realHome := filepath.Join(workspace, "real-home")
	homeLink := filepath.Join(workspace, "home")
	outside := filepath.Join(workspace, "outside")
	if err := os.MkdirAll(realHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realHome, homeLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// XDG spells the resolved HOME path. It must not become a fresh trusted
	// anchor: .config is still a descendant of HOME and its symlink is refused.
	xdg := filepath.Join(realHome, ".config")
	if err := os.Symlink(outside, xdg); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", homeLink)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	err := writeGeneratedConfig(filepath.Join(xdg, "glow", "glow.yml"), []byte("unsafe"), 0600)
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("writeGeneratedConfig error = %v, want safefile.ErrSymlink", err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "glow", "glow.yml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("resolved-HOME XDG alias modified outside target: %v", statErr)
	}
}

func TestWriteGeneratedConfigRejectsPathOutsideTrustedRoots(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	xdg := filepath.Join(workspace, "xdg")
	outside := filepath.Join(workspace, "outside", "config")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	called := false
	err := writeGeneratedConfigWith(outside, []byte("unsafe"), 0600, func(string, string, []byte, os.FileMode) error {
		called = true
		return nil
	})
	if !errors.Is(err, errGeneratedConfigOutsideTrustedRoots) {
		t.Fatalf("writeGeneratedConfigWith error = %v, want trusted-root refusal", err)
	}
	if called {
		t.Fatal("replacement kernel called for a destination outside trusted roots")
	}
}

func TestWriteGeneratedConfigPropagatesCommittedError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	want := &safefile.CommittedError{Operation: "parent fsync", Err: errors.New("injected durability failure")}

	err := writeGeneratedConfigWith(filepath.Join(home, ".gitconfig"), []byte("new"), 0600, func(root, rel string, _ []byte, _ os.FileMode) error {
		if root != home || rel != ".gitconfig" {
			t.Fatalf("replacement destination = root %q rel %q, want %q/.gitconfig", root, rel, home)
		}
		return want
	})
	var committed *safefile.CommittedError
	if !errors.As(err, &committed) || committed != want || !committed.Committed() {
		t.Fatalf("writeGeneratedConfigWith error = %v, want propagated CommittedError", err)
	}
}

func TestWriteGeneratedConfigRefusesNonRegularTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	target := filepath.Join(home, ".gitconfig")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}

	err := writeGeneratedConfig(target, []byte("unsafe"), 0600)
	if !errors.Is(err, safefile.ErrNonRegular) {
		t.Fatalf("writeGeneratedConfig error = %v, want safefile.ErrNonRegular", err)
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		t.Fatalf("non-regular target changed: info=%v err=%v", info, statErr)
	}
}

func TestRepresentativeToolWritersUseAtomicConfigPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-driven tool config paths are Unix-specific")
	}

	t.Run("ghostty replaces existing config with restrictive mode", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		path := filepath.Join(home, ".config", "ghostty", "config")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatalf("mkdir ghostty config: %v", err)
		}
		if err := os.WriteFile(path, []byte("legacy\n"), 0644); err != nil {
			t.Fatalf("write legacy ghostty config: %v", err)
		}

		cfg := GhosttyConfig{
			FontSize:          13,
			FontFamily:        "JetBrainsMono Nerd Font",
			Opacity:           90,
			TabBindings:       "super",
			ScrollbackLines:   10000,
			CursorStyle:       "block",
			WindowDecorations: true,
			ConfirmClose:      true,
		}
		if err := WriteGhosttyConfig(cfg, "catppuccin-mocha"); err != nil {
			t.Fatalf("WriteGhosttyConfig: %v", err)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read Ghostty config: %v", err)
		}
		if !strings.Contains(string(content), "font-size = 13") {
			t.Fatalf("Ghostty config missing generated content:\n%s", content)
		}
		assertMode(t, path, 0600)
		assertNoSafeFileStaging(t, filepath.Dir(path))
	})

	t.Run("neovim init append replaces atomically", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		nvimDir := filepath.Join(home, ".config", "nvim")
		initPath := filepath.Join(nvimDir, "init.lua")
		if err := os.MkdirAll(nvimDir, 0700); err != nil {
			t.Fatalf("mkdir nvim config: %v", err)
		}
		if err := os.WriteFile(initPath, []byte("-- existing\n"), 0644); err != nil {
			t.Fatalf("write existing init.lua: %v", err)
		}

		cfg := NeovimConfig{TabWidth: 4, LineNumbers: "absolute"}
		if err := writeNeovimUserPrefs(cfg, "catppuccin-mocha", nvimDir); err != nil {
			t.Fatalf("writeNeovimUserPrefs: %v", err)
		}

		content, err := os.ReadFile(initPath)
		if err != nil {
			t.Fatalf("read init.lua: %v", err)
		}
		if !strings.Contains(string(content), `pcall(require, "custom.options")`) {
			t.Fatalf("init.lua missing custom options require:\n%s", content)
		}
		assertMode(t, initPath, 0600)
		assertNoSafeFileStaging(t, nvimDir)

		prefsPath := filepath.Join(nvimDir, "lua", "custom", "options.lua")
		prefs, err := os.ReadFile(prefsPath)
		if err != nil {
			t.Fatalf("read Neovim preferences: %v", err)
		}
		if !strings.Contains(string(prefs), "vim.opt.tabstop = 4") {
			t.Fatalf("Neovim preferences missing generated content:\n%s", prefs)
		}
		assertMode(t, prefsPath, 0600)
		assertNoSafeFileStaging(t, filepath.Dir(prefsPath))
	})

	t.Run("neovim missing init leaves preferences untouched", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		nvimDir := filepath.Join(home, ".config", "nvim")
		prefsPath := filepath.Join(nvimDir, "lua", "custom", "options.lua")
		if err := os.MkdirAll(nvimDir, 0700); err != nil {
			t.Fatalf("mkdir nvim config: %v", err)
		}

		if err := writeNeovimUserPrefs(NeovimConfig{TabWidth: 4}, "catppuccin-mocha", nvimDir); err != nil {
			t.Fatalf("missing init.lua should be a no-op: %v", err)
		}
		if _, err := os.Lstat(prefsPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("preferences were written without a usable init.lua: %v", err)
		}
	})

	t.Run("neovim init read error leaves preferences untouched", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		nvimDir := filepath.Join(home, ".config", "nvim")
		initPath := filepath.Join(nvimDir, "init.lua")
		prefsPath := filepath.Join(nvimDir, "lua", "custom", "options.lua")
		if err := os.MkdirAll(initPath, 0700); err != nil {
			t.Fatalf("mkdir non-regular init.lua: %v", err)
		}

		err := writeNeovimUserPrefs(NeovimConfig{TabWidth: 4}, "catppuccin-mocha", nvimDir)
		if err == nil || !strings.Contains(err.Error(), "failed to read neovim init.lua") {
			t.Fatalf("non-readable init.lua error = %v", err)
		}
		if _, statErr := os.Lstat(prefsPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("preferences were written after init.lua read error: %v", statErr)
		}
	})
}

func TestLockedReadModifyWritersPreserveConcurrentEdits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("descriptor-anchored locks are Unix-specific")
	}

	t.Run("zsh merge waits for lock and preserves edit", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		path := filepath.Join(home, ".zshrc")
		if err := os.WriteFile(path, []byte("# user content\n"), 0600); err != nil {
			t.Fatal(err)
		}

		locked := make(chan struct{})
		releaseManual := make(chan struct{})
		manualDone := make(chan error, 1)
		go func() {
			manualDone <- withToolConfigLock(path, func(root, rel string) error {
				existing, revision, err := readToolConfig(root, rel)
				if err != nil {
					return err
				}
				close(locked)
				<-releaseManual
				return replaceToolConfigAtRevision(root, rel, revision, append(existing, []byte("# concurrent edit\n")...))
			})
		}()
		<-locked

		writerDone := make(chan error, 1)
		go func() {
			writerDone <- WriteZshConfig(ZshConfig{HistorySize: 1000, PromptStyle: "minimal"}, "catppuccin-mocha")
		}()
		assertWriterWaitsForHeldLock(t, writerDone, releaseManual, manualDone)

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "# concurrent edit") || !strings.Contains(string(content), zshManagedStart) {
			t.Fatalf("locked zsh merge lost content:\n%s", content)
		}
	})

	t.Run("neovim append waits for lock and preserves edit", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		nvimDir := filepath.Join(home, ".config", "nvim")
		path := filepath.Join(nvimDir, "init.lua")
		if err := os.MkdirAll(nvimDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("-- user init\n"), 0600); err != nil {
			t.Fatal(err)
		}

		locked := make(chan struct{})
		releaseManual := make(chan struct{})
		manualDone := make(chan error, 1)
		go func() {
			manualDone <- withToolConfigLock(path, func(root, rel string) error {
				existing, revision, err := readToolConfig(root, rel)
				if err != nil {
					return err
				}
				close(locked)
				<-releaseManual
				return replaceToolConfigAtRevision(root, rel, revision, append(existing, []byte("-- concurrent edit\n")...))
			})
		}()
		<-locked

		writerDone := make(chan error, 1)
		go func() {
			writerDone <- writeNeovimUserPrefs(NeovimConfig{TabWidth: 4}, "catppuccin-mocha", nvimDir)
		}()
		assertWriterWaitsForHeldLock(t, writerDone, releaseManual, manualDone)

		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "-- concurrent edit") || !strings.Contains(string(content), `pcall(require, "custom.options")`) {
			t.Fatalf("locked neovim append lost content:\n%s", content)
		}
	})
}

func TestRevisionGuardRejectsObservedNonCooperatingReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("descriptor-anchored revisions are Unix-specific")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".zshrc")
	original := []byte("# original user content\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	root, rel, err := prepareGeneratedConfigDestination(path)
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := readToolConfig(root, rel)
	if err != nil {
		t.Fatal(err)
	}

	replacement := []byte("# non-cooperating edit\n")
	temporary := path + ".replacement"
	if err := os.WriteFile(temporary, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatal(err)
	}

	err = replaceToolConfigAtRevision(root, rel, revision, []byte("# stale dotfiles result\n"))
	if !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("replaceToolConfigAtRevision error = %v, want safefile.ErrRevisionChanged", err)
	}
	assertFileContent(t, path, string(replacement))
}

func assertWriterWaitsForHeldLock(t *testing.T, writerDone <-chan error, releaseManual chan<- struct{}, manualDone <-chan error) {
	t.Helper()
	var premature error
	writerReturned := false
	select {
	case premature = <-writerDone:
		writerReturned = true
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseManual)
	if err := <-manualDone; err != nil {
		t.Fatalf("manual locked edit: %v", err)
	}
	if writerReturned {
		t.Fatalf("writer returned before held config lock was released: %v", premature)
	}
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("writer after lock release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writer remained blocked after config lock release")
	}
}

func TestProductionToolsMutationCallsitesMatchAuditedManifest(t *testing.T) {
	// Generated tool configs owned by this package must funnel through
	// writeToolConfig. Claude settings are deliberately not included here:
	// config.SaveClaudeConfig is owned by internal/config and has its own
	// safefile-backed tests. Install-time Neovim preset and TPM directory changes
	// are also outside the generated-file writer; the mutation manifest below
	// makes those exceptions explicit and reviewable instead of silently ignoring
	// every API other than os.WriteFile.
	expectedWriters := map[string]int{
		"btop.go":    2,
		"fzf.go":     1,
		"ghostty.go": 1,
		"git.go":     1,
		"glow.go":    1,
		"lazygit.go": 1,
		"neovim.go":  2,
		"tmux.go":    1,
		"yazi.go":    3,
	}
	expectedLockedUpdates := map[string]int{
		"neovim.go": 1,
		"zsh.go":    1,
	}
	type mutationExpectation struct {
		count          int
		classification string
	}
	expectedMutations := map[string]mutationExpectation{
		"apps.go:exec.Command":                   {1, "read-only Flatpak installation observation"},
		"claude_code.go:config.SaveClaudeConfig": {1, "config-owned Claude settings writer"},
		"neovim.go:exec.Command":                 {1, "install-time preset clone"},
		"neovim.go:os.MkdirAll":                  {1, "install-time preset parent"},
		"neovim.go:os.MkdirTemp":                 {1, "install-time preset staging"},
		"neovim.go:os.RemoveAll":                 {2, "install-time preset staging cleanup"},
		"neovim.go:os.Rename":                    {3, "install-time preset commit/rollback"},
		"tmux.go:exec.Command":                   {2, "install-time TPM clone/plugin installer"},
		"tmux.go:os.MkdirAll":                    {1, "install-time TPM parent"},
		"tool.go:os.MkdirAll":                    {1, "external XDG trusted-root bootstrap"},
		"tool.go:safefile.AcquireLockWithin":     {1, "shared generated-config advisory lock kernel"},
		"tool.go:safefile.ReadWithin":            {1, "shared generated-config anchored read kernel"},
		"tool.go:safefile.ReplaceWithin":         {2, "shared generated-config replacement kernel"},
	}
	auditedFilesystemSelectors := map[string]bool{
		"config.SaveClaudeConfig":    true,
		"exec.Command":               true,
		"exec.CommandContext":        true,
		"io.Copy":                    true,
		"io.CopyBuffer":              true,
		"io.WriteString":             true,
		"ioutil.WriteFile":           true,
		"os.Chmod":                   true,
		"os.Chown":                   true,
		"os.CopyFS":                  true,
		"os.Create":                  true,
		"os.CreateTemp":              true,
		"os.Link":                    true,
		"os.Mkdir":                   true,
		"os.MkdirAll":                true,
		"os.MkdirTemp":               true,
		"os.OpenFile":                true,
		"os.Remove":                  true,
		"os.RemoveAll":               true,
		"os.Rename":                  true,
		"os.Symlink":                 true,
		"os.Truncate":                true,
		"os.WriteFile":               true,
		"safefile.AcquireLockWithin": true,
		"safefile.ReadWithin":        true,
		"safefile.ReplaceWithin":     true,
		"syscall.Open":               true,
		"syscall.Rename":             true,
		"syscall.Unlink":             true,
		"unix.Open":                  true,
		"unix.Openat":                true,
		"unix.Renameat":              true,
		"unix.Unlinkat":              true,
	}
	expectedProtectedHelperCalls := map[string]int{
		"tool.go:writeGeneratedConfig":          1,
		"tool.go:writeGeneratedConfigWith":      1,
		"neovim.go:replaceToolConfigAtRevision": 1,
		"zsh.go:replaceToolConfigAtRevision":    1,
	}
	protectedHelpers := map[string]bool{
		"writeGeneratedConfig":        true,
		"writeGeneratedConfigWith":    true,
		"replaceToolConfigAtRevision": true,
	}
	actualWriters := make(map[string]int)
	actualLockedUpdates := make(map[string]int)
	actualMutations := make(map[string]int)
	actualProtectedHelperCalls := make(map[string]int)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		base := filepath.Base(file)
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CallExpr:
				if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "writeToolConfig" {
					actualWriters[base]++
				}
				if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "withToolConfigLock" {
					actualLockedUpdates[base]++
				}
				if ident, ok := node.Fun.(*ast.Ident); ok && protectedHelpers[ident.Name] {
					actualProtectedHelperCalls[base+":"+ident.Name]++
				}
			case *ast.SelectorExpr:
				pkgIdent, ok := node.X.(*ast.Ident)
				if !ok {
					return true
				}
				symbol := pkgIdent.Name + "." + node.Sel.Name
				if auditedFilesystemSelectors[symbol] {
					actualMutations[base+":"+symbol]++
				}
			}
			return true
		})
	}
	for file, want := range expectedWriters {
		if got := actualWriters[file]; got != want {
			t.Errorf("%s writeToolConfig callsites = %d, want %d", file, got, want)
		}
		delete(actualWriters, file)
	}
	for file, count := range actualWriters {
		t.Errorf("unexpected generated-config callsite in %s (%d call(s)); add it to the audited manifest", file, count)
	}
	for file, want := range expectedLockedUpdates {
		if got := actualLockedUpdates[file]; got != want {
			t.Errorf("%s withToolConfigLock callsites = %d, want %d", file, got, want)
		}
		delete(actualLockedUpdates, file)
	}
	for file, count := range actualLockedUpdates {
		t.Errorf("unexpected locked generated-config update in %s (%d call(s)); add it to the audited manifest", file, count)
	}
	for key, want := range expectedProtectedHelperCalls {
		if got := actualProtectedHelperCalls[key]; got != want {
			t.Errorf("%s protected helper callsites = %d, want %d", key, got, want)
		}
		delete(actualProtectedHelperCalls, key)
	}
	for key, count := range actualProtectedHelperCalls {
		t.Errorf("unexpected protected generated-writer helper callsite %s (%d occurrence(s)); route whole-file writers through writeToolConfig and RMW writers through withToolConfigLock", key, count)
	}
	for key, want := range expectedMutations {
		if want.classification == "" {
			t.Errorf("audited mutation %s has no scope classification", key)
		}
		if got := actualMutations[key]; got != want.count {
			t.Errorf("%s callsites = %d, want %d (%s)", key, got, want.count, want.classification)
		}
		delete(actualMutations, key)
	}
	for key, count := range actualMutations {
		t.Errorf("unexpected production mutation callsite %s (%d occurrence(s)); route generated files through writeToolConfig or classify the exception", key, count)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("content of %s = %q, want %q", path, got, want)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want.Perm() {
		t.Fatalf("mode of %s = %04o, want %04o", path, got, want.Perm())
	}
}

func assertNoSafeFileStaging(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".safefile-*"))
	if err != nil {
		t.Fatalf("glob staged configs: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("staged configs were not cleaned up: %v", matches)
	}
}
