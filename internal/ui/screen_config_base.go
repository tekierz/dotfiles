package ui

import (
	tea "github.com/charmbracelet/bubbletea"
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

// Init returns any initial commands (none on entry).
func (s *configFieldNav) Init() tea.Cmd { return nil }

// back resets the focused field and returns to the deep-dive menu through the
// manager. Mirrors the legacy "esc/enter" behavior (configFieldIndex = 0).
func (s *configFieldNav) back() (bool, tea.Cmd) {
	if a := s.App(); a != nil {
		a.configFieldIndex = 0
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
		case "esc", "enter":
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

// handleConfigFieldMouse mirrors the legacy handleConfigScreenMouse behavior for
// the field-based config screens: scroll wheel and click move configFieldIndex.
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

	// Config screens have fields listed vertically. Approximate click detection
	// based on Y position (matches the legacy heuristic). maxFields is the
	// highest valid index, so the field count is maxFields+1.
	fieldCount := maxFields + 1
	contentHeight := fieldCount + 8
	startY := (a.height - contentHeight) / 2
	fieldStartY := startY + 4 // After title

	if m.Y >= fieldStartY {
		fieldIdx := m.Y - fieldStartY
		if fieldIdx >= 0 && fieldIdx <= maxFields {
			a.configFieldIndex = fieldIdx
		}
	}
	return nil
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

// Init returns any initial commands (none on entry).
func (s *configListNav) Init() tea.Cmd { return nil }

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

	maxFields := len(s.itemIDs)
	contentHeight := maxFields + 8
	startY := (a.height - contentHeight) / 2
	fieldStartY := startY + 4

	if m.Y >= fieldStartY {
		fieldIdx := m.Y - fieldStartY
		if fieldIdx >= 0 && fieldIdx < maxFields {
			s.setIndex(a, fieldIdx)
		}
	}
	return nil
}
