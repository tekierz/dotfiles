package installapply

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
)

var (
	ErrInvalidApplyRequest = errors.New("invalid apply request")
	ErrPlanNotReady        = errors.New("fresh plan is not ready")
	ErrPlanHashMismatch    = errors.New("fresh plan hash does not match")
	ErrApplyFailed         = errors.New("installation apply failed")
)

const packageOnlyWarning = "package-only operation has no filesystem mutations; no filesystem rollback point was created"

type Request struct {
	RawTools     []string
	ExpectedHash string
}

type Result struct {
	OperationID string
	PlanHash    string
	Status      operation.Status
	Succeeded   int
	Failed      int
}

type JournalWriter interface {
	Write(operation.Record) error
}

type Dependencies struct {
	PlanFresh      func(context.Context, []string) (installplan.FreshSession, error)
	UserHomeDir    func() (string, error)
	BootstrapState func(*operation.StatePlan) (*operation.StateAuthority, error)
	AcquireLock    func(*operation.StateAuthority, string, string) (func() error, error)
	OpenJournal    func(*operation.StateAuthority) (JournalWriter, error)
	StartRecord    func(operation.Plan, time.Time) (operation.Record, error)
	Now            func() time.Time
	ExecuteRecipe  func(context.Context, operation.InstallRecipe, pkg.PackageManager, func(string)) error
	DetectRecipe   func(operation.InstallRecipe, pkg.PackageManager) (bool, error)
}

func SystemDependencies(planFresh func(context.Context, []string) (installplan.FreshSession, error)) Dependencies {
	return Dependencies{
		PlanFresh: planFresh, UserHomeDir: defaultUserHomeDir,
		BootstrapState: operation.BootstrapStateNamespaceTracked,
		AcquireLock:    operation.AcquireStateLockWithAuthority,
		OpenJournal: func(authority *operation.StateAuthority) (JournalWriter, error) {
			journal, err := operation.DefaultJournalWithAuthority(authority)
			return journal, err
		},
		StartRecord: operation.StartRecord, Now: time.Now,
		ExecuteRecipe: ExecuteRecipe, DetectRecipe: DetectRecipe,
	}
}

var defaultUserHomeDir = func() (string, error) {
	return os.UserHomeDir()
}

func Apply(ctx context.Context, request Request, dependencies Dependencies) (Result, error) {
	if ctx == nil || !validApplyHash(request.ExpectedHash) || explicitApplyIntentEmpty(request.RawTools) || !validApplyDependencies(dependencies) {
		return Result{}, ErrInvalidApplyRequest
	}
	if err := ctx.Err(); err != nil {
		return Result{}, errors.Join(ErrApplyFailed, err)
	}
	session, err := dependencies.PlanFresh(ctx, slices.Clone(request.RawTools))
	if err != nil {
		return Result{}, errors.Join(ErrApplyFailed, err)
	}
	if session.Result().Public().Status() != planpublic.StatusReady {
		return Result{}, ErrPlanNotReady
	}
	accepted, ok := session.Result().Accepted()
	if !ok {
		return Result{}, ErrPlanNotReady
	}
	if accepted.Hash() != request.ExpectedHash {
		return Result{}, ErrPlanHashMismatch
	}
	manager := session.Manager()
	actions, recipes, detectorAuthority, err := validatePackageOnlyAuthority(accepted, manager)
	if err != nil {
		return Result{}, errors.Join(ErrApplyFailed, err)
	}
	// Detector drift must block before state bootstrap, locking, journaling, or
	// product mutation. Compare against the exact boolean observed in the plan.
	for _, action := range actions {
		recipe := operation.CloneInstallRecipe(recipes[action.ToolID])
		detected, detectErr := dependencies.DetectRecipe(recipe, manager)
		if detectErr != nil || detected != detectorAuthority[action.ToolID] {
			return Result{}, ErrPlanNotReady
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, errors.Join(ErrApplyFailed, err)
	}
	home, err := dependencies.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return Result{}, errors.Join(ErrApplyFailed, fmt.Errorf("absolute HOME is unavailable"))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, errors.Join(ErrApplyFailed, err)
	}
	state, err := dependencies.BootstrapState(accepted.StatePlan())
	if err != nil || state == nil {
		return Result{}, errors.Join(ErrApplyFailed, err)
	}
	release, err := dependencies.AcquireLock(state, "install-operation", home)
	if err != nil || release == nil {
		return Result{}, errors.Join(ErrApplyFailed, err)
	}
	startedAt := dependencies.Now()
	record, err := dependencies.StartRecord(accepted.Operation(), startedAt)
	if err != nil {
		return Result{}, errors.Join(ErrApplyFailed, release(), err)
	}
	record.PlanHash = accepted.Hash()
	journal, err := dependencies.OpenJournal(state)
	if err != nil || journal == nil {
		return Result{}, errors.Join(ErrApplyFailed, release(), err)
	}
	if err := journal.Write(record); err != nil {
		return Result{}, errors.Join(ErrApplyFailed, release(), err)
	}
	results := slices.Clone(record.Actions)
	cancelled := false
	mark := func(actionID string, status operation.ActionStatus, summary string) {
		for index := range results {
			if results[index].ActionID == actionID {
				results[index].Status, results[index].Summary = status, summary
				return
			}
		}
	}
	var failures []error
	for _, action := range actions {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			cancelled = true
			break
		}
		recipe := operation.CloneInstallRecipe(recipes[action.ToolID])
		detected, detectErr := dependencies.DetectRecipe(operation.CloneInstallRecipe(recipe), manager)
		if detectErr != nil || detected != detectorAuthority[action.ToolID] {
			if detectErr == nil {
				detectErr = fmt.Errorf("reviewed install detector changed")
			}
			failures = append(failures, fmt.Errorf("%s: %w", action.ToolID, detectErr))
			mark(action.ID, operation.ActionFailed, "detector changed before install")
			break
		}
		executeErr := dependencies.ExecuteRecipe(ctx, operation.CloneInstallRecipe(recipe), manager, nil)
		if executeErr == nil {
			var detected bool
			detected, executeErr = dependencies.DetectRecipe(operation.CloneInstallRecipe(recipe), manager)
			if executeErr == nil && !detected {
				executeErr = fmt.Errorf("install postcondition failed")
			}
		}
		if executeErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", action.ToolID, executeErr))
			mark(action.ID, operation.ActionFailed, "install or postcondition failed")
			if errors.Is(executeErr, context.Canceled) || errors.Is(executeErr, context.DeadlineExceeded) || ctx.Err() != nil {
				cancelled = true
				break
			}
			continue
		}
		mark(action.ID, operation.ActionSucceeded, "installed and detected")
	}
	for index := range results {
		if results[index].Status == operation.ActionPending {
			results[index].Status = operation.ActionSkipped
			results[index].Summary = "not completed before operation ended"
		}
	}
	// Release verification is part of the operation result. The journal takes
	// its own serialization lock, so release the operation lock first and make
	// any release failure visible in the terminal record.
	if releaseErr := release(); releaseErr != nil {
		failures = append(failures, fmt.Errorf("release install operation lock: %w", releaseErr))
	}
	status := operation.StatusSucceeded
	if cancelled || errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status = operation.StatusCancelled
	} else if len(failures) != 0 {
		status = operation.StatusFailed
	}
	finishedAt := dependencies.Now()
	if err := record.Finish(status, finishedAt, results, []string{packageOnlyWarning}); err != nil {
		failures = append(failures, err)
		status = operation.StatusFailed
	} else if err := journal.Write(record); err != nil {
		failures = append(failures, err)
		status = operation.StatusFailed
	}
	succeeded, failed := 0, 0
	for _, result := range results {
		switch result.Status {
		case operation.ActionSucceeded:
			succeeded++
		case operation.ActionFailed:
			failed++
		case operation.ActionPending, operation.ActionSkipped:
			// Pending and skipped actions are intentionally excluded from the
			// terminal success/failure totals.
		}
	}
	result := Result{OperationID: record.OperationID, PlanHash: accepted.Hash(), Status: status, Succeeded: succeeded, Failed: failed}
	if status != operation.StatusSucceeded || len(failures) != 0 {
		return result, errors.Join(append([]error{ErrApplyFailed}, failures...)...)
	}
	return result, nil
}

func validApplyHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}

func explicitApplyIntentEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func validApplyDependencies(value Dependencies) bool {
	return value.PlanFresh != nil && value.UserHomeDir != nil && value.BootstrapState != nil && value.AcquireLock != nil && value.OpenJournal != nil &&
		value.StartRecord != nil && value.Now != nil && value.ExecuteRecipe != nil && value.DetectRecipe != nil
}

func validatePackageOnlyAuthority(accepted installplan.AcceptedPlan, manager pkg.PackageManager) ([]operation.Action, map[string]operation.InstallRecipe, map[string]bool, error) {
	if packageManagerNil(manager) || accepted.StatePlan() == nil {
		return nil, nil, nil, fmt.Errorf("accepted package authority is incomplete")
	}
	snapshot := accepted.SnapshotAuthority()
	if snapshot.SchemaVersion != health.CurrentInstallationSchemaVersion || snapshot.Generation == 0 || snapshot.Platform == "" ||
		snapshot.Manager == "" || !validApplyHash(snapshot.Digest) || manager.Name() != snapshot.Manager {
		return nil, nil, nil, fmt.Errorf("accepted package manager changed")
	}
	if _, err := operation.StatePlanAuthorityDigest(accepted.StatePlan()); err != nil {
		return nil, nil, nil, fmt.Errorf("accepted state authority is invalid")
	}
	actions, recipes := accepted.Operation().Actions(), accepted.Recipes()
	intent := accepted.Intent()
	authorities := accepted.ToolAuthorities()
	if len(actions) == 0 || len(recipes) != len(actions) || len(authorities) != len(intent.Tools) {
		return nil, nil, nil, fmt.Errorf("accepted package action coverage is incomplete")
	}
	seen := make(map[string]struct{}, len(actions))
	detected := make(map[string]bool, len(actions))
	for _, action := range actions {
		if action.Kind != operation.KindInstallTool || action.Disposition != operation.DispositionApply || action.Ownership != operation.OwnershipPackageManager ||
			action.Reversibility != operation.ReversibilityManual || action.ToolID == "" || action.Target != action.ToolID || action.ID != "install:"+action.ToolID ||
			action.InstallRecipe == nil || action.InstallDetected == nil || action.BackupTarget != "" || len(action.BackupTargets) != 0 || len(action.TargetOwnership) != 0 {
			return nil, nil, nil, fmt.Errorf("accepted action %s is not package-only", action.ID)
		}
		if _, duplicate := seen[action.ToolID]; duplicate {
			return nil, nil, nil, fmt.Errorf("duplicate accepted tool %s", action.ToolID)
		}
		seen[action.ToolID] = struct{}{}
		recipe, ok := recipes[action.ToolID]
		authority, authorityOK := accepted.ToolAuthority(action.ToolID)
		digest, digestErr := operation.InstallRecipeDigest(recipe)
		actionDigest, actionDigestErr := operation.InstallRecipeDigest(*action.InstallRecipe)
		intentCoherent := authority.Intent == "install" && authority.Presence == health.PresenceMissing && !*action.InstallDetected ||
			authority.Intent == "repair" && authority.Presence == health.PresencePartial
		if !ok || !authorityOK || !slices.Contains(intent.Tools, action.ToolID) || digestErr != nil || actionDigestErr != nil || recipe.ToolID != action.ToolID ||
			recipe.Platform != snapshot.Platform || recipe.Manager != snapshot.Manager || digest != actionDigest || digest != authority.RecipeDigest || digest != action.DesiredDigest || !intentCoherent {
			return nil, nil, nil, fmt.Errorf("accepted recipe authority for %s is inconsistent", action.ToolID)
		}
		detected[action.ToolID] = *action.InstallDetected
	}
	for _, toolID := range intent.Tools {
		authority, ok := authorities[toolID]
		_, hasAction := seen[toolID]
		_, hasRecipe := recipes[toolID]
		switch authority.Presence {
		case health.PresencePresent:
			if !ok || authority.Intent != "none" || authority.RecipeDigest != "" || hasAction || hasRecipe {
				return nil, nil, nil, fmt.Errorf("accepted present authority for %s is inconsistent", toolID)
			}
		case health.PresenceMissing:
			if !ok || authority.Intent != "install" || !hasAction || !hasRecipe || detected[toolID] {
				return nil, nil, nil, fmt.Errorf("accepted install authority for %s is inconsistent", toolID)
			}
		case health.PresencePartial:
			if !ok || authority.Intent != "repair" || !hasAction || !hasRecipe {
				return nil, nil, nil, fmt.Errorf("accepted repair authority for %s is inconsistent", toolID)
			}
		case health.PresenceUnknown:
			return nil, nil, nil, fmt.Errorf("accepted tool authority for %s is inconsistent", toolID)
		default:
			return nil, nil, nil, fmt.Errorf("accepted tool authority for %s is inconsistent", toolID)
		}
	}
	return actions, recipes, detected, nil
}
