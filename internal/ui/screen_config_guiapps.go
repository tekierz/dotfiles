package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configGUIAppsScreen is the migrated ScreenHandler for the GUI apps selection
// screen. Navigation + back + toggle-guarded-by-install are inherited from
// configListNav (it uses a.guiAppIndex for the cursor).
type configGUIAppsScreen struct {
	configListNav
}

// guiAppItems is the ordered list of GUI app ids/labels shown on this screen.
var guiAppItems = []struct {
	id   string
	name string
	desc string
}{
	{"zen-browser", "Zen Browser", "Privacy-focused browser based on Firefox"},
	{"cursor", "Cursor", "AI-first code editor"},
	{"sunshine", "Sunshine", "Game streaming host (NVIDIA GameStream)"},
	{"moonlight", "Moonlight", "Game streaming client"},
	{"lm-studio", "LM Studio", "Run local LLMs"},
	{"obs", "OBS Studio", "Streaming and recording software"},
}

// NewConfigGUIAppsScreen creates a new GUI apps config screen handler.
func NewConfigGUIAppsScreen(ctx *ScreenContext) *configGUIAppsScreen {
	s := &configGUIAppsScreen{}
	s.id = ScreenConfigGUIApps
	s.itemIDs = make([]string, len(guiAppItems))
	for i, app := range guiAppItems {
		s.itemIDs[i] = app.id
	}
	s.index = func(a *App) int { return a.guiAppIndex }
	s.setIndex = func(a *App, v int) { a.guiAppIndex = v }
	s.toggle = func(a *App, id string) {
		// Don't allow toggling if already installed.
		if !a.manageInstalled[id] {
			a.deepDiveConfig.GUIApps[id] = !a.deepDiveConfig.GUIApps[id]
		}
	}
	s.SetContext(ctx)
	return s
}

// Update delegates to the shared list-navigation handler.
func (s *configGUIAppsScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the GUI apps selection screen.
func (s *configGUIAppsScreen) View(width, height int) string {
	a := s.App()
	if a.installCacheLoading {
		return installStatusLoadingView(a, width, height)
	}

	title := renderConfigTitle("", "GUI Apps", "Desktop applications (cross-platform)")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(70))

	for i, app := range guiAppItems {
		rec.field(i)
		focused := a.guiAppIndex == i
		enabled := cfg.GUIApps[app.id]
		installed := a.manageInstalled[app.id]

		cursor := "  "
		if focused && !installed {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused && installed {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderCheckboxInlineWithInstallState(enabled, focused, installed)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			descStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		} else if focused {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		suffix := ""
		if installed {
			suffix = lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)")
		}

		rec.write(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-14s", app.name)),
			suffix,
			descStyle.Render(app.desc),
		))
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footerInstalled()
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
