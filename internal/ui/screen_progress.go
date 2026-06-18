package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/runner"
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
		a.installOutput = append(a.installOutput, msg.line.Text)
		s.capOutput()
		if msg.line.Type == runner.OutputStep {
			a.installStep++
		}
		return s, nil

	case installEventMsg:
		// Streamed progress from the install worker goroutine, applied here on the
		// main loop so the worker never touches shared App state.
		if msg.line != "" {
			a.installOutput = append(a.installOutput, msg.line)
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
		if msg.err != nil {
			if msg.context != "" {
				a.lastError = fmt.Errorf("%v\n\nOutput:\n%s", msg.err, msg.context)
			} else {
				a.lastError = msg.err
			}
			return s, a.showError(a.lastError)
		}
		return s, nil

	case installLogMsg:
		// Shared install/update streaming log line. Currently emitted by neither
		// path, but handled here so a future per-line streamer works while this
		// screen is active.
		a.appendInstallLog(msg.line)
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

	steps := []struct {
		name string
	}{
		{"Initializing backup"},
		{"Installing packages"},
		{"Configuring zsh"},
		{"Configuring tmux"},
		{"Configuring ghostty"},
		{"Configuring yazi"},
		{"Configuring git"},
		{"Setting up utilities"},
		{"Setting up neovim"},
		{"Finalizing"},
	}

	// a.installStep is an opaque, monotonically increasing counter (one tick per
	// installed tool plus one per config phase), so it does not line up with the
	// fixed 10-entry display list and routinely exceeds len(steps). Map it onto a
	// bounded "current phase" so the list never renders every step as complete
	// while the install is still running.
	currentPhase := a.installStep
	if a.installComplete {
		// Everything done: mark all steps complete.
		currentPhase = len(steps)
	} else if a.installRunning {
		// In progress: keep an active step visible and never let the whole list
		// flip to complete (clamp to the last step at most).
		if currentPhase > len(steps)-1 {
			currentPhase = len(steps) - 1
		}
		if currentPhase < 0 {
			currentPhase = 0
		}
	} else {
		// Not started yet.
		currentPhase = 0
	}

	var stepList strings.Builder
	for i, st := range steps {
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

	// Calculate progress, clamped to [0,1] (currentPhase can equal len(steps)).
	progressPercent := float64(currentPhase) / float64(len(steps))
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
