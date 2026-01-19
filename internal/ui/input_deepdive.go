package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// handleDeepDiveKey handles key events for deep dive screens:
// ScreenDeepDiveMenu and all ScreenConfig* screens
func (a *App) handleDeepDiveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.screen {
	// Deep dive menu navigation
	case ScreenDeepDiveMenu:
		menuItems := GetFilteredDeepDiveMenuItems()
		maxIdx := len(menuItems) // +1 for "Continue" option
		switch key {
		case "up", "k":
			if a.deepDiveMenuIndex > 0 {
				a.deepDiveMenuIndex--
			}
		case "down", "j":
			if a.deepDiveMenuIndex < maxIdx {
				a.deepDiveMenuIndex++
			}
		case "enter":
			if a.deepDiveMenuIndex == maxIdx {
				// "Continue to Installation" selected
				a.screen = ScreenThemePicker
			} else {
				// Navigate to specific config screen
				a.screen = menuItems[a.deepDiveMenuIndex].Screen
			}
		case "esc":
			a.screen = ScreenWelcome
		}

	// Ghostty config
	// Fields: 0=font family, 1=font size, 2=opacity, 3=blur, 4=scrollback, 5=cursor style, 6=tab bindings
	case ScreenConfigGhostty:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 6 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 0: // Font family
				opts := []string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"}
				a.deepDiveConfig.GhosttyFontFamily = cycleOption(opts, a.deepDiveConfig.GhosttyFontFamily, false)
			case 1: // Font size
				if a.deepDiveConfig.GhosttyFontSize > 8 {
					a.deepDiveConfig.GhosttyFontSize--
				}
			case 2: // Opacity
				if a.deepDiveConfig.GhosttyOpacity > 0 {
					a.deepDiveConfig.GhosttyOpacity -= 5
				}
			case 3: // Blur radius
				if a.deepDiveConfig.GhosttyBlurRadius > 0 {
					a.deepDiveConfig.GhosttyBlurRadius -= 5
				}
			case 4: // Scrollback lines
				opts := []string{"1000", "5000", "10000", "50000", "100000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.GhosttyScrollbackLines)
				result := cycleOption(opts, current, false)
				a.deepDiveConfig.GhosttyScrollbackLines = atoi(result, 10000)
			case 5: // Cursor style
				opts := []string{"block", "bar", "underline"}
				a.deepDiveConfig.GhosttyCursorStyle = cycleOption(opts, a.deepDiveConfig.GhosttyCursorStyle, false)
			case 6: // Tab bindings
				opts := []string{"super", "ctrl", "alt"}
				a.deepDiveConfig.GhosttyTabBindings = cycleOption(opts, a.deepDiveConfig.GhosttyTabBindings, false)
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 0: // Font family
				opts := []string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"}
				a.deepDiveConfig.GhosttyFontFamily = cycleOption(opts, a.deepDiveConfig.GhosttyFontFamily, true)
			case 1: // Font size
				if a.deepDiveConfig.GhosttyFontSize < 32 {
					a.deepDiveConfig.GhosttyFontSize++
				}
			case 2: // Opacity
				if a.deepDiveConfig.GhosttyOpacity < 100 {
					a.deepDiveConfig.GhosttyOpacity += 5
				}
			case 3: // Blur radius
				if a.deepDiveConfig.GhosttyBlurRadius < 100 {
					a.deepDiveConfig.GhosttyBlurRadius += 5
				}
			case 4: // Scrollback lines
				opts := []string{"1000", "5000", "10000", "50000", "100000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.GhosttyScrollbackLines)
				result := cycleOption(opts, current, true)
				a.deepDiveConfig.GhosttyScrollbackLines = atoi(result, 10000)
			case 5: // Cursor style
				opts := []string{"block", "bar", "underline"}
				a.deepDiveConfig.GhosttyCursorStyle = cycleOption(opts, a.deepDiveConfig.GhosttyCursorStyle, true)
			case 6: // Tab bindings
				opts := []string{"super", "ctrl", "alt"}
				a.deepDiveConfig.GhosttyTabBindings = cycleOption(opts, a.deepDiveConfig.GhosttyTabBindings, true)
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Tmux config
	// Fields: 0=prefix, 1=splits, 2=status, 3=mouse, 4=history, 5=escape, 6=base, 7=TPM toggle
	// If TPM enabled: 8=sensible, 9=resurrect, 10=continuum, 11=yank, 12=interval (if continuum)
	case ScreenConfigTmux:
		// Calculate max field based on TPM state
		maxField := 7 // Fields 0-7 (basic settings + TPM toggle)
		if a.deepDiveConfig.TmuxTPMEnabled {
			maxField = 11 // + plugin toggles (8-11)
			if a.deepDiveConfig.TmuxPluginContinuum {
				maxField = 12 // + continuum interval
			}
		}

		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
				// Skip hidden fields when TPM is disabled
				if !a.deepDiveConfig.TmuxTPMEnabled && a.configFieldIndex > 7 {
					a.configFieldIndex = 7
				}
				// Skip interval field when continuum is disabled
				if a.deepDiveConfig.TmuxTPMEnabled && !a.deepDiveConfig.TmuxPluginContinuum && a.configFieldIndex == 12 {
					a.configFieldIndex = 11
				}
			}
		case "down", "j":
			if a.configFieldIndex < maxField {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			fwd := key == "right" || key == "l"
			switch a.configFieldIndex {
			case 0: // Prefix
				opts := []string{"ctrl-a", "ctrl-b", "ctrl-space"}
				a.deepDiveConfig.TmuxPrefix = cycleOption(opts, a.deepDiveConfig.TmuxPrefix, fwd)
			case 1: // Split binds
				if a.deepDiveConfig.TmuxSplitBinds == "pipes" {
					a.deepDiveConfig.TmuxSplitBinds = "percent"
				} else {
					a.deepDiveConfig.TmuxSplitBinds = "pipes"
				}
			case 2: // Status bar
				if a.deepDiveConfig.TmuxStatusBar == "top" {
					a.deepDiveConfig.TmuxStatusBar = "bottom"
				} else {
					a.deepDiveConfig.TmuxStatusBar = "top"
				}
			case 4: // History limit
				opts := []string{"10000", "25000", "50000", "100000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.TmuxHistoryLimit)
				a.deepDiveConfig.TmuxHistoryLimit = atoi(cycleOption(opts, current, fwd), 50000)
			case 5: // Escape time
				opts := []string{"0", "10", "50", "100"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.TmuxEscapeTime)
				a.deepDiveConfig.TmuxEscapeTime = atoi(cycleOption(opts, current, fwd), 10)
			case 6: // Base index
				if a.deepDiveConfig.TmuxBaseIndex == 0 {
					a.deepDiveConfig.TmuxBaseIndex = 1
				} else {
					a.deepDiveConfig.TmuxBaseIndex = 0
				}
			case 12: // Continuum interval
				if a.deepDiveConfig.TmuxTPMEnabled && a.deepDiveConfig.TmuxPluginContinuum {
					if fwd {
						if a.deepDiveConfig.TmuxContinuumSaveMin < 60 {
							a.deepDiveConfig.TmuxContinuumSaveMin += 5
						}
					} else {
						if a.deepDiveConfig.TmuxContinuumSaveMin > 5 {
							a.deepDiveConfig.TmuxContinuumSaveMin -= 5
						}
					}
				}
			}
		case " ":
			switch a.configFieldIndex {
			case 3: // Mouse mode
				a.deepDiveConfig.TmuxMouseMode = !a.deepDiveConfig.TmuxMouseMode
			case 7: // TPM enabled
				a.deepDiveConfig.TmuxTPMEnabled = !a.deepDiveConfig.TmuxTPMEnabled
			case 8: // tmux-sensible
				a.deepDiveConfig.TmuxPluginSensible = !a.deepDiveConfig.TmuxPluginSensible
			case 9: // tmux-resurrect
				a.deepDiveConfig.TmuxPluginResurrect = !a.deepDiveConfig.TmuxPluginResurrect
			case 10: // tmux-continuum
				a.deepDiveConfig.TmuxPluginContinuum = !a.deepDiveConfig.TmuxPluginContinuum
			case 11: // tmux-yank
				a.deepDiveConfig.TmuxPluginYank = !a.deepDiveConfig.TmuxPluginYank
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Zsh config
	// Fields: 0-3=prompts, 4=history, 5=autocd, 6=syntax, 7=autosuggestions, 8-12=plugins
	case ScreenConfigZsh:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 12 { // 4 prompts + 4 shell options + 5 plugins - 1
				a.configFieldIndex++
			}
		case "left", "right", "h", "l", " ":
			fwd := key == "right" || key == "l"
			if a.configFieldIndex < 4 {
				// Prompt style selection
				opts := []string{"p10k", "starship", "pure", "minimal"}
				a.deepDiveConfig.ZshPromptStyle = opts[a.configFieldIndex]
			} else if a.configFieldIndex == 4 {
				// History size
				opts := []string{"1000", "5000", "10000", "50000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.ZshHistorySize)
				a.deepDiveConfig.ZshHistorySize = atoi(cycleOption(opts, current, fwd), 10000)
			} else if a.configFieldIndex == 5 {
				// Auto CD
				a.deepDiveConfig.ZshAutoCD = !a.deepDiveConfig.ZshAutoCD
			} else if a.configFieldIndex == 6 {
				// Syntax highlighting
				a.deepDiveConfig.ZshSyntaxHighlight = !a.deepDiveConfig.ZshSyntaxHighlight
			} else if a.configFieldIndex == 7 {
				// Autosuggestions
				a.deepDiveConfig.ZshAutosuggestions = !a.deepDiveConfig.ZshAutosuggestions
			} else {
				// Plugin toggle
				plugins := []string{"zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions", "fzf-tab", "zsh-history-substring-search"}
				pluginIdx := a.configFieldIndex - 8
				if pluginIdx >= 0 && pluginIdx < len(plugins) {
					togglePlugin(&a.deepDiveConfig.ZshPlugins, plugins[pluginIdx])
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Neovim config
	// Fields: 0-3=configs, 4=tabwidth, 5=wrap, 6=cursorline, 7=clipboard, 8-13=LSPs
	case ScreenConfigNeovim:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 13 { // 4 configs + 4 editor settings + 6 LSPs - 1
				a.configFieldIndex++
			}
		case "left", "right", "h", "l", " ":
			fwd := key == "right" || key == "l"
			if a.configFieldIndex < 4 {
				// Config preset selection
				opts := []string{"kickstart", "lazyvim", "nvchad", "custom"}
				a.deepDiveConfig.NeovimConfig = opts[a.configFieldIndex]
			} else if a.configFieldIndex == 4 {
				// Tab width
				opts := []string{"2", "4", "8"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.NeovimTabWidth)
				a.deepDiveConfig.NeovimTabWidth = atoi(cycleOption(opts, current, fwd), 4)
			} else if a.configFieldIndex == 5 {
				// Wrap
				a.deepDiveConfig.NeovimWrap = !a.deepDiveConfig.NeovimWrap
			} else if a.configFieldIndex == 6 {
				// Cursor line
				a.deepDiveConfig.NeovimCursorLine = !a.deepDiveConfig.NeovimCursorLine
			} else if a.configFieldIndex == 7 {
				// Clipboard
				opts := []string{"unnamedplus", "unnamed", "none"}
				a.deepDiveConfig.NeovimClipboard = cycleOption(opts, a.deepDiveConfig.NeovimClipboard, fwd)
			} else {
				// LSP toggle
				lsps := []string{"lua_ls", "pyright", "tsserver", "gopls", "rust_analyzer", "clangd"}
				lspIdx := a.configFieldIndex - 8
				if lspIdx >= 0 && lspIdx < len(lsps) {
					togglePlugin(&a.deepDiveConfig.NeovimLSPs, lsps[lspIdx])
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Git config
	// Fields: 0=delta, 1=branch, 2=rebase, 3=sign, 4=credential
	case ScreenConfigGit:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 4 {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			fwd := key == "right" || key == "l"
			switch a.configFieldIndex {
			case 1: // Default branch
				opts := []string{"main", "master", "develop"}
				a.deepDiveConfig.GitDefaultBranch = cycleOption(opts, a.deepDiveConfig.GitDefaultBranch, fwd)
			case 4: // Credential helper
				opts := []string{"cache", "store", "osxkeychain", "none"}
				a.deepDiveConfig.GitCredentialHelper = cycleOption(opts, a.deepDiveConfig.GitCredentialHelper, fwd)
			}
		case " ":
			switch a.configFieldIndex {
			case 0: // Delta side-by-side
				a.deepDiveConfig.GitDeltaSideBySide = !a.deepDiveConfig.GitDeltaSideBySide
			case 2: // Pull rebase
				a.deepDiveConfig.GitPullRebase = !a.deepDiveConfig.GitPullRebase
			case 3: // Sign commits
				a.deepDiveConfig.GitSignCommits = !a.deepDiveConfig.GitSignCommits
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Yazi config
	case ScreenConfigYazi:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			switch a.configFieldIndex {
			case 0:
				if a.deepDiveConfig.YaziKeymap == "vim" {
					a.deepDiveConfig.YaziKeymap = "emacs"
				} else {
					a.deepDiveConfig.YaziKeymap = "vim"
				}
			case 2:
				opts := []string{"auto", "always", "never"}
				a.deepDiveConfig.YaziPreviewMode = cycleOption(opts, a.deepDiveConfig.YaziPreviewMode, key == "right" || key == "l")
			}
		case " ":
			if a.configFieldIndex == 1 {
				a.deepDiveConfig.YaziShowHidden = !a.deepDiveConfig.YaziShowHidden
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// FZF config
	case ScreenConfigFzf:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 1:
				if a.deepDiveConfig.FzfHeight > 20 {
					a.deepDiveConfig.FzfHeight -= 10
				}
			case 2:
				opts := []string{"reverse", "default", "reverse-list"}
				a.deepDiveConfig.FzfLayout = cycleOption(opts, a.deepDiveConfig.FzfLayout, false)
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 1:
				if a.deepDiveConfig.FzfHeight < 100 {
					a.deepDiveConfig.FzfHeight += 10
				}
			case 2:
				opts := []string{"reverse", "default", "reverse-list"}
				a.deepDiveConfig.FzfLayout = cycleOption(opts, a.deepDiveConfig.FzfLayout, true)
			}
		case " ":
			if a.configFieldIndex == 0 {
				a.deepDiveConfig.FzfPreview = !a.deepDiveConfig.FzfPreview
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// macOS Apps config
	case ScreenConfigMacApps:
		apps := []string{"rectangle", "raycast", "stats", "alt-tab", "monitor-control", "mos", "karabiner", "iina", "the-unarchiver", "appcleaner"}
		switch key {
		case "up", "k":
			if a.macAppIndex > 0 {
				a.macAppIndex--
			}
		case "down", "j":
			if a.macAppIndex < len(apps)-1 {
				a.macAppIndex++
			}
		case " ":
			app := apps[a.macAppIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[app] {
				a.deepDiveConfig.MacApps[app] = !a.deepDiveConfig.MacApps[app]
			}
		case "esc", "enter":
			a.macAppIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Utilities config
	case ScreenConfigUtilities:
		utilities := []string{"hk", "caff", "sshh"}
		switch key {
		case "up", "k":
			if a.utilityIndex > 0 {
				a.utilityIndex--
			}
		case "down", "j":
			if a.utilityIndex < len(utilities)-1 {
				a.utilityIndex++
			}
		case " ":
			util := utilities[a.utilityIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[util] {
				a.deepDiveConfig.Utilities[util] = !a.deepDiveConfig.Utilities[util]
			}
		case "esc", "enter":
			a.utilityIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// CLI Tools config
	case ScreenConfigCLITools:
		tools := []string{"lazygit", "lazydocker", "btop", "glow"}
		switch key {
		case "up", "k":
			if a.cliToolIndex > 0 {
				a.cliToolIndex--
			}
		case "down", "j":
			if a.cliToolIndex < len(tools)-1 {
				a.cliToolIndex++
			}
		case " ":
			tool := tools[a.cliToolIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[tool] {
				a.deepDiveConfig.CLITools[tool] = !a.deepDiveConfig.CLITools[tool]
			}
		case "esc", "enter":
			a.cliToolIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// GUI Apps config
	case ScreenConfigGUIApps:
		apps := []string{"zen-browser", "cursor", "lm-studio", "obs"}
		switch key {
		case "up", "k":
			if a.guiAppIndex > 0 {
				a.guiAppIndex--
			}
		case "down", "j":
			if a.guiAppIndex < len(apps)-1 {
				a.guiAppIndex++
			}
		case " ":
			app := apps[a.guiAppIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[app] {
				a.deepDiveConfig.GUIApps[app] = !a.deepDiveConfig.GUIApps[app]
			}
		case "esc", "enter":
			a.guiAppIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// CLI Utilities config (bat, eza, zoxide, ripgrep, fd, delta, fswatch)
	case ScreenConfigCLIUtilities:
		utilities := []string{"bat", "eza", "zoxide", "ripgrep", "fd", "delta", "fswatch"}
		switch key {
		case "up", "k":
			if a.cliUtilityIndex > 0 {
				a.cliUtilityIndex--
			}
		case "down", "j":
			if a.cliUtilityIndex < len(utilities)-1 {
				a.cliUtilityIndex++
			}
		case " ":
			util := utilities[a.cliUtilityIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[util] {
				a.deepDiveConfig.CLIUtilities[util] = !a.deepDiveConfig.CLIUtilities[util]
			}
		case "esc", "enter":
			a.cliUtilityIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// LazyGit config
	case ScreenConfigLazyGit:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			if a.configFieldIndex == 2 {
				opts := []string{"auto", "dark", "light"}
				a.deepDiveConfig.LazyGitTheme = cycleOption(opts, a.deepDiveConfig.LazyGitTheme, key == "right" || key == "l")
			}
		case " ":
			switch a.configFieldIndex {
			case 0:
				a.deepDiveConfig.LazyGitSideBySide = !a.deepDiveConfig.LazyGitSideBySide
			case 1:
				a.deepDiveConfig.LazyGitMouseMode = !a.deepDiveConfig.LazyGitMouseMode
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// LazyDocker config
	case ScreenConfigLazyDocker:
		switch key {
		case " ":
			a.deepDiveConfig.LazyDockerMouseMode = !a.deepDiveConfig.LazyDockerMouseMode
		case "esc", "enter":
			a.screen = ScreenDeepDiveMenu
		}

	// Btop config
	case ScreenConfigBtop:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 3 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}
				a.deepDiveConfig.BtopTheme = cycleOption(opts, a.deepDiveConfig.BtopTheme, false)
			case 1:
				if a.deepDiveConfig.BtopUpdateMs > 500 {
					a.deepDiveConfig.BtopUpdateMs -= 500
				}
			case 3:
				opts := []string{"braille", "block", "tty"}
				a.deepDiveConfig.BtopGraphType = cycleOption(opts, a.deepDiveConfig.BtopGraphType, false)
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}
				a.deepDiveConfig.BtopTheme = cycleOption(opts, a.deepDiveConfig.BtopTheme, true)
			case 1:
				if a.deepDiveConfig.BtopUpdateMs < 10000 {
					a.deepDiveConfig.BtopUpdateMs += 500
				}
			case 3:
				opts := []string{"braille", "block", "tty"}
				a.deepDiveConfig.BtopGraphType = cycleOption(opts, a.deepDiveConfig.BtopGraphType, true)
			}
		case " ":
			if a.configFieldIndex == 2 {
				a.deepDiveConfig.BtopShowTemp = !a.deepDiveConfig.BtopShowTemp
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Glow config
	case ScreenConfigGlow:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dark", "light", "notty"}
				a.deepDiveConfig.GlowStyle = cycleOption(opts, a.deepDiveConfig.GlowStyle, false)
			case 1:
				opts := []string{"auto", "less", "more", "none"}
				a.deepDiveConfig.GlowPager = cycleOption(opts, a.deepDiveConfig.GlowPager, false)
			case 2:
				if a.deepDiveConfig.GlowWidth > 40 {
					a.deepDiveConfig.GlowWidth -= 10
				}
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dark", "light", "notty"}
				a.deepDiveConfig.GlowStyle = cycleOption(opts, a.deepDiveConfig.GlowStyle, true)
			case 1:
				opts := []string{"auto", "less", "more", "none"}
				a.deepDiveConfig.GlowPager = cycleOption(opts, a.deepDiveConfig.GlowPager, true)
			case 2:
				if a.deepDiveConfig.GlowWidth < 200 {
					a.deepDiveConfig.GlowWidth += 10
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Claude Code config (MCP servers)
	case ScreenConfigClaudeCode:
		mcps := []string{"context7", "task-master", "github", "supabase", "convex", "puppeteer", "sequential-thinking"}
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < len(mcps)-1 {
				a.configFieldIndex++
			}
		case " ":
			mcp := mcps[a.configFieldIndex]
			a.deepDiveConfig.ClaudeCodeMCPs[mcp] = !a.deepDiveConfig.ClaudeCodeMCPs[mcp]
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}
	}

	return a, nil
}
