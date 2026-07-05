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

	// Save theme and nav style before installation. A failure here means the
	// user's theme / nav-style / animation choices will not persist across runs.
	// Hand any preferences-save failure to the worker (savePrefsErr); it emits a
	// single non-fatal warning line into the install log without failing the
	// install. Don't also append here — that would double-log the same warning.
	savePrefsErr := a.saveInstallerConfig()

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

	go runInstallWorker(ctx, events, selectedTools, cfg, theme, savePrefsErr)

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
// savePrefsErr, when a non-nil error is supplied, is the failure from saving the
// user's theme/nav-style/animation preferences (captured on the main goroutine
// before the worker started). It is emitted as a NON-FATAL warning line (the
// same treatment as a backup-cleanup error): it is never added to the failures
// slice, so a preferences-record-save failure alone does not flip an otherwise
// successful install to the error screen. It is variadic (optional) so callers
// that do not track a preferences save — e.g. tests exercising the config-apply
// path — can omit it entirely.
func runInstallWorker(ctx context.Context, events chan<- installEventMsg, selectedTools []string, cfg DeepDiveConfig, theme string, savePrefsErr ...error) {
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

	// Surface the pre-install preferences-save failure (if any) as a non-fatal
	// warning line in the install log (same treatment as a backup-cleanup error),
	// so it is visible but does not fail the install. Recorded first because it
	// happened before any phase below.
	if len(savePrefsErr) > 0 && savePrefsErr[0] != nil {
		emitLine(fmt.Sprintf("⚠ Failed to save preferences (theme/nav-style/animations will not persist): %v", savePrefsErr[0]))
	}

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
	if err := installBinary(execPath, destPath); err != nil {
		return err
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
		// Write atomically via temp+rename (installScriptFile) rather than
		// os.WriteFile: WriteFile opens the destination path directly, so a
		// pre-existing ~/.local/bin/{hk,caff,sshh} SYMLINK would be followed and
		// its target overwritten. Renaming a fresh temp file over the path replaces
		// the symlink itself — the same O_NOFOLLOW-safe pattern used for the binary.
		if err := installScriptFile(scriptPath, []byte(script)); err != nil {
			return fmt.Errorf("cannot write %s: %w", name, err)
		}
	}

	return nil
}

// installScriptFile writes script content to destPath atomically, mirroring the
// temp+rename pattern installBinary uses for the main binary. It writes the bytes
// to a fresh temp file in the destination directory, chmods it owner-only (0700,
// matching the per-user executable policy), then renames it over destPath. Because
// the rename replaces the destination NAME (never opening destPath for writing),
// a pre-existing destPath SYMLINK is replaced by the real file instead of being
// followed and having its target overwritten — the O_NOFOLLOW-safe behavior.
func installScriptFile(destPath string, content []byte) error {
	tempFile, err := os.CreateTemp(filepath.Dir(destPath), ".dotfiles-script-*")
	if err != nil {
		return fmt.Errorf("cannot create temporary script: %w", err)
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("cannot write temporary script: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary script: %w", err)
	}
	// Owner-only (0700): private per-user executable, matching installBinary and
	// the bin directory. CreateTemp makes the file 0600, so this is the final mode.
	if err := os.Chmod(tempPath, 0o700); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}
	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("cannot replace script: %w", err)
	}
	cleanupTemp = false

	return nil
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

	// macOS Apps (rectangle, raycast, iina, etc.) - only on macOS
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
		g = config.DefaultGlobalConfig()
	}
	g.Theme = a.theme
	g.NavStyle = a.navStyle
	g.DisableAnimations = !a.animationsEnabled

	// Save synchronously since we're about to start installation
	return config.SaveGlobalConfig(g)
}
