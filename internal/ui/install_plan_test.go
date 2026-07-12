package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/scripts"
	"github.com/tekierz/dotfiles/internal/tools"
)

type installHookManager struct {
	pkg.PackageManager
	hook func()
}

func (m *installHookManager) InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if m.hook != nil {
		m.hook()
		m.hook = nil
	}
	return m.PackageManager.InstallStreaming(ctx, packages...)
}

func planActionByID(t *testing.T, plan *installPlan, id string) operation.Action {
	t.Helper()
	for _, action := range plan.actions() {
		if action.ID == id {
			return action
		}
	}
	t.Fatalf("plan action %q not found", id)
	return operation.Action{}
}

func newPlanTestApp(t *testing.T) (*App, string, toolInstallRuntime) {
	t.Helper()
	home := withTempHome(t)
	app := NewApp(true)
	app.manageInstalledReady = true
	app.installCacheLoading = false
	app.manageInstalled = map[string]bool{}
	runtime := registryRuntime(pkg.PlatformMacOS, app.manageInstalled)
	runtime.backupTargets = func([]backup.Target) (autoBackupResult, error) { return autoBackupResult{}, nil }
	return app, home, runtime
}

func TestBuildInstallPlanContainsCoreInstallsAndExactConfigActions(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	now := time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC)
	plan, err := buildInstallPlan(app, runtime, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.hash() == "" || plan.hasBlocked() {
		t.Fatalf("fresh install plan identity/blocked = hash %q blocked %v", plan.hash(), plan.hasBlocked())
	}
	for _, id := range alwaysConfiguredToolIDs {
		if action := planActionByID(t, plan, "install:"+id); action.Disposition != operation.DispositionApply {
			t.Fatalf("core install %s disposition = %s", id, action.Disposition)
		}
	}
	for _, id := range []string{"tmux", "ghostty", "zsh", "neovim", "git", "yazi", "fzf"} {
		if action := planActionByID(t, plan, "config:"+id); action.Disposition != operation.DispositionApply {
			t.Fatalf("core config %s disposition = %s", id, action.Disposition)
		}
	}
	for _, action := range plan.actions() {
		if action.Target == "dotfiles" || strings.HasSuffix(action.Target, "/dotfiles") {
			t.Fatalf("plan schedules a self-copied main binary: %+v", action)
		}
	}
}

func TestInstallPlanHashCoversDesiredSettingsAndTheme(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	now := time.Date(2026, 7, 10, 20, 5, 0, 0, time.UTC)
	first, err := buildInstallPlan(app, runtime, now)
	if err != nil {
		t.Fatal(err)
	}
	app.deepDiveConfig.GhosttyOpacity--
	second, err := buildInstallPlan(app, runtime, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.hash() == second.hash() {
		t.Fatal("Ghostty desired-state change did not change exact plan hash")
	}
	app.deepDiveConfig.GhosttyOpacity++
	app.theme = "nord"
	third, err := buildInstallPlan(app, runtime, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.hash() == third.hash() {
		t.Fatal("theme change did not change exact plan hash")
	}
}

func TestInstallPlanTargetsMatchDynamicWriters(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	app.deepDiveConfig.CLITools["btop"] = true
	app.deepDiveConfig.CLITools["glow"] = true
	app.theme = "nord"
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	btop := planActionByID(t, plan, "config:btop")
	if !slices.Contains(btop.BackupTargets, ".config/btop/themes/nord.theme") {
		t.Fatalf("btop plan targets do not match theme writer: %v", btop.BackupTargets)
	}
	glow := planActionByID(t, plan, "config:glow")
	if glow.Ownership != operation.OwnershipManagedFragment {
		t.Fatalf("Glow ownership=%s", glow.Ownership)
	}
	if goruntime.GOOS == "darwin" && !slices.Contains(glow.BackupTargets, "Library/Preferences/glow/glow.yml") {
		t.Fatalf("macOS Glow plan target = %v", glow.BackupTargets)
	}
	path, err := tools.GlowConfigMutationPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := planTargetPath(home, path); len(glow.BackupTargets) != 1 || glow.BackupTargets[0] != want {
		t.Fatalf("Glow plan path=%v, writer path=%s", glow.BackupTargets, want)
	}
	tmux := planActionByID(t, plan, "config:tmux")
	for _, target := range []string{".tmux/plugins/tpm"} {
		if !slices.Contains(tmux.BackupTargets, target) {
			t.Errorf("tmux side-effect target %q missing from %v", target, tmux.BackupTargets)
		}
	}
}

func TestInstallPlanLazyGitUsesExactConfigDirTargetAndManagedFileOwnership(t *testing.T) {
	home := withTempHome(t)
	configDir := filepath.Join(home, "reviewed", "lazygit")
	t.Setenv("CONFIG_DIR", configDir)
	t.Setenv("LG_CONFIG_FILE", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	app := NewApp(true)
	app.manageInstalledReady = true
	app.installCacheLoading = false
	app.manageInstalled = map[string]bool{"delta": true}
	app.deepDiveConfig.CLITools["lazygit"] = true
	runtime := registryRuntime(pkg.PlatformMacOS, app.manageInstalled)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action := planActionByID(t, plan, "config:lazygit")
	want := "reviewed/lazygit/config.yml"
	if action.Disposition != operation.DispositionApply || action.Ownership != operation.OwnershipManagedFile || !slices.Equal(action.BackupTargets, []string{want}) {
		t.Fatalf("LazyGit action = %+v", action)
	}
	if accepted := plan.authority[action.ID][want]; accepted.kind != acceptedFileTarget || !accepted.file.Tracked() || !accepted.parents.Tracked() {
		t.Fatalf("LazyGit authority = %+v", accepted)
	}
}

func TestInstallPlanLazyGitArbitraryNativeIsVisiblyBlockedAtCurrentBytes(t *testing.T) {
	home := withTempHome(t)
	configDir := filepath.Join(home, ".config", "lazygit")
	t.Setenv("CONFIG_DIR", configDir)
	t.Setenv("LG_CONFIG_FILE", "")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.yml")
	native := []byte("gui:\n  mouseEvents: false\ncustom: keep\n")
	if err := os.WriteFile(path, native, 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	app.manageInstalledReady = true
	app.installCacheLoading = false
	app.manageInstalled = map[string]bool{"delta": true}
	app.deepDiveConfig.CLITools["lazygit"] = true
	plan, err := buildInstallPlan(app, registryRuntime(pkg.PlatformMacOS, app.manageInstalled), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action := planActionByID(t, plan, "config:lazygit")
	if action.Disposition != operation.DispositionBlocked || !strings.Contains(action.Reason, "arbitrary native LazyGit YAML") {
		t.Fatalf("LazyGit action = %+v", action)
	}
	if got, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(got, native) {
		t.Fatalf("blocked plan mutated native bytes: %q, %v", got, readErr)
	}
}

func TestInstallPlanBlocksMalformedNativeGlowAtExactDynamicPath(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	app.deepDiveConfig.CLITools["glow"] = true
	t.Setenv("GLOW_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path := filepath.Join(home, ".config", "glow", "glow.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("style: dark\nSTYLE: light\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action := planActionByID(t, plan, "config:glow")
	if action.Disposition != operation.DispositionBlocked || !strings.Contains(action.Reason, "cannot be merged safely") {
		t.Fatalf("Glow action=%+v", action)
	}
	if len(action.BackupTargets) != 1 || action.BackupTargets[0] != ".config/glow/glow.yml" {
		t.Fatalf("Glow backups=%v", action.BackupTargets)
	}
}

func TestInstallPlanRejectsGlowRuntimeSettingOverride(t *testing.T) {
	for _, env := range []string{"GLOW_STYLE", "GLAMOUR_STYLE"} {
		t.Run(env, func(t *testing.T) {
			app, _, runtime := newPlanTestApp(t)
			app.deepDiveConfig.CLITools["glow"] = true
			t.Setenv("GLOW_STYLE", "")
			t.Setenv("GLAMOUR_STYLE", "")
			t.Setenv(env, "dark")
			if _, err := buildInstallPlan(app, runtime, time.Now()); err == nil || !strings.Contains(err.Error(), env) {
				t.Fatalf("runtime override plan error=%v", err)
			}
		})
	}
}

func TestReviewedInstallerWritesBtopToAcceptedXDGTargets(t *testing.T) {
	home := withTempHome(t)
	xdg := filepath.Join(home, "xdg-config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "btop", "themes"), 0o700); err != nil {
		t.Fatal(err)
	}
	installed := make(map[string]bool)
	for _, tool := range tools.GetRegistry().All() {
		installed[tool.ID()] = true
	}
	app := NewApp(true)
	app.manageInstalledReady = true
	app.manageInstalled = installed
	app.deepDiveConfig.CLITools["btop"] = true
	runtime := registryRuntime(pkg.PlatformMacOS, installed)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	btopAction := planActionByID(t, plan, "config:btop")
	wantConfigRel := "xdg-config/btop/btop.conf"
	wantThemeRel := "xdg-config/btop/themes/catppuccin-mocha.theme"
	if !slices.Contains(btopAction.BackupTargets, wantConfigRel) || !slices.Contains(btopAction.BackupTargets, wantThemeRel) {
		t.Fatalf("reviewed XDG targets = %v", btopAction.BackupTargets)
	}
	keep := map[string]bool{"state:parents": true, "state:global": true, "config:btop": true}
	var actions []operation.Action
	authority := make(map[string]map[string]acceptedTarget)
	for _, action := range plan.actions() {
		if keep[action.ID] || action.Kind == operation.KindInstallFile {
			actions = append(actions, action)
			authority[action.ID] = plan.authority[action.ID]
		}
	}
	document, err := operation.NewPlan(time.Now(), actions)
	if err != nil {
		t.Fatal(err)
	}
	reviewed := &installPlan{
		document:      document,
		configTools:   []string{"btop"},
		selectedTools: []string{},
		config:        plan.config,
		theme:         plan.theme,
		navStyle:      plan.navStyle,
		animations:    plan.animations,
		globalConfig:  plan.globalConfig,
		authority:     authority,
		parentDirs:    plan.parentDirs,
		statePlan:     plan.statePlan,
	}
	runtime.backupTargetsWithState = backupPlanTargetsWithState
	runtime.backupTargets = nil
	events := make(chan installEventMsg, 256)
	runInstallWorkerFromPlanWithRuntime(context.Background(), events, reviewed, runtime, true)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err != nil {
		t.Fatalf("reviewed installer result = %v\n%s", terminal.err, terminal.context)
	}
	for _, path := range []string{filepath.Join(home, filepath.FromSlash(wantConfigRel)), filepath.Join(home, filepath.FromSlash(wantThemeRel))} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("reviewed XDG target was not written: %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "btop", "btop.conf")); !os.IsNotExist(err) {
		t.Fatalf("installer wrote inactive HOME fallback: %v", err)
	}
}

func TestInstallExecutionUsesExactAcceptedGhosttyTarget(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	accepted := plan.ghosttyConfigTarget
	if accepted == "" {
		t.Fatal("plan did not retain accepted Ghostty target")
	}
	// A later-created legacy source would win a fresh resolver call. Execution
	// must still use the exact destination reviewed in the accepted plan.
	later := filepath.Join(home, ".config", "ghostty", "config")
	if later == accepted {
		later = filepath.Join(home, "Library", "Application Support", "com.mitchellh.ghostty", "config")
	}
	if err := os.MkdirAll(filepath.Dir(later), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("font-size = 31\n# arrived after preview\n")
	if err := os.WriteFile(later, original, 0o600); err != nil {
		t.Fatal(err)
	}
	plan.configTools = []string{"ghostty"}
	plan.selectedTools = []string{}
	installed := map[string]bool{"ghostty": true}
	runtime = registryRuntime(pkg.PlatformMacOS, installed)
	events := make(chan installEventMsg, 64)
	runInstallWorkerFromPlanWithRuntime(context.Background(), events, plan, runtime, false)
	for range events {
	}
	got, err := os.ReadFile(accepted)
	if err != nil || !strings.Contains(string(got), "dotfiles ghostty (managed)") {
		t.Fatalf("accepted target was not written: %q err=%v", got, err)
	}
	if got, err := os.ReadFile(later); err != nil || !slices.Equal(got, original) {
		t.Fatalf("later source changed instead of accepted target: %q err=%v", got, err)
	}
}

func TestAcceptedGhosttyTargetIsBoundToHashedPlanAction(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.acceptedGhosttyConfigTarget(); err != nil {
		t.Fatalf("valid accepted target was rejected: %v", err)
	}
	plan.ghosttyConfigTarget += ".redirected"
	if _, err := plan.acceptedGhosttyConfigTarget(); err == nil {
		t.Fatal("execution target mutation outside hashed action was accepted")
	}
}

func TestBuildInstallPlanAcceptsNativeTmuxFragmentTarget(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	path := filepath.Join(home, ".tmux.conf")
	if err := os.WriteFile(path, []byte("# user's tmux config\nset -g mouse off\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action := planActionByID(t, plan, "config:tmux")
	if action.Disposition != operation.DispositionApply || action.Ownership != operation.OwnershipManagedFragment {
		t.Fatalf("native tmux action = %+v", action)
	}
	if !slices.Contains(action.BackupTargets, ".tmux.conf") {
		t.Fatalf("native tmux action omitted rollback target: %+v", action)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "# user's tmux config\nset -g mouse off\n" {
		t.Fatalf("planning mutated native config: %q err=%v", content, err)
	}
}

func TestBuildInstallPlanBlocksAmbiguousNativeTmuxImport(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	app.nativeConfigState.TmuxError = "line 2 can change effective settings through indirection"
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action := planActionByID(t, plan, "config:tmux")
	if action.Disposition != operation.DispositionBlocked || !strings.Contains(action.Reason, "could not be imported safely") {
		t.Fatalf("ambiguous tmux action = %+v", action)
	}
}

func TestBuildInstallPlanBacksUpExistingManagedTmuxFile(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	path := filepath.Join(home, ".tmux.conf")
	if err := os.WriteFile(path, []byte("# Generated by dotfiles TUI\nset -g mouse on\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action := planActionByID(t, plan, "config:tmux")
	if action.Disposition != operation.DispositionApply || !action.Observation.Exists || !slices.Contains(action.BackupTargets, ".tmux.conf") {
		t.Fatalf("managed tmux action = %+v", action)
	}
	found := false
	for _, target := range plan.backupTargets() {
		if target == ".tmux.conf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("plan backup scope omits managed tmux config: %v", plan.backupTargets())
	}
}

func TestBuildInstallPlanAcceptsExactLegacyYaziThemeMigration(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	path := filepath.Join(home, ".config", "yazi", "theme.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Theme: dracula (generated by dotfiles)\n[mgr]\ncwd = {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if action := planActionByID(t, plan, "config:yazi"); action.Disposition != operation.DispositionApply {
		t.Fatalf("exact legacy Yazi migration was blocked: %+v", action)
	}
}

func TestBuildInstallPlanBlocksUnownedOrphanGitManagedDestination(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	path := filepath.Join(home, filepath.FromSlash(gitManagedConfigRelForPlan))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[user]\n\tname = User Owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if action := planActionByID(t, plan, "config:git"); action.Disposition != operation.DispositionBlocked {
		t.Fatalf("unowned orphan Git managed destination was accepted: %+v", action)
	}
}

func TestInstallPlanRevalidationRejectsPostPreviewTargetCreation(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".tmux.conf"), []byte("user arrived after preview\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := revalidateInstallPlan(plan); err == nil || !strings.Contains(err.Error(), "changed after preview") {
		t.Fatalf("revalidation error = %v, want post-preview change refusal", err)
	}
}

func TestInstallPlanRevalidationRejectsSameContentReplacementAndMetadataChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string, content []byte)
	}{
		{
			name: "same bytes new inode",
			mutate: func(t *testing.T, path string, content []byte) {
				t.Helper()
				old := path + ".old"
				if err := os.Rename(path, old); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "mode only",
			mutate: func(t *testing.T, path string, _ []byte) {
				t.Helper()
				if err := os.Chmod(path, 0o640); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hardlink count",
			mutate: func(t *testing.T, path string, _ []byte) {
				t.Helper()
				if err := os.Link(path, path+".link"); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, home, runtime := newPlanTestApp(t)
			path := filepath.Join(home, ".tmux.conf")
			content := []byte("# Generated by dotfiles TUI\nset -g mouse on\n")
			if err := os.WriteFile(path, content, 0o600); err != nil {
				t.Fatal(err)
			}
			plan, err := buildInstallPlan(app, runtime, time.Now())
			if err != nil || plan.hasBlocked() {
				t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
			}
			test.mutate(t, path, content)
			if err := revalidateInstallPlan(plan); err == nil || !strings.Contains(err.Error(), "changed after preview") {
				t.Fatalf("revalidation error = %v, want exact-authority refusal", err)
			}
		})
	}
}

func TestInstallPlanAuthorityAccessorsAreActionScoped(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.acceptedFileRevision("config:tmux", ".tmux.conf"); err != nil {
		t.Fatalf("accepted tmux authority: %v", err)
	}
	if _, err := plan.acceptedFileRevision("config:zsh", ".tmux.conf"); err == nil {
		t.Fatal("cross-action path gained accepted authority")
	}
	if _, err := plan.acceptedDirectorySnapshot("config:tmux", ".tmux.conf"); err == nil {
		t.Fatal("file authority was accepted as a directory")
	}
}

func TestBuildInstallPlanBlocksHelperCollisionButAcceptsExactOwnedBytes(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	app.deepDiveConfig.Utilities["hk"] = true
	path := filepath.Join(home, ".local", "bin", "hk")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho user-owned\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if action := planActionByID(t, plan, "helper:hk"); action.Disposition != operation.DispositionBlocked {
		t.Fatalf("colliding helper action = %+v", action)
	}

	if err := os.WriteFile(path, []byte(scripts.HKScript), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err = buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if action := planActionByID(t, plan, "helper:hk"); action.Disposition != operation.DispositionApply {
		t.Fatalf("exact owned helper action = %+v", action)
	}
}

func TestFileTreeRefusesBlockedPlan(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.manageInstalledReady = true
	ctx.app.installCacheLoading = false
	path := filepath.Join(os.Getenv("HOME"), ".config", "yazi", "yazi.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user config\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	screen := NewFileTreeScreen(ctx)
	_ = screen.Init()
	if ctx.app.pendingInstallPlan == nil || !ctx.app.pendingInstallPlan.hasBlocked() {
		t.Fatal("fixture did not create blocked plan")
	}
	_, cmd := screen.Update(keyMsg("enter"))
	if cmd != nil {
		t.Fatal("blocked plan navigated to execution")
	}
	if !strings.Contains(screen.View(ctx.Width, ctx.Height), "Resolve blocked ownership") {
		t.Fatal("blocked plan did not render resolution guidance")
	}
}

func TestFileTreeViewportCanReachFinalReviewedLine(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.manageInstalledReady = true
	ctx.app.installCacheLoading = false
	screen := NewFileTreeScreen(ctx)
	_ = screen.Init()
	if ctx.app.pendingInstallPlan == nil || ctx.app.pendingInstallPlan.hasBlocked() {
		t.Fatalf("fixture plan unavailable or blocked: plan=%v err=%v", ctx.app.pendingInstallPlan, ctx.app.installPlanError)
	}
	const shortHeight = 18
	initial := screen.View(ctx.Width, shortHeight)
	if strings.Contains(initial, "Verified rollback scope") {
		t.Fatal("fixture is not long enough to exercise viewport scrolling")
	}
	_, _ = screen.Update(keyMsg("end"))
	lastPage := screen.View(ctx.Width, shortHeight)
	if !strings.Contains(lastPage, "Verified rollback scope") {
		t.Fatalf("final reviewed line is unreachable at viewport end:\n%s", lastPage)
	}
}

func TestInstallPlanWorkerJournalsExactReviewedHash(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	global, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	global.AutoBackup = false
	if err := config.SaveGlobalConfig(global); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.GetRegistry().All() {
		app.manageInstalled[tool.ID()] = true
	}
	runtime = registryRuntime(pkg.PlatformMacOS, app.manageInstalled)
	runtime.backupTargets = func(targets []backup.Target) (autoBackupResult, error) {
		if len(targets) == 0 {
			t.Fatal("fresh plan omitted newly-created paths from rollback scope")
		}
		backupDir := filepath.Join(home, ".config", "dotfiles", "backups", "test-plan")
		result, err := backup.CreatePlanTracked(home, backupDir, targets)
		return autoBackupResult{enabled: true, count: result.Count, backupDir: backupDir, plan: &result}, err
	}
	app.deepDiveConfig.NeovimConfig = "custom"
	app.deepDiveConfig.TmuxTPMEnabled = false
	for id := range app.deepDiveConfig.CLITools {
		app.deepDiveConfig.CLITools[id] = false
	}
	globalPath := filepath.Join(config.ConfigDir(), "global.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.hasBlocked() {
		t.Fatalf("fresh plan unexpectedly blocked: %+v", plan.actions())
	}

	events := make(chan installEventMsg, 128)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err != nil {
		t.Fatalf("plan worker failed: %v\n%s", terminal.err, terminal.context)
	}
	for _, helper := range []string{"caff", "hk", "sshh"} {
		info, err := os.Stat(filepath.Join(home, ".local", "bin", helper))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("fresh-HOME helper %s = %v, %v", helper, info, err)
		}
	}

	operationsDir := filepath.Join(home, ".local", "state", "dotfiles", "operations")
	entries, err := os.ReadDir(operationsDir)
	if err != nil {
		t.Fatal(err)
	}
	var recordFiles []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			recordFiles = append(recordFiles, entry.Name())
		}
	}
	if len(recordFiles) != 1 {
		t.Fatalf("operation journal files = %v, want one", recordFiles)
	}
	data, err := os.ReadFile(filepath.Join(operationsDir, recordFiles[0]))
	if err != nil {
		t.Fatal(err)
	}
	var record operation.Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.PlanHash != plan.hash() || record.Status != operation.StatusSucceeded || record.FinishedAt == nil || record.Backup == "" {
		t.Fatalf("terminal journal does not identify reviewed plan: %+v", record)
	}
	for _, result := range record.Actions {
		if result.Status == operation.ActionPending {
			t.Fatalf("terminal journal left pending action: %+v", result)
		}
	}
}

func TestInstallPlanWorkerStopsOnManifestBackedPartialBackupError(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	for _, tool := range tools.GetRegistry().All() {
		app.manageInstalled[tool.ID()] = true
	}
	app.deepDiveConfig.NeovimConfig = "custom"
	app.deepDiveConfig.TmuxTPMEnabled = false
	for id := range app.deepDiveConfig.CLITools {
		app.deepDiveConfig.CLITools[id] = false
	}
	runtime := registryRuntime(pkg.PlatformMacOS, app.manageInstalled)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil || plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}

	backupTargets := plan.backupTargets()
	if len(backupTargets) == 0 {
		t.Fatal("accepted plan has no exact mutation targets")
	}
	productPaths := make([]string, 0, len(backupTargets))
	for _, rel := range backupTargets {
		if filepath.IsAbs(rel) || rel == "." || rel == "" {
			t.Fatalf("accepted plan has non-relative mutation target %q", rel)
		}
		productPaths = append(productPaths, filepath.Join(home, filepath.FromSlash(rel)))
	}
	for _, path := range productPaths {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("precondition product path %s unexpectedly exists: %v", path, statErr)
		}
	}

	partialRoot := filepath.Join(home, "partial-manifest-backup")
	backupErr := errors.New("injected failure after partial manifest commit")
	backupCalled := false
	runtime.backupTargets = func([]backup.Target) (autoBackupResult, error) {
		backupCalled = true
		if err := os.Mkdir(partialRoot, 0o700); err != nil {
			return autoBackupResult{}, err
		}
		manifest := "# dotfiles-backup-manifest v2\n# records: home-relative-path<TAB>octal-mode; payload uses legacy flat-name encoding\n.zshrc\t600\n"
		if err := os.WriteFile(filepath.Join(partialRoot, backup.ManifestName), []byte(manifest), 0o600); err != nil {
			return autoBackupResult{}, err
		}
		return autoBackupResult{enabled: true, count: 1, backupDir: partialRoot}, backupErr
	}

	events := make(chan installEventMsg, 128)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if !backupCalled || !errors.Is(terminal.err, backupErr) {
		t.Fatalf("backup called=%v terminal error=%v, want injected backup failure", backupCalled, terminal.err)
	}
	for _, path := range productPaths {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("backup failure mutated product path %s: %v", path, statErr)
		}
	}
}

func TestInstallLockReleaseFailureIsJournaledAsTerminalFailure(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	for _, tool := range tools.GetRegistry().All() {
		app.manageInstalled[tool.ID()] = true
	}
	runtime = registryRuntime(pkg.PlatformMacOS, app.manageInstalled)
	runtime.backupTargets = backupPlanTargets
	runtime.acquireInstallLock = func(*operation.StateAuthority, string, string) (func() error, error) {
		return func() error { return errors.New("injected release verification failure") }, nil
	}
	app.deepDiveConfig.NeovimConfig = "custom"
	app.deepDiveConfig.TmuxTPMEnabled = false
	for id := range app.deepDiveConfig.CLITools {
		app.deepDiveConfig.CLITools[id] = false
	}
	if err := os.MkdirAll(config.ConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil || plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}
	events := make(chan installEventMsg, 128)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err == nil || !strings.Contains(terminal.err.Error(), "release install operation lock") {
		t.Fatalf("terminal error = %v, want release failure", terminal.err)
	}
	journal, err := operation.DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	record, err := journal.Read(terminal.operationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != operation.StatusFailed {
		t.Fatalf("journal status = %s, want failed", record.Status)
	}
}

func TestStateProductSeparationRejectsAncestorsDescendantsAndAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateRoot := filepath.Join(home, "state")
	if err := os.Mkdir(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", stateRoot)
	check := func(target string) error {
		return validateStateProductSeparation(home, []operation.Action{{
			Disposition:  operation.DispositionApply,
			BackupTarget: target,
		}})
	}
	for _, target := range []string{"state", "state/dotfiles/config"} {
		if err := check(target); err == nil {
			t.Fatalf("overlap target %q was accepted", target)
		}
	}
	if err := check(".config/ghostty/config"); err != nil {
		t.Fatalf("unrelated product target was rejected: %v", err)
	}
	if err := os.Symlink(stateRoot, filepath.Join(home, "state-alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := check("state-alias/dotfiles/config"); err == nil {
		t.Fatal("symlink alias of state namespace was accepted")
	}
}

func TestStateProductSeparationRejectsDarwinCaseAlias(t *testing.T) {
	if goruntime.GOOS != "darwin" {
		t.Skip("Darwin path policy")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "State"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "State"))
	err := validateStateProductSeparation(home, []operation.Action{{
		Disposition:  operation.DispositionApply,
		BackupTarget: "state/dotfiles/config",
	}})
	if err == nil {
		t.Fatal("case-folded state namespace alias was accepted")
	}
}

func TestInstallPlanFailureAutomaticallyRestoresGlobalState(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	globalPath := filepath.Join(config.ConfigDir(), "global.json")
	global, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	global.Theme = "nord"
	global.NavStyle = "vim"
	global.AutoBackup = false
	if err := config.SaveGlobalConfig(global); err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(globalPath)
	if err != nil {
		t.Fatal(err)
	}

	app.theme = "catppuccin-mocha"
	app.navStyle = "emacs"
	app.deepDiveConfig.NeovimConfig = "custom"
	app.deepDiveConfig.TmuxTPMEnabled = false
	for id := range app.deepDiveConfig.CLITools {
		app.deepDiveConfig.CLITools[id] = false
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if plan.hasBlocked() {
		t.Fatalf("fresh rollback plan unexpectedly blocked: %+v", plan.actions())
	}
	mgr, ok := runtime.detectManager().(*pkg.MockPackageManager)
	if !ok {
		t.Fatal("fixture manager is not mutable")
	}
	mgr.InstallErr = errors.New("injected package failure")
	runtime.detectManager = func() pkg.PackageManager { return mgr }
	runtime.backupTargets = backupPlanTargets
	events := make(chan installEventMsg, 256)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err == nil {
		t.Fatal("fixture expected package postcondition failures")
	}
	gotBytes, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	gotInfo, err := os.Stat(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != string(wantBytes) || gotInfo.Mode().Perm() != wantInfo.Mode().Perm() {
		t.Fatalf("automatic rollback changed global state: bytes=%q mode=%04o; want bytes=%q mode=%04o", gotBytes, gotInfo.Mode().Perm(), wantBytes, wantInfo.Mode().Perm())
	}
	if terminal.operationID == "" {
		t.Fatal("failed operation did not retain a journal operation ID")
	}
	entries, err := os.ReadDir(filepath.Join(home, ".local", "state", "dotfiles", "operations"))
	journalCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			journalCount++
		}
	}
	if err != nil || journalCount != 1 {
		t.Fatalf("failed operation journal entries=%v err=%v", entries, err)
	}
	journal, err := operation.DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	record, err := journal.Read(terminal.operationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != operation.StatusFailed || record.Rollback == nil || record.Rollback.Status != operation.RollbackSucceeded || record.Backup == "" {
		t.Fatalf("failed operation did not journal successful rollback: %+v", record)
	}
}

func TestSecondRevalidationRefusalDoesNotRollbackExternalEdit(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	app.deepDiveConfig.NeovimConfig = "custom"
	app.deepDiveConfig.TmuxTPMEnabled = false
	for id := range app.deepDiveConfig.CLITools {
		app.deepDiveConfig.CLITools[id] = false
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil || plan.hasBlocked() {
		t.Fatalf("plan = %v blocked=%v err=%v", plan, plan != nil && plan.hasBlocked(), err)
	}
	tmuxPath := filepath.Join(home, ".tmux.conf")
	external := []byte("# external edit during backup\n")
	runtime.backupTargets = func(targets []backup.Target) (autoBackupResult, error) {
		result, err := backupPlanTargets(targets)
		if err != nil {
			return result, err
		}
		if err := os.WriteFile(tmuxPath, external, 0o600); err != nil {
			return result, err
		}
		return result, nil
	}
	events := make(chan installEventMsg, 128)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err == nil || !strings.Contains(terminal.err.Error(), "changed while creating rollback point") {
		t.Fatalf("terminal error = %v, want second revalidation refusal", terminal.err)
	}
	if got, err := os.ReadFile(tmuxPath); err != nil || !slices.Equal(got, external) {
		t.Fatalf("pre-mutation refusal rolled back external edit: %q err=%v", got, err)
	}
	journal, err := operation.DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	record, err := journal.Read(terminal.operationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Rollback != nil || record.Status != operation.StatusFailed {
		t.Fatalf("pre-mutation refusal journaled a rollback that did not run: %+v", record)
	}
}

func TestPostPackageRevalidationPreservesExternalEditWithoutRollback(t *testing.T) {
	app, _, runtime := newPlanTestApp(t)
	app.deepDiveConfig.NeovimConfig = "custom"
	app.deepDiveConfig.TmuxTPMEnabled = false
	for id := range app.deepDiveConfig.CLITools {
		app.deepDiveConfig.CLITools[id] = false
	}
	globalPath := filepath.Join(config.ConfigDir(), "global.json")
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil || plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}
	runtime.backupTargets = backupPlanTargets
	external := []byte("{\"schema_version\":1,\"theme\":\"nord\",\"nav_style\":\"vim\",\"external_edit\":true}\n")
	edited := false
	var injectionErr error
	originalManager := runtime.detectManager()
	runtime.detectManager = func() pkg.PackageManager {
		return &installHookManager{PackageManager: originalManager, hook: func() {
			if !edited {
				edited = true
				injectionErr = os.WriteFile(globalPath, external, 0o600)
			}
		}}
	}
	events := make(chan installEventMsg, 256)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if injectionErr != nil {
		t.Fatalf("inject external edit: %v", injectionErr)
	}
	if terminal.err == nil || !strings.Contains(terminal.err.Error(), "changed during package installation") {
		t.Fatalf("terminal error = %v, want post-package stale-plan refusal", terminal.err)
	}
	if got, err := os.ReadFile(globalPath); err != nil || !slices.Equal(got, external) {
		t.Fatalf("automatic rollback overwrote external edit: %q err=%v", got, err)
	}
	journal, err := operation.DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	record, err := journal.Read(terminal.operationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Rollback != nil {
		t.Fatalf("pre-config refusal journaled a rollback that did not run: %+v", record)
	}
}

func TestPostPackageRevalidationPreservesExternalConfigCollision(t *testing.T) {
	app, home, _ := newPlanTestApp(t)
	for _, tool := range tools.GetRegistry().All() {
		app.manageInstalled[tool.ID()] = true
	}
	app.manageInstalled["zsh"] = false
	app.deepDiveConfig.NeovimConfig = "custom"
	app.deepDiveConfig.TmuxTPMEnabled = false
	for id := range app.deepDiveConfig.CLITools {
		app.deepDiveConfig.CLITools[id] = false
	}
	runtime := registryRuntime(pkg.PlatformMacOS, app.manageInstalled)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil || plan.hasBlocked() {
		t.Fatalf("plan blocked=%v err=%v", plan != nil && plan.hasBlocked(), err)
	}
	runtime.backupTargets = backupPlanTargets
	ghosttyTarget := plan.ghosttyConfigTarget
	victim := filepath.Join(home, "external-ghostty-config")
	external := []byte("font-size = 31\n# external collision\n")
	if err := os.WriteFile(victim, external, 0o600); err != nil {
		t.Fatal(err)
	}
	injected := false
	originalManager := runtime.detectManager()
	runtime.detectManager = func() pkg.PackageManager {
		return &installHookManager{PackageManager: originalManager, hook: func() {
			if !injected {
				injected = true
				if err := os.MkdirAll(filepath.Dir(ghosttyTarget), 0o700); err != nil {
					t.Errorf("create Ghostty parent: %v", err)
					return
				}
				if err := os.Symlink(victim, ghosttyTarget); err != nil {
					t.Errorf("inject Ghostty collision: %v", err)
				}
			}
		}}
	}
	events := make(chan installEventMsg, 256)
	go runInstallPlanWorker(context.Background(), events, plan, runtime)
	var terminal installEventMsg
	for event := range events {
		if event.done {
			terminal = event
		}
	}
	if terminal.err == nil || !strings.Contains(terminal.err.Error(), "changed during package installation") {
		t.Fatalf("terminal error = %v, want post-package collision refusal", terminal.err)
	}
	if got, err := os.ReadFile(victim); err != nil || !slices.Equal(got, external) {
		t.Fatalf("rollback overwrote external collision victim: %q err=%v", got, err)
	}
	if info, err := os.Lstat(ghosttyTarget); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("rollback removed external collision link: mode=%v err=%v", info, err)
	}
	journal, err := operation.DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	record, err := journal.Read(terminal.operationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Rollback != nil {
		t.Fatalf("pre-config collision refusal journaled a rollback: %+v", record)
	}
}

func TestPartialWriterEvidenceCannotAuthorizeEditBeforeWorkerHandling(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rel := ".config/yazi/yazi.toml"
	path := filepath.Join(home, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// The writer's committed evidence describes different desired bytes; this
	// file models an external edit after that commit but before worker handling.
	if err := os.WriteFile(path, []byte("# external edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	partial := &tools.PartialMutationError{
		Err:      errors.New("later target failed"),
		Evidence: []tools.MutationEvidence{{Path: path}},
	}
	expected := make(map[string]backup.ExpectedState)
	if err := recordFailedActionRollbackState(home, plan, "config:yazi", partial, expected); err == nil || !strings.Contains(err.Error(), "manual recovery required") {
		t.Fatalf("untracked partial evidence error = %v, want manual recovery requirement", err)
	}
	state := expected[rel]
	if !state.Attempted || state.Captured {
		t.Fatalf("external edit gained rollback authority: %+v", state)
	}
}

func TestMutationEvidenceSetsRejectOutOfScopePathsAtomicallyInBothOrders(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	makeEvidence := func(rel, content string) tools.MutationEvidence {
		t.Helper()
		path := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		_, revision, parents, err := safefile.ObserveFileWithin(home, rel)
		if err != nil {
			t.Fatal(err)
		}
		return tools.MutationEvidence{Path: path, Revision: revision, Parents: parents}
	}
	validRel := ".config/yazi/yazi.toml"
	valid := makeEvidence(validRel, "# generated\n")
	tests := []struct {
		name    string
		invalid tools.MutationEvidence
	}{
		{name: "fully unplanned", invalid: makeEvidence(".outside-plan", "outside\n")},
		{name: "different planned action", invalid: makeEvidence(".zshrc", "planned elsewhere\n")},
	}
	for _, test := range tests {
		for _, invalidFirst := range []bool{false, true} {
			name := test.name + "/invalid-last"
			evidence := []tools.MutationEvidence{valid, test.invalid}
			if invalidFirst {
				name = test.name + "/invalid-first"
				evidence = []tools.MutationEvidence{test.invalid, valid}
			}
			t.Run(name, func(t *testing.T) {
				expected := make(map[string]backup.ExpectedState)
				if err := authorizeMutationEvidenceSet(home, plan, "config:yazi", evidence, expected); err == nil {
					t.Fatal("mixed-scope evidence set was authorized")
				}
				if len(expected) != 0 {
					t.Fatalf("evidence authorization partially mutated rollback state: %+v", expected)
				}

				partialExpected := make(map[string]backup.ExpectedState)
				partial := &tools.PartialMutationError{Err: errors.New("later failure"), Evidence: evidence}
				if err := recordFailedActionRollbackState(home, plan, "config:yazi", partial, partialExpected); err == nil || !strings.Contains(err.Error(), "manual recovery required") {
					t.Fatalf("mixed-scope partial evidence error = %v, want manual recovery requirement", err)
				}
				for _, rel := range []string{".config/yazi/yazi.toml", ".config/yazi/keymap.toml", ".config/yazi/theme.toml"} {
					state := partialExpected[rel]
					if !state.Attempted || state.Captured {
						t.Fatalf("%s partial evidence gained rollback authority: %+v", rel, state)
					}
				}
			})
		}
	}
}

func TestInstallUtilitiesTrackedStopsAtExactFailureBoundary(t *testing.T) {
	app, home, runtime := newPlanTestApp(t)
	utilities := map[string]bool{"caff": true, "hk": true, "sshh": true}
	app.deepDiveConfig.Utilities = utilities
	var calls []string
	result := installUtilitiesTrackedWith(utilities, func(_ string, name string, _ []byte) (tools.MutationEvidence, error) {
		calls = append(calls, name)
		if name == "hk" {
			return tools.MutationEvidence{}, errors.New("injected helper failure")
		}
		rel := filepath.ToSlash(filepath.Join(".local", "bin", name))
		path := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o700); err != nil {
			t.Fatal(err)
		}
		_, revision, parents, err := safefile.ObserveFileWithin(home, rel)
		if err != nil {
			t.Fatal(err)
		}
		return tools.MutationEvidence{Path: path, Revision: revision, Parents: parents}, nil
	})
	if !slices.Equal(calls, []string{"caff", "hk"}) || !slices.Equal(result.Attempted, []string{"caff", "hk"}) {
		t.Fatalf("helper boundary calls=%v attempted=%v", calls, result.Attempted)
	}
	if result.Failed != "hk" || result.Err == nil || len(result.Evidence) != 1 {
		t.Fatalf("helper result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "sshh")); !os.IsNotExist(err) {
		t.Fatalf("helper after failure was attempted: %v", err)
	}

	plan, err := buildInstallPlan(app, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	expected := make(map[string]backup.ExpectedState)
	if err := authorizeHelperMutationEvidence(home, plan, result.Attempted, result.Evidence, expected); err != nil {
		t.Fatal(err)
	}
	if err := recordFailedActionRollbackState(home, plan, "helper:hk", result.Err, expected); err != nil {
		t.Fatal(err)
	}
	if state := expected[".local/bin/caff"]; !state.Captured {
		t.Fatalf("successful helper evidence not captured: %+v", state)
	}
	if state := expected[".local/bin/hk"]; !state.Attempted || state.Captured {
		t.Fatalf("failed helper state = %+v", state)
	}
	if _, exists := expected[".local/bin/sshh"]; exists {
		t.Fatalf("unattempted helper forced rollback state: %+v", expected[".local/bin/sshh"])
	}
}
