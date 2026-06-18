package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// updateScreen is the migrated ScreenHandler for the package Updates screen.
//
// State stays on App: the cursor (updateIndex), the async check flags
// (updateChecking / updateCheckDone), the cached results (updateResults /
// updateError), the run flags (updateRunning), the selection set
// (updateSelected), and the install-log buffer (installLogs / installLogScroll /
// installLogAutoScroll) are read/written through s.App().
//
// On-enter load: Init() kicks the async update check via checkUpdatesCmd when it
// has not already run/started, mirroring the legacy on-enter trigger. The main
// menu and tab navigation also kick this check before navigating (shared
// startTabTargetLoad / mainMenuScreen.selectItem), and Init() is idempotent, so
// the check runs exactly once however the screen is entered.
//
// Async-in-handler: because the ScreenManager delegates every non-navigation
// message to this handler while it is active, the update async results are
// handled here (not in App.Update): updateCheckDoneMsg, updateRunDoneMsg,
// updateWithLogsMsg, updateSudoRequiredMsg, updateStartMsg. The streaming chain
// is re-issued from the handler:
//   - updateSudoRequiredMsg -> tea.Exec(sudo prompt) whose callback yields
//     updateStartMsg (handled here next)
//   - updateStartMsg -> a.streamingUpdateCmd / a.streamingUpdateAllCmd
//   - updateRunDoneMsg / updateWithLogsMsg success -> checkUpdatesCmd to refresh
//
// Note: the streaming commands emit updateWithLogsMsg (not a per-line stream), so
// there is no separate per-line "listen" continuation to re-arm; the single
// terminal updateWithLogsMsg carries the collected logs. The shared installLogMsg
// (used by both install and update streaming, and not currently emitted by the
// update path) stays in App.Update.
type updateScreen struct {
	BaseScreen
}

// NewUpdateScreen creates a new update screen handler.
func NewUpdateScreen(ctx *ScreenContext) *updateScreen {
	s := &updateScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *updateScreen) ID() Screen { return ScreenUpdate }

// Init kicks the async update check on entry (idempotent).
func (s *updateScreen) Init() tea.Cmd {
	a := s.App()
	if a == nil {
		return nil
	}
	if !a.updateChecking && !a.updateCheckDone {
		a.updateChecking = true
		return checkUpdatesCmd()
	}
	return nil
}

// Update handles keyboard, mouse, and the update async result messages.
func (s *updateScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return s, tea.Quit
		}
		// 'q' quits (no inline edit on this screen; allow even mid-update to match
		// the legacy global quit, which only blocked 'q' during the installer's
		// installRunning, not update runs).
		if msg.String() == "q" {
			return s, tea.Quit
		}
		return s, s.handleKey(msg)

	case tea.MouseMsg:
		return s, s.handleMouse(msg)

	// --- Async results (delegated here while this screen is active) ---
	case updateCheckDoneMsg:
		a.updateChecking = false
		a.updateCheckDone = true
		a.updateResults = msg.updates
		a.updateError = msg.err
		return s, nil

	case updateSudoRequiredMsg:
		// Need to prompt for sudo before update. Re-issue the exec+continuation so
		// the streaming update chain keeps going while this screen is active.
		return s, tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
			if err != nil {
				return updateRunDoneMsg{err: err}
			}
			// Sudo cached, now start the streaming update.
			return updateStartMsg{packages: msg.packages, all: msg.all}
		})

	case updateStartMsg:
		// Start the streaming update (sudo already cached).
		a.clearInstallLogs()
		a.updateRunning = true
		if msg.all {
			return s, a.streamingUpdateAllCmd()
		}
		return s, a.streamingUpdateCmd(msg.packages)

	case updateRunDoneMsg:
		a.updateRunning = false
		a.installLogAutoScroll = false // Allow user to scroll through logs
		if msg.err != nil {
			a.updateStatus = fmt.Sprintf("Update failed: %v", msg.err)
		} else {
			// Count successes and failures
			successes := 0
			failures := 0
			for _, r := range msg.results {
				if r.Success {
					successes++
				} else {
					failures++
				}
			}
			if failures > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d, failed %d", successes, failures)
			} else if successes > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d package(s) ✓", successes)
			} else {
				a.updateStatus = "Update complete ✓"
			}
			// Clear selections and refresh the package list.
			a.updateSelected = make(map[int]bool)
			a.updateCheckDone = false
			a.updateChecking = true
			return s, checkUpdatesCmd()
		}
		return s, nil

	case updateWithLogsMsg:
		// Update completed with logs.
		a.updateRunning = false
		a.installLogAutoScroll = false
		// Append all logs
		for _, line := range msg.logs {
			a.appendInstallLog(line)
		}
		// Process results
		if msg.err != nil {
			a.updateStatus = fmt.Sprintf("Update failed: %v", msg.err)
		} else {
			successes := 0
			failures := 0
			for _, r := range msg.results {
				if r.Success {
					successes++
				} else {
					failures++
				}
			}
			if failures > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d, failed %d", successes, failures)
			} else if successes > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d package(s) ✓", successes)
			} else {
				a.updateStatus = "Update complete ✓"
			}
			// Clear selections and refresh the package list.
			a.updateSelected = make(map[int]bool)
			a.updateCheckDone = false
			a.updateChecking = true
			return s, checkUpdatesCmd()
		}
		return s, nil
	}
	return s, nil
}

// navigateTab routes a management-tab switch through the ScreenManager and kicks
// the destination's on-enter load (shared with the legacy tab navigation).
func (s *updateScreen) navigateTab(target Screen) tea.Cmd {
	a := s.App()
	return tea.Batch(NavigateTo(target), startTabTargetLoad(a, target))
}

func (s *updateScreen) handleKey(msg tea.KeyMsg) tea.Cmd {
	a := s.App()
	key := msg.String()

	// Don't allow actions while update is running.
	if a.updateRunning {
		return nil
	}

	// Handle tab navigation first. A number key for the already-active tab is a
	// no-op.
	if target, ok := tabNavigationTarget(key); ok {
		if target == s.ID() {
			return nil
		}
		return s.navigateTab(target)
	}

	switch key {
	case "up", "k":
		if a.updateIndex > 0 {
			a.updateIndex--
		}
	case "down", "j":
		if a.updateIndex < len(a.updateResults)-1 {
			a.updateIndex++
		}
	case " ": // Toggle selection for batch update
		if len(a.updateResults) > 0 && a.updateIndex < len(a.updateResults) {
			if a.updateSelected[a.updateIndex] {
				delete(a.updateSelected, a.updateIndex)
			} else {
				a.updateSelected[a.updateIndex] = true
			}
		}
	case "enter": // Update selected or current package
		if len(a.updateResults) > 0 && !a.updateChecking && !a.updateRunning {
			var packagesToUpdate []pkg.Package
			if len(a.updateSelected) > 0 {
				// Update selected packages
				for idx := range a.updateSelected {
					if idx < len(a.updateResults) {
						packagesToUpdate = append(packagesToUpdate, a.updateResults[idx])
					}
				}
			} else if a.updateIndex < len(a.updateResults) {
				// Update current package
				packagesToUpdate = append(packagesToUpdate, a.updateResults[a.updateIndex])
			}
			if len(packagesToUpdate) > 0 {
				a.clearInstallLogs()
				a.updateStatus = fmt.Sprintf("Updating %d package(s)...", len(packagesToUpdate))
				return checkSudoAndUpdateCmd(packagesToUpdate, false)
			}
		}
	case "a": // Update all packages
		if len(a.updateResults) > 0 && !a.updateChecking && !a.updateRunning {
			a.clearInstallLogs()
			a.updateStatus = "Updating all packages..."
			return checkSudoAndUpdateCmd(nil, true)
		}
	case "r": // Refresh updates
		a.updateCheckDone = false
		a.updateChecking = true
		a.updateResults = nil
		a.updateError = nil
		a.updateStatus = ""
		a.updateSelected = make(map[int]bool)
		a.clearInstallLogs()
		return checkUpdatesCmd()
	case "c", "C": // Clear logs
		if !a.updateRunning && len(a.installLogs) > 0 {
			a.clearInstallLogs()
			a.updateStatus = "Logs cleared"
		}
	case "pgup", "ctrl+u": // Scroll logs up
		if len(a.installLogs) > 0 {
			a.installLogScroll += 10
			maxScroll := CalculateMaxLogScroll(len(a.installLogs), a.height-14)
			if a.installLogScroll > maxScroll {
				a.installLogScroll = maxScroll
			}
			a.installLogAutoScroll = false
		}
	case "pgdown", "ctrl+d": // Scroll logs down
		if len(a.installLogs) > 0 {
			a.installLogScroll -= 10
			if a.installLogScroll < 0 {
				a.installLogScroll = 0
			}
		}
	case "esc":
		// ScreenMainMenu is migrated; route through the ScreenManager.
		return NavigateTo(ScreenMainMenu)
	}
	return nil
}

// handleMouse handles tab-bar clicks on the update screen (routes through the
// ScreenManager via NavigateTo).
func (s *updateScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	// Only handle left clicks on the tab bar (Y=0).
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}
	if m.Y != 0 {
		return nil
	}
	// Ignore a click on the already-active tab (this screen).
	if screen, _ := a.detectTabClick(m.X); screen != 0 && screen != s.ID() {
		return s.navigateTab(screen)
	}
	return nil
}

// View renders the Updates screen.
func (s *updateScreen) View(width, height int) string {
	a := s.App()

	// Tab bar at top
	tabBar := RenderTabBar(ScreenUpdate, width)

	title := TitleStyle.Render("Package Updates")

	// Check if we're running an update or have logs to show
	if a.updateRunning || len(a.installLogs) > 0 {
		return s.viewWithLogs(width, height, tabBar, title)
	}

	// Check if we're still loading
	if a.updateChecking {
		spinnerText := "Checking for updates..."
		if a.animationsEnabled {
			spinnerText = AnimatedSpinnerDots(a.uiFrame) + " Checking for updates..."
		}
		body := lipgloss.NewStyle().Foreground(ColorCyan).Render(spinnerText)
		progressBar := ProgressBarAnimated(0.5, min(60, width-20), a.uiFrame)
		help := HelpStyle.Render("1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, progressBar, "", help)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
	}

	// Check for errors
	if a.updateError != nil {
		body := lipgloss.NewStyle().Foreground(ColorRed).Render(fmt.Sprintf("Error: %v", a.updateError))
		help := HelpStyle.Render("r refresh • 1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
	}

	// Check if no package manager detected (results will be nil with no error)
	mgr := pkg.DetectManager()
	if mgr == nil {
		body := lipgloss.NewStyle().Foreground(ColorRed).Render("No package manager detected")
		help := HelpStyle.Render("1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
	}

	updates := a.updateResults

	if len(updates) == 0 {
		body := lipgloss.NewStyle().Foreground(ColorGreen).Render("All packages are up to date!")
		help := HelpStyle.Render("r refresh • 1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
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

	boxOuterW := min(92, maxInt(44, width-8))
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

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Top,
		content)
}

// viewWithLogs renders the update screen with the streaming log panel.
func (s *updateScreen) viewWithLogs(width, height int, tabBar, title string) string {
	a := s.App()

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
	panelW := min(100, width-4)
	panelH := height - 10 // Leave room for header/footer

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
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
}
