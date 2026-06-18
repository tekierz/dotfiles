package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/tools"
)

// =====================================
// Update Screen
// =====================================
//
// Note: the Update screen (renderUpdate/renderUpdateWithLogs + its key/mouse
// handling and async messages) is migrated to a ScreenHandler in
// screen_update.go and driven by the ScreenManager.

// =====================================
// Hotkeys Screen (uses internal/hotkeys package)
// =====================================
//
// Note: the Hotkeys screen is migrated to a ScreenHandler in screen_hotkeys.go
// (dual-pane viewer + key/mouse handling) and driven by the ScreenManager.

// =====================================
// Manage Screen
// =====================================

func (a *App) renderManage() string {
	registry := tools.GetRegistry()

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan).
		Render("  Manage Tools")

	subtitle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render(fmt.Sprintf("%d tools registered • %d installed", registry.Count(), registry.InstalledCount()))

	// Group by category
	categories := []tools.Category{
		tools.CategoryShell,
		tools.CategoryTerminal,
		tools.CategoryEditor,
		tools.CategoryFile,
		tools.CategoryGit,
		tools.CategoryContainer,
		tools.CategoryUtility,
	}

	var lines []string
	itemIndex := 0

	for _, cat := range categories {
		catTools := registry.ByCategory(cat)
		if len(catTools) == 0 {
			continue
		}

		// Category header
		catStyle := lipgloss.NewStyle().
			Foreground(ColorMagenta).
			Bold(true)
		lines = append(lines, catStyle.Render(fmt.Sprintf("\n  %s", strings.ToUpper(string(cat)))))

		for _, tool := range catTools {
			cursor := "  "
			nameStyle := lipgloss.NewStyle().Foreground(ColorText)
			descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

			if itemIndex == a.manageIndex {
				cursor = " "
				nameStyle = nameStyle.Foreground(ColorGreen).Bold(true)
			}

			status := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("○")
			if tool.IsInstalled() {
				status = lipgloss.NewStyle().Foreground(ColorGreen).Render("●")
			}

			line := fmt.Sprintf("%s%s %s %s  %s",
				cursor,
				status,
				tool.Icon(),
				nameStyle.Render(tool.Name()),
				descStyle.Render(tool.Description()))
			lines = append(lines, line)
			itemIndex++
		}
	}

	toolList := strings.Join(lines, "\n")

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("↑↓ Navigate • Enter Configure • Esc Back • q Quit")

	content := fmt.Sprintf("\n\n%s\n%s\n%s\n\n%s",
		title, subtitle, toolList, footer)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// =====================================
// Backups Screen
// =====================================
//
// Note: the Backups screen (renderBackups + handleBackupsMouse + its key handling
// and async messages) is migrated to a ScreenHandler in screen_backups.go and
// driven by the ScreenManager.
