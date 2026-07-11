package ui

import (
	"fmt"
	"maps"
	"os"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/config"
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
		a.applyStandaloneConfigWorker(),
		tea.Quit,
	)
}

// applyStandaloneConfigWorker builds the tea.Cmd that performs the standalone
// scoped write. The config data is DEEP-SNAPSHOTTED here — on the UI goroutine,
// before the Cmd is returned — so the closure the tea runtime later runs on a
// worker goroutine reads only fully-owned data and never touches a.deepDiveConfig.
//
// This is the crash fix: the screen that dispatched this Cmd (e.g. `dotfiles
// config claude-code`) stays active until tea.Quit lands and keeps toggling
// a.deepDiveConfig's maps (ClaudeCodeMCPs, CLITools) on the UI goroutine. Handing
// the worker a shallow `*a.deepDiveConfig` — whose map/slice fields alias those
// live maps — makes the worker read a map the UI goroutine is writing: a fatal
// concurrent map read/write. snapshotDeepDiveConfig clones every reference field
// so there is no shared mutable state. Mirrors saveManageConfigCmd's pre-return
// snapshot in manage_dualpane.go.
func (a *App) applyStandaloneConfigWorker() tea.Cmd {
	startScreen := a.startScreen
	snapshot := snapshotDeepDiveConfig(a.deepDiveConfig)
	theme := a.theme
	return func() tea.Msg {
		if errs := applyStandaloneSnapshot(startScreen, snapshot, theme); len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "dotfiles: failed to apply config: %v\n", errs[0])
		}
		return nil
	}
}

// applyStandaloneConfig writes ONLY the config file for the tool whose screen was
// opened standalone (a.startScreen), using a deep snapshot of the current
// in-memory deepDiveConfig. It returns any generator error(s). Splitting this out
// from the Cmd keeps it directly testable (no tea.Quit).
func (a *App) applyStandaloneConfig() []error {
	return applyStandaloneSnapshot(a.startScreen, snapshotDeepDiveConfig(a.deepDiveConfig), a.theme)
}

// applyStandaloneSnapshot writes ONLY the opened tool's config file from an
// already-owned DeepDiveConfig snapshot. It is pure (no App or goroutine-shared
// state), so it is safe to call on any goroutine and both standalone paths — the
// synchronous applyStandaloneConfig and the async applyStandaloneConfigWorker —
// funnel through it so they cannot drift. An unknown/non-config startScreen is a
// no-op returning nil.
func applyStandaloneSnapshot(startScreen Screen, cfg DeepDiveConfig, theme string) []error {
	toolID, ok := toolIDForScreen(startScreen)
	if !ok {
		// Not a per-tool config screen; nothing scoped to write.
		return nil
	}
	// Every mutating entry point must first prove that global.json is readable by
	// this binary. Proceeding with a malformed or future-schema global config can
	// create a partially updated environment whose settings no longer agree.
	if _, err := config.LoadGlobalConfig(); err != nil {
		return []error{fmt.Errorf("failed to validate global config before applying %s: %w", toolID, err)}
	}
	return applyOneToolConfig(toolID, cfg, theme)
}

// snapshotDeepDiveConfig returns a value copy of *cfg whose every map and slice
// field is freshly allocated (a deep copy). A plain `*cfg` is only a SHALLOW copy:
// its reference-type fields (ClaudeCodeMCPs, CLITools, ZshAliases, …) keep ALIASING
// the live maps that the still-open `dotfiles config <tool>` screen mutates on the
// UI goroutine. Passing that shallow copy to a worker goroutine makes the worker
// read a map the UI goroutine is concurrently writing — a fatal concurrent map
// read/write. Deep-copying here, on the UI goroutine before the worker starts,
// hands the worker fully-owned data.
//
// This is the DeepDiveConfig analogue of the value snapshot saveManageConfigCmd
// takes: ManageConfig is flat so a shallow copy suffices there, whereas
// DeepDiveConfig owns reference types and needs the per-field clone below. maps/
// slices.Clone preserve nil, so gating checks like applyClaudeCodeConfig's
// len(ClaudeCodeMCPs) == 0 behave identically on the snapshot.
func snapshotDeepDiveConfig(cfg *DeepDiveConfig) DeepDiveConfig {
	snap := *cfg
	snap.ZshPlugins = slices.Clone(cfg.ZshPlugins)
	snap.NeovimLSPs = slices.Clone(cfg.NeovimLSPs)
	snap.NeovimPlugins = slices.Clone(cfg.NeovimPlugins)
	snap.GitAliases = slices.Clone(cfg.GitAliases)
	snap.ZshAliases = maps.Clone(cfg.ZshAliases)
	snap.MacApps = maps.Clone(cfg.MacApps)
	snap.Utilities = maps.Clone(cfg.Utilities)
	snap.CLITools = maps.Clone(cfg.CLITools)
	snap.GUIApps = maps.Clone(cfg.GUIApps)
	snap.CLIUtilities = maps.Clone(cfg.CLIUtilities)
	snap.ClaudeCodeMCPs = maps.Clone(cfg.ClaudeCodeMCPs)
	return snap
}

// config_apply.go is the SINGLE place that turns the TUI's in-memory config into
// real tool config files. The DeepDiveConfig -> tools.*Config translation lives
// once per tool in the *ConfigFrom builders below; every path that needs a tool's
// config struct (the config-apply generators here AND the install worker in
// installation.go) calls the same builder, so the mapping cannot drift.
//
// The per-tool generators live once in toolConfigGenerators; the standalone
// `dotfiles config <tool>` editor (C27) and the Manage editor's save (C12) both
// call applyOneToolConfig — Manage via applyChangedManageTools over the changed
// tools — to write ONLY the relevant tool(s) so a save can never clobber the
// others with defaults (FIX 1). The install worker uses the builders directly
// with its install-only writers (SetupTPM / WriteNeovimConfig do the clones), so
// only the WRITE action differs between install and config-apply; the
// TRANSLATION is shared.

// ghosttyConfigFrom is the single mapping of DeepDiveConfig to tools.GhosttyConfig,
// shared by the config-apply generator and the install worker.
func ghosttyConfigFrom(cfg DeepDiveConfig) tools.GhosttyConfig {
	return tools.GhosttyConfig{
		FontSize:          cfg.GhosttyFontSize,
		FontFamily:        cfg.GhosttyFontFamily,
		Opacity:           cfg.GhosttyOpacity,
		BlurRadius:        cfg.GhosttyBlurRadius,
		TabBindings:       cfg.GhosttyTabBindings,
		ScrollbackLines:   cfg.GhosttyScrollbackLines,
		CursorStyle:       cfg.GhosttyCursorStyle,
		WindowDecorations: cfg.GhosttyWindowDecorations,
		ConfirmClose:      cfg.GhosttyConfirmClose,
	}
}

// tmuxConfigFrom is the single mapping of DeepDiveConfig to tools.TmuxConfig. Both
// the pure config-apply write (WriteTmuxConfig) and the install worker's TPM setup
// (SetupTPM) consume the same struct; only the write action differs.
func tmuxConfigFrom(cfg DeepDiveConfig) tools.TmuxConfig {
	return tools.TmuxConfig{
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
	}
}

// zshConfigFrom is the single mapping of DeepDiveConfig to tools.ZshConfig.
func zshConfigFrom(cfg DeepDiveConfig) tools.ZshConfig {
	return tools.ZshConfig{
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
	}
}

// neovimConfigFrom is the single mapping of DeepDiveConfig to tools.NeovimConfig.
// The pure config-apply overlay (WriteNeovimUserPrefs) and the install worker's
// preset clone (WriteNeovimConfig) consume the same struct; only the write action
// differs.
func neovimConfigFrom(cfg DeepDiveConfig) tools.NeovimConfig {
	return tools.NeovimConfig{
		ConfigPreset: cfg.NeovimConfig,
		LSPs:         cfg.NeovimLSPs,
		Plugins:      cfg.NeovimPlugins,
		TabWidth:     cfg.NeovimTabWidth,
		Wrap:         cfg.NeovimWrap,
		CursorLine:   cfg.NeovimCursorLine,
		Clipboard:    cfg.NeovimClipboard,
		LineNumbers:  cfg.NeovimLineNumbers,
		ExpandTab:    cfg.NeovimExpandTab,
		UndoFile:     cfg.NeovimUndoFile,
	}
}

// gitConfigFrom is the single mapping of DeepDiveConfig to tools.GitConfig.
func gitConfigFrom(cfg DeepDiveConfig) tools.GitConfig {
	return tools.GitConfig{
		DeltaSideBySide:  cfg.GitDeltaSideBySide,
		DefaultBranch:    cfg.GitDefaultBranch,
		Aliases:          cfg.GitAliases,
		PullRebase:       cfg.GitPullRebase,
		SignCommits:      cfg.GitSignCommits,
		CredentialHelper: cfg.GitCredentialHelper,
		AutoSetupRemote:  cfg.GitAutoSetupRemote,
		MergeTool:        cfg.GitMergeTool,
		DiffTool:         cfg.GitDiffTool,
	}
}

// yaziConfigFrom is the single mapping of DeepDiveConfig to tools.YaziConfig.
func yaziConfigFrom(cfg DeepDiveConfig) tools.YaziConfig {
	return tools.YaziConfig{
		Keymap:      cfg.YaziKeymap,
		ShowHidden:  cfg.YaziShowHidden,
		PreviewMode: cfg.YaziPreviewMode,
		SortBy:      cfg.YaziSortBy,
		SortReverse: cfg.YaziSortReverse,
		LineMode:    cfg.YaziLineMode,
		ScrollOff:   cfg.YaziScrollOff,
	}
}

// fzfConfigFrom is the single mapping of DeepDiveConfig to tools.FzfConfig.
func fzfConfigFrom(cfg DeepDiveConfig) tools.FzfConfig {
	return tools.FzfConfig{
		Preview:       cfg.FzfPreview,
		Height:        cfg.FzfHeight,
		Layout:        cfg.FzfLayout,
		DefaultOpts:   cfg.FzfDefaultOpts,
		BorderStyle:   cfg.FzfBorderStyle,
		PreviewWindow: cfg.FzfPreviewWindow,
	}
}

// lazygitConfigFrom is the single mapping of DeepDiveConfig to tools.LazyGitConfig.
func lazygitConfigFrom(cfg DeepDiveConfig) tools.LazyGitConfig {
	return tools.LazyGitConfig{
		SideBySide: cfg.LazyGitSideBySide,
		MouseMode:  cfg.LazyGitMouseMode,
		Theme:      cfg.LazyGitTheme,
		Paging:     cfg.LazyGitPaging,
	}
}

// btopConfigFrom is the single mapping of DeepDiveConfig to tools.BtopConfig.
func btopConfigFrom(cfg DeepDiveConfig) tools.BtopConfig {
	return tools.BtopConfig{
		Theme:      cfg.BtopTheme,
		UpdateMs:   cfg.BtopUpdateMs,
		ShowTemp:   cfg.BtopShowTemp,
		GraphType:  cfg.BtopGraphType,
		TempScale:  cfg.BtopTempScale,
		ShownBoxes: cfg.BtopShownBoxes,
	}
}

// glowConfigFrom is the single mapping of DeepDiveConfig to tools.GlowConfig.
func glowConfigFrom(cfg DeepDiveConfig) tools.GlowConfig {
	return tools.GlowConfig{
		Pager: cfg.GlowPager,
		Style: cfg.GlowStyle,
		Width: cfg.GlowWidth,
		Mouse: cfg.GlowMouse,
	}
}

// toolConfigGenerators maps a tool ID to the function that writes that one tool's
// config file from a DeepDiveConfig, using the shared *ConfigFrom builder for the
// translation. It is the SINGLE source of the generator invocations:
// applyOneToolConfig runs exactly one entry (standalone `dotfiles config <tool>`,
// and Manage save via applyChangedManageTools), so those paths can never drift.
// Keys match the tool IDs in toolConfigScreens.
//
// claude-code is intentionally omitted here because its generator only runs when
// MCP servers are configured; applyOneToolConfig handles that gated case
// explicitly.
//
// tmux and neovim generators are PURE writes: WriteTmuxConfig writes only
// ~/.tmux.conf (TPM clone is an install-only side-effect, installation.go calls
// SetupTPM), and WriteNeovimUserPrefs overlays only lua/custom/options.lua (preset
// clone + the destructive nvim move are install-only, installation.go calls
// WriteNeovimConfig). Config-apply — Manage save and `dotfiles config <tool>` —
// must never clone or hit the network.
var toolConfigGenerators = map[string]func(cfg DeepDiveConfig, theme string) error{
	"ghostty": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteGhosttyConfig(ghosttyConfigFrom(cfg), theme)
	},
	"tmux": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteTmuxConfig(tmuxConfigFrom(cfg), theme)
	},
	"zsh": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteZshConfig(zshConfigFrom(cfg), theme)
	},
	"neovim": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteNeovimUserPrefs(neovimConfigFrom(cfg), theme)
	},
	"git": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteGitConfig(gitConfigFrom(cfg), theme)
	},
	"yazi": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteYaziConfig(yaziConfigFrom(cfg), theme)
	},
	"fzf": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteFzfConfig(fzfConfigFrom(cfg), theme)
	},
	"lazygit": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteLazyGitConfig(lazygitConfigFrom(cfg), theme)
	},
	"btop": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteBtopConfig(btopConfigFrom(cfg), theme)
	},
	"glow": func(cfg DeepDiveConfig, theme string) error {
		return tools.WriteGlowConfig(glowConfigFrom(cfg), theme)
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

// applyOneToolConfig writes ONLY the named tool's config file from a
// DeepDiveConfig. It is the single scoped writer used by the standalone
// `dotfiles config <tool>` exit and, one tool at a time via
// applyChangedManageTools, by the Manage save — so editing one tool's settings can
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
// DeepDiveConfig suitable for the config-apply generators. It starts from
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
// they are not-applied (manageNotAppliedFields). GitDiffTool is now mapped to the
// real GitConfig.DiffTool field, so delta/difftastic/vimdiff each produce a
// distinct .gitconfig (delta pager vs difftastic external diff vs vim difftool).
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
	dd.GhosttyTabBindings = mc.GhosttyTabBindings

	// Tmux (prefix vocabulary reconciled for the generator).
	dd.TmuxPrefix = tmuxPrefixToGenerator(mc.TmuxPrefix)
	dd.TmuxSplitBinds = mc.TmuxSplitBinds
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
	dd.NeovimExpandTab = mc.NeovimExpandTab
	dd.NeovimUndoFile = mc.NeovimUndoFile

	// Git. GitDiffTool now drives the real DiffTool field (delta/difftastic/
	// vimdiff); it is no longer collapsed into the DeltaSideBySide boolean.
	// DeltaSideBySide keeps its NewDeepDiveConfig default (true) because the Manage
	// UI exposes no side-by-side toggle — only the wizard's Git screen does.
	dd.GitDefaultBranch = mc.GitDefaultBranch
	dd.GitPullRebase = mc.GitPullRebase
	dd.GitSignCommits = mc.GitSignCommits
	dd.GitCredentialHelper = mc.GitCredentialHelper
	dd.GitDiffTool = mc.GitDiffTool
	dd.GitAutoSetupRemote = mc.GitAutoSetupRemote
	dd.GitMergeTool = mc.GitMergeTool
	dd.GitDeltaSideBySide = mc.GitDeltaSideBySide
	dd.GitAliases = nil
	if mc.GitAliasStatus {
		dd.GitAliases = append(dd.GitAliases, "st")
	}
	if mc.GitAliasCheckout {
		dd.GitAliases = append(dd.GitAliases, "co")
	}
	if mc.GitAliasBranch {
		dd.GitAliases = append(dd.GitAliases, "br")
	}
	if mc.GitAliasCommit {
		dd.GitAliases = append(dd.GitAliases, "ci")
	}
	if mc.GitAliasLogGraph {
		dd.GitAliases = append(dd.GitAliases, "lg")
	}

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
