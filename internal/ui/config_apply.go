package ui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/tools"
)

// applyStandaloneConfigCmd persists the edits from a `dotfiles config <tool>`
// session to the real config files and then quits (C27). It writes ONLY the tool
// whose config screen was opened — NOT every generator — so a single-tool config
// session can never clobber the other tools' config files with compiled-in
// defaults (the data-loss regression FIX 1 closes). The opened tool is derived
// from a.startScreen via the authoritative screen<->tool mapping.
//
// A failed apply is surfaced on stderr (instead of being silently swallowed) so a
// failed `config <tool>` save is observable; the app is quitting so there is no
// screen left to render the error to.
func (a *App) applyStandaloneConfigCmd() tea.Cmd {
	return tea.Sequence(
		func() tea.Msg {
			if errs := a.applyStandaloneConfig(); len(errs) > 0 {
				fmt.Fprintf(os.Stderr, "dotfiles: failed to apply config: %v\n", errs[0])
			}
			return nil
		},
		tea.Quit,
	)
}

// applyStandaloneConfig writes ONLY the config file for the tool whose screen was
// opened standalone (a.startScreen), using the current in-memory deepDiveConfig.
// It returns any generator error(s). Splitting this out from the Cmd keeps it
// directly testable (no tea.Quit) and is the single scoped-write entry point for
// the standalone path.
func (a *App) applyStandaloneConfig() []error {
	toolID, ok := toolIDForScreen(a.startScreen)
	if !ok {
		// Not a per-tool config screen; nothing scoped to write.
		return nil
	}
	return applyOneToolConfig(toolID, *a.deepDiveConfig, a.theme)
}

// config_apply.go is the SINGLE place that turns the TUI's in-memory config into
// real tool config files. The per-tool generators live once in
// toolConfigGenerators; the Manage editor's save (C12) calls applyDeepDiveConfig
// to write EVERY tool from manage.json, while the standalone `dotfiles config
// <tool>` editor (C27) calls applyOneToolConfig to write ONLY the opened tool so
// it can never clobber the others with defaults (FIX 1). Both share
// toolConfigGenerators so the generator-calling logic cannot drift.
//
// The install worker (installation.go) historically inlined the same
// DeepDiveConfig -> tools.*Config translation; the configuration phase there
// pre-dates this helper and is left as-is to avoid disturbing the streaming
// install path (T1), but it mirrors the same mapping.

// toolConfigGenerators maps a tool ID to the function that writes that one tool's
// config file from a DeepDiveConfig. It is the SINGLE source of the generator
// invocations: applyDeepDiveConfig runs every entry (Manage save / install) and
// applyOneToolConfig runs exactly one (standalone `dotfiles config <tool>`), so
// the two paths can never drift. Keys match the tool IDs in toolConfigScreens.
//
// claude-code is intentionally omitted here because its generator only runs when
// MCP servers are configured; applyDeepDiveConfig and applyOneToolConfig handle
// that gated case explicitly.
var toolConfigGenerators = map[string]func(cfg DeepDiveConfig, theme string) error{
	"ghostty": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteGhosttyConfig(tools.GhosttyConfig{
			FontSize:          cfg.GhosttyFontSize,
			FontFamily:        cfg.GhosttyFontFamily,
			Opacity:           cfg.GhosttyOpacity,
			BlurRadius:        cfg.GhosttyBlurRadius,
			TabBindings:       cfg.GhosttyTabBindings,
			ScrollbackLines:   cfg.GhosttyScrollbackLines,
			CursorStyle:       cfg.GhosttyCursorStyle,
			WindowDecorations: cfg.GhosttyWindowDecorations,
			ConfirmClose:      cfg.GhosttyConfirmClose,
		}, theme)
	},
	// tmux generator is a PURE file write (~/.tmux.conf only). TPM installation
	// (git clone + plugin install) is an INSTALL side-effect and lives ONLY in the
	// install worker (installation.go calls tools.SetupTPM directly). Config-apply
	// — Manage save and `dotfiles config tmux` — must never clone or hit the
	// network, so it uses WriteTmuxConfig, not SetupTPM.
	"tmux": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteTmuxConfig(tools.TmuxConfig{
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
		}, theme)
	},
	"zsh": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteZshConfig(tools.ZshConfig{
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
		}, theme)
	},
	// neovim generator is a PURE user-prefs overlay (lua/custom/options.lua in an
	// existing ~/.config/nvim). Cloning a preset repo and the destructive
	// move/remove of ~/.config/nvim are INSTALL side-effects and live ONLY in the
	// install worker (installation.go calls tools.WriteNeovimConfig directly).
	// Config-apply must never clone or clobber the user's nvim config, so it uses
	// WriteNeovimUserPrefs (a no-op when nvim isn't installed yet).
	"neovim": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteNeovimUserPrefs(tools.NeovimConfig{
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
		}, theme)
	},
	"git": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteGitConfig(tools.GitConfig{
			DeltaSideBySide:  cfg.GitDeltaSideBySide,
			DefaultBranch:    cfg.GitDefaultBranch,
			Aliases:          cfg.GitAliases,
			PullRebase:       cfg.GitPullRebase,
			SignCommits:      cfg.GitSignCommits,
			CredentialHelper: cfg.GitCredentialHelper,
			AutoSetupRemote:  cfg.GitAutoSetupRemote,
			MergeTool:        cfg.GitMergeTool,
		}, theme)
	},
	"yazi": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteYaziConfig(tools.YaziConfig{
			Keymap:      cfg.YaziKeymap,
			ShowHidden:  cfg.YaziShowHidden,
			PreviewMode: cfg.YaziPreviewMode,
			SortBy:      cfg.YaziSortBy,
			SortReverse: cfg.YaziSortReverse,
			LineMode:    cfg.YaziLineMode,
			ScrollOff:   cfg.YaziScrollOff,
		}, theme)
	},
	"fzf": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteFzfConfig(tools.FzfConfig{
			Preview:       cfg.FzfPreview,
			Height:        cfg.FzfHeight,
			Layout:        cfg.FzfLayout,
			DefaultOpts:   cfg.FzfDefaultOpts,
			BorderStyle:   cfg.FzfBorderStyle,
			PreviewWindow: cfg.FzfPreviewWindow,
		}, theme)
	},
	"lazygit": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteLazyGitConfig(tools.LazyGitConfig{
			SideBySide: cfg.LazyGitSideBySide,
			MouseMode:  cfg.LazyGitMouseMode,
			Theme:      cfg.LazyGitTheme,
			Paging:     cfg.LazyGitPaging,
		}, theme)
	},
	"btop": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteBtopConfig(tools.BtopConfig{
			Theme:      cfg.BtopTheme,
			UpdateMs:   cfg.BtopUpdateMs,
			ShowTemp:   cfg.BtopShowTemp,
			GraphType:  cfg.BtopGraphType,
			TempScale:  cfg.BtopTempScale,
			ShownBoxes: cfg.BtopShownBoxes,
		}, theme)
	},
	"glow": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteGlowConfig(tools.GlowConfig{
			Pager: cfg.GlowPager,
			Style: cfg.GlowStyle,
			Width: cfg.GlowWidth,
			Mouse: cfg.GlowMouse,
		}, theme)
	},
}

// applyClaudeCodeConfig applies the Claude Code MCP server config, but only when
// at least one server is configured (mirrors the gating both apply paths use).
func applyClaudeCodeConfig(cfg DeepDiveConfig) error {
	if len(cfg.ClaudeCodeMCPs) == 0 {
		return nil
	}
	return tools.NewClaudeCodeTool().ApplyConfigWithMCPs(cfg.ClaudeCodeMCPs)
}

// applyDeepDiveConfig writes every tool config file derived from a DeepDiveConfig
// using the tools.Write*Config generators. It is best-effort: each generator is
// attempted independently and ALL failures are collected and returned, so one
// tool failing does not silently skip the rest (consistent with the T2
// silent-failure work). A nil/empty slice means everything succeeded.
//
// This is the ALL-TOOLS path used by the install worker and the Manage save (the
// latter legitimately re-applies every tool from manage.json). The standalone
// `dotfiles config <tool>` path uses applyOneToolConfig so it cannot clobber the
// other tools (FIX 1).
func applyDeepDiveConfig(cfg DeepDiveConfig, theme string) []error {
	var errs []error
	try := func(name string, fn func() error) {
		if err := fn(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	// Apply in a stable order so collected errors are deterministic.
	for _, name := range []string{
		"ghostty", "tmux", "zsh", "neovim", "git", "yazi", "fzf", "lazygit", "btop", "glow",
	} {
		gen := toolConfigGenerators[name]
		try(name, func() error { return gen(cfg, theme) })
	}

	// Claude Code MCP servers (only when any are configured).
	try("claude-code", func() error { return applyClaudeCodeConfig(cfg) })

	return errs
}

// applyOneToolConfig writes ONLY the named tool's config file from a
// DeepDiveConfig. It is the scoped counterpart of applyDeepDiveConfig used by the
// standalone `dotfiles config <tool>` exit, so editing one tool's settings can
// never overwrite another tool's config file with defaults (FIX 1). An unknown
// toolID (no generator) is a no-op returning nil.
func applyOneToolConfig(toolID string, cfg DeepDiveConfig, theme string) []error {
	if toolID == "claude-code" {
		if err := applyClaudeCodeConfig(cfg); err != nil {
			return []error{fmt.Errorf("claude-code: %w", err)}
		}
		return nil
	}
	gen, ok := toolConfigGenerators[toolID]
	if !ok {
		return nil
	}
	if err := gen(cfg, theme); err != nil {
		return []error{fmt.Errorf("%s: %w", toolID, err)}
	}
	return nil
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
// P1-B closed the "editable but does nothing" gap: every Manage field is now
// EITHER overlaid here and wired through a generator (the round-trip is proven by
// manage_field_apply_test.go's table) OR explicitly listed as not-applied in
// manageNotAppliedFields so the UI never claims success for a no-op.
//
// The ONLY fields intentionally NOT mapped here are LazyDocker's (MouseMode,
// LogsTail): no generator or config-file writer exists for lazydocker at all, so
// they are not-applied (manageNotAppliedFields). GitDiffTool is mapped indirectly —
// it only drives the generator's boolean DeltaSideBySide (delta vs not);
// difftastic/vimdiff are not separately modeled in the .gitconfig template.
//
// CRITICAL (Task 19 coupling): any field overlaid here must ALSO be listed in
// toolDeepDiveFields (manage_save_scope.go) or the scoped Manage save will not
// detect a change to it and will silently under-apply.
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
	dd.GhosttyWindowDecorations = mc.GhosttyWindowDecorations
	dd.GhosttyConfirmClose = mc.GhosttyConfirmClose

	// Tmux (prefix vocabulary reconciled for the generator).
	dd.TmuxPrefix = tmuxPrefixToGenerator(mc.TmuxPrefix)
	dd.TmuxMouseMode = mc.TmuxMouseMode
	dd.TmuxStatusBar = mc.TmuxStatusPosition
	dd.TmuxHistoryLimit = mc.TmuxHistoryLimit
	dd.TmuxEscapeTime = mc.TmuxEscapeTime
	dd.TmuxBaseIndex = mc.TmuxBaseIndex
	dd.TmuxPaneBorderStyle = mc.TmuxPaneBorderStyle
	dd.TmuxAggressiveResize = mc.TmuxAggressiveResize
	dd.TmuxTPMEnabled = mc.TmuxTPMEnabled
	dd.TmuxPluginSensible = mc.TmuxPluginSensible
	dd.TmuxPluginResurrect = mc.TmuxPluginResurrect
	dd.TmuxPluginContinuum = mc.TmuxPluginContinuum
	dd.TmuxPluginYank = mc.TmuxPluginYank
	dd.TmuxContinuumSaveMin = mc.TmuxContinuumSaveMin
	dd.TmuxContinuumRestore = mc.TmuxContinuumRestore

	// Zsh
	dd.ZshHistorySize = mc.ZshHistorySize
	dd.ZshAutoCD = mc.ZshAutoCD
	dd.ZshSyntaxHighlight = mc.ZshSyntaxHighlight
	dd.ZshAutosuggestions = mc.ZshAutosuggestions
	dd.ZshHistoryIgnoreDups = mc.ZshHistoryIgnoreDups
	dd.ZshCorrection = mc.ZshCorrection
	dd.ZshCompletionMenu = mc.ZshCompletionMenu

	// Neovim
	dd.NeovimTabWidth = mc.NeovimTabWidth
	dd.NeovimWrap = mc.NeovimWrap
	dd.NeovimCursorLine = mc.NeovimCursorLine
	dd.NeovimClipboard = mc.NeovimClipboard
	dd.NeovimLineNumbers = mc.NeovimLineNumbers
	dd.NeovimRelativeNum = mc.NeovimRelativeNum
	dd.NeovimExpandTab = mc.NeovimExpandTab
	dd.NeovimUndoFile = mc.NeovimUndoFile

	// Git
	dd.GitDefaultBranch = mc.GitDefaultBranch
	dd.GitPullRebase = mc.GitPullRebase
	dd.GitSignCommits = mc.GitSignCommits
	dd.GitCredentialHelper = mc.GitCredentialHelper
	dd.GitDeltaSideBySide = mc.GitDiffTool == "delta"
	dd.GitAutoSetupRemote = mc.GitAutoSetupRemote
	dd.GitMergeTool = mc.GitMergeTool

	// Yazi
	dd.YaziShowHidden = mc.YaziShowHidden
	dd.YaziSortBy = mc.YaziSortBy
	dd.YaziSortReverse = mc.YaziSortReverse
	dd.YaziLineMode = mc.YaziLineMode
	dd.YaziScrollOff = mc.YaziScrollOff

	// FZF
	dd.FzfHeight = mc.FzfHeight
	dd.FzfLayout = mc.FzfLayout
	dd.FzfPreview = mc.FzfPreview
	dd.FzfDefaultOpts = mc.FzfDefaultOpts
	dd.FzfBorderStyle = mc.FzfBorderStyle
	dd.FzfPreviewWindow = mc.FzfPreviewWindow

	// LazyGit
	dd.LazyGitSideBySide = mc.LazyGitSideBySide
	dd.LazyGitMouseMode = mc.LazyGitMouseMode
	dd.LazyGitTheme = mc.LazyGitGuiTheme
	dd.LazyGitPaging = mc.LazyGitPaging

	// LazyDocker: NO generator/config file exists for lazydocker (see
	// manageNotAppliedFields). Its Manage fields are intentionally not applied; the
	// preference is remembered in manage.json but no config file is written, so
	// nothing is overlaid here.

	// Btop (graph symbol vocabulary matches the generator's GraphType).
	dd.BtopTheme = mc.BtopTheme
	dd.BtopUpdateMs = mc.BtopUpdateMs
	dd.BtopShowTemp = mc.BtopShowTemp
	dd.BtopGraphType = mc.BtopGraphSymbol
	dd.BtopTempScale = mc.BtopTempScale
	dd.BtopShownBoxes = mc.BtopShownBoxes

	// Glow (pager vocabulary reconciled for the generator).
	dd.GlowStyle = mc.GlowStyle
	dd.GlowPager = glowPagerToGenerator(mc.GlowPager)
	dd.GlowWidth = mc.GlowWidth
	dd.GlowMouse = mc.GlowMouse

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
