package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Focused field styles
var (
	focusedStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	unfocusedStyle = lipgloss.NewStyle().
			Foreground(ColorTextMuted)

	sectionHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorMagenta).
				Bold(true).
				MarginTop(1)

	configBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2)

	activeOptionStyle = lipgloss.NewStyle().
				Background(ColorCyan).
				Foreground(lipgloss.Color("#000000")).
				Padding(0, 1)

	inactiveOptionStyle = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Padding(0, 1)
)

// renderDeepDiveMenu renders the deep dive tool selection menu
func (a *App) renderDeepDiveMenu() string {
	// Title with decorative border
	titleBox := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorMagenta).
		Padding(0, 2).
		Render(lipgloss.NewStyle().
			Foreground(ColorMagenta).
			Bold(true).
			Render("◈ DEEP DIVE CONFIGURATION ◈"))

	subtitle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("Customize each tool before installation")

	items := GetDeepDiveMenuItems()
	var menuList strings.Builder

	for i, item := range items {
		isSelected := i == a.deepDiveMenuIndex

		// Icon
		iconStyle := unfocusedStyle
		if isSelected {
			iconStyle = lipgloss.NewStyle().Foreground(ColorNeonPink)
		}

		// Name
		nameStyle := unfocusedStyle
		if isSelected {
			nameStyle = focusedStyle
		}

		// Description
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if isSelected {
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		// Cursor
		cursor := "  "
		if isSelected {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		}

		menuList.WriteString(fmt.Sprintf("%s%s %s  %s\n",
			cursor,
			iconStyle.Render(item.Icon),
			nameStyle.Render(fmt.Sprintf("%-12s", item.Name)),
			descStyle.Render(item.Description),
		))
	}

	// Continue option
	continueIdx := len(items)
	continueSelected := a.deepDiveMenuIndex == continueIdx
	continueCursor := "  "
	continueStyle := unfocusedStyle
	if continueSelected {
		continueCursor = lipgloss.NewStyle().Foreground(ColorGreen).Render("▸ ")
		continueStyle = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	}
	menuList.WriteString("\n")
	menuList.WriteString(fmt.Sprintf("%s%s\n", continueCursor, continueStyle.Render("▶ Continue to Installation")))

	// Wrap menu in a box
	menuBox := configBoxStyle.Render(menuList.String())

	help := HelpStyle.Render("↑↓/jk navigate • enter select • esc back")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		titleBox,
		subtitle,
		"",
		menuBox,
		"",
		help,
	)

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderConfigGhostty renders the Ghostty configuration screen
func (a *App) renderConfigGhostty() string {
	title := renderConfigTitle("󰆍", "Ghostty", "Terminal emulator settings")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Font size
	fontFocused := a.configFieldIndex == 0
	content.WriteString(renderFieldLabel("Font Size", fontFocused))
	content.WriteString(renderNumberControl(cfg.GhosttyFontSize, 8, 32, fontFocused))
	content.WriteString("\n\n")

	// Opacity
	opacityFocused := a.configFieldIndex == 1
	content.WriteString(renderFieldLabel("Background Opacity", opacityFocused))
	content.WriteString(renderSliderControl(cfg.GhosttyOpacity, 100, 24, opacityFocused))
	content.WriteString("\n\n")

	// Tab keybindings
	tabFocused := a.configFieldIndex == 2
	content.WriteString(renderFieldLabel("New Tab Keybinding", tabFocused))
	content.WriteString(renderOptionSelector(
		[]string{"super", "ctrl", "alt"},
		[]string{"⌘/Super+N", "Ctrl+N", "Alt+N"},
		cfg.GhosttyTabBindings,
		tabFocused,
	))

	box := configBoxStyle.Width(50).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • enter/esc save & back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigTmux renders the Tmux configuration screen
func (a *App) renderConfigTmux() string {
	title := renderConfigTitle("", "Tmux", "Terminal multiplexer settings")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Prefix key
	prefixFocused := a.configFieldIndex == 0
	content.WriteString(renderFieldLabel("Prefix Key", prefixFocused))
	content.WriteString(renderOptionSelector(
		[]string{"ctrl-a", "ctrl-b", "ctrl-space"},
		[]string{"Ctrl-A", "Ctrl-B", "Ctrl-Space"},
		cfg.TmuxPrefix,
		prefixFocused,
	))
	content.WriteString("\n\n")

	// Split bindings
	splitFocused := a.configFieldIndex == 1
	content.WriteString(renderFieldLabel("Split Pane Keys", splitFocused))
	content.WriteString(renderOptionSelector(
		[]string{"pipes", "percent"},
		[]string{"| and − (intuitive)", "% and \" (default)"},
		cfg.TmuxSplitBinds,
		splitFocused,
	))
	content.WriteString("\n\n")

	// Status bar
	statusFocused := a.configFieldIndex == 2
	content.WriteString(renderFieldLabel("Status Bar Position", statusFocused))
	content.WriteString(renderOptionSelector(
		[]string{"bottom", "top"},
		[]string{"Bottom", "Top"},
		cfg.TmuxStatusBar,
		statusFocused,
	))
	content.WriteString("\n\n")

	// Mouse mode
	mouseFocused := a.configFieldIndex == 3
	content.WriteString(renderFieldLabel("Mouse Support", mouseFocused))
	content.WriteString(renderToggle(cfg.TmuxMouseMode, mouseFocused))

	box := configBoxStyle.Width(50).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • enter/esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigZsh renders the Zsh configuration screen
func (a *App) renderConfigZsh() string {
	title := renderConfigTitle("", "Zsh", "Shell prompt and plugins")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Prompt style - radio buttons
	content.WriteString(sectionHeaderStyle.Render("Prompt Style"))
	content.WriteString("\n")
	prompts := []struct {
		value string
		label string
		desc  string
	}{
		{"p10k", "Powerlevel10k", "Feature-rich, customizable"},
		{"starship", "Starship", "Fast, minimal, cross-shell"},
		{"pure", "Pure", "Pretty, minimal, fast"},
		{"minimal", "Minimal", "Simple $ prompt"},
	}
	for i, p := range prompts {
		focused := a.configFieldIndex == i
		selected := cfg.ZshPromptStyle == p.value
		content.WriteString(renderRadioOption(p.label, p.desc, selected, focused))
		content.WriteString("\n")
	}

	// Plugins - checkboxes
	content.WriteString(sectionHeaderStyle.Render("Plugins"))
	content.WriteString("\n")
	plugins := []struct {
		id   string
		name string
	}{
		{"zsh-autosuggestions", "Auto-suggestions"},
		{"zsh-syntax-highlighting", "Syntax highlighting"},
		{"zsh-completions", "Extra completions"},
		{"fzf-tab", "FZF tab completion"},
		{"zsh-history-substring-search", "History search"},
	}
	for i, p := range plugins {
		focused := a.configFieldIndex == i+4
		enabled := false
		for _, ep := range cfg.ZshPlugins {
			if ep == p.id {
				enabled = true
				break
			}
		}
		content.WriteString(renderCheckbox(p.name, enabled, focused))
		content.WriteString("\n")
	}

	box := configBoxStyle.Width(50).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space/enter select • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigNeovim renders the Neovim configuration screen
func (a *App) renderConfigNeovim() string {
	title := renderConfigTitle("", "Neovim", "Editor configuration and LSP")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Config preset - radio buttons
	content.WriteString(sectionHeaderStyle.Render("Configuration"))
	content.WriteString("\n")
	configs := []struct {
		value string
		label string
		desc  string
	}{
		{"kickstart", "Kickstart.nvim", "Minimal, well-documented"},
		{"lazyvim", "LazyVim", "Full IDE experience"},
		{"nvchad", "NvChad", "Beautiful and fast"},
		{"custom", "Keep existing", "Don't modify config"},
	}
	for i, c := range configs {
		focused := a.configFieldIndex == i
		selected := cfg.NeovimConfig == c.value
		content.WriteString(renderRadioOption(c.label, c.desc, selected, focused))
		content.WriteString("\n")
	}

	// LSP servers - checkboxes
	content.WriteString(sectionHeaderStyle.Render("LSP Servers"))
	content.WriteString("\n")
	lsps := []struct {
		id   string
		name string
	}{
		{"lua_ls", "Lua"},
		{"pyright", "Python"},
		{"tsserver", "TypeScript/JS"},
		{"gopls", "Go"},
		{"rust_analyzer", "Rust"},
		{"clangd", "C/C++"},
	}
	for i, l := range lsps {
		focused := a.configFieldIndex == i+4
		enabled := false
		for _, el := range cfg.NeovimLSPs {
			if el == l.id {
				enabled = true
				break
			}
		}
		content.WriteString(renderCheckbox(l.name, enabled, focused))
		content.WriteString("\n")
	}

	box := configBoxStyle.Width(50).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space/enter select • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigGit renders the Git configuration screen
func (a *App) renderConfigGit() string {
	title := renderConfigTitle("", "Git", "Version control settings")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Delta side-by-side
	deltaFocused := a.configFieldIndex == 0
	content.WriteString(renderFieldLabel("Delta Diff View", deltaFocused))
	content.WriteString(renderToggleLabeled(cfg.GitDeltaSideBySide, "Side-by-side", "Unified", deltaFocused))
	content.WriteString("\n\n")

	// Default branch
	branchFocused := a.configFieldIndex == 1
	content.WriteString(renderFieldLabel("Default Branch", branchFocused))
	content.WriteString(renderOptionSelector(
		[]string{"main", "master", "develop"},
		[]string{"main", "master", "develop"},
		cfg.GitDefaultBranch,
		branchFocused,
	))
	content.WriteString("\n\n")

	// Aliases preview
	content.WriteString(sectionHeaderStyle.Render("Included Aliases"))
	content.WriteString("\n")
	aliases := []string{
		"git st → status",
		"git co → checkout",
		"git br → branch",
		"git ci → commit",
		"git lg → log --graph",
	}
	for _, alias := range aliases {
		content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  " + alias + "\n"))
	}

	box := configBoxStyle.Width(50).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigYazi renders the Yazi configuration screen
func (a *App) renderConfigYazi() string {
	title := renderConfigTitle("󰉋", "Yazi", "File manager settings")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Keymap
	keymapFocused := a.configFieldIndex == 0
	content.WriteString(renderFieldLabel("Keymap Style", keymapFocused))
	content.WriteString(renderOptionSelector(
		[]string{"vim", "emacs"},
		[]string{"Vim (hjkl)", "Emacs (arrows)"},
		cfg.YaziKeymap,
		keymapFocused,
	))
	content.WriteString("\n\n")

	// Show hidden
	hiddenFocused := a.configFieldIndex == 1
	content.WriteString(renderFieldLabel("Show Hidden Files", hiddenFocused))
	content.WriteString(renderToggle(cfg.YaziShowHidden, hiddenFocused))
	content.WriteString("\n\n")

	// Preview mode
	previewFocused := a.configFieldIndex == 2
	content.WriteString(renderFieldLabel("File Preview", previewFocused))
	content.WriteString(renderOptionSelector(
		[]string{"auto", "always", "never"},
		[]string{"Auto", "Always", "Never"},
		cfg.YaziPreviewMode,
		previewFocused,
	))

	box := configBoxStyle.Width(50).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigFzf renders the FZF configuration screen
func (a *App) renderConfigFzf() string {
	title := renderConfigTitle("", "FZF", "Fuzzy finder settings")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Preview toggle
	previewFocused := a.configFieldIndex == 0
	content.WriteString(renderFieldLabel("File Preview", previewFocused))
	content.WriteString(renderToggle(cfg.FzfPreview, previewFocused))
	content.WriteString("\n\n")

	// Height slider
	heightFocused := a.configFieldIndex == 1
	content.WriteString(renderFieldLabel("Window Height", heightFocused))
	content.WriteString(renderSliderControl(cfg.FzfHeight, 100, 24, heightFocused))
	content.WriteString("\n\n")

	// Layout
	layoutFocused := a.configFieldIndex == 2
	content.WriteString(renderFieldLabel("Layout", layoutFocused))
	content.WriteString(renderOptionSelector(
		[]string{"reverse", "default", "reverse-list"},
		[]string{"Reverse ↑", "Default ↓", "Reverse List"},
		cfg.FzfLayout,
		layoutFocused,
	))

	box := configBoxStyle.Width(50).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • space toggle • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigMacApps renders the macOS apps selection screen
func (a *App) renderConfigMacApps() string {
	title := renderConfigTitle("", "macOS Apps", "Optional productivity applications")

	cfg := a.deepDiveConfig
	var content strings.Builder

	apps := []struct {
		id   string
		name string
		desc string
	}{
		{"rectangle", "Rectangle", "Window management"},
		{"raycast", "Raycast", "Spotlight replacement"},
		{"stats", "Stats", "System monitor"},
		{"alt-tab", "AltTab", "Window switcher"},
		{"monitor-control", "MonitorControl", "Display brightness"},
		{"mos", "Mos", "Smooth scrolling"},
		{"karabiner", "Karabiner", "Keyboard customizer"},
		{"iina", "IINA", "Media player"},
		{"the-unarchiver", "The Unarchiver", "Archive utility"},
		{"appcleaner", "AppCleaner", "App uninstaller"},
	}

	for i, app := range apps {
		focused := a.macAppIndex == i
		enabled := cfg.MacApps[app.id]

		cursor := "  "
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		}

		checkbox := renderCheckboxInline(enabled, focused)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		content.WriteString(fmt.Sprintf("%s%s %s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-16s", app.name)),
			descStyle.Render(app.desc),
		))
	}

	box := configBoxStyle.Width(55).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • enter/esc save & back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// Helper render functions

func renderConfigTitle(icon, name, subtitle string) string {
	titleText := fmt.Sprintf("%s %s", icon, name)
	title := lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true).
		Render(titleText)

	sub := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render(subtitle)

	return lipgloss.JoinVertical(lipgloss.Center, title, sub)
}

func renderFieldLabel(label string, focused bool) string {
	style := unfocusedStyle
	if focused {
		style = focusedStyle
	}
	cursor := "  "
	if focused {
		cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
	}
	return cursor + style.Render(label) + "\n"
}

func renderNumberControl(value, min, max int, focused bool) string {
	leftArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("◀")
	rightArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("▶")
	if focused {
		leftArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("◀")
		rightArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("▶")
	}

	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	if focused {
		valueStyle = lipgloss.NewStyle().
			Background(ColorCyan).
			Foreground(lipgloss.Color("#000000")).
			Padding(0, 1)
	}

	return fmt.Sprintf("    %s %s %s", leftArrow, valueStyle.Render(fmt.Sprintf("%d", value)), rightArrow)
}

func renderSliderControl(value, max, width int, focused bool) string {
	filled := (value * width) / max
	if filled > width {
		filled = width
	}
	empty := width - filled

	fillColor := ColorTextMuted
	emptyColor := lipgloss.Color("#333333")
	if focused {
		fillColor = ColorCyan
	}

	filledStr := strings.Repeat("━", filled)
	emptyStr := strings.Repeat("─", empty)

	slider := lipgloss.NewStyle().Foreground(fillColor).Render(filledStr) +
		lipgloss.NewStyle().Foreground(emptyColor).Render(emptyStr)

	valueStr := fmt.Sprintf(" %d%%", value)
	if focused {
		valueStr = lipgloss.NewStyle().Foreground(ColorCyan).Render(valueStr)
	} else {
		valueStr = lipgloss.NewStyle().Foreground(ColorTextMuted).Render(valueStr)
	}

	return "    " + slider + valueStr
}

func renderOptionSelector(values, labels []string, selected string, focused bool) string {
	var parts []string
	for i, v := range values {
		label := labels[i]
		if v == selected {
			style := activeOptionStyle
			if !focused {
				style = lipgloss.NewStyle().
					Background(ColorTextMuted).
					Foreground(lipgloss.Color("#000000")).
					Padding(0, 1)
			}
			parts = append(parts, style.Render(label))
		} else {
			parts = append(parts, inactiveOptionStyle.Render(label))
		}
	}
	return "    " + strings.Join(parts, " ")
}

func renderToggle(value bool, focused bool) string {
	onStyle := inactiveOptionStyle
	offStyle := inactiveOptionStyle

	if value {
		onStyle = activeOptionStyle
		if !focused {
			onStyle = lipgloss.NewStyle().
				Background(ColorGreen).
				Foreground(lipgloss.Color("#000000")).
				Padding(0, 1)
		}
	} else {
		offStyle = lipgloss.NewStyle().
			Background(ColorRed).
			Foreground(lipgloss.Color("#ffffff")).
			Padding(0, 1)
		if !focused {
			offStyle = lipgloss.NewStyle().
				Background(ColorTextMuted).
				Foreground(lipgloss.Color("#000000")).
				Padding(0, 1)
		}
	}

	return "    " + offStyle.Render("OFF") + " " + onStyle.Render("ON")
}

func renderToggleLabeled(value bool, onLabel, offLabel string, focused bool) string {
	onStyle := inactiveOptionStyle
	offStyle := inactiveOptionStyle

	if value {
		onStyle = activeOptionStyle
		if !focused {
			onStyle = lipgloss.NewStyle().
				Background(ColorTextMuted).
				Foreground(lipgloss.Color("#000000")).
				Padding(0, 1)
		}
	} else {
		offStyle = activeOptionStyle
		if !focused {
			offStyle = lipgloss.NewStyle().
				Background(ColorTextMuted).
				Foreground(lipgloss.Color("#000000")).
				Padding(0, 1)
		}
	}

	return "    " + offStyle.Render(offLabel) + " " + onStyle.Render(onLabel)
}

func renderRadioOption(label, desc string, selected, focused bool) string {
	cursor := "  "
	if focused {
		cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
	}

	radio := "○"
	radioStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if selected {
		radio = "●"
		radioStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	}
	if focused {
		radioStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	}

	labelStyle := unfocusedStyle
	if focused {
		labelStyle = focusedStyle
	}

	descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

	return fmt.Sprintf("%s%s %s %s",
		cursor,
		radioStyle.Render(radio),
		labelStyle.Render(fmt.Sprintf("%-16s", label)),
		descStyle.Render(desc),
	)
}

func renderCheckbox(label string, checked, focused bool) string {
	cursor := "  "
	if focused {
		cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
	}

	box := "☐"
	boxStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if checked {
		box = "☑"
		boxStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	}
	if focused {
		boxStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	}

	labelStyle := unfocusedStyle
	if focused {
		labelStyle = focusedStyle
	}

	return fmt.Sprintf("%s%s %s", cursor, boxStyle.Render(box), labelStyle.Render(label))
}

func renderCheckboxInline(checked, focused bool) string {
	box := "☐"
	boxStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if checked {
		box = "☑"
		boxStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	}
	if focused {
		boxStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	}
	return boxStyle.Render(box)
}

// renderConfigUtilities renders the utilities selection screen
func (a *App) renderConfigUtilities() string {
	title := renderConfigTitle("", "Utilities", "Helper tools from tekierz/homebrew-tap")

	cfg := a.deepDiveConfig
	var content strings.Builder

	utilities := []struct {
		id   string
		name string
		desc string
	}{
		{"hk", "hk", "SSH host key manager"},
		{"caff", "caff", "Keep system awake utility"},
		{"sshh", "sshh", "SSH config helper"},
	}

	content.WriteString(lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("Select which utilities to install:\n\n"))

	for i, util := range utilities {
		focused := a.utilityIndex == i
		enabled := cfg.Utilities[util.id]

		cursor := "  "
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		}

		checkbox := renderCheckboxInline(enabled, focused)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		content.WriteString(fmt.Sprintf("%s%s %s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-8s", util.name)),
			descStyle.Render(util.desc),
		))
	}

	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("These tools are installed from the tekierz/homebrew-tap."))

	box := configBoxStyle.Width(55).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • enter/esc save & back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
