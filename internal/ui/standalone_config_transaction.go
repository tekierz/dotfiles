package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/tools"
)

type standaloneConfigSaveDoneMsg struct {
	err            error
	manualRecovery bool
	applied        bool
	warning        string
}

type standaloneConfigExecutionResult struct {
	err            error
	manualRecovery bool
	applied        bool
	warning        string
}

type standaloneConfigAppliedError struct{ err error }

func (e *standaloneConfigAppliedError) Error() string {
	return "configuration was applied and preserved, but an operational step failed: " + e.err.Error()
}
func (e *standaloneConfigAppliedError) Unwrap() error { return e.err }

// prepareStandaloneConfigSave freezes the editor state and builds a read-only,
// config-only plan. The first Enter never writes product or operation-state
// files; execution is reachable only from the confirmation screen.
func (a *App) prepareStandaloneConfigSave() tea.Cmd {
	plan, err := buildStandaloneConfigPlan(a, time.Now())
	a.standaloneConfigPlan = plan
	a.standaloneConfigErr = err
	a.standaloneConfigStatus = ""
	a.standaloneConfigWarning = ""
	a.standaloneConfigRunning = false
	a.standaloneConfigDone = false
	a.standaloneConfigManualRecovery = false
	return NavigateTo(ScreenConfigSaveConfirm)
}

func (a *App) executeStandaloneConfigPlanCmd() tea.Cmd {
	plan := a.standaloneConfigPlan
	return func() tea.Msg {
		result := executeStandaloneConfigPlanResult(context.Background(), plan, defaultStandaloneConfigRuntime())
		return standaloneConfigSaveDoneMsg(result)
	}
}

type standaloneConfigRuntime struct {
	backup       func(*operation.StateAuthority, []backup.Target) (autoBackupResult, error)
	acquire      func(*operation.StateAuthority, string, string) (func() error, error)
	ensureParent func(string, string, *safefile.DirectorySnapshot, *safefile.ParentChain, os.FileMode) (*safefile.DirectorySnapshot, error)
	write        func(string, DeepDiveConfig, string, map[string]acceptedTarget, operation.Locker) ([]tools.MutationEvidence, error)
}

func defaultStandaloneConfigRuntime() standaloneConfigRuntime {
	return standaloneConfigRuntime{
		backup:       backupPlanTargetsWithState,
		acquire:      operation.AcquireStateLockWithAuthority,
		ensureParent: safefile.EnsureShallowDirectoryWithinParentChainTracked,
		write:        writeStandaloneConfigAtAuthority,
	}
}

func executeStandaloneConfigPlanWithRuntime(ctx context.Context, plan *installPlan, runtime standaloneConfigRuntime) (error, bool) {
	result := executeStandaloneConfigPlanResult(ctx, plan, runtime)
	return result.err, result.manualRecovery
}

func executeStandaloneConfigPlanResult(ctx context.Context, plan *installPlan, runtime standaloneConfigRuntime) standaloneConfigExecutionResult {
	if plan == nil || plan.hasBlocked() || len(plan.configTools) != 1 {
		return standaloneConfigExecutionResult{err: fmt.Errorf("standalone config plan is blocked or has no applicable write")}
	}
	if runtime.write == nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("standalone config writer is unavailable")}
	}
	return executeConfigTransactionResult(ctx, plan, runtime, "standalone-config-operation", func(home string, plan *installPlan, bound map[string]map[string]acceptedTarget, locker operation.Locker, expected map[string]backup.ExpectedState) (bool, bool, error) {
		toolID := plan.configTools[0]
		return executeConfigTransactionAction(home, plan, "config:"+toolID, expected, func() ([]tools.MutationEvidence, error) {
			return runtime.write(toolID, plan.config, plan.theme, bound["config:"+toolID], locker)
		})
	})
}

type configTransactionActionRunner func(home string, plan *installPlan, bound map[string]map[string]acceptedTarget, locker operation.Locker, expected map[string]backup.ExpectedState) (mutationStarted bool, manualRecovery bool, err error)

func executeConfigTransactionResult(ctx context.Context, plan *installPlan, runtime standaloneConfigRuntime, lockScope string, runActions configTransactionActionRunner) standaloneConfigExecutionResult {
	if plan == nil || plan.hasBlocked() {
		return standaloneConfigExecutionResult{err: fmt.Errorf("config transaction plan is blocked or unavailable")}
	}
	if runtime.backup == nil || runtime.acquire == nil || runtime.ensureParent == nil || runActions == nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("config transaction runtime is incomplete")}
	}
	if err := ctx.Err(); err != nil {
		return standaloneConfigExecutionResult{err: err}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("determine HOME for config transaction: %w", err)}
	}
	if home == "" || !filepath.IsAbs(home) {
		return standaloneConfigExecutionResult{err: fmt.Errorf("determine HOME for config transaction: %q is not absolute", home)}
	}
	if err := revalidateInstallPlan(plan); err != nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("configuration changed after preview: %w", err)}
	}
	state, err := operation.BootstrapStateNamespaceTracked(plan.statePlan)
	if err != nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("bootstrap reviewed operation state: %w", err)}
	}
	stateCreated := state.CreatedDirectoriesWithin(home)
	release, err := runtime.acquire(state, lockScope, home)
	if err != nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("serialize config transaction: %w", err)}
	}
	result := executeConfigTransactionLocked(ctx, plan, runtime, state, stateCreated, home, runActions)
	if releaseErr := release(); releaseErr != nil {
		releaseErr = fmt.Errorf("release config transaction lock: %w", releaseErr)
		if result.err == nil {
			result.err = &standaloneConfigAppliedError{err: releaseErr}
			result.applied = true
		} else {
			result.err = errors.Join(result.err, releaseErr)
		}
	}
	return result
}

func executeConfigTransactionLocked(ctx context.Context, plan *installPlan, runtime standaloneConfigRuntime, state *operation.StateAuthority, stateCreated map[string]*safefile.DirectorySnapshot, home string, runActions configTransactionActionRunner) standaloneConfigExecutionResult {
	if err := revalidateInstallPlanWithCreated(plan, stateCreated); err != nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("configuration changed before backup: %w", err)}
	}
	targets, err := plan.backupTargetSpecs()
	if err != nil {
		return standaloneConfigExecutionResult{err: err}
	}
	backupResult, err := runtime.backup(state, targets)
	if err != nil || !backupResult.enabled || backupResult.backupDir == "" || backupResult.plan == nil {
		if err == nil {
			err = fmt.Errorf("backup returned no exact PlanResult authority")
		}
		return standaloneConfigExecutionResult{err: fmt.Errorf("mandatory config backup failed before mutation: %w", err)}
	}
	if err := backup.ValidatePlanRoot(*backupResult.plan); err != nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("mandatory config backup authority is invalid: %w", err)}
	}
	warning := ""
	if backupResult.cleanupErr != nil {
		warning = "backup retention cleanup failed: " + backupResult.cleanupErr.Error()
	}
	if err := revalidateInstallPlanWithCreated(plan, stateCreated); err != nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("configuration changed while creating backup: %w", err), warning: warning}
	}

	mutationStarted := false
	expected := make(map[string]backup.ExpectedState)
	rollbackFailure := func(primary error, manual bool) standaloneConfigExecutionResult {
		err, manualRecovery := rollbackStandaloneConfigFailure(primary, manual, mutationStarted, backupResult.plan, home, expected)
		return standaloneConfigExecutionResult{err: err, manualRecovery: manualRecovery, warning: warning}
	}
	createdParents := make(map[string]*safefile.DirectorySnapshot, len(stateCreated)+len(plan.parentDirectoryTargets()))
	for rel, snapshot := range stateCreated {
		createdParents[rel] = snapshot
	}
	for _, rel := range plan.parentDirectoryTargets() {
		accepted, acceptedParents, err := plan.acceptedDirectoryAuthority("state:parents", rel)
		if err != nil {
			return rollbackFailure(fmt.Errorf("resolve accepted config parent %s: %w", rel, err), false)
		}
		boundParents, err := safefile.BindParentChainWithin(home, rel, acceptedParents, createdParents)
		if err != nil {
			return rollbackFailure(fmt.Errorf("bind accepted config parent %s: %w", rel, err), false)
		}
		expected[rel] = backup.ExpectedState{Attempted: true, Kind: backup.TargetDirectory, OriginalCaptured: true, Parents: boundParents}
		snapshot, err := runtime.ensureParent(home, rel, accepted, boundParents, 0o700)
		if err != nil {
			manual := false
			if snapshot != nil {
				mutationStarted = true
				createdParents[rel] = snapshot
				expected[rel] = backup.ExpectedState{Attempted: true, Captured: true, Kind: backup.TargetDirectory, Exists: true, DirectorySnapshot: snapshot, Parents: boundParents, OriginalCaptured: true}
			} else {
				var committed interface{ Committed() bool }
				if errors.As(err, &committed) && committed.Committed() {
					mutationStarted = true
					manual = true
				} else {
					delete(expected, rel)
				}
			}
			return rollbackFailure(fmt.Errorf("create accepted config parent %s: %w", rel, err), manual)
		}
		mutationStarted = true
		createdParents[rel] = snapshot
		expected[rel] = backup.ExpectedState{Attempted: true, Captured: true, Kind: backup.TargetDirectory, Exists: true, DirectorySnapshot: snapshot, Parents: boundParents, OriginalCaptured: true}
	}
	bound, err := bindInstallPlanAuthority(home, plan, createdParents)
	if err != nil {
		return rollbackFailure(fmt.Errorf("bind config transaction authority: %w", err), false)
	}

	actionMutation, actionManual, actionErr := runActions(home, plan, bound, operation.BoundLocker(state), expected)
	mutationStarted = mutationStarted || actionMutation
	if actionErr != nil {
		return rollbackFailure(actionErr, actionManual)
	}
	if err := ctx.Err(); err != nil {
		mutationStarted = true
		return rollbackFailure(err, false)
	}
	return standaloneConfigExecutionResult{applied: true, warning: warning}
}

func executeConfigTransactionAction(home string, plan *installPlan, actionID string, expected map[string]backup.ExpectedState, write func() ([]tools.MutationEvidence, error)) (bool, bool, error) {
	invalidateRollbackAction(plan, actionID, expected)
	evidence, writeErr := write()
	if writeErr != nil {
		manual := false
		mutationStarted := false
		var partial interface {
			CommittedEvidence() []tools.MutationEvidence
		}
		hasPartial := errors.As(writeErr, &partial) && len(partial.CommittedEvidence()) != 0
		if hasPartial {
			mutationStarted = true
			if captureErr := recordFailedActionRollbackState(home, plan, actionID, writeErr, expected); captureErr != nil {
				writeErr = errors.Join(writeErr, captureErr)
				manual = true
			}
		} else {
			var committed interface{ Committed() bool }
			if errors.As(writeErr, &committed) && committed.Committed() {
				mutationStarted = true
				manual = true
			} else {
				removeRollbackActionState(plan, actionID, expected)
			}
		}
		return mutationStarted, manual, fmt.Errorf("apply %s: %w", actionID, writeErr)
	}
	if err := authorizeMutationEvidenceSet(home, plan, actionID, evidence, expected); err != nil {
		return true, true, fmt.Errorf("authorize %s mutation evidence: %w", actionID, err)
	}
	return true, false, nil
}

func buildStandaloneConfigPlan(a *App, now time.Time) (*installPlan, error) {
	if a == nil || a.deepDiveConfig == nil {
		return nil, fmt.Errorf("standalone config editor state is unavailable")
	}
	if _, err := config.LoadGlobalConfig(); err != nil {
		return nil, fmt.Errorf("validate global config before planning: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine absolute HOME for config plan: %w", err)
	}
	if home == "" || !filepath.IsAbs(home) {
		return nil, fmt.Errorf("determine absolute HOME for config plan: %q is not absolute", home)
	}
	statePlan, err := operation.CaptureStatePlan()
	if err != nil {
		return nil, fmt.Errorf("capture operation state before config planning: %w", err)
	}
	cfg := snapshotDeepDiveConfig(a.deepDiveConfig)
	toolID, ok := toolIDForScreen(a.startScreen)
	if !ok {
		return blockedStandaloneConfigPlan(now, statePlan, cfg, a.theme, "unknown", "the selected screen has no standalone config writer")
	}

	allowBtopThemeReplacement := a.nativeConfigState.BtopThemeExplicit || cfg.BtopTheme != manageConfigToDeepDive(&a.manageConfigBaseline).BtopTheme || (cfg.BtopTheme == "auto" && a.theme != a.manageConfigBaselineTheme)
	spec, blockedReason, err := standaloneConfigPlanSpec(home, a.theme, cfg, toolID, allowBtopThemeReplacement)
	if err != nil {
		return nil, err
	}
	if blockedReason != "" {
		return blockedStandaloneConfigPlan(now, statePlan, cfg, a.theme, toolID, blockedReason)
	}
	action, actionAuthority, err := planConfigAction(home, spec, digestPlanValue(struct {
		ToolID string
		Theme  string
		Config DeepDiveConfig
	}{toolID, a.theme, cfg}))
	if err != nil {
		return nil, err
	}

	if reason := standaloneNativeBlockReason(a, toolID, allowBtopThemeReplacement); reason != "" {
		action.Disposition = operation.DispositionBlocked
		action.Reason = reason
		actionAuthority = nil
	}
	if toolID == "claude-code" {
		changed, err := claudeSelectionChanges(cfg.ClaudeCodeMCPs)
		if err != nil {
			return nil, fmt.Errorf("compare Claude MCP selection: %w", err)
		}
		if !changed {
			action.Disposition = operation.DispositionSkip
			action.Reason = "Claude MCP selection already matches the saved file"
			actionAuthority = nil
		}
	}

	actions := []operation.Action{action}
	authority := make(map[string]map[string]acceptedTarget)
	if action.Disposition == operation.DispositionApply {
		authority[action.ID] = actionAuthority
	}
	parents, parentAction, parentAuthority, err := planStandaloneConfigParents(home, actions)
	if err != nil {
		return nil, err
	}
	if parentAction != nil {
		actions = append([]operation.Action{*parentAction}, actions...)
		authority[parentAction.ID] = parentAuthority
	}
	document, err := operation.NewPlan(now, actions)
	if err != nil {
		return nil, err
	}
	if err := validateInstallPlanAuthority(actions, authority); err != nil {
		return nil, err
	}
	configTools := []string(nil)
	if action.Disposition == operation.DispositionApply {
		configTools = []string{toolID}
	}
	return &installPlan{
		document: document, configTools: configTools, config: cfg, theme: a.theme,
		authority: authority, parentDirs: parents, statePlan: statePlan,
	}, nil
}

func blockedStandaloneConfigPlan(now time.Time, statePlan *operation.StatePlan, cfg DeepDiveConfig, theme, toolID, reason string) (*installPlan, error) {
	action := operation.Action{
		ID: "config:" + toolID, Kind: operation.KindWriteConfig, ToolID: toolID,
		Target: toolID, Description: "standalone configuration is unavailable",
		Disposition: operation.DispositionBlocked, Reason: reason,
		DesiredDigest: digestPlanValue(struct{ ToolID, Theme string }{toolID, theme}),
		Ownership:     operation.OwnershipUnknown, Reversibility: operation.ReversibilityManual,
	}
	document, err := operation.NewPlan(now, []operation.Action{action})
	if err != nil {
		return nil, err
	}
	return &installPlan{document: document, config: cfg, theme: theme, authority: map[string]map[string]acceptedTarget{}, statePlan: statePlan}, nil
}

func standaloneConfigPlanSpec(home, theme string, cfg DeepDiveConfig, toolID string, allowBtopThemeReplacement bool) (configPlanSpec, string, error) {
	spec := configPlanSpec{toolID: toolID}
	switch toolID {
	case "ghostty":
		path, err := tools.GhosttyConfigMutationPath()
		if err != nil {
			return spec, "", fmt.Errorf("resolve active Ghostty config: %w", err)
		}
		spec.targets, spec.ownership, spec.description = []string{planTargetPath(home, path)}, operation.OwnershipManagedFragment, "merge managed Ghostty settings"
	case "tmux":
		path, err := tools.TmuxConfigMutationPath()
		if err != nil {
			return spec, "", fmt.Errorf("resolve active tmux config: %w", err)
		}
		spec.targets, spec.ownership, spec.description = []string{planTargetPath(home, path)}, operation.OwnershipManagedFragment, "merge managed tmux settings"
	case "zsh":
		spec.targets, spec.ownership, spec.description = []string{".zshrc"}, operation.OwnershipManagedFragment, "merge managed Zsh settings"
	case "neovim":
		spec.targets, spec.ownership, spec.description = []string{".config/nvim/init.lua", ".config/nvim/lua/custom/options.lua"}, operation.OwnershipManagedFragment, "merge managed Neovim preferences"
	case "git":
		spec.targets, spec.ownership, spec.description = []string{".gitconfig", ".config/dotfiles/git/config"}, operation.OwnershipManagedFragment, "install managed Git include"
	case "yazi":
		spec.targets, spec.ownership, spec.description, spec.fullFilePolicy = []string{".config/yazi/yazi.toml", ".config/yazi/keymap.toml", ".config/yazi/theme.toml"}, operation.OwnershipManagedFile, "write managed Yazi configuration", true
	case "fzf":
		spec.targets, spec.ownership, spec.description, spec.fullFilePolicy = []string{".config/fzf/fzf.zsh"}, operation.OwnershipManagedFile, "write managed fzf configuration", true
	case "lazygit":
		spec.targets, spec.ownership, spec.description, spec.fullFilePolicy = []string{".config/lazygit/config.yml"}, operation.OwnershipManagedFile, "write managed LazyGit configuration", true
	case "btop":
		btopCfg := btopConfigFrom(cfg)
		if err := tools.ValidateBtopConfig(btopCfg, theme); err != nil {
			return spec, "", err
		}
		configPath, err := tools.BtopConfigMutationPath()
		if err != nil {
			return spec, "", err
		}
		themePath, err := tools.BtopThemeMutationPath(btopCfg, theme)
		if err != nil {
			return spec, "", err
		}
		configTarget, themeTarget := planTargetPath(home, configPath), planTargetPath(home, themePath)
		spec.targets, spec.ownership, spec.description = []string{configTarget, themeTarget}, operation.OwnershipManagedSet, "merge managed btop settings and write generated theme"
		spec.targetOwnership = map[string]operation.Ownership{configTarget: operation.OwnershipManagedFragment, themeTarget: operation.OwnershipManagedFile}
		spec.currentTheme = theme
		spec.allowBtopThemeReplacement = allowBtopThemeReplacement
	case "glow":
		if err := tools.ValidateGlowConfig(glowConfigFrom(cfg), theme); err != nil {
			return spec, "", err
		}
		path, err := tools.GlowConfigMutationPath()
		if err != nil {
			return spec, "", err
		}
		spec.targets, spec.ownership, spec.description = []string{planTargetPath(home, path)}, operation.OwnershipManagedFragment, "merge managed Glow settings"
	case "claude-code":
		spec.targets, spec.ownership, spec.description = []string{".claude.json"}, operation.OwnershipManagedFragment, "merge selected Claude Code MCP servers"
	default:
		return spec, "the selected tool has no reviewed standalone config writer", nil
	}
	return spec, "", nil
}

func standaloneNativeBlockReason(a *App, toolID string, allowBtopThemeReplacement bool) string {
	if a.nativeConfigState.PreferenceError != "" && (toolID == "ghostty" || toolID == "tmux" || toolID == "git" || toolID == "btop" || toolID == "glow") {
		return "saved management preferences could not be read safely: " + a.nativeConfigState.PreferenceError
	}
	switch toolID {
	case "ghostty":
		if a.nativeConfigState.GhosttyError != "" {
			return "native Ghostty configuration could not be imported safely: " + a.nativeConfigState.GhosttyError
		}
	case "tmux":
		if a.nativeConfigState.TmuxError != "" {
			return "native tmux configuration could not be imported safely: " + a.nativeConfigState.TmuxError
		}
	case "git":
		if a.nativeConfigState.GitError != "" {
			return "native Git configuration could not be imported safely: " + a.nativeConfigState.GitError
		}
	case "btop":
		if a.nativeConfigState.BtopError != "" && (!a.nativeConfigState.BtopThemeUnsupported || !allowBtopThemeReplacement) {
			return "native btop configuration could not be imported safely: " + a.nativeConfigState.BtopError
		}
	case "glow":
		if a.nativeConfigState.GlowError != "" {
			return "native Glow configuration could not be imported safely: " + a.nativeConfigState.GlowError
		}
	}
	return ""
}

func claudeSelectionChanges(selection map[string]bool) (bool, error) {
	current, err := config.LoadClaudeConfig()
	if err != nil {
		return false, err
	}
	known := config.AllMCPServers()
	for name, definition := range known {
		existing, exists := current.MCPServers[name]
		if selection[name] != exists || (selection[name] && !reflect.DeepEqual(existing, definition)) {
			return true, nil
		}
	}
	return false, nil
}

func planStandaloneConfigParents(home string, actions []operation.Action) ([]string, *operation.Action, map[string]acceptedTarget, error) {
	parents, err := missingProductParentDirectories(home, actions)
	if err != nil || len(parents) == 0 {
		return parents, nil, nil, err
	}
	observations := make([]operation.Observation, 0, len(parents))
	authority := make(map[string]acceptedTarget, len(parents))
	for _, rel := range parents {
		snapshot, chain, observeErr := safefile.ObserveDirectoryWithin(home, rel)
		if !errors.Is(observeErr, os.ErrNotExist) || snapshot != nil {
			return nil, nil, nil, fmt.Errorf("observe missing config parent %s: %w", rel, observeErr)
		}
		observations = append(observations, operation.Observation{Source: rel})
		authority[rel] = acceptedTarget{kind: acceptedDirectoryTarget, parents: chain}
	}
	action := &operation.Action{
		ID: "state:parents", Kind: operation.KindUpdateState,
		Target: strings.Join(parents, ", "), Description: "create reviewed private parent directories for standalone configuration",
		Disposition: operation.DispositionApply, DesiredDigest: digestPlanValue(parents),
		Ownership: operation.OwnershipManagedFile, Reversibility: operation.ReversibilityBackup,
		BackupTargets: slices.Clone(parents), Observation: operation.Observation{Source: strings.Join(parents, ",")}, Observations: observations,
	}
	return parents, action, authority, nil
}

func rollbackStandaloneConfigFailure(primary error, manual, mutationStarted bool, plan *backup.PlanResult, home string, expected map[string]backup.ExpectedState) (error, bool) {
	if !mutationStarted {
		return primary, manual
	}
	if plan == nil {
		return errors.Join(primary, fmt.Errorf("manual recovery required: rollback PlanResult is unavailable")), true
	}
	result, err := backup.RestoreExpectedPlan(*plan, home, expected)
	if err != nil {
		return errors.Join(primary, fmt.Errorf("manual recovery required: automatic rollback failed: %w", err)), true
	}
	if len(result.Skipped) != 0 || len(result.Warnings) != 0 {
		return errors.Join(primary, fmt.Errorf("manual recovery required: rollback skipped %d target(s) with %d warning(s)", len(result.Skipped), len(result.Warnings))), true
	}
	if manual {
		return errors.Join(primary, fmt.Errorf("automatic rollback completed for proven writes, but unproven mutation evidence still requires manual review")), true
	}
	return errors.Join(primary, fmt.Errorf("automatic rollback completed")), false
}

func removeRollbackActionState(plan *installPlan, actionID string, expected map[string]backup.ExpectedState) {
	if plan == nil {
		return
	}
	for _, action := range plan.actions() {
		if action.ID != actionID {
			continue
		}
		for _, rel := range append([]string{action.BackupTarget}, action.BackupTargets...) {
			delete(expected, filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))))
		}
		return
	}
}

func standaloneFileAuthority(targets map[string]acceptedTarget, rel string) (safefile.Revision, *safefile.ParentChain, error) {
	target, ok := targets[filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))]
	if !ok || target.kind != acceptedFileTarget || !target.file.Tracked() || !target.parents.Tracked() {
		return safefile.Revision{}, nil, fmt.Errorf("accepted file authority is unavailable for %s", rel)
	}
	return target.file, target.parents, nil
}

func writeStandaloneConfigAtAuthority(toolID string, cfg DeepDiveConfig, theme string, targets map[string]acceptedTarget, locker operation.Locker) ([]tools.MutationEvidence, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	one := func(evidence tools.MutationEvidence, err error) ([]tools.MutationEvidence, error) {
		if err != nil {
			return nil, err
		}
		return []tools.MutationEvidence{evidence}, nil
	}
	file := func(rel string) (safefile.Revision, *safefile.ParentChain, error) {
		return standaloneFileAuthority(targets, rel)
	}
	switch toolID {
	case "ghostty":
		path, err := tools.GhosttyConfigMutationPath()
		if err != nil {
			return nil, err
		}
		rel := planTargetPath(home, path)
		revision, parents, err := file(rel)
		if err != nil {
			return nil, err
		}
		return one(tools.WriteGhosttyConfigAtBoundAuthorityTracked(path, ghosttyConfigFrom(cfg), theme, revision, parents, locker))
	case "tmux":
		path, err := tools.TmuxConfigMutationPath()
		if err != nil {
			return nil, err
		}
		rel := planTargetPath(home, path)
		revision, parents, err := file(rel)
		if err != nil {
			return nil, err
		}
		return one(tools.WriteTmuxConfigAtAuthorityTracked(tmuxConfigFrom(cfg), theme, revision, parents, locker))
	case "zsh":
		revision, parents, err := file(".zshrc")
		if err != nil {
			return nil, err
		}
		return one(tools.WriteZshConfigAtBoundAuthorityTracked(zshConfigFrom(cfg), theme, revision, parents, locker))
	case "neovim":
		initRevision, initParents, err := file(".config/nvim/init.lua")
		if err != nil {
			return nil, err
		}
		optionsRevision, optionsParents, err := file(".config/nvim/lua/custom/options.lua")
		if err != nil {
			return nil, err
		}
		return tools.WriteNeovimUserPrefsAtBoundAuthoritiesTracked(neovimConfigFrom(cfg), theme, initRevision, initParents, optionsRevision, optionsParents, locker)
	case "git":
		rootRevision, rootParents, err := file(".gitconfig")
		if err != nil {
			return nil, err
		}
		managedRevision, managedParents, err := file(".config/dotfiles/git/config")
		if err != nil {
			return nil, err
		}
		return tools.WriteGitConfigAtBoundAuthoritiesTracked(gitConfigFrom(cfg), theme, rootRevision, rootParents, managedRevision, managedParents, locker)
	case "yazi":
		yaziRevision, yaziParents, err := file(".config/yazi/yazi.toml")
		if err != nil {
			return nil, err
		}
		keymapRevision, keymapParents, err := file(".config/yazi/keymap.toml")
		if err != nil {
			return nil, err
		}
		themeRevision, themeParents, err := file(".config/yazi/theme.toml")
		if err != nil {
			return nil, err
		}
		return tools.WriteYaziConfigAtAuthoritiesTracked(yaziConfigFrom(cfg), theme, yaziRevision, yaziParents, keymapRevision, keymapParents, themeRevision, themeParents, locker)
	case "fzf":
		revision, parents, err := file(".config/fzf/fzf.zsh")
		if err != nil {
			return nil, err
		}
		return one(tools.WriteFzfConfigAtAuthorityTracked(fzfConfigFrom(cfg), theme, revision, parents, locker))
	case "lazygit":
		revision, parents, err := file(".config/lazygit/config.yml")
		if err != nil {
			return nil, err
		}
		return one(tools.WriteLazyGitConfigAtAuthorityTracked(lazygitConfigFrom(cfg), theme, revision, parents, locker))
	case "btop":
		btopCfg := btopConfigFrom(cfg)
		configPath, err := tools.BtopConfigMutationPath()
		if err != nil {
			return nil, err
		}
		themePath, err := tools.BtopThemeMutationPath(btopCfg, theme)
		if err != nil {
			return nil, err
		}
		configRel := planTargetPath(home, configPath)
		themeRel := planTargetPath(home, themePath)
		configRevision, configParents, err := file(configRel)
		if err != nil {
			return nil, err
		}
		themeRevision, themeParents, err := file(themeRel)
		if err != nil {
			return nil, err
		}
		return tools.WriteBtopConfigAtAuthoritiesTracked(btopCfg, theme, themeRevision, themeParents, configRevision, configParents, locker)
	case "glow":
		paths := tools.NewGlowTool().ConfigPaths()
		if len(paths) != 1 {
			return nil, fmt.Errorf("glow registry returned %d config paths", len(paths))
		}
		rel := planTargetPath(home, paths[0])
		revision, parents, err := file(rel)
		if err != nil {
			return nil, err
		}
		return one(tools.WriteGlowConfigAtAuthorityTracked(glowConfigFrom(cfg), theme, revision, parents, locker))
	case "claude-code":
		revision, parents, err := file(".claude.json")
		if err != nil {
			return nil, err
		}
		return one(tools.NewClaudeCodeTool().ApplyConfigWithMCPsAtBoundAuthorityTracked(cfg.ClaudeCodeMCPs, revision, parents, locker))
	default:
		return nil, fmt.Errorf("no reviewed standalone writer for %s", toolID)
	}
}
