package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configTmuxScreen is the migrated ScreenHandler for the Tmux config screen.
//
// Tmux has a dynamic field set (TPM plugins appear only when TPM is enabled, and
// the continuum interval appears only when continuum is enabled), so it provides
// its own Update to preserve the legacy "skip hidden field" clamping on
// up-navigation. View/back/adjust still reuse the shared configFieldNav so the
// value-adjustment + back transition stay consistent.
//
// Fields: 0=prefix, 1=splits, 2=status, 3=mouse, 4=history, 5=escape, 6=base,
// 7=TPM toggle. If TPM enabled: 8=sensible, 9=resurrect, 10=continuum, 11=yank,
// 12=interval (if continuum).
type configTmuxScreen struct {
	configFieldNav
}

// NewConfigTmuxScreen creates a new Tmux config screen handler.
func NewConfigTmuxScreen(ctx *ScreenContext) *configTmuxScreen {
	s := &configTmuxScreen{}
	s.id = ScreenConfigTmux
	s.maxField = tmuxMaxField
	s.adjust = tmuxAdjust
	s.SetContext(ctx)
	return s
}

// tmuxMaxField returns the highest valid field index given the TPM/continuum state.
func tmuxMaxField(a *App) int {
	maxField := 7 // Fields 0-7 (basic settings + TPM toggle)
	if a.deepDiveConfig.TmuxTPMEnabled {
		maxField = 11 // + plugin toggles (8-11)
		if a.deepDiveConfig.TmuxPluginContinuum {
			maxField = 12 // + continuum interval
		}
	}
	return maxField
}

func tmuxAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 0: // Prefix
			opts := []string{"ctrl-a", "ctrl-b", "ctrl-space"}
			cfg.TmuxPrefix = cycleOption(opts, cfg.TmuxPrefix, fwd)
		case 1: // Split binds
			if cfg.TmuxSplitBinds == "pipes" {
				cfg.TmuxSplitBinds = "percent"
			} else {
				cfg.TmuxSplitBinds = "pipes"
			}
		case 2: // Status bar
			if cfg.TmuxStatusBar == "top" {
				cfg.TmuxStatusBar = "bottom"
			} else {
				cfg.TmuxStatusBar = "top"
			}
		case 4: // History limit
			opts := []string{"10000", "25000", "50000", "100000"}
			current := fmt.Sprintf("%d", cfg.TmuxHistoryLimit)
			cfg.TmuxHistoryLimit = atoi(cycleOption(opts, current, fwd), 50000)
		case 5: // Escape time
			opts := []string{"0", "10", "50", "100"}
			current := fmt.Sprintf("%d", cfg.TmuxEscapeTime)
			cfg.TmuxEscapeTime = atoi(cycleOption(opts, current, fwd), 10)
		case 6: // Base index
			if cfg.TmuxBaseIndex == 0 {
				cfg.TmuxBaseIndex = 1
			} else {
				cfg.TmuxBaseIndex = 0
			}
		case 12: // Continuum interval
			if cfg.TmuxTPMEnabled && cfg.TmuxPluginContinuum {
				if fwd {
					if cfg.TmuxContinuumSaveMin < 60 {
						cfg.TmuxContinuumSaveMin += 5
					}
				} else if cfg.TmuxContinuumSaveMin > 5 {
					cfg.TmuxContinuumSaveMin -= 5
				}
			}
		}
	case " ":
		switch a.configFieldIndex {
		case 3: // Mouse mode
			cfg.TmuxMouseMode = !cfg.TmuxMouseMode
		case 7: // TPM enabled
			cfg.TmuxTPMEnabled = !cfg.TmuxTPMEnabled
		case 8: // tmux-sensible
			cfg.TmuxPluginSensible = !cfg.TmuxPluginSensible
		case 9: // tmux-resurrect
			cfg.TmuxPluginResurrect = !cfg.TmuxPluginResurrect
		case 10: // tmux-continuum
			cfg.TmuxPluginContinuum = !cfg.TmuxPluginContinuum
		case 11: // tmux-yank
			cfg.TmuxPluginYank = !cfg.TmuxPluginYank
		}
	}
}

// Update handles Tmux's dynamic field navigation, preserving the legacy
// hidden-field clamping when TPM/continuum are disabled.
func (s *configTmuxScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	if km, ok := msg.(tea.KeyMsg); ok {
		key := km.String()
		switch key {
		case "ctrl+c", "q":
			return s, tea.Quit
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
				// Skip hidden fields when TPM is disabled.
				if !a.deepDiveConfig.TmuxTPMEnabled && a.configFieldIndex > 7 {
					a.configFieldIndex = 7
				}
				// Skip interval field when continuum is disabled.
				if a.deepDiveConfig.TmuxTPMEnabled && !a.deepDiveConfig.TmuxPluginContinuum && a.configFieldIndex == 12 {
					a.configFieldIndex = 11
				}
			}
			return s, nil
		case "down", "j":
			if a.configFieldIndex < tmuxMaxField(a) {
				a.configFieldIndex++
			}
			return s, nil
		case "esc", "enter":
			_, cmd := s.back()
			return s, cmd
		default:
			fwd := key == "right" || key == "l"
			tmuxAdjust(a, key, fwd)
			return s, nil
		}
	}
	// Mouse + everything else: defer to the shared handler.
	return s, s.handleMsg(msg)
}

// View renders the Tmux configuration screen.
func (s *configTmuxScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "Tmux", "Terminal multiplexer settings")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

	prefixFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Prefix Key", prefixFocused))
	content.WriteString(renderOptionSelector(
		[]string{"ctrl-a", "ctrl-b", "ctrl-space"},
		[]string{"Ctrl-A", "Ctrl-B", "Ctrl-Space"},
		cfg.TmuxPrefix,
		prefixFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	splitFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Split Pane Keys", splitFocused))
	content.WriteString(renderOptionSelector(
		[]string{"pipes", "percent"},
		[]string{"| and − (intuitive)", "% and \" (default)"},
		cfg.TmuxSplitBinds,
		splitFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	statusFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Status Bar Position", statusFocused))
	content.WriteString(renderOptionSelector(
		[]string{"bottom", "top"},
		[]string{"Bottom", "Top"},
		cfg.TmuxStatusBar,
		statusFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	mouseFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Mouse Support", mouseFocused))
	content.WriteString(renderToggle(cfg.TmuxMouseMode, mouseFocused))
	content.WriteString("\n\n")
	fieldIdx++

	historyFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("History Limit", historyFocused))
	content.WriteString(renderOptionSelector(
		[]string{"10000", "25000", "50000", "100000"},
		[]string{"10K", "25K", "50K", "100K"},
		fmt.Sprintf("%d", cfg.TmuxHistoryLimit),
		historyFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	escapeFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Escape Time (ms)", escapeFocused))
	content.WriteString(renderOptionSelector(
		[]string{"0", "10", "50", "100"},
		[]string{"0ms", "10ms", "50ms", "100ms"},
		fmt.Sprintf("%d", cfg.TmuxEscapeTime),
		escapeFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	baseFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Base Index", baseFocused))
	content.WriteString(renderOptionSelector(
		[]string{"0", "1"},
		[]string{"0 (default)", "1 (starts at 1)"},
		fmt.Sprintf("%d", cfg.TmuxBaseIndex),
		baseFocused,
	))
	fieldIdx++

	// TPM section.
	content.WriteString("\n\n")
	content.WriteString(sectionHeaderStyle.Render("TPM Plugins"))
	content.WriteString("\n\n")

	tpmFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Enable TPM", tpmFocused))
	content.WriteString(renderToggle(cfg.TmuxTPMEnabled, tpmFocused))
	fieldIdx++

	if cfg.TmuxTPMEnabled {
		content.WriteString("\n\n")

		sensibleFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-sensible", cfg.TmuxPluginSensible, sensibleFocused))
		if sensibleFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Sensible defaults"))
		}
		content.WriteString("\n")
		fieldIdx++

		resurrectFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-resurrect", cfg.TmuxPluginResurrect, resurrectFocused))
		if resurrectFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Session save/restore"))
		}
		content.WriteString("\n")
		fieldIdx++

		continuumFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-continuum", cfg.TmuxPluginContinuum, continuumFocused))
		if continuumFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Auto-save sessions"))
		}
		content.WriteString("\n")
		fieldIdx++

		yankFocused := a.configFieldIndex == fieldIdx
		content.WriteString(renderCheckbox("tmux-yank", cfg.TmuxPluginYank, yankFocused))
		if yankFocused {
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Clipboard integration"))
		}
		fieldIdx++

		if cfg.TmuxPluginContinuum {
			content.WriteString("\n\n")
			intervalFocused := a.configFieldIndex == fieldIdx
			content.WriteString(renderFieldLabel("Auto-save Interval", intervalFocused))
			content.WriteString(renderNumberControl(cfg.TmuxContinuumSaveMin, 5, 60, intervalFocused))
			content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" min"))
		}
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ select • space toggle • enter/esc back")

	return PlaceWithBackground(
		width, height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
