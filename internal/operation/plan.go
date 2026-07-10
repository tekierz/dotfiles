// Package operation defines immutable, serializable plans and non-secret
// execution records shared by the CLI and TUI.
package operation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const CurrentPlanSchemaVersion = 1

var ErrInvalidPlan = errors.New("invalid operation plan")

type Kind string

const (
	KindInstallTool Kind = "install_tool"
	KindWriteConfig Kind = "write_config"
	KindInstallFile Kind = "install_file"
	KindUpdateState Kind = "update_state"
)

type Disposition string

const (
	DispositionApply   Disposition = "apply"
	DispositionSkip    Disposition = "skip"
	DispositionBlocked Disposition = "blocked"
)

type Ownership string

const (
	OwnershipUnknown         Ownership = "unknown"
	OwnershipPackageManager  Ownership = "package_manager"
	OwnershipManagedFile     Ownership = "managed_file"
	OwnershipManagedFragment Ownership = "managed_fragment"
	OwnershipUser            Ownership = "user"
)

type Reversibility string

const (
	ReversibilityBackup     Reversibility = "backup_restore"
	ReversibilityBestEffort Reversibility = "best_effort"
	ReversibilityManual     Reversibility = "manual"
)

// Observation is the planner's typed view of a target before mutation. Digest
// is a content hash, never raw config data; Source identifies which precedence
// path was observed.
type Observation struct {
	Exists  bool   `json:"exists"`
	Source  string `json:"source,omitempty"`
	Digest  string `json:"digest,omitempty"`
	Managed bool   `json:"managed"`
}

// Action is one immutable unit shown in preview and consumed by execution.
// Existing file mutations must carry a durable backup target and proven
// ownership. Blocked actions remain in the plan so omissions are visible.
type Action struct {
	ID            string        `json:"id"`
	Kind          Kind          `json:"kind"`
	ToolID        string        `json:"tool_id,omitempty"`
	Target        string        `json:"target"`
	Description   string        `json:"description"`
	Disposition   Disposition   `json:"disposition"`
	Reason        string        `json:"reason,omitempty"`
	Ownership     Ownership     `json:"ownership"`
	Reversibility Reversibility `json:"reversibility"`
	BackupTarget  string        `json:"backup_target,omitempty"`
	Observation   Observation   `json:"observation"`
}

type planDocument struct {
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	Actions       []Action  `json:"actions"`
}

// Plan keeps actions private so callers cannot mutate the preview after it has
// been accepted. Actions returns a defensive copy. Hash identifies the exact
// canonical document consumed by execution and the operation journal.
type Plan struct {
	document planDocument
	hash     string
}

func NewPlan(createdAt time.Time, actions []Action) (Plan, error) {
	if createdAt.IsZero() {
		return Plan{}, fmt.Errorf("%w: created_at is required", ErrInvalidPlan)
	}
	doc := planDocument{
		SchemaVersion: CurrentPlanSchemaVersion,
		CreatedAt:     createdAt.UTC(),
		Actions:       slices.Clone(actions),
	}
	if err := validateDocument(doc); err != nil {
		return Plan{}, err
	}
	canonical, err := json.Marshal(doc)
	if err != nil {
		return Plan{}, fmt.Errorf("marshal canonical plan: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return Plan{document: doc, hash: hex.EncodeToString(digest[:])}, nil
}

func validateDocument(doc planDocument) error {
	if doc.SchemaVersion != CurrentPlanSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrInvalidPlan, doc.SchemaVersion)
	}
	seen := make(map[string]struct{}, len(doc.Actions))
	for index, action := range doc.Actions {
		prefix := fmt.Sprintf("action %d", index)
		if action.ID == "" || strings.ContainsAny(action.ID, "\x00\r\n\t ") {
			return fmt.Errorf("%w: %s has invalid id %q", ErrInvalidPlan, prefix, action.ID)
		}
		if _, ok := seen[action.ID]; ok {
			return fmt.Errorf("%w: duplicate action id %q", ErrInvalidPlan, action.ID)
		}
		seen[action.ID] = struct{}{}
		if !validKind(action.Kind) || !validDisposition(action.Disposition) ||
			!validOwnership(action.Ownership) || !validReversibility(action.Reversibility) {
			return fmt.Errorf("%w: %s has invalid enum value", ErrInvalidPlan, prefix)
		}
		if action.Target == "" || strings.ContainsRune(action.Target, '\x00') || action.Description == "" {
			return fmt.Errorf("%w: %s requires target and description", ErrInvalidPlan, prefix)
		}
		if action.Disposition != DispositionApply && action.Reason == "" {
			return fmt.Errorf("%w: %s must explain %s disposition", ErrInvalidPlan, prefix, action.Disposition)
		}
		if action.Disposition == DispositionApply && action.Kind == KindWriteConfig {
			if action.Ownership != OwnershipManagedFile && action.Ownership != OwnershipManagedFragment {
				return fmt.Errorf("%w: %s cannot apply config without managed ownership", ErrInvalidPlan, prefix)
			}
			if action.Observation.Exists && action.BackupTarget == "" {
				return fmt.Errorf("%w: %s mutates an existing config without a backup target", ErrInvalidPlan, prefix)
			}
		}
	}
	return nil
}

func validKind(value Kind) bool {
	return value == KindInstallTool || value == KindWriteConfig || value == KindInstallFile || value == KindUpdateState
}

func validDisposition(value Disposition) bool {
	return value == DispositionApply || value == DispositionSkip || value == DispositionBlocked
}

func validOwnership(value Ownership) bool {
	return value == OwnershipUnknown || value == OwnershipPackageManager || value == OwnershipManagedFile ||
		value == OwnershipManagedFragment || value == OwnershipUser
}

func validReversibility(value Reversibility) bool {
	return value == ReversibilityBackup || value == ReversibilityBestEffort || value == ReversibilityManual
}

func (p Plan) SchemaVersion() int   { return p.document.SchemaVersion }
func (p Plan) CreatedAt() time.Time { return p.document.CreatedAt }
func (p Plan) Hash() string         { return p.hash }
func (p Plan) Actions() []Action    { return slices.Clone(p.document.Actions) }

func (p Plan) HasBlocked() bool {
	for _, action := range p.document.Actions {
		if action.Disposition == DispositionBlocked {
			return true
		}
	}
	return false
}

func (p Plan) BackupTargets() []string {
	set := make(map[string]struct{})
	for _, action := range p.document.Actions {
		if action.Disposition == DispositionApply && action.BackupTarget != "" {
			set[filepath.Clean(action.BackupTarget)] = struct{}{}
		}
	}
	targets := make([]string, 0, len(set))
	for target := range set {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

func (p Plan) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		planDocument
		Hash string `json:"hash"`
	}{planDocument: p.document, Hash: p.hash})
}
