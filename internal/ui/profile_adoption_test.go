package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"

	"github.com/tekierz/dotfiles/internal/config"
)

func TestProfileSwitchAdoptsPersistedSettings(t *testing.T) {
	withTempHome(t)
	cfg := config.DefaultGlobalConfig()
	cfg.DisableAnimations = true
	if err := config.SaveGlobalConfig(cfg); err != nil {
		t.Fatal(err)
	}
	profile := config.DefaultUserProfile("Alice")
	profile.Theme = "dracula"
	profile.NavStyle = "vim"
	profile.KeyboardStyle = "macos"
	if err := config.SaveUserProfile(profile); err != nil {
		t.Fatal(err)
	}
	a := NewApp(true)
	a.screenMgr.Navigate(ScreenMainMenu)
	done := a.startAsync(asyncUserOperation, switchUserCmd("Alice"))()
	a.Update(done)
	persisted, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	ctx := a.screenMgr.Context()
	if a.theme != persisted.Theme || a.navStyle != persisted.NavStyle || a.animationsEnabled == persisted.DisableAnimations || ctx.Theme != a.theme || ctx.NavStyle != a.navStyle || ctx.AnimationsEnabled != a.animationsEnabled {
		t.Fatalf("persisted=%s/%s App=%s/%s context=%s/%s", persisted.Theme, persisted.NavStyle, a.theme, a.navStyle, ctx.Theme, ctx.NavStyle)
	}
}

func TestProfileSwitchInvalidatesAuthorityAndNextPlanUsesSavedSettings(t *testing.T) {
	a, _, runtime := newPlanTestApp(t)
	global := config.DefaultGlobalConfig()
	global.DisableAnimations = true
	if err := config.SaveGlobalConfig(global); err != nil {
		t.Fatal(err)
	}
	profile := config.DefaultUserProfile("Alice")
	profile.Theme = "dracula"
	profile.NavStyle = "vim"
	if err := config.SaveUserProfile(profile); err != nil {
		t.Fatal(err)
	}
	a.pendingInstallPlan = &installPlan{}
	a.deepDiveContinuation = &deepDiveContinuation{}
	a.installReviewTools = []string{"tmux"}
	a.pendingManageSavePlan = &manageSavePlan{}
	a.standaloneConfigPlan = &installPlan{}
	oldGeneration := a.installationSnapshotGeneration
	oldUpdate := a.startAsync(asyncUpdates, func() tea.Msg { return updateCheckDoneMsg{updates: []pkg.Package{{Name: "stale"}}} })()
	a.updateSelected = map[int]bool{0: true}
	deepDive := a.deepDiveConfig
	deepDive.GhosttyFontSize = 21
	done := a.startAsync(asyncUserOperation, switchUserCmd("Alice"))()
	// A later profile edit must not change the already persisted switch result.
	profile.Theme = "nord"
	if err := config.SaveUserProfile(profile); err != nil {
		t.Fatal(err)
	}
	a.Update(done)
	if a.theme != "dracula" || a.navStyle != "vim" {
		t.Fatal("switch reloaded a later profile edit")
	}
	if a.pendingInstallPlan != nil || a.deepDiveContinuation != nil || a.installReviewTools != nil || a.pendingManageSavePlan != nil || a.standaloneConfigPlan != nil {
		t.Fatal("profile switch retained stale review authority")
	}
	if a.deepDiveConfig != deepDive || a.deepDiveConfig.GhosttyFontSize != 21 {
		t.Fatal("switch discarded per-tool installer preferences")
	}
	if a.installationSnapshotGeneration <= oldGeneration || a.installationSnapshotReady || !a.installationSnapshotLoading || a.manageInstalledReady {
		t.Fatal("switch did not refresh installation evidence")
	}
	a.Update(installationSnapshotDoneMsg{Generation: oldGeneration, Err: errors.New("stale")})
	if !a.installationSnapshotLoading {
		t.Fatal("old snapshot completed new generation")
	}
	a.Update(oldUpdate)
	if len(a.updateResults) != 0 || len(a.updateSelected) != 0 || a.updateCheckDone || a.updateChecking {
		t.Fatal("stale update cache survived switch")
	}
	if a.hotkeysActiveUser != "Alice" || !a.hotkeysActiveUserCached {
		t.Fatal("active-user hotkey identity remained stale")
	}
	seedTypedReadyInstallCache(t, a, map[string]bool{})
	plan, err := buildInstallPlan(a, runtime, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	planned, err := plan.plannedGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if planned.Theme != "dracula" || planned.NavStyle != "vim" || !planned.DisableAnimations || planned.ActiveUser != "Alice" {
		t.Fatalf("next plan would revert switched settings: %+v", planned)
	}
}

func TestProfileSwitchFailurePreservesSharedState(t *testing.T) {
	withTempHome(t)
	profile := config.DefaultUserProfile("Alice")
	profile.Theme = "dracula"
	if err := config.SaveUserProfile(profile); err != nil {
		t.Fatal(err)
	}
	a := NewApp(true)
	a.usersLoaded = true
	a.screenMgr.Navigate(ScreenUsers)
	beforeTheme, beforeNav, beforeMotion := a.theme, a.navStyle, a.animationsEnabled
	plan := &installPlan{}
	a.pendingInstallPlan = plan
	generation := a.installationSnapshotGeneration
	if err := os.WriteFile(filepath.Join(config.ConfigDir(), "global.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	a.Update(a.startAsync(asyncUserOperation, switchUserCmd("Alice"))())
	ctx := a.screenMgr.Context()
	if a.theme != beforeTheme || a.navStyle != beforeNav || a.animationsEnabled != beforeMotion || ctx.Theme != beforeTheme || ctx.NavStyle != beforeNav || ctx.AnimationsEnabled != beforeMotion {
		t.Fatal("failed switch changed live settings")
	}
	if a.pendingInstallPlan != plan || a.installationSnapshotGeneration != generation {
		t.Fatal("failed switch invalidated existing authority")
	}
	if !strings.Contains(a.usersStatus, "Switch failed") {
		t.Fatal("switch failure hidden")
	}
}

func TestProfileSwitchDuplicateCannotUndoNewerSwitch(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	for _, name := range []string{"Alice", "Bob"} {
		profile := config.DefaultUserProfile(name)
		profile.Theme = "dracula"
		if name == "Bob" {
			profile.Theme = "nord"
		}
		if err := config.SaveUserProfile(profile); err != nil {
			t.Fatal(err)
		}
	}
	first := a.startAsync(asyncUserOperation, switchUserCmd("Alice"))()
	a.Update(first)
	second := a.startAsync(asyncUserOperation, switchUserCmd("Bob"))()
	a.Update(second)
	a.Update(first)
	if a.theme != "nord" || a.screenMgr.Context().Theme != "nord" || a.hotkeysActiveUser != "Bob" {
		t.Fatal("duplicate switch replaced newer settings")
	}
}

func TestProfileAdoptionPreservesRunningAuthority(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	install := &installPlan{}
	manage := &manageSavePlan{}
	standalone := &installPlan{}
	continuation := &deepDiveContinuation{}
	a.installRunning = true
	a.manageSaveRunning = true
	a.standaloneConfigRunning = true
	a.pendingInstallPlan = install
	a.pendingManageSavePlan = manage
	a.standaloneConfigPlan = standalone
	a.deepDiveContinuation = continuation
	a.adoptProfileSettings(config.DefaultGlobalConfig())
	if a.pendingInstallPlan != install || a.pendingManageSavePlan != manage || a.standaloneConfigPlan != standalone || a.deepDiveContinuation != continuation {
		t.Fatal("adoption erased an executing operation's authority")
	}
}

func TestProfileAdoptionRestartsMotionAndRejectsMissingSettings(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	a.animationsEnabled = false
	a.syncSharedSettings()
	settings := config.DefaultGlobalConfig()
	settings.DisableAnimations = false
	cmd := a.adoptProfileSettings(settings)
	if !a.animationsEnabled || !a.screenMgr.Context().AnimationsEnabled || cmd == nil {
		t.Fatal("motion not adopted")
	}
	// Fresh installation observation plus one restarted tick; no worker is run.
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("enabling motion did not restart the tick alongside refresh")
	}
	oldTheme := a.theme
	a.Update(a.startAsync(asyncUserOperation, func() tea.Msg { return userSwitchedMsg{name: "Alice"} })())
	if a.theme != oldTheme || !strings.Contains(a.usersStatus, "saved settings unavailable") {
		t.Fatal("uncaptured switch result was accepted")
	}
}
