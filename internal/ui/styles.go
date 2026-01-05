package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

// ThemePalettes maps theme names to their color palettes
var ThemePalettes = map[string]ColorPalette{
	"neon-seapunk": {
		Accent:     "#00F5D4", // seafoam neon
		AccentAlt:  "#F15BB5", // hot pink
		Info:       "#00BBF9", // ocean neon
		Success:    "#00F5A0", // mint
		Warning:    "#FEE440", // neon sand
		Error:      "#FF4D6D", // red
		Bg:         "#070B1A", // deep ocean
		Surface:    "#0F1633",
		Overlay:    "#172046",
		Border:     "#25305A",
		Text:       "#E6F1FF",
		TextMuted:  "#97A7C7",
		TextBright: "#FFFFFF",
	},
	"catppuccin-mocha": {
		Accent:     "#89b4fa", // blue
		AccentAlt:  "#cba6f7", // mauve
		Info:       "#89dceb", // sky
		Success:    "#a6e3a1", // green
		Warning:    "#f9e2af", // yellow
		Error:      "#f38ba8", // red
		Bg:         "#1e1e2e", // base
		Surface:    "#313244", // surface0
		Overlay:    "#45475a", // surface1
		Border:     "#585b70", // surface2
		Text:       "#cdd6f4", // text
		TextMuted:  "#a6adc8", // subtext0
		TextBright: "#ffffff",
	},
	"catppuccin-latte": {
		Accent:     "#1e66f5", // blue
		AccentAlt:  "#8839ef", // mauve
		Info:       "#04a5e5", // sky
		Success:    "#40a02b", // green
		Warning:    "#df8e1d", // yellow
		Error:      "#d20f39", // red
		Bg:         "#eff1f5", // base
		Surface:    "#e6e9ef", // surface0
		Overlay:    "#dce0e8", // surface1
		Border:     "#ccd0da", // surface2
		Text:       "#4c4f69", // text
		TextMuted:  "#6c6f85", // subtext0
		TextBright: "#000000",
	},
	"catppuccin-frappe": {
		Accent:     "#8caaee", // blue
		AccentAlt:  "#ca9ee6", // mauve
		Info:       "#99d1db", // sky
		Success:    "#a6d189", // green
		Warning:    "#e5c890", // yellow
		Error:      "#e78284", // red
		Bg:         "#303446", // base
		Surface:    "#414559", // surface0
		Overlay:    "#51576d", // surface1
		Border:     "#626880", // surface2
		Text:       "#c6d0f5", // text
		TextMuted:  "#a5adce", // subtext0
		TextBright: "#ffffff",
	},
	"catppuccin-macchiato": {
		Accent:     "#8aadf4", // blue
		AccentAlt:  "#c6a0f6", // mauve
		Info:       "#91d7e3", // sky
		Success:    "#a6da95", // green
		Warning:    "#eed49f", // yellow
		Error:      "#ed8796", // red
		Bg:         "#24273a", // base
		Surface:    "#363a4f", // surface0
		Overlay:    "#494d64", // surface1
		Border:     "#5b6078", // surface2
		Text:       "#cad3f5", // text
		TextMuted:  "#a5adcb", // subtext0
		TextBright: "#ffffff",
	},
	"dracula": {
		Accent:     "#bd93f9", // purple
		AccentAlt:  "#ff79c6", // pink
		Info:       "#8be9fd", // cyan
		Success:    "#50fa7b", // green
		Warning:    "#f1fa8c", // yellow
		Error:      "#ff5555", // red
		Bg:         "#282a36", // background
		Surface:    "#44475a", // current line
		Overlay:    "#6272a4", // comment
		Border:     "#44475a",
		Text:       "#f8f8f2", // foreground
		TextMuted:  "#6272a4", // comment
		TextBright: "#ffffff",
	},
	"gruvbox-dark": {
		Accent:     "#83a598", // blue
		AccentAlt:  "#d3869b", // purple
		Info:       "#83a598", // blue
		Success:    "#b8bb26", // green
		Warning:    "#fabd2f", // yellow
		Error:      "#fb4934", // red
		Bg:         "#282828", // bg
		Surface:    "#3c3836", // bg1
		Overlay:    "#504945", // bg2
		Border:     "#665c54", // bg3
		Text:       "#ebdbb2", // fg
		TextMuted:  "#a89984", // gray
		TextBright: "#fbf1c7", // fg0
	},
	"gruvbox-light": {
		Accent:     "#076678", // blue
		AccentAlt:  "#8f3f71", // purple
		Info:       "#076678", // blue
		Success:    "#79740e", // green
		Warning:    "#b57614", // yellow
		Error:      "#9d0006", // red
		Bg:         "#fbf1c7", // bg
		Surface:    "#ebdbb2", // bg1
		Overlay:    "#d5c4a1", // bg2
		Border:     "#bdae93", // bg3
		Text:       "#3c3836", // fg
		TextMuted:  "#7c6f64", // gray
		TextBright: "#282828", // fg0
	},
	"nord": {
		Accent:     "#88c0d0", // nord8 frost
		AccentAlt:  "#b48ead", // nord15 aurora
		Info:       "#81a1c1", // nord9
		Success:    "#a3be8c", // nord14
		Warning:    "#ebcb8b", // nord13
		Error:      "#bf616a", // nord11
		Bg:         "#2e3440", // nord0
		Surface:    "#3b4252", // nord1
		Overlay:    "#434c5e", // nord2
		Border:     "#4c566a", // nord3
		Text:       "#eceff4", // nord6
		TextMuted:  "#d8dee9", // nord4
		TextBright: "#ffffff",
	},
	"tokyo-night": {
		Accent:     "#7aa2f7", // blue
		AccentAlt:  "#bb9af7", // purple
		Info:       "#7dcfff", // cyan
		Success:    "#9ece6a", // green
		Warning:    "#e0af68", // yellow
		Error:      "#f7768e", // red
		Bg:         "#1a1b26", // bg
		Surface:    "#24283b", // bg_highlight
		Overlay:    "#414868", // terminal_black
		Border:     "#565f89", // comment
		Text:       "#c0caf5", // fg
		TextMuted:  "#a9b1d6", // fg_dark
		TextBright: "#ffffff",
	},
	"solarized-dark": {
		Accent:     "#268bd2", // blue
		AccentAlt:  "#6c71c4", // violet
		Info:       "#2aa198", // cyan
		Success:    "#859900", // green
		Warning:    "#b58900", // yellow
		Error:      "#dc322f", // red
		Bg:         "#002b36", // base03
		Surface:    "#073642", // base02
		Overlay:    "#586e75", // base01
		Border:     "#657b83", // base00
		Text:       "#839496", // base0
		TextMuted:  "#93a1a1", // base1
		TextBright: "#fdf6e3", // base3
	},
	"solarized-light": {
		Accent:     "#268bd2", // blue
		AccentAlt:  "#6c71c4", // violet
		Info:       "#2aa198", // cyan
		Success:    "#859900", // green
		Warning:    "#b58900", // yellow
		Error:      "#dc322f", // red
		Bg:         "#fdf6e3", // base3
		Surface:    "#eee8d5", // base2
		Overlay:    "#93a1a1", // base1
		Border:     "#839496", // base0
		Text:       "#657b83", // base00
		TextMuted:  "#586e75", // base01
		TextBright: "#002b36", // base03
	},
	"monokai": {
		Accent:     "#66d9ef", // cyan
		AccentAlt:  "#ae81ff", // purple
		Info:       "#66d9ef", // cyan
		Success:    "#a6e22e", // green
		Warning:    "#e6db74", // yellow
		Error:      "#f92672", // red/pink
		Bg:         "#272822", // bg
		Surface:    "#3e3d32", // line highlight
		Overlay:    "#49483e", // selection
		Border:     "#75715e", // comment
		Text:       "#f8f8f2", // fg
		TextMuted:  "#75715e", // comment
		TextBright: "#ffffff",
	},
	"rose-pine": {
		Accent:     "#c4a7e7", // iris
		AccentAlt:  "#ebbcba", // rose
		Info:       "#9ccfd8", // foam
		Success:    "#9ccfd8", // foam
		Warning:    "#f6c177", // gold
		Error:      "#eb6f92", // love
		Bg:         "#191724", // base
		Surface:    "#1f1d2e", // surface
		Overlay:    "#26233a", // overlay
		Border:     "#403d52", // highlight_med
		Text:       "#e0def4", // text
		TextMuted:  "#908caa", // subtle
		TextBright: "#ffffff",
	},
	"everforest": {
		Accent:     "#7fbbb3", // aqua
		AccentAlt:  "#d699b6", // purple
		Info:       "#7fbbb3", // aqua
		Success:    "#a7c080", // green
		Warning:    "#dbbc7f", // yellow
		Error:      "#e67e80", // red
		Bg:         "#2d353b", // bg0
		Surface:    "#343f44", // bg1
		Overlay:    "#3d484d", // bg2
		Border:     "#475258", // bg3
		Text:       "#d3c6aa", // fg
		TextMuted:  "#859289", // gray1
		TextBright: "#ffffff",
	},
	"one-dark": {
		Accent:     "#61afef", // blue
		AccentAlt:  "#c678dd", // purple
		Info:       "#56b6c2", // cyan
		Success:    "#98c379", // green
		Warning:    "#e5c07b", // yellow
		Error:      "#e06c75", // red
		Bg:         "#282c34", // bg
		Surface:    "#21252b", // bg_dark
		Overlay:    "#2c323c", // cursor_grey
		Border:     "#3e4451", // gutter_grey
		Text:       "#abb2bf", // fg
		TextMuted:  "#5c6370", // comment
		TextBright: "#ffffff",
	},
}

// CurrentPalette holds the active theme's colors
var CurrentPalette = ThemePalettes["neon-seapunk"]

// SetTheme updates the current palette based on theme name
func SetTheme(theme string) {
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
	ColorOrange     = lipgloss.Color("#FF9F1C")
	ColorRed        = lipgloss.Color("#FF4D6D")

	// Gradient colors for smooth transitions
	GradientCyan = []lipgloss.Color{
		"#00F5D4", "#00E5FF", "#00BBF9", "#4EA8DE", "#5B7CFA",
	}
	GradientPink = []lipgloss.Color{
		"#F15BB5", "#FF5DA2", "#C77DFF", "#9B5DE5", "#5B7CFA",
	}
	GradientRainbow = []lipgloss.Color{
		"#ff0000", "#ff7f00", "#ffff00", "#00ff00", "#00ffff", "#0000ff", "#8b00ff",
	}
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
var SpinnerBlockFrames = []string{"▖", "▘", "▝", "▗"}
var SpinnerPulseFrames = []string{"█", "▓", "▒", "░", "▒", "▓"}

// Styles
var (
	// Container styles
	ContainerStyle = lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Background(ColorSurface)

	// Title styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true).
			Padding(0, 1)

	LogoStyle = lipgloss.NewStyle().
			Foreground(ColorNeonPink).
			Bold(true)

	// Button styles
	ButtonStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Background(ColorSurface)

	ButtonActiveStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorCyan).
				Background(ColorOverlay).
				Foreground(ColorCyan).
				Bold(true)

	// Glowing button style
	ButtonGlowStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorNeonPink).
			Foreground(ColorNeonPink).
			Bold(true)

	// Status indicator styles
	StatusReadyStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	StatusPendingStyle = lipgloss.NewStyle().
				Foreground(ColorYellow)

	StatusErrorStyle = lipgloss.NewStyle().
				Foreground(ColorRed)

	// Help text style
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Padding(1, 0)

	// Accent box style
	AccentBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorNeonPurple).
			Background(ColorSurface).
			Padding(0, 1)
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

// GlitchText adds a glitch effect to text with gradient
func GlitchText(text string) string {
	prefix := GradientText("░▒▓█", GradientCyber)
	suffix := GradientText("█▓▒░", []lipgloss.Color{
		"#bf00ff", "#0044ff", "#006eff", "#0099ff", "#00c3ff", "#00e1ff", "#00ffff",
	})
	middle := lipgloss.NewStyle().
		Foreground(ColorTextBright).
		Bold(true).
		Render(" " + text + " ")
	return prefix + middle + suffix
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

// PulsingText returns text that appears to pulse based on frame
func PulsingText(text string, frame int) string {
	colors := []lipgloss.Color{ColorCyan, ColorNeonBlue, ColorMagenta, ColorNeonPink, ColorMagenta, ColorNeonBlue}
	colorIdx := frame % len(colors)
	return lipgloss.NewStyle().Foreground(colors[colorIdx]).Bold(true).Render(text)
}

// AnimatedSpinner returns the current spinner frame
func AnimatedSpinner(frame int) string {
	idx := frame % len(SpinnerFrames)
	return lipgloss.NewStyle().Foreground(ColorCyan).Render(SpinnerFrames[idx])
}

// AnimatedSpinnerDots returns the current dots spinner frame
func AnimatedSpinnerDots(frame int) string {
	idx := frame % len(SpinnerDotsFrames)
	return lipgloss.NewStyle().Foreground(ColorMagenta).Render(SpinnerDotsFrames[idx])
}

// ScanlineEffect returns a scanline decoration
func ScanlineEffect(width int) string {
	var line strings.Builder
	for i := 0; i < width; i++ {
		colorIdx := (i * len(GradientCyber)) / width
		if colorIdx >= len(GradientCyber) {
			colorIdx = len(GradientCyber) - 1
		}
		style := lipgloss.NewStyle().Foreground(GradientCyber[colorIdx])
		if i%2 == 0 {
			line.WriteString(style.Render("▀"))
		} else {
			line.WriteString(style.Render("▄"))
		}
	}
	return line.String()
}

// ScanlineEffectAnimated returns a scanline decoration with a subtle animated
// gradient shift. Useful for headers and separators.
func ScanlineEffectAnimated(width int, frame int) string {
	if width <= 0 {
		return ""
	}

	var line strings.Builder
	for i := 0; i < width; i++ {
		colorIdx := ((i + frame) * len(GradientCyber)) / width
		colorIdx = colorIdx % len(GradientCyber)
		style := lipgloss.NewStyle().Foreground(GradientCyber[colorIdx])
		if (i+frame)%2 == 0 {
			line.WriteString(style.Render("▀"))
		} else {
			line.WriteString(style.Render("▄"))
		}
	}
	return line.String()
}

// ShimmerDivider renders a subtle divider with a moving highlight segment.
// This is intentionally less "glitchy" than ScanlineEffectAnimated.
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

// NeonBox creates a neon-style box with glowing border
func NeonBox(content string, color lipgloss.Color) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(1, 2).
		Render(content)
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
	case "running", "active", "in_progress":
		return lipgloss.NewStyle().Foreground(ColorCyan).Render("●")
	case "pending", "waiting":
		return lipgloss.NewStyle().Foreground(ColorTextMuted).Render("○")
	case "error", "failed":
		return lipgloss.NewStyle().Foreground(ColorRed).Render("●")
	case "warning":
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
	// Left-align for consistent mouse hit detection, with background to prevent transparency
	return lipgloss.NewStyle().Width(width).Background(ColorBg).Render(line)
}

// PlaceWithBackground centers content within a full-screen area with the app's background color.
// This prevents the terminal's default background from showing through.
func PlaceWithBackground(width, height int, content string) string {
	// First center the content
	centered := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)

	// Then apply the background color to the entire area
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(ColorBg).
		Render(centered)
}
