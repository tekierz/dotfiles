// Package installplan builds explicit, install-only plans without depending on
// terminal UI state or rediscovering the caller's environment.
package installplan

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"maps"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/tools"
)

var (
	ErrInvalidRequest                   = errors.New("invalid install plan request")
	ErrNPMPhaseBoundaryRequired         = errors.New("npm execution requires a fresh phase boundary")
	ErrNPMExecutionAuthorityUnavailable = errors.New("npm execution authority is unavailable")
)

type Environment struct {
	Platform           pkg.Platform
	Manager            string
	ManagerIdentity    pkg.ExecutableIdentity
	NPMIdentity        pkg.NPMExecutionIdentity
	ExpectedGeneration uint64
}

type Request struct {
	Intent      planpublic.Intent
	Snapshot    health.InstallationSnapshot
	Environment Environment
}

type Dependencies struct {
	LookupTool       func(string) (tools.Tool, bool)
	DescribeInstall  func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error)
	CaptureStatePlan func() (*operation.StatePlan, error)
	Now              func() time.Time
}

type SnapshotAuthority struct {
	SchemaVersion int
	Generation    uint64
	Platform      string
	Manager       string
	Digest        string
}

type ToolAuthority struct {
	Presence     health.Presence
	Intent       string
	RecipeDigest string
}

type AcceptedPlan struct {
	document        operation.Plan
	statePlan       *operation.StatePlan
	snapshot        SnapshotAuthority
	tools           map[string]ToolAuthority
	recipes         map[string]operation.InstallRecipe
	intent          planpublic.Intent
	managerIdentity pkg.ExecutableIdentity
	npmIdentity     pkg.NPMExecutionIdentity
	hash            string
}

func (plan AcceptedPlan) Operation() operation.Plan            { return plan.document }
func (plan AcceptedPlan) StatePlan() *operation.StatePlan      { return plan.statePlan }
func (plan AcceptedPlan) SnapshotAuthority() SnapshotAuthority { return plan.snapshot }
func (plan AcceptedPlan) Hash() string                         { return plan.hash }

func (plan AcceptedPlan) ManagerExecutableIdentity() (pkg.ExecutableIdentity, bool) {
	if !validManagerExecutableIdentity(plan.managerIdentity) {
		return pkg.ExecutableIdentity{}, false
	}
	return plan.managerIdentity, true
}

func (plan AcceptedPlan) NPMExecutionIdentity() (pkg.NPMExecutionIdentity, bool) {
	if !validNPMExecutionIdentity(plan.npmIdentity) {
		return pkg.NPMExecutionIdentity{}, false
	}
	return plan.npmIdentity, true
}

func (plan AcceptedPlan) Intent() planpublic.Intent {
	return cloneIntent(plan.intent)
}

func (plan AcceptedPlan) ToolAuthority(id string) (ToolAuthority, bool) {
	authority, ok := plan.tools[id]
	return authority, ok
}

func (plan AcceptedPlan) ToolAuthorities() map[string]ToolAuthority {
	return maps.Clone(plan.tools)
}

func (plan AcceptedPlan) Recipes() map[string]operation.InstallRecipe {
	result := make(map[string]operation.InstallRecipe, len(plan.recipes))
	for id, recipe := range plan.recipes {
		result[id] = operation.CloneInstallRecipe(recipe)
	}
	return result
}

type Result struct {
	public      planpublic.Document
	accepted    AcceptedPlan
	hasAccepted bool
}

func (result Result) Public() planpublic.Document { return result.public }

func (result Result) Accepted() (AcceptedPlan, bool) {
	if !result.hasAccepted {
		return AcceptedPlan{}, false
	}
	return cloneAccepted(result.accepted), true
}

func Build(request Request, dependencies Dependencies) (Result, error) {
	intent, intentRequired, err := validateIntent(request.Intent)
	if err != nil {
		return Result{}, err
	}
	if intentRequired {
		document, documentErr := planpublic.NewDocument(planpublic.DocumentSpec{
			Status: planpublic.StatusIntentRequired,
			Intent: planpublic.Intent{Source: "explicit_tools", Tools: []string{}},
		})
		if documentErr != nil {
			return Result{}, ErrInvalidRequest
		}
		return Result{public: document}, nil
	}
	if dependencies.LookupTool == nil || dependencies.DescribeInstall == nil || dependencies.CaptureStatePlan == nil || dependencies.Now == nil {
		return Result{}, ErrInvalidRequest
	}
	if !validRequestSnapshot(request.Snapshot, request.Environment) {
		return Result{}, ErrInvalidRequest
	}
	if request.Snapshot.Generation() != request.Environment.ExpectedGeneration {
		return blockedResult(intent, request.Snapshot, "stale")
	}
	if request.Snapshot.Platform() != string(request.Environment.Platform) || request.Snapshot.Manager() != request.Environment.Manager {
		return blockedResult(intent, request.Snapshot, "environment_mismatch")
	}

	publicActions := make([]planpublic.ActionSpec, 0, len(intent.Tools))
	mutationRequired := false
	toolAuthorities := make(map[string]ToolAuthority, len(intent.Tools))
	for _, id := range intent.Tools {
		observation, observed := request.Snapshot.Tool(id)
		if !observed {
			tool, found := dependencies.LookupTool(id)
			if !found || toolIsNil(tool) {
				return blockedResult(intent, request.Snapshot, "unknown")
			}
			if tool.ID() != id {
				return blockedResult(intent, request.Snapshot, "recipe_drift")
			}
			return blockedResult(intent, request.Snapshot, "stale")
		}
		if observation.Presence() == health.PresenceUnknown || observation.Installability() == health.InstallabilityUnknown {
			return blockedResult(intent, request.Snapshot, "unknown")
		}
		if observation.Presence() != health.PresencePresent && observation.Installability() == health.InstallabilityUnsupported {
			return blockedResult(intent, request.Snapshot, "unsupported")
		}
		if observation.Presence() == health.PresencePresent {
			publicActions = append(publicActions, publicDecisionAction(id, "skip", "present", publicObservation(observation, true)))
			toolAuthorities[id] = ToolAuthority{Presence: health.PresencePresent, Intent: "none"}
			continue
		}
		if observation.Presence() != health.PresenceMissing && observation.Presence() != health.PresencePartial {
			return blockedResult(intent, request.Snapshot, "unknown")
		}
		mutationRequired = true
	}
	if !mutationRequired {
		document, err := planpublic.NewDocument(planpublic.DocumentSpec{
			Status: planpublic.StatusNoChanges, Platform: request.Snapshot.Platform(), Manager: request.Snapshot.Manager(), Intent: intent,
			Snapshot: publicSnapshot(request.Snapshot, intent.Tools), Capabilities: plannedCapabilities(planpublic.StatusNoChanges),
			Summary: planpublic.Summary{Skip: len(publicActions)}, Actions: publicActions,
		})
		if err != nil {
			return Result{}, ErrInvalidRequest
		}
		return Result{public: document}, nil
	}
	validatedRecipes := make(map[string]operation.InstallRecipe)
	detectorObservations := make(map[string]bool)
	publicInstalls := make(map[string]*planpublic.InstallSpec)
	for _, id := range intent.Tools {
		observation, _ := request.Snapshot.Tool(id)
		if observation.Presence() == health.PresencePresent {
			continue
		}
		tool, found := dependencies.LookupTool(id)
		if !found || toolIsNil(tool) || tool.ID() != id {
			return blockedResult(intent, request.Snapshot, "recipe_drift")
		}
		recipe, err := dependencies.DescribeInstall(tool, tools.InstallEnvironment{Platform: request.Environment.Platform, Manager: request.Environment.Manager})
		if err != nil {
			return Result{}, err
		}
		digest, err := operation.InstallRecipeDigest(recipe)
		if err != nil || recipe.ToolID != id || recipe.Platform != string(request.Environment.Platform) || recipe.Manager != request.Environment.Manager || digest != observation.InstallRecipeDigest() {
			return blockedResult(intent, request.Snapshot, "recipe_drift")
		}
		detected, known := ObservedInstallDetector(observation, recipe.Detector)
		if !known {
			return blockedResult(intent, request.Snapshot, "unknown")
		}
		validatedRecipes[id] = operation.CloneInstallRecipe(recipe)
		detectorObservations[id] = detected
		publicInstall, publicErr := projectInstallRecipe(recipe, digest)
		if publicErr != nil {
			return Result{}, publicErr
		}
		publicInstalls[id] = publicInstall
		installIntent := "install"
		if observation.Presence() == health.PresencePartial {
			installIntent = "repair"
		}
		toolAuthorities[id] = ToolAuthority{Presence: observation.Presence(), Intent: installIntent, RecipeDigest: digest}
	}

	phase := classifyNPMExecutionPhase(validatedRecipes)
	if phase == npmExecutionPhaseMixed {
		return Result{}, errors.Join(ErrInvalidRequest, ErrNPMPhaseBoundaryRequired)
	}
	managerIdentity := pkg.ExecutableIdentity{}
	npmIdentity := pkg.NPMExecutionIdentity{}
	if phase == npmExecutionPhasePure {
		if !validNPMExecutionIdentity(request.Environment.NPMIdentity) {
			return Result{}, errors.Join(ErrInvalidRequest, ErrNPMExecutionAuthorityUnavailable)
		}
		npmIdentity = request.Environment.NPMIdentity
	} else if recipesRequireManagerIdentity(validatedRecipes) {
		if !validManagerExecutableIdentity(request.Environment.ManagerIdentity) {
			return Result{}, ErrInvalidRequest
		}
		managerIdentity = request.Environment.ManagerIdentity
	}
	statePlan, err := dependencies.CaptureStatePlan()
	if err != nil || statePlan == nil {
		return Result{}, errors.Join(ErrInvalidRequest, err)
	}
	if _, err := operation.StatePlanAuthorityDigest(statePlan); err != nil {
		return Result{}, ErrInvalidRequest
	}
	now := dependencies.Now()
	if now.IsZero() {
		return Result{}, ErrInvalidRequest
	}
	privateActions := make([]operation.Action, 0, len(validatedRecipes))
	publicActions = make([]planpublic.ActionSpec, 0, len(intent.Tools))
	for _, id := range intent.Tools {
		authority := toolAuthorities[id]
		if authority.Intent == "none" {
			observation, _ := request.Snapshot.Tool(id)
			publicActions = append(publicActions, publicDecisionAction(id, "skip", "present", publicObservation(observation, true)))
			continue
		}
		recipe := operation.CloneInstallRecipe(validatedRecipes[id])
		detected := detectorObservations[id]
		privateActions = append(privateActions, operation.Action{
			ID: "install:" + id, Kind: operation.KindInstallTool, ToolID: id, Target: id,
			Description: authority.Intent + " " + id, Disposition: operation.DispositionApply,
			DesiredDigest: authority.RecipeDigest, Ownership: operation.OwnershipPackageManager,
			Reversibility: operation.ReversibilityManual, InstallRecipe: &recipe, InstallDetected: &detected,
		})
		publicActions = append(publicActions, planpublic.ActionSpec{
			ActionID: "install:" + id, Kind: "install_tool", ToolID: id, Description: "install " + id,
			Disposition: "apply", Ownership: "package_manager", Reversibility: "external",
			Observation: publicObservationForTool(request.Snapshot, id), Install: clonePublicInstall(publicInstalls[id]),
		})
	}
	document, err := operation.NewPlan(now, privateActions)
	if err != nil {
		return Result{}, err
	}
	snapshotAuthority := SnapshotAuthority{
		SchemaVersion: request.Snapshot.SchemaVersion(), Generation: request.Snapshot.Generation(),
		Platform: request.Snapshot.Platform(), Manager: request.Snapshot.Manager(), Digest: request.Snapshot.Digest(),
	}
	accepted := AcceptedPlan{
		document: document, statePlan: statePlan, snapshot: snapshotAuthority,
		tools: maps.Clone(toolAuthorities), recipes: cloneRecipes(validatedRecipes), intent: cloneIntent(intent),
		managerIdentity: managerIdentity, npmIdentity: npmIdentity,
	}
	accepted.hash, err = acceptedPlanHash(accepted)
	if err != nil {
		return Result{}, ErrInvalidRequest
	}
	publicDocument, err := planpublic.NewDocument(planpublic.DocumentSpec{
		Status: planpublic.StatusReady, PlanHash: accepted.hash, Platform: request.Snapshot.Platform(), Manager: request.Snapshot.Manager(), Intent: intent,
		Snapshot: publicSnapshot(request.Snapshot, intent.Tools), Capabilities: plannedCapabilities(planpublic.StatusReady),
		Summary: planpublic.Summary{Apply: len(privateActions), Skip: len(intent.Tools) - len(privateActions)}, Actions: publicActions,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{public: publicDocument, accepted: accepted, hasAccepted: true}, nil
}

// ObservedInstallDetector derives the exact reviewed detector result from one
// generation-bound health observation without performing any live probes.
func ObservedInstallDetector(observation health.InstallationObservation, detector operation.InstallDetector) (bool, bool) {
	contains := func(values []string, target string) bool {
		return slices.Contains(values, target)
	}
	switch detector.Kind {
	case operation.InstallDetectorPackageReceipt:
		facet := observation.Package()
		if !facet.Authoritative || !facet.Complete {
			return false, false
		}
		for _, value := range detector.Values {
			if contains(facet.ObservedReceipts, value) {
				continue
			}
			if contains(facet.MissingReceipts, value) {
				return false, true
			}
			return false, false
		}
		return true, true
	case operation.InstallDetectorBinary, operation.InstallDetectorAppBundle:
		facet := observation.Direct()
		if !facet.Authoritative {
			return false, false
		}
		kind := health.DirectSourceBinary
		if detector.Kind == operation.InstallDetectorAppBundle {
			kind = health.DirectSourceAppBundle
		}
		unknown := false
		for _, value := range detector.Values {
			matched := false
			for _, alternative := range facet.Alternatives {
				if alternative.Kind != kind || !contains(alternative.Identifiers, value) {
					continue
				}
				matched = true
				switch alternative.State {
				case health.ComponentPresent:
				case health.ComponentMissing:
					return false, true
				case health.ComponentUnknown, health.ComponentNotApplicable:
					unknown = true
				default:
					unknown = true
				}
			}
			if !matched {
				unknown = true
			}
		}
		if unknown {
			return false, false
		}
		return true, true
	default:
		return false, false
	}
}

func validRequestSnapshot(snapshot health.InstallationSnapshot, environment Environment) bool {
	return snapshot.SchemaVersion() == health.CurrentInstallationSchemaVersion && snapshot.Generation() > 0 &&
		snapshot.Digest() != "" && environment.Platform != "" && environment.Manager != "" && environment.ExpectedGeneration > 0
}

func validManagerExecutableIdentity(identity pkg.ExecutableIdentity) bool {
	return identity.SchemaVersion() == pkg.CurrentExecutableIdentitySchemaVersion && validAcceptedAuthorityDigest(identity.Digest())
}

func validNPMExecutionIdentity(identity pkg.NPMExecutionIdentity) bool {
	return identity.SchemaVersion() == pkg.CurrentNPMExecutionIdentitySchemaVersion && validAcceptedAuthorityDigest(identity.Digest())
}

type npmExecutionPhase uint8

const (
	npmExecutionPhaseNone npmExecutionPhase = iota
	npmExecutionPhasePure
	npmExecutionPhaseMixed
)

func classifyNPMExecutionPhase(recipes map[string]operation.InstallRecipe) npmExecutionPhase {
	hasNPM, hasManager := false, false
	for _, recipe := range recipes {
		for _, step := range recipe.Steps {
			switch step.Kind {
			case operation.InstallStepNPMGlobal:
				hasNPM = true
			case operation.InstallStepPackageManager, operation.InstallStepHomebrewCask:
				hasManager = true
			}
		}
	}
	if hasNPM && hasManager {
		return npmExecutionPhaseMixed
	}
	if hasNPM {
		return npmExecutionPhasePure
	}
	return npmExecutionPhaseNone
}

func recipesRequireManagerIdentity(recipes map[string]operation.InstallRecipe) bool {
	for _, recipe := range recipes {
		for _, step := range recipe.Steps {
			if step.Kind == operation.InstallStepPackageManager || step.Kind == operation.InstallStepHomebrewCask {
				return true
			}
		}
	}
	return false
}

func validAcceptedAuthorityDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == value
}

func blockedResult(intent planpublic.Intent, snapshot health.InstallationSnapshot, code string) (Result, error) {
	actions := make([]planpublic.ActionSpec, len(intent.Tools))
	for index, id := range intent.Tools {
		actions[index] = publicDecisionAction(id, "blocked", code, publicObservationForTool(snapshot, id))
	}
	document, err := planpublic.NewDocument(planpublic.DocumentSpec{
		Status: planpublic.StatusBlocked, Platform: snapshot.Platform(), Manager: snapshot.Manager(), Intent: cloneIntent(intent),
		Snapshot: publicSnapshot(snapshot, intent.Tools), Capabilities: plannedCapabilities(planpublic.StatusBlocked),
		Summary: planpublic.Summary{Blocked: len(actions)}, Actions: actions,
	})
	if err != nil {
		return Result{}, ErrInvalidRequest
	}
	return Result{public: document}, nil
}

func publicDecisionAction(id, disposition, code string, observation *planpublic.ObservationSpec) planpublic.ActionSpec {
	reasons := map[string]string{
		"present": "already present", "unsupported": "installation unsupported", "unknown": "installation status unknown",
		"stale": "installation evidence stale", "environment_mismatch": "installation environment changed", "recipe_drift": "installation recipe changed",
	}
	return planpublic.ActionSpec{
		ActionID: "install:" + id, Kind: "install_tool", ToolID: id, Description: "install " + id,
		Disposition: disposition, ReasonCode: code, Reason: reasons[code], Ownership: "package_manager", Reversibility: "external",
		Observation: observation,
	}
}

func publicObservationForTool(snapshot health.InstallationSnapshot, id string) *planpublic.ObservationSpec {
	observation, observed := snapshot.Tool(id)
	return publicObservation(observation, observed)
}

func publicObservation(observation health.InstallationObservation, observed bool) *planpublic.ObservationSpec {
	exists := false
	if observed {
		exists = observation.Presence() == health.PresencePresent || observation.Presence() == health.PresencePartial
	}
	return &planpublic.ObservationSpec{Exists: exists, Managed: false}
}

func toolIsNil(tool tools.Tool) bool {
	if tool == nil {
		return true
	}
	value := reflect.ValueOf(tool)
	//nolint:exhaustive // Only nil-capable reflect kinds may be passed to IsNil.
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func plannedCapabilities(status planpublic.Status) planpublic.Capabilities {
	apply := "not_available"
	if status == planpublic.StatusReady {
		apply = "hash_required"
	}
	return planpublic.Capabilities{Installation: "planned", Config: "not_planned", Service: "not_collected", Auth: "not_collected", Apply: apply}
}

func publicSnapshot(snapshot health.InstallationSnapshot, selected []string) *planpublic.Snapshot {
	type publicTool struct {
		ToolID         string                `json:"tool_id"`
		Observed       bool                  `json:"observed"`
		Presence       health.Presence       `json:"presence"`
		Installability health.Installability `json:"installability"`
		RecipeDigest   string                `json:"recipe_digest"`
	}
	projection := struct {
		SchemaVersion int          `json:"schema_version"`
		Generation    uint64       `json:"generation"`
		Platform      string       `json:"platform"`
		Manager       string       `json:"manager"`
		Tools         []publicTool `json:"tools"`
	}{SchemaVersion: snapshot.SchemaVersion(), Generation: snapshot.Generation(), Platform: snapshot.Platform(), Manager: snapshot.Manager(), Tools: make([]publicTool, 0, len(selected))}
	for _, id := range selected {
		entry := publicTool{ToolID: id}
		if observation, ok := snapshot.Tool(id); ok {
			entry.Observed = true
			entry.Presence = observation.Presence()
			entry.Installability = observation.Installability()
			entry.RecipeDigest = observation.InstallRecipeDigest()
		}
		projection.Tools = append(projection.Tools, entry)
	}
	canonical, _ := json.Marshal(projection)
	digest := sha256.Sum256(canonical)
	return &planpublic.Snapshot{SchemaVersion: health.CurrentInstallationSchemaVersion, Generation: snapshot.Generation(), PublicDigest: hex.EncodeToString(digest[:])}
}

func projectInstallRecipe(recipe operation.InstallRecipe, digest string) (*planpublic.InstallSpec, error) {
	authentication, ok := publicAuthentication(recipe.Authentication)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported public authentication vocabulary", ErrInvalidRequest)
	}
	risk, ok := publicRisk(recipe.Risk)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported public risk vocabulary", ErrInvalidRequest)
	}
	steps := make([]planpublic.InstallStepSpec, len(recipe.Steps))
	for index, step := range recipe.Steps {
		kind := ""
		switch step.Kind {
		case operation.InstallStepPackageManager:
			kind = "package_manager"
		case operation.InstallStepNPMGlobal:
			kind = "npm_global"
		case operation.InstallStepHomebrewCask:
			kind = "homebrew_cask"
		default:
			return nil, fmt.Errorf("%w: unsupported public install step", ErrInvalidRequest)
		}
		steps[index] = planpublic.InstallStepSpec{
			Kind: kind, Provider: step.Provider, Packages: nonnilStrings(step.Packages),
			Casks: nonnilStrings(step.Casks), Arguments: nonnilStrings(step.Args),
		}
	}
	detectorKind := ""
	switch recipe.Detector.Kind {
	case operation.InstallDetectorBinary:
		detectorKind = "binary"
	case operation.InstallDetectorPackageReceipt:
		detectorKind = "package_receipt"
	case operation.InstallDetectorAppBundle:
		detectorKind = "app_bundle"
	default:
		return nil, fmt.Errorf("%w: unsupported public detector", ErrInvalidRequest)
	}
	return &planpublic.InstallSpec{
		SchemaVersion: recipe.SchemaVersion, Platform: recipe.Platform, Manager: recipe.Manager,
		Steps: steps, Detector: planpublic.DetectorSpec{Kind: detectorKind, Values: nonnilStrings(recipe.Detector.Values)},
		Authentication: authentication, Risk: risk, RecipeDigest: digest,
	}, nil
}

func publicAuthentication(value string) (string, bool) {
	mapping := map[string]string{
		"":                                     "none",
		"interactive provider login":           "interactive_provider_login",
		"ChatGPT sign-in or an OpenAI API key": "chatgpt_or_openai_api_key",
		"provider login or API key; local providers may require neither":                             "provider_login_or_api_key",
		"uses existing coding-agent authentication; T3 Code stores no dashboard-managed credentials": "existing_app_auth",
	}
	result, ok := mapping[value]
	return result, ok
}

func publicRisk(value string) (string, bool) {
	mapping := map[string]string{
		"installs packages from the configured system package manager":                                                   "package_manager_install",
		"downloads and executes npm package lifecycle code":                                                              "npm_lifecycle_code",
		"installs an npm package with lifecycle scripts disabled; package code runs when Pi is launched":                 "npm_scripts_disabled_runtime_code",
		"resolves the current OpenCode release from the reviewed package-manager source; artifact content is not pinned": "package_manager_current_release_unpinned",
		"installs the current t3-code Homebrew cask; artifact content is not pinned":                                     "homebrew_cask_unpinned",
	}
	result, ok := mapping[value]
	return result, ok
}

func clonePublicInstall(install *planpublic.InstallSpec) *planpublic.InstallSpec {
	if install == nil {
		return nil
	}
	copy := *install
	copy.Steps = make([]planpublic.InstallStepSpec, len(install.Steps))
	for index, step := range install.Steps {
		copy.Steps[index] = step
		copy.Steps[index].Packages = nonnilStrings(step.Packages)
		copy.Steps[index].Casks = nonnilStrings(step.Casks)
		copy.Steps[index].Arguments = nonnilStrings(step.Arguments)
	}
	copy.Detector.Values = nonnilStrings(install.Detector.Values)
	return &copy
}

func nonnilStrings(values []string) []string {
	result := slices.Clone(values)
	if result == nil {
		return []string{}
	}
	return result
}

func cloneRecipes(recipes map[string]operation.InstallRecipe) map[string]operation.InstallRecipe {
	result := make(map[string]operation.InstallRecipe, len(recipes))
	for id, recipe := range recipes {
		result[id] = operation.CloneInstallRecipe(recipe)
	}
	return result
}

func acceptedPlanHash(plan AcceptedPlan) (string, error) {
	if plan.snapshot.SchemaVersion != health.CurrentInstallationSchemaVersion {
		return "", ErrInvalidRequest
	}
	phase := classifyNPMExecutionPhase(plan.recipes)
	if phase == npmExecutionPhaseMixed {
		return "", errors.Join(ErrInvalidRequest, ErrNPMPhaseBoundaryRequired)
	}
	requiresManagerIdentity := phase == npmExecutionPhaseNone && recipesRequireManagerIdentity(plan.recipes)
	hasManagerIdentity := validManagerExecutableIdentity(plan.managerIdentity)
	if requiresManagerIdentity != hasManagerIdentity || (!hasManagerIdentity && plan.managerIdentity != (pkg.ExecutableIdentity{})) {
		return "", ErrInvalidRequest
	}
	requiresNPMIdentity := phase == npmExecutionPhasePure
	hasNPMIdentity := validNPMExecutionIdentity(plan.npmIdentity)
	if requiresNPMIdentity != hasNPMIdentity || (!hasNPMIdentity && plan.npmIdentity != (pkg.NPMExecutionIdentity{})) {
		if requiresNPMIdentity {
			return "", errors.Join(ErrInvalidRequest, ErrNPMExecutionAuthorityUnavailable)
		}
		return "", ErrInvalidRequest
	}
	stateDigest, err := operation.StatePlanAuthorityDigest(plan.statePlan)
	if err != nil {
		return "", err
	}
	documentDigest, err := decodeAcceptedAuthorityDigest(plan.document.Hash())
	if err != nil {
		return "", err
	}
	snapshotDigest, err := decodeAcceptedAuthorityDigest(plan.snapshot.Digest)
	if err != nil {
		return "", err
	}
	stateDigestBytes, err := decodeAcceptedAuthorityDigest(stateDigest)
	if err != nil {
		return "", err
	}
	intentDigest, err := decodeAcceptedAuthorityDigest(plan.intent.Digest)
	if err != nil {
		return "", err
	}
	var managerIdentityDigest []byte
	if hasManagerIdentity {
		managerIdentityDigest, err = decodeAcceptedAuthorityDigest(plan.managerIdentity.Digest())
		if err != nil {
			return "", err
		}
	}
	var npmIdentityDigest []byte
	if hasNPMIdentity {
		npmIdentityDigest, err = decodeAcceptedAuthorityDigest(plan.npmIdentity.Digest())
		if err != nil {
			return "", err
		}
	}
	ids := make([]string, 0, len(plan.tools))
	for id := range plan.tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	digest := sha256.New()
	_, _ = digest.Write([]byte("dotfiles/installplan-accepted-authority/v3\x00"))
	hashAcceptedAuthorityBytes(digest, documentDigest)
	hashAcceptedAuthorityUint64(digest, uint64(health.CurrentInstallationSchemaVersion))
	hashAcceptedAuthorityUint64(digest, plan.snapshot.Generation)
	hashAcceptedAuthorityBytes(digest, []byte(plan.snapshot.Platform))
	hashAcceptedAuthorityBytes(digest, []byte(plan.snapshot.Manager))
	if hasManagerIdentity {
		hashAcceptedAuthorityUint64(digest, 1)
		hashAcceptedAuthorityUint64(digest, uint64(pkg.CurrentExecutableIdentitySchemaVersion))
		hashAcceptedAuthorityBytes(digest, managerIdentityDigest)
	} else {
		hashAcceptedAuthorityUint64(digest, 0)
	}
	if hasNPMIdentity {
		hashAcceptedAuthorityUint64(digest, 1)
		hashAcceptedAuthorityUint64(digest, uint64(pkg.CurrentNPMExecutionIdentitySchemaVersion))
		hashAcceptedAuthorityBytes(digest, npmIdentityDigest)
	} else {
		hashAcceptedAuthorityUint64(digest, 0)
	}
	hashAcceptedAuthorityBytes(digest, snapshotDigest)
	hashAcceptedAuthorityBytes(digest, stateDigestBytes)
	hashAcceptedAuthorityBytes(digest, intentDigest)
	hashAcceptedAuthorityUint64(digest, uint64(len(ids)))
	for _, id := range ids {
		authority := plan.tools[id]
		hashAcceptedAuthorityBytes(digest, []byte(id))
		hashAcceptedAuthorityBytes(digest, []byte(authority.Presence))
		hashAcceptedAuthorityBytes(digest, []byte(authority.Intent))
		if authority.RecipeDigest == "" {
			hashAcceptedAuthorityBytes(digest, nil)
			continue
		}
		recipeDigest, decodeErr := decodeAcceptedAuthorityDigest(authority.RecipeDigest)
		if decodeErr != nil {
			return "", decodeErr
		}
		hashAcceptedAuthorityBytes(digest, recipeDigest)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func decodeAcceptedAuthorityDigest(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != value {
		return nil, ErrInvalidRequest
	}
	return decoded, nil
}

func hashAcceptedAuthorityBytes(digest hash.Hash, value []byte) {
	hashAcceptedAuthorityUint64(digest, uint64(len(value)))
	_, _ = digest.Write(value)
}

func hashAcceptedAuthorityUint64(digest hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = digest.Write(encoded[:])
}

func validateIntent(value planpublic.Intent) (planpublic.Intent, bool, error) {
	if value.Source == "explicit_tools" && len(value.Tools) == 0 && value.Digest == "" {
		return planpublic.Intent{Source: "explicit_tools", Tools: []string{}}, true, nil
	}
	normalized, err := planpublic.NormalizeExplicitTools(value.Tools, value.Tools)
	if err != nil || value.Source != normalized.Source || value.Digest != normalized.Digest || !slices.Equal(value.Tools, normalized.Tools) {
		return planpublic.Intent{}, false, planpublic.ErrInvalidIntent
	}
	return normalized, false, nil
}

func cloneIntent(intent planpublic.Intent) planpublic.Intent {
	intent.Tools = slices.Clone(intent.Tools)
	if intent.Tools == nil {
		intent.Tools = []string{}
	}
	return intent
}

func cloneAccepted(plan AcceptedPlan) AcceptedPlan {
	plan.intent = cloneIntent(plan.intent)
	plan.tools = maps.Clone(plan.tools)
	plan.recipes = plan.Recipes()
	return plan
}
