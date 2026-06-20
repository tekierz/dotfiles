package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configLazyDockerScreen is the migrated ScreenHandler for the LazyDocker config
// screen. It has a single field (mouse mode), so the field cursor never moves;
// navigation + back are still inherited from configFieldNav (maxField == 0).
type configLazyDockerScreen struct {
	configFieldNav
}

// NewConfigLazyDockerScreen creates a new LazyDocker config screen handler.
func NewConfigLazyDockerScreen(ctx *ScreenContext) *configLazyDockerScreen {
	s := &configLazyDockerScreen{}
	s.id = ScreenConfigLazyDocker
	s.maxField = func(*App) int { return 0 }
	s.adjust = lazyDockerAdjust
	s.SetContext(ctx)
	return s
}

// lazyDockerAdjust toggles the only field (mouse mode) on space. Mirrors the
// legacy handleDeepDiveKey behavior, which toggled on space regardless of the
// focused index.
func lazyDockerAdjust(a *App, key string, _ bool) {
	if key == " " {
		a.deepDiveConfig.LazyDockerMouseMode = !a.deepDiveConfig.LazyDockerMouseMode
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configLazyDockerScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the LazyDocker configuration screen. The single field is always
// shown focused (matching the legacy renderConfigLazyDocker).
func (s *configLazyDockerScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "LazyDocker", "Simple terminal UI for Docker")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(50))

	// Mouse mode (always focused)
	rec.field(0)
	rec.write(renderFieldLabel("Mouse Mode", true))
	rec.write(renderToggle(cfg.LazyDockerMouseMode, true))

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
