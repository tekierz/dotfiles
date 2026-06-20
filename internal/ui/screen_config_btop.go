package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configBtopScreen is the migrated ScreenHandler for the Btop config screen.
// Navigation + the back-to-menu transition are inherited from configFieldNav;
// only the field layout (View) and value adjustment (adjust) are
// screen-specific.
//
// Fields: 0=theme (option), 1=update interval (stepper), 2=show CPU temp
// (toggle), 3=graph type (option).
type configBtopScreen struct {
	configFieldNav
}

// NewConfigBtopScreen creates a new Btop config screen handler.
func NewConfigBtopScreen(ctx *ScreenContext) *configBtopScreen {
	s := &configBtopScreen{}
	s.id = ScreenConfigBtop
	s.maxField = func(*App) int { return 3 }
	s.adjust = btopAdjust
	s.SetContext(ctx)
	return s
}

// btopAdjust applies a left/right/space change to the focused field. Mirrors
// the legacy handleDeepDiveKey behavior exactly (left/right step the value or
// cycle the option; space toggles the temperature field).
func btopAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 0:
			opts := []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}
			cfg.BtopTheme = cycleOption(opts, cfg.BtopTheme, fwd)
		case 1:
			if fwd {
				if cfg.BtopUpdateMs < 10000 {
					cfg.BtopUpdateMs += 500
				}
			} else if cfg.BtopUpdateMs > 500 {
				cfg.BtopUpdateMs -= 500
			}
		case 3:
			opts := []string{"braille", "block", "tty"}
			cfg.BtopGraphType = cycleOption(opts, cfg.BtopGraphType, fwd)
		}
	case " ":
		if a.configFieldIndex == 2 {
			cfg.BtopShowTemp = !cfg.BtopShowTemp
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configBtopScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Btop configuration screen.
func (s *configBtopScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "Btop", "Resource monitor with beautiful TUI")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(55))

	// Theme
	rec.field(0)
	rec.write(renderFieldLabel("Theme", a.configFieldIndex == 0))
	rec.write(renderOptionSelector(
		[]string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"},
		[]string{"Auto", "Dracula", "Gruvbox", "Nord", "Tokyo"},
		cfg.BtopTheme,
		a.configFieldIndex == 0,
	))
	rec.write("\n\n")

	// Update interval
	rec.field(1)
	rec.write(renderFieldLabel("Update Interval", a.configFieldIndex == 1))
	intervalStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if a.configFieldIndex == 1 {
		intervalStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	}
	rec.write(fmt.Sprintf("    ◀ %s ▶", intervalStyle.Render(fmt.Sprintf("%dms", cfg.BtopUpdateMs))))
	rec.write("\n\n")

	// Show temperature
	rec.field(2)
	rec.write(renderFieldLabel("Show CPU Temp", a.configFieldIndex == 2))
	rec.write(renderToggle(cfg.BtopShowTemp, a.configFieldIndex == 2))
	rec.write("\n\n")

	// Graph type
	rec.field(3)
	rec.write(renderFieldLabel("Graph Type", a.configFieldIndex == 3))
	rec.write(renderOptionSelector(
		[]string{"braille", "block", "tty"},
		[]string{"Braille", "Block", "TTY"},
		cfg.BtopGraphType,
		a.configFieldIndex == 3,
	))

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
