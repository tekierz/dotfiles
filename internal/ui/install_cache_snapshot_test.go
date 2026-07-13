package ui

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type cacheSnapshotRuntimeCounters struct {
	health    atomic.Int32
	utilities atomic.Int32
	platform  atomic.Int32
	manager   atomic.Int32
}

func cacheSnapshotRuntime(counters *cacheSnapshotRuntimeCounters, snapshot health.InstallationSnapshot, healthErr error, utilities map[string]bool) installationSnapshotCacheRuntime {
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = snapshot.Manager()
	return installationSnapshotCacheRuntime{
		allTools:       func() []tools.Tool { return nil },
		detectPlatform: func() pkg.Platform { counters.platform.Add(1); return pkg.Platform(snapshot.Platform()) },
		detectManager:  func() pkg.PackageManager { counters.manager.Add(1); return manager },
		observeHealth: func(_ context.Context, _ []tools.Tool, gotManager pkg.PackageManager, gotPlatform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
			counters.health.Add(1)
			if generation != snapshot.Generation() || gotPlatform != pkg.Platform(snapshot.Platform()) || gotManager.Name() != snapshot.Manager() {
				return health.InstallationSnapshot{}, errors.New("runtime received wrong generation or environment")
			}
			return snapshot, healthErr
		},
		observeUtilities: func(context.Context, uint64) (map[string]bool, error) {
			counters.utilities.Add(1)
			return utilities, nil
		},
	}
}

func cacheObservation(t *testing.T, id string, presence health.Presence) health.InstallationObservation {
	t.Helper()
	spec := health.InstallationObservationSpec{ToolID: id, Installability: health.InstallabilityUnknown, Package: health.PackageFacet{State: health.PackageNotApplicable}, Direct: health.DirectFacet{State: health.ComponentNotApplicable}}
	switch presence {
	case health.PresencePresent:
		spec.Direct = health.DirectFacet{State: health.ComponentPresent, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: health.DirectSourceLegacy, Identifiers: []string{id}, State: health.ComponentPresent}}}
	case health.PresencePartial:
		spec.Package = health.PackageFacet{State: health.PackagePartial, ExpectedReceipts: []string{"one", "two"}, ObservedReceipts: []string{"one"}, MissingReceipts: []string{"two"}, Authoritative: true, Complete: true}
	case health.PresenceMissing:
		spec.Package = health.PackageFacet{State: health.PackageMissing, ExpectedReceipts: []string{id}, MissingReceipts: []string{id}, Authoritative: true, Complete: true}
	case health.PresenceUnknown:
		spec.Package = health.PackageFacet{State: health.PackageUnknown, ExpectedReceipts: []string{id}, UnresolvedReceipts: []string{id}, Authoritative: true, Complete: false}
	}
	observation, err := health.NewInstallationObservation(spec)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func cacheSnapshot(t *testing.T, generation uint64, observations ...health.InstallationObservation) health.InstallationSnapshot {
	t.Helper()
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: generation, Platform: "macos", Manager: "brew", Tools: observations})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func applyCacheSnapshotResult(t *testing.T, app *App, message installationSnapshotDoneMsg) {
	t.Helper()
	app.applyInstallationSnapshotDone(message)
}

func coherentCacheResult(generation uint64, snapshot health.InstallationSnapshot, utilities map[string]bool) installationSnapshotDoneMsg {
	return installationSnapshotDoneMsg{Generation: generation, Platform: pkg.Platform(snapshot.Platform()), Manager: snapshot.Manager(), Snapshot: snapshot, Utilities: utilities}
}

// seedTypedReadyInstallCache gives screen tests one coherent, immutable cache
// generation. Tests may still mutate the compatibility map afterward when they
// need cosmetic installed-state variants, but Init must see typed planning
// readiness rather than the retired boolean cache alone.
func seedTypedReadyInstallCache(t *testing.T, app *App, installed map[string]bool) {
	t.Helper()
	registry := tools.NewRegistry()
	all := registry.All()
	observations := make([]health.InstallationObservation, 0, len(all))
	cosmetic := make(map[string]bool, len(installed))
	for id, present := range installed {
		cosmetic[id] = present
	}
	for _, tool := range all {
		present := installed[tool.ID()]
		observations = append(observations, planningCacheObservation(t, tool, present))
		cosmetic[tool.ID()] = present
	}
	snapshot := cacheSnapshot(t, 1, observations...)
	app.installationSnapshotGeneration = 1
	app.installationSnapshotTerminal = true
	app.installationSnapshot = snapshot
	app.installationSnapshotReady = true
	app.installationSnapshotLoading = false
	app.installationSnapshotStale = false
	app.installationSnapshotError = ""
	app.installationSnapshotUtilities = map[string]bool{}
	app.installationSnapshotCosmetic = cosmetic
	app.manageInstalled = maps.Clone(cosmetic)
	app.manageInstalledReady = true
	app.installCacheLoading = false
}

func planningCacheObservation(t *testing.T, tool tools.Tool, present bool) health.InstallationObservation {
	t.Helper()
	recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		alternatives, unavailable := unavailablePlanningDirectAlternatives(tool, err)
		if !unavailable {
			t.Fatalf("describe %s test install: %v", tool.ID(), err)
		}
		direct := health.DirectFacet{State: health.ComponentUnknown, Authoritative: true, Alternatives: alternatives}
		if len(alternatives) == 1 {
			direct.DiagnosticCode = alternatives[0].DiagnosticCode
			direct.DiagnosticSummary = alternatives[0].DiagnosticSummary
		}
		observation, observationErr := health.NewInstallationObservation(health.InstallationObservationSpec{
			ToolID: tool.ID(), Installability: health.InstallabilityUnsupported,
			Package: health.PackageFacet{State: health.PackageNotApplicable}, Direct: direct,
		})
		if observationErr != nil {
			t.Fatalf("build unavailable %s planning observation: %v", tool.ID(), observationErr)
		}
		return observation
	}
	packageFacet := health.PackageFacet{State: health.PackageNotApplicable}
	directFacet := health.DirectFacet{State: health.ComponentNotApplicable}
	if recipe.Detector.Kind == operation.InstallDetectorPackageReceipt {
		receipts := slices.Clone(recipe.Detector.Values)
		packageFacet = health.PackageFacet{
			State: health.PackageMissing, Provider: "brew", ExpectedReceipts: receipts,
			MissingReceipts: receipts, Authoritative: true, Complete: true,
		}
		if present {
			packageFacet.State = health.PackagePresent
			packageFacet.ObservedReceipts = receipts
			packageFacet.MissingReceipts = nil
		}
	} else {
		state := health.ComponentMissing
		if present {
			state = health.ComponentPresent
		}
		kind := health.DirectSourceBinary
		if recipe.Detector.Kind == operation.InstallDetectorAppBundle {
			kind = health.DirectSourceAppBundle
		}
		directFacet = health.DirectFacet{State: state, Authoritative: true, Alternatives: []health.DirectAlternative{{Kind: kind, Identifiers: slices.Clone(recipe.Detector.Values), State: state}}}
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
		ToolID: tool.ID(), Installability: health.InstallabilitySupported,
		InstallRecipeDigest: installRecipeDigest(recipe), Package: packageFacet,
		Direct: directFacet,
	})
	if err != nil {
		t.Fatalf("build %s planning observation: %v", tool.ID(), err)
	}
	return observation
}

func unavailablePlanningDirectAlternatives(tool tools.Tool, describeErr error) ([]health.DirectAlternative, bool) {
	if tool == nil || describeErr == nil || len(tool.Packages()) != 0 {
		return nil, false
	}
	if _, recipeProvider := tool.(tools.InstallRecipeProvider); recipeProvider {
		return nil, false
	}
	reasonProvider, ok := tool.(interface{ InstallationUnavailableReason() string })
	if !ok {
		return nil, false
	}
	reason := strings.TrimSpace(reasonProvider.InstallationUnavailableReason())
	if reason == "" || !strings.Contains(describeErr.Error(), reason) {
		return nil, false
	}
	directProvider, ok := tool.(tools.InstallationDirectAlternativesProvider)
	if !ok {
		return nil, false
	}
	provided := directProvider.InstallationDirectAlternatives(tools.DirectInstallationObservation{FlatpakApplications: map[string]bool{}})
	if len(provided) == 0 {
		return nil, false
	}
	alternatives := make([]health.DirectAlternative, len(provided))
	for index, alternative := range provided {
		if alternative.State != health.ComponentUnknown {
			return nil, false
		}
		alternative.Identifiers = slices.Clone(alternative.Identifiers)
		alternatives[index] = alternative
	}
	return alternatives, true
}

func TestPlanningCacheUnavailableToolIgnoresCosmeticPresent(t *testing.T) {
	observation := planningCacheObservation(t, tools.NewCursorAgentTool(), true)
	if observation.Presence() != health.PresenceUnknown || observation.Installability() != health.InstallabilityUnsupported || observation.InstallRecipeDigest() != "" {
		t.Fatalf("unavailable cosmetic-present observation=(%q,%q,%q), want unknown/unsupported/empty digest", observation.Presence(), observation.Installability(), observation.InstallRecipeDigest())
	}
	direct := observation.Direct()
	if !direct.Authoritative || direct.State != health.ComponentUnknown || len(direct.Alternatives) != 1 || direct.Alternatives[0].State != health.ComponentUnknown {
		t.Fatalf("unavailable direct fixture=%+v", direct)
	}
}

func TestInstallationSnapshotCacheRequestGenerationRejectsStaleFutureAndMismatchedResults(t *testing.T) {
	app := &App{}
	gen1 := app.beginInstallationSnapshotLoad(cacheSnapshotRuntime(&cacheSnapshotRuntimeCounters{}, cacheSnapshot(t, 1, cacheObservation(t, "one", health.PresencePresent)), nil, map[string]bool{"hk": true}))
	if gen1 == nil || app.installationSnapshotCacheView().Generation != 1 {
		t.Fatal("generation 1 did not start")
	}
	gen2 := app.beginInstallationSnapshotLoad(cacheSnapshotRuntime(&cacheSnapshotRuntimeCounters{}, cacheSnapshot(t, 2, cacheObservation(t, "two", health.PresenceMissing)), nil, map[string]bool{"caff": true}))
	if gen2 == nil || app.installationSnapshotCacheView().Generation != 2 {
		t.Fatal("generation 2 did not start")
	}
	loadingGen2 := app.installationSnapshotCacheView()
	for _, staleWhileLoading := range []installationSnapshotDoneMsg{
		coherentCacheResult(1, cacheSnapshot(t, 1, cacheObservation(t, "stale", health.PresencePresent)), map[string]bool{"stale": true}),
		{Generation: 1, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New("stale error")},
	} {
		applyCacheSnapshotResult(t, app, staleWhileLoading)
		if got := app.installationSnapshotCacheView(); !reflect.DeepEqual(got, loadingGen2) || !got.Loading || got.Ready || app.pendingInstallPlan != nil {
			t.Fatalf("stale result mutated active generation 2: got=%+v want=%+v", got, loadingGen2)
		}
	}
	accepted := cacheSnapshot(t, 2, cacheObservation(t, "two", health.PresenceMissing))
	applyCacheSnapshotResult(t, app, coherentCacheResult(2, accepted, map[string]bool{"caff": true}))
	baseline := app.installationSnapshotCacheView()
	conflicting := cacheSnapshot(t, 2, cacheObservation(t, "conflict", health.PresencePresent))
	for _, stale := range []installationSnapshotDoneMsg{
		coherentCacheResult(2, accepted, map[string]bool{"caff": true}),
		coherentCacheResult(2, conflicting, map[string]bool{"conflict": true}),
		{Generation: 2, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New("same generation replay error"), Utilities: map[string]bool{"error": true}},
		coherentCacheResult(1, cacheSnapshot(t, 1, cacheObservation(t, "stale", health.PresencePresent)), map[string]bool{"hk": true}),
		{Generation: 1, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New("stale error"), Utilities: map[string]bool{"hk": true}},
		coherentCacheResult(3, cacheSnapshot(t, 3, cacheObservation(t, "future", health.PresencePresent)), map[string]bool{"future": true}),
		coherentCacheResult(2, cacheSnapshot(t, 9, cacheObservation(t, "mismatch", health.PresencePresent)), map[string]bool{"mismatch": true}),
		{Generation: 2, Platform: pkg.PlatformArch, Manager: "brew", Snapshot: accepted, Utilities: map[string]bool{"wrong-platform": true}},
		{Generation: 2, Platform: pkg.PlatformMacOS, Manager: "apt", Snapshot: accepted, Utilities: map[string]bool{"wrong-manager": true}},
		{Generation: 2, Platform: pkg.PlatformMacOS, Manager: "brew", Utilities: map[string]bool{"zero": true}},
	} {
		applyCacheSnapshotResult(t, app, stale)
		if got := app.installationSnapshotCacheView(); !reflect.DeepEqual(got, baseline) {
			t.Fatalf("stale/future/mismatched result mutated state:\n got=%+v\nwant=%+v", got, baseline)
		}
	}
	fresh := &App{}
	fresh.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	fresh.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	freshLoading := fresh.installationSnapshotCacheView()
	for _, stale := range []installationSnapshotDoneMsg{
		coherentCacheResult(1, cacheSnapshot(t, 1, cacheObservation(t, "old", health.PresencePresent)), map[string]bool{"old": true}),
		{Generation: 1, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New("old failure")},
	} {
		applyCacheSnapshotResult(t, fresh, stale)
		if got := fresh.installationSnapshotCacheView(); !reflect.DeepEqual(got, freshLoading) || !got.Loading || got.Stale || got.Ready {
			t.Fatalf("stale result mutated initial generation 2 load: got=%+v want=%+v", got, freshLoading)
		}
	}
}

func TestInstallationSnapshotCacheLastGoodStaleAndErrorPolicy(t *testing.T) {
	app := &App{}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	first := cacheSnapshot(t, 1, cacheObservation(t, "tool", health.PresencePresent))
	applyCacheSnapshotResult(t, app, coherentCacheResult(1, first, nil))
	app.pendingInstallPlan = &installPlan{}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	loading := app.installationSnapshotCacheView()
	if !loading.Loading || !loading.Stale || loading.Ready || loading.Snapshot.Digest() != first.Digest() || app.pendingInstallPlan != nil || app.installationSnapshotPlanningReady() {
		t.Fatalf("start did not retain stale display while invalidating planning: %+v", loading)
	}
	for _, stale := range []installationSnapshotDoneMsg{
		coherentCacheResult(1, cacheSnapshot(t, 1, cacheObservation(t, "stale", health.PresenceMissing)), map[string]bool{"stale": true}),
		{Generation: 1, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New("stale last-good error")},
	} {
		applyCacheSnapshotResult(t, app, stale)
		if got := app.installationSnapshotCacheView(); !reflect.DeepEqual(got, loading) || app.pendingInstallPlan != nil {
			t.Fatalf("stale gen1 mutated gen2 last-good load: got=%+v want=%+v", got, loading)
		}
	}
	raw := "DISTINCTIVE_RAW_FAILURE SECRET=/Users/private/credentials\n\x1b[31mred\x1b[0m\u202E\x00" + strings.Repeat("x", 500)
	applyCacheSnapshotResult(t, app, installationSnapshotDoneMsg{Generation: 2, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New(raw)})
	failed := app.installationSnapshotCacheView()
	if failed.Loading || !failed.Stale || failed.Ready || failed.Snapshot.Digest() != first.Digest() || failed.Error != "installation status unavailable" || len(failed.Error) > 256 || strings.Contains(failed.Error, "DISTINCTIVE_RAW_FAILURE") || strings.Contains(failed.Error, "/Users/private/credentials") || strings.Contains(failed.Error, "SECRET") || strings.Contains(failed.Error, "[31m") || strings.ContainsAny(failed.Error, "\x00\x1b\r\n") || strings.ContainsRune(failed.Error, '\u202e') || app.installationSnapshotPlanningReady() || app.pendingInstallPlan != nil {
		t.Fatalf("matching error violated last-good policy: %+v", failed)
	}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	third := cacheSnapshot(t, 3, cacheObservation(t, "tool", health.PresenceMissing))
	applyCacheSnapshotResult(t, app, coherentCacheResult(3, third, nil))
	ready := app.installationSnapshotCacheView()
	if !ready.Ready || ready.Loading || ready.Stale || ready.Error != "" || ready.Snapshot.Digest() != third.Digest() || !app.installationSnapshotPlanningReady() {
		t.Fatalf("matching success was not atomic: %+v", ready)
	}
}

func TestInstallationSnapshotCacheActiveRequestRejectsEnvironmentAndZeroSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message func(health.InstallationSnapshot) installationSnapshotDoneMsg
	}{
		{name: "wrong platform", message: func(snapshot health.InstallationSnapshot) installationSnapshotDoneMsg {
			return installationSnapshotDoneMsg{Generation: 2, Platform: pkg.PlatformArch, Manager: "brew", Snapshot: snapshot, Utilities: map[string]bool{"wrong": true}}
		}},
		{name: "wrong manager", message: func(snapshot health.InstallationSnapshot) installationSnapshotDoneMsg {
			return installationSnapshotDoneMsg{Generation: 2, Platform: pkg.PlatformMacOS, Manager: "apt", Snapshot: snapshot, Utilities: map[string]bool{"wrong": true}}
		}},
		{name: "zero snapshot", message: func(health.InstallationSnapshot) installationSnapshotDoneMsg {
			return installationSnapshotDoneMsg{Generation: 2, Platform: pkg.PlatformMacOS, Manager: "brew", Utilities: map[string]bool{"wrong": true}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{}
			app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
			lastGood := cacheSnapshot(t, 1, cacheObservation(t, "last-good", health.PresencePresent))
			applyCacheSnapshotResult(t, app, coherentCacheResult(1, lastGood, map[string]bool{"hk": true}))
			app.pendingInstallPlan = &installPlan{}
			app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
			candidate := cacheSnapshot(t, 2, cacheObservation(t, "candidate", health.PresenceMissing))
			applyCacheSnapshotResult(t, app, tc.message(candidate))
			got := app.installationSnapshotCacheView()
			if got.Loading || got.Ready || !got.Stale || got.Error != "installation status unavailable" || got.Snapshot.Digest() != lastGood.Digest() || !got.Utilities["hk"] || got.Utilities["wrong"] || app.installationSnapshotPlanningReady() || app.pendingInstallPlan != nil {
				t.Fatalf("active invalid terminal result did not fail closed: %+v", got)
			}
		})
	}
}

func TestInstallationSnapshotCacheInitialErrorPublishesNoToolState(t *testing.T) {
	app := &App{}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	applyCacheSnapshotResult(t, app, installationSnapshotDoneMsg{Generation: 1, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New("health unavailable"), Utilities: map[string]bool{"hk": true}})
	view := app.installationSnapshotCacheView()
	if view.Ready || view.Loading || view.Stale || view.Snapshot.Digest() != "" || view.Error == "" || len(view.CosmeticInstalled) != 0 {
		t.Fatalf("initial error published health state: %+v", view)
	}
	if _, ok := app.installationHealthObservation("missing"); ok {
		t.Fatal("initial error invented missing observation")
	}
}

func TestInstallationSnapshotCacheUtilitiesAreSeparateGenerationBoundAndDefensive(t *testing.T) {
	app := &App{}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	utilities := map[string]bool{"hk": true, "caff": false}
	applyCacheSnapshotResult(t, app, coherentCacheResult(1, cacheSnapshot(t, 1, cacheObservation(t, "tool", health.PresencePresent)), utilities))
	utilities["hk"] = false
	view := app.installationSnapshotCacheView()
	if !view.Utilities["hk"] || len(view.Snapshot.Tools()) != 1 || view.Snapshot.Tools()[0].ToolID() != "tool" {
		t.Fatalf("utilities leaked into or aliased health: %+v", view)
	}
	view.Utilities["hk"] = false
	if !app.installationSnapshotCacheView().Utilities["hk"] {
		t.Fatal("utility accessor exposed internal map")
	}
	baseline := app.installationSnapshotCacheView()
	applyCacheSnapshotResult(t, app, installationSnapshotDoneMsg{Generation: 0, Utilities: map[string]bool{"stale": true}})
	applyCacheSnapshotResult(t, app, installationSnapshotDoneMsg{Generation: 2, Utilities: map[string]bool{"future": true}})
	if !reflect.DeepEqual(app.installationSnapshotCacheView(), baseline) {
		t.Fatal("stale/future utilities mutated cache")
	}

	failed := &App{}
	failed.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	applyCacheSnapshotResult(t, failed, installationSnapshotDoneMsg{Generation: 1, Platform: pkg.PlatformMacOS, Manager: "brew", Err: errors.New("health failed"), Utilities: map[string]bool{"hk": true}})
	if failed.installationSnapshotCacheView().Ready {
		t.Fatal("utility success authorized failed health")
	}
}

func TestInstallationSnapshotCacheProjectionIsAtomicImmutableAndCosmeticOnly(t *testing.T) {
	app := &App{}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	snapshot := cacheSnapshot(t, 1,
		cacheObservation(t, "present", health.PresencePresent), cacheObservation(t, "partial", health.PresencePartial), cacheObservation(t, "missing", health.PresenceMissing), cacheObservation(t, "unknown", health.PresenceUnknown),
	)
	messageUtilities := map[string]bool{"hk": true}
	applyCacheSnapshotResult(t, app, coherentCacheResult(1, snapshot, messageUtilities))
	digest, canonical := snapshot.Digest(), snapshot.CanonicalBytes()
	messageUtilities["hk"] = false
	view := app.installationSnapshotCacheView()
	view.CosmeticInstalled["partial"] = true
	view.Utilities["hk"] = false
	if got := app.installationSnapshotCacheView(); got.Snapshot.Digest() != digest || !reflect.DeepEqual(got.Snapshot.CanonicalBytes(), canonical) || !got.CosmeticInstalled["present"] || got.CosmeticInstalled["partial"] || got.CosmeticInstalled["missing"] || got.CosmeticInstalled["unknown"] || !got.Utilities["hk"] {
		t.Fatalf("projection was mutable or unsafe: %+v", got)
	}
	if _, exists := app.installationSnapshotCacheView().CosmeticInstalled["absent"]; exists {
		t.Fatal("absent tool was projected as missing/false authority")
	}
	if observation, ok := app.installationHealthObservation("partial"); !ok || observation.Presence() != health.PresencePartial {
		t.Fatalf("typed lookup=(%+v,%v)", observation, ok)
	}
	if _, ok := app.installationHealthObservation("absent"); ok {
		t.Fatal("typed lookup invented absent entry")
	}
}

func TestInstallationSnapshotCacheCommandAndReducerPerformNoExtraProbes(t *testing.T) {
	counters := &cacheSnapshotRuntimeCounters{}
	snapshot := cacheSnapshot(t, 1, cacheObservation(t, "tool", health.PresencePresent))
	runtime := cacheSnapshotRuntime(counters, snapshot, nil, map[string]bool{"hk": true})
	app := &App{}
	cmd := app.beginInstallationSnapshotLoad(runtime)
	if cmd == nil {
		t.Fatal("cache command was nil")
	}
	if counters.health.Load() != 0 || counters.utilities.Load() != 0 || counters.platform.Load() != 0 || counters.manager.Load() != 0 {
		t.Fatalf("begin performed synchronous probes: health=%d utilities=%d platform=%d manager=%d", counters.health.Load(), counters.utilities.Load(), counters.platform.Load(), counters.manager.Load())
	}
	message, ok := cmd().(installationSnapshotDoneMsg)
	if !ok {
		t.Fatalf("command returned %T", cmd())
	}
	applyCacheSnapshotResult(t, app, message)
	_ = app.installationSnapshotCacheView()
	_, _ = app.installationHealthObservation("tool")
	_ = app.installationSnapshotPlanningReady()
	if counters.health.Load() != 1 || counters.utilities.Load() != 1 || counters.platform.Load() != 1 || counters.manager.Load() != 1 {
		t.Fatalf("probe counts health=%d utilities=%d platform=%d manager=%d", counters.health.Load(), counters.utilities.Load(), counters.platform.Load(), counters.manager.Load())
	}
}

func TestInstallationSnapshotCacheLegacyInvalidationStartsFreshGeneration(t *testing.T) {
	app := &App{}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	first := cacheSnapshot(t, 1, cacheObservation(t, "tool", health.PresencePresent))
	applyCacheSnapshotResult(t, app, coherentCacheResult(1, first, nil))
	app.pendingInstallPlan = &installPlan{}
	app.manageInstalledReady = false // existing install-success refresh signal
	cmd := app.startInstallCacheLoad()
	view := app.installationSnapshotCacheView()
	if cmd == nil || view.Generation != 2 || !view.Loading || !view.Stale || view.Ready || view.Snapshot.Digest() != first.Digest() || app.pendingInstallPlan != nil {
		t.Fatalf("legacy invalidation did not re-arm typed cache: cmd=%v view=%+v pending=%v", cmd != nil, view, app.pendingInstallPlan != nil)
	}
}

func TestInstallationSnapshotCacheLegacyReadyCannotSuppressFirstTypedGeneration(t *testing.T) {
	app := &App{manageInstalledReady: true, manageInstalled: map[string]bool{"legacy": true}}
	cmd := app.startInstallCacheLoad()
	view := app.installationSnapshotCacheView()
	if cmd == nil || view.Generation != 1 || !view.Loading || view.Ready || view.Snapshot.Digest() != "" {
		t.Fatalf("legacy cosmetic readiness suppressed typed generation: cmd=%v view=%+v", cmd != nil, view)
	}
}

func TestInstallationSnapshotCacheUtilityErrorRetainsLastObservedUtilities(t *testing.T) {
	app := &App{}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	first := cacheSnapshot(t, 1, cacheObservation(t, "tool", health.PresencePresent))
	applyCacheSnapshotResult(t, app, coherentCacheResult(1, first, map[string]bool{"hk": true, "caff": false}))
	app.manageInstalledReady = false
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	second := cacheSnapshot(t, 2, cacheObservation(t, "tool", health.PresenceMissing))
	message := coherentCacheResult(2, second, nil)
	message.UtilityErr = errors.New("utility probe failed")
	applyCacheSnapshotResult(t, app, message)
	view := app.installationSnapshotCacheView()
	if !view.Ready || view.Stale || view.Error != "" || !view.Utilities["hk"] || view.Utilities["caff"] {
		t.Fatalf("utility error erased last observation or poisoned health: %+v", view)
	}
}

func TestInstallationSnapshotCacheHelperScreenUsesSeparateUtilityResults(t *testing.T) {
	app := &App{deepDiveConfig: NewDeepDiveConfig()}
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	snapshot := cacheSnapshot(t, 1, cacheObservation(t, "tool", health.PresencePresent))
	applyCacheSnapshotResult(t, app, coherentCacheResult(1, snapshot, map[string]bool{"hk": true, "caff": false, "sshh": false}))
	if _, merged := app.manageInstalled["hk"]; merged {
		t.Fatal("utility leaked into tool cosmetic map")
	}
	utilities := app.installationUtilityResults()
	utilities["hk"] = false
	if installed, observed := app.installationUtilityInstalled("hk"); !observed || !installed {
		t.Fatalf("utility lookup=(%v,%v)", installed, observed)
	}
	screen := NewConfigUtilitiesScreen(&ScreenContext{app: app})
	before := app.deepDiveConfig.Utilities["hk"]
	screen.toggle(app, "hk")
	if app.deepDiveConfig.Utilities["hk"] != before {
		t.Fatal("installed helper remained toggleable")
	}
	if view := screen.View(80, 24); !strings.Contains(view, "(installed)") {
		t.Fatalf("helper screen omitted installed state:\n%s", view)
	}
}

func TestInstallationSnapshotCacheFileTreeSuccessRefreshesReviewedPlanBeforeEnter(t *testing.T) {
	app, _, _ := newPlanTestApp(t)
	app.screen = ScreenWelcome // stale legacy value must not control managed routing
	app.manageInstalledReady = false
	app.pendingInstallPlan, app.installPlanError = nil, nil
	app.beginInstallationSnapshotLoad(installationSnapshotCacheRuntime{})
	app.screenMgr.Navigate(ScreenFileTree)
	generation := app.installationSnapshotGeneration
	observations := make([]health.InstallationObservation, 0, len(tools.GetRegistry().All()))
	for _, tool := range tools.GetRegistry().All() {
		observations = append(observations, planningCacheObservation(t, tool, false))
	}
	snapshot := cacheSnapshot(t, generation, observations...)
	applyCacheSnapshotResult(t, app, coherentCacheResult(generation, snapshot, nil))
	if app.pendingInstallPlan == nil || app.installPlanError != nil {
		t.Fatalf("typed success did not prebuild reviewed plan: plan=%v err=%v", app.pendingInstallPlan != nil, app.installPlanError)
	}
	reviewed, hash := app.pendingInstallPlan, app.pendingInstallPlan.hash()
	screen := NewFileTreeScreen(&ScreenContext{app: app})
	_, cmd := screen.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if app.pendingInstallPlan != reviewed || app.pendingInstallPlan.hash() != hash {
		t.Fatal("Enter rebuilt or replaced the reviewed exact plan")
	}
	if cmd == nil {
		t.Fatal("Enter did not consume the already-reviewed plan")
	}
}
