package ui

import (
	"context"
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
	for _, actionID := range []string{"config:ghostty", "config:yazi", "state:manage-preferences"} {
		found := false
		for _, action := range plan.plan.actions() {
			found = found || action.ID == actionID
		}
		if !found {
			t.Errorf("Manage plan omits %s", actionID)
		}
	}
	for _, rel := range []string{".config/yazi/yazi.toml", ".config/yazi/keymap.toml", ".config/yazi/theme.toml", planTargetPath(os.Getenv("HOME"), filepath.Join(config.ToolsDir(), "manage.json"))} {
		if !slices.Contains(plan.plan.backupTargets(), rel) {
			t.Errorf("backup scope %v omits %s", plan.plan.backupTargets(), rel)
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
	runtime.transaction.write = nil
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

func TestManageSaveStateOnlySkipsProductWriter(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.manageConfig.LazyDockerMouseMode = !app.manageConfig.LazyDockerMouseMode
	plan, err := buildManageSavePlan(app, time.Now())
	if err != nil || len(plan.plan.configTools) != 0 || !plan.saveManageState {
		t.Fatalf("plan tools=%v saveManage=%v err=%v", plan.plan.configTools, plan.saveManageState, err)
	}
	runtime := defaultManageSaveRuntime()
	writes := 0
	actualWrite := runtime.transaction.write
	runtime.transaction.write = func(toolID string, cfg DeepDiveConfig, theme string, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		writes++
		return actualWrite(toolID, cfg, theme, authority, locker)
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
	runtime.saveManage = func(*ManageConfig, safefile.Revision, *safefile.ParentChain, operation.Locker) (safefile.Revision, error) {
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
	runtime.saveGlobal = func(cfg *config.GlobalConfig, revision safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (safefile.Revision, error) {
		committed, err := actualSaveGlobal(cfg, revision, parents, locker)
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
