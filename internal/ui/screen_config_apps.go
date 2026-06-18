package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configAppsScreen is the migrated ScreenHandler for ScreenConfigApps.
//
// ScreenConfigApps is a vestigial screen id: it is referenced only by
// GetToolConfigScreen (the legacy "apps" tool->screen mapping) and previously
// appeared in the legacy mouse dispatch. It never had a dedicated render
// function (the legacy App.View() had no case for it, so it rendered the
// "Unknown screen" fallback) nor a key handler (handleKey had no case, so input
// was a no-op). This handler preserves that behavior faithfully — there are no
// fields to configure — while routing esc/enter back to the deep-dive menu so
// the screen is no longer an orphan in the legacy switches.
type configAppsScreen struct {
	BaseScreen
}

// NewConfigAppsScreen creates a new apps config screen handler.
func NewConfigAppsScreen(ctx *ScreenContext) *configAppsScreen {
	s := &configAppsScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *configAppsScreen) ID() Screen { return ScreenConfigApps }

// Init returns any initial commands (none on entry).
func (s *configAppsScreen) Init() tea.Cmd { return nil }

// Update handles input: quit on ctrl+c/q, otherwise esc/enter returns to the
// deep-dive menu. There are no fields, so other keys are no-ops (matching the
// legacy no-op handling).
func (s *configAppsScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "q":
			return s, tea.Quit
		case "esc", "enter":
			return s, NavigateTo(ScreenDeepDiveMenu)
		}
	}
	return s, nil
}

// View renders the (content-free) apps screen. The legacy screen had no render
// function; this provides an honest placeholder rather than the old "Unknown
// screen" fallback.
func (s *configAppsScreen) View(width, height int) string {
	title := renderConfigTitle("", "Apps", "Application selection")
	help := HelpStyle.Render("enter/esc back")

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", help),
	)
}
