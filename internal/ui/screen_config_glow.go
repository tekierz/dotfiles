package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configGlowScreen is the migrated ScreenHandler for the Glow config screen.
// Navigation + the back-to-menu transition are inherited from configFieldNav;
// only the field layout (View) and value adjustment (adjust) are
// screen-specific.
//
// Fields: 0=style (option), 1=pager (option), 2=width (stepper).
type configGlowScreen struct {
	configFieldNav
}

// NewConfigGlowScreen creates a new Glow config screen handler.
func NewConfigGlowScreen(ctx *ScreenContext) *configGlowScreen {
	s := &configGlowScreen{}
	s.id = ScreenConfigGlow
	s.maxField = func(*App) int { return 2 }
	s.adjust = glowAdjust
	s.SetContext(ctx)
	return s
}

// glowAdjust applies a left/right change to the focused field. Mirrors the
// legacy handleDeepDiveKey behavior exactly (options cycle; width steps by 10
// within [40, 200]). Glow has no space-toggle fields.
func glowAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 0:
			opts := []string{"auto", "dark", "light", "notty"}
			cfg.GlowStyle = cycleOption(opts, cfg.GlowStyle, fwd)
		case 1:
			opts := []string{"auto", "less", "more", "none"}
			cfg.GlowPager = cycleOption(opts, cfg.GlowPager, fwd)
		case 2:
			if fwd {
				if cfg.GlowWidth < 200 {
					cfg.GlowWidth += 10
				}
			} else if cfg.GlowWidth > 40 {
				cfg.GlowWidth -= 10
			}
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configGlowScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Glow configuration screen.
func (s *configGlowScreen) View(width, height int) string {
	a := s.App()
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
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
