package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/health"
)

// configCLIToolsScreen is the migrated ScreenHandler for the CLI tools selection
// screen. Navigation + back + toggle-guarded-by-install are inherited from
// configListNav (it uses a.cliToolIndex for the cursor).
//
// Claude Code is rendered for context but configured on its dedicated MCP
// screen. Cursor Agent and Hermes remain navigable discovery rows but are inert
// until their typed snapshot observations report supported installation.
type configCLIToolsScreen struct {
	configListNav
}

// cliToolItems is the ordered list of CLI tool rows rendered on this screen.
// Every row except the trailing claude-code context row participates in cursor
// navigation and toggling.
var cliToolItems = []struct {
	id   string
	name string
	desc string
}{
	{"lazygit", "LazyGit", "Simple terminal UI for Git"},
	{"lazydocker", "LazyDocker", "Simple terminal UI for Docker"},
	{"btop", "btop", "Resource monitor with TUI"},
	{"glow", "Glow", "Render markdown on the CLI"},
	{"codex", "Codex", "OpenAI coding agent (reviewed npm install)"},
	{"opencode", "OpenCode", "Open-source coding agent (package manager)"},
	{"pi", "Pi", "Extensible coding agent (npm, scripts disabled)"},
	{"cursor-agent", "Cursor Agent", "verified artifact support pending"},
	{"hermes", "Hermes Agent", "verified artifact and architecture support pending"},
	{"claude-code", "Claude Code", "Configure from the dedicated Claude Code screen"},
}

// navigableCLIToolCount is the number of cliToolItems rows the cursor can reach
// (excludes the trailing claude-code context row).
const navigableCLIToolCount = 9

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
		if cliToolSnapshotSelectable(a, id) {
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
	cache := a.installationSnapshotCacheView()
	if cache.Loading {
		return cliToolLoadingView(a, width, height)
	}

	title := renderConfigTitle("", "CLI Tools", "Terminal-based productivity tools")
	cfg := a.deepDiveConfig
	boxWidth := min(70, max(24, width-8))
	rec := newFieldLayoutRecorder(boxWidth)
	fresh, cacheStatus := cliToolSnapshotFreshness(cache)
	focusedUnavailableReason := ""
	start, end := cliToolViewport(a.cliToolIndex, height)

	for i := start; i < end; i++ {
		tool := cliToolItems[i]
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
		state := cliToolSnapshotProjection(cache, tool.id, tool.desc)
		if !fresh {
			state = cliToolSnapshotState{label: strings.ToLower(cacheStatus)}
		}
		selectable := fresh && state.selectable && i < navigableCLIToolCount
		enabled := selectable && cfg.CLITools[tool.id]
		if focused && state.unavailableReason != "" {
			focusedUnavailableReason = state.unavailableReason
		}

		cursor := "  "
		if focused && selectable {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		} else if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorYellow).Render("▸ ")
		}

		checkbox := renderCheckboxInlineWithInstallState(enabled, focused, state.installed)

		nameStyle := unfocusedStyle
		if state.installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorYellow)
		} else if focused && selectable {
			nameStyle = focusedStyle
		}
		statusStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if focused {
			statusStyle = lipgloss.NewStyle().Foreground(ColorText)
		}
		line := fmt.Sprintf("%s%s %s • %s", cursor, checkbox,
			nameStyle.Render(fmt.Sprintf("%-12s", tool.name)), statusStyle.Render(state.label))
		if tool.desc != "" && tool.desc != state.unavailableReason {
			line += lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" — " + tool.desc)
		}
		rec.write(truncateVisible(line, rec.innerWidth) + "\n")
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := HelpStyle.Render(truncateVisible("↑↓ navigate • space toggle • enter/esc back • text status", max(1, width-2)))
	content := []string{title, "", box}
	if cacheStatus != "" {
		content = append(content, lipgloss.NewStyle().Foreground(ColorYellow).Render(cacheStatus))
	}
	if focusedUnavailableReason != "" {
		content = append(content, lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(focusedUnavailableReason, max(1, width-2))))
	}
	if start <= navigableCLIToolCount && end > navigableCLIToolCount {
		content = append(content, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Configure from the dedicated Claude Code screen"))
	}
	content = append(content, "", help)
	composed := lipgloss.JoinVertical(lipgloss.Center, content...)
	a.configFieldLayout = rec.finalizeComposed(width, height, composed, box, lipgloss.Height(title)+1)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		composed,
	)
}

type cliToolSnapshotState struct {
	label             string
	installed         bool
	selectable        bool
	unavailableReason string
}

func cliToolSnapshotFreshness(cache installationSnapshotCacheView) (bool, string) {
	coherent := cache.Snapshot.Digest() != "" && cache.Generation == cache.Snapshot.Generation()
	switch {
	case cache.Loading:
		return false, "Installation status loading"
	case cache.Error != "":
		return false, installationSnapshotUnavailable
	case cache.Stale:
		return false, "Installation status stale"
	case !cache.Ready:
		return false, "Installation status unknown"
	case !coherent:
		return false, installationSnapshotUnavailable
	default:
		return true, ""
	}
}

func cliToolLoadingView(a *App, width, height int) string {
	spinner := AnimatedSpinnerDots(a.uiFrame)
	label := fmt.Sprintf("%s Installation status loading...", spinner)
	return PlaceWithBackground(width, height, lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render(label))
}

func cliToolSnapshotProjection(cache installationSnapshotCacheView, id, description string) cliToolSnapshotState {
	observation, observed := cache.Snapshot.Tool(id)
	if !observed {
		return cliToolSnapshotState{label: "installation status unknown"}
	}

	presence := observation.Presence()
	installability := observation.Installability()
	switch presence {
	case health.PresencePresent:
		return cliToolSnapshotState{label: "installed", installed: true}
	case health.PresencePartial:
		switch installability {
		case health.InstallabilitySupported:
			return cliToolSnapshotState{label: "partial — repair", selectable: true}
		case health.InstallabilityUnsupported:
			return cliToolUnsupportedState(cache)
		case health.InstallabilityUnknown:
			return cliToolSnapshotState{label: "installation availability unknown"}
		default:
			return cliToolSnapshotState{label: "installation status unknown"}
		}
	case health.PresenceMissing:
		switch installability {
		case health.InstallabilitySupported:
			return cliToolSnapshotState{label: "missing — install", selectable: true}
		case health.InstallabilityUnsupported:
			return cliToolUnsupportedState(cache)
		case health.InstallabilityUnknown:
			return cliToolSnapshotState{label: "installation availability unknown"}
		default:
			return cliToolSnapshotState{label: "installation status unknown"}
		}
	case health.PresenceUnknown:
		switch installability {
		case health.InstallabilitySupported:
			return cliToolSnapshotState{label: "status unknown"}
		case health.InstallabilityUnsupported:
			return cliToolSnapshotState{label: "status unknown — unavailable", unavailableReason: pendingCLIToolReason(id, description)}
		case health.InstallabilityUnknown:
			return cliToolSnapshotState{label: "installation availability unknown"}
		default:
			return cliToolSnapshotState{label: "installation status unknown"}
		}
	default:
		return cliToolSnapshotState{label: "installation status unknown"}
	}
}

func cliToolUnsupportedState(cache installationSnapshotCacheView) cliToolSnapshotState {
	platform := strings.TrimSpace(cache.Snapshot.Platform())
	if platform == "" {
		return cliToolSnapshotState{label: "installation unavailable"}
	}
	return cliToolSnapshotState{label: "installation unavailable on " + platform}
}

func pendingCLIToolReason(id, description string) string {
	if id == "cursor-agent" || id == "hermes" {
		return description
	}
	return ""
}

func cliToolSnapshotSelectable(a *App, id string) bool {
	if a == nil {
		return false
	}
	cache := a.installationSnapshotCacheView()
	fresh, _ := cliToolSnapshotFreshness(cache)
	return fresh && cliToolSnapshotProjection(cache, id, "").selectable
}

func cliToolViewport(focused, height int) (int, int) {
	rowCount := len(cliToolItems)
	visible := rowCount
	if height < 24 {
		// Four rows leave room for focused pending detail and the Claude context
		// note at 60x18 while keeping that trailing context row reachable from the
		// final navigable item.
		visible = min(4, rowCount)
	}
	start := focused - visible/2
	start = max(0, min(start, rowCount-visible))
	return start, start + visible
}
