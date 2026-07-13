package installplan

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

type freshCounts struct{ registry, platform, manager, collect, describe, capture, clock int }

func TestPlanFreshEmptyIntentUsesNoDependencies(t *testing.T) {
	panicCall := func() { t.Fatal("empty intent called a fresh-plan dependency") }
	session, err := PlanFresh(context.Background(), []string{"  ", "\t"}, FreshDependencies{
		Registry: func() []tools.Tool { panicCall(); return nil }, DetectPlatform: func() pkg.Platform { panicCall(); return "" },
		DetectManager: func() pkg.PackageManager { panicCall(); return nil },
	})
	if err != nil || session.Result().Public().Status() != planpublic.StatusIntentRequired || session.Manager() != nil {
		t.Fatalf("empty session status=%q manager=%v err=%v", session.Result().Public().Status(), session.Manager(), err)
	}
}

func TestPlanFreshReadyBindsOriginalSnapshotAndManagerOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", "")
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 1, mustObservation(t, "git", health.PackageMissing, recipe))
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	dependencies, counts := freshFixture(t, manager, snapshot, recipe)
	before, err := os.ReadDir(os.Getenv("HOME"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := PlanFresh(context.Background(), []string{"git", "git"}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(os.Getenv("HOME"))
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := session.Result().Accepted()
	if !ok || session.Manager() != manager || accepted.SnapshotAuthority().Digest != snapshot.Digest() {
		t.Fatalf("ready session accepted=%v manager identity=%v snapshot=%q/%q", ok, session.Manager() == manager, accepted.SnapshotAuthority().Digest, snapshot.Digest())
	}
	encoded := mustPublicJSON(t, session.Result().Public())
	if !strings.Contains(string(encoded), `"plan_hash":"`+accepted.Hash()+`"`) {
		t.Fatalf("ready public hash mismatch: %s", encoded)
	}
	if *counts != (freshCounts{registry: 1, platform: 1, manager: 1, collect: 1, describe: 1, capture: 1, clock: 1}) {
		t.Fatalf("fresh dependency counts=%+v", *counts)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("fresh planning created state: before=%v after=%v", before, after)
	}
}

func TestPlanFreshUnknownNoChangesAndCollectorMismatchRemainNonExecutable(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	for _, test := range []struct {
		name     string
		raw      []string
		snapshot health.InstallationSnapshot
		status   planpublic.Status
	}{
		{name: "unknown", raw: []string{"unknown-tool"}, snapshot: mustSnapshot(t, 1, mustObservation(t, "git", health.PackagePresent, recipe)), status: planpublic.StatusBlocked},
		{name: "no changes", raw: []string{"git"}, snapshot: mustSnapshot(t, 1, mustObservation(t, "git", health.PackagePresent, recipe)), status: planpublic.StatusNoChanges},
	} {
		t.Run(test.name, func(t *testing.T) {
			dependencies, counts := freshFixture(t, manager, test.snapshot, recipe)
			session, err := PlanFresh(context.Background(), test.raw, dependencies)
			if err != nil || session.Result().Public().Status() != test.status {
				t.Fatalf("status=%q err=%v", session.Result().Public().Status(), err)
			}
			if _, ok := session.Result().Accepted(); ok || counts.capture != 0 || counts.clock != 0 || counts.describe != 0 {
				t.Fatalf("non-ready authority/calls accepted=%v counts=%+v", ok, *counts)
			}
		})
	}

	extra := mustSnapshot(t, 1, mustObservation(t, "git", health.PackageMissing, recipe), mustObservation(t, "codex", health.PackagePresent, reviewedPackageRecipe("codex")))
	dependencies, _ := freshFixture(t, manager, extra, recipe)
	if _, err := PlanFresh(context.Background(), []string{"git"}, dependencies); !errors.Is(err, ErrFreshPlan) {
		t.Fatalf("extra collector observation error=%v", err)
	}
}

func TestPlanFreshInvalidSyntaxStopsBeforeDetection(t *testing.T) {
	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 1, mustObservation(t, "git", health.PackageMissing, recipe))
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	dependencies, counts := freshFixture(t, manager, snapshot, recipe)
	if _, err := PlanFresh(context.Background(), []string{"Git"}, dependencies); !errors.Is(err, planpublic.ErrInvalidIntent) {
		t.Fatalf("invalid intent error=%v", err)
	}
	if counts.registry != 1 || counts.platform != 0 || counts.manager != 0 || counts.collect != 0 {
		t.Fatalf("invalid intent dependency counts=%+v", *counts)
	}
}

func TestPlanFreshRejectsInvalidBoundaryValues(t *testing.T) {
	if _, err := PlanFresh(context.Background(), []string{"git"}, FreshDependencies{}); !errors.Is(err, ErrFreshPlan) {
		t.Fatalf("missing dependencies error=%v", err)
	}
	var nilContext context.Context
	if _, err := PlanFresh(nilContext, []string{"git"}, FreshDependencies{}); !errors.Is(err, ErrFreshPlan) {
		t.Fatalf("nil context error=%v", err)
	}
	var nilTool *tools.GitTool
	dependencies := FreshDependencies{
		Registry:       func() []tools.Tool { return []tools.Tool{nilTool} },
		DetectPlatform: func() pkg.Platform { t.Fatal("typed-nil registry reached platform detection"); return "" },
		DetectManager:  func() pkg.PackageManager { return nil },
		Collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return health.InstallationSnapshot{}, nil
		},
		DescribeInstall: func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
			return operation.InstallRecipe{}, nil
		},
		CaptureStatePlan: operation.CaptureStatePlan, Now: time.Now,
	}
	if _, err := PlanFresh(context.Background(), []string{"git"}, dependencies); !errors.Is(err, ErrFreshPlan) {
		t.Fatalf("typed-nil registry error=%v", err)
	}

	recipe := reviewedPackageRecipe("git")
	snapshot := mustSnapshot(t, 1, mustObservation(t, "git", health.PackageMissing, recipe))
	validManager := pkg.NewMockPackageManager()
	validManager.ManagerName = "brew"
	dependencies, counts := freshFixture(t, validManager, snapshot, recipe)
	var nilManager *pkg.MockPackageManager
	dependencies.DetectManager = func() pkg.PackageManager { counts.manager++; return nilManager }
	if _, err := PlanFresh(context.Background(), []string{"git"}, dependencies); !errors.Is(err, ErrFreshPlan) || counts.collect != 0 {
		t.Fatalf("typed-nil manager error=%v counts=%+v", err, *counts)
	}

	var zero FreshSession
	if zero.Manager() != nil || zero.Result().Public().Status() != "" {
		t.Fatalf("zero session manager=%v status=%q", zero.Manager(), zero.Result().Public().Status())
	}
	if _, ok := zero.Result().Accepted(); ok {
		t.Fatal("zero session exposed accepted authority")
	}
}

func freshFixture(t *testing.T, manager pkg.PackageManager, snapshot health.InstallationSnapshot, recipe operation.InstallRecipe) (FreshDependencies, *freshCounts) {
	t.Helper()
	counts := &freshCounts{}
	return FreshDependencies{
		Registry:       func() []tools.Tool { counts.registry++; return []tools.Tool{tools.NewGitTool()} },
		DetectPlatform: func() pkg.Platform { counts.platform++; return pkg.PlatformMacOS },
		DetectManager:  func() pkg.PackageManager { counts.manager++; return manager },
		Collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			counts.collect++
			return snapshot, nil
		},
		DescribeInstall: func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
			counts.describe++
			return operation.CloneInstallRecipe(recipe), nil
		},
		CaptureStatePlan: func() (*operation.StatePlan, error) { counts.capture++; return operation.CaptureStatePlan() },
		Now:              func() time.Time { counts.clock++; return time.Date(2026, 7, 12, 21, 0, 0, 0, time.UTC) },
	}, counts
}
