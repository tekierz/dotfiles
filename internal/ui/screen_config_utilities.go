package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

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
		installed, _ := a.installationUtilityInstalled(id)
		if !installed {
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
	if a.installCacheLoading {
		return installStatusLoadingView(a, width, height)
	}

	title := renderConfigTitle("", "Utilities", "Helper tools from tekierz/homebrew-tap")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(60))

	for i, util := range utilityItems {
		rec.field(i)
		focused := a.utilityIndex == i
		enabled := cfg.Utilities[util.id]
		installed, _ := a.installationUtilityInstalled(util.id)

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
			nameStyle.Render(fmt.Sprintf("%-8s", util.name)),
			suffix,
			descStyle.Render(util.desc),
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
