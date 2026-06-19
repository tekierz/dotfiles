package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configGhosttyScreen is the migrated ScreenHandler for the Ghostty config
// screen. Navigation + the back-to-menu transition are inherited from
// configFieldNav; only the field layout (View) and value adjustment (adjust)
// are screen-specific.
//
// Fields: 0=font family, 1=font size, 2=opacity, 3=blur, 4=scrollback,
// 5=cursor style, 6=tab bindings.
type configGhosttyScreen struct {
	configFieldNav
}

// NewConfigGhosttyScreen creates a new Ghostty config screen handler.
func NewConfigGhosttyScreen(ctx *ScreenContext) *configGhosttyScreen {
	s := &configGhosttyScreen{}
	s.id = ScreenConfigGhostty
	s.maxField = func(*App) int { return 6 }
	s.adjust = ghosttyAdjust
	s.SetContext(ctx)
	return s
}

func ghosttyAdjust(a *App, key string, fwd bool) {
	cfg := a.deepDiveConfig
	switch a.configFieldIndex {
	case 0: // Font family
		opts := []string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"}
		cfg.GhosttyFontFamily = cycleOption(opts, cfg.GhosttyFontFamily, fwd)
	case 1: // Font size
		if fwd {
			if cfg.GhosttyFontSize < 32 {
				cfg.GhosttyFontSize++
			}
		} else if cfg.GhosttyFontSize > 8 {
			cfg.GhosttyFontSize--
		}
	case 2: // Opacity
		if fwd {
			if cfg.GhosttyOpacity < 100 {
				cfg.GhosttyOpacity += 5
			}
		} else if cfg.GhosttyOpacity > 0 {
			cfg.GhosttyOpacity -= 5
		}
	case 3: // Blur radius
		if fwd {
			if cfg.GhosttyBlurRadius < 100 {
				cfg.GhosttyBlurRadius += 5
			}
		} else if cfg.GhosttyBlurRadius > 0 {
			cfg.GhosttyBlurRadius -= 5
		}
	case 4: // Scrollback lines
		opts := []string{"1000", "5000", "10000", "50000", "100000"}
		current := fmt.Sprintf("%d", cfg.GhosttyScrollbackLines)
		cfg.GhosttyScrollbackLines = atoi(cycleOption(opts, current, fwd), 10000)
	case 5: // Cursor style
		opts := []string{"block", "bar", "underline"}
		cfg.GhosttyCursorStyle = cycleOption(opts, cfg.GhosttyCursorStyle, fwd)
	case 6: // Tab bindings
		opts := []string{"super", "ctrl", "ctrl-shift"}
		cfg.GhosttyTabBindings = cycleOption(opts, cfg.GhosttyTabBindings, fwd)
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configGhosttyScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Ghostty configuration screen.
func (s *configGhosttyScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("󰆍", "Ghostty", "Terminal emulator settings")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

	fontFamilyFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Font Family", fontFamilyFocused))
	content.WriteString(renderOptionSelector(
		[]string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"},
		[]string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"},
		cfg.GhosttyFontFamily,
		fontFamilyFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	fontFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Font Size", fontFocused))
	content.WriteString(renderNumberControl(cfg.GhosttyFontSize, 8, 32, fontFocused))
	content.WriteString("\n\n")
	fieldIdx++

	opacityFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Background Opacity", opacityFocused))
	content.WriteString(renderSliderControl(cfg.GhosttyOpacity, 100, 24, opacityFocused))
	content.WriteString("\n\n")
	fieldIdx++

	blurFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Blur Radius", blurFocused))
	content.WriteString(renderSliderControl(cfg.GhosttyBlurRadius, 100, 24, blurFocused))
	content.WriteString("\n\n")
	fieldIdx++

	scrollFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Scrollback Lines", scrollFocused))
	content.WriteString(renderOptionSelector(
		[]string{"1000", "5000", "10000", "50000", "100000"},
		[]string{"1K", "5K", "10K", "50K", "100K"},
		fmt.Sprintf("%d", cfg.GhosttyScrollbackLines),
		scrollFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	cursorFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Cursor Style", cursorFocused))
	content.WriteString(renderOptionSelector(
		[]string{"block", "bar", "underline"},
		[]string{"█ Block", "│ Bar", "_ Underline"},
		cfg.GhosttyCursorStyle,
		cursorFocused,
	))
	content.WriteString("\n\n")
	fieldIdx++

	tabFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("New Tab Keybinding", tabFocused))
	content.WriteString(renderOptionSelector(
		[]string{"super", "ctrl", "ctrl-shift"},
		[]string{"⌘/Super+T", "Ctrl+T", "Ctrl+Shift+T"},
		cfg.GhosttyTabBindings,
		tabFocused,
	))

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • ←→ adjust • enter/esc save & back")

	return PlaceWithBackground(
		width, height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
