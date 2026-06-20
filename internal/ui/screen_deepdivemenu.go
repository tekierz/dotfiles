package ui

import (
	"fmt"

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
		// route through the ScreenManager. Explicitly reset themeStandalone so a
		// previous main-menu Theme visit can't leak into this wizard step (C8).
		a.themeStandalone = false
		a.themeReturn = ScreenDeepDiveMenu
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

	// Resolve the click against the per-item geometry recorded by the most recent
	// View (a.deepDiveMenuLayout). The recorder marks each item row (after any
	// category header) and the Continue row, so this maps clicks correctly across
	// category-header MarginTop blanks and the box border/padding the legacy
	// anchor ignored. The recorded indices run 0..len(items) (Continue = len).
	fl := a.deepDiveMenuLayout
	if fl.hasXBounds && (m.X < fl.boxLeft || m.X > fl.boxRight) {
		return nil
	}
	if idx, ok := fl.fieldAt(m.Y); ok {
		if idx >= 0 && idx <= len(items) {
			a.deepDiveMenuIndex = idx
			return s.selectItem(idx)
		}
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
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(64))

	// Category header style.
	categoryStyle := lipgloss.NewStyle().
		Foreground(ColorMagenta).
		Bold(true).
		MarginTop(1)

	for i, item := range items {
		// Render category header if this item starts a new category. These rows
		// are written without a field() mark so they are excluded from every
		// item's extent (the click handler must not select on a header row).
		if item.Category != "" {
			if i > 0 {
				rec.write("\n")
			}
			rec.write(categoryStyle.Render("  "+item.Category) + "\n")
		}

		// Mark the start of this item's own row (after any category header) so the
		// recorded offset points at the clickable row, not the header.
		rec.field(i)

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

		rec.write(fmt.Sprintf("%s%s %s %s  %s\n",
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
	// The blank separator before Continue is non-field content; mark the Continue
	// row itself so a click on it selects the continue index.
	rec.write("\n")
	rec.field(continueIdx)
	rec.write(fmt.Sprintf("%s%s\n", continueCursor, continueStyle.Render("▶ Continue to Installation")))

	// Wrap menu in a box.
	menuBox := configBoxStyle.Width(rec.boxWidth).Render(rec.String())

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

	// Record per-item geometry: the menuBox sits below titleBox + subtitle + the
	// blank separator. finalizeComposed measures the whole centered block.
	rowsAboveBox := lipgloss.Height(titleBox) + lipgloss.Height(subtitle) + 1
	a.deepDiveMenuLayout = rec.finalizeComposed(width, height, content, menuBox, rowsAboveBox)

	return PlaceWithBackground(width, height, content)
}
