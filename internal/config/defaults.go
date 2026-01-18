package config

// DefaultGhosttyConfig returns default Ghostty settings
func DefaultGhosttyConfig() *GhosttyConfig {
	return &GhosttyConfig{
		FontSize:    14,
		Opacity:     100,
		TabBindings: "super",
	}
}

// DefaultTmuxConfig returns default tmux settings
func DefaultTmuxConfig() *TmuxConfig {
	return &TmuxConfig{
		Prefix:     "ctrl-a",
		SplitBinds: "pipes",
		StatusBar:  "bottom",
		MouseMode:  true,
	}
}

// DefaultZshConfig returns default zsh settings
func DefaultZshConfig() *ZshConfig {
	return &ZshConfig{
		PromptStyle: "p10k",
		Plugins: []string{
			"zsh-autosuggestions",
			"zsh-syntax-highlighting",
			"zsh-completions",
			"zsh-autocorrect",
		},
		Aliases: map[string]bool{
			"ll":     true,
			"la":     true,
			"gs":     true,
			"gp":     true,
			"gc":     true,
			"docker": true,
		},
	}
}

// DefaultNeovimConfig returns default neovim settings
func DefaultNeovimConfig() *NeovimConfig {
	return &NeovimConfig{
		Config: "kickstart",
		LSPs: []string{
			"lua_ls",
			"pyright",
			"tsserver",
			"gopls",
		},
		Plugins: []string{
			"telescope",
			"treesitter",
			"lsp",
			"cmp",
		},
	}
}

// DefaultGitConfig returns default git settings
func DefaultGitConfig() *GitConfig {
	return &GitConfig{
		DeltaSideBySide: true,
		DefaultBranch:   "main",
		Aliases:         []string{"st", "co", "br", "ci", "lg"},
	}
}

// DefaultYaziConfig returns default yazi settings
func DefaultYaziConfig() *YaziConfig {
	return &YaziConfig{
		Keymap:      "vim",
		ShowHidden:  false,
		PreviewMode: "auto",
	}
}

// DefaultFzfConfig returns default fzf settings
func DefaultFzfConfig() *FzfConfig {
	return &FzfConfig{
		Preview: true,
		Height:  40,
		Layout:  "reverse",
	}
}

// DefaultAppsConfig returns default app selections
func DefaultAppsConfig() *AppsConfig {
	return &AppsConfig{
		// macOS productivity (popular ones enabled)
		Rectangle:      true,
		Raycast:        true,
		Stats:          true,
		AltTab:         false,
		MonitorControl: false,
		Mos:            false,
		Karabiner:      false,
		IINA:           false,
		TheUnarchiver:  true,
		AppCleaner:     true,

		// Cross-platform (disabled by default, user chooses)
		ZenBrowser: false,
		Cursor:     false,
		LMStudio:   false,
		OBS:        false,
	}
}

// DefaultUtilitiesConfig returns default utility selections
func DefaultUtilitiesConfig() *UtilitiesConfig {
	return &UtilitiesConfig{
		// Existing utilities (all enabled)
		HK:   true,
		Caff: true,
		Sshh: true,

		// New CLI tools (popular ones enabled)
		LazyGit:    true,
		LazyDocker: true,
		ClaudeCode: false, // Requires npm, optional
		OpenCode:   false, // Optional
		Glow:       true,
		Btop:       true,
		Fswatch:    false, // Optional
	}
}
