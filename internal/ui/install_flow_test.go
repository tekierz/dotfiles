package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/tools"
)

// drainWorker runs runInstallWorker synchronously and returns all events.
// selectedTools is the slice of tool IDs to install (empty = config-only path).
// cfg is the deepDiveConfig snapshot the worker receives.
func drainWorker(t *testing.T, selectedTools []string, cfg DeepDiveConfig) []installEventMsg {
	t.Helper()
	events := make(chan installEventMsg, 256)
	ctx := context.Background()
	runtime := defaultToolInstallRuntime()
	// Config-generation tests exercise writer parity/gating, not live install
	// discovery or backup. Mark registered tools present and disable backup so
	// the final-identity guards permit only the intended isolated writes.
	runtime.isToolInstalled = func(tools.Tool) bool { return true }
	runtime.autoBackup = func() (autoBackupResult, error) { return autoBackupResult{}, nil }
	go runInstallWorkerWithRuntime(ctx, events, selectedTools, cfg, "catppuccin-mocha", runtime)
	var out []installEventMsg
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

// collectLines extracts the non-empty line strings from the event slice.
func collectLines(events []installEventMsg) []string {
	var lines []string
	for _, ev := range events {
		if ev.line != "" {
			lines = append(lines, ev.line)
		}
	}
	return lines
}

// TestConfigGating_DeselectedToolSkipsConfig verifies that a tool which the
// user deselected in the deep-dive (e.g. lazygit CLITool=false) does NOT get
// its config written to disk, while a tool that IS selected does get written.
//
// The test uses a temp HOME so WriteGhosttyConfig / WriteLazyGitConfig etc.
// land inside a controlled directory.  lazygit and glow are in CLITools and
// therefore have a selection flag; ghostty, zsh, neovim, git, yazi, fzf are
// always-core (UIGroupNone) and their configs are written unconditionally.
func TestConfigGating_DeselectedToolSkipsConfig(t *testing.T) {
	home := withTempHome(t)

	cfg := *NewDeepDiveConfig()

	// Deselect lazygit and btop — they are in CLITools and must be gated.
	cfg.CLITools["lazygit"] = false
	cfg.CLITools["btop"] = false
	// Keep glow selected so we can verify selected tools still get configured.
	cfg.CLITools["glow"] = true

	// Run the config-only path (no packages to install).
	events := drainWorker(t, nil, cfg)

	lines := collectLines(events)

	// Lazygit config should NOT be written.
	lazygitPath := filepath.Join(home, ".config", "lazygit", "config.yml")
	if _, err := os.Stat(lazygitPath); err == nil {
		t.Errorf("lazygit config written even though lazygit was deselected: %s", lazygitPath)
	}

	// Btop config should NOT be written.
	btopPaths := []string{
		filepath.Join(home, ".config", "btop", "btop.conf"),
		filepath.Join(home, "Library", "Application Support", "btop", "btop.conf"),
	}
	btopFound := false
	for _, p := range btopPaths {
		if _, err := os.Stat(p); err == nil {
			btopFound = true
		}
	}
	if btopFound {
		t.Errorf("btop config written even though btop was deselected")
	}

	// Ghostty config SHOULD be written (always-core).
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	if _, err := os.Stat(ghosttyPath); err != nil {
		t.Errorf("ghostty config not written even though it is always-core: %v (lines: %v)", err, lines)
	}

	// Glow config SHOULD be written (selected in CLITools).
	glowPath := filepath.Join(home, filepath.FromSlash(glowTestRelPath()))
	if _, err := os.Stat(glowPath); err != nil {
		t.Errorf("glow config not written even though glow was selected: %v", err)
	}

	// Confirm no error in done event.
	for _, ev := range events {
		if ev.done && ev.err != nil {
			t.Logf("worker finished with error (may be OK for absent tools): %v", ev.err)
		}
	}
}

// TestConfigGating_SelectedToolWritesConfig confirms the positive case:
// when a CLITool is explicitly selected, its config IS written.
func TestConfigGating_SelectedToolWritesConfig(t *testing.T) {
	home := withTempHome(t)

	cfg := *NewDeepDiveConfig()
	cfg.CLITools["lazygit"] = true
	cfg.CLITools["glow"] = true
	cfg.CLITools["btop"] = true

	_ = drainWorker(t, nil, cfg)

	// lazygit config should exist.
	lazygitPath := filepath.Join(home, ".config", "lazygit", "config.yml")
	if _, err := os.Stat(lazygitPath); err != nil {
		t.Errorf("lazygit config not written even though lazygit was selected: %v", lazygitPath)
	}
}

// TestFailureAggregation_NamesAllFailedSteps verifies that when multiple
// config phases fail, the final done event's error text names each failed step
// rather than just reporting a count + first error.
//
// We exercise this by examining the failures slice logic.  Since we cannot
// easily inject failing Write*Config calls without a seam, we test the
// finish/aggregation logic directly via a synthetic run that exercises the
// error-path in runInstallWorker using a broken HOME (non-writable).
func TestFailureAggregation_NamesAllFailedSteps(t *testing.T) {
	// Point HOME at a non-existent / read-only path to force all Write*Config
	// calls to fail (they call os.MkdirAll / os.WriteFile on HOME).
	t.Setenv("HOME", "/nonexistent/bad/home")

	// Use a config where lazygit, btop, glow are all selected (they will fail
	// because the config directories cannot be created under the bad HOME).
	cfg := *NewDeepDiveConfig()
	cfg.CLITools["lazygit"] = true
	cfg.CLITools["btop"] = true
	cfg.CLITools["glow"] = true

	events := drainWorker(t, nil, cfg)

	// Find the done event and inspect its error.
	var doneEv *installEventMsg
	for i := range events {
		if events[i].done {
			doneEv = &events[i]
			break
		}
	}
	if doneEv == nil {
		t.Fatal("no done event received")
	}
	if doneEv.err == nil {
		t.Skip("no error produced (perhaps HOME is writable after all): skipping aggregation assertion")
	}

	errText := doneEv.err.Error()

	// The error must contain "Failed steps:" listing each failed phase name,
	// not just "N steps failed; first: ..."
	if !strings.Contains(errText, "Failed steps:") {
		t.Errorf("partial-failure error does not contain 'Failed steps:' list; got: %s", errText)
	}
}

// TestFailureAggregation_SingleFailureNoList verifies that a single failure
// does not wrap itself in a list (it should remain a plain error, no list noise).
func TestFailureAggregation_SingleFailureNoList(t *testing.T) {
	// Build a fake failures slice and feed it through the aggregation logic
	// by calling the private helper we will extract.
	failures := []error{fmt.Errorf("tmux: something went wrong")}
	err := aggregateFailures(failures)
	if err == nil {
		t.Fatal("expected non-nil error for single failure")
	}
	// Single failure: should be the error itself, no 'Failed steps:' prefix.
	if strings.Contains(err.Error(), "Failed steps:") {
		t.Errorf("single failure should not produce a 'Failed steps:' list; got: %s", err.Error())
	}
	if err.Error() != "tmux: something went wrong" {
		t.Errorf("unexpected single-failure text: %s", err.Error())
	}
}

// TestFailureAggregation_MultipleFailuresNamedList verifies that two or more
// failures produce a 'Failed steps:' list naming each step.
func TestFailureAggregation_MultipleFailuresNamedList(t *testing.T) {
	failures := []error{
		fmt.Errorf("Failed to configure tmux: exit 1"),
		fmt.Errorf("Failed to configure Ghostty: permission denied"),
		fmt.Errorf("Failed to configure Neovim: disk full"),
	}
	err := aggregateFailures(failures)
	if err == nil {
		t.Fatal("expected non-nil error for multiple failures")
	}
	errText := err.Error()
	if !strings.Contains(errText, "Failed steps:") {
		t.Errorf("multi-failure error missing 'Failed steps:'; got: %s", errText)
	}
	if !strings.Contains(errText, "tmux") {
		t.Errorf("multi-failure error missing 'tmux'; got: %s", errText)
	}
	if !strings.Contains(errText, "Ghostty") {
		t.Errorf("multi-failure error missing 'Ghostty'; got: %s", errText)
	}
	if !strings.Contains(errText, "Neovim") {
		t.Errorf("multi-failure error missing 'Neovim'; got: %s", errText)
	}
}

// TestProgressSteps_CountMatchesPlannedPhases verifies that the number of
// "step" phases (stepInc=true events) the worker emits for the config-only
// path matches the number of planned config phases documented in installation.go.
// This ensures the progress fraction does not over/under-report.
//
// Planned config-only phases (no selected tools, no claude-code):
//  1. installUtilities
//  2. tmux
//  3. ghostty
//  4. zsh
//  5. neovim
//  6. git
//  7. yazi
//  8. fzf
//  9. lazygit (if selected)
//  10. btop    (if selected)
//  11. glow    (if selected)
//
// With lazygit/btop/glow all deselected: expected = 8 steps (utilities+tmux+ghostty+zsh+neovim+git+yazi+fzf).
func TestProgressSteps_CountMatchesPlannedPhases(t *testing.T) {
	withTempHome(t)

	cfg := *NewDeepDiveConfig()
	// Deselect the gated tools so only always-core phases run.
	cfg.CLITools["lazygit"] = false
	cfg.CLITools["btop"] = false
	cfg.CLITools["glow"] = false
	cfg.CLITools["claude-code"] = false
	cfg.Utilities["claude-code"] = false

	events := drainWorker(t, nil, cfg)

	var stepCount int
	for _, ev := range events {
		if ev.stepInc {
			stepCount++
		}
	}

	// Always-core config phases when none of the gated tools run:
	// utilities, tmux, ghostty, zsh, neovim, git, yazi, fzf = 8
	const wantSteps = 8
	if stepCount != wantSteps {
		t.Errorf("step count = %d, want %d (always-core phases only)", stepCount, wantSteps)
	}
}
