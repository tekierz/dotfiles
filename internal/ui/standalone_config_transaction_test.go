package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestStandaloneConfigFirstEnterBuildsExactNonMutatingPreview(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	before := testTreeState(t, home)
	cmd := app.prepareStandaloneConfigSave()
	after := testTreeState(t, home)
	if !slices.Equal(before, after) {
		t.Fatalf("first Enter mutated HOME:\nbefore=%v\nafter=%v", before, after)
	}
	if cmd == nil {
		t.Fatal("first Enter returned no preview navigation")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenConfigSaveConfirm {
		t.Fatalf("first Enter message = %#v, want config-save confirmation", nav)
	}
	want := []string{".config", ".config/yazi", ".config/yazi/keymap.toml", ".config/yazi/yazi.toml"}
	if got := app.standaloneConfigPlan.backupTargets(); !slices.Equal(got, want) {
		t.Fatalf("Yazi preview targets = %v, want %v", got, want)
	}
	if app.standaloneConfigPlan.hash() == "" || app.standaloneConfigPlan.hasBlocked() {
		t.Fatalf("preview hash=%q blocked=%v", app.standaloneConfigPlan.hash(), app.standaloneConfigPlan.hasBlocked())
	}
}

func TestStandaloneSingleSpecYaziFailsClosedToResolvedPerFilePlanner(t *testing.T) {
	_, home, _ := newPlanTestApp(t)
	spec, reason, err := standaloneConfigPlanSpec(home, "nord", DeepDiveConfig{}, "yazi", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.targets) != 0 {
		t.Fatalf("single-spec Yazi fallback exposed aggregate targets: %v", spec.targets)
	}
	if !strings.Contains(strings.ToLower(reason), "yazi") || !strings.Contains(strings.ToLower(reason), "resolved per-file") {
		t.Fatalf("single-spec Yazi fallback reason = %q, want explicit resolved per-file direction", reason)
	}
}

func TestStandaloneYaziPlansOnlyDirtyWritableGroups(t *testing.T) {
	t.Run("keymap only ignores native main", func(t *testing.T) {
		app, home, _ := newPlanTestApp(t)
		prepareStandaloneYaziDirtyApp(app, false, true)
		main := seedStandaloneYaziObservation(t, tools.YaziFileKindMain, "[mgr]\nshow_hidden = true\n")
		keymap := seedStandaloneYaziObservation(t, tools.YaziFileKindKeymap, tools.GenerateYaziKeymap(yaziConfigFrom(*app.deepDiveConfig), app.theme))
		app.nativeConfigState.Yazi = tools.YaziConfigImport{Main: main, Keymap: keymap}
		plan, err := buildStandaloneConfigPlan(app, time.Now())
		if err != nil || plan.hasBlocked() {
			t.Fatalf("keymap-only plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
		}
		assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{"config:yazi:keymap": operation.DispositionApply})
		assertStandaloneYaziScopeExcludes(t, home, plan, tools.YaziFileKindMain, tools.YaziFileKindTheme)
	})

	t.Run("main only ignores native keymap", func(t *testing.T) {
		app, home, _ := newPlanTestApp(t)
		prepareStandaloneYaziDirtyApp(app, true, false)
		main := seedStandaloneYaziObservation(t, tools.YaziFileKindMain, tools.GenerateYaziConfig(yaziConfigFrom(*app.deepDiveConfig), app.theme))
		keymap := seedStandaloneYaziObservation(t, tools.YaziFileKindKeymap, "[mgr]\nprepend_keymap = []\n")
		app.nativeConfigState.Yazi = tools.YaziConfigImport{Main: main, Keymap: keymap}
		plan, err := buildStandaloneConfigPlan(app, time.Now())
		if err != nil || plan.hasBlocked() {
			t.Fatalf("main-only plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
		}
		assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{"config:yazi:main": operation.DispositionApply})
		assertStandaloneYaziScopeExcludes(t, home, plan, tools.YaziFileKindKeymap, tools.YaziFileKindTheme)
	})

	t.Run("both groups changed omit theme", func(t *testing.T) {
		app, home, _ := newPlanTestApp(t)
		prepareStandaloneYaziDirtyApp(app, true, true)
		plan, err := buildStandaloneConfigPlan(app, time.Now())
		if err != nil || plan.hasBlocked() {
			t.Fatalf("combined plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
		}
		assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{
			"config:yazi:main": operation.DispositionApply, "config:yazi:keymap": operation.DispositionApply,
		})
		assertStandaloneYaziScopeExcludes(t, home, plan, tools.YaziFileKindTheme)
	})

	t.Run("non-default baseline compares accepted values", func(t *testing.T) {
		app, home, _ := newPlanTestApp(t)
		baseline := *NewManageConfig()
		baseline.YaziKeymap = "emacs"
		baseline.YaziShowHidden = true
		baseline.YaziPreviewMode = "never"
		baseline.YaziSortBy = "size"
		baseline.YaziSortReverse = true
		baseline.YaziLineMode = "permissions"
		baseline.YaziScrollOff = 9
		app.manageConfigBaseline = baseline
		app.manageConfigBaselineTheme = app.theme
		deep := manageConfigToDeepDive(&baseline)
		app.deepDiveConfig = &deep
		app.startScreen = ScreenConfigYazi
		app.deepDiveConfig.YaziKeymap = "vim"
		plan, err := buildStandaloneConfigPlan(app, time.Now())
		if err != nil || plan.hasBlocked() {
			t.Fatalf("non-default baseline plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
		}
		assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{"config:yazi:keymap": operation.DispositionApply})
		assertStandaloneYaziScopeExcludes(t, home, plan, tools.YaziFileKindMain, tools.YaziFileKindTheme)
	})

	t.Run("no edits produce no preview mutation scope", func(t *testing.T) {
		app, _, _ := newPlanTestApp(t)
		prepareStandaloneYaziDirtyApp(app, false, false)
		t.Setenv("YAZI_CONFIG_HOME", "relative-hostile-yazi")
		t.Setenv("XDG_CONFIG_HOME", "relative-hostile-xdg")
		app.nativeConfigState.YaziError = "stored resolver failure"
		plan, err := buildStandaloneConfigPlan(app, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{})
		if plan.hasBlocked() || len(plan.configTools) != 0 || len(plan.backupTargets()) != 0 || len(plan.authority) != 0 || plan.yaziConfigPaths != (tools.YaziConfigPaths{}) {
			t.Fatalf("no-edit plan blocked=%v tools=%v backups=%v authority=%v paths=%+v", plan.hasBlocked(), plan.configTools, plan.backupTargets(), plan.authority, plan.yaziConfigPaths)
		}
	})
}

func TestStandaloneYaziBlocksOnlyAffectedOwnedFile(t *testing.T) {
	tests := []struct {
		name       string
		changeMain bool
		kind       tools.YaziFileKind
		content    string
	}{
		{"native keymap", false, tools.YaziFileKindKeymap, "[mgr]\nprepend_keymap = []\n"},
		{"malformed main", true, tools.YaziFileKindMain, "[mgr\nshow_hidden = true\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, home, _ := newPlanTestApp(t)
			prepareStandaloneYaziDirtyApp(app, test.changeMain, !test.changeMain)
			observation := seedStandaloneYaziObservation(t, test.kind, test.content)
			if observation.ReadOnlyReason == "" {
				t.Fatalf("fixture observation=%+v has no exact reason", observation)
			}
			if test.kind == tools.YaziFileKindMain {
				app.nativeConfigState.Yazi.Main = observation
			} else {
				app.nativeConfigState.Yazi.Keymap = observation
			}
			plan, err := buildStandaloneConfigPlan(app, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			actionID := "config:yazi:" + string(test.kind)
			assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{actionID: operation.DispositionBlocked})
			action := planActionByID(t, plan, actionID)
			detail := observation.ReadOnlyReason
			if observation.Error != "" {
				detail = observation.Error
			}
			wantReason := fmt.Sprintf("%s has %s ownership: %s", filepath.Base(observation.Path), observation.Ownership, detail)
			if action.Reason != wantReason || len(plan.authority[actionID]) != 0 || slices.Contains(plan.backupTargets(), action.Target) {
				t.Fatalf("affected blocked action=%+v authority=%v backups=%v", action, plan.authority[actionID], plan.backupTargets())
			}
			excluded := []tools.YaziFileKind{tools.YaziFileKindMain, tools.YaziFileKindTheme}
			if test.kind == tools.YaziFileKindMain {
				excluded = []tools.YaziFileKind{tools.YaziFileKindKeymap, tools.YaziFileKindTheme}
			}
			assertStandaloneYaziScopeExcludes(t, home, plan, excluded...)
			assertStandaloneYaziNoParentAction(t, plan)
			if len(plan.authority) != 0 || len(plan.parentDirs) != 0 {
				t.Errorf("blocked per-file plan authority=%v parents=%v, want empty", plan.authority, plan.parentDirs)
			}
		})
	}
}

func TestStandaloneYaziGlobalErrorsBlockForcedAffectedEdit(t *testing.T) {
	tests := []struct {
		name   string
		state  NativeManageConfigState
		reason string
	}{
		{"preference error", NativeManageConfigState{PreferenceError: "invalid manage.json"}, "saved management preferences could not be read safely: invalid manage.json"},
		{"Yazi import error", NativeManageConfigState{YaziError: "resolver failed"}, "native Yazi configuration could not be imported safely: resolver failed"},
		{"preference error wins over Yazi import error", NativeManageConfigState{PreferenceError: "invalid manage.json", YaziError: "resolver failed"}, "saved management preferences could not be read safely: invalid manage.json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, _, _ := newPlanTestApp(t)
			prepareStandaloneYaziDirtyApp(app, true, false)
			app.nativeConfigState = test.state
			plan, err := buildStandaloneConfigPlan(app, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{"config:yazi:main": operation.DispositionBlocked})
			if action := planActionByID(t, plan, "config:yazi:main"); action.Reason != test.reason {
				t.Fatalf("global block reason=%q want=%q", action.Reason, test.reason)
			}
			if len(plan.authority) != 0 || len(plan.parentDirs) != 0 {
				t.Errorf("globally blocked plan authority=%v parents=%v, want empty", plan.authority, plan.parentDirs)
			}
			assertStandaloneYaziNoParentAction(t, plan)
		})
	}

	t.Run("both groups use winning preference error without mutation authority", func(t *testing.T) {
		app, _, _ := newPlanTestApp(t)
		prepareStandaloneYaziDirtyApp(app, true, true)
		app.nativeConfigState.PreferenceError = "invalid manage.json"
		app.nativeConfigState.YaziError = "resolver failed"
		plan, err := buildStandaloneConfigPlan(app, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		wantReason := "saved management preferences could not be read safely: invalid manage.json"
		assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{
			"config:yazi:main": operation.DispositionBlocked, "config:yazi:keymap": operation.DispositionBlocked,
		})
		for _, actionID := range []string{"config:yazi:main", "config:yazi:keymap"} {
			if action := planActionByID(t, plan, actionID); action.Reason != wantReason {
				t.Errorf("%s reason=%q want=%q", actionID, action.Reason, wantReason)
			}
		}
		if len(plan.configTools) != 0 || len(plan.authority) != 0 || len(plan.parentDirs) != 0 {
			t.Fatalf("blocked combined plan tools=%v authority=%v parents=%v", plan.configTools, plan.authority, plan.parentDirs)
		}
		assertStandaloneYaziNoParentAction(t, plan)
	})
}

func prepareStandaloneYaziDirtyApp(app *App, mainChanged, keymapChanged bool) {
	baseline := *NewManageConfig()
	app.manageConfigBaseline = baseline
	app.manageConfigBaselineTheme = app.theme
	deep := manageConfigToDeepDive(&baseline)
	app.deepDiveConfig = &deep
	app.startScreen = ScreenConfigYazi
	if mainChanged {
		app.deepDiveConfig.YaziShowHidden = !app.deepDiveConfig.YaziShowHidden
	}
	if keymapChanged {
		if app.deepDiveConfig.YaziKeymap == "vim" {
			app.deepDiveConfig.YaziKeymap = "emacs"
		} else {
			app.deepDiveConfig.YaziKeymap = "vim"
		}
	}
}

func seedStandaloneYaziObservation(t *testing.T, kind tools.YaziFileKind, content string) tools.YaziFileObservation {
	t.Helper()
	paths, err := tools.ResolveYaziConfigPaths()
	if err != nil {
		t.Fatal(err)
	}
	path := map[tools.YaziFileKind]string{
		tools.YaziFileKindMain: paths.Main, tools.YaziFileKindKeymap: paths.Keymap, tools.YaziFileKindTheme: paths.Theme,
	}[kind]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	observation := tools.InspectYaziConfigContent(kind, path, []byte(content), true)
	if observation.Kind != kind || !observation.Exists {
		t.Fatalf("seeded Yazi observation=%+v", observation)
	}
	return observation
}

func assertStandaloneYaziActionSet(t *testing.T, plan *installPlan, want map[string]operation.Disposition) {
	t.Helper()
	got := make(map[string]operation.Disposition)
	actualCount := 0
	applyCount := 0
	for _, action := range plan.actions() {
		if action.Kind == operation.KindWriteConfig && action.ToolID == "yazi" {
			actualCount++
			got[action.ID] = action.Disposition
			if action.Disposition == operation.DispositionApply {
				applyCount++
			}
		}
	}
	if actualCount != len(want) || !slices.Equal(sortedMapKeys(got), sortedMapKeys(want)) {
		t.Fatalf("standalone Yazi actions=%v want=%v", got, want)
	}
	for actionID, disposition := range want {
		if got[actionID] != disposition {
			t.Errorf("standalone Yazi action %s disposition=%s want=%s", actionID, got[actionID], disposition)
		}
	}
	if applyCount > 0 {
		if !slices.Equal(plan.configTools, []string{"yazi"}) {
			t.Errorf("applicable Yazi plan configTools=%v, want [yazi]", plan.configTools)
		}
	} else if len(plan.configTools) != 0 {
		t.Errorf("non-applicable Yazi plan configTools=%v, want empty", plan.configTools)
	}
}

func assertStandaloneYaziScopeExcludes(t *testing.T, home string, plan *installPlan, kinds ...tools.YaziFileKind) {
	t.Helper()
	paths := map[tools.YaziFileKind]string{
		tools.YaziFileKindMain: plan.yaziConfigPaths.Main, tools.YaziFileKindKeymap: plan.yaziConfigPaths.Keymap, tools.YaziFileKindTheme: plan.yaziConfigPaths.Theme,
	}
	for _, kind := range kinds {
		target := planTargetPath(home, paths[kind])
		if slices.Contains(plan.backupTargets(), target) {
			t.Errorf("excluded Yazi %s target leaked into backups: %v", kind, plan.backupTargets())
		}
		for actionID, authority := range plan.authority {
			if _, ok := authority[target]; ok {
				t.Errorf("excluded Yazi %s target leaked into %s authority", kind, actionID)
			}
		}
	}
}

func assertStandaloneYaziNoParentAction(t *testing.T, plan *installPlan) {
	t.Helper()
	for _, action := range plan.actions() {
		if action.ID == "state:parents" {
			t.Errorf("blocked Yazi plan emitted parent action: %+v", action)
		}
	}
	if _, ok := plan.authority["state:parents"]; ok {
		t.Errorf("blocked Yazi plan emitted parent authority: %+v", plan.authority["state:parents"])
	}
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func TestStandaloneYaziThemeOwnershipNeverBlocksOrdinaryDirtyGroups(t *testing.T) {
	tests := []struct {
		name       string
		changeMain bool
		content    string
	}{
		{"native theme with main edit", true, "[flavor]\ndark = \"catppuccin-mocha\"\n"},
		{"malformed theme with keymap edit", false, "[mgr\ncwd = {}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, home, _ := newPlanTestApp(t)
			prepareStandaloneYaziDirtyApp(app, test.changeMain, !test.changeMain)
			app.nativeConfigState.Yazi.Theme = seedStandaloneYaziObservation(t, tools.YaziFileKindTheme, test.content)
			plan, err := buildStandaloneConfigPlan(app, time.Now())
			if err != nil || plan.hasBlocked() {
				t.Fatalf("theme-isolated plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
			}
			actionID := "config:yazi:keymap"
			if test.changeMain {
				actionID = "config:yazi:main"
			}
			assertStandaloneYaziActionSet(t, plan, map[string]operation.Disposition{actionID: operation.DispositionApply})
			assertStandaloneYaziScopeExcludes(t, home, plan, tools.YaziFileKindTheme)
		})
	}
}

func TestStandaloneYaziSavePlansMainAndKeymapButOmitsTheme(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	override := filepath.Join(home, "reviewed-yazi-override")
	t.Setenv("YAZI_CONFIG_HOME", override)
	t.Setenv("XDG_CONFIG_HOME", "")
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var mainAction, keymapAction operation.Action
	foundMain, foundKeymap := false, false
	for _, action := range plan.actions() {
		switch action.ID {
		case "config:yazi":
			t.Fatalf("legacy aggregate Yazi action remains in standalone plan: %+v", action)
		case "config:yazi:main":
			mainAction, foundMain = action, true
		case "config:yazi:keymap":
			keymapAction, foundKeymap = action, true
		case "config:yazi:theme":
			t.Fatalf("ordinary standalone Yazi save unexpectedly planned theme: %+v", action)
		}
	}
	if !foundMain || !foundKeymap {
		t.Fatalf("standalone Yazi split actions found main=%t keymap=%t", foundMain, foundKeymap)
	}
	mainPath := filepath.Join(override, tools.YaziFileMain)
	keymapPath := filepath.Join(override, tools.YaziFileKeymap)
	themePath := filepath.Join(override, tools.YaziFileTheme)
	if plan.yaziConfigPaths.Origin != tools.YaziConfigOriginOverride || plan.yaziConfigPaths.Dir != override || plan.yaziConfigPaths.Main != mainPath || plan.yaziConfigPaths.Keymap != keymapPath || plan.yaziConfigPaths.Theme != themePath {
		t.Fatalf("standalone frozen Yazi paths=%+v", plan.yaziConfigPaths)
	}
	for _, item := range []struct {
		action operation.Action
		path   string
	}{
		{mainAction, mainPath},
		{keymapAction, keymapPath},
	} {
		wantTarget := planTargetPath(home, item.path)
		if item.action.Disposition != operation.DispositionApply || item.action.ToolID != "yazi" || item.action.Ownership != operation.OwnershipManagedFile || item.action.Target != wantTarget || !slices.Equal(item.action.BackupTargets, []string{wantTarget}) || len(item.action.Observations) != 1 || item.action.Observations[0].Source != wantTarget || item.action.Observations[0].Exists || item.action.Observations[0].Managed {
			t.Fatalf("standalone split action=%+v", item.action)
		}
		authority := plan.authority[item.action.ID]
		accepted, ok := authority[wantTarget]
		if len(authority) != 1 || !ok || accepted.kind != acceptedFileTarget || !accepted.file.Tracked() || accepted.file.Exists() || !accepted.parents.Tracked() {
			t.Fatalf("standalone split authority=%+v map=%+v", accepted, authority)
		}
	}
	if !slices.Equal(plan.configTools, []string{"yazi"}) {
		t.Fatalf("standalone logical config tools=%v, want [yazi]", plan.configTools)
	}
	themeTarget := planTargetPath(home, themePath)
	if _, ok := plan.authority["config:yazi:theme"]; ok || slices.Contains(plan.backupTargets(), themeTarget) {
		t.Fatalf("ordinary standalone plan leaked theme authority/backup: authority=%+v backups=%v", plan.authority["config:yazi:theme"], plan.backupTargets())
	}
	for actionID, scope := range plan.authority {
		if _, ok := scope[themeTarget]; ok {
			t.Fatalf("standalone authority %s leaked theme target %s", actionID, themeTarget)
		}
	}
	for _, path := range []string{override, mainPath, keymapPath, themePath} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("standalone preview created %s: %v", path, statErr)
		}
	}
}

func TestStandaloneNonYaziPlanIgnoresHostileYaziEnvironment(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigFzf
	t.Setenv("YAZI_CONFIG_HOME", "relative-hostile-yazi")
	t.Setenv("XDG_CONFIG_HOME", "relative-hostile-xdg")
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatalf("non-Yazi standalone plan failed on unrelated Yazi environment: %v", err)
	}
	action := planActionByID(t, plan, "config:fzf")
	if action.Disposition != operation.DispositionApply || action.ToolID != "fzf" {
		t.Fatalf("non-Yazi standalone action=%+v", action)
	}
	if plan.yaziConfigPaths != (tools.YaziConfigPaths{}) {
		t.Fatalf("non-Yazi standalone plan captured Yazi paths: %+v", plan.yaziConfigPaths)
	}
	for _, candidate := range plan.actions() {
		if strings.HasPrefix(candidate.ID, "config:yazi") {
			t.Fatalf("non-Yazi standalone plan emitted Yazi action: %+v", candidate)
		}
	}
	for actionID := range plan.authority {
		if strings.HasPrefix(actionID, "config:yazi") {
			t.Fatalf("non-Yazi standalone plan captured Yazi authority: %s", actionID)
		}
	}
}

func TestStandaloneYaziExecuteUsesFrozenSplitAuthoritiesAndOmitsTheme(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	configA := filepath.Join(home, "config-a")
	t.Setenv("YAZI_CONFIG_HOME", configA)
	t.Setenv("XDG_CONFIG_HOME", "")
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	configB := filepath.Join(home, "config-b-override")
	configBParent := filepath.Join(home, "config-b-parent")
	t.Setenv("YAZI_CONFIG_HOME", configB)
	t.Setenv("XDG_CONFIG_HOME", configBParent)
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, defaultStandaloneConfigRuntime())
	if err != nil || manual {
		t.Fatalf("standalone frozen Yazi execution err=%v manual=%t", err, manual)
	}
	wantCfg := yaziConfigFrom(plan.config)
	want := map[string][]byte{
		filepath.Join(configA, tools.YaziFileMain):   []byte(tools.GenerateYaziConfig(wantCfg, plan.theme)),
		filepath.Join(configA, tools.YaziFileKeymap): []byte(tools.GenerateYaziKeymap(wantCfg, plan.theme)),
	}
	for path, content := range want {
		got, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(got, content) {
			t.Fatalf("frozen standalone target %s data=%q err=%v want=%q", path, got, readErr, content)
		}
	}
	if _, statErr := os.Lstat(filepath.Join(configA, tools.YaziFileTheme)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("ordinary standalone execution mutated frozen theme: %v", statErr)
	}
	for _, dir := range []string{configB, filepath.Join(configBParent, "yazi")} {
		for _, name := range []string{tools.YaziFileMain, tools.YaziFileKeymap, tools.YaziFileTheme} {
			if _, statErr := os.Lstat(filepath.Join(dir, name)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("environment-drift target %s was mutated: %v", filepath.Join(dir, name), statErr)
			}
		}
	}
	for _, dir := range []string{configB, configBParent, filepath.Join(configBParent, "yazi")} {
		if _, statErr := os.Lstat(dir); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("environment drift created directory %s: %v", dir, statErr)
		}
	}
}

func TestStandaloneGlowPreviewThenExecuteAdoptsSevenFieldManagedBlock(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	t.Setenv("GLOW_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path, err := tools.GlowConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	native := []byte("style: /missing/custom.json\ncustom: keep\n")
	if err := os.WriteFile(path, native, 0o600); err != nil {
		t.Fatal(err)
	}
	app.startScreen = ScreenConfigGlow
	app.deepDiveConfig.CLITools["glow"] = true
	cfg := app.deepDiveConfig
	cfg.GlowStyle = "dracula"
	cfg.GlowPager = "auto"
	cfg.GlowWidth = 0
	cfg.GlowMouse = true
	cfg.GlowAll = true
	cfg.GlowShowLineNumbers = true
	cfg.GlowPreserveNewLines = true
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, native) {
		t.Fatalf("preview mutated native config: %q", got)
	}
	action := planActionByID(t, plan, "config:glow")
	if action.Ownership != operation.OwnershipManagedFragment {
		t.Fatalf("ownership=%s", action.Ownership)
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, defaultStandaloneConfigRuntime())
	if err != nil || manual {
		t.Fatalf("execute err=%v manual=%v", err, manual)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"custom: keep\n", "style: \"dracula\"", "pager: true", "width: 0", "mouse: true", "all: true", "showLineNumbers: true", "preserveNewLines: true"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("result missing %q:\n%s", want, got)
		}
	}
}

func TestStandaloneGlowCustomStyleRemainsReadOnlyAndNonMutating(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	t.Setenv("GLOW_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path, err := tools.GlowConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	native := []byte("style: /missing/custom.json\ncustom: keep\n")
	if err := os.WriteFile(path, native, 0o600); err != nil {
		t.Fatal(err)
	}
	app.startScreen = ScreenConfigGlow
	app.deepDiveConfig.GlowStyle = "/missing/custom.json"
	if _, err := buildStandaloneConfigPlan(app, time.Now()); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("custom plan error=%v", err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, native) {
		t.Fatalf("blocked custom preview mutated file: %q", got)
	}
}

func TestStandaloneConfigPlansAuthorityCapableWriterTargets(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.deepDiveConfig.CLITools["lazygit"] = true
	app.deepDiveConfig.CLITools["btop"] = true
	app.deepDiveConfig.CLITools["glow"] = true
	app.deepDiveConfig.ClaudeCodeMCPs["context7"] = true
	ghosttyPath, err := tools.GhosttyConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	tmuxPath, err := tools.TmuxConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	artifact := tools.BtopThemeArtifactName(btopConfigFrom(*app.deepDiveConfig), app.theme) + ".theme"
	lazyGitPath, err := tools.LazyGitConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		screen Screen
		want   []string
	}{
		{ScreenConfigGhostty, []string{planTargetPath(home, ghosttyPath)}},
		{ScreenConfigTmux, []string{planTargetPath(home, tmuxPath)}},
		{ScreenConfigZsh, []string{".zshrc"}},
		{ScreenConfigGit, []string{".gitconfig", ".config/dotfiles/git/config"}},
		{ScreenConfigYazi, []string{".config/yazi/yazi.toml", ".config/yazi/keymap.toml"}},
		{ScreenConfigFzf, []string{".config/fzf/fzf.zsh"}},
		{ScreenConfigLazyGit, []string{planTargetPath(home, lazyGitPath)}},
		{ScreenConfigBtop, []string{".config/btop/btop.conf", filepath.ToSlash(filepath.Join(".config", "btop", "themes", artifact))}},
		{ScreenConfigGlow, []string{planTargetPath(home, tools.NewGlowTool().ConfigPaths()[0])}},
		{ScreenConfigClaudeCode, []string{".claude.json"}},
	}
	for _, test := range tests {
		if test.screen == ScreenConfigYazi {
			prepareStandaloneYaziDirtyApp(app, true, true)
			app.deepDiveConfig.CLITools["lazygit"] = true
			app.deepDiveConfig.CLITools["btop"] = true
			app.deepDiveConfig.CLITools["glow"] = true
			app.deepDiveConfig.ClaudeCodeMCPs["context7"] = true
		}
		app.startScreen = test.screen
		plan, err := buildStandaloneConfigPlan(app, time.Now())
		if err != nil || plan.hasBlocked() {
			t.Fatalf("screen %d plan blocked=%v err=%v", test.screen, plan != nil && plan.hasBlocked(), err)
		}
		for _, target := range test.want {
			if !slices.Contains(plan.backupTargets(), target) {
				t.Errorf("screen %d targets %v omit %s", test.screen, plan.backupTargets(), target)
			}
		}
	}
}

func TestStandaloneNeovimMissingInitPlanIsVisiblyBlocked(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || !plan.hasBlocked() {
		t.Fatalf("Neovim plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}
	viewApp := app
	viewApp.standaloneConfigPlan = plan
	ctx := NewTestScreenContext()
	ctx.app = viewApp
	view := NewConfigSaveConfirmScreen(ctx).View(90, 30)
	if !strings.Contains(view, "existing regular init.lua") {
		t.Fatalf("blocked preview omits reason:\n%s", view)
	}
}

func seedReviewedNeovimInit(t *testing.T, home string) (string, string) {
	t.Helper()
	initPath := filepath.Join(home, ".config", "nvim", "init.lua")
	optionsPath := filepath.Join(home, ".config", "nvim", "lua", "custom", "options.lua")
	if err := os.MkdirAll(filepath.Dir(initPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initPath, []byte("-- user init\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return initPath, optionsPath
}

func TestStandaloneNeovimPlanHasExactTargetsAndMissingParents(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	initPath, _ := seedReviewedNeovimInit(t, home)
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || plan.hasBlocked() || !slices.Equal(plan.configTools, []string{"neovim"}) {
		t.Fatalf("plan blocked=%v tools=%v err=%v", plan != nil && plan.hasBlocked(), plan.configTools, err)
	}
	wantTargets := []string{".config/nvim/init.lua", ".config/nvim/lua/custom/options.lua"}
	for _, target := range wantTargets {
		if !slices.Contains(plan.backupTargets(), target) {
			t.Errorf("backup targets %v omit %s", plan.backupTargets(), target)
		}
	}
	for _, parent := range []string{".config/nvim/lua", ".config/nvim/lua/custom"} {
		if !slices.Contains(plan.parentDirs, parent) {
			t.Errorf("parent plan %v omits %s", plan.parentDirs, parent)
		}
	}
	if got, _ := os.ReadFile(initPath); string(got) != "-- user init\n" {
		t.Fatalf("planning changed init.lua: %q", got)
	}
}

func TestStandaloneNeovimPlanBlocksUnmanagedOptions(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	_, optionsPath := seedReviewedNeovimInit(t, home)
	if err := os.MkdirAll(filepath.Dir(optionsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(optionsPath, []byte("vim.opt.wrap = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || !plan.hasBlocked() || len(plan.configTools) != 0 {
		t.Fatalf("plan blocked=%v tools=%v err=%v", plan != nil && plan.hasBlocked(), plan.configTools, err)
	}
	if action := planActionByID(t, plan, "config:neovim"); !strings.Contains(action.Reason, "not marked") {
		t.Fatalf("unmanaged options reason=%q", action.Reason)
	}
}

func TestStandaloneNeovimPlanBlocksSymlinkInit(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	victim := filepath.Join(home, "victim.lua")
	if err := os.WriteFile(victim, []byte("-- victim\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	initPath := filepath.Join(home, ".config", "nvim", "init.lua")
	if err := os.MkdirAll(filepath.Dir(initPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, initPath); err != nil {
		t.Fatal(err)
	}
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || !plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}
	if action := planActionByID(t, plan, "config:neovim"); !strings.Contains(action.Reason, "regular file") {
		t.Fatalf("symlink init reason=%q", action.Reason)
	}
	if got, _ := os.ReadFile(victim); string(got) != "-- victim\n" {
		t.Fatalf("planning changed symlink victim: %q", got)
	}
}

func TestStandaloneNeovimReviewedOverlayRealSuccess(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	initPath, optionsPath := seedReviewedNeovimInit(t, home)
	app.startScreen = ScreenConfigNeovim
	app.deepDiveConfig.NeovimTabWidth = 8
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result := executeStandaloneConfigPlanResult(context.Background(), plan, defaultStandaloneConfigRuntime())
	if result.err != nil || !result.applied {
		t.Fatalf("result=%+v", result)
	}
	initContent, _ := os.ReadFile(initPath)
	optionsContent, _ := os.ReadFile(optionsPath)
	if !strings.Contains(string(initContent), ">>> dotfiles neovim (managed)") || !strings.Contains(string(optionsContent), "vim.opt.tabstop = 8") {
		t.Fatalf("init:\n%s\noptions:\n%s", initContent, optionsContent)
	}
}

func TestStandaloneNeovimBackupFailureWritesNothing(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	initPath, optionsPath := seedReviewedNeovimInit(t, home)
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	runtime.backup = func(*operation.StateAuthority, []backup.Target) (autoBackupResult, error) {
		return autoBackupResult{}, errors.New("injected backup failure")
	}
	result := executeStandaloneConfigPlanResult(context.Background(), plan, runtime)
	if result.err == nil {
		t.Fatal("backup failure succeeded")
	}
	if got, _ := os.ReadFile(initPath); string(got) != "-- user init\n" {
		t.Fatalf("backup failure changed init.lua: %q", got)
	}
	if _, err := os.Lstat(optionsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup failure created options.lua: %v", err)
	}
}

func TestStandaloneNeovimConcurrentEditBeforeBackupIsPreserved(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	initPath, optionsPath := seedReviewedNeovimInit(t, home)
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	external := []byte("-- external edit\n")
	if err := os.WriteFile(initPath, external, 0o600); err != nil {
		t.Fatal(err)
	}
	backupCalled := false
	runtime := defaultStandaloneConfigRuntime()
	runtime.backup = func(*operation.StateAuthority, []backup.Target) (autoBackupResult, error) {
		backupCalled = true
		return autoBackupResult{}, nil
	}
	result := executeStandaloneConfigPlanResult(context.Background(), plan, runtime)
	if result.err == nil || backupCalled {
		t.Fatalf("result=%+v backupCalled=%v", result, backupCalled)
	}
	if got, _ := os.ReadFile(initPath); !bytes.Equal(got, external) {
		t.Fatalf("stale refusal changed external edit: %q", got)
	}
	if _, err := os.Lstat(optionsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale refusal created options.lua: %v", err)
	}
}

func TestStandaloneNeovimPostcommitFailureRollsBackBothTargetsAndParents(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	initPath, optionsPath := seedReviewedNeovimInit(t, home)
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualWrite := runtime.writeAction
	failure := errors.New("injected failure after Neovim overlay commit")
	runtime.writeAction = func(actionID, toolID string, cfg DeepDiveConfig, theme string, paths tools.YaziConfigPaths, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		evidence, err := actualWrite(actionID, toolID, cfg, theme, paths, authority, locker)
		if err != nil {
			return nil, err
		}
		return nil, &tools.PartialMutationError{Err: failure, Evidence: evidence}
	}
	result := executeStandaloneConfigPlanResult(context.Background(), plan, runtime)
	if !errors.Is(result.err, failure) || result.manualRecovery || !strings.Contains(result.err.Error(), "automatic rollback completed") {
		t.Fatalf("result=%+v", result)
	}
	if got, _ := os.ReadFile(initPath); string(got) != "-- user init\n" {
		t.Fatalf("rollback did not restore init.lua: %q", got)
	}
	if _, err := os.Lstat(optionsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback left options.lua: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "nvim", "lua")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback left created lua parent: %v", err)
	}
}

func TestStandaloneNeovimUnprovenInitCommitRollsBackOptionsAndRequiresManualRecovery(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	initPath, optionsPath := seedReviewedNeovimInit(t, home)
	app.startScreen = ScreenConfigNeovim
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualWrite := runtime.writeAction
	unknownCommit := &safefile.CommittedError{Operation: "injected unproven init commit", Err: errors.New("revision lost")}
	runtime.writeAction = func(actionID, toolID string, cfg DeepDiveConfig, theme string, paths tools.YaziConfigPaths, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		evidence, err := actualWrite(actionID, toolID, cfg, theme, paths, authority, locker)
		if err != nil {
			return nil, err
		}
		return nil, &tools.PartialMutationError{Err: unknownCommit, Evidence: evidence[:1]}
	}
	result := executeStandaloneConfigPlanResult(context.Background(), plan, runtime)
	if !result.manualRecovery || result.err == nil || !strings.Contains(result.err.Error(), "manual recovery required") {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Lstat(optionsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("proven options write was not rolled back: %v", err)
	}
	if initContent, _ := os.ReadFile(initPath); !strings.Contains(string(initContent), ">>> dotfiles neovim (managed)") {
		t.Fatalf("fixture did not leave unproven init mutation for manual review:\n%s", initContent)
	}
}

func TestStandalonePreviewSnapshotIsImmutableAndCancelReturnsToEditor(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.SetStartScreen(ScreenConfigZsh)
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	hash := plan.hash()
	app.deepDiveConfig.ZshHistorySize++
	if plan.hash() != hash || plan.config.ZshHistorySize == app.deepDiveConfig.ZshHistorySize {
		t.Fatal("accepted standalone plan aliases live editor state")
	}
	app.standaloneConfigPlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewConfigSaveConfirmScreen(ctx)
	_, cmd := screen.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc returned no editor navigation")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenConfigZsh {
		t.Fatalf("Esc message = %#v, want original editor", nav)
	}
}

func TestStandaloneEditorEscapeQuitsWithoutPlanningOrMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		screen Screen
		new    func(*ScreenContext) ScreenHandler
	}{
		{"field navigation", ScreenConfigGhostty, func(ctx *ScreenContext) ScreenHandler { return NewConfigGhosttyScreen(ctx) }},
		{"Claude", ScreenConfigClaudeCode, func(ctx *ScreenContext) ScreenHandler { return NewConfigClaudeCodeScreen(ctx) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, home, _ := newPlanTestApp(t)
			app.SetStartScreen(test.screen)
			ctx := NewTestScreenContext()
			ctx.app = app
			screen := test.new(ctx)
			before := testTreeState(t, home)
			_, cmd := screen.Update(tea.KeyMsg{Type: tea.KeyEsc})
			if cmd == nil {
				t.Fatal("Esc returned no quit command")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("Esc returned %T, want tea.QuitMsg", cmd())
			}
			if app.standaloneConfigPlan != nil || !slices.Equal(before, testTreeState(t, home)) {
				t.Fatal("Esc planned or mutated standalone configuration")
			}
		})
	}
}

func TestStandaloneConfigTransactionSingleFileSuccess(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigZsh
	app.deepDiveConfig.ZshHistorySize = 4321
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}
	if err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, defaultStandaloneConfigRuntime()); err != nil || manual {
		t.Fatalf("execute error=%v manual=%v", err, manual)
	}
	data, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil || !strings.Contains(string(data), "HISTSIZE=4321") {
		t.Fatalf("saved zsh config=%q err=%v", data, err)
	}
}

func TestStandaloneSupportedWritersExecuteTheirReviewedTargets(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
		setup  func(*App)
	}{
		{"Ghostty", ScreenConfigGhostty, nil},
		{"tmux", ScreenConfigTmux, func(app *App) { app.deepDiveConfig.TmuxTPMEnabled = false }},
		{"zsh", ScreenConfigZsh, nil},
		{"Git", ScreenConfigGit, nil},
		{"Yazi", ScreenConfigYazi, func(app *App) { prepareStandaloneYaziDirtyApp(app, true, true) }},
		{"fzf", ScreenConfigFzf, nil},
		{"LazyGit", ScreenConfigLazyGit, func(app *App) { app.deepDiveConfig.CLITools["lazygit"] = true }},
		{"btop", ScreenConfigBtop, func(app *App) { app.deepDiveConfig.CLITools["btop"] = true }},
		{"Glow", ScreenConfigGlow, func(app *App) { app.deepDiveConfig.CLITools["glow"] = true }},
		{"Claude", ScreenConfigClaudeCode, func(app *App) { app.deepDiveConfig.ClaudeCodeMCPs["context7"] = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, home, _ := newPlanTestApp(t)
			t.Setenv("PATH", "")
			app.startScreen = test.screen
			if test.setup != nil {
				test.setup(app)
			}
			plan, err := buildStandaloneConfigPlan(app, time.Now())
			if err != nil || plan.hasBlocked() || len(plan.configTools) != 1 {
				t.Fatalf("plan blocked=%v tools=%v err=%v", plan != nil && plan.hasBlocked(), plan.configTools, err)
			}
			err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, defaultStandaloneConfigRuntime())
			if err != nil || manual {
				t.Fatalf("execute error=%v manual=%v", err, manual)
			}
			for _, action := range plan.actions() {
				if action.Kind != operation.KindWriteConfig || action.Disposition != operation.DispositionApply {
					continue
				}
				for _, rel := range action.BackupTargets {
					info, statErr := os.Lstat(filepath.Join(home, filepath.FromSlash(rel)))
					if statErr != nil || !info.Mode().IsRegular() {
						t.Errorf("reviewed writer target %s was not committed as a regular file: info=%v err=%v", rel, info, statErr)
					}
				}
			}
		})
	}
}

func TestStandaloneConfigBackupFailureStopsBeforeEveryTarget(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	backupErr := errors.New("injected backup failure")
	runtime := defaultStandaloneConfigRuntime()
	runtime.backup = func(*operation.StateAuthority, []backup.Target) (autoBackupResult, error) {
		return autoBackupResult{}, backupErr
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if !errors.Is(err, backupErr) || manual {
		t.Fatalf("execute error=%v manual=%v", err, manual)
	}
	for _, rel := range plan.backupTargets() {
		if _, statErr := os.Lstat(filepath.Join(home, filepath.FromSlash(rel))); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("backup failure mutated %s: %v", rel, statErr)
		}
	}
}

func TestStandaloneConfigPostPreviewChangeBlocksBeforeBackup(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigFzf
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".config", "fzf", "fzf.zsh")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupCalled := false
	runtime := defaultStandaloneConfigRuntime()
	runtime.backup = func(*operation.StateAuthority, []backup.Target) (autoBackupResult, error) {
		backupCalled = true
		return autoBackupResult{}, nil
	}
	err, _ = executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if err == nil || backupCalled {
		t.Fatalf("changed target error=%v backupCalled=%v", err, backupCalled)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "external\n" {
		t.Fatalf("stale-plan refusal changed external target: %q", data)
	}
}

func TestStandaloneConfigMultiFileFailureRollsBackProvenWrites(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualWrite := runtime.writeAction
	writeErr := errors.New("injected keymap action failure after main commit")
	runtime.writeAction = func(actionID, toolID string, cfg DeepDiveConfig, theme string, paths tools.YaziConfigPaths, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		if actionID == "config:yazi:keymap" {
			return nil, writeErr
		}
		return actualWrite(actionID, toolID, cfg, theme, paths, authority, locker)
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if !errors.Is(err, writeErr) || manual || !strings.Contains(err.Error(), "automatic rollback completed") {
		t.Fatalf("execute error=%v manual=%v", err, manual)
	}
	for _, rel := range plan.backupTargets() {
		if _, statErr := os.Lstat(filepath.Join(home, filepath.FromSlash(rel))); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("rollback left created target %s: %v", rel, statErr)
		}
	}
}

func TestStandaloneConfigMissingEvidenceRequiresManualRecovery(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualWrite := runtime.writeAction
	runtime.writeAction = func(actionID, toolID string, cfg DeepDiveConfig, theme string, paths tools.YaziConfigPaths, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		evidence, err := actualWrite(actionID, toolID, cfg, theme, paths, authority, locker)
		if err != nil {
			return nil, err
		}
		if actionID == "config:yazi:main" {
			return nil, nil
		}
		return evidence, nil
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if err == nil || !manual || !strings.Contains(err.Error(), "manual recovery required") {
		t.Fatalf("execute error=%v manual=%v", err, manual)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".config", "yazi", "yazi.toml")); statErr != nil {
		t.Fatalf("fixture did not prove committed main remains for manual recovery: %v", statErr)
	}
	for _, rel := range []string{".config/yazi/keymap.toml", ".config/yazi/theme.toml"} {
		if _, statErr := os.Lstat(filepath.Join(home, filepath.FromSlash(rel))); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("missing-evidence execution unexpectedly wrote %s: %v", rel, statErr)
		}
	}
}

func TestStandaloneConfigConcurrentEditIsPreserved(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigZsh
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualWrite := runtime.writeAction
	external := []byte("# external edit after commit\n")
	runtime.writeAction = func(actionID, toolID string, cfg DeepDiveConfig, theme string, paths tools.YaziConfigPaths, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		evidence, err := actualWrite(actionID, toolID, cfg, theme, paths, authority, locker)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(home, ".zshrc"), external, 0o600); err != nil {
			return nil, err
		}
		return nil, &tools.PartialMutationError{Err: fmt.Errorf("post-write failure"), Evidence: evidence}
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if err == nil || !manual {
		t.Fatalf("execute error=%v manual=%v", err, manual)
	}
	data, readErr := os.ReadFile(filepath.Join(home, ".zshrc"))
	if readErr != nil || !slices.Equal(data, external) {
		t.Fatalf("conditional rollback overwrote concurrent edit: %q err=%v", data, readErr)
	}
}

func TestStandaloneClaudeNoChangeClosesWithoutMutation(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.SetStartScreen(ScreenConfigClaudeCode)
	for name := range app.deepDiveConfig.ClaudeCodeMCPs {
		app.deepDiveConfig.ClaudeCodeMCPs[name] = false
	}
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil || plan.hasBlocked() || len(plan.configTools) != 0 {
		t.Fatalf("no-change plan blocked=%v tools=%v err=%v", plan != nil && plan.hasBlocked(), plan.configTools, err)
	}
	app.standaloneConfigPlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewConfigSaveConfirmScreen(ctx)
	before := testTreeState(t, home)
	_, cmd := screen.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no-change Enter returned no close command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("no-change Enter returned %T, want tea.QuitMsg", cmd())
	}
	if !slices.Equal(before, testTreeState(t, home)) {
		t.Fatal("no-change confirmation mutated HOME")
	}
	view := screen.View(90, 30)
	if strings.Contains(view, "Backup: mandatory") || !strings.Contains(view, "no changes") {
		t.Fatalf("no-change preview is misleading:\n%s", view)
	}
}

func TestStandaloneParentCommittedErrorRequiresManualRecovery(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	writerCalled := false
	runtime.writeAction = func(string, string, DeepDiveConfig, string, tools.YaziConfigPaths, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
		writerCalled = true
		return nil, nil
	}
	runtime.ensureParent = func(string, string, *safefile.DirectorySnapshot, *safefile.ParentChain, os.FileMode) (*safefile.DirectorySnapshot, error) {
		return nil, &safefile.CommittedError{Operation: "injected mkdir postcondition", Err: errors.New("lost directory evidence")}
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if err == nil || !manual || writerCalled || !strings.Contains(err.Error(), "manual recovery required") {
		t.Fatalf("execute error=%v manual=%v writerCalled=%v", err, manual, writerCalled)
	}
}

func TestStandalonePrecommitParentErrorDoesNotClaimManualRecovery(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	prepareStandaloneYaziDirtyApp(app, true, true)
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	precommit := errors.New("injected parent validation failure")
	writerCalled := false
	runtime.ensureParent = func(string, string, *safefile.DirectorySnapshot, *safefile.ParentChain, os.FileMode) (*safefile.DirectorySnapshot, error) {
		return nil, precommit
	}
	runtime.writeAction = func(string, string, DeepDiveConfig, string, tools.YaziConfigPaths, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
		writerCalled = true
		return nil, nil
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if !errors.Is(err, precommit) || manual || writerCalled || strings.Contains(err.Error(), "manual recovery") {
		t.Fatalf("execute error=%v manual=%v writerCalled=%v", err, manual, writerCalled)
	}
}

func TestStandalonePrecommitWriterErrorDoesNotClaimManualRecovery(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigZsh
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	precommit := errors.New("injected writer preflight failure")
	runtime.writeAction = func(string, string, DeepDiveConfig, string, tools.YaziConfigPaths, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
		return nil, precommit
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if !errors.Is(err, precommit) || manual || strings.Contains(err.Error(), "manual recovery") {
		t.Fatalf("execute error=%v manual=%v", err, manual)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".zshrc")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("precommit writer error changed target: %v", statErr)
	}
}

func TestStandaloneLockReleaseFailureIsSurfacedWithoutRewritingSuccess(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigZsh
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	releaseErr := errors.New("injected operation lock release failure")
	runtime.acquire = func(*operation.StateAuthority, string, string) (func() error, error) {
		return func() error { return releaseErr }, nil
	}
	result := executeStandaloneConfigPlanResult(context.Background(), plan, runtime)
	if !errors.Is(result.err, releaseErr) || result.manualRecovery || !result.applied || !strings.Contains(result.err.Error(), "applied and preserved") {
		t.Fatalf("execute result=%+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".zshrc")); statErr != nil {
		t.Fatalf("operational release failure rewrote successful product config: %v", statErr)
	}
	app.standaloneConfigPlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewConfigSaveConfirmScreen(ctx)
	_, _ = screen.Update(standaloneConfigSaveDoneMsg{err: result.err, applied: result.applied})
	if !app.standaloneConfigDone || strings.Contains(screen.View(90, 30), "enter confirm") {
		t.Fatal("applied operational error did not enter a terminal result state")
	}
	_, cmd := screen.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("applied operational error cannot be closed")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("applied operational error Enter returned %T, want tea.QuitMsg", cmd())
	}
}

func TestStandaloneBackupRetentionWarningIsVisibleAndNonfatal(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigZsh
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualBackup := runtime.backup
	cleanupErr := errors.New("injected retention cleanup failure")
	runtime.backup = func(state *operation.StateAuthority, targets []backup.Target) (autoBackupResult, error) {
		result, err := actualBackup(state, targets)
		result.cleanupErr = cleanupErr
		return result, err
	}
	result := executeStandaloneConfigPlanResult(context.Background(), plan, runtime)
	if result.err != nil || !result.applied || !strings.Contains(result.warning, cleanupErr.Error()) {
		t.Fatalf("execute result=%+v", result)
	}
	app.standaloneConfigPlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewConfigSaveConfirmScreen(ctx)
	_, _ = screen.Update(standaloneConfigSaveDoneMsg{applied: true, warning: result.warning})
	view := screen.View(90, 30)
	if !strings.Contains(view, "saved with a backup-retention warning") || !strings.Contains(view, "injected retention cleanup") {
		t.Fatalf("warning is not durable in success screen:\n%s", view)
	}
}

func TestStandaloneFailureAndLockReleaseErrorsAreBothPreserved(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.startScreen = ScreenConfigZsh
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	writeErr := errors.New("injected precommit writer failure")
	releaseErr := errors.New("injected release failure")
	releaseCalls := 0
	runtime.writeAction = func(string, string, DeepDiveConfig, string, tools.YaziConfigPaths, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
		return nil, writeErr
	}
	runtime.acquire = func(*operation.StateAuthority, string, string) (func() error, error) {
		return func() error {
			releaseCalls++
			return releaseErr
		}, nil
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if !errors.Is(err, writeErr) || !errors.Is(err, releaseErr) || manual || releaseCalls != 1 {
		t.Fatalf("execute error=%v manual=%v releaseCalls=%d", err, manual, releaseCalls)
	}
}

func testTreeState(t *testing.T, root string) []string {
	t.Helper()
	var state []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		state = append(state, rel+":"+info.Mode().String())
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	slices.Sort(state)
	return state
}
