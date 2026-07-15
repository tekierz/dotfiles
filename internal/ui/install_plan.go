package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/installapply"
	headless "github.com/tekierz/dotfiles/internal/installplan"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/planpublic"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/scripts"
	"github.com/tekierz/dotfiles/internal/tools"
)

const gitManagedConfigRelForPlan = ".config/dotfiles/git/config"

// installPlan is the immutable production snapshot shared by preview, backup,
// execution, and summary. DeepDiveConfig is already deeply cloned before it is
// stored here; all slice/map accessors return copies.
type installPlan struct {
	document        operation.Plan
	installHash     string
	installSnapshot installationSnapshotAuthority
	installTools    map[string]installToolAuthority
	installRecipes  map[string]operation.InstallRecipe
	installPhase    operation.InstallPhase
	hasInstallPhase bool
	npmIdentity     pkg.NPMExecutionIdentity
	// selectedTools is reserved for the explicitly unreviewed legacy test
	// harness. Reviewed production plans leave it nil and derive installs from
	// the hash-bound operation actions.
	selectedTools   []string
	configTools     []string
	config          DeepDiveConfig
	theme           string
	navStyle        string
	animations      bool
	globalConfig    *config.GlobalConfig
	authority       map[string]map[string]acceptedTarget
	parentDirs      []string
	statePlan       *operation.StatePlan
	yaziConfigPaths tools.YaziConfigPaths
	// ghosttyConfigTarget is the exact absolute destination accepted during
	// planning. Execution must not rediscover a different higher-precedence file
	// after preview/revalidation.
	ghosttyConfigTarget string
}

type installationSnapshotAuthority struct {
	schema          int
	generation      uint64
	platform        string
	manager         string
	digest          string
	managerIdentity pkg.ExecutableIdentity
}

type installToolAuthority struct {
	presence     health.Presence
	intent       string
	recipeDigest string
}

func validUIManagerExecutableIdentity(identity pkg.ExecutableIdentity) bool {
	return identity.SchemaVersion() == pkg.CurrentExecutableIdentitySchemaVersion && validAdapterDigest(identity.Digest())
}

func installRecipeRequiresManagerIdentity(recipe operation.InstallRecipe) bool {
	for _, step := range recipe.Steps {
		if step.Kind == operation.InstallStepPackageManager || step.Kind == operation.InstallStepHomebrewCask {
			return true
		}
	}
	return false
}

func installRecipesRequireManagerIdentity(recipes map[string]operation.InstallRecipe) bool {
	for _, recipe := range recipes {
		if installRecipeRequiresManagerIdentity(recipe) {
			return true
		}
	}
	return false
}

func recipeDetectorRequiresManagerIdentity(detector operation.InstallDetector) bool {
	return detector.Kind == operation.InstallDetectorPackageReceipt
}

func validateUIManagerExecutableIdentity(manager pkg.PackageManager, accepted pkg.ExecutableIdentity, revalidate bool) error {
	if manager == nil || !validUIManagerExecutableIdentity(accepted) {
		return installapply.ErrManagerIdentityChanged
	}
	provider, ok := manager.(pkg.ExecutableIdentityProvider)
	if !ok {
		return installapply.ErrManagerIdentityChanged
	}
	current, available := provider.ExecutableIdentity()
	if !available || !validUIManagerExecutableIdentity(current) || current.SchemaVersion() != accepted.SchemaVersion() || current.Digest() != accepted.Digest() {
		return installapply.ErrManagerIdentityChanged
	}
	if revalidate && (accepted.Revalidate() != nil || current.Revalidate() != nil) {
		return installapply.ErrManagerIdentityChanged
	}
	// A revalidation-to-spawn race remains because PackageManager does not expose
	// a descriptor-bound process launch API; ExecuteRecipe repeats this check at
	// each manager-backed mutation boundary to keep that interval minimal.
	return nil
}

type acceptedTargetKind uint8

const (
	acceptedFileTarget acceptedTargetKind = iota + 1
	acceptedDirectoryTarget
)

// acceptedTarget is private execution authority captured by the same stable
// planning read that produced the public plan observation. File revisions are
// copied values; directory snapshots are opaque immutable values. A nil
// directory snapshot means the directory was accepted as absent.
type acceptedTarget struct {
	kind      acceptedTargetKind
	file      safefile.Revision
	directory *safefile.DirectorySnapshot
	parents   *safefile.ParentChain
	data      []byte
}

func (p *installPlan) hash() string {
	if p == nil {
		return ""
	}
	if p.installHash != "" {
		return p.installHash
	}
	return p.document.Hash()
}

func (p *installPlan) phase() (operation.InstallPhase, bool) {
	if p == nil || !p.hasInstallPhase {
		return operation.InstallPhase{}, false
	}
	return p.installPhase, true
}

func (p *installPlan) needsSudo() bool {
	phase, phased := p.phase()
	return !phased || phase.Authority() == operation.InstallAuthorityManager
}

func (p *installPlan) installAuthority(toolID string) (health.Presence, string, bool) {
	if p == nil {
		return health.PresenceUnknown, "", false
	}
	authority, ok := p.installTools[toolID]
	return authority.presence, authority.intent, ok
}

func (p *installPlan) installSnapshotAuthority() (int, uint64, string, string, string) {
	if p == nil {
		return 0, 0, "", "", ""
	}
	a := p.installSnapshot
	return a.schema, a.generation, a.platform, a.manager, a.digest
}

func (p *installPlan) installRecipeAuthority(toolID string) (string, bool) {
	if p == nil {
		return "", false
	}
	authority, ok := p.installTools[toolID]
	if !ok || authority.recipeDigest == "" {
		return "", false
	}
	return authority.recipeDigest, true
}

func (p *installPlan) actions() []operation.Action {
	if p == nil {
		return nil
	}
	return p.document.Actions()
}

func (p *installPlan) remoteArtifact(actionID string) (operation.RemoteArtifact, bool) {
	if p == nil {
		return operation.RemoteArtifact{}, false
	}
	for _, action := range p.document.Actions() {
		if action.ID == actionID && action.RemoteArtifact != nil && action.RemoteArtifact.AuthorityDigest() != "" {
			return *action.RemoteArtifact, true
		}
	}
	return operation.RemoteArtifact{}, false
}

func (p *installPlan) backupTargets() []string {
	if p == nil {
		return nil
	}
	return p.document.BackupTargets()
}

// packageOnlyReviewedExecution identifies the one reviewed plan class that has
// no filesystem state to restore. It is deliberately strict: at least one
// mutation must be applied, and every applied mutation must be a package-manager
// install. Any mixed or filesystem/state plan still requires a rollback point.
func (p *installPlan) packageOnlyReviewedExecution() bool {
	if p == nil {
		return false
	}
	hasApply := false
	for _, action := range p.actions() {
		if action.Disposition != operation.DispositionApply {
			continue
		}
		hasApply = true
		if action.Kind != operation.KindInstallTool || action.Ownership != operation.OwnershipPackageManager {
			return false
		}
	}
	return hasApply
}

func (p *installPlan) backupTargetSpecs() ([]backup.Target, error) {
	if p == nil {
		return nil, fmt.Errorf("no accepted plan")
	}
	var targets []backup.Target
	for _, action := range p.actions() {
		if action.Disposition != operation.DispositionApply {
			continue
		}
		paths := append([]string(nil), action.BackupTargets...)
		if action.BackupTarget != "" {
			paths = append(paths, action.BackupTarget)
		}
		for _, rel := range paths {
			target, err := p.acceptedTarget(action.ID, rel)
			if err != nil {
				return nil, err
			}
			kind := backup.TargetFile
			if target.kind == acceptedDirectoryTarget {
				kind = backup.TargetDirectory
			}
			targets = append(targets, backup.Target{RelPath: rel, Kind: kind})
		}
	}
	return targets, nil
}

func (p *installPlan) hasBlocked() bool { return p != nil && p.document.HasBlocked() }
func (p *installPlan) selectedToolIDs() []string {
	if p == nil {
		return nil
	}
	if p.selectedTools != nil {
		return slices.Clone(p.selectedTools)
	}
	var selected []string
	for _, action := range p.actions() {
		if action.Kind == operation.KindInstallTool && action.Disposition == operation.DispositionApply {
			selected = append(selected, action.ToolID)
		}
	}
	return selected
}

func (p *installPlan) installExecutionSnapshot() (installExecutionSnapshot, error) {
	if p == nil {
		return installExecutionSnapshot{}, fmt.Errorf("no accepted plan")
	}
	snapshot := installExecutionSnapshot{
		platform: pkg.Platform(p.installSnapshot.platform), manager: p.installSnapshot.manager,
		managerIdentity: p.installSnapshot.managerIdentity,
		recipes:         make(map[string]operation.InstallRecipe), detected: make(map[string]bool),
		digests: make(map[string]string), authority: make(map[string]installToolAuthority),
	}
	for _, action := range p.actions() {
		if action.Kind != operation.KindInstallTool || action.Disposition != operation.DispositionApply {
			continue
		}
		if action.InstallRecipe == nil || action.InstallDetected == nil {
			return installExecutionSnapshot{}, fmt.Errorf("install action %s lacks accepted authority", action.ID)
		}
		if _, duplicate := snapshot.recipes[action.ToolID]; duplicate {
			return installExecutionSnapshot{}, fmt.Errorf("duplicate install action for %s", action.ToolID)
		}
		authority, authoritative := p.installTools[action.ToolID]
		pinned, pinnedOK := p.installRecipes[action.ToolID]
		if !authoritative || !pinnedOK || (authority.intent != "install" && authority.intent != "repair") {
			return installExecutionSnapshot{}, fmt.Errorf("install action %s lacks typed accepted authority", action.ID)
		}
		recipe := operation.CloneInstallRecipe(pinned)
		if snapshot.platform != pkg.Platform(recipe.Platform) || snapshot.manager != recipe.Manager {
			return installExecutionSnapshot{}, fmt.Errorf("install actions disagree on environment")
		}
		if digest := installRecipeDigest(recipe); digest == "" || digest != authority.recipeDigest || digest != action.DesiredDigest {
			return installExecutionSnapshot{}, fmt.Errorf("install action %s has inconsistent pinned recipe", action.ID)
		}
		snapshot.recipes[action.ToolID] = recipe
		snapshot.detected[action.ToolID] = *action.InstallDetected
		snapshot.digests[action.ToolID] = action.DesiredDigest
		snapshot.authority[action.ToolID] = authority
	}
	identityRequired := installRecipesRequireManagerIdentity(snapshot.recipes)
	if identityRequired != validUIManagerExecutableIdentity(snapshot.managerIdentity) {
		return installExecutionSnapshot{}, fmt.Errorf("install manager executable authority is inconsistent")
	}
	return snapshot, nil
}
func (p *installPlan) configToolIDs() []string {
	if p == nil {
		return nil
	}
	return slices.Clone(p.configTools)
}

func (p *installPlan) acceptedFileRevision(actionID, rel string) (safefile.Revision, error) {
	target, err := p.acceptedTarget(actionID, rel)
	if err != nil {
		return safefile.Revision{}, err
	}
	if target.kind != acceptedFileTarget || !target.file.Tracked() {
		return safefile.Revision{}, fmt.Errorf("accepted target %s for %s is not a tracked file revision", rel, actionID)
	}
	return target.file, nil
}

func (p *installPlan) acceptedDirectorySnapshot(actionID, rel string) (*safefile.DirectorySnapshot, error) {
	target, err := p.acceptedTarget(actionID, rel)
	if err != nil {
		return nil, err
	}
	if target.kind != acceptedDirectoryTarget {
		return nil, fmt.Errorf("accepted target %s for %s is not a directory snapshot", rel, actionID)
	}
	return target.directory, nil
}

func (p *installPlan) acceptedDirectoryAuthority(actionID, rel string) (*safefile.DirectorySnapshot, *safefile.ParentChain, error) {
	target, err := p.acceptedTarget(actionID, rel)
	if err != nil {
		return nil, nil, err
	}
	if target.kind != acceptedDirectoryTarget || !target.parents.Tracked() {
		return nil, nil, fmt.Errorf("accepted target %s for %s has incomplete directory authority", rel, actionID)
	}
	return target.directory, target.parents, nil
}

func (p *installPlan) acceptedTarget(actionID, rel string) (acceptedTarget, error) {
	if p == nil {
		return acceptedTarget{}, fmt.Errorf("no accepted plan")
	}
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	byPath := p.authority[actionID]
	target, ok := byPath[rel]
	if !ok {
		return acceptedTarget{}, fmt.Errorf("accepted plan has no authority for %s target %s", actionID, rel)
	}
	return target, nil
}

func (p *installPlan) plannedGlobalConfig() (*config.GlobalConfig, error) {
	if p == nil || p.globalConfig == nil {
		return nil, fmt.Errorf("accepted plan has no global config snapshot")
	}
	return config.CloneGlobalConfig(p.globalConfig), nil
}

func (p *installPlan) parentDirectoryTargets() []string {
	if p == nil {
		return nil
	}
	return slices.Clone(p.parentDirs)
}

func (p *installPlan) acceptedGhosttyConfigTarget() (string, error) {
	if p == nil {
		return "", fmt.Errorf("no accepted plan")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	for _, action := range p.actions() {
		if action.ID != "config:ghostty" || action.Disposition != operation.DispositionApply {
			continue
		}
		if len(action.Observations) != 1 || filepath.IsAbs(action.Observations[0].Source) {
			return "", fmt.Errorf("accepted Ghostty action has invalid target observations")
		}
		target := filepath.Join(home, filepath.FromSlash(action.Observations[0].Source))
		if filepath.Clean(target) != filepath.Clean(p.ghosttyConfigTarget) {
			return "", fmt.Errorf("accepted Ghostty execution target no longer matches hashed plan action")
		}
		return target, nil
	}
	return "", fmt.Errorf("accepted plan has no applicable Ghostty action")
}

type configPlanSpec struct {
	actionID                  string
	toolID                    string
	yaziKind                  tools.YaziFileKind
	targets                   []string
	ownership                 operation.Ownership
	description               string
	fullFilePolicy            bool
	targetOwnership           map[string]operation.Ownership
	currentTheme              string
	allowBtopThemeReplacement bool
	preflightBlockReason      string
}

func (a *App) refreshPendingInstallPlan() {
	if a == nil || !a.manageInstalledReady || a.installCacheLoading {
		return
	}
	runtime := defaultToolInstallRuntime()
	var plan *installPlan
	var err error
	if a.installReviewTools != nil {
		plan, err = buildInstallPlanForTools(a, runtime, time.Now(), slices.Clone(a.installReviewTools))
	} else {
		plan, err = buildInstallPlan(a, runtime, time.Now())
	}
	a.pendingInstallPlan = plan
	a.installPlanError = err
	a.installPlanScroll = 0
}

func (a *App) invalidatePendingInstallPlan() {
	a.pendingInstallPlan = nil
	a.installPlanError = nil
}

func buildInstallPlan(a *App, installRuntime toolInstallRuntime, now time.Time) (*installPlan, error) {
	return buildInstallPlanForTools(a, installRuntime, now, nil)
}

func buildInstallPlanForTools(a *App, installRuntime toolInstallRuntime, now time.Time, onlyTools []string) (*installPlan, error) {
	if onlyTools != nil && len(onlyTools) == 0 {
		return nil, errors.New("explicit tool intent required")
	}
	if a == nil || !a.installationSnapshotPlanningReady() || !a.installationSnapshotTerminal ||
		a.installationSnapshot.Digest() == "" || a.installationSnapshot.Generation() == 0 ||
		a.installationSnapshot.Generation() != a.installationSnapshotGeneration {
		return nil, errors.New(installationSnapshotUnavailable)
	}
	snapshot := a.installationSnapshot
	if onlyTools != nil {
		return buildPackageOnlyInstallPlan(a, installRuntime, now, snapshot, onlyTools)
	}
	if a.deepDiveConfig == nil {
		return nil, fmt.Errorf("cannot build install plan without installer state")
	}
	cfg := snapshotDeepDiveConfig(a.deepDiveConfig)
	candidates := installPlanCandidateTools(a, pkg.Platform(snapshot.Platform()), onlyTools)
	for _, id := range candidates {
		observation, observed := snapshot.Tool(id)
		if !observed || (observation.Presence() != health.PresenceMissing && observation.Presence() != health.PresencePartial) || observation.Installability() != health.InstallabilitySupported {
			continue
		}
		tool, found := installRuntime.lookupTool(id)
		if !found {
			continue
		}
		recipe, recipeErr := installRuntime.describeInstall(tool, tools.InstallEnvironment{Platform: pkg.Platform(snapshot.Platform()), Manager: snapshot.Manager()})
		if recipeErr != nil {
			continue
		}
		for _, step := range recipe.Steps {
			if step.Kind == operation.InstallStepNPMGlobal {
				return nil, fmt.Errorf("%s requires a separate phased install; install it from Manage, then return for configuration", tool.Name())
			}
		}
	}
	statePlan, err := operation.CaptureStatePlan()
	if err != nil {
		return nil, fmt.Errorf("capture private operation state before planning: %w", err)
	}
	globalConfig, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("validate global config before planning: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, fmt.Errorf("determine home for install plan: %w", err)
	}
	yaziConfigPaths, err := tools.ResolveYaziConfigPaths()
	if err != nil {
		return nil, fmt.Errorf("resolve Yazi config paths for install plan: %w", err)
	}
	platform := pkg.Platform(snapshot.Platform())
	managerName := snapshot.Manager()
	installAuthorities := make(map[string]installToolAuthority, len(candidates))
	pinnedRecipes := make(map[string]operation.InstallRecipe)

	globalPath := filepath.Join(config.ConfigDir(), "global.json")
	globalTarget := planTargetPath(home, globalPath)
	globalDisposition := operation.DispositionApply
	globalReason := ""
	var globalObservation operation.Observation
	globalBackupTarget := ""
	var globalObservations []operation.Observation
	if filepath.IsAbs(globalTarget) {
		globalDisposition = operation.DispositionBlocked
		globalReason = "global settings outside HOME cannot yet receive a verified rollback point"
	} else {
		globalRevision, tracked := config.GlobalConfigRevision(globalConfig)
		if !tracked {
			return nil, fmt.Errorf("observe global settings: loaded config has no tracked revision")
		}
		globalObservation = observationFromFileRevision(globalTarget, globalRevision, false)
		globalBackupTarget = globalTarget
		globalObservations = []operation.Observation{globalObservation}
	}
	plannedGlobal := config.CloneGlobalConfig(globalConfig)
	plannedGlobal.Theme = a.theme
	plannedGlobal.NavStyle = a.navStyle
	plannedGlobal.DisableAnimations = !a.animationsEnabled
	authority := make(map[string]map[string]acceptedTarget)
	if globalDisposition == operation.DispositionApply {
		globalRevision, _ := config.GlobalConfigRevision(globalConfig)
		globalData, observedRevision, globalParents, observeErr := safefile.ObserveFileWithin(home, globalTarget)
		if observeErr != nil || observedRevision != globalRevision {
			return nil, fmt.Errorf("stably observe global settings namespace: %w", errors.Join(observeErr, safefile.ErrRevisionChanged))
		}
		authority["state:global"] = map[string]acceptedTarget{globalTarget: {kind: acceptedFileTarget, file: globalRevision, parents: globalParents, data: slices.Clone(globalData)}}
	}
	actions := []operation.Action{{
		ID:          "state:global",
		Kind:        operation.KindUpdateState,
		Target:      globalTarget,
		Description: "persist selected theme, navigation, and motion preferences",
		Disposition: globalDisposition,
		Reason:      globalReason,
		DesiredDigest: digestPlanValue(struct {
			Theme      string
			Navigation string
			Animations bool
		}{a.theme, a.navStyle, a.animationsEnabled}),
		Ownership:     operation.OwnershipManagedFile,
		Reversibility: operation.ReversibilityBackup,
		BackupTarget:  globalBackupTarget,
		Observation:   globalObservation,
		Observations:  globalObservations,
	}}
	for _, toolID := range candidates {
		observation, observed := snapshot.Tool(toolID)
		if !observed {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		presence := observation.Presence()
		if presence == health.PresencePresent {
			installAuthorities[toolID] = installToolAuthority{presence: presence, intent: "none"}
			continue
		}
		if presence != health.PresenceMissing && presence != health.PresencePartial {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		if observation.Installability() != health.InstallabilitySupported {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		tool, ok := installRuntime.lookupTool(toolID)
		if !ok {
			return nil, fmt.Errorf("selected tool %s disappeared while planning", toolID)
		}
		recipe, err := tools.DescribeInstall(tool, tools.InstallEnvironment{Platform: platform, Manager: managerName})
		if err != nil {
			return nil, fmt.Errorf("describe selected tool %s install: %w", toolID, err)
		}
		recipeDigest := installRecipeDigest(recipe)
		if recipeDigest == "" || recipeDigest != observation.InstallRecipeDigest() {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		intent := "install"
		description := "install " + toolID
		if presence == health.PresencePartial {
			intent = "repair"
			description = "repair " + toolID
		}
		installAuthorities[toolID] = installToolAuthority{presence: presence, intent: intent, recipeDigest: recipeDigest}
		pinnedRecipes[toolID] = operation.CloneInstallRecipe(recipe)
		detected, detectorKnown := headless.ObservedInstallDetector(observation, recipe.Detector)
		if !detectorKnown {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		actions = append(actions, operation.Action{
			ID:              "install:" + toolID,
			Kind:            operation.KindInstallTool,
			ToolID:          toolID,
			Target:          toolID,
			Description:     description,
			Disposition:     operation.DispositionApply,
			DesiredDigest:   recipeDigest,
			Ownership:       operation.OwnershipPackageManager,
			Reversibility:   operation.ReversibilityManual,
			InstallRecipe:   &recipe,
			InstallDetected: &detected,
		})
	}
	managerIdentity := pkg.ExecutableIdentity{}
	if installRecipesRequireManagerIdentity(pinnedRecipes) {
		if !validUIManagerExecutableIdentity(a.installationSnapshotManagerIdentity) {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		managerIdentity = a.installationSnapshotManagerIdentity
	}

	for _, helper := range enabledHelpers(cfg.Utilities) {
		rel := filepath.ToSlash(filepath.Join(".local", "bin", helper))
		desiredContent := scriptsForPlan(helper)
		desired := sha256.Sum256(desiredContent)
		observation, revision, parents, original, err := observeRelativeFile(home, rel)
		if err != nil {
			return nil, fmt.Errorf("observe helper %s: %w", helper, err)
		}
		disposition := operation.DispositionApply
		reason := ""
		if observation.Exists {
			if observation.Digest != hex.EncodeToString(desired[:]) {
				disposition = operation.DispositionBlocked
				reason = "an existing helper at this path is not proven to be owned by dotfiles"
			}
		}
		actions = append(actions, operation.Action{
			ID:            "helper:" + helper,
			Kind:          operation.KindInstallFile,
			ToolID:        helper,
			Target:        rel,
			Description:   "install managed helper " + helper,
			Disposition:   disposition,
			Reason:        reason,
			DesiredDigest: hex.EncodeToString(desired[:]),
			Ownership:     operation.OwnershipManagedFile,
			Reversibility: operation.ReversibilityBackup,
			BackupTarget:  rel,
			Observation:   observation,
			Observations:  []operation.Observation{observation},
		})
		if disposition == operation.DispositionApply {
			authority["helper:"+helper] = map[string]acceptedTarget{rel: {kind: acceptedFileTarget, file: revision, parents: parents, data: original}}
		}
	}

	allowBtopThemeReplacement := a.nativeConfigState.BtopThemeExplicit || cfg.BtopTheme != manageConfigToDeepDive(&a.manageConfigBaseline).BtopTheme || (cfg.BtopTheme == "auto" && a.theme != a.manageConfigBaselineTheme)
	configSpecs, err := installerConfigSpecsAtResolved(home, a.theme, cfg, allowBtopThemeReplacement, yaziConfigPaths)
	if err != nil {
		return nil, err
	}
	configTools := make([]string, 0, len(configSpecs))
	ghosttyConfigTarget := ""
	for _, spec := range configSpecs {
		action, actionAuthority, err := planConfigAction(home, spec, digestPlanValue(struct {
			ToolID string
			Theme  string
			Config DeepDiveConfig
		}{spec.toolID, a.theme, cfg}))
		if err != nil {
			return nil, err
		}
		switch spec.toolID {
		case "tmux":
			if cfg.TmuxTPMEnabled {
				artifact, artifactErr := tools.TPMRemoteArtifact()
				if artifactErr != nil {
					return nil, fmt.Errorf("bind pinned TPM artifact: %w", artifactErr)
				}
				action.RemoteArtifact = &artifact
			}
		case "neovim":
			artifact, artifactErr := tools.NeovimRemoteArtifact(cfg.NeovimConfig)
			if artifactErr != nil {
				return nil, fmt.Errorf("bind pinned Neovim artifact: %w", artifactErr)
			}
			action.RemoteArtifact = &artifact
		}
		if spec.toolID == "ghostty" && len(spec.targets) == 1 {
			ghosttyConfigTarget = spec.targets[0]
			if !filepath.IsAbs(ghosttyConfigTarget) {
				ghosttyConfigTarget = filepath.Join(home, filepath.FromSlash(ghosttyConfigTarget))
			}
			if a.nativeConfigState.PreferenceError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
			} else if a.nativeConfigState.GhosttyError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "native Ghostty configuration could not be imported safely: " + a.nativeConfigState.GhosttyError
			}
		}
		if spec.toolID == "git" {
			if a.nativeConfigState.PreferenceError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
			} else if a.nativeConfigState.GitError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "native Git configuration could not be imported safely: " + a.nativeConfigState.GitError
			}
			for _, observed := range action.Observations {
				if observed.Source == gitManagedConfigRelForPlan && observed.Exists && !observed.Managed {
					action.Disposition = operation.DispositionBlocked
					action.Reason = "the dotfiles Git include destination already contains an unowned file"
				}
			}
		}
		if spec.toolID == "tmux" {
			if a.nativeConfigState.PreferenceError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
			} else if a.nativeConfigState.TmuxError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "native tmux configuration could not be imported safely: " + a.nativeConfigState.TmuxError
			}
		}
		if spec.toolID == "btop" {
			if a.nativeConfigState.PreferenceError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
			} else if a.nativeConfigState.BtopError != "" && (!a.nativeConfigState.BtopThemeUnsupported || !spec.allowBtopThemeReplacement) {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "native btop configuration could not be imported safely: " + a.nativeConfigState.BtopError
			}
		}
		if spec.toolID == "glow" {
			if a.nativeConfigState.PreferenceError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
			} else if a.nativeConfigState.GlowError != "" {
				action.Disposition = operation.DispositionBlocked
				action.Reason = "native Glow configuration could not be imported safely: " + a.nativeConfigState.GlowError
			}
		}
		if spec.toolID == "lazygit" {
			switch {
			case a.nativeConfigState.PreferenceError != "":
				action.Disposition = operation.DispositionBlocked
				action.Reason = "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
			case a.nativeConfigState.LazyGitError != "":
				action.Disposition = operation.DispositionBlocked
				action.Reason = "native LazyGit configuration could not be imported safely: " + a.nativeConfigState.LazyGitError
			case a.nativeConfigState.LazyGit.ReadOnlyReason != "":
				action.Disposition = operation.DispositionBlocked
				action.Reason = a.nativeConfigState.LazyGit.ReadOnlyReason
			case cfg.LazyGitPagerPreset == "delta" && !cfg.CLIUtilities["delta"]:
				action.Disposition = operation.DispositionBlocked
				action.Reason = "select the Delta CLI utility before using the LazyGit Delta pager preset"
			}
		}
		actions = append(actions, action)
		if action.Disposition == operation.DispositionApply {
			if !slices.Contains(configTools, spec.toolID) {
				configTools = append(configTools, spec.toolID)
			}
			authority[action.ID] = actionAuthority
		}
	}
	if err := validateStateProductSeparation(home, actions); err != nil {
		return nil, err
	}

	parents, err := missingProductParentDirectories(home, actions)
	if err != nil {
		return nil, err
	}
	if len(parents) > 0 {
		observations := make([]operation.Observation, 0, len(parents))
		parentAuthority := make(map[string]acceptedTarget, len(parents))
		for _, rel := range parents {
			_, parentChain, observeErr := safefile.ObserveDirectoryWithin(home, rel)
			if observeErr != nil && !errors.Is(observeErr, os.ErrNotExist) {
				return nil, fmt.Errorf("observe planned parent %s: %w", rel, observeErr)
			}
			observations = append(observations, operation.Observation{Source: rel})
			parentAuthority[rel] = acceptedTarget{kind: acceptedDirectoryTarget, parents: parentChain}
		}
		parentAction := operation.Action{
			ID:            "state:parents",
			Kind:          operation.KindUpdateState,
			Target:        strings.Join(parents, ", "),
			Description:   "create reviewed private parent directories for selected configuration",
			Disposition:   operation.DispositionApply,
			DesiredDigest: digestPlanValue(parents),
			Ownership:     operation.OwnershipManagedFile,
			Reversibility: operation.ReversibilityBackup,
			BackupTargets: slices.Clone(parents),
			Observation:   operation.Observation{Source: strings.Join(parents, ",")},
			Observations:  observations,
		}
		actions = append([]operation.Action{parentAction}, actions...)
		authority[parentAction.ID] = parentAuthority
	}

	document, err := operation.NewPlan(now, actions)
	if err != nil {
		return nil, err
	}
	if err := validateInstallPlanAuthority(actions, authority); err != nil {
		return nil, err
	}
	snapshotAuthority := installationSnapshotAuthority{
		schema: snapshot.SchemaVersion(), generation: snapshot.Generation(), platform: snapshot.Platform(),
		manager: snapshot.Manager(), digest: snapshot.Digest(), managerIdentity: managerIdentity,
	}
	identityDiscriminator := 0
	identitySchema := 0
	identityDigest := ""
	if validUIManagerExecutableIdentity(managerIdentity) {
		identityDiscriminator = 1
		identitySchema = managerIdentity.SchemaVersion()
		identityDigest = managerIdentity.Digest()
	}
	installHash := digestPlanValue(struct {
		Document                     string
		Schema                       int
		Generation                   uint64
		Platform                     string
		Manager                      string
		Snapshot                     string
		Recipes                      []string
		ManagerIdentityDiscriminator int
		ManagerIdentitySchema        int
		ManagerIdentityDigest        string
	}{document.Hash(), snapshotAuthority.schema, snapshotAuthority.generation, snapshotAuthority.platform,
		snapshotAuthority.manager, snapshotAuthority.digest, sortedInstallRecipeAuthorities(installAuthorities),
		identityDiscriminator, identitySchema, identityDigest})
	return &installPlan{
		document:            document,
		installHash:         installHash,
		installSnapshot:     snapshotAuthority,
		installTools:        maps.Clone(installAuthorities),
		installRecipes:      cloneInstallRecipes(pinnedRecipes),
		configTools:         configTools,
		config:              cfg,
		theme:               a.theme,
		navStyle:            a.navStyle,
		animations:          a.animationsEnabled,
		globalConfig:        plannedGlobal,
		authority:           authority,
		parentDirs:          slices.Clone(parents),
		statePlan:           statePlan,
		yaziConfigPaths:     yaziConfigPaths,
		ghosttyConfigTarget: ghosttyConfigTarget,
	}, nil
}

func buildPackageOnlyInstallPlan(a *App, installRuntime toolInstallRuntime, now time.Time, snapshot health.InstallationSnapshot, onlyTools []string) (*installPlan, error) {
	if installRuntime.registeredToolIDs == nil || installRuntime.lookupTool == nil || installRuntime.describeInstall == nil || installRuntime.captureStatePlan == nil {
		return nil, errors.New(installationSnapshotUnavailable)
	}
	intent, err := planpublic.NormalizeExplicitTools(onlyTools, installRuntime.registeredToolIDs())
	if errors.Is(err, planpublic.ErrIntentRequired) {
		return nil, errors.New("explicit tool intent required")
	}
	if err != nil {
		return nil, errors.New(installationSnapshotUnavailable)
	}
	// NP4 re-plans immediately before Apply using the coordinator's canonical
	// generation. Normalize the already-fresh UI observation to that same
	// session generation so an unchanged host reproduces the reviewed hash;
	// any actual observation drift still changes the snapshot digest and blocks.
	phaseSnapshot, err := health.NewInstallationSnapshot(health.InstallationSnapshotSpec{
		Generation: 1, Platform: snapshot.Platform(), Manager: snapshot.Manager(), Tools: snapshot.Tools(),
	})
	if err != nil {
		return nil, errors.New(installationSnapshotUnavailable)
	}
	request := headless.Request{
		Intent:   intent,
		Snapshot: phaseSnapshot,
		Environment: headless.Environment{
			Platform:           pkg.Platform(snapshot.Platform()),
			Manager:            snapshot.Manager(),
			ManagerIdentity:    a.installationSnapshotManagerIdentity,
			ExpectedGeneration: phaseSnapshot.Generation(),
		},
	}
	dependencies := headless.Dependencies{
		LookupTool:       installRuntime.lookupTool,
		DescribeInstall:  installRuntime.describeInstall,
		CaptureStatePlan: installRuntime.captureStatePlan,
		Now:              func() time.Time { return now },
	}
	result, err := headless.BuildPhased(request, dependencies)
	if errors.Is(err, headless.ErrNPMExecutionAuthorityUnavailable) {
		npmIdentity, identityErr := observeUINPMExecutionIdentity()
		if identityErr != nil {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		request.Environment.NPMIdentity = npmIdentity
		result, err = headless.BuildPhased(request, dependencies)
	}
	if err != nil {
		return nil, errors.New(installationSnapshotUnavailable)
	}
	accepted, hasAccepted := result.Accepted()
	switch result.Public().Status() {
	case planpublic.StatusReady:
		if !hasAccepted {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		plan, adoptErr := adoptHeadlessInstallPlan(a, accepted)
		if adoptErr != nil {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		return plan, nil
	case planpublic.StatusNoChanges:
		if hasAccepted {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		return nil, errors.New("no installation changes required")
	case planpublic.StatusIntentRequired:
		if hasAccepted {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		return nil, errors.New("explicit tool intent required")
	case planpublic.StatusBlocked:
		if hasAccepted {
			return nil, errors.New(installationSnapshotUnavailable)
		}
		return nil, errors.New(installationSnapshotUnavailable)
	default:
		return nil, errors.New(installationSnapshotUnavailable)
	}
}

func observeUINPMExecutionIdentity() (pkg.NPMExecutionIdentity, error) {
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return pkg.NPMExecutionIdentity{}, err
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return pkg.NPMExecutionIdentity{}, err
	}
	return pkg.ObserveNPMExecutionIdentity(npmPath, nodePath)
}

func installPlanCandidateTools(a *App, platform pkg.Platform, onlyTools []string) []string {
	if len(onlyTools) != 0 {
		result := slices.Clone(onlyTools)
		sort.Strings(result)
		return slices.Compact(result)
	}
	seen := make(map[string]struct{})
	add := func(id string, enabled bool) {
		if enabled && id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, id := range alwaysConfiguredToolIDs {
		add(id, true)
	}
	for id, enabled := range a.deepDiveConfig.CLITools {
		add(id, enabled)
	}
	for id, enabled := range a.deepDiveConfig.GUIApps {
		add(id, enabled)
	}
	for id, enabled := range a.deepDiveConfig.CLIUtilities {
		add(id, enabled)
	}
	if platform == pkg.PlatformMacOS {
		for id, enabled := range a.deepDiveConfig.MacApps {
			add(id, enabled)
		}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func sortedInstallRecipeAuthorities(authorities map[string]installToolAuthority) []string {
	result := make([]string, 0, len(authorities))
	for id, authority := range authorities {
		result = append(result, id+":"+string(authority.presence)+":"+authority.intent+":"+authority.recipeDigest)
	}
	sort.Strings(result)
	return result
}

func cloneInstallRecipes(recipes map[string]operation.InstallRecipe) map[string]operation.InstallRecipe {
	cloned := make(map[string]operation.InstallRecipe, len(recipes))
	for id, recipe := range recipes {
		cloned[id] = operation.CloneInstallRecipe(recipe)
	}
	return cloned
}

func installRecipeDigest(recipe operation.InstallRecipe) string {
	digest, err := operation.InstallRecipeDigest(recipe)
	if err != nil {
		return ""
	}
	return digest
}

func validateStateProductSeparation(home string, actions []operation.Action) error {
	stateChild, err := operation.StateSubdirectory("separation-probe")
	if err != nil {
		return err
	}
	stateRoot, err := canonicalProspectivePath(filepath.Dir(stateChild))
	if err != nil {
		return fmt.Errorf("canonicalize operation state namespace: %w", err)
	}
	for _, action := range actions {
		if action.Disposition != operation.DispositionApply {
			continue
		}
		targets := append([]string(nil), action.BackupTargets...)
		if action.BackupTarget != "" {
			targets = append(targets, action.BackupTarget)
		}
		for _, rel := range targets {
			absolute := rel
			if !filepath.IsAbs(absolute) {
				absolute = filepath.Join(home, filepath.FromSlash(rel))
			}
			product, err := canonicalProspectivePath(absolute)
			if err != nil {
				return fmt.Errorf("canonicalize product target %s: %w", rel, err)
			}
			if pathContains(stateRoot, product) || pathContains(product, stateRoot) {
				return fmt.Errorf("operation state namespace overlaps reviewed product target %s", rel)
			}
		}
	}
	return nil
}

func canonicalProspectivePath(path string) (string, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("path must be absolute: %s", path)
	}
	ancestor := clean
	var suffix []string
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("no existing ancestor for %s", path)
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{resolved}, suffix...)...), nil
}

func pathContains(parent, child string) bool {
	if runtime.GOOS == "darwin" {
		parent = strings.ToLower(parent)
		child = strings.ToLower(child)
	}
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func missingProductParentDirectories(home string, actions []operation.Action) ([]string, error) {
	seen := make(map[string]struct{})
	operationalAncestors := make(map[string]struct{})
	stateChild, err := operation.StateSubdirectory("planning-anchor")
	if err != nil {
		return nil, fmt.Errorf("resolve operational state namespace: %w", err)
	}
	stateRoot := filepath.Dir(stateChild)
	if stateRel, relErr := filepath.Rel(home, stateRoot); relErr == nil && stateRel != "." && !filepath.IsAbs(stateRel) && stateRel != ".." && !strings.HasPrefix(stateRel, ".."+string(filepath.Separator)) {
		current := filepath.ToSlash(filepath.Clean(stateRel))
		for current != "." && current != "" {
			operationalAncestors[current] = struct{}{}
			current = filepath.ToSlash(filepath.Dir(filepath.FromSlash(current)))
		}
	}
	for _, action := range actions {
		if action.Disposition != operation.DispositionApply {
			continue
		}
		targets := append([]string(nil), action.BackupTargets...)
		if action.BackupTarget != "" {
			targets = append(targets, action.BackupTarget)
		}
		for _, target := range targets {
			parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(target)))
			for parent != "." && parent != "" {
				absolute := filepath.Join(home, filepath.FromSlash(parent))
				info, err := os.Lstat(absolute)
				switch {
				case err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0:
					parent = "."
					continue
				case err == nil:
					return nil, fmt.Errorf("planned parent %s is not a real directory", parent)
				case !errors.Is(err, os.ErrNotExist):
					return nil, fmt.Errorf("inspect planned parent %s: %w", parent, err)
				default:
					if _, operational := operationalAncestors[parent]; !operational {
						seen[parent] = struct{}{}
					}
					parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent)))
				}
			}
		}
	}
	parents := make([]string, 0, len(seen))
	for rel := range seen {
		parents = append(parents, rel)
	}
	sort.Slice(parents, func(i, j int) bool {
		leftDepth := strings.Count(parents[i], "/")
		rightDepth := strings.Count(parents[j], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return parents[i] < parents[j]
	})
	return parents, nil
}

func validateInstallPlanAuthority(actions []operation.Action, authority map[string]map[string]acceptedTarget) error {
	applicable := make(map[string]map[string]struct{})
	for _, action := range actions {
		if action.Disposition != operation.DispositionApply {
			continue
		}
		targets := append([]string(nil), action.BackupTargets...)
		if action.BackupTarget != "" {
			targets = append(targets, action.BackupTarget)
		}
		if len(targets) == 0 {
			continue
		}
		allowed := make(map[string]struct{}, len(targets))
		for _, rel := range targets {
			rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
			if filepath.IsAbs(rel) || rel == "." || strings.HasPrefix(rel, "../") {
				return fmt.Errorf("action %s has invalid authority target %s", action.ID, rel)
			}
			allowed[rel] = struct{}{}
			if _, ok := authority[action.ID][rel]; !ok {
				return fmt.Errorf("action %s has no accepted authority for rollback target %s", action.ID, rel)
			}
		}
		applicable[action.ID] = allowed
	}
	for actionID, byPath := range authority {
		allowed, ok := applicable[actionID]
		if !ok {
			return fmt.Errorf("accepted authority exists outside applicable action %s", actionID)
		}
		for rel, target := range byPath {
			if _, ok := allowed[rel]; !ok {
				return fmt.Errorf("accepted authority for %s is outside rollback scope: %s", actionID, rel)
			}
			if target.kind == acceptedFileTarget && !target.file.Tracked() {
				return fmt.Errorf("accepted file authority for %s target %s is untracked", actionID, rel)
			}
			if target.kind == acceptedFileTarget {
				if target.file.Exists() && sha256.Sum256(target.data) != target.file.Digest() {
					return fmt.Errorf("accepted file bytes for %s target %s do not match its revision", actionID, rel)
				}
				if !target.file.Exists() && len(target.data) != 0 {
					return fmt.Errorf("accepted absent file %s target %s carries unexpected bytes", actionID, rel)
				}
			}
			if target.kind != acceptedFileTarget && target.kind != acceptedDirectoryTarget {
				return fmt.Errorf("accepted authority for %s target %s has invalid kind", actionID, rel)
			}
			if !target.parents.Tracked() {
				return fmt.Errorf("accepted parent-chain authority for %s target %s is untracked", actionID, rel)
			}
		}
	}
	return nil
}

func enabledHelpers(values map[string]bool) []string {
	var enabled []string
	for _, name := range []string{"caff", "hk", "sshh"} {
		if values[name] {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

func scriptsForPlan(name string) []byte {
	// Kept behind this helper so the planner and installer compare the exact
	// embedded bytes. Unknown names are never scheduled by enabledHelpers.
	return []byte(scripts.GetScript(name))
}

func installerConfigSpecs(home, theme string, cfg DeepDiveConfig, allowBtopThemeReplacement bool) ([]configPlanSpec, error) {
	yaziConfigPaths, err := tools.ResolveYaziConfigPaths()
	if err != nil {
		return nil, fmt.Errorf("resolve Yazi config paths: %w", err)
	}
	return installerConfigSpecsAtResolved(home, theme, cfg, allowBtopThemeReplacement, yaziConfigPaths)
}

func installerConfigSpecsAtResolved(home, theme string, cfg DeepDiveConfig, allowBtopThemeReplacement bool, yaziConfigPaths tools.YaziConfigPaths) ([]configPlanSpec, error) {
	tmuxPath, err := tools.TmuxConfigMutationPath()
	if err != nil {
		return nil, fmt.Errorf("resolve tmux mutation path: %w", err)
	}
	tmuxTargets := []string{planTargetPath(home, tmuxPath)}
	if cfg.TmuxTPMEnabled {
		tmuxTargets = append(tmuxTargets, ".tmux/plugins/tpm")
	}
	specs := []configPlanSpec{{toolID: "tmux", targets: tmuxTargets, ownership: operation.OwnershipManagedFragment, description: "merge managed tmux settings and install selected TPM plugins"}}
	if cfg.CLITools["claude-code"] || cfg.Utilities["claude-code"] {
		specs = append(specs, configPlanSpec{toolID: "claude-code", targets: []string{".claude.json"}, ownership: operation.OwnershipManagedFragment, description: "merge selected Claude Code MCP servers"})
	}
	ghosttyPath, err := tools.GhosttyConfigMutationPath()
	if err != nil {
		return nil, fmt.Errorf("resolve Ghostty mutation path: %w", err)
	}
	ghosttyTarget := planTargetPath(home, ghosttyPath)
	specs = append(specs,
		configPlanSpec{toolID: "ghostty", targets: []string{ghosttyTarget}, ownership: operation.OwnershipManagedFragment, description: "merge managed Ghostty settings"},
		configPlanSpec{toolID: "zsh", targets: []string{".zshrc"}, ownership: operation.OwnershipManagedFragment, description: "merge managed Zsh settings"},
	)
	if cfg.NeovimConfig != "custom" {
		specs = append(specs, configPlanSpec{toolID: "neovim", targets: []string{".config/nvim"}, ownership: operation.OwnershipManagedFile, description: "install selected Neovim preset", fullFilePolicy: true})
	}
	specs = append(specs,
		configPlanSpec{toolID: "git", targets: []string{".gitconfig", ".config/dotfiles/git/config"}, ownership: operation.OwnershipManagedFragment, description: "install managed Git include"},
		configPlanSpec{actionID: "config:yazi:main", toolID: "yazi", yaziKind: tools.YaziFileKindMain, targets: []string{planTargetPath(home, yaziConfigPaths.Main)}, ownership: operation.OwnershipManagedFile, description: "write managed Yazi main configuration", fullFilePolicy: true},
		configPlanSpec{actionID: "config:yazi:keymap", toolID: "yazi", yaziKind: tools.YaziFileKindKeymap, targets: []string{planTargetPath(home, yaziConfigPaths.Keymap)}, ownership: operation.OwnershipManagedFile, description: "write managed Yazi keymap configuration", fullFilePolicy: true},
		configPlanSpec{actionID: "config:yazi:theme", toolID: "yazi", yaziKind: tools.YaziFileKindTheme, targets: []string{planTargetPath(home, yaziConfigPaths.Theme)}, ownership: operation.OwnershipManagedFile, description: "write managed Yazi theme configuration", fullFilePolicy: true},
		configPlanSpec{toolID: "fzf", targets: []string{".config/fzf/fzf.zsh"}, ownership: operation.OwnershipManagedFile, description: "write managed fzf configuration", fullFilePolicy: true},
	)
	if cfg.CLITools["lazygit"] {
		path, blockReason := lazyGitPlanTarget(home)
		if err := tools.ValidateLazyGitConfig(lazygitConfigFrom(cfg), theme); err != nil && blockReason == "" {
			blockReason = err.Error()
		}
		specs = append(specs, configPlanSpec{toolID: "lazygit", targets: []string{path}, ownership: operation.OwnershipManagedFile, description: "write managed LazyGit configuration", fullFilePolicy: true, preflightBlockReason: blockReason})
	}
	if cfg.CLITools["btop"] {
		btopCfg := btopConfigFrom(cfg)
		if err := tools.ValidateBtopConfig(btopCfg, theme); err != nil {
			return nil, fmt.Errorf("validate planned btop configuration: %w", err)
		}
		configPath, err := tools.BtopConfigMutationPath()
		if err != nil {
			return nil, fmt.Errorf("resolve planned btop config: %w", err)
		}
		themePath, err := tools.BtopThemeMutationPath(btopCfg, theme)
		if err != nil {
			return nil, fmt.Errorf("resolve planned btop theme: %w", err)
		}
		configTarget, themeTarget := planTargetPath(home, configPath), planTargetPath(home, themePath)
		specs = append(specs, configPlanSpec{toolID: "btop", targets: []string{configTarget, themeTarget}, ownership: operation.OwnershipManagedSet, description: "merge managed btop settings and write generated theme", targetOwnership: map[string]operation.Ownership{configTarget: operation.OwnershipManagedFragment, themeTarget: operation.OwnershipManagedFile}, currentTheme: theme, allowBtopThemeReplacement: allowBtopThemeReplacement})
	}
	if cfg.CLITools["glow"] {
		if err := tools.ValidateGlowConfig(glowConfigFrom(cfg), theme); err != nil {
			return nil, fmt.Errorf("validate planned Glow configuration: %w", err)
		}
		path, err := tools.GlowConfigMutationPath()
		if err != nil {
			return nil, fmt.Errorf("resolve planned Glow config: %w", err)
		}
		specs = append(specs, configPlanSpec{toolID: "glow", targets: []string{planTargetPath(home, path)}, ownership: operation.OwnershipManagedFragment, description: "merge managed Glow settings"})
	}
	return specs, nil
}

// lazyGitPlanTarget keeps reviewed planning on the same dynamic global target
// as the writer. When source discovery itself is unsafe, the action is still
// rendered as blocked instead of turning the confirmation screen into an
// opaque planning error; the fallback is observed only and can never execute.
func lazyGitPlanTarget(home string) (string, string) {
	path, err := tools.LazyGitConfigMutationPath()
	if err == nil {
		return planTargetPath(home, path), ""
	}
	if imported, importErr := tools.ImportLazyGitConfig(); importErr == nil {
		for _, source := range imported.Sources {
			if source.Active && source.Path != "" {
				return planTargetPath(home, source.Path), err.Error()
			}
		}
	}
	return ".config/lazygit/config.yml", err.Error()
}

func planTargetPath(home, absolute string) string {
	rel, err := filepath.Rel(home, absolute)
	if err == nil && rel != "." && rel != "" && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.Clean(absolute)
}

func planConfigAction(home string, spec configPlanSpec, desiredDigest string) (operation.Action, map[string]acceptedTarget, error) {
	backups := make([]string, 0, len(spec.targets))
	authority := make(map[string]acceptedTarget, len(spec.targets))
	combined := operation.Observation{Source: strings.Join(spec.targets, ",")}
	observations := make([]operation.Observation, 0, len(spec.targets))
	allExistingManaged := true
	disposition := operation.DispositionApply
	reason := ""
	if spec.preflightBlockReason != "" {
		disposition = operation.DispositionBlocked
		reason = spec.preflightBlockReason
	}
	for _, rel := range spec.targets {
		external := filepath.IsAbs(rel)
		absolute := filepath.Join(home, filepath.FromSlash(rel))
		if external {
			disposition = operation.DispositionBlocked
			if reason == "" {
				reason = "configuration outside HOME cannot yet receive a verified rollback point"
			}
			continue
		}
		backups = append(backups, rel)
		info, statErr := os.Lstat(absolute)
		if spec.toolID == "neovim" && rel == ".config/nvim/init.lua" {
			switch {
			case errors.Is(statErr, os.ErrNotExist):
				disposition = operation.DispositionBlocked
				reason = "Neovim settings require an existing regular init.lua; preset installation remains a separate reviewed action"
			case statErr == nil && !info.Mode().IsRegular():
				disposition = operation.DispositionBlocked
				reason = "Neovim settings require init.lua to be a regular file"
				observations = append(observations, operation.Observation{Source: rel})
				continue
			}
		}
		if statErr == nil && info.IsDir() {
			if external {
				return operation.Action{}, nil, fmt.Errorf("external directory config target is unsupported: %s", absolute)
			}
			snapshot, parents, snapshotErr := safefile.ObserveDirectoryWithin(home, filepath.ToSlash(rel))
			if snapshotErr != nil {
				return operation.Action{}, nil, fmt.Errorf("snapshot %s target %s: %w", spec.toolID, rel, snapshotErr)
			}
			digest := snapshot.Digest()
			combined.Exists = true
			allExistingManaged = false
			observations = append(observations, operation.Observation{Exists: true, Source: rel, Digest: hex.EncodeToString(digest[:])})
			authority[rel] = acceptedTarget{kind: acceptedDirectoryTarget, directory: snapshot, parents: parents}
			if spec.fullFilePolicy {
				disposition = operation.DispositionBlocked
				reason = "an existing managed-directory target has no ownership manifest"
			}
			continue
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return operation.Action{}, nil, fmt.Errorf("observe %s target %s: %w", spec.toolID, rel, statErr)
		}
		if statErr == nil && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return operation.Action{}, nil, fmt.Errorf("observe %s target %s: unsupported file type", spec.toolID, rel)
		}
		if errors.Is(statErr, os.ErrNotExist) && plannedDirectoryConfigTarget(spec.toolID, rel) {
			snapshot, parents, observeErr := safefile.ObserveDirectoryWithin(home, filepath.ToSlash(rel))
			if !errors.Is(observeErr, os.ErrNotExist) || snapshot != nil {
				return operation.Action{}, nil, fmt.Errorf("observe absent %s directory target %s: %w", spec.toolID, rel, observeErr)
			}
			observations = append(observations, operation.Observation{Source: rel})
			authority[rel] = acceptedTarget{kind: acceptedDirectoryTarget, parents: parents}
			allExistingManaged = false
			continue
		}
		content, revision, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
		if err != nil {
			return operation.Action{}, nil, fmt.Errorf("observe %s target %s: %w", spec.toolID, rel, err)
		}
		observed := tools.ConfigOwnershipObservation{Exists: revision.Exists()}
		if revision.Exists() {
			digest := revision.Digest()
			observed.Digest = hex.EncodeToString(digest[:])
			observed.Managed = tools.IsManagedGeneratedConfigContent(content)
		}
		authority[rel] = acceptedTarget{kind: acceptedFileTarget, file: revision, parents: parents, data: slices.Clone(content)}
		if spec.toolID == "btop" && spec.targetOwnership[rel] == operation.OwnershipManagedFragment {
			imported, importErr := tools.InspectBtopConfigContent(absolute, content, revision.Exists())
			switch {
			case importErr != nil:
				disposition = operation.DispositionBlocked
				reason = "native btop configuration cannot be merged safely: " + importErr.Error()
			case len(imported.Warnings) != 0:
				disposition = operation.DispositionBlocked
				reason = "native btop configuration cannot be merged safely: " + strings.Join(imported.Warnings, "; ")
			case btopImportedThemeNeedsReplacement(imported, spec.currentTheme) && !spec.allowBtopThemeReplacement:
				disposition = operation.DispositionBlocked
				reason = "native btop color_theme cannot be represented by the dashboard without explicit replacement"
			}
			observed.Managed = imported.Managed
		}
		if spec.yaziKind != "" {
			inspected := tools.InspectYaziConfigContent(spec.yaziKind, absolute, content, revision.Exists())
			observed.Managed = inspected.Ownership == tools.YaziOwnershipExactCurrent
			switch inspected.Ownership {
			case tools.YaziOwnershipMissing, tools.YaziOwnershipExactCurrent:
			case tools.YaziOwnershipNative, tools.YaziOwnershipExactHistorical, tools.YaziOwnershipMalformed:
				disposition = operation.DispositionBlocked
				detail := inspected.ReadOnlyReason
				if inspected.Error != "" {
					detail = inspected.Error
				}
				if detail == "" {
					detail = "ownership is read-only"
				}
				reason = fmt.Sprintf("%s has %s ownership: %s", filepath.Base(absolute), inspected.Ownership, detail)
			}
		}
		if spec.toolID == "glow" && spec.ownership == operation.OwnershipManagedFragment {
			imported, importErr := tools.InspectGlowConfigContent(absolute, content, revision.Exists())
			switch {
			case importErr != nil:
				disposition = operation.DispositionBlocked
				reason = "native Glow configuration cannot be merged safely: " + importErr.Error()
			case len(imported.Warnings) != 0:
				disposition = operation.DispositionBlocked
				reason = "native Glow configuration cannot be merged safely: " + strings.Join(imported.Warnings, "; ")
			}
			observed.Managed = imported.Managed
		}
		if spec.toolID == "lazygit" {
			imported, importErr := tools.InspectLazyGitConfigContent(absolute, content, revision.Exists())
			switch {
			case importErr != nil:
				disposition = operation.DispositionBlocked
				reason = "native LazyGit configuration cannot be replaced safely: " + importErr.Error()
			case imported.ReadOnlyReason != "":
				disposition = operation.DispositionBlocked
				reason = imported.ReadOnlyReason
			}
			observed.Managed = imported.Managed
		}
		if observed.Exists {
			combined.Exists = true
			if !observed.Managed {
				allExistingManaged = false
			}
		}
		observations = append(observations, operation.Observation{Exists: observed.Exists, Source: rel, Digest: observed.Digest, Managed: observed.Managed})
		managedWholeFile := spec.fullFilePolicy || spec.targetOwnership[rel] == operation.OwnershipManagedFile || (spec.toolID == "neovim" && rel == ".config/nvim/lua/custom/options.lua")
		if managedWholeFile && observed.Exists && !observed.Managed {
			if disposition != operation.DispositionBlocked {
				disposition = operation.DispositionBlocked
				reason = "an existing config is not marked as a dotfiles-managed file"
			}
		}
	}
	combined.Managed = combined.Exists && allExistingManaged
	combined.Digest = digestPlanValue(observations)
	actionID := spec.actionID
	if actionID == "" {
		actionID = "config:" + spec.toolID
	}
	return operation.Action{
		ID:              actionID,
		Kind:            operation.KindWriteConfig,
		ToolID:          spec.toolID,
		Target:          strings.Join(spec.targets, ", "),
		Description:     spec.description,
		Disposition:     disposition,
		Reason:          reason,
		DesiredDigest:   desiredDigest,
		Ownership:       spec.ownership,
		TargetOwnership: maps.Clone(spec.targetOwnership),
		Reversibility:   operation.ReversibilityBackup,
		BackupTargets:   backups,
		Observation:     combined,
		Observations:    observations,
	}, authority, nil
}

func plannedDirectoryConfigTarget(toolID, rel string) bool {
	return (toolID == "neovim" && rel == ".config/nvim") ||
		(toolID == "tmux" && filepath.ToSlash(rel) == ".tmux/plugins/tpm")
}

func digestPlanValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("canonical plan value is not JSON-marshalable: %v", err))
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func observeRelativeFile(home, rel string) (operation.Observation, safefile.Revision, *safefile.ParentChain, []byte, error) {
	data, revision, parents, err := safefile.ObserveFileWithin(home, filepath.ToSlash(rel))
	if err != nil {
		return operation.Observation{}, safefile.Revision{}, nil, nil, err
	}
	return observationFromFileRevision(rel, revision, false), revision, parents, slices.Clone(data), nil
}

func observationFromFileRevision(rel string, revision safefile.Revision, managed bool) operation.Observation {
	observation := operation.Observation{Source: rel}
	if revision.Exists() {
		digest := revision.Digest()
		observation.Exists = true
		observation.Digest = hex.EncodeToString(digest[:])
		observation.Managed = managed
	}
	return observation
}

func revalidateInstallPlan(plan *installPlan) error {
	return revalidateInstallPlanWithCreated(plan, nil)
}

func revalidateInstallPlanWithCreated(plan *installPlan, created map[string]*safefile.DirectorySnapshot) error {
	if plan == nil {
		return fmt.Errorf("no accepted plan")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	for _, action := range plan.actions() {
		if action.Disposition != operation.DispositionApply {
			continue
		}
		for rel, expected := range plan.authority[action.ID] {
			boundParents, bindErr := safefile.ValidateParentChainWithin(home, rel, expected.parents, created)
			if bindErr != nil {
				return fmt.Errorf("action %s target %s parent authority changed after preview: %w", action.ID, rel, bindErr)
			}
			var verifyErr error
			switch expected.kind {
			case acceptedFileTarget:
				_, current, parents, err := safefile.ObserveFileWithin(home, rel)
				if err != nil {
					verifyErr = err
				} else if current != expected.file || !safefile.SameParentChain(parents, boundParents) {
					verifyErr = safefile.ErrRevisionChanged
				}
			case acceptedDirectoryTarget:
				current, parents, observeErr := safefile.ObserveDirectoryWithin(home, rel)
				if expected.directory == nil && errors.Is(observeErr, os.ErrNotExist) && safefile.SameParentChain(parents, boundParents) {
					verifyErr = nil
				} else if observeErr != nil {
					verifyErr = observeErr
				} else if !safefile.SameParentChain(parents, boundParents) || current == nil || expected.directory == nil || current.Digest() != expected.directory.Digest() || !safefile.SameDirectoryRootState(current, expected.directory) {
					verifyErr = safefile.ErrDirectoryChanged
				}
			default:
				verifyErr = fmt.Errorf("invalid accepted authority kind")
			}
			if verifyErr != nil {
				return fmt.Errorf("action %s target %s changed after preview: %w", action.ID, rel, verifyErr)
			}
		}
	}
	return nil
}

func bindInstallPlanAuthority(home string, plan *installPlan, created map[string]*safefile.DirectorySnapshot) (map[string]map[string]acceptedTarget, error) {
	bound := make(map[string]map[string]acceptedTarget, len(plan.authority))
	for actionID, targets := range plan.authority {
		bound[actionID] = make(map[string]acceptedTarget, len(targets))
		for rel, accepted := range targets {
			parents, err := safefile.BindParentChainWithin(home, rel, accepted.parents, created)
			if err != nil {
				return nil, fmt.Errorf("bind %s target %s parent authority: %w", actionID, rel, err)
			}
			accepted.parents = parents
			bound[actionID][rel] = accepted
		}
	}
	return bound, nil
}
