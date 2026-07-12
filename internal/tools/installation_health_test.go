package tools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func healthTool(t *testing.T, snapshot health.InstallationSnapshot, id string) health.InstallationObservation {
	t.Helper()
	observation, ok := snapshot.Tool(id)
	if !ok {
		t.Fatalf("snapshot has no tool %q", id)
	}
	return observation
}

func mustObserveInstallationHealth(t *testing.T, ctx context.Context, all []Tool, mgr pkg.PackageManager, platform pkg.Platform, generation uint64, runtime installationObservationRuntime) health.InstallationSnapshot {
	t.Helper()
	snapshot, err := observeInstallationHealthWithRuntime(ctx, all, mgr, platform, generation, runtime)
	if err != nil {
		t.Fatalf("collect installation health: %v", err)
	}
	return snapshot
}

func healthNamespace(t *testing.T, facet health.PackageFacet, namespace health.PackageNamespace) health.PackageNamespaceFacet {
	t.Helper()
	for _, candidate := range facet.Namespaces {
		if candidate.Namespace == namespace {
			return candidate
		}
	}
	t.Fatalf("package facet has no %q namespace: %+v", namespace, facet)
	return health.PackageNamespaceFacet{}
}

func assertZeroHealthSnapshot(t *testing.T, snapshot health.InstallationSnapshot) {
	t.Helper()
	_, lookup := snapshot.Tool("valid")
	if snapshot.SchemaVersion() != 0 || snapshot.Generation() != 0 || snapshot.Platform() != "" || snapshot.Manager() != "" || len(snapshot.Tools()) != 0 || lookup {
		t.Fatalf("non-zero failed snapshot: schema=%d generation=%d platform=%q manager=%q tools=%v lookup=%v", snapshot.SchemaVersion(), snapshot.Generation(), snapshot.Platform(), snapshot.Manager(), snapshot.Tools(), lookup)
	}
}

type typedDirectHealthTool struct {
	BaseTool
	alternatives []health.DirectAlternative
}

type countingTypedDirectTool struct {
	typedDirectHealthTool
	flatpakIDs []string
	calls      atomic.Int32
}

func (t *countingTypedDirectTool) InstallationDirectAlternatives(observation DirectInstallationObservation) []health.DirectAlternative {
	t.calls.Add(1)
	return t.typedDirectHealthTool.InstallationDirectAlternatives(observation)
}
func (t *countingTypedDirectTool) FlatpakApplicationIDs() []string {
	return append([]string(nil), t.flatpakIDs...)
}

type mutatingDirectProviderTool struct {
	BaseTool
	calls atomic.Int32
}

func (t *mutatingDirectProviderTool) PackageMetadataIsAuthoritative() bool { return false }
func (t *mutatingDirectProviderTool) InstallationDirectAlternatives(observation DirectInstallationObservation) []health.DirectAlternative {
	t.calls.Add(1)
	delete(observation.FlatpakApplications, "com.example.Shared")
	observation.FlatpakApplications["com.example.Injected"] = true
	return []health.DirectAlternative{{Kind: health.DirectSourceLegacy, Identifiers: []string{t.ID()}, State: health.ComponentUnknown}}
}

type recipeHealthTool struct {
	BaseTool
	recipe operation.InstallRecipe
	err    error
}

func (t *recipeHealthTool) InstallRecipe(InstallEnvironment) (operation.InstallRecipe, error) {
	return t.recipe, t.err
}

func (t *typedDirectHealthTool) InstallationDirectAlternatives(DirectInstallationObservation) []health.DirectAlternative {
	return append([]health.DirectAlternative(nil), t.alternatives...)
}

func (t *typedDirectHealthTool) PackageMetadataIsAuthoritative() bool { return false }

func TestObserveInstallationHealthBatchesFormulaCaskAndFlatpakOnce(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "formula"}}
	mgr.casks = []string{"cask"}
	flatpak := observationDirectTool("flatpak", false, false)
	flatpak.flatpakIDs = []string{"com.example.Flatpak"}
	runtime := defaultInstallationObservationRuntime()
	flatpakCalls := 0
	runtime.listFlatpakApplications = func(context.Context) ([]string, error) {
		flatpakCalls++
		return []string{"com.example.Flatpak"}, nil
	}

	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{
		observationBaseTool("formula", "formula"),
		observationBaseTool("cask", "cask"),
		flatpak,
	}, mgr, pkg.PlatformMacOS, 11, runtime)
	if snapshot.Generation() != 11 || snapshot.Platform() != string(pkg.PlatformMacOS) || snapshot.Manager() != mgr.Name() {
		t.Fatalf("snapshot metadata generation=%d platform=%q manager=%q", snapshot.Generation(), snapshot.Platform(), snapshot.Manager())
	}
	tools := snapshot.Tools()
	if len(tools) != 3 || tools[0].ToolID() != "cask" || tools[1].ToolID() != "flatpak" || tools[2].ToolID() != "formula" {
		t.Fatalf("snapshot order=%v", []string{tools[0].ToolID(), tools[1].ToolID(), tools[2].ToolID()})
	}

	if mgr.formulaCalls != 1 || mgr.caskCalls != 1 || flatpakCalls != 1 || len(mgr.queryCalls) != 0 {
		t.Fatalf("formula=%d cask=%d flatpak=%d fallback=%v", mgr.formulaCalls, mgr.caskCalls, flatpakCalls, mgr.queryCalls)
	}
	for _, id := range []string{"formula", "cask", "flatpak"} {
		if got := healthTool(t, snapshot, id).Presence(); got != health.PresencePresent {
			t.Errorf("%s presence=%q", id, got)
		}
	}
	flatpakDirect := healthTool(t, snapshot, "flatpak").Direct()
	if flatpakDirect.State != health.ComponentPresent || len(flatpakDirect.Alternatives) != 2 || flatpakDirect.Alternatives[0].Kind != health.DirectSourceFlatpak || flatpakDirect.Alternatives[0].Identifiers[0] != "com.example.Flatpak" || flatpakDirect.Alternatives[1].Kind != health.DirectSourceLegacy || flatpakDirect.Alternatives[1].State != health.ComponentUnknown {
		t.Fatalf("flatpak direct evidence=%+v", flatpakDirect)
	}
}

func TestObserveInstallationHealthDistinguishesCompletePartialAndMissing(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "one"}}
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{
		observationBaseTool("complete", "one"),
		observationBaseTool("partial", "one", "two"),
		observationBaseTool("missing", "three"),
	}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())

	for id, want := range map[string]health.Presence{"complete": health.PresencePresent, "partial": health.PresencePartial, "missing": health.PresenceMissing} {
		observation := healthTool(t, snapshot, id)
		if observation.Presence() != want {
			t.Errorf("%s presence=%q want %q", id, observation.Presence(), want)
		}
		if !observation.Package().Complete {
			t.Errorf("%s package observation is not authoritative complete: %+v", id, observation.Package())
		}
	}
}

func TestObserveInstallationHealthFailedNamespacesRetainOnlyPositiveKnowledge(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = []pkg.Package{{Name: "known"}}
	mgr.caskErr = errors.New("cask failed\nTOKEN=do-not-leak")
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{
		observationBaseTool("known", "known"),
		observationBaseTool("unresolved", "unresolved"),
	}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())

	if got := healthTool(t, snapshot, "known").Presence(); got != health.PresencePresent {
		t.Fatalf("independent formula positive became %q", got)
	}
	unresolved := healthTool(t, snapshot, "unresolved")
	if unresolved.Presence() != health.PresenceUnknown || unresolved.Package().State != health.PackageUnknown || unresolved.Package().Complete {
		t.Fatalf("failed namespace became negative: %+v", unresolved.Package())
	}
	if strings.Contains(unresolved.Package().DiagnosticSummary, "do-not-leak") || strings.ContainsAny(unresolved.Package().DiagnosticSummary, "\r\n") {
		t.Fatalf("unsafe diagnostic: %q", unresolved.Package().DiagnosticSummary)
	}
	if unresolved.Package().DiagnosticCode != health.DiagnosticPackageBatchFailed || !reflect.DeepEqual(unresolved.Package().UnresolvedReceipts, []string{"unresolved"}) || len(unresolved.Package().MissingReceipts) != 0 {
		t.Fatalf("failed cask receipt was not unresolved: %+v", unresolved.Package())
	}
	formulaMissing := healthNamespace(t, unresolved.Package(), health.PackageNamespaceFormula)
	caskUnknown := healthNamespace(t, unresolved.Package(), health.PackageNamespaceCask)
	if formulaMissing.State != health.PackageMissing || !formulaMissing.Complete || caskUnknown.State != health.PackageUnknown || caskUnknown.Complete || !reflect.DeepEqual(caskUnknown.UnresolvedReceipts, []string{"unresolved"}) {
		t.Fatalf("symmetric cask failure authority formula=%+v cask=%+v", formulaMissing, caskUnknown)
	}
}

func TestObserveInstallationHealthFormulaFailurePreservesCaskPositive(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulaErr = errors.New("formula batch failed")
	mgr.casks = []string{"gui"}
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{
		observationBaseTool("gui", "gui"),
		observationBaseTool("unresolved", "unresolved"),
	}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())

	gui := healthTool(t, snapshot, "gui")
	if gui.Presence() != health.PresencePresent {
		t.Fatalf("cask positive was lost: %+v", gui.Package())
	}
	cask := healthNamespace(t, gui.Package(), health.PackageNamespaceCask)
	formula := healthNamespace(t, gui.Package(), health.PackageNamespaceFormula)
	if cask.State != health.PackagePresent || !cask.Complete || formula.State != health.PackageUnknown || formula.Complete {
		t.Fatalf("namespace authority collapsed: formula=%+v cask=%+v", formula, cask)
	}
	if got := healthTool(t, snapshot, "unresolved").Presence(); got != health.PresenceUnknown {
		t.Fatalf("failed formula absence became %q", got)
	}
	unresolved := healthTool(t, snapshot, "unresolved").Package()
	if unresolved.DiagnosticCode != health.DiagnosticPackageBatchFailed || !reflect.DeepEqual(unresolved.UnresolvedReceipts, []string{"unresolved"}) || len(unresolved.MissingReceipts) != 0 {
		t.Fatalf("failed formula receipt was not unresolved: %+v", unresolved)
	}
	formulaUnknown := healthNamespace(t, unresolved, health.PackageNamespaceFormula)
	caskMissing := healthNamespace(t, unresolved, health.PackageNamespaceCask)
	if formulaUnknown.State != health.PackageUnknown || formulaUnknown.Complete || !reflect.DeepEqual(formulaUnknown.UnresolvedReceipts, []string{"unresolved"}) || caskMissing.State != health.PackageMissing || !caskMissing.Complete {
		t.Fatalf("symmetric formula failure authority formula=%+v cask=%+v", formulaUnknown, caskMissing)
	}
}

func TestObserveInstallationHealthFailedBatchFallbackOnlyPositiveIsConclusive(t *testing.T) {
	for _, tc := range []struct {
		name      string
		installed bool
		block     bool
		want      health.Presence
	}{
		{name: "legacy false unresolved", want: health.PresenceUnknown},
		{name: "known true", installed: true, want: health.PresencePresent},
		{name: "timeout", block: true, want: health.PresenceUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newCountingObservationManager()
			mgr.formulaErr = errors.New("batch unavailable")
			mgr.blockQueries = tc.block
			mgr.queryInstalled["receipt"] = tc.installed
			runtime := defaultInstallationObservationRuntime()
			runtime.fallbackTimeout = 20 * time.Millisecond
			runtime.fallbackWorkers = 1
			snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{observationBaseTool("tool", "receipt")}, mgr, pkg.PlatformMacOS, 1, runtime)
			observation := healthTool(t, snapshot, "tool")
			if got := observation.Presence(); got != tc.want {
				t.Fatalf("presence=%q want %q", got, tc.want)
			}
			wantCode := health.DiagnosticPackageBatchFailed
			if tc.block {
				wantCode = health.DiagnosticFallbackTimeout
			}
			if observation.Package().DiagnosticCode != wantCode {
				t.Fatalf("diagnostic code=%q want %q", observation.Package().DiagnosticCode, wantCode)
			}
			if mgr.activeQueries.Load() != 0 {
				t.Fatalf("fallback left %d active queries", mgr.activeQueries.Load())
			}
		})
	}
}

func TestObserveInstallationHealthInFlightTimeoutIsBoundedJoinedAndImmutable(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulaErr = errors.New("batch unavailable")
	mgr.blockQueries = true
	runtime := defaultInstallationObservationRuntime()
	runtime.fallbackTimeout = 20 * time.Millisecond
	runtime.fallbackWorkers = 2
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{
		observationBaseTool("tool", "a", "b", "c", "d", "e"),
	}, mgr, pkg.PlatformMacOS, 1, runtime)
	before := healthTool(t, snapshot, "tool").Package()
	if len(mgr.queryCalls) == 0 || len(mgr.queryCalls) > 2 || mgr.activeQueries.Load() != 0 {
		t.Fatalf("queries=%v active=%d", mgr.queryCalls, mgr.activeQueries.Load())
	}
	if before.DiagnosticCode != health.DiagnosticFallbackTimeout || len(before.MissingReceipts) != 0 || !reflect.DeepEqual(before.UnresolvedReceipts, []string{"a", "b", "c", "d", "e"}) {
		t.Fatalf("timeout authority=%+v", before)
	}
	time.Sleep(30 * time.Millisecond)
	after := healthTool(t, snapshot, "tool").Package()
	if !reflect.DeepEqual(before, after) || mgr.activeQueries.Load() != 0 {
		t.Fatalf("snapshot mutated after return: before=%+v after=%+v active=%d", before, after, mgr.activeQueries.Load())
	}
}

func TestObserveInstallationHealthPreservesTypedDirectAlternativesAndORSemantics(t *testing.T) {
	tool := &typedDirectHealthTool{
		BaseTool: *observationBaseTool("typed-direct"),
		alternatives: []health.DirectAlternative{
			{Kind: health.DirectSourceBinary, Identifiers: []string{"typed-direct"}, State: health.ComponentMissing},
			{Kind: health.DirectSourceAppBundle, Identifiers: []string{"com.example.TypedDirect"}, State: health.ComponentPresent},
		},
	}
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	got := healthTool(t, snapshot, "typed-direct")
	if got.Presence() != health.PresencePresent {
		t.Fatalf("OR presence=%q", got.Presence())
	}
	alternatives := got.Direct().Alternatives
	if len(alternatives) != 2 || alternatives[0].Kind != health.DirectSourceAppBundle || alternatives[0].Identifiers[0] != "com.example.TypedDirect" || alternatives[1].Kind != health.DirectSourceBinary || alternatives[1].Identifiers[0] != "typed-direct" {
		t.Fatalf("typed alternatives not deterministic/preserved: %+v", alternatives)
	}
}

func TestObserveInstallationHealthTypedBinaryAndAppPositivesPreserveIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kind       health.DirectSourceKind
		identifier string
	}{
		{name: "binary", kind: health.DirectSourceBinary, identifier: "agent"},
		{name: "app bundle", kind: health.DirectSourceAppBundle, identifier: "com.example.Agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := &typedDirectHealthTool{
				BaseTool: *observationBaseTool("agent"),
				alternatives: []health.DirectAlternative{{
					Kind: tc.kind, Identifiers: []string{tc.identifier}, State: health.ComponentPresent,
				}},
			}
			snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
			got := healthTool(t, snapshot, "agent")
			if got.Presence() != health.PresencePresent || got.Direct().Alternatives[0].Kind != tc.kind || got.Direct().Alternatives[0].Identifiers[0] != tc.identifier {
				t.Fatalf("typed positive was not preserved: %+v", got.Direct())
			}
		})
	}
}

func TestObserveInstallationHealthLegacyDirectFalseRemainsUnknown(t *testing.T) {
	mgr := newCountingObservationManager()
	legacy := observationDirectTool("legacy", true, false, "missing")
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{legacy}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	got := healthTool(t, snapshot, "legacy")
	if got.Direct().State != health.ComponentUnknown || got.Presence() != health.PresenceUnknown {
		t.Fatalf("legacy false was treated as negative: direct=%q presence=%q", got.Direct().State, got.Presence())
	}
}

func TestObserveInstallationHealthCancellationAndNonContextFallbackStayUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr := newCountingObservationManager()
	mgr.formulaErr = errors.New("batch unavailable")
	cancelled := mustObserveInstallationHealth(t, ctx, []Tool{observationBaseTool("cancelled", "receipt")}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	cancelledObservation := healthTool(t, cancelled, "cancelled")
	if got := cancelledObservation.Presence(); got != health.PresenceUnknown {
		t.Fatalf("cancelled presence=%q", got)
	}
	if cancelledObservation.Package().DiagnosticCode != health.DiagnosticCancelled || mgr.formulaCalls != 0 || mgr.caskCalls != 0 || len(mgr.queryCalls) != 0 {
		t.Fatalf("pre-cancel performed work or lost code: package=%+v formula=%d cask=%d queries=%v", cancelledObservation.Package(), mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls)
	}

	legacy := &nonContextObservationManager{MockPackageManager: pkg.NewMockPackageManager()}
	unresolved := mustObserveInstallationHealth(t, context.Background(), []Tool{observationBaseTool("legacy", "receipt")}, legacy, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	if got := healthTool(t, unresolved, "legacy").Presence(); got != health.PresenceUnknown || legacy.queryCalls != 0 {
		t.Fatalf("non-context presence=%q queryCalls=%d", got, legacy.queryCalls)
	}
}

func TestObserveInstallationHealthFlatpakFailureSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want health.ComponentState
	}{
		{name: "not installed", err: exec.ErrNotFound, want: health.ComponentNotApplicable},
		{name: "successful empty", want: health.ComponentMissing},
		{name: "failure", err: errors.New("flatpak raw failure must not leak"), want: health.ComponentUnknown},
		{name: "timeout", err: context.DeadlineExceeded, want: health.ComponentUnknown},
		{name: "cancelled", err: context.Canceled, want: health.ComponentUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := observationDirectTool("flatpak", false, false)
			tool.flatpakIDs = []string{"com.example.App"}
			runtime := defaultInstallationObservationRuntime()
			runtime.listFlatpakApplications = func(context.Context) ([]string, error) { return nil, tc.err }
			snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, nil, pkg.PlatformArch, 1, runtime)
			observation := healthTool(t, snapshot, "flatpak")
			alternative := observation.Direct().Alternatives[0]
			if got := alternative.State; got != tc.want {
				t.Fatalf("flatpak state=%q want %q", got, tc.want)
			}
			wantCode := health.DiagnosticCode("")
			if tc.err != nil && !errors.Is(tc.err, exec.ErrNotFound) {
				wantCode = health.DiagnosticProbeFailed
				if errors.Is(tc.err, context.Canceled) {
					wantCode = health.DiagnosticCancelled
				} else if errors.Is(tc.err, context.DeadlineExceeded) {
					wantCode = health.DiagnosticProbeTimeout
				}
			}
			direct := observation.Direct()
			if direct.DiagnosticCode != wantCode || alternative.DiagnosticCode != wantCode || strings.Contains(direct.DiagnosticSummary, "raw failure") || strings.Contains(alternative.DiagnosticSummary, "raw failure") {
				t.Fatalf("flatpak diagnostic=%+v want code %q", alternative, wantCode)
			}
			if errors.Is(tc.err, exec.ErrNotFound) && observation.Presence() != health.PresenceUnknown {
				t.Fatalf("Flatpak N/A-only presence=%q", observation.Presence())
			}
		})
	}
}

func TestObserveInstallationHealthPositiveEvidenceWinsWithoutChangingInstallability(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulae = nil
	external := observationDirectTool("external", true, true, "missing")
	unsupported := observationDirectTool("unsupported", true, true)
	unsupported.packages = map[pkg.Platform][]string{pkg.PlatformMacOS: {"unsupported"}}
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{external, unsupported}, mgr, pkg.PlatformArch, 4, defaultInstallationObservationRuntime())

	if got := healthTool(t, snapshot, "external"); got.Presence() != health.PresencePresent || got.Direct().State != health.ComponentPresent {
		t.Fatalf("legacy true presence=%q direct=%q", got.Presence(), got.Direct().State)
	}
	got := healthTool(t, snapshot, "unsupported")
	if got.Presence() != health.PresencePresent || got.Installability() != health.InstallabilityUnsupported {
		t.Fatalf("unsupported external positive presence=%q installability=%q", got.Presence(), got.Installability())
	}
}

func TestObserveInstallationHealthNoManagerDoesNotClaimSupportOrAbsence(t *testing.T) {
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{observationBaseTool("tool", "receipt")}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	got := healthTool(t, snapshot, "tool")
	if got.Presence() != health.PresenceUnknown || got.Installability() != health.InstallabilityUnknown {
		t.Fatalf("presence=%q installability=%q", got.Presence(), got.Installability())
	}
}

func TestObserveInstallationHealthRejectsInvalidTypedProviderMetadata(t *testing.T) {
	tool := &typedDirectHealthTool{
		BaseTool: *observationBaseTool("invalid-provider"),
		alternatives: []health.DirectAlternative{{
			Kind: health.DirectSourceKind("shell"), Identifiers: []string{"bad\nidentifier"}, State: health.ComponentPresent,
		}},
	}
	snapshot, err := observeInstallationHealthWithRuntime(context.Background(), []Tool{tool}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	if err == nil {
		t.Fatal("invalid typed provider metadata was accepted")
	}
	if len(snapshot.Tools()) != 0 || snapshot.SchemaVersion() != 0 {
		t.Fatalf("collector published a partial snapshot after contract error: schema=%d tools=%v", snapshot.SchemaVersion(), snapshot.Tools())
	}
}

func TestObserveInstallationHealthAggregatesMultipleDirectDiagnosticsDeterministically(t *testing.T) {
	tool := &typedDirectHealthTool{
		BaseTool: *observationBaseTool("multi-diagnostic"),
		alternatives: []health.DirectAlternative{
			{Kind: health.DirectSourceBinary, Identifiers: []string{"agent"}, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticProbeFailed, DiagnosticSummary: "binary probe failed"},
			{Kind: health.DirectSourceAppBundle, Identifiers: []string{"com.example.Agent"}, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticProbeTimeout, DiagnosticSummary: "app probe timed out"},
		},
	}
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	direct := healthTool(t, snapshot, "multi-diagnostic").Direct()
	if direct.State != health.ComponentUnknown || direct.DiagnosticCode != health.DiagnosticProbeFailed || direct.DiagnosticSummary != "multiple direct probes failed" {
		t.Fatalf("aggregate diagnostic=%+v", direct)
	}
	if len(direct.Alternatives) != 2 || direct.Alternatives[0].Kind != health.DirectSourceAppBundle || direct.Alternatives[0].DiagnosticCode != health.DiagnosticProbeTimeout || direct.Alternatives[1].Kind != health.DirectSourceBinary || direct.Alternatives[1].DiagnosticCode != health.DiagnosticProbeFailed {
		t.Fatalf("alternative diagnostics lost association/order: %+v", direct.Alternatives)
	}
}

func TestObserveInstallationHealthBlocker1DoesNotMutateToolPackagesAndRepeatsDeterministically(t *testing.T) {
	packages := []string{"zeta", "alpha", "middle"}
	tool := observationBaseTool("unsorted", packages...)
	tool.packages[pkg.PlatformArch] = []string{"arch-z", "arch-a"}
	tool.packages[pkg.Platform("all")] = []string{"all-z", "all-a"}
	original := make(map[pkg.Platform][]string, len(tool.packages))
	for platform, values := range tool.packages {
		original[platform] = append([]string(nil), values...)
	}
	mgr := newCountingObservationManager()
	first := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	second := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	if !reflect.DeepEqual(tool.packages, original) {
		t.Fatalf("collector mutated registry package map: got=%v want=%v", tool.packages, original)
	}
	want := []string{"alpha", "middle", "zeta"}
	if !reflect.DeepEqual(healthTool(t, first, "unsorted").Package().ExpectedReceipts, want) || !reflect.DeepEqual(healthTool(t, first, "unsorted").Package(), healthTool(t, second, "unsorted").Package()) {
		t.Fatalf("snapshots are not deterministic: first=%+v second=%+v", healthTool(t, first, "unsorted").Package(), healthTool(t, second, "unsorted").Package())
	}
}

func TestObserveInstallationHealthBlocker2LegacyFalseRemainderSurvivesEmptyFlatpak(t *testing.T) {
	tool := observationDirectTool("mixed-legacy", false, false)
	tool.flatpakIDs = []string{"com.example.Empty"}
	runtime := defaultInstallationObservationRuntime()
	runtime.listFlatpakApplications = func(context.Context) ([]string, error) { return nil, nil }
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, nil, pkg.PlatformArch, 1, runtime)
	direct := healthTool(t, snapshot, "mixed-legacy").Direct()
	if direct.State != health.ComponentUnknown || healthTool(t, snapshot, "mixed-legacy").Presence() != health.PresenceUnknown || len(direct.Alternatives) != 2 {
		t.Fatalf("legacy unknown remainder was lost: %+v", direct)
	}
	if direct.Alternatives[0].Kind != health.DirectSourceFlatpak || direct.Alternatives[0].State != health.ComponentMissing || direct.Alternatives[1].Kind != health.DirectSourceLegacy || direct.Alternatives[1].State != health.ComponentUnknown || !reflect.DeepEqual(direct.Alternatives[1].Identifiers, []string{"mixed-legacy"}) {
		t.Fatalf("unexpected alternatives: %+v", direct.Alternatives)
	}
}

func TestObserveInstallationHealthBlocker3LegacyPositiveUsesLegacySourceNeverBinary(t *testing.T) {
	tool := observationDirectTool("app-only-legacy", false, true)
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{tool}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	direct := healthTool(t, snapshot, "app-only-legacy").Direct()
	if direct.State != health.ComponentPresent || len(direct.Alternatives) != 1 || direct.Alternatives[0].Kind != health.DirectSourceLegacy || !reflect.DeepEqual(direct.Alternatives[0].Identifiers, []string{"app-only-legacy"}) {
		t.Fatalf("legacy positive fabricated typed evidence: %+v", direct)
	}
}

func TestObserveInstallationHealthBlocker4ParentCancelStopsLaterStagesAndJoinsFallback(t *testing.T) {
	mgr := newCountingObservationManager()
	mgr.formulaErr = errors.New("batch unavailable")
	mgr.blockQueries = true
	direct := &countingTypedDirectTool{typedDirectHealthTool: typedDirectHealthTool{
		BaseTool:     *observationBaseTool("cancelled", "a", "b", "c"),
		alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"cancelled"}, State: health.ComponentPresent}},
	}, flatpakIDs: []string{"com.example.Cancelled"}}
	flatpakCalls := atomic.Int32{}
	runtime := defaultInstallationObservationRuntime()
	runtime.fallbackTimeout = time.Second
	runtime.fallbackWorkers = 2
	runtime.listFlatpakApplications = func(context.Context) ([]string, error) { flatpakCalls.Add(1); return nil, nil }
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		snapshot health.InstallationSnapshot
		err      error
	}
	done := make(chan result, 1)
	go func() {
		snapshot, err := observeInstallationHealthWithRuntime(ctx, []Tool{direct}, mgr, pkg.PlatformMacOS, 1, runtime)
		done <- result{snapshot, err}
	}()
	deadline := time.Now().Add(time.Second)
	for mgr.activeQueries.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if mgr.activeQueries.Load() == 0 {
		t.Fatal("fallback queries never became active before cancellation")
	}
	cancel()
	var got result
	select {
	case got = <-done:
	case <-time.After(time.Second):
		t.Fatal("collector did not join cancellation-bounded workers")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	observation := healthTool(t, got.snapshot, "cancelled")
	packageFacet, directFacet := observation.Package(), observation.Direct()
	if mgr.activeQueries.Load() != 0 || mgr.caskCalls != 0 || flatpakCalls.Load() != 0 || direct.calls.Load() != 0 || packageFacet.State != health.PackageUnknown || directFacet.State != health.ComponentUnknown || observation.Presence() != health.PresenceUnknown || !reflect.DeepEqual(packageFacet.UnresolvedReceipts, []string{"a", "b", "c"}) || len(packageFacet.MissingReceipts) != 0 || packageFacet.DiagnosticCode != health.DiagnosticCancelled || directFacet.DiagnosticCode != health.DiagnosticCancelled {
		t.Fatalf("cancel leaked later work: active=%d cask=%d flatpak=%d direct=%d package=%+v directFacet=%+v", mgr.activeQueries.Load(), mgr.caskCalls, flatpakCalls.Load(), direct.calls.Load(), observation.Package(), observation.Direct())
	}
}

func TestObserveInstallationHealthBlocker4DirectOnlyPreCancelPublishesReasonWithoutProbe(t *testing.T) {
	tool := &countingTypedDirectTool{typedDirectHealthTool: typedDirectHealthTool{BaseTool: *observationBaseTool("direct-only"), alternatives: []health.DirectAlternative{{Kind: health.DirectSourceAppBundle, Identifiers: []string{"com.example.Direct"}, State: health.ComponentPresent}}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot := mustObserveInstallationHealth(t, ctx, []Tool{tool}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	direct := healthTool(t, snapshot, "direct-only").Direct()
	if tool.calls.Load() != 0 || direct.State != health.ComponentUnknown || direct.DiagnosticCode != health.DiagnosticCancelled || healthTool(t, snapshot, "direct-only").Presence() != health.PresenceUnknown {
		t.Fatalf("pre-cancel direct evidence=%+v calls=%d", direct, tool.calls.Load())
	}
}

func TestObserveInstallationHealthBlocker5NonBrewUsesSystemNamespaceOnly(t *testing.T) {
	for _, manager := range []string{"apt", "pacman"} {
		t.Run(manager, func(t *testing.T) {
			mgr := newCountingObservationManager()
			mgr.ManagerName = manager
			mgr.formulae = []pkg.Package{{Name: "tool"}}
			mgr.casks = []string{"tool"}
			snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{observationBaseTool("tool", "tool")}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
			namespaces := healthTool(t, snapshot, "tool").Package().Namespaces
			facet := healthTool(t, snapshot, "tool").Package()
			if mgr.caskCalls != 0 || facet.Provider != manager || len(namespaces) != 1 || namespaces[0].Namespace != health.PackageNamespaceSystem {
				t.Fatalf("manager=%s provider=%q caskCalls=%d namespaces=%+v", manager, facet.Provider, mgr.caskCalls, namespaces)
			}
		})
	}
}

func TestObserveInstallationHealthBlocker6InstallabilityComesFromValidatedRecipe(t *testing.T) {
	mgr := newCountingObservationManager()
	valid := observationBaseTool("valid", "valid")
	rejected := &recipeHealthTool{BaseTool: *observationBaseTool("rejected", "rejected"), err: errors.New("route rejected")}
	snapshot := mustObserveInstallationHealth(t, context.Background(), []Tool{valid, rejected}, mgr, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	if healthTool(t, snapshot, "valid").Installability() != health.InstallabilitySupported || healthTool(t, snapshot, "rejected").Installability() != health.InstallabilityUnsupported {
		t.Fatalf("installability valid=%q rejected=%q", healthTool(t, snapshot, "valid").Installability(), healthTool(t, snapshot, "rejected").Installability())
	}
	invalid := &recipeHealthTool{BaseTool: *observationBaseTool("invalid", "invalid"), recipe: operation.InstallRecipe{ToolID: "invalid"}}
	published, err := observeInstallationHealthWithRuntime(context.Background(), []Tool{valid, invalid}, mgr, pkg.PlatformMacOS, 9, defaultInstallationObservationRuntime())
	_, lookup := published.Tool("valid")
	if err == nil || published.SchemaVersion() != 0 || published.Generation() != 0 || published.Platform() != "" || published.Manager() != "" || len(published.Tools()) != 0 || lookup {
		t.Fatalf("invalid recipe published snapshot=%+v err=%v", published, err)
	}
}

func TestObserveInstallationHealthBlocker7ProviderMutationIsIsolatedPerToolAndRun(t *testing.T) {
	mutator := &mutatingDirectProviderTool{BaseTool: *observationBaseTool("mutator")}
	follower := observationDirectTool("follower", false, false)
	follower.flatpakIDs = []string{"com.example.Shared"}
	runtime := defaultInstallationObservationRuntime()
	runtime.listFlatpakApplications = func(context.Context) ([]string, error) { return []string{"com.example.Shared"}, nil }
	first := mustObserveInstallationHealth(t, context.Background(), []Tool{mutator, follower}, nil, pkg.PlatformArch, 1, runtime)
	second := mustObserveInstallationHealth(t, context.Background(), []Tool{mutator, follower}, nil, pkg.PlatformArch, 2, runtime)
	for _, snapshot := range []health.InstallationSnapshot{first, second} {
		direct := healthTool(t, snapshot, "follower").Direct()
		if direct.State != health.ComponentPresent || direct.Alternatives[0].Kind != health.DirectSourceFlatpak || direct.Alternatives[0].State != health.ComponentPresent {
			t.Fatalf("mutating provider corrupted follower: %+v", direct)
		}
	}
	if mutator.calls.Load() != 2 {
		t.Fatalf("mutator calls=%d", mutator.calls.Load())
	}
}

func validHealthRecipe(toolID string) operation.InstallRecipe {
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        toolID, Platform: string(pkg.PlatformMacOS), Manager: "brew",
		Steps:    []operation.InstallStep{{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"candidate"}}},
		Detector: operation.InstallDetector{Kind: operation.InstallDetectorPackageReceipt, Values: []string{"candidate"}},
		Risk:     "test package install",
	}
}

func TestObserveInstallationHealthFinalARejectsRecipeIdentityMismatchAtomically(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*operation.InstallRecipe)
	}{
		{name: "tool ID", mutate: func(recipe *operation.InstallRecipe) { recipe.ToolID = "different-tool" }},
		{name: "platform", mutate: func(recipe *operation.InstallRecipe) { recipe.Platform = string(pkg.PlatformArch) }},
		{name: "manager", mutate: func(recipe *operation.InstallRecipe) { recipe.Manager = "apt"; recipe.Steps[0].Provider = "apt" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valid := observationBaseTool("valid", "valid")
			recipe := validHealthRecipe("mismatch")
			tc.mutate(&recipe)
			if _, err := operation.InstallRecipeDigest(recipe); err != nil {
				t.Fatalf("fixture recipe is structurally invalid: %v", err)
			}
			mismatch := &recipeHealthTool{BaseTool: *observationBaseTool("mismatch", "candidate"), recipe: recipe}
			mgr := newCountingObservationManager()
			snapshot, err := observeInstallationHealthWithRuntime(context.Background(), []Tool{valid, mismatch}, mgr, pkg.PlatformMacOS, 9, defaultInstallationObservationRuntime())
			if err == nil {
				t.Fatal("recipe identity mismatch was accepted")
			}
			assertZeroHealthSnapshot(t, snapshot)
		})
	}
}

func TestObserveInstallationHealthFinalBTypedOnlyPreCancelHasNoFabricatedLegacySource(t *testing.T) {
	tool := &countingTypedDirectTool{typedDirectHealthTool: typedDirectHealthTool{BaseTool: *observationBaseTool("typed-only"), alternatives: []health.DirectAlternative{{Kind: health.DirectSourceAppBundle, Identifiers: []string{"com.example.Typed"}, State: health.ComponentPresent}}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot := mustObserveInstallationHealth(t, ctx, []Tool{tool}, nil, pkg.PlatformMacOS, 1, defaultInstallationObservationRuntime())
	observation := healthTool(t, snapshot, "typed-only")
	direct := observation.Direct()
	if tool.calls.Load() != 0 || observation.Presence() != health.PresenceUnknown || direct.State != health.ComponentUnknown || direct.DiagnosticCode != health.DiagnosticCancelled || len(direct.Alternatives) != 0 {
		t.Fatalf("typed-only cancellation fabricated provenance: calls=%d presence=%q direct=%+v", tool.calls.Load(), observation.Presence(), direct)
	}
}

func TestObserveInstallationHealthFinalCRejectsNilAndDuplicateBeforeAnyProbe(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tools func() ([]Tool, []*countingTypedDirectTool)
	}{
		{name: "nil tool", tools: func() ([]Tool, []*countingTypedDirectTool) { return []Tool{nil}, nil }},
		{name: "duplicate IDs", tools: func() ([]Tool, []*countingTypedDirectTool) {
			first := &countingTypedDirectTool{typedDirectHealthTool: typedDirectHealthTool{BaseTool: *observationBaseTool("duplicate", "one"), alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"one"}, State: health.ComponentPresent}}}}
			second := &countingTypedDirectTool{typedDirectHealthTool: typedDirectHealthTool{BaseTool: *observationBaseTool("duplicate", "two"), alternatives: []health.DirectAlternative{{Kind: health.DirectSourceBinary, Identifiers: []string{"two"}, State: health.ComponentPresent}}}}
			return []Tool{first, second}, []*countingTypedDirectTool{first, second}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			all, providers := tc.tools()
			mgr := newCountingObservationManager()
			flatpakCalls := atomic.Int32{}
			runtime := defaultInstallationObservationRuntime()
			runtime.listFlatpakApplications = func(context.Context) ([]string, error) { flatpakCalls.Add(1); return nil, nil }
			snapshot, err := observeInstallationHealthWithRuntime(context.Background(), all, mgr, pkg.PlatformMacOS, 1, runtime)
			if err == nil {
				t.Fatal("invalid tool set was accepted")
			}
			assertZeroHealthSnapshot(t, snapshot)
			if mgr.formulaCalls != 0 || mgr.caskCalls != 0 || len(mgr.queryCalls) != 0 || flatpakCalls.Load() != 0 {
				t.Fatalf("invalid preflight performed probes: formula=%d cask=%d query=%v flatpak=%d", mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls, flatpakCalls.Load())
			}
			for _, provider := range providers {
				if provider.calls.Load() != 0 {
					t.Fatalf("direct provider called %d times", provider.calls.Load())
				}
			}
		})
	}
}

func TestObserveInstallationHealthFinalCRejectsInvalidIDsAndToolLimitBeforeAnyProbe(t *testing.T) {
	invalidSets := map[string][]Tool{
		"empty ID":   {observationBaseTool("", "one")},
		"control ID": {observationBaseTool("bad\nID", "one")},
	}
	tooMany := make([]Tool, health.MaxInstallationTools+1)
	for index := range tooMany {
		tooMany[index] = observationBaseTool(fmt.Sprintf("tool-%04d", index), "one")
	}
	invalidSets["over tool limit"] = tooMany
	for name, all := range invalidSets {
		t.Run(name, func(t *testing.T) {
			mgr := newCountingObservationManager()
			flatpakCalls := atomic.Int32{}
			runtime := defaultInstallationObservationRuntime()
			runtime.listFlatpakApplications = func(context.Context) ([]string, error) { flatpakCalls.Add(1); return nil, nil }
			snapshot, err := observeInstallationHealthWithRuntime(context.Background(), all, mgr, pkg.PlatformMacOS, 1, runtime)
			if err == nil {
				t.Fatal("invalid preflight input was accepted")
			}
			assertZeroHealthSnapshot(t, snapshot)
			if mgr.formulaCalls != 0 || mgr.caskCalls != 0 || len(mgr.queryCalls) != 0 || flatpakCalls.Load() != 0 {
				t.Fatalf("invalid preflight performed probes: formula=%d cask=%d query=%v flatpak=%d", mgr.formulaCalls, mgr.caskCalls, mgr.queryCalls, flatpakCalls.Load())
			}
		})
	}
}

func TestObserveInstallationHealthHostileRecipeMetadataReturnsGenericBoundedError(t *testing.T) {
	marker := "HOSTILE_SECRET_PATH_Users_private_credentials_"
	longMarker := marker + strings.Repeat("X", 600)
	for _, tc := range []struct {
		name              string
		structurallyValid bool
		recipe            func() operation.InstallRecipe
	}{
		{name: "structurally invalid token", recipe: func() operation.InstallRecipe {
			return operation.InstallRecipe{
				SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
				ToolID:        "hostile", Platform: string(pkg.PlatformMacOS), Manager: "brew",
				Steps:    []operation.InstallStep{{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{marker + "/Users/private/credentials"}}},
				Detector: operation.InstallDetector{Kind: operation.InstallDetectorAppBundle, Values: []string{"com.example.Hostile"}},
				Risk:     "test Homebrew cask install",
			}
		}},
		{name: "valid but identity mismatched", structurallyValid: true, recipe: func() operation.InstallRecipe {
			recipe := validHealthRecipe("hostile")
			recipe.ToolID = longMarker
			return recipe
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			valid := observationBaseTool("valid", "valid")
			recipe := tc.recipe()
			if tc.structurallyValid {
				if _, err := operation.InstallRecipeDigest(recipe); err != nil {
					t.Fatalf("fixture must be structurally valid: %v", err)
				}
			}
			hostile := &recipeHealthTool{BaseTool: *observationBaseTool("hostile", "candidate"), recipe: recipe}
			snapshot, err := observeInstallationHealthWithRuntime(context.Background(), []Tool{valid, hostile}, newCountingObservationManager(), pkg.PlatformMacOS, 7, defaultInstallationObservationRuntime())
			if err == nil {
				t.Fatal("hostile recipe metadata was accepted")
			}
			assertZeroHealthSnapshot(t, snapshot)
			message := err.Error()
			if len(message) > 256 {
				t.Fatalf("error is unbounded (%d bytes): %q", len(message), message)
			}
			if !strings.Contains(message, "hostile") || !strings.Contains(message, "invalid install recipe metadata") {
				t.Fatalf("error lacks stable safe context: %q", message)
			}
			for _, forbidden := range []string{marker, longMarker, "/Users/private", "credentials", `"`, "invalid Homebrew cask token", "package-manager step", "accepted manager", "recipe identity mismatch"} {
				if strings.Contains(message, forbidden) {
					t.Fatalf("error reflected hostile/raw validator content %q: %q", forbidden, message)
				}
			}
		})
	}
}
