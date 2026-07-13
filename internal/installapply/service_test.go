package installapply

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type applyJournal struct {
	sequence *[]string
	records  []operation.Record
	err      error
	failAt   int
	onWrite  func(int)
}

func (journal *applyJournal) Write(record operation.Record) error {
	*journal.sequence = append(*journal.sequence, "write:"+string(record.Status))
	journal.records = append(journal.records, record)
	if journal.onWrite != nil {
		journal.onWrite(len(journal.records))
	}
	if journal.failAt == len(journal.records) {
		return journal.err
	}
	if journal.failAt != 0 {
		return nil
	}
	return journal.err
}

func TestApplyRejectsInvalidAndWrongHashBeforeMutation(t *testing.T) {
	session, _, hash := applyFreshSession(t, health.PresenceMissing)
	plans := 0
	dependencies := Dependencies{PlanFresh: func(context.Context, []string) (installplan.FreshSession, error) { plans++; return session, nil }}
	for _, request := range []Request{{RawTools: []string{"git"}}, {RawTools: []string{"git"}, ExpectedHash: "ABC"}, {RawTools: []string{"  "}, ExpectedHash: strings.Repeat("a", 64)}} {
		if _, err := Apply(context.Background(), request, dependencies); !errors.Is(err, ErrInvalidApplyRequest) {
			t.Fatalf("invalid request=%+v error=%v", request, err)
		}
	}
	if plans != 0 {
		t.Fatalf("invalid request planned %d times", plans)
	}
	dependencies = rejectingMutationDependencies(t, func(context.Context, []string) (installplan.FreshSession, error) { plans++; return session, nil })
	wrong := strings.Repeat("f", 64)
	if wrong == hash {
		wrong = strings.Repeat("e", 64)
	}
	if _, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: wrong}, dependencies); !errors.Is(err, ErrPlanHashMismatch) {
		t.Fatalf("wrong hash error=%v", err)
	}
	if plans != 1 {
		t.Fatalf("wrong hash plans=%d", plans)
	}
}

func TestApplyRejectsNonReadyFreshSessionBeforeMutation(t *testing.T) {
	session, _, _ := applyFreshSession(t, health.PresencePresent)
	dependencies := rejectingMutationDependencies(t, func(context.Context, []string) (installplan.FreshSession, error) { return session, nil })
	if _, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: strings.Repeat("a", 64)}, dependencies); !errors.Is(err, ErrPlanNotReady) {
		t.Fatalf("no-changes error=%v", err)
	}
}

func TestApplyHappyPathUsesExactSessionManagerAndJournalsPackageOnlyWarning(t *testing.T) {
	session, manager, hash := applyFreshSession(t, health.PresenceMissing)
	accepted, ok := session.Result().Accepted()
	if !ok {
		t.Fatal("fixture has no accepted plan")
	}
	var sequence []string
	journal := &applyJournal{sequence: &sequence}
	nowCalls := 0
	detectCalls := 0
	var executed operation.InstallRecipe
	var executedManager pkg.PackageManager
	dependencies := Dependencies{
		PlanFresh: func(context.Context, []string) (installplan.FreshSession, error) {
			sequence = append(sequence, "plan")
			return session, nil
		},
		UserHomeDir: func() (string, error) { sequence = append(sequence, "home"); return t.TempDir(), nil },
		BootstrapState: func(plan *operation.StatePlan) (*operation.StateAuthority, error) {
			sequence = append(sequence, "bootstrap")
			if plan != accepted.StatePlan() {
				t.Fatal("bootstrap received different StatePlan")
			}
			return operation.BootstrapStateNamespaceTracked(plan)
		},
		AcquireLock: func(state *operation.StateAuthority, scope, target string) (func() error, error) {
			sequence = append(sequence, "lock:"+scope)
			if state == nil || !filepathIsAbsolute(target) {
				t.Fatal("invalid lock authority")
			}
			return func() error { sequence = append(sequence, "release"); return nil }, nil
		},
		OpenJournal: func(*operation.StateAuthority) (JournalWriter, error) {
			sequence = append(sequence, "open-journal")
			return journal, nil
		},
		StartRecord: func(plan operation.Plan, now time.Time) (operation.Record, error) {
			sequence = append(sequence, "start-record")
			return operation.StartRecord(plan, now)
		},
		Now: func() time.Time {
			nowCalls++
			sequence = append(sequence, "now")
			return time.Date(2026, 7, 12, 22, nowCalls, 0, 0, time.UTC)
		},
		ExecuteRecipe: func(_ context.Context, recipe operation.InstallRecipe, gotManager pkg.PackageManager, _ pkg.ExecutableIdentity, _ func(string)) error {
			sequence = append(sequence, "execute:"+recipe.ToolID)
			executed, executedManager = operation.CloneInstallRecipe(recipe), gotManager
			return nil
		},
		DetectRecipe: func(recipe operation.InstallRecipe, gotManager pkg.PackageManager) (bool, error) {
			detectCalls++
			sequence = append(sequence, "detect:"+recipe.ToolID)
			if gotManager != manager {
				t.Fatal("detector received different manager")
			}
			return detectCalls == 3, nil
		},
	}
	result, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != operation.StatusSucceeded || result.PlanHash != hash || result.Succeeded != 1 || result.Failed != 0 || executedManager != manager {
		t.Fatalf("result=%+v manager identity=%v", result, executedManager == manager)
	}
	wantRecipe := accepted.Recipes()["git"]
	if !reflect.DeepEqual(executed, wantRecipe) {
		t.Fatalf("executed recipe=%+v want=%+v", executed, wantRecipe)
	}
	if len(journal.records) != 2 || journal.records[0].Status != operation.StatusRunning || journal.records[0].PlanHash != hash || journal.records[1].Status != operation.StatusSucceeded ||
		len(journal.records[1].Warnings) != 1 || journal.records[1].Warnings[0] != packageOnlyWarning || journal.records[1].Backup != "" || journal.records[1].Rollback != nil {
		t.Fatalf("journal records=%+v", journal.records)
	}
	wantSequence := []string{"plan", "detect:git", "home", "bootstrap", "lock:install-operation", "now", "start-record", "open-journal", "write:running", "detect:git", "execute:git", "detect:git", "release", "now", "write:succeeded"}
	if !reflect.DeepEqual(sequence, wantSequence) {
		t.Fatalf("sequence=%v want=%v", sequence, wantSequence)
	}
}

func applyFreshSession(t *testing.T, presence health.Presence) (installplan.FreshSession, pkg.PackageManager, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	tool := tools.NewGitTool()
	recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		t.Fatal(err)
	}
	state := health.PackageMissing
	facet := health.PackageFacet{State: state, Provider: "brew", ExpectedReceipts: []string{"git"}, MissingReceipts: []string{"git"}, Authoritative: true, Complete: true,
		Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: state, ExpectedReceipts: []string{"git"}, MissingReceipts: []string{"git"}, Complete: true}}}
	if presence == health.PresencePresent {
		facet.State, facet.MissingReceipts, facet.ObservedReceipts = health.PackagePresent, nil, []string{"git"}
		facet.Namespaces[0].State, facet.Namespaces[0].MissingReceipts, facet.Namespaces[0].ObservedReceipts = health.PackagePresent, nil, []string{"git"}
	}
	observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{ToolID: "git", Installability: health.InstallabilitySupported, InstallRecipeDigest: digest, Package: facet, Direct: health.DirectFacet{State: health.ComponentNotApplicable}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: []health.InstallationObservation{observation}})
	if err != nil {
		t.Fatal(err)
	}
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	managerIdentity, _ := applyManagerIdentity(t, "planned-brew", "exit 0")
	if err := manager.SetExecutableIdentity(managerIdentity); err != nil {
		t.Fatal(err)
	}
	session, err := installplan.PlanFresh(context.Background(), []string{"git"}, installplan.FreshDependencies{
		Registry: func() []tools.Tool { return []tools.Tool{tool} }, DetectPlatform: func() pkg.Platform { return pkg.PlatformMacOS }, DetectManager: func() pkg.PackageManager { return manager },
		Collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return snapshot, nil
		},
		DescribeInstall: func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error) {
			return operation.CloneInstallRecipe(recipe), nil
		},
		CaptureStatePlan: operation.CaptureStatePlan, Now: func() time.Time { return time.Date(2026, 7, 12, 21, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := session.Result().Accepted()
	hash := strings.Repeat("a", 64)
	if ok {
		hash = accepted.Hash()
	}
	return session, manager, hash
}

func rejectingMutationDependencies(t *testing.T, plan func(context.Context, []string) (installplan.FreshSession, error)) Dependencies {
	t.Helper()
	panicCall := func(name string) { t.Fatalf("%s called before hash/readiness gate", name) }
	return Dependencies{
		PlanFresh:      plan,
		UserHomeDir:    func() (string, error) { panicCall("HOME"); return "", nil },
		BootstrapState: func(*operation.StatePlan) (*operation.StateAuthority, error) { panicCall("bootstrap"); return nil, nil },
		AcquireLock: func(*operation.StateAuthority, string, string) (func() error, error) {
			panicCall("lock")
			return nil, nil
		},
		OpenJournal: func(*operation.StateAuthority) (JournalWriter, error) { panicCall("journal"); return nil, nil },
		StartRecord: func(operation.Plan, time.Time) (operation.Record, error) {
			panicCall("record")
			return operation.Record{}, nil
		},
		Now: func() time.Time { panicCall("clock"); return time.Time{} },
		ExecuteRecipe: func(context.Context, operation.InstallRecipe, pkg.PackageManager, pkg.ExecutableIdentity, func(string)) error {
			panicCall("execute")
			return nil
		},
		DetectRecipe: func(operation.InstallRecipe, pkg.PackageManager) (bool, error) {
			panicCall("detect")
			return false, nil
		},
	}
}

func filepathIsAbsolute(value string) bool { return filepath.IsAbs(value) }

func TestApplyPlanningErrorsAndCancellationStopBeforeBootstrap(t *testing.T) {
	session, _, hash := applyFreshSession(t, health.PresenceMissing)
	for _, tc := range []struct {
		name string
		ctx  func() context.Context
		plan func(context.Context, []string) (installplan.FreshSession, error)
	}{
		{name: "planning failure", ctx: context.Background, plan: func(context.Context, []string) (installplan.FreshSession, error) {
			return installplan.FreshSession{}, errors.New("private planning detail")
		}},
		{name: "cancelled before planning", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, plan: func(context.Context, []string) (installplan.FreshSession, error) {
			t.Fatal("planning called after cancellation")
			return installplan.FreshSession{}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mutations := 0
			deps := rejectingMutationDependencies(t, tc.plan)
			deps.BootstrapState = func(*operation.StatePlan) (*operation.StateAuthority, error) {
				mutations++
				return nil, errors.New("unexpected bootstrap")
			}
			_, err := Apply(tc.ctx(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps)
			if !errors.Is(err, ErrApplyFailed) || mutations != 0 {
				t.Fatalf("error=%v mutations=%d", err, mutations)
			}
		})
	}
	_ = session
}

func TestApplyCancellationDuringPreflightStopsBeforeBootstrap(t *testing.T) {
	session, _, hash := applyFreshSession(t, health.PresenceMissing)
	ctx, cancel := context.WithCancel(context.Background())
	deps := rejectingMutationDependencies(t, func(context.Context, []string) (installplan.FreshSession, error) { return session, nil })
	deps.DetectRecipe = func(operation.InstallRecipe, pkg.PackageManager) (bool, error) {
		cancel()
		return false, nil
	}
	if _, err := Apply(ctx, Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps); !errors.Is(err, ErrApplyFailed) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestApplyCancellationDuringHomeLookupStopsBeforeBootstrap(t *testing.T) {
	session, manager, hash := applyFreshSession(t, health.PresenceMissing)
	ctx, cancel := context.WithCancel(context.Background())
	var sequence []string
	journal := &applyJournal{sequence: &sequence}
	deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
	deps.UserHomeDir = func() (string, error) {
		cancel()
		return t.TempDir(), nil
	}
	deps.BootstrapState = func(*operation.StatePlan) (*operation.StateAuthority, error) {
		t.Fatal("bootstrap called after HOME cancelled context")
		return nil, nil
	}
	if _, err := Apply(ctx, Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps); !errors.Is(err, ErrApplyFailed) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestApplyRejectsSessionManagerDriftBeforeInfrastructureMutation(t *testing.T) {
	session, manager, hash := applyFreshSession(t, health.PresenceMissing)
	mock, ok := manager.(*pkg.MockPackageManager)
	if !ok {
		t.Fatalf("manager type=%T", manager)
	}
	mock.ManagerName = "apt"
	deps := rejectingMutationDependencies(t, func(context.Context, []string) (installplan.FreshSession, error) { return session, nil })
	if _, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps); !errors.Is(err, ErrApplyFailed) {
		t.Fatalf("error=%v", err)
	}
}

func TestApplyCancellationAfterRunningJournalSkipsMutationAndReleasesOnce(t *testing.T) {
	session, manager, hash := applyFreshSession(t, health.PresenceMissing)
	ctx, cancel := context.WithCancel(context.Background())
	var sequence []string
	journal := &applyJournal{sequence: &sequence, onWrite: func(index int) {
		if index == 1 {
			cancel()
		}
	}}
	releases, executes := 0, 0
	deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
	deps.AcquireLock = func(*operation.StateAuthority, string, string) (func() error, error) {
		return func() error { releases++; return nil }, nil
	}
	deps.ExecuteRecipe = func(context.Context, operation.InstallRecipe, pkg.PackageManager, pkg.ExecutableIdentity, func(string)) error {
		executes++
		return nil
	}
	result, err := Apply(ctx, Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps)
	if !errors.Is(err, ErrApplyFailed) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if result.Status != operation.StatusCancelled || result.Succeeded != 0 || result.Failed != 0 || executes != 0 || releases != 1 {
		t.Fatalf("result=%+v executes=%d releases=%d", result, executes, releases)
	}
	if len(journal.records) != 2 || journal.records[1].Status != operation.StatusCancelled || journal.records[1].Actions[0].Status != operation.ActionSkipped {
		t.Fatalf("records=%+v", journal.records)
	}
}

func TestApplyLockedDetectorBecomingSatisfiedSkipsWithoutExecutionAndReleasesOnce(t *testing.T) {
	session, manager, hash := applyFreshSession(t, health.PresenceMissing)
	var sequence []string
	journal := &applyJournal{sequence: &sequence}
	releases, executes, detects := 0, 0, 0
	deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
	deps.AcquireLock = func(*operation.StateAuthority, string, string) (func() error, error) {
		return func() error { releases++; return nil }, nil
	}
	deps.DetectRecipe = func(operation.InstallRecipe, pkg.PackageManager) (bool, error) {
		detects++
		return detects == 2, nil
	}
	deps.ExecuteRecipe = func(context.Context, operation.InstallRecipe, pkg.PackageManager, pkg.ExecutableIdentity, func(string)) error {
		executes++
		return nil
	}
	result, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps)
	if err != nil || result.Status != operation.StatusSucceeded || result.Succeeded != 0 || result.Failed != 0 || executes != 0 || detects != 2 || releases != 1 {
		t.Fatalf("result=%+v error=%v executes=%d detects=%d releases=%d", result, err, executes, detects, releases)
	}
	if len(journal.records) != 2 || journal.records[1].Actions[0].Status != operation.ActionSkipped || journal.records[1].Actions[0].Summary != "already satisfied after review" {
		t.Fatalf("records=%+v", journal.records)
	}
}

func TestApplyOverlappingReceiptSkipsLaterSatisfiedAction(t *testing.T) {
	session, manager, hash := applyMultiFreshSession(t)
	var sequence []string
	journal := &applyJournal{sequence: &sequence}
	deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
	detectCalls := make(map[string]int)
	firstExecuted := false
	deps.DetectRecipe = func(recipe operation.InstallRecipe, _ pkg.PackageManager) (bool, error) {
		detectCalls[recipe.ToolID]++
		switch recipe.ToolID {
		case "git":
			return firstExecuted && detectCalls[recipe.ToolID] >= 3, nil
		case "zsh":
			return firstExecuted && detectCalls[recipe.ToolID] >= 2, nil
		default:
			return false, nil
		}
	}
	var executed []string
	deps.ExecuteRecipe = func(_ context.Context, recipe operation.InstallRecipe, _ pkg.PackageManager, _ pkg.ExecutableIdentity, _ func(string)) error {
		executed = append(executed, recipe.ToolID)
		if recipe.ToolID == "git" {
			firstExecuted = true
		}
		return nil
	}
	result, err := Apply(context.Background(), Request{RawTools: []string{"zsh", "git"}, ExpectedHash: hash}, deps)
	if err != nil || result.Status != operation.StatusSucceeded || result.Succeeded != 1 || result.Failed != 0 || !reflect.DeepEqual(executed, []string{"git"}) {
		t.Fatalf("result=%+v error=%v executed=%v detects=%v", result, err, executed, detectCalls)
	}
	if len(journal.records) != 2 || len(journal.records[1].Actions) != 2 || journal.records[1].Actions[0].Status != operation.ActionSucceeded || journal.records[1].Actions[1].Status != operation.ActionSkipped || journal.records[1].Actions[1].Summary != "already satisfied after review" {
		t.Fatalf("terminal overlap record=%+v", journal.records)
	}
}

func TestApplyAllActionPreflightStopsFirstMutationWhenLaterActionDrifted(t *testing.T) {
	session, _, hash := applyMultiFreshSession(t)
	executes := 0
	deps := rejectingMutationDependencies(t, func(context.Context, []string) (installplan.FreshSession, error) { return session, nil })
	deps.DetectRecipe = func(recipe operation.InstallRecipe, _ pkg.PackageManager) (bool, error) {
		return recipe.ToolID == "zsh", nil
	}
	deps.ExecuteRecipe = func(context.Context, operation.InstallRecipe, pkg.PackageManager, pkg.ExecutableIdentity, func(string)) error {
		executes++
		return nil
	}
	if _, err := Apply(context.Background(), Request{RawTools: []string{"git", "zsh"}, ExpectedHash: hash}, deps); !errors.Is(err, ErrPlanNotReady) || executes != 0 {
		t.Fatalf("error=%v executes=%d, want preflight refusal before mutation", err, executes)
	}
}

func TestApplyManagerIdentitySentinelStopsLaterActions(t *testing.T) {
	session, manager, hash := applyMultiFreshSession(t)
	var sequence []string
	journal := &applyJournal{sequence: &sequence}
	deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
	executes := 0
	deps.ExecuteRecipe = func(context.Context, operation.InstallRecipe, pkg.PackageManager, pkg.ExecutableIdentity, func(string)) error {
		executes++
		return fmt.Errorf("step boundary: %w", ErrManagerIdentityChanged)
	}
	result, err := Apply(context.Background(), Request{RawTools: []string{"git", "zsh"}, ExpectedHash: hash}, deps)
	if !errors.Is(err, ErrApplyFailed) || !errors.Is(err, ErrManagerIdentityChanged) || result.Status != operation.StatusFailed || result.Failed != 1 || executes != 1 {
		t.Fatalf("result=%+v error=%v executes=%d", result, err, executes)
	}
	if len(journal.records) != 2 || len(journal.records[1].Actions) != 2 || journal.records[1].Actions[0].Status != operation.ActionFailed || journal.records[1].Actions[1].Status != operation.ActionSkipped {
		t.Fatalf("sentinel terminal record=%+v", journal.records)
	}
}

func TestApplyPreflightDetectorFailureDoesNotBootstrapOrJournal(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(operation.InstallRecipe, pkg.PackageManager) (bool, error)
	}{
		{name: "drift", fn: func(operation.InstallRecipe, pkg.PackageManager) (bool, error) { return true, nil }},
		{name: "error", fn: func(operation.InstallRecipe, pkg.PackageManager) (bool, error) {
			return false, errors.New("private detector error")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, _, hash := applyFreshSession(t, health.PresenceMissing)
			deps := rejectingMutationDependencies(t, func(context.Context, []string) (installplan.FreshSession, error) { return session, nil })
			deps.DetectRecipe = tc.fn
			if _, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps); !errors.Is(err, ErrPlanNotReady) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestApplyLifecycleFailuresReleaseExactlyOnceAfterLock(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Dependencies, *applyJournal)
		wantRelease int
	}{
		{name: "home", mutate: func(deps *Dependencies, _ *applyJournal) {
			deps.UserHomeDir = func() (string, error) { return "", errors.New("home") }
		}},
		{name: "bootstrap", mutate: func(deps *Dependencies, _ *applyJournal) {
			deps.BootstrapState = func(*operation.StatePlan) (*operation.StateAuthority, error) { return nil, errors.New("bootstrap") }
		}},
		{name: "acquire", mutate: func(deps *Dependencies, _ *applyJournal) {
			deps.AcquireLock = func(*operation.StateAuthority, string, string) (func() error, error) { return nil, errors.New("lock") }
		}},
		{name: "start record", wantRelease: 1, mutate: func(deps *Dependencies, _ *applyJournal) {
			deps.StartRecord = func(operation.Plan, time.Time) (operation.Record, error) {
				return operation.Record{}, errors.New("start")
			}
		}},
		{name: "open journal", wantRelease: 1, mutate: func(deps *Dependencies, _ *applyJournal) {
			deps.OpenJournal = func(*operation.StateAuthority) (JournalWriter, error) { return nil, errors.New("journal") }
		}},
		{name: "running journal", wantRelease: 1, mutate: func(_ *Dependencies, journal *applyJournal) { journal.err = errors.New("running write") }},
		{name: "terminal journal", wantRelease: 1, mutate: func(_ *Dependencies, journal *applyJournal) {
			journal.err, journal.failAt = errors.New("terminal write"), 2
		}},
		{name: "finish record", wantRelease: 1, mutate: func(deps *Dependencies, _ *applyJournal) {
			calls := 0
			deps.Now = func() time.Time {
				calls++
				if calls == 1 {
					return time.Date(2026, 7, 12, 23, 0, 0, 0, time.UTC)
				}
				return time.Date(2026, 7, 12, 22, 0, 0, 0, time.UTC)
			}
		}},
		{name: "release", wantRelease: 1, mutate: func(deps *Dependencies, _ *applyJournal) {
			deps.AcquireLock = func(*operation.StateAuthority, string, string) (func() error, error) {
				return func() error { return errors.New("release") }, nil
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session, manager, hash := applyFreshSession(t, health.PresenceMissing)
			var sequence []string
			journal := &applyJournal{sequence: &sequence}
			releases := 0
			deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
			deps.AcquireLock = func(*operation.StateAuthority, string, string) (func() error, error) {
				return func() error { releases++; return nil }, nil
			}
			tc.mutate(&deps, journal)
			if tc.name == "release" {
				original := deps.AcquireLock
				deps.AcquireLock = func(state *operation.StateAuthority, scope, target string) (func() error, error) {
					release, err := original(state, scope, target)
					return func() error { sequence = append(sequence, "release"); releases++; return release() }, err
				}
			}
			_, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps)
			if !errors.Is(err, ErrApplyFailed) || releases != tc.wantRelease {
				t.Fatalf("error=%v releases=%d want=%d", err, releases, tc.wantRelease)
			}
			if tc.name == "release" {
				if len(journal.records) != 2 || journal.records[1].Status != operation.StatusFailed {
					t.Fatalf("release failure was not persisted in terminal record: %+v", journal.records)
				}
				wantTail := []string{"release", "write:failed"}
				if len(sequence) < 2 || !reflect.DeepEqual(sequence[len(sequence)-2:], wantTail) {
					t.Fatalf("release/terminal ordering=%v want tail=%v", sequence, wantTail)
				}
			}
		})
	}
}

func TestApplyMultiActionContinuesOrdinaryFailureButStopsOnCancellation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		firstError    error
		wantStatus    operation.Status
		wantSucceeded int
		wantFailed    int
		wantExecutes  int
		secondStatus  operation.ActionStatus
	}{
		{name: "ordinary failure continues", firstError: errors.New("install failed"), wantStatus: operation.StatusFailed, wantSucceeded: 1, wantFailed: 1, wantExecutes: 2, secondStatus: operation.ActionSucceeded},
		{name: "cancellation stops", firstError: context.Canceled, wantStatus: operation.StatusCancelled, wantFailed: 1, wantExecutes: 1, secondStatus: operation.ActionSkipped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, manager, hash := applyMultiFreshSession(t)
			var sequence []string
			journal := &applyJournal{sequence: &sequence}
			deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
			executes := 0
			deps.ExecuteRecipe = func(context.Context, operation.InstallRecipe, pkg.PackageManager, pkg.ExecutableIdentity, func(string)) error {
				executes++
				if executes == 1 {
					return tc.firstError
				}
				return nil
			}
			result, err := Apply(context.Background(), Request{RawTools: []string{"zsh", "git", "git"}, ExpectedHash: hash}, deps)
			if !errors.Is(err, ErrApplyFailed) || result.Status != tc.wantStatus || result.Succeeded != tc.wantSucceeded || result.Failed != tc.wantFailed || executes != tc.wantExecutes {
				t.Fatalf("result=%+v error=%v executes=%d", result, err, executes)
			}
			if len(journal.records) != 2 || len(journal.records[1].Actions) != 2 || journal.records[1].Actions[0].Status != operation.ActionFailed || journal.records[1].Actions[1].Status != tc.secondStatus {
				t.Fatalf("records=%+v", journal.records)
			}
		})
	}
}

func TestApplyMixedPresentAndMissingExecutesOnlyMissingAuthority(t *testing.T) {
	session, manager, hash := applyMultiFreshSessionWithPresence(t, map[string]health.Presence{
		"git": health.PresencePresent,
		"zsh": health.PresenceMissing,
	})
	accepted, ok := session.Result().Accepted()
	if !ok || len(accepted.ToolAuthorities()) != 2 || len(accepted.Operation().Actions()) != 1 {
		t.Fatalf("mixed accepted authority=%v actions=%d", ok, len(accepted.Operation().Actions()))
	}
	var sequence, executed []string
	journal := &applyJournal{sequence: &sequence}
	deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
	deps.ExecuteRecipe = func(_ context.Context, recipe operation.InstallRecipe, _ pkg.PackageManager, _ pkg.ExecutableIdentity, _ func(string)) error {
		executed = append(executed, recipe.ToolID)
		return nil
	}
	result, err := Apply(context.Background(), Request{RawTools: []string{"zsh", "git"}, ExpectedHash: hash}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != operation.StatusSucceeded || result.Succeeded != 1 || result.Failed != 0 || !reflect.DeepEqual(executed, []string{"zsh"}) {
		t.Fatalf("result=%+v executed=%v", result, executed)
	}
}

func TestApplyRecipeArgumentsAreDefensiveAgainstDependencyPoisoning(t *testing.T) {
	session, manager, hash := applyFreshSession(t, health.PresenceMissing)
	accepted, _ := session.Result().Accepted()
	want := accepted.Recipes()["git"]
	var sequence []string
	journal := &applyJournal{sequence: &sequence}
	deps := successfulApplyDependencies(t, session, manager, journal, &sequence)
	detects := 0
	deps.DetectRecipe = func(recipe operation.InstallRecipe, _ pkg.PackageManager) (bool, error) {
		detects++
		if !reflect.DeepEqual(recipe, want) {
			t.Fatalf("detector received poisoned recipe on call %d: %+v", detects, recipe)
		}
		recipe.Steps[0].Packages[0] = "poison-detect"
		recipe.Detector.Values[0] = "poison-detect"
		return detects == 3, nil
	}
	deps.ExecuteRecipe = func(_ context.Context, recipe operation.InstallRecipe, _ pkg.PackageManager, _ pkg.ExecutableIdentity, _ func(string)) error {
		if !reflect.DeepEqual(recipe, want) {
			t.Fatalf("executor received poisoned recipe: %+v", recipe)
		}
		recipe.Steps[0].Packages[0] = "poison-execute"
		recipe.Detector.Values[0] = "poison-execute"
		return nil
	}
	if _, err := Apply(context.Background(), Request{RawTools: []string{"git"}, ExpectedHash: hash}, deps); err != nil {
		t.Fatal(err)
	}
	if got := accepted.Recipes()["git"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("accepted recipe mutated: %+v", got)
	}
}

func successfulApplyDependencies(t *testing.T, session installplan.FreshSession, manager pkg.PackageManager, journal JournalWriter, sequence *[]string) Dependencies {
	t.Helper()
	detectCalls := make(map[string]int)
	return Dependencies{
		PlanFresh:      func(context.Context, []string) (installplan.FreshSession, error) { return session, nil },
		UserHomeDir:    func() (string, error) { return t.TempDir(), nil },
		BootstrapState: operation.BootstrapStateNamespaceTracked,
		AcquireLock: func(*operation.StateAuthority, string, string) (func() error, error) {
			return func() error { return nil }, nil
		},
		OpenJournal: func(*operation.StateAuthority) (JournalWriter, error) { return journal, nil },
		StartRecord: operation.StartRecord,
		Now:         func() time.Time { return time.Now().UTC() },
		ExecuteRecipe: func(context.Context, operation.InstallRecipe, pkg.PackageManager, pkg.ExecutableIdentity, func(string)) error {
			return nil
		},
		DetectRecipe: func(recipe operation.InstallRecipe, gotManager pkg.PackageManager) (bool, error) {
			if gotManager != manager {
				t.Fatal("manager identity changed")
			}
			detectCalls[recipe.ToolID]++
			return detectCalls[recipe.ToolID] == 3, nil
		},
	}
}

func applyMultiFreshSession(t *testing.T) (installplan.FreshSession, pkg.PackageManager, string) {
	return applyMultiFreshSessionWithPresence(t, map[string]health.Presence{
		"git": health.PresenceMissing,
		"zsh": health.PresenceMissing,
	})
}

func applyMultiFreshSessionWithPresence(t *testing.T, presences map[string]health.Presence) (installplan.FreshSession, pkg.PackageManager, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	all := []tools.Tool{tools.NewGitTool(), tools.NewZshTool()}
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	managerIdentity, _ := applyManagerIdentity(t, "multi-brew", "exit 0")
	if err := manager.SetExecutableIdentity(managerIdentity); err != nil {
		t.Fatal(err)
	}
	observations := make([]health.InstallationObservation, 0, len(all))
	for _, tool := range all {
		recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: pkg.PlatformMacOS, Manager: "brew"})
		if err != nil {
			t.Fatal(err)
		}
		digest, err := operation.InstallRecipeDigest(recipe)
		if err != nil {
			t.Fatal(err)
		}
		packages := recipe.Steps[0].Packages
		state := health.PackageMissing
		missing, observed := packages, []string(nil)
		if presences[tool.ID()] == health.PresencePresent {
			state, missing, observed = health.PackagePresent, nil, packages
		}
		facet := health.PackageFacet{State: state, Provider: "brew", ExpectedReceipts: packages, MissingReceipts: missing, ObservedReceipts: observed, Authoritative: true, Complete: true,
			Namespaces: []health.PackageNamespaceFacet{{Namespace: health.PackageNamespaceFormula, State: state, ExpectedReceipts: packages, MissingReceipts: missing, ObservedReceipts: observed, Complete: true}}}
		observation, err := health.NewInstallationObservation(health.InstallationObservationSpec{ToolID: tool.ID(), Installability: health.InstallabilitySupported, InstallRecipeDigest: digest, Package: facet, Direct: health.DirectFacet{State: health.ComponentNotApplicable}})
		if err != nil {
			t.Fatal(err)
		}
		observations = append(observations, observation)
	}
	snapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{Generation: 1, Platform: "macos", Manager: "brew", Tools: observations})
	if err != nil {
		t.Fatal(err)
	}
	session, err := installplan.PlanFresh(context.Background(), []string{"zsh", "git", "git"}, installplan.FreshDependencies{
		Registry: func() []tools.Tool { return all }, DetectPlatform: func() pkg.Platform { return pkg.PlatformMacOS }, DetectManager: func() pkg.PackageManager { return manager },
		Collect: func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error) {
			return snapshot, nil
		},
		DescribeInstall: tools.DescribeInstall, CaptureStatePlan: operation.CaptureStatePlan, Now: func() time.Time { return time.Date(2026, 7, 12, 21, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, ok := session.Result().Accepted()
	if !ok {
		t.Fatal("multi fixture did not produce accepted plan")
	}
	return session, manager, accepted.Hash()
}
