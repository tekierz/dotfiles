package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/tools"
)

type manageSaveDoneMsg struct {
	standaloneConfigExecutionResult
}

type manageSaveRuntime struct {
	transaction standaloneConfigRuntime
	saveManage  func(*ManageConfig, safefile.Revision, *safefile.ParentChain, operation.Locker) (safefile.Revision, error)
	saveGlobal  func(*config.GlobalConfig, safefile.Revision, *safefile.ParentChain, operation.Locker) (safefile.Revision, error)
}

func defaultManageSaveRuntime() manageSaveRuntime {
	return manageSaveRuntime{
		transaction: defaultStandaloneConfigRuntime(),
		saveManage: func(cfg *ManageConfig, revision safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (safefile.Revision, error) {
			return config.SaveToolConfigAtBoundAuthorityTracked("manage", cfg, revision, parents, locker)
		},
		saveGlobal: config.SaveGlobalConfigAtBoundAuthorityTracked,
	}
}

type manageSavePlan struct {
	plan              *installPlan
	snapshot          ManageConfig
	theme             string
	navStyle          string
	animationsEnabled bool
	global            *config.GlobalConfig
	saveManageState   bool
	saveGlobalState   bool
}

func (a *App) prepareManageSave() tea.Cmd {
	plan, err := buildManageSavePlan(a, time.Now())
	a.pendingManageSavePlan = plan
	a.manageSavePlanErr = err
	a.manageSaveScroll = 0
	a.manageSaveRunning = false
	a.manageSaveDone = false
	a.manageSaveManual = false
	a.manageSaveWarning = ""
	a.manageStatus = ""
	return NavigateTo(ScreenManageSaveConfirm)
}

func (a *App) executeManageSavePlanCmd() tea.Cmd {
	accepted := a.pendingManageSavePlan
	return func() tea.Msg {
		return manageSaveDoneMsg{executeManageSavePlanResult(context.Background(), accepted, defaultManageSaveRuntime())}
	}
}

func executeManageSavePlanResult(ctx context.Context, accepted *manageSavePlan, runtime manageSaveRuntime) standaloneConfigExecutionResult {
	if accepted == nil || accepted.plan == nil || accepted.plan.hasBlocked() || !managePlanHasApplicableChanges(accepted.plan) {
		return standaloneConfigExecutionResult{err: fmt.Errorf("manage save plan is blocked or has no applicable change")}
	}
	if len(accepted.plan.configTools) > 0 && runtime.transaction.write == nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("manage config writer is unavailable")}
	}
	if accepted.saveManageState && runtime.saveManage == nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("manage state writer is unavailable")}
	}
	if accepted.saveGlobalState && runtime.saveGlobal == nil {
		return standaloneConfigExecutionResult{err: fmt.Errorf("global state writer is unavailable")}
	}
	return executeConfigTransactionResult(ctx, accepted.plan, runtime.transaction, "manage-save-operation", func(home string, plan *installPlan, bound map[string]map[string]acceptedTarget, locker operation.Locker, expected map[string]backup.ExpectedState) (bool, bool, error) {
		mutationStarted := false
		for _, toolID := range plan.configTools {
			actionID := "config:" + toolID
			mutated, manual, err := executeConfigTransactionAction(home, plan, actionID, expected, func() ([]tools.MutationEvidence, error) {
				return runtime.transaction.write(toolID, plan.config, plan.theme, bound[actionID], locker)
			})
			mutationStarted = mutationStarted || mutated
			if err != nil {
				return mutationStarted, manual, err
			}
		}
		if accepted.saveManageState {
			actionID := "state:manage-preferences"
			mutated, manual, err := executeConfigTransactionAction(home, plan, actionID, expected, func() ([]tools.MutationEvidence, error) {
				rel := planTargetPath(home, filepath.Join(config.ToolsDir(), "manage.json"))
				target, ok := bound[actionID][rel]
				if !ok || target.kind != acceptedFileTarget {
					return nil, fmt.Errorf("accepted manage.json authority is unavailable")
				}
				revision, err := runtime.saveManage(&accepted.snapshot, target.file, target.parents, locker)
				evidence := tools.MutationEvidence{Path: filepath.Join(config.ToolsDir(), "manage.json"), Revision: revision, Parents: target.parents}
				return stateMutationEvidence(evidence, err)
			})
			mutationStarted = mutationStarted || mutated
			if err != nil {
				return mutationStarted, manual, err
			}
		}
		if accepted.saveGlobalState {
			actionID := "state:global"
			mutated, manual, err := executeConfigTransactionAction(home, plan, actionID, expected, func() ([]tools.MutationEvidence, error) {
				rel := planTargetPath(home, filepath.Join(config.ConfigDir(), "global.json"))
				target, ok := bound[actionID][rel]
				if !ok || target.kind != acceptedFileTarget {
					return nil, fmt.Errorf("accepted global.json authority is unavailable")
				}
				revision, err := runtime.saveGlobal(accepted.global, target.file, target.parents, locker)
				evidence := tools.MutationEvidence{Path: filepath.Join(config.ConfigDir(), "global.json"), Revision: revision, Parents: target.parents}
				return stateMutationEvidence(evidence, err)
			})
			mutationStarted = mutationStarted || mutated
			if err != nil {
				return mutationStarted, manual, err
			}
		}
		return mutationStarted, false, nil
	})
}

func stateMutationEvidence(evidence tools.MutationEvidence, err error) ([]tools.MutationEvidence, error) {
	if err == nil {
		return []tools.MutationEvidence{evidence}, nil
	}
	if evidence.Revision.Tracked() && evidence.Revision.Exists() && evidence.Parents.Tracked() {
		return nil, &tools.PartialMutationError{Err: err, Evidence: []tools.MutationEvidence{evidence}}
	}
	return nil, err
}

func buildManageSavePlan(a *App, now time.Time) (*manageSavePlan, error) {
	if a == nil || a.manageConfig == nil {
		return nil, fmt.Errorf("manage editor state is unavailable")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine HOME for Manage save plan: %w", err)
	}
	if home == "" || !filepath.IsAbs(home) {
		return nil, fmt.Errorf("determine HOME for Manage save plan: %q is not absolute", home)
	}
	global, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("validate global config before Manage planning: %w", err)
	}
	statePlan, err := operation.CaptureStatePlan()
	if err != nil {
		return nil, fmt.Errorf("capture operation state before Manage planning: %w", err)
	}

	snapshot := *a.manageConfig
	baseline := a.manageConfigBaseline
	deep := manageConfigToDeepDive(&snapshot)
	changed := changedManageTools(&baseline, &snapshot, a.manageConfigBaselineTheme, a.theme)
	actions := make([]operation.Action, 0, len(changed)+3)
	authority := make(map[string]map[string]acceptedTarget)
	configTools := make([]string, 0, len(changed))
	for _, toolID := range changed {
		if toolID == "neovim" {
			actions = append(actions, blockedManageToolAction(toolID, "tracked authority and partial-mutation evidence for the Neovim init.lua/options.lua overlay are not implemented yet"))
			continue
		}
		spec, reason, specErr := standaloneConfigPlanSpec(home, a.theme, deep, toolID)
		if specErr != nil {
			return nil, specErr
		}
		if reason != "" {
			actions = append(actions, blockedManageToolAction(toolID, reason))
			continue
		}
		action, accepted, actionErr := planConfigAction(home, spec, digestPlanValue(struct {
			ToolID string
			Theme  string
			Config DeepDiveConfig
		}{toolID, a.theme, deep}))
		if actionErr != nil {
			return nil, actionErr
		}
		if nativeReason := standaloneNativeBlockReason(a, toolID); nativeReason != "" {
			action.Disposition = operation.DispositionBlocked
			action.Reason = nativeReason
			accepted = nil
		}
		if toolID == "claude-code" {
			changes, compareErr := claudeSelectionChanges(deep.ClaudeCodeMCPs)
			if compareErr != nil {
				return nil, compareErr
			}
			if !changes {
				action.Disposition = operation.DispositionSkip
				action.Reason = "Claude MCP selection already matches the saved file"
				accepted = nil
			}
		}
		actions = append(actions, action)
		if action.Disposition == operation.DispositionApply {
			authority[action.ID] = accepted
			configTools = append(configTools, toolID)
		}
	}

	saveManage := snapshot != baseline
	if saveManage {
		action, accepted, actionErr := planManageStateFile(home, "state:manage-preferences", filepath.Join(config.ToolsDir(), "manage.json"), "persist reviewed Manage preferences", digestPlanValue(snapshot), safefile.Revision{})
		if actionErr != nil {
			return nil, actionErr
		}
		actions = append(actions, action)
		if action.Disposition == operation.DispositionApply {
			authority[action.ID] = accepted
		}
	}

	plannedGlobal := config.CloneGlobalConfig(global)
	plannedGlobal.Theme = a.theme
	plannedGlobal.NavStyle = a.navStyle
	plannedGlobal.DisableAnimations = !a.animationsEnabled
	saveGlobal := global.Theme != plannedGlobal.Theme || global.NavStyle != plannedGlobal.NavStyle || global.DisableAnimations != plannedGlobal.DisableAnimations
	if saveGlobal {
		revision, tracked := config.GlobalConfigRevision(global)
		if !tracked {
			return nil, fmt.Errorf("loaded global config has no tracked revision")
		}
		action, accepted, actionErr := planManageStateFile(home, "state:global", filepath.Join(config.ConfigDir(), "global.json"), "persist reviewed global theme, navigation, and motion", digestPlanValue(struct {
			Theme      string
			Navigation string
			Animations bool
		}{a.theme, a.navStyle, a.animationsEnabled}), revision)
		if actionErr != nil {
			return nil, actionErr
		}
		actions = append(actions, action)
		if action.Disposition == operation.DispositionApply {
			authority[action.ID] = accepted
		}
	}

	if len(actions) == 0 {
		actions = append(actions, operation.Action{
			ID: "state:manage-no-change", Kind: operation.KindUpdateState,
			Target: "Manage", Description: "Manage settings already match persisted state",
			Disposition: operation.DispositionSkip, Reason: "No settings changed",
			DesiredDigest: digestPlanValue(snapshot), Ownership: operation.OwnershipManagedFile,
			Reversibility: operation.ReversibilityManual,
		})
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
	inner := &installPlan{
		document: document, configTools: slices.Clone(configTools), config: deep,
		theme: a.theme, navStyle: a.navStyle, animations: a.animationsEnabled,
		globalConfig: plannedGlobal, authority: authority, parentDirs: parents, statePlan: statePlan,
	}
	return &manageSavePlan{
		plan: inner, snapshot: snapshot, theme: a.theme,
		navStyle: a.navStyle, animationsEnabled: a.animationsEnabled, global: plannedGlobal,
		saveManageState: saveManage, saveGlobalState: saveGlobal,
	}, nil
}

func blockedManageToolAction(toolID, reason string) operation.Action {
	return operation.Action{
		ID: "config:" + toolID, Kind: operation.KindWriteConfig, ToolID: toolID,
		Target: toolID, Description: "apply changed Manage settings for " + toolID,
		Disposition: operation.DispositionBlocked, Reason: reason,
		DesiredDigest: digestPlanValue(struct{ ToolID, Reason string }{toolID, reason}),
		Ownership:     operation.OwnershipUnknown, Reversibility: operation.ReversibilityManual,
	}
}

func planManageStateFile(home, actionID, path, description, desiredDigest string, expected safefile.Revision) (operation.Action, map[string]acceptedTarget, error) {
	rel := planTargetPath(home, path)
	action := operation.Action{
		ID: actionID, Kind: operation.KindUpdateState, Target: rel, Description: description,
		Disposition: operation.DispositionApply, DesiredDigest: desiredDigest,
		Ownership: operation.OwnershipManagedFile, Reversibility: operation.ReversibilityBackup,
	}
	if filepath.IsAbs(rel) {
		action.Disposition = operation.DispositionBlocked
		action.Reason = "state outside HOME cannot yet receive a verified rollback point"
		return action, nil, nil
	}
	data, revision, parents, err := safefile.ObserveFileWithin(home, rel)
	if err != nil {
		return operation.Action{}, nil, err
	}
	if expected.Tracked() && revision != expected {
		return operation.Action{}, nil, fmt.Errorf("%s changed while building Manage plan: %w", rel, safefile.ErrRevisionChanged)
	}
	observation := observationFromFileRevision(rel, revision, true)
	action.BackupTarget = rel
	action.Observation = observation
	action.Observations = []operation.Observation{observation}
	return action, map[string]acceptedTarget{rel: {kind: acceptedFileTarget, file: revision, parents: parents, data: slices.Clone(data)}}, nil
}
