package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configGlowScreen is the migrated ScreenHandler for the Glow config screen.
// Navigation + the back-to-menu transition are inherited from configFieldNav;
// only the field layout (View) and value adjustment (adjust) are
// screen-specific.
//
// Fields: style, pager, width, mouse, all files, line numbers, newlines.
type configGlowScreen struct {
	configFieldNav
	editing    bool
	editField  int
	editValue  string
	editCursor int
	editError  string
}

// NewConfigGlowScreen creates a new Glow config screen handler.
func NewConfigGlowScreen(ctx *ScreenContext) *configGlowScreen {
	s := &configGlowScreen{}
	s.id = ScreenConfigGlow
	s.maxField = func(*App) int { return 6 }
	s.adjust = glowAdjust
	s.SetContext(ctx)
	return s
}

// glowAdjust applies a left/right change to the focused field. Mirrors the
// Style arrows explicitly replace custom paths with a known preset. Pager is a
// boolean setting; $PAGER, not this file, selects the pager command.
func glowAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 0:
			opts := []string{"auto", "ascii", "dark", "dracula", "tokyo-night", "light", "notty", "pink"}
			cfg.GlowStyle = cycleOption(opts, cfg.GlowStyle, fwd)
		case 1:
			opts := []string{"auto", "never"}
			cfg.GlowPager = cycleOption(opts, cfg.GlowPager, fwd)
		case 2:
			if fwd {
				if cfg.GlowWidth < math.MaxInt {
					cfg.GlowWidth++
				}
			} else if cfg.GlowWidth > 0 {
				cfg.GlowWidth--
			}
		default:
			if a.configFieldIndex >= 3 && a.configFieldIndex <= 6 {
				toggleGlowField(cfg, a.configFieldIndex)
			}
		}
	case " ":
		if a.configFieldIndex == 1 {
			if cfg.GlowPager == "auto" {
				cfg.GlowPager = "never"
			} else {
				cfg.GlowPager = "auto"
			}
		}
		if a.configFieldIndex >= 3 && a.configFieldIndex <= 6 {
			toggleGlowField(cfg, a.configFieldIndex)
		}
	}
}

func toggleGlowField(cfg *DeepDiveConfig, index int) {
	switch index {
	case 3:
		cfg.GlowMouse = !cfg.GlowMouse
	case 4:
		cfg.GlowAll = !cfg.GlowAll
	case 5:
		cfg.GlowShowLineNumbers = !cfg.GlowShowLineNumbers
	case 6:
		cfg.GlowPreserveNewLines = !cfg.GlowPreserveNewLines
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configGlowScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	if s.editing {
		if key, ok := msg.(tea.KeyMsg); ok {
			s.updateEdit(key)
		}
		return s, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() == "e" && s.App().configFieldIndex == 2 {
			s.startEdit()
			return s, nil
		}
	}
	return s, s.handleMsg(msg)
}

func (s *configGlowScreen) startEdit() {
	s.editing = true
	s.editField = s.App().configFieldIndex
	s.editError = ""
	s.editValue = strconv.Itoa(s.App().deepDiveConfig.GlowWidth)
	s.editCursor = len([]rune(s.editValue))
}
func (s *configGlowScreen) updateEdit(key tea.KeyMsg) {
	runes := []rune(s.editValue)
	switch key.String() {
	case "esc":
		s.editing = false
		s.editError = ""
		return
	case "left":
		if s.editCursor > 0 {
			s.editCursor--
		}
		return
	case "right":
		if s.editCursor < len(runes) {
			s.editCursor++
		}
		return
	case "home":
		s.editCursor = 0
		return
	case "end":
		s.editCursor = len(runes)
		return
	case "backspace":
		if s.editCursor > 0 {
			runes = append(runes[:s.editCursor-1], runes[s.editCursor:]...)
			s.editCursor--
		}
	case "delete":
		if s.editCursor < len(runes) {
			runes = append(runes[:s.editCursor], runes[s.editCursor+1:]...)
		}
	case "enter":
		n, err := strconv.ParseInt(strings.TrimSpace(s.editValue), 10, strconv.IntSize)
		if err != nil || n < 0 {
			s.editError = "Width must be a non-negative integer"
			return
		}
		s.App().deepDiveConfig.GlowWidth = int(n)
		s.editing = false
		s.editError = ""
		return
	default:
		if key.Type == tea.KeyRunes && len(key.Runes) > 0 {
			next := make([]rune, 0, len(runes)+len(key.Runes))
			next = append(next, runes[:s.editCursor]...)
			next = append(next, key.Runes...)
			next = append(next, runes[s.editCursor:]...)
			runes = next
			s.editCursor += len(key.Runes)
		} else {
			return
		}
	}
	s.editValue = string(runes)
	s.editError = ""
}

func (s *configGlowScreen) editDisplay() string {
	r := []rune(s.editValue)
	cursor := clampInt(s.editCursor, 0, len(r))
	return string(r[:cursor]) + "█" + string(r[cursor:])
}

// View renders the Glow configuration screen.
func (s *configGlowScreen) View(width, height int) string {
	a := s.App()
	compact := height < 36
	title := renderConfigTitle("", "Glow", "Render markdown on the CLI")
	if compact {
		title = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("Glow")
	}

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(55))
	descriptionStyle := lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true)
	separator := "\n\n"
	if compact {
		separator = "\n"
	}

	// Style
	rec.field(0)
	rec.write(renderFieldLabel("Style", a.configFieldIndex == 0))
	styleValue := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if a.configFieldIndex == 0 {
		styleValue = styleValue.Foreground(ColorCyan).Bold(true)
	}
	styleDisplay := cfg.GlowStyle
	styleDisplay = truncatePlain(styleDisplay, rec.innerWidth-8)
	rec.write(fmt.Sprintf("    ◀ %s ▶", styleValue.Render(styleDisplay)))
	if !compact {
		rec.write("\n    " + descriptionStyle.Render("8 built-ins • imported custom paths preserved read-only until replaced"))
	}
	rec.write(separator)

	// Pager
	rec.field(1)
	rec.write(renderFieldLabel("Use Pager", a.configFieldIndex == 1))
	rec.write(renderOptionSelector(
		[]string{"auto", "never"},
		[]string{"Enabled", "Disabled"},
		cfg.GlowPager,
		a.configFieldIndex == 1,
	))
	if !compact {
		rec.write("\n    " + descriptionStyle.Render("CLI file rendering only • $PAGER selects the command"))
	}
	rec.write(separator)

	// Width
	rec.field(2)
	rec.write(renderFieldLabel("Width (e exact)", a.configFieldIndex == 2))
	widthStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if a.configFieldIndex == 2 {
		widthStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	}
	widthValue := fmt.Sprintf("%d chars", cfg.GlowWidth)
	if cfg.GlowWidth == 0 {
		widthValue = "Auto (max 120; fallback 80)"
	}
	if s.editing && s.editField == 2 {
		widthValue = s.editDisplay()
	}
	rec.write(fmt.Sprintf("    ◀ %s ▶", widthStyle.Render(widthValue)))
	rec.write(separator)

	// Mouse
	rec.field(3)
	rec.write(renderFieldLabel("Mouse", a.configFieldIndex == 3))
	rec.write(renderToggle(cfg.GlowMouse, a.configFieldIndex == 3))
	if !compact {
		rec.write("\n    " + descriptionStyle.Render("Glow TUI only"))
	}
	rec.write(separator)

	rec.field(4)
	rec.write(renderFieldLabel("Show All Files", a.configFieldIndex == 4))
	rec.write(renderToggle(cfg.GlowAll, a.configFieldIndex == 4))
	if !compact {
		rec.write("\n    " + descriptionStyle.Render("Hidden and ignored files in the Glow TUI"))
	}
	rec.write(separator)
	rec.field(5)
	rec.write(renderFieldLabel("Line Numbers", a.configFieldIndex == 5))
	rec.write(renderToggle(cfg.GlowShowLineNumbers, a.configFieldIndex == 5))
	if !compact {
		rec.write("\n    " + descriptionStyle.Render("Glow TUI only"))
	}
	rec.write(separator)
	rec.field(6)
	rec.write(renderFieldLabel("Preserve Newlines", a.configFieldIndex == 6))
	rec.write(renderToggle(cfg.GlowPreserveNewLines, a.configFieldIndex == 6))
	if !compact {
		rec.write("\n    " + descriptionStyle.Render("TUI setting; v2.1.2 CLI always preserves newlines"))
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	if !s.editing && a.configFieldIndex == 2 {
		help = HelpStyle.Render("e edit exact width • enter/esc back")
	}
	if s.editing {
		help = HelpStyle.Render("type to edit • ←→ cursor • enter commit • esc cancel")
		if s.editError != "" {
			help = lipgloss.JoinVertical(lipgloss.Center, lipgloss.NewStyle().Foreground(ColorYellow).Render(s.editError), help)
		}
	}
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
