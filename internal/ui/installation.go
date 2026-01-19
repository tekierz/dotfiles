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

// startInstallation begins the installation process using the Go-based package manager
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

	return func() tea.Msg {
		if len(selectedTools) == 0 {
			a.installOutput = append(a.installOutput, "No tools selected for installation")
			return installDoneMsg{err: nil}
		}

		// Auto-backup before making changes (if enabled)
		if err := autoBackupIfEnabled(); err != nil {
			a.installOutput = append(a.installOutput, fmt.Sprintf("⚠ Auto-backup failed: %v", err))
		} else {
			globalCfg, _ := config.LoadGlobalConfig()
			if globalCfg != nil && globalCfg.AutoBackup {
				a.installOutput = append(a.installOutput, "✓ Auto-backup created before installation")
			}
		}

		// Detect package manager
		mgr := pkg.DetectManager()
		if mgr == nil {
			return installDoneMsg{err: fmt.Errorf("no package manager detected")}
		}

		platform := pkg.DetectPlatform()
		reg := tools.GetRegistry()

		a.installOutput = append(a.installOutput, fmt.Sprintf("Installing %d tools using %s...", len(selectedTools), mgr.Name()))

		var lastErr error
		successCount := 0
		for _, toolID := range selectedTools {
			a.installStep++
			a.installOutput = append(a.installOutput, fmt.Sprintf("▶ Installing %s...", toolID))

			t, ok := reg.Get(toolID)
			if !ok {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Unknown tool: %s", toolID))
				continue
			}

			// Skip if already installed
			if t.IsInstalled() {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✓ %s already installed", toolID))
				successCount++
				continue
			}

			// Get packages for this platform
			pkgs := t.Packages()[platform]
			if len(pkgs) == 0 {
				pkgs = t.Packages()["all"]
			}
			if len(pkgs) == 0 {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ No packages for %s on this platform", toolID))
				continue
			}

			// Install using streaming command
			ctx := context.Background()
			cmd, err := mgr.InstallStreaming(ctx, pkgs...)
			if err != nil {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✗ Failed to start install: %v", err))
				lastErr = err
				continue
			}

			// Collect output
			for line := range cmd.Output {
				a.installOutput = append(a.installOutput, "  "+line)
				// Keep last 20 lines for display (using copy to avoid memory leak)
				const maxOutputLines = 20
				if len(a.installOutput) > maxOutputLines {
					copy(a.installOutput, a.installOutput[len(a.installOutput)-maxOutputLines:])
					a.installOutput = a.installOutput[:maxOutputLines]
				}
			}

			if err := cmd.Wait(); err != nil {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✗ Failed to install %s: %v", toolID, err))
				lastErr = err
			} else {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✓ %s installed successfully", toolID))
				successCount++
			}
		}

		if successCount == len(selectedTools) {
			a.installOutput = append(a.installOutput, fmt.Sprintf("\n✓ All %d tools installed successfully!", successCount))
		} else {
			a.installOutput = append(a.installOutput, fmt.Sprintf("\n✓ Installed %d/%d tools", successCount, len(selectedTools)))
		}

		// Install dotfiles binary and utilities to ~/.local/bin
		a.installStep++
		a.installOutput = append(a.installOutput, "\n▶ Installing dotfiles utilities...")
		if err := installUtilities(a.deepDiveConfig.Utilities); err != nil {
			a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Failed to install utilities: %v", err))
			lastErr = err
		} else {
			a.installOutput = append(a.installOutput, "  ✓ Utilities installed to ~/.local/bin")
		}

		// Configure tmux with TPM plugins
		a.installStep++
		a.installOutput = append(a.installOutput, "\n▶ Configuring tmux...")
		tmuxCfg := tools.TmuxConfig{
			Prefix:           a.deepDiveConfig.TmuxPrefix,
			SplitBinds:       a.deepDiveConfig.TmuxSplitBinds,
			StatusBar:        a.deepDiveConfig.TmuxStatusBar,
			MouseMode:        a.deepDiveConfig.TmuxMouseMode,
			TPMEnabled:       a.deepDiveConfig.TmuxTPMEnabled,
			PluginSensible:   a.deepDiveConfig.TmuxPluginSensible,
			PluginResurrect:  a.deepDiveConfig.TmuxPluginResurrect,
			PluginContinuum:  a.deepDiveConfig.TmuxPluginContinuum,
			PluginYank:       a.deepDiveConfig.TmuxPluginYank,
			ContinuumSaveMin: a.deepDiveConfig.TmuxContinuumSaveMin,
		}
		if err := tools.SetupTPM(tmuxCfg, a.theme); err != nil {
			a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Failed to configure tmux: %v", err))
			lastErr = err
		} else {
			a.installOutput = append(a.installOutput, "  ✓ Tmux configured with ~/.tmux.conf")
			if tmuxCfg.TPMEnabled {
				if tools.IsTPMInstalled() {
					a.installOutput = append(a.installOutput, "  ✓ TPM plugins ready (run prefix+I in tmux to install)")
				} else {
					a.installOutput = append(a.installOutput, "  ⚠ TPM installed but plugins pending")
				}
			}
		}

		// Apply Claude Code MCP configuration if claude-code was selected
		if a.deepDiveConfig.CLITools["claude-code"] || a.deepDiveConfig.Utilities["claude-code"] {
			a.installStep++
			a.installOutput = append(a.installOutput, "\n▶ Configuring Claude Code MCP servers...")
			claudeTool := tools.NewClaudeCodeTool()
			// Use user's MCP selections from deep dive config
			if err := claudeTool.ApplyConfigWithMCPs(a.deepDiveConfig.ClaudeCodeMCPs); err != nil {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Failed to configure Claude MCP: %v", err))
				lastErr = err
			} else {
				// Count enabled MCPs for status message
				enabledCount := 0
				for _, enabled := range a.deepDiveConfig.ClaudeCodeMCPs {
					if enabled {
						enabledCount++
					}
				}
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✓ Claude Code configured with %d MCP server(s)", enabledCount))
			}
		}

		// Build context from last few output lines for error display
		var context string
		if lastErr != nil && len(a.installOutput) > 0 {
			start := 0
			if len(a.installOutput) > 8 {
				start = len(a.installOutput) - 8
			}
			context = strings.Join(a.installOutput[start:], "\n")
		}

		return installDoneMsg{err: lastErr, context: context}
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
		if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
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

	// macOS Apps (rectangle, raycast, stats, etc.)
	for id, enabled := range a.deepDiveConfig.MacApps {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
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
