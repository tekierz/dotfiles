package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configFzfScreen is the migrated ScreenHandler for the FZF config screen.
// Navigation + back are inherited from configFieldNav.
//
// Fields: 0=preview (toggle), 1=height (slider), 2=layout.
type configFzfScreen struct {
	configFieldNav
}

// NewConfigFzfScreen creates a new FZF config screen handler.
func NewConfigFzfScreen(ctx *ScreenContext) *configFzfScreen {
	s := &configFzfScreen{}
	s.id = ScreenConfigFzf
	s.maxField = func(*App) int { return 2 }
	s.adjust = fzfAdjust
	s.SetContext(ctx)
	return s
}

func fzfAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 1:
			if fwd {
				if cfg.FzfHeight < 100 {
					cfg.FzfHeight += 10
				}
			} else if cfg.FzfHeight > 20 {
				cfg.FzfHeight -= 10
			}
		case 2:
			opts := []string{"reverse", "default", "reverse-list"}
			cfg.FzfLayout = cycleOption(opts, cfg.FzfLayout, fwd)
		}
	case " ":
		if a.configFieldIndex == 0 {
			cfg.FzfPreview = !cfg.FzfPreview
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configFzfScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the FZF configuration screen.
func (s *configFzfScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "FZF", "Fuzzy finder settings")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(50))

	rec.field(0)
	previewFocused := a.configFieldIndex == 0
	rec.write(renderFieldLabel("File Preview", previewFocused))
	rec.write(renderToggle(cfg.FzfPreview, previewFocused))
	rec.write("\n\n")

	rec.field(1)
	heightFocused := a.configFieldIndex == 1
	rec.write(renderFieldLabel("Window Height", heightFocused))
	rec.write(renderSliderControl(cfg.FzfHeight, 100, 24, heightFocused))
	rec.write("\n\n")

	rec.field(2)
	layoutFocused := a.configFieldIndex == 2
	rec.write(renderFieldLabel("Layout", layoutFocused))
	rec.write(renderOptionSelector(
		[]string{"reverse", "default", "reverse-list"},
		[]string{"Reverse ↑", "Default ↓", "Reverse List"},
		cfg.FzfLayout,
		layoutFocused,
	))

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • space toggle • esc back")
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
