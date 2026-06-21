package ui

import (
	"testing"
	"time"
)

// TestMainMenuClick verifies geometry-correct click-to-select on the main menu.
// Clicking the Nth menu row must select index N. The legacy anchor (startY+3,
// contentH=5+len) ignored the ContainerStyle border/padding and HelpStyle
// padding, selecting one row off.
func TestMainMenuClick(t *testing.T) {
	const w, h = 90, 40
	items := GetMainMenuItems()
	if len(items) == 0 {
		t.Skip("no main menu items")
	}

	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = w, h
	ctx.Width, ctx.Height = w, h
	screen := NewMainMenuScreen(ctx)
	out := screen.View(w, h)

	for i, item := range items {
		// Match the per-row Description (unique, on the same line as the name) so
		// the search isn't confused by the "Dotfiles Management" title etc.
		y := labelLineY(t, out, item.Description)
		if y < 0 {
			t.Fatalf("menu item %q not found in view", item.Description)
		}
		ctx.app.mainMenuIndex = 999
		screen.Update(clickAt(w/2, y))
		if ctx.app.mainMenuIndex != i {
			t.Errorf("click on %q (Y=%d): mainMenuIndex = %d, want %d", item.Name, y, ctx.app.mainMenuIndex, i)
		}
	}
}

// TestDeepDiveMenuClick verifies geometry-correct click-to-select on the deep
// dive menu, including rows below category headers (whose MarginTop blank line
// the legacy handler mishandled) and the Continue row.
func TestDeepDiveMenuClick(t *testing.T) {
	const w, h = 90, 60
	ctx := newDeepDiveContext(t)
	ctx.app.width, ctx.app.height = w, h
	ctx.Width, ctx.Height = w, h
	screen := NewDeepDiveMenuScreen(ctx)
	out := screen.View(w, h)

	items := GetFilteredDeepDiveMenuItems()
	if len(items) == 0 {
		t.Skip("no deep dive items")
	}

	for i, item := range items {
		y := labelLineY(t, out, item.Name)
		if y < 0 {
			t.Fatalf("deep dive item %q not found in view", item.Name)
		}
		ctx.app.deepDiveMenuIndex = 999
		// selectItem navigates away, but we only assert the index it set.
		screen.Update(clickAt(w/2, y))
		if ctx.app.deepDiveMenuIndex != i {
			t.Errorf("click on %q (Y=%d): deepDiveMenuIndex = %d, want %d", item.Name, y, ctx.app.deepDiveMenuIndex, i)
		}
	}

	// Continue row -> index len(items).
	y := labelLineY(t, out, "Continue to Installation")
	if y < 0 {
		t.Fatal("Continue row not found")
	}
	ctx.app.deepDiveMenuIndex = 999
	screen.Update(clickAt(w/2, y))
	if ctx.app.deepDiveMenuIndex != len(items) {
		t.Errorf("click on Continue (Y=%d): deepDiveMenuIndex = %d, want %d", y, ctx.app.deepDiveMenuIndex, len(items))
	}
}

// TestManageRightPaneLogGuard verifies that while the install-log view occupies
// the right pane (manageInstalling or installLogs present), a click in that
// region does NOT mutate the hidden settings fields.
func TestManageRightPaneLogGuard(t *testing.T) {
	ctx := newManageContext(t)
	ctx.app.manageIndex = 0
	ctx.app.managePane = managePaneSettings
	screen := NewManageScreen(ctx)
	_ = screen.View(ctx.Width, ctx.Height)

	layout := ctx.app.manageLayout()
	// A point inside the right (settings/log) list region.
	x := layout.rightX + 2
	y := layout.rightListY

	// Sanity: with no log active, a right-pane click DOES focus a field.
	ctx.app.installLogs = nil
	ctx.app.manageInstalling = false
	ctx.app.configFieldIndex = 99
	ctx.app.managePane = managePaneTools
	screen.Update(clickAt(x, y))
	if ctx.app.managePane != managePaneSettings {
		t.Skipf("right-pane click did not hit the fields region at this size; skipping (pane=%d)", ctx.app.managePane)
	}

	// Now activate the log view and assert a click is ignored.
	ctx.app.installLogs = []string{"installing...", "done"}
	ctx.app.configFieldIndex = 5
	ctx.app.managePane = managePaneTools
	screen.Update(clickAt(x, y))
	if ctx.app.configFieldIndex != 5 {
		t.Errorf("click in log region changed configFieldIndex to %d, want unchanged (5)", ctx.app.configFieldIndex)
	}
	if ctx.app.managePane == managePaneSettings {
		t.Errorf("click in log region switched to settings pane; want no field hit-test while log active")
	}
}

// TestWelcomeMouseSplit verifies the welcome toggle mirrors the View's layout:
// a Y-based split when the buttons are stacked (width < 78) and an X-based split
// when side-by-side. The legacy handler always used the X split, which is wrong
// in the narrow stacked layout.
func TestWelcomeMouseSplit(t *testing.T) {
	t.Run("wide: X split", func(t *testing.T) {
		const w, h = 100, 40
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		screen := NewWelcomeScreen(ctx)

		ctx.app.deepDive = true
		screen.Update(clickAt(w/4, h*3/4)) // left half, lower area -> Quick (false)
		if ctx.app.deepDive {
			t.Errorf("wide left click should set deepDive=false")
		}
		ctx.app.deepDive = false
		screen.Update(clickAt(w*3/4, h*3/4)) // right half -> Deep Dive (true)
		if !ctx.app.deepDive {
			t.Errorf("wide right click should set deepDive=true")
		}
	})

	t.Run("narrow: Y split", func(t *testing.T) {
		const w, h = 60, 40 // width < 78 -> stacked
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		screen := NewWelcomeScreen(ctx)

		centerY := h / 2
		splitY := centerY + (h-centerY)/2
		// Upper button area (between centerY and splitY) -> Quick (false).
		ctx.app.deepDive = true
		screen.Update(clickAt(w/4, centerY+1)) // left X, but should be ignored for narrow
		if ctx.app.deepDive {
			t.Errorf("narrow upper-button click should set deepDive=false regardless of X")
		}
		// Lower button area (>= splitY) -> Deep Dive (true), even on the left X.
		ctx.app.deepDive = false
		screen.Update(clickAt(w/4, splitY+1))
		if !ctx.app.deepDive {
			t.Errorf("narrow lower-button click should set deepDive=true regardless of X")
		}
	})
}

// TestHotkeysItemsPaneWidthCap verifies the items-pane hit region matches the
// rendered (capped) panel width. On a very wide terminal the panel is capped at
// 100 cols; a click beyond rightX+cappedWidth must NOT register as inRightList
// (the legacy uncapped rightW made the empty space to the right clickable).
func TestHotkeysItemsPaneWidthCap(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = 300, 50 // very wide -> rightW would exceed 100
	ctx.Width, ctx.Height = 300, 50

	layout := ctx.app.hotkeysLayout()
	if layout.rightW > 100 {
		t.Fatalf("rightW = %d, want capped at <= 100", layout.rightW)
	}

	y := layout.rightListY // a row inside the items list
	// Just inside the capped panel: should be in the right list.
	if !layout.inRightList(layout.rightX+layout.rightW-1, y) {
		t.Errorf("click at right edge of capped panel should be inRightList")
	}
	// Just past the capped panel (where empty space begins): must be excluded.
	if layout.inRightList(layout.rightX+layout.rightW, y) {
		t.Errorf("click past capped panel width should NOT be inRightList")
	}
	// A far-right click (where the legacy uncapped region would have reached).
	if layout.inRightList(290, y) {
		t.Errorf("far-right click should NOT be inRightList after width cap")
	}
}

// TestUsersSettingsClick verifies C19: clicking a settings-pane field accounts
// for the extra description line drawn under the currently-selected field. With
// field 0 selected (and a non-empty description), the renderer inserts a
// description line after field 0, shifting fields 1+ down one row. A click on
// field 1's rendered row must still select field 1 (the legacy handler selected
// field 2).
func TestUsersSettingsClick(t *testing.T) {
	const w, h = 90, 30
	const firstRowY = usersTabBarRows + usersHeaderRows // 3

	setup := func(t *testing.T) (*usersScreen, *App) {
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		ctx.app.usersItems = []userItem{
			{name: "alice", theme: "dracula", navStyle: "emacs", keyboard: "macos", isActive: true},
		}
		ctx.app.usersIndex = 0
		ctx.app.usersPane = usersPaneSettings
		return NewUsersScreen(ctx), ctx.app
	}

	rightX := w/3 + 1 // inside the right (settings) pane

	t.Run("no field selected above: rows map 1:1", func(t *testing.T) {
		screen, a := setup(t)
		a.usersFieldIndex = 0
		fields := a.getUserFields()
		// With field 0 selected, its description line sits at firstRowY+1, so
		// field 1 renders at firstRowY+2. (Field 0 itself is still at firstRowY.)
		_ = fields
		// Click field 0's own row -> field 0.
		screen.Update(clickAt(rightX, firstRowY))
		if a.usersFieldIndex != 0 {
			t.Errorf("click field0 row: usersFieldIndex = %d, want 0", a.usersFieldIndex)
		}
	})

	t.Run("field 0 selected: click field 1 row selects field 1", func(t *testing.T) {
		screen, a := setup(t)
		a.usersFieldIndex = 0
		// Field 1 renders at firstRowY + 2 (field0 row + its description line).
		screen.Update(clickAt(rightX, firstRowY+2))
		if a.usersFieldIndex != 1 {
			t.Errorf("click field1 row (Y=%d): usersFieldIndex = %d, want 1", firstRowY+2, a.usersFieldIndex)
		}
	})

	t.Run("field 0 selected: click field 2 row selects field 2", func(t *testing.T) {
		screen, a := setup(t)
		a.usersFieldIndex = 0
		// Field 2 renders at firstRowY + 3.
		screen.Update(clickAt(rightX, firstRowY+3))
		if a.usersFieldIndex != 2 {
			t.Errorf("click field2 row (Y=%d): usersFieldIndex = %d, want 2", firstRowY+3, a.usersFieldIndex)
		}
	})

	t.Run("click the description line keeps selection", func(t *testing.T) {
		screen, a := setup(t)
		a.usersFieldIndex = 0
		// The inserted description line is at firstRowY+1.
		screen.Update(clickAt(rightX, firstRowY+1))
		if a.usersFieldIndex != 0 {
			t.Errorf("click description line (Y=%d): usersFieldIndex = %d, want 0", firstRowY+1, a.usersFieldIndex)
		}
	})
}

// TestThemePickerClick verifies geometry-correct click-to-select on the theme
// picker across widths (including a non-80 width and the wide preview layout).
// Clicking the rendered row of the Nth theme must set themeIndex to N. The
// legacy handler underestimated the container height (len(themes)+6) and used a
// hardcoded width, shifting every row by ~2-3 and making the top themes
// unreachable (C20).
func TestThemePickerClick(t *testing.T) {
	const h = 50

	widths := []int{70, 100, 120}
	for _, w := range widths {
		w := w
		t.Run("width", func(t *testing.T) {
			for _, idx := range []int{0, 1, 5, len(themes) - 1} {
				ctx := newGoldenContext(t)
				ctx.app.width, ctx.app.height = w, h
				ctx.Width, ctx.Height = w, h
				ctx.app.themeIndex = 0
				screen := NewThemePickerScreen(ctx)
				out := screen.View(w, h)

				y := labelLineY(t, out, themes[idx].name)
				if y < 0 {
					t.Fatalf("w=%d: theme %q not found in view", w, themes[idx].name)
				}
				ctx.app.themeIndex = 999
				screen.Update(clickAt(w/2, y))
				if ctx.app.themeIndex != idx {
					t.Errorf("w=%d: click on %q (Y=%d) => themeIndex %d, want %d", w, themes[idx].name, y, ctx.app.themeIndex, idx)
				}
			}
		})
	}

	t.Run("click left of container selects nothing", func(t *testing.T) {
		const w = 100
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		screen := NewThemePickerScreen(ctx)
		out := screen.View(w, h)
		y := labelLineY(t, out, themes[2].name)
		if y < 0 {
			t.Fatal("theme row not found")
		}
		ctx.app.themeIndex = 4
		screen.Update(clickAt(0, y))
		if ctx.app.themeIndex != 4 {
			t.Errorf("click left of container changed themeIndex to %d, want unchanged (4)", ctx.app.themeIndex)
		}
	})
}

// TestConfigListClick verifies geometry-correct click-to-select on the
// configListNav screens (clitools, cliutilities, macapps, guiapps, utilities).
// Clicking the rendered row of the Nth item must select index N. The legacy
// math (contentHeight+4 anchor, no box border/padding, no X-bounds) selected
// the wrong row.
func TestConfigListClick(t *testing.T) {
	const w, h = 90, 50

	type itemCase struct {
		label string
		idx   int
	}
	cases := []struct {
		name  string
		build func(ctx *ScreenContext) ScreenHandler
		index func(a *App) int
		want  []itemCase
	}{
		{
			name:  "macapps",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigMacAppsScreen(ctx) },
			index: func(a *App) int { return a.macAppIndex },
			want:  []itemCase{{"Rectangle", 0}, {"Raycast", 1}, {"Karabiner", 6}, {"AppCleaner", 9}},
		},
		{
			name:  "cliutilities",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigCLIUtilitiesScreen(ctx) },
			index: func(a *App) int { return a.cliUtilityIndex },
			want:  []itemCase{{"cat with syntax", 0}, {"Modern ls", 1}, {"File system watcher", 6}},
		},
		{
			name:  "guiapps",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigGUIAppsScreen(ctx) },
			index: func(a *App) int { return a.guiAppIndex },
			want:  []itemCase{{"Zen Browser", 0}, {"Cursor", 1}, {"OBS Studio", 5}},
		},
		{
			name:  "utilities",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigUtilitiesScreen(ctx) },
			index: func(a *App) int { return a.utilityIndex },
			want:  []itemCase{{"Hotkey reference", 0}, {"Keep system awake", 1}, {"SSH config", 2}},
		},
		{
			name:  "clitools",
			build: func(ctx *ScreenContext) ScreenHandler { return NewConfigCLIToolsScreen(ctx) },
			index: func(a *App) int { return a.cliToolIndex },
			want:  []itemCase{{"LazyGit", 0}, {"LazyDocker", 1}, {"btop", 2}, {"Glow", 3}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			screen := tc.build(ctx)
			out := screen.View(w, h)
			for _, ic := range tc.want {
				y := labelLineY(t, out, ic.label)
				if y < 0 {
					t.Fatalf("%s: label %q not found", tc.name, ic.label)
				}
				screen.Update(clickAt(w/2, y))
				if got := tc.index(ctx.app); got != ic.idx {
					t.Errorf("%s: click on %q (Y=%d) => index %d, want %d", tc.name, ic.label, y, got, ic.idx)
				}
			}
		})
	}

	t.Run("clitools claude-code row selects nothing", func(t *testing.T) {
		ctx := newDeepDiveContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		screen := NewConfigCLIToolsScreen(ctx)
		out := screen.View(w, h)
		y := labelLineY(t, out, "AI-powered coding")
		if y < 0 {
			t.Fatal("claude-code row not found")
		}
		ctx.app.cliToolIndex = 1
		screen.Update(clickAt(w/2, y))
		if ctx.app.cliToolIndex != 1 {
			t.Errorf("click on claude-code context row changed index to %d, want unchanged (1)", ctx.app.cliToolIndex)
		}
	})

	t.Run("click left of box selects nothing", func(t *testing.T) {
		ctx := newDeepDiveContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		screen := NewConfigMacAppsScreen(ctx)
		out := screen.View(w, h)
		y := labelLineY(t, out, "Raycast")
		if y < 0 {
			t.Fatal("Raycast row not found")
		}
		ctx.app.macAppIndex = 3
		screen.Update(clickAt(0, y))
		if ctx.app.macAppIndex != 3 {
			t.Errorf("click left of box changed index to %d, want unchanged (3)", ctx.app.macAppIndex)
		}
	})
}

// TestBackupsClickRow verifies geometry-correct click-to-select on the backups
// list. Clicking the rendered row of the Nth backup must set backupIndex to N.
// The legacy handler undercounted the box top border (listStartY=7/8), so a
// click on the first visible backup selected the second and the last backup was
// unreachable.
func TestBackupsClickRow(t *testing.T) {
	const w, h = 80, 40

	backups := []BackupEntry{
		{Name: "2026-06-18_alpha", Timestamp: time.Date(2026, 6, 18, 10, 30, 0, 0, time.UTC), FileCount: 5, Size: 2048, Path: "/tmp/backups/alpha"},
		{Name: "2026-06-17_bravo", Timestamp: time.Date(2026, 6, 17, 9, 15, 0, 0, time.UTC), FileCount: 3, Size: 1024, Path: "/tmp/backups/bravo"},
		{Name: "2026-06-16_gamma", Timestamp: time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC), FileCount: 7, Size: 4096, Path: "/tmp/backups/gamma"},
	}

	t.Run("no status line", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		ctx.app.backupsLoading = false
		ctx.app.backupsLoaded = true
		ctx.app.backupStatus = ""
		ctx.app.backups = backups

		screen := NewBackupsScreen(ctx)
		out := screen.View(w, h)

		for i, b := range backups {
			y := labelLineY(t, out, b.Name)
			if y < 0 {
				t.Fatalf("backup %q not found in view", b.Name)
			}
			ctx.app.backupIndex = 999
			screen.Update(clickAt(w/2, y))
			if ctx.app.backupIndex != i {
				t.Errorf("click on %q (Y=%d): backupIndex = %d, want %d", b.Name, y, ctx.app.backupIndex, i)
			}
		}
	})

	t.Run("with status line", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		ctx.app.backupsLoading = false
		ctx.app.backupsLoaded = true
		ctx.app.backupStatus = "Created backup: foo"
		ctx.app.backups = backups

		screen := NewBackupsScreen(ctx)
		out := screen.View(w, h)

		for i, b := range backups {
			y := labelLineY(t, out, b.Name)
			if y < 0 {
				t.Fatalf("backup %q not found in view", b.Name)
			}
			ctx.app.backupIndex = 999
			screen.Update(clickAt(w/2, y))
			if ctx.app.backupIndex != i {
				t.Errorf("click on %q (Y=%d): backupIndex = %d, want %d", b.Name, y, ctx.app.backupIndex, i)
			}
		}
	})

	// X-bounds guard (FIX 4): the list box is LEFT-aligned at X=0 (the full-width
	// tab bar pins the block to X=0), so a click at X=0 on a real backup row MUST
	// select it, and a click at X >= boxOuterW (in the empty strip to the right)
	// must NOT. The legacy handler assumed a centered box, so it dropped the X=0
	// click and wrongly accepted the far-right click; this subtest would catch
	// that regression.
	t.Run("X-bounds left-aligned at zero", func(t *testing.T) {
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		ctx.app.backupsLoading = false
		ctx.app.backupsLoaded = true
		ctx.app.backups = backups

		screen := NewBackupsScreen(ctx)
		out := screen.View(w, h)
		// Target the SECOND backup row so a successful select is distinguishable
		// from the default index (0).
		y := labelLineY(t, out, backups[1].Name)
		if y < 0 {
			t.Fatal("backup row not found")
		}

		// Click at the absolute left edge (X=0): must select the row under it.
		ctx.app.backupIndex = 999
		screen.Update(clickAt(0, y))
		if ctx.app.backupIndex != 1 {
			t.Errorf("click at X=0 on row 1 (Y=%d): backupIndex = %d, want 1 (box is left-aligned)", y, ctx.app.backupIndex)
		}

		// Click far to the right, outside the box (X >= boxOuterW): must NOT change
		// the selection. boxOuterW = min(92, max(44, width-8)) = 72 at width=80.
		boxOuterW := min(92, maxInt(44, w-8))
		ctx.app.backupIndex = 0
		screen.Update(clickAt(boxOuterW+2, y))
		if ctx.app.backupIndex != 0 {
			t.Errorf("click at X=%d (right of box) changed backupIndex to %d, want unchanged (0)", boxOuterW+2, ctx.app.backupIndex)
		}
	})
}
