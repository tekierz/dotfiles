package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderDeepDiveMenu renders the deep dive tool selection menu
func (a *App) renderDeepDiveMenu() string {
	title := TitleStyle.Render("Deep Dive Configuration")
	subtitle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Customize individual tool settings")

	items := GetDeepDiveMenuItems()
	var menuList strings.Builder

	for i, item := range items {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

		if i == a.deepDiveMenuIndex {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		menuList.WriteString(style.Render(fmt.Sprintf("%s%s %-12s", prefix, item.Icon, item.Name)))
		menuList.WriteString(descStyle.Render(fmt.Sprintf("  %s", item.Description)))
		menuList.WriteString("\n")
	}

	// Add "Continue to Installation" option at the bottom
	continueIdx := len(items)
	continuePrefix := "  "
	continueStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if a.deepDiveMenuIndex == continueIdx {
		continuePrefix = "▶ "
		continueStyle = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	}
	menuList.WriteString("\n")
	menuList.WriteString(continueStyle.Render(fmt.Sprintf("%s→ Continue to Installation", continuePrefix)))

	help := HelpStyle.Render("[↑↓/jk] Navigate    [ENTER] Configure    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			subtitle,
			"",
			menuList.String(),
			"",
			help,
		)),
	)
}

// renderConfigGhostty renders the Ghostty configuration screen
func (a *App) renderConfigGhostty() string {
	title := TitleStyle.Render("󰆍 Ghostty Configuration")

	cfg := a.deepDiveConfig

	// Font size selector
	fontSizeLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Font Size:")
	fontSizeValue := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).
		Render(fmt.Sprintf(" %d ", cfg.GhosttyFontSize))
	fontSizeHint := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[←→] to adjust")

	// Opacity slider
	opacityLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Opacity:")
	opacityBar := renderSlider(cfg.GhosttyOpacity, 100, 20)
	opacityValue := lipgloss.NewStyle().Foreground(ColorCyan).Render(fmt.Sprintf(" %d%%", cfg.GhosttyOpacity))

	// Tab keybindings
	tabLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Tab Keybindings:")
	tabOptions := []string{"super", "ctrl", "alt"}
	var tabButtons strings.Builder
	for _, opt := range tabOptions {
		style := ButtonStyle
		if cfg.GhosttyTabBindings == opt {
			style = ButtonActiveStyle
		}
		tabButtons.WriteString(style.Render(fmt.Sprintf(" %s+N ", opt)))
		tabButtons.WriteString(" ")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		fontSizeLabel,
		lipgloss.JoinHorizontal(lipgloss.Center, fontSizeValue, fontSizeHint),
		"",
		opacityLabel,
		lipgloss.JoinHorizontal(lipgloss.Center, opacityBar, opacityValue),
		"",
		tabLabel,
		tabButtons.String(),
		"",
	)

	help := HelpStyle.Render("[←→] Adjust    [↑↓] Navigate    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderConfigTmux renders the Tmux configuration screen
func (a *App) renderConfigTmux() string {
	title := TitleStyle.Render(" Tmux Configuration")

	cfg := a.deepDiveConfig

	// Prefix key options
	prefixLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Prefix Key:")
	prefixOptions := []struct {
		value string
		label string
	}{
		{"ctrl-a", "Ctrl-A"},
		{"ctrl-b", "Ctrl-B (default)"},
		{"ctrl-space", "Ctrl-Space"},
	}
	var prefixButtons strings.Builder
	for _, opt := range prefixOptions {
		style := ButtonStyle
		if cfg.TmuxPrefix == opt.value {
			style = ButtonActiveStyle
		}
		prefixButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt.label)))
		prefixButtons.WriteString(" ")
	}

	// Split bindings
	splitLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Split Bindings:")
	splitOptions := []struct {
		value string
		label string
	}{
		{"pipes", "| and - (intuitive)"},
		{"percent", "% and \" (default)"},
	}
	var splitButtons strings.Builder
	for _, opt := range splitOptions {
		style := ButtonStyle
		if cfg.TmuxSplitBinds == opt.value {
			style = ButtonActiveStyle
		}
		splitButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt.label)))
		splitButtons.WriteString(" ")
	}

	// Status bar position
	statusLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Status Bar:")
	statusOptions := []string{"top", "bottom"}
	var statusButtons strings.Builder
	for _, opt := range statusOptions {
		style := ButtonStyle
		if cfg.TmuxStatusBar == opt {
			style = ButtonActiveStyle
		}
		statusButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt)))
		statusButtons.WriteString(" ")
	}

	// Mouse mode toggle
	mouseLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Mouse Mode:")
	mouseValue := "OFF"
	mouseStyle := lipgloss.NewStyle().Foreground(ColorRed)
	if cfg.TmuxMouseMode {
		mouseValue = "ON"
		mouseStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	}
	mouseToggle := lipgloss.JoinHorizontal(lipgloss.Center,
		ButtonStyle.Render(" "+mouseStyle.Render(mouseValue)+" "),
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  [SPACE] to toggle"),
	)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		prefixLabel,
		prefixButtons.String(),
		"",
		splitLabel,
		splitButtons.String(),
		"",
		statusLabel,
		statusButtons.String(),
		"",
		mouseLabel,
		mouseToggle,
	)

	help := HelpStyle.Render("[←→] Select    [SPACE] Toggle    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderConfigZsh renders the Zsh configuration screen
func (a *App) renderConfigZsh() string {
	title := TitleStyle.Render(" Zsh Configuration")

	cfg := a.deepDiveConfig

	// Prompt style
	promptLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Prompt Style:")
	promptOptions := []struct {
		value string
		label string
	}{
		{"p10k", "Powerlevel10k (recommended)"},
		{"starship", "Starship"},
		{"pure", "Pure"},
		{"minimal", "Minimal"},
	}
	var promptButtons strings.Builder
	for _, opt := range promptOptions {
		style := ButtonStyle
		if cfg.ZshPromptStyle == opt.value {
			style = ButtonActiveStyle
		}
		promptButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt.label)))
		promptButtons.WriteString("\n")
	}

	// Plugins checkboxes
	pluginsLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Plugins:")
	availablePlugins := []string{
		"zsh-autosuggestions",
		"zsh-syntax-highlighting",
		"zsh-completions",
		"fzf-tab",
		"zsh-history-substring-search",
	}
	var pluginsList strings.Builder
	for _, plugin := range availablePlugins {
		checked := "[ ]"
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		for _, p := range cfg.ZshPlugins {
			if p == plugin {
				checked = "[✓]"
				style = lipgloss.NewStyle().Foreground(ColorGreen)
				break
			}
		}
		pluginsList.WriteString(style.Render(fmt.Sprintf("  %s %s\n", checked, plugin)))
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		promptLabel,
		promptButtons.String(),
		"",
		pluginsLabel,
		pluginsList.String(),
	)

	help := HelpStyle.Render("[↑↓] Navigate    [SPACE] Toggle    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderConfigNeovim renders the Neovim configuration screen
func (a *App) renderConfigNeovim() string {
	title := TitleStyle.Render(" Neovim Configuration")

	cfg := a.deepDiveConfig

	// Config preset
	configLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Configuration:")
	configOptions := []struct {
		value string
		label string
		desc  string
	}{
		{"kickstart", "Kickstart.nvim", "Minimal, well-documented starting point"},
		{"lazyvim", "LazyVim", "Full-featured, pre-configured IDE"},
		{"nvchad", "NvChad", "Beautiful, fast, extensible"},
		{"custom", "Custom", "Use your own config"},
	}
	var configList strings.Builder
	for _, opt := range configOptions {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if cfg.NeovimConfig == opt.value {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		}
		configList.WriteString(style.Render(fmt.Sprintf("%s%-15s", prefix, opt.label)))
		configList.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render(opt.desc))
		configList.WriteString("\n")
	}

	// LSP servers
	lspLabel := lipgloss.NewStyle().Foreground(ColorText).Render("LSP Servers:")
	availableLSPs := []struct {
		id   string
		name string
	}{
		{"lua_ls", "Lua"},
		{"pyright", "Python"},
		{"tsserver", "TypeScript/JavaScript"},
		{"gopls", "Go"},
		{"rust_analyzer", "Rust"},
		{"clangd", "C/C++"},
	}
	var lspList strings.Builder
	for _, lsp := range availableLSPs {
		checked := "[ ]"
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		for _, l := range cfg.NeovimLSPs {
			if l == lsp.id {
				checked = "[✓]"
				style = lipgloss.NewStyle().Foreground(ColorGreen)
				break
			}
		}
		lspList.WriteString(style.Render(fmt.Sprintf("  %s %-20s", checked, lsp.name)))
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		configLabel,
		configList.String(),
		"",
		lspLabel,
		lspList.String(),
	)

	help := HelpStyle.Render("[↑↓] Navigate    [SPACE] Toggle    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderConfigGit renders the Git configuration screen
func (a *App) renderConfigGit() string {
	title := TitleStyle.Render(" Git Configuration")

	cfg := a.deepDiveConfig

	// Delta side-by-side toggle
	deltaLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Delta Diff View:")
	deltaValue := "Line-by-line"
	deltaStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if cfg.GitDeltaSideBySide {
		deltaValue = "Side-by-side"
		deltaStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	}
	deltaToggle := lipgloss.JoinHorizontal(lipgloss.Center,
		ButtonStyle.Render(" "+deltaStyle.Render(deltaValue)+" "),
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  [SPACE] to toggle"),
	)

	// Default branch
	branchLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Default Branch:")
	branchOptions := []string{"main", "master", "develop"}
	var branchButtons strings.Builder
	for _, opt := range branchOptions {
		style := ButtonStyle
		if cfg.GitDefaultBranch == opt {
			style = ButtonActiveStyle
		}
		branchButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt)))
		branchButtons.WriteString(" ")
	}

	// Aliases
	aliasLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Enable Aliases:")
	aliases := []struct {
		short string
		full  string
	}{
		{"st", "status"},
		{"co", "checkout"},
		{"br", "branch"},
		{"ci", "commit"},
		{"lg", "log --oneline --graph"},
		{"unstage", "reset HEAD --"},
	}
	var aliasList strings.Builder
	for _, alias := range aliases {
		checked := "[✓]"
		style := lipgloss.NewStyle().Foreground(ColorGreen)
		aliasList.WriteString(style.Render(fmt.Sprintf("  %s git %s → %s\n", checked, alias.short, alias.full)))
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		deltaLabel,
		deltaToggle,
		"",
		branchLabel,
		branchButtons.String(),
		"",
		aliasLabel,
		aliasList.String(),
	)

	help := HelpStyle.Render("[←→] Select    [SPACE] Toggle    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderConfigYazi renders the Yazi configuration screen
func (a *App) renderConfigYazi() string {
	title := TitleStyle.Render("󰉋 Yazi Configuration")

	cfg := a.deepDiveConfig

	// Keymap style
	keymapLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Keymap Style:")
	keymapOptions := []string{"vim", "emacs"}
	var keymapButtons strings.Builder
	for _, opt := range keymapOptions {
		style := ButtonStyle
		if cfg.YaziKeymap == opt {
			style = ButtonActiveStyle
		}
		keymapButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt)))
		keymapButtons.WriteString(" ")
	}

	// Show hidden files
	hiddenLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Show Hidden Files:")
	hiddenValue := "OFF"
	hiddenStyle := lipgloss.NewStyle().Foreground(ColorRed)
	if cfg.YaziShowHidden {
		hiddenValue = "ON"
		hiddenStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	}
	hiddenToggle := lipgloss.JoinHorizontal(lipgloss.Center,
		ButtonStyle.Render(" "+hiddenStyle.Render(hiddenValue)+" "),
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  [SPACE] to toggle"),
	)

	// Preview mode
	previewLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Preview Mode:")
	previewOptions := []string{"auto", "always", "never"}
	var previewButtons strings.Builder
	for _, opt := range previewOptions {
		style := ButtonStyle
		if cfg.YaziPreviewMode == opt {
			style = ButtonActiveStyle
		}
		previewButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt)))
		previewButtons.WriteString(" ")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		keymapLabel,
		keymapButtons.String(),
		"",
		hiddenLabel,
		hiddenToggle,
		"",
		previewLabel,
		previewButtons.String(),
	)

	help := HelpStyle.Render("[←→] Select    [SPACE] Toggle    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderConfigFzf renders the FZF configuration screen
func (a *App) renderConfigFzf() string {
	title := TitleStyle.Render(" FZF Configuration")

	cfg := a.deepDiveConfig

	// Preview toggle
	previewLabel := lipgloss.NewStyle().Foreground(ColorText).Render("File Preview:")
	previewValue := "OFF"
	previewStyle := lipgloss.NewStyle().Foreground(ColorRed)
	if cfg.FzfPreview {
		previewValue = "ON"
		previewStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	}
	previewToggle := lipgloss.JoinHorizontal(lipgloss.Center,
		ButtonStyle.Render(" "+previewStyle.Render(previewValue)+" "),
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  [SPACE] to toggle"),
	)

	// Height slider
	heightLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Height:")
	heightBar := renderSlider(cfg.FzfHeight, 100, 20)
	heightValue := lipgloss.NewStyle().Foreground(ColorCyan).Render(fmt.Sprintf(" %d%%", cfg.FzfHeight))

	// Layout
	layoutLabel := lipgloss.NewStyle().Foreground(ColorText).Render("Layout:")
	layoutOptions := []string{"reverse", "default", "reverse-list"}
	var layoutButtons strings.Builder
	for _, opt := range layoutOptions {
		style := ButtonStyle
		if cfg.FzfLayout == opt {
			style = ButtonActiveStyle
		}
		layoutButtons.WriteString(style.Render(fmt.Sprintf(" %s ", opt)))
		layoutButtons.WriteString(" ")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		previewLabel,
		previewToggle,
		"",
		heightLabel,
		lipgloss.JoinHorizontal(lipgloss.Center, heightBar, heightValue),
		"",
		layoutLabel,
		layoutButtons.String(),
	)

	help := HelpStyle.Render("[←→] Adjust    [SPACE] Toggle    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderConfigMacApps renders the macOS apps selection screen
func (a *App) renderConfigMacApps() string {
	title := TitleStyle.Render(" macOS Applications")
	subtitle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Select optional productivity applications to install")

	cfg := a.deepDiveConfig

	apps := []struct {
		id   string
		name string
		desc string
	}{
		{"rectangle", "Rectangle", "Window management with keyboard shortcuts"},
		{"raycast", "Raycast", "Spotlight replacement with extensions"},
		{"stats", "Stats", "System monitor in menu bar"},
		{"alt-tab", "AltTab", "Windows-style alt-tab switcher"},
		{"monitor-control", "MonitorControl", "External monitor brightness control"},
		{"mos", "Mos", "Smooth scrolling for external mice"},
		{"karabiner", "Karabiner-Elements", "Keyboard customization"},
		{"iina", "IINA", "Modern media player"},
		{"the-unarchiver", "The Unarchiver", "Archive extraction utility"},
		{"appcleaner", "AppCleaner", "Application uninstaller"},
	}

	var appsList strings.Builder
	for _, app := range apps {
		checked := "[ ]"
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if enabled, ok := cfg.MacApps[app.id]; ok && enabled {
			checked = "[✓]"
			style = lipgloss.NewStyle().Foreground(ColorGreen)
		}
		appsList.WriteString(style.Render(fmt.Sprintf("  %s %-18s", checked, app.name)))
		appsList.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render(app.desc))
		appsList.WriteString("\n")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		subtitle,
		"",
		appsList.String(),
	)

	help := HelpStyle.Render("[↑↓] Navigate    [SPACE] Toggle    [ENTER] Save    [ESC] Back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			content,
			"",
			help,
		)),
	)
}

// renderSlider creates a simple slider visualization
func renderSlider(value, max, width int) string {
	filled := (value * width) / max
	if filled > width {
		filled = width
	}
	empty := width - filled

	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", empty)

	return lipgloss.NewStyle().Foreground(ColorCyan).Render(filledStr) +
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render(emptyStr)
}
