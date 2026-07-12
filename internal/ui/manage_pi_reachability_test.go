package ui

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func assertReviewedPiPlan(t *testing.T, app *App, cmdPresent bool) {
	t.Helper()
	if !cmdPresent {
		t.Fatal("Pi install returned no navigation to reviewed plan")
	}
	plan := app.pendingInstallPlan
	if app.installPlanError != nil || plan == nil {
		t.Fatalf("Pi reviewed plan nonnil=%v error=%v", plan != nil, app.installPlanError)
	}
	actions := plan.actions()
	if len(actions) != 1 {
		t.Fatalf("Pi actions=%+v, want exactly one install action", actions)
	}
	action := actions[0]
	if action.ID != "install:pi" || action.Kind != operation.KindInstallTool || action.ToolID != "pi" || action.Disposition != operation.DispositionApply {
		t.Fatalf("Pi action=%+v, want one applicable typed install action", action)
	}
	if action.InstallRecipe == nil {
		t.Fatal("Pi action omitted reviewed install recipe")
	}
	recipe := action.InstallRecipe
	wantArgs := []string{"install", "-g", "--ignore-scripts", "@earendil-works/pi-coding-agent@0.80.3"}
	if recipe.SchemaVersion != operation.CurrentInstallRecipeSchemaVersion || recipe.ToolID != "pi" || recipe.Platform != string(pkg.PlatformMacOS) || recipe.Manager != "brew" {
		t.Fatalf("Pi recipe identity=%+v", recipe)
	}
	if len(recipe.Steps) != 2 || recipe.Steps[0].Kind != operation.InstallStepPackageManager || recipe.Steps[0].Provider != "brew" || !slices.Equal(recipe.Steps[0].Packages, []string{"node"}) || recipe.Steps[1].Kind != operation.InstallStepNPMGlobal || recipe.Steps[1].Provider != "npm" || !slices.Equal(recipe.Steps[1].Args, wantArgs) {
		t.Fatalf("Pi recipe steps=%+v, want Brew node then exact pinned npm arguments %v", recipe.Steps, wantArgs)
	}
	if recipe.Detector.Kind != operation.InstallDetectorBinary || !slices.Equal(recipe.Detector.Values, []string{"pi"}) {
		t.Fatalf("Pi detector=%+v, want binary:pi", recipe.Detector)
	}
	if want := "provider login or API key; local providers may require neither"; recipe.Authentication != want {
		t.Fatalf("Pi authentication=%q, want %q", recipe.Authentication, want)
	}
	if want := "installs an npm package with lifecycle scripts disabled; package code runs when Pi is launched"; recipe.Risk != want {
		t.Fatalf("Pi risk=%q, want %q", recipe.Risk, want)
	}
	digest := installRecipeDigest(*recipe)
	if digest == "" || action.DesiredDigest != digest || plan.installRecipes["pi"].ToolID != "pi" || plan.installTools["pi"].recipeDigest != digest {
		t.Fatalf("Pi recipe authority mismatch: action=%q recipe=%q plan=%+v", action.DesiredDigest, digest, plan.installTools["pi"])
	}
	for _, candidate := range actions {
		if candidate.Kind == operation.KindUpdateState || candidate.Kind == operation.KindWriteConfig || candidate.Kind == operation.KindInstallFile {
			t.Fatalf("Pi package-only review leaked state/config/helper action: %+v", candidate)
		}
	}

	// The action projection is a defensive clone. Mutating it after review must
	// neither change the accepted plan nor its authority hash/digest.
	hashBefore := plan.hash()
	actions[0].Description = "mutated"
	actions[0].InstallRecipe.Steps[0].Packages[0] = "mutated-node"
	actions[0].InstallRecipe.Steps[1].Args[len(wantArgs)-1] = "mutated-package"
	reread := plan.actions()
	if plan.hash() != hashBefore || reread[0].Description == "mutated" || !slices.Equal(reread[0].InstallRecipe.Steps[0].Packages, []string{"node"}) || !slices.Equal(reread[0].InstallRecipe.Steps[1].Args, wantArgs) || reread[0].DesiredDigest != digest {
		t.Fatalf("Pi plan projection mutation changed accepted authority: hash %q/%q action=%+v", hashBefore, plan.hash(), reread[0])
	}
}

func TestManagePiInstallIsReachableFromEitherPaneAndCase(t *testing.T) {
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
					assertReviewedPiPlan(t, app, cmd != nil)
					msg, ok := cmd().(NavigateMsg)
					if !ok || msg.To != ScreenFileTree {
						t.Fatalf("Pi navigation=%#v, want FileTree review", msg)
					}
					view := stripANSITest(NewFileTreeScreen(ctx).View(80, 24))
					actionVerb := "install pi"
					if presence == health.PresencePartial {
						actionVerb = "repair pi"
					}
					for _, want := range []string{actionVerb, "@earendil-works/pi-c", "Auth:", "Risk:"} {
						if !strings.Contains(view, want) {
							t.Errorf("Pi confirmation omitted %q:\n%s", want, view)
						}
					}
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
	for _, id := range []string{"cursor-agent", "hermes"} {
		if _, registered := registry.Get(id); registered {
			t.Errorf("unreviewed integration %q was fabricated as supported", id)
		}
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
	var allDescriptions []string
	for _, item := range GetDeepDiveMenuItems() {
		descriptions[item.Screen] = item.Description
		allDescriptions = append(allDescriptions, item.Description)
	}
	for screen, names := range map[Screen][]string{
		ScreenConfigCLITools: {"Codex", "OpenCode", "Pi"},
		ScreenConfigGUIApps:  {"Cursor", "LM Studio", "OBS"},
		ScreenConfigMacApps:  {"T3 Code"},
	} {
		for _, name := range names {
			if !strings.Contains(descriptions[screen], name) {
				t.Errorf("deep-dive overview for screen %d omits supported integration %q: %q", screen, name, descriptions[screen])
			}
		}
	}
	joined := strings.Join(allDescriptions, "\n")
	for _, forbidden := range []string{"Cursor Agent", "Hermes"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("deep-dive overview fabricated unsupported integration %q", forbidden)
		}
	}
}

func TestDeepDiveSupportedIntegrationRowsRenderAtSupportedSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		ctx := newGoldenContext(t)
		ctx.Width, ctx.Height = size.width, size.height
		ctx.app.width, ctx.app.height = size.width, size.height
		ctx.app.installCacheLoading = false
		views := []string{
			stripANSITest(NewConfigCLIToolsScreen(ctx).View(size.width, size.height)),
			stripANSITest(NewConfigGUIAppsScreen(ctx).View(size.width, size.height)),
			stripANSITest(NewConfigMacAppsScreen(ctx).View(size.width, size.height)),
		}
		joined := strings.Join(views, "\n")
		for _, want := range []string{"Codex", "OpenCode", "Pi", "Cursor", "LM Studio", "OBS Studio", "T3 Code"} {
			if !strings.Contains(joined, want) {
				t.Errorf("%dx%d deep-dive rows omit %q", size.width, size.height, want)
			}
		}
		for _, forbidden := range []string{"Cursor Agent", "Hermes"} {
			if strings.Contains(joined, forbidden) {
				t.Errorf("%dx%d fabricated unsupported row %q", size.width, size.height, forbidden)
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
