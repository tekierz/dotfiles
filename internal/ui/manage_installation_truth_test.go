package ui

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type manageInstallationTruthProjection interface {
	installationTruth() (health.Presence, health.Installability)
}

type manageInstallationActionProjection interface {
	installationAction() string
}

func manageTruthObservation(t *testing.T, id string, presence health.Presence) health.InstallationObservation {
	return manageTruthObservationWithInstallability(t, id, presence, health.InstallabilitySupported)
}

func manageTruthObservationWithInstallability(t *testing.T, id string, presence health.Presence, installability health.Installability) health.InstallationObservation {
	t.Helper()
	spec := health.InstallationObservationSpec{
		ToolID:         id,
		Installability: installability,
		Direct:         health.DirectFacet{State: health.ComponentNotApplicable},
	}
	var recipe operation.InstallRecipe
	if installability == health.InstallabilitySupported {
		tool, ok := tools.NewRegistry().Get(id)
		if !ok {
			t.Fatalf("registry tool %q missing", id)
		}
		var err error
		recipe, err = tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
		if err != nil {
			t.Fatalf("describe %s install: %v", id, err)
		}
		spec.InstallRecipeDigest = installRecipeDigest(recipe)
	} else {
		switch presence {
		case health.PresencePresent:
			spec.Package = health.PackageFacet{State: health.PackagePresent, Provider: "brew", ExpectedReceipts: []string{id}, ObservedReceipts: []string{id}, Authoritative: true, Complete: true}
		case health.PresencePartial:
			spec.Package = health.PackageFacet{State: health.PackagePartial, Provider: "brew", ExpectedReceipts: []string{id, id + "-extra"}, ObservedReceipts: []string{id}, MissingReceipts: []string{id + "-extra"}, Authoritative: true, Complete: true}
		case health.PresenceMissing:
			spec.Package = health.PackageFacet{State: health.PackageMissing, Provider: "brew", ExpectedReceipts: []string{id}, MissingReceipts: []string{id}, Authoritative: true, Complete: true}
		case health.PresenceUnknown:
			spec.Package = health.PackageFacet{State: health.PackageUnknown, Provider: "brew", ExpectedReceipts: []string{id}, UnresolvedReceipts: []string{id}, Authoritative: true, Complete: false}
		default:
			t.Fatalf("unsupported test presence %q", presence)
		}
		observation, err := health.NewInstallationObservation(spec)
		if err != nil {
			t.Fatal(err)
		}
		return observation
	}

	packageReceipts := make([]string, 0)
	seenReceipts := make(map[string]struct{})
	for _, step := range recipe.Steps {
		var receipts []string
		switch step.Kind {
		case operation.InstallStepPackageManager:
			receipts = step.Packages
		case operation.InstallStepHomebrewCask:
			receipts = step.Casks
		}
		for _, receipt := range receipts {
			if _, seen := seenReceipts[receipt]; seen {
				continue
			}
			seenReceipts[receipt] = struct{}{}
			packageReceipts = append(packageReceipts, receipt)
		}
	}

	directKind := health.DirectSourceKind("")
	switch recipe.Detector.Kind {
	case operation.InstallDetectorBinary:
		directKind = health.DirectSourceBinary
	case operation.InstallDetectorAppBundle:
		directKind = health.DirectSourceAppBundle
	}
	setDirect := func(state health.ComponentState) {
		if directKind == "" {
			spec.Direct = health.DirectFacet{State: health.ComponentNotApplicable}
			return
		}
		spec.Direct = health.DirectFacet{
			State:         state,
			Authoritative: true,
			Alternatives:  []health.DirectAlternative{{Kind: directKind, Identifiers: recipe.Detector.Values, State: state}},
		}
	}
	setPackages := func(state health.PackageState, observed, missing, unresolved []string, complete bool) {
		if len(packageReceipts) == 0 {
			spec.Package = health.PackageFacet{State: health.PackageNotApplicable}
			return
		}
		spec.Package = health.PackageFacet{
			State: state, Provider: "brew", ExpectedReceipts: packageReceipts,
			ObservedReceipts: observed, MissingReceipts: missing, UnresolvedReceipts: unresolved,
			Authoritative: true, Complete: complete,
		}
	}

	switch presence {
	case health.PresencePresent:
		setPackages(health.PackagePresent, packageReceipts, nil, nil, true)
		setDirect(health.ComponentPresent)
	case health.PresencePartial:
		if directKind != "" {
			setPackages(health.PackagePresent, packageReceipts, nil, nil, true)
			setDirect(health.ComponentMissing)
		} else {
			detectorReceipt := recipe.Detector.Values[0]
			observed := make([]string, 0, len(packageReceipts))
			missing := []string{detectorReceipt}
			for _, receipt := range packageReceipts {
				if receipt != detectorReceipt {
					observed = append(observed, receipt)
				}
			}
			if len(observed) == 0 {
				observed = append(observed, detectorReceipt+"-repair")
				packageReceipts = append(packageReceipts, detectorReceipt+"-repair")
			}
			setPackages(health.PackagePartial, observed, missing, nil, true)
			setDirect(health.ComponentNotApplicable)
		}
	case health.PresenceMissing:
		setPackages(health.PackageMissing, nil, packageReceipts, nil, true)
		setDirect(health.ComponentMissing)
	case health.PresenceUnknown:
		setPackages(health.PackageUnknown, nil, nil, packageReceipts, false)
		setDirect(health.ComponentUnknown)
	default:
		t.Fatalf("unsupported test presence %q", presence)
	}
	observation, err := health.NewInstallationObservation(spec)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func setManageTruthSnapshot(t *testing.T, app *App, generation uint64, platform pkg.Platform, manager string, observations ...health.InstallationObservation) {
	t.Helper()
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: generation, Platform: string(platform), Manager: manager, Tools: observations})
	if err != nil {
		t.Fatal(err)
	}
	app.installationSnapshotGeneration = generation
	app.installationSnapshotTerminal = true
	app.installationSnapshot = snapshot
	app.installationSnapshotReady = true
	app.installationSnapshotLoading = false
	app.installationSnapshotStale = false
	app.installationSnapshotError = ""
	app.installCacheLoading = false
	app.manageInstalledReady = true
}

func selectManageTruthItem(t *testing.T, app *App, id string) manageItem {
	t.Helper()
	items := app.manageItems()
	for index, item := range items {
		if item.id == id {
			app.manageIndex = index
			return item
		}
	}
	t.Fatalf("manage item %q not found", id)
	return manageItem{}
}

func selectedManageTruthRow(view, name string) string {
	visible := strings.ToUpper(stripANSITest(view))
	wantName := strings.ToUpper(name)
	for _, line := range strings.Split(visible, "\n") {
		normalized := strings.Join(strings.Fields(line), " ")
		if !strings.HasPrefix(normalized, "▸ [") {
			continue
		}
		tokenEnd := strings.Index(normalized, "] ")
		if tokenEnd > len("▸ [") && strings.HasPrefix(normalized[tokenEnd+2:], wantName+" • ") {
			return normalized
		}
	}
	return ""
}

func TestManageInstallationTruthRendersFourStatesAtSupportedSizes(t *testing.T) {
	const generation = 7
	states := []struct {
		id       string
		presence health.Presence
		label    string
		action   string
	}{
		{id: "ghostty", presence: health.PresencePresent, label: "INSTALLED", action: "none"},
		{id: "tmux", presence: health.PresencePartial, label: "PARTIAL — REPAIR", action: "repair"},
		{id: "neovim", presence: health.PresenceMissing, label: "NOT INSTALLED", action: "install"},
		{id: "yazi", presence: health.PresenceUnknown, label: "STATUS UNKNOWN", action: "blocked"},
	}
	observations := make([]health.InstallationObservation, 0, len(states))
	for _, state := range states {
		observations = append(observations, manageTruthObservation(t, state.id, state.presence))
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{
		Generation: generation,
		Platform:   string(pkg.PlatformMacOS),
		Manager:    "brew",
		Tools:      observations,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}} {
		for _, state := range states {
			name := state.id + "/" + strconv.Itoa(size.width) + "x" + strconv.Itoa(size.height)
			t.Run(name, func(t *testing.T) {
				ctx := newGoldenContext(t)
				app := ctx.app
				app.installationSnapshotGeneration = generation
				app.installationSnapshotTerminal = true
				app.installationSnapshot = snapshot
				app.installationSnapshotReady = true
				app.installationSnapshotLoading = false
				app.installationSnapshotStale = false
				app.installationSnapshotError = ""
				app.installCacheLoading = false
				app.manageInstalledReady = true
				app.manageInstalled = map[string]bool{
					"ghostty": false,
					"tmux":    true,
					"neovim":  true,
					"yazi":    true,
				}
				app.width, app.height = size.width, size.height
				ctx.Width, ctx.Height = size.width, size.height
				items := app.manageItems()
				app.manageIndex = -1
				var selected manageItem
				for index, item := range items {
					if item.id == state.id {
						app.manageIndex = index
						selected = item
						break
					}
				}
				if app.manageIndex < 0 {
					t.Fatalf("typed-observed tool %q was filtered out", state.id)
				}
				projection, ok := any(selected).(manageInstallationTruthProjection)
				if !ok {
					t.Errorf("selected manage item %q exposes no typed installation truth", state.id)
				} else if presence, installability := projection.installationTruth(); presence != state.presence || installability != health.InstallabilitySupported {
					t.Errorf("selected manage item typed truth=(%s,%s), want=(%s,%s)", presence, installability, state.presence, health.InstallabilitySupported)
				}
				action, ok := any(selected).(manageInstallationActionProjection)
				if !ok {
					t.Errorf("selected manage item %q exposes no typed installation action", state.id)
				} else if got := action.installationAction(); got != state.action {
					t.Errorf("selected manage item action=%q, want=%q", got, state.action)
				}
				app.managePane = managePaneTools
				visible := strings.ToUpper(stripANSITest(NewManageScreen(ctx).View(size.width, size.height)))
				selectedLine := selectedManageTruthRow(visible, selected.name)
				if selectedLine == "" {
					t.Fatalf("selected row for %q not rendered:\n%s", selected.name, visible)
				}
				wantRow := "▸ [" + tools.ApplicationTypeToken(selected.applicationType) + "] " + strings.ToUpper(selected.name) + " • " + state.label
				if size.width == 60 {
					if selectedLine != wantRow {
						t.Errorf("compact selected row=%q, want exact %q", selectedLine, wantRow)
					}
				} else if !strings.Contains(selectedLine, strings.ToUpper(selected.name)) || !strings.Contains(selectedLine, state.label) {
					t.Errorf("selected detail row=%q, want tool %q and state %q on the same row", selectedLine, selected.name, state.label)
				}
			})
		}
	}
}

func TestManageInstallationTruthBlocksActionsByStateAndInstallability(t *testing.T) {
	tests := []struct {
		name           string
		presence       health.Presence
		installability health.Installability
		legacy         bool
		wantCommand    bool
		wantStatus     string
		wantAction     string
		wantLabel      string
	}{
		{name: "present", presence: health.PresencePresent, installability: health.InstallabilitySupported, legacy: false, wantStatus: "Already installed", wantAction: "none", wantLabel: "installed"},
		{name: "partial supported", presence: health.PresencePartial, installability: health.InstallabilitySupported, legacy: true, wantCommand: true, wantStatus: "Repair requested", wantAction: "repair", wantLabel: "partial — repair"},
		{name: "missing supported", presence: health.PresenceMissing, installability: health.InstallabilitySupported, legacy: true, wantCommand: true, wantStatus: "Install requested", wantAction: "install", wantLabel: "not installed"},
		{name: "unknown presence", presence: health.PresenceUnknown, installability: health.InstallabilitySupported, legacy: false, wantStatus: "Installation status unknown", wantAction: "blocked", wantLabel: "status unknown"},
		{name: "partial unsupported", presence: health.PresencePartial, installability: health.InstallabilityUnsupported, legacy: false, wantStatus: "Installation unavailable on macos", wantAction: "blocked", wantLabel: "partial — unavailable"},
		{name: "missing unsupported", presence: health.PresenceMissing, installability: health.InstallabilityUnsupported, legacy: false, wantStatus: "Installation unavailable on macos", wantAction: "blocked", wantLabel: "not installed"},
		{name: "partial installability unknown", presence: health.PresencePartial, installability: health.InstallabilityUnknown, legacy: false, wantStatus: "Installation availability unknown", wantAction: "blocked", wantLabel: "partial — availability unknown"},
		{name: "missing installability unknown", presence: health.PresenceMissing, installability: health.InstallabilityUnknown, legacy: false, wantStatus: "Installation availability unknown", wantAction: "blocked", wantLabel: "not installed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := newGoldenContext(t)
			app := ctx.app
			setManageTruthSnapshot(t, app, 11, pkg.PlatformMacOS, "brew", manageTruthObservationWithInstallability(t, "ghostty", test.presence, test.installability))
			app.manageInstalled = map[string]bool{"ghostty": test.legacy}
			selected := selectManageTruthItem(t, app, "ghostty")
			if action, ok := any(selected).(manageInstallationActionProjection); !ok {
				t.Errorf("selected item exposes no typed action projection")
			} else if got := action.installationAction(); got != test.wantAction {
				t.Errorf("typed action=%q, want=%q", got, test.wantAction)
			}
			if got := selected.installationLabel(); got != test.wantLabel {
				t.Errorf("typed label=%q, want=%q", got, test.wantLabel)
			}
			app.managePane = managePaneSettings
			cmd := NewManageScreen(ctx).handleKey(keyMsg("i"))
			if (cmd != nil) != test.wantCommand {
				t.Errorf("I command present=%v, want=%v", cmd != nil, test.wantCommand)
			}
			if app.manageStatus != test.wantStatus {
				t.Errorf("I status=%q, want exact %q", app.manageStatus, test.wantStatus)
			}
		})
	}
}

func TestManageInstallationTruthShowsLoadingStaleAndErrorAtSupportedSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			ctx := newGoldenContext(t)
			app := ctx.app
			setManageTruthSnapshot(t, app, 13, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "ghostty", health.PresencePresent))
			app.manageInstalled = map[string]bool{"ghostty": false}
			app.width, app.height = size.width, size.height
			ctx.Width, ctx.Height = size.width, size.height
			screen := NewManageScreen(ctx)

			app.installationSnapshotReady = false
			app.installationSnapshotLoading = true
			app.installCacheLoading = true
			if view := strings.ToUpper(stripANSITest(screen.View(size.width, size.height))); !strings.Contains(view, "LOADING INSTALLATION STATUS") {
				t.Errorf("loading view omitted exact loading banner:\n%s", view)
			}

			app.installationSnapshotLoading = false
			app.installCacheLoading = false
			app.installationSnapshotStale = true
			selected := selectManageTruthItem(t, app, "ghostty")
			app.managePane = managePaneTools
			staleView := strings.ToUpper(stripANSITest(screen.View(size.width, size.height)))
			staleRow := selectedManageTruthRow(staleView, selected.name)
			if !strings.Contains(staleView, "STALE") {
				t.Errorf("stale last-good view omitted stale banner:\n%s", staleView)
			}
			if !strings.Contains(staleRow, "GHOSTTY") || !strings.Contains(staleRow, "INSTALLED") {
				t.Errorf("stale selected row=%q, want Ghostty + Installed on the same row", staleRow)
			}
			app.managePane = managePaneSettings
			if cmd := screen.handleKey(keyMsg("i")); cmd != nil || app.manageStatus != "Installation status stale" {
				t.Errorf("stale I=(cmd=%v,status=%q), want blocked generic stale", cmd != nil, app.manageStatus)
			}

			app.installationSnapshotError = installationSnapshotUnavailable
			app.manageStatus = ""
			app.managePane = managePaneTools
			errorView := strings.ToUpper(stripANSITest(screen.View(size.width, size.height)))
			errorRow := selectedManageTruthRow(errorView, selected.name)
			if !strings.Contains(errorView, strings.ToUpper(installationSnapshotUnavailable)) || !strings.Contains(errorView, "STALE") || strings.Contains(errorView, "SECRET-PROBE-DETAIL") {
				t.Errorf("error view omitted generic stale status or leaked raw detail:\n%s", errorView)
			}
			if !strings.Contains(errorRow, "GHOSTTY") || !strings.Contains(errorRow, "INSTALLED") {
				t.Errorf("error selected row=%q, want Ghostty + Installed on the same row", errorRow)
			}
			app.managePane = managePaneSettings
			if cmd := screen.handleKey(keyMsg("i")); cmd != nil || app.manageStatus != installationSnapshotUnavailable {
				t.Errorf("error I=(cmd=%v,status=%q), want generic blocked error", cmd != nil, app.manageStatus)
			}
		})
	}
}

func TestManageInstallationTruthMissingObservationIsUnknownAndBlocked(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	setManageTruthSnapshot(t, app, 17, pkg.PlatformMacOS, "brew")
	app.manageInstalled = map[string]bool{"ghostty": true}
	if _, observed := app.installationHealthObservation("ghostty"); observed {
		t.Fatal("fixture unexpectedly contains ghostty observation")
	}
	selectManageTruthItem(t, app, "ghostty")
	app.managePane = managePaneTools
	view := strings.ToUpper(stripANSITest(NewManageScreen(ctx).View(60, 18)))
	selectedRow := selectedManageTruthRow(view, "Ghostty")
	if selectedRow != "▸ [TERM] GHOSTTY • STATUS UNKNOWN" || strings.Contains(selectedRow, "NOT INSTALLED") {
		t.Errorf("absent observation did not render fail-closed unknown:\n%s", view)
	}
	app.managePane = managePaneSettings
	screen := NewManageScreen(ctx)
	if cmd := screen.handleKey(keyMsg("i")); cmd != nil || app.manageStatus != "Installation status unknown" {
		t.Errorf("absent observation I=(cmd=%v,status=%q), want unknown blocked", cmd != nil, app.manageStatus)
	}
}

type manageProbeTool struct {
	tools.Tool
	packageCalls     int
	isInstalledCalls int
	installCalls     int
	directCalls      int
	recipeCalls      int
}

func (tool *manageProbeTool) Packages() map[pkg.Platform][]string {
	tool.packageCalls++
	return map[pkg.Platform][]string{pkg.PlatformArch: {"appcleaner"}}
}
func (tool *manageProbeTool) IsInstalled() bool { tool.isInstalledCalls++; return false }
func (tool *manageProbeTool) Install(pkg.PackageManager) error {
	tool.installCalls++
	return errors.New("unexpected install")
}
func (tool *manageProbeTool) InstallationDirectAlternatives(tools.DirectInstallationObservation) []health.DirectAlternative {
	tool.directCalls++
	return nil
}
func (tool *manageProbeTool) InstallRecipe(tools.InstallEnvironment) (operation.InstallRecipe, error) {
	tool.recipeCalls++
	return operation.InstallRecipe{}, errors.New("unexpected recipe probe")
}

func TestManageInstallationTruthUsesSnapshotPlatformAndPerformsNoProbes(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	probe := &manageProbeTool{Tool: tools.NewAppCleanerTool()}
	source := []tools.Tool{probe, tools.NewGhosttyTool()}
	platformCalls := 0
	app.manageToolSource = func() []tools.Tool { return source }
	app.manageDetectPlatform = func() pkg.Platform {
		platformCalls++
		return pkg.PlatformMacOS
	}
	setManageTruthSnapshot(t, app, 19, pkg.PlatformArch, "paru", manageTruthObservation(t, "appcleaner", health.PresenceMissing))
	app.manageInstalled = map[string]bool{"appcleaner": false}
	items := app.manageItems()
	found := false
	for index, item := range items {
		if item.id != "appcleaner" {
			continue
		}
		found = true
		app.manageIndex = index
		if projection, ok := any(item).(manageInstallationTruthProjection); !ok {
			t.Errorf("snapshot-platform item exposes no typed truth projection")
		} else if presence, installability := projection.installationTruth(); presence != health.PresenceMissing || installability != health.InstallabilitySupported {
			t.Errorf("snapshot-platform truth=(%s,%s)", presence, installability)
		}
		break
	}
	if !found {
		t.Errorf("Arch-supported appcleaner was filtered using host platform instead of snapshot platform")
	}
	app.managePane = managePaneTools
	_ = NewManageScreen(ctx).View(80, 24)
	if found {
		app.managePane = managePaneSettings
		_ = NewManageScreen(ctx).handleKey(keyMsg("i"))
	}
	if platformCalls != 0 || probe.packageCalls != 0 || probe.isInstalledCalls != 0 || probe.installCalls != 0 || probe.directCalls != 0 || probe.recipeCalls != 0 {
		t.Errorf("Manage performed live probes: detectPlatform=%d packages=%d isInstalled=%d install=%d direct=%d recipe=%d", platformCalls, probe.packageCalls, probe.isInstalledCalls, probe.installCalls, probe.directCalls, probe.recipeCalls)
	}
	if len(source) != 2 || source[0].ID() != "appcleaner" || source[1].ID() != "ghostty" {
		t.Errorf("Manage mutated injected tool-source order: %v, want [appcleaner ghostty]", []string{source[0].ID(), source[1].ID()})
	}
}
