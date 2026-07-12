package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/scripts"
	"github.com/tekierz/dotfiles/internal/tools"
)

type pinnedRecipeExecutionManager struct {
	*pkg.MockPackageManager
	installed           bool
	preProbe, postProbe int
	installCalls        [][]string
}

func (m *pinnedRecipeExecutionManager) IsInstalled(string) bool {
	if m.installed {
		m.postProbe++
	} else {
		m.preProbe++
	}
	return true
}

func (m *pinnedRecipeExecutionManager) InstallStreaming(_ context.Context, packages ...string) (*runner.StreamingCmd, error) {
	m.installCalls = append(m.installCalls, append([]string(nil), packages...))
	m.installed = true
	return nil, nil
}

// TestInstallSnapshotExecutionDispatchesPinnedRecipeWithoutPreProbe proves the
// accepted plan, rather than a fresh boolean installation probe, authorizes the
// package mutation performed by the isolated install runner.
func TestInstallSnapshotExecutionDispatchesPinnedRecipeWithoutPreProbe(t *testing.T) {
	for _, presence := range []health.Presence{health.PresenceMissing, health.PresencePartial} {
		t.Run(string(presence), func(t *testing.T) {
			app, _, planRuntime := newPlanTestApp(t)
			app.deepDiveConfig.CLITools = map[string]bool{}
			app.deepDiveConfig.GUIApps = map[string]bool{}
			app.deepDiveConfig.CLIUtilities = map[string]bool{}
			app.deepDiveConfig.MacApps = map[string]bool{}
			observations := make([]health.InstallationObservation, 0, len(alwaysConfiguredToolIDs))
			for _, id := range alwaysConfiguredToolIDs {
				state := health.PresencePresent
				if id == "zsh" {
					state = presence
				}
				observations = append(observations, plannerSnapshotObservation(t, planRuntime, id, state, health.InstallabilitySupported))
			}
			setManageTruthSnapshot(t, app, 41, pkg.PlatformMacOS, "brew", observations...)
			plan, err := buildInstallPlan(app, planRuntime, time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := plan.installExecutionSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			intent := "install"
			if presence == health.PresencePartial {
				intent = "repair"
			}
			authority, ok := any(plan).(installPlanAuthorityProjection)
			if !ok {
				t.Fatal("accepted plan exposes no typed per-tool authority")
			}
			if gotPresence, gotIntent, found := authority.installAuthority("zsh"); !found || gotPresence != presence || gotIntent != intent {
				t.Fatalf("accepted authority=(%s,%q,%v), want (%s,%q,true)", gotPresence, gotIntent, found, presence, intent)
			}
			zsh, ok := planRuntime.lookupTool("zsh")
			if !ok {
				t.Fatal("zsh missing from registry")
			}
			wantRecipe, err := tools.DescribeInstall(zsh, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
			if err != nil {
				t.Fatal(err)
			}
			wantDigest := installRecipeDigest(wantRecipe)
			if accepted.platform != pkg.PlatformMacOS || accepted.manager != "brew" || !reflect.DeepEqual(accepted.recipes["zsh"], wantRecipe) || accepted.digests["zsh"] != wantDigest || accepted.detected["zsh"] {
				t.Fatalf("accepted execution authority=%+v", accepted)
			}

			mgr := &pinnedRecipeExecutionManager{MockPackageManager: pkg.NewMockPackageManager()}
			mgr.ManagerName = "brew"
			runtimeBoolCalls := 0
			executionRuntime := toolInstallRuntime{
				lookupTool:     planRuntime.lookupTool,
				detectManager:  func() pkg.PackageManager { return mgr },
				detectPlatform: func() pkg.Platform { return pkg.PlatformMacOS },
				isToolInstalled: func(tools.Tool) bool {
					runtimeBoolCalls++
					return true
				},
			}
			result := runSelectedToolInstalls(context.Background(), []string{"zsh"}, executionRuntime, func(string) {}, func(string) {}, accepted)
			if len(result.failures) != 0 || result.successCount != 1 || !result.installed["zsh"] {
				t.Fatalf("execution result=%#v", result)
			}
			if mgr.preProbe != 0 || runtimeBoolCalls != 0 {
				t.Fatalf("pre-mutation probes: recipe=%d runtime-bool=%d", mgr.preProbe, runtimeBoolCalls)
			}
			if mgr.postProbe != len(wantRecipe.Detector.Values) {
				t.Fatalf("postcondition probes=%d, want %d", mgr.postProbe, len(wantRecipe.Detector.Values))
			}
			if len(wantRecipe.Steps) != 1 || !reflect.DeepEqual(mgr.installCalls, [][]string{wantRecipe.Steps[0].Packages}) {
				t.Fatalf("install calls=%v, want exact recipe %v", mgr.installCalls, wantRecipe.Steps)
			}
		})
	}
}

type manageInstallRoutingProbe struct {
	packages, installed, direct int
}

type manageInstallRoutingProbeTool struct {
	tools.Tool
	probe *manageInstallRoutingProbe
}

func (t manageInstallRoutingProbeTool) Packages() map[pkg.Platform][]string {
	t.probe.packages++
	return t.Tool.Packages()
}

func (t manageInstallRoutingProbeTool) IsInstalled() bool {
	t.probe.installed++
	return t.Tool.IsInstalled()
}

func (t manageInstallRoutingProbeTool) IsInstalledOutsidePackageManager(observation tools.DirectInstallationObservation) bool {
	t.probe.direct++
	detector, ok := t.Tool.(tools.DirectInstallationDetector)
	return ok && detector.IsInstalledOutsidePackageManager(observation)
}

func TestManageInstallIRoutesTypedStateToReviewedPlan(t *testing.T) {
	tests := []struct {
		name        string
		presence    health.Presence
		intent      string
		legacyValue bool
	}{
		{name: "missing routes install to reviewed plan", presence: health.PresenceMissing, intent: "install", legacyValue: true},
		{name: "partial routes repair to reviewed plan", presence: health.PresencePartial, intent: "repair", legacyValue: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newGoldenContext(t)
			app := ctx.app
			app.deepDiveConfig.CLITools = map[string]bool{}
			app.deepDiveConfig.GUIApps = map[string]bool{}
			app.deepDiveConfig.CLIUtilities = map[string]bool{}
			app.deepDiveConfig.MacApps = map[string]bool{}
			registry := tools.NewRegistry()
			fixtureRuntime := registryRuntime(pkg.PlatformMacOS, map[string]bool{})
			observations := make([]health.InstallationObservation, 0, len(alwaysConfiguredToolIDs))
			for _, id := range alwaysConfiguredToolIDs {
				presence := health.PresencePresent
				if id == "zsh" {
					presence = tt.presence
				}
				observations = append(observations, plannerSnapshotObservation(t, fixtureRuntime, id, presence, health.InstallabilitySupported))
			}
			setManageTruthSnapshot(t, app, 31, pkg.PlatformMacOS, "brew", observations...)
			snapshot := app.installationSnapshot
			app.manageInstalled = make(map[string]bool, len(alwaysConfiguredToolIDs))
			for _, id := range alwaysConfiguredToolIDs {
				app.manageInstalled[id] = true
			}
			app.manageInstalled["zsh"] = tt.legacyValue

			probe := &manageInstallRoutingProbe{}
			platformCalls := 0
			all := registry.All()
			for index, tool := range all {
				if tool.ID() == "zsh" {
					all[index] = manageInstallRoutingProbeTool{Tool: tool, probe: probe}
				}
			}
			app.manageToolSource = func() []tools.Tool { return all }
			app.manageDetectPlatform = func() pkg.Platform { platformCalls++; return pkg.PlatformArch }
			selectManageTruthItem(t, app, "zsh")
			app.managePane = managePaneSettings
			app.pendingInstallPlan = nil
			app.installPlanError = nil

			cmd := NewManageScreen(ctx).handleKey(keyMsg("i"))
			if app.pendingInstallPlan == nil || app.installPlanError != nil {
				t.Fatalf("Manage I reviewed plan nonnil=%v error=%v", app.pendingInstallPlan != nil, app.installPlanError)
			}
			reviewed := app.pendingInstallPlan
			reviewedHash := reviewed.hash()
			if reviewedHash == "" {
				t.Fatal("reviewed plan has empty hash")
			}
			authority, ok := any(reviewed).(installPlanAuthorityProjection)
			if !ok {
				t.Fatal("reviewed plan exposes no typed per-tool authority")
			}
			if presence, intent, found := authority.installAuthority("zsh"); !found || presence != tt.presence || intent != tt.intent {
				t.Fatalf("zsh authority=(%s,%q,%v), want (%s,%q,true)", presence, intent, found, tt.presence, tt.intent)
			}
			snapshotAuthority, ok := any(reviewed).(installPlanSnapshotAuthorityProjection)
			if !ok {
				t.Fatal("reviewed plan exposes no snapshot authority")
			}
			schema, generation, platform, manager, digest := snapshotAuthority.installSnapshotAuthority()
			if schema != snapshot.SchemaVersion() || generation != snapshot.Generation() || platform != snapshot.Platform() || manager != snapshot.Manager() || digest != snapshot.Digest() {
				t.Fatalf("reviewed snapshot authority=(%d,%d,%q,%q,%q), want exact accepted snapshot", schema, generation, platform, manager, digest)
			}
			tool, _ := registry.Get("zsh")
			recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
			if err != nil {
				t.Fatal(err)
			}
			if got, found := snapshotAuthority.installRecipeAuthority("zsh"); !found || got != installRecipeDigest(recipe) {
				t.Fatalf("reviewed zsh recipe authority=(%q,%v)", got, found)
			}
			var installs []operation.Action
			for _, action := range reviewed.actions() {
				if action.Kind == operation.KindInstallTool && action.ToolID == "zsh" {
					installs = append(installs, action)
				}
			}
			if len(installs) != 1 || installs[0].Disposition != operation.DispositionApply || !strings.Contains(strings.ToLower(installs[0].Description), tt.intent) {
				t.Fatalf("reviewed zsh actions=%+v, want one Apply %s action", installs, tt.intent)
			}
			if cmd == nil {
				t.Fatal("Manage I returned no review navigation command")
			}
			message := cmd()
			navigation, ok := message.(NavigateMsg)
			if !ok || navigation.To != ScreenFileTree {
				t.Fatalf("Manage I command message=%#v, want NavigateMsg to FileTree", message)
			}
			if app.pendingInstallPlan != reviewed || app.pendingInstallPlan.hash() != reviewedHash {
				t.Fatal("review navigation rebuilt or replaced the exact reviewed plan")
			}
			if platformCalls != 0 || probe.packages != 0 || probe.installed != 0 || probe.direct != 0 {
				t.Fatalf("Manage I performed live probes: platform=%d packages=%d installed=%d direct=%d", platformCalls, probe.packages, probe.installed, probe.direct)
			}
		})
	}
}

// TestInstallSnapshotUtilitiesDoNotAuthorizePlanActions keeps the cosmetic
// utility cache and the legacy installed map outside the authority boundary for
// application installs. Managed helper files remain governed by their stable
// filesystem observation and embedded desired bytes.
func TestInstallSnapshotUtilitiesDoNotAuthorizePlanActions(t *testing.T) {
	app, home, planRuntime := newPlanTestApp(t)
	app.deepDiveConfig.CLITools = map[string]bool{}
	app.deepDiveConfig.GUIApps = map[string]bool{}
	app.deepDiveConfig.CLIUtilities = map[string]bool{}
	app.deepDiveConfig.MacApps = map[string]bool{}
	app.deepDiveConfig.Utilities = map[string]bool{"hk": true}

	observations := make([]health.InstallationObservation, 0, len(alwaysConfiguredToolIDs))
	for _, id := range alwaysConfiguredToolIDs {
		presence := health.PresencePresent
		if id == "zsh" {
			presence = health.PresenceMissing
		}
		observations = append(observations, plannerSnapshotObservation(t, planRuntime, id, presence, health.InstallabilitySupported))
	}
	setManageTruthSnapshot(t, app, 51, pkg.PlatformMacOS, "brew", observations...)

	path := filepath.Join(home, ".local", "bin", "hk")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("#!/bin/sh\necho user-owned helper\n")
	if err := os.WriteFile(path, foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignDigest := sha256.Sum256(foreign)
	desiredDigest := sha256.Sum256([]byte(scripts.HKScript))
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)

	app.installationSnapshotUtilities = map[string]bool{"hk": true}
	app.manageInstalled["hk"] = false
	first, err := buildInstallPlan(app, planRuntime, now)
	if err != nil {
		t.Fatal(err)
	}

	app.installationSnapshotUtilities["hk"] = false
	app.manageInstalled["hk"] = true
	second, err := buildInstallPlan(app, planRuntime, now)
	if err != nil {
		t.Fatal(err)
	}

	applicationActions := func(plan *installPlan) []operation.Action {
		var result []operation.Action
		for _, action := range plan.actions() {
			if action.Kind == operation.KindInstallTool {
				result = append(result, action)
			}
		}
		return result
	}
	if got, want := applicationActions(second), applicationActions(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("application install actions changed with utility booleans:\nfirst=%+v\nsecond=%+v", want, got)
	}
	if first.hash() == "" || second.hash() != first.hash() {
		t.Fatalf("plan hashes changed with utility booleans: first=%q second=%q", first.hash(), second.hash())
	}
	for _, plan := range []*installPlan{first, second} {
		action := planActionByID(t, plan, "helper:hk")
		if action.Kind != operation.KindInstallFile || action.Disposition != operation.DispositionBlocked {
			t.Fatalf("hk helper action ignored foreign filesystem state: %+v", action)
		}
		if !action.Observation.Exists || action.Observation.Digest != hex.EncodeToString(foreignDigest[:]) {
			t.Fatalf("hk helper observation=%+v, want foreign file digest", action.Observation)
		}
		if action.DesiredDigest != hex.EncodeToString(desiredDigest[:]) {
			t.Fatalf("hk desired digest=%q, want embedded script digest %q", action.DesiredDigest, hex.EncodeToString(desiredDigest[:]))
		}
	}
}
