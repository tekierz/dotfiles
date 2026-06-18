package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configLazyGitScreen is the migrated ScreenHandler for the LazyGit config
// screen. Navigation + the back-to-menu transition are inherited from
// configFieldNav; only the field layout (View) and value adjustment (adjust)
// are screen-specific.
//
// Fields: 0=side-by-side diff (toggle), 1=mouse mode (toggle), 2=theme (option).
type configLazyGitScreen struct {
	configFieldNav
}

// NewConfigLazyGitScreen creates a new LazyGit config screen handler.
func NewConfigLazyGitScreen(ctx *ScreenContext) *configLazyGitScreen {
	s := &configLazyGitScreen{}
	s.id = ScreenConfigLazyGit
	s.maxField = func(*App) int { return 2 }
	s.adjust = lazyGitAdjust
	s.SetContext(ctx)
	return s
}

// lazyGitAdjust applies a left/right/space change to the focused field. The
// toggles (fields 0/1) respond to space; the theme (field 2) cycles on
// left/right (h/l). Mirrors the legacy handleDeepDiveKey behavior exactly.
func lazyGitAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		if a.configFieldIndex == 2 {
			opts := []string{"auto", "dark", "light"}
			cfg.LazyGitTheme = cycleOption(opts, cfg.LazyGitTheme, fwd)
		}
	case " ":
		switch a.configFieldIndex {
		case 0:
			cfg.LazyGitSideBySide = !cfg.LazyGitSideBySide
		case 1:
			cfg.LazyGitMouseMode = !cfg.LazyGitMouseMode
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configLazyGitScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the LazyGit configuration screen.
func (s *configLazyGitScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "LazyGit", "Simple terminal UI for Git commands")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Side-by-side diff
	content.WriteString(renderFieldLabel("Side-by-Side Diff", a.configFieldIndex == 0))
	content.WriteString(renderToggle(cfg.LazyGitSideBySide, a.configFieldIndex == 0))
	content.WriteString("\n\n")

	// Mouse mode
	content.WriteString(renderFieldLabel("Mouse Mode", a.configFieldIndex == 1))
	content.WriteString(renderToggle(cfg.LazyGitMouseMode, a.configFieldIndex == 1))
	content.WriteString("\n\n")

	// Theme
	content.WriteString(renderFieldLabel("Theme", a.configFieldIndex == 2))
	content.WriteString(renderOptionSelector(
		[]string{"auto", "dark", "light"},
		[]string{"Auto", "Dark", "Light"},
		cfg.LazyGitTheme,
		a.configFieldIndex == 2,
	))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(50)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • esc back")

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
