package ui

import (
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

	title := renderConfigTitle("", "GUI Apps", "Desktop applications (cross-platform)")

	items := make([]installListItem, len(guiAppItems))
	for i, app := range guiAppItems {
		items[i] = installListItem{id: app.id, name: app.name, desc: app.desc}
	}

	return renderInstallStateList(a, width, height, a.deepDiveBoxWidth(70), 14, title, items, a.guiAppIndex,
		func(id string) bool { return a.deepDiveConfig.GUIApps[id] },
		func(id string) bool { return a.manageInstalled[id] },
		s.footerInstalled())
}
