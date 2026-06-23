package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// configUtilitiesScreen is the migrated ScreenHandler for the Helper Scripts
// (utilities) selection screen. Navigation + back + toggle-guarded-by-install
// are inherited from configListNav (it uses a.utilityIndex for the cursor).
type configUtilitiesScreen struct {
	configListNav
}

// utilityItems is the ordered list of utility ids/labels shown on this screen.
var utilityItems = []struct {
	id   string
	name string
	desc string
}{
	{"hk", "hk", "Hotkey reference viewer"},
	{"caff", "caff", "Keep system awake utility"},
	{"sshh", "sshh", "SSH config helper"},
}

// NewConfigUtilitiesScreen creates a new utilities config screen handler.
func NewConfigUtilitiesScreen(ctx *ScreenContext) *configUtilitiesScreen {
	s := &configUtilitiesScreen{}
	s.id = ScreenConfigUtilities
	s.itemIDs = make([]string, len(utilityItems))
	for i, u := range utilityItems {
		s.itemIDs[i] = u.id
	}
	s.index = func(a *App) int { return a.utilityIndex }
	s.setIndex = func(a *App, v int) { a.utilityIndex = v }
	s.toggle = func(a *App, id string) {
		// Don't allow toggling if already installed.
		if !a.manageInstalled[id] {
			a.deepDiveConfig.Utilities[id] = !a.deepDiveConfig.Utilities[id]
		}
	}
	s.SetContext(ctx)
	return s
}

// Update delegates to the shared list-navigation handler.
func (s *configUtilitiesScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the utilities selection screen.
func (s *configUtilitiesScreen) View(width, height int) string {
	a := s.App()

	// Ensure install status is cached.
	a.ensureInstallCache()

	title := renderConfigTitle("", "Utilities", "Helper tools from tekierz/homebrew-tap")

	items := make([]installListItem, len(utilityItems))
	for i, util := range utilityItems {
		items[i] = installListItem{id: util.id, name: util.name, desc: util.desc}
	}

	return renderInstallStateList(a, width, height, a.deepDiveBoxWidth(60), 8, title, items, a.utilityIndex,
		func(id string) bool { return a.deepDiveConfig.Utilities[id] },
		func(id string) bool { return a.manageInstalled[id] },
		s.footerInstalled())
}
