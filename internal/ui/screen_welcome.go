package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// welcomeScreen is the migrated ScreenHandler for the welcome/intro screen.
//
// State stays on App: the deepDive toggle is read/written through s.App() so
// the rest of the legacy wizard (and persistence) keeps seeing the same value.
type welcomeScreen struct {
	BaseScreen
}

// NewWelcomeScreen creates a new welcome screen handler.
func NewWelcomeScreen(ctx *ScreenContext) *welcomeScreen {
	s := &welcomeScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *welcomeScreen) ID() Screen { return ScreenWelcome }

// Init returns any initial commands (none on entry).
func (s *welcomeScreen) Init() tea.Cmd { return nil }

// Update handles keyboard and mouse input for the welcome screen.
func (s *welcomeScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return s, tea.Quit
		case "enter":
			if a.deepDive {
				// Start async install cache load for deep dive menu. NavigateTo
				// falls back to legacy mode for the (unmigrated) deep dive menu,
				// which syncs a.screen in App.Update.
				if cmd := a.startInstallCacheLoad(); cmd != nil {
					return s, tea.Batch(NavigateTo(ScreenDeepDiveMenu), cmd)
				}
				return s, NavigateTo(ScreenDeepDiveMenu)
			}
			// Entering the theme picker as the wizard's step 2: selecting a theme
			// should advance to the nav picker. Explicitly clear themeStandalone
			// so a previous standalone visit can't leak into the wizard (C8).
			a.themeStandalone = false
			a.themeReturn = ScreenWelcome
			return s, NavigateTo(ScreenThemePicker)
		case "tab", "left", "right", "h", "l":
			a.deepDive = !a.deepDive
		}

	case tea.MouseMsg:
		return s, s.handleMouse(msg)
	}
	return s, nil
}

// handleMouse toggles the deep dive option based on which half of the lower
// screen was clicked. Mirrors the legacy handleWelcomeMouse behavior.
func (s *welcomeScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		centerY := s.Height() / 2
		centerX := s.Width() / 2
		if m.Y > centerY {
			if m.X < centerX {
				a.deepDive = false
			} else {
				a.deepDive = true
			}
		}
	}
	return nil
}

// View renders the welcome screen content.
func (s *welcomeScreen) View(width, height int) string {
	a := s.App()

	// ASCII Logo with gradient.
	// Keep the logo from overflowing on smaller terminals.
	logoMaxW := maxInt(20, width-6)
	logo := lipgloss.NewStyle().MaxWidth(logoMaxW).Render(ASCIILogo())

	// Decorative border
	borderW := min(60, maxInt(24, width-8))
	topBorder := CyberBorder(borderW)

	// Status indicators with neon styling
	statusContent := fmt.Sprintf(
		"  %s %s  %s %s  %s %s",
		StatusDot("success"),
		GradientText("SYSTEM READY", GradientCyber),
		StatusDot("success"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("16 THEMES"),
		StatusDot("success"),
		lipgloss.NewStyle().Foreground(ColorMagenta).Render("ALL TOOLS"),
	)

	// Tools list with icons
	tools := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"zsh • tmux • neovim • yazi • ghostty • fzf • zoxide • bat • delta")

	// Description with gradient accent
	desc := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(ColorText).Render("Your terminal environment, configured in minutes."),
		lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render("Cross-platform · Fully reversible · Open source"),
	)

	// Buttons with better styling
	var quickSetup, deepDive string

	if !a.deepDive {
		quickSetup = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorCyan).
			Foreground(ColorCyan).
			Bold(true).
			Render("▶ QUICK SETUP\n  Recommended")
		deepDive = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Foreground(ColorTextMuted).
			Render("  DEEP DIVE\n  Customize all")
	} else {
		quickSetup = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Foreground(ColorTextMuted).
			Render("  QUICK SETUP\n  Recommended")
		deepDive = lipgloss.NewStyle().
			Padding(1, 3).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorMagenta).
			Foreground(ColorMagenta).
			Bold(true).
			Render("▶ DEEP DIVE\n  Customize all")
	}

	// Stack buttons vertically on narrower terminals.
	var buttons string
	if width < 78 {
		buttons = lipgloss.JoinVertical(lipgloss.Center, quickSetup, "", deepDive)
	} else {
		buttons = lipgloss.JoinHorizontal(lipgloss.Top, quickSetup, "  ", deepDive)
	}

	// Help text with gradient
	help := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"←→ select • enter continue • q quit")

	// Bottom border
	bottomBorder := CyberBorder(borderW)

	// Compose the screen
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		topBorder,
		logo,
		truncateVisible(statusContent, borderW),
		truncateVisible(tools, borderW),
		"",
		desc,
		"",
		buttons,
		"",
		help,
		bottomBorder,
	)

	// Center on screen with styled container
	container := lipgloss.NewStyle().
		Padding(1, 2).
		Render(content)

	return PlaceWithBackground(width, height, container)
}
