package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestUsersScreenViewNoPanicAtTinyDimensions is the regression guard for C2:
// usersScreen.View must never panic (strings: negative Repeat count) at small
// terminal dimensions. The audit proved panics at 1x1, 2x2, 3x3, 80x1, 80x2,
// 80x3, 5x5, 4x4 — this sweep covers all of those.
func TestUsersScreenViewNoPanicAtTinyDimensions(t *testing.T) {
	dims := [][2]int{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 4},
		{5, 5},
		{1, 24},
		{80, 1},
		{80, 2},
		{80, 3},
	}

	for _, d := range dims {
		w, h := d[0], d[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("usersScreen.View(%d, %d) panicked: %v", w, h, r)
				}
			}()

			ctx := newGoldenContext(t)
			ctx.app.width = w
			ctx.app.height = h
			screen := NewUsersScreen(ctx)
			// Must not panic regardless of content.
			_ = screen.View(w, h)
		})
	}
}

// TestUsersC19FirstClickNoShift is the regression guard for C19: the
// description-line row-shift in handleMouse must only apply when the settings
// pane was ALREADY active during the last render (prevPane==usersPaneSettings).
// A first click that switches INTO the settings pane (prevPane==usersPaneList)
// must resolve to the correct field without the shift.
func TestUsersC19FirstClickNoShift(t *testing.T) {
	// Setup: three fields, each with a description. Field 0 is selected.
	// usersPane starts as list pane (simulating "no prior settings render").
	makeCtx := func(t *testing.T) (*ScreenContext, *usersScreen) {
		t.Helper()
		ctx := newGoldenContext(t)
		ctx.app.width = 80
		ctx.app.height = 24
		ctx.app.usersItems = []userItem{
			{name: "alice", theme: defaultTheme, navStyle: navEmacs, keyboard: "linux"},
		}
		ctx.app.usersPane = usersPaneList
		ctx.app.usersFieldIndex = 0
		s := NewUsersScreen(ctx)
		return ctx, s
	}

	// firstRowY matches the constant in handleMouse.
	const firstRowY = usersTabBarRows + usersHeaderRows
	leftW := 80 / 3 // matches leftPaneWidth in handleMouse

	t.Run("first_click_into_settings_pane_no_shift", func(t *testing.T) {
		// Click at field row 2 (0-indexed relative to firstRowY) when coming from
		// the list pane. Because prevPane==usersPaneList, the description-line shift
		// must NOT fire, so fieldIdx stays at 2 and resolves to field[2] (Keyboard).
		ctx, s := makeCtx(t)
		click := tea.MouseMsg{
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
			X:      leftW + 1, // right side = settings pane
			Y:      firstRowY + 2,
		}
		s.Update(click)
		if got, want := ctx.app.usersFieldIndex, 2; got != want {
			t.Errorf("first click into settings pane: usersFieldIndex = %d, want %d (no description-line shift)", got, want)
		}
		if ctx.app.usersPane != usersPaneSettings {
			t.Error("usersPane should have switched to settings")
		}
	})

	t.Run("subsequent_click_in_settings_pane_applies_shift", func(t *testing.T) {
		// Already in settings pane, field 0 selected (has description), so the
		// description line occupies relative row 1. A click at relative row 2
		// should shift down by one, resolving to field[1] (Navigation).
		ctx, s := makeCtx(t)
		ctx.app.usersPane = usersPaneSettings // already in settings
		ctx.app.usersFieldIndex = 0           // field 0 selected — description drawn at row 1

		click := tea.MouseMsg{
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
			X:      leftW + 1,
			Y:      firstRowY + 2, // row 2 → after shift → field index 1
		}
		s.Update(click)
		if got, want := ctx.app.usersFieldIndex, 1; got != want {
			t.Errorf("subsequent click with description shift: usersFieldIndex = %d, want %d", got, want)
		}
	})
}

// TestUsersScreenViewNoPanicWithUsers ensures the clamps also hold when there
// are actual user items in the list (hits the settings-pane render path).
func TestUsersScreenViewNoPanicWithUsers(t *testing.T) {
	dims := [][2]int{
		{2, 2},
		{3, 3},
		{80, 2},
	}

	for _, d := range dims {
		w, h := d[0], d[1]
		t.Run(fmt.Sprintf("%dx%d_with_users", w, h), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("usersScreen.View(%d, %d) with users panicked: %v", w, h, r)
				}
			}()

			ctx := newGoldenContext(t)
			ctx.app.width = w
			ctx.app.height = h
			ctx.app.usersItems = []userItem{
				{name: "alice", theme: defaultTheme, navStyle: navEmacs, keyboard: "linux"},
			}
			screen := NewUsersScreen(ctx)
			_ = screen.View(w, h)
		})
	}
}
