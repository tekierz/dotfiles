package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// deepDiveMenuScreen is the migrated ScreenHandler for the deep-dive tool
// selection menu.
//
// State stays on App: the cursor (deepDiveMenuIndex), the install-status cache
// (manageInstalled / installCacheLoading) and the deep-dive config are all read
// through s.App(). Selecting a tool navigates to its config screen via the
// manager (NavigateTo, which falls back to legacy mode for any not-yet-migrated
// target); "Continue to Installation" routes to the (migrated) theme picker.
//
// On-enter cache loading: the install cache load is kicked off by the welcome
// screen before it navigates here (welcomeScreen.Update batches
// startInstallCacheLoad with NavigateTo(ScreenDeepDiveMenu)). Init() also calls
// startInstallCacheLoad as a safety net so entering the menu by any other route
// still triggers the load. startInstallCacheLoad is idempotent (it returns nil
// when the cache is already ready or loading), and the installCacheDoneMsg that
// clears installCacheLoading is handled in App.Update — kept there because the
// state lives on App and the same message is shared with the Manage screen, so
// moving it would be higher risk for no benefit.
type deepDiveMenuScreen struct {
	BaseScreen
}

// NewDeepDiveMenuScreen creates a new deep dive menu screen handler.
func NewDeepDiveMenuScreen(ctx *ScreenContext) *deepDiveMenuScreen {
	s := &deepDiveMenuScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *deepDiveMenuScreen) ID() Screen { return ScreenDeepDiveMenu }

// Init triggers the async install-cache load on entry (idempotent).
func (s *deepDiveMenuScreen) Init() tea.Cmd {
	if a := s.App(); a != nil {
		return a.startInstallCacheLoad()
	}
	return nil
}

// selectItem navigates to the chosen menu item's config screen (or to the
// installation/theme step for the "Continue" row).
func (s *deepDiveMenuScreen) selectItem(index int) tea.Cmd {
	a := s.App()
	items := GetFilteredDeepDiveMenuItems()
	if index == len(items) {
		// "Continue to Installation" selected. ScreenThemePicker is migrated;
		// route through the ScreenManager.
		return NavigateTo(ScreenThemePicker)
	}
	if index < 0 || index >= len(items) {
		return nil
	}
	a.configFieldIndex = 0
	// NavigateTo falls back to legacy mode for any config screen not yet
	// migrated, syncing a.screen in App.Update.
	return NavigateTo(items[index].Screen)
}

// Update handles keyboard and mouse input for the deep dive menu.
func (s *deepDiveMenuScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		menuItems := GetFilteredDeepDiveMenuItems()
		maxIdx := len(menuItems) // last index is the "Continue" option
		switch msg.String() {
		case "ctrl+c", "q":
			return s, tea.Quit
		case "up", "k":
			if a.deepDiveMenuIndex > 0 {
				a.deepDiveMenuIndex--
			}
		case "down", "j":
			if a.deepDiveMenuIndex < maxIdx {
				a.deepDiveMenuIndex++
			}
		case "enter":
			return s, s.selectItem(a.deepDiveMenuIndex)
		case "esc":
			return s, NavigateTo(ScreenWelcome)
		}

	case tea.MouseMsg:
		return s, s.handleMouse(msg)
	}
	return s, nil
}

// handleMouse mirrors the legacy handleDeepDiveMenuMouse behavior: scroll wheel
// moves the cursor and a left click on a menu row selects + enters it.
func (s *deepDiveMenuScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	items := GetFilteredDeepDiveMenuItems()

	if m.Button == tea.MouseButtonWheelUp {
		if a.deepDiveMenuIndex > 0 {
			a.deepDiveMenuIndex--
		}
		return nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		if a.deepDiveMenuIndex < len(items)-1 {
			a.deepDiveMenuIndex++
		}
		return nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}

	contentHeight := len(items) + 10 // items + headers + padding
	startY := (a.height - contentHeight) / 2
	listStartY := startY + 4 // After title and instructions

	// Account for category headers (they take up a line but aren't clickable).
	clickableY := listStartY
	for i, item := range items {
		if item.Category != "" {
			clickableY++ // Category header takes a line
		}
		if m.Y == clickableY {
			a.deepDiveMenuIndex = i
			return s.selectItem(i)
		}
		clickableY++
	}
	return nil
}

// View renders the deep dive tool selection menu.
func (s *deepDiveMenuScreen) View(width, height int) string {
	a := s.App()

	// Show loading state if cache is being populated.
	if a.installCacheLoading {
		spinner := AnimatedSpinnerDots(a.uiFrame)
		loadingStyle := lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)
		loadingText := loadingStyle.Render(fmt.Sprintf("%s Loading installation status...", spinner))

		return lipgloss.Place(
			width, height,
			lipgloss.Center, lipgloss.Center,
			loadingText,
		)
	}

	// Title with decorative border.
	titleBox := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorMagenta).
		Padding(0, 2).
		Render(lipgloss.NewStyle().
			Foreground(ColorMagenta).
			Bold(true).
			Render("◈ DEEP DIVE CONFIGURATION ◈"))

	subtitle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("Customize each tool before installation")

	items := GetFilteredDeepDiveMenuItems()
	var menuList strings.Builder

	// Category header style.
	categoryStyle := lipgloss.NewStyle().
		Foreground(ColorMagenta).
		Bold(true).
		MarginTop(1)

	for i, item := range items {
		// Render category header if this item starts a new category.
		if item.Category != "" {
			if i > 0 {
				menuList.WriteString("\n")
			}
			menuList.WriteString(categoryStyle.Render("  "+item.Category) + "\n")
		}

		isSelected := i == a.deepDiveMenuIndex

		// Get install status for this item.
		installStatus := a.getDeepDiveItemStatus(item)
		statusDot := StatusDot(installStatus)

		// Icon.
		iconStyle := unfocusedStyle
		if isSelected {
			iconStyle = lipgloss.NewStyle().Foreground(ColorNeonPink)
		}

		// Name.
		nameStyle := unfocusedStyle
		if isSelected {
			nameStyle = focusedStyle
		}

		// Description.
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if isSelected {
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		// Cursor.
		cursor := "  "
		if isSelected {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		}

		menuList.WriteString(fmt.Sprintf("%s%s %s %s  %s\n",
			cursor,
			statusDot,
			iconStyle.Render(item.Icon),
			nameStyle.Render(fmt.Sprintf("%-14s", item.Name)),
			descStyle.Render(item.Description),
		))
	}

	// Continue option.
	continueIdx := len(items)
	continueSelected := a.deepDiveMenuIndex == continueIdx
	continueCursor := "  "
	continueStyle := unfocusedStyle
	if continueSelected {
		continueCursor = lipgloss.NewStyle().Foreground(ColorGreen).Render("▸ ")
		continueStyle = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	}
	menuList.WriteString("\n")
	menuList.WriteString(fmt.Sprintf("%s%s\n", continueCursor, continueStyle.Render("▶ Continue to Installation")))

	// Wrap menu in a box.
	menuBox := configBoxStyle.Width(a.deepDiveBoxWidth(64)).Render(menuList.String())

	// Status legend.
	legendStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	installedDot := lipgloss.NewStyle().Foreground(ColorNeonBlue).Render("●")
	partialDot := lipgloss.NewStyle().Foreground(ColorYellow).Render("●")
	pendingDot := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("○")
	legend := legendStyle.Render(fmt.Sprintf("%s installed  %s partial  %s not installed",
		installedDot, partialDot, pendingDot))

	help := HelpStyle.Render("↑↓/jk navigate • enter select • esc back")

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		titleBox,
		subtitle,
		"",
		menuBox,
		"",
		legend,
		"",
		help,
	)

	return PlaceWithBackground(width, height, content)
}
