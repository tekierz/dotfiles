package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/tools"
)

// This file centralizes the handling of the streaming/terminal async messages
// for the install (Progress) and tool-install (Manage) and package-update
// (Update) flows. These messages re-arm a listen Cmd, finalize a run by
// resetting the "running" flags, and refresh the install-status cache. That
// chain MUST survive screen navigation: the worker goroutine + (often sudo)
// package-manager subprocess outlive the originating screen, so if the user
// navigates away (especially via the mouse tab-bar) the terminal message would
// otherwise be delivered to the now-active screen and dropped — permanently
// wedging the running flag, dropping the result, and leaking the worker +
// subprocess (cluster A: findings C0, C1, C9).
//
// The fix mirrors the existing global-handling pattern for installCacheDoneMsg:
// App.Update processes these messages globally BEFORE delegating to the active
// ScreenHandler. The screen handlers delegate to the same *App methods, so the
// behavior is identical whether or not the originating screen is still active.

// teardownStream cancels the active install/update worker context and the
// underlying StreamingCmd, then clears the retained handles. It is idempotent
// and safe to call when nothing is streaming. Cancelling the context unblocks
// any worker goroutine parked on a bounded channel send (the worker selects on
// ctx.Done()) and closes the StreamingCmd's subprocess, so the worker's range
// loop ends cleanly instead of leaking.
func (a *App) teardownStream() {
	if a.streamCmd != nil {
		a.streamCmd.Cancel()
		a.streamCmd = nil
	}
	if a.streamCancel != nil {
		a.streamCancel()
		a.streamCancel = nil
	}
	// Stop the sudo keep-alive goroutine (C16) on cancel/teardown so it never
	// outlives the install. The stop func blocks until the goroutine exits and is
	// a no-op when none is running (or on non-Linux).
	if a.sudoKeepAliveStop != nil {
		a.sudoKeepAliveStop()
		a.sudoKeepAliveStop = nil
	}
}

// --- Update (package update) streaming -------------------------------------

// handleUpdateStartMsg starts a streaming update once sudo is cached. Shared by
// the Update screen and the global App.Update dispatch.
func (a *App) handleUpdateStartMsg(msg updateStartMsg) tea.Cmd {
	a.clearInstallLogs()
	a.updateRunning = true
	a.installLogAutoScroll = true // Follow output live while the update runs.
	if msg.all {
		return a.streamingUpdateAllCmd()
	}
	return a.streamingUpdateCmd(msg.packages)
}

// handleUpdateSudoRequiredMsg prompts for sudo, then continues into the
// streaming update. The continuation yields updateStartMsg (handled next).
func (a *App) handleUpdateSudoRequiredMsg(msg updateSudoRequiredMsg) tea.Cmd {
	return tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
		if err != nil {
			return updateRunDoneMsg{err: err}
		}
		return updateStartMsg{packages: msg.packages, all: msg.all}
	})
}

// handleUpdateStreamMsg applies one streamed update event on the main loop. A
// live line is appended; a done event finalizes the run; otherwise the listen
// Cmd is re-armed so the stream keeps flowing regardless of the active screen.
func (a *App) handleUpdateStreamMsg(msg updateStreamMsg) tea.Cmd {
	if msg.line != "" {
		a.appendInstallLog(msg.line)
	}
	if msg.done {
		a.updateStream = nil
		a.teardownStream()
		return a.finishUpdate(msg.results, msg.err)
	}
	return a.listenUpdateStreamCmd()
}

// finishUpdate finalizes an update run: it stops the run, frees the log scroll,
// sets the status line from the results, and (on success) refreshes the package
// list via checkUpdatesCmd. Moved off the Update screen so the finalize chain
// survives navigation.
func (a *App) finishUpdate(results []pkg.UpdateResult, err error) tea.Cmd {
	a.updateRunning = false
	a.installLogAutoScroll = false // Allow user to scroll through logs.
	if err != nil {
		a.updateStatus = fmt.Sprintf("Update failed: %v", err)
		return nil
	}
	successes := 0
	failures := 0
	for _, r := range results {
		if r.Success {
			successes++
		} else {
			failures++
		}
	}
	if failures > 0 {
		a.updateStatus = fmt.Sprintf("Updated %d, failed %d", successes, failures)
	} else if successes > 0 {
		a.updateStatus = fmt.Sprintf("Updated %d package(s) ✓", successes)
	} else {
		a.updateStatus = "Update complete ✓"
	}
	// A successful update changes installed versions (and possibly install
	// status), so the install-status caches are now stale. Invalidate both the
	// registry's IsInstalled() cache and the App's manageInstalled cache so
	// status/version displays refresh on the next navigation (mirrors the
	// installer-success invalidation in screen_progress.go).
	tools.GetRegistry().InvalidateCache()
	a.manageInstalledReady = false

	// Clear selections and refresh the package list.
	a.updateSelected = make(map[int]bool)
	a.updateCheckDone = false
	a.updateChecking = true
	return checkUpdatesCmd()
}

// --- Manage (single-tool install) streaming --------------------------------

// handleManageSudoRequiredMsg prompts for sudo, then continues into the
// streaming install. The continuation yields manageStartInstallMsg.
func (a *App) handleManageSudoRequiredMsg(msg manageSudoRequiredMsg) tea.Cmd {
	return tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
		if err != nil {
			return manageInstallDoneMsg{toolID: msg.toolID, err: err}
		}
		return manageStartInstallMsg{toolID: msg.toolID}
	})
}

// handleManageStartInstallMsg starts the streaming install (sudo already cached).
//
// The cancelable context and its cancel handle are created HERE, on the main
// loop, before spawning the install command (FIX 3). This is the only place
// that's safe to set a.streamCancel without a data race: the worker goroutine
// the Cmd spawns must not touch App fields. Registering a.streamCancel lets
// teardownStream() cancel the context on Ctrl+C / q, which (via
// exec.CommandContext) kills the orphaned `sudo apt/pacman install ...`
// subprocess instead of leaking it to init. The Linux sudo keep-alive is started
// here too, mirroring the wizard/update paths, and torn down on completion.
func (a *App) handleManageStartInstallMsg(msg manageStartInstallMsg) tea.Cmd {
	a.clearInstallLogs()
	a.manageInstalling = true
	a.manageInstallID = msg.toolID

	ctx, cancel := context.WithCancel(context.Background())
	a.streamCancel = cancel
	// Keep the (often-expiring) sudo timestamp fresh during a long install, the
	// same as the wizard/update paths. No-op on macOS / when sudo isn't cached;
	// stopped by teardownStream on completion or cancel (C16).
	if runner.NeedsSudo() && runner.CheckSudoCached() {
		a.sudoKeepAliveStop = startSudoKeepAlive(refreshSudo)
	}

	return a.streamingInstallToolCmd(ctx, msg.toolID)
}

// handleManageInstallWithLogsMsg finalizes a manage install that carried its
// collected logs. On success it invalidates and reloads the install-status
// cache so the Manage screen reflects the new install. Survives navigation.
func (a *App) handleManageInstallWithLogsMsg(msg manageInstallWithLogsMsg) tea.Cmd {
	a.manageInstalling = false
	a.manageInstallID = ""
	a.installLogAutoScroll = false
	a.teardownStream()
	for _, line := range msg.logs {
		a.appendInstallLog(line)
	}
	if msg.err != nil {
		a.manageStatus = fmt.Sprintf("Install failed: %v", msg.err)
		tools.GetRegistry().InvalidateCache()
		a.manageInstalledReady = false
		return a.startInstallCacheLoad()
	}
	a.manageStatus = "Installed successfully ✓"
	// Refresh the install-status cache, then reload it so the Manage screen
	// reflects the newly installed tool immediately. The registry's own cache is
	// invalidated too so IsInstalled() re-checks (Phase B + C10 fix).
	tools.GetRegistry().InvalidateCache()
	a.manageInstalledReady = false
	return a.startInstallCacheLoad()
}

// handleManageInstallDoneMsg finalizes the non-streaming manage install path
// (retained for compatibility). Survives navigation.
func (a *App) handleManageInstallDoneMsg(msg manageInstallDoneMsg) tea.Cmd {
	a.manageInstalling = false
	a.manageInstallID = ""
	a.teardownStream()
	if msg.err != nil {
		a.manageStatus = fmt.Sprintf("Install failed: %v", msg.err)
		a.manageInstalledReady = false
		return a.startInstallCacheLoad()
	}
	a.manageStatus = "Installed ✓"
	tools.GetRegistry().InvalidateCache()
	a.manageInstalledReady = false
	return a.startInstallCacheLoad()
}
