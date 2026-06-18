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
		{"sunshine", "Sunshine", "Game streaming host (NVIDIA GameStream)"},
		{"moonlight", "Moonlight", "Game streaming client"},
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

// renderConfigClaudeCode renders the Claude Code MCP configuration screen
func (a *App) renderConfigClaudeCode() string {
	title := renderConfigTitle("󰚩", "Claude Code", "AI-powered coding assistant with MCP servers")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Install toggle
	content.WriteString(renderFieldLabel("Install Claude Code", a.configFieldIndex == -1))
	enabled := cfg.CLITools["claude-code"]
	installed := a.manageInstalled["claude-code"]
	content.WriteString(renderCheckboxInlineWithInstallState(enabled, a.configFieldIndex == -1, installed))
	if installed {
		content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)"))
	}
	content.WriteString("\n\n")

	// MCP Servers header
	mcpHeader := lipgloss.NewStyle().Foreground(ColorMagenta).Bold(true).Render("MCP Servers")
	mcpDesc := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" (Model Context Protocol)")
	content.WriteString(mcpHeader + mcpDesc + "\n\n")

	// MCP server list
	mcps := []struct {
		id   string
		name string
		desc string
	}{
		{"context7", "Context7", "Documentation lookup for any library"},
		{"task-master", "Task Master", "AI-driven task management"},
		{"github", "GitHub", "GitHub integration and automation"},
		{"supabase", "Supabase", "Supabase database integration"},
		{"convex", "Convex", "Convex backend integration"},
		{"puppeteer", "Puppeteer", "Browser automation and testing"},
		{"sequential-thinking", "Sequential Thinking", "Enhanced reasoning chains"},
	}

	for i, mcp := range mcps {
		focused := a.configFieldIndex == i
		enabled := cfg.ClaudeCodeMCPs[mcp.id]

		cursor := "  "
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		}

		checkbox := "[ ]"
		if enabled {
			checkbox = lipgloss.NewStyle().Foreground(ColorGreen).Render("[✓]")
		}

		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if focused {
			nameStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		// Show (default) indicator for context7
		suffix := ""
		if mcp.id == "context7" {
			suffix = lipgloss.NewStyle().Foreground(ColorYellow).Render(" (recommended)")
		}

		content.WriteString(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-20s", mcp.name)),
			suffix,
			descStyle.Render(mcp.desc),
		))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(65)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • esc back")

	return lipgloss.Place(
		a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
