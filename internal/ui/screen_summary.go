package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// SummaryScreen displays the installation completion summary.
type SummaryScreen struct {
	BaseScreen
}

// NewSummaryScreen creates a new summary screen.
func NewSummaryScreen(ctx *ScreenContext) *SummaryScreen {
	s := &SummaryScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *SummaryScreen) ID() Screen {
	return ScreenSummary
}

// Init returns any initial commands.
func (s *SummaryScreen) Init() tea.Cmd {
	return nil
}

// Update handles input messages.
func (s *SummaryScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
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
//
// Colors come from the active theme palette (ColorGreen/ColorCyan/etc.) rather
// than hardcoded hex values, so the summary matches the rest of the UI and the
// user's selected theme.
func (s *SummaryScreen) View(width, height int) string {
	title := lipgloss.NewStyle().
		Foreground(ColorGreen).
		Bold(true).
		Render("✓ Installation Complete!")

	theme := s.Theme()
	navStyle := s.NavStyle()
	planHash := "unavailable"
	operationID := "unavailable"
	actionCount := 0
	backupCount := 0
	if app := s.App(); app != nil {
		if app.pendingInstallPlan != nil {
			planHash = app.pendingInstallPlan.hash()
			if len(planHash) > 12 {
				planHash = planHash[:12]
			}
			actionCount = len(app.pendingInstallPlan.actions())
			backupCount = len(app.pendingInstallPlan.backupTargets())
		}
		if app.lastOperationID != "" {
			operationID = app.lastOperationID
		}
	}

	summary := lipgloss.NewStyle().Foreground(ColorText).Render(fmt.Sprintf(`
  Theme:      %s
  Navigation: %s
  Plan:       %s (%d actions)
  Operation:  %s
  Rollback:   %d plan-derived target(s), including newly created paths

  Next steps:

  1. %s or restart terminal
  2. %s to start tmux
  3. %s to finish plugin installation
  4. %s to customize prompt
  5. %s to see hotkey reference
`,
		lipgloss.NewStyle().Foreground(ColorCyan).Render(theme),
		lipgloss.NewStyle().Foreground(ColorCyan).Render(navStyle),
		lipgloss.NewStyle().Foreground(ColorCyan).Render(planHash),
		actionCount,
		lipgloss.NewStyle().Foreground(ColorCyan).Render(operationID),
		backupCount,
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("source ~/.zshrc"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("tmux"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("nvim"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("p10k configure"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("hk"),
	))
	summary = lipgloss.NewStyle().MaxWidth(max(20, width-6)).Render(summary)

	helpStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted)
	help := helpStyle.Render("[ENTER] Exit")

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
			summary,
			help,
		)),
	)
}
