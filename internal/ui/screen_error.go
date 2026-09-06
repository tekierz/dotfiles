package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
			// Retry requires a fresh observation and explicit plan review.
			if a := s.App(); a != nil {
				a.prepareInstallationReview()
			}
			return s, NavigateTo(ScreenFileTree)
		case "s":
			// Skip preserves a truthful failed terminal outcome.
			if a := s.App(); a != nil {
				a.finishInstallationAttempt(installationOutcomeFailed)
			}
			return s, NavigateTo(ScreenSummary)
		case "q":
			return s, tea.Quit
		case "esc":
			if a := s.App(); a != nil {
				a.prepareInstallationReview()
			}
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
	horizontalPadding := 2
	if width < 60 {
		horizontalPadding = 1
	}
	contentWidth := max(12, width-2-(horizontalPadding*2))
	errorWidth := max(8, contentWidth-4)
	compact := height < 18
	wrappedError := strings.Split(ansi.Wrap(sanitizeLogLine(errMsg), errorWidth, " /-_"), "\n")
	var errorBox string
	if compact {
		const maxCompactErrorLines = 2
		if len(wrappedError) > maxCompactErrorLines {
			wrappedError = wrappedError[:maxCompactErrorLines]
			wrappedError[maxCompactErrorLines-1] = ansi.Truncate(wrappedError[maxCompactErrorLines-1]+"…", errorWidth, "…")
		}
		errorBox = lipgloss.NewStyle().Foreground(ColorRed).MaxWidth(contentWidth).Render(strings.Join(wrappedError, "\n"))
	} else {
		errorBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorRed).
			Padding(1).
			MaxWidth(contentWidth).
			Render(strings.Join(wrappedError, "\n"))
	}

	buttonStyle := lipgloss.NewStyle().
		Foreground(ColorTextBright).
		Padding(0, 1)

	buttons := []string{
		buttonStyle.Render("[R] Review Plan"),
		buttonStyle.Render("[S] Skip to Summary"),
		buttonStyle.Render("[Q] Quit"),
	}
	options := lipgloss.JoinHorizontal(lipgloss.Top, buttons[0], "  ", buttons[1], "  ", buttons[2])
	if width < 72 || compact {
		options = lipgloss.JoinVertical(lipgloss.Center, buttons...)
	}
	verticalPadding := 1
	if compact {
		verticalPadding = 0
	}

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Padding(verticalPadding, horizontalPadding)

	content := lipgloss.JoinVertical(lipgloss.Center, title, "", errorBox, "", options)
	if compact {
		content = lipgloss.JoinVertical(lipgloss.Center, title, errorBox, options)
	}

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		containerStyle.Render(content),
	)
}
