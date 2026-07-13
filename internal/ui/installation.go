package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/installapply"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/safefile"
	"github.com/tekierz/dotfiles/internal/scripts"
	"github.com/tekierz/dotfiles/internal/tools"
)

// installEventMsg is a single event emitted by the install worker goroutine.
// The worker NEVER mutates App state directly; instead it sends these events
// over a.installEvents and the Update loop applies them on the main goroutine.
// This is the standard Bubble Tea channel + "listen" Cmd streaming pattern and
// avoids the data race between the worker and Update/View.
type installEventMsg struct {
	line        string // a line of output to append (empty if none)
	stepInc     bool   // advance the progress step counter
	done        bool   // the install/configure sequence finished
	err         error  // final error (only meaningful when done)
	context     string // last few output lines for error context (only when done)
	operationID string // durable journal record for this execution (when enabled)
}

var errInstallStreamClosed = errors.New("installation event stream closed without a terminal result")

const maxCollectedInstallLines = 500
const maxCollectedInstallLineBytes = 16 * 1024

func boundInstallLine(line string) string {
	if len(line) > maxCollectedInstallLineBytes {
		cut := maxCollectedInstallLineBytes
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		line = line[:cut] + " …[truncated]"
	}
	return line
}

func appendBoundedInstallLine(lines []string, line string) []string {
	line = boundInstallLine(line)
	if len(lines) < maxCollectedInstallLines {
		return append(lines, line)
	}
	copy(lines, lines[1:])
	lines[len(lines)-1] = line
	return lines
}

func emitInstallEvent(ctx context.Context, events chan<- installEventMsg, line string, step bool) {
	line = boundInstallLine(line)
	select {
	case events <- installEventMsg{line: line, stepInc: step}:
	case <-ctx.Done():
	}
}

// toolInstallRuntime provides the two environment dependencies used by the
// dashboard's install paths. Keeping these as function values gives focused
// tests a way to prove that the wizard and Manage both dispatch through a
// Tool's Install method without replacing the process-wide registry or package
// manager caches.
type toolInstallRuntime struct {
	lookupTool             func(string) (tools.Tool, bool)
	registeredToolIDs      func() []string
	describeInstall        func(tools.Tool, tools.InstallEnvironment) (operation.InstallRecipe, error)
	captureStatePlan       func() (*operation.StatePlan, error)
	detectManager          func() pkg.PackageManager
	detectPlatform         func() pkg.Platform
	isToolInstalled        func(tools.Tool) bool
	autoBackup             func() (autoBackupResult, error)
	backupTargets          func([]backup.Target) (autoBackupResult, error)
	backupTargetsWithState func(*operation.StateAuthority, []backup.Target) (autoBackupResult, error)
	acquireInstallLock     func(*operation.StateAuthority, string, string) (func() error, error)
}

func defaultToolInstallRuntime() toolInstallRuntime {
	reg := tools.GetRegistry()
	return toolInstallRuntime{
		lookupTool: reg.Get,
		registeredToolIDs: func() []string {
			registered := reg.All()
			ids := make([]string, 0, len(registered))
			for _, tool := range registered {
				ids = append(ids, tool.ID())
			}
			return ids
		},
		describeInstall:  tools.DescribeInstall,
		captureStatePlan: operation.CaptureStatePlan,
		detectManager:    pkg.DetectManager,
		detectPlatform:   pkg.DetectPlatform,
		isToolInstalled: func(t tools.Tool) bool {
			return t.IsInstalled()
		},
		autoBackup:             autoBackupIfEnabled,
		backupTargets:          backupPlanTargets,
		backupTargetsWithState: backupPlanTargetsWithState,
		acquireInstallLock:     operation.AcquireStateLockWithAuthority,
	}
}

// contextToolInstaller is an optional custom-install contract. Tools with
// subprocess work outside PackageManager should implement it so the TUI can
// cancel that work and stream bounded output instead of falling back to the
// legacy synchronous Install method.
type contextToolInstaller interface {
	InstallWithContext(context.Context, pkg.PackageManager, func(string)) error
}

// platformContextToolInstaller is the fully explicit custom-install contract.
// It carries the same platform snapshot used by planning into custom tools that
// also install BaseTool prerequisites. This must be checked before the legacy
// context interface and before the promoted BaseTool platform method: otherwise
// Claude Code can either re-detect the host for Node/npm or skip its npm phase.
type platformContextToolInstaller interface {
	InstallWithContextForPlatform(context.Context, pkg.PackageManager, pkg.Platform, func(string)) error
}

// platformToolInstaller lets ordinary BaseTool-backed tools execute against
// the exact platform snapshot used to build the plan. Custom context installers
// are dispatched first so a promoted BaseTool method can never bypass their
// tool-owned install steps.
type platformToolInstaller interface {
	InstallForPlatform(pkg.PackageManager, pkg.Platform) error
}

// packageManagerPolicy is separate from package observation authority. A tool
// can have non-authoritative/empty package metadata yet still require a package
// manager for prerequisites. Manager-independent custom installers opt out.
type packageManagerPolicy interface {
	RequiresPackageManager() bool
}

// installerAvailabilityPolicy is independent of package-receipt observation.
// It answers whether this process knows how to install a tool on a platform;
// package metadata may still be non-authoritative for final identity.
type installerAvailabilityPolicy interface {
	InstallerAvailable(pkg.Platform) bool
}

func installerAvailable(t tools.Tool, platform pkg.Platform) bool {
	policy, ok := t.(installerAvailabilityPolicy)
	if ok {
		return policy.InstallerAvailable(platform)
	}
	return len(tools.PackagesForPlatform(t.Packages(), platform)) > 0
}

func requiresPackageManager(t tools.Tool) bool {
	policy, ok := t.(packageManagerPolicy)
	return !ok || policy.RequiresPackageManager()
}

// streamingInstallManager adapts the synchronous PackageManager.Install method
// expected by tools.Tool.Install to the cancelable streaming operation used by
// the TUI. Calling the Tool method is important: package metadata is only one
// part of some installers (Claude Code installs Node through the manager and
// then installs its CLI through npm). The old dashboard called
// InstallStreaming directly and silently skipped those custom steps.
//
// Embedding PackageManager delegates the rest of the interface unchanged. A
// custom Tool.Install therefore sees a normal package manager, while ordinary
// BaseTool installs keep their live output and context cancellation.
type streamingInstallManager struct {
	pkg.PackageManager
	ctx      context.Context
	emitLine func(string)
}

func (m *streamingInstallManager) Install(packages ...string) error {
	if err := m.ctx.Err(); err != nil {
		return err
	}

	cmd, err := m.InstallStreaming(m.ctx, packages...)
	if err != nil {
		return err
	}
	// Test managers and adapters with no subprocess may complete the operation
	// synchronously and return no StreamingCmd.
	if cmd == nil {
		return nil
	}

	for line := range cmd.Output {
		if m.emitLine != nil {
			m.emitLine(line)
		}
	}
	return cmd.Wait()
}

// installTool executes the Tool-owned installation contract while preserving
// streaming for package-manager work. This is the single dispatch point shared
// by the wizard and Manage install paths.
func installTool(ctx context.Context, t tools.Tool, mgr pkg.PackageManager, platform pkg.Platform, emitLine func(string)) error {
	if mgr == nil && requiresPackageManager(t) {
		return fmt.Errorf("no package manager detected")
	}

	var adaptedManager pkg.PackageManager
	if mgr != nil {
		adaptedManager = &streamingInstallManager{
			PackageManager: mgr,
			ctx:            ctx,
			emitLine:       emitLine,
		}
	}
	if installer, ok := t.(platformContextToolInstaller); ok {
		return installer.InstallWithContextForPlatform(ctx, adaptedManager, platform, emitLine)
	}
	if installer, ok := t.(contextToolInstaller); ok {
		return installer.InstallWithContext(ctx, adaptedManager, emitLine)
	}
	if installer, ok := t.(platformToolInstaller); ok {
		return installer.InstallForPlatform(adaptedManager, platform)
	}
	return t.Install(adaptedManager)
}

// startInstallation begins the installation process using the Go-based package
// manager. State is reset here on the main goroutine (safe: this is called from
// Update). The actual work runs in a detached worker goroutine that only writes
// to the events channel, and the returned Cmd starts listening for those events.
func (a *App) startInstallation() tea.Cmd {
	if a.installRunning {
		return nil
	}

	a.installRunning = true
	a.installComplete = false // reset so a retry re-renders as "installing", not "complete"
	a.installOutcome = installationOutcomeRunning
	a.installSummaryFacts = installationSummaryFacts{}
	a.lastError = nil
	a.lastOperationID = ""
	a.installStep = 0
	a.installPlannedSteps = 0
	a.installOutput = []string{}
	plan := a.pendingInstallPlan
	if plan == nil && a.installPlanError == nil {
		a.refreshPendingInstallPlan()
		plan = a.pendingInstallPlan
	}
	a.beginInstallationAttempt(plan)
	if a.installPlanError != nil {
		return func() tea.Msg {
			return installDoneMsg{err: fmt.Errorf("installation plan is invalid: %w", a.installPlanError)}
		}
	}
	if plan == nil {
		if _, err := config.LoadGlobalConfig(); err != nil {
			return func() tea.Msg {
				return installDoneMsg{err: fmt.Errorf("installation blocked by global config error: %w", err)}
			}
		}
		return func() tea.Msg {
			return installDoneMsg{err: fmt.Errorf("installation blocked: no reviewed plan is available")}
		}
	}
	if plan.hasBlocked() {
		return func() tea.Msg {
			return installDoneMsg{err: fmt.Errorf("installation blocked by unresolved configuration ownership")}
		}
	}

	// The progress total comes from the same accepted plan consumed below.
	a.installPlannedSteps = len(plan.selectedToolIDs()) + len(plan.configToolIDs())
	if len(enabledHelpers(plan.config.Utilities)) > 0 {
		a.installPlannedSteps++
	}

	// Buffered channel so the worker can make progress without blocking on a
	// slow consumer; the listen Cmd drains it one event at a time.
	events := make(chan installEventMsg, 64)
	a.installEvents = events

	// Cancelable context so Ctrl+C (or any teardown) can stop the running
	// package-manager subprocess and unblock the worker's bounded-channel sends
	// instead of orphaning them (C15 / concurrency-medium). Stored on the App so
	// teardownStream() can cancel it.
	ctx, cancel := context.WithCancel(context.Background())
	a.streamCancel = cancel

	// On Linux the install runs many `sudo apt/pacman ...` steps non-interactively
	// (Stdin=nil), so the sudo timestamp (~5 min default) can expire during a long
	// multi-package install and a later step fails with "sudo: a password is
	// required" (C16). When sudo is needed and already cached, start a keep-alive
	// goroutine that refreshes the timestamp periodically. It is a no-op on macOS
	// (Homebrew, no sudo) and is stopped on BOTH normal completion (installDoneMsg)
	// and cancel/teardown (teardownStream), so it never leaks past the install.
	if runner.NeedsSudo() && runner.CheckSudoCached() {
		a.sudoKeepAliveStop = startSudoKeepAlive(refreshSudo)
	}

	go runInstallPlanWorker(ctx, events, plan, defaultToolInstallRuntime())

	return a.listenInstallEventsCmd()
}

// listenInstallEventsCmd reads the next event from the install channel and
// returns it as a message. Update re-subscribes by returning this Cmd again
// until it sees a `done` event.
func (a *App) listenInstallEventsCmd() tea.Cmd {
	ch := a.installEvents
	return func() tea.Msg {
		if ch == nil {
			return installEventMsg{done: true, err: errInstallStreamClosed}
		}
		ev, ok := <-ch
		if !ok {
			return installEventMsg{done: true, err: errInstallStreamClosed}
		}
		return ev
	}
}

// runInstallWorkerWithRuntime is the dependency-injected worker used by focused
// install-dispatch tests for the legacy unreviewed harness.
func runInstallWorkerWithRuntime(ctx context.Context, events chan installEventMsg, selectedTools []string, cfg DeepDiveConfig, theme string, installRuntime toolInstallRuntime, savePrefsErr ...error) {
	configTools := []string{"tmux", "ghostty", "zsh", "neovim", "git", "yazi", "fzf"}
	for _, optional := range []string{"claude-code", "lazygit", "btop", "glow"} {
		if optional == "claude-code" && (cfg.CLITools[optional] || cfg.Utilities[optional]) {
			configTools = append(configTools, optional)
		} else if optional != "claude-code" && cfg.CLITools[optional] {
			configTools = append(configTools, optional)
		}
	}
	legacy := &installPlan{
		selectedTools: slices.Clone(selectedTools),
		configTools:   configTools,
		config:        snapshotDeepDiveConfig(&cfg),
		theme:         theme,
	}
	runInstallWorkerFromPlanWithRuntime(ctx, events, legacy, installRuntime, false, savePrefsErr...)
}

func runInstallPlanWorker(ctx context.Context, events chan installEventMsg, plan *installPlan, installRuntime toolInstallRuntime) {
	runInstallWorkerFromPlanWithRuntime(ctx, events, plan, installRuntime, true)
}

func invalidateRollbackAction(plan *installPlan, actionID string, expected map[string]backup.ExpectedState) {
	if expected == nil || plan == nil {
		return
	}
	for _, action := range plan.actions() {
		if action.ID != actionID {
			continue
		}
		for _, rel := range append([]string{action.BackupTarget}, action.BackupTargets...) {
			if rel != "" {
				rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
				target, ok := plan.authority[actionID][rel]
				state := backup.ExpectedState{Attempted: true}
				if ok && target.kind == acceptedFileTarget {
					state.Kind = backup.TargetFile
					state.OriginalExists = target.file.Exists()
					state.OriginalCaptured = true
					state.OriginalData = slices.Clone(target.data)
					state.OriginalMode = target.file.Permissions()
				} else if ok && target.kind == acceptedDirectoryTarget {
					state.Kind = backup.TargetDirectory
					state.OriginalExists = target.directory != nil
					state.OriginalCaptured = true
					state.OriginalDirectory = target.directory
				}
				expected[rel] = state
			}
		}
		return
	}
}

func recordFailedActionRollbackState(home string, plan *installPlan, actionID string, actionErr error, expected map[string]backup.ExpectedState) error {
	if expected == nil {
		return nil
	}
	var files []string
	for _, action := range plan.actions() {
		if action.ID == actionID {
			if action.BackupTarget != "" {
				files = append(files, action.BackupTarget)
			}
			files = append(files, action.BackupTargets...)
			break
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("accepted action %s has no rollback targets", actionID)
	}
	var partial interface {
		CommittedEvidence() []tools.MutationEvidence
	}
	if errors.As(actionErr, &partial) {
		states, err := validateMutationEvidenceSet(home, files, partial.CommittedEvidence())
		if err != nil {
			invalidateRollbackAction(plan, actionID, expected)
			return fmt.Errorf("partial mutation evidence for %s is incomplete; manual recovery required: %w", actionID, err)
		}
		invalidateRollbackAction(plan, actionID, expected)
		for rel, state := range states {
			expected[rel] = preserveOriginalState(expected[rel], state)
		}
		return nil
	} else {
		// A bare CommittedError proves that some mutation crossed its commit
		// point, but without exact desired bytes/snapshot it does not authorize
		// rollback. Leave every target attempted-but-uncaptured.
		var committed interface{ Committed() bool }
		_ = errors.As(actionErr, &committed)
	}
	invalidateRollbackAction(plan, actionID, expected)
	return nil
}

func validateMutationEvidenceSet(home string, allowedFiles []string, evidenceSet []tools.MutationEvidence) (map[string]backup.ExpectedState, error) {
	allowed := make(map[string]struct{}, len(allowedFiles))
	for _, rel := range allowedFiles {
		allowed[filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))] = struct{}{}
	}
	validated := make(map[string]backup.ExpectedState, len(evidenceSet))
	for _, evidence := range evidenceSet {
		if evidence.Path == "" {
			return nil, fmt.Errorf("mutation evidence is missing destination path")
		}
		rel := evidence.Path
		if filepath.IsAbs(rel) {
			var err error
			rel, err = filepath.Rel(home, rel)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("mutation evidence path is outside HOME: %s", evidence.Path)
			}
		}
		rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
		if _, ok := allowed[rel]; !ok {
			return nil, fmt.Errorf("mutation evidence path is outside accepted rollback scope: %s", rel)
		}
		if _, duplicate := validated[rel]; duplicate {
			return nil, fmt.Errorf("duplicate mutation evidence for %s", rel)
		}
		if !evidence.Parents.Tracked() {
			return nil, fmt.Errorf("mutation evidence for %s has no bound parent-chain authority", rel)
		}
		targets, err := plannedBackupTargets(home, []string{rel})
		if err != nil {
			return nil, fmt.Errorf("classify mutation evidence %s: %w", rel, err)
		}
		if len(targets) != 1 {
			return nil, fmt.Errorf("classify mutation evidence %s: target classification returned %d entries", rel, len(targets))
		}
		if targets[0].Kind == backup.TargetDirectory {
			if evidence.Directory == nil {
				return nil, fmt.Errorf("directory mutation evidence for %s has no snapshot", rel)
			}
			if _, err := safefile.BindParentChainWithin(home, rel, evidence.Parents, nil); err != nil {
				return nil, fmt.Errorf("directory mutation evidence parent chain for %s changed: %w", rel, err)
			}
			current, err := safefile.SnapshotDirectoryWithin(home, rel)
			if err != nil || !safefile.SameDirectoryRootState(current, evidence.Directory) || current.Digest() != evidence.Directory.Digest() {
				return nil, fmt.Errorf("directory mutation evidence for %s no longer matches live state: %w", rel, errors.Join(err, safefile.ErrDirectoryChanged))
			}
			validated[rel] = backup.ExpectedState{Attempted: true, Captured: true, Kind: backup.TargetDirectory, Exists: true, DirectorySnapshot: evidence.Directory, Parents: evidence.Parents}
			continue
		}
		if !evidence.Revision.Tracked() || !evidence.Revision.Exists() {
			return nil, fmt.Errorf("file mutation evidence for %s has no tracked existing revision", rel)
		}
		_, current, err := safefile.ReadWithinAuthorized(home, rel, evidence.Parents)
		if err != nil || current != evidence.Revision {
			return nil, fmt.Errorf("file mutation evidence for %s no longer matches live state: %w", rel, errors.Join(err, safefile.ErrRevisionChanged))
		}
		validated[rel] = backup.ExpectedState{Attempted: true, Captured: true, Kind: backup.TargetFile, Exists: true, FileRevision: evidence.Revision, Parents: evidence.Parents}
	}
	return validated, nil
}

func rollbackTargetsForAction(plan *installPlan, actionID string) ([]string, error) {
	if plan == nil {
		return nil, fmt.Errorf("installation plan is unavailable")
	}
	for _, action := range plan.actions() {
		if action.ID != actionID {
			continue
		}
		files := append([]string(nil), action.BackupTargets...)
		if action.BackupTarget != "" {
			files = append(files, action.BackupTarget)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("accepted action %s has no rollback targets", actionID)
		}
		return files, nil
	}
	return nil, fmt.Errorf("accepted action %s is missing", actionID)
}

func authorizeMutationEvidenceSet(home string, plan *installPlan, actionID string, evidenceSet []tools.MutationEvidence, expected map[string]backup.ExpectedState) error {
	if expected == nil || plan == nil {
		return fmt.Errorf("mutation evidence destination state is unavailable")
	}
	allowed, err := rollbackTargetsForAction(plan, actionID)
	if err != nil {
		return err
	}
	validated, err := validateMutationEvidenceSet(home, allowed, evidenceSet)
	if err != nil {
		return err
	}
	if len(validated) != len(allowed) {
		return fmt.Errorf("mutation evidence path set is incomplete: got %d target(s), want %d", len(validated), len(allowed))
	}
	for rel, state := range validated {
		expected[rel] = preserveOriginalState(expected[rel], state)
	}
	return nil
}

func authorizeHelperMutationEvidence(home string, plan *installPlan, attempted []string, evidenceSet []tools.MutationEvidence, expected map[string]backup.ExpectedState) error {
	if expected == nil || len(evidenceSet) > len(attempted) {
		return fmt.Errorf("helper mutation evidence cannot be mapped to attempted actions")
	}
	validatedAll := make(map[string]backup.ExpectedState, len(evidenceSet))
	for index, evidence := range evidenceSet {
		actionID := "helper:" + attempted[index]
		allowed, err := rollbackTargetsForAction(plan, actionID)
		if err != nil {
			return err
		}
		validated, err := validateMutationEvidenceSet(home, allowed, []tools.MutationEvidence{evidence})
		if err != nil {
			return fmt.Errorf("%s evidence: %w", actionID, err)
		}
		if len(validated) != 1 {
			return fmt.Errorf("%s evidence did not prove its exact target", actionID)
		}
		for rel, state := range validated {
			if _, duplicate := validatedAll[rel]; duplicate {
				return fmt.Errorf("duplicate helper mutation evidence for %s", rel)
			}
			validatedAll[rel] = state
		}
	}
	for rel, state := range validatedAll {
		expected[rel] = preserveOriginalState(expected[rel], state)
	}
	return nil
}

func preserveOriginalState(original, post backup.ExpectedState) backup.ExpectedState {
	post.OriginalExists = original.OriginalExists
	post.OriginalCaptured = original.OriginalCaptured
	post.OriginalData = slices.Clone(original.OriginalData)
	post.OriginalMode = original.OriginalMode
	post.OriginalDirectory = original.OriginalDirectory
	return post
}

func runInstallWorkerFromPlanWithRuntime(ctx context.Context, events chan installEventMsg, plan *installPlan, installRuntime toolInstallRuntime, persistJournal bool, savePrefsErr ...error) {
	defer close(events)
	if plan == nil {
		events <- installEventMsg{done: true, err: fmt.Errorf("installation plan is nil")}
		return
	}
	home, homeErr := os.UserHomeDir()
	selectedTools := plan.selectedToolIDs()
	var acceptedInstalls installExecutionSnapshot
	if persistJournal {
		var snapshotErr error
		acceptedInstalls, snapshotErr = plan.installExecutionSnapshot()
		if snapshotErr != nil {
			events <- installEventMsg{done: true, err: fmt.Errorf("derive accepted install authority: %w", snapshotErr)}
			return
		}
	}
	cfg := snapshotDeepDiveConfig(&plan.config)
	theme := plan.theme
	configAllowed := make(map[string]bool, len(plan.configTools))
	for _, toolID := range plan.configToolIDs() {
		configAllowed[toolID] = true
	}
	ghosttyConfigTarget := plan.ghosttyConfigTarget
	// Sends select on ctx.Done() so a cancelled install (Ctrl+C / teardown)
	// unblocks the worker instead of parking forever on the bounded channel once
	// the consumer (the listen Cmd) stops draining it.
	emit := func(line string) {
		emitInstallEvent(ctx, events, line, false)
	}
	step := func(line string) {
		emitInstallEvent(ctx, events, line, true)
	}

	// output accumulates every line emitted so we can build error context that
	// matches the lines the user has seen, without reading App state.
	var output []string
	emitLine := func(line string) {
		line = boundInstallLine(line)
		output = appendBoundedInstallLine(output, line)
		emit(line)
	}
	stepLine := func(line string) {
		line = boundInstallLine(line)
		output = appendBoundedInstallLine(output, line)
		step(line)
	}
	var journal *operation.Journal
	var journalRecord *operation.Record
	var journalResults []operation.ActionResult
	var operationID string
	var releaseInstallOperation func() error
	var stateAuthority *operation.StateAuthority
	var stateCreated map[string]*safefile.DirectorySnapshot
	var boundLocker operation.Locker
	var backupRes autoBackupResult
	mutationStarted := false
	var rollbackExpected map[string]backup.ExpectedState
	var executionAuthority map[string]map[string]acceptedTarget
	executionTarget := func(actionID, rel string) (acceptedTarget, error) {
		rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
		target, ok := executionAuthority[actionID][rel]
		if !ok || !target.parents.Tracked() {
			return acceptedTarget{}, fmt.Errorf("execution authority unavailable for %s target %s", actionID, rel)
		}
		return target, nil
	}
	var journalWarnings []string
	markAction := func(actionID string, status operation.ActionStatus, summary string) {
		for index := range journalResults {
			if journalResults[index].ActionID == actionID {
				journalResults[index].Status = status
				journalResults[index].Summary = summary
				return
			}
		}
	}

	finish := func(err error) {
		var rollbackOutcome *operation.RollbackResult
		if err != nil && mutationStarted && backupRes.backupDir != "" {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				rollbackOutcome = &operation.RollbackResult{Status: operation.RollbackFailed, Summary: "could not determine HOME"}
				err = errors.Join(err, fmt.Errorf("automatic rollback could not determine HOME: %w", homeErr))
			} else {
				var rollback backup.RestoreResult
				var rollbackErr error
				if backupRes.plan == nil {
					rollbackErr = fmt.Errorf("reviewed installation has no exact plan-backup authority")
				} else {
					rollback, rollbackErr = backup.RestoreExpectedPlan(*backupRes.plan, home, rollbackExpected)
				}
				switch {
				case rollbackErr != nil:
					rollbackOutcome = &operation.RollbackResult{Status: operation.RollbackFailed, Summary: "rollback restore returned a fatal error"}
					err = errors.Join(err, fmt.Errorf("automatic rollback failed: %w", rollbackErr))
				case len(rollback.Skipped) != 0 || len(rollback.Warnings) != 0:
					rollbackOutcome = &operation.RollbackResult{Status: operation.RollbackIncomplete, Restored: len(rollback.Restored), Removed: len(rollback.Removed), Skipped: len(rollback.Skipped), Warnings: len(rollback.Warnings), Summary: "manual review required"}
					err = errors.Join(err, fmt.Errorf("automatic rollback incomplete: %d skipped, %d warning(s)", len(rollback.Skipped), len(rollback.Warnings)))
				default:
					rollbackOutcome = &operation.RollbackResult{Status: operation.RollbackSucceeded, Restored: len(rollback.Restored), Removed: len(rollback.Removed), Summary: "planned filesystem scope restored"}
					emitLine(fmt.Sprintf("↶ Rolled back %d restored and %d created path(s)", len(rollback.Restored), len(rollback.Removed)))
				}
			}
		}
		// Release the overall operation lock before deciding the terminal journal
		// status. A release verification failure means the operation did not end
		// cleanly and must never be journaled as succeeded.
		if releaseInstallOperation != nil {
			if releaseErr := releaseInstallOperation(); releaseErr != nil {
				err = errors.Join(err, fmt.Errorf("release install operation lock: %w", releaseErr))
			}
			releaseInstallOperation = nil
		}
		if journalRecord != nil && journal != nil {
			if rollbackOutcome != nil {
				if rollbackErr := journalRecord.SetRollback(*rollbackOutcome); rollbackErr != nil {
					err = errors.Join(err, fmt.Errorf("record automatic rollback outcome: %w", rollbackErr))
				}
			}
			if err == nil {
				for _, result := range journalResults {
					if result.Status == operation.ActionPending {
						err = errors.Join(err, fmt.Errorf("accepted action %s has no terminal execution result", result.ActionID))
					}
				}
			}
			terminalStatus := operation.StatusSucceeded
			if errors.Is(err, context.Canceled) {
				terminalStatus = operation.StatusCancelled
			} else if err != nil {
				terminalStatus = operation.StatusFailed
			}
			for index := range journalResults {
				if journalResults[index].Status != operation.ActionPending {
					continue
				}
				journalResults[index].Status = operation.ActionSkipped
				journalResults[index].Summary = "not completed before operation ended"
			}
			if finishErr := journalRecord.Finish(terminalStatus, time.Now(), journalResults, journalWarnings); finishErr != nil {
				err = errors.Join(err, fmt.Errorf("finalize operation journal: %w", finishErr))
			} else if writeErr := journal.Write(*journalRecord); writeErr != nil {
				err = errors.Join(err, fmt.Errorf("persist terminal operation journal: %w", writeErr))
			}
		}
		var errCtx string
		if err != nil && len(output) > 0 {
			start := 0
			if len(output) > 8 {
				start = len(output) - 8
			}
			errCtx = strings.Join(output[start:], "\n")
		}
		terminal := installEventMsg{done: true, err: err, context: errCtx, operationID: operationID}
		select {
		case events <- terminal:
		default:
			// Preserve the terminal result even when verbose output filled the
			// bounded channel. Sacrifice one old output event, never completion.
			select {
			case <-events:
			default:
			}
			events <- terminal
		}
	}

	if len(savePrefsErr) > 0 && savePrefsErr[0] != nil {
		finish(fmt.Errorf("installation blocked by global config error: %w", savePrefsErr[0]))
		return
	}
	if persistJournal && (homeErr != nil || home == "" || !filepath.IsAbs(home)) {
		if homeErr != nil {
			finish(fmt.Errorf("determine absolute HOME for reviewed installation: %w", homeErr))
		} else {
			finish(fmt.Errorf("determine absolute HOME for reviewed installation: %q is not absolute", home))
		}
		return
	}
	if persistJournal && configAllowed["ghostty"] {
		acceptedTarget, err := plan.acceptedGhosttyConfigTarget()
		if err != nil {
			finish(fmt.Errorf("validate accepted Ghostty execution target: %w", err))
			return
		}
		ghosttyConfigTarget = acceptedTarget
	}
	var err error
	if persistJournal {
		stateAuthority, err = operation.BootstrapStateNamespaceTracked(plan.statePlan)
		if err != nil {
			finish(fmt.Errorf("bootstrap accepted operation state: %w", err))
			return
		}
		stateCreated = stateAuthority.CreatedDirectoriesWithin(home)
		boundLocker = operation.BoundLocker(stateAuthority)
		acquireInstallLock := installRuntime.acquireInstallLock
		if acquireInstallLock == nil {
			acquireInstallLock = operation.AcquireStateLockWithAuthority
		}
		releaseInstallOperation, err = acquireInstallLock(stateAuthority, "install-operation", home)
		if err != nil {
			finish(fmt.Errorf("serialize reviewed installation: %w", err))
			return
		}
		record, err := operation.StartRecord(plan.document, time.Now())
		if err != nil {
			finish(fmt.Errorf("create operation journal record: %w", err))
			return
		}
		// The public operation document is only one component of the accepted
		// plan. Journal the hash that also binds the typed installation snapshot
		// and pinned recipes so audit records identify the exact reviewed plan.
		record.PlanHash = plan.hash()
		createdJournal, err := operation.DefaultJournalWithAuthority(stateAuthority)
		if err != nil {
			finish(fmt.Errorf("open operation journal: %w", err))
			return
		}
		markActionResults := append([]operation.ActionResult(nil), record.Actions...)
		journalResults = markActionResults
		if err := createdJournal.Write(record); err != nil {
			finish(fmt.Errorf("persist initial operation journal: %w", err))
			return
		}
		journal = &createdJournal
		journalRecord = &record
		operationID = record.OperationID
		emitLine("Operation " + record.OperationID + " • plan " + plan.hash()[:12])
		if err := revalidateInstallPlanWithCreated(plan, stateCreated); err != nil {
			finish(fmt.Errorf("installation plan expired before execution: %w", err))
			return
		}
	}

	// Auto-backup before making changes (if enabled). The result is honest:
	// it only reports a created backup when at least one file was captured and
	// the manifest persisted, so we never claim a rollback point exists right
	// before overwriting the user's dotfiles (C5).
	packageOnlyWithoutRollback := false
	if persistJournal {
		var targets []backup.Target
		targets, err = plan.backupTargetSpecs()
		if err == nil && len(targets) == 0 && plan.packageOnlyReviewedExecution() {
			packageOnlyWithoutRollback = true
			warning := "package-only operation has no filesystem mutations; no filesystem rollback point was created"
			journalWarnings = append(journalWarnings, warning)
			emitLine("ℹ " + warning)
		} else if err == nil && installRuntime.backupTargetsWithState != nil {
			backupRes, err = installRuntime.backupTargetsWithState(stateAuthority, targets)
		} else if err == nil && installRuntime.backupTargets != nil {
			backupRes, err = installRuntime.backupTargets(targets)
		} else if err == nil {
			backupRes, err = installRuntime.autoBackup()
		}
	} else {
		backupRes, err = installRuntime.autoBackup()
	}
	if err != nil {
		finish(fmt.Errorf("auto-backup failed; installation stopped before mutation: %w", err))
		return
	} else if persistJournal && !packageOnlyWithoutRollback && (!backupRes.enabled || backupRes.backupDir == "" || backupRes.plan == nil) {
		finish(fmt.Errorf("mandatory rollback point was not created; installation stopped before mutation"))
		return
	} else if backupRes.enabled {
		if journalRecord != nil {
			journalRecord.Backup = backupRes.backupDir
			if journal != nil {
				if writeErr := journal.Write(*journalRecord); writeErr != nil {
					finish(fmt.Errorf("persist rollback point in operation journal: %w", writeErr))
					return
				}
			}
		}
		if backupRes.count > 0 {
			emitLine(fmt.Sprintf("✓ Auto-backup created before installation (%d file(s))", backupRes.count))
		} else {
			emitLine("⚠ Auto-backup captured 0 files (nothing to roll back)")
		}
		// The backup succeeded but pruning old backups did not; surface it so the
		// stalled retention policy is visible rather than silently swallowed.
		if backupRes.cleanupErr != nil {
			emitLine(fmt.Sprintf("⚠ Backup retention cleanup failed: %v", backupRes.cleanupErr))
			journalWarnings = append(journalWarnings, "backup retention cleanup failed: "+backupRes.cleanupErr.Error())
		}
		if backupRes.omission != "" {
			emitLine("⚠ " + backupRes.omission)
			journalWarnings = append(journalWarnings, backupRes.omission)
		}
	}
	if persistJournal {
		// Backup reads every planned target, then a second observation check
		// narrows the final pre-mutation window and refuses a stale preview.
		if err := revalidateInstallPlanWithCreated(plan, stateCreated); err != nil {
			finish(fmt.Errorf("installation plan changed while creating rollback point: %w", err))
			return
		}
	}

	// failures aggregates every failed step so the final error reports how many
	// phases failed rather than silently overwriting a single lastErr.
	var failures []error
	noteFailure := func(err error) { failures = append(failures, err) }

	// Package installation is skipped when no NEW packages are selected (e.g. a
	// fully-installed machine), but the configuration phases below ALWAYS run.
	// runInstallWorker is the only path that writes deep-dive configs, so a
	// re-run with nothing to install must still re-apply config (C14).
	if len(selectedTools) == 0 {
		emitLine("No new tools to install; applying configuration...")
	} else {
		var result selectedToolInstallResult
		if persistJournal {
			result = runSelectedToolInstalls(ctx, selectedTools, installRuntime, emitLine, stepLine, acceptedInstalls)
		} else {
			result = runSelectedToolInstalls(ctx, selectedTools, installRuntime, emitLine, stepLine)
		}
		for _, toolID := range selectedTools {
			if result.installed[toolID] {
				markAction("install:"+toolID, operation.ActionSucceeded, "installed and detected")
			} else if result.satisfied[toolID] {
				markAction("install:"+toolID, operation.ActionSkipped, "already satisfied after review")
			} else if result.failed[toolID] {
				markAction("install:"+toolID, operation.ActionFailed, "install or reviewed precondition failed")
			} else {
				markAction("install:"+toolID, operation.ActionSkipped, "not attempted after an earlier failure")
			}
		}
		for _, installErr := range result.failures {
			noteFailure(installErr)
		}
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}
		if result.successCount == len(selectedTools) {
			emitLine(fmt.Sprintf("\n✓ All %d tools installed successfully!", result.successCount))
		} else if result.successCount+result.skippedCount == len(selectedTools) {
			emitLine(fmt.Sprintf("\n✓ Installed %d/%d tools; %d already satisfied after review", result.successCount, len(selectedTools), result.skippedCount))
		} else {
			emitLine(fmt.Sprintf("\n✓ Installed %d/%d tools", result.successCount, len(selectedTools)))
		}
	}

	if persistJournal {
		// Package/custom installers are allowed to run for minutes and may create
		// their own defaults. Revalidate the complete accepted config authority
		// again before the first reviewed user-config mutation; individual writers
		// still consume exact authority under their lock/transaction afterward.
		if err := revalidateInstallPlanWithCreated(plan, stateCreated); err != nil {
			finish(fmt.Errorf("installation plan changed during package installation: %w", err))
			return
		}
		if plan.packageOnlyReviewedExecution() {
			finish(aggregateFailures(failures))
			return
		}
		rollbackExpected = make(map[string]backup.ExpectedState)
		mutationStarted = true
		parentsCreated := 0
		createdParents := make(map[string]*safefile.DirectorySnapshot, len(stateCreated)+len(plan.parentDirectoryTargets()))
		for rel, snapshot := range stateCreated {
			createdParents[rel] = snapshot
		}
		for _, rel := range plan.parentDirectoryTargets() {
			accepted, acceptedParents, err := plan.acceptedDirectoryAuthority("state:parents", rel)
			if err != nil {
				markAction("state:parents", operation.ActionFailed, "parent authority was unavailable")
				finish(fmt.Errorf("resolve accepted parent %s: %w", rel, err))
				return
			}
			boundParents, bindErr := safefile.BindParentChainWithin(home, rel, acceptedParents, createdParents)
			if bindErr != nil {
				rollbackExpected[rel] = backup.ExpectedState{Attempted: true, Kind: backup.TargetDirectory, OriginalCaptured: true}
				markAction("state:parents", operation.ActionFailed, "parent namespace changed")
				finish(fmt.Errorf("bind accepted parent %s: %w", rel, bindErr))
				return
			}
			rollbackExpected[rel] = backup.ExpectedState{Attempted: true, Kind: backup.TargetDirectory, Parents: boundParents, OriginalCaptured: true}
			snapshot, err := safefile.EnsureShallowDirectoryWithinParentChainTracked(home, rel, accepted, boundParents, 0o700)
			if err != nil {
				if snapshot != nil {
					rollbackExpected[rel] = backup.ExpectedState{Attempted: true, Captured: true, Kind: backup.TargetDirectory, Exists: true, DirectorySnapshot: snapshot, EmptyOnly: true, Parents: boundParents, OriginalCaptured: true}
				}
				markAction("state:parents", operation.ActionFailed, "parent creation failed")
				finish(fmt.Errorf("create accepted parent %s: %w", rel, err))
				return
			}
			rollbackExpected[rel] = backup.ExpectedState{
				Attempted:         true,
				Captured:          true,
				Kind:              backup.TargetDirectory,
				Exists:            true,
				DirectorySnapshot: snapshot,
				EmptyOnly:         true,
				Parents:           boundParents,
				OriginalCaptured:  true,
			}
			createdParents[rel] = snapshot
			parentsCreated++
		}
		if parentsCreated > 0 {
			markAction("state:parents", operation.ActionSucceeded, fmt.Sprintf("created %d reviewed parent directories", parentsCreated))
		}
		executionAuthority, err = bindInstallPlanAuthority(home, plan, createdParents)
		if err != nil {
			finish(fmt.Errorf("bind accepted execution authority: %w", err))
			return
		}
		invalidateRollbackAction(plan, "state:global", rollbackExpected)
		globalRels, authorityErr := rollbackTargetsForAction(plan, "state:global")
		if authorityErr != nil || len(globalRels) != 1 {
			finish(fmt.Errorf("resolve global execution target: %w", authorityErr))
			return
		}
		globalTarget, authorityErr := executionTarget("state:global", globalRels[0])
		if authorityErr != nil {
			finish(authorityErr)
			return
		}
		globalEvidence, saveErr := savePlannedInstallerPreferencesTracked(plan, home, globalRels[0], globalTarget, boundLocker)
		if saveErr != nil {
			captureErr := recordFailedActionRollbackState(home, plan, "state:global", saveErr, rollbackExpected)
			markAction("state:global", operation.ActionFailed, "global preferences could not be persisted")
			finish(errors.Join(fmt.Errorf("installation blocked by global config error: %w", saveErr), captureErr))
			return
		}
		if err := authorizeMutationEvidenceSet(home, plan, "state:global", []tools.MutationEvidence{globalEvidence}, rollbackExpected); err != nil {
			markAction("state:global", operation.ActionFailed, "global preference evidence was rejected")
			finish(fmt.Errorf("authorize global preference mutation evidence: %w", err))
			return
		}
		markAction("state:global", operation.ActionSucceeded, "global preferences persisted")
	}

	// configPhase runs a single configuration step, emitting a header line,
	// advancing the progress step, and recording any failure.
	configPhase := func(header string, run func() error, okLine string) error {
		stepLine(header)
		if err := run(); err != nil {
			emitLine(fmt.Sprintf("  ⚠ %v", err))
			noteFailure(err)
			return err
		} else if okLine != "" {
			emitLine(okLine)
		}
		return nil
	}
	applyConfigAction := func(actionID, toolID string, run func() ([]tools.MutationEvidence, error)) bool {
		if persistJournal {
			invalidateRollbackAction(plan, actionID, rollbackExpected)
		}
		evidence, actionErr := run()
		if actionErr != nil {
			emitLine(fmt.Sprintf("  ⚠ %v", actionErr))
			noteFailure(actionErr)
		}
		succeeded := actionErr == nil
		if succeeded {
			if len(evidence) == 0 {
				invalidateRollbackAction(plan, actionID, rollbackExpected)
				noteFailure(fmt.Errorf("%s writer returned no exact mutation evidence", toolID))
				succeeded = false
			} else if err := authorizeMutationEvidenceSet(home, plan, actionID, evidence, rollbackExpected); err != nil {
				invalidateRollbackAction(plan, actionID, rollbackExpected)
				noteFailure(fmt.Errorf("authorize %s mutation evidence; automatic rollback is incomplete and manual recovery may be required: %w", toolID, err))
				succeeded = false
			}
		} else {
			if captureErr := recordFailedActionRollbackState(home, plan, actionID, actionErr, rollbackExpected); captureErr != nil {
				noteFailure(fmt.Errorf("capture proven %s partial writes: %w", toolID, captureErr))
			}
		}
		if succeeded {
			markAction(actionID, operation.ActionSucceeded, "configuration applied")
		} else {
			markAction(actionID, operation.ActionFailed, "configuration apply failed")
		}
		return succeeded
	}
	toolConfigPhase := func(toolID, header string, run func() ([]tools.MutationEvidence, error), okLine string) bool {
		if !configAllowed[toolID] {
			return false
		}
		available, reason := coreToolConfigAvailable(installRuntime, toolID)
		if !available {
			stepLine(header)
			emitLine("  ↷ Skipped configuration: " + reason)
			markAction("config:"+toolID, operation.ActionSkipped, reason)
			return false
		}
		stepLine(header)
		succeeded := applyConfigAction("config:"+toolID, toolID, run)
		if succeeded && okLine != "" {
			emitLine(okLine)
		}
		return succeeded
	}

	// Install only the helper files represented by the accepted plan. The main
	// dotfiles binary remains package-manager owned and never appears here.
	if len(enabledHelpers(cfg.Utilities)) > 0 {
		var helperResult utilityInstallResult
		_ = configPhase("\n▶ Installing dotfiles utilities...", func() error {
			if persistJournal {
				helperResult = installUtilitiesAtAuthorityTracked(executionAuthority, cfg.Utilities, boundLocker, func(name string) {
					invalidateRollbackAction(plan, "helper:"+name, rollbackExpected)
				})
			} else {
				helperResult = installUtilitiesTracked(cfg.Utilities)
			}
			if helperResult.Err != nil {
				return fmt.Errorf("failed to install utilities: %w", helperResult.Err)
			}
			return nil
		}, "  ✓ Utilities installed to ~/.local/bin")
		helpers := enabledHelpers(cfg.Utilities)
		authorizationOK := true
		if persistJournal {
			wantEvidence := len(helperResult.Attempted)
			if helperResult.Failed != "" {
				wantEvidence--
			}
			if len(helperResult.Evidence) != wantEvidence {
				authorizationOK = false
				noteFailure(fmt.Errorf("helper mutation evidence set is incomplete: got %d target(s), want %d", len(helperResult.Evidence), wantEvidence))
			}
		}
		if persistJournal && authorizationOK && len(helperResult.Evidence) > 0 {
			if err := authorizeHelperMutationEvidence(home, plan, helperResult.Attempted, helperResult.Evidence, rollbackExpected); err != nil {
				authorizationOK = false
				authorizationErr := fmt.Errorf("authorize helper mutation evidence; automatic rollback is incomplete and manual recovery may be required: %w", err)
				noteFailure(authorizationErr)
				for _, helper := range helperResult.Attempted {
					invalidateRollbackAction(plan, "helper:"+helper, rollbackExpected)
				}
			}
		}
		if authorizationOK && helperResult.Failed != "" {
			if captureErr := recordFailedActionRollbackState(home, plan, "helper:"+helperResult.Failed, helperResult.Err, rollbackExpected); captureErr != nil {
				noteFailure(fmt.Errorf("capture helper %s failure; manual recovery may be required: %w", helperResult.Failed, captureErr))
			}
		}
		succeededCount := len(helperResult.Evidence)
		attemptedIndex := make(map[string]int, len(helperResult.Attempted))
		for index, helper := range helperResult.Attempted {
			attemptedIndex[helper] = index
		}
		for _, helper := range helpers {
			index, attempted := attemptedIndex[helper]
			switch {
			case !attempted:
				markAction("helper:"+helper, operation.ActionSkipped, "not attempted after earlier helper failure")
			case !authorizationOK:
				markAction("helper:"+helper, operation.ActionFailed, "helper evidence could not be authorized")
			case helper == helperResult.Failed || index >= succeededCount:
				markAction("helper:"+helper, operation.ActionFailed, "helper installation failed")
			default:
				markAction("helper:"+helper, operation.ActionSucceeded, "helper installed")
			}
		}
	}

	// Configure tmux with TPM plugins. The DeepDiveConfig -> TmuxConfig translation
	// is shared with config-apply via tmuxConfigFrom; install additionally clones
	// TPM (SetupTPM), which is an install-only side-effect.
	tmuxCfg := tmuxConfigFrom(cfg)
	tmuxConfigured := toolConfigPhase("tmux", "\n▶ Configuring tmux...", func() ([]tools.MutationEvidence, error) {
		if persistJournal {
			tmuxAuthority, err := executionTarget("config:tmux", ".tmux.conf")
			if err != nil {
				return nil, err
			}
			var tpmAuthority acceptedTarget
			if tmuxCfg.TPMEnabled {
				tpmAuthority, err = executionTarget("config:tmux", ".tmux/plugins/tpm")
				if err != nil {
					return nil, err
				}
			}
			evidence, err := tools.SetupTPMAtAuthorityTracked(tmuxCfg, theme, tmuxAuthority.file, tmuxAuthority.parents, tpmAuthority.directory, tpmAuthority.parents, stateAuthority)
			return evidence, wrapMutationError("failed to configure tmux", err)
		}
		evidence, err := tools.SetupTPMTracked(tmuxCfg, theme)
		return evidence, wrapMutationError("failed to configure tmux", err)
	}, "  ✓ Tmux configured with ~/.tmux.conf")
	if tmuxConfigured {
		if tmuxCfg.TPMEnabled {
			if tools.IsTPMInstalled() {
				emitLine("  ✓ TPM plugins ready (run prefix+I in tmux to install)")
			} else {
				emitLine("  ⚠ TPM installed but plugins pending")
			}
		}
	}

	// Apply Claude Code MCP configuration if claude-code was selected
	if cfg.CLITools["claude-code"] || cfg.Utilities["claude-code"] {
		enabledCount := 0
		for _, enabled := range cfg.ClaudeCodeMCPs {
			if enabled {
				enabledCount++
			}
		}
		toolConfigPhase("claude-code", "\n▶ Configuring Claude Code MCP servers...", func() ([]tools.MutationEvidence, error) {
			claudeTool := tools.NewClaudeCodeTool()
			if persistJournal {
				accepted, err := executionTarget("config:claude-code", ".claude.json")
				if err != nil {
					return nil, err
				}
				evidence, err := claudeTool.ApplyConfigWithMCPsAtBoundAuthorityTracked(cfg.ClaudeCodeMCPs, accepted.file, accepted.parents, boundLocker)
				return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Claude MCP", err)
			}
			evidence, err := claudeTool.ApplyConfigWithMCPsTracked(cfg.ClaudeCodeMCPs)
			return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Claude MCP", err)
		}, fmt.Sprintf("  ✓ Claude Code configured with %d MCP server(s)", enabledCount))
	}

	// Configure Ghostty
	toolConfigPhase("ghostty", "\n▶ Configuring Ghostty...", func() ([]tools.MutationEvidence, error) {
		var evidence tools.MutationEvidence
		var err error
		if persistJournal {
			rel, relErr := filepath.Rel(home, ghosttyConfigTarget)
			if relErr != nil {
				return nil, relErr
			}
			accepted, acceptedErr := executionTarget("config:ghostty", filepath.ToSlash(rel))
			if acceptedErr != nil {
				return nil, acceptedErr
			}
			evidence, err = tools.WriteGhosttyConfigAtResolvedAuthorityTracked(ghosttyConfigTarget, ghosttyConfigFrom(cfg), theme, accepted.file, accepted.parents, boundLocker)
		} else if ghosttyConfigTarget != "" {
			evidence, err = tools.WriteGhosttyConfigAtTracked(ghosttyConfigTarget, ghosttyConfigFrom(cfg), theme)
		} else {
			// Compatibility-only workers created without an accepted production plan
			// retain the historical resolver path.
			evidence, err = tools.WriteGhosttyConfigTracked(ghosttyConfigFrom(cfg), theme)
		}
		return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Ghostty", err)
	}, "  ✓ Ghostty configured")

	// Configure Zsh
	toolConfigPhase("zsh", "\n▶ Configuring Zsh...", func() ([]tools.MutationEvidence, error) {
		if persistJournal {
			accepted, err := executionTarget("config:zsh", ".zshrc")
			if err != nil {
				return nil, err
			}
			evidence, err := tools.WriteZshConfigAtBoundAuthorityTracked(zshConfigFrom(cfg), theme, accepted.file, accepted.parents, boundLocker)
			return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Zsh", err)
		}
		evidence, err := tools.WriteZshConfigTracked(zshConfigFrom(cfg), theme)
		return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Zsh", err)
	}, "  ✓ Zsh configured with ~/.zshrc")

	// Configure Neovim. The DeepDiveConfig -> NeovimConfig translation is shared
	// with config-apply via neovimConfigFrom; install uses WriteNeovimConfig, which
	// clones the preset repo (an install-only side-effect), whereas config-apply
	// only overlays user prefs.
	neovimCfg := neovimConfigFrom(cfg)
	neovimSuccessMsg := fmt.Sprintf("  ✓ Neovim configured (%s)", neovimCfg.ConfigPreset)
	if neovimCfg.ConfigPreset == "custom" {
		neovimSuccessMsg = "  ✓ Neovim: using existing config (unchanged)"
	}
	toolConfigPhase("neovim", "\n▶ Configuring Neovim...", func() ([]tools.MutationEvidence, error) {
		if persistJournal {
			accepted, err := executionTarget("config:neovim", ".config/nvim")
			if err != nil {
				return nil, err
			}
			evidence, err := tools.WriteNeovimConfigAtBoundAuthorityTracked(neovimCfg, theme, accepted.directory, accepted.parents, stateAuthority)
			return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Neovim", err)
		}
		evidence, err := tools.WriteNeovimConfigTracked(neovimCfg, theme)
		return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Neovim", err)
	}, neovimSuccessMsg)

	// Configure Git
	toolConfigPhase("git", "\n▶ Configuring Git...", func() ([]tools.MutationEvidence, error) {
		if persistJournal {
			rootAccepted, err := executionTarget("config:git", ".gitconfig")
			if err != nil {
				return nil, err
			}
			managedAccepted, err := executionTarget("config:git", gitManagedConfigRelForPlan)
			if err != nil {
				return nil, err
			}
			evidence, err := tools.WriteGitConfigAtBoundAuthoritiesTracked(gitConfigFrom(cfg), theme, rootAccepted.file, rootAccepted.parents, managedAccepted.file, managedAccepted.parents, boundLocker)
			return evidence, wrapMutationError("failed to configure Git", err)
		}
		evidence, err := tools.WriteGitConfigTracked(gitConfigFrom(cfg), theme)
		return evidence, wrapMutationError("failed to configure Git", err)
	}, "  ✓ Git configured with ~/.gitconfig")

	// Configure Yazi
	if !persistJournal {
		toolConfigPhase("yazi", "\n▶ Configuring Yazi...", func() ([]tools.MutationEvidence, error) {
			evidence, err := tools.WriteYaziConfigTracked(yaziConfigFrom(cfg), theme)
			return evidence, wrapMutationError("failed to configure Yazi", err)
		}, "  ✓ Yazi configured")
	} else if configAllowed["yazi"] {
		const yaziHeader = "\n▶ Configuring Yazi..."
		yaziActions := []struct {
			actionID string
			kind     tools.YaziFileKind
			path     string
		}{
			{actionID: "config:yazi:main", kind: tools.YaziFileKindMain, path: plan.yaziConfigPaths.Main},
			{actionID: "config:yazi:keymap", kind: tools.YaziFileKindKeymap, path: plan.yaziConfigPaths.Keymap},
			{actionID: "config:yazi:theme", kind: tools.YaziFileKindTheme, path: plan.yaziConfigPaths.Theme},
		}
		available, reason := coreToolConfigAvailable(installRuntime, "yazi")
		stepLine(yaziHeader)
		if !available {
			emitLine("  ↷ Skipped configuration: " + reason)
			for _, item := range yaziActions {
				markAction(item.actionID, operation.ActionSkipped, reason)
			}
		} else {
			allSucceeded := true
			for _, item := range yaziActions {
				item := item
				succeeded := applyConfigAction(item.actionID, "yazi", func() ([]tools.MutationEvidence, error) {
					target := planTargetPath(home, item.path)
					foundApply := false
					for _, action := range plan.actions() {
						if action.ID != item.actionID {
							continue
						}
						if foundApply {
							return nil, fmt.Errorf("duplicate accepted Yazi action %s", item.actionID)
						}
						if action.Disposition != operation.DispositionApply || action.ToolID != "yazi" || action.Target != target {
							return nil, fmt.Errorf("accepted Yazi action %s does not match frozen target %s", item.actionID, target)
						}
						foundApply = true
					}
					if !foundApply {
						return nil, fmt.Errorf("accepted Yazi action %s is unavailable", item.actionID)
					}
					accepted, err := executionTarget(item.actionID, target)
					if err != nil {
						return nil, err
					}
					evidence, err := tools.WriteYaziFileAtResolvedAuthorityTracked(item.kind, yaziConfigFrom(cfg), theme, plan.yaziConfigPaths, accepted.file, accepted.parents, boundLocker)
					return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Yazi", err)
				})
				allSucceeded = allSucceeded && succeeded
			}
			if allSucceeded {
				emitLine("  ✓ Yazi configured")
			}
		}
	}

	// Configure FZF
	toolConfigPhase("fzf", "\n▶ Configuring FZF...", func() ([]tools.MutationEvidence, error) {
		if persistJournal {
			accepted, err := executionTarget("config:fzf", ".config/fzf/fzf.zsh")
			if err != nil {
				return nil, err
			}
			evidence, err := tools.WriteFzfConfigAtAuthorityTracked(fzfConfigFrom(cfg), theme, accepted.file, accepted.parents, boundLocker)
			return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure FZF", err)
		}
		evidence, err := tools.WriteFzfConfigTracked(fzfConfigFrom(cfg), theme)
		return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure FZF", err)
	}, "  ✓ FZF configured")

	// Configure LazyGit — only when the user selected it in the deep-dive.
	// lazygit is in CLITools (UIGroupCLITools) and therefore has an explicit
	// selection flag; skipping its config when deselected matches user intent.
	if cfg.CLITools["lazygit"] {
		toolConfigPhase("lazygit", "\n▶ Configuring LazyGit...", func() ([]tools.MutationEvidence, error) {
			if persistJournal {
				path, err := tools.LazyGitConfigMutationPath()
				if err != nil {
					return nil, err
				}
				rel := planTargetPath(home, path)
				accepted, err := executionTarget("config:lazygit", rel)
				if err != nil {
					return nil, err
				}
				evidence, err := tools.WriteLazyGitConfigAtAuthorityTracked(lazygitConfigFrom(cfg), theme, accepted.file, accepted.parents, boundLocker)
				return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure LazyGit", err)
			}
			evidence, err := tools.WriteLazyGitConfigTracked(lazygitConfigFrom(cfg), theme)
			return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure LazyGit", err)
		}, "  ✓ LazyGit configured")
	}

	// Configure Btop — only when the user selected it in the deep-dive.
	// btop is in CLITools (UIGroupCLITools) and has an explicit selection flag.
	if cfg.CLITools["btop"] {
		toolConfigPhase("btop", "\n▶ Configuring Btop...", func() ([]tools.MutationEvidence, error) {
			if persistJournal {
				btopCfg := btopConfigFrom(cfg)
				configPath, err := tools.BtopConfigMutationPath()
				if err != nil {
					return nil, err
				}
				themePath, err := tools.BtopThemeMutationPath(btopCfg, theme)
				if err != nil {
					return nil, err
				}
				themeRel := planTargetPath(home, themePath)
				themeAccepted, err := executionTarget("config:btop", themeRel)
				if err != nil {
					return nil, err
				}
				configAccepted, err := executionTarget("config:btop", planTargetPath(home, configPath))
				if err != nil {
					return nil, err
				}
				evidence, err := tools.WriteBtopConfigAtAuthoritiesTracked(btopCfg, theme, themeAccepted.file, themeAccepted.parents, configAccepted.file, configAccepted.parents, boundLocker)
				return evidence, wrapMutationError("failed to configure Btop", err)
			}
			evidence, err := tools.WriteBtopConfigTracked(btopConfigFrom(cfg), theme)
			return evidence, wrapMutationError("failed to configure Btop", err)
		}, "  ✓ Btop configured")
	}

	// Configure Glow — only when the user selected it in the deep-dive.
	// glow is in CLITools (UIGroupCLITools) and has an explicit selection flag.
	if cfg.CLITools["glow"] {
		toolConfigPhase("glow", "\n▶ Configuring Glow...", func() ([]tools.MutationEvidence, error) {
			if persistJournal {
				targets, err := rollbackTargetsForAction(plan, "config:glow")
				if err != nil {
					return nil, fmt.Errorf("resolve accepted Glow target: %w", err)
				}
				if len(targets) != 1 {
					return nil, fmt.Errorf("resolve accepted Glow target: got %d targets", len(targets))
				}
				accepted, err := executionTarget("config:glow", targets[0])
				if err != nil {
					return nil, err
				}
				evidence, err := tools.WriteGlowConfigAtAuthorityTracked(glowConfigFrom(cfg), theme, accepted.file, accepted.parents, boundLocker)
				return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Glow", err)
			}
			evidence, err := tools.WriteGlowConfigTracked(glowConfigFrom(cfg), theme)
			return []tools.MutationEvidence{evidence}, wrapMutationError("failed to configure Glow", err)
		}, "  ✓ Glow configured")
	}

	// Surface all failures: name each failed step so the Error screen lists
	// exactly what went wrong, not just a count + first error.
	finish(aggregateFailures(failures))
}

type selectedToolInstallResult struct {
	successCount int
	skippedCount int
	installed    map[string]bool
	satisfied    map[string]bool
	failed       map[string]bool
	failures     []error
}

type installExecutionSnapshot struct {
	platform        pkg.Platform
	manager         string
	managerIdentity pkg.ExecutableIdentity
	recipes         map[string]operation.InstallRecipe
	detected        map[string]bool
	digests         map[string]string
	authority       map[string]installToolAuthority
}

func wrapMutationError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// runSelectedToolInstalls is the wizard's package/custom-install phase. It is
// isolated from backup and configuration mutation so its plan, progress, error,
// cancellation, and postcondition behavior can be tested without touching the
// user's filesystem.
func runSelectedToolInstalls(
	ctx context.Context,
	selectedTools []string,
	installRuntime toolInstallRuntime,
	emitLine func(string),
	stepLine func(string),
	acceptedSnapshots ...installExecutionSnapshot,
) selectedToolInstallResult {
	result := selectedToolInstallResult{
		installed: make(map[string]bool, len(selectedTools)),
		satisfied: make(map[string]bool, len(selectedTools)),
		failed:    make(map[string]bool, len(selectedTools)),
	}
	mgr := installRuntime.detectManager()
	platform := pkg.Platform("")
	managerName := ""
	if mgr != nil {
		managerName = mgr.Name()
	}
	var accepted *installExecutionSnapshot
	if len(acceptedSnapshots) != 0 {
		accepted = &acceptedSnapshots[0]
		platform = accepted.platform
		if accepted.platform == "" || accepted.manager != managerName {
			result.failures = append(result.failures, fmt.Errorf("install environment changed after review: planned %s/%s, found manager %s", accepted.platform, accepted.manager, managerName))
			return result
		}
		if installRecipesRequireManagerIdentity(accepted.recipes) {
			if err := validateUIManagerExecutableIdentity(mgr, accepted.managerIdentity, false); err != nil {
				result.failures = append(result.failures, err)
				return result
			}
		} else if validUIManagerExecutableIdentity(accepted.managerIdentity) {
			result.failures = append(result.failures, fmt.Errorf("install manager authority is inconsistent"))
			return result
		}
		seen := make(map[string]struct{}, len(selectedTools))
		for _, toolID := range selectedTools {
			if _, duplicate := seen[toolID]; duplicate {
				result.failures = append(result.failures, fmt.Errorf("duplicate selected install %s", toolID))
				return result
			}
			seen[toolID] = struct{}{}
			if _, ok := accepted.recipes[toolID]; !ok {
				result.failures = append(result.failures, fmt.Errorf("selected install %s lacks accepted recipe", toolID))
				return result
			}
			authority, ok := accepted.authority[toolID]
			if !ok || (authority.intent != "install" && authority.intent != "repair") ||
				(authority.presence != health.PresenceMissing && authority.presence != health.PresencePartial) {
				result.failures = append(result.failures, fmt.Errorf("selected install %s lacks accepted typed authority", toolID))
				return result
			}
		}
		if len(seen) != len(accepted.recipes) {
			result.failures = append(result.failures, fmt.Errorf("selected installs do not match accepted install actions"))
			return result
		}
		// Validate every accepted recipe and detector before the first mutation.
		// The per-action check below is repeated because an earlier reviewed action
		// can legitimately satisfy a later overlapping package receipt.
		for _, toolID := range selectedTools {
			recipe := accepted.recipes[toolID]
			if recipe.ToolID != toolID || recipe.Platform != string(platform) || recipe.Manager != managerName {
				result.failed[toolID] = true
				result.failures = append(result.failures, fmt.Errorf("%s: reviewed installer provenance is inconsistent", toolID))
				return result
			}
			digest, digestErr := operation.InstallRecipeDigest(recipe)
			if digestErr != nil || digest != accepted.digests[toolID] {
				result.failed[toolID] = true
				result.failures = append(result.failures, fmt.Errorf("%s: reviewed installer digest is invalid", toolID))
				return result
			}
			acceptedDetected, detectedOK := accepted.detected[toolID]
			if !detectedOK {
				result.failed[toolID] = true
				result.failures = append(result.failures, fmt.Errorf("%s: reviewed detector authority is missing", toolID))
				return result
			}
			if recipeDetectorRequiresManagerIdentity(recipe.Detector) {
				if err := validateUIManagerExecutableIdentity(mgr, accepted.managerIdentity, true); err != nil {
					result.failed[toolID] = true
					result.failures = append(result.failures, fmt.Errorf("%s: %w", toolID, err))
					return result
				}
			}
			currentDetected, detectorErr := installRecipeDetected(recipe, mgr)
			if detectorErr != nil || currentDetected != acceptedDetected {
				result.failed[toolID] = true
				result.failures = append(result.failures, fmt.Errorf("%s: installation state changed after review", toolID))
				return result
			}
		}
	} else {
		platform = installRuntime.detectPlatform()
	}

	if mgr != nil {
		emitLine(fmt.Sprintf("Installing %d tools using %s...", len(selectedTools), mgr.Name()))
	} else {
		emitLine(fmt.Sprintf("Installing %d tools (no package manager detected; manager-independent installers only)...", len(selectedTools)))
	}
	for _, toolID := range selectedTools {
		if err := ctx.Err(); err != nil {
			result.failures = append(result.failures, err)
			return result
		}
		stepLine(fmt.Sprintf("▶ Installing %s...", toolID))

		t, ok := installRuntime.lookupTool(toolID)
		if !ok {
			emitLine(fmt.Sprintf("  ⚠ Unknown tool: %s", toolID))
			result.failures = append(result.failures, fmt.Errorf("%s: unknown tool", toolID))
			result.failed[toolID] = true
			continue
		}

		var recipe operation.InstallRecipe
		currentlyInstalled := false
		if accepted != nil {
			var recipeOK bool
			recipe, recipeOK = accepted.recipes[toolID]
			if !recipeOK {
				emitLine(fmt.Sprintf("  ✗ Cannot install %s: reviewed installer provenance is missing", toolID))
				result.failures = append(result.failures, fmt.Errorf("%s: reviewed installer provenance is missing", toolID))
				result.failed[toolID] = true
				continue
			}
			if recipe.ToolID != toolID || recipe.Platform != string(platform) || recipe.Manager != managerName {
				emitLine(fmt.Sprintf("  ✗ Cannot install %s: reviewed installer provenance is inconsistent", toolID))
				result.failures = append(result.failures, fmt.Errorf("%s: reviewed installer provenance is inconsistent", toolID))
				result.failed[toolID] = true
				continue
			}
			digest, digestErr := operation.InstallRecipeDigest(recipe)
			if digestErr != nil || digest != accepted.digests[toolID] {
				emitLine(fmt.Sprintf("  ✗ Cannot install %s: reviewed installer digest is invalid", toolID))
				result.failures = append(result.failures, fmt.Errorf("%s: reviewed installer digest is invalid", toolID))
				result.failed[toolID] = true
				continue
			}
		} else {
			currentlyInstalled = installRuntime.isToolInstalled(t)
		}
		if currentlyInstalled {
			emitLine(fmt.Sprintf("  ✓ %s already installed", toolID))
			result.successCount++
			result.installed[toolID] = true
			continue
		}
		if accepted != nil {
			acceptedDetected, detectedOK := accepted.detected[toolID]
			if !detectedOK {
				emitLine(fmt.Sprintf("  ✗ Cannot install %s: reviewed detector authority is missing", toolID))
				result.failures = append(result.failures, fmt.Errorf("%s: reviewed detector authority is missing", toolID))
				result.failed[toolID] = true
				continue
			}
			if recipeDetectorRequiresManagerIdentity(recipe.Detector) {
				if err := validateUIManagerExecutableIdentity(mgr, accepted.managerIdentity, true); err != nil {
					emitLine(fmt.Sprintf("  ✗ Failed to verify %s before installation: %v", toolID, err))
					result.failures = append(result.failures, fmt.Errorf("%s: %w", toolID, err))
					result.failed[toolID] = true
					return result
				}
			}
			currentDetected, detectorErr := installRecipeDetected(recipe, mgr)
			if detectorErr == nil && !acceptedDetected && currentDetected {
				emitLine(fmt.Sprintf("  ↷ %s already satisfied after review; skipping install", toolID))
				result.skippedCount++
				result.satisfied[toolID] = true
				continue
			}
			if detectorErr != nil || currentDetected != acceptedDetected {
				emitLine(fmt.Sprintf("  ✗ Cannot install %s: installation state changed after review", toolID))
				result.failures = append(result.failures, fmt.Errorf("%s: installation state changed after review", toolID))
				result.failed[toolID] = true
				return result
			}
			if installRecipeRequiresManagerIdentity(recipe) {
				if err := validateUIManagerExecutableIdentity(mgr, accepted.managerIdentity, true); err != nil {
					emitLine(fmt.Sprintf("  ✗ Failed to install %s: %v", toolID, err))
					result.failures = append(result.failures, fmt.Errorf("%s: %w", toolID, err))
					result.failed[toolID] = true
					return result
				}
			}
			if err := executeInstallRecipe(ctx, recipe, mgr, accepted.managerIdentity, func(line string) { emitLine("  " + line) }); err != nil {
				emitLine(fmt.Sprintf("  ✗ Failed to install %s: %v", toolID, err))
				result.failures = append(result.failures, fmt.Errorf("%s: %w", toolID, err))
				result.failed[toolID] = true
				if errors.Is(err, installapply.ErrManagerIdentityChanged) {
					return result
				}
				continue
			}
		} else {
			if !installerAvailable(t, platform) {
				emitLine(fmt.Sprintf("  ⚠ %s is not available through a supported installer on %s", toolID, platform))
				result.failures = append(result.failures, fmt.Errorf("%s: no supported installer for %s", toolID, platform))
				result.failed[toolID] = true
				continue
			}
			if mgr == nil && requiresPackageManager(t) {
				emitLine(fmt.Sprintf("  ✗ Cannot install %s: no package manager detected", toolID))
				result.failures = append(result.failures, fmt.Errorf("%s: no package manager detected", toolID))
				result.failed[toolID] = true
				continue
			}
			if err := installTool(ctx, t, mgr, platform, func(line string) { emitLine("  " + line) }); err != nil {
				emitLine(fmt.Sprintf("  ✗ Failed to install %s: %v", toolID, err))
				result.failures = append(result.failures, fmt.Errorf("%s: %w", toolID, err))
				result.failed[toolID] = true
				continue
			}
		}
		postcondition := false
		if accepted != nil {
			if recipeDetectorRequiresManagerIdentity(recipe.Detector) {
				if err := validateUIManagerExecutableIdentity(mgr, accepted.managerIdentity, true); err != nil {
					emitLine(fmt.Sprintf("  ✗ %s install verification failed: %v", toolID, err))
					result.failures = append(result.failures, fmt.Errorf("%s: %w", toolID, err))
					result.failed[toolID] = true
					return result
				}
			}
			postcondition, _ = installRecipeDetected(recipe, mgr)
		} else {
			postcondition = installRuntime.isToolInstalled(t)
		}
		if !postcondition {
			emitLine(fmt.Sprintf("  ✗ %s installer completed but the tool is still not detected", toolID))
			result.failures = append(result.failures, fmt.Errorf("%s: install postcondition failed (tool not detected)", toolID))
			result.failed[toolID] = true
			continue
		}

		emitLine(fmt.Sprintf("  ✓ %s installed successfully", toolID))
		result.successCount++
		result.installed[toolID] = true
	}

	return result
}

func installRecipeDetected(recipe operation.InstallRecipe, mgr pkg.PackageManager) (bool, error) {
	return installapply.DetectRecipe(recipe, mgr)
}

func executeInstallRecipe(ctx context.Context, recipe operation.InstallRecipe, mgr pkg.PackageManager, managerIdentity pkg.ExecutableIdentity, emitLine func(string)) error {
	return installapply.ExecuteRecipe(ctx, recipe, mgr, managerIdentity, emitLine)
}

// aggregateFailures builds the final installation error from a slice of per-step
// failures. A single failure is returned as-is. Two or more failures produce a
// "Failed steps:" list that names every failed phase so the Error screen gives
// the user an actionable summary rather than "N steps failed; first: ...".
func aggregateFailures(failures []error) error {
	switch len(failures) {
	case 0:
		return nil
	case 1:
		return failures[0]
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%d steps failed. Failed steps:\n", len(failures))
		for i, err := range failures {
			fmt.Fprintf(&b, "  %d. %v\n", i+1, err)
		}
		return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
	}
}

type utilityInstallResult struct {
	Evidence  []tools.MutationEvidence
	Attempted []string
	Failed    string
	Err       error
}

type utilityInstaller func(home, name string, content []byte) (tools.MutationEvidence, error)

func installUtilitiesTracked(utilities map[string]bool) utilityInstallResult {
	return installUtilitiesTrackedWith(utilities, installScriptFileTracked)
}

func installUtilitiesAtAuthorityTracked(authority map[string]map[string]acceptedTarget, utilities map[string]bool, locker operation.Locker, beforeAttempt func(string)) utilityInstallResult {
	return installUtilitiesTrackedWithBefore(utilities, beforeAttempt, func(home, name string, content []byte) (tools.MutationEvidence, error) {
		rel := filepath.ToSlash(filepath.Join(".local", "bin", name))
		accepted, ok := authority["helper:"+name][rel]
		if !ok || accepted.kind != acceptedFileTarget || !accepted.parents.Tracked() {
			return tools.MutationEvidence{}, fmt.Errorf("execution authority unavailable for helper %s", name)
		}
		return installScriptFileAtAuthorityTracked(home, name, content, accepted.file, accepted.parents, locker)
	})
}

func installUtilitiesTrackedWith(utilities map[string]bool, install utilityInstaller) utilityInstallResult {
	return installUtilitiesTrackedWithBefore(utilities, nil, install)
}

func installUtilitiesTrackedWithBefore(utilities map[string]bool, beforeAttempt func(string), install utilityInstaller) utilityInstallResult {
	var result utilityInstallResult
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			result.Err = fmt.Errorf("cannot determine home directory: %w", err)
			return result
		}
	}

	// Install selected utility scripts
	for _, name := range enabledHelpers(utilities) {
		result.Attempted = append(result.Attempted, name)
		if beforeAttempt != nil {
			beforeAttempt(name)
		}
		script := scripts.GetScript(name)
		if script == "" {
			result.Failed = name
			result.Err = fmt.Errorf("embedded helper %s is unavailable", name)
			return result
		}
		evidence, err := install(home, name, []byte(script))
		if err != nil {
			result.Failed = name
			result.Err = fmt.Errorf("cannot write %s: %w", name, err)
			return result
		}
		result.Evidence = append(result.Evidence, evidence)
	}

	return result
}

func installScriptFileTracked(home, name string, content []byte) (evidence tools.MutationEvidence, returnErr error) {
	return installScriptFileWithAuthority(home, name, content, nil, nil)
}

func installScriptFileAtAuthorityTracked(home, name string, content []byte, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (tools.MutationEvidence, error) {
	if !parents.Tracked() || locker == nil {
		return tools.MutationEvidence{}, fmt.Errorf("%w: helper authority is incomplete", safefile.ErrParentChanged)
	}
	return installScriptFileWithAuthority(home, name, content, &accepted, parents, locker)
}

func installScriptFileWithAuthority(home, name string, content []byte, accepted *safefile.Revision, parents *safefile.ParentChain, lockers ...operation.Locker) (evidence tools.MutationEvidence, returnErr error) {
	if name == "" || name == "." || filepath.Base(name) != name {
		return tools.MutationEvidence{}, fmt.Errorf("invalid utility name %q", name)
	}
	rel := filepath.ToSlash(filepath.Join(".local", "bin", name))
	targetPath := filepath.Join(home, filepath.FromSlash(rel))
	locker := operation.DefaultLocker
	if len(lockers) != 0 && lockers[0] != nil {
		locker = lockers[0]
	}
	release, err := locker("helper", targetPath)
	if err != nil {
		return tools.MutationEvidence{}, fmt.Errorf("lock utility %s: %w", name, err)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("unlock utility %s: %w", name, releaseErr))
		}
	}()
	parent := filepath.ToSlash(filepath.Dir(rel))
	if accepted == nil {
		if err := safefile.EnsureDirectoryWithin(home, parent, 0o700); err != nil {
			return tools.MutationEvidence{}, fmt.Errorf("create utility directory: %w", err)
		}
	}
	var existing []byte
	var revision safefile.Revision
	if accepted != nil && parents != nil {
		existing, revision, err = safefile.ReadWithinAuthorized(home, rel, parents)
	} else {
		existing, revision, err = safefile.ReadWithin(home, rel)
	}
	if err != nil {
		return tools.MutationEvidence{}, fmt.Errorf("inspect utility %s: %w", name, err)
	}
	if accepted != nil && revision != *accepted {
		return tools.MutationEvidence{}, fmt.Errorf("%w: utility %s changed after plan acceptance", safefile.ErrRevisionChanged, name)
	}
	if revision.Exists() && !bytes.Equal(existing, content) {
		return tools.MutationEvidence{}, fmt.Errorf("refusing to replace existing unowned utility %s at ~/%s", name, rel)
	}
	expected := revision
	if accepted != nil {
		expected = *accepted
	}
	var committed safefile.Revision
	if accepted != nil {
		if parents != nil {
			committed, err = safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(home, rel, expected, parents, content, 0o700)
		} else {
			committed, err = safefile.ReplaceWithinRevisionNoCreateTracked(home, rel, expected, content, 0o700)
		}
	} else {
		committed, err = safefile.ReplaceWithinRevisionTracked(home, rel, expected, content, 0o700)
	}
	if err != nil {
		return tools.MutationEvidence{}, err
	}
	return tools.MutationEvidence{Path: filepath.Join(home, filepath.FromSlash(rel)), Revision: committed, Parents: parents}, nil
}

// alwaysConfiguredToolIDs are configured unconditionally later in the wizard
// worker, so a clean-machine plan must also install their packages. Previously
// only tools represented by group-selection maps entered the package plan,
// allowing a "successful" first run with configuration files but no core
// executables.
var alwaysConfiguredToolIDs = []string{
	"ghostty",
	"tmux",
	"zsh",
	"neovim",
	"git",
	"yazi",
	"fzf",
}

func coreToolConfigAvailable(installRuntime toolInstallRuntime, toolID string) (bool, string) {
	t, ok := installRuntime.lookupTool(toolID)
	if !ok {
		return false, fmt.Sprintf("%s is missing from the tool registry", toolID)
	}
	platform := installRuntime.detectPlatform()
	if installRuntime.isToolInstalled(t) {
		return true, ""
	}
	if installerAvailable(t, platform) {
		return false, fmt.Sprintf("%s was not detected after installation; configuration was not written", t.Name())
	}
	return false, fmt.Sprintf("%s has no supported installer for %s and no external installation was detected", t.Name(), platform)
}

// collectSelectedTools gathers all missing tool IDs selected in deep dive config,
// including supported core tools whose configuration phases always run.
func (a *App) collectSelectedTools() []string {
	// Production planning owns cache initialization. The injected helper below
	// deliberately consumes only supplied App observations/runtime dependencies
	// so cross-platform tests cannot accidentally probe the host machine.
	a.ensureInstallCache()
	return a.collectSelectedToolsWithRuntime(defaultToolInstallRuntime())
}

func (a *App) collectSelectedToolsWithRuntime(installRuntime toolInstallRuntime) []string {
	var selected []string
	selectedSet := make(map[string]bool)
	addMissing := func(id string, enabled bool) {
		if enabled && !a.manageInstalled[id] && !selectedSet[id] {
			selected = append(selected, id)
			selectedSet[id] = true
		}
	}

	// Core tools enter package work only where the registry has a supported
	// package route. Unsupported-but-external tools stay out of install work and
	// may still be configured by the worker after direct detection.
	platform := installRuntime.detectPlatform()
	for _, id := range alwaysConfiguredToolIDs {
		t, ok := installRuntime.lookupTool(id)
		if !ok {
			continue
		}
		supported := installerAvailable(t, platform)
		addMissing(id, supported)
	}

	// CLI Tools (lazygit, lazydocker, btop, glow, claude-code)
	for id, enabled := range a.deepDiveConfig.CLITools {
		addMissing(id, enabled)
	}

	// GUI Apps (zen-browser, cursor, lm-studio, obs)
	for id, enabled := range a.deepDiveConfig.GUIApps {
		addMissing(id, enabled)
	}

	// CLI Utilities (bat, eza, zoxide, ripgrep, fd, delta, fswatch)
	for id, enabled := range a.deepDiveConfig.CLIUtilities {
		addMissing(id, enabled)
	}

	// Note: Utilities (hk, caff, sshh) are shell scripts handled by installUtilities()
	// They don't go through the package manager

	// macOS Apps (rectangle, raycast, iina, etc.) - only on macOS
	if platform == pkg.PlatformMacOS {
		for id, enabled := range a.deepDiveConfig.MacApps {
			addMissing(id, enabled)
		}
	}

	return selected
}

// listenUpdateStreamCmd reads the next event from the update stream channel and
// returns it as a message. Update re-subscribes by returning this Cmd again
// until it sees a `done` event (mirrors listenInstallEventsCmd).
func (a *App) listenUpdateStreamCmd() tea.Cmd {
	ch := a.updateStream
	return func() tea.Msg {
		if ch == nil {
			return updateStreamMsg{done: true}
		}
		ev, ok := <-ch
		if !ok {
			// Channel closed without a done event; treat as completion.
			return updateStreamMsg{done: true}
		}
		return ev
	}
}

// streamingUpdateCmd starts a streaming update of the given packages. A detached
// worker goroutine BUILDS the streaming command and drains its output, writing
// each line to a.updateStream (so lines render LIVE) and emitting a final `done`
// event with the results/error before closing the channel. The worker MUST NOT
// touch any App field; all App mutation happens in Update on the main loop.
//
// Constructing the manager command is done INSIDE the worker, not here, because
// mgr.UpdateStreaming can do blocking pre-work (e.g. apt's `apt update` index
// refresh) that would otherwise freeze the Bubble Tea event loop for seconds.
// Only the cancelable context (a.streamCancel) and the stream channel are set on
// the main loop. We do NOT set a.streamCmd: cancelling the parent context
// propagates into the RunStreaming-derived context and kills the subprocess, so
// teardownStream()'s a.streamCancel() call is sufficient (same contract as
// streamingInstallToolCmd). The returned Cmd listens for the first event.
func (a *App) streamingUpdateCmd(packages []pkg.Package) tea.Cmd {
	mgr := pkg.DetectManager()
	if mgr == nil {
		return func() tea.Msg {
			return updateStreamMsg{done: true, err: fmt.Errorf("no package manager detected")}
		}
	}

	var pkgNames []string
	for _, p := range packages {
		pkgNames = append(pkgNames, p.Name)
	}

	// Cancelable context stored on App so navigate-away / Ctrl+C / teardownStream()
	// cancels it, which (via exec.CommandContext inside RunStreaming) stops the
	// subprocess and unblocks the worker's bounded-channel sends instead of leaking
	// them. Set on the main loop; the worker only reads ctx.
	ctx, cancel := context.WithCancel(context.Background())
	a.streamCancel = cancel

	// Buffered so the worker can make progress without blocking on a slow
	// consumer; the listen Cmd drains it one event at a time.
	stream := make(chan updateStreamMsg, 64)
	a.updateStream = stream

	go func() {
		defer close(stream)

		// Build the streaming command off the UI goroutine: UpdateStreaming may run
		// blocking pre-work (apt index refresh) that must not stall the event loop.
		cmd, err := mgr.UpdateStreaming(ctx, pkgNames...)
		if err != nil {
			select {
			case stream <- updateStreamMsg{done: true, err: err}:
			case <-ctx.Done():
			}
			return
		}
		if cmd == nil {
			// Nothing to upgrade (no-op): clean completion.
			select {
			case stream <- updateStreamMsg{done: true}:
			case <-ctx.Done():
			}
			return
		}

		for line := range cmd.Output {
			select {
			case stream <- updateStreamMsg{line: line}:
			case <-ctx.Done():
				return
			}
		}
		err = cmd.Wait()

		// A batch `brew/apt/pacman upgrade a b c` that exits non-zero has NOT
		// necessarily failed every package: the manager upgrades the packages it
		// can and fails the rest. Marking the whole batch failed (Success = err==nil
		// for every package) mis-reported the ones that actually upgraded AND made
		// finishUpdate short-circuit to a blanket "Update failed". So on a batch
		// error (when not cancelled) re-check which of our packages are STILL
		// outdated: a package no longer outdated did upgrade.
		var stillOutdated map[string]bool
		recheckOK := false
		if err != nil && ctx.Err() == nil {
			stillOutdated, recheckOK = recheckOutdatedNames(mgr, packages)
		}

		results := make([]pkg.UpdateResult, 0, len(packages))
		for _, p := range packages {
			switch {
			case err == nil:
				results = append(results, pkg.UpdateResult{Package: p, Success: true})
			case recheckOK && !stillOutdated[p.Name]:
				results = append(results, pkg.UpdateResult{Package: p, Success: true})
			default:
				results = append(results, pkg.UpdateResult{Package: p, Success: false, Error: err})
			}
		}

		// When per-package results are authoritative (the recheck succeeded), drop
		// the top-level error so finishUpdate counts the results ("Updated N,
		// failed M") instead of short-circuiting on a batch error. If the recheck
		// failed we could not verify, so keep the conservative all-failed report
		// with the original error.
		doneErr := err
		if err != nil && recheckOK {
			doneErr = nil
		}

		select {
		case stream <- updateStreamMsg{done: true, results: results, err: doneErr}:
		case <-ctx.Done():
		}
	}()

	return a.listenUpdateStreamCmd()
}

// reliableOutdated returns the outdated set to use as a post-upgrade failure
// oracle. For Homebrew it uses a NON-greedy `brew outdated`: the default
// CheckOutdated is `--greedy`, which perpetually lists auto-updating and :latest
// casks as outdated no matter whether an upgrade succeeded. Counting those as
// "still outdated" would falsely mark them failed after any partial-batch failure.
// The non-greedy list only contains packages whose version brew can verify, so a
// package that remains in it genuinely failed to upgrade — keeping formulae (and
// version-tracked casks) honest while excluding the auto-updaters brew cannot
// judge. Every other manager's CheckOutdated is already a reliable oracle.
func reliableOutdated(mgr pkg.PackageManager) ([]pkg.Package, error) {
	if bm, ok := mgr.(*pkg.BrewManager); ok {
		return bm.CheckOutdatedNonGreedy()
	}
	return mgr.CheckOutdated()
}

// recheckOutdatedNames re-queries the package manager for still-outdated packages
// after a batch update and returns, for each package in `packages`, whether it
// remains outdated. ok is false if the re-check itself failed (the manager query
// errored), in which case the caller keeps its conservative report rather than
// guessing. Keyed by package name, which matches how UpdateStreaming was invoked
// (a list of names). The oracle comes from reliableOutdated so greedy/auto-update
// casks are not falsely counted as failed (see reliableOutdated).
func recheckOutdatedNames(mgr pkg.PackageManager, packages []pkg.Package) (stillOutdated map[string]bool, ok bool) {
	outdated, err := reliableOutdated(mgr)
	if err != nil {
		return nil, false
	}
	outdatedSet := make(map[string]bool, len(outdated))
	for _, p := range outdated {
		outdatedSet[p.Name] = true
	}
	stillOutdated = make(map[string]bool, len(packages))
	for _, p := range packages {
		stillOutdated[p.Name] = outdatedSet[p.Name]
	}
	return stillOutdated, true
}

// streamingUpdateAllCmd starts a streaming update of all packages, using the
// same live-streaming worker pattern as streamingUpdateCmd. The manager command
// is built INSIDE the worker goroutine because UpdateAllStreaming can do blocking
// pre-work (brew's `brew outdated --greedy` pre-check, apt's index refresh) that
// must not freeze the Bubble Tea event loop. Only a.streamCancel and the stream
// channel are set on the main loop; a.streamCmd is deliberately not set (context
// cancellation is sufficient for teardown — see streamingUpdateCmd).
func (a *App) streamingUpdateAllCmd() tea.Cmd {
	mgr := pkg.DetectManager()
	if mgr == nil {
		return func() tea.Msg {
			return updateStreamMsg{done: true, err: fmt.Errorf("no package manager detected")}
		}
	}

	// Cancelable context (see streamingUpdateCmd) so teardown stops the subprocess
	// and unblocks the worker's bounded-channel sends. Set on the main loop.
	ctx, cancel := context.WithCancel(context.Background())
	a.streamCancel = cancel

	stream := make(chan updateStreamMsg, 64)
	a.updateStream = stream

	go func() {
		defer close(stream)

		// Build the streaming command off the UI goroutine: UpdateAllStreaming may
		// run blocking pre-work (greedy outdated pre-check / apt index refresh) that
		// must not stall the event loop.
		cmd, err := mgr.UpdateAllStreaming(ctx)
		if err != nil {
			select {
			case stream <- updateStreamMsg{done: true, err: err}:
			case <-ctx.Done():
			}
			return
		}
		if cmd == nil {
			// Nothing outdated (no-op): clean completion.
			select {
			case stream <- updateStreamMsg{done: true}:
			case <-ctx.Done():
			}
			return
		}

		for line := range cmd.Output {
			select {
			case stream <- updateStreamMsg{line: line}:
			case <-ctx.Done():
				return
			}
		}
		err = cmd.Wait()
		select {
		case stream <- updateStreamMsg{done: true, err: err}:
		case <-ctx.Done():
		}
	}()

	return a.listenUpdateStreamCmd()
}

// saveInstallerConfig saves theme and nav style during installer flow. It
// returns any save error so the caller can surface it: silently dropping it left
// the user's theme / nav-style / animation preferences unpersisted with no
// indication anything went wrong.
func (a *App) saveInstallerConfig() error {
	return saveInstallerPreferences(a.theme, a.navStyle, a.animationsEnabled)
}

func saveInstallerPreferences(theme, navStyle string, animationsEnabled bool) error {
	_, err := saveInstallerPreferencesTracked(theme, navStyle, animationsEnabled)
	return err
}

func saveInstallerPreferencesTracked(theme, navStyle string, animationsEnabled bool) (tools.MutationEvidence, error) {
	g, err := config.LoadGlobalConfig()
	if err != nil {
		return tools.MutationEvidence{}, fmt.Errorf("failed to load global config: %w", err)
	}
	g.Theme = theme
	g.NavStyle = navStyle
	g.DisableAnimations = !animationsEnabled

	// Save synchronously since we're about to start installation
	if err := config.SaveGlobalConfig(g); err != nil {
		return tools.MutationEvidence{}, err
	}
	revision, ok := config.GlobalConfigRevision(g)
	if !ok || !revision.Exists() {
		return tools.MutationEvidence{}, fmt.Errorf("global config save returned no tracked revision")
	}
	return tools.MutationEvidence{Path: filepath.Join(config.ConfigDir(), "global.json"), Revision: revision}, nil
}

func savePlannedInstallerPreferencesTracked(plan *installPlan, home, plannedTarget string, authority acceptedTarget, locker operation.Locker) (tools.MutationEvidence, error) {
	planned, err := plan.plannedGlobalConfig()
	if err != nil {
		return tools.MutationEvidence{}, err
	}
	actionTargets, err := rollbackTargetsForAction(plan, "state:global")
	if err != nil {
		return tools.MutationEvidence{}, fmt.Errorf("resolve accepted global target: %w", err)
	}
	if len(actionTargets) != 1 || actionTargets[0] != plannedTarget {
		return tools.MutationEvidence{}, fmt.Errorf("resolve accepted global target: got %d targets", len(actionTargets))
	}
	cleanTarget := filepath.ToSlash(filepath.Clean(filepath.FromSlash(plannedTarget)))
	if !filepath.IsAbs(home) || filepath.IsAbs(plannedTarget) || cleanTarget == "." || cleanTarget == "" || cleanTarget == ".." || strings.HasPrefix(cleanTarget, "../") {
		return tools.MutationEvidence{}, fmt.Errorf("accepted global target must be a HOME-relative path")
	}
	absolutePath := filepath.Join(filepath.Clean(home), filepath.FromSlash(cleanTarget))
	if authority.kind != acceptedFileTarget || !authority.file.Tracked() || !authority.parents.Tracked() {
		return tools.MutationEvidence{}, fmt.Errorf("global execution authority is incomplete")
	}
	revision, err := config.SaveGlobalConfigAtPathBoundAuthorityTracked(absolutePath, planned, authority.file, authority.parents, locker)
	if err != nil {
		return tools.MutationEvidence{}, err
	}
	return tools.MutationEvidence{Path: absolutePath, Revision: revision, Parents: authority.parents}, nil
}
