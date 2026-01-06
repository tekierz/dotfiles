package ui

// DeepDiveConfig holds all deep dive configuration options
type DeepDiveConfig struct {
	// Ghostty settings
	GhosttyFontSize    int
	GhosttyOpacity     int // 0-100
	GhosttyTabBindings string

	// Tmux settings
	TmuxPrefix     string
	TmuxSplitBinds string
	TmuxStatusBar  string
	TmuxMouseMode  bool

	// Tmux TPM (Plugin Manager) settings
	TmuxTPMEnabled       bool
	TmuxPluginSensible   bool
	TmuxPluginResurrect  bool
	TmuxPluginContinuum  bool
	TmuxPluginYank       bool
	TmuxContinuumSaveMin int // 5-60 minutes

	// Zsh settings
	ZshPromptStyle string
	ZshPlugins     []string
	ZshAliases     map[string]bool

	// Neovim settings
	NeovimConfig  string
	NeovimLSPs    []string
	NeovimPlugins []string

	// Git settings
	GitDeltaSideBySide bool
	GitDefaultBranch   string
	GitAliases         []string

	// Yazi settings
	YaziKeymap      string
	YaziShowHidden  bool
	YaziPreviewMode string

	// FZF settings
	FzfPreview bool
	FzfHeight  int
	FzfLayout  string

	// macOS Apps
	MacApps map[string]bool

	// Utilities (hk, caff, sshh)
	Utilities map[string]bool

	// CLI Tools install flags
	CLITools map[string]bool

	// GUI Apps install flags
	GUIApps map[string]bool

	// CLI Utilities install flags (bat, eza, zoxide, ripgrep, fd, delta, fswatch)
	CLIUtilities map[string]bool

	// LazyGit settings
	LazyGitSideBySide bool
	LazyGitMouseMode  bool
	LazyGitTheme      string

	// LazyDocker settings
	LazyDockerMouseMode bool

	// Btop settings
	BtopTheme     string
	BtopUpdateMs  int
	BtopShowTemp  bool
	BtopGraphType string

	// Glow settings
	GlowPager string
	GlowStyle string
	GlowWidth int
}

// NewDeepDiveConfig creates a new config with defaults
func NewDeepDiveConfig() *DeepDiveConfig {
	return &DeepDiveConfig{
		// Ghostty defaults
		GhosttyFontSize:    14,
		GhosttyOpacity:     100,
		GhosttyTabBindings: "super",

		// Tmux defaults
		TmuxPrefix:     "ctrl-a",
		TmuxSplitBinds: "pipes", // | and -
		TmuxStatusBar:  "bottom",
		TmuxMouseMode:  true,

		// Tmux TPM defaults
		TmuxTPMEnabled:       true,
		TmuxPluginSensible:   true,
		TmuxPluginResurrect:  true,
		TmuxPluginContinuum:  false, // Opt-in for auto-save
		TmuxPluginYank:       true,
		TmuxContinuumSaveMin: 15,

		// Zsh defaults
		ZshPromptStyle: "p10k",
		ZshPlugins: []string{
			"zsh-autosuggestions",
			"zsh-syntax-highlighting",
			"zsh-completions",
		},
		ZshAliases: map[string]bool{
			"ll":     true,
			"la":     true,
			"gs":     true,
			"gp":     true,
			"gc":     true,
			"docker": true,
		},

		// Neovim defaults
		NeovimConfig: "kickstart",
		NeovimLSPs: []string{
			"lua_ls",
			"pyright",
			"tsserver",
			"gopls",
		},
		NeovimPlugins: []string{
			"telescope",
			"treesitter",
			"lsp",
			"cmp",
		},

		// Git defaults
		GitDeltaSideBySide: true,
		GitDefaultBranch:   "main",
		GitAliases: []string{
			"st", "co", "br", "ci", "lg",
		},

		// Yazi defaults
		YaziKeymap:      "vim",
		YaziShowHidden:  false,
		YaziPreviewMode: "auto",

		// FZF defaults
		FzfPreview: true,
		FzfHeight:  40,
		FzfLayout:  "reverse",

		// macOS Apps defaults
		MacApps: map[string]bool{
			"rectangle":       true,
			"raycast":         true,
			"stats":           true,
			"alt-tab":         false,
			"monitor-control": false,
			"mos":             false,
			"karabiner":       false,
			"iina":            false,
			"the-unarchiver":  true,
			"appcleaner":      true,
		},

		// Utilities defaults (all enabled by default)
		Utilities: map[string]bool{
			"hk":   true,
			"caff": true,
			"sshh": true,
		},

		// CLI Tools defaults
		CLITools: map[string]bool{
			"lazygit":     true,
			"lazydocker":  true,
			"btop":        true,
			"glow":        true,
			"claude-code": false,
		},

		// GUI Apps defaults
		GUIApps: map[string]bool{
			"zen-browser": false,
			"cursor":      false,
			"lm-studio":   false,
			"obs":         false,
		},

		// CLI Utilities defaults (commonly useful tools enabled by default)
		CLIUtilities: map[string]bool{
			"bat":     true,  // cat replacement with syntax highlighting
			"eza":     true,  // ls replacement
			"zoxide":  true,  // cd replacement
			"ripgrep": true,  // grep replacement
			"fd":      true,  // find replacement
			"delta":   true,  // git diff viewer
			"fswatch": false, // file watcher (optional)
		},

		// LazyGit defaults
		LazyGitSideBySide: true,
		LazyGitMouseMode:  true,
		LazyGitTheme:      "auto",

		// LazyDocker defaults
		LazyDockerMouseMode: true,

		// Btop defaults
		BtopTheme:     "auto",
		BtopUpdateMs:  2000,
		BtopShowTemp:  true,
		BtopGraphType: "braille",

		// Glow defaults
		GlowPager: "auto",
		GlowStyle: "auto",
		GlowWidth: 80,
	}
}

// DeepDiveMenuItem represents an item in the deep dive menu
type DeepDiveMenuItem struct {
	Name        string
	Description string
	Screen      Screen
	Icon        string
	Category    string // Category header (empty = same category as previous)
	Platform    string // Platform filter: "macos", "linux", or "" for all
}

// GetDeepDiveMenuItems returns the menu items for deep dive configuration
// Items are organized into logical categories for clarity
func GetDeepDiveMenuItems() []DeepDiveMenuItem {
	return []DeepDiveMenuItem{
		// Terminal & Shell - the foundation
		{
			Name:        "Ghostty",
			Description: "Terminal font, opacity, keybindings",
			Screen:      ScreenConfigGhostty,
			Icon:        "󰆍",
			Category:    "TERMINAL & SHELL",
		},
		{
			Name:        "Tmux",
			Description: "Prefix key, splits, mouse, TPM plugins",
			Screen:      ScreenConfigTmux,
			Icon:        "",
		},
		{
			Name:        "Zsh",
			Description: "Prompt style, plugins, aliases",
			Screen:      ScreenConfigZsh,
			Icon:        "",
		},
		// Development - coding essentials
		{
			Name:        "Neovim",
			Description: "Config preset, LSP servers, plugins",
			Screen:      ScreenConfigNeovim,
			Icon:        "",
			Category:    "DEVELOPMENT",
		},
		{
			Name:        "Git",
			Description: "Delta diff, default branch, aliases",
			Screen:      ScreenConfigGit,
			Icon:        "",
		},
		{
			Name:        "CLI Tools",
			Description: "LazyGit, LazyDocker, btop, Glow, Claude",
			Screen:      ScreenConfigCLITools,
			Icon:        "",
		},
		// Quality of Life tools
		{
			Name:        "Yazi",
			Description: "File manager keymaps, preview settings",
			Screen:      ScreenConfigYazi,
			Icon:        "󰉋",
			Category:    "QUALITY OF LIFE",
		},
		{
			Name:        "FZF",
			Description: "Fuzzy finder preview, layout, height",
			Screen:      ScreenConfigFzf,
			Icon:        "󰍉",
		},
		{
			Name:        "CLI Utilities",
			Description: "bat, eza, zoxide, ripgrep, fd, delta",
			Screen:      ScreenConfigCLIUtilities,
			Icon:        "󰘳",
		},
		// Optional Apps
		{
			Name:        "GUI Apps",
			Description: "Zen Browser, Cursor, LM Studio, OBS",
			Screen:      ScreenConfigGUIApps,
			Icon:        "󰏇",
			Category:    "OPTIONAL APPS",
		},
		{
			Name:        "macOS Apps",
			Description: "Rectangle, Raycast, Stats, more",
			Screen:      ScreenConfigMacApps,
			Icon:        "",
			Platform:    "macos",
		},
		{
			Name:        "Helper Scripts",
			Description: "hk, caff, sshh utilities",
			Screen:      ScreenConfigUtilities,
			Icon:        "󰘚",
		},
	}
}
