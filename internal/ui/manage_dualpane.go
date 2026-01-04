package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/tools"
)

// ==========================
// Manage Screen (Dual Pane)
// ==========================
//
// Design goals:
// - Always render in a predictable full-screen layout (no centering) so mouse hit-testing is simple.
// - Dual-pane by default: left = tools, right = settings for selected tool.
// - Keyboard first, but mouse friendly (click to select, wheel to scroll, click to toggle/adjust).
//
// Notes:
// - This UI edits a persistent ManageConfig stored at:
//   ~/.config/dotfiles/tools/manage.json
//   via internal/config's generic JSON helpers.
//
// - The underlying "apply config to actual tool config files" is a separate concern; here we focus
//   on the management experience + storing preferences.

const (
	managePaneTools    = 0
	managePaneSettings = 1
)

// manageFieldKind describes how a setting should be rendered and edited.
type manageFieldKind int

const (
	manageFieldText manageFieldKind = iota
	manageFieldToggle
	manageFieldNumber
	manageFieldOption
)

// manageField is a single editable field in the right pane.
//
// This is intentionally "small" and pointer-based so we can edit values without lots of boilerplate.
// For more complex types (slices, maps), add explicit handlers later.
type manageField struct {
	key         string
	label       string
	description string
	kind        manageFieldKind

	// One of these will be set depending on kind.
	str  *string
	b    *bool
	n    *int
	unit string

	// For option fields (kind == manageFieldOption).
	options []string

	// For numeric fields (kind == manageFieldNumber).
	min  int
	max  int
	step int
}

// manageItem is a tool entry in the left pane.
type manageItem struct {
	id          string
	name        string
	icon        string
	description string
	installed   bool
}

// manageSavedMsg is emitted after a save attempt.
type manageSavedMsg struct{ err error }

func (a *App) saveManageConfigCmd() tea.Cmd {
	// Capture by value (pointer is stable) and run file I/O in a command.
	cfg := a.manageConfig
	theme := a.theme
	nav := a.navStyle
	return func() tea.Msg {
		if err := config.SaveToolConfig("manage", cfg); err != nil {
			return manageSavedMsg{err: err}
		}

		// Also persist global theme/nav so installer + CLI stay in sync.
		g, err := config.LoadGlobalConfig()
		if err != nil {
			g = config.DefaultGlobalConfig()
		}
		g.Theme = theme
		g.NavStyle = nav

		if err := config.SaveGlobalConfig(g); err != nil {
			return manageSavedMsg{err: err}
		}

		return manageSavedMsg{err: nil}
	}
}

func (a *App) handleManageKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Inline string editor captures keys first so typing doesn't trigger global bindings.
	if a.manageEditing {
		switch key {
		case "esc":
			a.manageCancelEditing()
			return a, nil

		case "enter":
			a.manageCommitEditing()
			a.manageStatus = "Updated ✓"
			return a, nil

		case "left", "h":
			if a.manageEditCursor > 0 {
				a.manageEditCursor--
			}
			return a, nil

		case "right", "l":
			if a.manageEditCursor < utf8.RuneCountInString(a.manageEditValue) {
				a.manageEditCursor++
			}
			return a, nil

		case "home":
			a.manageEditCursor = 0
			return a, nil

		case "end":
			a.manageEditCursor = utf8.RuneCountInString(a.manageEditValue)
			return a, nil

		case "backspace":
			r := []rune(a.manageEditValue)
			cur := clampInt(a.manageEditCursor, 0, len(r))
			if cur > 0 {
				r = append(r[:cur-1], r[cur:]...)
				a.manageEditCursor = cur - 1
				a.manageEditValue = string(r)
			}
			return a, nil

		case "delete":
			r := []rune(a.manageEditValue)
			cur := clampInt(a.manageEditCursor, 0, len(r))
			if cur < len(r) {
				r = append(r[:cur], r[cur+1:]...)
				a.manageEditValue = string(r)
			}
			return a, nil

		default:
			// Insert typed runes (ignore non-rune keys and alt-modified keys).
			// Note: Bubble Tea represents Ctrl combinations as KeyType values (not KeyRunes).
			if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && !msg.Alt {
				r := []rune(a.manageEditValue)
				cur := clampInt(a.manageEditCursor, 0, len(r))
				insert := msg.Runes

				out := make([]rune, 0, len(r)+len(insert))
				out = append(out, r[:cur]...)
				out = append(out, insert...)
				out = append(out, r[cur:]...)

				a.manageEditValue = string(out)
				a.manageEditCursor = cur + len(insert)
			}
			return a, nil
		}
	}

	// Non-editing manage UI.
	items := a.manageItems()
	if len(items) == 0 {
		if key == "esc" {
			a.screen = ScreenMainMenu
		}
		return a, nil
	}

	layout := a.manageLayout()
	a.manageEnsureToolsVisible(layout, len(items))
	fields := a.manageFieldsFor(items[a.manageIndex].id)
	a.manageEnsureFieldsVisible(layout, len(fields))

	// Helpers.
	currentField := func() (manageField, bool) {
		if len(fields) == 0 {
			return manageField{}, false
		}
		idx := clampInt(a.configFieldIndex, 0, len(fields)-1)
		return fields[idx], true
	}

	adjustField := func(dir int) {
		f, ok := currentField()
		if !ok {
			return
		}
		switch f.kind {
		case manageFieldOption:
			if f.str != nil && len(f.options) > 0 {
				*f.str = cycleStringOption(f.options, *f.str, dir > 0)
			}
		case manageFieldNumber:
			if f.n != nil {
				step := f.step
				if step == 0 {
					step = 1
				}
				*f.n = clampInt(*f.n+(dir*step), f.min, f.max)
			}
		}
	}

	toggleField := func() {
		f, ok := currentField()
		if !ok {
			return
		}
		if f.kind == manageFieldToggle && f.b != nil {
			*f.b = !*f.b
		}
	}

	startEditingField := func() {
		f, ok := currentField()
		if !ok {
			return
		}
		a.manageStartEditing(f)
	}

	switch key {
	// Global navigation.
	case "esc":
		a.manageStatus = ""
		a.manageCancelEditing()
		a.managePane = managePaneTools
		a.screen = ScreenMainMenu
		return a, nil

	case "tab":
		if a.managePane == managePaneTools {
			a.managePane = managePaneSettings
		} else {
			a.managePane = managePaneTools
		}
		return a, nil

	// Save (persist to config).
	case "s", "ctrl+s":
		a.manageStatus = "Saving…"
		return a, a.saveManageConfigCmd()
	}

	// Pane-specific navigation.
	if a.managePane == managePaneTools {
		switch key {
		case "up", "k":
			if a.manageIndex > 0 {
				a.manageIndex--
				a.configFieldIndex = 0
				a.manageFieldsScroll = 0
			}
			a.manageEnsureToolsVisible(layout, len(items))
			return a, nil

		case "down", "j":
			if a.manageIndex < len(items)-1 {
				a.manageIndex++
				a.configFieldIndex = 0
				a.manageFieldsScroll = 0
			}
			a.manageEnsureToolsVisible(layout, len(items))
			return a, nil

		case "right", "l", "enter":
			a.managePane = managePaneSettings
			return a, nil
		}

		return a, nil
	}

	// Settings pane.
	switch key {
	case "up", "k":
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
		a.manageEnsureFieldsVisible(layout, len(fields))
		return a, nil

	case "down", "j":
		if a.configFieldIndex < len(fields)-1 {
			a.configFieldIndex++
		}
		a.manageEnsureFieldsVisible(layout, len(fields))
		return a, nil

	case "left", "h":
		adjustField(-1)
		return a, nil

	case "right", "l":
		adjustField(1)
		return a, nil

	case " ":
		// Space toggles booleans. For options/numbers, it acts as "forward".
		if f, ok := currentField(); ok {
			switch f.kind {
			case manageFieldToggle:
				toggleField()
			case manageFieldOption:
				adjustField(1)
			case manageFieldNumber:
				adjustField(1)
			}
		}
		return a, nil

	case "enter":
		// Enter toggles boolean fields, or starts editing for text fields.
		if f, ok := currentField(); ok {
			switch f.kind {
			case manageFieldToggle:
				toggleField()
			case manageFieldText:
				startEditingField()
			case manageFieldOption:
				adjustField(1)
			case manageFieldNumber:
				// No modal editor for numbers yet; treat as increment.
				adjustField(1)
			}
		}
		return a, nil
	}

	return a, nil
}

func (a *App) handleManageMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// When editing, keep interaction keyboard-driven to avoid confusing focus
	// shifts and accidental toggles.
	if a.manageEditing {
		return a, nil
	}

	// Nothing to do if we don't have a valid layout yet.
	if a.width <= 0 || a.height <= 0 {
		return a, nil
	}

	layout := a.manageLayout()
	items := a.manageItems()

	// Wheel scroll: choose pane based on mouse X.
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

		if m.X < layout.rightX { // left side (tools)
			a.manageToolsScroll = clampInt(a.manageToolsScroll+delta, 0, layout.maxToolsScroll(len(items)))
		} else { // right side (fields)
			fields := a.manageFieldsFor(items[a.manageIndex].id)
			a.manageFieldsScroll = clampInt(a.manageFieldsScroll+delta, 0, layout.maxFieldsScroll(len(fields)))
		}
		return a, nil
	}

	// Only respond to left click presses for now.
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Click in left pane list area: select tool.
	if layout.inLeftList(m.X, m.Y) {
		relY := m.Y - layout.leftListY
		idx := a.manageToolsScroll + relY
		if idx >= 0 && idx < len(items) {
			a.managePane = managePaneTools
			if idx != a.manageIndex {
				a.manageIndex = idx
				a.configFieldIndex = 0
				a.manageFieldsScroll = 0
				a.manageEditing = false
				a.manageEditField = nil
				a.manageEditValue = ""
				a.manageStatus = ""
			}
			a.manageEnsureToolsVisible(layout, len(items))
		}
		return a, nil
	}

	// Click in right pane fields area: focus + edit/toggle/adjust.
	if layout.inRightFields(m.X, m.Y) {
		items := a.manageItems()
		if len(items) == 0 {
			return a, nil
		}

		fields := a.manageFieldsFor(items[a.manageIndex].id)
		if len(fields) == 0 {
			return a, nil
		}

		relY := m.Y - layout.rightFieldsY
		fieldIdx := a.manageFieldsScroll + relY
		if fieldIdx < 0 || fieldIdx >= len(fields) {
			return a, nil
		}

		a.managePane = managePaneSettings
		a.configFieldIndex = fieldIdx
		a.manageEnsureFieldsVisible(layout, len(fields))

		f := fields[fieldIdx]
		switch f.kind {
		case manageFieldToggle:
			if f.b != nil {
				*f.b = !*f.b
			}
		case manageFieldOption:
			// Click on left half cycles backward, right half cycles forward.
			forward := m.X >= (layout.rightX + layout.rightW/2)
			if f.str != nil && len(f.options) > 0 {
				*f.str = cycleStringOption(f.options, *f.str, forward)
			}
		case manageFieldNumber:
			// Click on left half decrements, right half increments.
			dir := -1
			if m.X >= (layout.rightX + layout.rightW/2) {
				dir = 1
			}
			if f.n != nil {
				step := f.step
				if step == 0 {
					step = 1
				}
				*f.n = clampInt(*f.n+dir*step, f.min, f.max)
			}
		case manageFieldText:
			// Single click just focuses. Enter starts editing (keyboard) for now.
		}

		return a, nil
	}

	return a, nil
}

// manageLayout captures all geometry needed for consistent rendering and mouse hit-testing.
type manageLayout struct {
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

	// Panel internals (we keep these constants in sync with render styles).
	border int
	padX   int
	padY   int

	// List and fields areas (absolute coordinates in terminal space).
	leftListY int
	leftListH int

	rightFieldsY int
	rightFieldsH int
}

func (l manageLayout) maxToolsScroll(itemsLen int) int {
	return maxInt(0, itemsLen-l.leftListH)
}

func (l manageLayout) maxFieldsScroll(fieldsLen int) int {
	return maxInt(0, fieldsLen-l.rightFieldsH)
}

func (l manageLayout) inLeftList(x, y int) bool {
	if x < l.leftX || x >= l.leftX+l.leftW {
		return false
	}
	return y >= l.leftListY && y < l.leftListY+l.leftListH
}

func (l manageLayout) inRightFields(x, y int) bool {
	if x < l.rightX || x >= l.rightX+l.rightW {
		return false
	}
	return y >= l.rightFieldsY && y < l.rightFieldsY+l.rightFieldsH
}

func (a *App) manageLayout() manageLayout {
	// Header/footer heights are kept fixed for consistent mouse mapping.
	const headerH = 3
	const footerH = 2

	bodyY := headerH
	bodyH := a.height - headerH - footerH
	if bodyH < 5 {
		bodyH = 5
	}

	gap := 1

	// Default split: 1/3 tools, 2/3 details.
	leftW := clampInt(a.width/3, 26, 42)
	minRight := 38
	if a.width-leftW-gap < minRight {
		leftW = maxInt(22, a.width-minRight-gap)
	}
	rightW := maxInt(0, a.width-leftW-gap)

	// Panel styling constants (must match render functions).
	border := 1
	padX := 1
	padY := 1

	// Left panel: title line + subtitle line + blank line.
	leftHeaderLines := 3
	leftInnerY := bodyY + border + padY
	leftInnerH := bodyH - (border * 2) - (padY * 2)
	leftListY := leftInnerY + leftHeaderLines
	leftListH := maxInt(1, leftInnerH-leftHeaderLines)

	// Right panel: title line + subtitle line + blank line.
	rightHeaderLines := 3
	rightInnerY := bodyY + border + padY
	rightInnerH := bodyH - (border * 2) - (padY * 2)
	rightFieldsY := rightInnerY + rightHeaderLines
	rightFieldsH := maxInt(1, rightInnerH-rightHeaderLines)

	return manageLayout{
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

		rightFieldsY: rightFieldsY,
		rightFieldsH: rightFieldsH,
	}
}

func (a *App) manageEnsureToolsVisible(layout manageLayout, itemsLen int) {
	if itemsLen <= 0 {
		a.manageIndex = 0
		a.manageToolsScroll = 0
		return
	}

	a.manageIndex = clampInt(a.manageIndex, 0, itemsLen-1)
	maxScroll := layout.maxToolsScroll(itemsLen)
	a.manageToolsScroll = clampInt(a.manageToolsScroll, 0, maxScroll)

	// Keep selection within [scroll, scroll+visible).
	if a.manageIndex < a.manageToolsScroll {
		a.manageToolsScroll = a.manageIndex
	} else if a.manageIndex >= a.manageToolsScroll+layout.leftListH {
		a.manageToolsScroll = a.manageIndex - layout.leftListH + 1
	}
	a.manageToolsScroll = clampInt(a.manageToolsScroll, 0, maxScroll)
}

func (a *App) manageEnsureFieldsVisible(layout manageLayout, fieldsLen int) {
	if fieldsLen <= 0 {
		a.configFieldIndex = 0
		a.manageFieldsScroll = 0
		return
	}

	a.configFieldIndex = clampInt(a.configFieldIndex, 0, fieldsLen-1)
	maxScroll := layout.maxFieldsScroll(fieldsLen)
	a.manageFieldsScroll = clampInt(a.manageFieldsScroll, 0, maxScroll)

	if a.configFieldIndex < a.manageFieldsScroll {
		a.manageFieldsScroll = a.configFieldIndex
	} else if a.configFieldIndex >= a.manageFieldsScroll+layout.rightFieldsH {
		a.manageFieldsScroll = a.configFieldIndex - layout.rightFieldsH + 1
	}
	a.manageFieldsScroll = clampInt(a.manageFieldsScroll, 0, maxScroll)
}

func (a *App) manageItems() []manageItem {
	reg := tools.NewRegistry()
	base := getManageTools()

	// Cache install status so we don't run package manager checks every render.
	if a.manageInstalled == nil {
		a.manageInstalled = make(map[string]bool, len(base))
	}
	if !a.manageInstalledReady {
		for _, mt := range base {
			if t, ok := reg.Get(mt.id); ok {
				a.manageInstalled[mt.id] = t.IsInstalled()
			} else {
				a.manageInstalled[mt.id] = false
			}
		}
		a.manageInstalledReady = true
	}

	// Add a global section at the top.
	items := []manageItem{
		{id: "global", name: "Global", icon: "󰒓", description: "UI + platform preferences", installed: true},
	}

	for _, mt := range base {
		name := mt.name
		desc := "Configure settings"
		icon := mt.icon
		installed := a.manageInstalled[mt.id]
		if t, ok := reg.Get(mt.id); ok {
			name = t.Name()
			desc = t.Description()
			if t.Icon() != "" {
				icon = t.Icon()
			}
		}
		items = append(items, manageItem{
			id:          mt.id,
			name:        name,
			icon:        icon,
			description: desc,
			installed:   installed,
		})
	}

	return items
}

func (a *App) manageFieldsFor(itemID string) []manageField {
	cfg := a.manageConfig
	if cfg == nil {
		return nil
	}

	switch itemID {
	case "global":
		return []manageField{
			{
				key:         "theme",
				label:       "Theme",
				description: "Controls generated tool configs (installer) and visual accents",
				kind:        manageFieldOption,
				str:         &a.theme,
				options:     config.AvailableThemes,
			},
			{
				key:         "nav",
				label:       "Navigation",
				description: "Default navigation style throughout the TUI",
				kind:        manageFieldOption,
				str:         &a.navStyle,
				options:     []string{"emacs", "vim"},
			},
		}

	case "ghostty":
		return []manageField{
			{key: "font_family", label: "Font Family", description: "Terminal font family", kind: manageFieldText, str: &cfg.GhosttyFontFamily},
			{key: "font_size", label: "Font Size", description: "Font size (pt)", kind: manageFieldNumber, n: &cfg.GhosttyFontSize, min: 8, max: 32, step: 1, unit: "pt"},
			{key: "opacity", label: "Opacity", description: "Background opacity (%)", kind: manageFieldNumber, n: &cfg.GhosttyOpacity, min: 0, max: 100, step: 5, unit: "%"},
			{key: "blur", label: "Blur Radius", description: "Background blur (platform dependent)", kind: manageFieldNumber, n: &cfg.GhosttyBlurRadius, min: 0, max: 40, step: 1},
			{key: "cursor", label: "Cursor Style", description: "Cursor shape", kind: manageFieldOption, str: &cfg.GhosstyCursorStyle, options: []string{"block", "bar", "underline"}},
			{key: "scrollback", label: "Scrollback", description: "Scrollback history lines", kind: manageFieldNumber, n: &cfg.GhosttyScrollbackLines, min: 1000, max: 200000, step: 1000, unit: " lines"},
			{key: "decor", label: "Window Decorations", description: "Show native window decorations", kind: manageFieldToggle, b: &cfg.GhosttyWindowDecorations},
			{key: "confirm_close", label: "Confirm Close", description: "Prompt before closing window", kind: manageFieldToggle, b: &cfg.GhosttyConfirmClose},
		}

	case "tmux":
		return []manageField{
			{key: "prefix", label: "Prefix Key", description: "Leader key for tmux commands", kind: manageFieldOption, str: &cfg.TmuxPrefix, options: []string{"C-a", "C-b", "C-Space"}},
			{key: "base", label: "Base Index", description: "Start window/pane numbering at", kind: manageFieldNumber, n: &cfg.TmuxBaseIndex, min: 0, max: 10, step: 1},
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.TmuxMouseMode},
			{key: "status_pos", label: "Status Position", description: "Status bar placement", kind: manageFieldOption, str: &cfg.TmuxStatusPosition, options: []string{"top", "bottom"}},
			{key: "pane_border", label: "Pane Border", description: "Pane border style", kind: manageFieldOption, str: &cfg.TmuxPaneBorderStyle, options: []string{"single", "double", "heavy", "simple"}},
			{key: "history", label: "History Limit", description: "Scrollback lines per pane", kind: manageFieldNumber, n: &cfg.TmuxHistoryLimit, min: 1000, max: 200000, step: 1000, unit: " lines"},
			{key: "escape", label: "Escape Time", description: "Escape timing for key chords", kind: manageFieldNumber, n: &cfg.TmuxEscapeTime, min: 0, max: 1000, step: 5, unit: "ms"},
			{key: "resize", label: "Aggressive Resize", description: "Aggressively resize panes on window changes", kind: manageFieldToggle, b: &cfg.TmuxAggressiveResize},
		}

	case "zsh":
		return []manageField{
			{key: "hist_size", label: "History Size", description: "Maximum history entries", kind: manageFieldNumber, n: &cfg.ZshHistorySize, min: 1000, max: 500000, step: 1000, unit: " entries"},
			{key: "hist_dups", label: "Ignore Duplicates", description: "Don't store duplicated history entries", kind: manageFieldToggle, b: &cfg.ZshHistoryIgnoreDups},
			{key: "autocd", label: "Auto CD", description: "Allow entering directories without typing cd", kind: manageFieldToggle, b: &cfg.ZshAutoCD},
			{key: "correct", label: "Auto Correction", description: "Suggest corrections for commands", kind: manageFieldToggle, b: &cfg.ZshCorrection},
			{key: "menu", label: "Completion Menu", description: "Use menu selection for completions", kind: manageFieldToggle, b: &cfg.ZshCompletionMenu},
			{key: "syntax", label: "Syntax Highlight", description: "Syntax highlighting in shell", kind: manageFieldToggle, b: &cfg.ZshSyntaxHighlight},
			{key: "autosug", label: "Auto Suggestions", description: "Inline suggestions from history", kind: manageFieldToggle, b: &cfg.ZshAutosuggestions},
		}

	case "neovim":
		return []manageField{
			{key: "numbers", label: "Line Numbers", description: "Absolute/relative/none", kind: manageFieldOption, str: &cfg.NeovimLineNumbers, options: []string{"absolute", "relative", "none"}},
			{key: "rel", label: "Relative Num", description: "Show relative line numbers", kind: manageFieldToggle, b: &cfg.NeovimRelativeNum},
			{key: "tab", label: "Tab Width", description: "Indent width", kind: manageFieldNumber, n: &cfg.NeovimTabWidth, min: 2, max: 8, step: 1, unit: " spaces"},
			{key: "expand", label: "Expand Tab", description: "Use spaces instead of tabs", kind: manageFieldToggle, b: &cfg.NeovimExpandTab},
			{key: "wrap", label: "Line Wrap", description: "Soft wrap long lines", kind: manageFieldToggle, b: &cfg.NeovimWrap},
			{key: "cursor", label: "Cursor Line", description: "Highlight current line", kind: manageFieldToggle, b: &cfg.NeovimCursorLine},
			{key: "clip", label: "Clipboard", description: "Clipboard integration", kind: manageFieldOption, str: &cfg.NeovimClipboard, options: []string{"unnamedplus", "unnamed", "none"}},
			{key: "undo", label: "Undo File", description: "Persistent undo on disk", kind: manageFieldToggle, b: &cfg.NeovimUndoFile},
		}

	case "git":
		return []manageField{
			{key: "branch", label: "Default Branch", description: "Default init branch name", kind: manageFieldOption, str: &cfg.GitDefaultBranch, options: []string{"main", "master", "develop"}},
			{key: "setup_remote", label: "Auto Setup Remote", description: "Auto-create tracking remotes on push", kind: manageFieldToggle, b: &cfg.GitAutoSetupRemote},
			{key: "rebase", label: "Pull Rebase", description: "Prefer rebase on git pull", kind: manageFieldToggle, b: &cfg.GitPullRebase},
			{key: "diff", label: "Diff Tool", description: "Default diff tool", kind: manageFieldOption, str: &cfg.GitDiffTool, options: []string{"delta", "difftastic", "vimdiff"}},
			{key: "merge", label: "Merge Tool", description: "Default merge tool", kind: manageFieldOption, str: &cfg.GitMergeTool, options: []string{"vimdiff", "nvimdiff", "meld"}},
			{key: "creds", label: "Credential Helper", description: "Credential helper backend", kind: manageFieldOption, str: &cfg.GitCredentialHelper, options: []string{"store", "cache", "osxkeychain"}},
			{key: "sign", label: "Sign Commits", description: "Require signed commits", kind: manageFieldToggle, b: &cfg.GitSignCommits},
		}

	case "yazi":
		return []manageField{
			{key: "hidden", label: "Show Hidden", description: "Show dotfiles by default", kind: manageFieldToggle, b: &cfg.YaziShowHidden},
			{key: "sort_by", label: "Sort By", description: "Sort order", kind: manageFieldOption, str: &cfg.YaziSortBy, options: []string{"alphabetical", "modified", "size", "natural"}},
			{key: "sort_rev", label: "Sort Reverse", description: "Reverse sort direction", kind: manageFieldToggle, b: &cfg.YaziSortReverse},
			{key: "linemode", label: "Line Mode", description: "Line metadata style", kind: manageFieldOption, str: &cfg.YaziLineMode, options: []string{"size", "permissions", "mtime", "none"}},
			{key: "scrolloff", label: "Scroll Offset", description: "Keep N items visible above/below cursor", kind: manageFieldNumber, n: &cfg.YaziScrollOff, min: 0, max: 20, step: 1, unit: " lines"},
		}

	case "fzf":
		return []manageField{
			{key: "opts", label: "Default Opts", description: "Extra CLI options passed to fzf", kind: manageFieldText, str: &cfg.FzfDefaultOpts},
			{key: "height", label: "Height", description: "Height percentage for fzf UI", kind: manageFieldNumber, n: &cfg.FzfHeight, min: 20, max: 100, step: 5, unit: "%"},
			{key: "layout", label: "Layout", description: "Layout mode", kind: manageFieldOption, str: &cfg.FzfLayout, options: []string{"reverse", "default", "reverse-list"}},
			{key: "border", label: "Border Style", description: "Border style for fzf window", kind: manageFieldOption, str: &cfg.FzfBorderStyle, options: []string{"rounded", "sharp", "bold", "none"}},
			{key: "preview", label: "Preview", description: "Enable preview pane", kind: manageFieldToggle, b: &cfg.FzfPreview},
			{key: "preview_window", label: "Preview Window", description: "Preview placement/size", kind: manageFieldOption, str: &cfg.FzfPreviewWindow, options: []string{"right:50%", "up:50%", "down:50%"}},
		}

	case "lazygit":
		return []manageField{
			{key: "side", label: "Side-by-Side Diff", description: "Use side-by-side diffs", kind: manageFieldToggle, b: &cfg.LazyGitSideBySide},
			{key: "paging", label: "Paging", description: "Paging backend", kind: manageFieldOption, str: &cfg.LazyGitPaging, options: []string{"delta", "diff-so-fancy", "never"}},
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.LazyGitMouseMode},
			{key: "gui_theme", label: "GUI Theme", description: "GUI theme selection", kind: manageFieldOption, str: &cfg.LazyGitGuiTheme, options: []string{"auto", "light", "dark"}},
		}

	case "lazydocker":
		return []manageField{
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.LazyDockerMouseMode},
			{key: "tail", label: "Logs Tail", description: "How many log lines to show", kind: manageFieldNumber, n: &cfg.LazyDockerLogsTail, min: 10, max: 2000, step: 10, unit: " lines"},
		}

	case "btop":
		return []manageField{
			{key: "theme", label: "Theme", description: "btop theme name", kind: manageFieldOption, str: &cfg.BtopTheme, options: []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}},
			{key: "rate", label: "Update Rate", description: "Refresh interval", kind: manageFieldNumber, n: &cfg.BtopUpdateMs, min: 250, max: 10000, step: 250, unit: "ms"},
			{key: "temp", label: "Show Temp", description: "Show CPU temperature", kind: manageFieldToggle, b: &cfg.BtopShowTemp},
			{key: "scale", label: "Temp Scale", description: "Celsius/Fahrenheit", kind: manageFieldOption, str: &cfg.BtopTempScale, options: []string{"celsius", "fahrenheit"}},
			{key: "graph", label: "Graph Symbol", description: "Graph rendering symbol set", kind: manageFieldOption, str: &cfg.BtopGraphSymbol, options: []string{"braille", "block", "tty"}},
			{key: "boxes", label: "Shown Boxes", description: "Which panels to show", kind: manageFieldText, str: &cfg.BtopShownBoxes},
		}

	case "glow":
		return []manageField{
			{key: "style", label: "Style", description: "Style theme for Glow", kind: manageFieldOption, str: &cfg.GlowStyle, options: []string{"auto", "dark", "light", "notty"}},
			{key: "pager", label: "Pager", description: "Pager program", kind: manageFieldOption, str: &cfg.GlowPager, options: []string{"less", "more", "none"}},
			{key: "width", label: "Width", description: "Max render width", kind: manageFieldNumber, n: &cfg.GlowWidth, min: 40, max: 240, step: 5, unit: " chars"},
			{key: "mouse", label: "Mouse", description: "Enable mouse support in Glow", kind: manageFieldToggle, b: &cfg.GlowMouse},
		}
	}

	return nil
}

func (a *App) renderManageDualPane() string {
	if a.width == 0 || a.height == 0 {
		return "Loading..."
	}

	layout := a.manageLayout()
	items := a.manageItems()

	// Clamp selection safely (important if config/tools list changes).
	if len(items) > 0 {
		a.manageIndex = clampInt(a.manageIndex, 0, len(items)-1)
	} else {
		a.manageIndex = 0
	}

	// Keep scrolls sane.
	a.manageEnsureToolsVisible(layout, len(items))
	fields := []manageField(nil)
	if len(items) > 0 {
		fields = a.manageFieldsFor(items[a.manageIndex].id)
	}
	a.manageEnsureFieldsVisible(layout, len(fields))

	header := a.renderManageHeader(layout.w)
	footer := a.renderManageFooter(layout.w, items, fields)

	left := a.renderManageToolsPanel(layout, items)
	right := a.renderManageSettingsPanel(layout, items, fields)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", layout.gap), right)

	// Ensure the overall string is exactly terminal height. This helps avoid visual "jitter".
	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	if lipgloss.Height(view) < a.height {
		view += strings.Repeat("\n", a.height-lipgloss.Height(view))
	}
	return view
}

func (a *App) renderManageHeader(width int) string {
	title := lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true).
		Render("DOTFILES MANAGER")
	sub := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Dual-pane config editor • Click, scroll, and tweak everything")
	divider := ScanlineEffect(maxInt(0, width))

	// Keep this exactly 3 lines (see manageLayout.headerH).
	return lipgloss.JoinVertical(lipgloss.Left, title, sub, divider)
}

func (a *App) renderManageFooter(width int, items []manageItem, fields []manageField) string {
	// Hint line: short and consistent.
	hints := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"Tab switch pane • ↑↓ move • ←→ adjust • Space toggle • Enter edit • S save • Esc back • q quit",
	)

	// Status line: either save feedback, or focused field description.
	statusText := a.manageStatus
	if statusText == "" && a.managePane == managePaneSettings && len(fields) > 0 {
		idx := clampInt(a.configFieldIndex, 0, len(fields)-1)
		if fields[idx].description != "" {
			statusText = fields[idx].description
		}
	}

	if statusText == "" {
		statusText = " "
	}
	status := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(statusText, width))

	// Keep this exactly 2 lines (see manageLayout.footerH).
	return lipgloss.JoinVertical(lipgloss.Left, hints, status)
}

func (a *App) renderManageToolsPanel(layout manageLayout, items []manageItem) string {
	borderColor := ColorBorder
	if a.managePane == managePaneTools {
		borderColor = ColorCyan
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		// lipgloss applies borders after Width/Height, so subtract 2 to target an
		// exact outer size for predictable layouts and mouse hit-testing.
		Width(maxInt(1, layout.leftW-2)).
		Height(maxInt(1, layout.bodyH-2))

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("TOOLS")
	toolCount := maxInt(0, len(items)-1) // exclude "Global"
	installedCount := 0
	for _, it := range items {
		if it.id == "global" {
			continue
		}
		if it.installed {
			installedCount++
		}
	}
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(fmt.Sprintf("%d installed • %d tools", installedCount, toolCount))

	innerW := maxInt(0, layout.leftW-(layout.border*2)-(layout.padX*2))

	var lines []string
	for i := a.manageToolsScroll; i < len(items) && len(lines) < layout.leftListH; i++ {
		it := items[i]
		focused := i == a.manageIndex

		cursor := "  "
		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		if it.id != "global" && !it.installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		}
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			nameStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		}

		status := StatusDot("pending")
		if it.id == "global" {
			status = lipgloss.NewStyle().Foreground(ColorCyan).Render("●")
		} else if it.installed {
			status = StatusDot("success")
		}

		icon := it.icon
		if icon != "" {
			icon += " "
		}

		line := fmt.Sprintf("%s%s %s%s", cursor, status, icon, nameStyle.Render(it.name))
		lines = append(lines, truncateVisible(line, innerW))
	}

	// Pad list to keep the panel stable.
	for len(lines) < layout.leftListH {
		lines = append(lines, "")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		sub,
		"",
		strings.Join(lines, "\n"),
	)

	return panel.Render(content)
}

func (a *App) renderManageSettingsPanel(layout manageLayout, items []manageItem, fields []manageField) string {
	borderColor := ColorBorder
	if a.managePane == managePaneSettings {
		borderColor = ColorCyan
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		// lipgloss applies borders after Width/Height, so subtract 2 to target an
		// exact outer size for predictable layouts and mouse hit-testing.
		Width(maxInt(1, layout.rightW-2)).
		Height(maxInt(1, layout.bodyH-2))

	if len(items) == 0 {
		return panel.Render(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No tools found"))
	}

	item := items[a.manageIndex]

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("SETTINGS")
	statusBadge := ""
	if item.id != "global" {
		if item.installed {
			statusBadge = lipgloss.NewStyle().Foreground(ColorGreen).Render(" ● installed")
		} else {
			statusBadge = lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" ○ not installed")
		}
	}
	meta := lipgloss.NewStyle().Foreground(ColorTextBright).Bold(true).Render(item.name) +
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" — "+item.description) +
		statusBadge

	innerW := maxInt(0, layout.rightW-(layout.border*2)-(layout.padX*2))

	// Field list lines (fixed height for stable layout).
	visibleFieldLines := layout.rightFieldsH
	fieldCapacity := visibleFieldLines
	if a.manageEditing && a.manageEditField != nil && fieldCapacity > 0 {
		// Reserve the first line for the editor, but keep overall height stable.
		fieldCapacity--
	}

	var fieldLines []string
	for i := a.manageFieldsScroll; i < len(fields) && len(fieldLines) < fieldCapacity; i++ {
		f := fields[i]
		focused := (a.managePane == managePaneSettings) && (i == a.configFieldIndex)
		fieldLines = append(fieldLines, truncateVisible(renderManageFieldLine(f, focused), innerW))
	}
	for len(fieldLines) < fieldCapacity {
		fieldLines = append(fieldLines, "")
	}

	var fieldsBlock string
	if a.manageEditing && a.manageEditField != nil && visibleFieldLines > 0 {
		fieldsBlock = strings.Join(append([]string{a.renderManageInlineEditor(innerW)}, fieldLines...), "\n")
	} else {
		fieldsBlock = strings.Join(fieldLines, "\n")
	}

	// Exactly 3 header lines before the fields area (matches manageLayout.rightHeaderLines).
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		truncateVisible(meta, innerW),
		"",
		fieldsBlock,
	)

	return panel.Render(content)
}

func renderManageFieldLine(f manageField, focused bool) string {
	// Left label column.
	labelStyle := lipgloss.NewStyle().Foreground(ColorText).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	cursor := "  "

	if focused {
		cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
		labelStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Width(18)
		valueStyle = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	}

	switch f.kind {
	case manageFieldToggle:
		val := lipgloss.NewStyle().Foreground(ColorRed).Render("OFF")
		if f.b != nil && *f.b {
			val = lipgloss.NewStyle().Foreground(ColorGreen).Render("ON")
		}
		return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), val)

	case manageFieldNumber:
		if f.n == nil {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
		}
		leftArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("◀")
		rightArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("▶")
		if focused {
			leftArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("◀")
			rightArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("▶")
		}
		val := valueStyle.Render(fmt.Sprintf("%d%s", *f.n, f.unit))
		return fmt.Sprintf("%s%s %s %s %s", cursor, labelStyle.Render(f.label), leftArrow, val, rightArrow)

	case manageFieldOption:
		if f.str == nil || len(f.options) == 0 {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
		}

		var parts []string
		for _, opt := range f.options {
			s := lipgloss.NewStyle().Foreground(ColorTextMuted)
			if opt == *f.str {
				s = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
			}
			parts = append(parts, s.Render(opt))
		}
		return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), strings.Join(parts, " │ "))

	case manageFieldText:
		if f.str == nil {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
		}
		val := *f.str
		if val == "" {
			val = "—"
		}
		return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render(val))
	}

	return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
}

func (a *App) renderManageInlineEditor(width int) string {
	// Single-line editor used for string fields.
	//
	// Important: this must remain ONE LINE so the fields pane layout and mouse
	// hit-testing remain stable.
	if width <= 0 {
		return ""
	}

	// Render a caret by inserting a solid block at the current position.
	runes := []rune(a.manageEditValue)
	cur := clampInt(a.manageEditCursor, 0, len(runes))
	left := string(runes[:cur])
	right := string(runes[cur:])

	plain := fmt.Sprintf("EDIT %s: %s%s%s", a.manageEditFieldKey, left, "█", right)
	plain = truncatePlain(plain, width)

	return lipgloss.NewStyle().
		Foreground(ColorTextBright).
		Background(ColorOverlay).
		Render(plain)
}

func (a *App) manageStartEditing(field manageField) {
	if field.kind != manageFieldText || field.str == nil {
		return
	}

	a.manageEditing = true
	a.manageEditField = field.str
	a.manageEditFieldKey = field.label
	a.manageEditValue = *field.str
	a.manageEditCursor = utf8.RuneCountInString(a.manageEditValue)
}

func (a *App) manageCommitEditing() {
	if !a.manageEditing || a.manageEditField == nil {
		return
	}
	*a.manageEditField = a.manageEditValue
	a.manageEditing = false
	a.manageEditField = nil
	a.manageEditFieldKey = ""
}

func (a *App) manageCancelEditing() {
	a.manageEditing = false
	a.manageEditField = nil
	a.manageEditFieldKey = ""
	a.manageEditValue = ""
	a.manageEditCursor = 0
}

// Utility helpers local to this file.

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cycleStringOption(opts []string, current string, forward bool) string {
	if len(opts) == 0 {
		return current
	}
	for i, o := range opts {
		if o == current {
			if forward {
				return opts[(i+1)%len(opts)]
			}
			return opts[(i-1+len(opts))%len(opts)]
		}
	}
	return opts[0]
}

// truncateVisible truncates a string to a visible width, being conservative with ANSI sequences.
// We use lipgloss.Width which accounts for ANSI, and rune-based slicing as a best-effort.
func truncateVisible(s string, width int) string {
	if width <= 0 {
		return ""
	}
	// ANSI-safe truncation (won't break escape sequences).
	return ansi.Truncate(s, width, "…")
}

func truncatePlain(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}
