package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configYaziScreen is the migrated ScreenHandler for the Yazi config screen.
// Navigation + back are inherited from configFieldNav.
//
// Fields: 0=keymap, 1=show hidden (toggle), 2=preview mode.
type configYaziScreen struct {
	configFieldNav
}

// NewConfigYaziScreen creates a new Yazi config screen handler.
func NewConfigYaziScreen(ctx *ScreenContext) *configYaziScreen {
	s := &configYaziScreen{}
	s.id = ScreenConfigYazi
	s.maxField = func(*App) int { return 2 }
	s.adjust = yaziAdjust
	s.SetContext(ctx)
	return s
}

func yaziAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 0:
			if cfg.YaziKeymap == "vim" {
				cfg.YaziKeymap = "emacs"
			} else {
				cfg.YaziKeymap = "vim"
			}
		case 2:
			opts := []string{"auto", "always", "never"}
			cfg.YaziPreviewMode = cycleOption(opts, cfg.YaziPreviewMode, fwd)
		}
	case " ":
		if a.configFieldIndex == 1 {
			cfg.YaziShowHidden = !cfg.YaziShowHidden
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configYaziScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Yazi configuration screen.
func (s *configYaziScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("󰉋", "Yazi", "File manager settings")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(50))

	rec.field(0)
	keymapFocused := a.configFieldIndex == 0
	rec.write(renderFieldLabel("Keymap Style", keymapFocused))
	rec.write(renderOptionSelector(
		[]string{"vim", "emacs"},
		[]string{"Vim (hjkl)", "Emacs (arrows)"},
		cfg.YaziKeymap,
		keymapFocused,
	))
	rec.write("\n\n")

	rec.field(1)
	hiddenFocused := a.configFieldIndex == 1
	rec.write(renderFieldLabel("Show Hidden Files", hiddenFocused))
	rec.write(renderToggle(cfg.YaziShowHidden, hiddenFocused))
	rec.write("\n\n")

	rec.field(2)
	previewFocused := a.configFieldIndex == 2
	rec.write(renderFieldLabel("File Preview", previewFocused))
	rec.write(renderOptionSelector(
		[]string{"auto", "always", "never"},
		[]string{"Auto", "Always", "Never"},
		cfg.YaziPreviewMode,
		previewFocused,
	))

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
