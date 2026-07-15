package ui

import (
	"fmt"
	"strings"

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
	{"iina", "IINA", "Media player"},
	{"appcleaner", "AppCleaner", "App uninstaller"},
	{"t3-code", "T3 Code", "Desktop frontend for coding agents (Homebrew cask)"},
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
		if cliToolSnapshotSelectable(a, id) {
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
	cache := a.installationSnapshotCacheView()
	if cache.Loading {
		return installStatusLoadingView(a, width, height)
	}

	title := renderConfigTitle("", "macOS Apps", "Optional productivity applications")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(65))
	fresh, cacheStatus := cliToolSnapshotFreshness(cache)

	for i, app := range macAppItems {
		rec.field(i)
		focused := a.macAppIndex == i
		state := cliToolSnapshotProjection(cache, app.id, app.desc)
		if !fresh {
			state = cliToolSnapshotState{label: strings.ToLower(cacheStatus)}
		}
		selectable := fresh && state.selectable
		enabled := selectable && cfg.MacApps[app.id]

		cursor := "  "
		if focused && selectable {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderSnapshotInstallCheckbox(enabled, focused, state)

		nameStyle := unfocusedStyle
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if state.installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
		} else if focused && selectable {
			nameStyle = focusedStyle
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		rec.write(fmt.Sprintf("%s%s %s • %s — %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-16s", app.name)),
			lipgloss.NewStyle().Foreground(ColorTextMuted).Render(state.label),
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
