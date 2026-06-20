package ui

import (
	"fmt"
	"testing"
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
				{name: "alice", theme: "catppuccin-mocha", navStyle: "emacs", keyboard: "linux"},
			}
			screen := NewUsersScreen(ctx)
			_ = screen.View(w, h)
		})
	}
}
