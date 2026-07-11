package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// mainMenuScreen is the migrated ScreenHandler for the management platform's
// main menu.
//
// State stays on App: the cursor (mainMenuIndex) and the async loading flags
// for the destination screens are read/written through s.App(). Selecting an
// item navigates through the ScreenManager (NavigateTo) and batches any
// on-enter async load the destination needs (install cache, backups, updates,
// users), preserving the legacy behavior.
type mainMenuScreen struct {
	BaseScreen
}

// NewMainMenuScreen creates a new main menu screen handler.
func NewMainMenuScreen(ctx *ScreenContext) *mainMenuScreen {
	s := &mainMenuScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *mainMenuScreen) ID() Screen { return ScreenMainMenu }

// Init returns any initial commands (none on entry).
func (s *mainMenuScreen) Init() tea.Cmd { return nil }

// selectItem navigates to the given menu item's screen and triggers any
// on-enter async load that screen requires. It always routes navigation through
// the ScreenManager.
func (s *mainMenuScreen) selectItem(index int) tea.Cmd {
	a := s.App()
	items := GetMainMenuItems()
	if index < 0 || index >= len(items) {
		return nil
	}
	a.mainMenuIndex = index
	target := items[index].Screen

	nav := NavigateTo(target)

	// Start async operations for screens that need it (preserve legacy on-enter
	// behavior that used to fire when the App switched to the target screen).
	switch target { //nolint:exhaustive // Only menu destinations with on-enter work need cases.
	case ScreenManage:
		if cmd := a.startInstallCacheLoad(); cmd != nil {
			return tea.Batch(nav, cmd)
		}
	case ScreenBackups:
		if !a.backupsLoading && !a.backupsLoaded {
			a.backupsLoading = true
			return tea.Batch(nav, loadBackupsCmd())
		}
	case ScreenUpdate:
		if !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return tea.Batch(nav, checkUpdatesCmd())
		}
	case ScreenUsers:
		if !a.usersLoaded {
			a.usersLoaded = true
			return tea.Batch(nav, loadUsersCmd())
		}
	case ScreenHotkeys:
		a.hotkeysReturn = ScreenMainMenu
	case ScreenThemePicker:
		// Entered standalone from the main menu: selecting a theme should apply
		// it and return here, NOT advance through the install wizard (C7, C8).
		a.themeStandalone = true
		a.themeReturn = ScreenMainMenu
	}
	return nav
}

// Update handles keyboard and mouse input for the main menu.
func (s *mainMenuScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := GetMainMenuItems()
		switch msg.String() {
		case "ctrl+c", "q":
			return s, tea.Quit
		case "up", "k":
			if a.mainMenuIndex > 0 {
				a.mainMenuIndex--
			}
		case "down", "j":
			if a.mainMenuIndex < len(items)-1 {
				a.mainMenuIndex++
			}
		case "enter":
			return s, s.selectItem(a.mainMenuIndex)
		}

	case tea.MouseMsg:
		return s, s.handleMouse(msg)
	}
	return s, nil
}

// handleMouse handles click-to-select on the centered menu list. Mirrors the
// legacy handleMainMenuMouse behavior.
func (s *mainMenuScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	m := tea.MouseEvent(msg)

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}

	items := GetMainMenuItems()
	if len(items) == 0 {
		return nil
	}

	// Resolve the click against the per-item geometry recorded by the most
	// recent View (a.mainMenuLayout). It is derived from the actually rendered
	// container, so it accounts for ContainerStyle's border/padding and the
	// HelpStyle padding the legacy anchor ignored.
	a := s.App()
	fl := a.mainMenuLayout
	if fl.hasXBounds && (m.X < fl.boxLeft || m.X > fl.boxRight) {
		return nil
	}
	if idx, ok := fl.fieldAt(m.Y); ok {
		if idx >= 0 && idx < len(items) {
			return s.selectItem(idx)
		}
	}
	return nil
}

// View renders the management main menu.
func (s *mainMenuScreen) View(width, height int) string {
	a := s.App()

	items := GetMainMenuItems()

	maxLineW := maxInt(20, width-10)

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

	container := ContainerStyle.Render(content)

	// Record per-item geometry for the mouse handler. Menu rows live at content
	// offset title(1)+subtitle(1)+blank(1) = 3.
	a.mainMenuLayout = centeredContainerListLayout(width, height, container,
		len(items), lipgloss.Height(title)+lipgloss.Height(subtitle)+1)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, container)
}
