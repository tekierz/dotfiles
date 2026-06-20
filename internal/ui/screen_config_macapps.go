package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

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

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(65))

	for i, app := range macAppItems {
		rec.field(i)
		focused := a.macAppIndex == i
		enabled := cfg.MacApps[app.id]
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
			nameStyle.Render(fmt.Sprintf("%-16s", app.name)),
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
