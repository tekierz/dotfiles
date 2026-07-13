package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	theme := s.Theme()
	navStyle := s.NavStyle()
	facts := installationSummaryFacts{}
	outcome := installationOutcomePending
	complete := false
	if app := s.App(); app != nil {
		facts = app.installSummaryFacts
		outcome = app.installOutcome
		complete = app.installComplete
	}
	succeeded := complete && outcome == installationOutcomeSucceeded && facts.outcome == installationOutcomeSucceeded

	titleText := "! Installation Incomplete"
	titleColor := ColorYellow
	if succeeded {
		titleText = "✓ Installation Complete!"
		titleColor = ColorGreen
	}
	title := lipgloss.NewStyle().Foreground(titleColor).Bold(true).Render(titleText)

	planHash := "unavailable"
	operationID := "unavailable"
	if facts.planHash != "" {
		planHash = facts.planHash
		if len(planHash) > 12 {
			planHash = planHash[:12]
		}
	}
	if facts.operationID != "" {
		operationID = facts.operationID
	}

	lines := []string{
		fmt.Sprintf("Plan:       %s (%d actions)", lipgloss.NewStyle().Foreground(ColorCyan).Render(planHash), facts.actionCount),
		fmt.Sprintf("Operation:  %s", lipgloss.NewStyle().Foreground(ColorCyan).Render(operationID)),
		fmt.Sprintf("Rollback scope: %d verified target(s)", facts.rollbackTargetCount),
	}
	if height < 18 {
		if succeeded {
			lines = append(lines, "", "Next: "+lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("dotfiles status"))
		} else {
			lines = append(lines, "", "Not verified. Review plan before retry.")
		}
	} else {
		lines = append([]string{
			fmt.Sprintf("Theme:      %s", lipgloss.NewStyle().Foreground(ColorCyan).Render(theme)),
			fmt.Sprintf("Navigation: %s", lipgloss.NewStyle().Foreground(ColorCyan).Render(navStyle)),
		}, lines...)
	}
	if height >= 18 && succeeded {
		lines = append(lines,
			"",
			"Next steps:",
			"1. Run "+lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("dotfiles status")+" to verify health",
			"2. Open "+lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("dotfiles manage")+" for settings",
		)
	} else if height >= 18 {
		lines = append(lines,
			"",
			"Completion was not verified.",
			"Some actions may have completed.",
			"Review the plan before retrying.",
		)
	}

	horizontalPadding := 2
	if width < 60 {
		horizontalPadding = 1
	}
	contentWidth := max(12, width-2-(horizontalPadding*2))
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, strings.Split(ansi.Wrap(line, contentWidth, " /-_"), "\n")...)
	}
	summary := lipgloss.NewStyle().Foreground(ColorText).MaxWidth(contentWidth).Render(strings.Join(wrapped, "\n"))

	helpStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted)
	help := helpStyle.Render("[ENTER] Exit")

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Padding(1, horizontalPadding)

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
