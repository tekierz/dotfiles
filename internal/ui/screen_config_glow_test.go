package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestGlowManageFieldsExposeSevenHonestSettings(t *testing.T) {
	app := appForFields()
	fields := app.manageFieldsFor("glow")
	if len(fields) != 7 {
		t.Fatalf("fields=%d", len(fields))
	}
	if fields[0].kind != manageFieldOption || len(fields[0].options) != 8 || fields[0].str != &app.manageConfig.GlowStyle {
		t.Fatalf("style field=%+v", fields[0])
	}
	if fields[1].label != "Use Pager" || !strings.Contains(fields[1].description, "$PAGER") || len(fields[1].options) != 2 || fields[1].options[0] != "auto" || fields[1].options[1] != "never" {
		t.Fatalf("pager field=%+v", fields[1])
	}
	if fields[2].min != 0 || fields[2].max != math.MaxInt || fields[2].step != 1 {
		t.Fatalf("width bounds=%d..%d", fields[2].min, fields[2].max)
	}
	for i := 3; i < 7; i++ {
		if fields[i].kind != manageFieldToggle {
			t.Errorf("field %d kind=%v", i, fields[i].kind)
		}
	}
	app.manageConfig.GlowStyle = "/tmp/custom.json"
	app.manageConfig.GlowPager = "never"
	app.manageConfig.GlowWidth = 0
	if got := renderManageFieldLineBase(fields[0], true); !strings.Contains(got, "/tmp/custom.json") {
		t.Fatalf("custom Manage style hidden: %q", got)
	}
	if got := renderManageFieldLineBase(fields[1], true); !strings.Contains(got, "Disabled") {
		t.Fatalf("pager semantics hidden: %q", got)
	}
	if got := renderManageFieldLineBase(fields[2], true); !strings.Contains(got, "Auto (max 120; fallback 80)") {
		t.Fatalf("automatic width hidden: %q", got)
	}
}

func TestGlowManageWidthInlineEditorExactAndFailClosed(t *testing.T) {
	app := appForFields()
	field := app.manageFieldsFor("glow")[2]
	for _, value := range []string{"1", "7", "7001"} {
		app.manageStartEditing(field)
		app.manageEditValue = value
		if !app.manageCommitEditing() {
			t.Fatalf("commit %s failed", value)
		}
		if got := strconv.Itoa(app.manageConfig.GlowWidth); got != value {
			t.Fatalf("width=%s want %s", got, value)
		}
	}
	for _, value := range []string{"-1", "999999999999999999999999999999"} {
		before := app.manageConfig.GlowWidth
		app.manageStartEditing(field)
		app.manageEditValue = value
		if app.manageCommitEditing() {
			t.Fatalf("invalid %s committed", value)
		}
		if app.manageConfig.GlowWidth != before || !app.manageEditing {
			t.Fatalf("invalid edit changed state")
		}
		app.manageCancelEditing()
	}
	before := app.manageConfig.GlowWidth
	app.manageStartEditing(field)
	app.manageEditValue = "42"
	app.manageCancelEditing()
	if app.manageConfig.GlowWidth != before {
		t.Fatal("cancel changed width")
	}
}

func TestConfigGlowScreenShowsSevenFieldsAndCustomNativeValuesAt80x24(t *testing.T) {
	ctx := newDeepDiveContext(t)
	cfg := ctx.app.deepDiveConfig
	cfg.GlowStyle = "/tmp/custom glow.json"
	cfg.GlowPager = "auto"
	cfg.GlowWidth = 0
	cfg.GlowMouse = true
	cfg.GlowAll = true
	cfg.GlowShowLineNumbers = true
	cfg.GlowPreserveNewLines = true
	screen := NewConfigGlowScreen(ctx)
	out := screen.View(80, 24)
	if got := lipgloss.Height(out); got > 24 {
		t.Fatalf("80x24 view height=%d", got)
	}
	for _, want := range []string{"/tmp/custom glow.json", "Use Pager", "Enabled", "Auto (max 120; fallback 80)", "Mouse", "Show All Files", "Line Numbers", "Preserve Newlines", "navigate"} {
		if !strings.Contains(out, want) {
			t.Errorf("80x24 view missing %q:\n%s", want, out)
		}
	}
}

func TestConfigGlowWidthFocusAdvertisesExactEditorAt80x24(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.configFieldIndex = 2
	out := NewConfigGlowScreen(ctx).View(80, 24)
	if !strings.Contains(out, "e edit exact width") || !strings.Contains(out, "enter/esc back") {
		t.Fatalf("width help missing:\n%s", out)
	}
	if got := lipgloss.Height(out); got > 24 {
		t.Fatalf("height=%d", got)
	}
}

func TestConfigGlowLongReadOnlyCustomStyleDoesNotBreakCompactLayout(t *testing.T) {
	ctx := newDeepDiveContext(t)
	full := "/" + strings.Repeat("very-long-segment/", 50) + "style.json"
	ctx.app.deepDiveConfig.GlowStyle = full
	out := NewConfigGlowScreen(ctx).View(80, 24)
	for _, label := range []string{"Style", "Use Pager", "Width", "Mouse", "Show All Files", "Line Numbers", "Preserve Newlines", "navigate"} {
		if !strings.Contains(out, label) {
			t.Fatalf("long-style view missing %q", label)
		}
	}
	if lipgloss.Height(out) > 24 {
		t.Fatalf("long-style height=%d", lipgloss.Height(out))
	}
	if ctx.app.deepDiveConfig.GlowStyle != full {
		t.Fatal("display truncation changed stored style")
	}
}

func TestConfigGlowScreenAdjustsAllFieldsHonestly(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigGlowScreen(ctx)
	cfg := ctx.app.deepDiveConfig
	cfg.GlowStyle = "/tmp/custom.json"
	ctx.app.configFieldIndex = 0
	screen.Update(keyMsg("right"))
	if cfg.GlowStyle != "auto" {
		t.Fatalf("custom style replacement=%q", cfg.GlowStyle)
	}
	cfg.GlowPager = "auto"
	ctx.app.configFieldIndex = 1
	screen.Update(keyMsg(" "))
	if cfg.GlowPager != "never" {
		t.Fatalf("pager space=%q", cfg.GlowPager)
	}
	screen.Update(keyMsg("left"))
	if cfg.GlowPager != "auto" {
		t.Fatalf("pager left=%q", cfg.GlowPager)
	}
	ctx.app.configFieldIndex = 2
	for _, tc := range []struct {
		start int
		key   string
		want  int
	}{{7, "left", 6}, {995, "right", 996}, {0, "left", 0}, {math.MaxInt, "right", math.MaxInt}} {
		cfg.GlowWidth = tc.start
		screen.Update(keyMsg(tc.key))
		if cfg.GlowWidth != tc.want {
			t.Errorf("width %d %s=%d want %d", tc.start, tc.key, cfg.GlowWidth, tc.want)
		}
	}
	values := []*bool{&cfg.GlowMouse, &cfg.GlowAll, &cfg.GlowShowLineNumbers, &cfg.GlowPreserveNewLines}
	for i, value := range values {
		*value = false
		ctx.app.configFieldIndex = i + 3
		screen.Update(keyMsg("right"))
		if !*value {
			t.Errorf("right did not toggle field %d", i+3)
		}
		screen.Update(keyMsg(" "))
		if *value {
			t.Errorf("space did not toggle field %d", i+3)
		}
	}
	ctx.app.configFieldIndex = 6
	screen.Update(keyMsg("down"))
	if ctx.app.configFieldIndex != 6 {
		t.Fatalf("focus exceeded max: %d", ctx.app.configFieldIndex)
	}
}

func TestConfigGlowWidthInlineEditAndIsolation(t *testing.T) {
	ctx := newDeepDiveContext(t)
	screen := NewConfigGlowScreen(ctx)
	ctx.app.configFieldIndex = 2
	for _, value := range []string{"1", "7", "7001"} {
		screen.Update(keyMsg("e"))
		screen.editValue = value
		screen.editCursor = len(value)
		screen.Update(keyMsg("enter"))
		if got := strconv.Itoa(ctx.app.deepDiveConfig.GlowWidth); got != value {
			t.Fatalf("width=%s want %s", got, value)
		}
	}
	for _, value := range []string{"-1", "999999999999999999999999999999"} {
		before := ctx.app.deepDiveConfig.GlowWidth
		screen.Update(keyMsg("e"))
		screen.editValue = value
		screen.editCursor = len(value)
		screen.Update(keyMsg("enter"))
		if !screen.editing || ctx.app.deepDiveConfig.GlowWidth != before {
			t.Fatalf("invalid %s escaped editor", value)
		}
		screen.Update(keyMsg("esc"))
	}
	before := ctx.app.deepDiveConfig.GlowWidth
	screen.Update(keyMsg("e"))
	screen.editValue = "42"
	screen.Update(keyMsg("esc"))
	if ctx.app.deepDiveConfig.GlowWidth != before {
		t.Fatal("cancel changed width")
	}
	screen.Update(keyMsg("e"))
	screen.editValue = ""
	screen.editCursor = 0
	screen.Update(keyMsg("q"))
	if !screen.editing || screen.editValue != "q" {
		t.Fatalf("q escaped editor: editing=%v value=%q", screen.editing, screen.editValue)
	}
	focus := ctx.app.configFieldIndex
	screen.Update(clickAt(10, 10))
	if ctx.app.configFieldIndex != focus || !screen.editing {
		t.Fatal("mouse changed focus during edit")
	}
}
