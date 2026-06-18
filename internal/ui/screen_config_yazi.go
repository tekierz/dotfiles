package ui

import (
	"strings"

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
	var content strings.Builder

	keymapFocused := a.configFieldIndex == 0
	content.WriteString(renderFieldLabel("Keymap Style", keymapFocused))
	content.WriteString(renderOptionSelector(
		[]string{"vim", "emacs"},
		[]string{"Vim (hjkl)", "Emacs (arrows)"},
		cfg.YaziKeymap,
		keymapFocused,
	))
	content.WriteString("\n\n")

	hiddenFocused := a.configFieldIndex == 1
	content.WriteString(renderFieldLabel("Show Hidden Files", hiddenFocused))
	content.WriteString(renderToggle(cfg.YaziShowHidden, hiddenFocused))
	content.WriteString("\n\n")

	previewFocused := a.configFieldIndex == 2
	content.WriteString(renderFieldLabel("File Preview", previewFocused))
	content.WriteString(renderOptionSelector(
		[]string{"auto", "always", "never"},
		[]string{"Auto", "Always", "Never"},
		cfg.YaziPreviewMode,
		previewFocused,
	))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(50)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
