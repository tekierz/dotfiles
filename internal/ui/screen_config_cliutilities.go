package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

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
	if a.installCacheLoading {
		return installStatusLoadingView(a, width, height)
	}

	title := renderConfigTitle("󰘳", "CLI Utilities", "Essential command-line replacements")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(65))

	for i, util := range cliUtilityItems {
		rec.field(i)
		focused := a.cliUtilityIndex == i
		enabled := cfg.CLIUtilities[util.id]
		installed := a.manageInstalled[util.id]

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
			nameStyle.Render(fmt.Sprintf("%-10s", util.name)),
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
