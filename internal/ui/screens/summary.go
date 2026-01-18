package screens

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// Colors used by the summary screen
var (
	colorGreen    = lipgloss.Color("#00FF88")
	colorCyan     = lipgloss.Color("#00F5D4")
	colorNeonBlue = lipgloss.Color("#00D4FF")
	colorText     = lipgloss.Color("#E0E0E0")
)

// SummaryScreen displays the installation completion summary.
type SummaryScreen struct {
	ui.BaseScreen
}

// NewSummaryScreen creates a new summary screen.
func NewSummaryScreen(ctx *ui.ScreenContext) *SummaryScreen {
	s := &SummaryScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *SummaryScreen) ID() ui.Screen {
	return ui.ScreenSummary
}

// Init returns any initial commands.
func (s *SummaryScreen) Init() tea.Cmd {
	return nil
}

// Update handles input messages.
func (s *SummaryScreen) Update(msg tea.Msg) (ui.ScreenHandler, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return s, tea.Quit
		case "esc", "q":
			return s, tea.Quit
		}
	}
	return s, nil
}

// View renders the summary screen.
func (s *SummaryScreen) View(width, height int) string {
	title := lipgloss.NewStyle().
		Foreground(colorGreen).
		Bold(true).
		Render("✓ Installation Complete!")

	theme := s.Theme()
	navStyle := s.NavStyle()

	summary := lipgloss.NewStyle().Foreground(colorText).Render(fmt.Sprintf(`
  Theme:      %s
  Navigation: %s
  Backup:     ~/.config/dotfiles/backups/

  Next steps:

  1. %s or restart terminal
  2. %s to start tmux
  3. %s to finish plugin installation
  4. %s to customize prompt
  5. %s to see hotkey reference
`,
		lipgloss.NewStyle().Foreground(colorCyan).Render(theme),
		lipgloss.NewStyle().Foreground(colorCyan).Render(navStyle),
		lipgloss.NewStyle().Foreground(colorNeonBlue).Render("source ~/.zshrc"),
		lipgloss.NewStyle().Foreground(colorNeonBlue).Render("tmux"),
		lipgloss.NewStyle().Foreground(colorNeonBlue).Render("nvim"),
		lipgloss.NewStyle().Foreground(colorNeonBlue).Render("p10k configure"),
		lipgloss.NewStyle().Foreground(colorNeonBlue).Render("hk"),
	))
	summary = lipgloss.NewStyle().MaxWidth(max(20, width-6)).Render(summary)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080"))
	help := helpStyle.Render("[ENTER] Exit")

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(1, 2)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		containerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			summary,
			help,
		)),
	)
}
