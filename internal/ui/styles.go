package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Neon-seapunk color palette
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

// TabSpec describes a single tab in a one-line tab bar.
type TabSpec struct {
	Label  string
	Active bool
}

// RenderTabsBar renders a pill-style tab strip (single line).
// It is designed to feel closer to the Lip Gloss demo-style UI (tabs + content).
func RenderTabsBar(width int, tabs []TabSpec) string {
	if width <= 0 {
		return ""
	}

	activeBg := ColorCyan
	activeFg := ColorBg
	inactiveBg := ColorSurface
	inactiveFg := ColorTextMuted

	sep := lipgloss.NewStyle().Foreground(ColorBorder).Render(" ")
	leftCap := ""
	rightCap := ""

	var parts []string
	for _, t := range tabs {
		bg := inactiveBg
		fg := inactiveFg
		bold := false
		if t.Active {
			bg = activeBg
			fg = activeFg
			bold = true
		}

		cap := lipgloss.NewStyle().Foreground(bg)
		txt := lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(bold).Padding(0, 1)
		parts = append(parts, cap.Render(leftCap)+txt.Render(t.Label)+cap.Render(rightCap))
	}

	line := strings.Join(parts, sep)
	// Ensure stable width for consistent layout/mouse mapping.
	return lipgloss.NewStyle().Width(width).Render(line)
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
