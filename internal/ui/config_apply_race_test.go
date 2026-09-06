package ui

import (
	"runtime"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestStandaloneConfigApplyDeepSnapshotNoRace is the regression guard for the
// crash fixed by snapshotDeepDiveConfig. The standalone `dotfiles config <tool>`
// exit dispatches applyStandaloneConfigCmd, whose write runs on a tea worker
// goroutine. The claude-code config screen stays active until tea.Quit lands and
// keeps toggling a.deepDiveConfig.ClaudeCodeMCPs / CLITools on the UI goroutine.
//
// Before the fix the worker received a SHALLOW copy of deepDiveConfig whose map
// fields aliased those live maps, so ApplyConfigWithMCPs ranged over a map the UI
// goroutine was writing — a fatal concurrent map read/write. This test models the
// two goroutines directly; run under `go test -race` it must stay clean.
func TestStandaloneConfigApplyDeepSnapshotNoRace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	a := NewApp(true)
	a.deepDiveConfig = NewDeepDiveConfig()
	a.configStandalone = true
	a.startScreen = ScreenConfigClaudeCode
	a.theme = "catppuccin-mocha"
	// A non-empty MCP map guarantees the worker actually iterates it
	// (applyClaudeCodeConfig early-returns on an empty map).
	a.deepDiveConfig.ClaudeCodeMCPs["context7"] = true

	mcps := []string{
		"context7", "task-master", "github", "supabase",
		"convex", "puppeteer", "sequential-thinking",
	}

	// The worker goroutine drains and executes each apply Cmd — the goroutine
	// boundary the tea runtime crosses. Each Cmd's snapshot was already taken on
	// THIS (UI) goroutine inside applyStandaloneConfigWorker, so a correct worker
	// only ever reads owned data.
	cmds := make(chan tea.Cmd, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for cmd := range cmds {
			cmd()
		}
	}()

	const iterations = 5000
	for i := 0; i < iterations; i++ {
		// Build the apply Cmd on the UI goroutine (deep-snapshots here), hand it off.
		cmds <- a.applyStandaloneConfigWorker()

		// Keep toggling the live maps the claude-code screen owns, racing the
		// worker's execution of previously-dispatched Cmds.
		id := mcps[i%len(mcps)]
		a.deepDiveConfig.ClaudeCodeMCPs[id] = !a.deepDiveConfig.ClaudeCodeMCPs[id]
		a.deepDiveConfig.CLITools["claude-code"] = !a.deepDiveConfig.CLITools["claude-code"]
		if i%64 == 0 {
			runtime.Gosched()
		}
	}
	close(cmds)
	wg.Wait()
}
