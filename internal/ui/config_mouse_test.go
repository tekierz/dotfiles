package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// clickAt builds a left-button press MouseMsg at the given screen cell.
func clickAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}
}

// labelLineY returns the screen Y (line index) of the first rendered line that
// contains substr in the screen's View output. It strips ANSI so the search
// works against the visible text. Returns -1 when not found.
func labelLineY(t *testing.T, view, substr string) int {
	t.Helper()
	for i, ln := range strings.Split(view, "\n") {
		if strings.Contains(stripANSITest(ln), substr) {
			return i
		}
	}
	return -1
}

func stripANSITest(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestConfigFieldClickGhostty verifies geometry-correct click-to-select on the
// Ghostty config screen. A click on the Nth field's rendered label row must set
// configFieldIndex to N (not N*1-row as the legacy mapping assumed, and not the
// wrong field caused by selector wrapping). It also asserts that a click far
// outside the box selects nothing.
func TestConfigFieldClickGhostty(t *testing.T) {
	const w, h = 80, 50

	// (label substring, expected field index). Font Family (field 0) renders a
	// selector that wraps to a second line, so field 1's row is NOT at a uniform
	// 3-row stride — this is exactly what the legacy 1-row/3-row math gets wrong.
	cases := []struct {
		label string
		idx   int
	}{
		{"Font Family", 0},
		{"Font Size", 1},
		{"Background Opacity", 2},
		{"Blur Radius", 3},
		{"Scrollback Bytes", 4},
		{"Cursor Style", 5},
		{"New Tab Keybinding", 6},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			screen := NewConfigGhosttyScreen(ctx)

			// Render so per-field geometry is recorded for the mouse handler.
			out := screen.View(w, h)
			y := labelLineY(t, out, tc.label)
			if y < 0 {
				t.Fatalf("label %q not found in view", tc.label)
			}

			ctx.app.configFieldIndex = 999 // sentinel: must be overwritten
			if _, _ = screen.Update(clickAt(30, y)); ctx.app.configFieldIndex != tc.idx {
				t.Errorf("click on %q (Y=%d): configFieldIndex = %d, want %d",
					tc.label, y, ctx.app.configFieldIndex, tc.idx)
			}
		})
	}

	t.Run("click outside box selects nothing", func(t *testing.T) {
		ctx := newDeepDiveContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		screen := NewConfigGhosttyScreen(ctx)
		_ = screen.View(w, h)

		ctx.app.configFieldIndex = 3
		// Far above the box (in the title region) — must not change selection.
		screen.Update(clickAt(30, 0))
		if ctx.app.configFieldIndex != 3 {
			t.Errorf("click above box changed selection to %d, want unchanged (3)", ctx.app.configFieldIndex)
		}
		// Far to the left of the centered box — must not change selection.
		ctx.app.configFieldIndex = 2
		screen.Update(clickAt(0, 14))
		if ctx.app.configFieldIndex != 2 {
			t.Errorf("click left of box changed selection to %d, want unchanged (2)", ctx.app.configFieldIndex)
		}
	})
}

// TestConfigFieldClickTmuxHiddenField verifies that on the Tmux screen a mouse
// click can never land on a hidden field. With TPM disabled, fields 8..12 are
// not rendered; a click in the lower region of the box must resolve to a visible
// field (the last visible one) and never to a hidden index.
func TestConfigFieldClickTmuxHidden(t *testing.T) {
	const w, h = 80, 60

	t.Run("TPM disabled: click never selects hidden field", func(t *testing.T) {
		ctx := newDeepDiveContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		ctx.app.deepDiveConfig.TmuxTPMEnabled = false
		screen := NewConfigTmuxScreen(ctx)

		out := screen.View(w, h)
		// "Enable TPM" is field 7, the last visible field when TPM is off.
		y := labelLineY(t, out, "Enable TPM")
		if y < 0 {
			t.Fatal("Enable TPM label not found")
		}
		ctx.app.configFieldIndex = 0
		screen.Update(clickAt(30, y))
		if ctx.app.configFieldIndex != 7 {
			t.Errorf("click on Enable TPM: configFieldIndex = %d, want 7", ctx.app.configFieldIndex)
		}
		if ctx.app.configFieldIndex > tmuxMaxField(ctx.app) {
			t.Errorf("selected hidden field %d (maxField=%d)", ctx.app.configFieldIndex, tmuxMaxField(ctx.app))
		}
	})

	t.Run("continuum hidden: click on a visible plugin maps correctly", func(t *testing.T) {
		ctx := newDeepDiveContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		ctx.app.deepDiveConfig.TmuxTPMEnabled = true
		ctx.app.deepDiveConfig.TmuxPluginContinuum = false // field 12 hidden
		screen := NewConfigTmuxScreen(ctx)

		out := screen.View(w, h)
		y := labelLineY(t, out, "tmux-yank") // field 11, last visible
		if y < 0 {
			t.Fatal("tmux-yank label not found")
		}
		ctx.app.configFieldIndex = 0
		screen.Update(clickAt(30, y))
		if ctx.app.configFieldIndex != 11 {
			t.Errorf("click on tmux-yank: configFieldIndex = %d, want 11", ctx.app.configFieldIndex)
		}
	})
}

// TestConfigFieldClickFamily exercises the recorded geometry across the rest of
// the configFieldNav family (the screens converted alongside ghostty/tmux),
// asserting that clicking a representative field's rendered label row selects
// that field. This guards every sibling against regressing to the old
// 1-row-per-field mapping. Each case clicks a label known to map to the given
// logical index; section headers and looped radio/checkbox rows are covered too.
func TestConfigFieldClickFamily(t *testing.T) {
	const w, h = 80, 60

	type fieldCase struct {
		label string
		idx   int
	}
	cases := []struct {
		name  string
		build func(ctx *ScreenContext) ScreenHandler
		want  []fieldCase
	}{
		{
			name:  "btop",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigBtopScreen(ctx) },
			want:  []fieldCase{{"Theme", 0}, {"Update Interval", 1}, {"Show CPU Temp", 2}, {"Graph Type", 3}},
		},
		{
			name:  "yazi",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigYaziScreen(ctx) },
			want:  []fieldCase{{"Keymap Style", 0}, {"Show Hidden Files", 1}, {"File Preview", 2}},
		},
		{
			name:  "lazygit",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigLazyGitScreen(ctx) },
			want:  []fieldCase{{"Wide Side Panel", 0}, {"Mouse Mode", 1}, {"Theme", 2}, {"Paging", 3}},
		},
		{
			name:  "glow",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigGlowScreen(ctx) },
			want:  []fieldCase{{"Style", 0}, {"Use Pager", 1}, {"Width", 2}, {"Mouse", 3}, {"Show All Files", 4}, {"Line Numbers", 5}, {"Preserve Newlines", 6}},
		},
		{
			name:  "fzf",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigFzfScreen(ctx) },
			want:  []fieldCase{{"File Preview", 0}, {"Window Height", 1}, {"Layout", 2}},
		},
		{
			name:  "git",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigGitScreen(ctx) },
			want:  []fieldCase{{"Delta Diff View", 0}, {"Default Branch", 1}, {"Pull with Rebase", 2}, {"Credential Helper", 4}},
		},
		{
			// zsh: radio group (Powerlevel10k=field 0) under a section header, then
			// settings + a checkbox plugin loop.
			name:  "zsh",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigZshScreen(ctx) },
			want:  []fieldCase{{"Powerlevel10k", 0}, {"Minimal", 3}, {"History Size", 4}, {"Auto CD", 5}},
		},
		{
			// neovim: radio configs (Kickstart=field 0) + editor settings + LSP loop.
			name:  "neovim",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigNeovimScreen(ctx) },
			want:  []fieldCase{{"Kickstart.nvim", 0}, {"Tab Width", 4}, {"Line Wrapping", 5}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			screen := tc.build(ctx)
			out := screen.View(w, h)
			for _, fc := range tc.want {
				y := labelLineY(t, out, fc.label)
				if y < 0 {
					t.Fatalf("%s: label %q not found", tc.name, fc.label)
				}
				ctx.app.configFieldIndex = 999
				screen.Update(clickAt(30, y))
				if ctx.app.configFieldIndex != fc.idx {
					t.Errorf("%s: click on %q (Y=%d) => field %d, want %d",
						tc.name, fc.label, y, ctx.app.configFieldIndex, fc.idx)
				}
			}
		})
	}
}

// TestConfigFieldClickWidthSweep verifies that click-to-select resolves
// correctly across a wide range of terminal widths (40-120). For each width it
// renders the Ghostty screen and the Git screen (which has a section header),
// then asserts that each field's rendered label row falls within the field's
// recorded extent so a click on that row resolves to that field.
//
// Before the wrap-inset fix the recorder wrapped at boxWidth-6 (border+padding)
// while lipgloss wraps at boxWidth-4 (padding only), causing misalignment at
// widths such as 41-45 and 89-90 where a selector line falls in the 2-column
// gap. After the fix (configBoxWrapInset=2) no misalignment occurs.
func TestConfigFieldClickWidthSweep(t *testing.T) {
	const h = 60

	type fieldCase struct {
		label string
		idx   int
	}

	screens := []struct {
		name   string
		build  func(ctx *ScreenContext) ScreenHandler
		fields []fieldCase
	}{
		{
			name:  "ghostty",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigGhosttyScreen(ctx) },
			fields: []fieldCase{
				{"Font Family", 0},
				{"Font Size", 1},
				{"Background Opacity", 2},
				{"Blur Radius", 3},
				{"Scrollback Bytes", 4},
				{"Cursor Style", 5},
				{"New Tab Keybinding", 6},
			},
		},
		{
			name:  "git",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigGitScreen(ctx) },
			fields: []fieldCase{
				{"Delta Diff View", 0},
				{"Default Branch", 1},
				{"Pull with Rebase", 2},
			},
		},
	}

	for w := 40; w <= 120; w++ {
		for _, sc := range screens {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			screen := sc.build(ctx)
			out := screen.View(w, h)

			for _, fc := range sc.fields {
				y := labelLineY(t, out, fc.label)
				if y < 0 {
					// Label not visible at this width (e.g. too narrow); skip.
					continue
				}

				ctx.app.configFieldIndex = 999
				screen.Update(clickAt(w/2, y))
				if ctx.app.configFieldIndex != fc.idx {
					t.Errorf("w=%d screen=%s: click on %q (Y=%d) => field %d, want %d",
						w, sc.name, fc.label, y, ctx.app.configFieldIndex, fc.idx)
				}
			}
		}
	}
}

// TestConfigClaudeCodeClick verifies the Claude Code screen's geometry-correct
// click handler: clicking the install toggle row selects index -1, and clicking
// an MCP row (below the "MCP Servers" header block) selects that MCP's index.
func TestConfigClaudeCodeClick(t *testing.T) {
	const w, h = 80, 50
	ctx := newDeepDiveContext(t)
	ctx.app.width, ctx.app.height = w, h
	ctx.Width, ctx.Height = w, h
	screen := NewConfigClaudeCodeScreen(ctx)
	out := screen.View(w, h)

	// Install toggle row -> index -1.
	y := labelLineY(t, out, "Install Claude Code")
	if y < 0 {
		t.Fatal("Install Claude Code label not found")
	}
	ctx.app.configFieldIndex = 99
	screen.Update(clickAt(30, y))
	if ctx.app.configFieldIndex != -1 {
		t.Errorf("click on install toggle: configFieldIndex = %d, want -1", ctx.app.configFieldIndex)
	}

	// First MCP row (Context7) -> index 0; this lands BELOW the MCP header block,
	// which the legacy dead handler never resolved.
	y = labelLineY(t, out, "Context7")
	if y < 0 {
		t.Fatal("Context7 row not found")
	}
	ctx.app.configFieldIndex = 99
	screen.Update(clickAt(30, y))
	if ctx.app.configFieldIndex != 0 {
		t.Errorf("click on Context7 row: configFieldIndex = %d, want 0", ctx.app.configFieldIndex)
	}

	// A later MCP row (GitHub) -> index 2.
	y = labelLineY(t, out, "GitHub")
	if y < 0 {
		t.Fatal("GitHub row not found")
	}
	ctx.app.configFieldIndex = 99
	screen.Update(clickAt(30, y))
	if ctx.app.configFieldIndex != 2 {
		t.Errorf("click on GitHub row: configFieldIndex = %d, want 2", ctx.app.configFieldIndex)
	}
}
