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
				// Cancel the standalone change: revert the preview to the saved theme,
				// then route symmetrically with the Enter branch.
				// In-TUI main-menu entry (themeReturn==ScreenMainMenu): return there.
				// CLI `dotfiles theme` entry (themeReturn==ScreenWelcome, the
				// constructor default): quit so the shell regains control rather than
				// dropping into the install welcome screen.
				a.revertThemeToSaved()
				if a.themeReturn == ScreenMainMenu {
					return s, NavigateTo(ScreenMainMenu)
				}
				return s, tea.Quit
			}
			// Wizard step: go back to the caller's screen. themeReturn is set at
			// every wizard entry point (ScreenWelcome for the welcome quick-setup
			// path, ScreenDeepDiveMenu for the deep-dive continue path) so this
			// is always defined.
			return s, NavigateTo(a.themeReturn)
		}

	case tea.MouseMsg:
		s.handleMouse(msg)
		return s, nil
	}
	return s, nil
}

// handleMouse handles scroll-wheel navigation and click-to-select on the theme
// list. Mirrors the legacy handleThemePickerMouse behavior.
func (s *themePickerScreen) handleMouse(msg tea.MouseMsg) {
	a := s.App()
	m := tea.MouseEvent(msg)

	switch m.Button { //nolint:exhaustive // Only vertical wheel actions are meaningful here.
	case tea.MouseButtonWheelUp:
		if a.themeIndex > 0 {
			s.applyTheme(a.themeIndex - 1)
		}
		return
	case tea.MouseButtonWheelDown:
		if a.themeIndex < len(themes)-1 {
			s.applyTheme(a.themeIndex + 1)
		}
		return
	}

	if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		// Resolve the click against the per-theme geometry recorded by the most
		// recent View (a.themeListLayout). It is derived from the actually
		// rendered container (height + width measured with lipgloss), so it
		// accounts for ContainerStyle's border/padding and HelpStyle's padding
		// — unlike the legacy hand-derived len(themes)+6 / containerW=60 which
		// shifted every row by ~2-3 and used a wrong X span (C20).
		fl := a.themeListLayout
		if fl.hasXBounds && (m.X < fl.boxLeft || m.X > fl.boxRight) {
			return
		}
		if idx, ok := fl.fieldAt(m.Y); ok {
			if idx >= 0 && idx < len(themes) {
				s.applyTheme(idx)
			}
		}
	}
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

	container := ContainerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))

	// Record per-theme geometry for the mouse handler. The theme list rows live
	// at content offset title(1)+blank(1)=2; the container adds ContainerStyle's
	// top border(1)+padding(1)=2 before its content; PlaceWithBackground centers
	// it. Measure the rendered container so the anchor/width are exact.
	a.themeListLayout = centeredContainerListLayout(width, height, container, len(themes), lipgloss.Height(title)+1)

	return PlaceWithBackground(width, height, container)
}
