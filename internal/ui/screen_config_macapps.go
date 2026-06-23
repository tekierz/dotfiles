package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// configMacAppsScreen is the migrated ScreenHandler for the macOS apps selection
// screen. Navigation + back + toggle-guarded-by-install are inherited from
// configListNav (it uses a.macAppIndex for the cursor).
type configMacAppsScreen struct {
	configListNav
}

// macAppItems is the ordered list of macOS app ids/labels shown on this screen.
var macAppItems = []struct {
	id   string
	name string
	desc string
}{
	{"rectangle", "Rectangle", "Window management"},
	{"raycast", "Raycast", "Spotlight replacement"},
	{"stats", "Stats", "System monitor"},
	{"alt-tab", "AltTab", "Window switcher"},
	{"monitor-control", "MonitorControl", "Display brightness"},
	{"mos", "Mos", "Smooth scrolling"},
	{"karabiner", "Karabiner", "Keyboard customizer"},
	{"iina", "IINA", "Media player"},
	{"the-unarchiver", "The Unarchiver", "Archive utility"},
	{"appcleaner", "AppCleaner", "App uninstaller"},
}

// NewConfigMacAppsScreen creates a new macOS apps config screen handler.
func NewConfigMacAppsScreen(ctx *ScreenContext) *configMacAppsScreen {
	s := &configMacAppsScreen{}
	s.id = ScreenConfigMacApps
	s.itemIDs = make([]string, len(macAppItems))
	for i, app := range macAppItems {
		s.itemIDs[i] = app.id
	}
	s.index = func(a *App) int { return a.macAppIndex }
	s.setIndex = func(a *App, v int) { a.macAppIndex = v }
	s.toggle = func(a *App, id string) {
		// Don't allow toggling if already installed.
		if !a.manageInstalled[id] {
			a.deepDiveConfig.MacApps[id] = !a.deepDiveConfig.MacApps[id]
		}
	}
	s.SetContext(ctx)
	return s
}

// Update delegates to the shared list-navigation handler.
func (s *configMacAppsScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the macOS apps selection screen.
func (s *configMacAppsScreen) View(width, height int) string {
	a := s.App()

	// Ensure install status is cached.
	a.ensureInstallCache()

	title := renderConfigTitle("", "macOS Apps", "Optional productivity applications")

	items := make([]installListItem, len(macAppItems))
	for i, app := range macAppItems {
		items[i] = installListItem{id: app.id, name: app.name, desc: app.desc}
	}

	return renderInstallStateList(a, width, height, a.deepDiveBoxWidth(65), 16, title, items, a.macAppIndex,
		func(id string) bool { return a.deepDiveConfig.MacApps[id] },
		func(id string) bool { return a.manageInstalled[id] },
		s.footerInstalled())
}
