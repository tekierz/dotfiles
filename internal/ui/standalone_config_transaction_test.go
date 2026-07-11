package ui

import (
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
	app.SetStartScreen(ScreenConfigYazi)
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
	want := []string{".config", ".config/yazi", ".config/yazi/keymap.toml", ".config/yazi/theme.toml", ".config/yazi/yazi.toml"}
	if got := app.standaloneConfigPlan.backupTargets(); !slices.Equal(got, want) {
		t.Fatalf("Yazi preview targets = %v, want %v", got, want)
	}
	if app.standaloneConfigPlan.hash() == "" || app.standaloneConfigPlan.hasBlocked() {
		t.Fatalf("preview hash=%q blocked=%v", app.standaloneConfigPlan.hash(), app.standaloneConfigPlan.hasBlocked())
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
	tests := []struct {
		screen Screen
		want   []string
	}{
		{ScreenConfigGhostty, []string{planTargetPath(home, ghosttyPath)}},
		{ScreenConfigTmux, []string{planTargetPath(home, tmuxPath)}},
		{ScreenConfigZsh, []string{".zshrc"}},
		{ScreenConfigGit, []string{".gitconfig", ".config/dotfiles/git/config"}},
		{ScreenConfigYazi, []string{".config/yazi/yazi.toml", ".config/yazi/keymap.toml", ".config/yazi/theme.toml"}},
		{ScreenConfigFzf, []string{".config/fzf/fzf.zsh"}},
		{ScreenConfigLazyGit, []string{".config/lazygit/config.yml"}},
		{ScreenConfigBtop, []string{".config/btop/btop.conf", filepath.ToSlash(filepath.Join(".config", "btop", "themes", artifact))}},
		{ScreenConfigGlow, []string{planTargetPath(home, tools.NewGlowTool().ConfigPaths()[0])}},
		{ScreenConfigClaudeCode, []string{".claude.json"}},
	}
	for _, test := range tests {
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

func TestStandaloneNeovimPlanIsVisiblyBlocked(t *testing.T) {
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
	if !strings.Contains(view, "tracked authority") {
		t.Fatalf("blocked preview omits reason:\n%s", view)
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
		{"Yazi", ScreenConfigYazi, nil},
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
	app.startScreen = ScreenConfigYazi
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
	app.startScreen = ScreenConfigYazi
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualWrite := runtime.write
	writeErr := errors.New("injected failure after multi-file commit")
	runtime.write = func(toolID string, cfg DeepDiveConfig, theme string, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		evidence, err := actualWrite(toolID, cfg, theme, authority, locker)
		if err != nil {
			return nil, err
		}
		return nil, &tools.PartialMutationError{Err: writeErr, Evidence: evidence}
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
	app.startScreen = ScreenConfigYazi
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	actualWrite := runtime.write
	runtime.write = func(toolID string, cfg DeepDiveConfig, theme string, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		evidence, err := actualWrite(toolID, cfg, theme, authority, locker)
		if err != nil {
			return nil, err
		}
		return evidence[:1], nil
	}
	err, manual := executeStandaloneConfigPlanWithRuntime(context.Background(), plan, runtime)
	if err == nil || !manual || !strings.Contains(err.Error(), "manual recovery required") {
		t.Fatalf("execute error=%v manual=%v", err, manual)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".config", "yazi", "theme.toml")); statErr != nil {
		t.Fatalf("fixture did not prove an untracked committed target remains for manual recovery: %v", statErr)
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
	actualWrite := runtime.write
	external := []byte("# external edit after commit\n")
	runtime.write = func(toolID string, cfg DeepDiveConfig, theme string, authority map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
		evidence, err := actualWrite(toolID, cfg, theme, authority, locker)
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
	app.startScreen = ScreenConfigYazi
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := defaultStandaloneConfigRuntime()
	writerCalled := false
	runtime.write = func(string, DeepDiveConfig, string, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
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
	app.startScreen = ScreenConfigYazi
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
	runtime.write = func(string, DeepDiveConfig, string, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
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
	runtime.write = func(string, DeepDiveConfig, string, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
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
	runtime.write = func(string, DeepDiveConfig, string, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error) {
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
