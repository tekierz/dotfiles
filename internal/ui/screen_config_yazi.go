package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configYaziScreen is the migrated ScreenHandler for the Yazi config screen.
// Navigation + back are inherited from configFieldNav.
//
// Fields: 0=keymap, 1=show hidden, 2=preview mode, 3=sort order,
// 4=reverse sort, 5=line metadata, 6=scroll offset.
type configYaziScreen struct {
	configFieldNav
}

// NewConfigYaziScreen creates a new Yazi config screen handler.
func NewConfigYaziScreen(ctx *ScreenContext) *configYaziScreen {
	s := &configYaziScreen{}
	s.id = ScreenConfigYazi
	s.maxField = func(*App) int { return 6 }
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
		case 3:
			opts := []string{"alphabetical", "modified", "size", "natural"}
			cfg.YaziSortBy = cycleOption(opts, cfg.YaziSortBy, fwd)
		case 5:
			opts := []string{"size", "permissions", "mtime", "none"}
			cfg.YaziLineMode = cycleOption(opts, cfg.YaziLineMode, fwd)
		case 6:
			if fwd && cfg.YaziScrollOff < 20 {
				cfg.YaziScrollOff++
			} else if !fwd && cfg.YaziScrollOff > 0 {
				cfg.YaziScrollOff--
			}
		}
	case " ":
		switch a.configFieldIndex {
		case 1:
			cfg.YaziShowHidden = !cfg.YaziShowHidden
		case 4:
			cfg.YaziSortReverse = !cfg.YaziSortReverse
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
	rec.write("\n\n")

	rec.field(3)
	rec.write(renderFieldLabel("Sort By", a.configFieldIndex == 3))
	rec.write(renderOptionSelector(
		[]string{"alphabetical", "modified", "size", "natural"},
		[]string{"Alphabetical", "Modified", "Size", "Natural"},
		cfg.YaziSortBy,
		a.configFieldIndex == 3,
	))
	rec.write("\n\n")

	rec.field(4)
	rec.write(renderFieldLabel("Reverse Sort", a.configFieldIndex == 4))
	rec.write(renderToggle(cfg.YaziSortReverse, a.configFieldIndex == 4))
	rec.write("\n\n")

	rec.field(5)
	rec.write(renderFieldLabel("Line Metadata", a.configFieldIndex == 5))
	rec.write(renderOptionSelector(
		[]string{"size", "permissions", "mtime", "none"},
		[]string{"Size", "Permissions", "Modified", "None"},
		cfg.YaziLineMode,
		a.configFieldIndex == 5,
	))
	rec.write("\n\n")

	rec.field(6)
	rec.write(renderFieldLabel("Scroll Offset", a.configFieldIndex == 6))
	offsetStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if a.configFieldIndex == 6 {
		offsetStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	}
	rec.write(fmt.Sprintf("    ◀ %s ▶", offsetStyle.Render(fmt.Sprintf("%d lines", cfg.YaziScrollOff))))

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
