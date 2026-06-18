package ui

import (
	"strings"

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
	var content strings.Builder

	previewFocused := a.configFieldIndex == 0
	content.WriteString(renderFieldLabel("File Preview", previewFocused))
	content.WriteString(renderToggle(cfg.FzfPreview, previewFocused))
	content.WriteString("\n\n")

	heightFocused := a.configFieldIndex == 1
	content.WriteString(renderFieldLabel("Window Height", heightFocused))
	content.WriteString(renderSliderControl(cfg.FzfHeight, 100, 24, heightFocused))
	content.WriteString("\n\n")

	layoutFocused := a.configFieldIndex == 2
	content.WriteString(renderFieldLabel("Layout", layoutFocused))
	content.WriteString(renderOptionSelector(
		[]string{"reverse", "default", "reverse-list"},
		[]string{"Reverse ↑", "Default ↓", "Reverse List"},
		cfg.FzfLayout,
		layoutFocused,
	))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(50)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • space toggle • esc back")

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
