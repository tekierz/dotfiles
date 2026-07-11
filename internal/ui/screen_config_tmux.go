package ui

import (
	"fmt"

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
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(55))
	fieldIdx := 0

	rec.field(fieldIdx)
	prefixFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Prefix Key", prefixFocused))
	rec.write(renderOptionSelector(
		[]string{"ctrl-a", "ctrl-b", "ctrl-space"},
		[]string{"Ctrl-A", "Ctrl-B", "Ctrl-Space"},
		cfg.TmuxPrefix,
		prefixFocused,
	))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	splitFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Split Pane Keys", splitFocused))
	rec.write(renderOptionSelector(
		[]string{"pipes", "percent"},
		[]string{"| and − (intuitive)", "% and \" (default)"},
		cfg.TmuxSplitBinds,
		splitFocused,
	))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	statusFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Status Bar Position", statusFocused))
	rec.write(renderOptionSelector(
		[]string{"bottom", "top"},
		[]string{"Bottom", "Top"},
		cfg.TmuxStatusBar,
		statusFocused,
	))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	mouseFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Mouse Support", mouseFocused))
	rec.write(renderToggle(cfg.TmuxMouseMode, mouseFocused))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	historyFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("History Limit", historyFocused))
	rec.write(renderOptionSelector(
		[]string{"10000", "25000", "50000", "100000"},
		[]string{"10K", "25K", "50K", "100K"},
		fmt.Sprintf("%d", cfg.TmuxHistoryLimit),
		historyFocused,
	))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	escapeFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Escape Time (ms)", escapeFocused))
	rec.write(renderOptionSelector(
		[]string{"0", "10", "50", "100"},
		[]string{"0ms", "10ms", "50ms", "100ms"},
		fmt.Sprintf("%d", cfg.TmuxEscapeTime),
		escapeFocused,
	))
	rec.write("\n\n")
	fieldIdx++

	rec.field(fieldIdx)
	baseFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Base Index", baseFocused))
	rec.write(renderOptionSelector(
		[]string{"0", "1"},
		[]string{"0 (default)", "1 (starts at 1)"},
		fmt.Sprintf("%d", cfg.TmuxBaseIndex),
		baseFocused,
	))
	fieldIdx++

	// TPM section. The section header is non-field content, so it is written
	// without a field() mark and excluded from every field's hit extent.
	rec.write("\n\n")
	rec.write(sectionHeaderStyle.Render("TPM Plugins"))
	rec.write("\n\n")

	rec.field(fieldIdx)
	tpmFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("Enable TPM", tpmFocused))
	rec.write(renderToggle(cfg.TmuxTPMEnabled, tpmFocused))
	fieldIdx++

	if cfg.TmuxTPMEnabled {
		rec.write("\n\n")

		rec.field(fieldIdx)
		sensibleFocused := a.configFieldIndex == fieldIdx
		rec.write(renderCheckbox("tmux-sensible", cfg.TmuxPluginSensible, sensibleFocused))
		if sensibleFocused {
			rec.write(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Sensible defaults"))
		}
		rec.write("\n")
		fieldIdx++

		rec.field(fieldIdx)
		resurrectFocused := a.configFieldIndex == fieldIdx
		rec.write(renderCheckbox("tmux-resurrect", cfg.TmuxPluginResurrect, resurrectFocused))
		if resurrectFocused {
			rec.write(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Session save/restore"))
		}
		rec.write("\n")
		fieldIdx++

		rec.field(fieldIdx)
		continuumFocused := a.configFieldIndex == fieldIdx
		rec.write(renderCheckbox("tmux-continuum", cfg.TmuxPluginContinuum, continuumFocused))
		if continuumFocused {
			rec.write(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Auto-save sessions"))
		}
		rec.write("\n")
		fieldIdx++

		rec.field(fieldIdx)
		yankFocused := a.configFieldIndex == fieldIdx
		rec.write(renderCheckbox("tmux-yank", cfg.TmuxPluginYank, yankFocused))
		if yankFocused {
			rec.write(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  Clipboard integration"))
		}
		fieldIdx++

		if cfg.TmuxPluginContinuum {
			rec.write("\n\n")
			rec.field(fieldIdx)
			intervalFocused := a.configFieldIndex == fieldIdx
			rec.write(renderFieldLabel("Auto-save Interval", intervalFocused))
			rec.write(renderNumberControl(cfg.TmuxContinuumSaveMin, 60, intervalFocused))
			rec.write(lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" min"))
		}
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return PlaceWithBackground(
		width, height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
