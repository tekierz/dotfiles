package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

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
	// ghosttyConfigTarget is the exact absolute destination accepted during
	// planning. Execution must not rediscover a different higher-precedence file
	// after preview/revalidation.
	ghosttyConfigTarget string
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
	if _, err := config.LoadGlobalConfig(); err != nil {
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
		globalObservation, err = observeRelativeFile(home, globalTarget)
		if err != nil {
			return nil, fmt.Errorf("observe global settings: %w", err)
		}
		globalBackupTarget = globalTarget
		globalObservations = []operation.Observation{globalObservation}
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
		observation, err := observeRelativeFile(home, rel)
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
	}

	configSpecs, err := installerConfigSpecs(home, a.theme, cfg)
	if err != nil {
		return nil, err
	}
	configTools := make([]string, 0, len(configSpecs))
	ghosttyConfigTarget := ""
	for _, spec := range configSpecs {
		action, err := planConfigAction(home, spec, digestPlanValue(struct {
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
		actions = append(actions, action)
		if action.Disposition == operation.DispositionApply {
			configTools = append(configTools, spec.toolID)
		}
	}

	document, err := operation.NewPlan(now, actions)
	if err != nil {
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
		ghosttyConfigTarget: ghosttyConfigTarget,
	}, nil
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
	tmuxTargets := []string{".tmux.conf"}
	if cfg.TmuxTPMEnabled {
		tmuxTargets = append(tmuxTargets, ".tmux/plugins/tpm")
	}
	specs := []configPlanSpec{{"tmux", tmuxTargets, operation.OwnershipManagedFile, "write managed tmux configuration and selected TPM plugins", true}}
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
		specs = append(specs, configPlanSpec{"btop", []string{".config/btop/btop.conf", filepath.ToSlash(filepath.Join(".config", "btop", "themes", theme+".theme"))}, operation.OwnershipManagedFile, "write managed btop configuration", true})
	}
	if cfg.CLITools["glow"] {
		paths := tools.NewGlowTool().ConfigPaths()
		if len(paths) != 1 {
			return nil, fmt.Errorf("Glow registry returned %d config paths", len(paths))
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

func planConfigAction(home string, spec configPlanSpec, desiredDigest string) (operation.Action, error) {
	backups := make([]string, 0, len(spec.targets))
	combined := operation.Observation{Source: strings.Join(spec.targets, ",")}
	observations := make([]operation.Observation, 0, len(spec.targets))
	allExistingManaged := true
	disposition := operation.DispositionApply
	reason := ""
	for _, rel := range spec.targets {
		external := filepath.IsAbs(rel)
		absolute := filepath.Join(home, filepath.FromSlash(rel))
		if external {
			absolute = filepath.Clean(rel)
			disposition = operation.DispositionBlocked
			reason = "configuration outside HOME cannot yet receive a verified rollback point"
			continue
		}
		backups = append(backups, rel)
		info, statErr := os.Lstat(absolute)
		if statErr == nil && info.IsDir() {
			if external {
				return operation.Action{}, fmt.Errorf("external directory config target is unsupported: %s", absolute)
			}
			snapshot, snapshotErr := safefile.SnapshotDirectoryWithin(home, filepath.ToSlash(rel))
			if snapshotErr != nil {
				return operation.Action{}, fmt.Errorf("snapshot %s target %s: %w", spec.toolID, rel, snapshotErr)
			}
			digest := snapshot.Digest()
			combined.Exists = true
			allExistingManaged = false
			observations = append(observations, operation.Observation{Exists: true, Source: rel, Digest: hex.EncodeToString(digest[:])})
			if spec.fullFilePolicy {
				disposition = operation.DispositionBlocked
				reason = "an existing managed-directory target has no ownership manifest"
			}
			continue
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return operation.Action{}, fmt.Errorf("observe %s target %s: %w", spec.toolID, rel, statErr)
		}
		if statErr == nil && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return operation.Action{}, fmt.Errorf("observe %s target %s: unsupported file type", spec.toolID, rel)
		}
		observed, err := tools.ObserveGeneratedConfig(absolute)
		if err != nil {
			return operation.Action{}, fmt.Errorf("observe %s target %s: %w", spec.toolID, rel, err)
		}
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
				migratable, err = tools.IsLegacyGeneratedYaziThemeConfig(absolute)
				if err != nil {
					return operation.Action{}, fmt.Errorf("inspect legacy Yazi theme %s: %w", rel, err)
				}
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
	}, nil
}

func digestPlanValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("canonical plan value is not JSON-marshalable: %v", err))
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func observeRelativeFile(home, rel string) (operation.Observation, error) {
	data, revision, err := safefile.ReadWithin(home, filepath.ToSlash(rel))
	if err != nil {
		return operation.Observation{}, err
	}
	if !revision.Exists() {
		return operation.Observation{Source: rel}, nil
	}
	digest := sha256.Sum256(data)
	return operation.Observation{Exists: true, Source: rel, Digest: hex.EncodeToString(digest[:])}, nil
}

func revalidateInstallPlan(plan *installPlan) error {
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
		for _, expected := range action.Observations {
			if filepath.IsAbs(expected.Source) {
				return fmt.Errorf("action %s targets unsupported external path %s", action.ID, expected.Source)
			}
			current, err := observePlanTarget(home, expected.Source)
			if err != nil {
				return fmt.Errorf("revalidate action %s target %s: %w", action.ID, expected.Source, err)
			}
			if current != expected {
				return fmt.Errorf("action %s target %s changed after preview", action.ID, expected.Source)
			}
		}
	}
	return nil
}

func observePlanTarget(home, rel string) (operation.Observation, error) {
	absolute := filepath.Join(home, filepath.FromSlash(rel))
	info, err := os.Lstat(absolute)
	if err == nil && info.IsDir() {
		snapshot, err := safefile.SnapshotDirectoryWithin(home, filepath.ToSlash(rel))
		if err != nil {
			return operation.Observation{}, err
		}
		digest := snapshot.Digest()
		return operation.Observation{Exists: true, Source: rel, Digest: hex.EncodeToString(digest[:])}, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return operation.Observation{}, err
	}
	observed, err := tools.ObserveGeneratedConfig(absolute)
	if err != nil {
		return operation.Observation{}, err
	}
	return operation.Observation{Exists: observed.Exists, Source: rel, Digest: observed.Digest, Managed: observed.Managed}, nil
}
