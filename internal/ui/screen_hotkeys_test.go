package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func hotkeysRuneMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func hotkeysSpaceMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
}

func hotkeysCmdIsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestHotkeysAliasInputCapturesPrintableKeys(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	screen := NewHotkeysScreen(ctx)

	app.hotkeysAddingAlias = true
	app.hotkeysAliasField = 0
	app.hotkeysAliasCursor = 0

	for _, msg := range []tea.KeyMsg{
		hotkeysRuneMsg("h"),
		hotkeysRuneMsg("l"),
		hotkeysSpaceMsg(),
		hotkeysRuneMsg("q"),
	} {
		_, cmd := screen.Update(msg)
		if hotkeysCmdIsQuit(cmd) {
			t.Fatalf("%q quit while alias input was focused", msg.String())
		}
	}

	const want = "hl q"
	if app.hotkeysAliasName != want {
		t.Fatalf("alias name = %q, want %q", app.hotkeysAliasName, want)
	}
	if app.hotkeysAliasCursor != len([]rune(want)) {
		t.Fatalf("alias cursor = %d, want %d", app.hotkeysAliasCursor, len([]rune(want)))
	}
	if !app.hotkeysAddingAlias {
		t.Fatal("alias input lost focus after printable input")
	}
}

func TestHotkeysQQuitsOnlyWhenAliasInputNotFocused(t *testing.T) {
	t.Run("not typing", func(t *testing.T) {
		ctx := newGoldenContext(t)
		screen := NewHotkeysScreen(ctx)

		_, cmd := screen.Update(hotkeysRuneMsg("q"))
		if !hotkeysCmdIsQuit(cmd) {
			t.Fatal("q outside alias input should quit")
		}
	})

	t.Run("typing", func(t *testing.T) {
		ctx := newGoldenContext(t)
		app := ctx.app
		screen := NewHotkeysScreen(ctx)
		app.hotkeysAddingAlias = true

		_, cmd := screen.Update(hotkeysRuneMsg("q"))
		if hotkeysCmdIsQuit(cmd) {
			t.Fatal("q inside alias input should be captured as text, not quit")
		}
		if app.hotkeysAliasName != "q" {
			t.Fatalf("alias name = %q, want %q", app.hotkeysAliasName, "q")
		}
	})
}
