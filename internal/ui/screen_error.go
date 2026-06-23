package ui

import (
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// ErrorScreen displays an error and offers retry/skip/quit options.
type ErrorScreen struct {
	BaseScreen
	err error
}

// NewErrorScreen creates a new error screen with the given error.
func NewErrorScreen(ctx *ScreenContext, err error) *ErrorScreen {
	s := &ErrorScreen{
		err: err,
	}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *ErrorScreen) ID() Screen {
	return ScreenError
}

// Init returns any initial commands.
func (s *ErrorScreen) Init() tea.Cmd {
	return nil
}

// Update handles input messages.
func (s *ErrorScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "r":
			// Retry - navigate to progress screen
			return s, NavigateTo(ScreenProgress)
		case "s":
			// Skip - continue to summary
			return s, NavigateTo(ScreenSummary)
		case "q":
			return s, tea.Quit
		case keyEsc:
			return s, NavigateTo(ScreenFileTree)
		}
	}
	return s, nil
}

// View renders the error screen.
func (s *ErrorScreen) View(width, height int) string {
	title := lipgloss.NewStyle().
		Foreground(ColorRed).
		Bold(true).
		Render("✗ Error Occurred")

	errMsg := "Unknown error"
	if s.err != nil {
		errMsg = s.err.Error()
	}

	errorBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorRed).
		Padding(1).
		MaxWidth(max(20, width-10)).
		Render(errMsg)

	buttonStyle := lipgloss.NewStyle().
		Foreground(ColorTextBright).
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
		BorderForeground(ColorCyan).
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
