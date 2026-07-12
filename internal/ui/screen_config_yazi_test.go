package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestManageYaziFieldsRespectIndependentMainAndKeymapOwnership(t *testing.T) {
	const mainReason = "arbitrary native Yazi TOML is read-only in this release"
	app := &App{
		manageConfig: NewManageConfig(),
		nativeConfigState: NativeManageConfigState{Yazi: tools.YaziConfigImport{
			Main: tools.YaziFileObservation{
				Kind:           tools.YaziFileKindMain,
				Exists:         true,
				Ownership:      tools.YaziOwnershipNative,
				ReadOnlyReason: mainReason,
			},
			Keymap: tools.YaziFileObservation{
				Kind:      tools.YaziFileKindKeymap,
				Exists:    true,
				Ownership: tools.YaziOwnershipExactCurrent,
			},
		}},
	}

	fields := app.manageFieldsFor("yazi")
	wantMain := map[string]bool{
		"hidden": false, "preview_mode": false, "sort_by": false,
		"sort_rev": false, "linemode": false, "scrolloff": false,
	}
	if len(fields) != len(wantMain)+1 {
		t.Fatalf("Yazi field count = %d, want %d", len(fields), len(wantMain)+1)
	}
	for _, field := range fields {
		switch field.key {
		case "keymap":
			if field.readOnlyReason != "" {
				t.Errorf("exact-current keymap field is read-only: %q", field.readOnlyReason)
			}
		default:
			if _, ok := wantMain[field.key]; !ok {
				t.Errorf("unexpected Yazi field key %q", field.key)
				continue
			}
			wantMain[field.key] = true
			if field.readOnlyReason != mainReason {
				t.Errorf("Yazi main field %q read-only reason = %q, want %q", field.key, field.readOnlyReason, mainReason)
			}
		}
	}
	for key, found := range wantMain {
		if !found {
			t.Errorf("Yazi main field %q was not exposed", key)
		}
	}
}

func TestManageYaziFieldsReadOnlyPolicyMatrix(t *testing.T) {
	mainKeys := map[string]bool{
		"hidden": true, "preview_mode": true, "sort_by": true,
		"sort_rev": true, "linemode": true, "scrolloff": true,
	}
	allKeys := map[string]bool{"keymap": true}
	for key := range mainKeys {
		allKeys[key] = true
	}
	current := func(kind tools.YaziFileKind) tools.YaziFileObservation {
		return tools.YaziFileObservation{Kind: kind, Exists: true, Ownership: tools.YaziOwnershipExactCurrent}
	}
	missing := func(kind tools.YaziFileKind) tools.YaziFileObservation {
		return tools.YaziFileObservation{Kind: kind, Ownership: tools.YaziOwnershipMissing}
	}

	tests := []struct {
		name      string
		state     NativeManageConfigState
		want      map[string]string
		wantEmpty map[string]bool
	}{
		{
			name: "native keymap blocks only keymap",
			state: NativeManageConfigState{Yazi: tools.YaziConfigImport{
				Main: current(tools.YaziFileKindMain),
				Keymap: tools.YaziFileObservation{
					Kind: tools.YaziFileKindKeymap, Exists: true, Ownership: tools.YaziOwnershipNative,
					ReadOnlyReason: "arbitrary native Yazi TOML is read-only in this release",
				},
			}},
			want:      map[string]string{"keymap": "arbitrary native Yazi TOML is read-only in this release"},
			wantEmpty: mainKeys,
		},
		{
			name: "both missing remain editable",
			state: NativeManageConfigState{Yazi: tools.YaziConfigImport{
				Main: missing(tools.YaziFileKindMain), Keymap: missing(tools.YaziFileKindKeymap),
			}},
			wantEmpty: allKeys,
		},
		{
			name: "preference error blocks every field",
			state: NativeManageConfigState{
				PreferenceError: "invalid manage.json",
				Yazi:            tools.YaziConfigImport{Main: current(tools.YaziFileKindMain), Keymap: current(tools.YaziFileKindKeymap)},
			},
			want: reasonsForKeys(allKeys, "saved management preferences could not be read safely: invalid manage.json"),
		},
		{
			name: "Yazi import error blocks every field",
			state: NativeManageConfigState{
				YaziError: "resolver failed",
			},
			want: reasonsForKeys(allKeys, "native Yazi configuration could not be imported safely: resolver failed"),
		},
		{
			name: "preference error wins over Yazi resolver error",
			state: NativeManageConfigState{
				PreferenceError: "invalid manage.json",
				YaziError:       "resolver failed",
			},
			want: reasonsForKeys(allKeys, "saved management preferences could not be read safely: invalid manage.json"),
		},
		{
			name: "preference error wins over per-file errors",
			state: NativeManageConfigState{
				PreferenceError: "invalid manage.json",
				Yazi: tools.YaziConfigImport{
					Main: tools.YaziFileObservation{Kind: tools.YaziFileKindMain, Exists: true, Ownership: tools.YaziOwnershipMalformed,
						Error: "parse failed", ReadOnlyReason: "malformed or unreadable Yazi TOML is read-only"},
					Keymap: tools.YaziFileObservation{Kind: tools.YaziFileKindKeymap, Exists: true, Ownership: tools.YaziOwnershipNative,
						ReadOnlyReason: "arbitrary native Yazi TOML is read-only in this release"},
				},
			},
			want: reasonsForKeys(allKeys, "saved management preferences could not be read safely: invalid manage.json"),
		},
		{
			name: "external resolved directory blocks every field",
			state: NativeManageConfigState{Yazi: tools.YaziConfigImport{
				Main: tools.YaziFileObservation{Kind: tools.YaziFileKindMain, Exists: true, Ownership: tools.YaziOwnershipNative, External: true,
					ReadOnlyReason: "active Yazi config is outside HOME or HOME containment cannot be verified"},
				Keymap: tools.YaziFileObservation{Kind: tools.YaziFileKindKeymap, Exists: true, Ownership: tools.YaziOwnershipNative, External: true,
					ReadOnlyReason: "active Yazi config is outside HOME or HOME containment cannot be verified"},
			}},
			want: reasonsForKeys(allKeys, "active Yazi config is outside HOME or HOME containment cannot be verified"),
		},
	}
	for _, ownershipCase := range []struct {
		name        string
		observation tools.YaziFileObservation
	}{
		{
			name: "historical main blocks only main fields",
			observation: tools.YaziFileObservation{Kind: tools.YaziFileKindMain, Exists: true, Ownership: tools.YaziOwnershipExactHistorical,
				ReadOnlyReason: "exact historical dotfiles Yazi config is read-only until explicitly migrated"},
		},
		{
			name: "malformed main blocks only main fields",
			observation: tools.YaziFileObservation{Kind: tools.YaziFileKindMain, Exists: true, Ownership: tools.YaziOwnershipMalformed,
				Error: "parse failed", ReadOnlyReason: "malformed or unreadable Yazi TOML is read-only"},
		},
	} {
		tests = append(tests, struct {
			name      string
			state     NativeManageConfigState
			want      map[string]string
			wantEmpty map[string]bool
		}{
			name: ownershipCase.name,
			state: NativeManageConfigState{Yazi: tools.YaziConfigImport{
				Main: ownershipCase.observation, Keymap: current(tools.YaziFileKindKeymap),
			}},
			want:      reasonsForKeys(mainKeys, ownershipCase.observation.ReadOnlyReason),
			wantEmpty: map[string]bool{"keymap": true},
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &App{manageConfig: NewManageConfig(), nativeConfigState: test.state}
			fields := app.manageFieldsFor("yazi")
			if len(fields) != len(allKeys) {
				t.Fatalf("Yazi field count = %d, want %d", len(fields), len(allKeys))
			}
			seen := make(map[string]bool, len(fields))
			for _, field := range fields {
				if !allKeys[field.key] {
					t.Errorf("unexpected Yazi field key %q", field.key)
					continue
				}
				seen[field.key] = true
				if reason, ok := test.want[field.key]; ok && field.readOnlyReason != reason {
					t.Errorf("field %q read-only reason = %q, want %q", field.key, field.readOnlyReason, reason)
				}
				if test.wantEmpty[field.key] && field.readOnlyReason != "" {
					t.Errorf("field %q unexpectedly read-only: %q", field.key, field.readOnlyReason)
				}
			}
			for key := range allKeys {
				if !seen[key] {
					t.Errorf("Yazi field %q was not exposed", key)
				}
			}
		})
	}
}

func reasonsForKeys(keys map[string]bool, reason string) map[string]string {
	reasons := make(map[string]string, len(keys))
	for key := range keys {
		reasons[key] = reason
	}
	return reasons
}

func TestManageYaziReadOnlyFieldsBlockEveryKeyboardMutationRoute(t *testing.T) {
	const mainReason = "arbitrary native Yazi TOML is read-only in this release"
	const keymapReason = "arbitrary native Yazi TOML is read-only in this release"
	tests := []struct {
		name      string
		fieldKey  string
		keys      []string
		blockMain bool
		value     func(*ManageConfig) any
	}{
		{"keymap option", "keymap", []string{"left", "right", "h", "l", " ", "enter"}, false, func(cfg *ManageConfig) any { return cfg.YaziKeymap }},
		{"main option", "sort_by", []string{"left", "right", "h", "l", " ", "enter"}, true, func(cfg *ManageConfig) any { return cfg.YaziSortBy }},
		{"main toggle", "hidden", []string{" ", "enter"}, true, func(cfg *ManageConfig) any { return cfg.YaziShowHidden }},
		{"main number", "scrolloff", []string{"left", "right", "h", "l", " ", "enter"}, true, func(cfg *ManageConfig) any { return cfg.YaziScrollOff }},
	}
	for _, test := range tests {
		for _, key := range test.keys {
			t.Run(test.name+"/"+key, func(t *testing.T) {
				ctx, screen := newManageYaziPolicyScreen(t, test.blockMain, !test.blockMain)
				fieldIndex := manageYaziFieldIndex(t, ctx.app, test.fieldKey)
				ctx.app.configFieldIndex = fieldIndex
				before := test.value(ctx.app.manageConfig)
				screen.Update(keyMsg(key))
				if got := test.value(ctx.app.manageConfig); got != before {
					t.Fatalf("blocked %s route %q mutated value from %v to %v", test.fieldKey, key, before, got)
				}
				wantReason := keymapReason
				if test.blockMain {
					wantReason = mainReason
				}
				if !strings.Contains(ctx.app.manageStatus, wantReason) {
					t.Fatalf("blocked %s route %q status = %q, want reason %q", test.fieldKey, key, ctx.app.manageStatus, wantReason)
				}
				if test.fieldKey == "scrolloff" && key == "enter" && ctx.app.manageEditing {
					t.Fatal("blocked scrolloff Enter started number editor")
				}
			})
		}
	}
}

func TestManageYaziReadOnlyFieldsBlockMouseMutationRoutes(t *testing.T) {
	const reason = "arbitrary native Yazi TOML is read-only in this release"
	tests := []struct {
		fieldKey  string
		blockMain bool
		value     func(*ManageConfig) any
	}{
		{"keymap", false, func(cfg *ManageConfig) any { return cfg.YaziKeymap }},
		{"hidden", true, func(cfg *ManageConfig) any { return cfg.YaziShowHidden }},
		{"sort_by", true, func(cfg *ManageConfig) any { return cfg.YaziSortBy }},
		{"scrolloff", true, func(cfg *ManageConfig) any { return cfg.YaziScrollOff }},
	}
	for _, test := range tests {
		t.Run(test.fieldKey, func(t *testing.T) {
			ctx, screen := newManageYaziPolicyScreen(t, test.blockMain, !test.blockMain)
			fieldIndex := manageYaziFieldIndex(t, ctx.app, test.fieldKey)
			before := test.value(ctx.app.manageConfig)
			_ = screen.View(ctx.Width, ctx.Height)
			layout := ctx.app.manageLayout()
			screen.Update(clickAt(layout.rightX+layout.rightW-2, layout.rightListY+fieldIndex))
			if ctx.app.configFieldIndex != fieldIndex {
				t.Fatalf("mouse focused field %d, want %d", ctx.app.configFieldIndex, fieldIndex)
			}
			if got := test.value(ctx.app.manageConfig); got != before {
				t.Fatalf("blocked mouse route mutated %s from %v to %v", test.fieldKey, before, got)
			}
			if !strings.Contains(ctx.app.manageStatus, reason) {
				t.Fatalf("blocked mouse status = %q, want reason %q", ctx.app.manageStatus, reason)
			}
		})
	}
}

func TestManageYaziWritableKeyboardAndMouseRoutesStillMutate(t *testing.T) {
	ctx, screen := newManageYaziPolicyScreen(t, false, false)
	keymapIndex := manageYaziFieldIndex(t, ctx.app, "keymap")
	ctx.app.configFieldIndex = keymapIndex
	keymapBefore := ctx.app.manageConfig.YaziKeymap
	screen.Update(keyMsg("right"))
	if ctx.app.manageConfig.YaziKeymap == keymapBefore {
		t.Fatal("writable keymap option did not change by keyboard")
	}

	hiddenIndex := manageYaziFieldIndex(t, ctx.app, "hidden")
	ctx.app.configFieldIndex = hiddenIndex
	hiddenBefore := ctx.app.manageConfig.YaziShowHidden
	screen.Update(keyMsg(" "))
	if ctx.app.manageConfig.YaziShowHidden == hiddenBefore {
		t.Fatal("writable hidden toggle did not change by keyboard")
	}

	scrollIndex := manageYaziFieldIndex(t, ctx.app, "scrolloff")
	ctx.app.configFieldIndex = scrollIndex
	scrollBefore := ctx.app.manageConfig.YaziScrollOff
	screen.Update(keyMsg("right"))
	if ctx.app.manageConfig.YaziScrollOff == scrollBefore {
		t.Fatal("writable scrolloff number did not change by keyboard")
	}

	sortIndex := manageYaziFieldIndex(t, ctx.app, "sort_by")
	sortBefore := ctx.app.manageConfig.YaziSortBy
	_ = screen.View(ctx.Width, ctx.Height)
	layout := ctx.app.manageLayout()
	screen.Update(clickAt(layout.rightX+layout.rightW-2, layout.rightListY+sortIndex))
	if ctx.app.configFieldIndex != sortIndex || ctx.app.manageConfig.YaziSortBy == sortBefore {
		t.Fatalf("writable mouse option focused=%d value=%q before=%q", ctx.app.configFieldIndex, ctx.app.manageConfig.YaziSortBy, sortBefore)
	}
}

func TestManageYaziWritableEnterRoutesRemainDistinct(t *testing.T) {
	t.Run("keymap option cycles", func(t *testing.T) {
		ctx, screen := newManageYaziPolicyScreen(t, false, false)
		ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "keymap")
		before := ctx.app.manageConfig.YaziKeymap
		screen.Update(keyMsg("enter"))
		if ctx.app.manageConfig.YaziKeymap == before || ctx.app.manageEditing || ctx.app.manageStatus != "" {
			t.Fatalf("writable keymap Enter value=%q before=%q editing=%t status=%q", ctx.app.manageConfig.YaziKeymap, before, ctx.app.manageEditing, ctx.app.manageStatus)
		}
	})

	t.Run("hidden toggle toggles", func(t *testing.T) {
		ctx, screen := newManageYaziPolicyScreen(t, false, false)
		ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "hidden")
		before := ctx.app.manageConfig.YaziShowHidden
		screen.Update(keyMsg("enter"))
		if ctx.app.manageConfig.YaziShowHidden == before || ctx.app.manageEditing || ctx.app.manageStatus != "" {
			t.Fatalf("writable hidden Enter value=%t before=%t editing=%t status=%q", ctx.app.manageConfig.YaziShowHidden, before, ctx.app.manageEditing, ctx.app.manageStatus)
		}
	})

	t.Run("scrolloff number starts exact editor", func(t *testing.T) {
		ctx, screen := newManageYaziPolicyScreen(t, false, false)
		ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "scrolloff")
		screen.Update(keyMsg("enter"))
		if !ctx.app.manageEditing || ctx.app.manageEditNumber != &ctx.app.manageConfig.YaziScrollOff || ctx.app.manageStatus != "" {
			t.Fatalf("writable scrolloff Enter editing=%t number=%p want=%p status=%q", ctx.app.manageEditing, ctx.app.manageEditNumber, &ctx.app.manageConfig.YaziScrollOff, ctx.app.manageStatus)
		}
	})
}

func newManageYaziPolicyScreen(t *testing.T, blockMain, blockKeymap bool) (*ScreenContext, *manageScreen) {
	t.Helper()
	ctx := newManageContext(t)
	const width, height = 100, 40
	ctx.Width, ctx.Height = width, height
	ctx.app.width, ctx.app.height = width, height
	ctx.app.managePane = managePaneSettings
	foundYazi := false
	for index, item := range ctx.app.manageItems() {
		if item.id == "yazi" {
			ctx.app.manageIndex = index
			foundYazi = true
			break
		}
	}
	if !foundYazi {
		t.Fatal("Yazi Manage item not found")
	}
	main := tools.YaziFileObservation{Kind: tools.YaziFileKindMain, Exists: true, Ownership: tools.YaziOwnershipExactCurrent}
	keymap := tools.YaziFileObservation{Kind: tools.YaziFileKindKeymap, Exists: true, Ownership: tools.YaziOwnershipExactCurrent}
	if blockMain {
		main.Ownership = tools.YaziOwnershipNative
		main.ReadOnlyReason = "arbitrary native Yazi TOML is read-only in this release"
	}
	if blockKeymap {
		keymap.Ownership = tools.YaziOwnershipNative
		keymap.ReadOnlyReason = "arbitrary native Yazi TOML is read-only in this release"
	}
	ctx.app.nativeConfigState.Yazi = tools.YaziConfigImport{Main: main, Keymap: keymap}
	return ctx, NewManageScreen(ctx)
}

func manageYaziFieldIndex(t *testing.T, app *App, key string) int {
	t.Helper()
	for index, field := range app.manageFieldsFor("yazi") {
		if field.key == key {
			return index
		}
	}
	t.Fatalf("Yazi field %q not found", key)
	return -1
}

func TestStandaloneYaziPerFileKeyboardReadOnlyPolicy(t *testing.T) {
	const reason = "arbitrary native Yazi TOML is read-only in this release"
	mutationCases := []struct {
		name       string
		index      int
		keys       []string
		nativeKind tools.YaziFileKind
	}{
		{name: "keymap", index: 0, keys: []string{"left", "right", "h", "l"}, nativeKind: tools.YaziFileKindKeymap},
		{name: "show hidden", index: 1, keys: []string{" "}, nativeKind: tools.YaziFileKindMain},
		{name: "preview mode", index: 2, keys: []string{"left", "right", "h", "l"}, nativeKind: tools.YaziFileKindMain},
		{name: "sort by", index: 3, keys: []string{"left", "right", "h", "l"}, nativeKind: tools.YaziFileKindMain},
		{name: "reverse sort", index: 4, keys: []string{" "}, nativeKind: tools.YaziFileKindMain},
		{name: "line metadata", index: 5, keys: []string{"left", "right", "h", "l"}, nativeKind: tools.YaziFileKindMain},
		{name: "scroll offset", index: 6, keys: []string{"left", "right", "h", "l"}, nativeKind: tools.YaziFileKindMain},
	}
	for _, test := range mutationCases {
		for _, key := range test.keys {
			t.Run(fmt.Sprintf("native %s/%s", test.name, key), func(t *testing.T) {
				main := yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipExactCurrent, "")
				keymap := yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipExactCurrent, "")
				if test.nativeKind == tools.YaziFileKindMain {
					main = yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipNative, reason)
				} else {
					keymap = yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipNative, reason)
				}
				ctx, screen := newStandaloneYaziPolicyScreen(t, main, keymap)
				ctx.app.configFieldIndex = test.index
				before := yaziConfigFrom(*ctx.app.deepDiveConfig)
				screen.Update(keyMsg(key))
				if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
					t.Fatalf("blocked field %d route %q mutated from %+v to %+v", test.index, key, before, got)
				}
				if view := screen.View(ctx.Width, ctx.Height); !strings.Contains(normalizedYaziVisibleText(view), normalizedYaziVisibleText(reason)) {
					t.Fatalf("blocked focused field %d omitted reason %q:\n%s", test.index, reason, view)
				}
			})
		}
	}

	t.Run("exact-current keymap remains writable", func(t *testing.T) {
		ctx, screen := newStandaloneYaziPolicyScreen(t, yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipNative, reason), yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipExactCurrent, ""))
		ctx.app.configFieldIndex = 0
		before := ctx.app.deepDiveConfig.YaziKeymap
		screen.Update(keyMsg("right"))
		if ctx.app.deepDiveConfig.YaziKeymap == before {
			t.Fatal("exact-current keymap did not mutate")
		}
	})

	t.Run("exact-current main remains writable", func(t *testing.T) {
		ctx, screen := newStandaloneYaziPolicyScreen(t, yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipExactCurrent, ""), yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipNative, reason))
		ctx.app.configFieldIndex = 1
		hiddenBefore := ctx.app.deepDiveConfig.YaziShowHidden
		screen.Update(keyMsg(" "))
		if ctx.app.deepDiveConfig.YaziShowHidden == hiddenBefore {
			t.Fatal("exact-current main representative did not mutate")
		}
	})
}

func TestStandaloneYaziGlobalErrorsBlockEveryKeyboardField(t *testing.T) {
	mutationKeys := []string{"right", " ", "right", "right", " ", "right", "right"}
	tests := []struct {
		name   string
		state  NativeManageConfigState
		reason string
	}{
		{"preference", NativeManageConfigState{PreferenceError: "invalid manage.json"}, "saved management preferences could not be read safely: invalid manage.json"},
		{"Yazi import", NativeManageConfigState{YaziError: "resolver failed"}, "native Yazi configuration could not be imported safely: resolver failed"},
	}
	for _, test := range tests {
		for index, key := range mutationKeys {
			t.Run(fmt.Sprintf("%s/field-%d", test.name, index), func(t *testing.T) {
				ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
				ctx.app.nativeConfigState = test.state
				ctx.app.configFieldIndex = index
				before := yaziConfigFrom(*ctx.app.deepDiveConfig)
				screen.Update(keyMsg(key))
				if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
					t.Fatalf("globally blocked field %d mutated from %+v to %+v", index, before, got)
				}
				if view := screen.View(ctx.Width, ctx.Height); !strings.Contains(normalizedYaziVisibleText(view), normalizedYaziVisibleText(test.reason)) {
					t.Fatalf("global reason missing for field %d:\n%s", index, view)
				}
			})
		}
	}
}

func TestStandaloneYaziReadOnlyEnterAndNavigationRemainAvailable(t *testing.T) {
	const reason = "arbitrary native Yazi TOML is read-only in this release"

	t.Run("Enter opens save preview without config mutation", func(t *testing.T) {
		ctx, screen := newStandaloneYaziPolicyScreen(t, yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipNative, reason), yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipNative, reason))
		ctx.app.configFieldIndex = 3
		before := yaziConfigFrom(*ctx.app.deepDiveConfig)
		_, cmd := screen.Update(keyMsg("enter"))
		if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
			t.Fatalf("read-only Enter mutated config from %+v to %+v", before, got)
		}
		if cmd == nil {
			t.Fatal("read-only Enter returned no save-preview transition")
		}
		nav, ok := cmd().(NavigateMsg)
		if !ok || nav.To != ScreenConfigSaveConfirm {
			t.Fatalf("read-only Enter command = %#v, want NavigateTo(ScreenConfigSaveConfirm)", nav)
		}
	})

	navigationCases := []struct {
		name  string
		start int
		want  int
		msg   tea.Msg
	}{
		{name: "up", start: 3, want: 2, msg: keyMsg("up")},
		{name: "down", start: 3, want: 4, msg: keyMsg("down")},
		{name: "wheel up", start: 3, want: 2, msg: tea.MouseMsg{Button: tea.MouseButtonWheelUp}},
		{name: "wheel down", start: 3, want: 4, msg: tea.MouseMsg{Button: tea.MouseButtonWheelDown}},
	}
	for _, test := range navigationCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipNative, reason), yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipNative, reason))
			ctx.app.configFieldIndex = test.start
			before := yaziConfigFrom(*ctx.app.deepDiveConfig)
			screen.Update(test.msg)
			if ctx.app.configFieldIndex != test.want {
				t.Fatalf("focus = %d, want %d", ctx.app.configFieldIndex, test.want)
			}
			if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
				t.Fatalf("navigation mutated config from %+v to %+v", before, got)
			}
		})
	}
}

func TestStandaloneYaziPreferenceErrorPrecedenceIsReachable(t *testing.T) {
	const preferenceReason = "saved management preferences could not be read safely: invalid manage.json"
	const resolverReason = "native Yazi configuration could not be imported safely: resolver failed"
	const nativeReason = "arbitrary native Yazi TOML is read-only in this release"
	tests := []struct {
		name            string
		state           NativeManageConfigState
		notFocusedCause string
	}{
		{
			name: "preference error wins over resolver error with empty import",
			state: NativeManageConfigState{
				PreferenceError: "invalid manage.json",
				YaziError:       "resolver failed",
			},
			notFocusedCause: resolverReason,
		},
		{
			name: "preference error wins over native per-file cause",
			state: NativeManageConfigState{
				PreferenceError: "invalid manage.json",
				Yazi: tools.YaziConfigImport{
					Main:   yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipNative, nativeReason),
					Keymap: yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipNative, nativeReason),
				},
			},
			notFocusedCause: nativeReason,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
			ctx.app.nativeConfigState = test.state
			ctx.app.configFieldIndex = 3
			before := yaziConfigFrom(*ctx.app.deepDiveConfig)
			screen.Update(keyMsg("right"))
			if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
				t.Errorf("preference-blocked field mutated from %+v to %+v", before, got)
			}
			view := screen.View(ctx.Width, ctx.Height)
			visible := normalizedYaziVisibleText(view)
			if !strings.Contains(visible, normalizedYaziVisibleText(preferenceReason)) {
				t.Fatalf("focused view omitted preference cause %q:\n%s", preferenceReason, view)
			}
			if strings.Contains(visible, normalizedYaziVisibleText(test.notFocusedCause)) {
				t.Fatalf("focused view presented lower-precedence cause %q:\n%s", test.notFocusedCause, view)
			}
		})
	}
}

func TestStandaloneYaziWritableAllFieldMutationSmoke(t *testing.T) {
	mutationKeys := []string{"right", " ", "right", "right", " ", "right", "right"}
	for index, key := range mutationKeys {
		t.Run(fmt.Sprintf("field-%d", index), func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipExactCurrent, ""), yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipExactCurrent, ""))
			ctx.app.configFieldIndex = index
			before := yaziConfigFrom(*ctx.app.deepDiveConfig)
			screen.Update(keyMsg(key))
			if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got == before {
				t.Fatalf("writable field %d did not mutate", index)
			}
		})
	}
}

func TestStandaloneYaziMouseFocusExposesPerFileReasonWithoutMutation(t *testing.T) {
	const reason = "arbitrary native Yazi TOML is read-only in this release"
	tests := []struct {
		name       string
		index      int
		main       tools.YaziFileObservation
		keymap     tools.YaziFileObservation
		startIndex int
	}{
		{"keymap", 0, yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipExactCurrent, ""), yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipNative, reason), 3},
		{"main", 3, yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipNative, reason), yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipExactCurrent, ""), 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, test.main, test.keymap)
			ctx.app.configFieldIndex = test.startIndex
			before := yaziConfigFrom(*ctx.app.deepDiveConfig)
			_ = screen.View(ctx.Width, ctx.Height)
			extent := standaloneYaziFieldExtent(t, ctx.app, test.index)
			x := (ctx.app.configFieldLayout.boxLeft + ctx.app.configFieldLayout.boxRight) / 2
			screen.Update(clickAt(x, extent.startY))
			if ctx.app.configFieldIndex != test.index || yaziConfigFrom(*ctx.app.deepDiveConfig) != before {
				t.Fatalf("mouse focus index=%d want=%d config=%+v before=%+v", ctx.app.configFieldIndex, test.index, yaziConfigFrom(*ctx.app.deepDiveConfig), before)
			}
			if view := screen.View(ctx.Width, ctx.Height); !strings.Contains(normalizedYaziVisibleText(view), normalizedYaziVisibleText(reason)) {
				t.Fatalf("mouse-focused reason missing:\n%s", view)
			}
		})
	}
}

func TestStandaloneYaziRawReadOnlyValuesRemainLiteralAndImmutable(t *testing.T) {
	const mainContent = "[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n"
	const keymapContent = "[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n"
	tests := []struct {
		name        string
		index       int
		key         string
		want        string
		wantVisible string
		value       func(DeepDiveConfig) string
	}{
		{name: "keymap", index: 0, key: "right", want: "custom", wantVisible: "custom", value: func(cfg DeepDiveConfig) string { return cfg.YaziKeymap }},
		{name: "preview", index: 2, key: "right", want: "custom", wantVisible: "custom", value: func(cfg DeepDiveConfig) string { return cfg.YaziPreviewMode }},
		{name: "sort", index: 3, key: "right", want: "extension", wantVisible: "extension", value: func(cfg DeepDiveConfig) string { return cfg.YaziSortBy }},
		{name: "line mode", index: 5, key: "right", want: "owner", wantVisible: "owner", value: func(cfg DeepDiveConfig) string { return cfg.YaziLineMode }},
		{name: "scroll offset", index: 6, key: "left", want: "99", wantVisible: "99 lines", value: func(cfg DeepDiveConfig) string { return fmt.Sprintf("%d", cfg.YaziScrollOff) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen, imported := newStandaloneYaziImportedRawScreen(t, mainContent, keymapContent)
			if imported.Config.Keymap != "custom" || imported.Config.PreviewMode != "custom" || imported.Config.SortBy != "extension" || imported.Config.LineMode != "owner" || imported.Config.ScrollOff != 99 {
				t.Fatalf("imported raw config = %+v", imported.Config)
			}
			ctx.app.configFieldIndex = test.index
			if got := test.value(*ctx.app.deepDiveConfig); got != test.want {
				t.Fatalf("hydrated value = %q, want imported %q", got, test.want)
			}
			screen.Update(keyMsg(test.key))
			if got := test.value(*ctx.app.deepDiveConfig); got != test.want {
				t.Errorf("imported value mutated to %q, want unchanged %q", got, test.want)
			}
			_, fieldLines := standaloneYaziVisibleFieldLines(t, ctx, screen, test.index)
			if !strings.Contains(strings.Join(fieldLines, "\n"), test.wantVisible) {
				t.Errorf("focused field omitted imported value %q:\n%s", test.wantVisible, strings.Join(fieldLines, "\n"))
			}
		})
	}
}

func TestStandaloneYaziImportedRawValueCannotInjectTerminalControls(t *testing.T) {
	const injected = "evil\x1b[31m\nowned"
	const mainContent = "[mgr]\nsort_by = \"evil\\u001b[31m\\nowned\"\n"
	const keymapContent = "[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n"
	ctx, screen, imported := newStandaloneYaziImportedRawScreen(t, mainContent, keymapContent)
	if imported.Config.SortBy != injected {
		t.Fatalf("imported sort token = %q, want %q", imported.Config.SortBy, injected)
	}
	ctx.app.configFieldIndex = 3
	view, fieldLines := standaloneYaziVisibleFieldLines(t, ctx, screen, 3)
	safeVisible := false
	evilLine, ownedLine := -1, -1
	for index, line := range fieldLines {
		safeVisible = safeVisible || strings.Contains(line, "evilowned")
		if strings.Contains(line, "evil") {
			evilLine = index
		}
		if strings.Contains(line, "owned") {
			ownedLine = index
		}
	}
	if !safeVisible {
		t.Errorf("focused sort field omitted contiguous safe representation %q:\n%s", "evilowned", strings.Join(fieldLines, "\n"))
	}
	// The styled application output legitimately contains ANSI sequences, but
	// it must not copy the imported SGR sequence through verbatim.
	if strings.Contains(view, "\x1b[31m") {
		t.Errorf("render contains imported terminal-control sequence %q", "\x1b[31m")
	}
	if evilLine >= 0 && ownedLine >= 0 && evilLine != ownedLine {
		t.Errorf("sanitized token split evil/owned across focused field lines %d and %d:\n%s", evilLine, ownedLine, strings.Join(fieldLines, "\n"))
	}
}

func newStandaloneYaziImportedRawScreen(t *testing.T, mainContent, keymapContent string) (*ScreenContext, *configYaziScreen, tools.YaziConfigImport) {
	t.Helper()
	ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
	dir := filepath.Join(os.Getenv("HOME"), ".config", "yazi-imported")
	t.Setenv("YAZI_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tools.YaziFileMain), []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tools.YaziFileKeymap), []byte(keymapContent), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := tools.ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Main.Ownership != tools.YaziOwnershipNative || imported.Keymap.Ownership != tools.YaziOwnershipNative {
		t.Fatalf("raw fixtures must import as native: main=%s keymap=%s", imported.Main.Ownership, imported.Keymap.Ownership)
	}
	cfg := ctx.app.deepDiveConfig
	cfg.YaziKeymap = imported.Config.Keymap
	cfg.YaziShowHidden = imported.Config.ShowHidden
	cfg.YaziPreviewMode = imported.Config.PreviewMode
	cfg.YaziSortBy = imported.Config.SortBy
	cfg.YaziSortReverse = imported.Config.SortReverse
	cfg.YaziLineMode = imported.Config.LineMode
	cfg.YaziScrollOff = imported.Config.ScrollOff
	ctx.app.nativeConfigState.Yazi = imported
	return ctx, screen, imported
}

func TestStandaloneYaziFocusedProvenanceShowsOnlySelectedField(t *testing.T) {
	const mainContent = "[mgr]\nshow_hidden = true\nsort_by = \"extension\"\n"
	keymapContent := tools.GenerateYaziKeymap(tools.YaziConfig{Keymap: "vim"}, "nord")
	ctx, screen, imported := newStandaloneYaziImportedProvenanceScreen(t, mainContent, keymapContent)

	keymapField, ok := imported.Fields[tools.YaziFieldKeymap]
	if !ok || keymapField.Path != imported.Paths.Keymap || keymapField.Line != 3 || keymapField.Key != "header.keymap-style" || keymapField.Scope != tools.ConfigValueManaged {
		t.Fatalf("imported keymap provenance = %#v, exists=%t", keymapField, ok)
	}
	sortField, ok := imported.Fields[tools.YaziFieldSortBy]
	if !ok || sortField.Path != imported.Paths.Main || sortField.Line != 3 || sortField.Key != "mgr.sort_by" || sortField.Scope != tools.ConfigValueNative {
		t.Fatalf("imported sort provenance = %#v, exists=%t", sortField, ok)
	}

	tests := []struct {
		name     string
		index    int
		focused  tools.ConfigFieldProvenance
		other    tools.ConfigFieldProvenance
		otherKey string
	}{
		{name: "keymap", index: 0, focused: keymapField, other: sortField, otherKey: sortField.Key},
		{name: "main sort", index: 3, focused: sortField, other: keymapField, otherKey: keymapField.Key},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx.app.configFieldIndex = test.index
			view := stripANSITest(screen.View(ctx.Width, ctx.Height))
			assertYaziViewFitsWidth(t, view, ctx.Width)
			if !strings.Contains(view, "Observed at") {
				t.Errorf("focused provenance missing %q:\n%s", "Observed at", view)
			}
			focusedLocation := fmt.Sprintf("%s:%d", compactYaziTestPath(test.focused.Path), test.focused.Line)
			var focusedLines []string
			for _, line := range strings.Split(view, "\n") {
				if strings.Contains(line, focusedLocation) {
					focusedLines = append(focusedLines, line)
				}
			}
			if len(focusedLines) != 1 {
				t.Errorf("focused provenance location %q appeared on %d lines, want exactly one:\n%s", focusedLocation, len(focusedLines), view)
			} else {
				for _, want := range []string{test.focused.Key, string(test.focused.Scope)} {
					if !strings.Contains(focusedLines[0], want) {
						t.Errorf("focused provenance line missing %q:\n%s", want, focusedLines[0])
					}
				}
			}
			if otherLocation := fmt.Sprintf("%s:%d", compactYaziTestPath(test.other.Path), test.other.Line); strings.Contains(view, otherLocation) {
				t.Errorf("view leaked unfocused provenance location %q:\n%s", otherLocation, view)
			}
			if strings.Contains(view, test.otherKey) {
				t.Errorf("view leaked unfocused provenance key %q:\n%s", test.otherKey, view)
			}
		})
	}
}

func TestStandaloneYaziKeymapWithoutFieldProvenanceUsesObservationFallback(t *testing.T) {
	const nativeReason = "arbitrary native Yazi TOML is read-only in this release"
	const tasksOnlyKeymap = "[tasks]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n"
	ctx, screen, imported := newStandaloneYaziImportedProvenanceScreen(t, "[mgr]\nsort_by = \"extension\"\n", tasksOnlyKeymap)
	if imported.Keymap.Ownership != tools.YaziOwnershipNative {
		t.Fatalf("tasks-only keymap ownership = %s, want native", imported.Keymap.Ownership)
	}
	if provenance, exists := imported.Fields[tools.YaziFieldKeymap]; exists {
		t.Fatalf("tasks-only keymap fabricated provenance: %#v", provenance)
	}
	ctx.app.configFieldIndex = 0
	view := stripANSITest(screen.View(ctx.Width, ctx.Height))
	assertYaziViewFitsWidth(t, view, ctx.Width)
	compactPath := compactYaziTestPath(imported.Keymap.Path)
	for _, want := range []string{compactPath, string(imported.Keymap.Ownership), nativeReason} {
		if !strings.Contains(view, want) {
			t.Errorf("keymap observation fallback missing %q:\n%s", want, view)
		}
	}
	pathWithLine := regexp.MustCompile(regexp.QuoteMeta(compactPath) + `:\d+`)
	if strings.Contains(view, "header.keymap-style") || pathWithLine.MatchString(view) {
		t.Errorf("keymap observation fallback fabricated source line/key:\n%s", view)
	}
}

func TestStandaloneYaziPreferenceErrorDoesNotClaimImportedValueSource(t *testing.T) {
	const preferenceReason = "saved management preferences could not be read safely: invalid manage.json"
	ctx, screen, imported := newStandaloneYaziImportedProvenanceScreen(t, "[mgr]\nsort_by = \"extension\"\n", "[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n")
	sortField, exists := imported.Fields[tools.YaziFieldSortBy]
	if !exists {
		t.Fatal("native imported sort field has no provenance")
	}
	if imported.Config.SortBy != "extension" {
		t.Fatalf("imported sort = %q, want extension", imported.Config.SortBy)
	}
	ctx.app.deepDiveConfig.YaziSortBy = "alphabetical"
	if ctx.app.deepDiveConfig.YaziSortBy == imported.Config.SortBy {
		t.Fatal("displayed preference value must differ from native observation")
	}
	ctx.app.nativeConfigState.PreferenceError = "invalid manage.json"
	ctx.app.configFieldIndex = 3
	view := stripANSITest(screen.View(ctx.Width, ctx.Height))
	assertYaziViewFitsWidth(t, view, ctx.Width)
	if !strings.Contains(normalizedYaziVisibleText(view), normalizedYaziVisibleText(preferenceReason)) {
		t.Errorf("preference-error view omitted honest observation text %q:\n%s", preferenceReason, view)
	}
	for _, want := range []string{
		"Observed only",
		fmt.Sprintf("%s:%d", compactYaziTestPath(sortField.Path), sortField.Line),
		sortField.Key,
		string(sortField.Scope),
		"not applied",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("preference-error view omitted honest observation text %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Source:") {
		t.Errorf("preference-error view claimed imported value source with %q:\n%s", "Source:", view)
	}
}

func newStandaloneYaziImportedProvenanceScreen(t *testing.T, mainContent, keymapContent string) (*ScreenContext, *configYaziScreen, tools.YaziConfigImport) {
	t.Helper()
	ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
	dir := filepath.Join(os.Getenv("HOME"), ".config", "yazi-provenance")
	t.Setenv("YAZI_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tools.YaziFileMain), []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tools.YaziFileKeymap), []byte(keymapContent), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := tools.ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg := ctx.app.deepDiveConfig
	cfg.YaziKeymap = imported.Config.Keymap
	cfg.YaziShowHidden = imported.Config.ShowHidden
	cfg.YaziPreviewMode = imported.Config.PreviewMode
	cfg.YaziSortBy = imported.Config.SortBy
	cfg.YaziSortReverse = imported.Config.SortReverse
	cfg.YaziLineMode = imported.Config.LineMode
	cfg.YaziScrollOff = imported.Config.ScrollOff
	ctx.app.nativeConfigState.Yazi = imported
	return ctx, screen, imported
}

func TestStandaloneYaziThemeObservationIsDisplayOnlyNotice(t *testing.T) {
	tests := []struct {
		name        string
		write       bool
		content     string
		ownership   tools.YaziFileOwnership
		reason      string
		wantAnError bool
		errorTokens []string
	}{
		{name: "missing", ownership: tools.YaziOwnershipMissing},
		{name: "exact current", write: true, content: tools.GenerateYaziTheme("nord"), ownership: tools.YaziOwnershipExactCurrent},
		{name: "native flavor", write: true, content: "[flavor]\ndark = \"nord\"\n", ownership: tools.YaziOwnershipNative, reason: "native Yazi flavor selection is read-only in this release"},
		{name: "malformed", write: true, content: "[flavor\ndark = \"nord\"\n", ownership: tools.YaziOwnershipMalformed, reason: "malformed or unreadable Yazi TOML is read-only", wantAnError: true, errorTokens: []string{"invalid Yazi TOML preflight", "expected ']' to close table name"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
			dir := filepath.Join(os.Getenv("HOME"), ".config", "yazi-theme")
			t.Setenv("YAZI_CONFIG_HOME", dir)
			t.Setenv("XDG_CONFIG_HOME", "")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if test.write {
				if err := os.WriteFile(filepath.Join(dir, tools.YaziFileTheme), []byte(test.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			imported, err := tools.ImportYaziConfig()
			if err != nil {
				t.Fatal(err)
			}
			theme := imported.Theme
			if theme.Ownership != test.ownership || theme.ReadOnlyReason != test.reason {
				t.Fatalf("imported theme = %#v, want ownership=%s reason=%q", theme, test.ownership, test.reason)
			}
			if test.wantAnError && theme.Error == "" {
				t.Fatal("malformed imported theme has no useful error")
			}
			ctx.app.nativeConfigState.Yazi = imported
			ctx.app.configFieldIndex = 3
			beforeTheme := ctx.app.theme
			beforeConfig := fmt.Sprintf("%#v", *ctx.app.deepDiveConfig)
			view := screen.View(ctx.Width, ctx.Height)
			assertYaziViewFitsWidth(t, view, ctx.Width)
			wants := []string{"Theme file", "display-only", compactYaziTestPath(theme.Path), string(theme.Ownership)}
			if theme.ReadOnlyReason != "" {
				wants = append(wants, theme.ReadOnlyReason)
			}
			wants = append(wants, test.errorTokens...)
			visible := normalizedYaziVisibleText(view)
			for _, want := range wants {
				if !strings.Contains(strings.ToLower(visible), strings.ToLower(normalizedYaziVisibleText(want))) {
					t.Errorf("theme notice missing %q:\n%s", want, view)
				}
			}
			if screen.maxField(ctx.app) != 6 || len(ctx.app.configFieldLayout.extents) != 7 {
				t.Fatalf("theme notice changed field model: max=%d extents=%v", screen.maxField(ctx.app), ctx.app.configFieldLayout.extents)
			}
			noticeY := labelLineY(t, view, "Theme file")
			if noticeY < 0 {
				t.Error("theme notice has no visible Theme file line")
			} else {
				if field, found := ctx.app.configFieldLayout.fieldAt(noticeY); found {
					t.Errorf("theme notice row %d belongs to editable field %d", noticeY, field)
				}
				x := (ctx.app.configFieldLayout.boxLeft + ctx.app.configFieldLayout.boxRight) / 2
				screen.Update(clickAt(x, noticeY))
				if ctx.app.configFieldIndex != 3 {
					t.Errorf("clicking theme notice moved focus to %d, want 3", ctx.app.configFieldIndex)
				}
			}
			if ctx.app.theme != beforeTheme || fmt.Sprintf("%#v", *ctx.app.deepDiveConfig) != beforeConfig {
				t.Errorf("theme notice click mutated theme/config: theme=%q want=%q", ctx.app.theme, beforeTheme)
			}
			ctx.app.configFieldIndex = 6
			screen.Update(keyMsg("down"))
			if ctx.app.configFieldIndex != 6 {
				t.Errorf("down escaped seven-field boundary to %d", ctx.app.configFieldIndex)
			}
			ctx.app.configFieldIndex = 6
			screen.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
			if ctx.app.configFieldIndex != 6 {
				t.Errorf("wheel-down escaped seven-field boundary to %d", ctx.app.configFieldIndex)
			}
			if ctx.app.theme != beforeTheme || fmt.Sprintf("%#v", *ctx.app.deepDiveConfig) != beforeConfig {
				t.Errorf("theme notice navigation mutated theme/config: theme=%q want=%q", ctx.app.theme, beforeTheme)
			}
			for _, extent := range ctx.app.configFieldLayout.extents {
				if extent.index == 7 {
					t.Fatal("theme notice created an eighth field")
				}
			}
		})
	}
}

func TestStandaloneYaziResolverErrorRendersReachableCause(t *testing.T) {
	ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
	hostileDir := filepath.Join(os.Getenv("HOME"), ".config", "safe\x1b[31m\nowned")
	t.Setenv("YAZI_CONFIG_HOME", hostileDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	_, err := tools.ImportYaziConfig()
	if err == nil {
		t.Fatal("hostile YAZI_CONFIG_HOME unexpectedly passed resolver validation")
	}
	ctx.app.nativeConfigState = NativeManageConfigState{YaziError: err.Error()}
	ctx.app.configFieldIndex = 3
	before := yaziConfigFrom(*ctx.app.deepDiveConfig)
	screen.Update(keyMsg("right"))
	if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
		t.Errorf("resolver-error field mutated from %+v to %+v", before, got)
	}
	view := screen.View(ctx.Width, ctx.Height)
	wantCause := "native Yazi configuration could not be imported safely: " + err.Error()
	if !strings.Contains(normalizedYaziVisibleText(view), normalizedYaziVisibleText(wantCause)) {
		t.Errorf("resolver-error view omitted exact cause %q", wantCause)
	}
}

func TestCompactYaziDisplayTextRespectsHomeBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		home  string
		value string
		want  string
	}{
		{name: "exact home", home: "/tmp/foo", value: "/tmp/foo", want: "~"},
		{name: "contained path", home: "/tmp/foo", value: "/tmp/foo/.config/yazi/theme.toml: err", want: "~/.config/yazi/theme.toml: err"},
		{name: "prefix collision", home: "/tmp/foo", value: "/tmp/foobar/theme.toml: err", want: "/tmp/foobar/theme.toml: err"},
		{name: "root home", home: "/", value: "/tmp/theme.toml: err", want: "/tmp/theme.toml: err"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", test.home)
			if got := compactYaziDisplayText(test.value); got != test.want {
				t.Fatalf("compactYaziDisplayText(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestStandaloneYaziKnownValuesExplainGeneratedBehavior(t *testing.T) {
	keymapCases := []struct {
		value string
		want  string
	}{
		{value: "vim", want: "Vim/Yazi defaults"},
		{value: "emacs", want: "Emacs Ctrl-P/N/B/F + Space"},
	}
	for _, test := range keymapCases {
		t.Run("keymap/"+test.value, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
			ctx.app.deepDiveConfig.YaziKeymap = test.value
			ctx.app.configFieldIndex = 0
			visible := normalizedYaziVisibleText(screen.View(ctx.Width, ctx.Height))
			if !strings.Contains(visible, test.want) {
				t.Errorf("keymap %q copy missing %q", test.value, test.want)
			}
			if strings.Contains(visible, "Emacs (arrows)") {
				t.Error("standalone Yazi still advertises stale Emacs arrows semantics")
			}
		})
	}

	previewCases := []struct {
		value string
		want  string
	}{
		{value: "auto", want: "Default image-preview delay (30ms)"},
		{value: "always", want: "Immediate image previews (0ms delay)"},
		{value: "never", want: "Disable previewers + preloaders"},
	}
	misleadingPreview := regexp.MustCompile(`\b(?:Always|Never)\b`)
	for _, test := range previewCases {
		t.Run("preview/"+test.value, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
			ctx.app.deepDiveConfig.YaziPreviewMode = test.value
			ctx.app.configFieldIndex = 2
			visible := normalizedYaziVisibleText(screen.View(ctx.Width, ctx.Height))
			if !strings.Contains(visible, test.want) {
				t.Errorf("preview %q copy missing %q", test.value, test.want)
			}
			if misleadingPreview.MatchString(visible) {
				t.Error("standalone Yazi still exposes bare Always/Never preview labels")
			}
		})
	}

	fieldCases := []struct {
		name  string
		index int
		set   func(*DeepDiveConfig)
		want  string
	}{
		{name: "modified sort", index: 3, set: func(cfg *DeepDiveConfig) { cfg.YaziSortBy = "modified" }, want: "Modified (mtime)"},
		{name: "modified line metadata", index: 5, set: func(cfg *DeepDiveConfig) { cfg.YaziLineMode = "mtime" }, want: "Modified metadata (mtime)"},
		{name: "scroll context", index: 6, set: func(*DeepDiveConfig) {}, want: "entries above and below cursor"},
		{name: "hidden files", index: 1, set: func(*DeepDiveConfig) {}, want: "Show dotfiles by default"},
		{name: "reverse sort", index: 4, set: func(*DeepDiveConfig) {}, want: "Reverse selected sort order"},
	}
	for _, test := range fieldCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
			test.set(ctx.app.deepDiveConfig)
			ctx.app.configFieldIndex = test.index
			visible := normalizedYaziVisibleText(screen.View(ctx.Width, ctx.Height))
			if !strings.Contains(visible, test.want) {
				t.Errorf("focused field copy missing %q", test.want)
			}
		})
	}
}

func TestManageYaziFocusedDescriptionsExplainGeneratedBehavior(t *testing.T) {
	tests := []struct {
		name  string
		field string
		set   func(*ManageConfig)
		want  string
	}{
		{name: "vim keymap", field: "keymap", set: func(cfg *ManageConfig) { cfg.YaziKeymap = "vim" }, want: "Vim/Yazi defaults"},
		{name: "emacs keymap", field: "keymap", set: func(cfg *ManageConfig) { cfg.YaziKeymap = "emacs" }, want: "Emacs Ctrl-P/N/B/F + Space"},
		{name: "default previews", field: "preview_mode", set: func(cfg *ManageConfig) { cfg.YaziPreviewMode = "auto" }, want: "Default image-preview delay (30ms)"},
		{name: "immediate previews", field: "preview_mode", set: func(cfg *ManageConfig) { cfg.YaziPreviewMode = "always" }, want: "Immediate image previews (0ms delay)"},
		{name: "disabled previews", field: "preview_mode", set: func(cfg *ManageConfig) { cfg.YaziPreviewMode = "never" }, want: "Disable previewers + preloaders"},
		{name: "modified sort", field: "sort_by", set: func(cfg *ManageConfig) { cfg.YaziSortBy = "modified" }, want: "Modified (mtime)"},
		{name: "modified line metadata", field: "linemode", set: func(cfg *ManageConfig) { cfg.YaziLineMode = "mtime" }, want: "Modified metadata (mtime)"},
		{name: "scroll context", field: "scrolloff", set: func(*ManageConfig) {}, want: "entries above and below cursor"},
		{name: "hidden files", field: "hidden", set: func(*ManageConfig) {}, want: "Show dotfiles by default"},
		{name: "reverse sort", field: "sort_rev", set: func(*ManageConfig) {}, want: "Reverse selected sort order"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newManageYaziPolicyScreen(t, false, false)
			test.set(ctx.app.manageConfig)
			ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, test.field)
			visible := normalizedYaziVisibleText(screen.View(ctx.Width, ctx.Height))
			if !strings.Contains(visible, test.want) {
				t.Errorf("Manage focused description missing %q", test.want)
			}
		})
	}
}

func TestManageYaziCompactWritableRowsAndFooter(t *testing.T) {
	dimensions := []struct{ width, height int }{{80, 24}, {60, 18}}
	fields := []struct {
		key   string
		label string
		value string
	}{
		{"keymap", "Keymap", "vim"},
		{"hidden", "Show Hidden", "OFF"},
		{"preview_mode", "Preview Mode", "auto"},
		{"sort_by", "Sort By", "alphabetical"},
		{"sort_rev", "Sort Reverse", "OFF"},
		{"linemode", "Line Mode", "none"},
		{"scrolloff", "Scroll Offset", "5 lines"},
	}
	for _, size := range dimensions {
		for _, field := range fields {
			t.Run(fmt.Sprintf("%dx%d/%s", size.width, size.height, field.key), func(t *testing.T) {
				ctx, screen := newManageYaziPolicyScreen(t, false, false)
				ctx.Width, ctx.Height = size.width, size.height
				ctx.app.width, ctx.app.height = size.width, size.height
				ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, field.key)
				view := screen.View(size.width, size.height)
				assertYaziRenderBounds(t, view, size.width, size.height)
				row := manageYaziFocusedRowText(t, view, field.label)
				for _, want := range []string{"▸", field.label, field.value} {
					if !strings.Contains(row, want) {
						t.Errorf("focused Manage row missing %q", want)
					}
				}
				help := manageYaziHelpLineText(t, view)
				wants := []string{"Tab tools", "↑↓", "S save", "Esc back", "q quit"}
				if size.width == 80 {
					wants = append(wants, "? hotkeys")
				}
				for _, want := range wants {
					if !strings.Contains(help, want) {
						t.Errorf("compact Manage help missing %q", want)
					}
				}
				if !strings.Contains(help, "change") && (!strings.Contains(help, "←→") || !strings.Contains(help, "Space")) {
					t.Error("writable Manage help omitted edit/change route")
				}
			})
		}
	}
}

func TestManageYaziCompactReadOnlyRawRowsExposeCause(t *testing.T) {
	dimensions := []struct{ width, height int }{{80, 24}, {60, 18}}
	const reason = "arbitrary native Yazi TOML is read-only in this release"
	fields := []struct {
		key   string
		label string
		value string
	}{{"keymap", "Keymap", "custom"}, {"sort_by", "Sort By", "extension"}, {"scrolloff", "Scroll Offset", "99 lines"}}
	for _, size := range dimensions {
		for _, field := range fields {
			t.Run(fmt.Sprintf("%dx%d/%s", size.width, size.height, field.key), func(t *testing.T) {
				ctx, screen := newManageYaziPolicyScreen(t, true, true)
				ctx.app.manageConfig.YaziKeymap = "custom"
				ctx.app.manageConfig.YaziSortBy = "extension"
				ctx.app.manageConfig.YaziScrollOff = 99
				ctx.Width, ctx.Height = size.width, size.height
				ctx.app.width, ctx.app.height = size.width, size.height
				ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, field.key)
				before := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig))
				view := screen.View(size.width, size.height)
				assertYaziRenderBounds(t, view, size.width, size.height)
				row := manageYaziFocusedRowText(t, view, field.label)
				for _, want := range []string{"▸", field.label, field.value, "(read-only)"} {
					if !strings.Contains(row, want) {
						t.Errorf("focused read-only Manage row missing %q", want)
					}
				}
				if !strings.Contains(normalizedYaziVisibleText(view), reason) {
					t.Error("read-only Manage view omitted exact cause before interaction")
				}
				if !strings.Contains(normalizedYaziVisibleText(view), "focused read-only") {
					t.Error("read-only Manage view omitted focused read-only status")
				}
				help := manageYaziHelpLineText(t, view)
				wants := []string{"Tab tools", "↑↓", "S save", "Esc back", "q quit"}
				if size.width == 80 {
					wants = append(wants, "? hotkeys")
				}
				for _, want := range wants {
					if !strings.Contains(help, want) {
						t.Errorf("compact read-only Manage help missing %q", want)
					}
				}
				for _, forbidden := range []string{"←→", "Space", "change"} {
					if strings.Contains(help, forbidden) {
						t.Errorf("read-only Manage help advertises %q", forbidden)
					}
				}
				if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != before {
					t.Errorf("read-only Manage render mutated config from %+v to %+v", before, got)
				}
			})
		}
	}
}

func TestManageYaziNativeImportBadgesAreTruthful(t *testing.T) {
	tests := []struct {
		name      string
		state     func(*App)
		wants     []string
		forbidden []string
	}{
		{name: "native", state: func(*App) {}, wants: []string{"NATIVE SOURCE"}},
		{name: "managed", state: func(app *App) {
			app.nativeConfigState.Yazi = tools.YaziConfigImport{
				Main:   yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipExactCurrent, ""),
				Keymap: yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipExactCurrent, ""),
			}
		}, wants: []string{"MANAGED SOURCE"}},
		{name: "mixed native and managed", state: func(app *App) {
			app.nativeConfigState.Yazi = tools.YaziConfigImport{
				Main:   yaziObservation(tools.YaziFileKindMain, tools.YaziOwnershipNative, "arbitrary native Yazi TOML is read-only in this release"),
				Keymap: yaziObservation(tools.YaziFileKindKeymap, tools.YaziOwnershipExactCurrent, ""),
			}
		}, wants: []string{"NATIVE SOURCE", "MANAGED SOURCE"}},
		{name: "malformed", state: func(app *App) {
			app.nativeConfigState.Yazi = tools.YaziConfigImport{
				Main: tools.YaziFileObservation{Kind: tools.YaziFileKindMain, Path: filepath.Join(os.Getenv("HOME"), ".config/yazi/yazi.toml"), Exists: true, Ownership: tools.YaziOwnershipMalformed, ReadOnlyReason: "malformed or unreadable Yazi TOML is read-only", Error: "parse failed"},
			}
		}, wants: []string{"IMPORT BLOCKED"}},
		{name: "missing", state: func(app *App) {
			app.nativeConfigState.Yazi = tools.YaziConfigImport{
				Main:   tools.YaziFileObservation{Kind: tools.YaziFileKindMain, Path: filepath.Join(os.Getenv("HOME"), ".config/yazi/yazi.toml"), Ownership: tools.YaziOwnershipMissing},
				Keymap: tools.YaziFileObservation{Kind: tools.YaziFileKindKeymap, Path: filepath.Join(os.Getenv("HOME"), ".config/yazi/keymap.toml"), Ownership: tools.YaziOwnershipMissing},
			}
		}, forbidden: []string{"NATIVE SOURCE", "MANAGED SOURCE", "IMPORT BLOCKED"}},
		{name: "Yazi error", state: func(app *App) { app.nativeConfigState = NativeManageConfigState{YaziError: "resolver failed"} }, wants: []string{"IMPORT BLOCKED"}},
		{name: "preference error", state: func(app *App) { app.nativeConfigState.PreferenceError = "invalid manage.json" }, wants: []string{"IMPORT BLOCKED"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newManageYaziPolicyScreen(t, true, true)
			test.state(ctx.app)
			visible := normalizedYaziVisibleText(screen.View(ctx.Width, ctx.Height))
			for _, want := range test.wants {
				if !strings.Contains(visible, want) {
					t.Errorf("Yazi Manage badge missing %q", want)
				}
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(visible, forbidden) {
					t.Errorf("Yazi Manage badge unexpectedly contains %q", forbidden)
				}
			}
		})
	}
}

func TestManageYaziCompactScrolledMouseMapsToLogicalScrollOffset(t *testing.T) {
	const width, height = 60, 18
	tests := []struct {
		name  string
		x     func(manageLayout) int
		delta int
	}{
		{name: "right half increments", x: func(layout manageLayout) int { return layout.rightX + 3*layout.rightW/4 }, delta: 1},
		{name: "left half decrements", x: func(layout manageLayout) int { return layout.rightX + layout.rightW/4 }, delta: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newManageYaziPolicyScreen(t, false, false)
			ctx.Width, ctx.Height = width, height
			ctx.app.width, ctx.app.height = width, height
			ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "scrolloff")

			view := screen.View(width, height)
			assertYaziRenderBounds(t, view, width, height)
			if ctx.app.manageFieldsScroll <= 0 {
				t.Fatalf("focused Scroll Offset did not establish a scrolled viewport: scroll=%d", ctx.app.manageFieldsScroll)
			}
			if !strings.Contains(manageYaziFocusedRowText(t, view, "Scroll Offset"), "Scroll Offset") {
				t.Fatal("scrolled Scroll Offset row is not visible")
			}

			layout := ctx.app.manageLayout()
			scroll := ctx.app.manageFieldsScroll
			rowY := layout.rightListY + (6 - scroll)
			before := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig))
			ctx.app.configFieldIndex = 0 // Deliberately stale focus; do not rerender before clicking.
			screen.Update(clickAt(test.x(layout), rowY))

			if ctx.app.configFieldIndex != 6 {
				t.Fatalf("scrolled row click focused logical field %d, want 6 (scroll=%d rowY=%d)", ctx.app.configFieldIndex, scroll, rowY)
			}
			if got, want := ctx.app.manageConfig.YaziScrollOff, before.ScrollOff+test.delta; got != want {
				t.Fatalf("scrolled row click changed Scroll Offset to %d, want exact %d", got, want)
			}
			wantConfig := before
			wantConfig.ScrollOff += test.delta
			if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != wantConfig {
				t.Fatalf("scrolled row click changed wrong field: got %+v want %+v", got, wantConfig)
			}
		})
	}
}

func TestManageYaziCompactMaximumPressureNative(t *testing.T) {
	const width, height = 60, 18
	const nativeReason = "arbitrary native Yazi TOML is read-only in this release"
	ctx, screen, imported := newManageYaziPressureScreen(t)
	ctx.app.manageConfig.YaziKeymap = imported.Config.Keymap
	ctx.app.manageConfig.YaziShowHidden = imported.Config.ShowHidden
	ctx.app.manageConfig.YaziPreviewMode = imported.Config.PreviewMode
	ctx.app.manageConfig.YaziSortBy = imported.Config.SortBy
	ctx.app.manageConfig.YaziSortReverse = imported.Config.SortReverse
	ctx.app.manageConfig.YaziLineMode = imported.Config.LineMode
	ctx.app.manageConfig.YaziScrollOff = imported.Config.ScrollOff
	ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "sort_by")
	before := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig))
	stateBefore := fmt.Sprintf("%#v", ctx.app.nativeConfigState.Yazi)

	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	row := manageYaziFocusedRowText(t, view, "Sort By")
	for _, want := range []string{"▸", "Sort By", "extension", "(read-only)"} {
		if !strings.Contains(row, want) {
			t.Errorf("maximum-pressure native focused row missing %q", want)
		}
	}
	visible := normalizedYaziVisibleText(view)
	for _, want := range []string{nativeReason, "NATIVE SOURCE", "Observed at", "mgr.sort_by", "native", "Theme", "malformed", "display-only"} {
		if !strings.Contains(visible, want) {
			t.Errorf("maximum-pressure native Manage view missing %q", want)
		}
	}
	assertStandaloneYaziNearbyCopy(t, view, "mgr.sort_by", []string{"Observed at", "native"})
	assertManageYaziPressureHelp(t, view)
	if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != before {
		t.Errorf("maximum-pressure native render mutated config from %+v to %+v", before, got)
	}
	if got := fmt.Sprintf("%#v", ctx.app.nativeConfigState.Yazi); got != stateBefore {
		t.Error("maximum-pressure native render mutated imported observation state")
	}
}

func TestManageYaziCompactMaximumPressurePreferenceError(t *testing.T) {
	const width, height = 60, 18
	const preferenceReason = "saved management preferences could not be read safely: invalid manage.json"
	const nativeReason = "arbitrary native Yazi TOML is read-only in this release"
	ctx, screen, imported := newManageYaziPressureScreen(t)
	ctx.app.nativeConfigState.PreferenceError = "invalid manage.json"
	ctx.app.manageConfig = NewManageConfig()
	wantDefaults := yaziConfigFrom(manageConfigToDeepDive(NewManageConfig()))
	if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != wantDefaults {
		t.Fatalf("preference fixture did not reset all seven Yazi values: got %+v want %+v", got, wantDefaults)
	}
	if ctx.app.manageConfig.YaziSortBy == imported.Config.SortBy {
		t.Fatal("preference fixture did not distinguish displayed and observed sort values")
	}
	ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "sort_by")
	before := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig))
	stateBefore := fmt.Sprintf("%#v", ctx.app.nativeConfigState.Yazi)

	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	row := manageYaziFocusedRowText(t, view, "Sort By")
	for _, want := range []string{"▸", "Sort By", "alphabetical", "(read-only)"} {
		if !strings.Contains(row, want) {
			t.Errorf("maximum-pressure preference focused row missing %q", want)
		}
	}
	visible := normalizedYaziVisibleText(view)
	for _, want := range []string{"IMPORT BLOCKED", preferenceReason, "Observed only (not applied)", "mgr.sort_by", "native", "Theme", "malformed", "display-only"} {
		if !strings.Contains(visible, want) {
			t.Errorf("maximum-pressure preference Manage view missing %q", want)
		}
	}
	if strings.Contains(visible, nativeReason) {
		t.Error("maximum-pressure preference view exposed lower-precedence native cause")
	}
	assertStandaloneYaziNearbyCopy(t, view, "mgr.sort_by", []string{"Observed only (not applied)", "native"})
	assertManageYaziPressureHelp(t, view)
	if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != before {
		t.Errorf("maximum-pressure preference render mutated config from %+v to %+v", before, got)
	}
	if got := fmt.Sprintf("%#v", ctx.app.nativeConfigState.Yazi); got != stateBefore {
		t.Error("maximum-pressure preference render mutated imported observation state")
	}
}

func newManageYaziPressureScreen(t *testing.T) (*ScreenContext, *manageScreen, tools.YaziConfigImport) {
	t.Helper()
	ctx, screen := newManageYaziPolicyScreen(t, false, false)
	const width, height = 60, 18
	ctx.Width, ctx.Height = width, height
	ctx.app.width, ctx.app.height = width, height
	dir := filepath.Join(os.Getenv("HOME"), ".config", "yazi-manage-pressure")
	t.Setenv("YAZI_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	mainContent := "[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n"
	keymapContent := "[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n"
	if err := os.WriteFile(filepath.Join(dir, tools.YaziFileMain), []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tools.YaziFileKeymap), []byte(keymapContent), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := tools.ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Main.Ownership != tools.YaziOwnershipNative || imported.Keymap.Ownership != tools.YaziOwnershipNative {
		t.Fatalf("pressure fixtures must import as native: main=%s keymap=%s", imported.Main.Ownership, imported.Keymap.Ownership)
	}
	if provenance, ok := imported.Fields[tools.YaziFieldSortBy]; !ok || provenance.Key != "mgr.sort_by" || provenance.Scope != tools.ConfigValueNative {
		t.Fatalf("pressure fixture sort provenance=%#v exists=%t", provenance, ok)
	}
	imported.Theme = tools.InspectYaziConfigContent(tools.YaziFileKindTheme, imported.Paths.Theme, []byte("[flavor\ndark = \"nord\"\n"), true)
	ctx.app.nativeConfigState.Yazi = imported
	return ctx, screen, imported
}

func assertManageYaziPressureHelp(t *testing.T, view string) {
	t.Helper()
	help := manageYaziHelpLineText(t, view)
	for _, want := range []string{"Tab tools", "↑↓", "focused read-only", "S save", "Esc back", "q quit"} {
		if !strings.Contains(help, want) {
			t.Errorf("maximum-pressure Manage help missing %q", want)
		}
	}
	for _, forbidden := range []string{"←→", "Space", "change"} {
		if strings.Contains(help, forbidden) {
			t.Errorf("maximum-pressure read-only Manage help advertises %q", forbidden)
		}
	}
}

func TestManageYaziCompactWheelMaintainsCoherentSelection(t *testing.T) {
	const width, height = 60, 18
	ctx, screen := newManageYaziPolicyScreen(t, false, false)
	ctx.Width, ctx.Height = width, height
	ctx.app.width, ctx.app.height = width, height
	ctx.app.configFieldIndex = 0
	before := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig))
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	if ctx.app.manageFieldsScroll != 0 {
		t.Fatalf("wheel fixture initial scroll=%d, want 0", ctx.app.manageFieldsScroll)
	}
	layout := ctx.app.manageLayout()
	wheelX := layout.rightX + layout.rightW/2

	screen.Update(tea.MouseMsg{X: wheelX, Y: layout.rightListY, Button: tea.MouseButtonWheelDown})
	view = screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	if ctx.app.manageFieldsScroll != 1 || ctx.app.configFieldIndex != 1 {
		t.Errorf("wheel down + rerender scroll/focus=%d/%d, want 1/1", ctx.app.manageFieldsScroll, ctx.app.configFieldIndex)
	}
	if !strings.Contains(manageYaziFocusedRowText(t, view, "Show Hidden"), "▸") {
		t.Error("wheel-down selected logical field is not visibly focused")
	}
	if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != before {
		t.Fatalf("wheel down mutated config from %+v to %+v", before, got)
	}

	screen.Update(tea.MouseMsg{X: wheelX, Y: layout.rightListY, Button: tea.MouseButtonWheelUp})
	view = screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	if ctx.app.manageFieldsScroll != 0 || ctx.app.configFieldIndex != 0 {
		t.Errorf("wheel up + rerender scroll/focus=%d/%d, want 0/0", ctx.app.manageFieldsScroll, ctx.app.configFieldIndex)
	}
	if !strings.Contains(manageYaziFocusedRowText(t, view, "Keymap"), "▸") {
		t.Error("wheel-up selected logical field is not visibly focused")
	}
	if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != before {
		t.Fatalf("wheel up mutated config from %+v to %+v", before, got)
	}
}

func TestManageYaziCompactHeaderAndFooterClicksPreserveState(t *testing.T) {
	const width, height = 60, 18
	tests := []struct {
		name string
		y    func(t *testing.T, view string, layout manageLayout) int
	}{
		{name: "panel header", y: func(_ *testing.T, _ string, layout manageLayout) int { return layout.rightListY - 2 }},
		{name: "help footer", y: func(t *testing.T, view string, _ manageLayout) int {
			y := labelLineY(t, view, "S save")
			if y < 0 {
				t.Fatal("compact Manage help/footer row is not visible")
			}
			return y
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newManageYaziPolicyScreen(t, false, false)
			ctx.Width, ctx.Height = width, height
			ctx.app.width, ctx.app.height = width, height
			ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "scrolloff")
			ctx.app.manageStatus = "sentinel status"
			view := screen.View(width, height)
			assertYaziRenderBounds(t, view, width, height)
			layout := ctx.app.manageLayout()
			y := test.y(t, view, layout)
			before := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig))
			focusBefore := ctx.app.configFieldIndex
			scrollBefore := ctx.app.manageFieldsScroll
			paneBefore := ctx.app.managePane
			statusBefore := ctx.app.manageStatus
			editingBefore := ctx.app.manageEditing

			_, cmd := screen.Update(clickAt(layout.rightX+3*layout.rightW/4, y))
			if cmd != nil {
				t.Errorf("%s click returned non-nil command", test.name)
			}
			if ctx.app.configFieldIndex != focusBefore || ctx.app.manageFieldsScroll != scrollBefore {
				t.Errorf("%s click changed focus/scroll from %d/%d to %d/%d", test.name, focusBefore, scrollBefore, ctx.app.configFieldIndex, ctx.app.manageFieldsScroll)
			}
			if ctx.app.managePane != paneBefore || ctx.app.manageStatus != statusBefore || ctx.app.manageEditing != editingBefore {
				t.Errorf("%s click changed pane/status/editing from %d/%q/%t to %d/%q/%t", test.name, paneBefore, statusBefore, editingBefore, ctx.app.managePane, ctx.app.manageStatus, ctx.app.manageEditing)
			}
			if got := yaziConfigFrom(manageConfigToDeepDive(ctx.app.manageConfig)); got != before {
				t.Errorf("%s click mutated config from %+v to %+v", test.name, before, got)
			}
		})
	}
}

func TestManageYaziCompactFullWidthSettingsGeometry(t *testing.T) {
	const width, height = 60, 18
	for _, test := range []struct {
		name      string
		x         int
		wantValue int
	}{
		{name: "left edge", x: 1, wantValue: 4},
		{name: "right edge", x: width - 1, wantValue: 6},
	} {
		t.Run("click/"+test.name, func(t *testing.T) {
			ctx, screen := newManageYaziPolicyScreen(t, false, false)
			ctx.Width, ctx.Height = width, height
			ctx.app.width, ctx.app.height = width, height
			ctx.app.configFieldIndex = 6
			_ = screen.View(width, height)
			layout := ctx.app.manageLayout()
			if layout.rightX != 0 || layout.rightW != width || layout.leftW != 0 {
				t.Fatalf("compact Yazi geometry left/right=%d/%d+%d, want inactive left and full-width right", layout.leftW, layout.rightX, layout.rightW)
			}
			rowY := layout.rightListY + (6 - ctx.app.manageFieldsScroll)
			ctx.app.configFieldIndex = 0 // Preserve rendered scroll geometry.
			screen.Update(clickAt(test.x, rowY))
			if ctx.app.configFieldIndex != 6 || ctx.app.manageConfig.YaziScrollOff != test.wantValue {
				t.Fatalf("full-span click x=%d focused/value=%d/%d, want 6/%d", test.x, ctx.app.configFieldIndex, ctx.app.manageConfig.YaziScrollOff, test.wantValue)
			}
		})

		t.Run("wheel/"+test.name, func(t *testing.T) {
			ctx, screen := newManageYaziPolicyScreen(t, false, false)
			ctx.Width, ctx.Height = width, height
			ctx.app.width, ctx.app.height = width, height
			ctx.app.configFieldIndex = 0
			_ = screen.View(width, height)
			screen.Update(tea.MouseMsg{X: test.x, Y: ctx.app.manageLayout().rightListY, Button: tea.MouseButtonWheelDown})
			_ = screen.View(width, height)
			if ctx.app.manageFieldsScroll != 1 || ctx.app.configFieldIndex != 1 {
				t.Fatalf("full-span wheel x=%d scroll/focus=%d/%d, want 1/1", test.x, ctx.app.manageFieldsScroll, ctx.app.configFieldIndex)
			}
		})
	}
}

func TestManageYaziCompactPaneSwitchRendersActivePane(t *testing.T) {
	const width, height = 60, 18
	ctx, screen := newManageYaziPolicyScreen(t, false, false)
	ctx.Width, ctx.Height = width, height
	ctx.app.width, ctx.app.height = width, height
	yaziIndex := ctx.app.manageIndex

	screen.Update(keyMsg("tab"))
	if ctx.app.managePane != managePaneTools {
		t.Fatal("Tab did not switch compact Manage to tools pane")
	}
	view := normalizedYaziVisibleText(screen.View(width, height))
	if !strings.Contains(view, "▸ Yazi") || strings.Contains(view, "Keymap:") {
		t.Fatalf("compact tools pane does not match pane state: %q", view)
	}

	if yaziIndex > 0 {
		screen.Update(keyMsg("up"))
		if ctx.app.manageIndex != yaziIndex-1 {
			t.Fatalf("compact tools Up index=%d, want %d", ctx.app.manageIndex, yaziIndex-1)
		}
		selected := ctx.app.manageItems()[ctx.app.manageIndex].name
		if row := normalizedYaziVisibleText(screen.View(width, height)); !strings.Contains(row, "▸ "+selected) {
			t.Fatalf("moved compact tool selection %q is not visible: %q", selected, row)
		}
	}

	ctx.app.manageIndex = yaziIndex
	_ = screen.View(width, height)
	layout := ctx.app.manageLayout()
	clickIndex := ctx.app.manageToolsScroll
	screen.Update(clickAt(width-1, layout.leftListY))
	if ctx.app.manageIndex != clickIndex || ctx.app.managePane != managePaneTools {
		t.Fatalf("full-width compact tool click index/pane=%d/%d, want %d/tools", ctx.app.manageIndex, ctx.app.managePane, clickIndex)
	}
	ctx.app.manageIndex = 0
	ctx.app.manageToolsScroll = 0
	_ = screen.View(width, height)
	screen.Update(tea.MouseMsg{X: width - 1, Y: layout.leftListY, Button: tea.MouseButtonWheelDown})
	_ = screen.View(width, height)
	if ctx.app.manageToolsScroll != 1 || ctx.app.manageIndex != 1 {
		t.Fatalf("full-width compact tool wheel scroll/index=%d/%d, want 1/1", ctx.app.manageToolsScroll, ctx.app.manageIndex)
	}

	ctx.app.manageIndex = yaziIndex
	screen.Update(keyMsg("tab"))
	if ctx.app.managePane != managePaneSettings || !strings.Contains(normalizedYaziVisibleText(screen.View(width, height)), "Keymap:") {
		t.Fatal("Tab did not restore visible compact Yazi settings pane")
	}
}

func TestManageYaziCompactTabSpansRouteExactly(t *testing.T) {
	tests := []struct {
		label  string
		target Screen
	}{
		{"1 Manage", ScreenManage},
		{"2 Users", ScreenUsers},
		{"3 Hotkeys", ScreenHotkeys},
		{"4 Update", ScreenUpdate},
		{"5 Backups", ScreenBackups},
	}
	for _, test := range tests {
		start := strings.Index(compactManageTabLine, test.label)
		if start < 0 {
			t.Fatalf("compact tab line missing %q", test.label)
		}
		for _, x := range []int{start, start + lipgloss.Width(test.label) - 1} {
			if got := detectCompactManageTabClick(x); got != test.target {
				t.Errorf("compact tab %q x=%d routed to %v, want %v", test.label, x, got, test.target)
			}
		}
	}
}

func TestManageYaziCompactSanitizesStatusBeforeLayout(t *testing.T) {
	const width, height = 60, 18
	ctx, screen := newManageYaziPolicyScreen(t, false, false)
	ctx.Width, ctx.Height = width, height
	ctx.app.width, ctx.app.height = width, height
	ctx.app.manageStatus = "safe\x1b[31m\nINJECTED\rstatus"
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	if strings.Contains(view, "\x1b[31m") || strings.Contains(stripANSITest(view), "\nINJECTED") {
		t.Fatal("compact Manage status injected terminal controls or a new row")
	}
	lines := strings.Split(stripANSITest(view), "\n")
	if !strings.Contains(lines[len(lines)-1], "q quit") {
		t.Fatal("injected status displaced the pinned compact help row")
	}
}

func TestManageYaziCompactTabsInstallAndSourceMetadataAreTruthful(t *testing.T) {
	ctx, screen := newManageYaziPolicyScreen(t, false, false)
	ctx.Width, ctx.Height = 60, 18
	ctx.app.width, ctx.app.height = 60, 18
	view := screen.View(60, 18)
	first := strings.TrimSpace(strings.Split(stripANSITest(view), "\n")[0])
	if first != "1 Manage  2 Users  3 Hotkeys  4 Update  5 Backups" {
		t.Fatalf("compact Manage tabs=%q", first)
	}
	visible := normalizedYaziVisibleText(view)
	for _, want := range []string{"I install", "MANAGED SOURCE"} {
		if !strings.Contains(visible, want) {
			t.Errorf("compact uninstalled Yazi view missing %q", want)
		}
	}

	ctx.Width, ctx.Height = 100, 40
	ctx.app.width, ctx.app.height = 100, 40
	view = screen.View(100, 40)
	visible = normalizedYaziVisibleText(view)
	for _, want := range []string{"I: install this tool/app", "MANAGED SOURCE"} {
		if !strings.Contains(visible, want) {
			t.Errorf("full uninstalled Yazi view missing %q", want)
		}
	}
}

func TestManageYaziCompactProvenanceIncludesPathAndLine(t *testing.T) {
	ctx, screen, imported := newManageYaziPressureScreen(t)
	ctx.app.configFieldIndex = manageYaziFieldIndex(t, ctx.app, "sort_by")
	view := normalizedYaziVisibleText(screen.View(60, 18))
	provenance := imported.Fields[tools.YaziFieldSortBy]
	for _, want := range []string{"Observed at", filepath.Base(provenance.Path) + fmt.Sprintf(":%d", provenance.Line), provenance.Key, string(provenance.Scope), filepath.Base(imported.Paths.Theme)} {
		if !strings.Contains(view, want) {
			t.Errorf("compact provenance/theme context missing %q", want)
		}
	}
}

func TestManageYaziHistoricalProductSourceUsesManagedBadge(t *testing.T) {
	ctx, screen := newManageYaziPolicyScreen(t, false, false)
	ctx.app.nativeConfigState.Yazi.Main.Ownership = tools.YaziOwnershipExactHistorical
	ctx.app.nativeConfigState.Yazi.Keymap.Ownership = tools.YaziOwnershipExactHistorical
	visible := normalizedYaziVisibleText(screen.View(ctx.Width, ctx.Height))
	if !strings.Contains(visible, "MANAGED SOURCE") || strings.Contains(visible, "NATIVE SOURCE") {
		t.Fatalf("historical product source badge is not managed-only: %q", visible)
	}
}

func TestManageYaziSaveKeyFreezesReviewedSnapshotWithoutWriting(t *testing.T) {
	app, home, paths, _ := newManageYaziConfirmFixture(t, true, false)
	oldPlan := app.pendingManageSavePlan
	app.pendingManageSavePlan = nil
	app.manageSavePlanErr = nil
	app.manageStatus = ""
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewManageScreen(ctx)
	before := snapshotManageYaziDisk(t, home, paths)

	_, cmd := screen.Update(keyMsg("s"))
	if cmd == nil {
		t.Fatal("Manage S returned no confirmation navigation")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenManageSaveConfirm {
		t.Fatalf("Manage S navigation=%#v", nav)
	}
	plan := app.pendingManageSavePlan
	if plan == nil || plan.plan == nil || plan.plan.hasBlocked() {
		t.Fatalf("Manage S plan=%#v err=%v", plan, app.manageSavePlanErr)
	}
	if plan == oldPlan {
		t.Fatal("Manage S reused the stale prebuilt plan pointer")
	}
	action := planActionByID(t, plan.plan, "config:yazi:main")
	wantTarget := planTargetPath(home, paths.Main)
	if action.Target != wantTarget || !slices.Equal(action.BackupTargets, []string{wantTarget}) || !slices.Equal(plan.plan.configTools, []string{"yazi"}) {
		t.Fatalf("frozen reviewed Yazi action=%+v tools=%v", action, plan.plan.configTools)
	}
	wantHidden := app.manageConfig.YaziShowHidden
	hash := plan.plan.hash()
	app.manageConfig.YaziShowHidden = !app.manageConfig.YaziShowHidden
	if plan.snapshot.YaziShowHidden != wantHidden || plan.plan.hash() != hash {
		t.Fatal("reviewed Manage Yazi snapshot aliases later editor changes")
	}
	assertManageYaziDiskUnchanged(t, home, paths, before)
}

func TestManageYaziCompactApplicableConfirmRoutesPreserveDisk(t *testing.T) {
	const width, height = 60, 18
	for _, key := range []string{"esc", "q"} {
		t.Run(key, func(t *testing.T) {
			app, home, paths, screen := newManageYaziConfirmFixture(t, true, false)
			before := snapshotManageYaziDisk(t, home, paths)
			pending := app.manageConfig.YaziShowHidden
			view := screen.View(width, height)
			assertYaziRenderBounds(t, view, width, height)
			helpY := labelLineY(t, strings.ToLower(stripANSITest(view)), "enter confirm")
			if helpY < 0 || helpY >= height {
				t.Fatalf("applicable state-aware footer is not pinned inside 60x18 (y=%d)", helpY)
			}
			footer := strings.ToLower(strings.Join(strings.Fields(strings.Split(stripANSITest(view), "\n")[helpY]), " "))
			for _, want := range []string{"enter confirm", "esc edit", "q cancel"} {
				if !strings.Contains(footer, want) {
					t.Errorf("applicable footer missing %q", want)
				}
			}
			_, cmd := screen.Update(keyMsg(key))
			if key == "esc" {
				if cmd == nil {
					t.Fatal("Esc returned no Manage navigation")
				}
				if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenManage {
					t.Fatalf("Esc navigation=%#v", nav)
				}
			} else {
				if cmd == nil {
					t.Fatal("q returned no quit command")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatalf("q command returned %T", cmd())
				}
			}
			if app.manageConfig.YaziShowHidden != pending {
				t.Fatal("confirmation route discarded pending Yazi edit")
			}
			assertManageYaziDiskUnchanged(t, home, paths, before)
		})
	}
}

func TestManageYaziCompactNoChangeEnterReturnsWithoutWrite(t *testing.T) {
	const width, height = 60, 18
	app, home, paths, screen := newManageYaziConfirmFixture(t, false, false)
	before := snapshotManageYaziDisk(t, home, paths)
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	_ = manageYaziConfirmHelpLine(t, view, "no changes", "enter/q close", "esc edit")
	_, cmd := screen.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("no-change Enter returned no Manage navigation")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenManage {
		t.Fatalf("no-change Enter navigation=%#v", nav)
	}
	if app.manageSaveRunning || app.manageStatus != "No changes" {
		t.Fatalf("no-change Enter running=%t status=%q", app.manageSaveRunning, app.manageStatus)
	}
	assertManageYaziDiskUnchanged(t, home, paths, before)
}

func TestManageYaziCompactBlockedNativeConfirmIsInert(t *testing.T) {
	const width, height = 60, 18
	const reason = "arbitrary native Yazi TOML is read-only in this release"
	const actionReason = "yazi.toml has native ownership: " + reason
	app, home, paths, screen := newManageYaziConfirmFixture(t, true, true)
	action := planActionByID(t, app.pendingManageSavePlan.plan, "config:yazi:main")
	if action.Disposition != operation.DispositionBlocked || action.Reason != actionReason {
		t.Fatalf("blocked Manage Yazi action disposition/reason=%s/%q, want blocked/%q", action.Disposition, action.Reason, actionReason)
	}
	before := snapshotManageYaziDisk(t, home, paths)
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	visible := strings.ToLower(normalizedYaziVisibleText(view))
	for _, want := range []string{"blocked", strings.ToLower(actionReason)} {
		if !strings.Contains(visible, want) {
			t.Errorf("blocked Manage confirmation missing %q", want)
		}
	}
	_ = manageYaziConfirmHelpLine(t, view, "blocked", "esc edit", "q cancel")
	for _, forbidden := range []string{"enter confirm", "enter/q close"} {
		if strings.Contains(visible, forbidden) {
			t.Errorf("blocked Manage confirmation advertises %q", forbidden)
		}
	}
	_, cmd := screen.Update(keyMsg("enter"))
	if cmd != nil || app.manageSaveRunning {
		t.Fatal("blocked Manage Enter was not inert")
	}
	assertManageYaziDiskUnchanged(t, home, paths, before)
}

func TestManageYaziCompactRunningConfirmInputsAreInert(t *testing.T) {
	const width, height = 60, 18
	app, home, paths, screen := newManageYaziConfirmFixture(t, true, false)
	app.manageSaveRunning = true
	app.manageStatus = "Saving reviewed changes..."
	app.manageSaveScroll = 1
	before := snapshotManageYaziDisk(t, home, paths)
	scrollBefore := app.manageSaveScroll
	planHash := app.pendingManageSavePlan.plan.hash()
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	visible := strings.ToLower(normalizedYaziVisibleText(view))
	for _, want := range []string{"saving reviewed changes", "input is paused"} {
		if !strings.Contains(visible, want) {
			t.Errorf("running Manage confirmation missing %q", want)
		}
	}
	for _, forbidden := range []string{"enter confirm", "esc edit", "q cancel"} {
		if strings.Contains(visible, forbidden) {
			t.Errorf("running Manage confirmation advertises %q", forbidden)
		}
	}
	inputs := []tea.Msg{keyMsg("esc"), keyMsg("q"), keyMsg("enter"), clickAt(30, 10), tea.MouseMsg{X: 30, Y: 10, Button: tea.MouseButtonWheelDown}}
	for _, input := range inputs {
		_, cmd := screen.Update(input)
		if cmd != nil || !app.manageSaveRunning || app.manageStatus != "Saving reviewed changes..." || app.manageSaveScroll != scrollBefore || app.pendingManageSavePlan.plan.hash() != planHash {
			t.Errorf("running Manage confirmation accepted %T: cmd=%v running=%t status=%q scroll=%d/%d hashChanged=%t", input, cmd != nil, app.manageSaveRunning, app.manageStatus, app.manageSaveScroll, scrollBefore, app.pendingManageSavePlan.plan.hash() != planHash)
		}
		assertManageYaziDiskUnchanged(t, home, paths, before)
	}
}

func manageYaziConfirmHelpLine(t *testing.T, view string, wants ...string) string {
	t.Helper()
	for _, raw := range strings.Split(stripANSITest(view), "\n") {
		line := strings.ToLower(strings.Join(strings.Fields(raw), " "))
		matched := true
		for _, want := range wants {
			matched = matched && strings.Contains(line, strings.ToLower(want))
		}
		if matched {
			return line
		}
	}
	t.Errorf("confirmation has no single pinned help line containing %q", wants)
	return ""
}

type manageYaziDiskSnapshot struct {
	tree             []string
	main, key, theme string
	mainMode         os.FileMode
	keyMode          os.FileMode
	themeMode        os.FileMode
}

func newManageYaziConfirmFixture(t *testing.T, dirty, nativeMain bool) (*App, string, tools.YaziConfigPaths, *manageSaveConfirmScreen) {
	t.Helper()
	app, home, _ := newPlanTestApp(t)
	dir := filepath.Join(home, ".config", "yazi-confirm")
	t.Setenv("YAZI_CONFIG_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := yaziConfigFrom(manageConfigToDeepDive(app.manageConfig))
	main := tools.GenerateYaziConfig(cfg, app.theme)
	if nativeMain {
		main = "[mgr]\nsort_by = \"extension\"\n"
	}
	for _, file := range []struct {
		name    string
		content string
		mode    os.FileMode
	}{
		{tools.YaziFileMain, main, 0o640},
		{tools.YaziFileKeymap, tools.GenerateYaziKeymap(cfg, app.theme), 0o600},
		{tools.YaziFileTheme, tools.GenerateYaziTheme(app.theme), 0o644},
	} {
		if err := os.WriteFile(filepath.Join(dir, file.name), []byte(file.content), file.mode); err != nil {
			t.Fatal(err)
		}
	}
	imported, err := tools.ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	app.nativeConfigState.Yazi = imported
	if nativeMain && imported.Main.Ownership != tools.YaziOwnershipNative {
		t.Fatalf("blocked fixture main ownership=%s, want native", imported.Main.Ownership)
	}
	if dirty {
		app.manageConfig.YaziShowHidden = !app.manageConfig.YaziShowHidden
	}
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || plan.plan == nil {
		t.Fatal("Manage Yazi confirmation fixture produced no reviewed plan")
	}
	if nativeMain != plan.plan.hasBlocked() {
		t.Fatalf("Manage Yazi fixture blocked=%t want=%t", plan.plan.hasBlocked(), nativeMain)
	}
	app.pendingManageSavePlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	return app, home, imported.Paths, NewManageSaveConfirmScreen(ctx)
}

func snapshotManageYaziDisk(t *testing.T, home string, paths tools.YaziConfigPaths) manageYaziDiskSnapshot {
	t.Helper()
	read := func(path string) (string, os.FileMode) {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(content), info.Mode()
	}
	main, mainMode := read(paths.Main)
	key, keyMode := read(paths.Keymap)
	theme, themeMode := read(paths.Theme)
	return manageYaziDiskSnapshot{tree: testTreeState(t, home), main: main, key: key, theme: theme, mainMode: mainMode, keyMode: keyMode, themeMode: themeMode}
}

func assertManageYaziDiskUnchanged(t *testing.T, home string, paths tools.YaziConfigPaths, want manageYaziDiskSnapshot) {
	t.Helper()
	got := snapshotManageYaziDisk(t, home, paths)
	if !slices.Equal(got.tree, want.tree) || got.main != want.main || got.key != want.key || got.theme != want.theme || got.mainMode != want.mainMode || got.keyMode != want.keyMode || got.themeMode != want.themeMode {
		t.Errorf("Manage Yazi confirmation changed HOME tree, bytes, or modes: got=%+v want=%+v", got, want)
	}
}

func manageYaziFocusedRowText(t *testing.T, view, label string) string {
	t.Helper()
	for _, line := range strings.Split(stripANSITest(view), "\n") {
		if strings.Contains(line, label) {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	t.Errorf("focused Manage row %q is not visible", label)
	return ""
}

func manageYaziHelpLineText(t *testing.T, view string) string {
	t.Helper()
	for _, line := range strings.Split(stripANSITest(view), "\n") {
		if strings.Contains(line, "S save") {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	t.Error("Manage help line containing S save is not visible")
	return ""
}

func TestStandaloneYaziCompactWritableViewportContract(t *testing.T) {
	dimensions := []struct{ width, height int }{{80, 24}, {60, 18}}
	fields := []struct {
		index int
		label string
		value string
	}{
		{0, "Keymap Style", "Vim/Yazi defaults"},
		{1, "Show Hidden Files", "OFF"},
		{2, "File Preview", "Default image-preview delay (30ms)"},
		{3, "Sort By", "Alphabetical"},
		{4, "Reverse Sort", "OFF"},
		{5, "Line Metadata", "None"},
		{6, "Scroll Offset", "5 lines"},
	}
	for _, size := range dimensions {
		for _, field := range fields {
			t.Run(fmt.Sprintf("%dx%d/field-%d", size.width, size.height, field.index), func(t *testing.T) {
				ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
				setStandaloneYaziTestSize(ctx, size.width, size.height)
				ctx.app.configFieldIndex = field.index
				view := screen.View(size.width, size.height)
				assertYaziRenderBounds(t, view, size.width, size.height)
				if assertStandaloneYaziFocusedExtentOnScreen(t, ctx.app, field.index, size.height) {
					fieldText := standaloneYaziFieldTextFromView(t, view, ctx.app, field.index)
					for _, want := range []string{field.label, field.value} {
						if !strings.Contains(fieldText, want) {
							t.Errorf("focused writable field rows missing %q", want)
						}
					}
				}
				if size.width == 60 && len(ctx.app.configFieldLayout.extents) >= 7 {
					t.Errorf("60x18 rendered %d field extents, want a real viewport", len(ctx.app.configFieldLayout.extents))
				}
				footer := standaloneYaziFooterText(t, view)
				for _, want := range []string{"↑↓", "change", "enter preview", "esc/q cancel"} {
					if !strings.Contains(footer, want) {
						t.Errorf("writable footer missing %q", want)
					}
				}
			})
		}
	}
}

func TestStandaloneYaziCompactReadOnlyRawViewportContract(t *testing.T) {
	dimensions := []struct{ width, height int }{{80, 24}, {60, 18}}
	const reason = "arbitrary native Yazi TOML is read-only in this release"
	fields := []struct {
		index int
		label string
		value string
	}{
		{0, "Keymap Style", "custom"},
		{3, "Sort By", "extension"},
		{6, "Scroll Offset", "99 lines"},
	}
	for _, size := range dimensions {
		for _, field := range fields {
			t.Run(fmt.Sprintf("%dx%d/field-%d", size.width, size.height, field.index), func(t *testing.T) {
				ctx, screen, _ := newStandaloneYaziImportedRawScreen(t,
					"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
					"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
				)
				setStandaloneYaziTestSize(ctx, size.width, size.height)
				ctx.app.configFieldIndex = field.index
				before := yaziConfigFrom(*ctx.app.deepDiveConfig)
				view := screen.View(size.width, size.height)
				assertYaziRenderBounds(t, view, size.width, size.height)
				if assertStandaloneYaziFocusedExtentOnScreen(t, ctx.app, field.index, size.height) {
					fieldText := standaloneYaziFieldTextFromView(t, view, ctx.app, field.index)
					for _, want := range []string{field.label, field.value, "(read-only)"} {
						if !strings.Contains(fieldText, want) {
							t.Errorf("focused read-only field rows missing %q", want)
						}
					}
				}
				if !strings.Contains(normalizedYaziVisibleText(view), reason) {
					t.Error("compact read-only view omitted canonical winning cause")
				}
				footer := standaloneYaziFooterText(t, view)
				for _, want := range []string{"↑↓", "enter preview", "esc/q cancel", "focused read-only"} {
					if !strings.Contains(footer, want) {
						t.Errorf("compact read-only footer missing %q", want)
					}
				}
				if strings.Contains(footer, "change") {
					t.Error("compact read-only footer advertises focused change")
				}
				if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
					t.Errorf("compact read-only render mutated config from %+v to %+v", before, got)
				}
				if size.width == 60 && len(ctx.app.configFieldLayout.extents) >= 7 {
					t.Errorf("60x18 rendered %d field extents, want a real viewport", len(ctx.app.configFieldLayout.extents))
				}
			})
		}
	}
}

func TestStandaloneYaziCompactKeyboardAndWheelKeepFocusVisible(t *testing.T) {
	const width, height = 60, 18
	labels := []string{"Keymap Style", "Show Hidden Files", "File Preview", "Sort By", "Reverse Sort", "Line Metadata", "Scroll Offset"}

	t.Run("sequential keyboard navigation", func(t *testing.T) {
		ctx, screen, _ := newStandaloneYaziImportedRawScreen(t,
			"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
			"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
		)
		setStandaloneYaziTestSize(ctx, width, height)
		before := yaziConfigFrom(*ctx.app.deepDiveConfig)
		ctx.app.configFieldIndex = 0
		for index := 0; index <= 6; index++ {
			if ctx.app.configFieldIndex != index {
				t.Errorf("down sequence focus=%d, want %d", ctx.app.configFieldIndex, index)
			}
			view := screen.View(width, height)
			assertStandaloneYaziFocusedGeometry(t, view, ctx.app, index, labels[index], height)
			if index < 6 {
				screen.Update(keyMsg("down"))
			}
		}
		for index := 6; index >= 0; index-- {
			if ctx.app.configFieldIndex != index {
				t.Errorf("up sequence focus=%d, want %d", ctx.app.configFieldIndex, index)
			}
			view := screen.View(width, height)
			assertStandaloneYaziFocusedGeometry(t, view, ctx.app, index, labels[index], height)
			if index > 0 {
				screen.Update(keyMsg("up"))
			}
		}
		if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
			t.Errorf("keyboard navigation mutated config from %+v to %+v", before, got)
		}
	})

	t.Run("wheel navigation", func(t *testing.T) {
		ctx, screen, _ := newStandaloneYaziImportedRawScreen(t,
			"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
			"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
		)
		setStandaloneYaziTestSize(ctx, width, height)
		ctx.app.configFieldIndex = 3
		before := yaziConfigFrom(*ctx.app.deepDiveConfig)
		screen.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
		view := screen.View(width, height)
		assertStandaloneYaziFocusedGeometry(t, view, ctx.app, 4, labels[4], height)
		screen.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
		view = screen.View(width, height)
		assertStandaloneYaziFocusedGeometry(t, view, ctx.app, 3, labels[3], height)
		if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
			t.Errorf("wheel navigation mutated config from %+v to %+v", before, got)
		}
	})
}

func TestStandaloneYaziCompactVisibleExtentClicksMapExactly(t *testing.T) {
	const width, height = 60, 18
	ctx, screen, _ := newStandaloneYaziImportedRawScreen(t,
		"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
		"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
	)
	setStandaloneYaziTestSize(ctx, width, height)
	ctx.app.configFieldIndex = 3
	_ = screen.View(width, height)
	var visible []fieldExtent
	for _, extent := range ctx.app.configFieldLayout.extents {
		if extent.startY >= 0 && extent.startY+extent.height <= height {
			visible = append(visible, extent)
		}
	}
	if len(visible) == 0 {
		t.Fatal("60x18 viewport exposes no clickable Yazi field extents")
	}
	if len(visible) >= 7 {
		t.Errorf("60x18 viewport exposes all %d fields; want at least one offscreen", len(visible))
	}
	for _, expected := range visible {
		t.Run(fmt.Sprintf("field-%d", expected.index), func(t *testing.T) {
			ctx, screen, _ := newStandaloneYaziImportedRawScreen(t,
				"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
				"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
			)
			setStandaloneYaziTestSize(ctx, width, height)
			ctx.app.configFieldIndex = 3
			before := yaziConfigFrom(*ctx.app.deepDiveConfig)
			_ = screen.View(width, height)
			extent := standaloneYaziFieldExtent(t, ctx.app, expected.index)
			x := (ctx.app.configFieldLayout.boxLeft + ctx.app.configFieldLayout.boxRight) / 2
			screen.Update(clickAt(x, extent.startY))
			if ctx.app.configFieldIndex != expected.index {
				t.Errorf("click focused index=%d, want %d", ctx.app.configFieldIndex, expected.index)
			}
			if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
				t.Errorf("field click mutated config from %+v to %+v", before, got)
			}
		})
	}
}

func TestStandaloneYaziCompactNoticeAndFooterClicksAreInert(t *testing.T) {
	const width, height = 60, 18
	ctx, screen, _ := newStandaloneYaziImportedRawScreen(t,
		"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
		"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
	)
	setStandaloneYaziTestSize(ctx, width, height)
	ctx.app.configFieldIndex = 3
	before := yaziConfigFrom(*ctx.app.deepDiveConfig)
	view := screen.View(width, height)
	x := (ctx.app.configFieldLayout.boxLeft + ctx.app.configFieldLayout.boxRight) / 2
	for _, row := range []struct {
		name string
		text string
	}{{"notice", "Read-only:"}, {"footer", "enter preview"}} {
		t.Run(row.name, func(t *testing.T) {
			y := labelLineY(t, view, row.text)
			if y < 0 || y >= height {
				t.Errorf("%s row %q not visible inside 60x18 (y=%d)", row.name, row.text, y)
				return
			}
			if field, found := ctx.app.configFieldLayout.fieldAt(y); found {
				t.Errorf("%s row belongs to field %d", row.name, field)
			}
			screen.Update(clickAt(x, y))
			if ctx.app.configFieldIndex != 3 {
				t.Errorf("%s click moved focus to %d, want 3", row.name, ctx.app.configFieldIndex)
			}
			if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
				t.Errorf("%s click mutated config from %+v to %+v", row.name, before, got)
			}
		})
	}
}

func TestStandaloneYaziCompactMaximumPressureNative(t *testing.T) {
	const width, height = 60, 18
	const nativeReason = "arbitrary native Yazi TOML is read-only in this release"
	ctx, screen, imported := newStandaloneYaziImportedRawScreen(t,
		"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
		"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
	)
	imported.Theme = tools.InspectYaziConfigContent(tools.YaziFileKindTheme, imported.Paths.Theme, []byte("[flavor\ndark = \"nord\"\n"), true)
	ctx.app.nativeConfigState.Yazi = imported
	setStandaloneYaziTestSize(ctx, width, height)
	ctx.app.configFieldIndex = 3
	before := yaziConfigFrom(*ctx.app.deepDiveConfig)
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	if assertStandaloneYaziFocusedExtentOnScreen(t, ctx.app, 3, height) {
		fieldText := standaloneYaziFieldTextFromView(t, view, ctx.app, 3)
		for _, want := range []string{"Sort By", "extension", "(read-only)"} {
			if !strings.Contains(fieldText, want) {
				t.Errorf("maximum-pressure focused field missing %q", want)
			}
		}
	}
	visible := normalizedYaziVisibleText(view)
	for _, want := range []string{nativeReason, "Observed at", "mgr.sort_by", "native", "Theme file", "display-only", "malformed"} {
		if !strings.Contains(visible, want) {
			t.Errorf("maximum-pressure native view missing %q", want)
		}
	}
	assertStandaloneYaziExternalRow(t, view, ctx.app, "Observed at", height)
	assertStandaloneYaziExternalRow(t, view, ctx.app, "Theme file", height)
	assertStandaloneYaziNearbyCopy(t, view, "mgr.sort_by", []string{"Observed at", "native"})
	assertStandaloneYaziPressureIndicators(t, view, ctx, screen, before, height)
	footer := standaloneYaziFooterText(t, view)
	for _, want := range []string{"↑↓", "focused read-only", "enter preview", "esc/q cancel"} {
		if !strings.Contains(footer, want) {
			t.Errorf("maximum-pressure native footer missing %q", want)
		}
	}
	if len(ctx.app.configFieldLayout.extents) >= 7 {
		t.Errorf("60x18 maximum-pressure view exposes %d extents, want fewer than 7", len(ctx.app.configFieldLayout.extents))
	}
	for _, extent := range ctx.app.configFieldLayout.extents {
		if extent.index == 7 {
			t.Fatal("maximum-pressure theme/provenance notice created field index 7")
		}
	}
}

func TestStandaloneYaziCompactMaximumPressurePreferenceError(t *testing.T) {
	const width, height = 60, 18
	const preferenceReason = "saved management preferences could not be read safely: invalid manage.json"
	const nativeReason = "arbitrary native Yazi TOML is read-only in this release"
	ctx, screen, imported := newStandaloneYaziImportedRawScreen(t,
		"[mgr]\nsort_by = \"extension\"\nlinemode = \"owner\"\nscrolloff = 99\n\n[plugin]\npreviewers = []\n",
		"[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n",
	)
	imported.Theme = tools.InspectYaziConfigContent(tools.YaziFileKindTheme, imported.Paths.Theme, []byte("[flavor\ndark = \"nord\"\n"), true)
	ctx.app.nativeConfigState.Yazi = imported
	ctx.app.nativeConfigState.PreferenceError = "invalid manage.json"
	preferred := manageConfigToDeepDive(NewManageConfig())
	ctx.app.deepDiveConfig.YaziKeymap = preferred.YaziKeymap
	ctx.app.deepDiveConfig.YaziShowHidden = preferred.YaziShowHidden
	ctx.app.deepDiveConfig.YaziPreviewMode = preferred.YaziPreviewMode
	ctx.app.deepDiveConfig.YaziSortBy = preferred.YaziSortBy
	ctx.app.deepDiveConfig.YaziSortReverse = preferred.YaziSortReverse
	ctx.app.deepDiveConfig.YaziLineMode = preferred.YaziLineMode
	ctx.app.deepDiveConfig.YaziScrollOff = preferred.YaziScrollOff
	if ctx.app.deepDiveConfig.YaziSortBy == imported.Config.SortBy {
		t.Fatal("preference pressure fixture did not separate displayed and imported sort values")
	}
	setStandaloneYaziTestSize(ctx, width, height)
	ctx.app.configFieldIndex = 3
	before := yaziConfigFrom(*ctx.app.deepDiveConfig)
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	if assertStandaloneYaziFocusedExtentOnScreen(t, ctx.app, 3, height) {
		fieldText := standaloneYaziFieldTextFromView(t, view, ctx.app, 3)
		for _, want := range []string{"Sort By", "alphabetical", "(read-only)"} {
			if !strings.Contains(fieldText, want) {
				t.Errorf("preference pressure focused field missing %q", want)
			}
		}
	}
	visible := normalizedYaziVisibleText(view)
	for _, want := range []string{preferenceReason, "Observed only (not applied)", "mgr.sort_by", "native", "Theme file", "display-only", "malformed"} {
		if !strings.Contains(visible, want) {
			t.Errorf("maximum-pressure preference view missing %q", want)
		}
	}
	if strings.Contains(visible, nativeReason) {
		t.Error("maximum-pressure preference view exposed lower-precedence native cause")
	}
	assertStandaloneYaziExternalRow(t, view, ctx.app, "Observed only", height)
	assertStandaloneYaziExternalRow(t, view, ctx.app, "Theme file", height)
	assertStandaloneYaziNearbyCopy(t, view, "mgr.sort_by", []string{"Observed only", "native"})
	assertStandaloneYaziPressureIndicators(t, view, ctx, screen, before, height)
	footer := standaloneYaziFooterText(t, view)
	for _, want := range []string{"↑↓", "focused read-only", "enter preview", "esc/q cancel"} {
		if !strings.Contains(footer, want) {
			t.Errorf("maximum-pressure preference footer missing %q", want)
		}
	}
}

func TestStandaloneYaziCompactViewportIndicatorsAreDiscoverableAndInert(t *testing.T) {
	const width, height = 60, 18
	tests := []struct {
		name      string
		focus     int
		wantAbove bool
		wantBelow bool
	}{
		{name: "top", focus: 0, wantBelow: true},
		{name: "middle", focus: 3, wantAbove: true, wantBelow: true},
		{name: "bottom", focus: 6, wantAbove: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, screen := newStandaloneYaziPolicyScreen(t, tools.YaziFileObservation{}, tools.YaziFileObservation{})
			setStandaloneYaziTestSize(ctx, width, height)
			ctx.app.configFieldIndex = test.focus
			before := yaziConfigFrom(*ctx.app.deepDiveConfig)
			view := screen.View(width, height)
			assertYaziRenderBounds(t, view, width, height)
			for _, indicator := range []struct {
				text string
				want bool
			}{{"more above", test.wantAbove}, {"more below", test.wantBelow}} {
				y := labelLineY(t, view, indicator.text)
				if indicator.want {
					if y < 0 || y >= height {
						t.Errorf("wanted indicator %q is not visible (y=%d)", indicator.text, y)
						continue
					}
					if field, found := ctx.app.configFieldLayout.fieldAt(y); found {
						t.Errorf("indicator %q row belongs to field %d", indicator.text, field)
					}
					x := (ctx.app.configFieldLayout.boxLeft + ctx.app.configFieldLayout.boxRight) / 2
					screen.Update(clickAt(x, y))
					if ctx.app.configFieldIndex != test.focus {
						t.Errorf("indicator %q click moved focus to %d, want %d", indicator.text, ctx.app.configFieldIndex, test.focus)
					}
				} else if y >= 0 {
					t.Errorf("unexpected indicator %q visible at y=%d", indicator.text, y)
				}
			}
			if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
				t.Errorf("indicator clicks mutated config from %+v to %+v", before, got)
			}
		})
	}
}

func TestStandaloneYaziCompactEditorSaveAndCancelRoutesDoNotWrite(t *testing.T) {
	const width, height = 60, 18
	for _, key := range []string{"esc", "q"} {
		t.Run(key+" cancels", func(t *testing.T) {
			app, home, _ := newPlanTestApp(t)
			prepareStandaloneYaziDirtyApp(app, false, true)
			app.configStandalone = true
			ctx := NewTestScreenContext()
			ctx.app, ctx.Width, ctx.Height = app, width, height
			app.width, app.height = width, height
			screen := NewConfigYaziScreen(ctx)
			before := testTreeState(t, home)
			view := screen.View(width, height)
			assertYaziRenderBounds(t, view, width, height)
			footer := standaloneYaziFooterText(t, view)
			for _, want := range []string{"↑↓", "change", "enter preview", "esc/q cancel"} {
				if !strings.Contains(footer, want) {
					t.Errorf("compact editor footer missing %q", want)
				}
			}
			_, cmd := screen.Update(keyMsg(key))
			if cmd == nil {
				t.Fatalf("%s returned no quit command", key)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("%s returned %T, want tea.QuitMsg", key, cmd())
			}
			if app.standaloneConfigPlan != nil || !slices.Equal(before, testTreeState(t, home)) {
				t.Fatalf("%s planned or mutated HOME", key)
			}
		})
	}

	t.Run("enter freezes keymap-only preview", func(t *testing.T) {
		app, home, _ := newPlanTestApp(t)
		prepareStandaloneYaziDirtyApp(app, false, true)
		app.configStandalone = true
		ctx := NewTestScreenContext()
		ctx.app, ctx.Width, ctx.Height = app, width, height
		app.width, app.height = width, height
		screen := NewConfigYaziScreen(ctx)
		before := testTreeState(t, home)
		view := screen.View(width, height)
		assertYaziRenderBounds(t, view, width, height)
		footer := standaloneYaziFooterText(t, view)
		for _, want := range []string{"↑↓", "change", "enter preview", "esc/q cancel"} {
			if !strings.Contains(footer, want) {
				t.Errorf("compact editor footer missing %q", want)
			}
		}
		_, cmd := screen.Update(keyMsg("enter"))
		if cmd == nil {
			t.Fatal("Enter returned no confirmation navigation")
		}
		if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenConfigSaveConfirm {
			t.Fatalf("Enter message=%#v, want config save confirmation", nav)
		}
		plan := app.standaloneConfigPlan
		if plan == nil {
			t.Fatal("Enter did not freeze a plan")
		}
		if plan.hasBlocked() || !slices.Equal(plan.configTools, []string{"yazi"}) {
			t.Fatalf("keymap-only preview plan=%#v tools=%v", plan, plan.configTools)
		}
		var applied []string
		for _, action := range plan.actions() {
			if action.Kind == operation.KindWriteConfig && action.Disposition == operation.DispositionApply {
				applied = append(applied, action.ID)
			}
		}
		if !slices.Equal(applied, []string{"config:yazi:keymap"}) {
			t.Errorf("keymap-only applied actions=%v", applied)
		}
		if !slices.Equal(before, testTreeState(t, home)) {
			t.Fatal("Enter preview mutated HOME")
		}
	})
}

func TestStandaloneYaziCompactConfirmApplicableRoutesDoNotWrite(t *testing.T) {
	const width, height = 60, 18
	for _, key := range []string{"esc", "q"} {
		t.Run(key, func(t *testing.T) {
			app, home, screen := newStandaloneYaziConfirmTest(t, true)
			before := testTreeState(t, home)
			view := screen.View(width, height)
			assertYaziRenderBounds(t, view, width, height)
			visible := normalizedYaziVisibleText(view)
			for _, want := range []string{"enter confirm", "esc edit", "q cancel"} {
				if !strings.Contains(visible, want) {
					t.Errorf("applicable confirm footer missing %q", want)
				}
			}
			_, cmd := screen.Update(keyMsg(key))
			if cmd == nil {
				t.Fatalf("%s returned no command", key)
			}
			if key == "esc" {
				if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenConfigYazi {
					t.Fatalf("Esc message=%#v, want Yazi editor", nav)
				}
			} else if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("q returned %T, want tea.QuitMsg", cmd())
			}
			if app.standaloneConfigRunning || !slices.Equal(before, testTreeState(t, home)) {
				t.Fatalf("%s executed or mutated HOME", key)
			}
		})
	}
}

func TestStandaloneYaziCompactConfirmNoChangeClosesWithoutWrite(t *testing.T) {
	const width, height = 60, 18
	app, home, screen := newStandaloneYaziConfirmTest(t, false)
	before := testTreeState(t, home)
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	visible := normalizedYaziVisibleText(view)
	for _, want := range []string{"no changes", "enter/q close", "esc edit"} {
		if !strings.Contains(visible, want) {
			t.Errorf("no-change confirm footer missing %q", want)
		}
	}
	_, cmd := screen.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("no-change Enter returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("no-change Enter returned %T, want tea.QuitMsg", cmd())
	}
	if app.standaloneConfigRunning || !slices.Equal(before, testTreeState(t, home)) {
		t.Fatal("no-change Enter executed or mutated HOME")
	}
}

func TestStandaloneYaziCompactConfirmBlockedEnterIsInert(t *testing.T) {
	const width, height = 60, 18
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, false, true)
	paths, err := tools.ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	nativeKeymap := []byte("[mgr]\nkeymap = [{ on = \"x\", run = \"noop\" }]\n")
	if err := os.WriteFile(paths.Keymap, nativeKeymap, 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := tools.ImportYaziConfig()
	if err != nil {
		t.Fatal(err)
	}
	app.nativeConfigState.Yazi = imported
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || plan == nil || !plan.hasBlocked() {
		t.Fatalf("blocked keymap plan=%#v err=%v", plan, err)
	}
	app.standaloneConfigPlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewConfigSaveConfirmScreen(ctx)
	before := testTreeState(t, home)
	keymapBefore, err := os.ReadFile(paths.Keymap)
	if err != nil {
		t.Fatal(err)
	}
	keymapInfoBefore, err := os.Stat(paths.Keymap)
	if err != nil {
		t.Fatal(err)
	}
	view := screen.View(width, height)
	assertYaziRenderBounds(t, view, width, height)
	visible := strings.ToLower(normalizedYaziVisibleText(view))
	for _, want := range []string{"blocked", "esc edit", "q cancel"} {
		if !strings.Contains(visible, want) {
			t.Errorf("blocked confirmation View omitted %q", want)
		}
	}
	if strings.Contains(visible, "enter confirm") {
		t.Error("blocked confirmation advertises enter confirm")
	}
	_, cmd := screen.Update(keyMsg("enter"))
	if cmd != nil || app.standaloneConfigRunning || !slices.Equal(before, testTreeState(t, home)) {
		t.Fatal("blocked Enter executed, ran, or mutated HOME")
	}
	keymapAfter, err := os.ReadFile(paths.Keymap)
	if err != nil {
		t.Fatal(err)
	}
	keymapInfoAfter, err := os.Stat(paths.Keymap)
	if err != nil {
		t.Fatal(err)
	}
	if string(keymapAfter) != string(keymapBefore) || keymapInfoAfter.Mode() != keymapInfoBefore.Mode() {
		t.Fatal("blocked Enter changed native keymap bytes or mode")
	}
}

func TestStandaloneYaziCompactConfirmRunningInputsAreInert(t *testing.T) {
	const width, height = 60, 18
	for _, key := range []string{"esc", "q", "enter"} {
		t.Run(key, func(t *testing.T) {
			app, home, screen := newStandaloneYaziConfirmTest(t, true)
			app.standaloneConfigRunning = true
			app.standaloneConfigStatus = "Saving reviewed configuration..."
			before := testTreeState(t, home)
			view := screen.View(width, height)
			assertYaziRenderBounds(t, view, width, height)
			visible := strings.ToLower(normalizedYaziVisibleText(view))
			for _, want := range []string{"saving reviewed configuration", "input paused"} {
				if !strings.Contains(visible, want) {
					t.Errorf("running confirmation View omitted %q", want)
				}
			}
			for _, forbidden := range []string{"enter confirm", "esc edit", "q cancel"} {
				if strings.Contains(visible, forbidden) {
					t.Errorf("running confirmation advertises %q", forbidden)
				}
			}
			_, cmd := screen.Update(keyMsg(key))
			if cmd != nil || !app.standaloneConfigRunning || !slices.Equal(before, testTreeState(t, home)) {
				t.Fatalf("running %s was not inert", key)
			}
		})
	}
}

func newStandaloneYaziConfirmTest(t *testing.T, dirty bool) (*App, string, *configSaveConfirmScreen) {
	t.Helper()
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, false, dirty)
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || plan == nil || plan.hasBlocked() {
		t.Fatalf("confirm fixture plan=%#v err=%v", plan, err)
	}
	app.standaloneConfigPlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	return app, home, NewConfigSaveConfirmScreen(ctx)
}

func assertStandaloneYaziFocusedGeometry(t *testing.T, view string, app *App, index int, label string, height int) {
	t.Helper()
	if !assertStandaloneYaziFocusedExtentOnScreen(t, app, index, height) {
		return
	}
	if fieldText := standaloneYaziFieldTextFromView(t, view, app, index); !strings.Contains(fieldText, label) {
		t.Errorf("focused field %d rows missing label %q", index, label)
	}
}

func assertStandaloneYaziExternalRow(t *testing.T, view string, app *App, text string, height int) {
	t.Helper()
	y := labelLineY(t, view, text)
	if y < 0 || y >= height {
		t.Errorf("external row %q not visible inside height=%d (y=%d)", text, height, y)
		return
	}
	if field, found := app.configFieldLayout.fieldAt(y); found {
		t.Errorf("external row %q belongs to field %d", text, field)
	}
}

func assertStandaloneYaziNearbyCopy(t *testing.T, view, anchor string, wants []string) {
	t.Helper()
	lines := strings.Split(stripANSITest(view), "\n")
	y := labelLineY(t, view, anchor)
	if y < 0 {
		t.Errorf("nearby-copy anchor %q is absent", anchor)
		return
	}
	start, end := y-1, y+2
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	nearby := normalizedYaziVisibleText(strings.Join(lines[start:end], "\n"))
	for _, want := range wants {
		if !strings.Contains(nearby, want) {
			t.Errorf("copy near %q omitted %q", anchor, want)
		}
	}
}

func assertStandaloneYaziPressureIndicators(t *testing.T, view string, ctx *ScreenContext, screen *configYaziScreen, before tools.YaziConfig, height int) {
	t.Helper()
	x := (ctx.app.configFieldLayout.boxLeft + ctx.app.configFieldLayout.boxRight) / 2
	for _, text := range []string{"more above", "more below"} {
		y := labelLineY(t, view, text)
		if y < 0 || y >= height {
			t.Errorf("pressure indicator %q not visible inside height=%d (y=%d)", text, height, y)
			continue
		}
		if field, found := ctx.app.configFieldLayout.fieldAt(y); found {
			t.Errorf("pressure indicator %q belongs to field %d", text, field)
		}
		screen.Update(clickAt(x, y))
		if ctx.app.configFieldIndex != 3 {
			t.Errorf("pressure indicator %q click moved focus to %d", text, ctx.app.configFieldIndex)
		}
		if got := yaziConfigFrom(*ctx.app.deepDiveConfig); got != before {
			t.Errorf("pressure indicator %q click mutated config from %+v to %+v", text, before, got)
		}
	}
}

func setStandaloneYaziTestSize(ctx *ScreenContext, width, height int) {
	ctx.Width, ctx.Height = width, height
	ctx.app.width, ctx.app.height = width, height
}

func assertYaziRenderBounds(t *testing.T, view string, width, height int) {
	t.Helper()
	if got := lipgloss.Height(view); got > height {
		t.Errorf("rendered height=%d exceeds terminal height=%d", got, height)
	}
	for index, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("rendered line %d width=%d exceeds terminal width=%d", index, got, width)
		}
	}
}

func assertStandaloneYaziFocusedExtentOnScreen(t *testing.T, app *App, index, height int) bool {
	t.Helper()
	extent := standaloneYaziFieldExtent(t, app, index)
	if extent.startY < 0 || extent.startY+extent.height > height {
		t.Errorf("focused field extent %+v outside terminal height=%d", extent, height)
		return false
	}
	return true
}

func standaloneYaziFieldTextFromView(t *testing.T, view string, app *App, index int) string {
	t.Helper()
	extent := standaloneYaziFieldExtent(t, app, index)
	lines := strings.Split(view, "\n")
	if extent.startY < 0 || extent.startY+extent.height > len(lines) {
		t.Fatalf("focused field extent %+v outside %d rendered rows", extent, len(lines))
	}
	return normalizedYaziVisibleText(strings.Join(lines[extent.startY:extent.startY+extent.height], "\n"))
}

func standaloneYaziFooterText(t *testing.T, view string) string {
	t.Helper()
	lines := strings.Split(stripANSITest(view), "\n")
	for _, line := range lines {
		if strings.Contains(line, "enter preview") {
			return strings.ToLower(strings.Join(strings.Fields(line), " "))
		}
	}
	t.Error("standalone Yazi footer with enter preview is not visible")
	return ""
}

func newStandaloneYaziPolicyScreen(t *testing.T, main, keymap tools.YaziFileObservation) (*ScreenContext, *configYaziScreen) {
	t.Helper()
	ctx := newDeepDiveContext(t)
	const width, height = 100, 60
	ctx.Width, ctx.Height = width, height
	ctx.app.width, ctx.app.height = width, height
	ctx.app.configStandalone = true
	ctx.app.nativeConfigState.Yazi = tools.YaziConfigImport{Main: main, Keymap: keymap}
	return ctx, NewConfigYaziScreen(ctx)
}

func compactYaziTestPath(path string) string {
	home := filepath.Clean(os.Getenv("HOME"))
	path = filepath.Clean(path)
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func assertYaziViewFitsWidth(t *testing.T, view string, width int) {
	t.Helper()
	for index, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("rendered line %d width=%d exceeds screen width=%d: %q", index, got, width, stripANSITest(line))
		}
	}
}

func normalizedYaziVisibleText(value string) string {
	return strings.Join(strings.Fields(stripANSITest(value)), " ")
}

func yaziObservation(kind tools.YaziFileKind, ownership tools.YaziFileOwnership, reason string) tools.YaziFileObservation {
	path := filepath.Join(os.Getenv("HOME"), ".config", "yazi", map[tools.YaziFileKind]string{
		tools.YaziFileKindMain: tools.YaziFileMain, tools.YaziFileKindKeymap: tools.YaziFileKeymap,
	}[kind])
	return tools.YaziFileObservation{Kind: kind, Path: path, Exists: true, Ownership: ownership, ReadOnlyReason: reason}
}

func standaloneYaziVisibleFieldLines(t *testing.T, ctx *ScreenContext, screen *configYaziScreen, index int) (string, []string) {
	t.Helper()
	view := screen.View(ctx.Width, ctx.Height)
	extent := standaloneYaziFieldExtent(t, ctx.app, index)
	viewLines := strings.Split(view, "\n")
	if extent.startY < 0 || extent.startY+extent.height > len(viewLines) {
		t.Fatalf("standalone Yazi field %d extent %+v outside %d rendered lines", index, extent, len(viewLines))
	}
	fieldLines := make([]string, 0, extent.height)
	for _, line := range viewLines[extent.startY : extent.startY+extent.height] {
		fieldLines = append(fieldLines, stripANSITest(line))
	}
	return view, fieldLines
}

func standaloneYaziFieldExtent(t *testing.T, app *App, index int) fieldExtent {
	t.Helper()
	for _, extent := range app.configFieldLayout.extents {
		if extent.index == index {
			return extent
		}
	}
	t.Fatalf("standalone Yazi field extent %d not found", index)
	return fieldExtent{}
}
