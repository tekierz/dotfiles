package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/pkg"
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
				Foreground(ColorBg).
				Padding(0, 1)

	inactiveOptionStyle = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Padding(0, 1)
)

// GetFilteredDeepDiveMenuItems returns deep dive menu items filtered for the current platform
func GetFilteredDeepDiveMenuItems() []DeepDiveMenuItem {
	platform := pkg.DetectPlatform()
	allItems := GetDeepDiveMenuItems()

	var filtered []DeepDiveMenuItem
	for _, item := range allItems {
		// If item has no platform restriction, include it
		if item.Platform == "" {
			filtered = append(filtered, item)
			continue
		}
		// If item is for macos, only include on macOS
		if item.Platform == "macos" && platform == pkg.PlatformMacOS {
			filtered = append(filtered, item)
			continue
		}
		// If item is for linux, only include on Linux (arch or debian)
		if item.Platform == "linux" && (platform == pkg.PlatformArch || platform == pkg.PlatformDebian) {
			filtered = append(filtered, item)
			continue
		}
	}

	return filtered
}

// renderDeepDiveMenu renders the deep dive tool selection menu
func (a *App) renderDeepDiveMenu() string {
	// Show loading state if cache is being populated
	if a.installCacheLoading {
		spinner := AnimatedSpinnerDots(a.uiFrame)
		loadingStyle := lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)
		loadingText := loadingStyle.Render(fmt.Sprintf("%s Loading installation status...", spinner))

		return lipgloss.Place(
			a.width, a.height,
			lipgloss.Center, lipgloss.Center,
			loadingText,
		)
	}

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

	items := GetFilteredDeepDiveMenuItems()
	var menuList strings.Builder

	// Category header style
	categoryStyle := lipgloss.NewStyle().
		Foreground(ColorMagenta).
		Bold(true).
		MarginTop(1)

	for i, item := range items {
		// Render category header if this item starts a new category
		if item.Category != "" {
			if i > 0 {
				menuList.WriteString("\n")
			}
			menuList.WriteString(categoryStyle.Render("  "+item.Category) + "\n")
		}

		isSelected := i == a.deepDiveMenuIndex

		// Get install status for this item
		installStatus := a.getDeepDiveItemStatus(item)
		statusDot := StatusDot(installStatus)

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

		menuList.WriteString(fmt.Sprintf("%s%s %s %s  %s\n",
			cursor,
			statusDot,
			iconStyle.Render(item.Icon),
			nameStyle.Render(fmt.Sprintf("%-14s", item.Name)),
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
	menuBox := configBoxStyle.Width(a.deepDiveBoxWidth(64)).Render(menuList.String())

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

	return PlaceWithBackground(a.width, a.height, content)
}

// renderConfigGhostty renders the Ghostty configuration screen
func (a *App) renderConfigGhostty() string {
	title := renderConfigTitle("󰆍", "Ghostty", "Terminal emulator settings")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

	// Font family
	fontFamilyFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Font Family", fontFamilyFocused))
	content.WriteString(renderOptionSelector(
		[]string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"},
		[]string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"},
		cfg.GhosttyFontFamily,
		fontFamilyFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Font size
	fontFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Font Size", fontFocused))
	content.WriteString(renderNumberControl(cfg.GhosttyFontSize, 8, 32, fontFocused))
	content.WriteString("\n\n")
	fieldIdx++

	// Opacity
	opacityFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Background Opacity", opacityFocused))
	content.WriteString(renderSliderControl(cfg.GhosttyOpacity, 100, 24, opacityFocused))
	content.WriteString("\n\n")
	fieldIdx++

	// Blur radius
	blurFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Blur Radius", blurFocused))
	content.WriteString(renderSliderControl(cfg.GhosttyBlurRadius, 100, 24, blurFocused))
	content.WriteString("\n\n")
	fieldIdx++

	// Scrollback lines
	scrollFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Scrollback Lines", scrollFocused))
	content.WriteString(renderOptionSelector(
		[]string{"1000", "5000", "10000", "50000", "100000"},
		[]string{"1K", "5K", "10K", "50K", "100K"},
		fmt.Sprintf("%d", cfg.GhosttyScrollbackLines),
		scrollFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Cursor style
	cursorFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Cursor Style", cursorFocused))
	content.WriteString(renderOptionSelector(
		[]string{"block", "bar", "underline"},
		[]string{"█ Block", "│ Bar", "_ Underline"},
		cfg.GhosttyCursorStyle,
		cursorFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Tab keybindings
	tabFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("New Tab Keybinding", tabFocused))
	content.WriteString(renderOptionSelector(
		[]string{"super", "ctrl", "alt"},
		[]string{"⌘/Super+N", "Ctrl+N", "Alt+N"},
		cfg.GhosttyTabBindings,
		tabFocused,
	))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • enter/esc save & back")

	return PlaceWithBackground(
		a.width, a.height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigTmux renders the Tmux configuration screen
func (a *App) renderConfigTmux() string {
	title := renderConfigTitle("", "Tmux", "Terminal multiplexer settings")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

	// Prefix key
	prefixFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Prefix Key", prefixFocused))
	content.WriteString(renderOptionSelector(
		[]string{"ctrl-a", "ctrl-b", "ctrl-space"},
		[]string{"Ctrl-A", "Ctrl-B", "Ctrl-Space"},
		cfg.TmuxPrefix,
		prefixFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Split bindings
	splitFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Split Pane Keys", splitFocused))
	content.WriteString(renderOptionSelector(
		[]string{"pipes", "percent"},
		[]string{"| and − (intuitive)", "% and \" (default)"},
		cfg.TmuxSplitBinds,
		splitFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Status bar
	statusFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Status Bar Position", statusFocused))
	content.WriteString(renderOptionSelector(
		[]string{"bottom", "top"},
		[]string{"Bottom", "Top"},
		cfg.TmuxStatusBar,
		statusFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Mouse mode
	mouseFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Mouse Support", mouseFocused))
	content.WriteString(renderToggle(cfg.TmuxMouseMode, mouseFocused))
	content.WriteString("\n\n")
	fieldIdx++

	// History limit
	historyFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("History Limit", historyFocused))
	content.WriteString(renderOptionSelector(
		[]string{"10000", "25000", "50000", "100000"},
		[]string{"10K", "25K", "50K", "100K"},
		fmt.Sprintf("%d", cfg.TmuxHistoryLimit),
		historyFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Escape time
	escapeFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Escape Time (ms)", escapeFocused))
	content.WriteString(renderOptionSelector(
		[]string{"0", "10", "50", "100"},
		[]string{"0ms", "10ms", "50ms", "100ms"},
		fmt.Sprintf("%d", cfg.TmuxEscapeTime),
		escapeFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Base index
	baseFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Base Index", baseFocused))
	content.WriteString(renderOptionSelector(
		[]string{"0", "1"},
		[]string{"0 (default)", "1 (starts at 1)"},
		fmt.Sprintf("%d", cfg.TmuxBaseIndex),
		baseFocused,
	))
	fieldIdx++

	// TPM section
	content.WriteString("\n\n")
	content.WriteString(sectionHeaderStyle.Render("TPM Plugins"))
	content.WriteString("\n\n")

	// TPM Enable toggle
	tpmFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Enable TPM", tpmFocused))
	content.WriteString(renderToggle(cfg.TmuxTPMEnabled, tpmFocused))
	fieldIdx++

	// Plugin checkboxes - only if TPM enabled
	if cfg.TmuxTPMEnabled {
		content.WriteString("\n\n")

		// tmux-sensible
		sensibleFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-sensible", cfg.TmuxPluginSensible, sensibleFocused))
		if sensibleFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Sensible defaults"))
		}
		content.WriteString("\n")
		fieldIdx++

		// tmux-resurrect
		resurrectFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-resurrect", cfg.TmuxPluginResurrect, resurrectFocused))
		if resurrectFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Session save/restore"))
		}
		content.WriteString("\n")
		fieldIdx++

		// tmux-continuum
		continuumFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-continuum", cfg.TmuxPluginContinuum, continuumFocused))
		if continuumFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Auto-save sessions"))
		}
		content.WriteString("\n")
		fieldIdx++

		// tmux-yank
		yankFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-yank", cfg.TmuxPluginYank, yankFocused))
		if yankFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Clipboard integration"))
		}
		fieldIdx++

		// Continuum interval - only if continuum enabled
		if cfg.TmuxPluginContinuum {
			content.WriteString("\n\n")
			intervalFocused := a.configFieldIndex == fieldIdx
			content.WriteString(renderFieldLabel("Auto-save Interval", intervalFocused))
			content.WriteString(renderNumberControl(cfg.TmuxContinuumSaveMin, 5, 60, intervalFocused))
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" min"))
		}
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • enter/esc back")

	return PlaceWithBackground(
		a.width, a.height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigZsh renders the Zsh configuration screen
func (a *App) renderConfigZsh() string {
	title := renderConfigTitle("", "Zsh", "Shell prompt and plugins")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

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
	for _, p := range prompts {
		focused := a.configFieldIndex == fieldIdx
		selected := cfg.ZshPromptStyle == p.value
		content.WriteString(renderRadioOption(p.label, p.desc, selected, focused))
		content.WriteString("\n")
		fieldIdx++
	}

	// Shell options section
	content.WriteString(sectionHeaderStyle.Render("Shell Options"))
	content.WriteString("\n")

	// History size
	historyFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("History Size", historyFocused))
	content.WriteString(renderOptionSelector(
		[]string{"1000", "5000", "10000", "50000"},
		[]string{"1K", "5K", "10K", "50K"},
		fmt.Sprintf("%d", cfg.ZshHistorySize),
		historyFocused,
	))
	content.WriteString("\n")
	fieldIdx++

	// Auto CD
	autoCDFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Auto CD (cd into directories)", cfg.ZshAutoCD, autoCDFocused))
	content.WriteString("\n")
	fieldIdx++

	// Syntax highlighting
	syntaxFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Syntax Highlighting", cfg.ZshSyntaxHighlight, syntaxFocused))
	content.WriteString("\n")
	fieldIdx++

	// Autosuggestions
	suggestFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Auto-suggestions", cfg.ZshAutosuggestions, suggestFocused))
	content.WriteString("\n")
	fieldIdx++

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
	for _, p := range plugins {
		focused := a.configFieldIndex == fieldIdx
		enabled := false
		for _, ep := range cfg.ZshPlugins {
			if ep == p.id {
				enabled = true
				break
			}
		}
		content.WriteString(renderCheckbox(p.name, enabled, focused))
		content.WriteString("\n")
		fieldIdx++
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space/enter select • esc back")

	return PlaceWithBackground(
		a.width, a.height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigNeovim renders the Neovim configuration screen
func (a *App) renderConfigNeovim() string {
	title := renderConfigTitle("", "Neovim", "Editor configuration and LSP")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

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
	for _, c := range configs {
		focused := a.configFieldIndex == fieldIdx
		selected := cfg.NeovimConfig == c.value
		content.WriteString(renderRadioOption(c.label, c.desc, selected, focused))
		content.WriteString("\n")
		fieldIdx++
	}

	// Editor settings section
	content.WriteString(sectionHeaderStyle.Render("Editor Settings"))
	content.WriteString("\n")

	// Tab width
	tabFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Tab Width", tabFocused))
	content.WriteString(renderOptionSelector(
		[]string{"2", "4", "8"},
		[]string{"2", "4", "8"},
		fmt.Sprintf("%d", cfg.NeovimTabWidth),
		tabFocused,
	))
	content.WriteString("\n")
	fieldIdx++

	// Line wrap
	wrapFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Line Wrapping", cfg.NeovimWrap, wrapFocused))
	content.WriteString("\n")
	fieldIdx++

	// Cursor line
	cursorFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Highlight Cursor Line", cfg.NeovimCursorLine, cursorFocused))
	content.WriteString("\n")
	fieldIdx++

	// Clipboard
	clipboardFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Clipboard", clipboardFocused))
	content.WriteString(renderOptionSelector(
		[]string{"unnamedplus", "unnamed", "none"},
		[]string{"System (+)", "Selection (*)", "None"},
		cfg.NeovimClipboard,
		clipboardFocused,
	))
	content.WriteString("\n")
	fieldIdx++

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
	for _, l := range lsps {
		focused := a.configFieldIndex == fieldIdx
		enabled := false
		for _, el := range cfg.NeovimLSPs {
			if el == l.id {
				enabled = true
				break
			}
		}
		content.WriteString(renderCheckbox(l.name, enabled, focused))
		content.WriteString("\n")
		fieldIdx++
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space/enter select • esc back")

	return PlaceWithBackground(
		a.width, a.height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigGit renders the Git configuration screen
func (a *App) renderConfigGit() string {
	title := renderConfigTitle("", "Git", "Version control settings")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

	// Delta side-by-side
	deltaFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Delta Diff View", deltaFocused))
	content.WriteString(renderToggleLabeled(cfg.GitDeltaSideBySide, "Side-by-side", "Unified", deltaFocused))
	content.WriteString("\n\n")
	fieldIdx++

	// Default branch
	branchFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Default Branch", branchFocused))
	content.WriteString(renderOptionSelector(
		[]string{"main", "master", "develop"},
		[]string{"main", "master", "develop"},
		cfg.GitDefaultBranch,
		branchFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	// Pull rebase
	rebaseFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Pull with Rebase", cfg.GitPullRebase, rebaseFocused))
	content.WriteString("\n")
	fieldIdx++

	// Sign commits
	signFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("GPG Sign Commits", cfg.GitSignCommits, signFocused))
	content.WriteString("\n\n")
	fieldIdx++

	// Credential helper
	credFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Credential Helper", credFocused))
	content.WriteString(renderOptionSelector(
		[]string{"cache", "store", "osxkeychain", "none"},
		[]string{"Cache (temp)", "Store (file)", "macOS Keychain", "None"},
		cfg.GitCredentialHelper,
		credFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

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

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")

	return PlaceWithBackground(
		a.width, a.height,
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

	box := configBoxStyle.Width(a.deepDiveBoxWidth(50)).Render(content.String())
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

	box := configBoxStyle.Width(a.deepDiveBoxWidth(50)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • space toggle • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigMacApps renders the macOS apps selection screen
func (a *App) renderConfigMacApps() string {
	// Ensure install status is cached
	a.ensureInstallCache()

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
		installed := a.manageInstalled[app.id]

		cursor := "  "
		if focused && !installed {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused && installed {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderCheckboxInlineWithInstallState(enabled, focused, installed)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			descStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		} else if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		suffix := ""
		if installed {
			suffix = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)")
		}

		content.WriteString(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-16s", app.name)),
			suffix,
			descStyle.Render(app.desc),
		))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(65)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • enter/esc save & back • yellow = installed")

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
			Foreground(ColorBg).
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
	emptyColor := ColorBorder
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
					Foreground(ColorBg).
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
				Foreground(ColorBg).
				Padding(0, 1)
		}
	} else {
		offStyle = lipgloss.NewStyle().
			Background(ColorRed).
			Foreground(ColorTextBright).
			Padding(0, 1)
		if !focused {
			offStyle = lipgloss.NewStyle().
				Background(ColorTextMuted).
				Foreground(ColorBg).
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
				Foreground(ColorBg).
				Padding(0, 1)
		}
	} else {
		offStyle = activeOptionStyle
		if !focused {
			offStyle = lipgloss.NewStyle().
				Background(ColorTextMuted).
				Foreground(ColorBg).
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

// renderCheckboxInlineWithInstallState renders a checkbox with install status awareness
// - installed: shows yellow checkbox (already installed, can update), item is not selectable
// - not installed: normal checkbox behavior (green when checked)
func renderCheckboxInlineWithInstallState(checked, focused, installed bool) string {
	if installed {
		// Already installed - show yellow filled checkbox, not selectable
		boxStyle := lipgloss.NewStyle().Foreground(ColorYellow)
		return boxStyle.Render("☑")
	}

	// Not installed - normal checkbox
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
	// Ensure install status is cached
	a.ensureInstallCache()

	title := renderConfigTitle("", "Utilities", "Helper tools from tekierz/homebrew-tap")

	cfg := a.deepDiveConfig
	var content strings.Builder

	utilities := []struct {
		id   string
		name string
		desc string
	}{
		{"hk", "hk", "Hotkey reference viewer"},
		{"caff", "caff", "Keep system awake utility"},
		{"sshh", "sshh", "SSH config helper"},
	}

	for i, util := range utilities {
		focused := a.utilityIndex == i
		enabled := cfg.Utilities[util.id]
		installed := a.manageInstalled[util.id]

		cursor := "  "
		if focused && !installed {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused && installed {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderCheckboxInlineWithInstallState(enabled, focused, installed)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			descStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		} else if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		suffix := ""
		if installed {
			suffix = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)")
		}

		content.WriteString(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-8s", util.name)),
			suffix,
			descStyle.Render(util.desc),
		))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(60)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • enter/esc save & back • yellow = installed")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigCLITools renders the CLI tools selection screen
func (a *App) renderConfigCLITools() string {
	// Ensure install status is cached
	a.ensureInstallCache()

	title := renderConfigTitle("", "CLI Tools", "Terminal-based productivity tools")

	cfg := a.deepDiveConfig
	var content strings.Builder

	tools := []struct {
		id   string
		name string
		desc string
	}{
		{"lazygit", "LazyGit", "Simple terminal UI for Git"},
		{"lazydocker", "LazyDocker", "Simple terminal UI for Docker"},
		{"btop", "btop", "Resource monitor with TUI"},
		{"glow", "Glow", "Render markdown on the CLI"},
		{"claude-code", "Claude Code", "AI-powered coding assistant (npm)"},
	}

	for i, tool := range tools {
		focused := a.cliToolIndex == i
		enabled := cfg.CLITools[tool.id]
		installed := a.manageInstalled[tool.id]

		cursor := "  "
		if focused && !installed {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused && installed {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderCheckboxInlineWithInstallState(enabled, focused, installed)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if installed {
			// Installed items show in yellow with "installed" suffix
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			descStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		} else if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		suffix := ""
		if installed {
			suffix = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)")
		}

		content.WriteString(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-14s", tool.name)),
			suffix,
			descStyle.Render(tool.desc),
		))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(70)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • enter/esc save & back • yellow = installed")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigCLIUtilities renders the CLI utilities selection screen
func (a *App) renderConfigCLIUtilities() string {
	// Ensure install status is cached
	a.ensureInstallCache()

	title := renderConfigTitle("󰘳", "CLI Utilities", "Essential command-line replacements")

	cfg := a.deepDiveConfig
	var content strings.Builder

	utilities := []struct {
		id   string
		name string
		desc string
	}{
		{"bat", "bat", "cat with syntax highlighting"},
		{"eza", "eza", "Modern ls replacement"},
		{"zoxide", "zoxide", "Smarter cd command"},
		{"ripgrep", "ripgrep", "Fast grep replacement"},
		{"fd", "fd", "Fast find replacement"},
		{"delta", "delta", "Beautiful git diffs"},
		{"fswatch", "fswatch", "File system watcher"},
	}

	for i, util := range utilities {
		focused := a.cliUtilityIndex == i
		enabled := cfg.CLIUtilities[util.id]
		installed := a.manageInstalled[util.id]

		cursor := "  "
		if focused && !installed {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused && installed {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderCheckboxInlineWithInstallState(enabled, focused, installed)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			descStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		} else if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		suffix := ""
		if installed {
			suffix = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)")
		}

		content.WriteString(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-10s", util.name)),
			suffix,
			descStyle.Render(util.desc),
		))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(65)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • enter/esc save & back • yellow = installed")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigGUIApps renders the GUI apps selection screen
func (a *App) renderConfigGUIApps() string {
	// Ensure install status is cached
	a.ensureInstallCache()

	title := renderConfigTitle("", "GUI Apps", "Desktop applications (cross-platform)")

	cfg := a.deepDiveConfig
	var content strings.Builder

	apps := []struct {
		id   string
		name string
		desc string
	}{
		{"zen-browser", "Zen Browser", "Privacy-focused browser based on Firefox"},
		{"cursor", "Cursor", "AI-first code editor"},
		{"lm-studio", "LM Studio", "Run local LLMs"},
		{"obs", "OBS Studio", "Streaming and recording software"},
	}

	for i, app := range apps {
		focused := a.guiAppIndex == i
		enabled := cfg.GUIApps[app.id]
		installed := a.manageInstalled[app.id]

		cursor := "  "
		if focused && !installed {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused && installed {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderCheckboxInlineWithInstallState(enabled, focused, installed)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			descStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		} else if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		suffix := ""
		if installed {
			suffix = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)")
		}

		content.WriteString(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-14s", app.name)),
			suffix,
			descStyle.Render(app.desc),
		))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(70)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • enter/esc save & back • yellow = installed")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigLazyGit renders the LazyGit configuration screen
func (a *App) renderConfigLazyGit() string {
	title := renderConfigTitle("", "LazyGit", "Simple terminal UI for Git commands")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Side-by-side diff
	content.WriteString(renderFieldLabel("Side-by-Side Diff", a.configFieldIndex == 0))
	content.WriteString(renderToggle(cfg.LazyGitSideBySide, a.configFieldIndex == 0))
	content.WriteString("\n\n")

	// Mouse mode
	content.WriteString(renderFieldLabel("Mouse Mode", a.configFieldIndex == 1))
	content.WriteString(renderToggle(cfg.LazyGitMouseMode, a.configFieldIndex == 1))
	content.WriteString("\n\n")

	// Theme
	content.WriteString(renderFieldLabel("Theme", a.configFieldIndex == 2))
	content.WriteString(renderOptionSelector(
		[]string{"auto", "dark", "light"},
		[]string{"Auto", "Dark", "Light"},
		cfg.LazyGitTheme,
		a.configFieldIndex == 2,
	))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(50)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigLazyDocker renders the LazyDocker configuration screen
func (a *App) renderConfigLazyDocker() string {
	title := renderConfigTitle("", "LazyDocker", "Simple terminal UI for Docker")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Mouse mode
	content.WriteString(renderFieldLabel("Mouse Mode", true))
	content.WriteString(renderToggle(cfg.LazyDockerMouseMode, true))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(50)).Render(content.String())
	help := HelpStyle.Render("space toggle • enter/esc save & back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigBtop renders the Btop configuration screen
func (a *App) renderConfigBtop() string {
	title := renderConfigTitle("", "Btop", "Resource monitor with beautiful TUI")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Theme
	content.WriteString(renderFieldLabel("Theme", a.configFieldIndex == 0))
	content.WriteString(renderOptionSelector(
		[]string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"},
		[]string{"Auto", "Dracula", "Gruvbox", "Nord", "Tokyo"},
		cfg.BtopTheme,
		a.configFieldIndex == 0,
	))
	content.WriteString("\n\n")

	// Update interval
	content.WriteString(renderFieldLabel("Update Interval", a.configFieldIndex == 1))
	intervalStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if a.configFieldIndex == 1 {
		intervalStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	}
	content.WriteString(fmt.Sprintf("    ◀ %s ▶", intervalStyle.Render(fmt.Sprintf("%dms", cfg.BtopUpdateMs))))
	content.WriteString("\n\n")

	// Show temperature
	content.WriteString(renderFieldLabel("Show CPU Temp", a.configFieldIndex == 2))
	content.WriteString(renderToggle(cfg.BtopShowTemp, a.configFieldIndex == 2))
	content.WriteString("\n\n")

	// Graph type
	content.WriteString(renderFieldLabel("Graph Type", a.configFieldIndex == 3))
	content.WriteString(renderOptionSelector(
		[]string{"braille", "block", "tty"},
		[]string{"Braille", "Block", "TTY"},
		cfg.BtopGraphType,
		a.configFieldIndex == 3,
	))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • space toggle • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// renderConfigGlow renders the Glow configuration screen
func (a *App) renderConfigGlow() string {
	title := renderConfigTitle("", "Glow", "Render markdown on the CLI")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Style
	content.WriteString(renderFieldLabel("Style", a.configFieldIndex == 0))
	content.WriteString(renderOptionSelector(
		[]string{"auto", "dark", "light", "notty"},
		[]string{"Auto", "Dark", "Light", "No TTY"},
		cfg.GlowStyle,
		a.configFieldIndex == 0,
	))
	content.WriteString("\n\n")

	// Pager
	content.WriteString(renderFieldLabel("Pager", a.configFieldIndex == 1))
	content.WriteString(renderOptionSelector(
		[]string{"auto", "less", "more", "none"},
		[]string{"Auto", "Less", "More", "None"},
		cfg.GlowPager,
		a.configFieldIndex == 1,
	))
	content.WriteString("\n\n")

	// Width
	content.WriteString(renderFieldLabel("Width", a.configFieldIndex == 2))
	widthStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if a.configFieldIndex == 2 {
		widthStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	}
	content.WriteString(fmt.Sprintf("    ◀ %s ▶", widthStyle.Render(fmt.Sprintf("%d chars", cfg.GlowWidth))))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

// deepDiveBoxWidth returns a responsive width for config boxes in the deep-dive
// flow. The goal is to preserve the "tight" defaults on typical terminals while
// scaling up on wider terminals and scaling down gracefully on narrow ones.
func (a *App) deepDiveBoxWidth(preferred int) int {
	if a.width <= 0 {
		return preferred
	}

	// Grow with terminal width, but cap so screens don't feel excessively wide.
	w := maxInt(preferred, a.width-30)
	w = min(70, w)

	// Ensure it fits with a small margin around the centered content.
	w = min(w, a.width-6)
	if w < 0 {
		w = 0
	}

	return w
}
