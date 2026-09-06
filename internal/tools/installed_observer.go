package tools

import (
	"bufio"
	"context"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
)

const (
	installationFallbackTimeout = 30 * time.Second
	installationFallbackWorkers = 4
	flatpakObservationTimeout   = 5 * time.Second
)

// DirectInstallationObservation is a process-wide snapshot of direct product
// evidence that is expensive to query repeatedly. Filesystem, app-bundle, and
// PATH checks remain local and cheap; Flatpak applications are enumerated once.
type DirectInstallationObservation struct {
	FlatpakApplications map[string]bool
}

// DirectInstallationDetector is implemented by tools that can be installed
// outside their package manager (binary, app bundle, desktop entry, AppImage,
// or Flatpak). Package-backed and direct evidence are combined centrally.
type DirectInstallationDetector interface {
	IsInstalledOutsidePackageManager(DirectInstallationObservation) bool
}

type flatpakApplicationProvider interface {
	FlatpakApplicationIDs() []string
}

type packageMetadataPolicy interface {
	PackageMetadataIsAuthoritative() bool
}

type installedCaskLister interface {
	ListInstalledCasks() ([]string, error)
}

type contextPackageQuery interface {
	IsInstalledContext(context.Context, string) bool
}

type installationObservationRuntime struct {
	listFlatpakApplications func(context.Context) ([]string, error)
	fallbackTimeout         time.Duration
	fallbackWorkers         int
}

func defaultInstallationObservationRuntime() installationObservationRuntime {
	return installationObservationRuntime{
		listFlatpakApplications: listFlatpakApplications,
		fallbackTimeout:         installationFallbackTimeout,
		fallbackWorkers:         installationFallbackWorkers,
	}
}

// ObserveInstallations returns one coherent installation snapshot for all
// tools. A complete package-manager batch is authoritative and never falls
// back to per-package probes. Direct product evidence is still ORed in so
// manual/out-of-band installations remain visible.
func ObserveInstallations(ctx context.Context, all []Tool, mgr pkg.PackageManager, platform pkg.Platform) map[string]bool {
	return observeInstallationsWithRuntime(ctx, all, mgr, platform, defaultInstallationObservationRuntime())
}

func observeInstallationsWithRuntime(ctx context.Context, all []Tool, mgr pkg.PackageManager, platform pkg.Platform, runtime installationObservationRuntime) map[string]bool {
	direct := captureDirectInstallationObservation(ctx, all, runtime)
	installedPackages, complete := captureInstalledPackageBatch(mgr)
	if !complete {
		installedPackages = captureInstalledPackageFallback(ctx, all, mgr, platform, installedPackages, runtime)
	}

	observed := make(map[string]bool, len(all))
	for _, tool := range all {
		directInstalled := false
		if detector, ok := tool.(DirectInstallationDetector); ok {
			directInstalled = detector.IsInstalledOutsidePackageManager(direct)
		}
		if !packageMetadataIsAuthoritative(tool) {
			observed[tool.ID()] = directInstalled
			continue
		}
		packages := PackagesForPlatform(tool.Packages(), platform)
		observed[tool.ID()] = directInstalled || allPackagesInObservation(installedPackages, packages)
	}
	return observed
}

func packageMetadataIsAuthoritative(tool Tool) bool {
	policy, ok := tool.(packageMetadataPolicy)
	return !ok || policy.PackageMetadataIsAuthoritative()
}

func captureInstalledPackageBatch(mgr pkg.PackageManager) (map[string]bool, bool) {
	if mgr == nil {
		return nil, false
	}
	packages, err := mgr.ListInstalled()
	if err != nil {
		return nil, false
	}
	installed := make(map[string]bool, len(packages))
	for _, candidate := range packages {
		installed[candidate.Name] = true
	}
	if casks, ok := mgr.(installedCaskLister); ok {
		values, err := casks.ListInstalledCasks()
		if err != nil {
			return installed, false
		}
		for _, value := range values {
			installed[value] = true
		}
	}
	return installed, true
}

func captureInstalledPackageFallback(ctx context.Context, all []Tool, mgr pkg.PackageManager, platform pkg.Platform, seed map[string]bool, runtime installationObservationRuntime) map[string]bool {
	installed := make(map[string]bool, len(seed))
	for name, present := range seed {
		installed[name] = present
	}
	if mgr == nil {
		return installed
	}
	query, contextual := mgr.(contextPackageQuery)
	if !contextual {
		// An outer timeout cannot safely abandon an interface that does not
		// accept its context. Supported managers implement contextPackageQuery;
		// unknown legacy managers fail closed on unresolved receipts.
		return installed
	}
	unique := make(map[string]struct{})
	for _, tool := range all {
		if !packageMetadataIsAuthoritative(tool) {
			continue
		}
		for _, name := range PackagesForPlatform(tool.Packages(), platform) {
			if !installed[name] {
				unique[name] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) == 0 {
		return installed
	}

	timeout := runtime.fallbackTimeout
	if timeout <= 0 {
		timeout = installationFallbackTimeout
	}
	fallbackCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	workers := runtime.fallbackWorkers
	if workers <= 0 {
		workers = 1
	}
	workers = min(workers, len(names))
	jobs := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				if fallbackCtx.Err() != nil {
					continue
				}
				present := query.IsInstalledContext(fallbackCtx, name)
				if present {
					mu.Lock()
					installed[name] = true
					mu.Unlock()
				}
			}
		}()
	}
	for _, name := range names {
		jobs <- name
	}
	close(jobs)
	wg.Wait()
	return installed
}

func allPackagesInObservation(installed map[string]bool, packages []string) bool {
	if len(packages) == 0 {
		return false
	}
	for _, name := range packages {
		if !installed[name] {
			return false
		}
	}
	return true
}

func captureDirectInstallationObservation(ctx context.Context, all []Tool, runtime installationObservationRuntime) DirectInstallationObservation {
	observation := DirectInstallationObservation{FlatpakApplications: make(map[string]bool)}
	needsFlatpak := false
	for _, tool := range all {
		provider, ok := tool.(flatpakApplicationProvider)
		if ok && len(provider.FlatpakApplicationIDs()) > 0 {
			needsFlatpak = true
			break
		}
	}
	if !needsFlatpak || runtime.listFlatpakApplications == nil {
		return observation
	}
	applications, err := runtime.listFlatpakApplications(ctx)
	if err != nil {
		return observation
	}
	for _, application := range applications {
		observation.FlatpakApplications[strings.TrimSpace(application)] = true
	}
	return observation
}

func listFlatpakApplications(ctx context.Context) ([]string, error) {
	flatpak, err := exec.LookPath("flatpak")
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, flatpakObservationTimeout)
	defer cancel()
	cmd := exec.CommandContext(queryCtx, flatpak, "list", "--app", "--columns=application") // #nosec G204 -- fixed executable and arguments.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var applications []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if value := strings.TrimSpace(scanner.Text()); value != "" {
			applications = append(applications, value)
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return applications, nil
}

func directInstallationDetected(tool Tool) bool {
	runtime := defaultInstallationObservationRuntime()
	observation := captureDirectInstallationObservation(context.Background(), []Tool{tool}, runtime)
	detector, ok := tool.(DirectInstallationDetector)
	return ok && detector.IsInstalledOutsidePackageManager(observation)
}
