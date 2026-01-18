package config

// GhosttyConfig holds Ghostty terminal settings
type GhosttyConfig struct {
	FontSize    int    `json:"font_size"`
	Opacity     int    `json:"opacity"`
	TabBindings string `json:"tab_bindings"`
}

// TmuxConfig holds tmux settings
type TmuxConfig struct {
	Prefix     string `json:"prefix"`
	SplitBinds string `json:"split_binds"`
	StatusBar  string `json:"status_bar"`
	MouseMode  bool   `json:"mouse_mode"`
}

// ZshConfig holds zsh shell settings
type ZshConfig struct {
	PromptStyle string          `json:"prompt_style"`
	Plugins     []string        `json:"plugins"`
	Aliases     map[string]bool `json:"aliases"`
}

// NeovimConfig holds neovim settings
type NeovimConfig struct {
	Config  string   `json:"config"`
	LSPs    []string `json:"lsps"`
	Plugins []string `json:"plugins"`
}

// GitConfig holds git settings
type GitConfig struct {
	DeltaSideBySide bool     `json:"delta_side_by_side"`
	DefaultBranch   string   `json:"default_branch"`
	Aliases         []string `json:"aliases"`
}

// YaziConfig holds yazi file manager settings
type YaziConfig struct {
	Keymap      string `json:"keymap"`
	ShowHidden  bool   `json:"show_hidden"`
	PreviewMode string `json:"preview_mode"`
}

// FzfConfig holds fzf fuzzy finder settings
type FzfConfig struct {
	Preview bool   `json:"preview"`
	Height  int    `json:"height"`
	Layout  string `json:"layout"`
}

// AppsConfig holds optional application selections
type AppsConfig struct {
	// macOS productivity apps
	Rectangle      bool `json:"rectangle"`
	Raycast        bool `json:"raycast"`
	Stats          bool `json:"stats"`
	AltTab         bool `json:"alt_tab"`
	MonitorControl bool `json:"monitor_control"`
	Mos            bool `json:"mos"`
	Karabiner      bool `json:"karabiner"`
	IINA           bool `json:"iina"`
	TheUnarchiver  bool `json:"the_unarchiver"`
	AppCleaner     bool `json:"appcleaner"`

	// Cross-platform apps
	ZenBrowser bool `json:"zen_browser"`
	Cursor     bool `json:"cursor"`
	LMStudio   bool `json:"lm_studio"`
	OBS        bool `json:"obs"`
}

// UtilitiesConfig holds utility tool selections
type UtilitiesConfig struct {
	// Existing utilities
	HK   bool `json:"hk"`
	Caff bool `json:"caff"`
	Sshh bool `json:"sshh"`

	// New CLI tools
	LazyGit    bool `json:"lazygit"`
	LazyDocker bool `json:"lazydocker"`
	ClaudeCode bool `json:"claude_code"`
	OpenCode   bool `json:"opencode"`
	Glow       bool `json:"glow"`
	Btop       bool `json:"btop"`
	Fswatch    bool `json:"fswatch"`
}
