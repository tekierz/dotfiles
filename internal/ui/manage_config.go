package ui

// ManageConfig holds detailed management configuration for all tools
type ManageConfig struct {
	NativeImportSchemaVersion int
	// Ghostty detailed settings
	GhosttyFontFamily  string
	GhosttyFontSize    int
	GhosttyOpacity     int
	GhosttyBlurRadius  int
	GhosstyCursorStyle string
	// GhosttyScrollbackLines is retained as a serialized compatibility name;
	// Ghostty interprets scrollback-limit as bytes, not terminal rows.
	GhosttyScrollbackLines   int
	GhosttyWindowDecorations bool
	GhosttyConfirmClose      bool
	GhosttyTabBindings       string

	// Tmux detailed settings
	TmuxPrefix           string
	TmuxBaseIndex        int
	TmuxMouseMode        bool
	TmuxStatusPosition   string
	TmuxPaneBorderStyle  string
	TmuxHistoryLimit     int
	TmuxEscapeTime       int
	TmuxAggressiveResize bool

	// Tmux TPM settings
	TmuxTPMEnabled       bool
	TmuxPluginSensible   bool
	TmuxPluginResurrect  bool
	TmuxPluginContinuum  bool
	TmuxPluginYank       bool
	TmuxContinuumSaveMin int
	TmuxContinuumRestore bool

	// Zsh detailed settings
	ZshHistorySize       int
	ZshHistoryIgnoreDups bool
	ZshAutoCD            bool
	ZshCorrection        bool
	ZshCompletionMenu    bool
	ZshSyntaxHighlight   bool
	ZshAutosuggestions   bool

	// Neovim detailed settings
	NeovimLineNumbers string // absolute/relative/none — also drives relativenumber
	NeovimTabWidth    int
	NeovimExpandTab   bool
	NeovimWrap        bool
	NeovimCursorLine  bool
	NeovimClipboard   string
	NeovimUndoFile    bool

	// Git detailed settings
	GitDefaultBranch    string
	GitAutoSetupRemote  bool
	GitPullRebase       bool
	GitDiffTool         string
	GitMergeTool        string
	GitCredentialHelper string
	GitSignCommits      bool
	GitDeltaSideBySide  bool
	GitAliasStatus      bool
	GitAliasCheckout    bool
	GitAliasBranch      bool
	GitAliasCommit      bool
	GitAliasLogGraph    bool

	// Yazi detailed settings
	YaziShowHidden  bool
	YaziSortBy      string
	YaziSortReverse bool
	YaziLineMode    string
	YaziScrollOff   int

	// FZF detailed settings
	FzfDefaultOpts   string // additional fzf flags stored as inert data
	FzfHeight        int
	FzfLayout        string
	FzfBorderStyle   string
	FzfPreview       bool
	FzfPreviewWindow string

	// LazyGit detailed settings
	LazyGitSideBySide bool
	LazyGitPaging     string
	LazyGitMouseMode  bool
	LazyGitGuiTheme   string

	// LazyDocker detailed settings
	LazyDockerMouseMode bool
	LazyDockerLogsTail  int

	// Btop detailed settings
	BtopTheme       string
	BtopUpdateMs    int
	BtopShowTemp    bool
	BtopTempScale   string
	BtopGraphSymbol string
	BtopShownBoxes  string

	// Glow detailed settings
	GlowStyle string
	GlowPager string
	GlowWidth int
	GlowMouse bool

	// Claude Code MCP server settings
	ClaudeCodeMCPContext7           bool
	ClaudeCodeMCPTaskMaster         bool
	ClaudeCodeMCPGitHub             bool
	ClaudeCodeMCPSupabase           bool
	ClaudeCodeMCPConvex             bool
	ClaudeCodeMCPPuppeteer          bool
	ClaudeCodeMCPSequentialThinking bool
}

// NewManageConfig creates a new management config with defaults
func NewManageConfig() *ManageConfig {
	return &ManageConfig{
		NativeImportSchemaVersion: 1,
		// Ghostty
		// Use the same default as NewDeepDiveConfig so the two config models do
		// not disagree on the same setting (C13). "JetBrains Mono" is also the
		// canonical value cycled by the Ghostty deep-dive config screen.
		GhosttyFontFamily:        "JetBrains Mono",
		GhosttyFontSize:          14,
		GhosttyOpacity:           100,
		GhosttyBlurRadius:        0,
		GhosstyCursorStyle:       "block",
		GhosttyScrollbackLines:   10_000_000,
		GhosttyWindowDecorations: true,
		GhosttyConfirmClose:      true,
		GhosttyTabBindings:       "super",

		// Tmux
		TmuxPrefix:           "C-a",
		TmuxBaseIndex:        1,
		TmuxMouseMode:        true,
		TmuxStatusPosition:   "bottom",
		TmuxPaneBorderStyle:  "single",
		TmuxHistoryLimit:     50000,
		TmuxEscapeTime:       10,
		TmuxAggressiveResize: true,

		// Tmux TPM
		TmuxTPMEnabled:       true,
		TmuxPluginSensible:   true,
		TmuxPluginResurrect:  true,
		TmuxPluginContinuum:  false,
		TmuxPluginYank:       true,
		TmuxContinuumSaveMin: 15,
		TmuxContinuumRestore: true,

		// Zsh
		ZshHistorySize:       10000,
		ZshHistoryIgnoreDups: true,
		ZshAutoCD:            true,
		ZshCorrection:        true,
		ZshCompletionMenu:    true,
		ZshSyntaxHighlight:   true,
		ZshAutosuggestions:   true,

		// Neovim. "relative" preserves the prior default (number + relativenumber
		// both on) now that LineNumbers is the single source for both opts.
		NeovimLineNumbers: "relative",
		NeovimTabWidth:    4,
		NeovimExpandTab:   true,
		NeovimWrap:        false,
		NeovimCursorLine:  true,
		NeovimClipboard:   "unnamedplus",
		NeovimUndoFile:    true,

		// Git
		GitDefaultBranch:   "main",
		GitAutoSetupRemote: true,
		GitPullRebase:      true,
		GitDiffTool:        "delta",
		GitMergeTool:       "vimdiff",
		// Match NewDeepDiveConfig's default (C13). "cache" avoids writing
		// credentials to disk in plaintext the way "store" does.
		GitCredentialHelper: "cache",
		GitSignCommits:      false,
		GitDeltaSideBySide:  true,
		GitAliasStatus:      true,
		GitAliasCheckout:    true,
		GitAliasBranch:      true,
		GitAliasCommit:      true,
		GitAliasLogGraph:    true,

		// Yazi
		YaziShowHidden:  false,
		YaziSortBy:      "alphabetical",
		YaziSortReverse: false,
		YaziLineMode:    "size",
		YaziScrollOff:   5,

		// FZF
		FzfDefaultOpts:   "",
		FzfHeight:        40,
		FzfLayout:        "reverse",
		FzfBorderStyle:   "rounded",
		FzfPreview:       true,
		FzfPreviewWindow: "right:50%",

		// LazyGit
		LazyGitSideBySide: true,
		LazyGitPaging:     "delta",
		LazyGitMouseMode:  true,
		LazyGitGuiTheme:   "auto",

		// LazyDocker
		LazyDockerMouseMode: true,
		LazyDockerLogsTail:  100,

		// Btop
		BtopTheme:       "auto",
		BtopUpdateMs:    2000,
		BtopShowTemp:    true,
		BtopTempScale:   "celsius",
		BtopGraphSymbol: "braille",
		BtopShownBoxes:  "cpu mem net proc",

		// Glow
		// Match NewDeepDiveConfig's default of "auto" (C13).
		GlowStyle: "auto",
		GlowPager: "auto",
		GlowWidth: 80,
		GlowMouse: true,

		// Claude Code MCPs (context7 enabled by default)
		ClaudeCodeMCPContext7:           true,
		ClaudeCodeMCPTaskMaster:         false,
		ClaudeCodeMCPGitHub:             false,
		ClaudeCodeMCPSupabase:           false,
		ClaudeCodeMCPConvex:             false,
		ClaudeCodeMCPPuppeteer:          false,
		ClaudeCodeMCPSequentialThinking: false,
	}
}
