package installplan

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

var ErrFreshPlan = errors.New("fresh install plan failed")

type FreshDependencies struct {
	Registry         func() []tools.Tool
	DetectPlatform   func() pkg.Platform
	DetectManager    func() pkg.PackageManager
	Collect          func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error)
	DescribeInstall  func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error)
	CaptureStatePlan func() (*operation.StatePlan, error)
	Now              func() time.Time
}

type FreshSession struct {
	result  Result
	manager pkg.PackageManager
}

func (session FreshSession) Result() Result              { return session.result }
func (session FreshSession) Manager() pkg.PackageManager { return session.manager }

func PlanFresh(ctx context.Context, rawTools []string, dependencies FreshDependencies) (FreshSession, error) {
	if explicitIntentEmpty(rawTools) {
		result, err := Build(Request{Intent: planpublic.Intent{Source: "explicit_tools", Tools: []string{}}}, Dependencies{})
		if err != nil {
			return FreshSession{}, ErrFreshPlan
		}
		return FreshSession{result: result}, nil
	}
	if ctx == nil {
		return FreshSession{}, ErrFreshPlan
	}
	if dependencies.Registry == nil || dependencies.DetectPlatform == nil || dependencies.DetectManager == nil || dependencies.Collect == nil ||
		dependencies.DescribeInstall == nil || dependencies.CaptureStatePlan == nil || dependencies.Now == nil {
		return FreshSession{}, ErrFreshPlan
	}
	registryTools := slices.Clone(dependencies.Registry())
	if len(registryTools) == 0 {
		return FreshSession{}, ErrFreshPlan
	}
	registryByID := make(map[string]tools.Tool, len(registryTools))
	registeredIDs := make([]string, 0, len(registryTools))
	for _, tool := range registryTools {
		if toolIsNil(tool) {
			return FreshSession{}, ErrFreshPlan
		}
		id := tool.ID()
		if _, duplicate := registryByID[id]; duplicate {
			return FreshSession{}, ErrFreshPlan
		}
		registryByID[id] = tool
		registeredIDs = append(registeredIDs, id)
	}
	if _, err := planpublic.NormalizeExplicitTools(registeredIDs, registeredIDs); err != nil {
		return FreshSession{}, ErrFreshPlan
	}
	universeSet := make(map[string]struct{}, len(registeredIDs)+len(rawTools))
	for _, id := range registeredIDs {
		universeSet[id] = struct{}{}
	}
	for _, raw := range rawTools {
		if id := strings.TrimSpace(raw); id != "" {
			universeSet[id] = struct{}{}
		}
	}
	universe := make([]string, 0, len(universeSet))
	for id := range universeSet {
		universe = append(universe, id)
	}
	sort.Strings(universe)
	intent, err := planpublic.NormalizeExplicitTools(rawTools, universe)
	if err != nil {
		return FreshSession{}, planpublic.ErrInvalidIntent
	}
	platform := dependencies.DetectPlatform()
	manager := dependencies.DetectManager()
	if platform == pkg.PlatformUnknown || interfaceNil(manager) {
		return FreshSession{}, ErrFreshPlan
	}
	managerName := manager.Name()
	if managerName == "" {
		return FreshSession{}, ErrFreshPlan
	}
	var managerIdentity pkg.ExecutableIdentity
	if provider, ok := manager.(pkg.ExecutableIdentityProvider); ok {
		if observed, available := provider.ExecutableIdentity(); available && validManagerExecutableIdentity(observed) {
			managerIdentity = observed
		}
	}
	const generation uint64 = 1
	snapshot, err := dependencies.Collect(ctx, registryTools, manager, platform, generation)
	if err != nil || snapshot.SchemaVersion() != health.CurrentInstallationSchemaVersion || snapshot.Generation() != generation ||
		snapshot.Platform() != string(platform) || snapshot.Manager() != managerName || snapshot.Digest() == "" || !snapshotMatchesRegistry(snapshot, registryByID) {
		return FreshSession{}, ErrFreshPlan
	}
	result, err := Build(Request{Intent: intent, Snapshot: snapshot, Environment: Environment{Platform: platform, Manager: managerName, ManagerIdentity: managerIdentity, ExpectedGeneration: generation}}, Dependencies{
		LookupTool:      func(id string) (tools.Tool, bool) { tool, ok := registryByID[id]; return tool, ok },
		DescribeInstall: dependencies.DescribeInstall, CaptureStatePlan: dependencies.CaptureStatePlan, Now: dependencies.Now,
	})
	if err != nil {
		return FreshSession{}, ErrFreshPlan
	}
	return FreshSession{result: result, manager: manager}, nil
}

func explicitIntentEmpty(rawTools []string) bool {
	for _, raw := range rawTools {
		if strings.TrimSpace(raw) != "" {
			return false
		}
	}
	return true
}

func snapshotMatchesRegistry(snapshot health.InstallationSnapshot, registry map[string]tools.Tool) bool {
	for _, observation := range snapshot.Tools() {
		if _, ok := registry[observation.ToolID()]; !ok {
			return false
		}
	}
	return true
}

func interfaceNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	//nolint:exhaustive // Only nil-capable reflect kinds may be passed to IsNil.
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
