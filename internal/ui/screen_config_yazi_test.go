package ui

import (
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/tools"
)

func TestManageYaziFieldsRespectIndependentMainAndKeymapOwnership(t *testing.T) {
	const mainReason = "arbitrary native Yazi TOML is read-only"
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
					ReadOnlyReason: "arbitrary native Yazi TOML is read-only",
				},
			}},
			want:      map[string]string{"keymap": "arbitrary native Yazi TOML is read-only"},
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
						ReadOnlyReason: "arbitrary native Yazi TOML is read-only"},
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
	const mainReason = "arbitrary native Yazi TOML is read-only"
	const keymapReason = "arbitrary native Yazi TOML is read-only"
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
	const reason = "arbitrary native Yazi TOML is read-only"
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
		main.ReadOnlyReason = "arbitrary native Yazi TOML is read-only"
	}
	if blockKeymap {
		keymap.Ownership = tools.YaziOwnershipNative
		keymap.ReadOnlyReason = "arbitrary native Yazi TOML is read-only"
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
