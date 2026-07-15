package planpublic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode"
)

const (
	legacyPlanSchemaVersion = 1
	phasedPlanSchemaVersion = 2
	installSchemaVersion    = 1
	planKind                = "dotfiles.plan"
	maxPublicActions        = 256
	maxRecipeItems          = 128
	applyHashRequired       = "hash_required"
	applyNotAvailable       = "not_available"
)

var ErrInvalidDocument = errors.New("invalid public plan document")

type Status string

const (
	StatusReady          Status = "ready"
	StatusNoChanges      Status = "no_changes"
	StatusBlocked        Status = "blocked"
	StatusIntentRequired Status = "intent_required"
	StatusReplanRequired Status = "replan_required"
)

// PhaseSpec is the public, non-executable description of one independently
// confirmed install phase. RemainingIntent is empty only for the final npm
// phase; Next is either complete or replan_required.
type PhaseSpec struct {
	Kind            string `json:"kind"`
	Index           int    `json:"index"`
	Authority       string `json:"authority"`
	RemainingIntent Intent `json:"remaining_intent"`
	Next            string `json:"next"`
}

type Snapshot struct {
	SchemaVersion int    `json:"schema_version"`
	Generation    uint64 `json:"generation"`
	PublicDigest  string `json:"public_digest"`
}

type Capabilities struct {
	Installation string `json:"installation"`
	Config       string `json:"config"`
	Service      string `json:"service"`
	Auth         string `json:"auth"`
	Apply        string `json:"apply"`
}

type Summary struct {
	Apply         int `json:"apply"`
	Skip          int `json:"skip"`
	Blocked       int `json:"blocked"`
	BackupTargets int `json:"backup_targets"`
}

type ObservationSpec struct {
	Exists  bool `json:"exists"`
	Managed bool `json:"managed"`
}

type DetectorSpec struct {
	Kind   string   `json:"kind"`
	Values []string `json:"values"`
}

type InstallStepSpec struct {
	Kind      string   `json:"kind"`
	Provider  string   `json:"provider"`
	Packages  []string `json:"packages"`
	Casks     []string `json:"casks"`
	Arguments []string `json:"arguments"`
}

type InstallSpec struct {
	SchemaVersion  int               `json:"schema_version"`
	Platform       string            `json:"platform"`
	Manager        string            `json:"manager"`
	Steps          []InstallStepSpec `json:"steps"`
	Detector       DetectorSpec      `json:"detector"`
	Authentication string            `json:"authentication"`
	Risk           string            `json:"risk"`
	RecipeDigest   string            `json:"recipe_digest"`
}

type ActionSpec struct {
	ActionID      string           `json:"action_id"`
	Kind          string           `json:"kind"`
	ToolID        string           `json:"tool_id"`
	Description   string           `json:"description"`
	Disposition   string           `json:"disposition"`
	ReasonCode    string           `json:"reason_code"`
	Reason        string           `json:"reason"`
	Ownership     string           `json:"ownership"`
	Reversibility string           `json:"reversibility"`
	Observation   *ObservationSpec `json:"observation,omitempty"`
	Install       *InstallSpec     `json:"install,omitempty"`
}

type DocumentSpec struct {
	Status       Status
	PlanHash     string
	Platform     string
	Manager      string
	Intent       Intent
	Snapshot     *Snapshot
	Capabilities Capabilities
	Summary      Summary
	Actions      []ActionSpec
	Phase        *PhaseSpec
}

// Document is a defensive, already-redacted public projection. It contains no
// executable planning authority.
type Document struct {
	schemaVersion int
	status        Status
	planHash      string
	platform      string
	manager       string
	intent        Intent
	snapshot      *Snapshot
	capabilities  Capabilities
	summary       Summary
	actions       []ActionSpec
	phase         *PhaseSpec
	publicDigest  string
}

type publicAuthority struct {
	PlanHash     string `json:"plan_hash,omitempty"`
	PublicDigest string `json:"public_digest"`
}

type publicAction struct {
	Ordinal       int              `json:"ordinal"`
	ActionID      string           `json:"action_id"`
	Kind          string           `json:"kind"`
	ToolID        string           `json:"tool_id"`
	Description   string           `json:"description"`
	Disposition   string           `json:"disposition"`
	ReasonCode    string           `json:"reason_code"`
	Reason        string           `json:"reason"`
	Ownership     string           `json:"ownership"`
	Reversibility string           `json:"reversibility"`
	Observation   *ObservationSpec `json:"observation,omitempty"`
	Install       *InstallSpec     `json:"install,omitempty"`
}

type publicDocument struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          string          `json:"kind"`
	Status        Status          `json:"status"`
	Platform      string          `json:"platform,omitempty"`
	Manager       string          `json:"manager,omitempty"`
	Intent        Intent          `json:"intent"`
	Snapshot      *Snapshot       `json:"snapshot,omitempty"`
	Authority     publicAuthority `json:"authority"`
	Capabilities  Capabilities    `json:"capabilities"`
	Summary       Summary         `json:"summary"`
	Actions       []publicAction  `json:"actions"`
	Phase         *PhaseSpec      `json:"phase,omitempty"`
}

func NewDocument(spec DocumentSpec) (Document, error) {
	if !validStatus(spec.Status) || !validDocumentIntent(spec.Status, spec.Intent) {
		return Document{}, ErrInvalidDocument
	}
	if !validPlanHash(spec.Status, spec.PlanHash) || !validStatusShape(spec) || !validSnapshot(spec.Snapshot) || !validCapabilities(spec.Status, spec.Capabilities) || !validSummary(spec.Summary, spec.Actions) || !validActions(spec) {
		return Document{}, ErrInvalidDocument
	}

	doc := Document{
		schemaVersion: documentSchemaVersion(spec.Phase),
		status:        spec.Status,
		planHash:      spec.PlanHash,
		platform:      spec.Platform,
		manager:       spec.Manager,
		intent:        cloneIntent(spec.Intent),
		snapshot:      cloneSnapshot(spec.Snapshot),
		capabilities:  spec.Capabilities,
		summary:       spec.Summary,
		actions:       clonePublicActions(spec.Actions),
		phase:         clonePhase(spec.Phase),
	}
	if doc.intent.Tools == nil {
		doc.intent.Tools = []string{}
	}
	digest, err := digestPublicDocument(doc.publicDocument(""))
	if err != nil {
		return Document{}, ErrInvalidDocument
	}
	doc.publicDigest = digest
	return doc, nil
}

func (d Document) Actions() []ActionSpec {
	return cloneActions(d.actions)
}

// Phase returns a defensive public phase description when this is a v2 phased
// plan. A nil phase identifies the existing v1 non-npm projection.
func (d Document) Phase() *PhaseSpec { return clonePhase(d.phase) }

// Status returns the validated public outcome represented by the document.
// The zero value returns the invalid empty status.
func (d Document) Status() Status {
	return d.status
}

func MarshalDocument(document Document) ([]byte, error) {
	if !validStatus(document.status) || document.publicDigest == "" {
		return nil, ErrInvalidDocument
	}
	encoded, err := json.Marshal(document.publicDocument(document.publicDigest))
	if err != nil {
		return nil, ErrInvalidDocument
	}
	return append(encoded, '\n'), nil
}

func (d Document) publicDocument(digest string) publicDocument {
	actions := make([]publicAction, len(d.actions))
	for index, action := range d.actions {
		actions[index] = publicAction{
			Ordinal: index, ActionID: action.ActionID, Kind: action.Kind, ToolID: action.ToolID,
			Description: action.Description, Disposition: action.Disposition, ReasonCode: action.ReasonCode,
			Reason: action.Reason, Ownership: action.Ownership, Reversibility: action.Reversibility,
			Observation: cloneObservation(action.Observation), Install: cloneInstall(action.Install),
		}
	}
	return publicDocument{
		SchemaVersion: d.schemaVersion, Kind: planKind, Status: d.status,
		Platform: d.platform, Manager: d.manager, Intent: cloneIntent(d.intent),
		Snapshot: cloneSnapshot(d.snapshot), Authority: publicAuthority{PlanHash: d.planHash, PublicDigest: digest},
		Capabilities: d.capabilities, Summary: d.summary, Actions: actions, Phase: clonePhase(d.phase),
	}
}

func digestPublicDocument(document publicDocument) (string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var canonical map[string]any
	if err := decoder.Decode(&canonical); err != nil {
		return "", err
	}
	authority, ok := canonical["authority"].(map[string]any)
	if !ok {
		return "", ErrInvalidDocument
	}
	delete(authority, "public_digest")
	encoded, err = json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusReady, StatusNoChanges, StatusBlocked, StatusIntentRequired, StatusReplanRequired:
		return true
	default:
		return false
	}
}

func validPlanHash(status Status, planHash string) bool {
	if status == StatusReady {
		return validSHA256(planHash)
	}
	return planHash == ""
}

func validDocumentIntent(status Status, intent Intent) bool {
	if status == StatusIntentRequired {
		return intent.Source == "explicit_tools" && len(intent.Tools) == 0 && intent.Digest == ""
	}
	if len(intent.Tools) == 0 {
		return status == StatusBlocked && intent.Source == "explicit_tools" && intent.Digest == ""
	}
	normalized, err := NormalizeExplicitTools(intent.Tools, intent.Tools)
	return err == nil && intent.Source == normalized.Source && intent.Digest == normalized.Digest && slices.Equal(intent.Tools, normalized.Tools)
}

func validStatusShape(spec DocumentSpec) bool {
	switch spec.Status {
	case StatusIntentRequired:
		return spec.Platform == "" && spec.Manager == "" && spec.Snapshot == nil && len(spec.Actions) == 0 && spec.Summary == (Summary{}) && spec.Capabilities == (Capabilities{})
	case StatusReady, StatusNoChanges, StatusBlocked:
		return validStructuralAtom(spec.Platform) && validStructuralAtom(spec.Manager) && spec.Snapshot != nil
	case StatusReplanRequired:
		return validStructuralAtom(spec.Platform) && validStructuralAtom(spec.Manager) && spec.Snapshot != nil && len(spec.Actions) == 0 && spec.Summary == (Summary{})
	default:
		return false
	}
}

func validSnapshot(snapshot *Snapshot) bool {
	if snapshot == nil {
		return true
	}
	return snapshot.SchemaVersion == installSchemaVersion && validSHA256(snapshot.PublicDigest)
}

func validCapabilities(status Status, capabilities Capabilities) bool {
	if status == StatusIntentRequired {
		return capabilities == (Capabilities{})
	}
	if capabilities.Installation != "planned" || capabilities.Config != "not_planned" ||
		capabilities.Service != "not_collected" || capabilities.Auth != "not_collected" {
		return false
	}
	if status == StatusReady {
		return capabilities.Apply == applyHashRequired
	}
	return capabilities.Apply == applyNotAvailable
}

func validSummary(summary Summary, actions []ActionSpec) bool {
	if summary.Apply < 0 || summary.Skip < 0 || summary.Blocked < 0 || summary.BackupTargets < 0 {
		return false
	}
	apply, skip, blocked := 0, 0, 0
	for _, action := range actions {
		switch action.Disposition {
		case "apply":
			apply++
		case "skip":
			skip++
		case "blocked":
			blocked++
		default:
			return false
		}
	}
	return summary.Apply == apply && summary.Skip == skip && summary.Blocked == blocked
}

func validActions(spec DocumentSpec) bool {
	if len(spec.Actions) > maxPublicActions {
		return false
	}
	if spec.Status == StatusIntentRequired {
		return len(spec.Actions) == 0
	}
	if spec.Status == StatusBlocked && (len(spec.Actions) == 0 || spec.Summary.Blocked != len(spec.Actions)) {
		return false
	}
	if spec.Status == StatusReady && (spec.Summary.Apply == 0 || spec.Summary.Blocked != 0) {
		return false
	}
	if spec.Status == StatusNoChanges && (spec.Summary.Apply != 0 || spec.Summary.Blocked != 0) {
		return false
	}
	if !validPhase(spec.Status, spec.Intent, spec.Phase) {
		return false
	}
	seen := make(map[string]struct{}, len(spec.Actions))
	for _, action := range spec.Actions {
		if !validAction(action, spec.Platform, spec.Manager) {
			return false
		}
		if action.Disposition == "apply" && spec.Phase != nil && !installMatchesPhase(action.Install, spec.Phase.Authority) {
			return false
		}
		if _, duplicate := seen[action.ActionID]; duplicate {
			return false
		}
		seen[action.ActionID] = struct{}{}
		if spec.Status == StatusBlocked && action.Disposition != "blocked" {
			return false
		}
	}
	return true
}

func installMatchesPhase(install *InstallSpec, authority string) bool {
	if install == nil {
		return false
	}
	for _, step := range install.Steps {
		isNPM := step.Kind == "npm_global"
		if (authority == "npm") != isNPM {
			return false
		}
	}
	return authority == "npm" || authority == "package_manager"
}

func documentSchemaVersion(phase *PhaseSpec) int {
	if phase != nil {
		return phasedPlanSchemaVersion
	}
	return legacyPlanSchemaVersion
}

func validPhase(status Status, requested Intent, phase *PhaseSpec) bool {
	if phase == nil {
		return status != StatusReplanRequired
	}
	if status == StatusIntentRequired || phase.Index < 1 || phase.Index > 2 {
		return false
	}
	remaining := phase.RemainingIntent
	if remaining.Source != "explicit_tools" || remaining.Tools == nil {
		return false
	}
	if len(remaining.Tools) == 0 {
		if remaining.Digest != "" {
			return false
		}
	} else {
		normalized, err := NormalizeExplicitTools(remaining.Tools, requested.Tools)
		if err != nil || normalized.Digest != remaining.Digest || !slices.Equal(normalized.Tools, remaining.Tools) {
			return false
		}
	}
	switch phase.Kind {
	case "prerequisite":
		return phase.Index == 1 && phase.Authority == "package_manager" && len(remaining.Tools) > 0 && phase.Next == "replan_required"
	case "npm":
		return phase.Index == 2 && phase.Authority == "npm" && len(remaining.Tools) == 0 && phase.Next == "complete" && status != StatusReplanRequired
	default:
		return false
	}
}

func validAction(action ActionSpec, platform, manager string) bool {
	if !validActionID(action.ActionID) || !validToolID(action.ToolID) || action.Kind != "install_tool" {
		return false
	}
	if action.Disposition != "apply" && action.Disposition != "skip" && action.Disposition != "blocked" {
		return false
	}
	if action.Ownership != "package_manager" || action.Reversibility != "external" {
		return false
	}
	if action.Description != "install "+action.ToolID || !validReason(action.Disposition, action.ReasonCode, action.Reason) {
		return false
	}
	if action.Disposition == "apply" && action.Install == nil {
		return false
	}
	if action.Disposition != "apply" && action.Install != nil {
		return false
	}
	if action.Install != nil && !validInstall(action.Install, platform, manager) {
		return false
	}
	return true
}

func validReason(disposition, code, reason string) bool {
	if disposition == "apply" {
		return code == "" && reason == ""
	}
	messages := map[string]string{
		"present":              "already present",
		"unsupported":          "installation unsupported",
		"unknown":              "installation status unknown",
		"stale":                "installation evidence stale",
		"environment_mismatch": "installation environment changed",
		"recipe_drift":         "installation recipe changed",
		"ownership_conflict":   "configuration ownership conflict",
		"phase_boundary":       "installation requires a fresh phase coordinator",
	}
	want, ok := messages[code]
	return ok && reason == want
}

func validInstall(install *InstallSpec, platform, manager string) bool {
	if install.SchemaVersion != installSchemaVersion || install.Platform != platform || install.Manager != manager || !validSHA256(install.RecipeDigest) || len(install.Steps) == 0 || len(install.Steps) > 16 {
		return false
	}
	if !validAuthentication(install.Authentication) || !validRisk(install.Risk) {
		return false
	}
	for _, step := range install.Steps {
		if step.Kind != "package_manager" && step.Kind != "npm_global" && step.Kind != "homebrew_cask" {
			return false
		}
		if !validStructuralAtom(step.Provider) || step.Packages == nil || step.Casks == nil || step.Arguments == nil || len(step.Packages) > maxRecipeItems || len(step.Casks) > maxRecipeItems || len(step.Arguments) > maxRecipeItems {
			return false
		}
		switch step.Kind {
		case "package_manager":
			if step.Provider != install.Manager || len(step.Packages) == 0 || len(step.Casks) != 0 || len(step.Arguments) != 0 {
				return false
			}
		case "npm_global":
			if step.Provider != "npm" || len(step.Packages) != 0 || len(step.Casks) != 0 || !validNPMGlobalArguments(step.Arguments) {
				return false
			}
		case "homebrew_cask":
			if install.Platform != "macos" || install.Manager != "brew" || step.Provider != "brew" ||
				len(step.Casks) == 0 || len(step.Packages) != 0 || len(step.Arguments) != 0 {
				return false
			}
		}
		for _, value := range append(slices.Clone(step.Packages), step.Casks...) {
			if !validPackageToken(value) {
				return false
			}
		}
		for _, value := range step.Arguments {
			if !validRecipeToken(value) {
				return false
			}
		}
	}
	if len(install.Detector.Values) == 0 || len(install.Detector.Values) > 32 {
		return false
	}
	switch install.Detector.Kind {
	case "binary", "package_receipt", "app_bundle":
	default:
		return false
	}
	for _, value := range install.Detector.Values {
		if !validDetectorValue(install.Detector.Kind, value) {
			return false
		}
	}
	return true
}

func validNPMGlobalArguments(arguments []string) bool {
	if len(arguments) != 3 && len(arguments) != 4 {
		return false
	}
	if arguments[0] != "install" || arguments[1] != "-g" {
		return false
	}
	packageIndex := 2
	if len(arguments) == 4 {
		if arguments[2] != "--ignore-scripts" {
			return false
		}
		packageIndex = 3
	}
	return validPackageToken(arguments[packageIndex])
}

func validAuthentication(value string) bool {
	switch value {
	case "none", "interactive_provider_login", "chatgpt_or_openai_api_key", "provider_login_or_api_key", "existing_app_auth":
		return true
	default:
		return false
	}
}

func validRisk(value string) bool {
	switch value {
	case "package_manager_install", "npm_lifecycle_code", "npm_scripts_disabled_runtime_code", "package_manager_current_release_unpinned", "homebrew_cask_unpinned":
		return true
	default:
		return false
	}
}

func validDetectorValue(kind, value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	if kind == "app_bundle" {
		if !strings.HasSuffix(value, ".app") {
			return false
		}
		for _, character := range value {
			if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune(" ._+-", character) {
				continue
			}
			return false
		}
		return true
	}
	return validPackageToken(value)
}

func validPackageToken(value string) bool {
	return validRecipeToken(value) && !strings.HasPrefix(value, "-")
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validActionID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && !strings.ContainsRune("-._:", character) {
			return false
		}
	}
	return true
}

func validStructuralAtom(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func validRecipeToken(value string) bool {
	if value == "" || len(value) > 256 || unsafeRecipeToken(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") || strings.HasPrefix(value, ".") || strings.Contains(value, "\\") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		for _, character := range segment {
			if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("@._+-=", character) {
				continue
			}
			return false
		}
	}
	return true
}

func unsafeRecipeToken(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"authorization", "bearer ", "sk-proj", "api_key", "api-key", "token=", "password=", "secret=", "credential=", "file:"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, character := range value {
		if unicode.IsControl(character) || isBidiControl(character) {
			return true
		}
	}
	return false
}

func cloneIntent(intent Intent) Intent {
	intent.Tools = append([]string(nil), intent.Tools...)
	if intent.Tools == nil {
		intent.Tools = []string{}
	}
	return intent
}

func cloneSnapshot(snapshot *Snapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	copy := *snapshot
	return &copy
}

func clonePhase(phase *PhaseSpec) *PhaseSpec {
	if phase == nil {
		return nil
	}
	cloned := *phase
	cloned.RemainingIntent = cloneIntent(phase.RemainingIntent)
	return &cloned
}

func cloneActions(actions []ActionSpec) []ActionSpec {
	result := make([]ActionSpec, len(actions))
	for index, action := range actions {
		result[index] = action
		result[index].Observation = cloneObservation(action.Observation)
		result[index].Install = cloneInstall(action.Install)
	}
	return result
}

func clonePublicActions(actions []ActionSpec) []ActionSpec {
	result := cloneActions(actions)
	if result == nil {
		return []ActionSpec{}
	}
	return result
}

func cloneObservation(observation *ObservationSpec) *ObservationSpec {
	if observation == nil {
		return nil
	}
	copy := *observation
	return &copy
}

func cloneInstall(install *InstallSpec) *InstallSpec {
	if install == nil {
		return nil
	}
	copy := *install
	copy.Steps = make([]InstallStepSpec, len(install.Steps))
	for index, step := range install.Steps {
		copy.Steps[index] = step
		copy.Steps[index].Packages = append([]string(nil), step.Packages...)
		copy.Steps[index].Casks = append([]string(nil), step.Casks...)
		copy.Steps[index].Arguments = append([]string(nil), step.Arguments...)
		if copy.Steps[index].Packages == nil {
			copy.Steps[index].Packages = []string{}
		}
		if copy.Steps[index].Casks == nil {
			copy.Steps[index].Casks = []string{}
		}
		if copy.Steps[index].Arguments == nil {
			copy.Steps[index].Arguments = []string{}
		}
	}
	if copy.Steps == nil {
		copy.Steps = []InstallStepSpec{}
	}
	copy.Detector.Values = append([]string(nil), install.Detector.Values...)
	if copy.Detector.Values == nil {
		copy.Detector.Values = []string{}
	}
	return &copy
}

func isBidiControl(value rune) bool {
	return value == '\u061c' || value == '\u200e' || value == '\u200f' ||
		(value >= '\u202a' && value <= '\u202e') || (value >= '\u2066' && value <= '\u2069')
}
