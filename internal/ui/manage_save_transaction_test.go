package ui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestManageSavePreviewIsNonMutatingAndSnapshotImmutable(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	app.manageStatus = "Saved ✓ from an older transaction"
	before := testTreeState(t, home)
	cmd := app.prepareManageSave()
	if !slices.Equal(before, testTreeState(t, home)) {
		t.Fatal("Manage first save key mutated HOME")
	}
	if cmd == nil {
		t.Fatal("Manage first save key returned no preview navigation")
	}
	if app.manageStatus != "" {
		t.Fatalf("new preview retained stale status %q", app.manageStatus)
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenManageSaveConfirm {
		t.Fatalf("Manage save message=%#v", nav)
	}
	accepted := app.pendingManageSavePlan.snapshot.GhosttyFontSize
	hash := app.pendingManageSavePlan.plan.hash()
	app.manageConfig.GhosttyFontSize++
	if app.pendingManageSavePlan.snapshot.GhosttyFontSize != accepted || app.pendingManageSavePlan.plan.hash() != hash {
		t.Fatal("Manage plan aliases live editor values")
	}
}

func TestManageGlowPreviewThenExecuteAdoptsSevenFieldManagedBlock(t *testing.T) {
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
	cfg := app.manageConfig
	cfg.GlowStyle = "dracula"
	cfg.GlowPager = "auto"
	cfg.GlowWidth = 0
	cfg.GlowMouse = true
	cfg.GlowAll = true
	cfg.GlowShowLineNumbers = true
	cfg.GlowPreserveNewLines = true
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil || plan.plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.plan.hasBlocked(), err)
	}
	if got, _ := os.ReadFile(path); !slices.Equal(got, native) {
		t.Fatalf("preview mutated native config: %q", got)
	}
	action := planActionByID(t, plan.plan, "config:glow")
	if action.Ownership != operation.OwnershipManagedFragment {
		t.Fatalf("ownership=%s", action.Ownership)
	}
	result := executeManageSavePlanResult(context.Background(), plan, defaultManageSaveRuntime())
	if result.err != nil || !result.applied {
		t.Fatalf("result=%+v", result)
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

func TestManageGlowCustomStyleRemainsReadOnlyAndNonMutating(t *testing.T) {
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
	app.manageConfig.GlowStyle = "/missing/custom.json"
	app.manageConfig.GlowPager = "auto"
	if _, err := buildManageSavePlan(app, time.Now()); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("custom plan error=%v", err)
	}
	if got, _ := os.ReadFile(path); !slices.Equal(got, native) {
		t.Fatalf("blocked custom preview mutated file: %q", got)
	}
}

func TestManageSavePlanExactMultiToolAndStateScope(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	app.manageConfig.YaziShowHidden = !app.manageConfig.YaziShowHidden
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil || plan.plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.plan.hasBlocked(), err)
	}
	if !slices.Equal(plan.plan.configTools, []string{"ghostty", "yazi"}) || !plan.saveManageState || plan.saveGlobalState {
		t.Fatalf("configTools=%v saveManage=%v saveGlobal=%v", plan.plan.configTools, plan.saveManageState, plan.saveGlobalState)
	}
	for _, actionID := range []string{"config:ghostty", "config:yazi:main", "state:manage-preferences"} {
		found := false
		for _, action := range plan.plan.actions() {
			found = found || action.ID == actionID
		}
		if !found {
			t.Errorf("Manage plan omits %s", actionID)
		}
	}
	for _, rel := range []string{".config/yazi/yazi.toml", planTargetPath(os.Getenv("HOME"), filepath.Join(config.ToolsDir(), "manage.json"))} {
		if !slices.Contains(plan.plan.backupTargets(), rel) {
			t.Errorf("backup scope %v omits %s", plan.plan.backupTargets(), rel)
		}
	}
	for _, rel := range []string{".config/yazi/keymap.toml", ".config/yazi/theme.toml"} {
		if slices.Contains(plan.plan.backupTargets(), rel) {
			t.Errorf("main-only Yazi change leaked backup %s into %v", rel, plan.plan.backupTargets())
		}
	}
}

func TestManageYaziChangesPlanOnlyAffectedFilesAndOmitTheme(t *testing.T) {
	for _, test := range []struct {
		name       string
		actionID   string
		file       string
		change     func(*ManageConfig)
		unexpected string
	}{
		{"show hidden", "config:yazi:main", tools.YaziFileMain, func(cfg *ManageConfig) { cfg.YaziShowHidden = !cfg.YaziShowHidden }, "config:yazi:keymap"},
		{"keymap", "config:yazi:keymap", tools.YaziFileKeymap, func(cfg *ManageConfig) {
			if cfg.YaziKeymap == "vim" {
				cfg.YaziKeymap = "emacs"
			} else {
				cfg.YaziKeymap = "vim"
			}
		}, "config:yazi:main"},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, home, _ := newPlanTestApp(t)
			override := filepath.Join(home, "reviewed-yazi-override")
			t.Setenv("YAZI_CONFIG_HOME", override)
			t.Setenv("XDG_CONFIG_HOME", "")
			test.change(app.manageConfig)
			planned, err := buildManageSavePlan(app, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			plan := planned.plan
			var selected operation.Action
			found := false
			for _, action := range plan.actions() {
				switch action.ID {
				case "config:yazi":
					t.Fatalf("legacy aggregate Yazi action remains in Manage plan: %+v", action)
				case "config:yazi:theme":
					t.Fatalf("ordinary Manage Yazi save unexpectedly planned theme: %+v", action)
				case test.unexpected:
					t.Fatalf("unaffected Yazi file action was planned: %+v", action)
				case test.actionID:
					selected, found = action, true
				}
			}
			if !found {
				t.Fatalf("affected Yazi action %s not found", test.actionID)
			}
			selectedPath := filepath.Join(override, test.file)
			wantTarget := planTargetPath(home, selectedPath)
			if selected.Disposition != operation.DispositionApply || selected.ToolID != "yazi" || selected.Ownership != operation.OwnershipManagedFile || selected.Target != wantTarget || !slices.Equal(selected.BackupTargets, []string{wantTarget}) || len(selected.Observations) != 1 || selected.Observations[0].Source != wantTarget || selected.Observations[0].Exists || selected.Observations[0].Managed {
				t.Fatalf("affected Yazi action=%+v", selected)
			}
			authority := plan.authority[selected.ID]
			accepted, ok := authority[wantTarget]
			if len(authority) != 1 || !ok || accepted.kind != acceptedFileTarget || !accepted.file.Tracked() || accepted.file.Exists() || !accepted.parents.Tracked() {
				t.Fatalf("affected Yazi authority=%+v map=%+v", accepted, authority)
			}
			if !slices.Equal(plan.configTools, []string{"yazi"}) {
				t.Fatalf("Manage logical config tools=%v, want [yazi]", plan.configTools)
			}
			mainPath := filepath.Join(override, tools.YaziFileMain)
			keymapPath := filepath.Join(override, tools.YaziFileKeymap)
			themePath := filepath.Join(override, tools.YaziFileTheme)
			if plan.yaziConfigPaths.Origin != tools.YaziConfigOriginOverride || plan.yaziConfigPaths.Dir != override || plan.yaziConfigPaths.Main != mainPath || plan.yaziConfigPaths.Keymap != keymapPath || plan.yaziConfigPaths.Theme != themePath {
				t.Fatalf("Manage frozen Yazi paths=%+v", plan.yaziConfigPaths)
			}
			themeTarget := planTargetPath(home, themePath)
			if _, ok := plan.authority["config:yazi:theme"]; ok || slices.Contains(plan.backupTargets(), themeTarget) {
				t.Fatalf("Manage plan leaked theme authority/backup: authority=%+v backups=%v", plan.authority["config:yazi:theme"], plan.backupTargets())
			}
			for actionID, scope := range plan.authority {
				if _, ok := scope[themeTarget]; ok {
					t.Fatalf("Manage authority %s leaked theme target %s", actionID, themeTarget)
				}
			}
			unaffectedPath := mainPath
			if test.file == tools.YaziFileMain {
				unaffectedPath = keymapPath
			}
			selectedCount, unaffectedCount := 0, 0
			for _, target := range plan.backupTargets() {
				switch target {
				case wantTarget:
					selectedCount++
				case planTargetPath(home, unaffectedPath):
					unaffectedCount++
				}
			}
			if selectedCount != 1 || unaffectedCount != 0 {
				t.Fatalf("Manage Yazi backup counts selected=%d unaffected=%d in %v", selectedCount, unaffectedCount, plan.backupTargets())
			}
			for _, path := range []string{override, filepath.Join(override, tools.YaziFileMain), filepath.Join(override, tools.YaziFileKeymap), themePath} {
				if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("Manage preview created %s: %v", path, statErr)
				}
			}
		})
	}
}

func TestManageNonYaziPlanIgnoresHostileYaziEnvironment(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	t.Setenv("YAZI_CONFIG_HOME", "relative-hostile-yazi")
	t.Setenv("XDG_CONFIG_HOME", "relative-hostile-xdg")
	app.manageConfig.GhosttyFontSize++
	planned, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatalf("Ghostty-only Manage plan failed on unrelated Yazi environment: %v", err)
	}
	plan := planned.plan
	action := planActionByID(t, plan, "config:ghostty")
	if action.Disposition != operation.DispositionApply || action.ToolID != "ghostty" {
		t.Fatalf("Ghostty-only Manage action=%+v", action)
	}
	if plan.yaziConfigPaths != (tools.YaziConfigPaths{}) {
		t.Fatalf("Ghostty-only Manage plan captured Yazi paths: %+v", plan.yaziConfigPaths)
	}
	if !slices.Equal(plan.configTools, []string{"ghostty"}) {
		t.Fatalf("Ghostty-only Manage config tools=%v", plan.configTools)
	}
	for _, candidate := range plan.actions() {
		if strings.HasPrefix(candidate.ID, "config:yazi") {
			t.Fatalf("Ghostty-only Manage plan emitted Yazi action: %+v", candidate)
		}
	}
	for actionID := range plan.authority {
		if strings.HasPrefix(actionID, "config:yazi") {
			t.Fatalf("Ghostty-only Manage plan captured Yazi authority: %s", actionID)
		}
	}
}

func TestManageCombinedYaziChangesPlanMainAndKeymapOnly(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	override := filepath.Join(home, "reviewed-yazi-override")
	t.Setenv("YAZI_CONFIG_HOME", override)
	t.Setenv("XDG_CONFIG_HOME", "")
	app.manageConfig.YaziShowHidden = !app.manageConfig.YaziShowHidden
	if app.manageConfig.YaziKeymap == "vim" {
		app.manageConfig.YaziKeymap = "emacs"
	} else {
		app.manageConfig.YaziKeymap = "vim"
	}
	planned, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	plan := planned.plan
	actions := map[string]operation.Action{}
	for _, action := range plan.actions() {
		switch action.ID {
		case "config:yazi":
			t.Fatalf("combined Manage save retained aggregate Yazi action: %+v", action)
		case "config:yazi:theme":
			t.Fatalf("combined Manage save planned theme: %+v", action)
		case "config:yazi:main", "config:yazi:keymap":
			actions[action.ID] = action
		}
	}
	if len(actions) != 2 {
		t.Fatalf("combined Manage Yazi actions=%v, want main+keymap", actions)
	}
	mainPath := filepath.Join(override, tools.YaziFileMain)
	keymapPath := filepath.Join(override, tools.YaziFileKeymap)
	themePath := filepath.Join(override, tools.YaziFileTheme)
	for _, item := range []struct {
		id   string
		path string
	}{{"config:yazi:main", mainPath}, {"config:yazi:keymap", keymapPath}} {
		action := actions[item.id]
		wantTarget := planTargetPath(home, item.path)
		if action.Disposition != operation.DispositionApply || action.ToolID != "yazi" || action.Ownership != operation.OwnershipManagedFile || action.Target != wantTarget || !slices.Equal(action.BackupTargets, []string{wantTarget}) || len(action.Observations) != 1 || action.Observations[0].Source != wantTarget || action.Observations[0].Exists || action.Observations[0].Managed {
			t.Fatalf("combined Manage action %s=%+v", item.id, action)
		}
		authority := plan.authority[item.id]
		accepted, ok := authority[wantTarget]
		if len(authority) != 1 || !ok || accepted.kind != acceptedFileTarget || !accepted.file.Tracked() || accepted.file.Exists() || !accepted.parents.Tracked() {
			t.Fatalf("combined Manage authority %s=%+v map=%+v", item.id, accepted, authority)
		}
		count := 0
		for _, target := range plan.backupTargets() {
			if target == wantTarget {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("combined Manage backup count for %s=%d in %v", wantTarget, count, plan.backupTargets())
		}
	}
	if !slices.Equal(plan.configTools, []string{"yazi"}) {
		t.Fatalf("combined Manage logical config tools=%v", plan.configTools)
	}
	themeTarget := planTargetPath(home, themePath)
	if slices.Contains(plan.backupTargets(), themeTarget) {
		t.Fatalf("combined Manage backups leaked theme: %v", plan.backupTargets())
	}
	for actionID, scope := range plan.authority {
		if _, ok := scope[themeTarget]; ok {
			t.Fatalf("combined Manage authority %s leaked theme", actionID)
		}
	}
	for _, path := range []string{override, mainPath, keymapPath, themePath} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("combined Manage preview created %s: %v", path, statErr)
		}
	}
}

func TestManageYaziExecutionUsesFrozenSplitAuthorities(t *testing.T) {
	for _, test := range []struct {
		name       string
		changeMain bool
		changeKey  bool
	}{{"main only", true, false}, {"keymap only", false, true}, {"combined", true, true}} {
		t.Run(test.name, func(t *testing.T) {
			app, home, _ := newPlanTestApp(t)
			configA := filepath.Join(home, "config-a")
			stateA := filepath.Join(home, "state-a")
			t.Setenv("YAZI_CONFIG_HOME", configA)
			t.Setenv("XDG_CONFIG_HOME", stateA)
			if test.changeMain {
				app.manageConfig.YaziShowHidden = !app.manageConfig.YaziShowHidden
			}
			if test.changeKey {
				if app.manageConfig.YaziKeymap == "vim" {
					app.manageConfig.YaziKeymap = "emacs"
				} else {
					app.manageConfig.YaziKeymap = "vim"
				}
			}
			planned, err := buildManageSavePlan(app, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			configB := filepath.Join(home, "config-b-override")
			configBParent := filepath.Join(home, "config-b-parent")
			t.Setenv("YAZI_CONFIG_HOME", configB)
			t.Setenv("XDG_CONFIG_HOME", configBParent)
			result := executeManageSavePlanResult(context.Background(), planned, defaultManageSaveRuntime())
			if result.err != nil || result.manualRecovery || !result.applied {
				t.Fatalf("Manage frozen Yazi execution err=%v manual=%t applied=%t warning=%q", result.err, result.manualRecovery, result.applied, result.warning)
			}
			managePathA := filepath.Join(stateA, "dotfiles", "tools", "manage.json")
			manageBytes, readErr := os.ReadFile(managePathA)
			if readErr != nil {
				t.Fatalf("frozen Manage state missing at %s: %v", managePathA, readErr)
			}
			var saved ManageConfig
			if err := json.Unmarshal(manageBytes, &saved); err != nil {
				t.Fatalf("decode frozen Manage state: %v", err)
			}
			acceptedSnapshot := planned.snapshot
			if saved != acceptedSnapshot {
				t.Fatalf("frozen Manage state=%+v, want accepted=%+v", saved, acceptedSnapshot)
			}
			cfg := yaziConfigFrom(planned.plan.config)
			selected := map[string][]byte{}
			if test.changeMain {
				selected[tools.YaziFileMain] = []byte(tools.GenerateYaziConfig(cfg, planned.plan.theme))
			}
			if test.changeKey {
				selected[tools.YaziFileKeymap] = []byte(tools.GenerateYaziKeymap(cfg, planned.plan.theme))
			}
			for _, name := range []string{tools.YaziFileMain, tools.YaziFileKeymap} {
				path := filepath.Join(configA, name)
				want, shouldExist := selected[name]
				if shouldExist {
					got, readErr := os.ReadFile(path)
					if readErr != nil || !slices.Equal(got, want) {
						t.Fatalf("frozen Manage target %s data=%q err=%v want=%q", path, got, readErr, want)
					}
				} else if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("unaffected Manage split target %s was written: %v", path, statErr)
				}
			}
			if _, statErr := os.Lstat(filepath.Join(configA, tools.YaziFileTheme)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("Manage execution wrote theme: %v", statErr)
			}
			for _, dir := range []string{configB, configBParent, filepath.Join(configBParent, "yazi")} {
				if _, statErr := os.Lstat(dir); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("Manage environment drift created directory %s: %v", dir, statErr)
				}
			}
			for _, dir := range []string{configB, filepath.Join(configBParent, "yazi")} {
				for _, name := range []string{tools.YaziFileMain, tools.YaziFileKeymap, tools.YaziFileTheme} {
					if _, statErr := os.Lstat(filepath.Join(dir, name)); !errors.Is(statErr, os.ErrNotExist) {
						t.Fatalf("Manage environment-drift target %s was written: %v", filepath.Join(dir, name), statErr)
					}
				}
			}
			for _, path := range []string{
				filepath.Join(configBParent, "dotfiles", "tools", "manage.json"),
				filepath.Join(configBParent, "dotfiles", "tools"),
				filepath.Join(configBParent, "dotfiles"),
			} {
				if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("Manage state drift created %s: %v", path, statErr)
				}
			}
		})
	}
}

func TestManageStateOnlyExecutionUsesFrozenXDGPath(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	stateA := filepath.Join(home, "state-a")
	stateB := filepath.Join(home, "state-b")
	t.Setenv("XDG_CONFIG_HOME", stateA)
	app.manageConfig.LazyDockerMouseMode = !app.manageConfig.LazyDockerMouseMode
	planned, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", stateB)
	result := executeManageSavePlanResult(context.Background(), planned, defaultManageSaveRuntime())
	if result.err != nil || result.manualRecovery || !result.applied {
		t.Fatalf("state-only frozen execution err=%v manual=%t applied=%t", result.err, result.manualRecovery, result.applied)
	}
	pathA := filepath.Join(stateA, "dotfiles", "tools", "manage.json")
	data, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	var saved ManageConfig
	if err := json.Unmarshal(data, &saved); err != nil || saved != planned.snapshot {
		t.Fatalf("state-only saved=%+v err=%v want=%+v", saved, err, planned.snapshot)
	}
	for _, path := range []string{stateB, filepath.Join(stateB, "dotfiles"), filepath.Join(stateB, "dotfiles", "tools"), filepath.Join(stateB, "dotfiles", "tools", "manage.json")} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("state-only drift created %s: %v", path, statErr)
		}
	}
}

func TestManageGlobalOnlyExecutionUsesFrozenXDGPath(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	stateA := filepath.Join(home, "state-a")
	stateB := filepath.Join(home, "state-b")
	t.Setenv("XDG_CONFIG_HOME", stateA)
	app.theme = "nord"
	planned, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", stateB)
	result := executeManageSavePlanResult(context.Background(), planned, defaultManageSaveRuntime())
	if result.err != nil || result.manualRecovery || !result.applied {
		t.Fatalf("global-only frozen execution err=%v manual=%t applied=%t", result.err, result.manualRecovery, result.applied)
	}
	pathA := filepath.Join(stateA, "dotfiles", "global.json")
	data, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatal(err)
	}
	var saved config.GlobalConfig
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode global-only state: %v", err)
	}
	wantGlobal := planned.global
	if saved.SchemaVersion != wantGlobal.SchemaVersion || saved.Theme != wantGlobal.Theme || saved.NavStyle != wantGlobal.NavStyle || saved.ActiveUser != wantGlobal.ActiveUser || saved.DisableAnimations != wantGlobal.DisableAnimations || saved.AutoBackup != wantGlobal.AutoBackup || saved.BackupMaxCount != wantGlobal.BackupMaxCount || saved.BackupMaxAgeDays != wantGlobal.BackupMaxAgeDays {
		t.Fatalf("global-only saved=%+v want=%+v", saved, wantGlobal)
	}
	for _, path := range []string{stateB, filepath.Join(stateB, "dotfiles"), filepath.Join(stateB, "dotfiles", "global.json")} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("global-only drift created %s: %v", path, statErr)
		}
	}
}

func TestManageSaveThemeOnlySchedulesGlobalAndNoProductWriter(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.theme = "nord"
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil || plan.plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.plan.hasBlocked(), err)
	}
	if len(plan.plan.configTools) != 0 || plan.saveManageState || !plan.saveGlobalState {
		t.Fatalf("theme-only tools=%v manage=%v global=%v", plan.plan.configTools, plan.saveManageState, plan.saveGlobalState)
	}
	globalRel := planTargetPath(home, filepath.Join(config.ConfigDir(), "global.json"))
	if !slices.Contains(plan.plan.backupTargets(), globalRel) {
		t.Fatalf("theme-only backup scope=%v, want %s", plan.plan.backupTargets(), globalRel)
	}
}

func TestManageSaveNeovimChangeSchedulesReviewedOverlay(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	_, optionsPath := seedReviewedNeovimInit(t, home)
	app.manageConfig.NeovimTabWidth++
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil || plan.plan.hasBlocked() || !slices.Equal(plan.plan.configTools, []string{"neovim"}) {
		t.Fatalf("Neovim plan blocked=%v tools=%v err=%v", plan != nil && plan.plan.hasBlocked(), plan.plan.configTools, err)
	}
	result := executeManageSavePlanResult(context.Background(), plan, defaultManageSaveRuntime())
	if result.err != nil || !result.applied {
		t.Fatalf("result=%+v", result)
	}
	content, err := os.ReadFile(optionsPath)
	if err != nil || !strings.Contains(string(content), "vim.opt.tabstop") {
		t.Fatalf("reviewed Manage overlay options=%q err=%v", content, err)
	}
}

func TestManageSavePreviewCancelPreservesEditsAndCompactScroll(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	app.manageConfig.YaziShowHidden = !app.manageConfig.YaziShowHidden
	app.manageConfig.BtopUpdateMs++
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app.pendingManageSavePlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewManageSaveConfirmScreen(ctx)
	first := screen.View(60, 18)
	if !strings.Contains(first, "Review Manage Save") {
		t.Fatalf("compact preview missing title:\n%s", first)
	}
	_, _ = screen.Update(tea.KeyMsg{Type: tea.KeyEnd})
	last := screen.View(60, 18)
	if !strings.Contains(last, "esc edit") {
		t.Fatalf("compact preview end is unreachable:\n%s", last)
	}
	want := app.manageConfig.GhosttyFontSize
	_, cmd := screen.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc returned no Manage navigation")
	}
	if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenManage || app.manageConfig.GhosttyFontSize != want {
		t.Fatalf("Esc message=%#v preserved=%v", nav, app.manageConfig.GhosttyFontSize == want)
	}
}

func TestManageSaveFailsClosedBeforeBackupWithoutProductWriter(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultManageSaveRuntime()
	runtime.transaction.writeAction = nil
	backupCalled := false
	runtime.transaction.backup = func(*operation.StateAuthority, []backup.Target) (autoBackupResult, error) {
		backupCalled = true
		return autoBackupResult{}, nil
	}
	result := executeManageSavePlanResult(context.Background(), plan, runtime)
	if result.err == nil || backupCalled || result.applied {
		t.Fatalf("result=%+v backupCalled=%v", result, backupCalled)
	}
}

func TestManageSaveRunningIgnoresQuitAndNavigation(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app.pendingManageSavePlan = plan
	app.manageSaveRunning = true
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewManageSaveConfirmScreen(ctx)
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'q'}}, {Type: tea.KeyCtrlC}, {Type: tea.KeyEsc}} {
		_, cmd := screen.Update(key)
		if cmd != nil {
			t.Fatalf("running transaction accepted %q", key.String())
		}
	}
	if view := screen.View(80, 24); !strings.Contains(view, "input is paused") {
		t.Fatalf("running preview lacks truthful help:\n%s", view)
	}
}

func TestCompactManageSaveLongPlanConsumesScrollState(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	app.manageConfig.TmuxHistoryLimit++
	app.manageConfig.ZshHistorySize++
	app.manageConfig.NeovimTabWidth++
	app.manageConfig.GitAliasStatus = !app.manageConfig.GitAliasStatus
	app.manageConfig.YaziShowHidden = !app.manageConfig.YaziShowHidden
	app.manageConfig.BtopUpdateMs++
	app.manageConfig.GlowMouse = !app.manageConfig.GlowMouse
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil || plan == nil || plan.plan == nil {
		t.Fatalf("long compact Manage plan=%#v err=%v", plan, err)
	}
	app.pendingManageSavePlan = plan
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewManageSaveConfirmScreen(ctx)
	const width, height = 60, 18
	hash := plan.plan.hash()

	first := screen.View(width, height)
	_, _ = screen.Update(keyMsg("down"))
	second := screen.View(width, height)
	if app.manageSaveScroll != 1 || second == first {
		t.Fatalf("compact long-plan Down scroll=%d changedView=%t", app.manageSaveScroll, second != first)
	}
	if plan.plan.hash() != hash {
		t.Fatal("compact long-plan scroll mutated reviewed plan")
	}

	_, _ = screen.Update(tea.KeyMsg{Type: tea.KeyEnd})
	last := screen.View(width, height)
	if last == first || last == second {
		t.Fatal("compact long-plan End did not expose the viewport end")
	}
	maxScroll := app.manageSaveScroll
	if maxScroll <= 1 || maxScroll == 1<<20 {
		t.Fatalf("compact long-plan End normalized scroll=%d", maxScroll)
	}
	_, _ = screen.Update(keyMsg("up"))
	beforeEnd := screen.View(width, height)
	if app.manageSaveScroll != maxScroll-1 || beforeEnd == last {
		t.Fatalf("compact long-plan Up from normalized end scroll=%d want=%d changed=%t", app.manageSaveScroll, maxScroll-1, beforeEnd != last)
	}
	for index := 0; index < maxScroll+10; index++ {
		_, _ = screen.Update(keyMsg("down"))
	}
	if bottom := screen.View(width, height); app.manageSaveScroll != maxScroll || bottom != last {
		t.Fatalf("repeated Down normalized scroll=%d want=%d bottomMatches=%t", app.manageSaveScroll, maxScroll, bottom == last)
	}
	_, _ = screen.Update(tea.KeyMsg{Type: tea.KeyHome})
	if home := screen.View(width, height); home != first || app.manageSaveScroll != 0 {
		t.Fatalf("compact long-plan Home did not restore start: scroll=%d equal=%t", app.manageSaveScroll, home == first)
	}
	wantHelp := manageSaveHelp(app)
	for _, view := range []string{first, second, last} {
		lines := strings.Split(stripANSITest(view), "\n")
		if len(lines) != height || !strings.Contains(lines[len(lines)-1], wantHelp) {
			t.Fatal("compact long-plan viewport displaced pinned state-aware help")
		}
	}
}

func TestManageSaveStateOnlySkipsProductWriter(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.manageConfig.LazyDockerMouseMode = !app.manageConfig.LazyDockerMouseMode
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil || len(plan.plan.configTools) != 0 || !plan.saveManageState {
		t.Fatalf("plan tools=%v saveManage=%v err=%v", plan.plan.configTools, plan.saveManageState, err)
	}
	runtime := defaultManageSaveRuntime()
	writes := 0
	actualWrite := runtime.transaction.writeAction
	runtime.transaction.writeAction = func(actionID, toolID string, cfg DeepDiveConfig, theme string, paths tools.YaziConfigPaths, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		writes++
		return actualWrite(actionID, toolID, cfg, theme, paths, authority, locker)
	}
	result := executeManageSavePlanResult(context.Background(), plan, runtime)
	if result.err != nil || !result.applied || writes != 0 {
		t.Fatalf("result=%+v productWrites=%d", result, writes)
	}
	persisted, err := config.LoadToolConfig("manage", NewManageConfig)
	if err != nil || persisted.LazyDockerMouseMode != app.manageConfig.LazyDockerMouseMode {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
}

func TestManageSaveStateFailureRollsBackEarlierProductWrite(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultManageSaveRuntime()
	stateErr := errors.New("injected manage state failure")
	runtime.saveManage = func(string, *ManageConfig, safefile.Revision, *safefile.ParentChain, operation.Locker) (safefile.Revision, error) {
		return safefile.Revision{}, stateErr
	}
	result := executeManageSavePlanResult(context.Background(), plan, runtime)
	if !errors.Is(result.err, stateErr) || result.manualRecovery || !strings.Contains(result.err.Error(), "automatic rollback completed") {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "ghostty", "config.ghostty")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback left Ghostty config: %v", err)
	}
	for _, path := range []string{
		filepath.Join(home, ".config", "ghostty"),
		filepath.Join(config.ToolsDir(), "manage.json"),
		config.ToolsDir(),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rollback left reviewed target or created parent %s: %v", path, err)
		}
	}
}

func TestManageSaveCommittedGlobalFailureRollsBackAllProvenWrites(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	app.manageConfig.GhosttyFontSize++
	app.theme = "nord"
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultManageSaveRuntime()
	actualSaveGlobal := runtime.saveGlobal
	committedErr := errors.New("injected global post-commit failure")
	runtime.saveGlobal = func(path string, cfg *config.GlobalConfig, revision safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (safefile.Revision, error) {
		committed, err := actualSaveGlobal(path, cfg, revision, parents, locker)
		if err != nil {
			return committed, err
		}
		return committed, &config.GlobalConfigCommittedError{Operation: "injected hook", Err: committedErr}
	}
	result := executeManageSavePlanResult(context.Background(), plan, runtime)
	if !errors.Is(result.err, committedErr) || result.manualRecovery || !strings.Contains(result.err.Error(), "automatic rollback completed") {
		t.Fatalf("result=%+v", result)
	}
	for _, path := range []string{
		filepath.Join(home, ".config", "ghostty", "config.ghostty"),
		filepath.Join(config.ToolsDir(), "manage.json"),
		filepath.Join(config.ConfigDir(), "global.json"),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rollback left %s: %v", path, err)
		}
	}
}

func TestManageSaveResultAdvancesAcceptedStateOnlyWhenApplied(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	originalBaseline := app.manageConfigBaseline
	app.manageConfig.GhosttyFontSize++
	app.theme = "nord"
	app.navStyle = "vim"
	app.animationsEnabled = false
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app.pendingManageSavePlan = plan

	// Later editor/application values must not be replaced by a rolled-back
	// accepted transaction.
	app.theme = "dracula"
	app.navStyle = "emacs"
	app.animationsEnabled = true
	ctx := NewTestScreenContext()
	ctx.app = app
	screen := NewManageSaveConfirmScreen(ctx)
	_, _ = screen.Update(manageSaveDoneMsg{standaloneConfigExecutionResult: standaloneConfigExecutionResult{err: errors.New("rolled back")}})
	if app.manageConfigBaseline != originalBaseline || app.theme != "dracula" || app.navStyle != "emacs" || !app.animationsEnabled {
		t.Fatalf("rolled-back result advanced state: baseline=%+v theme=%s nav=%s animations=%v", app.manageConfigBaseline, app.theme, app.navStyle, app.animationsEnabled)
	}

	_, _ = screen.Update(manageSaveDoneMsg{standaloneConfigExecutionResult: standaloneConfigExecutionResult{applied: true}})
	if app.manageConfigBaseline != plan.snapshot || app.theme != plan.theme || app.navStyle != plan.navStyle || app.animationsEnabled != plan.animationsEnabled {
		t.Fatalf("applied result did not advance accepted state: baseline=%+v theme=%s nav=%s animations=%v", app.manageConfigBaseline, app.theme, app.navStyle, app.animationsEnabled)
	}
}
