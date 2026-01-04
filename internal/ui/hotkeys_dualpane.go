package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	const footerH = 2
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

func (a *App) handleHotkeysKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

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
	items := cats[a.hotkeyCategory].Items
	if len(items) == 0 {
		a.hotkeyCursor = 0
	} else {
		a.hotkeyCursor = clampInt(a.hotkeyCursor, 0, len(items)-1)
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
		maxScroll := layout.maxItemScroll(len(items))
		a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll, 0, maxScroll)
		if a.hotkeyCursor < a.hotkeyItemScroll {
			a.hotkeyItemScroll = a.hotkeyCursor
		} else if a.hotkeyCursor >= a.hotkeyItemScroll+layout.rightListH {
			a.hotkeyItemScroll = a.hotkeyCursor - layout.rightListH + 1
		}
		a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll, 0, maxScroll)
	}

	switch key {
	case "esc":
		a.hotkeyFilter = ""
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
		if a.hotkeyCursor < len(items)-1 {
			a.hotkeyCursor++
		}
		ensureItemVisible()
		return a, nil
	case "left", "h":
		a.hotkeysPane = hotkeysPaneCategories
		return a, nil
	}

	return a, nil
}

func (a *App) handleHotkeysMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)
	if a.width <= 0 || a.height <= 0 {
		return a, nil
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
			items := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)].Items
			a.hotkeyItemScroll = clampInt(a.hotkeyItemScroll+delta, 0, layout.maxItemScroll(len(items)))
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
		items := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)].Items
		rel := m.Y - layout.rightListY
		idx := a.hotkeyItemScroll + rel
		if idx >= 0 && idx < len(items) {
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

		items := cats[a.hotkeyCategory].Items
		if len(items) == 0 {
			a.hotkeyCursor = 0
			a.hotkeyItemScroll = 0
			if a.hotkeysPane == hotkeysPaneItems {
				a.hotkeysPane = hotkeysPaneCategories
			}
		} else {
			a.hotkeyCursor = clampInt(a.hotkeyCursor, 0, len(items)-1)
			maxItemScroll := layout.maxItemScroll(len(items))
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
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", layout.gap), right)

	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	if lipgloss.Height(view) < a.height {
		view += strings.Repeat("\n", a.height-lipgloss.Height(view))
	}
	return view
}

func (a *App) renderHotkeysHeader(width int) string {
	tabs := RenderTabsBar(width, []TabSpec{
		{Label: "Manage", Active: false},
		{Label: "Hotkeys", Active: true},
		{Label: "Update", Active: false},
		{Label: "Backups", Active: false},
	})

	subText := "Cheatsheets + keybindings (matches manager)"
	if a.animationsEnabled {
		subText = AnimatedSpinnerDots(a.uiFrame/2) + " " + subText
	}
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(subText, width))

	divider := ShimmerDivider(maxInt(0, width), a.uiFrame, a.animationsEnabled)
	return lipgloss.JoinVertical(lipgloss.Left, tabs, sub, divider)
}

func (a *App) renderHotkeysFooter(width int, cats []hotkeys.Category) string {
	hints := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"Tab switch pane • ↑↓ move • Click select • Esc back • q quit",
	)

	statusText := ""
	if len(cats) > 0 {
		cat := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)]
		statusText = fmt.Sprintf("%s %s — %d items", cat.Icon, cat.Name, len(cat.Items))
	}
	if a.hotkeyFilter != "" {
		statusText = statusText + lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  (filtered)")
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
		Background(ColorSurface).
		Padding(1, 1).
		Width(maxInt(1, layout.leftW-2)).
		Height(maxInt(1, layout.bodyH-2))

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("CATEGORIES")
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(fmt.Sprintf("%d sections", len(cats)))

	innerW := maxInt(0, layout.leftW-(layout.border*2)-(layout.padX*2))
	countStyle := lipgloss.NewStyle().Foreground(ColorTextMuted).Background(ColorOverlay).Padding(0, 1)

	lines := make([]string, 0, layout.leftListH)
	for i := a.hotkeyCatScroll; i < len(cats) && len(lines) < layout.leftListH; i++ {
		c := cats[i]
		focused := i == a.hotkeyCategory

		cursor := "  "
		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			nameStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		}

		left := fmt.Sprintf("%s%s %s", cursor, c.Icon, nameStyle.Render(c.Name))
		badge := countStyle.Render(fmt.Sprintf("%d", len(c.Items)))

		leftW := ansi.StringWidth(left)
		badgeW := ansi.StringWidth(badge)
		availLeft := innerW - badgeW - 1
		if availLeft < 0 {
			availLeft = 0
		}
		if leftW > availLeft {
			left = truncateVisible(left, availLeft)
			leftW = ansi.StringWidth(left)
		}
		spaces := innerW - leftW - badgeW
		if spaces < 1 {
			spaces = 1
		}

		line := left + strings.Repeat(" ", spaces) + badge
		lines = append(lines, truncateVisible(line, innerW))
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

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(ColorSurface).
		Padding(1, 1).
		Width(maxInt(1, layout.rightW-2)).
		Height(maxInt(1, layout.bodyH-2))

	if len(cats) == 0 {
		msg := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No hotkeys found.")
		return panel.Render(msg)
	}

	cat := cats[clampInt(a.hotkeyCategory, 0, len(cats)-1)]
	items := cat.Items

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("ITEMS")
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(fmt.Sprintf("%s %s", cat.Icon, cat.Name))

	innerW := maxInt(0, layout.rightW-(layout.border*2)-(layout.padX*2))
	keyW := min(22, maxInt(12, innerW/3))

	lines := make([]string, 0, layout.rightListH)
	for i := a.hotkeyItemScroll; i < len(items) && len(lines) < layout.rightListH; i++ {
		it := items[i]
		focused := i == a.hotkeyCursor

		cursor := "  "
		keyStyle := lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).Width(keyW)
		descStyle := lipgloss.NewStyle().Foreground(ColorText)
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			descStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		}

		line := fmt.Sprintf("%s%s %s", cursor, keyStyle.Render(it.Keys), descStyle.Render(it.Description))
		lines = append(lines, truncateVisible(line, innerW))
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
