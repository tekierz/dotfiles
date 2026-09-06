package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// configFieldNav factors out the field-navigation logic shared by the
// field-based deep-dive config screens (Ghostty, Tmux, Zsh, Neovim, Git, Yazi,
// Fzf). These screens all:
//
//   - keep the focused field in a.configFieldIndex,
//   - move the cursor with up/down (j/k),
//   - adjust the focused field with left/right (h/l) and/or space,
//   - and return to the deep-dive menu on esc/enter (resetting the cursor).
//
// Each screen embeds configFieldNav and supplies:
//   - id: its Screen identifier (used by ID()),
//   - maxField: a function returning the highest valid field index, and
//   - adjust: a function applying a left/right/space change to the focused field.
//
// The concrete screen keeps a tiny Update that delegates here (handleMsg) and a
// screen-specific View. handleMsg returns only a tea.Cmd so the concrete screen
// can return itself as the ScreenHandler (embedding a base that returned itself
// would lose the concrete View method).
type configFieldNav struct {
	BaseScreen

	id       Screen
	maxField func(a *App) int
	// adjust applies a value change to the field at a.configFieldIndex.
	// key is the raw key string ("left"/"right"/"h"/"l"/" "). fwd is true for
	// right/l. Screens that only care about toggles can ignore fwd.
	adjust func(a *App, key string, fwd bool)
}

// ID returns the screen identifier.
func (s *configFieldNav) ID() Screen { return s.id }

// Init clamps configFieldIndex into [0, maxField] on every screen entry so
// that abnormal navigation paths (e.g. jumping directly to a config screen
// without going through back()) cannot leave a stale out-of-range index.
func (s *configFieldNav) Init() tea.Cmd {
	if a := s.App(); a != nil && s.maxField != nil {
		max := s.maxField(a)
		if a.configFieldIndex < 0 || a.configFieldIndex > max {
			a.configFieldIndex = 0
		}
	}
	return nil
}

// footer returns the pre-rendered help line for all field-nav config screens.
// Accurate to handleMsg: up/down move, left/right/space change, enter/esc back.
// Do NOT advertise "enter select" — enter calls back(), not adjust().
func (s *configFieldNav) footer() string {
	if a := s.App(); a != nil && a.configStandalone {
		return HelpStyle.Render("↑↓ navigate • ←→/space change • enter preview • esc/q cancel")
	}
	return HelpStyle.Render("↑↓ navigate • ←→/space change • enter/esc back")
}

// back resets the focused field. In the wizard it returns to the deep-dive
// menu; in standalone mode Enter freezes a reviewed config-save plan and opens
// its confirmation screen. Standalone Esc/q are handled before back and cancel
// without planning or mutation.
func (s *configFieldNav) back() (bool, tea.Cmd) {
	a := s.App()
	if a != nil {
		a.configFieldIndex = 0
		if a.configStandalone {
			return true, a.prepareStandaloneConfigSave()
		}
	}
	return true, NavigateTo(ScreenDeepDiveMenu)
}

// handleMsg processes a message using the shared field-navigation rules and
// reports whether it produced a transition command. The bool is true when a
// command (quit/back) should be returned to the manager; callers ignore it and
// always return the cmd alongside the concrete screen.
func (s *configFieldNav) handleMsg(msg tea.Msg) tea.Cmd {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c", "q":
			return tea.Quit
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < s.maxField(a) {
				a.configFieldIndex++
			}
		case "esc":
			if a.configStandalone {
				return tea.Quit
			}
			_, cmd := s.back()
			return cmd
		case "enter":
			_, cmd := s.back()
			return cmd
		default:
			if s.adjust != nil {
				fwd := key == "right" || key == "l"
				s.adjust(a, key, fwd)
			}
		}
	case tea.MouseMsg:
		return handleConfigFieldMouse(a, msg, s.maxField(a))
	}
	return nil
}

// fieldExtent records one rendered field's position in absolute screen
// coordinates: the field occupies rows [startY, startY+height).
type fieldExtent struct {
	index  int // logical field index (the value configFieldIndex takes)
	startY int // absolute screen Y of the field's first rendered row
	height int // number of rendered rows the field occupies
}

// fieldLayout is the geometry of a rendered field-config screen, resolved to
// absolute screen coordinates. The mouse handler maps a click against it. It is
// rebuilt on every View via fieldLayoutRecorder; an empty layout (no extents)
// means "geometry unknown", and the mouse handler falls back to ignoring clicks.
type fieldLayout struct {
	extents    []fieldExtent
	boxLeft    int // absolute X of the box's inner content left edge (inclusive)
	boxRight   int // absolute X of the box's inner content right edge (inclusive)
	hasXBounds bool
}

// fieldAt returns the logical field index whose vertical extent contains y, and
// true, or 0/false when no field covers that row.
func (fl fieldLayout) fieldAt(y int) (int, bool) {
	for _, e := range fl.extents {
		if y >= e.startY && y < e.startY+e.height {
			return e.index, true
		}
	}
	return 0, false
}

// fieldLayoutRecorder accumulates a field-config screen's content while tracking
// where each logical field begins, so the resulting fieldLayout maps clicks to
// fields geometry-correctly (handling variable-height fields, wrapped selectors,
// section headers, and hidden fields — a hidden field simply records no extent).
//
// Usage in a View:
//
//	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(55))
//	rec.field(0)                       // mark the start of field 0
//	rec.write(renderFieldLabel(...))   // write its content
//	rec.write(renderControl(...))
//	rec.write("\n\n")
//	rec.field(1)                       // mark the start of field 1
//	...
//	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
//	... compose title/box/help, then:
//	a.configFieldLayout = rec.finalize(width, height, title, box, help)
//
// Non-field content (section headers, blank lines) is written without a
// preceding field() call and is correctly excluded from every field's extent.
type fieldLayoutRecorder struct {
	boxWidth int
	content  strings.Builder
	// innerWidth is the wrap width lipgloss uses inside configBoxStyle, so start
	// offsets are measured against the same wrapping the box will apply.
	innerWidth int
	// marks pairs a logical field index with the rendered line offset (within the
	// box content) at which it begins.
	marks []struct {
		index   int
		lineOff int
	}
}

// configBoxHFrame is the horizontal frame (border + padding) configBoxStyle adds
// on each side: RoundedBorder() = 1 col + Padding(1, 2) = 2 cols.
//
// configBoxWrapInset is the horizontal padding only (no border). This is the
// inset that lipgloss uses when wrapping content: configBoxStyle.Width(w) wraps
// at w-2*padding = w-4, because the border is rendered OUTSIDE .Width() and
// does not narrow the content area. Use configBoxHFrame for X-bounds (the
// border occupies a screen column), but configBoxWrapInset for line-break
// counting so the recorder matches how the box actually wraps.
const (
	configBoxHFrame    = 3 // border(1) + horizontal padding(2), used for X-bounds
	configBoxVFrame    = 2 // border(1) + vertical padding(1) on each edge (top/bottom)
	configBoxWrapInset = 2 // horizontal padding only, used for wrap/line-break counting
)

// newFieldLayoutRecorder creates a recorder for a box of the given outer width.
func newFieldLayoutRecorder(boxWidth int) *fieldLayoutRecorder {
	// The border is outside .Width(), so lipgloss wraps content at
	// boxWidth - 2*padding (not boxWidth - 2*(border+padding)).
	inner := boxWidth - 2*configBoxWrapInset
	if inner < 1 {
		inner = 1
	}
	return &fieldLayoutRecorder{boxWidth: boxWidth, innerWidth: inner}
}

// field marks that the logical field with the given index begins at the current
// content position. Call it immediately before writing the field's content.
func (r *fieldLayoutRecorder) field(index int) {
	r.marks = append(r.marks, struct {
		index   int
		lineOff int
	}{index: index, lineOff: r.currentLineOffset()})
}

// write appends content (label/control/blank lines/section headers).
func (r *fieldLayoutRecorder) write(s string) { r.content.WriteString(s) }

// String returns the accumulated content for rendering inside configBoxStyle.
func (r *fieldLayoutRecorder) String() string { return r.content.String() }

// currentLineOffset returns the rendered line index (within the box content) at
// which the next written character lands, accounting for the box's wrapping.
// This equals the number of line breaks in the width-wrapped content so far —
// the same rule the box itself uses when it renders.
func (r *fieldLayoutRecorder) currentLineOffset() int {
	s := r.content.String()
	if s == "" {
		return 0
	}
	wrapped := lipgloss.NewStyle().Width(r.innerWidth).Render(s)
	return strings.Count(wrapped, "\n")
}

// finalize resolves the recorded per-field offsets to absolute screen
// coordinates using the composed layout (title, box, help) that the View passes
// to PlaceWithBackground. It measures the composed pieces so the anchor stays
// correct regardless of title/help height, and centers exactly as lipgloss.Place
// does. The last recorded field extends to the bottom of the box content.
func (r *fieldLayoutRecorder) finalize(width, height int, title, box, help string) fieldLayout {
	// Compose exactly as the View does: title, blank, box, blank, help.
	composedH := lipgloss.Height(title) + 1 + lipgloss.Height(box) + 1 + lipgloss.Height(help)
	composedW := lipgloss.Width(box)

	// lipgloss.Place centers content vertically/horizontally within width x
	// height by padding the top/left with floor((avail - size) / 2).
	topPad := (height - composedH) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (width - composedW) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	// Box content starts after the title block, the blank separator, then the
	// box's own top border + top padding.
	boxContentTop := topPad + lipgloss.Height(title) + 1 + configBoxVFrame
	boxContentRows := lipgloss.Height(box) - 2*configBoxVFrame
	boxContentBottom := boxContentTop + boxContentRows // exclusive

	// Box inner content horizontal span (absolute X, inclusive both ends).
	boxInnerLeft := leftPad + configBoxHFrame
	boxInnerRight := leftPad + composedW - 1 - configBoxHFrame

	fl := fieldLayout{
		boxLeft:    boxInnerLeft,
		boxRight:   boxInnerRight,
		hasXBounds: composedW > 0,
	}

	for i, mk := range r.marks {
		startY := boxContentTop + mk.lineOff
		// A field extends until the next field begins, or to the bottom of the
		// box content for the last field.
		var endY int
		if i+1 < len(r.marks) {
			endY = boxContentTop + r.marks[i+1].lineOff
		} else {
			endY = boxContentBottom
		}
		if endY <= startY {
			endY = startY + 1
		}
		fl.extents = append(fl.extents, fieldExtent{
			index:  mk.index,
			startY: startY,
			height: endY - startY,
		})
	}
	return fl
}

// finalizeComposed resolves the recorded per-field offsets to absolute screen
// coordinates for a screen whose box is embedded in a larger composed block
// (e.g. extra subtitle/legend rows above/below the box) that is centered with
// lipgloss.Place / PlaceWithBackground (Center, Center).
//
// composed is the full block passed to Place (measured for total height/width
// to compute the centering pads); box is the rendered configBoxStyle box;
// rowsAboveBox is the number of rendered rows within composed that precede the
// box. Unlike finalize (which assumes the fixed title/blank/box/blank/help
// layout and that the box spans the full composed width), this centers the box
// horizontally within composed so the X-bounds are correct when other rows are
// wider or narrower than the box.
func (r *fieldLayoutRecorder) finalizeComposed(width, height int, composed, box string, rowsAboveBox int) fieldLayout {
	composedH := lipgloss.Height(composed)
	composedW := lipgloss.Width(composed)
	boxW := lipgloss.Width(box)

	topPad := (height - composedH) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (width - composedW) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	// The box is centered horizontally within composed (JoinVertical(Center)).
	boxLeftInComposed := (composedW - boxW) / 2
	if boxLeftInComposed < 0 {
		boxLeftInComposed = 0
	}

	boxContentTop := topPad + rowsAboveBox + configBoxVFrame
	boxContentRows := lipgloss.Height(box) - 2*configBoxVFrame
	boxContentBottom := boxContentTop + boxContentRows // exclusive

	boxInnerLeft := leftPad + boxLeftInComposed + configBoxHFrame
	boxInnerRight := leftPad + boxLeftInComposed + boxW - 1 - configBoxHFrame

	fl := fieldLayout{
		boxLeft:    boxInnerLeft,
		boxRight:   boxInnerRight,
		hasXBounds: boxW > 0,
	}
	for i, mk := range r.marks {
		startY := boxContentTop + mk.lineOff
		var endY int
		if i+1 < len(r.marks) {
			endY = boxContentTop + r.marks[i+1].lineOff
		} else {
			endY = boxContentBottom
		}
		if endY <= startY {
			endY = startY + 1
		}
		fl.extents = append(fl.extents, fieldExtent{
			index:  mk.index,
			startY: startY,
			height: endY - startY,
		})
	}
	return fl
}

// handleConfigFieldMouse resolves a wheel/click for the field-based config
// screens. Wheel-scroll moves configFieldIndex by one and intentionally ignores
// geometry. A left click is mapped against the recorded per-field geometry
// (a.configFieldLayout, populated by the screen's most recent View) so it
// selects the field actually under the cursor; clicks outside the box's
// horizontal span or off every field select nothing.
func handleConfigFieldMouse(a *App, msg tea.MouseMsg, maxFields int) tea.Cmd {
	m := tea.MouseEvent(msg)

	if m.Button == tea.MouseButtonWheelUp {
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
		return nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		if a.configFieldIndex < maxFields {
			a.configFieldIndex++
		}
		return nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}

	fl := a.configFieldLayout
	// X-bounds: a click outside the centered box selects nothing.
	if fl.hasXBounds && (m.X < fl.boxLeft || m.X > fl.boxRight) {
		return nil
	}
	if idx, ok := fl.fieldAt(m.Y); ok {
		a.configFieldIndex = idx
	}
	return nil
}

// ContainerStyle frame sizes (RoundedBorder + Padding(1, 2)), used to resolve
// click geometry for screens wrapped in ContainerStyle and centered with
// lipgloss.Place / PlaceWithBackground.
const (
	containerHFrame = 3 // border(1) + horizontal padding(2)
	containerVFrame = 2 // border(1) + vertical padding(1)
)

// centeredContainerListLayout resolves per-row geometry for a one-row-per-item
// list rendered inside ContainerStyle and centered (lipgloss.Place Center,
// Center / PlaceWithBackground). container is the fully rendered ContainerStyle
// box (measured for exact height/width); rowsAboveList is the number of
// rendered rows inside the container that precede the first list row. width/
// height are the terminal dimensions the content is centered within.
func centeredContainerListLayout(width, height int, container string, count, rowsAboveList int) fieldLayout {
	containerW := lipgloss.Width(container)
	containerH := lipgloss.Height(container)

	topPad := (height - containerH) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (width - containerW) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	// First list row: top pad + container top frame + rows above the list.
	firstRowY := topPad + containerVFrame + rowsAboveList

	fl := fieldLayout{
		boxLeft:    leftPad + containerHFrame,
		boxRight:   leftPad + containerW - 1 - containerHFrame,
		hasXBounds: containerW > 0,
	}
	for i := 0; i < count; i++ {
		fl.extents = append(fl.extents, fieldExtent{index: i, startY: firstRowY + i, height: 1})
	}
	return fl
}

// configListNav factors out the list-selection logic shared by the checkbox-list
// deep-dive screens (macOS Apps, Helper Scripts/Utilities). These screens:
//
//   - keep the focused row in a dedicated App index field (e.g. macAppIndex),
//   - move with up/down (j/k),
//   - toggle the focused item with space (unless already installed), and
//   - return to the deep-dive menu on esc/enter (resetting the index).
//
// Each screen supplies its id, the item id list, accessors for its App index
// field, and a toggle func.
type configListNav struct {
	BaseScreen

	id      Screen
	itemIDs []string
	// index reads the current selection from App.
	index func(a *App) int
	// setIndex writes the current selection to App.
	setIndex func(a *App, v int)
	// toggle flips the enabled state for the given item id (guarded by install
	// state in the screen-supplied func).
	toggle func(a *App, id string)
}

// ID returns the screen identifier.
func (s *configListNav) ID() Screen { return s.id }

// Init triggers the async install-cache load on entry (idempotent). Every
// list-nav screen renders install-state color coding, so the base owns the
// trigger; a future embedder that doesn't need it can override with nil.
func (s *configListNav) Init() tea.Cmd {
	if a := s.App(); a != nil {
		return a.startInstallCacheLoad()
	}
	return nil
}

// installStatusLoadingView is the shared loading frame shown while the async
// install-status query is in flight (mirrors the manage screen's treatment).
func installStatusLoadingView(a *App, width, height int) string {
	spinner := AnimatedSpinnerDots(a.uiFrame)
	style := lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	return PlaceWithBackground(width, height,
		style.Render(fmt.Sprintf("%s Loading installation status...", spinner)))
}

// footer returns the pre-rendered help line for list-nav screens.
// Accurate to handleMsg: up/down move, space toggles, enter/esc back.
// Do NOT say "save & back" — persistence only occurs in standalone mode (C27).
func (s *configListNav) footer() string {
	return HelpStyle.Render("↑↓ navigate • space toggle • enter/esc back")
}

// footerInstalled returns the pre-rendered help line for list-nav screens that
// show install-state color coding (macapps, clitools, cliutilities, guiapps,
// utilities).
func (s *configListNav) footerInstalled() string {
	return HelpStyle.Render("↑↓ navigate • space toggle • enter/esc back • yellow = installed")
}

// back resets the selection index and returns to the deep-dive menu.
func (s *configListNav) back() tea.Cmd {
	if a := s.App(); a != nil {
		s.setIndex(a, 0)
	}
	return NavigateTo(ScreenDeepDiveMenu)
}

// handleMsg processes a message using the shared list-navigation rules,
// returning any transition command.
func (s *configListNav) handleMsg(msg tea.Msg) tea.Cmd {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return tea.Quit
		case "up", "k":
			if s.index(a) > 0 {
				s.setIndex(a, s.index(a)-1)
			}
		case "down", "j":
			if s.index(a) < len(s.itemIDs)-1 {
				s.setIndex(a, s.index(a)+1)
			}
		case " ":
			i := s.index(a)
			if i >= 0 && i < len(s.itemIDs) {
				s.toggle(a, s.itemIDs[i])
			}
		case "esc", "enter":
			return s.back()
		}
	case tea.MouseMsg:
		return s.handleMouse(msg)
	}
	return nil
}

// handleMouse mirrors handleConfigScreenMouse for list screens, moving the
// dedicated selection index instead of configFieldIndex.
func (s *configListNav) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	if m.Button == tea.MouseButtonWheelUp {
		if s.index(a) > 0 {
			s.setIndex(a, s.index(a)-1)
		}
		return nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		if s.index(a) < len(s.itemIDs)-1 {
			s.setIndex(a, s.index(a)+1)
		}
		return nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return nil
	}

	// Resolve the click against the per-item geometry recorded by the screen's
	// most recent View (a.configFieldLayout, shared with the field-config family
	// and rebuilt every View). The list screens record one extent per navigable
	// row, so this maps clicks geometry-correctly and is X-bounded to the box —
	// replacing the old hand-derived contentHeight+4 anchor that ignored the
	// box border/padding and X entirely.
	fl := a.configFieldLayout
	if fl.hasXBounds && (m.X < fl.boxLeft || m.X > fl.boxRight) {
		return nil
	}
	if idx, ok := fl.fieldAt(m.Y); ok {
		if idx >= 0 && idx < len(s.itemIDs) {
			s.setIndex(a, idx)
		}
	}
	return nil
}
