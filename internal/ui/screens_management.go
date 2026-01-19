package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

	// Check if we're running an update or have logs to show
	if a.updateRunning || len(a.installLogs) > 0 {
		return a.renderUpdateWithLogs(tabBar, title)
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

// renderUpdateWithLogs renders the update screen with log panel
func (a *App) renderUpdateWithLogs(tabBar, title string) string {
	// Build title with status
	var statusTitle string
	if a.updateRunning {
		spinner := AnimatedSpinnerDots(a.uiFrame)
		if !a.animationsEnabled {
			spinner = "..."
		}
		statusTitle = fmt.Sprintf("UPDATING %s", spinner)
	} else {
		statusTitle = "UPDATE LOG"
	}

	logPanelTitle := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render(statusTitle)

	// Calculate log panel dimensions
	panelW := min(100, a.width-4)
	panelH := a.height - 10 // Leave room for header/footer

	// Calculate visible log range
	innerHeight := maxInt(1, panelH-4)
	totalLines := len(a.installLogs)

	var logLines []string
	if totalLines == 0 {
		if a.updateRunning {
			logLines = append(logLines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Waiting for output..."))
		} else {
			logLines = append(logLines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No logs"))
		}
	} else {
		// Calculate range (scroll from bottom)
		endIdx := totalLines - a.installLogScroll
		if endIdx > totalLines {
			endIdx = totalLines
		}
		if endIdx < 0 {
			endIdx = 0
		}
		startIdx := endIdx - innerHeight
		if startIdx < 0 {
			startIdx = 0
		}

		innerWidth := panelW - 4
		for i := startIdx; i < endIdx; i++ {
			line := a.installLogs[i]
			if lipgloss.Width(line) > innerWidth {
				line = truncateVisible(line, innerWidth)
			}
			logLines = append(logLines, line)
		}
	}

	// Pad to fill height
	for len(logLines) < innerHeight {
		logLines = append([]string{""}, logLines...)
	}

	// Build log box
	borderColor := ColorCyan
	if !a.updateRunning {
		borderColor = ColorBorder
	}

	logBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(maxInt(1, panelW-2)).
		Height(maxInt(1, panelH-2)).
		Render(lipgloss.JoinVertical(lipgloss.Left, logPanelTitle, "", strings.Join(logLines, "\n")))

	// Status line
	var statusLine string
	if a.updateStatus != "" {
		statusStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if strings.Contains(a.updateStatus, "failed") {
			statusStyle = lipgloss.NewStyle().Foreground(ColorRed)
		} else if strings.Contains(a.updateStatus, "✓") {
			statusStyle = lipgloss.NewStyle().Foreground(ColorGreen)
		}
		statusLine = statusStyle.Render(a.updateStatus)
	}

	// Help text
	var help string
	if a.updateRunning {
		help = HelpStyle.Render("updating... please wait")
	} else {
		help = HelpStyle.Render("c clear logs • pgup/pgdn scroll • r refresh • esc menu")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, statusLine, "", logBox, "", help)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
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

func (a *App) renderBackups() string {
	// Tab bar at top
	tabBar := RenderTabBar(ScreenBackups, a.width)

	title := TitleStyle.Render("Backups")

	// Check if we're still loading
	if a.backupsLoading {
		spinnerText := "Loading backups..."
		if a.animationsEnabled {
			spinnerText = AnimatedSpinnerDots(a.uiFrame) + " Loading backups..."
		}
		body := lipgloss.NewStyle().Foreground(ColorCyan).Render(spinnerText)
		help := HelpStyle.Render("1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	// Check for errors
	if a.backupError != nil {
		body := lipgloss.NewStyle().Foreground(ColorRed).Render(fmt.Sprintf("Error: %v", a.backupError))
		help := HelpStyle.Render("r refresh • 1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	// Build subtitle
	subtitleText := fmt.Sprintf("%d backup(s) available", len(a.backups))
	subtitle := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(subtitleText)

	// Show status message if any
	var statusLine string
	if a.backupStatus != "" {
		statusStyle := lipgloss.NewStyle().Foreground(ColorYellow)
		if strings.Contains(a.backupStatus, "Restored") || strings.Contains(a.backupStatus, "Created") {
			statusStyle = lipgloss.NewStyle().Foreground(ColorGreen)
		} else if strings.Contains(a.backupStatus, "failed") || strings.Contains(a.backupStatus, "Error") {
			statusStyle = lipgloss.NewStyle().Foreground(ColorRed)
		} else if a.backupConfirmMode {
			statusStyle = lipgloss.NewStyle().Foreground(ColorMagenta).Bold(true)
		}
		statusLine = statusStyle.Render(a.backupStatus)
	}

	// Check if no backups
	if len(a.backups) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No backups found.\n\nPress 'n' to create a new backup.")
		var helpText string
		if a.backupRunning {
			helpText = "creating backup... please wait"
		} else {
			helpText = "n new backup • r refresh • 1-4 switch tabs • esc menu • q quit"
		}
		help := HelpStyle.Render(helpText)

		var contentParts []string
		contentParts = append(contentParts, tabBar, "", title, subtitle)
		if statusLine != "" {
			contentParts = append(contentParts, statusLine)
		}
		contentParts = append(contentParts, "", emptyMsg, "", help)
		content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, content)
	}

	// Clamp cursor to actual list length
	if a.backupIndex < 0 {
		a.backupIndex = 0
	}
	if a.backupIndex > len(a.backups)-1 {
		a.backupIndex = len(a.backups) - 1
	}

	boxOuterW := min(92, maxInt(44, a.width-8))
	innerTextW := maxInt(20, boxOuterW-4) // border(2) + paddingX(2)

	// Backup list header
	var backupLines []string
	headerStyle := lipgloss.NewStyle().Foreground(ColorMagenta).Bold(true)
	backupLines = append(backupLines, truncateVisible(headerStyle.Render(fmt.Sprintf("   %-24s %-16s %6s %8s", "NAME", "DATE", "FILES", "SIZE")), innerTextW))
	backupLines = append(backupLines, truncateVisible(headerStyle.Render(fmt.Sprintf("   %-24s %-16s %6s %8s", strings.Repeat("-", 24), strings.Repeat("-", 16), strings.Repeat("-", 6), strings.Repeat("-", 8))), innerTextW))

	// List backups
	for i, b := range a.backups {
		cursor := "  "
		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		dateStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		countStyle := lipgloss.NewStyle().Foreground(ColorCyan)
		sizeStyle := lipgloss.NewStyle().Foreground(ColorYellow)

		if i == a.backupIndex {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("> ")
			nameStyle = nameStyle.Foreground(ColorCyan).Bold(true)
			dateStyle = dateStyle.Foreground(ColorText)
		}

		// Format date
		dateStr := b.Timestamp.Format("Jan 02 15:04")

		// Format size
		sizeStr := formatBytes(b.Size)

		// Truncate name if needed
		displayName := b.Name
		if len(displayName) > 24 {
			displayName = displayName[:21] + "..."
		}

		line := fmt.Sprintf("%s%-24s %s %s %s",
			cursor,
			nameStyle.Render(displayName),
			dateStyle.Render(fmt.Sprintf("%-16s", dateStr)),
			countStyle.Render(fmt.Sprintf("%6d", b.FileCount)),
			sizeStyle.Render(fmt.Sprintf("%8s", sizeStr)))
		backupLines = append(backupLines, truncateVisible(line, innerTextW))
	}

	backupList := strings.Join(backupLines, "\n")

	// Style the list box
	borderColor := ColorBorder
	if a.backupConfirmMode {
		borderColor = ColorMagenta
	}

	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(maxInt(1, boxOuterW-2)). // border adds 2
		Render(backupList)

	// Details panel for selected backup
	var detailsBox string
	if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
		selected := a.backups[a.backupIndex]
		detailLines := []string{
			lipgloss.NewStyle().Foreground(ColorMagenta).Bold(true).Render("DETAILS"),
			"",
			fmt.Sprintf("%s %s", lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Name:"), lipgloss.NewStyle().Foreground(ColorText).Render(selected.Name)),
			fmt.Sprintf("%s %s", lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Date:"), lipgloss.NewStyle().Foreground(ColorText).Render(selected.Timestamp.Format("2006-01-02 15:04:05"))),
			fmt.Sprintf("%s %d", lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Files:"), selected.FileCount),
			fmt.Sprintf("%s %s", lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Size:"), formatBytes(selected.Size)),
			fmt.Sprintf("%s %s", lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Path:"), lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(selected.Path, 40))),
		}
		detailsContent := strings.Join(detailLines, "\n")
		detailsBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1).
			Width(maxInt(1, boxOuterW-2)).
			Render(detailsContent)
	}

	// Help text
	var helpText string
	if a.backupRunning {
		helpText = "please wait..."
	} else if a.backupConfirmMode {
		helpText = "y confirm • n cancel"
	} else {
		helpText = "up/down navigate • enter restore • d delete • n new backup • r refresh • esc menu"
	}
	help := HelpStyle.Render(helpText)

	// Build content with optional status line
	var contentParts []string
	contentParts = append(contentParts, tabBar, "", title, subtitle)
	if statusLine != "" {
		contentParts = append(contentParts, statusLine)
	}
	contentParts = append(contentParts, "", listBox)
	if detailsBox != "" {
		contentParts = append(contentParts, "", detailsBox)
	}
	contentParts = append(contentParts, "", help)
	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Top,
		content)
}

// =====================================
// Mouse Handlers
// =====================================

// handleMainMenuMouse handles mouse clicks on the main menu
func (a *App) handleMainMenuMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Only handle left clicks
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	items := GetMainMenuItems()
	if len(items) == 0 {
		return a, nil
	}

	// The main menu is centered. Calculate the content boundaries.
	// Content structure: title(1), subtitle(1), empty(1), menu items(n), empty(1), help(1)
	// Total content height = 5 + len(items)
	contentH := 5 + len(items)
	startY := (a.height - contentH) / 2 // Center vertically

	// Menu items start at line 3 (after title, subtitle, empty)
	menuStartY := startY + 3

	// Check if click is within menu area
	for i := range items {
		itemY := menuStartY + i
		if m.Y == itemY {
			// Select this item
			a.mainMenuIndex = i
			// Trigger enter action
			targetScreen := items[i].Screen
			a.screen = targetScreen
			// Start async operations for screens that need it
			switch targetScreen {
			case ScreenManage:
				if cmd := a.startInstallCacheLoad(); cmd != nil {
					return a, cmd
				}
			case ScreenBackups:
				if !a.backupsLoading && !a.backupsLoaded {
					a.backupsLoading = true
					return a, loadBackupsCmd()
				}
			case ScreenUpdate:
				if !a.updateChecking && !a.updateCheckDone {
					a.updateChecking = true
					return a, checkUpdatesCmd()
				}
			case ScreenUsers:
				if !a.usersLoaded {
					a.usersLoaded = true
					return a, loadUsersCmd()
				}
			case ScreenHotkeys:
				a.hotkeysReturn = ScreenMainMenu
			}
			return a, nil
		}
	}

	return a, nil
}

// handleBackupsMouse handles mouse clicks on the backups screen
func (a *App) handleBackupsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle tab bar clicks (Y=0 is the tab bar line)
	if m.Y == 0 && m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		if screen, cmd := a.detectTabClick(m.X); screen != 0 {
			a.screen = screen
			return a, cmd
		}
	}

	// Handle mouse wheel scrolling for backup list
	if m.IsWheel() && len(a.backups) > 0 {
		delta := 0
		switch m.Button {
		case tea.MouseButtonWheelUp:
			delta = -1
		case tea.MouseButtonWheelDown:
			delta = 1
		default:
			return a, nil
		}
		a.backupIndex = clampInt(a.backupIndex+delta, 0, len(a.backups)-1)
		return a, nil
	}

	// Only handle left clicks for list selection
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Skip if loading or no backups
	if a.backupsLoading || len(a.backups) == 0 {
		return a, nil
	}

	// The backup list starts after: tabBar(1), empty(1), title(1), subtitle(1), status?(1), empty(1), header(1), divider(1)
	// So list items start around Y=7-8 depending on status
	listStartY := 7
	if a.backupStatus != "" {
		listStartY = 8
	}

	// Check if click is within list area
	clickedIndex := m.Y - listStartY
	if clickedIndex >= 0 && clickedIndex < len(a.backups) {
		a.backupIndex = clickedIndex
		return a, nil
	}

	return a, nil
}
