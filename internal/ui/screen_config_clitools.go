package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configCLIToolsScreen is the migrated ScreenHandler for the CLI tools selection
// screen. Navigation + back + toggle-guarded-by-install are inherited from
// configListNav (it uses a.cliToolIndex for the cursor).
//
// Note: the navigable/toggleable item list (cliToolItems) is the four
// homebrew-installable tools. The Claude Code row is rendered for context but is
// configured on its own screen (ScreenConfigClaudeCode), so it is intentionally
// not part of itemIDs — this matches the legacy handleDeepDiveKey, which only
// navigated/toggled lazygit/lazydocker/btop/glow.
type configCLIToolsScreen struct {
	configListNav
}

// cliToolItems is the ordered list of CLI tool rows rendered on this screen.
// Only the first four (NavigableCLITools) participate in cursor navigation and
// toggling; claude-code is shown for context only.
var cliToolItems = []struct {
	id   string
	name string
	desc string
}{
	{"lazygit", "LazyGit", "Simple terminal UI for Git"},
	{"lazydocker", "LazyDocker", "Simple terminal UI for Docker"},
	{"btop", "btop", "Resource monitor with TUI"},
	{"glow", "Glow", "Render markdown on the CLI"},
	{"claude-code", "Claude Code", "AI-powered coding assistant (npm)"},
}

// navigableCLIToolCount is the number of cliToolItems rows the cursor can reach
// (excludes the trailing claude-code context row).
const navigableCLIToolCount = 4

// NewConfigCLIToolsScreen creates a new CLI tools config screen handler.
func NewConfigCLIToolsScreen(ctx *ScreenContext) *configCLIToolsScreen {
	s := &configCLIToolsScreen{}
	s.id = ScreenConfigCLITools
	s.itemIDs = make([]string, navigableCLIToolCount)
	for i := 0; i < navigableCLIToolCount; i++ {
		s.itemIDs[i] = cliToolItems[i].id
	}
	s.index = func(a *App) int { return a.cliToolIndex }
	s.setIndex = func(a *App, v int) { a.cliToolIndex = v }
	s.toggle = func(a *App, id string) {
		// Don't allow toggling if already installed.
		if !a.manageInstalled[id] {
			a.deepDiveConfig.CLITools[id] = !a.deepDiveConfig.CLITools[id]
		}
	}
	s.SetContext(ctx)
	return s
}

// Update delegates to the shared list-navigation handler.
func (s *configCLIToolsScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the CLI tools selection screen.
func (s *configCLIToolsScreen) View(width, height int) string {
	a := s.App()
	if a.installCacheLoading {
		return installStatusLoadingView(a, width, height)
	}

	title := renderConfigTitle("", "CLI Tools", "Terminal-based productivity tools")

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(70))

	for i, tool := range cliToolItems {
		// Only the first navigableCLIToolCount rows participate in selection. Mark
		// the trailing claude-code context row with a sentinel index (-1) so it
		// records its own extent (bounding the last navigable field) but a click on
		// it resolves to nothing (the handler rejects negative indices).
		if i < navigableCLIToolCount {
			rec.field(i)
		} else {
			rec.field(-1)
		}
		focused := a.cliToolIndex == i
		enabled := cfg.CLITools[tool.id]
		installed := a.manageInstalled[tool.id]

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
			nameStyle.Render(fmt.Sprintf("%-14s", tool.name)),
			suffix,
			descStyle.Render(tool.desc),
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
