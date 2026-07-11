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
	"unicode"
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

const CurrentInstallRecipeSchemaVersion = 1

type InstallStepKind string

const (
	InstallStepPackageManager InstallStepKind = "package_manager"
	InstallStepNPMGlobal      InstallStepKind = "npm_global"
)

type InstallDetectorKind string

const (
	InstallDetectorBinary         InstallDetectorKind = "binary"
	InstallDetectorAppBundle      InstallDetectorKind = "app_bundle"
	InstallDetectorPackageReceipt InstallDetectorKind = "package_receipt"
)

// InstallStep intentionally has no generic shell-command or remote-script
// variant. Mutable vendor scripts require a separate verified-artifact design.
type InstallStep struct {
	Kind     InstallStepKind `json:"kind"`
	Provider string          `json:"provider"`
	Packages []string        `json:"packages,omitempty"`
	Args     []string        `json:"args,omitempty"`
}

type InstallDetector struct {
	Kind   InstallDetectorKind `json:"kind"`
	Values []string            `json:"values"`
}

// InstallRecipe is the complete, display-safe provenance and execution input
// for an install action. It contains no environment values, tokens, or secrets.
type InstallRecipe struct {
	SchemaVersion  int             `json:"schema_version"`
	ToolID         string          `json:"tool_id"`
	Platform       string          `json:"platform"`
	Manager        string          `json:"manager,omitempty"`
	Steps          []InstallStep   `json:"steps"`
	Detector       InstallDetector `json:"detector"`
	Authentication string          `json:"authentication,omitempty"`
	Risk           string          `json:"risk"`
}

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
	ID              string         `json:"id"`
	Kind            Kind           `json:"kind"`
	ToolID          string         `json:"tool_id,omitempty"`
	Target          string         `json:"target"`
	Description     string         `json:"description"`
	Disposition     Disposition    `json:"disposition"`
	Reason          string         `json:"reason,omitempty"`
	DesiredDigest   string         `json:"desired_digest"`
	Ownership       Ownership      `json:"ownership"`
	Reversibility   Reversibility  `json:"reversibility"`
	BackupTarget    string         `json:"backup_target,omitempty"`
	BackupTargets   []string       `json:"backup_targets,omitempty"`
	Observation     Observation    `json:"observation"`
	Observations    []Observation  `json:"observations,omitempty"`
	InstallRecipe   *InstallRecipe `json:"install_recipe,omitempty"`
	InstallDetected *bool          `json:"install_detected,omitempty"`
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
		Actions:       cloneActions(actions),
	}
	if err := validateDocument(doc); err != nil {
		return Plan{}, err
	}
	// created_at is metadata, not desired/observed state. Excluding it keeps the
	// same actions and host observations deterministically addressable across a
	// preview refresh; operation IDs carry execution-time uniqueness.
	canonical, err := json.Marshal(struct {
		SchemaVersion int      `json:"schema_version"`
		Actions       []Action `json:"actions"`
	}{SchemaVersion: doc.SchemaVersion, Actions: doc.Actions})
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
		if !validDigest(action.DesiredDigest) {
			return fmt.Errorf("%w: %s requires a canonical desired digest", ErrInvalidPlan, prefix)
		}
		if action.Disposition != DispositionApply && action.Reason == "" {
			return fmt.Errorf("%w: %s must explain %s disposition", ErrInvalidPlan, prefix, action.Disposition)
		}
		if action.Disposition == DispositionApply && action.Kind == KindWriteConfig {
			if action.Ownership != OwnershipManagedFile && action.Ownership != OwnershipManagedFragment {
				return fmt.Errorf("%w: %s cannot apply config without managed ownership", ErrInvalidPlan, prefix)
			}
			if action.Observation.Exists && action.BackupTarget == "" && len(action.BackupTargets) == 0 {
				return fmt.Errorf("%w: %s mutates an existing config without a backup target", ErrInvalidPlan, prefix)
			}
		}
		if action.Kind == KindInstallTool {
			if action.InstallRecipe == nil || action.InstallDetected == nil {
				return fmt.Errorf("%w: %s requires an install recipe and detector observation", ErrInvalidPlan, prefix)
			}
			if err := validateInstallRecipe(*action.InstallRecipe); err != nil {
				return fmt.Errorf("%w: %s has invalid install recipe: %w", ErrInvalidPlan, prefix, err)
			}
			if action.InstallRecipe.ToolID != action.ToolID || action.Target != action.ToolID {
				return fmt.Errorf("%w: %s install identity does not match its recipe", ErrInvalidPlan, prefix)
			}
			digest, err := InstallRecipeDigest(*action.InstallRecipe)
			if err != nil || digest != action.DesiredDigest {
				return fmt.Errorf("%w: %s desired digest does not match its install recipe", ErrInvalidPlan, prefix)
			}
		} else if action.InstallRecipe != nil || action.InstallDetected != nil {
			return fmt.Errorf("%w: %s non-install action carries install authority", ErrInvalidPlan, prefix)
		}
		if action.Disposition == DispositionApply && (action.Kind == KindWriteConfig || action.Kind == KindInstallFile || action.Kind == KindUpdateState) {
			if len(action.Observations) == 0 {
				return fmt.Errorf("%w: %s requires per-target observations", ErrInvalidPlan, prefix)
			}
		}
		backupSet := make(map[string]struct{}, len(action.BackupTargets)+1)
		for _, target := range append([]string{action.BackupTarget}, action.BackupTargets...) {
			if target == "" {
				continue
			}
			if !validRelativeTarget(target) {
				return fmt.Errorf("%w: %s has unsafe backup target %q", ErrInvalidPlan, prefix, target)
			}
			clean := filepath.Clean(target)
			if _, duplicate := backupSet[clean]; duplicate {
				return fmt.Errorf("%w: %s has duplicate backup target %q", ErrInvalidPlan, prefix, target)
			}
			backupSet[clean] = struct{}{}
		}
		observationSet := make(map[string]struct{}, len(action.Observations))
		for _, observation := range action.Observations {
			if !validRelativeTarget(observation.Source) {
				return fmt.Errorf("%w: %s has unsafe observation source %q", ErrInvalidPlan, prefix, observation.Source)
			}
			if observation.Exists && !validDigest(observation.Digest) {
				return fmt.Errorf("%w: %s existing observation %q requires a content digest", ErrInvalidPlan, prefix, observation.Source)
			}
			if !observation.Exists && observation.Digest != "" {
				return fmt.Errorf("%w: %s absent observation %q cannot carry a digest", ErrInvalidPlan, prefix, observation.Source)
			}
			clean := filepath.Clean(observation.Source)
			if _, duplicate := observationSet[clean]; duplicate {
				return fmt.Errorf("%w: %s has duplicate observation source %q", ErrInvalidPlan, prefix, observation.Source)
			}
			observationSet[clean] = struct{}{}
		}
		if action.Disposition == DispositionApply {
			for target := range backupSet {
				if _, observed := observationSet[target]; !observed {
					return fmt.Errorf("%w: %s backup target %q lacks an observation", ErrInvalidPlan, prefix, target)
				}
			}
		}
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateInstallRecipe(recipe InstallRecipe) error {
	if recipe.SchemaVersion != CurrentInstallRecipeSchemaVersion || recipe.ToolID == "" || recipe.Platform == "" || recipe.Risk == "" {
		return fmt.Errorf("schema, tool, platform, and risk are required")
	}
	if len(recipe.Detector.Values) == 0 {
		return fmt.Errorf("detector values are required")
	}
	for _, value := range append([]string{recipe.ToolID, recipe.Platform, recipe.Manager, recipe.Authentication, recipe.Risk}, recipe.Detector.Values...) {
		if hasUnsafeDisplayControl(value) {
			return fmt.Errorf("control characters are not allowed")
		}
	}
	if recipe.Detector.Kind != InstallDetectorBinary && recipe.Detector.Kind != InstallDetectorAppBundle && recipe.Detector.Kind != InstallDetectorPackageReceipt {
		return fmt.Errorf("unsupported detector %q", recipe.Detector.Kind)
	}
	if len(recipe.Steps) == 0 {
		return fmt.Errorf("at least one install step is required")
	}
	for _, step := range recipe.Steps {
		if step.Provider == "" || hasUnsafeDisplayControl(step.Provider) {
			return fmt.Errorf("step provider is invalid")
		}
		if step.Kind != InstallStepPackageManager && step.Kind != InstallStepNPMGlobal {
			return fmt.Errorf("unsupported install step %q", step.Kind)
		}
		if step.Kind == InstallStepPackageManager && (recipe.Manager == "" || step.Provider != recipe.Manager || len(step.Packages) == 0 || len(step.Args) != 0) {
			return fmt.Errorf("package-manager step does not match the accepted manager")
		}
		if step.Kind == InstallStepNPMGlobal && (step.Provider != "npm" || len(step.Args) == 0 || len(step.Packages) != 0) {
			return fmt.Errorf("npm step requires exact npm arguments")
		}
		for _, value := range append(slices.Clone(step.Packages), step.Args...) {
			if value == "" || hasUnsafeDisplayControl(value) {
				return fmt.Errorf("step argument is invalid")
			}
		}
	}
	if recipe.Detector.Kind == InstallDetectorPackageReceipt {
		installed := make(map[string]struct{})
		for _, step := range recipe.Steps {
			if step.Kind == InstallStepPackageManager {
				for _, name := range step.Packages {
					installed[name] = struct{}{}
				}
			}
		}
		for _, value := range recipe.Detector.Values {
			if _, ok := installed[value]; !ok {
				return fmt.Errorf("package detector %q is not installed by this recipe", value)
			}
		}
	}
	return nil
}

func hasUnsafeDisplayControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return true
		}
	}
	return false
}

func CloneInstallRecipe(recipe InstallRecipe) InstallRecipe {
	recipe.Detector.Values = slices.Clone(recipe.Detector.Values)
	recipe.Steps = slices.Clone(recipe.Steps)
	for index := range recipe.Steps {
		recipe.Steps[index].Packages = slices.Clone(recipe.Steps[index].Packages)
		recipe.Steps[index].Args = slices.Clone(recipe.Steps[index].Args)
	}
	return recipe
}

func InstallRecipeDigest(recipe InstallRecipe) (string, error) {
	if err := validateInstallRecipe(recipe); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(recipe)
	if err != nil {
		return "", fmt.Errorf("marshal install recipe: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func validRelativeTarget(target string) bool {
	if target == "" || filepath.IsAbs(target) || strings.ContainsAny(target, "\x00\r\n") {
		return false
	}
	clean := filepath.Clean(target)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
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
func (p Plan) Actions() []Action    { return cloneActions(p.document.Actions) }

func cloneActions(actions []Action) []Action {
	cloned := slices.Clone(actions)
	for index := range cloned {
		cloned[index].BackupTargets = slices.Clone(actions[index].BackupTargets)
		cloned[index].Observations = slices.Clone(actions[index].Observations)
		if actions[index].InstallRecipe != nil {
			recipe := CloneInstallRecipe(*actions[index].InstallRecipe)
			cloned[index].InstallRecipe = &recipe
		}
		if actions[index].InstallDetected != nil {
			detected := *actions[index].InstallDetected
			cloned[index].InstallDetected = &detected
		}
	}
	return cloned
}

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
		if action.Disposition != DispositionApply {
			continue
		}
		if action.BackupTarget != "" {
			set[filepath.Clean(action.BackupTarget)] = struct{}{}
		}
		for _, target := range action.BackupTargets {
			if target != "" {
				set[filepath.Clean(target)] = struct{}{}
			}
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
