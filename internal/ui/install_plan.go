package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/scripts"
	"github.com/tekierz/dotfiles/internal/tools"
)

const gitManagedConfigRelForPlan = ".config/dotfiles/git/config"

// installPlan is the immutable production snapshot shared by preview, backup,
// execution, and summary. DeepDiveConfig is already deeply cloned before it is
// stored here; all slice/map accessors return copies.
type installPlan struct {
	document      operation.Plan
	selectedTools []string
	configTools   []string
	config        DeepDiveConfig
	theme         string
	navStyle      string
	animations    bool
	globalConfig  *config.GlobalConfig
	authority     map[string]map[string]acceptedTarget
	parentDirs    []string
	statePlan     *operation.StatePlan
	// ghosttyConfigTarget is the exact absolute destination accepted during
	// planning. Execution must not rediscover a different higher-precedence file
	// after preview/revalidation.
	ghosttyConfigTarget string
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
	return p.document.Hash()
}

func (p *installPlan) actions() []operation.Action {
	if p == nil {
		return nil
	}
	return p.document.Actions()
}

func (p *installPlan) backupTargets() []string {
	if p == nil {
		return nil
	}
	return p.document.BackupTargets()
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
	return slices.Clone(p.selectedTools)
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
	toolID         string
	targets        []string
	ownership      operation.Ownership
	description    string
	fullFilePolicy bool
}

func (a *App) refreshPendingInstallPlan() {
	if a == nil || !a.manageInstalledReady || a.installCacheLoading {
		return
	}
	plan, err := buildInstallPlan(a, defaultToolInstallRuntime(), time.Now())
	a.pendingInstallPlan = plan
	a.installPlanError = err
	a.installPlanScroll = 0
}

func (a *App) invalidatePendingInstallPlan() {
	a.pendingInstallPlan = nil
	a.installPlanError = nil
}

func buildInstallPlan(a *App, installRuntime toolInstallRuntime, now time.Time) (*installPlan, error) {
	if a == nil || a.deepDiveConfig == nil {
		return nil, fmt.Errorf("cannot build install plan without installer state")
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
	cfg := snapshotDeepDiveConfig(a.deepDiveConfig)
	selected := a.collectSelectedToolsWithRuntime(installRuntime)
	sort.Strings(selected)
	platform := installRuntime.detectPlatform()

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
	for _, toolID := range selected {
		tool, ok := installRuntime.lookupTool(toolID)
		if !ok {
			return nil, fmt.Errorf("selected tool %s disappeared while planning", toolID)
		}
		actions = append(actions, operation.Action{
			ID:          "install:" + toolID,
			Kind:        operation.KindInstallTool,
			ToolID:      toolID,
			Target:      toolID,
			Description: "install or verify " + toolID,
			Disposition: operation.DispositionApply,
			DesiredDigest: digestPlanValue(struct {
				ToolID   string
				Platform string
				Packages []string
			}{toolID, string(platform), tools.PackagesForPlatform(tool.Packages(), platform)}),
			Ownership:     operation.OwnershipPackageManager,
			Reversibility: operation.ReversibilityManual,
		})
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

	configSpecs, err := installerConfigSpecs(home, a.theme, cfg)
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
		actions = append(actions, action)
		if action.Disposition == operation.DispositionApply {
			configTools = append(configTools, spec.toolID)
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
	return &installPlan{
		document:            document,
		selectedTools:       slices.Clone(selected),
		configTools:         configTools,
		config:              cfg,
		theme:               a.theme,
		navStyle:            a.navStyle,
		animations:          a.animationsEnabled,
		globalConfig:        plannedGlobal,
		authority:           authority,
		parentDirs:          slices.Clone(parents),
		statePlan:           statePlan,
		ghosttyConfigTarget: ghosttyConfigTarget,
	}, nil
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

func installerConfigSpecs(home, theme string, cfg DeepDiveConfig) ([]configPlanSpec, error) {
	tmuxPath, err := tools.TmuxConfigMutationPath()
	if err != nil {
		return nil, fmt.Errorf("resolve tmux mutation path: %w", err)
	}
	tmuxTargets := []string{planTargetPath(home, tmuxPath)}
	if cfg.TmuxTPMEnabled {
		tmuxTargets = append(tmuxTargets, ".tmux/plugins/tpm")
	}
	specs := []configPlanSpec{{"tmux", tmuxTargets, operation.OwnershipManagedFragment, "merge managed tmux settings and install selected TPM plugins", false}}
	if cfg.CLITools["claude-code"] || cfg.Utilities["claude-code"] {
		specs = append(specs, configPlanSpec{"claude-code", []string{".claude.json"}, operation.OwnershipManagedFragment, "merge selected Claude Code MCP servers", false})
	}
	ghosttyPath, err := tools.GhosttyConfigMutationPath()
	if err != nil {
		return nil, fmt.Errorf("resolve Ghostty mutation path: %w", err)
	}
	ghosttyTarget := planTargetPath(home, ghosttyPath)
	specs = append(specs,
		configPlanSpec{"ghostty", []string{ghosttyTarget}, operation.OwnershipManagedFragment, "merge managed Ghostty settings", false},
		configPlanSpec{"zsh", []string{".zshrc"}, operation.OwnershipManagedFragment, "merge managed Zsh settings", false},
	)
	if cfg.NeovimConfig != "custom" {
		specs = append(specs, configPlanSpec{"neovim", []string{".config/nvim"}, operation.OwnershipManagedFile, "install selected Neovim preset", true})
	}
	specs = append(specs,
		configPlanSpec{"git", []string{".gitconfig", ".config/dotfiles/git/config"}, operation.OwnershipManagedFragment, "install managed Git include", false},
		configPlanSpec{"yazi", []string{".config/yazi/yazi.toml", ".config/yazi/keymap.toml", ".config/yazi/theme.toml"}, operation.OwnershipManagedFile, "write managed Yazi configuration", true},
		configPlanSpec{"fzf", []string{".config/fzf/fzf.zsh"}, operation.OwnershipManagedFile, "write managed fzf configuration", true},
	)
	if cfg.CLITools["lazygit"] {
		specs = append(specs, configPlanSpec{"lazygit", []string{".config/lazygit/config.yml"}, operation.OwnershipManagedFile, "write managed LazyGit configuration", true})
	}
	if cfg.CLITools["btop"] {
		btopCfg := btopConfigFrom(cfg)
		if err := tools.ValidateBtopConfig(btopCfg, theme); err != nil {
			return nil, fmt.Errorf("validate planned btop configuration: %w", err)
		}
		artifactName := tools.BtopThemeArtifactName(btopCfg, theme)
		specs = append(specs, configPlanSpec{"btop", []string{".config/btop/btop.conf", filepath.ToSlash(filepath.Join(".config", "btop", "themes", artifactName+".theme"))}, operation.OwnershipManagedFile, "write managed btop configuration", true})
	}
	if cfg.CLITools["glow"] {
		if err := tools.ValidateGlowConfig(glowConfigFrom(cfg), theme); err != nil {
			return nil, fmt.Errorf("validate planned Glow configuration: %w", err)
		}
		paths := tools.NewGlowTool().ConfigPaths()
		if len(paths) != 1 {
			return nil, fmt.Errorf("glow registry returned %d config paths", len(paths))
		}
		specs = append(specs, configPlanSpec{"glow", []string{planTargetPath(home, paths[0])}, operation.OwnershipManagedFile, "write managed Glow configuration", true})
	}
	return specs, nil
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
	for _, rel := range spec.targets {
		external := filepath.IsAbs(rel)
		absolute := filepath.Join(home, filepath.FromSlash(rel))
		if external {
			disposition = operation.DispositionBlocked
			reason = "configuration outside HOME cannot yet receive a verified rollback point"
			continue
		}
		backups = append(backups, rel)
		info, statErr := os.Lstat(absolute)
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
		if observed.Exists {
			combined.Exists = true
			if !observed.Managed {
				allExistingManaged = false
			}
		}
		observations = append(observations, operation.Observation{Exists: observed.Exists, Source: rel, Digest: observed.Digest, Managed: observed.Managed})
		if spec.fullFilePolicy && observed.Exists && !observed.Managed {
			migratable := false
			if spec.toolID == "yazi" && filepath.Base(rel) == "theme.toml" {
				migratable = tools.IsLegacyGeneratedYaziThemeContent(content)
			}
			if !migratable {
				disposition = operation.DispositionBlocked
				reason = "an existing config is not marked as a dotfiles-managed file"
			}
		}
	}
	combined.Managed = combined.Exists && allExistingManaged
	combined.Digest = digestPlanValue(observations)
	return operation.Action{
		ID:            "config:" + spec.toolID,
		Kind:          operation.KindWriteConfig,
		ToolID:        spec.toolID,
		Target:        strings.Join(spec.targets, ", "),
		Description:   spec.description,
		Disposition:   disposition,
		Reason:        reason,
		DesiredDigest: desiredDigest,
		Ownership:     spec.ownership,
		Reversibility: operation.ReversibilityBackup,
		BackupTargets: backups,
		Observation:   combined,
		Observations:  observations,
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
