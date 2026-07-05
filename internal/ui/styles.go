package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/tekierz/dotfiles/internal/theme"
)

// ColorPalette represents a theme's color scheme for the TUI
type ColorPalette struct {
	// Primary accent colors
	Accent    lipgloss.Color // Main accent (selections, highlights)
	AccentAlt lipgloss.Color // Secondary accent
	Info      lipgloss.Color // Blue/info color

	// Semantic colors
	Success lipgloss.Color // Green
	Warning lipgloss.Color // Yellow
	Error   lipgloss.Color // Red

	// Background/surface colors
	Bg      lipgloss.Color // Main background
	Surface lipgloss.Color // Elevated surfaces
	Overlay lipgloss.Color // Popup backgrounds
	Border  lipgloss.Color // Borders

	// Text colors
	Text       lipgloss.Color // Primary text
	TextMuted  lipgloss.Color // Muted text
	TextBright lipgloss.Color // Bright/emphasized text
}

// ThemePalettes maps theme names to their TUI color palettes, derived from the
// shared internal/theme palette data so the TUI and the tool config generators
// use one source of truth. The public shape (map[string]ColorPalette) is
// unchanged for existing callers.
var ThemePalettes = buildThemePalettes()

func buildThemePalettes() map[string]ColorPalette {
	out := make(map[string]ColorPalette)
	for _, name := range theme.Names() {
		p, _ := theme.Get(name)
		out[name] = colorPaletteFromTheme(p)
	}
	return out
}

// colorPaletteFromTheme converts a shared hex-string palette into the TUI's
// lipgloss-based ColorPalette.
func colorPaletteFromTheme(p theme.Palette) ColorPalette {
	return ColorPalette{
		Accent:     lipgloss.Color(p.Accent),
		AccentAlt:  lipgloss.Color(p.AccentAlt),
		Info:       lipgloss.Color(p.Info),
		Success:    lipgloss.Color(p.Success),
		Warning:    lipgloss.Color(p.Warning),
		Error:      lipgloss.Color(p.Error),
		Bg:         lipgloss.Color(p.Bg),
		Surface:    lipgloss.Color(p.Surface),
		Overlay:    lipgloss.Color(p.Overlay),
		Border:     lipgloss.Color(p.Border),
		Text:       lipgloss.Color(p.Text),
		TextMuted:  lipgloss.Color(p.TextMuted),
		TextBright: lipgloss.Color(p.TextBright),
	}
}

// CurrentPalette holds the active theme's colors
var CurrentPalette = ThemePalettes["neon-seapunk"]

// themeMu serializes the SetTheme write sequence. In production there is a
// single App driven by Bubble Tea on one goroutine, so this never contends;
// it exists so the parallel-test construction storm (each NewApp -> SetTheme)
// cannot race write-write on the package-global palette/color/style vars.
var themeMu sync.Mutex

// SetTheme updates the current palette based on theme name
func SetTheme(theme string) {
	themeMu.Lock()
	defer themeMu.Unlock()
	if p, ok := ThemePalettes[theme]; ok {
		CurrentPalette = p
		updateDynamicColors()
	}
}

// updateDynamicColors updates the legacy color variables from CurrentPalette
func updateDynamicColors() {
	ColorCyan = CurrentPalette.Accent
	ColorNeonBlue = CurrentPalette.Info
	ColorMagenta = CurrentPalette.AccentAlt
	ColorNeonPink = CurrentPalette.AccentAlt
	ColorNeonPurple = CurrentPalette.AccentAlt
	ColorGreen = CurrentPalette.Success
	ColorYellow = CurrentPalette.Warning
	ColorRed = CurrentPalette.Error
	ColorBg = CurrentPalette.Bg
	ColorSurface = CurrentPalette.Surface
	ColorOverlay = CurrentPalette.Overlay
	ColorMuted = CurrentPalette.Border // Use border color as muted background
	ColorBorder = CurrentPalette.Border
	ColorText = CurrentPalette.Text
	ColorTextMuted = CurrentPalette.TextMuted
	ColorTextBright = CurrentPalette.TextBright

	// Update gradients based on current palette
	GradientCyber = []lipgloss.Color{
		CurrentPalette.Accent,
		CurrentPalette.Info,
		CurrentPalette.AccentAlt,
	}

	// Update all styles with new colors
	updateStyles()
}

// updateStyles recreates all styles with current theme colors
func updateStyles() {
	ContainerStyle = lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder)

	TitleStyle = lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true).
		Padding(0, 1)

	ButtonStyle = lipgloss.NewStyle().
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder)

	ButtonActiveStyle = lipgloss.NewStyle().
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Foreground(ColorCyan).
		Bold(true)

	HelpStyle = lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Padding(1, 0)
}

// Legacy color variables (updated by SetTheme via updateDynamicColors)
var (
	// Accents (aqua / hot pink / purple) with high-contrast but clean usage.
	ColorCyan       = lipgloss.Color("#00F5D4") // seafoam neon
	ColorNeonBlue   = lipgloss.Color("#00BBF9") // ocean neon
	ColorMagenta    = lipgloss.Color("#F15BB5") // hot pink
	ColorNeonPink   = lipgloss.Color("#FF5DA2") // softer pink highlight
	ColorNeonPurple = lipgloss.Color("#9B5DE5") // electric purple
	ColorGreen      = lipgloss.Color("#00F5A0") // mint success
	ColorYellow     = lipgloss.Color("#FEE440") // neon sand
	ColorRed        = lipgloss.Color("#FF4D6D")

	// Primary UI gradient used across borders, dividers, and logo accents.
	GradientCyber = []lipgloss.Color{
		"#00F5D4", "#00E5FF", "#00BBF9", "#4EA8DE", "#5B7CFA", "#9B5DE5", "#F15BB5",
	}

	// Background/surface colors (deep ocean).
	ColorBg      = lipgloss.Color("#070B1A")
	ColorSurface = lipgloss.Color("#0F1633")
	ColorOverlay = lipgloss.Color("#172046")
	ColorMuted   = lipgloss.Color("#3A466B")
	ColorBorder  = lipgloss.Color("#25305A")

	// Text colors (slightly cool for readability).
	ColorText       = lipgloss.Color("#E6F1FF")
	ColorTextMuted  = lipgloss.Color("#97A7C7")
	ColorTextBright = lipgloss.Color("#FFFFFF")
)

// Spinner frames for animation
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var SpinnerDotsFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// Styles - no explicit backgrounds to respect terminal transparency
var (
	// Container styles
	ContainerStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	// Title styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true).
			Padding(0, 1)

	// Button styles
	ButtonStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	ButtonActiveStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorCyan).
				Foreground(ColorCyan).
				Bold(true)

	// Help text style
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Padding(1, 0)
)

// RenderBadge renders a compact pill badge.
func RenderBadge(label string, fg, bg lipgloss.Color) string {
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Bold(true).
		Padding(0, 1).
		Render(label)
}

// GradientText renders text with a horizontal gradient
func GradientText(text string, colors []lipgloss.Color) string {
	if len(colors) == 0 || len(text) == 0 {
		return text
	}

	var result strings.Builder
	for i, char := range text {
		colorIdx := (i * len(colors)) / len(text)
		if colorIdx >= len(colors) {
			colorIdx = len(colors) - 1
		}
		style := lipgloss.NewStyle().Foreground(colors[colorIdx])
		result.WriteString(style.Render(string(char)))
	}
	return result.String()
}

// CyberBorder creates a cyberpunk-style border decoration
func CyberBorder(width int) string {
	if width < 4 {
		return ""
	}

	left := GradientText("◢", GradientCyber[:1])
	right := GradientText("◣", GradientCyber[len(GradientCyber)-1:])

	middle := ""
	for i := 0; i < width-2; i++ {
		colorIdx := (i * len(GradientCyber)) / (width - 2)
		if colorIdx >= len(GradientCyber) {
			colorIdx = len(GradientCyber) - 1
		}
		style := lipgloss.NewStyle().Foreground(GradientCyber[colorIdx])
		middle += style.Render("═")
	}

	return left + middle + right
}

// AnimatedSpinnerDots returns the current dots spinner frame
func AnimatedSpinnerDots(frame int) string {
	idx := frame % len(SpinnerDotsFrames)
	return lipgloss.NewStyle().Foreground(ColorMagenta).Render(SpinnerDotsFrames[idx])
}

// ShimmerDivider renders a subtle divider with a moving highlight segment.
func ShimmerDivider(width int, frame int, enabled bool) string {
	if width <= 0 {
		return ""
	}

	baseStyle := lipgloss.NewStyle().Foreground(ColorBorder).Faint(true)
	if !enabled {
		return baseStyle.Render(strings.Repeat("─", width))
	}

	seg := maxInt(6, width/10)
	pos := frame % (width + seg)
	pos -= seg

	var sb strings.Builder
	for i := 0; i < width; i++ {
		if i >= pos && i < pos+seg {
			color := GradientCyber[(i+frame)%len(GradientCyber)]
			sb.WriteString(lipgloss.NewStyle().Foreground(color).Render("─"))
			continue
		}
		sb.WriteString(baseStyle.Render("─"))
	}
	return sb.String()
}

// ProgressBar renders a progress bar with gradient
func ProgressBar(percent float64, width int) string {
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	var bar strings.Builder
	for i := 0; i < filled; i++ {
		colorIdx := (i * len(GradientCyber)) / width
		if colorIdx >= len(GradientCyber) {
			colorIdx = len(GradientCyber) - 1
		}
		style := lipgloss.NewStyle().Foreground(GradientCyber[colorIdx])
		bar.WriteString(style.Render("█"))
	}

	emptyStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	bar.WriteString(emptyStyle.Render(strings.Repeat("░", empty)))

	return bar.String()
}

// ProgressBarAnimated renders an animated progress bar
func ProgressBarAnimated(percent float64, width int, frame int) string {
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	var bar strings.Builder
	for i := 0; i < filled; i++ {
		colorIdx := ((i + frame) * len(GradientCyber)) / width
		colorIdx = colorIdx % len(GradientCyber)
		style := lipgloss.NewStyle().Foreground(GradientCyber[colorIdx])
		bar.WriteString(style.Render("█"))
	}

	// Animated edge
	if empty > 0 && filled > 0 {
		pulseChars := []string{"▓", "▒", "░"}
		pulseIdx := frame % len(pulseChars)
		bar.WriteString(lipgloss.NewStyle().Foreground(ColorCyan).Render(pulseChars[pulseIdx]))
		empty--
	}

	emptyStyle := lipgloss.NewStyle().Foreground(ColorBorder)
	bar.WriteString(emptyStyle.Render(strings.Repeat("░", empty)))

	return bar.String()
}

// ASCIILogo returns an ASCII art logo with gradient
func ASCIILogo() string {
	logo := `
    ██████╗  ██████╗ ████████╗███████╗██╗██╗     ███████╗███████╗
    ██╔══██╗██╔═══██╗╚══██╔══╝██╔════╝██║██║     ██╔════╝██╔════╝
    ██║  ██║██║   ██║   ██║   █████╗  ██║██║     █████╗  ███████╗
    ██║  ██║██║   ██║   ██║   ██╔══╝  ██║██║     ██╔══╝  ╚════██║
    ██████╔╝╚██████╔╝   ██║   ██║     ██║███████╗███████╗███████║
    ╚═════╝  ╚═════╝    ╚═╝   ╚═╝     ╚═╝╚══════╝╚══════╝╚══════╝`

	lines := strings.Split(logo, "\n")
	var result strings.Builder

	for lineIdx, line := range lines {
		if line == "" {
			result.WriteString("\n")
			continue
		}
		// Apply gradient based on line position
		colors := GradientCyber
		for i, char := range line {
			if char == ' ' || char == '\n' {
				result.WriteRune(char)
				continue
			}
			// Mix horizontal and vertical gradient
			colorIdx := ((i + lineIdx*2) * len(colors)) / (len(line) + len(lines)*2)
			colorIdx = colorIdx % len(colors)
			style := lipgloss.NewStyle().Foreground(colors[colorIdx])
			result.WriteString(style.Render(string(char)))
		}
		result.WriteString("\n")
	}

	return result.String()
}

// StatusDot returns a colored status dot
func StatusDot(status string) string {
	switch status {
	case "done", "complete", "success":
		return lipgloss.NewStyle().Foreground(ColorGreen).Render("●")
	case "installed":
		return lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("●")
	case "running", "active", "in_progress":
		return lipgloss.NewStyle().Foreground(ColorCyan).Render("●")
	case "pending", "waiting":
		return lipgloss.NewStyle().Foreground(ColorTextMuted).Render("○")
	case "error", "failed":
		return lipgloss.NewStyle().Foreground(ColorRed).Render("●")
	case "warning", "partial":
		return lipgloss.NewStyle().Foreground(ColorYellow).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(ColorTextMuted).Render("○")
	}
}

// ManagementTab represents a tab in the management UI
type ManagementTab struct {
	Name   string
	Icon   string
	Screen Screen
}

// GetManagementTabs returns the tabs for the management UI
func GetManagementTabs() []ManagementTab {
	return []ManagementTab{
		{Name: "Manage", Icon: "󰒓", Screen: ScreenManage},
		{Name: "Users", Icon: "󰀄", Screen: ScreenUsers},
		{Name: "Hotkeys", Icon: "󰌌", Screen: ScreenHotkeys},
		{Name: "Update", Icon: "󰚰", Screen: ScreenUpdate},
		{Name: "Backups", Icon: "󰁯", Screen: ScreenBackups},
	}
}

// RenderTabBar renders a pill-style tab bar for the given active screen.
// All management screens (Manage, Hotkeys, Update, Backups) use this.
func RenderTabBar(activeScreen Screen, width int) string {
	if width <= 0 {
		return ""
	}

	tabs := GetManagementTabs()

	activeBg := ColorCyan
	activeFg := ColorBg
	inactiveBg := ColorSurface
	inactiveFg := ColorTextMuted

	sep := lipgloss.NewStyle().Foreground(ColorBorder).Render(" ")

	var parts []string
	for i, tab := range tabs {
		bg := inactiveBg
		fg := inactiveFg
		bold := false
		if tab.Screen == activeScreen {
			bg = activeBg
			fg = activeFg
			bold = true
		}

		// Format: "N 󰒓 Name" with pill-style background
		label := fmt.Sprintf("%d %s %s", i+1, tab.Icon, tab.Name)
		txt := lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(bold).Padding(0, 1)
		parts = append(parts, txt.Render(label))
	}

	line := strings.Join(parts, sep)
	// Left-align for consistent mouse hit detection
	return lipgloss.NewStyle().Width(width).Render(line)
}

// PlaceWithBackground centers content within a full-screen area.
// Does not set an explicit background to respect terminal transparency.
func PlaceWithBackground(width, height int, content string) string {
	// Center the content without forcing a background color
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
