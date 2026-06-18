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
	a.installStep = 0
	a.installOutput = []string{}

	// Save theme and nav style before installation
	a.saveInstallerConfig()

	// Collect all selected tools from deep dive config
	selectedTools := a.collectSelectedTools()

	// Buffered channel so the worker can make progress without blocking on a
	// slow consumer; the listen Cmd drains it one event at a time.
	events := make(chan installEventMsg, 64)
	a.installEvents = events

	// Snapshot the values the worker needs so it never reads App fields after
	// this point (they may be mutated by the Update loop concurrently). The
	// deep-dive config is copied by value; the install progress screen does not
	// allow editing it, so the shared maps inside are effectively immutable here.
	cfg := *a.deepDiveConfig
	theme := a.theme

	go runInstallWorker(events, selectedTools, cfg, theme)

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
func runInstallWorker(events chan<- installEventMsg, selectedTools []string, cfg DeepDiveConfig, theme string) {
	defer close(events)

	emit := func(line string) { events <- installEventMsg{line: line} }
	step := func(line string) { events <- installEventMsg{line: line, stepInc: true} }

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
		var context string
		if err != nil && len(output) > 0 {
			start := 0
			if len(output) > 8 {
				start = len(output) - 8
			}
			context = strings.Join(output[start:], "\n")
		}
		events <- installEventMsg{done: true, err: err, context: context}
	}

	if len(selectedTools) == 0 {
		emitLine("No tools selected for installation")
		finish(nil)
		return
	}

	// Auto-backup before making changes (if enabled)
	if err := autoBackupIfEnabled(); err != nil {
		emitLine(fmt.Sprintf("⚠ Auto-backup failed: %v", err))
	} else {
		globalCfg, _ := config.LoadGlobalConfig()
		if globalCfg != nil && globalCfg.AutoBackup {
			emitLine("✓ Auto-backup created before installation")
		}
	}

	// Detect package manager
	mgr := pkg.DetectManager()
	if mgr == nil {
		finish(fmt.Errorf("no package manager detected"))
		return
	}

	platform := pkg.DetectPlatform()
	reg := tools.GetRegistry()

	emitLine(fmt.Sprintf("Installing %d tools using %s...", len(selectedTools), mgr.Name()))

	// failures aggregates every failed step so the final error reports how many
	// phases failed rather than silently overwriting a single lastErr.
	var failures []error
	noteFailure := func(err error) { failures = append(failures, err) }

	successCount := 0
	for _, toolID := range selectedTools {
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

		// Get packages for this platform
		pkgs := t.Packages()[platform]
		if len(pkgs) == 0 {
			pkgs = t.Packages()["all"]
		}
		if len(pkgs) == 0 {
			emitLine(fmt.Sprintf("  ⚠ No packages for %s on this platform", toolID))
			continue
		}

		// Install using streaming command
		ctx := context.Background()
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

	// Configure tmux with TPM plugins
	tmuxCfg := tools.TmuxConfig{
		Prefix:           cfg.TmuxPrefix,
		SplitBinds:       cfg.TmuxSplitBinds,
		StatusBar:        cfg.TmuxStatusBar,
		MouseMode:        cfg.TmuxMouseMode,
		TPMEnabled:       cfg.TmuxTPMEnabled,
		PluginSensible:   cfg.TmuxPluginSensible,
		PluginResurrect:  cfg.TmuxPluginResurrect,
		PluginContinuum:  cfg.TmuxPluginContinuum,
		PluginYank:       cfg.TmuxPluginYank,
		ContinuumSaveMin: cfg.TmuxContinuumSaveMin,
	}
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
		ghosttyCfg := tools.GhosttyConfig{
			FontSize:        cfg.GhosttyFontSize,
			FontFamily:      cfg.GhosttyFontFamily,
			Opacity:         cfg.GhosttyOpacity,
			BlurRadius:      cfg.GhosttyBlurRadius,
			TabBindings:     cfg.GhosttyTabBindings,
			ScrollbackLines: cfg.GhosttyScrollbackLines,
			CursorStyle:     cfg.GhosttyCursorStyle,
		}
		if err := tools.WriteGhosttyConfig(ghosttyCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Ghostty: %w", err)
		}
		return nil
	}, "  ✓ Ghostty configured")

	// Configure Zsh
	configPhase("\n▶ Configuring Zsh...", func() error {
		zshCfg := tools.ZshConfig{
			PromptStyle:     cfg.ZshPromptStyle,
			Plugins:         cfg.ZshPlugins,
			Aliases:         cfg.ZshAliases,
			HistorySize:     cfg.ZshHistorySize,
			AutoCD:          cfg.ZshAutoCD,
			SyntaxHighlight: cfg.ZshSyntaxHighlight,
			Autosuggestions: cfg.ZshAutosuggestions,
		}
		if err := tools.WriteZshConfig(zshCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Zsh: %w", err)
		}
		return nil
	}, "  ✓ Zsh configured with ~/.zshrc")

	// Configure Neovim
	neovimCfg := tools.NeovimConfig{
		ConfigPreset: cfg.NeovimConfig,
		LSPs:         cfg.NeovimLSPs,
		Plugins:      cfg.NeovimPlugins,
		TabWidth:     cfg.NeovimTabWidth,
		Wrap:         cfg.NeovimWrap,
		CursorLine:   cfg.NeovimCursorLine,
		Clipboard:    cfg.NeovimClipboard,
	}
	configPhase("\n▶ Configuring Neovim...", func() error {
		if err := tools.WriteNeovimConfig(neovimCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Neovim: %w", err)
		}
		return nil
	}, fmt.Sprintf("  ✓ Neovim configured (%s)", neovimCfg.ConfigPreset))

	// Configure Git
	configPhase("\n▶ Configuring Git...", func() error {
		gitCfg := tools.GitConfig{
			DeltaSideBySide:  cfg.GitDeltaSideBySide,
			DefaultBranch:    cfg.GitDefaultBranch,
			Aliases:          cfg.GitAliases,
			PullRebase:       cfg.GitPullRebase,
			SignCommits:      cfg.GitSignCommits,
			CredentialHelper: cfg.GitCredentialHelper,
		}
		if err := tools.WriteGitConfig(gitCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Git: %w", err)
		}
		return nil
	}, "  ✓ Git configured with ~/.gitconfig")

	// Configure Yazi
	configPhase("\n▶ Configuring Yazi...", func() error {
		yaziCfg := tools.YaziConfig{
			Keymap:      cfg.YaziKeymap,
			ShowHidden:  cfg.YaziShowHidden,
			PreviewMode: cfg.YaziPreviewMode,
		}
		if err := tools.WriteYaziConfig(yaziCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Yazi: %w", err)
		}
		return nil
	}, "  ✓ Yazi configured")

	// Configure FZF
	configPhase("\n▶ Configuring FZF...", func() error {
		fzfCfg := tools.FzfConfig{
			Preview: cfg.FzfPreview,
			Height:  cfg.FzfHeight,
			Layout:  cfg.FzfLayout,
		}
		if err := tools.WriteFzfConfig(fzfCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure FZF: %w", err)
		}
		return nil
	}, "  ✓ FZF configured")

	// Configure LazyGit
	configPhase("\n▶ Configuring LazyGit...", func() error {
		lazygitCfg := tools.LazyGitConfig{
			SideBySide: cfg.LazyGitSideBySide,
			MouseMode:  cfg.LazyGitMouseMode,
			Theme:      cfg.LazyGitTheme,
		}
		if err := tools.WriteLazyGitConfig(lazygitCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure LazyGit: %w", err)
		}
		return nil
	}, "  ✓ LazyGit configured")

	// Configure Btop
	configPhase("\n▶ Configuring Btop...", func() error {
		btopCfg := tools.BtopConfig{
			Theme:     cfg.BtopTheme,
			UpdateMs:  cfg.BtopUpdateMs,
			ShowTemp:  cfg.BtopShowTemp,
			GraphType: cfg.BtopGraphType,
		}
		if err := tools.WriteBtopConfig(btopCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Btop: %w", err)
		}
		return nil
	}, "  ✓ Btop configured")

	// Configure Glow
	configPhase("\n▶ Configuring Glow...", func() error {
		glowCfg := tools.GlowConfig{
			Pager: cfg.GlowPager,
			Style: cfg.GlowStyle,
			Width: cfg.GlowWidth,
		}
		if err := tools.WriteGlowConfig(glowCfg, theme); err != nil {
			return fmt.Errorf("Failed to configure Glow: %w", err)
		}
		return nil
	}, "  ✓ Glow configured")

	// Surface all failures: report the count and the first failing step so the
	// Error screen makes clear that one or more phases failed (not just the last).
	var finalErr error
	if len(failures) == 1 {
		finalErr = failures[0]
	} else if len(failures) > 1 {
		finalErr = fmt.Errorf("%d steps failed; first: %w", len(failures), failures[0])
	}
	finish(finalErr)
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

	// Create ~/.local/bin if it doesn't exist
	if err := os.MkdirAll(binDir, 0755); err != nil {
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

	// Make it executable
	if err := os.Chmod(destPath, 0755); err != nil {
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

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
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

// streamingInstallToolCmd returns a command that installs a tool with output collection
func (a *App) streamingInstallToolCmd(toolID string) tea.Cmd {
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

		// Get packages for this platform
		platform := pkg.DetectPlatform()
		pkgs := t.Packages()[platform]
		if len(pkgs) == 0 {
			pkgs = t.Packages()["all"]
		}
		if len(pkgs) == 0 {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("no packages defined for %s", toolID)}
		}

		// Start streaming install
		ctx := context.Background()
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

// streamingUpdateCmd returns a command that updates packages with output collection
func (a *App) streamingUpdateCmd(packages []pkg.Package) tea.Cmd {
	return func() tea.Msg {
		mgr := pkg.DetectManager()
		if mgr == nil {
			return updateWithLogsMsg{err: fmt.Errorf("no package manager detected")}
		}

		var pkgNames []string
		for _, p := range packages {
			pkgNames = append(pkgNames, p.Name)
		}

		ctx := context.Background()
		cmd, err := mgr.UpdateStreaming(ctx, pkgNames...)
		if err != nil {
			return updateWithLogsMsg{err: err}
		}

		// Collect all output
		var logs []string
		for line := range cmd.Output {
			logs = append(logs, line)
		}

		err = cmd.Wait()

		// Build results
		var results []pkg.UpdateResult
		for _, p := range packages {
			results = append(results, pkg.UpdateResult{
				Package: p,
				Success: err == nil,
				Error:   err,
			})
		}
		return updateWithLogsMsg{logs: logs, results: results, err: err}
	}
}

// streamingUpdateAllCmd returns a command that updates all packages with output collection
func (a *App) streamingUpdateAllCmd() tea.Cmd {
	return func() tea.Msg {
		mgr := pkg.DetectManager()
		if mgr == nil {
			return updateWithLogsMsg{err: fmt.Errorf("no package manager detected")}
		}

		ctx := context.Background()
		cmd, err := mgr.UpdateAllStreaming(ctx)
		if err != nil {
			return updateWithLogsMsg{err: err}
		}

		// Collect all output
		var logs []string
		for line := range cmd.Output {
			logs = append(logs, line)
		}

		err = cmd.Wait()
		return updateWithLogsMsg{logs: logs, err: err}
	}
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
