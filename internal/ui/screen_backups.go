package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// backupsScreen is the migrated ScreenHandler for the Backups management screen.
//
// State stays on App: the cursor (backupIndex), the loaded list (backups /
// backupsLoaded / backupsLoading), the confirm/run flags (backupConfirmMode,
// backupConfirmType, backupRunning), and the status/error fields are read/written
// through s.App().
//
// On-enter load: Init() kicks the async backup-list load via loadBackupsCmd when
// it has not already loaded/started, mirroring the legacy on-enter trigger. The
// main menu and tab navigation also kick this load before navigating (shared
// startTabTargetLoad / mainMenuScreen.selectItem), and Init() is idempotent, so
// the list loads exactly once however the screen is entered.
//
// Async-in-handler: because the ScreenManager delegates every non-navigation
// message to this handler while it is active, the backup async results are
// handled here (not in App.Update): backupsLoadedMsg, backupRestoreDoneMsg,
// backupDeleteDoneMsg, backupCreateDoneMsg. The delete/create handlers re-issue
// loadBackupsCmd to refresh the list, keeping the async chain going.
type backupsScreen struct {
	BaseScreen
}

// NewBackupsScreen creates a new backups screen handler.
func NewBackupsScreen(ctx *ScreenContext) *backupsScreen {
	s := &backupsScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *backupsScreen) ID() Screen { return ScreenBackups }

// Init kicks the async backup-list load on entry (idempotent).
func (s *backupsScreen) Init() tea.Cmd {
	a := s.App()
	if a == nil {
		return nil
	}
	if !a.backupsLoading && !a.backupsLoaded {
		a.backupsLoading = true
		return loadBackupsCmd()
	}
	return nil
}

// Update handles keyboard, mouse, and the backup async result messages.
func (s *backupsScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == keyCtrlC {
			return s, tea.Quit
		}
		// 'q' quits (no install/edit in-progress on this screen).
		if msg.String() == "q" {
			return s, tea.Quit
		}
		return s, s.handleKey(msg)

	case tea.MouseMsg:
		return s, s.handleMouse(msg)

	// --- Async results (delegated here while this screen is active) ---
	case backupsLoadedMsg:
		a.backupsLoading = false
		a.backupsLoaded = true
		if msg.err != nil {
			a.backupError = msg.err
			a.backups = []BackupEntry{}
		} else {
			a.backups = msg.backups
			a.backupError = nil
		}
		return s, nil

	case backupRestoreDoneMsg:
		a.handleBackupRestoreDoneMsg(msg)
		return s, nil

	case backupDeleteDoneMsg:
		return s, a.handleBackupDeleteDoneMsg(msg)

	case backupCreateDoneMsg:
		return s, a.handleBackupCreateDoneMsg(msg)
	}
	return s, nil
}

// navigateTab routes a management-tab switch through the ScreenManager and kicks
// the destination's on-enter load (shared with the legacy tab navigation).
func (s *backupsScreen) navigateTab(target Screen) tea.Cmd {
	a := s.App()
	return tea.Batch(NavigateTo(target), startTabTargetLoad(a, target))
}

func (s *backupsScreen) handleKey(msg tea.KeyMsg) tea.Cmd {
	a := s.App()
	key := msg.String()

	// Don't allow actions while a backup operation is running.
	if a.backupRunning {
		return nil
	}

	// Handle confirmation mode.
	if a.backupConfirmMode {
		return s.backupsHandleConfirmKey(key)
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
		if a.backupIndex > 0 {
			a.backupIndex--
		}
	case keyDown, "j":
		if len(a.backups) > 0 && a.backupIndex < len(a.backups)-1 {
			a.backupIndex++
		}
	case keyEnter: // Restore selected backup
		s.backupsEnterConfirm("restore", "Restore backup '%s'? (y/n)")
	case "d", "D": // Delete selected backup
		s.backupsEnterConfirm(keyDelete, "Delete backup '%s'? (y/n)")
	case "n", "N": // Create new backup
		a.backupRunning = true
		a.backupStatus = "Creating backup..."
		return createBackupCmd()
	case "r", "R": // Refresh backup list
		a.backupsLoaded = false
		a.backupsLoading = true
		a.backupStatus = ""
		a.backupError = nil
		return loadBackupsCmd()
	case keyEsc:
		// ScreenMainMenu is migrated; route through the ScreenManager.
		return NavigateTo(ScreenMainMenu)
	}
	return nil
}

// backupsHandleConfirmKey handles a key event while a restore/delete
// confirmation prompt is active.
func (s *backupsScreen) backupsHandleConfirmKey(key string) tea.Cmd {
	a := s.App()
	switch key {
	case "y", "Y":
		if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
			a.backupRunning = true
			backup := a.backups[a.backupIndex]
			switch a.backupConfirmType {
			case "restore":
				return restoreBackupCmd(backup)
			case keyDelete:
				return deleteBackupCmd(backup)
			}
		}
		a.backupConfirmMode = false
	case "n", "N", keyEsc:
		a.backupConfirmMode = false
		a.backupStatus = ""
	}
	return nil
}

// backupsEnterConfirm arms a confirmation prompt of the given type for the
// selected backup, formatting statusFmt with the backup name.
func (s *backupsScreen) backupsEnterConfirm(confirmType, statusFmt string) {
	a := s.App()
	if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
		a.backupConfirmMode = true
		a.backupConfirmType = confirmType
		a.backupStatus = fmt.Sprintf(statusFmt, a.backups[a.backupIndex].Name)
	}
}

// handleMouse handles mouse clicks on the backups screen.
func (s *backupsScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	// Handle tab bar clicks (Y=0 is the tab bar line). Ignore a click on the
	// already-active tab (this screen).
	if m.Y == 0 && m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		if screen := a.detectTabClick(m.X); screen != 0 && screen != s.ID() {
			return s.navigateTab(screen)
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
		case tea.MouseButtonNone, tea.MouseButtonLeft, tea.MouseButtonMiddle,
			tea.MouseButtonRight, tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight,
			tea.MouseButtonBackward, tea.MouseButtonForward, tea.MouseButton10, tea.MouseButton11:
			return nil
		}
		a.backupIndex = clampInt(a.backupIndex+delta, 0, len(a.backups)-1)
		return nil
	}

	// Only handle left clicks for list selection
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}

	// Skip if loading or no backups
	if a.backupsLoading || len(a.backups) == 0 {
		return nil
	}

	// The View is top-aligned (lipgloss.Place(Center, Top)), so the first row is
	// at Y=0. Derive the first backup row from the same constants the View uses
	// to compose its content:
	//   tabBar(1) + blank(1) + title(1) + subtitle(1) + [status(1)] + blank(1) +
	//   listBox top border(1) + NAME/DATE header(1) + dashes divider(1)
	// so backup[0] is at Y=8 (no status) / Y=9 (with status). The legacy value
	// (7/8) pointed at the dashes row, selecting one row too low (C18).
	listStartY := 8
	if a.backupStatus != "" {
		listStartY = 9
	}

	// X-bounds: the list box is LEFT-aligned at absolute X=0, not centered. The
	// View composes a full-width RenderTabBar first in JoinVertical(Left, ...),
	// which pins the whole content block to the full terminal width, so the
	// lipgloss.Place(Center, ...) adds zero left pad and the box lands at X=0. The
	// previous centered boxLeft := (width-boxOuterW)/2 dropped clicks on the left
	// columns (cursor/name) and wrongly accepted clicks in the empty strip to the
	// right (FIX 4). Accept [0, boxOuterW).
	boxOuterW := min(92, maxInt(44, a.width-8))
	if m.X < 0 || m.X >= boxOuterW {
		return nil
	}

	// Check if click is within list area
	clickedIndex := m.Y - listStartY
	if clickedIndex >= 0 && clickedIndex < len(a.backups) {
		a.backupIndex = clickedIndex
		return nil
	}

	return nil
}

// View renders the Backups screen.
func (s *backupsScreen) View(width, height int) string {
	a := s.App()

	// Tab bar at top
	tabBar := RenderTabBar(ScreenBackups, width)

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
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
	}

	// Check for errors
	if a.backupError != nil {
		body := lipgloss.NewStyle().Foreground(ColorRed).Render(fmt.Sprintf("Error: %v", a.backupError))
		help := HelpStyle.Render("r refresh • 1-4 switch tabs • esc menu • q quit")
		content := lipgloss.JoinVertical(lipgloss.Left, tabBar, "", title, "", body, "", help)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
	}

	// Build subtitle
	subtitleText := fmt.Sprintf("%d backup(s) available", len(a.backups))
	subtitle := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(subtitleText)

	// Show status message if any
	statusLine := a.backupsRenderStatusLine()

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
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Top, content)
	}

	// Clamp cursor to actual list length
	if a.backupIndex < 0 {
		a.backupIndex = 0
	}
	if a.backupIndex > len(a.backups)-1 {
		a.backupIndex = len(a.backups) - 1
	}

	boxOuterW := min(92, maxInt(44, width-8))
	innerTextW := maxInt(20, boxOuterW-4) // border(2) + paddingX(2)

	listBox := a.backupsRenderListBox(boxOuterW, innerTextW)

	// Details panel for selected backup
	detailsBox := a.backupsRenderDetailsBox(boxOuterW)

	// Help text
	help := HelpStyle.Render(a.backupsHelpText())

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

	return lipgloss.Place(width, height,
		lipgloss.Center, lipgloss.Top,
		content)
}

// backupsRenderStatusLine renders the styled status message, or "" when none is
// set.
func (a *App) backupsRenderStatusLine() string {
	if a.backupStatus == "" {
		return ""
	}
	statusStyle := lipgloss.NewStyle().Foreground(ColorYellow)
	switch {
	case strings.Contains(a.backupStatus, "skipped"):
		// Partial/failed restore: keep the warning (yellow) style even though
		// the message contains "Restored" (C3).
		statusStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	case strings.Contains(a.backupStatus, "Restored") || strings.Contains(a.backupStatus, "Created"):
		statusStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	case strings.Contains(a.backupStatus, "failed") || strings.Contains(a.backupStatus, "Error"):
		statusStyle = lipgloss.NewStyle().Foreground(ColorRed)
	case a.backupConfirmMode:
		statusStyle = lipgloss.NewStyle().Foreground(ColorMagenta).Bold(true)
	}
	return statusStyle.Render(a.backupStatus)
}

// backupsRenderListBox renders the bordered backup list (header + one row per
// backup).
func (a *App) backupsRenderListBox(boxOuterW, innerTextW int) string {
	// Backup list header
	backupLines := make([]string, 0, len(a.backups)+2)
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

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(maxInt(1, boxOuterW-2)). // border adds 2
		Render(backupList)
}

// backupsRenderDetailsBox renders the details panel for the selected backup, or
// "" when there is no valid selection.
func (a *App) backupsRenderDetailsBox(boxOuterW int) string {
	if len(a.backups) == 0 || a.backupIndex >= len(a.backups) {
		return ""
	}
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
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Width(maxInt(1, boxOuterW-2)).
		Render(detailsContent)
}

// backupsHelpText returns the footer help text for the loaded-list state.
func (a *App) backupsHelpText() string {
	switch {
	case a.backupRunning:
		return "please wait..."
	case a.backupConfirmMode:
		return "y confirm • n cancel"
	default:
		return "up/down navigate • enter restore • d delete • n new backup • r refresh • esc menu"
	}
}
