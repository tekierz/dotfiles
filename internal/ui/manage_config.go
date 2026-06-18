package ui

// ManageConfig holds detailed management configuration for all tools
type ManageConfig struct {
	// Ghostty detailed settings
	GhosttyFontFamily        string
	GhosttyFontSize          int
	GhosttyOpacity           int
	GhosttyBlurRadius        int
	GhosstyCursorStyle       string
	GhosttyScrollbackLines   int
	GhosttyWindowDecorations bool
	GhosttyConfirmClose      bool

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
	NeovimLineNumbers string
	NeovimRelativeNum bool
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

	// Yazi detailed settings
	YaziShowHidden  bool
	YaziSortBy      string
	YaziSortReverse bool
	YaziLineMode    string
	YaziScrollOff   int

	// FZF detailed settings
	FzfDefaultOpts   string
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
		// Ghostty
		GhosttyFontFamily:        "JetBrainsMono Nerd Font",
		GhosttyFontSize:          14,
		GhosttyOpacity:           100,
		GhosttyBlurRadius:        0,
		GhosstyCursorStyle:       "block",
		GhosttyScrollbackLines:   10000,
		GhosttyWindowDecorations: true,
		GhosttyConfirmClose:      true,

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

		// Neovim
		NeovimLineNumbers: "absolute",
		NeovimRelativeNum: true,
		NeovimTabWidth:    4,
		NeovimExpandTab:   true,
		NeovimWrap:        false,
		NeovimCursorLine:  true,
		NeovimClipboard:   "unnamedplus",
		NeovimUndoFile:    true,

		// Git
		GitDefaultBranch:    "main",
		GitAutoSetupRemote:  true,
		GitPullRebase:       true,
		GitDiffTool:         "delta",
		GitMergeTool:        "vimdiff",
		GitCredentialHelper: "store",
		GitSignCommits:      false,

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
		GlowStyle: "auto",
		GlowPager: "less",
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
