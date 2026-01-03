package ui

// DeepDiveConfig holds all deep dive configuration options
type DeepDiveConfig struct {
	// Ghostty settings
	GhosttyFontSize    int
	GhosttyOpacity     int // 0-100
	GhosttyTabBindings string

	// Tmux settings
	TmuxPrefix      string
	TmuxSplitBinds  string
	TmuxStatusBar   string
	TmuxMouseMode   bool

	// Zsh settings
	ZshPromptStyle string
	ZshPlugins     []string
	ZshAliases     map[string]bool

	// Neovim settings
	NeovimConfig   string
	NeovimLSPs     []string
	NeovimPlugins  []string

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

	// CLI Tools (lazygit, lazydocker, etc.)
	CLITools map[string]bool

	// GUI Apps (Zen Browser, Cursor, etc.)
	GUIApps map[string]bool
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

		// Zsh defaults
		ZshPromptStyle: "p10k",
		ZshPlugins: []string{
			"zsh-autosuggestions",
			"zsh-syntax-highlighting",
			"zsh-completions",
		},
		ZshAliases: map[string]bool{
			"ll":    true,
			"la":    true,
			"gs":    true,
			"gp":    true,
			"gc":    true,
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
	}
}

// DeepDiveMenuItem represents an item in the deep dive menu
type DeepDiveMenuItem struct {
	Name        string
	Description string
	Screen      Screen
	Icon        string
}

// GetDeepDiveMenuItems returns the menu items for deep dive configuration
func GetDeepDiveMenuItems() []DeepDiveMenuItem {
	return []DeepDiveMenuItem{
		{
			Name:        "Ghostty",
			Description: "Terminal font, opacity, keybindings",
			Screen:      ScreenConfigGhostty,
			Icon:        "󰆍",
		},
		{
			Name:        "Tmux",
			Description: "Prefix key, splits, status bar, mouse",
			Screen:      ScreenConfigTmux,
			Icon:        "",
		},
		{
			Name:        "Zsh",
			Description: "Prompt style, plugins, aliases",
			Screen:      ScreenConfigZsh,
			Icon:        "",
		},
		{
			Name:        "Neovim",
			Description: "Config preset, LSP servers, plugins",
			Screen:      ScreenConfigNeovim,
			Icon:        "",
		},
		{
			Name:        "Git",
			Description: "Delta diff, default branch, aliases",
			Screen:      ScreenConfigGit,
			Icon:        "",
		},
		{
			Name:        "Yazi",
			Description: "File manager keymaps, preview settings",
			Screen:      ScreenConfigYazi,
			Icon:        "󰉋",
		},
		{
			Name:        "FZF",
			Description: "Fuzzy finder preview, layout, height",
			Screen:      ScreenConfigFzf,
			Icon:        "",
		},
		{
			Name:        "CLI Tools",
			Description: "LazyGit, LazyDocker, btop, glow",
			Screen:      ScreenConfigCLITools,
			Icon:        "",
		},
		{
			Name:        "GUI Apps",
			Description: "Zen Browser, Cursor, LM Studio, OBS",
			Screen:      ScreenConfigGUIApps,
			Icon:        "",
		},
		{
			Name:        "macOS Apps",
			Description: "Rectangle, Raycast, and more",
			Screen:      ScreenConfigMacApps,
			Icon:        "",
		},
		{
			Name:        "Utilities",
			Description: "hk, caff, sshh helper tools",
			Screen:      ScreenConfigUtilities,
			Icon:        "",
		},
	}
}
