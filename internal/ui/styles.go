package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Cyberpunk color palette
var (
	// Primary neon colors
	ColorCyan       = lipgloss.Color("#00ffff")
	ColorMagenta    = lipgloss.Color("#ff00ff")
	ColorNeonPink   = lipgloss.Color("#ff6ec7")
	ColorNeonBlue   = lipgloss.Color("#00d4ff")
	ColorNeonPurple = lipgloss.Color("#bf00ff")
	ColorGreen      = lipgloss.Color("#39ff14")
	ColorYellow     = lipgloss.Color("#ffff00")
	ColorOrange     = lipgloss.Color("#ff6600")
	ColorRed        = lipgloss.Color("#ff0040")

	// Gradient colors for smooth transitions
	GradientCyan = []lipgloss.Color{
		"#00ffff", "#00e5ff", "#00ccff", "#00b3ff", "#0099ff",
	}
	GradientPink = []lipgloss.Color{
		"#ff00ff", "#ff33cc", "#ff6699", "#ff9966", "#ffcc33",
	}
	GradientRainbow = []lipgloss.Color{
		"#ff0000", "#ff7f00", "#ffff00", "#00ff00", "#00ffff", "#0000ff", "#8b00ff",
	}
	GradientCyber = []lipgloss.Color{
		"#00ffff", "#00e1ff", "#00c3ff", "#0099ff", "#006eff", "#0044ff", "#bf00ff",
	}

	// Background/surface colors
	ColorBg      = lipgloss.Color("#0a0a0f")
	ColorSurface = lipgloss.Color("#1a1a2e")
	ColorOverlay = lipgloss.Color("#16213e")
	ColorMuted   = lipgloss.Color("#4a4a6a")
	ColorBorder  = lipgloss.Color("#2a2a4a")

	// Text colors
	ColorText       = lipgloss.Color("#e0e0e0")
	ColorTextMuted  = lipgloss.Color("#808090")
	ColorTextBright = lipgloss.Color("#ffffff")
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
			BorderForeground(ColorCyan)

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
			BorderForeground(ColorBorder)

	ButtonActiveStyle = lipgloss.NewStyle().
				Padding(0, 2).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorCyan).
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
			BorderForeground(ColorMagenta).
			Padding(0, 1)
)

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
