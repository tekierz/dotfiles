package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// navPickerScreen is the migrated ScreenHandler for navigation-style selection.
//
// State stays on App: the selected nav style (navStyle) is read/written through
// s.App(). The handler also mirrors it into ctx.NavStyle so other migrated
// screens observe the change immediately.
type navPickerScreen struct {
	BaseScreen
}

// NewNavPickerScreen creates a new nav picker screen handler.
func NewNavPickerScreen(ctx *ScreenContext) *navPickerScreen {
	s := &navPickerScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *navPickerScreen) ID() Screen { return ScreenNavPicker }

// Init returns any initial commands (none on entry).
func (s *navPickerScreen) Init() tea.Cmd { return nil }

// setNavStyle records the nav style on App and mirrors it into the context.
func (s *navPickerScreen) setNavStyle(style string) {
	a := s.App()
	a.navStyle = style
	if ctx := s.Context(); ctx != nil {
		ctx.NavStyle = style
	}
}

// Update handles keyboard and mouse input for the nav picker.
func (s *navPickerScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case keyCtrlC, "q":
			return s, tea.Quit
		case keyLeft, keyRight, "h", "l", keyTab:
			if a.navStyle == navEmacs {
				s.setNavStyle(navVim)
			} else {
				s.setNavStyle(navEmacs)
			}
		case keyEnter:
			return s, NavigateTo(ScreenFileTree)
		case keyEsc:
			return s, NavigateTo(ScreenThemePicker)
		}

	case tea.MouseMsg:
		s.handleMouse(msg)
	}
	return s, nil
}

// handleMouse selects the nav style based on click position. Mirrors the legacy
// handleNavPickerMouse behavior (side-by-side on wide, stacked on narrow).
func (s *navPickerScreen) handleMouse(msg tea.MouseMsg) {
	m := tea.MouseEvent(msg)
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return
	}

	centerX := s.Width() / 2
	centerY := s.Height() / 2

	if s.Width() >= 78 {
		// Side by side layout: Emacs on the left, Vim on the right.
		if m.X < centerX {
			s.setNavStyle(navEmacs)
		} else {
			s.setNavStyle(navVim)
		}
	} else {
		// Stacked layout: Emacs on top, Vim on bottom.
		if m.Y < centerY {
			s.setNavStyle(navEmacs)
		} else {
			s.setNavStyle(navVim)
		}
	}
}

// View renders the navigation style selection screen.
func (s *navPickerScreen) View(width, height int) string {
	a := s.App()

	title := TitleStyle.Render("Select Navigation Style")

	emacsStyle := ButtonStyle
	vimStyle := ButtonStyle

	if a.navStyle == navEmacs {
		emacsStyle = ButtonActiveStyle
	} else {
		vimStyle = ButtonActiveStyle
	}

	emacsBox := emacsStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		" EMACS / MAC STYLE ",
		"",
		" Arrow keys for navigation",
		" Ctrl-A/E for line start/end",
		" Ctrl-W to delete word",
		"",
		" Recommended for beginners",
	))

	vimBox := vimStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		" VIM STYLE ",
		"",
		" hjkl for navigation",
		" Modal editing (Esc/i)",
		" Efficient for experts",
		"",
		" Power user choice",
	))

	content := lipgloss.JoinHorizontal(lipgloss.Top, emacsBox, "  ", vimBox)
	if width < 78 {
		content = lipgloss.JoinVertical(lipgloss.Center, emacsBox, "", vimBox)
	}

	help := HelpStyle.Render("[←→] Select    [ENTER] Continue    [ESC] Back")

	return PlaceWithBackground(
		width, height,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			"",
			content,
			"",
			help,
		)),
	)
}
