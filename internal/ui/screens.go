package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Note: renderAnimation and renderProgress moved to the migrated ScreenHandlers
// in screen_animation.go (animationScreen.View) and screen_progress.go
// (progressScreen.View). Only the still-legacy renderSummary / renderError live
// here.

// renderSummary renders the post-installation summary
func (a *App) renderSummary() string {
	title := lipgloss.NewStyle().
		Foreground(ColorGreen).
		Bold(true).
		Render("✓ Installation Complete!")

	summary := lipgloss.NewStyle().Foreground(ColorText).Render(fmt.Sprintf(`
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
		lipgloss.NewStyle().Foreground(ColorCyan).Render(a.theme),
		lipgloss.NewStyle().Foreground(ColorCyan).Render(a.navStyle),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("source ~/.zshrc"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("tmux"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("nvim"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("p10k configure"),
		lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("hk"),
	))
	summary = lipgloss.NewStyle().MaxWidth(maxInt(20, a.width-6)).Render(summary)

	help := HelpStyle.Render("[ENTER] Exit")

	return PlaceWithBackground(
		a.width, a.height,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			summary,
			help,
		)),
	)
}

// renderError renders the error recovery screen
func (a *App) renderError() string {
	title := lipgloss.NewStyle().
		Foreground(ColorRed).
		Bold(true).
		Render("✗ Error Occurred")

	errMsg := "Unknown error"
	if a.lastError != nil {
		errMsg = a.lastError.Error()
	}

	errorBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorRed).
		Padding(1).
		MaxWidth(maxInt(20, a.width-10)).
		Render(errMsg)

	options := lipgloss.JoinHorizontal(
		lipgloss.Top,
		ButtonStyle.Render(" [R] Retry "),
		"  ",
		ButtonStyle.Render(" [S] Skip "),
		"  ",
		ButtonStyle.Render(" [Q] Quit "),
	)

	return PlaceWithBackground(
		a.width, a.height,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			"",
			errorBox,
			"",
			options,
		)),
	)
}
