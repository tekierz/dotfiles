package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Cyberpunk color palette
var (
	// Primary colors
	ColorCyan     = lipgloss.Color("#00ffff")
	ColorMagenta  = lipgloss.Color("#ff00ff")
	ColorNeonPink = lipgloss.Color("#ff6ec7")
	ColorNeonBlue = lipgloss.Color("#00d4ff")
	ColorGreen    = lipgloss.Color("#39ff14")
	ColorYellow   = lipgloss.Color("#ffff00")
	ColorOrange   = lipgloss.Color("#ff6600")
	ColorRed      = lipgloss.Color("#ff0040")

	// Background/surface colors
	ColorBg       = lipgloss.Color("#0a0a0f")
	ColorSurface  = lipgloss.Color("#1a1a2e")
	ColorOverlay  = lipgloss.Color("#16213e")
	ColorMuted    = lipgloss.Color("#4a4a6a")
	ColorBorder   = lipgloss.Color("#2a2a4a")

	// Text colors
	ColorText       = lipgloss.Color("#e0e0e0")
	ColorTextMuted  = lipgloss.Color("#808090")
	ColorTextBright = lipgloss.Color("#ffffff")
)

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

	// Box drawing characters for neo-cyberpunk aesthetic
	BoxChars = struct {
		TopLeft, TopRight, BottomLeft, BottomRight string
		Horizontal, Vertical                       string
		LeftT, RightT, TopT, BottomT               string
		Cross                                      string
		GlitchH, GlitchV                           string
	}{
		TopLeft:     "┌",
		TopRight:    "┐",
		BottomLeft:  "└",
		BottomRight: "┘",
		Horizontal:  "─",
		Vertical:    "│",
		LeftT:       "├",
		RightT:      "┤",
		TopT:        "┬",
		BottomT:     "┴",
		Cross:       "┼",
		GlitchH:     "░▒▓",
		GlitchV:     "▓▒░",
	}
)

// GlitchText adds a glitch effect to text
func GlitchText(text string) string {
	return lipgloss.NewStyle().
		Foreground(ColorNeonPink).
		Background(ColorSurface).
		Render("░▒▓█ " + text + " █▓▒░")
}

// ScanlineEffect returns a scanline decoration
func ScanlineEffect(width int) string {
	line := ""
	for i := 0; i < width; i++ {
		if i%2 == 0 {
			line += "▀"
		} else {
			line += "▄"
		}
	}
	return lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(line)
}

// ProgressBar renders a progress bar
func ProgressBar(percent float64, width int) string {
	filled := int(percent * float64(width))
	empty := width - filled

	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}

	return lipgloss.NewStyle().
		Foreground(ColorCyan).
		Render(bar)
}
