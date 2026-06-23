package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// configCLIUtilitiesScreen is the migrated ScreenHandler for the CLI utilities
// selection screen. Navigation + back + toggle-guarded-by-install are inherited
// from configListNav (it uses a.cliUtilityIndex for the cursor).
type configCLIUtilitiesScreen struct {
	configListNav
}

// cliUtilityItems is the ordered list of utility ids/labels shown on this screen.
var cliUtilityItems = []struct {
	id   string
	name string
	desc string
}{
	{"bat", "bat", "cat with syntax highlighting"},
	{"eza", "eza", "Modern ls replacement"},
	{"zoxide", "zoxide", "Smarter cd command"},
	{"ripgrep", "ripgrep", "Fast grep replacement"},
	{"fd", "fd", "Fast find replacement"},
	{"delta", "delta", "Beautiful git diffs"},
	{"fswatch", "fswatch", "File system watcher"},
}

// NewConfigCLIUtilitiesScreen creates a new CLI utilities config screen handler.
func NewConfigCLIUtilitiesScreen(ctx *ScreenContext) *configCLIUtilitiesScreen {
	s := &configCLIUtilitiesScreen{}
	s.id = ScreenConfigCLIUtilities
	s.itemIDs = make([]string, len(cliUtilityItems))
	for i, util := range cliUtilityItems {
		s.itemIDs[i] = util.id
	}
	s.index = func(a *App) int { return a.cliUtilityIndex }
	s.setIndex = func(a *App, v int) { a.cliUtilityIndex = v }
	s.toggle = func(a *App, id string) {
		// Don't allow toggling if already installed.
		if !a.manageInstalled[id] {
			a.deepDiveConfig.CLIUtilities[id] = !a.deepDiveConfig.CLIUtilities[id]
		}
	}
	s.SetContext(ctx)
	return s
}

// Update delegates to the shared list-navigation handler.
func (s *configCLIUtilitiesScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the CLI utilities selection screen.
func (s *configCLIUtilitiesScreen) View(width, height int) string {
	a := s.App()

	// Ensure install status is cached.
	a.ensureInstallCache()

	title := renderConfigTitle("󰘳", "CLI Utilities", "Essential command-line replacements")

	items := make([]installListItem, len(cliUtilityItems))
	for i, util := range cliUtilityItems {
		items[i] = installListItem{id: util.id, name: util.name, desc: util.desc}
	}

	return renderInstallStateList(a, width, height, a.deepDiveBoxWidth(65), 10, title, items, a.cliUtilityIndex,
		func(id string) bool { return a.deepDiveConfig.CLIUtilities[id] },
		func(id string) bool { return a.manageInstalled[id] },
		s.footerInstalled())
}
