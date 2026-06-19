package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/tools"
)

// applyStandaloneConfigCmd persists the in-memory deepDiveConfig to the real
// tool config files (via the shared apply path) and then quits. It is the exit
// action for a `dotfiles config <tool>` session, where there is no install step
// to apply the edits (C27). Failures are intentionally swallowed here: this runs
// as the app is tearing down, so there is no screen left to surface an error to;
// the generators themselves are best-effort and the apply path already isolates
// per-tool failures.
func (a *App) applyStandaloneConfigCmd() tea.Cmd {
	cfg := *a.deepDiveConfig
	theme := a.theme
	return tea.Sequence(
		func() tea.Msg {
			_ = applyDeepDiveConfig(cfg, theme)
			return nil
		},
		tea.Quit,
	)
}

// config_apply.go is the SINGLE place that turns the TUI's in-memory config into
// real tool config files. Both the Manage editor's save (C12) and the standalone
// `dotfiles config <tool>` editor (C27) funnel through applyDeepDiveConfig so the
// generator-calling logic lives in exactly one spot and cannot drift.
//
// The install worker (installation.go) historically inlined the same
// DeepDiveConfig -> tools.*Config translation; the configuration phase there
// pre-dates this helper and is left as-is to avoid disturbing the streaming
// install path (T1), but it mirrors the same mapping.

// applyDeepDiveConfig writes every tool config file derived from a DeepDiveConfig
// using the tools.Write*Config generators. It is best-effort: each generator is
// attempted independently and ALL failures are collected and returned, so one
// tool failing does not silently skip the rest (consistent with the T2
// silent-failure work). A nil/empty slice means everything succeeded.
func applyDeepDiveConfig(cfg DeepDiveConfig, theme string) []error {
	var errs []error
	try := func(name string, fn func() error) {
		if err := fn(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	try("ghostty", func() error {
		return tools.WriteGhosttyConfig(tools.GhosttyConfig{
			FontSize:        cfg.GhosttyFontSize,
			FontFamily:      cfg.GhosttyFontFamily,
			Opacity:         cfg.GhosttyOpacity,
			BlurRadius:      cfg.GhosttyBlurRadius,
			TabBindings:     cfg.GhosttyTabBindings,
			ScrollbackLines: cfg.GhosttyScrollbackLines,
			CursorStyle:     cfg.GhosttyCursorStyle,
		}, theme)
	})

	try("tmux", func() error {
		return tools.SetupTPM(tools.TmuxConfig{
			Prefix:           cfg.TmuxPrefix,
			SplitBinds:       cfg.TmuxSplitBinds,
			StatusBar:        cfg.TmuxStatusBar,
			MouseMode:        cfg.TmuxMouseMode,
			TPMEnabled:       cfg.TmuxTPMEnabled,
			PluginSensible:   cfg.TmuxPluginSensible,
			PluginResurrect:  cfg.TmuxPluginResurrect,
			PluginContinuum:  cfg.TmuxPluginContinuum,
			PluginYank:       cfg.TmuxPluginYank,
			ContinuumSaveMin: cfg.TmuxContinuumSaveMin,
		}, theme)
	})

	try("zsh", func() error {
		return tools.WriteZshConfig(tools.ZshConfig{
			PromptStyle:     cfg.ZshPromptStyle,
			Plugins:         cfg.ZshPlugins,
			Aliases:         cfg.ZshAliases,
			HistorySize:     cfg.ZshHistorySize,
			AutoCD:          cfg.ZshAutoCD,
			SyntaxHighlight: cfg.ZshSyntaxHighlight,
			Autosuggestions: cfg.ZshAutosuggestions,
		}, theme)
	})

	try("neovim", func() error {
		return tools.WriteNeovimConfig(tools.NeovimConfig{
			ConfigPreset: cfg.NeovimConfig,
			LSPs:         cfg.NeovimLSPs,
			Plugins:      cfg.NeovimPlugins,
			TabWidth:     cfg.NeovimTabWidth,
			Wrap:         cfg.NeovimWrap,
			CursorLine:   cfg.NeovimCursorLine,
			Clipboard:    cfg.NeovimClipboard,
		}, theme)
	})

	try("git", func() error {
		return tools.WriteGitConfig(tools.GitConfig{
			DeltaSideBySide:  cfg.GitDeltaSideBySide,
			DefaultBranch:    cfg.GitDefaultBranch,
			Aliases:          cfg.GitAliases,
			PullRebase:       cfg.GitPullRebase,
			SignCommits:      cfg.GitSignCommits,
			CredentialHelper: cfg.GitCredentialHelper,
		}, theme)
	})

	try("yazi", func() error {
		return tools.WriteYaziConfig(tools.YaziConfig{
			Keymap:      cfg.YaziKeymap,
			ShowHidden:  cfg.YaziShowHidden,
			PreviewMode: cfg.YaziPreviewMode,
		}, theme)
	})

	try("fzf", func() error {
		return tools.WriteFzfConfig(tools.FzfConfig{
			Preview: cfg.FzfPreview,
			Height:  cfg.FzfHeight,
			Layout:  cfg.FzfLayout,
		}, theme)
	})

	try("lazygit", func() error {
		return tools.WriteLazyGitConfig(tools.LazyGitConfig{
			SideBySide: cfg.LazyGitSideBySide,
			MouseMode:  cfg.LazyGitMouseMode,
			Theme:      cfg.LazyGitTheme,
		}, theme)
	})

	try("btop", func() error {
		return tools.WriteBtopConfig(tools.BtopConfig{
			Theme:     cfg.BtopTheme,
			UpdateMs:  cfg.BtopUpdateMs,
			ShowTemp:  cfg.BtopShowTemp,
			GraphType: cfg.BtopGraphType,
		}, theme)
	})

	try("glow", func() error {
		return tools.WriteGlowConfig(tools.GlowConfig{
			Pager: cfg.GlowPager,
			Style: cfg.GlowStyle,
			Width: cfg.GlowWidth,
		}, theme)
	})

	// Claude Code MCP servers (only when any are configured).
	if len(cfg.ClaudeCodeMCPs) > 0 {
		try("claude-code", func() error {
			return tools.NewClaudeCodeTool().ApplyConfigWithMCPs(cfg.ClaudeCodeMCPs)
		})
	}

	return errs
}

// tmuxPrefixToGenerator converts the Manage UI's prefix vocabulary ("C-a",
// "C-b", "C-Space") into the value tmux.go's prefixToTmuxFormat understands
// ("ctrl-a", "ctrl-b", "ctrl-space"). Without this the generator silently falls
// through to its "C-a" default for anything other than the ctrl-* spellings
// (C26). Unknown values are passed through unchanged so a deep-dive value that
// is already in ctrl-* form is left intact.
func tmuxPrefixToGenerator(prefix string) string {
	switch prefix {
	case "C-a":
		return "ctrl-a"
	case "C-b":
		return "ctrl-b"
	case "C-Space":
		return "ctrl-space"
	default:
		return prefix
	}
}

// glowPagerToGenerator maps the Manage UI's pager vocabulary onto the values
// GenerateGlowConfig understands ("auto"/"less" -> pager on, "never" -> off).
// The Manage options include "more" and "none" which the generator does not
// recognize; map them to the closest supported value so the written file is
// never wrong (C26).
func glowPagerToGenerator(pager string) string {
	switch pager {
	case "none":
		return "never"
	case "more":
		return "less"
	default:
		return pager
	}
}

// manageConfigToDeepDive translates the persisted ManageConfig into a
// DeepDiveConfig suitable for applyDeepDiveConfig. It starts from
// NewDeepDiveConfig() so fields the Manage UI does not expose keep their sane
// defaults (e.g. install-flag maps, zsh plugins, neovim LSPs), then overlays
// every ManageConfig field that has a generator equivalent.
//
// Fields present in ManageConfig but with NO generator equivalent are
// intentionally NOT mapped here (documented inline); persisting them in
// manage.json is harmless, but they do not affect generated files:
//   - Ghostty: WindowDecorations, ConfirmClose
//   - Tmux: BaseIndex, StatusPosition, PaneBorderStyle, HistoryLimit, EscapeTime,
//     AggressiveResize, ContinuumRestore
//   - Zsh: HistoryIgnoreDups, Correction, CompletionMenu
//   - Neovim: LineNumbers, RelativeNum, ExpandTab, UndoFile
//   - Git: AutoSetupRemote, MergeTool (DiffTool only influences the generator's
//     boolean DeltaSideBySide, set below; difftastic/vimdiff are not modeled)
//   - Yazi: SortBy, SortReverse, LineMode, ScrollOff (no generator fields)
//   - FZF: DefaultOpts, BorderStyle, PreviewWindow
//   - LazyGit: Paging; LazyDocker: all (no generator at all)
//   - Btop: TempScale, ShownBoxes
//   - Glow: Mouse
func manageConfigToDeepDive(mc *ManageConfig) DeepDiveConfig {
	dd := *NewDeepDiveConfig()
	if mc == nil {
		return dd
	}

	// Ghostty
	dd.GhosttyFontFamily = mc.GhosttyFontFamily
	dd.GhosttyFontSize = mc.GhosttyFontSize
	dd.GhosttyOpacity = mc.GhosttyOpacity
	dd.GhosttyBlurRadius = mc.GhosttyBlurRadius
	dd.GhosttyCursorStyle = mc.GhosstyCursorStyle
	dd.GhosttyScrollbackLines = mc.GhosttyScrollbackLines

	// Tmux (prefix vocabulary reconciled for the generator).
	dd.TmuxPrefix = tmuxPrefixToGenerator(mc.TmuxPrefix)
	dd.TmuxMouseMode = mc.TmuxMouseMode
	dd.TmuxStatusBar = mc.TmuxStatusPosition
	dd.TmuxHistoryLimit = mc.TmuxHistoryLimit
	dd.TmuxEscapeTime = mc.TmuxEscapeTime
	dd.TmuxBaseIndex = mc.TmuxBaseIndex
	dd.TmuxTPMEnabled = mc.TmuxTPMEnabled
	dd.TmuxPluginSensible = mc.TmuxPluginSensible
	dd.TmuxPluginResurrect = mc.TmuxPluginResurrect
	dd.TmuxPluginContinuum = mc.TmuxPluginContinuum
	dd.TmuxPluginYank = mc.TmuxPluginYank
	dd.TmuxContinuumSaveMin = mc.TmuxContinuumSaveMin

	// Zsh
	dd.ZshHistorySize = mc.ZshHistorySize
	dd.ZshAutoCD = mc.ZshAutoCD
	dd.ZshSyntaxHighlight = mc.ZshSyntaxHighlight
	dd.ZshAutosuggestions = mc.ZshAutosuggestions

	// Neovim
	dd.NeovimTabWidth = mc.NeovimTabWidth
	dd.NeovimWrap = mc.NeovimWrap
	dd.NeovimCursorLine = mc.NeovimCursorLine
	dd.NeovimClipboard = mc.NeovimClipboard

	// Git
	dd.GitDefaultBranch = mc.GitDefaultBranch
	dd.GitPullRebase = mc.GitPullRebase
	dd.GitSignCommits = mc.GitSignCommits
	dd.GitCredentialHelper = mc.GitCredentialHelper
	dd.GitDeltaSideBySide = mc.GitDiffTool == "delta"

	// Yazi (only ShowHidden has a generator equivalent here).
	dd.YaziShowHidden = mc.YaziShowHidden

	// FZF
	dd.FzfHeight = mc.FzfHeight
	dd.FzfLayout = mc.FzfLayout
	dd.FzfPreview = mc.FzfPreview

	// LazyGit
	dd.LazyGitSideBySide = mc.LazyGitSideBySide
	dd.LazyGitMouseMode = mc.LazyGitMouseMode
	dd.LazyGitTheme = mc.LazyGitGuiTheme

	// LazyDocker (DeepDiveConfig field exists; no generator consumes it yet).
	dd.LazyDockerMouseMode = mc.LazyDockerMouseMode

	// Btop (graph symbol vocabulary matches the generator's GraphType).
	dd.BtopTheme = mc.BtopTheme
	dd.BtopUpdateMs = mc.BtopUpdateMs
	dd.BtopShowTemp = mc.BtopShowTemp
	dd.BtopGraphType = mc.BtopGraphSymbol

	// Glow (pager vocabulary reconciled for the generator).
	dd.GlowStyle = mc.GlowStyle
	dd.GlowPager = glowPagerToGenerator(mc.GlowPager)
	dd.GlowWidth = mc.GlowWidth

	// Claude Code MCP servers: translate the flat bools into the map the
	// generator consumes (keys must match config.AllMCPServers()).
	dd.ClaudeCodeMCPs = map[string]bool{
		"context7":            mc.ClaudeCodeMCPContext7,
		"task-master":         mc.ClaudeCodeMCPTaskMaster,
		"github":              mc.ClaudeCodeMCPGitHub,
		"supabase":            mc.ClaudeCodeMCPSupabase,
		"convex":              mc.ClaudeCodeMCPConvex,
		"puppeteer":           mc.ClaudeCodeMCPPuppeteer,
		"sequential-thinking": mc.ClaudeCodeMCPSequentialThinking,
	}

	return dd
}
