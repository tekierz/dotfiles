package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
