package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/hotkeys"
)

const (
	hotkeysPaneCategories = 0
	hotkeysPaneItems      = 1
)

type hotkeysLayout struct {
	w int
	h int

	headerH int
	footerH int
	bodyY   int
	bodyH   int

	gap int

	leftX int
	leftW int

	rightX int
	rightW int

	// Panel internals (keep aligned with render styles).
	border int
	padX   int
	padY   int

	leftListY int
	leftListH int

	rightListY int
	rightListH int

	// Optional widget area (globe) under the items list.
	rightGlobeY int
	rightGlobeH int
}

func (l hotkeysLayout) maxCatScroll(n int) int  { return maxInt(0, n-l.leftListH) }
func (l hotkeysLayout) maxItemScroll(n int) int { return maxInt(0, n-l.rightListH) }

func (l hotkeysLayout) inLeftList(x, y int) bool {
	if x < l.leftX || x >= l.leftX+l.leftW {
		return false
	}
	return y >= l.leftListY && y < l.leftListY+l.leftListH
}

func (l hotkeysLayout) inRightList(x, y int) bool {
	if x < l.rightX || x >= l.rightX+l.rightW {
		return false
	}
	return y >= l.rightListY && y < l.rightListY+l.rightListH
}

func (a *App) hotkeysLayout() hotkeysLayout {
	const headerH = 3
	const footerH = 3 // Two lines for help text + one for status
	bodyY := headerH
	bodyH := a.height - headerH - footerH
	if bodyH < 5 {
		bodyH = 5
	}

	gap := 1
	leftW := clampInt(a.width/3, 26, 42)
	minRight := 38
	if a.width-leftW-gap < minRight {
		leftW = maxInt(22, a.width-minRight-gap)
	}
	rightW := maxInt(0, a.width-leftW-gap)

	border := 1
	padX := 1
	padY := 1

	leftHeaderLines := 3
	leftInnerY := bodyY + border + padY
	leftInnerH := bodyH - (border * 2) - (padY * 2)
	leftListY := leftInnerY + leftHeaderLines
	leftListH := maxInt(1, leftInnerH-leftHeaderLines)

	rightHeaderLines := 3
	rightInnerY := bodyY + border + padY
	rightInnerH := bodyH - (border * 2) - (padY * 2)
	rightListY := rightInnerY + rightHeaderLines
	rightBodyH := maxInt(1, rightInnerH-rightHeaderLines)

	rightListH := rightBodyH
	rightGlobeH := 0
	rightGlobeY := 0
	if a.animationsEnabled && rightW >= 56 && rightBodyH >= 16 {
		globeH := 8
		if rightBodyH >= 20 {
			globeH = 10
		}
		rightGlobeH = globeH
		rightListH = maxInt(1, rightBodyH-rightGlobeH-1) // 1 line gap above globe
		rightGlobeY = rightListY + rightListH + 1
	}

	return hotkeysLayout{
		w: a.width,
		h: a.height,

		headerH: headerH,
		footerH: footerH,
		bodyY:   bodyY,
		bodyH:   bodyH,

		gap: gap,

		leftX:  0,
		leftW:  leftW,
		rightX: leftW + gap,
		rightW: rightW,

		border: border,
		padX:   padX,
		padY:   padY,

		leftListY: leftListY,
		leftListH: leftListH,

		rightListY: rightListY,
		rightListH: rightListH,

		rightGlobeY: rightGlobeY,
		rightGlobeH: rightGlobeH,
	}
}

func (a *App) hotkeyCategories() []hotkeys.Category {
	cats := hotkeys.Categories(a.navStyle)
	if a.hotkeyFilter == "" {
		return cats
	}

	// Filter by tool/category id primarily; fall back to name match.
	var out []hotkeys.Category
	for _, c := range cats {
		if strings.EqualFold(c.ID, a.hotkeyFilter) || strings.EqualFold(c.Name, a.hotkeyFilter) {
			out = append(out, c)
		}
	}
	if len(out) > 0 {
		return out
	}
	return cats
}

// getCurrentUsername returns the active user name from global config, or "default" if none set.
func (a *App) getCurrentUsername() string {
	cfg, err := config.LoadGlobalConfig()
	if err != nil || cfg == nil || cfg.ActiveUser == "" {
		return "default"
	}
	return cfg.ActiveUser
}

// getCurrentUserHotkeys returns the hotkeys config for the current user.
func (a *App) getCurrentUserHotkeys() *config.UserHotkeys {
	if a.hotkeysFavorites == nil {
		a.hotkeysFavorites = &config.HotkeysConfig{Users: make(map[string]*config.UserHotkeys)}
	}
	username := a.getCurrentUsername()
	return a.hotkeysFavorites.GetUserHotkeys(username)
}

// isHotkeyFavorite checks if the given hotkey item is a favorite.
func (a *App) isHotkeyFavorite(categoryID, itemKey string) bool {
	userHotkeys := a.getCurrentUserHotkeys()
	return userHotkeys.IsFavorite(categoryID, itemKey)
}

// toggleHotkeyFavorite toggles the favorite status of the current hotkey item.
func (a *App) toggleHotkeyFavorite(categoryID, itemKey string) {
	username := a.getCurrentUsername()
	userHotkeys := a.getCurrentUserHotkeys()
	userHotkeys.ToggleFavorite(categoryID, itemKey)
	a.hotkeysFavorites.SetUserHotkeys(username, userHotkeys)
	// Save to disk
	_ = config.SaveHotkeysConfig(a.hotkeysFavorites)
}

func (a *App) handleHotkeysKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Handle alias input mode first (captures all keys)
	if a.hotkeysAddingAlias {
		return a.handleHotkeysAliasInput(msg)
	}

	cats := a.hotkeyCategories()
	if len(cats) == 0 {
		if key == "esc" {
			a.screen = a.hotkeysReturn
		}
		return a, nil
	}

	layout := a.hotkeysLayout()

	// Clamp indices.
	a.hotkeyCategory = clampInt(a.hotkeyCategory, 0, len(cats)-1)
	cat := cats[a.hotkeyCategory]
	allItems := cat.Items

	// Get display items (filtered if favorites-only mode)
	displayItems := allItems
	if a.hotkeysFavoritesOnly {
		var filtered []hotkeys.Item
		for _, it := range allItems {
			if a.isHotkeyFavorite(cat.ID, it.Keys) {
				filtered = append(filtered, it)
			}
		}
		displayItems = filtered
	}

	if len(displayItems) == 0 {
		a.hotkeyCursor = 0
	} else {
		a.hotkeyCursor = clampInt(a.hotkeyCursor, 0, len(displayItems)-1)
	}

	ensureCatVisible := func() {
		maxScroll := layout.maxCatScroll(len(cats))
		a.hotkeyCatScroll = clampInt(a.hotkeyCatScroll, 0, maxScroll)
		if a.hotkeyCategory < a.hotkeyCatScroll {
			a.hotkeyCatScroll = a.hotkeyCategory
		} else if a.hotkeyCategory >= a.hotkeyCatScroll+layout.leftListH {
			a.hotkeyCatScroll = a.hotkeyCategory - layout.leftListH + 1
		}
		a.hotkeyCatScroll = clampInt(a.hotkeyCatScroll, 0, maxScroll)
	}

	ensureItemVisible := func() {
		maxScroll := layout.maxItemScroll(len(displayItems))
		a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll, 0, maxScroll)
		if a.hotkeyCursor < a.hotkeyItemScroll {
			a.hotkeyItemScroll = a.hotkeyCursor
		} else if a.hotkeyCursor >= a.hotkeyItemScroll+layout.rightListH {
			a.hotkeyItemScroll = a.hotkeyCursor - layout.rightListH + 1
		}
		a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll, 0, maxScroll)
	}

	// Handle tab navigation first (1-4 keys)
	if handled, cmd := a.handleTabNavigationWithCmd(key); handled {
		return a, cmd
	}

	switch key {
	case "esc":
		a.hotkeyFilter = ""
		a.hotkeysFavoritesOnly = false // Reset favorites filter on exit
		a.screen = a.hotkeysReturn
		a.hotkeysReturn = ScreenMainMenu
		return a, nil

	case "tab":
		if a.hotkeysPane == hotkeysPaneCategories {
			a.hotkeysPane = hotkeysPaneItems
		} else {
			a.hotkeysPane = hotkeysPaneCategories
		}
		return a, nil
	}

	// Categories pane navigation.
	if a.hotkeysPane == hotkeysPaneCategories {
		switch key {
		case "up", "k":
			if a.hotkeyCategory > 0 {
				a.hotkeyCategory--
				a.hotkeyCursor = 0
				a.hotkeyItemScroll = 0
			}
			ensureCatVisible()
			return a, nil
		case "down", "j":
			if a.hotkeyCategory < len(cats)-1 {
				a.hotkeyCategory++
				a.hotkeyCursor = 0
				a.hotkeyItemScroll = 0
			}
			ensureCatVisible()
			return a, nil
		case "right", "l", "enter":
			a.hotkeysPane = hotkeysPaneItems
			return a, nil
		case "F":
			// Allow toggling favorites filter from categories pane too
			a.hotkeysFavoritesOnly = !a.hotkeysFavoritesOnly
			a.hotkeyCursor = 0
			a.hotkeyItemScroll = 0
			return a, nil
		}
		return a, nil
	}

	// Items pane navigation.
	switch key {
	case "up", "k":
		if a.hotkeyCursor > 0 {
			a.hotkeyCursor--
		}
		ensureItemVisible()
		return a, nil
	case "down", "j":
		if a.hotkeyCursor < len(displayItems)-1 {
			a.hotkeyCursor++
		}
		ensureItemVisible()
		return a, nil
	case "left", "h":
		a.hotkeysPane = hotkeysPaneCategories
		return a, nil
	case "f":
		// Toggle favorite for current item
		if len(displayItems) > 0 && a.hotkeyCursor >= 0 && a.hotkeyCursor < len(displayItems) {
			item := displayItems[a.hotkeyCursor]
			a.toggleHotkeyFavorite(cat.ID, item.Keys)
			// If in favorites-only mode and we just unfavorited, adjust cursor
			if a.hotkeysFavoritesOnly {
				// Recalculate filtered list
				var newFiltered []hotkeys.Item
				for _, it := range allItems {
					if a.isHotkeyFavorite(cat.ID, it.Keys) {
						newFiltered = append(newFiltered, it)
					}
				}
				if len(newFiltered) == 0 {
					a.hotkeyCursor = 0
				} else if a.hotkeyCursor >= len(newFiltered) {
					a.hotkeyCursor = len(newFiltered) - 1
				}
			}
		}
		return a, nil
	case "F":
		// Toggle favorites-only filter mode
		a.hotkeysFavoritesOnly = !a.hotkeysFavoritesOnly
		// Reset cursor when toggling filter
		a.hotkeyCursor = 0
		a.hotkeyItemScroll = 0
		return a, nil
	case "a":
		// Start adding alias - pre-fill command with current item if one is selected
		a.hotkeysAddingAlias = true
		a.hotkeysAliasField = 0 // Start with name field
		a.hotkeysAliasCursor = 0
		a.hotkeysAliasName = ""
		if len(displayItems) > 0 && a.hotkeyCursor >= 0 && a.hotkeyCursor < len(displayItems) {
			item := displayItems[a.hotkeyCursor]
			a.hotkeysAliasCommand = item.Keys // Pre-fill command from selected hotkey
		} else {
			a.hotkeysAliasCommand = ""
		}
		return a, nil
	}

	return a, nil
}

// handleHotkeysAliasInput handles key input when adding an alias
func (a *App) handleHotkeysAliasInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		a.hotkeysCancelAlias()
		return a, nil

	case "enter":
		if a.hotkeysAliasName != "" && a.hotkeysAliasCommand != "" {
			a.hotkeysSaveAlias()
		}
		a.hotkeysCancelAlias()
		return a, nil

	case "tab":
		// Switch between name and command fields
		a.hotkeysAliasField = (a.hotkeysAliasField + 1) % 2
		// Move cursor to end of new field
		if a.hotkeysAliasField == 0 {
			a.hotkeysAliasCursor = utf8.RuneCountInString(a.hotkeysAliasName)
		} else {
			a.hotkeysAliasCursor = utf8.RuneCountInString(a.hotkeysAliasCommand)
		}
		return a, nil

	case "left", "h":
		if a.hotkeysAliasCursor > 0 {
			a.hotkeysAliasCursor--
		}
		return a, nil

	case "right", "l":
		maxLen := a.hotkeysAliasCurrentFieldLen()
		if a.hotkeysAliasCursor < maxLen {
			a.hotkeysAliasCursor++
		}
		return a, nil

	case "home":
		a.hotkeysAliasCursor = 0
		return a, nil

	case "end":
		a.hotkeysAliasCursor = a.hotkeysAliasCurrentFieldLen()
		return a, nil

	case "backspace":
		a.hotkeysAliasBackspace()
		return a, nil

	case "delete":
		a.hotkeysAliasDelete()
		return a, nil

	default:
		// Insert typed runes (ignore non-rune keys and alt-modified keys)
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && !msg.Alt {
			a.hotkeysAliasInsertRunes(msg.Runes)
		}
		return a, nil
	}
}

// hotkeysAliasCurrentFieldLen returns the rune count of the current alias field
func (a *App) hotkeysAliasCurrentFieldLen() int {
	if a.hotkeysAliasField == 0 {
		return utf8.RuneCountInString(a.hotkeysAliasName)
	}
	return utf8.RuneCountInString(a.hotkeysAliasCommand)
}

// hotkeysAliasBackspace deletes the character before the cursor
func (a *App) hotkeysAliasBackspace() {
	if a.hotkeysAliasField == 0 {
		r := []rune(a.hotkeysAliasName)
		cur := clampInt(a.hotkeysAliasCursor, 0, len(r))
		if cur > 0 {
			r = append(r[:cur-1], r[cur:]...)
			a.hotkeysAliasCursor = cur - 1
			a.hotkeysAliasName = string(r)
		}
	} else {
		r := []rune(a.hotkeysAliasCommand)
		cur := clampInt(a.hotkeysAliasCursor, 0, len(r))
		if cur > 0 {
			r = append(r[:cur-1], r[cur:]...)
			a.hotkeysAliasCursor = cur - 1
			a.hotkeysAliasCommand = string(r)
		}
	}
}

// hotkeysAliasDelete deletes the character at the cursor
func (a *App) hotkeysAliasDelete() {
	if a.hotkeysAliasField == 0 {
		r := []rune(a.hotkeysAliasName)
		cur := clampInt(a.hotkeysAliasCursor, 0, len(r))
		if cur < len(r) {
			r = append(r[:cur], r[cur+1:]...)
			a.hotkeysAliasName = string(r)
		}
	} else {
		r := []rune(a.hotkeysAliasCommand)
		cur := clampInt(a.hotkeysAliasCursor, 0, len(r))
		if cur < len(r) {
			r = append(r[:cur], r[cur+1:]...)
			a.hotkeysAliasCommand = string(r)
		}
	}
}

// hotkeysAliasInsertRunes inserts runes at the cursor position
func (a *App) hotkeysAliasInsertRunes(runes []rune) {
	if a.hotkeysAliasField == 0 {
		r := []rune(a.hotkeysAliasName)
		cur := clampInt(a.hotkeysAliasCursor, 0, len(r))
		out := make([]rune, 0, len(r)+len(runes))
		out = append(out, r[:cur]...)
		out = append(out, runes...)
		out = append(out, r[cur:]...)
		a.hotkeysAliasName = string(out)
		a.hotkeysAliasCursor = cur + len(runes)
	} else {
		r := []rune(a.hotkeysAliasCommand)
		cur := clampInt(a.hotkeysAliasCursor, 0, len(r))
		out := make([]rune, 0, len(r)+len(runes))
		out = append(out, r[:cur]...)
		out = append(out, runes...)
		out = append(out, r[cur:]...)
		a.hotkeysAliasCommand = string(out)
		a.hotkeysAliasCursor = cur + len(runes)
	}
}

// hotkeysSaveAlias saves the alias to the user's config
func (a *App) hotkeysSaveAlias() {
	userHotkeys := a.getCurrentUserHotkeys()
	if userHotkeys.Aliases == nil {
		userHotkeys.Aliases = make(map[string]string)
	}
	userHotkeys.Aliases[a.hotkeysAliasName] = a.hotkeysAliasCommand
	username := a.getCurrentUsername()
	a.hotkeysFavorites.SetUserHotkeys(username, userHotkeys)
	_ = config.SaveHotkeysConfig(a.hotkeysFavorites)
}

// hotkeysCancelAlias cancels alias editing and resets state
func (a *App) hotkeysCancelAlias() {
	a.hotkeysAddingAlias = false
	a.hotkeysAliasName = ""
	a.hotkeysAliasCommand = ""
	a.hotkeysAliasField = 0
	a.hotkeysAliasCursor = 0
}

func (a *App) handleHotkeysMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)
	if a.width <= 0 || a.height <= 0 {
		return a, nil
	}

	// Handle tab bar clicks (Y=0 is the tab bar line)
	if m.Y == 0 && m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		if screen, cmd := a.detectTabClick(m.X); screen != 0 {
			a.screen = screen
			return a, cmd
		}
	}

	layout := a.hotkeysLayout()
	cats := a.hotkeyCategories()
	if len(cats) == 0 {
		return a, nil
	}

	// Wheel scroll.
	if m.IsWheel() {
		delta := 0
		switch m.Button {
		case tea.MouseButtonWheelUp:
			delta = -1
		case tea.MouseButtonWheelDown:
			delta = 1
		default:
			return a, nil
		}

		if m.X < layout.rightX {
			a.hotkeyCatScroll = clampInt(a.hotkeyCatScroll+delta, 0, layout.maxCatScroll(len(cats)))
		} else {
			cat := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)]
			allItems := cat.Items

			// Get display items (filtered if favorites-only mode)
			displayItems := allItems
			if a.hotkeysFavoritesOnly {
				var filtered []hotkeys.Item
				for _, it := range allItems {
					if a.isHotkeyFavorite(cat.ID, it.Keys) {
						filtered = append(filtered, it)
					}
				}
				displayItems = filtered
			}
			a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll+delta, 0, layout.maxItemScroll(len(displayItems)))
		}
		return a, nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Click categories.
	if layout.inLeftList(m.X, m.Y) {
		rel := m.Y - layout.leftListY
		idx := a.hotkeyCatScroll + rel
		if idx >= 0 && idx < len(cats) {
			a.hotkeysPane = hotkeysPaneCategories
			a.hotkeyCategory = idx
			a.hotkeyCursor = 0
			a.hotkeyItemScroll = 0
		}
		return a, nil
	}

	// Click items.
	if layout.inRightList(m.X, m.Y) {
		cat := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)]
		allItems := cat.Items

		// Get display items (filtered if favorites-only mode)
		displayItems := allItems
		if a.hotkeysFavoritesOnly {
			var filtered []hotkeys.Item
			for _, it := range allItems {
				if a.isHotkeyFavorite(cat.ID, it.Keys) {
					filtered = append(filtered, it)
				}
			}
			displayItems = filtered
		}

		rel := m.Y - layout.rightListY
		idx := a.hotkeyItemScroll + rel
		if idx >= 0 && idx < len(displayItems) {
			a.hotkeysPane = hotkeysPaneItems
			a.hotkeyCursor = idx
		}
		return a, nil
	}

	return a, nil
}

func (a *App) renderHotkeysDualPane() string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}

	layout := a.hotkeysLayout()
	cats := a.hotkeyCategories()

	// Keep indices/scrolls valid and keep the selection visible even after a
	// terminal resize.
	if len(cats) > 0 {
		a.hotkeyCategory = clampInt(a.hotkeyCategory, 0, len(cats)-1)
		maxCatScroll := layout.maxCatScroll(len(cats))
		a.hotkeyCatScroll = clampInt(a.hotkeyCatScroll, 0, maxCatScroll)
		if a.hotkeyCategory < a.hotkeyCatScroll {
			a.hotkeyCatScroll = a.hotkeyCategory
		} else if a.hotkeyCategory >= a.hotkeyCatScroll+layout.leftListH {
			a.hotkeyCatScroll = a.hotkeyCategory - layout.leftListH + 1
		}
		a.hotkeyCatScroll = clampInt(a.hotkeyCatScroll, 0, maxCatScroll)

		cat := cats[a.hotkeyCategory]
		allItems := cat.Items

		// Get display items (filtered if favorites-only mode)
		displayItems := allItems
		if a.hotkeysFavoritesOnly {
			var filtered []hotkeys.Item
			for _, it := range allItems {
				if a.isHotkeyFavorite(cat.ID, it.Keys) {
					filtered = append(filtered, it)
				}
			}
			displayItems = filtered
		}

		if len(displayItems) == 0 {
			a.hotkeyCursor = 0
			a.hotkeyItemScroll = 0
			// Don't force back to categories pane if filtering - user might want to toggle filter
		} else {
			a.hotkeyCursor = clampInt(a.hotkeyCursor, 0, len(displayItems)-1)
			maxItemScroll := layout.maxItemScroll(len(displayItems))
			a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll, 0, maxItemScroll)
			if a.hotkeyCursor < a.hotkeyItemScroll {
				a.hotkeyItemScroll = a.hotkeyCursor
			} else if a.hotkeyCursor >= a.hotkeyItemScroll+layout.rightListH {
				a.hotkeyItemScroll = a.hotkeyCursor - layout.rightListH + 1
			}
			a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll, 0, maxItemScroll)
		}
	} else {
		a.hotkeyCategory = 0
		a.hotkeyCursor = 0
		a.hotkeyCatScroll = 0
		a.hotkeyItemScroll = 0
		a.hotkeysPane = hotkeysPaneCategories
	}

	header := a.renderHotkeysHeader(layout.w)
	footer := a.renderHotkeysFooter(layout.w, cats)

	left := a.renderHotkeysCategoriesPanel(layout, cats)
	right := a.renderHotkeysItemsPanel(layout, cats)

	// Style the gap between panels (no explicit background to respect terminal transparency)
	gapStyle := lipgloss.NewStyle().
		Height(layout.bodyH)
	gap := gapStyle.Render(strings.Repeat(" ", layout.gap))

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	// No explicit background to respect terminal transparency
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Top, view)
}

func (a *App) renderHotkeysHeader(width int) string {
	tabs := RenderTabBar(ScreenHotkeys, width)

	subText := "Cheatsheets + keybindings (matches manager)"
	if a.animationsEnabled {
		subText = AnimatedSpinnerDots(a.uiFrame/2) + " " + subText
	}
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(subText, width))

	divider := ShimmerDivider(maxInt(0, width), a.uiFrame, a.animationsEnabled)
	return lipgloss.JoinVertical(lipgloss.Left, tabs, sub, divider)
}

func (a *App) renderHotkeysFooter(width int, cats []hotkeys.Category) string {
	// Split help text into two lines for better readability
	helpLine1 := "Tab pane  ↑↓ move  ←→ switch  f favorite  F filter  a add alias"
	helpLine2 := "Click select  Scroll  Esc back  q quit"
	hints := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		helpLine1 + "\n" + helpLine2,
	)

	statusText := ""
	if len(cats) > 0 {
		cat := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)]
		statusText = fmt.Sprintf("%s %s — %d items", cat.Icon, cat.Name, len(cat.Items))
	}
	if a.hotkeyFilter != "" {
		statusText = statusText + lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  (filtered)")
	}
	if a.hotkeysFavoritesOnly {
		statusText = statusText + lipgloss.NewStyle().Foreground(ColorYellow).Render("  [favorites only]")
	}
	if statusText == "" {
		statusText = " "
	}
	status := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(statusText, width))
	return lipgloss.JoinVertical(lipgloss.Left, hints, status)
}

func (a *App) renderHotkeysCategoriesPanel(layout hotkeysLayout, cats []hotkeys.Category) string {
	borderColor := ColorBorder
	if a.hotkeysPane == hotkeysPaneCategories {
		borderColor = ColorCyan
	}

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		Width(maxInt(1, layout.leftW-2)).
		Height(maxInt(1, layout.bodyH-2))

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("CATEGORIES")
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(fmt.Sprintf("%d sections", len(cats)))

	innerW := maxInt(0, layout.leftW-(layout.border*2)-(layout.padX*2))
	countStyle := lipgloss.NewStyle().Foreground(ColorText).Background(ColorOverlay).Padding(0, 1)
	const minBadgeSpacing = 2 // Ensure minimum spacing between name and badge

	lines := make([]string, 0, layout.leftListH)
	for i := a.hotkeyCatScroll; i < len(cats) && len(lines) < layout.leftListH; i++ {
		c := cats[i]
		focused := i == a.hotkeyCategory

		cursor := "  "
		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		lineStyle := lipgloss.NewStyle()
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			nameStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
			// Add background highlight for focused item
			lineStyle = lipgloss.NewStyle().Background(ColorOverlay)
		}

		left := fmt.Sprintf("%s%s %s", cursor, c.Icon, nameStyle.Render(c.Name))
		badge := countStyle.Render(fmt.Sprintf("%d", len(c.Items)))

		leftW := ansi.StringWidth(left)
		badgeW := ansi.StringWidth(badge)
		availLeft := innerW - badgeW - minBadgeSpacing
		if availLeft < 0 {
			availLeft = 0
		}
		if leftW > availLeft {
			left = truncateVisible(left, availLeft)
			leftW = ansi.StringWidth(left)
		}
		spaces := innerW - leftW - badgeW
		if spaces < minBadgeSpacing {
			spaces = minBadgeSpacing
		}

		line := left + strings.Repeat(" ", spaces) + badge
		// Apply background highlight for focused line
		if focused {
			line = lineStyle.Width(innerW).Render(truncateVisible(line, innerW))
		} else {
			line = truncateVisible(line, innerW)
		}
		lines = append(lines, line)
	}
	for len(lines) < layout.leftListH {
		lines = append(lines, "")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, sub, "", strings.Join(lines, "\n"))
	return panel.Render(content)
}

func (a *App) renderHotkeysItemsPanel(layout hotkeysLayout, cats []hotkeys.Category) string {
	borderColor := ColorBorder
	if a.hotkeysPane == hotkeysPaneItems {
		borderColor = ColorCyan
	}

	// Cap right pane width for better readability
	maxRightW := 100
	rightW := layout.rightW
	if rightW > maxRightW {
		rightW = maxRightW
	}

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		Width(maxInt(1, rightW-2)).
		Height(maxInt(1, layout.bodyH-2))

	if len(cats) == 0 {
		msg := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No hotkeys found.")
		return panel.Render(msg)
	}

	cat := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)]
	items := cat.Items

	// Filter to favorites only if mode is enabled
	displayItems := items
	itemIndices := make([]int, len(items)) // Map display index to original index
	for i := range items {
		itemIndices[i] = i
	}
	if a.hotkeysFavoritesOnly {
		var filteredItems []hotkeys.Item
		var filteredIndices []int
		for i, it := range items {
			if a.isHotkeyFavorite(cat.ID, it.Keys) {
				filteredItems = append(filteredItems, it)
				filteredIndices = append(filteredIndices, i)
			}
		}
		displayItems = filteredItems
		itemIndices = filteredIndices
	}

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("ITEMS")
	subText := fmt.Sprintf("%s %s", cat.Icon, cat.Name)
	if a.hotkeysFavoritesOnly {
		subText += fmt.Sprintf(" (%d favorites)", len(displayItems))
	}
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(subText)

	// Use capped rightW for inner width calculation
	innerW := maxInt(0, rightW-(layout.border*2)-(layout.padX*2))
	keyW := min(22, maxInt(12, innerW/3))

	// Show alias input dialog if adding
	if a.hotkeysAddingAlias {
		aliasContent := a.renderHotkeysAliasDialog(innerW)
		content := lipgloss.JoinVertical(lipgloss.Left, title, sub, "", aliasContent)
		return panel.Render(content)
	}

	// Show message if no favorites in filter mode
	if a.hotkeysFavoritesOnly && len(displayItems) == 0 {
		msg := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No favorites in this category.\nPress 'F' to show all items.")
		content := lipgloss.JoinVertical(lipgloss.Left, title, sub, "", msg)
		return panel.Render(content)
	}

	lines := make([]string, 0, layout.rightListH)
	for i := a.hotkeyItemScroll; i < len(displayItems) && len(lines) < layout.rightListH; i++ {
		it := displayItems[i]
		focused := i == a.hotkeyCursor

		// Check if this item is a favorite
		isFavorite := a.isHotkeyFavorite(cat.ID, it.Keys)
		starIndicator := "  "
		if isFavorite {
			starIndicator = lipgloss.NewStyle().Foreground(ColorYellow).Render("* ")
		}

		cursor := "  "
		keyStyle := lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).Width(keyW)
		descStyle := lipgloss.NewStyle().Foreground(ColorText)
		lineStyle := lipgloss.NewStyle()
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("> ")
			descStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
			// Add background highlight for focused item
			lineStyle = lipgloss.NewStyle().Background(ColorOverlay)
		}

		line := fmt.Sprintf("%s%s%s %s", starIndicator, cursor, keyStyle.Render(it.Keys), descStyle.Render(it.Description))
		// Apply background highlight for focused line
		if focused {
			line = lineStyle.Width(innerW).Render(truncateVisible(line, innerW))
		} else {
			line = truncateVisible(line, innerW)
		}
		lines = append(lines, line)
	}
	for len(lines) < layout.rightListH {
		lines = append(lines, "")
	}

	contentLines := []string{title, sub, "", strings.Join(lines, "\n")}

	// Optional widget area (globe) in the bottom-right.
	if a.animationsEnabled && layout.rightGlobeH > 0 {
		globeW := min(24, maxInt(18, innerW/3))
		globe := RenderMiniGlobe(globeW, layout.rightGlobeH, a.uiFrame)
		globePlaced := lipgloss.Place(innerW, layout.rightGlobeH, lipgloss.Right, lipgloss.Center, globe)
		contentLines = append(contentLines, "", globePlaced)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, contentLines...)
	return panel.Render(content)
}

// renderHotkeysAliasDialog renders the alias input dialog
func (a *App) renderHotkeysAliasDialog(width int) string {
	if width <= 0 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	fieldStyle := lipgloss.NewStyle().Foreground(ColorText)
	focusedFieldStyle := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

	// Build cursor-aware field display
	renderField := func(value string, isFocused bool, cursorPos int) string {
		if !isFocused {
			display := value
			if display == "" {
				display = "(empty)"
			}
			return fieldStyle.Render(truncateVisible(display, width-10))
		}

		// Show cursor for focused field
		runes := []rune(value)
		cur := clampInt(cursorPos, 0, len(runes))
		left := string(runes[:cur])
		right := string(runes[cur:])

		cursorChar := lipgloss.NewStyle().Background(ColorCyan).Foreground(ColorBg).Render(" ")
		if cur < len(runes) {
			cursorChar = lipgloss.NewStyle().Background(ColorCyan).Foreground(ColorBg).Render(string(runes[cur]))
			right = string(runes[cur+1:])
		}

		display := focusedFieldStyle.Render(left) + cursorChar + focusedFieldStyle.Render(right)
		return truncateVisible(display, width-10)
	}

	nameLabel := "Name:    "
	cmdLabel := "Command: "

	if a.hotkeysAliasField == 0 {
		nameLabel = labelStyle.Bold(true).Foreground(ColorCyan).Render("Name:    ")
	} else {
		nameLabel = labelStyle.Render("Name:    ")
	}

	if a.hotkeysAliasField == 1 {
		cmdLabel = labelStyle.Bold(true).Foreground(ColorCyan).Render("Command: ")
	} else {
		cmdLabel = labelStyle.Render("Command: ")
	}

	nameLine := nameLabel + renderField(a.hotkeysAliasName, a.hotkeysAliasField == 0, a.hotkeysAliasCursor)
	cmdLine := cmdLabel + renderField(a.hotkeysAliasCommand, a.hotkeysAliasField == 1, a.hotkeysAliasCursor)

	title := titleStyle.Render("ADD ALIAS")
	hint := hintStyle.Render("Tab switch field  Enter save  Esc cancel")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		nameLine,
		cmdLine,
		"",
		hint,
	)
}
