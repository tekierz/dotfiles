package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func assertPiPhaseReady(t *testing.T, app *App, cmdPresent bool, action string, presence health.Presence) {
	t.Helper()
	if !cmdPresent || app.pendingInstallPlan == nil || app.installPlanError != nil || app.manageStatus != action+" requested" {
		t.Fatalf("Pi phase review=(cmd=%v,plan=%v,error=%v,status=%q), want command/plan/nil/%q", cmdPresent, app.pendingInstallPlan != nil, app.installPlanError, app.manageStatus, action+" requested")
	}
	phase, phased := app.pendingInstallPlan.phase()
	wantKind := operation.InstallPhasePrerequisite
	wantAuthority := operation.InstallAuthorityManager
	wantRemaining := []string{"pi"}
	if presence == health.PresencePartial {
		wantKind = operation.InstallPhaseNPM
		wantAuthority = operation.InstallAuthorityNPM
		wantRemaining = nil
	}
	if !phased || phase.Kind() != wantKind || phase.Authority() != wantAuthority ||
		!slices.Equal(phase.RequestedTools(), []string{"pi"}) || !slices.Equal(phase.RemainingTools(), wantRemaining) {
		t.Fatalf("Pi reviewed phase = %+v (phased=%v)", phase, phased)
	}
}

func TestManagePiInstallReachesReviewedPrerequisitePhaseFromEitherPaneAndCase(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)
	// Plan construction observes these identities; the returned install command is never run.
	native, err := os.ReadFile("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "node"), native, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "npm"), []byte("#!/usr/bin/env node\n// reviewed npm identity fixture\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, presence := range []health.Presence{health.PresenceMissing, health.PresencePartial} {
		for _, pane := range []int{managePaneTools, managePaneSettings} {
			for _, key := range []string{"i", "I"} {
				name := strings.Join([]string{string(presence), key, map[bool]string{true: "tools", false: "settings"}[pane == managePaneTools]}, "/")
				t.Run(name, func(t *testing.T) {
					ctx := newGoldenContext(t)
					app := ctx.app
					setManageTruthSnapshot(t, app, 81, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "pi", presence))
					selectManageTruthItem(t, app, "pi")
					app.managePane = pane
					cmd := NewManageScreen(ctx).handleKey(keyMsg(key))
					action := "Install"
					if presence == health.PresencePartial {
						action = "Repair"
					}
					assertPiPhaseReady(t, app, cmd != nil, action, presence)
				})
			}
		}
	}
}

func TestManagePiInstallFailsClosedFromToolsPane(t *testing.T) {
	tests := []struct {
		name           string
		presence       health.Presence
		installability health.Installability
		stale          bool
		snapshotError  string
		wantStatus     string
	}{
		{name: "present", presence: health.PresencePresent, installability: health.InstallabilitySupported, wantStatus: "Already installed"},
		{name: "unknown", presence: health.PresenceUnknown, installability: health.InstallabilitySupported, wantStatus: "Installation status unknown"},
		{name: "unsupported", presence: health.PresenceMissing, installability: health.InstallabilityUnsupported, wantStatus: "Installation unavailable on macos"},
		{name: "installability unknown", presence: health.PresenceMissing, installability: health.InstallabilityUnknown, wantStatus: "Installation availability unknown"},
		{name: "stale", presence: health.PresenceMissing, installability: health.InstallabilitySupported, stale: true, wantStatus: "Installation status stale"},
		{name: "snapshot error", presence: health.PresenceMissing, installability: health.InstallabilitySupported, snapshotError: "private probe detail", wantStatus: installationSnapshotUnavailable},
	}
	for _, test := range tests {
		for _, pane := range []int{managePaneTools, managePaneSettings} {
			for _, key := range []string{"i", "I"} {
				name := strings.Join([]string{test.name, key, map[bool]string{true: "tools", false: "settings"}[pane == managePaneTools]}, "/")
				t.Run(name, func(t *testing.T) {
					ctx := newGoldenContext(t)
					app := ctx.app
					setManageTruthSnapshot(t, app, 83, pkg.PlatformMacOS, "brew", manageTruthObservationWithInstallability(t, "pi", test.presence, test.installability))
					selectManageTruthItem(t, app, "pi")
					app.managePane = pane
					app.installationSnapshotStale = test.stale
					app.installationSnapshotError = test.snapshotError
					if cmd := NewManageScreen(ctx).handleKey(keyMsg(key)); cmd != nil || app.pendingInstallPlan != nil || app.manageStatus != test.wantStatus {
						t.Fatalf("blocked Pi install=(cmd=%v,plan=%v,status=%q), want nil/nil/%q", cmd != nil, app.pendingInstallPlan != nil, app.manageStatus, test.wantStatus)
					}
				})
			}
		}
	}
}

func TestManageEditingConsumesPiInstallKey(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	setManageTruthSnapshot(t, app, 85, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "pi", health.PresenceMissing))
	selectManageTruthItem(t, app, "pi")
	app.managePane = managePaneSettings
	app.manageEditing = true
	app.manageEditValue = "before"
	app.manageEditCursor = len([]rune(app.manageEditValue))
	if cmd := NewManageScreen(ctx).handleKey(keyMsg("i")); cmd != nil || app.pendingInstallPlan != nil || app.manageEditValue != "beforei" {
		t.Fatalf("editing Pi key=(cmd=%v,plan=%v,value=%q), want text insertion only", cmd != nil, app.pendingInstallPlan != nil, app.manageEditValue)
	}
}

func TestDeepDiveSelectorRegistryParity(t *testing.T) {
	rows := map[tools.UIGroup][]string{
		tools.UIGroupCLITools:     make([]string, 0, len(cliToolItems)),
		tools.UIGroupCLIUtilities: make([]string, 0, len(cliUtilityItems)),
		tools.UIGroupGUIApps:      make([]string, 0, len(guiAppItems)),
		tools.UIGroupMacApps:      make([]string, 0, len(macAppItems)),
	}
	for _, item := range cliToolItems {
		rows[tools.UIGroupCLITools] = append(rows[tools.UIGroupCLITools], item.id)
	}
	for _, item := range cliUtilityItems {
		rows[tools.UIGroupCLIUtilities] = append(rows[tools.UIGroupCLIUtilities], item.id)
	}
	for _, item := range guiAppItems {
		rows[tools.UIGroupGUIApps] = append(rows[tools.UIGroupGUIApps], item.id)
	}
	for _, item := range macAppItems {
		rows[tools.UIGroupMacApps] = append(rows[tools.UIGroupMacApps], item.id)
	}

	registry := tools.NewRegistry()
	want := make(map[tools.UIGroup][]string, len(rows))
	for _, tool := range registry.All() {
		group := tool.UIGroup()
		switch group {
		case tools.UIGroupNone:
			if !tool.HasConfig() || tool.ConfigScreen() == 0 {
				t.Errorf("registry tool %q has UIGroupNone without a dedicated config surface: hasConfig=%v screen=%d", tool.ID(), tool.HasConfig(), tool.ConfigScreen())
			}
			continue
		case tools.UIGroupCLITools, tools.UIGroupCLIUtilities, tools.UIGroupGUIApps, tools.UIGroupMacApps:
			want[group] = append(want[group], tool.ID())
		case tools.UIGroupUtilities:
			t.Errorf("registry tool %q uses unrepresented helper-script UIGroup %q", tool.ID(), group)
		default:
			t.Errorf("registry tool %q has unknown or unrepresented UIGroup %q", tool.ID(), group)
		}
	}
	for group := range rows {
		slices.Sort(rows[group])
		slices.Sort(want[group])
		if !slices.Equal(rows[group], want[group]) {
			t.Errorf("deep-dive group %q rows=%v, registry=%v", group, rows[group], want[group])
		}
		for index, id := range rows[group] {
			if index > 0 && rows[group][index-1] == id {
				t.Errorf("deep-dive group %q duplicates %q", group, id)
			}
			tool, ok := registry.Get(id)
			if !ok || tool.UIGroup() != group {
				t.Errorf("deep-dive row %q in %q has no matching registry identity", id, group)
			}
		}
	}

	// Claude Code is intentionally present as a non-navigable context row in CLI
	// Tools and owns its settings on the dedicated Claude screen.
	claudeCount := 0
	for index, item := range cliToolItems {
		if item.id != "claude-code" {
			continue
		}
		claudeCount++
		if index < navigableCLIToolCount {
			t.Error("Claude dedicated/context row became navigable")
		}
	}
	if claudeCount != 1 || navigableCLIToolCount != len(cliToolItems)-claudeCount {
		t.Fatalf("Claude dedicated/context exception count=%d navigable=%d rows=%d", claudeCount, navigableCLIToolCount, len(cliToolItems))
	}
	cursorAgentCount := 0
	for _, tool := range registry.All() {
		if tool.ID() == "cursor-agent" {
			cursorAgentCount++
			if tool.UIGroup() != tools.UIGroupCLITools {
				t.Errorf("cursor-agent UIGroup=%q, want CLI Tools", tool.UIGroup())
			}
		}
	}
	if cursorAgentCount != 1 {
		t.Errorf("cursor-agent registry count=%d, want exactly one", cursorAgentCount)
	}
	hermesCount := 0
	for _, tool := range registry.All() {
		if tool.ID() == "hermes" {
			hermesCount++
			if tool.UIGroup() != tools.UIGroupCLITools {
				t.Errorf("hermes UIGroup=%q, want CLI Tools", tool.UIGroup())
			}
		}
	}
	if hermesCount != 1 {
		t.Errorf("hermes registry count=%d, want exactly one", hermesCount)
	}
	if tool, ok := registry.Get("t3-code"); !ok || tool.PlatformFilter() != pkg.PlatformMacOS || !slices.Contains(rows[tools.UIGroupMacApps], "t3-code") {
		t.Fatal("T3 Code must remain a macOS-filtered deep-dive integration")
	}
}

func TestDeepDiveDefaultsApplyRegistryPlatformFilters(t *testing.T) {
	macApps := buildToolGroupDefaultsForPlatform(tools.UIGroupMacApps, pkg.PlatformMacOS)
	linuxApps := buildToolGroupDefaultsForPlatform(tools.UIGroupMacApps, pkg.PlatformDebian)
	registry := tools.NewRegistry()
	for _, tool := range registry.All() {
		if tool.UIGroup() != tools.UIGroupMacApps || tool.PlatformFilter() != pkg.PlatformMacOS {
			continue
		}
		if _, ok := macApps[tool.ID()]; !ok {
			t.Errorf("macOS defaults omit platform-filtered tool %q", tool.ID())
		}
		if _, ok := linuxApps[tool.ID()]; ok {
			t.Errorf("Linux defaults expose macOS-filtered tool %q", tool.ID())
		}
	}
	if enabled, ok := macApps["t3-code"]; !ok || enabled {
		t.Fatalf("T3 Code macOS default=(present=%v,enabled=%v), want present and opt-in", ok, enabled)
	}
}

func TestDeepDiveOverviewMakesSupportedNewIntegrationsDiscoverable(t *testing.T) {
	descriptions := make(map[Screen]string)
	for _, item := range GetDeepDiveMenuItems() {
		descriptions[item.Screen] = item.Description
	}
	for screen, names := range map[Screen][]string{
		ScreenConfigCLITools: {"Codex", "Cursor Agent", "Hermes Agent", "OpenCode", "Pi"},
		ScreenConfigGUIApps:  {"Cursor", "LM Studio", "OBS"},
		ScreenConfigMacApps:  {"T3 Code"},
	} {
		for _, name := range names {
			if !strings.Contains(descriptions[screen], name) {
				t.Errorf("deep-dive overview for screen %d omits supported integration %q: %q", screen, name, descriptions[screen])
			}
		}
	}
}

func TestDeepDiveSupportedIntegrationRowsRenderAtSupportedSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		ctx := newGoldenContext(t)
		ctx.Width, ctx.Height = size.width, size.height
		ctx.app.width, ctx.app.height = size.width, size.height
		ctx.app.installCacheLoading = false
		cliScreen := NewConfigCLIToolsScreen(ctx)
		for _, target := range []struct {
			id   string
			name string
		}{
			{id: "codex", name: "Codex"},
			{id: "cursor-agent", name: "Cursor Agent"},
			{id: "hermes", name: "Hermes Agent"},
			{id: "opencode", name: "OpenCode"},
			{id: "pi", name: "Pi"},
		} {
			ctx.app.cliToolIndex = cliToolIndexForTest(t, target.id)
			view := stripANSITest(cliScreen.View(size.width, size.height))
			if !strings.Contains(view, target.name) {
				t.Errorf("%dx%d focused CLI viewport omits %q", size.width, size.height, target.name)
			}
		}
		views := []string{
			stripANSITest(NewConfigGUIAppsScreen(ctx).View(size.width, size.height)),
			stripANSITest(NewConfigMacAppsScreen(ctx).View(size.width, size.height)),
		}
		joined := strings.Join(views, "\n")
		for _, want := range []string{"Cursor", "LM Studio", "OBS Studio", "T3 Code"} {
			if !strings.Contains(joined, want) {
				t.Errorf("%dx%d deep-dive rows omit %q", size.width, size.height, want)
			}
		}
	}
}

func TestReviewedPiRecipeFixtureMatchesRegistry(t *testing.T) {
	tool, ok := tools.NewRegistry().Get("pi")
	if !ok {
		t.Fatal("Pi missing from registry")
	}
	recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "-g", "--ignore-scripts", "@earendil-works/pi-coding-agent@0.80.3"}
	if !reflect.DeepEqual(recipe.Steps[1].Args, want) {
		t.Fatalf("Pi args=%v, want %v", recipe.Steps[1].Args, want)
	}
}
