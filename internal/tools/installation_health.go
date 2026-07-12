package tools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"sync"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// InstallationDirectAlternativesProvider publishes typed, non-secret direct
// installation evidence. Legacy DirectInstallationDetector implementations are
// adapted conservatively: true is evidence, false remains unknown.
type InstallationDirectAlternativesProvider interface {
	InstallationDirectAlternatives(DirectInstallationObservation) []health.DirectAlternative
}

type receiptKnowledge uint8

var errInvalidInstallRecipeMetadata = errors.New("invalid install recipe metadata")

const (
	receiptMissing receiptKnowledge = iota + 1
	receiptPresent
	receiptUnknown
)

type namespaceObservation struct {
	available bool
	complete  bool
	receipts  map[string]receiptKnowledge
	code      health.DiagnosticCode
	summary   string
}

// ObserveInstallationHealth returns one immutable, generation-tagged
// installation snapshot. Probe failures are represented as typed unknown
// evidence; only invalid provider metadata or snapshot invariants return an
// error and prevent publication.
func ObserveInstallationHealth(ctx context.Context, all []Tool, mgr pkg.PackageManager, platform pkg.Platform, generation uint64) (health.InstallationSnapshot, error) {
	return observeInstallationHealthWithRuntime(ctx, all, mgr, platform, generation, defaultInstallationObservationRuntime())
}

func observeInstallationHealthWithRuntime(ctx context.Context, all []Tool, mgr pkg.PackageManager, platform pkg.Platform, generation uint64, runtime installationObservationRuntime) (health.InstallationSnapshot, error) {
	ids := make([]string, len(all))
	for index, tool := range all {
		if tool == nil {
			return health.InstallationSnapshot{}, fmt.Errorf("tool %d is nil", index)
		}
		ids[index] = tool.ID()
	}
	if err := health.ValidateInstallationToolIDs(ids); err != nil {
		return health.InstallationSnapshot{}, fmt.Errorf("validate installation health input: %w", err)
	}
	if runtime.fallbackWorkers <= 0 {
		runtime.fallbackWorkers = 1
	}
	managerName := ""
	if mgr != nil {
		managerName = mgr.Name()
	}
	expected := make(map[string][]string, len(all))
	uniqueReceipts := make(map[string]struct{})
	flatpakNeeded := false
	for _, tool := range all {
		receipts := slices.Clone(PackagesForPlatform(tool.Packages(), platform))
		sort.Strings(receipts)
		expected[tool.ID()] = receipts
		for _, receipt := range receipts {
			uniqueReceipts[receipt] = struct{}{}
		}
		if provider, ok := tool.(flatpakApplicationProvider); ok && len(provider.FlatpakApplicationIDs()) > 0 {
			flatpakNeeded = true
		}
	}

	formula := namespaceObservation{available: mgr != nil, receipts: make(map[string]receiptKnowledge)}
	cask := namespaceObservation{receipts: make(map[string]receiptKnowledge)}
	flatpakValues := map[string]bool{}
	flatpakState := health.ComponentNotApplicable
	flatpakCode := health.DiagnosticCode("")
	flatpakSummary := ""

	if ctx.Err() != nil {
		formula.code, formula.summary = health.DiagnosticCancelled, "installation observation cancelled"
		for receipt := range uniqueReceipts {
			formula.receipts[receipt] = receiptUnknown
		}
	} else {
		formula = observeFormulaNamespace(ctx, mgr, uniqueReceipts, runtime)
		if ctx.Err() == nil && managerName == "brew" {
			cask = observeCaskNamespace(mgr, uniqueReceipts)
		} else if ctx.Err() != nil && managerName == "brew" {
			cask = cancelledNamespace(uniqueReceipts)
		}
		if ctx.Err() == nil && flatpakNeeded {
			flatpakValues, flatpakState, flatpakCode, flatpakSummary = observeFlatpakNamespace(ctx, runtime)
		}
	}

	directObservation := DirectInstallationObservation{FlatpakApplications: flatpakValues}
	observations := make([]health.InstallationObservation, 0, len(all))
	for _, tool := range all {
		primaryNamespace := health.PackageNamespaceSystem
		if managerName == "brew" {
			primaryNamespace = health.PackageNamespaceFormula
		}
		packageFacet := buildPackageHealth(tool, expected[tool.ID()], managerName, primaryNamespace, formula, cask)
		directFacet := cancelledDirectHealth(tool)
		if ctx.Err() == nil {
			directFacet = buildDirectHealth(tool, directObservation, flatpakState, flatpakCode, flatpakSummary)
		}
		installability, err := observeInstallability(tool, platform, managerName, mgr != nil)
		if err != nil {
			return health.InstallationSnapshot{}, fmt.Errorf("describe %s installability: %w", tool.ID(), err)
		}
		observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{
			ToolID: tool.ID(), Installability: installability, Package: packageFacet, Direct: directFacet,
		})
		if err != nil {
			return health.InstallationSnapshot{}, fmt.Errorf("collect %s installation health: %w", tool.ID(), err)
		}
		observations = append(observations, observation)
	}
	return health.NewInstallationSnapshot(health.InstallationSnapshotSpec{
		Generation: generation, Platform: string(platform), Manager: managerName, Tools: observations,
	})
}

func observeFormulaNamespace(ctx context.Context, mgr pkg.PackageManager, expected map[string]struct{}, runtime installationObservationRuntime) namespaceObservation {
	result := namespaceObservation{available: mgr != nil, receipts: make(map[string]receiptKnowledge)}
	if mgr == nil {
		for receipt := range expected {
			result.receipts[receipt] = receiptUnknown
		}
		return result
	}
	installed, err := mgr.ListInstalled()
	if err == nil {
		present := make(map[string]bool, len(installed))
		for _, candidate := range installed {
			present[candidate.Name] = true
		}
		result.complete = true
		for receipt := range expected {
			if present[receipt] {
				result.receipts[receipt] = receiptPresent
			} else {
				result.receipts[receipt] = receiptMissing
			}
		}
		return result
	}
	result.code, result.summary = health.DiagnosticPackageBatchFailed, "package receipt batch failed"
	for receipt := range expected {
		result.receipts[receipt] = receiptUnknown
	}
	query, ok := mgr.(contextPackageQuery)
	if !ok || len(expected) == 0 {
		return result
	}

	names := make([]string, 0, len(expected))
	for receipt := range expected {
		names = append(names, receipt)
	}
	sort.Strings(names)
	fallbackCtx, cancel := context.WithTimeout(ctx, runtime.fallbackTimeout)
	defer cancel()
	workers := min(runtime.fallbackWorkers, len(names))
	jobs := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for receipt := range jobs {
				present := query.IsInstalledContext(fallbackCtx, receipt)
				if present {
					mu.Lock()
					result.receipts[receipt] = receiptPresent
					mu.Unlock()
				}
			}
		}()
	}
sendLoop:
	for _, receipt := range names {
		select {
		case jobs <- receipt:
		case <-fallbackCtx.Done():
			break sendLoop
		}
	}
	close(jobs)
	wg.Wait()
	if fallbackCtx.Err() != nil {
		if ctx.Err() != nil {
			result.code, result.summary = health.DiagnosticCancelled, "installation observation cancelled"
		} else {
			result.code, result.summary = health.DiagnosticFallbackTimeout, "package fallback timed out"
		}
	}
	return result
}

func observeCaskNamespace(mgr pkg.PackageManager, expected map[string]struct{}) namespaceObservation {
	result := namespaceObservation{receipts: make(map[string]receiptKnowledge)}
	lister, ok := mgr.(installedCaskLister)
	if !ok {
		return result
	}
	result.available = true
	installed, err := lister.ListInstalledCasks()
	if err != nil {
		result.code, result.summary = health.DiagnosticPackageBatchFailed, "cask receipt batch failed"
		for receipt := range expected {
			result.receipts[receipt] = receiptUnknown
		}
		return result
	}
	present := make(map[string]bool, len(installed))
	for _, value := range installed {
		present[value] = true
	}
	result.complete = true
	for receipt := range expected {
		if present[receipt] {
			result.receipts[receipt] = receiptPresent
		} else {
			result.receipts[receipt] = receiptMissing
		}
	}
	return result
}

func cancelledNamespace(expected map[string]struct{}) namespaceObservation {
	result := namespaceObservation{available: true, receipts: make(map[string]receiptKnowledge), code: health.DiagnosticCancelled, summary: "installation observation cancelled"}
	for receipt := range expected {
		result.receipts[receipt] = receiptUnknown
	}
	return result
}

func observeFlatpakNamespace(ctx context.Context, runtime installationObservationRuntime) (map[string]bool, health.ComponentState, health.DiagnosticCode, string) {
	if runtime.listFlatpakApplications == nil {
		return nil, health.ComponentUnknown, health.DiagnosticProbeFailed, "Flatpak probe failed"
	}
	values, err := runtime.listFlatpakApplications(ctx)
	if err != nil {
		switch {
		case errors.Is(err, exec.ErrNotFound):
			return nil, health.ComponentNotApplicable, "", ""
		case errors.Is(err, context.Canceled):
			return nil, health.ComponentUnknown, health.DiagnosticCancelled, "installation observation cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			return nil, health.ComponentUnknown, health.DiagnosticProbeTimeout, "Flatpak probe timed out"
		default:
			return nil, health.ComponentUnknown, health.DiagnosticProbeFailed, "Flatpak probe failed"
		}
	}
	present := make(map[string]bool, len(values))
	for _, value := range values {
		present[value] = true
	}
	return present, health.ComponentMissing, "", ""
}

func buildPackageHealth(tool Tool, expected []string, provider string, primaryNamespace health.PackageNamespace, formula, cask namespaceObservation) health.PackageFacet {
	if len(expected) == 0 {
		return health.PackageFacet{State: health.PackageNotApplicable}
	}
	namespaces := make([]health.PackageNamespaceFacet, 0, 2)
	if formula.available {
		namespaces = append(namespaces, buildNamespaceFacet(primaryNamespace, expected, formula))
	}
	if cask.available {
		namespaces = append(namespaces, buildNamespaceFacet(health.PackageNamespaceCask, expected, cask))
	}
	observed, missing, unresolved := aggregateReceipts(expected, formula, cask)
	state := packageState(expected, observed, missing, unresolved)
	code, summary := aggregatePackageDiagnostic(formula, cask)
	return health.PackageFacet{
		State: state, Provider: provider, ExpectedReceipts: expected, ObservedReceipts: observed,
		MissingReceipts: missing, UnresolvedReceipts: unresolved,
		Authoritative: packageMetadataIsAuthoritative(tool), Complete: len(unresolved) == 0,
		Namespaces: namespaces, DiagnosticCode: code, DiagnosticSummary: summary,
	}
}

func buildNamespaceFacet(namespace health.PackageNamespace, expected []string, observed namespaceObservation) health.PackageNamespaceFacet {
	present, missing, unresolved := partitionNamespace(expected, observed)
	return health.PackageNamespaceFacet{
		Namespace: namespace, State: packageState(expected, present, missing, unresolved),
		ExpectedReceipts: expected, ObservedReceipts: present, MissingReceipts: missing,
		UnresolvedReceipts: unresolved, Complete: len(unresolved) == 0,
		DiagnosticCode: observed.code, DiagnosticSummary: observed.summary,
	}
}

func partitionNamespace(expected []string, observed namespaceObservation) (present, missing, unresolved []string) {
	for _, receipt := range expected {
		switch observed.receipts[receipt] {
		case receiptPresent:
			present = append(present, receipt)
		case receiptMissing:
			missing = append(missing, receipt)
		case receiptUnknown:
			unresolved = append(unresolved, receipt)
		default:
			unresolved = append(unresolved, receipt)
		}
	}
	return present, missing, unresolved
}

func aggregateReceipts(expected []string, formula, cask namespaceObservation) (present, missing, unresolved []string) {
	available := []namespaceObservation{}
	if formula.available {
		available = append(available, formula)
	}
	if cask.available {
		available = append(available, cask)
	}
	for _, receipt := range expected {
		positive, unknown := false, len(available) == 0
		for _, namespace := range available {
			switch namespace.receipts[receipt] {
			case receiptPresent:
				positive = true
			case receiptUnknown:
				unknown = true
			case receiptMissing:
				// This namespace contributes conclusive negative evidence.
			default:
				unknown = true
			}
		}
		switch {
		case positive:
			present = append(present, receipt)
		case unknown:
			unresolved = append(unresolved, receipt)
		default:
			missing = append(missing, receipt)
		}
	}
	return present, missing, unresolved
}

func packageState(expected, observed, missing, unresolved []string) health.PackageState {
	switch {
	case len(expected) == 0:
		return health.PackageNotApplicable
	case len(observed) == len(expected):
		return health.PackagePresent
	case len(missing) == len(expected):
		return health.PackageMissing
	case len(unresolved) > 0 && len(observed) == 0:
		return health.PackageUnknown
	case len(observed) > 0 || len(missing) > 0:
		return health.PackagePartial
	default:
		return health.PackageUnknown
	}
}

func aggregatePackageDiagnostic(formula, cask namespaceObservation) (health.DiagnosticCode, string) {
	for _, candidate := range []health.DiagnosticCode{health.DiagnosticFallbackTimeout, health.DiagnosticCancelled, health.DiagnosticPackageBatchFailed} {
		if formula.code == candidate || cask.code == candidate {
			switch candidate {
			case health.DiagnosticFallbackTimeout:
				return candidate, "package fallback timed out"
			case health.DiagnosticCancelled:
				return candidate, "installation observation cancelled"
			case health.DiagnosticPackageBatchFailed:
				return candidate, "package receipt batch failed"
			case health.DiagnosticProbeFailed, health.DiagnosticProbeTimeout:
				return candidate, "package observation failed"
			}
		}
	}
	return "", ""
}

func observeInstallability(tool Tool, platform pkg.Platform, manager string, managerAvailable bool) (health.Installability, error) {
	if !managerAvailable {
		if len(PackagesForPlatform(tool.Packages(), platform)) == 0 {
			if _, provider := tool.(InstallRecipeProvider); !provider {
				return health.InstallabilityUnsupported, nil
			}
		}
		return health.InstallabilityUnknown, nil
	}
	recipe, available := describeInstallabilityRecipe(tool, platform, manager)
	if !available {
		return health.InstallabilityUnsupported, nil
	}
	if _, err := operation.InstallRecipeDigest(recipe); err != nil {
		return health.InstallabilityUnknown, errInvalidInstallRecipeMetadata
	}
	if recipe.ToolID != tool.ID() || recipe.Platform != string(platform) || recipe.Manager != manager {
		return health.InstallabilityUnknown, errInvalidInstallRecipeMetadata
	}
	return health.InstallabilitySupported, nil
}

func describeInstallabilityRecipe(tool Tool, platform pkg.Platform, manager string) (operation.InstallRecipe, bool) {
	recipe, err := DescribeInstall(tool, InstallEnvironment{Platform: platform, Manager: manager})
	return recipe, err == nil
}

func cancelledDirectHealth(tool Tool) health.DirectFacet {
	var alternatives []health.DirectAlternative
	if provider, ok := tool.(flatpakApplicationProvider); ok {
		ids := slices.Clone(provider.FlatpakApplicationIDs())
		if len(ids) > 0 {
			alternatives = append(alternatives, health.DirectAlternative{Kind: health.DirectSourceFlatpak, Identifiers: ids, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticCancelled, DiagnosticSummary: "installation observation cancelled"})
		}
	}
	if implementsLegacyDirect(tool) {
		alternatives = append(alternatives, health.DirectAlternative{Kind: health.DirectSourceLegacy, Identifiers: []string{tool.ID()}, State: health.ComponentUnknown, DiagnosticCode: health.DiagnosticCancelled, DiagnosticSummary: "installation observation cancelled"})
	}
	state := aggregateAlternativeState(alternatives)
	code, summary := aggregateDirectDiagnostic(alternatives)
	if _, typed := tool.(InstallationDirectAlternativesProvider); typed && len(alternatives) == 0 {
		state, code, summary = health.ComponentUnknown, health.DiagnosticCancelled, "installation observation cancelled"
	}
	return health.DirectFacet{State: state, Authoritative: !packageMetadataIsAuthoritative(tool), Alternatives: alternatives, DiagnosticCode: code, DiagnosticSummary: summary}
}

func implementsLegacyDirect(tool Tool) bool { _, ok := tool.(DirectInstallationDetector); return ok }

func buildDirectHealth(tool Tool, observation DirectInstallationObservation, flatpakState health.ComponentState, flatpakCode health.DiagnosticCode, flatpakSummary string) health.DirectFacet {
	var alternatives []health.DirectAlternative
	if provider, ok := tool.(InstallationDirectAlternativesProvider); ok {
		alternatives = provider.InstallationDirectAlternatives(cloneDirectInstallationObservation(observation))
	} else if detector, ok := tool.(DirectInstallationDetector); ok {
		if flatpakProvider, ok := tool.(flatpakApplicationProvider); ok && len(flatpakProvider.FlatpakApplicationIDs()) > 0 {
			ids := append([]string(nil), flatpakProvider.FlatpakApplicationIDs()...)
			state := flatpakState
			for _, id := range ids {
				if observation.FlatpakApplications[id] {
					state = health.ComponentPresent
					break
				}
			}
			alternatives = append(alternatives, health.DirectAlternative{Kind: health.DirectSourceFlatpak, Identifiers: ids, State: state, DiagnosticCode: flatpakCode, DiagnosticSummary: flatpakSummary})
		}
		legacyState := health.ComponentUnknown
		legacyObservation := DirectInstallationObservation{FlatpakApplications: map[string]bool{}}
		if detector.IsInstalledOutsidePackageManager(legacyObservation) {
			legacyState = health.ComponentPresent
		}
		alternatives = append(alternatives, health.DirectAlternative{Kind: health.DirectSourceLegacy, Identifiers: []string{tool.ID()}, State: legacyState})
	}
	state := aggregateAlternativeState(alternatives)
	code, summary := aggregateDirectDiagnostic(alternatives)
	return health.DirectFacet{State: state, Authoritative: !packageMetadataIsAuthoritative(tool), Alternatives: alternatives, DiagnosticCode: code, DiagnosticSummary: summary}
}

func cloneDirectInstallationObservation(observation DirectInstallationObservation) DirectInstallationObservation {
	cloned := DirectInstallationObservation{FlatpakApplications: make(map[string]bool, len(observation.FlatpakApplications))}
	for id, present := range observation.FlatpakApplications {
		cloned.FlatpakApplications[id] = present
	}
	return cloned
}

func aggregateAlternativeState(alternatives []health.DirectAlternative) health.ComponentState {
	if len(alternatives) == 0 {
		return health.ComponentNotApplicable
	}
	unknown, missing := false, false
	for _, value := range alternatives {
		switch value.State {
		case health.ComponentPresent:
			return health.ComponentPresent
		case health.ComponentUnknown:
			unknown = true
		case health.ComponentMissing:
			missing = true
		case health.ComponentNotApplicable:
			// This alternative contributes no installation evidence.
		}
	}
	if unknown {
		return health.ComponentUnknown
	}
	if missing {
		return health.ComponentMissing
	}
	return health.ComponentNotApplicable
}
func aggregateDirectDiagnostic(alternatives []health.DirectAlternative) (health.DiagnosticCode, string) {
	var codes []health.DiagnosticCode
	for _, alternative := range alternatives {
		if alternative.DiagnosticCode != "" {
			codes = append(codes, alternative.DiagnosticCode)
		}
	}
	if len(codes) == 0 {
		return "", ""
	}
	if len(codes) == 1 {
		for _, alternative := range alternatives {
			if alternative.DiagnosticCode != "" {
				return alternative.DiagnosticCode, alternative.DiagnosticSummary
			}
		}
	}
	allSame := true
	for _, code := range codes[1:] {
		allSame = allSame && code == codes[0]
	}
	if allSame {
		switch codes[0] {
		case health.DiagnosticCancelled:
			return health.DiagnosticCancelled, "installation observation cancelled"
		case health.DiagnosticProbeTimeout:
			return health.DiagnosticProbeTimeout, "direct probes timed out"
		case health.DiagnosticProbeFailed:
			return health.DiagnosticProbeFailed, "direct probes failed"
		case health.DiagnosticPackageBatchFailed:
			return health.DiagnosticPackageBatchFailed, "direct package observation failed"
		case health.DiagnosticFallbackTimeout:
			return health.DiagnosticFallbackTimeout, "direct fallback timed out"
		}
	}
	return health.DiagnosticProbeFailed, "multiple direct probes failed"
}
