package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

// =====================================
// Main Menu Screen
// =====================================

func (a *App) renderMainMenu() string {
	items := GetMainMenuItems()

	maxLineW := maxInt(20, a.width-10)

	title := TitleStyle.Render("Dotfiles Management")
	subtitle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render(truncateVisible("Terminal environment management platform", maxLineW))

	// Menu items
	var menuLines []string
	for i, item := range items {
		cursor := "  "
		itemStyle := lipgloss.NewStyle().Foreground(ColorText)
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

		if i == a.mainMenuIndex {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			itemStyle = itemStyle.Foreground(ColorCyan).Bold(true)
			descStyle = descStyle.Foreground(ColorText)
		}

		line := fmt.Sprintf("%s%s %s  %s",
			cursor,
			item.Icon,
			itemStyle.Render(item.Name),
			descStyle.Render(item.Description))
		menuLines = append(menuLines, truncateVisible(line, maxLineW))
	}

	menu := strings.Join(menuLines, "\n")

	help := HelpStyle.Render(truncateVisible("↑↓ navigate • enter select • q quit", maxLineW))
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		subtitle,
		"",
		menu,
		"",
		help,
	)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(content))
}

// =====================================
// Update Screen
// =====================================

// updatePackage represents a package that can be updated
type updatePackage struct {
	name           string
	currentVersion string
	latestVersion  string
	selected       bool
}

func (a *App) renderUpdate() string {
	// Tab bar at top
	tabBar := RenderTabBar(ScreenUpdate, a.width)

	title := TitleStyle.Render("Package Updates")

	// Check if we're running an update
	if a.updateRunning {
		spinnerText := a.updateStatus
		if a.animationsEnabled {
			spinnerText = AnimatedSpinnerDots(a.uiFrame) + " " + a.updateStatus
		}
		body := lipgloss.NewStyle().Foreground(ColorCyan).Render(spinnerText)
		progressBar := ProgressBarAnimated(0.5, min(60, a.width-20), a.uiFrame)
		help := HelpStyle.Render("updating... please wait")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, progressBar, "", help)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	// Check if we're still loading
	if a.updateChecking {
		spinnerText := "Checking for updates..."
		if a.animationsEnabled {
			spinnerText = AnimatedSpinnerDots(a.uiFrame) + " Checking for updates..."
		}
		body := lipgloss.NewStyle().Foreground(ColorCyan).Render(spinnerText)
		progressBar := ProgressBarAnimated(0.5, min(60, a.width-20), a.uiFrame)
		help := HelpStyle.Render("1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, progressBar, "", help)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	// Check for errors
	if a.updateError != nil {
		body := lipgloss.NewStyle().Foreground(ColorRed).Render(fmt.Sprintf("Error: %v", a.updateError))
		help := HelpStyle.Render("r refresh • 1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	// Check if no package manager detected (results will be nil with no error)
	mgr := pkg.DetectManager()
	if mgr == nil {
		body := lipgloss.NewStyle().Foreground(ColorRed).Render("No package manager detected")
		help := HelpStyle.Render("1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	updates := a.updateResults

	if len(updates) == 0 {
		body := lipgloss.NewStyle().Foreground(ColorGreen).Render("All packages are up to date!")
		help := HelpStyle.Render("r refresh • 1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	// Clamp cursor to actual list length (rendering-only; avoids "lost" cursor).
	if a.updateIndex < 0 {
		a.updateIndex = 0
	}
	if a.updateIndex > len(updates)-1 {
		a.updateIndex = len(updates) - 1
	}

	// Build subtitle with selection count
	selectedCount := len(a.updateSelected)
	subtitleText := fmt.Sprintf("Found %d outdated package(s)", len(updates))
	if selectedCount > 0 {
		subtitleText += fmt.Sprintf(" • %d selected", selectedCount)
	}
	subtitle := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(subtitleText)

	// Show status message if any
	var statusLine string
	if a.updateStatus != "" {
		statusStyle := lipgloss.NewStyle().Foreground(ColorGreen)
		if strings.Contains(a.updateStatus, "failed") {
			statusStyle = lipgloss.NewStyle().Foreground(ColorRed)
		}
		statusLine = statusStyle.Render(a.updateStatus)
	}

	boxOuterW := min(92, maxInt(44, a.width-8))
	innerTextW := maxInt(20, boxOuterW-4) // border(2) + paddingX(2)

	// Package list
	var pkgLines []string
	headerStyle := lipgloss.NewStyle().Foreground(ColorMagenta).Bold(true)
	pkgLines = append(pkgLines, truncateVisible(headerStyle.Render(fmt.Sprintf("   %-25s %-12s %-12s", "PACKAGE", "CURRENT", "LATEST")), innerTextW))
	pkgLines = append(pkgLines, truncateVisible(headerStyle.Render(fmt.Sprintf("   %-25s %-12s %-12s", strings.Repeat("─", 25), strings.Repeat("─", 12), strings.Repeat("─", 12))), innerTextW))

	for i, p := range updates {
		cursor := "  "
		checkbox := "○"
		style := lipgloss.NewStyle().Foreground(ColorText)
		versionStyle := lipgloss.NewStyle().Foreground(ColorYellow)
		newStyle := lipgloss.NewStyle().Foreground(ColorGreen)
		checkStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

		if a.updateSelected[i] {
			checkbox = "●"
			checkStyle = lipgloss.NewStyle().Foreground(ColorCyan)
		}

		if i == a.updateIndex {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			style = style.Bold(true)
		}

		line := fmt.Sprintf("%s%s %-25s %s → %s",
			cursor,
			checkStyle.Render(checkbox),
			style.Render(p.Name),
			versionStyle.Render(p.CurrentVersion),
			newStyle.Render(p.LatestVersion))
		pkgLines = append(pkgLines, truncateVisible(line, innerTextW))
	}

	packageList := strings.Join(pkgLines, "\n")

	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Width(maxInt(1, boxOuterW-2)). // border adds 2
		Render(packageList)

	help := HelpStyle.Render("↑↓ navigate • space select • enter update • a update all • r refresh • esc menu")

	// Build content with optional status line
	var contentParts []string
	contentParts = append(contentParts, tabBar, "", title, subtitle)
	if statusLine != "" {
		contentParts = append(contentParts, statusLine)
	}
	contentParts = append(contentParts, "", listBox, "", help)
	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Top,
		content)
}

// =====================================
// Hotkeys Screen (uses internal/hotkeys package)
// =====================================

// Note: Hotkey definitions are now in internal/hotkeys/hotkeys.go
// This screen renders the dual-pane hotkeys viewer from hotkeys_dualpane.go

// =====================================
// Manage Screen
// =====================================

func (a *App) renderManage() string {
	registry := tools.NewRegistry()

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa")).
		Render("  Manage Tools")

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
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
			Foreground(lipgloss.Color("#cba6f7")).
			Bold(true)
		lines = append(lines, catStyle.Render(fmt.Sprintf("\n  %s", strings.ToUpper(string(cat)))))

		for _, tool := range catTools {
			cursor := "  "
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
			descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

			if itemIndex == a.manageIndex {
				cursor = " "
				nameStyle = nameStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
			}

			status := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("○")
			if tool.IsInstalled() {
				status = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("●")
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
		Foreground(lipgloss.Color("#6c7086")).
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

func (a *App) renderBackups() string {
	// Tab bar at top
	tabBar := RenderTabBar(ScreenBackups, a.width)

	title := TitleStyle.Render("Backups")

	// TODO: Read actual backups from ~/.config/dotfiles/backups/
	message := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Backup management coming soon...\n\nUse CLI: dotfiles backups")

	help := HelpStyle.Render("1-4 switch tabs • esc menu • q quit")

	content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", message, "", help)
	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Top,
		content)
}
