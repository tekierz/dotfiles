package screens

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// ErrorScreen displays an error and offers retry/skip/quit options.
type ErrorScreen struct {
	ui.BaseScreen
	err error
}

// NewErrorScreen creates a new error screen with the given error.
func NewErrorScreen(ctx *ui.ScreenContext, err error) *ErrorScreen {
	s := &ErrorScreen{
		err: err,
	}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *ErrorScreen) ID() ui.Screen {
	return ui.ScreenError
}

// Init returns any initial commands.
func (s *ErrorScreen) Init() tea.Cmd {
	return nil
}

// Update handles input messages.
func (s *ErrorScreen) Update(msg tea.Msg) (ui.ScreenHandler, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			// Retry - navigate to progress screen
			return s, ui.NavigateTo(ui.ScreenProgress)
		case "s":
			// Skip - continue to summary
			return s, ui.NavigateTo(ui.ScreenSummary)
		case "q":
			return s, tea.Quit
		case "esc":
			return s, ui.NavigateTo(ui.ScreenFileTree)
		}
	}
	return s, nil
}

// View renders the error screen.
func (s *ErrorScreen) View(width, height int) string {
	title := lipgloss.NewStyle().
		Foreground(ui.ColorRed).
		Bold(true).
		Render("✗ Error Occurred")

	errMsg := "Unknown error"
	if s.err != nil {
		errMsg = s.err.Error()
	}

	errorBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorRed).
		Padding(1).
		MaxWidth(max(20, width-10)).
		Render(errMsg)

	buttonStyle := lipgloss.NewStyle().
		Foreground(ui.ColorTextBright).
		Padding(0, 1)

	options := lipgloss.JoinHorizontal(
		lipgloss.Top,
		buttonStyle.Render(" [R] Retry "),
		"  ",
		buttonStyle.Render(" [S] Skip "),
		"  ",
		buttonStyle.Render(" [Q] Quit "),
	)

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorCyan).
		Padding(1, 2)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		containerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			"",
			errorBox,
			"",
			options,
		)),
	)
}
