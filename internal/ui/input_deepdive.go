package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleDeepDiveKey handles key events for the deep dive config screens that
// have NOT yet been migrated to ScreenHandlers (CLITools, GUIApps, CLIUtilities,
// LazyGit, LazyDocker, Btop, Glow, ClaudeCode). The migrated screens
// (ScreenDeepDiveMenu and ScreenConfig{Ghostty,Tmux,Zsh,Neovim,Git,Yazi,Fzf,
// MacApps,Utilities}) are handled by their own ScreenHandlers via the manager.
func (a *App) handleDeepDiveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch a.screen {
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
			if a.cliToolIndex >= 0 && a.cliToolIndex < len(tools) {
				tool := tools[a.cliToolIndex]
				// Don't allow toggling if already installed
				if !a.manageInstalled[tool] {
					a.deepDiveConfig.CLITools[tool] = !a.deepDiveConfig.CLITools[tool]
				}
			}
		case "esc", "enter":
			a.cliToolIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// GUI Apps config
	case ScreenConfigGUIApps:
		apps := []string{"zen-browser", "cursor", "sunshine", "moonlight", "lm-studio", "obs"}
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
			if a.guiAppIndex >= 0 && a.guiAppIndex < len(apps) {
				app := apps[a.guiAppIndex]
				// Don't allow toggling if already installed
				if !a.manageInstalled[app] {
					a.deepDiveConfig.GUIApps[app] = !a.deepDiveConfig.GUIApps[app]
				}
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
			if a.cliUtilityIndex >= 0 && a.cliUtilityIndex < len(utilities) {
				util := utilities[a.cliUtilityIndex]
				// Don't allow toggling if already installed
				if !a.manageInstalled[util] {
					a.deepDiveConfig.CLIUtilities[util] = !a.deepDiveConfig.CLIUtilities[util]
				}
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
			// Allow navigating to -1 for the install toggle
			if a.configFieldIndex > -1 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < len(mcps)-1 {
				a.configFieldIndex++
			}
		case " ":
			if a.configFieldIndex == -1 {
				// Toggle Claude Code installation
				current := a.deepDiveConfig.CLITools["claude-code"]
				a.deepDiveConfig.CLITools["claude-code"] = !current
			} else if a.configFieldIndex >= 0 && a.configFieldIndex < len(mcps) {
				// Toggle MCP server
				mcp := mcps[a.configFieldIndex]
				a.deepDiveConfig.ClaudeCodeMCPs[mcp] = !a.deepDiveConfig.ClaudeCodeMCPs[mcp]
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}
	}

	return a, nil
}
