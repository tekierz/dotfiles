package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
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
	a.installOutput = []string{}

	// Save theme and nav style before installation
	a.saveInstallerConfig()

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
	cfg := *a.deepDiveConfig // snapshot for planned-steps computation
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

	// cfg is already a snapshot of deepDiveConfig (taken above for planned-steps
	// computation); theme is snapshotted here. Both are passed to the worker so
	// it never reads App fields after this point (they may be mutated by the
	// Update loop concurrently).
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
			return installDoneMsg{}
		}
		ev, ok := <-ch
		if !ok {
			// Channel closed without a done event; treat as completion.
			return installDoneMsg{}
		}
		return ev
	}
}

// runInstallWorker performs the entire install/configure sequence on a detached
// goroutine, emitting progress as installEventMsg values. It MUST NOT touch any
// App field. It closes the channel when finished.
func runInstallWorker(ctx context.Context, events chan<- installEventMsg, selectedTools []string, cfg DeepDiveConfig, theme string) {
	defer close(events)

	// Sends select on ctx.Done() so a cancelled install (Ctrl+C / teardown)
	// unblocks the worker instead of parking forever on the bounded channel once
	// the consumer (the listen Cmd) stops draining it.
	emit := func(line string) {
		select {
		case events <- installEventMsg{line: line}:
		case <-ctx.Done():
		}
	}
	step := func(line string) {
		select {
		case events <- installEventMsg{line: line, stepInc: true}:
		case <-ctx.Done():
		}
	}

	// output accumulates every line emitted so we can build error context that
	// matches the lines the user has seen, without reading App state.
	var output []string
	emitLine := func(line string) {
		output = append(output, line)
		emit(line)
	}
	stepLine := func(line string) {
		output = append(output, line)
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
		select {
		case events <- installEventMsg{done: true, err: err, context: errCtx}:
		case <-ctx.Done():
		}
	}

	// Auto-backup before making changes (if enabled). The result is honest:
	// it only reports a created backup when at least one file was captured and
	// the manifest persisted, so we never claim a rollback point exists right
	// before overwriting the user's dotfiles (C5).
	backupRes, err := autoBackupIfEnabled()
	if err != nil {
		emitLine(fmt.Sprintf("⚠ Auto-backup failed: %v", err))
	} else if backupRes.enabled {
		if backupRes.count > 0 {
			emitLine(fmt.Sprintf("✓ Auto-backup created before installation (%d file(s))", backupRes.count))
		} else {
			emitLine("⚠ Auto-backup captured 0 files (nothing to roll back)")
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
		// Detect package manager (only needed for the package-install loop).
		mgr := pkg.DetectManager()
		if mgr == nil {
			finish(fmt.Errorf("no package manager detected"))
			return
		}

		platform := pkg.DetectPlatform()
		reg := tools.GetRegistry()

		emitLine(fmt.Sprintf("Installing %d tools using %s...", len(selectedTools), mgr.Name()))

		successCount := 0
		for _, toolID := range selectedTools {
			// Stop promptly if the install was cancelled (Ctrl+C / teardown).
			if ctx.Err() != nil {
				finish(ctx.Err())
				return
			}
			stepLine(fmt.Sprintf("▶ Installing %s...", toolID))

			t, ok := reg.Get(toolID)
			if !ok {
				emitLine(fmt.Sprintf("  ⚠ Unknown tool: %s", toolID))
				continue
			}

			// Skip if already installed
			if t.IsInstalled() {
				emitLine(fmt.Sprintf("  ✓ %s already installed", toolID))
				successCount++
				continue
			}

			// Resolve packages via the single source of truth (which applies the
			// Raspberry Pi -> Debian fallback). Using the raw map lookup here was a
			// silent no-op on Pi, where standard tools define only MacOS/Arch/Debian
			// keys: the loop printed a warning and continued with NO noteFailure, so
			// selecting standard tools on a Pi installed nothing and never surfaced an
			// Error screen (FIX 2 — the RC-C silent-failure class re-introduced).
			pkgs := tools.PackagesForPlatform(t.Packages(), platform)
			if len(pkgs) == 0 {
				// A genuinely-unsupported selected tool must surface as a failure
				// (Error screen), not be silently skipped.
				emitLine(fmt.Sprintf("  ⚠ No packages for %s on this platform", toolID))
				noteFailure(fmt.Errorf("%s: no packages for this platform", toolID))
				continue
			}

			// Install using streaming command, derived from the cancelable worker
			// context so Ctrl+C / teardown stops the subprocess.
			cmd, err := mgr.InstallStreaming(ctx, pkgs...)
			if err != nil {
				emitLine(fmt.Sprintf("  ✗ Failed to start install: %v", err))
				noteFailure(fmt.Errorf("%s: %w", toolID, err))
				continue
			}

			// Collect output
			for line := range cmd.Output {
				emitLine("  " + line)
			}

			if err := cmd.Wait(); err != nil {
				emitLine(fmt.Sprintf("  ✗ Failed to install %s: %v", toolID, err))
				noteFailure(fmt.Errorf("%s: %w", toolID, err))
			} else {
				emitLine(fmt.Sprintf("  ✓ %s installed successfully", toolID))
				successCount++
			}
		}

		if successCount == len(selectedTools) {
			emitLine(fmt.Sprintf("\n✓ All %d tools installed successfully!", successCount))
		} else {
			emitLine(fmt.Sprintf("\n✓ Installed %d/%d tools", successCount, len(selectedTools)))
		}
	}

	// configPhase runs a single configuration step, emitting a header line,
	// advancing the progress step, and recording any failure.
	configPhase := func(header string, run func() error, okLine string) {
		stepLine(header)
		if err := run(); err != nil {
			emitLine(fmt.Sprintf("  ⚠ %v", err))
			noteFailure(err)
		} else if okLine != "" {
			emitLine(okLine)
		}
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
	stepLine("\n▶ Configuring tmux...")
	if err := tools.SetupTPM(tmuxCfg, theme); err != nil {
		emitLine(fmt.Sprintf("  ⚠ Failed to configure tmux: %v", err))
		noteFailure(fmt.Errorf("Failed to configure tmux: %w", err))
	} else {
		emitLine("  ✓ Tmux configured with ~/.tmux.conf")
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
		stepLine("\n▶ Configuring Claude Code MCP servers...")
		claudeTool := tools.NewClaudeCodeTool()
		// Use user's MCP selections from deep dive config
		if err := claudeTool.ApplyConfigWithMCPs(cfg.ClaudeCodeMCPs); err != nil {
			emitLine(fmt.Sprintf("  ⚠ Failed to configure Claude MCP: %v", err))
			noteFailure(fmt.Errorf("Failed to configure Claude MCP: %w", err))
		} else {
			// Count enabled MCPs for status message
			enabledCount := 0
			for _, enabled := range cfg.ClaudeCodeMCPs {
				if enabled {
					enabledCount++
				}
			}
			emitLine(fmt.Sprintf("  ✓ Claude Code configured with %d MCP server(s)", enabledCount))
		}
	}

	// Configure Ghostty
	configPhase("\n▶ Configuring Ghostty...", func() error {
		if err := tools.WriteGhosttyConfig(ghosttyConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure Ghostty: %w", err)
		}
		return nil
	}, "  ✓ Ghostty configured")

	// Configure Zsh
	configPhase("\n▶ Configuring Zsh...", func() error {
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
	configPhase("\n▶ Configuring Neovim...", func() error {
		if err := tools.WriteNeovimConfig(neovimCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Neovim: %w", err)
		}
		return nil
	}, neovimSuccessMsg)

	// Configure Git
	configPhase("\n▶ Configuring Git...", func() error {
		if err := tools.WriteGitConfig(gitConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure Git: %w", err)
		}
		return nil
	}, "  ✓ Git configured with ~/.gitconfig")

	// Configure Yazi
	configPhase("\n▶ Configuring Yazi...", func() error {
		if err := tools.WriteYaziConfig(yaziConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure Yazi: %w", err)
		}
		return nil
	}, "  ✓ Yazi configured")

	// Configure FZF
	configPhase("\n▶ Configuring FZF...", func() error {
		if err := tools.WriteFzfConfig(fzfConfigFrom(cfg), theme); err != nil {
			return fmt.Errorf("Failed to configure FZF: %w", err)
		}
		return nil
	}, "  ✓ FZF configured")

	// Configure LazyGit — only when the user selected it in the deep-dive.
	// lazygit is in CLITools (UIGroupCLITools) and therefore has an explicit
	// selection flag; skipping its config when deselected matches user intent.
	if cfg.CLITools["lazygit"] {
		configPhase("\n▶ Configuring LazyGit...", func() error {
			if err := tools.WriteLazyGitConfig(lazygitConfigFrom(cfg), theme); err != nil {
				return fmt.Errorf("Failed to configure LazyGit: %w", err)
			}
			return nil
		}, "  ✓ LazyGit configured")
	}

	// Configure Btop — only when the user selected it in the deep-dive.
	// btop is in CLITools (UIGroupCLITools) and has an explicit selection flag.
	if cfg.CLITools["btop"] {
		configPhase("\n▶ Configuring Btop...", func() error {
			if err := tools.WriteBtopConfig(btopConfigFrom(cfg), theme); err != nil {
				return fmt.Errorf("Failed to configure Btop: %w", err)
			}
			return nil
		}, "  ✓ Btop configured")
	}

	// Configure Glow — only when the user selected it in the deep-dive.
	// glow is in CLITools (UIGroupCLITools) and has an explicit selection flag.
	if cfg.CLITools["glow"] {
		configPhase("\n▶ Configuring Glow...", func() error {
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

// installUtilities copies the dotfiles binary and shell utilities to ~/.local/bin
func installUtilities(utilities map[string]bool) error {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
	}

	binDir := filepath.Join(home, ".local", "bin")

	// Create ~/.local/bin if it doesn't exist. Owner-only (0700) matches the
	// project's per-user permission policy (config dirs 700) and avoids creating
	// a world-readable bin directory.
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w", binDir, err)
	}

	// Clean up legacy binaries from previous installations
	cleanupOldInstallations()

	// Get the path to the currently running executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot get executable path: %w", err)
	}

	// Resolve any symlinks to get the real path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	// Copy the binary to ~/.local/bin/dotfiles
	destPath := filepath.Join(binDir, "dotfiles")
	// Remove existing binary first to avoid "text file busy" error
	// (Linux allows deleting a running binary, but not overwriting it)
	_ = os.Remove(destPath)
	if err := copyFile(execPath, destPath); err != nil {
		return fmt.Errorf("cannot copy binary: %w", err)
	}

	// Make it executable. Owner-only (0700) matches the per-user script policy
	// used for hk/caff/sshh and the bin directory above; this is the final
	// authoritative mode on the binary.
	if err := os.Chmod(destPath, 0o700); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
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
		scriptPath := filepath.Join(binDir, name)
		// Private per-user executables: owner-only (rwx) per the project's
		// documented permission policy (config dirs 700, settings 600).
		if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
			return fmt.Errorf("cannot write %s: %w", name, err)
		}
	}

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

// collectSelectedTools gathers all tool IDs selected in deep dive config
func (a *App) collectSelectedTools() []string {
	// Ensure we have install status cached
	a.ensureInstallCache()

	var selected []string

	// CLI Tools (lazygit, lazydocker, btop, glow, claude-code)
	for id, enabled := range a.deepDiveConfig.CLITools {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
		}
	}

	// GUI Apps (zen-browser, cursor, lm-studio, obs)
	for id, enabled := range a.deepDiveConfig.GUIApps {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
		}
	}

	// CLI Utilities (bat, eza, zoxide, ripgrep, fd, delta, fswatch)
	for id, enabled := range a.deepDiveConfig.CLIUtilities {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
		}
	}

	// Note: Utilities (hk, caff, sshh) are shell scripts handled by installUtilities()
	// They don't go through the package manager

	// macOS Apps (rectangle, raycast, stats, etc.) - only on macOS
	if pkg.DetectPlatform() == pkg.PlatformMacOS {
		for id, enabled := range a.deepDiveConfig.MacApps {
			if enabled && !a.manageInstalled[id] {
				selected = append(selected, id)
			}
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
	return func() tea.Msg {
		reg := tools.GetRegistry()
		t, ok := reg.Get(toolID)
		if !ok {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("unknown tool: %s", toolID)}
		}

		mgr := pkg.DetectManager()
		if mgr == nil {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("no package manager detected")}
		}

		// Resolve packages via the single source of truth (Pi -> Debian fallback),
		// consistent with the wizard loop and IsInstalled/cache (FIX 2). The empty
		// case is already surfaced (manageInstallWithLogsMsg carries the error), so
		// this path is not silent — but routing through PackagesForPlatform keeps the
		// Pi behavior correct here too.
		platform := pkg.DetectPlatform()
		pkgs := tools.PackagesForPlatform(t.Packages(), platform)
		if len(pkgs) == 0 {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("no packages defined for %s", toolID)}
		}

		// Start streaming install on the caller-provided cancelable context so
		// teardownStream stops the subprocess on quit (FIX 3). The cancel handle was
		// already registered on the main loop (handleManageStartInstallMsg); we do
		// NOT write a.streamCmd from this worker-goroutine closure, since that would
		// race teardownStream's main-loop read. Cancelling the context is sufficient:
		// InstallStreaming runs via exec.CommandContext, so a.streamCancel() kills the
		// subprocess.
		cmd, err := mgr.InstallStreaming(ctx, pkgs...)
		if err != nil {
			return manageInstallWithLogsMsg{toolID: toolID, err: err}
		}

		// Collect all output
		var logs []string
		for line := range cmd.Output {
			logs = append(logs, line)
		}

		// Wait for completion
		err = cmd.Wait()
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

// streamingUpdateCmd starts a streaming update of the given packages. The
// streaming command's output is drained on a detached worker goroutine that
// writes each line to a.updateStream (so lines render LIVE) and emits a final
// `done` event with the results/error before closing the channel. The worker
// MUST NOT touch any App field; all App mutation happens in Update on the main
// loop. The returned Cmd listens for the first event (re-armed from Update).
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

	// Cancelable context (derived from a cancelable parent stored on App) so
	// navigate-away / Ctrl+C / teardownStream() stops the subprocess and unblocks
	// the worker's bounded-channel sends instead of leaking them.
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := mgr.UpdateStreaming(ctx, pkgNames...)
	if err != nil {
		cancel()
		return func() tea.Msg { return updateStreamMsg{done: true, err: err} }
	}
	if cmd == nil {
		cancel()
		return func() tea.Msg { return updateStreamMsg{done: true} }
	}
	// Retained on App (set here on the main loop) so teardownStream() can cancel.
	a.streamCmd = cmd
	a.streamCancel = cancel

	// Buffered so the worker can make progress without blocking on a slow
	// consumer; the listen Cmd drains it one event at a time.
	stream := make(chan updateStreamMsg, 64)
	a.updateStream = stream

	go func() {
		defer close(stream)
		for line := range cmd.Output {
			select {
			case stream <- updateStreamMsg{line: line}:
			case <-ctx.Done():
				return
			}
		}
		err := cmd.Wait()
		results := make([]pkg.UpdateResult, 0, len(packages))
		for _, p := range packages {
			results = append(results, pkg.UpdateResult{
				Package: p,
				Success: err == nil,
				Error:   err,
			})
		}
		select {
		case stream <- updateStreamMsg{done: true, results: results, err: err}:
		case <-ctx.Done():
		}
	}()

	return a.listenUpdateStreamCmd()
}

// streamingUpdateAllCmd starts a streaming update of all packages, using the
// same live-streaming worker pattern as streamingUpdateCmd.
func (a *App) streamingUpdateAllCmd() tea.Cmd {
	mgr := pkg.DetectManager()
	if mgr == nil {
		return func() tea.Msg {
			return updateStreamMsg{done: true, err: fmt.Errorf("no package manager detected")}
		}
	}

	// Cancelable context (see streamingUpdateCmd) so teardown stops the subprocess
	// and unblocks the worker's bounded-channel sends.
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := mgr.UpdateAllStreaming(ctx)
	if err != nil {
		cancel()
		return func() tea.Msg { return updateStreamMsg{done: true, err: err} }
	}
	if cmd == nil {
		cancel()
		return func() tea.Msg { return updateStreamMsg{done: true} }
	}
	a.streamCmd = cmd
	a.streamCancel = cancel

	stream := make(chan updateStreamMsg, 64)
	a.updateStream = stream

	go func() {
		defer close(stream)
		for line := range cmd.Output {
			select {
			case stream <- updateStreamMsg{line: line}:
			case <-ctx.Done():
				return
			}
		}
		err := cmd.Wait()
		select {
		case stream <- updateStreamMsg{done: true, err: err}:
		case <-ctx.Done():
		}
	}()

	return a.listenUpdateStreamCmd()
}

// saveInstallerConfig saves theme and nav style during installer flow
func (a *App) saveInstallerConfig() {
	g, err := config.LoadGlobalConfig()
	if err != nil {
		g = config.DefaultGlobalConfig()
	}
	g.Theme = a.theme
	g.NavStyle = a.navStyle
	g.DisableAnimations = !a.animationsEnabled

	// Save synchronously since we're about to start installation
	_ = config.SaveGlobalConfig(g)
}
