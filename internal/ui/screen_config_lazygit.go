package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configLazyGitScreen is the migrated ScreenHandler for the LazyGit config
// screen. Navigation + the back-to-menu transition are inherited from
// configFieldNav; only the field layout (View) and value adjustment (adjust)
// are screen-specific.
//
// Fields: 0=wide side panel (toggle), 1=mouse mode (toggle), 2=theme,
// 3=paging backend.
type configLazyGitScreen struct {
	configFieldNav
}

// NewConfigLazyGitScreen creates a new LazyGit config screen handler.
func NewConfigLazyGitScreen(ctx *ScreenContext) *configLazyGitScreen {
	s := &configLazyGitScreen{}
	s.id = ScreenConfigLazyGit
	s.maxField = func(*App) int { return 3 }
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
		switch a.configFieldIndex {
		case 2:
			opts := []string{"auto", "dark", "light"}
			cfg.LazyGitTheme = cycleOption(opts, cfg.LazyGitTheme, fwd)
		case 3:
			opts := []string{"delta", "diff-so-fancy", "never"}
			cfg.LazyGitPaging = cycleOption(opts, cfg.LazyGitPaging, fwd)
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
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(50))

	// Side panel width
	rec.field(0)
	rec.write(renderFieldLabel("Wide Side Panel", a.configFieldIndex == 0))
	rec.write(renderToggle(cfg.LazyGitSideBySide, a.configFieldIndex == 0))
	rec.write("\n\n")

	// Mouse mode
	rec.field(1)
	rec.write(renderFieldLabel("Mouse Mode", a.configFieldIndex == 1))
	rec.write(renderToggle(cfg.LazyGitMouseMode, a.configFieldIndex == 1))
	rec.write("\n\n")

	// Theme
	rec.field(2)
	rec.write(renderFieldLabel("Theme", a.configFieldIndex == 2))
	rec.write(renderOptionSelector(
		[]string{"auto", "dark", "light"},
		[]string{"Auto", "Dark", "Light"},
		cfg.LazyGitTheme,
		a.configFieldIndex == 2,
	))
	rec.write("\n\n")

	// Paging backend
	rec.field(3)
	rec.write(renderFieldLabel("Paging", a.configFieldIndex == 3))
	rec.write(renderOptionSelector(
		[]string{"delta", "diff-so-fancy", "never"},
		[]string{"Delta", "diff-so-fancy", "Disabled"},
		cfg.LazyGitPaging,
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
