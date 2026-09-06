package tools

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
)

type countingObservationManager struct {
	*pkg.MockPackageManager
	formulae       []pkg.Package
	formulaErr     error
	casks          []string
	caskErr        error
	formulaCalls   int
	caskCalls      int
	queryMu        sync.Mutex
	queryCalls     map[string]int
	queryInstalled map[string]bool
	blockQueries   bool
	listStarted    chan struct{}
	listRelease    chan struct{}
	activeQueries  atomic.Int32
}

func newCountingObservationManager() *countingObservationManager {
	mock := pkg.NewMockPackageManager()
	mock.ManagerName = "brew"
	return &countingObservationManager{
		MockPackageManager: mock,
		queryCalls:         make(map[string]int),
		queryInstalled:     make(map[string]bool),
	}
}

func (m *countingObservationManager) ListInstalled() ([]pkg.Package, error) {
	m.formulaCalls++
	if m.listStarted != nil {
		close(m.listStarted)
	}
	if m.listRelease != nil {
		<-m.listRelease
	}
	return m.formulae, m.formulaErr
}

func (m *countingObservationManager) ListInstalledCasks() ([]string, error) {
	m.caskCalls++
	return m.casks, m.caskErr
}

func (m *countingObservationManager) IsInstalledContext(ctx context.Context, name string) bool {
	m.activeQueries.Add(1)
	defer m.activeQueries.Add(-1)
	m.queryMu.Lock()
	m.queryCalls[name]++
	m.queryMu.Unlock()
	if m.blockQueries {
		<-ctx.Done()
		return false
	}
	return m.queryInstalled[name]
}

type directObservationTool struct {
	BaseTool
	direct        bool
	authoritative bool
	flatpakIDs    []string
}

func (t *directObservationTool) IsInstalledOutsidePackageManager(observation DirectInstallationObservation) bool {
	if t.direct {
		return true
	}
	for _, id := range t.flatpakIDs {
		if observation.FlatpakApplications[id] {
			return true
		}
	}
	return false
}

func (t *directObservationTool) FlatpakApplicationIDs() []string { return t.flatpakIDs }
func (t *directObservationTool) PackageMetadataIsAuthoritative() bool {
	return t.authoritative
}

func observationBaseTool(id string, packages ...string) *BaseTool {
	return &BaseTool{
		id: id, name: id, category: CategoryUtility,
		packages: map[pkg.Platform][]string{pkg.PlatformMacOS: packages},
	}
}

func observationDirectTool(id string, authoritative, direct bool, packages ...string) *directObservationTool {
	return &directObservationTool{
		BaseTool:      *observationBaseTool(id, packages...),
		direct:        direct,
		authoritative: authoritative,
	}
}

func TestObserveInstallationsUsesOneFormulaAndCaskBatchWithoutPackageProbes(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "one"}}
	mgr.casks = []string{"gui"}
	all := []Tool{
		observationBaseTool("complete", "one"),
		observationBaseTool("partial", "one", "two"),
		observationBaseTool("cask", "gui"),
	}

	got := ObserveInstallations(context.Background(), all, mgr, pkg.PlatformMacOS)
	if !got["complete"] || got["partial"] || !got["cask"] {
		t.Fatalf("observations=%v", got)
	}
	if mgr.formulaCalls != 1 || mgr.caskCalls != 1 || len(mgr.queryCalls) != 0 {
		t.Fatalf("formulaCalls=%d caskCalls=%d queryCalls=%v", mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls)
	}
}

func TestObserveInstallationsCombinesDirectAndMetadataPolicy(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "receipt"}}
	all := []Tool{
		observationDirectTool("external", true, true, "missing"),
		observationDirectTool("ai-missing", false, false, "receipt"),
		observationDirectTool("ai-present", false, true, "missing"),
	}
	got := ObserveInstallations(context.Background(), all, mgr, pkg.PlatformMacOS)
	if !got["external"] || got["ai-missing"] || !got["ai-present"] {
		t.Fatalf("observations=%v", got)
	}
	if len(mgr.queryCalls) != 0 {
		t.Fatalf("complete batch used per-package probes: %v", mgr.queryCalls)
	}
}

func TestObserveInstallationsBatchesFlatpakEvidenceOnce(t *testing.T) {
	mgr := newCountingObservationManager()
	first := observationDirectTool("first", true, false)
	first.flatpakIDs = []string{"app.first", "app.shared"}
	second := observationDirectTool("second", true, false)
	second.flatpakIDs = []string{"app.second", "app.shared"}
	flatpakCalls := 0
	runtime := defaultInstallationObservationRuntime()
	runtime.listFlatpakApplications = func(context.Context) ([]string, error) {
		flatpakCalls++
		return []string{"app.shared"}, nil
	}
	got := observeInstallationsWithRuntime(context.Background(), []Tool{first, second}, mgr, pkg.PlatformMacOS, runtime)
	if !got["first"] || !got["second"] || flatpakCalls != 1 {
		t.Fatalf("observations=%v flatpakCalls=%d", got, flatpakCalls)
	}
	if len(mgr.queryCalls) != 0 {
		t.Fatalf("flatpak observation caused package probes: %v", mgr.queryCalls)
	}
}

func TestObserveInstallationsBatchFailureUsesDedupedFallback(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulaErr = errors.New("batch unavailable")
	mgr.queryInstalled["shared"] = true
	mgr.queryInstalled["extra"] = true
	all := []Tool{
		observationBaseTool("one", "shared"),
		observationBaseTool("two", "shared", "extra"),
	}
	got := ObserveInstallations(context.Background(), all, mgr, pkg.PlatformMacOS)
	if !got["one"] || !got["two"] {
		t.Fatalf("observations=%v", got)
	}
	if mgr.formulaCalls != 1 || mgr.caskCalls != 0 || mgr.queryCalls["shared"] != 1 || mgr.queryCalls["extra"] != 1 || len(mgr.queryCalls) != 2 {
		t.Fatalf("formulaCalls=%d caskCalls=%d queryCalls=%v", mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls)
	}
}

func TestObserveInstallationsCaskFailurePreservesFormulaSnapshot(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "formula"}}
	mgr.caskErr = errors.New("cask batch unavailable")
	mgr.queryInstalled["cask"] = true
	all := []Tool{
		observationBaseTool("formula-tool", "formula"),
		observationBaseTool("cask-tool", "cask"),
	}
	got := ObserveInstallations(context.Background(), all, mgr, pkg.PlatformMacOS)
	if !got["formula-tool"] || !got["cask-tool"] {
		t.Fatalf("observations=%v", got)
	}
	if mgr.formulaCalls != 1 || mgr.caskCalls != 1 || mgr.queryCalls["formula"] != 0 || mgr.queryCalls["cask"] != 1 || len(mgr.queryCalls) != 1 {
		t.Fatalf("formulaCalls=%d caskCalls=%d queryCalls=%v", mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls)
	}
}

type nonContextObservationManager struct {
	*pkg.MockPackageManager
	queryCalls int
}

func (m *nonContextObservationManager) ListInstalled() ([]pkg.Package, error) {
	return nil, errors.New("batch unavailable")
}

func (m *nonContextObservationManager) IsInstalled(string) bool {
	m.queryCalls++
	return true
}

func TestObserveInstallationsBatchFailureFailsClosedForNonContextManager(t *testing.T) {
	mgr := &nonContextObservationManager{MockPackageManager: pkg.NewMockPackageManager()}
	got := ObserveInstallations(context.Background(), []Tool{observationBaseTool("tool", "receipt")}, mgr, pkg.PlatformMacOS)
	if got["tool"] || mgr.queryCalls != 0 {
		t.Fatalf("observations=%v queryCalls=%d", got, mgr.queryCalls)
	}
}

func TestObserveInstallationsFallbackDeadlineJoinsContextQueries(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulaErr = errors.New("batch unavailable")
	mgr.blockQueries = true
	all := []Tool{
		observationBaseTool("one", "a", "b", "c", "d", "e"),
	}
	runtime := defaultInstallationObservationRuntime()
	runtime.fallbackTimeout = 20 * time.Millisecond
	runtime.fallbackWorkers = 2
	started := time.Now()
	got := observeInstallationsWithRuntime(context.Background(), all, mgr, pkg.PlatformMacOS, runtime)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded fallback took %s", elapsed)
	}
	if got["one"] || mgr.activeQueries.Load() != 0 {
		t.Fatalf("observations=%v activeQueries=%d", got, mgr.activeQueries.Load())
	}
	if len(mgr.queryCalls) == 0 || len(mgr.queryCalls) > 2 {
		t.Fatalf("fallback queryCalls=%v, want at most worker count before deadline", mgr.queryCalls)
	}
}

func TestRegistryCacheUsesSharedBatchObserver(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "one"}}
	registry := &Registry{tools: make(map[string]Tool), installedCache: make(map[string]bool)}
	registry.Register(observationBaseTool("tool", "one"))
	registry.ensureCacheWith(context.Background(), mgr, pkg.PlatformMacOS)
	if !registry.isInstalledCached("tool") || mgr.formulaCalls != 1 || mgr.caskCalls != 1 || len(mgr.queryCalls) != 0 {
		t.Fatalf("cache=%v formula=%d cask=%d queries=%v", registry.installedCache, mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls)
	}
}

func TestRegistryConcurrentCachePopulationUsesOneBatch(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "one"}}
	registry := &Registry{tools: make(map[string]Tool), installedCache: make(map[string]bool)}
	registry.Register(observationBaseTool("tool", "one"))

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			registry.ensureCacheWith(context.Background(), mgr, pkg.PlatformMacOS)
		}()
	}
	close(start)
	wg.Wait()
	if !registry.isInstalledCached("tool") || mgr.formulaCalls != 1 || mgr.caskCalls != 1 || len(mgr.queryCalls) != 0 {
		t.Fatalf("cache=%v formula=%d cask=%d queries=%v", registry.installedCache, mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls)
	}
}

func TestRegistryInvalidationWaitsForInFlightPopulation(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "one"}}
	mgr.listStarted = make(chan struct{})
	mgr.listRelease = make(chan struct{})
	registry := &Registry{tools: make(map[string]Tool), installedCache: make(map[string]bool)}
	registry.Register(observationBaseTool("tool", "one"))

	populationDone := make(chan struct{})
	go func() {
		registry.ensureCacheWith(context.Background(), mgr, pkg.PlatformMacOS)
		close(populationDone)
	}()
	<-mgr.listStarted
	invalidationDone := make(chan struct{})
	go func() {
		registry.InvalidateCache()
		close(invalidationDone)
	}()
	close(mgr.listRelease)
	<-populationDone
	<-invalidationDone

	registry.cacheMu.RLock()
	populated := registry.cachePopulated
	cacheSize := len(registry.installedCache)
	registry.cacheMu.RUnlock()
	if populated || cacheSize != 0 || mgr.formulaCalls != 1 {
		t.Fatalf("populated=%v cacheSize=%d formulaCalls=%d", populated, cacheSize, mgr.formulaCalls)
	}
}

func TestRegistryCustomToolsImplementDirectObservation(t *testing.T) {
	registry := NewRegistry()
	wantDirect := []string{
		"appcleaner", "claude-code", "codex", "cursor", "ghostty", "iina",
		"lm-studio", "moonlight", "obs", "opencode", "pi", "raycast",
		"rectangle", "sunshine", "t3-code", "tailscale", "yazi", "zen-browser",
	}
	for _, id := range wantDirect {
		tool, ok := registry.Get(id)
		if !ok {
			t.Fatalf("custom tool %q is not registered", id)
		}
		if _, ok := tool.(DirectInstallationDetector); !ok {
			t.Errorf("custom tool %q does not implement DirectInstallationDetector", id)
		}
	}
	for _, tool := range registry.All() {
		if policy, ok := tool.(packageMetadataPolicy); ok && !policy.PackageMetadataIsAuthoritative() {
			_, legacy := tool.(DirectInstallationDetector)
			_, typed := tool.(InstallationDirectAlternativesProvider)
			if !legacy && !typed {
				t.Errorf("non-authoritative tool %q has no direct detector", tool.ID())
			}
		}
	}
}
