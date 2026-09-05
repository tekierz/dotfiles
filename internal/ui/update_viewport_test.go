package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestUpdatesViewportKeepsCursorAndChromeVisible(t *testing.T) {
	ctx := newGoldenContext(t)
	a := ctx.app
	for i := 0; i < 35; i++ {
		a.updateResults = append(a.updateResults, pkg.Package{Name: fmt.Sprintf("package-%02d", i), CurrentVersion: "1", LatestVersion: "2"})
	}
	a.updateCheckDone = true
	a.updateStatus = "Prior update finished"
	a.updateError = errors.New("one provider unavailable")
	screen := NewUpdateScreen(ctx)
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 18}} {
		a.width, a.height = size[0], size[1]
		for _, index := range []int{0, 17, 34} {
			a.updateIndex = index
			out := screen.View(size[0], size[1])
			if got := lipgloss.Height(out); got > size[1] {
				t.Fatalf("%v cursor%d: height%d", size, index, got)
			}
			if got := lipgloss.Width(out); got > size[0] {
				t.Fatalf("%v cursor%d: width%d", size, index, got)
			}
			for _, text := range []string{"Package Updates", "PACKAGE", "space", fmt.Sprintf("package-%02d", index), "▸"} {
				if !strings.Contains(out, text) {
					t.Fatalf("%v cursor%d missing %q", size, index, text)
				}
			}
		}
	}
	a.updateIndex = 0
	for i := 0; i < 34; i++ {
		screen.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	screen.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	if a.updateIndex != 34 || !a.updateSelected[34] {
		t.Fatalf("selection lost actual package index: cursor%d selected%v", a.updateIndex, a.updateSelected)
	}
}
