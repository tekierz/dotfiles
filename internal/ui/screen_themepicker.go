package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// themePickerScreen is the migrated ScreenHandler for theme selection.
//
// State stays on App: the selection index (themeIndex) and the resolved theme
// name (theme) are read/written through s.App(). On every selection change the
// handler applies the theme globally (SetTheme) and updates ctx.Theme so other
// migrated screens see the new theme immediately (live preview).
type themePickerScreen struct {
	BaseScreen
}

// NewThemePickerScreen creates a new theme picker screen handler.
func NewThemePickerScreen(ctx *ScreenContext) *themePickerScreen {
	s := &themePickerScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *themePickerScreen) ID() Screen { return ScreenThemePicker }

// Init returns any initial commands (none on entry).
func (s *themePickerScreen) Init() tea.Cmd { return nil }

// applyTheme records the selected theme on App + context and applies it globally
// so the live preview matches across the UI.
func (s *themePickerScreen) applyTheme(index int) {
	a := s.App()
	a.themeIndex = index
	a.theme = themes[index].name
	SetTheme(a.theme) // Apply theme immediately for live preview
	if ctx := s.Context(); ctx != nil {
		ctx.Theme = a.theme
	}
}

// Update handles keyboard and mouse input for the theme picker.
func (s *themePickerScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return s, tea.Quit
		case "q":
			if a.themeStandalone {
				// Standalone quit: revert the in-session preview so the global
				// theme palette is not left in a mutated state (q-revert medium).
				a.revertThemeToSaved()
			}
			return s, tea.Quit
		case "up", "k":
			if a.themeIndex > 0 {
				s.applyTheme(a.themeIndex - 1)
			}
		case "down", "j":
			if a.themeIndex < len(themes)-1 {
				s.applyTheme(a.themeIndex + 1)
			}
		case "enter":
			if a.themeStandalone {
				// Standalone mode: persist the already-live theme.
				a.persistTheme()
				if a.themeReturn == ScreenMainMenu {
					// In-TUI call (main menu): return to the main menu, stay in TUI.
					return s, NavigateTo(ScreenMainMenu)
				}
				// CLI call (`dotfiles theme`): the whole TUI was just the picker;
				// quit so the shell regains control.
				return s, tea.Quit
			}
			// Wizard step: advance to nav-style picker.
			return s, NavigateTo(ScreenNavPicker)
		case "esc":
			if a.themeStandalone {
				// Cancel the standalone change: revert the preview to the saved
				// theme and return to the caller's screen.
				a.revertThemeToSaved()
				return s, NavigateTo(a.themeReturn)
			}
			// Wizard step: go back to the caller's screen. themeReturn is set at
			// every wizard entry point (ScreenWelcome for the welcome quick-setup
			// path, ScreenDeepDiveMenu for the deep-dive continue path) so this
			// is always defined.
			return s, NavigateTo(a.themeReturn)
		}

	case tea.MouseMsg:
		return s, s.handleMouse(msg)
	}
	return s, nil
}

// handleMouse handles scroll-wheel navigation and click-to-select on the theme
// list. Mirrors the legacy handleThemePickerMouse behavior.
func (s *themePickerScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	switch m.Button {
	case tea.MouseButtonWheelUp:
		if a.themeIndex > 0 {
			s.applyTheme(a.themeIndex - 1)
		}
		return nil
	case tea.MouseButtonWheelDown:
		if a.themeIndex < len(themes)-1 {
			s.applyTheme(a.themeIndex + 1)
		}
		return nil
	}

	if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		// The content is centered using PlaceWithBackground + ContainerStyle.
		// Container has Padding(1, 2) and Border, then title + empty line + themes.
		containerH := len(themes) + 6 // title + empty + themes + empty + help + padding
		containerW := 60              // approximate
		startY := (s.Height() - containerH) / 2
		startX := (s.Width() - containerW) / 2
		// Clamp to >= 0 so a negative startX (terminal narrower than the
		// container) doesn't make the entire row width a hit target.
		if startX < 0 {
			startX = 0
		}

		// Theme list starts after: container border (1) + padding (1) + title (1) + empty (1)
		listStartY := startY + 4

		if m.Y >= listStartY && m.Y < listStartY+len(themes) && m.X >= startX && m.X < startX+containerW {
			themeIdx := m.Y - listStartY
			if themeIdx >= 0 && themeIdx < len(themes) {
				s.applyTheme(themeIdx)
			}
		}
	}
	return nil
}

// View renders the theme selection screen.
func (s *themePickerScreen) View(width, height int) string {
	a := s.App()

	title := TitleStyle.Render("Select Theme")

	// Layout adapts based on terminal width:
	// - wide: list + preview side-by-side
	// - narrow: list only (preview below would overflow easily)
	showPreview := width >= 86

	previewOuterW := min(36, maxInt(24, width/3))
	if !showPreview {
		previewOuterW = 0
	}

	listW := maxInt(24, width-10)
	if showPreview {
		listW = maxInt(24, width-previewOuterW-12)
	}

	var themeList strings.Builder
	for i, t := range themes {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if i == a.themeIndex {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(t.color)).Bold(true)
		}
		line := style.Render(fmt.Sprintf("%s%-20s %s", prefix, t.name, t.desc))
		themeList.WriteString(truncateVisible(line, listW))
		themeList.WriteByte('\n')
	}

	content := themeList.String()

	if showPreview {
		// Preview box with color swatches
		selectedTheme := themes[a.themeIndex]
		previewColor := lipgloss.Color(selectedTheme.color)

		preview := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(previewColor).
			Width(maxInt(1, previewOuterW-2)). // border adds 2
			Padding(1).
			Render(lipgloss.JoinVertical(
				lipgloss.Left,
				lipgloss.NewStyle().Foreground(previewColor).Bold(true).Render(selectedTheme.name),
				"",
				lipgloss.NewStyle().Foreground(previewColor).Render("████████████████████"),
				"",
				lipgloss.NewStyle().Foreground(ColorText).Render("  normal text"),
				lipgloss.NewStyle().Foreground(previewColor).Render("  highlighted text"),
				lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  muted text"),
				"",
				lipgloss.NewStyle().Foreground(ColorGreen).Render("  ✓ success"),
				lipgloss.NewStyle().Foreground(ColorRed).Render("  ✗ error"),
			))

		content = lipgloss.JoinHorizontal(lipgloss.Top, content, "  ", preview)
	}

	help := HelpStyle.Render("[↑↓/jk] Navigate    [ENTER] Select    [ESC] Back")

	rows := []string{title, "", content, "", help}

	// Surface any save error from the last persistTheme call (error-medium).
	if a.themeStatus != "" {
		statusLine := lipgloss.NewStyle().Foreground(ColorRed).Render(a.themeStatus)
		rows = append(rows, "", statusLine)
	}

	return PlaceWithBackground(
		width, height,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			rows...,
		)),
	)
}
