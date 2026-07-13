package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/tools"
)

// customInstallSentinel is deliberately more than package metadata. The old
// wizard/Manage implementations called InstallStreaming directly, so the
// custom-dispatch tests below fail under the old behavior.
type customInstallSentinel struct {
	installCalls       int
	installed          bool
	packages           map[pkg.Platform][]string
	managerIndependent bool
	remainMissing      bool
	emitLines          int
	blockUntilCancel   bool
	started            chan struct{}
}

type caskInstallManager struct {
	*pkg.MockPackageManager
	home  string
	calls [][]string
}

func (m *caskInstallManager) InstallCasksStreaming(_ context.Context, casks ...string) (*runner.StreamingCmd, error) {
	m.calls = append(m.calls, slices.Clone(casks))
	if err := os.MkdirAll(filepath.Join(m.home, "Applications", "T3 Code.app"), 0o700); err != nil {
		return nil, err
	}
	return nil, nil
}

func TestInstallRecipeDetectedAppBundleRequiresDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	applications := filepath.Join(home, "Applications")
	if err := os.MkdirAll(applications, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(applications, "Dotfiles Detector Fixture.app")
	if err := os.WriteFile(bundle, []byte("not an app bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	recipe := operation.InstallRecipe{
		Detector: operation.InstallDetector{
			Kind:   operation.InstallDetectorAppBundle,
			Values: []string{filepath.Base(bundle)},
		},
	}

	detected, err := installRecipeDetected(recipe, nil)
	if err != nil {
		t.Fatalf("detect plain file: %v", err)
	}
	if detected {
		t.Fatal("plain file with .app suffix was accepted as an application bundle")
	}

	if err := os.Remove(bundle); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bundle, 0o700); err != nil {
		t.Fatal(err)
	}
	detected, err = installRecipeDetected(recipe, nil)
	if err != nil {
		t.Fatalf("detect app bundle directory: %v", err)
	}
	if !detected {
		t.Fatal("application bundle directory was not detected")
	}
}

func (t *customInstallSentinel) ID() string                   { return "custom-sentinel" }
func (t *customInstallSentinel) Name() string                 { return "Custom Sentinel" }
func (t *customInstallSentinel) Description() string          { return "test-only custom installer" }
func (t *customInstallSentinel) Icon() string                 { return "" }
func (t *customInstallSentinel) Category() tools.Category     { return tools.CategoryUtility }
func (t *customInstallSentinel) IsInstalled() bool            { return t.installed }
func (t *customInstallSentinel) ConfigPaths() []string        { return nil }
func (t *customInstallSentinel) HasConfig() bool              { return false }
func (t *customInstallSentinel) IsHeavy() bool                { return false }
func (t *customInstallSentinel) UIGroup() tools.UIGroup       { return tools.UIGroupCLITools }
func (t *customInstallSentinel) ConfigScreen() int            { return 0 }
func (t *customInstallSentinel) DefaultEnabled() bool         { return false }
func (t *customInstallSentinel) PlatformFilter() pkg.Platform { return "" }
func (t *customInstallSentinel) PackageMetadataIsAuthoritative() bool {
	return false
}
func (t *customInstallSentinel) InstallerAvailable(pkg.Platform) bool { return true }
func (t *customInstallSentinel) RequiresPackageManager() bool {
	return !t.managerIndependent
}
func (t *customInstallSentinel) Packages() map[pkg.Platform][]string {
	return t.packages
}
func (t *customInstallSentinel) Install(mgr pkg.PackageManager) error {
	t.installCalls++
	if mgr != nil {
		if err := mgr.Install("sentinel-runtime"); err != nil {
			return err
		}
	}
	if !t.remainMissing {
		t.installed = true
	}
	return nil
}
func (t *customInstallSentinel) InstallWithContext(ctx context.Context, mgr pkg.PackageManager, emitLine func(string)) error {
	for i := 0; i < t.emitLines; i++ {
		emitLine(fmt.Sprintf("output-%04d", i))
	}
	if t.blockUntilCancel {
		if t.started != nil {
			close(t.started)
		}
		<-ctx.Done()
		return ctx.Err()
	}
	return t.Install(mgr)
}

func sentinelRuntime(sentinel tools.Tool, mgr pkg.PackageManager) toolInstallRuntime {
	return toolInstallRuntime{
		lookupTool: func(id string) (tools.Tool, bool) {
			if id == sentinel.ID() {
				return sentinel, true
			}
			return nil, false
		},
		detectManager:  func() pkg.PackageManager { return mgr },
		detectPlatform: pkg.DetectPlatform,
		isToolInstalled: func(t tools.Tool) bool {
			return t.IsInstalled()
		},
		autoBackup: func() (autoBackupResult, error) { return autoBackupResult{}, nil },
	}
}

func registryRuntime(platform pkg.Platform, installed map[string]bool) toolInstallRuntime {
	reg := tools.NewRegistry()
	manager := pkg.NewMockPackageManager()
	switch platform {
	case pkg.PlatformMacOS:
		manager.ManagerName = "brew"
	case pkg.PlatformArch:
		manager.ManagerName = "pacman"
	case pkg.PlatformDebian, pkg.PlatformPi:
		manager.ManagerName = "apt"
	case pkg.PlatformUnknown:
		manager.ManagerName = "unknown"
	}
	return toolInstallRuntime{
		lookupTool: reg.Get,
		registeredToolIDs: func() []string {
			registered := reg.All()
			ids := make([]string, 0, len(registered))
			for _, tool := range registered {
				ids = append(ids, tool.ID())
			}
			return ids
		},
		describeInstall:  tools.DescribeInstall,
		captureStatePlan: operation.CaptureStatePlan,
		detectManager:    func() pkg.PackageManager { return manager },
		detectPlatform:   func() pkg.Platform { return platform },
		isToolInstalled: func(t tools.Tool) bool {
			return installed[t.ID()]
		},
		autoBackup: func() (autoBackupResult, error) { return autoBackupResult{}, nil },
	}
}

func containsToolID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func TestCollectSelectedToolsIncludesMissingSupportedCore(t *testing.T) {
	cfg := NewDeepDiveConfig()
	a := &App{
		deepDiveConfig:       cfg,
		manageInstalled:      make(map[string]bool),
		manageInstalledReady: true,
	}

	selected := a.collectSelectedTools()
	for _, id := range alwaysConfiguredToolIDs {
		if !containsToolID(selected, id) {
			t.Errorf("missing supported core tool %q from fresh-machine install plan: %v", id, selected)
		}
	}
}

func TestCorePlanSkipsUnsupportedDebianAndPiTools(t *testing.T) {
	for _, platform := range []pkg.Platform{pkg.PlatformDebian, pkg.PlatformPi} {
		t.Run(string(platform), func(t *testing.T) {
			runtime := registryRuntime(platform, map[string]bool{})
			a := &App{
				deepDiveConfig:       NewDeepDiveConfig(),
				manageInstalled:      make(map[string]bool),
				manageInstalledReady: true,
			}
			selected := a.collectSelectedToolsWithRuntime(runtime)
			for _, unsupported := range []string{"ghostty", "yazi"} {
				if containsToolID(selected, unsupported) {
					t.Errorf("unsupported %s unexpectedly entered %s install plan: %v", unsupported, platform, selected)
				}
				available, reason := coreToolConfigAvailable(runtime, unsupported)
				if available {
					t.Errorf("unsupported absent %s would be configured on %s", unsupported, platform)
				}
				if !strings.Contains(reason, string(platform)) || !strings.Contains(reason, "no external installation") {
					t.Errorf("skip reason is not truthful/actionable: %q", reason)
				}
			}
			for _, supported := range []string{"tmux", "zsh", "neovim", "git", "fzf"} {
				if !containsToolID(selected, supported) {
					t.Errorf("supported core %s missing from %s install plan: %v", supported, platform, selected)
				}
			}
		})
	}
}

func TestInjectedPlanningDoesNotProbeHostOrAdmitMacApps(t *testing.T) {
	cfg := NewDeepDiveConfig()
	cfg.MacApps["rectangle"] = true
	runtime := registryRuntime(pkg.PlatformDebian, map[string]bool{})
	a := &App{deepDiveConfig: cfg}

	selected := a.collectSelectedToolsWithRuntime(runtime)
	if containsToolID(selected, "rectangle") {
		t.Fatalf("injected Debian plan admitted a macOS-only app: %v", selected)
	}
	if a.manageInstalledReady || a.manageInstalled != nil {
		t.Fatalf("injected planning unexpectedly initialized the host cache: ready=%v cache=%v", a.manageInstalledReady, a.manageInstalled)
	}
}

func TestWizardExecutesBaseToolAgainstInjectedPiPlatform(t *testing.T) {
	mgr := pkg.NewMockPackageManager()
	zsh := tools.NewZshTool()
	runtime := toolInstallRuntime{
		lookupTool: func(id string) (tools.Tool, bool) {
			if id == zsh.ID() {
				return zsh, true
			}
			return nil, false
		},
		detectManager:  func() pkg.PackageManager { return mgr },
		detectPlatform: func() pkg.Platform { return pkg.PlatformPi },
		isToolInstalled: func(t tools.Tool) bool {
			return zsh.IsInstalledForPlatform(mgr, pkg.PlatformPi)
		},
		autoBackup: func() (autoBackupResult, error) { return autoBackupResult{}, nil },
	}

	result := runSelectedToolInstalls(context.Background(), []string{zsh.ID()}, runtime, func(string) {}, func(string) {})
	want := tools.PackagesForPlatform(zsh.Packages(), pkg.PlatformPi)
	if result.successCount != 1 || len(result.failures) != 0 {
		t.Fatalf("Pi install result = %#v", result)
	}
	if len(mgr.InstallCalls) != 1 || !reflect.DeepEqual(mgr.InstallCalls[0], want) {
		t.Fatalf("Pi execution installed %v, want %v", mgr.InstallCalls, want)
	}
}

func TestClaudeCustomInstallerUsesInjectedPiPrerequisitesAndNPMPhase(t *testing.T) {
	binDir := t.TempDir()
	npmPath := filepath.Join(binDir, "npm")
	if err := os.WriteFile(npmPath, []byte("#!/bin/sh\necho npm-phase\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	mgr := pkg.NewMockPackageManager()
	claude := tools.NewClaudeCodeTool()
	var lines []string
	err := installTool(context.Background(), claude, mgr, pkg.PlatformPi, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("injected Pi Claude install: %v", err)
	}
	want := tools.PackagesForPlatform(claude.Packages(), pkg.PlatformPi)
	if len(mgr.InstallCalls) != 1 || !reflect.DeepEqual(mgr.InstallCalls[0], want) {
		t.Fatalf("Claude Pi prerequisites = %v, want Debian fallback %v", mgr.InstallCalls, want)
	}
	if !reflect.DeepEqual(lines, []string{"npm-phase"}) {
		t.Fatalf("Claude custom npm phase was bypassed or not streamed: %v", lines)
	}
}

func TestUnsupportedOptionalToolsDefaultOffOnDebianAndPi(t *testing.T) {
	for _, platform := range []pkg.Platform{pkg.PlatformDebian, pkg.PlatformPi} {
		defaults := buildToolGroupDefaultsForPlatform(tools.UIGroupCLITools, platform)
		for _, id := range []string{"lazygit", "lazydocker", "glow"} {
			if defaults[id] {
				t.Errorf("%s defaulted on for unsupported platform %s: %v", id, platform, defaults)
			}
		}
	}
}

func TestAIInstallRowsExposeOnlyReviewedPlatformRoutes(t *testing.T) {
	for _, test := range []struct {
		id       string
		platform pkg.Platform
		want     bool
	}{
		{"codex", pkg.PlatformMacOS, true},
		{"codex", pkg.PlatformDebian, false},
		{"pi", pkg.PlatformMacOS, true},
		{"pi", pkg.PlatformPi, false},
		{"opencode", pkg.PlatformMacOS, true},
		{"opencode", pkg.PlatformArch, true},
		{"opencode", pkg.PlatformDebian, false},
	} {
		if got := cliToolAvailableForPlatform(test.id, test.platform); got != test.want {
			t.Errorf("%s availability on %s = %v, want %v", test.id, test.platform, got, test.want)
		}
	}
}

func TestSupportedInstallerDoesNotAuthorizeConfigWithoutFinalIdentity(t *testing.T) {
	runtime := registryRuntime(pkg.PlatformMacOS, map[string]bool{})
	for _, id := range []string{"ghostty", "tmux", "zsh", "neovim", "git", "yazi", "fzf", "lazygit", "btop", "glow", "claude-code"} {
		available, reason := coreToolConfigAvailable(runtime, id)
		if available {
			t.Errorf("missing %s passed final-identity config gate", id)
		}
		if !strings.Contains(reason, "not detected after installation") {
			t.Errorf("%s final-identity reason = %q", id, reason)
		}
	}
}

func TestUnsupportedPlatformStillConfiguresExternalCoreInstall(t *testing.T) {
	installed := map[string]bool{"ghostty": true, "yazi": true}
	runtime := registryRuntime(pkg.PlatformDebian, installed)
	a := &App{
		deepDiveConfig:       NewDeepDiveConfig(),
		manageInstalled:      installed,
		manageInstalledReady: true,
	}
	selected := a.collectSelectedToolsWithRuntime(runtime)
	for _, id := range []string{"ghostty", "yazi"} {
		if containsToolID(selected, id) {
			t.Errorf("externally installed %s unexpectedly entered package plan", id)
		}
		if available, reason := coreToolConfigAvailable(runtime, id); !available {
			t.Errorf("externally installed %s would not be configured: %s", id, reason)
		}
	}
}

func TestCollectSelectedToolsPreservesInstalledObservations(t *testing.T) {
	cfg := NewDeepDiveConfig()
	cfg.CLITools["claude-code"] = true
	installed := make(map[string]bool)
	for _, id := range alwaysConfiguredToolIDs {
		installed[id] = true
	}
	installed["claude-code"] = true
	a := &App{deepDiveConfig: cfg, manageInstalled: installed, manageInstalledReady: true}
	selected := a.collectSelectedTools()
	for _, id := range append(alwaysConfiguredToolIDs, "claude-code") {
		if containsToolID(selected, id) {
			t.Errorf("installed tool %q unexpectedly re-entered install plan: %v", id, selected)
		}
	}
}

func TestCustomToolPrerequisitePackageIsNotAuthoritativeObservation(t *testing.T) {
	sentinel := &customInstallSentinel{
		packages: map[pkg.Platform][]string{"all": {"node-prerequisite"}},
	}
	mgr := pkg.NewMockPackageManager()
	mgr.InstalledPkgs["node-prerequisite"] = "1"
	if tools.ObserveInstallations(context.Background(), []tools.Tool{sentinel}, mgr, pkg.DetectPlatform())[sentinel.ID()] {
		t.Fatal("prerequisite package presence incorrectly hid a missing custom-installed binary")
	}
}

func TestClaudeNodePresentWithoutClaudeRemainsMissingInPlan(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	claude := tools.NewClaudeCodeTool()
	platform := pkg.DetectPlatform()
	mgr := pkg.NewMockPackageManager()
	for _, packageName := range tools.PackagesForPlatform(claude.Packages(), platform) {
		mgr.InstalledPkgs[packageName] = "1"
	}
	if len(mgr.InstalledPkgs) == 0 {
		t.Fatal("Claude test requires prerequisite package metadata for this platform")
	}
	if tools.ObserveInstallations(context.Background(), []tools.Tool{claude}, mgr, platform)[claude.ID()] {
		t.Fatal("Node/npm package receipts incorrectly reported a missing Claude CLI as installed")
	}

	cfg := NewDeepDiveConfig()
	cfg.CLITools["claude-code"] = true
	observations := make(map[string]bool)
	for _, id := range alwaysConfiguredToolIDs {
		observations[id] = true
	}
	a := &App{deepDiveConfig: cfg, manageInstalled: observations, manageInstalledReady: true}
	if selected := a.collectSelectedTools(); !containsToolID(selected, "claude-code") {
		t.Fatalf("Claude did not enter the install plan when Node existed but its CLI was absent: %v", selected)
	}
}

func TestWizardInstallPhaseInvokesCustomToolAndReportsProgress(t *testing.T) {
	sentinel := &customInstallSentinel{}
	mgr := pkg.NewMockPackageManager()
	var lines []string
	steps := 0
	result := runSelectedToolInstalls(
		context.Background(),
		[]string{sentinel.ID()},
		sentinelRuntime(sentinel, mgr),
		func(line string) { lines = append(lines, line) },
		func(line string) { steps++ },
	)

	if sentinel.installCalls != 1 {
		t.Fatalf("wizard dispatched custom Tool.Install %d times, want 1", sentinel.installCalls)
	}
	if result.successCount != 1 || len(result.failures) != 0 || !result.installed[sentinel.ID()] {
		t.Fatalf("unexpected final install result: %#v (lines=%v)", result, lines)
	}
	if steps != 1 {
		t.Fatalf("wizard emitted %d progress steps, want 1", steps)
	}
	if len(mgr.InstallCalls) != 1 || mgr.InstallCalls[0][0] != "sentinel-runtime" {
		t.Fatalf("wizard did not route package work through streaming adapter: %#v", mgr.InstallCalls)
	}
}

func acceptedSentinelSnapshot(t *testing.T, runtime toolInstallRuntime, manager pkg.PackageManager) installExecutionSnapshot {
	t.Helper()
	platform := runtime.detectPlatform()
	recipe := operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        "custom-sentinel",
		Platform:      string(platform),
		Manager:       manager.Name(),
		Steps: []operation.InstallStep{{
			Kind: operation.InstallStepPackageManager, Provider: manager.Name(), Packages: []string{"reviewed-package"},
		}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{"reviewed-package"}},
		Risk:     "test package-manager mutation",
	}
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return installExecutionSnapshot{
		platform:  platform,
		manager:   manager.Name(),
		recipes:   map[string]operation.InstallRecipe{"custom-sentinel": recipe},
		detected:  map[string]bool{"custom-sentinel": false},
		digests:   map[string]string{"custom-sentinel": digest},
		authority: map[string]installToolAuthority{"custom-sentinel": {presence: health.PresenceMissing, intent: "install", recipeDigest: digest}},
	}
}

func TestAcceptedRecipeExecutesExactStepsAndNeverLegacyInstaller(t *testing.T) {
	sentinel := &customInstallSentinel{}
	mgr := pkg.NewMockPackageManager()
	runtime := sentinelRuntime(sentinel, mgr)
	// A recipe-backed install must ignore arbitrary Tool.IsInstalled behavior and
	// evaluate only the detector recorded in the accepted recipe.
	runtime.isToolInstalled = func(tools.Tool) bool { return true }
	snapshot := acceptedSentinelSnapshot(t, runtime, mgr)

	result := runSelectedToolInstalls(context.Background(), []string{sentinel.ID()}, runtime, func(string) {}, func(string) {}, snapshot)
	if len(result.failures) != 0 || result.successCount != 1 {
		t.Fatalf("recipe execution result = %#v", result)
	}
	if sentinel.installCalls != 0 {
		t.Fatalf("recipe-backed execution invoked legacy installer %d times", sentinel.installCalls)
	}
	if len(mgr.InstallCalls) != 1 || !reflect.DeepEqual(mgr.InstallCalls[0], []string{"reviewed-package"}) {
		t.Fatalf("executed packages = %#v", mgr.InstallCalls)
	}
}

func TestAcceptedRecipeFailsClosedOnEnvironmentOrDetectorDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*installExecutionSnapshot, *toolInstallRuntime)
	}{
		{name: "platform", mutate: func(snapshot *installExecutionSnapshot, _ *toolInstallRuntime) {
			snapshot.platform = pkg.PlatformUnknown
		}},
		{name: "manager", mutate: func(snapshot *installExecutionSnapshot, _ *toolInstallRuntime) {
			snapshot.manager = "different-manager"
		}},
		{name: "authority", mutate: func(snapshot *installExecutionSnapshot, _ *toolInstallRuntime) {
			snapshot.authority["custom-sentinel"] = installToolAuthority{presence: health.PresencePresent, intent: "none"}
		}},
		{name: "missing recipe", mutate: func(snapshot *installExecutionSnapshot, _ *toolInstallRuntime) {
			delete(snapshot.recipes, "custom-sentinel")
		}},
		{name: "tampered recipe", mutate: func(snapshot *installExecutionSnapshot, _ *toolInstallRuntime) {
			snapshot.recipes["custom-sentinel"].Steps[0].Packages[0] = "unreviewed-package"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			sentinel := &customInstallSentinel{}
			mgr := pkg.NewMockPackageManager()
			runtime := sentinelRuntime(sentinel, mgr)
			runtime.isToolInstalled = func(tools.Tool) bool { return false }
			snapshot := acceptedSentinelSnapshot(t, runtime, mgr)
			test.mutate(&snapshot, &runtime)
			result := runSelectedToolInstalls(context.Background(), []string{sentinel.ID()}, runtime, func(string) {}, func(string) {}, snapshot)
			if len(result.failures) == 0 || len(mgr.InstallCalls) != 0 || sentinel.installCalls != 0 {
				t.Fatalf("drift was not stopped before mutation: result=%#v calls=%#v legacy=%d", result, mgr.InstallCalls, sentinel.installCalls)
			}
		})
	}
}

func TestAcceptedRecipeRejectsOmittedAndDuplicateSelections(t *testing.T) {
	for _, selected := range [][]string{{}, {"custom-sentinel", "custom-sentinel"}} {
		sentinel := &customInstallSentinel{}
		mgr := pkg.NewMockPackageManager()
		runtime := sentinelRuntime(sentinel, mgr)
		snapshot := acceptedSentinelSnapshot(t, runtime, mgr)
		result := runSelectedToolInstalls(context.Background(), selected, runtime, func(string) {}, func(string) {}, snapshot)
		if len(result.failures) == 0 || len(mgr.InstallCalls) != 0 {
			t.Fatalf("selection %v was not rejected before mutation: %#v calls=%v", selected, result, mgr.InstallCalls)
		}
	}
}

func TestAcceptedT3RecipeUsesOnlyHomebrewCaskCapability(t *testing.T) {
	home := withTempHome(t)
	mgr := &caskInstallManager{MockPackageManager: pkg.NewMockPackageManager(), home: home}
	mgr.ManagerName = "brew"
	tool := tools.NewT3CodeTool()
	runtime := toolInstallRuntime{
		lookupTool:      func(id string) (tools.Tool, bool) { return tool, id == tool.ID() },
		detectManager:   func() pkg.PackageManager { return mgr },
		detectPlatform:  func() pkg.Platform { return pkg.PlatformMacOS },
		isToolInstalled: func(tools.Tool) bool { return false },
	}
	recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := installExecutionSnapshot{platform: pkg.PlatformMacOS, manager: "brew", recipes: map[string]operation.InstallRecipe{tool.ID(): recipe}, detected: map[string]bool{tool.ID(): false}, digests: map[string]string{tool.ID(): digest}, authority: map[string]installToolAuthority{tool.ID(): {presence: health.PresenceMissing, intent: "install", recipeDigest: digest}}}
	result := runSelectedToolInstalls(context.Background(), []string{tool.ID()}, runtime, func(string) {}, func(string) {}, snapshot)
	if len(result.failures) != 0 || result.successCount != 1 || !reflect.DeepEqual(mgr.calls, [][]string{{"t3-code"}}) {
		t.Fatalf("T3 execution result=%#v cask calls=%v", result, mgr.calls)
	}
	if len(mgr.InstallCalls) != 0 {
		t.Fatalf("T3 escaped to generic package install: %v", mgr.InstallCalls)
	}
}

func TestWorkerGlobalConfigErrorStopsBeforeEveryAction(t *testing.T) {
	sentinel := &customInstallSentinel{managerIndependent: true}
	events := make(chan installEventMsg, 8)
	runInstallWorkerWithRuntime(
		context.Background(),
		events,
		[]string{sentinel.ID()},
		DeepDiveConfig{},
		"dracula",
		sentinelRuntime(sentinel, nil),
		fmt.Errorf("malformed global.json"),
	)
	var received []installEventMsg
	for event := range events {
		received = append(received, event)
	}
	if sentinel.installCalls != 0 {
		t.Fatalf("worker invoked install after global-config failure: %d calls", sentinel.installCalls)
	}
	if len(received) != 1 || !received[0].done || received[0].err == nil || received[0].stepInc || received[0].line != "" {
		t.Fatalf("worker did not terminate with one fatal zero-progress result: %#v", received)
	}
}

func TestWorkerAutoBackupErrorStopsBeforeEveryMutation(t *testing.T) {
	sentinel := &customInstallSentinel{managerIndependent: true}
	runtime := sentinelRuntime(sentinel, nil)
	runtime.autoBackup = func() (autoBackupResult, error) {
		return autoBackupResult{enabled: true}, fmt.Errorf("backup unavailable")
	}
	events := make(chan installEventMsg, 8)
	runInstallWorkerWithRuntime(context.Background(), events, []string{sentinel.ID()}, DeepDiveConfig{}, "dracula", runtime)
	var received []installEventMsg
	for event := range events {
		received = append(received, event)
	}
	if sentinel.installCalls != 0 || len(received) != 1 || !received[0].done || received[0].err == nil || received[0].stepInc {
		t.Fatalf("backup failure did not stop with a zero-mutation terminal result: calls=%d events=%#v", sentinel.installCalls, received)
	}
	if !strings.Contains(received[0].err.Error(), "installation stopped before mutation") {
		t.Fatalf("backup failure error was not actionable: %v", received[0].err)
	}
}

func TestWorkerSurfacesConvenienceBackupOmission(t *testing.T) {
	sentinel := &customInstallSentinel{managerIndependent: true}
	runtime := sentinelRuntime(sentinel, nil)
	runtime.autoBackup = func() (autoBackupResult, error) {
		return autoBackupResult{enabled: true, count: 1, omission: "Yazi configs omitted: active config is outside HOME"}, nil
	}
	events := make(chan installEventMsg)
	go runInstallWorkerWithRuntime(context.Background(), events, []string{sentinel.ID()}, DeepDiveConfig{}, "dracula", runtime)
	found := false
	for event := range events {
		found = found || strings.Contains(event.line, "Yazi configs omitted")
	}
	if !found || sentinel.installCalls != 1 {
		t.Fatalf("omission surfaced=%t installCalls=%d", found, sentinel.installCalls)
	}
}

func TestInstallStreamClosureAndNilAreFatal(t *testing.T) {
	for _, tc := range []struct {
		name string
		app  *App
	}{
		{name: "nil", app: &App{}},
		{name: "closed", app: func() *App {
			ch := make(chan installEventMsg)
			close(ch)
			return &App{installEvents: ch}
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := tc.app.listenInstallEventsCmd()().(installEventMsg)
			if !ok || !msg.done || !errors.Is(msg.err, errInstallStreamClosed) {
				t.Fatalf("stream closure result = %#v", msg)
			}
		})
	}
}

func TestWorkerCancellationAlwaysDeliversTerminalResult(t *testing.T) {
	sentinel := &customInstallSentinel{managerIndependent: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events := make(chan installEventMsg, 1)
	runInstallWorkerWithRuntime(ctx, events, []string{sentinel.ID()}, DeepDiveConfig{}, "dracula", sentinelRuntime(sentinel, nil))
	received := <-events
	if !received.done || !errors.Is(received.err, context.Canceled) {
		t.Fatalf("cancellation terminal result = %#v", received)
	}
}

func TestTerminalResultDisplacesOutputWhenChannelIsFull(t *testing.T) {
	events := make(chan installEventMsg, 1)
	events <- installEventMsg{line: "old output"}
	sentinel := &customInstallSentinel{managerIndependent: true}
	runInstallWorkerWithRuntime(context.Background(), events, []string{sentinel.ID()}, DeepDiveConfig{}, "dracula", sentinelRuntime(sentinel, nil), fmt.Errorf("global config failed"))
	received := <-events
	if !received.done || received.err == nil {
		t.Fatalf("full channel lost terminal result: %#v", received)
	}
}

func TestInstallEventLineIsBoundedBeforeChannelEmission(t *testing.T) {
	events := make(chan installEventMsg, 1)
	emitInstallEvent(context.Background(), events, strings.Repeat("x", maxCollectedInstallLineBytes+1024), false)
	received := <-events
	if len(received.line) > maxCollectedInstallLineBytes+32 || !strings.HasSuffix(received.line, "[truncated]") {
		t.Fatalf("emitted line was not bounded: %d bytes", len(received.line))
	}
}

func TestManagerIndependentPackageFreeInstallerRunsWithoutManager(t *testing.T) {
	sentinel := &customInstallSentinel{managerIndependent: true}
	result := runSelectedToolInstalls(
		context.Background(),
		[]string{sentinel.ID()},
		sentinelRuntime(sentinel, nil),
		func(string) {},
		func(string) {},
	)
	if result.successCount != 1 || len(result.failures) != 0 || sentinel.installCalls != 1 {
		t.Fatalf("manager-independent result = %#v, installCalls=%d", result, sentinel.installCalls)
	}
}

func TestInstallPostconditionFailureIsNotReportedAsSuccess(t *testing.T) {
	sentinel := &customInstallSentinel{managerIndependent: true, remainMissing: true}
	steps := 0
	result := runSelectedToolInstalls(
		context.Background(),
		[]string{sentinel.ID()},
		sentinelRuntime(sentinel, nil),
		func(string) {},
		func(string) { steps++ },
	)
	if result.successCount != 0 || len(result.failures) != 1 || result.installed[sentinel.ID()] {
		t.Fatalf("postcondition failure was not preserved in final result: %#v", result)
	}
	if !strings.Contains(result.failures[0].Error(), "postcondition failed") || steps != 1 {
		t.Fatalf("unexpected postcondition error/progress: failures=%v steps=%d", result.failures, steps)
	}
}

func TestContextCustomInstallerCancellationStopsInstallPhase(t *testing.T) {
	started := make(chan struct{})
	sentinel := &customInstallSentinel{
		managerIndependent: true,
		blockUntilCancel:   true,
		started:            started,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan selectedToolInstallResult, 1)
	go func() {
		done <- runSelectedToolInstalls(ctx, []string{sentinel.ID()}, sentinelRuntime(sentinel, nil), func(string) {}, func(string) {})
	}()
	<-started
	cancel()
	select {
	case result := <-done:
		if len(result.failures) == 0 || !errors.Is(result.failures[0], context.Canceled) {
			t.Fatalf("cancellation result = %#v, want context.Canceled", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("context-aware custom installer did not stop after cancellation")
	}
}
