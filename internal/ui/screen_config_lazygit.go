package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/tools"
)

// configLazyGitScreen edits the four deliberately bounded global LazyGit
// concepts. Arbitrary/custom native YAML is displayed but remains read-only.
// Fields: 0=side-panel fraction, 1=mouse events, 2=color preset, 3=pager preset.
type configLazyGitScreen struct {
	configFieldNav
	editing    bool
	editValue  string
	editCursor int
	editError  string
}

func NewConfigLazyGitScreen(ctx *ScreenContext) *configLazyGitScreen {
	s := &configLazyGitScreen{}
	s.id = ScreenConfigLazyGit
	s.maxField = func(*App) int { return 3 }
	s.adjust = lazyGitAdjust
	s.SetContext(ctx)
	return s
}

func lazyGitAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 2:
			if oneOf(cfg.LazyGitColorPreset, "standard", "light-high-contrast") {
				cfg.LazyGitColorPreset = cycleOption([]string{"standard", "light-high-contrast"}, cfg.LazyGitColorPreset, fwd)
			}
		case 3:
			if oneOf(cfg.LazyGitPagerPreset, "builtin", "delta") {
				cfg.LazyGitPagerPreset = cycleOption([]string{"builtin", "delta"}, cfg.LazyGitPagerPreset, fwd)
			}
		}
	case " ":
		if a.configFieldIndex == 1 {
			cfg.LazyGitMouseEvents = !cfg.LazyGitMouseEvents
		}
	}
}

func (s *configLazyGitScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	blockedReason := lazyGitStandaloneUIBlockReason(s.App())
	if blockedReason != "" && s.editing {
		s.editing = false
		s.editError = ""
	}
	if s.editing {
		if key, ok := msg.(tea.KeyMsg); ok {
			s.updateEdit(key)
		}
		// Mouse and global hotkeys are deliberately inert while editing.
		return s, nil
	}
	if blockedReason != "" {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "e", "left", "right", "h", "l", " ":
				return s, nil
			}
		}
		// Mouse still changes focus through the shared geometry handler, but never
		// mutates a value. Enter still opens the reviewable blocked preview.
		return s, s.handleMsg(msg)
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "e" && s.App().configFieldIndex == 0 {
		s.startEdit()
		return s, nil
	}
	return s, s.handleMsg(msg)
}

func (s *configLazyGitScreen) startEdit() {
	s.editing = true
	s.editValue = s.App().deepDiveConfig.LazyGitSidePanelWidth
	s.editCursor = len([]rune(s.editValue))
	s.editError = ""
}

func (s *configLazyGitScreen) updateEdit(key tea.KeyMsg) {
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
		if err := tools.ValidateLazyGitSidePanelWidth(s.editValue); err != nil {
			s.editError = err.Error()
			return
		}
		s.App().deepDiveConfig.LazyGitSidePanelWidth = s.editValue
		s.editing = false
		s.editError = ""
		return
	default:
		if key.Type != tea.KeyRunes || key.Alt || len(key.Runes) == 0 {
			return
		}
		for _, r := range key.Runes {
			if (r < '0' || r > '9') && r != '.' {
				return
			}
		}
		next := make([]rune, 0, len(runes)+len(key.Runes))
		next = append(next, runes[:s.editCursor]...)
		next = append(next, key.Runes...)
		next = append(next, runes[s.editCursor:]...)
		runes = next
		s.editCursor += len(key.Runes)
	}
	s.editValue = string(runes)
	s.editError = ""
}

func (s *configLazyGitScreen) editDisplay() string {
	runes := []rune(s.editValue)
	cursor := clampInt(s.editCursor, 0, len(runes))
	return string(runes[:cursor]) + "█" + string(runes[cursor:])
}

func lazyGitPresetDisplay(value string, labels map[string]string, focused, readOnly bool) string {
	style := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if focused {
		style = style.Foreground(ColorCyan).Bold(true)
	}
	if label, ok := labels[value]; ok {
		if readOnly {
			return "    " + style.Render(label) + " " + lipgloss.NewStyle().Foreground(ColorYellow).Render("(read-only)")
		}
		return fmt.Sprintf("    ◀ %s ▶", style.Render(label))
	}
	return "    " + lipgloss.NewStyle().Foreground(ColorYellow).Render("Custom (read-only)")
}

func (s *configLazyGitScreen) View(width, height int) string {
	a := s.App()
	compact := height < 30
	title := renderConfigTitle("", "LazyGit", "Global Git terminal UI defaults")
	if compact {
		title = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("LazyGit")
	}
	cfg := a.deepDiveConfig
	blockedReason := lazyGitStandaloneUIBlockReason(a)
	readOnly := blockedReason != ""
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(54))
	separator := "\n\n"
	if compact {
		separator = "\n"
	}

	rec.field(0)
	rec.write(renderFieldLabel("Side Panel Fraction (e exact)", a.configFieldIndex == 0))
	fraction := cfg.LazyGitSidePanelWidth
	if s.editing {
		fraction = s.editDisplay()
	}
	rec.write("    " + lipgloss.NewStyle().Foreground(ColorCyan).Render(fraction))
	if readOnly {
		rec.write(" " + lipgloss.NewStyle().Foreground(ColorYellow).Render("(read-only)"))
	}
	rec.write(separator)

	rec.field(1)
	rec.write(renderFieldLabel("Mouse Events", a.configFieldIndex == 1))
	if readOnly {
		mouse := "OFF"
		if cfg.LazyGitMouseEvents {
			mouse = "ON"
		}
		rec.write("    " + mouse + " " + lipgloss.NewStyle().Foreground(ColorYellow).Render("(read-only)"))
	} else {
		rec.write(renderToggle(cfg.LazyGitMouseEvents, a.configFieldIndex == 1))
	}
	rec.write(separator)

	rec.field(2)
	rec.write(renderFieldLabel("Color Preset", a.configFieldIndex == 2))
	rec.write(lazyGitPresetDisplay(cfg.LazyGitColorPreset, map[string]string{"standard": "Standard", "light-high-contrast": "Light High Contrast"}, a.configFieldIndex == 2, readOnly))
	rec.write(separator)

	rec.field(3)
	rec.write(renderFieldLabel("Pager Preset", a.configFieldIndex == 3))
	rec.write(lazyGitPresetDisplay(cfg.LazyGitPagerPreset, map[string]string{"builtin": "Builtin", "delta": "Delta (dark)"}, a.configFieldIndex == 3, readOnly))

	notices := make([]string, 0, 3)
	if blockedReason != "" {
		notices = append(notices, lipgloss.NewStyle().Foreground(ColorYellow).Width(rec.innerWidth).Render("Read-only: "+blockedReason))
	}
	if a.nativeConfigState.LazyGit.RepoOverridesPossible {
		notices = append(notices, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Repository config may override these global values."))
	}
	if cfg.LazyGitPagerPreset == "delta" {
		if reason := lazyGitDeltaAvailabilityReason(a); reason != "" {
			notices = append(notices, lipgloss.NewStyle().Foreground(ColorYellow).Render(truncatePlain(reason, 66)))
		} else {
			notices = append(notices, lipgloss.NewStyle().Foreground(ColorGreen).Render("Delta installed • delta --dark --paging=never"))
		}
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	if readOnly {
		if a.configStandalone {
			help = HelpStyle.Render("↑↓ navigate • enter blocked preview • esc/q cancel")
		} else {
			help = HelpStyle.Render("↑↓ navigate • enter/esc back • LazyGit settings read-only")
		}
	} else if !s.editing && a.configFieldIndex == 0 {
		if a.configStandalone {
			help = HelpStyle.Render("↑↓ navigate • e edit exact fraction • enter preview • esc/q cancel")
		} else {
			help = HelpStyle.Render("↑↓ navigate • ←→/space change • enter/esc back • e edit exact fraction")
		}
	}
	if s.editing {
		help = HelpStyle.Render("digits/dot edit • ←→ cursor • enter commit • esc cancel")
		if s.editError != "" {
			help = lipgloss.JoinVertical(lipgloss.Center, lipgloss.NewStyle().Foreground(ColorYellow).Render(s.editError), help)
		}
	}
	parts := []string{title, "", box}
	if len(notices) > 0 {
		parts = append(parts, strings.Join(notices, "\n"))
	}
	parts = append(parts, "", help)
	content := lipgloss.JoinVertical(lipgloss.Center, parts...)
	a.configFieldLayout = rec.finalizeComposed(width, height, content, box, lipgloss.Height(title)+1)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}
