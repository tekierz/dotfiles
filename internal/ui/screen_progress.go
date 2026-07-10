package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/tools"
)

// progressScreen is the migrated ScreenHandler for the installation progress
// screen (the streaming step list + output panel shown while the installer
// runs).
//
// State stays on App: the step counter (installStep), the rolling output buffer
// (installOutput, capped at 20 lines), the run/complete flags (installRunning,
// installComplete), the install events channel (installEvents) and the final
// error (lastError) are read/written through s.App().
//
// On-enter trigger (robust): the install is started from Init() rather than by
// waiting for a separate installStartMsg to arrive after the NavigateMsg. In a
// tea.Batch the relative ordering of a NavigateMsg and a sibling installStartMsg
// is not guaranteed, so an installStartMsg could be consumed by the FileTree
// handler (or dropped) before this screen became active. Init() does the sudo
// check itself: if the platform needs sudo and it is not cached, it returns the
// sudo-prompt exec (whose callback yields sudoCachedMsg, handled here); otherwise
// it starts the installation directly via a.startInstallation(). The FileTree
// handler now simply NavigateTo(ScreenProgress) and lets Init start the work.
//
// Async-in-handler / streaming: because the ScreenManager delegates every
// non-navigation message to this handler while it is active, the install async
// results are handled here (not in App.Update). The streaming install worker
// (runInstallWorker) writes ONLY to a.installEvents; this handler applies each
// installEventMsg to App state on the main goroutine and RE-ISSUES
// listenInstallEventsCmd to keep the stream flowing, exactly as the legacy
// App.Update did. No goroutine touches shared App state, so there is no data
// race (verified with go test -race). On a `done` event it transitions to the
// installDoneMsg handling (mark complete; navigate to Summary, or to the Error
// screen on failure). installStartMsg, sudoRequiredMsg and sudoCachedMsg are
// also handled here in case any are still emitted into the stream.
type progressScreen struct {
	BaseScreen
}

// NewProgressScreen creates a new install-progress screen handler.
func NewProgressScreen(ctx *ScreenContext) *progressScreen {
	s := &progressScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *progressScreen) ID() Screen { return ScreenProgress }

// Init triggers the installation on entry. It does the sudo check itself so the
// install start does not depend on the ordering of a separate installStartMsg
// relative to the NavigateMsg in a tea.Batch.
func (s *progressScreen) Init() tea.Cmd {
	a := s.App()
	if a == nil {
		return nil
	}
	// Guard against re-triggering if the screen is re-entered while a run is in
	// flight (startInstallation also no-ops when installRunning).
	if a.installRunning {
		return nil
	}
	if runner.NeedsSudo() && !runner.CheckSudoCached() {
		// Prompt for sudo (exits the alt screen), then start the install.
		return tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
			return sudoCachedMsg{err: err}
		})
	}
	return a.startInstallation()
}

// Update handles the install streaming events, the sudo flow, and the keys for
// continuing (enter, when complete) and going back (esc, before the run starts).
func (s *progressScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// Cancel the running install worker + (sudo) package-manager subprocess
			// before quitting so they are not orphaned when the TUI exits (C15).
			a.teardownStream()
			return s, tea.Quit
		case "enter":
			// Only advance once the installation is complete.
			if !a.installRunning {
				return s, a.showSummary()
			}
			return s, nil
		case "esc":
			// Allow backing out only before the run starts (mirrors the legacy
			// "[ESC] Back" hint, which is only shown when not running/complete).
			if !a.installRunning && !a.installComplete {
				return s, NavigateTo(ScreenFileTree)
			}
			return s, nil
		}
		return s, nil

	// --- Install start / sudo flow (in case still emitted into the stream) ---
	case installStartMsg:
		if a.installRunning {
			return s, nil
		}
		if runner.NeedsSudo() && !runner.CheckSudoCached() {
			return s, func() tea.Msg { return sudoRequiredMsg{} }
		}
		return s, a.startInstallation()

	case sudoRequiredMsg:
		return s, tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
			return sudoCachedMsg{err: err}
		})

	case sudoCachedMsg:
		if msg.err != nil {
			a.lastError = msg.err
			return s, a.showError(msg.err)
		}
		return s, a.startInstallation()

	// --- Streaming install output ---
	case installOutputMsg:
		a.installOutput = append(a.installOutput, sanitizeLogLine(msg.line.Text))
		s.capOutput()
		if msg.line.Type == runner.OutputStep {
			a.installStep++
		}
		return s, nil

	case installEventMsg:
		// Streamed progress from the install worker goroutine, applied here on the
		// main loop so the worker never touches shared App state.
		if msg.line != "" {
			a.installOutput = append(a.installOutput, sanitizeLogLine(msg.line))
			s.capOutput()
		}
		if msg.stepInc {
			a.installStep++
		}
		if msg.done {
			a.installEvents = nil
			// Route into the done handler (mark complete / navigate / error).
			return s.Update(installDoneMsg{err: msg.err, context: msg.context})
		}
		// Re-subscribe for the next event to keep the stream flowing.
		return s, a.listenInstallEventsCmd()

	case installDoneMsg:
		a.installRunning = false
		a.installComplete = true
		// The install worker has finished; drop the retained cancel handle so a
		// later teardown (Ctrl+C on the summary) is a harmless no-op.
		a.streamCancel = nil
		a.streamCmd = nil
		// Stop the sudo keep-alive goroutine on normal completion (C16). The stop
		// func blocks until the goroutine exits, so no `sudo -v` keeps running after
		// the install finishes. nil/no-op when none was started (e.g. macOS).
		if a.sudoKeepAliveStop != nil {
			a.sudoKeepAliveStop()
			a.sudoKeepAliveStop = nil
		}
		if msg.err != nil {
			// Some tools may have installed successfully before a later step
			// failed. Invalidate and reload observations even on the error path so
			// retry/Manage does not operate from stale pre-install state.
			tools.GetRegistry().InvalidateCache()
			a.manageInstalledReady = false
			reloadCmd := a.startInstallCacheLoad()
			if msg.context != "" {
				a.lastError = fmt.Errorf("%v\n\nOutput:\n%s", msg.err, msg.context)
			} else {
				a.lastError = msg.err
			}
			return s, tea.Batch(a.showError(a.lastError), reloadCmd)
		}
		// Successful install: the Manage / Deep-Dive install-status caches now
		// show stale "not installed" for the just-installed tools. Invalidate both
		// the registry's IsInstalled() cache and the App's manageInstalled cache so
		// the next navigation triggers a fresh load (C10).
		tools.GetRegistry().InvalidateCache()
		a.manageInstalledReady = false
		return s, nil

	}
	return s, nil
}

// capOutput keeps only the last 20 output lines for display, copying in place to
// avoid a memory leak from re-slicing a growing backing array.
func (s *progressScreen) capOutput() {
	a := s.App()
	const maxOutputLines = 20
	if len(a.installOutput) > maxOutputLines {
		copy(a.installOutput, a.installOutput[len(a.installOutput)-maxOutputLines:])
		a.installOutput = a.installOutput[:maxOutputLines]
	}
}

// View renders the installation progress screen. It ports renderProgress,
// reading the live App state. The width/height args are accepted for interface
// conformance; the layout reads a.width/a.height directly.
func (s *progressScreen) View(width, height int) string {
	a := s.App()

	title := TitleStyle.Render("Installing...")
	if a.installComplete {
		title = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("✓ Installation Complete!")
	}

	// Build a concise display list from the always-core phases only; selected
	// tool installs and gated config phases (lazygit/btop/glow/claude-code) are
	// collapsed into "Installing packages" so the list stays compact.  The list is
	// decorative — the progress bar is the authoritative indicator. The step counter
	// (a.installStep) maps to the FULL planned-phase count (installPlannedSteps) to
	// drive the bar accurately rather than against this smaller display list.
	displaySteps := []struct{ name string }{
		{"Installing packages"},
		{"Setting up utilities"},
		{"Configuring tmux"},
		{"Configuring ghostty"},
		{"Configuring zsh"},
		{"Configuring neovim"},
		{"Configuring git"},
		{"Configuring yazi"},
		{"Configuring fzf"},
		{"Configuring tools"},
	}

	// Total planned steps, set by startInstallation when the install begins.
	// Falls back to the display list length for the rare case where the screen
	// is rendered before startInstallation has run (no over/under-report).
	totalSteps := a.installPlannedSteps
	if totalSteps <= 0 {
		totalSteps = len(displaySteps)
	}

	// Map a.installStep (cumulative step count, one per worker stepLine) onto
	// displaySteps so the highlighted entry tracks rough progress without ever
	// flipping the whole list to complete while the install is still running.
	currentPhase := 0
	if a.installComplete {
		currentPhase = len(displaySteps)
	} else if a.installRunning && a.installStep > 0 {
		// Scale the raw step counter proportionally onto the display list.
		scaled := int(float64(a.installStep) / float64(totalSteps) * float64(len(displaySteps)))
		if scaled >= len(displaySteps) {
			scaled = len(displaySteps) - 1
		}
		currentPhase = scaled
	}

	var stepList strings.Builder
	for i, st := range displaySteps {
		var status string
		var style lipgloss.Style

		if i < currentPhase {
			status = "✓"
			style = lipgloss.NewStyle().Foreground(ColorGreen)
		} else if i == currentPhase && a.installRunning {
			status = "▶"
			style = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		} else {
			status = "○"
			style = lipgloss.NewStyle().Foreground(ColorTextMuted)
		}
		stepList.WriteString(style.Render(fmt.Sprintf("  %s %s\n", status, st.name)))
	}

	// Progress fraction derived from the ACTUAL planned phase count, so the bar
	// reflects real completion rather than the fixed display list length.
	progressPercent := float64(a.installStep) / float64(totalSteps)
	if a.installComplete {
		progressPercent = 1.0
	}
	if progressPercent < 0 {
		progressPercent = 0
	} else if progressPercent > 1 {
		progressPercent = 1
	}
	progressW := min(50, maxInt(20, a.width-30))
	progress := ProgressBar(progressPercent, progressW)

	// Output panel - show real output.
	var outputLines string
	if len(a.installOutput) > 0 {
		// Show last 6 lines.
		start := 0
		if len(a.installOutput) > 6 {
			start = len(a.installOutput) - 6
		}
		outputLines = strings.Join(a.installOutput[start:], "\n")
	} else if a.installRunning {
		outputLines = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Starting installation...")
	} else if !a.installComplete {
		outputLines = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Press ENTER to start")
	}

	output := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		// Keep the output panel responsive so it doesn't overflow smaller terminals.
		// Note: Width/Height apply before borders in lipgloss, so subtract 2 to
		// target an approximate outer size.
		Width(maxInt(20, min(72, a.width-10)-2)).
		Height(clampInt(a.height/4, 6, 10)).
		Padding(0, 1).
		Render(outputLines)

	var help string
	if a.installComplete {
		help = HelpStyle.Render("[ENTER] Continue")
	} else if a.installRunning {
		help = HelpStyle.Render("Installation in progress...")
	} else {
		help = HelpStyle.Render("[ENTER] Start    [ESC] Back")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		stepList.String(),
		"",
		progress,
		"",
		output,
		"",
		help,
	)

	return PlaceWithBackground(a.width, a.height, ContainerStyle.Render(content))
}
