package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/config"
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
	line    string // a line of output to append (empty if none)
	stepInc bool   // advance the progress step counter
	done    bool   // the install/configure sequence finished
	err     error  // final error (only meaningful when done)
	context string // last few output lines for error context (only when done)
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
	lookupTool      func(string) (tools.Tool, bool)
	detectManager   func() pkg.PackageManager
	detectPlatform  func() pkg.Platform
	isToolInstalled func(tools.Tool) bool
	autoBackup      func() (autoBackupResult, error)
}

func defaultToolInstallRuntime() toolInstallRuntime {
	reg := tools.GetRegistry()
	return toolInstallRuntime{
		lookupTool:     reg.Get,
		detectManager:  pkg.DetectManager,
		detectPlatform: pkg.DetectPlatform,
		isToolInstalled: func(t tools.Tool) bool {
			return t.IsInstalled()
		},
		autoBackup: autoBackupIfEnabled,
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

	cmd, err := m.PackageManager.InstallStreaming(m.ctx, packages...)
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
	a.installStep = 0
	a.installPlannedSteps = 0
	a.installOutput = []string{}

	// Validate and persist the global record before observing the machine or
	// starting any backup/package/config action. Continuing after a malformed,
	// future-schema, or unwritable global.json would apply compiled defaults to
	// real application configs while failing to persist the desired state.
	if err := a.saveInstallerConfig(); err != nil {
		return func() tea.Msg {
			return installDoneMsg{err: fmt.Errorf("installation blocked by global config error: %w", err)}
		}
	}

	// Collect all selected tools from deep dive config
	selectedTools := a.collectSelectedTools()

	// Compute the total number of step-increments the worker will emit so the
	// progress fraction in the View is accurate. Always-core phases (utilities,
	// tmux, ghostty, zsh, neovim, git, yazi, fzf) each emit one step. Selected
	// gated phases (claude-code, lazygit, btop, glow) contribute one step each
	// when their selection flag is set. Each selected tool package-install also
	// emits one step.
	plannedSteps := len(selectedTools) // one stepLine per tool install
	// Always-core config phases: utilities + tmux + ghostty + zsh + neovim + git + yazi + fzf
	const alwaysCoreSteps = 8
	plannedSteps += alwaysCoreSteps
	// Deep-snapshot deepDiveConfig on the Update goroutine BEFORE it is handed to the
	// worker below. A plain *a.deepDiveConfig is only a SHALLOW copy: its map/slice
	// fields (Utilities, CLITools, ClaudeCodeMCPs, ZshAliases, …) keep ALIASING the
	// live maps owned by a.deepDiveConfig, and the worker ranges over them (e.g.
	// ApplyConfigWithMCPs over cfg.ClaudeCodeMCPs). If a config screen mutated those
	// maps concurrently that range would be a fatal concurrent map read/write — the
	// exact aliasing the standalone path eliminated with this same helper. It is
	// currently mitigated only because progressScreen blocks navigation during
	// install; snapshotDeepDiveConfig clones every reference field so the worker owns
	// its data regardless. This snapshot also drives the planned-steps reads below.
	cfg := snapshotDeepDiveConfig(a.deepDiveConfig)
	if cfg.CLITools["claude-code"] || cfg.Utilities["claude-code"] {
		plannedSteps++ // claude-code step
	}
	if cfg.CLITools["lazygit"] {
		plannedSteps++
	}
	if cfg.CLITools["btop"] {
		plannedSteps++
	}
	if cfg.CLITools["glow"] {
		plannedSteps++
	}
	a.installPlannedSteps = plannedSteps

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

	// cfg is a DEEP snapshot of deepDiveConfig (taken above via
	// snapshotDeepDiveConfig, whose clones the planned-steps computation reused);
	// theme is snapshotted here. Both are passed by value to the worker so it never
	// reads App fields — nor the live config maps they used to alias — after this
	// point, even if the Update loop mutates them concurrently.
	theme := a.theme

	go runInstallWorker(ctx, events, selectedTools, cfg, theme)

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

// runInstallWorker performs the entire install/configure sequence on a detached
// goroutine, emitting progress as installEventMsg values. It MUST NOT touch any
// App field. It closes the channel when finished.
// savePrefsErr is retained as an optional test/compatibility guard for callers
// that captured global-config validation before entering the worker. Any such
// error is fatal before backup, package, utility, or application-config work.
func runInstallWorker(ctx context.Context, events chan installEventMsg, selectedTools []string, cfg DeepDiveConfig, theme string, savePrefsErr ...error) {
	runInstallWorkerWithRuntime(ctx, events, selectedTools, cfg, theme, defaultToolInstallRuntime(), savePrefsErr...)
}

// runInstallWorkerWithRuntime is the dependency-injected worker used by focused
// install-dispatch tests. Production callers use runInstallWorker above.
func runInstallWorkerWithRuntime(ctx context.Context, events chan installEventMsg, selectedTools []string, cfg DeepDiveConfig, theme string, installRuntime toolInstallRuntime, savePrefsErr ...error) {
	defer close(events)

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

	finish := func(err error) {
		var errCtx string
		if err != nil && len(output) > 0 {
			start := 0
			if len(output) > 8 {
				start = len(output) - 8
			}
			errCtx = strings.Join(output[start:], "\n")
		}
		terminal := installEventMsg{done: true, err: err, context: errCtx}
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

	// Auto-backup before making changes (if enabled). The result is honest:
	// it only reports a created backup when at least one file was captured and
	// the manifest persisted, so we never claim a rollback point exists right
	// before overwriting the user's dotfiles (C5).
	backupRes, err := installRuntime.autoBackup()
	if err != nil {
		finish(fmt.Errorf("auto-backup failed; installation stopped before mutation: %w", err))
		return
	} else if backupRes.enabled {
		if backupRes.count > 0 {
			emitLine(fmt.Sprintf("✓ Auto-backup created before installation (%d file(s))", backupRes.count))
		} else {
			emitLine("⚠ Auto-backup captured 0 files (nothing to roll back)")
		}
		// The backup succeeded but pruning old backups did not; surface it so the
		// stalled retention policy is visible rather than silently swallowed.
		if backupRes.cleanupErr != nil {
			emitLine(fmt.Sprintf("⚠ Backup retention cleanup failed: %v", backupRes.cleanupErr))
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
		result := runSelectedToolInstalls(ctx, selectedTools, installRuntime, emitLine, stepLine)
		for _, installErr := range result.failures {
			noteFailure(installErr)
		}
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}
		if result.successCount == len(selectedTools) {
			emitLine(fmt.Sprintf("\n✓ All %d tools installed successfully!", result.successCount))
		} else {
			emitLine(fmt.Sprintf("\n✓ Installed %d/%d tools", result.successCount, len(selectedTools)))
		}
	}

	// configPhase runs a single configuration step, emitting a header line,
	// advancing the progress step, and recording any failure.
	configPhase := func(header string, run func() error, okLine string) bool {
		stepLine(header)
		if err := run(); err != nil {
			emitLine(fmt.Sprintf("  ⚠ %v", err))
			noteFailure(err)
			return false
		} else if okLine != "" {
			emitLine(okLine)
		}
		return true
	}
	toolConfigPhase := func(toolID, header string, run func() error, okLine string) bool {
		available, reason := coreToolConfigAvailable(installRuntime, toolID)
		if !available {
			stepLine(header)
			emitLine("  ↷ Skipped configuration: " + reason)
			return false
		}
		return configPhase(header, run, okLine)
	}

	// Install dotfiles binary and utilities to ~/.local/bin
	configPhase("\n▶ Installing dotfiles utilities...", func() error {
		if err := installUtilities(cfg.Utilities); err != nil {
			return fmt.Errorf("Failed to install utilities: %w", err)
		}
		return nil
	}, "  ✓ Utilities installed to ~/.local/bin")

	// Configure tmux with TPM plugins. The DeepDiveConfig -> TmuxConfig translation
	// is shared with config-apply via tmuxConfigFrom; install additionally clones
	// TPM (SetupTPM), which is an install-only side-effect.
	tmuxCfg := tmuxConfigFrom(cfg)
	tmuxConfigured := toolConfigPhase("tmux", "\n▶ Configuring tmux...", func() error {
		if err := tools.SetupTPM(tmuxCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure tmux: %w", err)
		}
		return nil
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
		toolConfigPhase("claude-code", "\n▶ Configuring Claude Code MCP servers...", func() error {
			claudeTool := tools.NewClaudeCodeTool()
			if err := claudeTool.ApplyConfigWithMCPs(cfg.ClaudeCodeMCPs); err != nil {
				return fmt.Errorf("Failed to configure Claude MCP: %w", err)
			}
			return nil
		}, fmt.Sprintf("  ✓ Claude Code configured with %d MCP server(s)", enabledCount))
	}

	// Configure Ghostty
	toolConfigPhase("ghostty", "\n▶ Configuring Ghostty...", func() error {
		if err := tools.WriteGhosttyConfig(ghosttyConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure Ghostty: %w", err)
		}
		return nil
	}, "  ✓ Ghostty configured")

	// Configure Zsh
	toolConfigPhase("zsh", "\n▶ Configuring Zsh...", func() error {
		if err := tools.WriteZshConfig(zshConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure Zsh: %w", err)
		}
		return nil
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
	toolConfigPhase("neovim", "\n▶ Configuring Neovim...", func() error {
		if err := tools.WriteNeovimConfig(neovimCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Neovim: %w", err)
		}
		return nil
	}, neovimSuccessMsg)

	// Configure Git
	toolConfigPhase("git", "\n▶ Configuring Git...", func() error {
		if err := tools.WriteGitConfig(gitConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure Git: %w", err)
		}
		return nil
	}, "  ✓ Git configured with ~/.gitconfig")

	// Configure Yazi
	toolConfigPhase("yazi", "\n▶ Configuring Yazi...", func() error {
		if err := tools.WriteYaziConfig(yaziConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure Yazi: %w", err)
		}
		return nil
	}, "  ✓ Yazi configured")

	// Configure FZF
	toolConfigPhase("fzf", "\n▶ Configuring FZF...", func() error {
		if err := tools.WriteFzfConfig(fzfConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure FZF: %w", err)
		}
		return nil
	}, "  ✓ FZF configured")

	// Configure LazyGit — only when the user selected it in the deep-dive.
	// lazygit is in CLITools (UIGroupCLITools) and therefore has an explicit
	// selection flag; skipping its config when deselected matches user intent.
	if cfg.CLITools["lazygit"] {
		toolConfigPhase("lazygit", "\n▶ Configuring LazyGit...", func() error {
			if err := tools.WriteLazyGitConfig(lazygitConfigFrom(cfg), theme); err != nil {
				return fmt.Errorf("Failed to configure LazyGit: %w", err)
			}
			return nil
		}, "  ✓ LazyGit configured")
	}

	// Configure Btop — only when the user selected it in the deep-dive.
	// btop is in CLITools (UIGroupCLITools) and has an explicit selection flag.
	if cfg.CLITools["btop"] {
		toolConfigPhase("btop", "\n▶ Configuring Btop...", func() error {
			if err := tools.WriteBtopConfig(btopConfigFrom(cfg), theme); err != nil {
				return fmt.Errorf("Failed to configure Btop: %w", err)
			}
			return nil
		}, "  ✓ Btop configured")
	}

	// Configure Glow — only when the user selected it in the deep-dive.
	// glow is in CLITools (UIGroupCLITools) and has an explicit selection flag.
	if cfg.CLITools["glow"] {
		toolConfigPhase("glow", "\n▶ Configuring Glow...", func() error {
			if err := tools.WriteGlowConfig(glowConfigFrom(cfg), theme); err != nil {
				return fmt.Errorf("Failed to configure Glow: %w", err)
			}
			return nil
		}, "  ✓ Glow configured")
	}

	// Surface all failures: name each failed step so the Error screen lists
	// exactly what went wrong, not just a count + first error.
	finish(aggregateFailures(failures))
}

type selectedToolInstallResult struct {
	successCount int
	installed    map[string]bool
	failures     []error
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
) selectedToolInstallResult {
	result := selectedToolInstallResult{installed: make(map[string]bool, len(selectedTools))}
	mgr := installRuntime.detectManager()
	platform := installRuntime.detectPlatform()

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
			continue
		}

		if installRuntime.isToolInstalled(t) {
			emitLine(fmt.Sprintf("  ✓ %s already installed", toolID))
			result.successCount++
			result.installed[toolID] = true
			continue
		}

		if !installerAvailable(t, platform) {
			emitLine(fmt.Sprintf("  ⚠ %s is not available through a supported installer on %s", toolID, platform))
			result.failures = append(result.failures, fmt.Errorf("%s: no supported installer for %s", toolID, platform))
			continue
		}
		if mgr == nil && requiresPackageManager(t) {
			emitLine(fmt.Sprintf("  ✗ Cannot install %s: no package manager detected", toolID))
			result.failures = append(result.failures, fmt.Errorf("%s: no package manager detected", toolID))
			continue
		}

		if err := installTool(ctx, t, mgr, platform, func(line string) {
			emitLine("  " + line)
		}); err != nil {
			emitLine(fmt.Sprintf("  ✗ Failed to install %s: %v", toolID, err))
			result.failures = append(result.failures, fmt.Errorf("%s: %w", toolID, err))
			continue
		}
		if !installRuntime.isToolInstalled(t) {
			emitLine(fmt.Sprintf("  ✗ %s installer completed but the tool is still not detected", toolID))
			result.failures = append(result.failures, fmt.Errorf("%s: install postcondition failed (tool not detected)", toolID))
			continue
		}

		emitLine(fmt.Sprintf("  ✓ %s installed successfully", toolID))
		result.successCount++
		result.installed[toolID] = true
	}

	return result
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

// installUtilities installs selected helper scripts to ~/.local/bin. The main
// dotfiles executable remains owned by its package manager/build installation;
// copying the running executable here created a second, PATH-order-dependent
// product installation that could shadow Homebrew upgrades.
func installUtilities(utilities map[string]bool) error {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
	}

	// Install selected utility scripts
	for name, enabled := range utilities {
		if !enabled {
			continue
		}
		script := scripts.GetScript(name)
		if script == "" {
			continue
		}
		if err := installScriptFile(home, name, []byte(script)); err != nil {
			return fmt.Errorf("cannot write %s: %w", name, err)
		}
	}

	return nil
}

// installScriptFile writes one known helper below the trusted HOME descriptor.
// The shared kernel refuses symlinks/non-regular files in every descendant,
// creates missing directories 0700, commits atomically, and sets mode 0700
// before the helper becomes visible.
func installScriptFile(home, name string, content []byte) error {
	if name == "" || name == "." || filepath.Base(name) != name {
		return fmt.Errorf("invalid utility name %q", name)
	}
	rel := filepath.ToSlash(filepath.Join(".local", "bin", name))
	return safefile.ReplaceWithin(home, rel, content, 0o700)
}

func installBinary(execPath, destPath string) error {
	tempFile, err := os.CreateTemp(filepath.Dir(destPath), ".dotfiles-*")
	if err != nil {
		return fmt.Errorf("cannot create temporary binary: %w", err)
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary binary: %w", err)
	}
	if err := copyFile(execPath, tempPath); err != nil {
		return fmt.Errorf("cannot copy binary: %w", err)
	}
	// Owner-only (0700) matches the per-user script policy used for
	// hk/caff/sshh and the bin directory above; this is the final
	// authoritative mode on the binary.
	if err := os.Chmod(tempPath, 0o700); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}
	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("cannot replace binary: %w", err)
	}
	cleanupTemp = false

	return nil
}

// copyFile copies a file from src to dst. The destination is created with
// explicit owner-only permissions (0700) via OpenFile rather than os.Create's
// umask-default 0666, so the file is never momentarily group- or other-readable
// /writable before the caller applies the final authoritative mode.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// cleanupOldInstallations removes legacy binaries from previous installations.
// This handles the transition from separate dotfiles-tui/dotfiles-setup to unified dotfiles.
func cleanupOldInstallations() (removed []string) {
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return nil
	}

	// Locations where old binaries might exist
	locations := []string{
		filepath.Join(home, ".local", "bin", "dotfiles-tui"),
		filepath.Join(home, ".local", "bin", "dotfiles-setup"),
		"/usr/local/bin/dotfiles-tui",
		"/usr/local/bin/dotfiles-setup",
	}

	for _, path := range locations {
		if _, err := os.Stat(path); err == nil {
			// Binary exists, try to remove it
			if err := os.Remove(path); err == nil {
				removed = append(removed, filepath.Base(path))
			}
			// Silently ignore removal errors (permission issues, etc.)
		}
	}

	return removed
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

// streamingInstallToolCmd returns a command that installs a tool with output
// collection. The cancelable ctx is created and its cancel handle (a.streamCancel)
// registered by the caller (handleManageStartInstallMsg) on the main loop so
// teardownStream can stop this (often sudo) subprocess on Ctrl+C / q instead of
// orphaning it (FIX 3). This closure must not touch App state (it runs on a
// bubbletea worker goroutine), so it relies on the context for cancellation.
func (a *App) streamingInstallToolCmd(ctx context.Context, toolID string) tea.Cmd {
	return a.streamingInstallToolCmdWithRuntime(ctx, toolID, defaultToolInstallRuntime())
}

// streamingInstallToolCmdWithRuntime is the dependency-injected Manage install
// command. It shares installTool with the wizard so neither dashboard path can
// accidentally regress to package-metadata-only execution.
func (a *App) streamingInstallToolCmdWithRuntime(ctx context.Context, toolID string, installRuntime toolInstallRuntime) tea.Cmd {
	return func() tea.Msg {
		t, ok := installRuntime.lookupTool(toolID)
		if !ok {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("unknown tool: %s", toolID)}
		}

		mgr := installRuntime.detectManager()
		if installRuntime.isToolInstalled(t) {
			return manageInstallWithLogsMsg{toolID: toolID, logs: []string{fmt.Sprintf("✓ %s is already installed", t.Name())}}
		}

		// Resolve packages via the single source of truth (Pi -> Debian fallback),
		// consistent with the wizard loop and IsInstalled/cache (FIX 2). The empty
		// case is already surfaced (manageInstallWithLogsMsg carries the error), so
		// this path is not silent — but routing through PackagesForPlatform keeps the
		// Pi behavior correct here too.
		platform := installRuntime.detectPlatform()
		if !installerAvailable(t, platform) {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("no supported installer for %s on %s", toolID, platform)}
		}
		if mgr == nil && requiresPackageManager(t) {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("no package manager detected")}
		}

		// Dispatch through Tool.Install. streamingInstallManager preserves live
		// package-manager output/cancellation while allowing custom installers to
		// run their additional steps.
		var logs []string
		err := installTool(ctx, t, mgr, platform, func(line string) {
			logs = appendBoundedInstallLine(logs, line)
		})
		if err == nil && !installRuntime.isToolInstalled(t) {
			err = fmt.Errorf("install postcondition failed: %s is still not detected", toolID)
		}
		return manageInstallWithLogsMsg{toolID: toolID, logs: logs, err: err}
	}
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
	g, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}
	g.Theme = a.theme
	g.NavStyle = a.navStyle
	g.DisableAnimations = !a.animationsEnabled

	// Save synchronously since we're about to start installation
	return config.SaveGlobalConfig(g)
}
