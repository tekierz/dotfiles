package operation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tekierz/dotfiles/internal/safefile"
)

const CurrentJournalSchemaVersion = 1

var ErrInvalidRecord = errors.New("invalid operation journal record")

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type ActionStatus string

const (
	ActionPending   ActionStatus = "pending"
	ActionSucceeded ActionStatus = "succeeded"
	ActionFailed    ActionStatus = "failed"
	ActionSkipped   ActionStatus = "skipped"
)

// ActionResult intentionally stores no command output or config data. Summary
// is bounded and stripped of terminal/control characters before persistence.
type ActionResult struct {
	ActionID string       `json:"action_id"`
	Status   ActionStatus `json:"status"`
	Summary  string       `json:"summary,omitempty"`
}

type RollbackStatus string

const (
	RollbackSucceeded  RollbackStatus = "succeeded"
	RollbackIncomplete RollbackStatus = "incomplete"
	RollbackFailed     RollbackStatus = "failed"
)

// RollbackResult records the outcome without persisting config paths or data.
// Counts make a failed operation auditable while Summary remains sanitized.
type RollbackResult struct {
	Status   RollbackStatus `json:"status"`
	Restored int            `json:"restored"`
	Removed  int            `json:"removed"`
	Skipped  int            `json:"skipped"`
	Warnings int            `json:"warnings"`
	Summary  string         `json:"summary,omitempty"`
}

type Record struct {
	SchemaVersion int             `json:"schema_version"`
	OperationID   string          `json:"operation_id"`
	PlanHash      string          `json:"plan_hash"`
	StartedAt     time.Time       `json:"started_at"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	Status        Status          `json:"status"`
	Backup        string          `json:"backup,omitempty"`
	Rollback      *RollbackResult `json:"rollback,omitempty"`
	Actions       []ActionResult  `json:"actions"`
	Warnings      []string        `json:"warnings,omitempty"`
}

func (r *Record) SetRollback(result RollbackResult) error {
	result.Summary = sanitizeSummary(result.Summary)
	candidate := *r
	candidate.Rollback = &result
	if err := validateRecord(candidate); err != nil {
		return err
	}
	r.Rollback = &result
	return nil
}

func StartRecord(plan Plan, now time.Time) (Record, error) {
	if plan.Hash() == "" {
		return Record{}, fmt.Errorf("%w: plan has no hash", ErrInvalidRecord)
	}
	if now.IsZero() {
		return Record{}, fmt.Errorf("%w: start time is required", ErrInvalidRecord)
	}
	id, err := newOperationID(now)
	if err != nil {
		return Record{}, err
	}
	actions := plan.Actions()
	results := make([]ActionResult, 0, len(actions))
	for _, action := range actions {
		status := ActionPending
		if action.Disposition != DispositionApply {
			status = ActionSkipped
		}
		results = append(results, ActionResult{ActionID: action.ID, Status: status, Summary: sanitizeSummary(action.Reason)})
	}
	record := Record{
		SchemaVersion: CurrentJournalSchemaVersion,
		OperationID:   id,
		PlanHash:      plan.Hash(),
		StartedAt:     now.UTC(),
		Status:        StatusRunning,
		Actions:       results,
	}
	return record, validateRecord(record)
}

func newOperationID(now time.Time) (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate operation id: %w", err)
	}
	return now.UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random[:]), nil
}

func (r *Record) Finish(status Status, finishedAt time.Time, results []ActionResult, warnings []string) error {
	if status != StatusSucceeded && status != StatusFailed && status != StatusCancelled {
		return fmt.Errorf("%w: invalid terminal status %q", ErrInvalidRecord, status)
	}
	if finishedAt.IsZero() || finishedAt.Before(r.StartedAt) {
		return fmt.Errorf("%w: invalid finish time", ErrInvalidRecord)
	}
	r.Status = status
	finished := finishedAt.UTC()
	r.FinishedAt = &finished
	if results != nil {
		r.Actions = make([]ActionResult, len(results))
		for index, result := range results {
			result.Summary = sanitizeSummary(result.Summary)
			r.Actions[index] = result
		}
	}
	r.Warnings = make([]string, len(warnings))
	for index, warning := range warnings {
		r.Warnings[index] = sanitizeSummary(warning)
	}
	return validateRecord(*r)
}

func sanitizeSummary(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	const maxBytes = 2048
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "…"
}

func validateRecord(record Record) error {
	_, planHashErr := hex.DecodeString(record.PlanHash)
	if record.SchemaVersion != CurrentJournalSchemaVersion || record.OperationID == "" ||
		strings.ContainsAny(record.OperationID, "/\\\x00\r\n\t ") || len(record.PlanHash) != 64 || planHashErr != nil || record.StartedAt.IsZero() {
		return fmt.Errorf("%w: missing or malformed record identity", ErrInvalidRecord)
	}
	if record.Status != StatusRunning && record.Status != StatusSucceeded && record.Status != StatusFailed && record.Status != StatusCancelled {
		return fmt.Errorf("%w: invalid status %q", ErrInvalidRecord, record.Status)
	}
	if record.Status == StatusRunning && record.FinishedAt != nil {
		return fmt.Errorf("%w: running record has finish time", ErrInvalidRecord)
	}
	if record.Status != StatusRunning && record.FinishedAt == nil {
		return fmt.Errorf("%w: terminal record has no finish time", ErrInvalidRecord)
	}
	seen := make(map[string]struct{}, len(record.Actions))
	for _, result := range record.Actions {
		if result.ActionID == "" {
			return fmt.Errorf("%w: action result has no id", ErrInvalidRecord)
		}
		if _, ok := seen[result.ActionID]; ok {
			return fmt.Errorf("%w: duplicate action result %q", ErrInvalidRecord, result.ActionID)
		}
		seen[result.ActionID] = struct{}{}
		if result.Status != ActionPending && result.Status != ActionSucceeded && result.Status != ActionFailed && result.Status != ActionSkipped {
			return fmt.Errorf("%w: invalid action status %q", ErrInvalidRecord, result.Status)
		}
	}
	if record.Rollback != nil {
		rollback := record.Rollback
		if rollback.Status != RollbackSucceeded && rollback.Status != RollbackIncomplete && rollback.Status != RollbackFailed {
			return fmt.Errorf("%w: invalid rollback status %q", ErrInvalidRecord, rollback.Status)
		}
		if rollback.Restored < 0 || rollback.Removed < 0 || rollback.Skipped < 0 || rollback.Warnings < 0 {
			return fmt.Errorf("%w: rollback counts must be nonnegative", ErrInvalidRecord)
		}
	}
	return nil
}

// Journal persists one atomic JSON record per operation below a trusted state
// anchor. It never appends to a shared log file, avoiding torn interleaved
// records between CLI/TUI processes.
type Journal struct {
	root  string
	rel   string
	state *StateAuthority
}

func DefaultJournal() (Journal, error) {
	plan, err := CaptureStatePlan()
	if err != nil {
		return Journal{}, err
	}
	authority, err := BootstrapStateNamespaceTracked(plan)
	if err != nil {
		return Journal{}, err
	}
	return DefaultJournalWithAuthority(authority)
}

func DefaultJournalWithAuthority(authority *StateAuthority) (Journal, error) {
	if authority == nil {
		return Journal{}, fmt.Errorf("operation state authority is unavailable")
	}
	return Journal{root: authority.root, rel: filepath.ToSlash(filepath.Join(authority.stateRel, "operations")), state: authority}, nil
}

func stateAnchor() (root, rel string, err error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", "", fmt.Errorf("XDG_STATE_HOME must be absolute: %q", xdg)
		}
		xdg = filepath.Clean(xdg)
		if info, lstatErr := os.Lstat(xdg); lstatErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", "", fmt.Errorf("trusted XDG state root must be a real directory: %s", xdg)
			}
			return xdg, "dotfiles", nil
		} else if !errors.Is(lstatErr, os.ErrNotExist) {
			return "", "", fmt.Errorf("inspect trusted XDG state root: %w", lstatErr)
		}
		anchor := filepath.Dir(xdg)
		for {
			info, lstatErr := os.Lstat(anchor)
			if lstatErr == nil {
				if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
					return "", "", fmt.Errorf("XDG state ancestor must be a real directory: %s", anchor)
				}
				break
			}
			if !errors.Is(lstatErr, os.ErrNotExist) {
				return "", "", fmt.Errorf("inspect XDG state ancestor: %w", lstatErr)
			}
			parent := filepath.Dir(anchor)
			if parent == anchor {
				return "", "", fmt.Errorf("no existing XDG state ancestor for %s", xdg)
			}
			anchor = parent
		}
		xdgRel, relErr := filepath.Rel(anchor, xdg)
		if relErr != nil || filepath.IsAbs(xdgRel) || xdgRel == ".." || strings.HasPrefix(xdgRel, ".."+string(os.PathSeparator)) {
			return "", "", fmt.Errorf("resolve XDG state root below trusted ancestor")
		}
		return anchor, filepath.ToSlash(filepath.Join(xdgRel, "dotfiles")), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(home) {
		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdgConfig) {
			xdgConfig = filepath.Clean(xdgConfig)
			if info, statErr := os.Lstat(xdgConfig); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				return xdgConfig, ".dotfiles-state", nil
			}
		}
		return "", "", fmt.Errorf("determine home for state: %w", err)
	}
	return filepath.Clean(home), filepath.ToSlash(filepath.Join(".local", "state", "dotfiles")), nil
}

func (j Journal) Write(record Record) (returnErr error) {
	if err := validateRecord(record); err != nil {
		return err
	}
	if j.root == "" || j.rel == "" {
		return fmt.Errorf("%w: journal has no state anchor", ErrInvalidRecord)
	}
	if j.state == nil {
		return fmt.Errorf("operation journal has no bound state authority")
	}
	release, err := AcquireStateLockWithAuthority(j.state, "operation-journal", j.root)
	if err != nil {
		return fmt.Errorf("lock operation journal: %w", err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("unlock operation journal: %w", releaseErr))
		}
	}()

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal operation record: %w", err)
	}
	data = append(data, '\n')
	recordRel := filepath.ToSlash(filepath.Join(j.rel, record.OperationID+".json"))
	_, parents, err := stateChildDescendantAuthority(j.state, "operations", recordRel)
	if err != nil {
		return fmt.Errorf("bind operation record parent authority: %w", err)
	}
	_, revision, err := safefile.ReadWithinAuthorized(j.root, recordRel, parents)
	if err != nil {
		return fmt.Errorf("read operation record before write: %w", err)
	}
	if _, err := safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(j.root, recordRel, revision, parents, data, 0o600); err != nil {
		return fmt.Errorf("write operation record: %w", err)
	}
	return nil
}

func (j Journal) Read(operationID string) (Record, error) {
	if operationID == "" || strings.ContainsAny(operationID, "/\\\x00\r\n\t ") {
		return Record{}, fmt.Errorf("%w: invalid operation id", ErrInvalidRecord)
	}
	recordRel := filepath.ToSlash(filepath.Join(j.rel, operationID+".json"))
	if j.state == nil {
		return Record{}, fmt.Errorf("operation journal has no bound state authority")
	}
	_, parents, err := stateChildDescendantAuthority(j.state, "operations", recordRel)
	if err != nil {
		return Record{}, fmt.Errorf("bind operation record read authority: %w", err)
	}
	data, revision, err := safefile.ReadWithinAuthorized(j.root, recordRel, parents)
	if err != nil {
		return Record{}, fmt.Errorf("read operation record: %w", err)
	}
	if !revision.Exists() {
		return Record{}, os.ErrNotExist
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("parse operation record: %w", err)
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}
