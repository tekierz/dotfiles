package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/tools"
)

// sampleDeepDiveConfig returns a DeepDiveConfig with every generator-consumed
// field set to a non-default value, so a builder that drops a field would be
// caught by a struct/file comparison.
func sampleDeepDiveConfig() DeepDiveConfig {
	dd := *NewDeepDiveConfig()

	dd.GhosttyFontSize = 17
	dd.GhosttyFontFamily = "Fira Code"
	dd.GhosttyOpacity = 85
	dd.GhosttyBlurRadius = 30
	dd.GhosttyTabBindings = "vim"
	dd.GhosttyScrollbackLines = 50000
	dd.GhosttyCursorStyle = "bar"
	dd.GhosttyWindowDecorations = false
	dd.GhosttyConfirmClose = false

	dd.TmuxPrefix = "ctrl-b"
	dd.TmuxSplitBinds = "vim"
	dd.TmuxStatusBar = "top"
	dd.TmuxMouseMode = false
	dd.TmuxBaseIndex = 0
	dd.TmuxPaneBorderStyle = "double"
	dd.TmuxHistoryLimit = 99999
	dd.TmuxEscapeTime = 50
	dd.TmuxAggressiveResize = true
	dd.TmuxTPMEnabled = false
	dd.TmuxPluginSensible = false
	dd.TmuxPluginResurrect = true
	dd.TmuxPluginContinuum = true
	dd.TmuxPluginYank = false
	dd.TmuxContinuumSaveMin = 30
	dd.TmuxContinuumRestore = false

	dd.ZshPromptStyle = "minimal"
	dd.ZshPlugins = []string{"git", "z"}
	dd.ZshAliases = map[string]bool{"ll": true}
	dd.ZshHistorySize = 99999
	dd.ZshAutoCD = false
	dd.ZshSyntaxHighlight = false
	dd.ZshAutosuggestions = false
	dd.ZshHistoryIgnoreDups = false
	dd.ZshCorrection = true
	dd.ZshCompletionMenu = false

	dd.NeovimConfig = "kickstart"
	dd.NeovimLSPs = []string{"gopls"}
	dd.NeovimPlugins = []string{"telescope"}
	dd.NeovimTabWidth = 8
	dd.NeovimWrap = true
	dd.NeovimCursorLine = false
	dd.NeovimClipboard = "unnamed"
	dd.NeovimLineNumbers = "relative"
	dd.NeovimRelativeNum = true
	dd.NeovimExpandTab = false
	dd.NeovimUndoFile = false

	dd.GitDeltaSideBySide = false
	dd.GitDefaultBranch = "develop"
	dd.GitAliases = []string{"co", "st"}
	dd.GitPullRebase = false
	dd.GitSignCommits = true
	dd.GitCredentialHelper = "store"
	dd.GitAutoSetupRemote = false
	dd.GitMergeTool = "vimdiff"
	dd.GitDiffTool = "difftastic"

	dd.YaziKeymap = "vim"
	dd.YaziShowHidden = true
	dd.YaziPreviewMode = "full"
	dd.YaziSortBy = "size"
	dd.YaziSortReverse = true
	dd.YaziLineMode = "size"
	dd.YaziScrollOff = 8

	dd.FzfPreview = false
	dd.FzfHeight = 80
	dd.FzfLayout = "default"
	dd.FzfDefaultOpts = "--cycle"
	dd.FzfBorderStyle = "rounded"
	dd.FzfPreviewWindow = "up:50%"

	dd.LazyGitSideBySide = false
	dd.LazyGitMouseMode = false
	dd.LazyGitTheme = "dark"
	dd.LazyGitPaging = "less"

	dd.BtopTheme = "tokyo-night"
	dd.BtopUpdateMs = 500
	dd.BtopShowTemp = false
	dd.BtopGraphType = "block"
	dd.BtopTempScale = "fahrenheit"
	dd.BtopShownBoxes = "cpu mem"

	dd.GlowPager = "never"
	dd.GlowStyle = "dark"
	dd.GlowWidth = 100
	dd.GlowMouse = true

	return dd
}

// TestConfigBuildersMatchInstallStructs locks the shared builders to the exact
// struct each tool's generator and the install worker pass to their writers.
// The builders are the SINGLE source of the DeepDiveConfig -> tools.*Config
// translation; this asserts each builder reproduces the full field set.
func TestConfigBuildersMatchInstallStructs(t *testing.T) {
	cfg := sampleDeepDiveConfig()

	if got, want := ghosttyConfigFrom(cfg), (tools.GhosttyConfig{
		FontSize:          cfg.GhosttyFontSize,
		FontFamily:        cfg.GhosttyFontFamily,
		Opacity:           cfg.GhosttyOpacity,
		BlurRadius:        cfg.GhosttyBlurRadius,
		TabBindings:       cfg.GhosttyTabBindings,
		ScrollbackLines:   cfg.GhosttyScrollbackLines,
		CursorStyle:       cfg.GhosttyCursorStyle,
		WindowDecorations: cfg.GhosttyWindowDecorations,
		ConfirmClose:      cfg.GhosttyConfirmClose,
	}); got != want {
		t.Errorf("ghosttyConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := tmuxConfigFrom(cfg), (tools.TmuxConfig{
		Prefix:           cfg.TmuxPrefix,
		SplitBinds:       cfg.TmuxSplitBinds,
		StatusBar:        cfg.TmuxStatusBar,
		MouseMode:        cfg.TmuxMouseMode,
		BaseIndex:        cfg.TmuxBaseIndex,
		PaneBorderStyle:  cfg.TmuxPaneBorderStyle,
		HistoryLimit:     cfg.TmuxHistoryLimit,
		EscapeTime:       cfg.TmuxEscapeTime,
		AggressiveResize: cfg.TmuxAggressiveResize,
		TPMEnabled:       cfg.TmuxTPMEnabled,
		PluginSensible:   cfg.TmuxPluginSensible,
		PluginResurrect:  cfg.TmuxPluginResurrect,
		PluginContinuum:  cfg.TmuxPluginContinuum,
		PluginYank:       cfg.TmuxPluginYank,
		ContinuumSaveMin: cfg.TmuxContinuumSaveMin,
		ContinuumRestore: cfg.TmuxContinuumRestore,
	}); got != want {
		t.Errorf("tmuxConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := neovimConfigFrom(cfg), (tools.NeovimConfig{
		ConfigPreset: cfg.NeovimConfig,
		LSPs:         cfg.NeovimLSPs,
		Plugins:      cfg.NeovimPlugins,
		TabWidth:     cfg.NeovimTabWidth,
		Wrap:         cfg.NeovimWrap,
		CursorLine:   cfg.NeovimCursorLine,
		Clipboard:    cfg.NeovimClipboard,
		LineNumbers:  cfg.NeovimLineNumbers,
		RelativeNum:  cfg.NeovimRelativeNum,
		ExpandTab:    cfg.NeovimExpandTab,
		UndoFile:     cfg.NeovimUndoFile,
	}); got.ConfigPreset != want.ConfigPreset || got.TabWidth != want.TabWidth ||
		got.Wrap != want.Wrap || got.CursorLine != want.CursorLine ||
		got.Clipboard != want.Clipboard || got.LineNumbers != want.LineNumbers ||
		got.RelativeNum != want.RelativeNum || got.ExpandTab != want.ExpandTab ||
		got.UndoFile != want.UndoFile || len(got.LSPs) != len(want.LSPs) ||
		len(got.Plugins) != len(want.Plugins) {
		t.Errorf("neovimConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := gitConfigFrom(cfg), (tools.GitConfig{
		DeltaSideBySide:  cfg.GitDeltaSideBySide,
		DefaultBranch:    cfg.GitDefaultBranch,
		Aliases:          cfg.GitAliases,
		PullRebase:       cfg.GitPullRebase,
		SignCommits:      cfg.GitSignCommits,
		CredentialHelper: cfg.GitCredentialHelper,
		AutoSetupRemote:  cfg.GitAutoSetupRemote,
		MergeTool:        cfg.GitMergeTool,
		DiffTool:         cfg.GitDiffTool,
	}); got.DeltaSideBySide != want.DeltaSideBySide || got.DefaultBranch != want.DefaultBranch ||
		got.PullRebase != want.PullRebase || got.SignCommits != want.SignCommits ||
		got.CredentialHelper != want.CredentialHelper || got.AutoSetupRemote != want.AutoSetupRemote ||
		got.MergeTool != want.MergeTool || got.DiffTool != want.DiffTool ||
		len(got.Aliases) != len(want.Aliases) {
		t.Errorf("gitConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := zshConfigFrom(cfg), (tools.ZshConfig{
		PromptStyle:       cfg.ZshPromptStyle,
		Plugins:           cfg.ZshPlugins,
		Aliases:           cfg.ZshAliases,
		HistorySize:       cfg.ZshHistorySize,
		AutoCD:            cfg.ZshAutoCD,
		SyntaxHighlight:   cfg.ZshSyntaxHighlight,
		Autosuggestions:   cfg.ZshAutosuggestions,
		HistoryIgnoreDups: cfg.ZshHistoryIgnoreDups,
		Correction:        cfg.ZshCorrection,
		CompletionMenu:    cfg.ZshCompletionMenu,
	}); got.PromptStyle != want.PromptStyle || got.HistorySize != want.HistorySize ||
		got.AutoCD != want.AutoCD || got.SyntaxHighlight != want.SyntaxHighlight ||
		got.Autosuggestions != want.Autosuggestions || got.HistoryIgnoreDups != want.HistoryIgnoreDups ||
		got.Correction != want.Correction || got.CompletionMenu != want.CompletionMenu ||
		len(got.Plugins) != len(want.Plugins) || len(got.Aliases) != len(want.Aliases) {
		t.Errorf("zshConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := yaziConfigFrom(cfg), (tools.YaziConfig{
		Keymap:      cfg.YaziKeymap,
		ShowHidden:  cfg.YaziShowHidden,
		PreviewMode: cfg.YaziPreviewMode,
		SortBy:      cfg.YaziSortBy,
		SortReverse: cfg.YaziSortReverse,
		LineMode:    cfg.YaziLineMode,
		ScrollOff:   cfg.YaziScrollOff,
	}); got != want {
		t.Errorf("yaziConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := fzfConfigFrom(cfg), (tools.FzfConfig{
		Preview:       cfg.FzfPreview,
		Height:        cfg.FzfHeight,
		Layout:        cfg.FzfLayout,
		DefaultOpts:   cfg.FzfDefaultOpts,
		BorderStyle:   cfg.FzfBorderStyle,
		PreviewWindow: cfg.FzfPreviewWindow,
	}); got != want {
		t.Errorf("fzfConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := lazygitConfigFrom(cfg), (tools.LazyGitConfig{
		SideBySide: cfg.LazyGitSideBySide,
		MouseMode:  cfg.LazyGitMouseMode,
		Theme:      cfg.LazyGitTheme,
		Paging:     cfg.LazyGitPaging,
	}); got != want {
		t.Errorf("lazygitConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := btopConfigFrom(cfg), (tools.BtopConfig{
		Theme:      cfg.BtopTheme,
		UpdateMs:   cfg.BtopUpdateMs,
		ShowTemp:   cfg.BtopShowTemp,
		GraphType:  cfg.BtopGraphType,
		TempScale:  cfg.BtopTempScale,
		ShownBoxes: cfg.BtopShownBoxes,
	}); got != want {
		t.Errorf("btopConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}

	if got, want := glowConfigFrom(cfg), (tools.GlowConfig{
		Pager: cfg.GlowPager,
		Style: cfg.GlowStyle,
		Width: cfg.GlowWidth,
		Mouse: cfg.GlowMouse,
	}); got != want {
		t.Errorf("glowConfigFrom mismatch:\n got %+v\nwant %+v", got, want)
	}
}

// TestInstallAndConfigApplyProduceSameFiles is the single-source / anti-drift
// guarantee: for the pure tools, the config file written by the config-apply
// path (applyOneToolConfig — Manage save / standalone) is byte-identical to the
// file written by the install translation (WriteXConfig(xConfigFrom(cfg))). Both
// go through the same shared builder, so this stays GREEN; if the two paths ever
// build different structs it goes RED.
func TestInstallAndConfigApplyProduceSameFiles(t *testing.T) {
	cfg := sampleDeepDiveConfig()
	const theme = "catppuccin-mocha"

	// Each case writes via the install-translation writer and via the
	// config-apply generator, into separate temp HOMEs, then compares the file.
	cases := []struct {
		id      string
		relPath string
		write   func(theme string) error // install-side translation
	}{
		{"ghostty", ".config/ghostty/config", func(th string) error {
			return tools.WriteGhosttyConfig(ghosttyConfigFrom(cfg), th)
		}},
		{"tmux", ".tmux.conf", func(th string) error {
			return tools.WriteTmuxConfig(tmuxConfigFrom(cfg), th)
		}},
		{"zsh", ".zshrc", func(th string) error {
			return tools.WriteZshConfig(zshConfigFrom(cfg), th)
		}},
		{"git", ".gitconfig", func(th string) error {
			return tools.WriteGitConfig(gitConfigFrom(cfg), th)
		}},
		{"yazi", ".config/yazi/yazi.toml", func(th string) error {
			return tools.WriteYaziConfig(yaziConfigFrom(cfg), th)
		}},
		{"fzf", ".config/fzf/fzf.zsh", func(th string) error {
			return tools.WriteFzfConfig(fzfConfigFrom(cfg), th)
		}},
		{"lazygit", ".config/lazygit/config.yml", func(th string) error {
			return tools.WriteLazyGitConfig(lazygitConfigFrom(cfg), th)
		}},
		{"btop", ".config/btop/btop.conf", func(th string) error {
			return tools.WriteBtopConfig(btopConfigFrom(cfg), th)
		}},
		{"glow", ".config/glow/glow.yml", func(th string) error {
			return tools.WriteGlowConfig(glowConfigFrom(cfg), th)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			installHome := writeInIsolatedHome(t, func() error { return tc.write(theme) })
			applyHome := writeInIsolatedHome(t, func() error {
				if errs := applyOneToolConfig(tc.id, cfg, theme); len(errs) > 0 {
					return errs[0]
				}
				return nil
			})

			installFile := readFileGlob(t, installHome, tc.relPath)
			applyFile := readFileGlob(t, applyHome, tc.relPath)
			if installFile != applyFile {
				t.Errorf("%s: install path and config-apply path produced different files:\n--- install ---\n%s\n--- apply ---\n%s",
					tc.id, installFile, applyFile)
			}
		})
	}
}

// writeInIsolatedHome points HOME at a fresh temp dir, runs fn, then restores the
// previous HOME, returning the temp dir. Each call is independent so two writes
// can be compared without interference.
func writeInIsolatedHome(t *testing.T, fn func() error) string {
	t.Helper()
	dir := t.TempDir()
	origHome, hadHome := os.LookupEnv("HOME")
	origXDG, hadXDG := os.LookupEnv("XDG_CONFIG_HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	_ = os.Unsetenv("XDG_CONFIG_HOME")
	defer func() {
		if hadHome {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
		if hadXDG {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()
	if err := fn(); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

// readFileGlob reads the file at home/relPath, failing the test if it is absent.
func readFileGlob(t *testing.T, home, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(data)
}
