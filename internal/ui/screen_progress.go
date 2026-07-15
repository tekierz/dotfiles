package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/installapply"
	"github.com/tekierz/dotfiles/internal/operation"
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
	a.stageInstallationAttempt(a.pendingInstallPlan)
	if a.pendingInstallPlan != nil && a.pendingInstallPlan.needsSudo() && runner.NeedsSudo() && !runner.CheckSudoCached() {
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
			a.deepDiveContinuation = nil
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
				if a.deepDiveContinuation != nil {
					a.deepDiveContinuation = nil
					a.invalidatePendingInstallPlan()
					return s, NavigateTo(ScreenNavPicker)
				}
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
		a.stageInstallationAttempt(a.pendingInstallPlan)
		if a.pendingInstallPlan != nil && a.pendingInstallPlan.needsSudo() && runner.NeedsSudo() && !runner.CheckSudoCached() {
			return s, func() tea.Msg { return sudoRequiredMsg{} }
		}
		return s, a.startInstallation()

	case sudoRequiredMsg:
		return s, tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
			return sudoCachedMsg{err: err}
		})

	case sudoCachedMsg:
		if msg.err != nil {
			a.stageInstallationAttempt(a.pendingInstallPlan)
			a.lastError = msg.err
			a.finishInstallationAttempt(installationOutcomeFailed)
			a.deepDiveContinuation = nil
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
			if msg.operationID != "" {
				a.lastOperationID = msg.operationID
			}
			a.installEvents = nil
			// Route into the done handler (mark complete / navigate / error).
			return s.Update(installDoneMsg{err: msg.err, context: msg.context, result: msg.result})
		}
		// Re-subscribe for the next event to keep the stream flowing.
		return s, a.listenInstallEventsCmd()

	case installDoneMsg:
		outcome := installationOutcomeSucceeded
		continuation := a.deepDiveContinuation
		configurationOnly := a.pendingInstallPlan != nil && a.pendingInstallPlan.configurationOnly
		if msg.err == nil && msg.result.Status == operation.StatusPhaseComplete && msg.result.Next == installapply.NextReplanRequired {
			outcome = installationOutcomeReplanRequired
		} else if msg.err != nil {
			outcome = installationOutcomeFailed
		}
		if msg.err == nil && continuation != nil {
			switch {
			case continuation.stage == deepDiveContinuationPackageReview &&
				outcome == installationOutcomeReplanRequired &&
				msg.result.PhaseKind == operation.InstallPhasePrerequisite &&
				msg.result.PhaseIndex == 1 &&
				len(msg.result.RemainingTools()) != 0 &&
				slices.Equal(msg.result.RequestedTools(), continuation.requestedTools):
				// Phase 1 completed. Retain only intent and require a fresh phase 2 review.
			case continuation.stage == deepDiveContinuationPackageReview &&
				msg.result.Status == operation.StatusSucceeded &&
				msg.result.Next == installapply.NextComplete &&
				msg.result.PhaseKind == operation.InstallPhaseNPM &&
				msg.result.PhaseIndex == 2 &&
				len(msg.result.RemainingTools()) == 0 &&
				slices.Equal(msg.result.RequestedTools(), continuation.requestedTools):
				continuation.stage = deepDiveContinuationConfigReview
				outcome = installationOutcomeConfigurationReviewRequired
			case continuation.stage == deepDiveContinuationConfigReview && configurationOnly:
				// The existing reviewed config worker completed the final operation.
				outcome = installationOutcomeSucceeded
				a.deepDiveContinuation = nil
			default:
				msg.err = fmt.Errorf("Deep Dive continuation result did not match the reviewed phase")
				outcome = installationOutcomeFailed
				a.deepDiveContinuation = nil
			}
		}
		if msg.err != nil {
			a.deepDiveContinuation = nil
		}
		a.finishInstallationAttemptWithResult(outcome, msg.result)
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
				a.lastError = fmt.Errorf("%w\n\nOutput:\n%s", msg.err, msg.context)
			} else {
				a.lastError = msg.err
			}
			return s, tea.Batch(a.showError(a.lastError), reloadCmd)
		}
		a.lastError = nil
		// Successful install: the Manage / Deep-Dive install-status caches now
		// show stale "not installed" for the just-installed tools. Invalidate both
		// the registry's IsInstalled() cache and the App's manageInstalled cache so
		// the next navigation triggers a fresh load (C10).
		tools.GetRegistry().InvalidateCache()
		a.manageInstalledReady = false
		if outcome == installationOutcomeReplanRequired || outcome == installationOutcomeConfigurationReviewRequired {
			a.invalidatePendingInstallPlan()
			return s, a.startInstallCacheLoad()
		}
		a.installReviewTools = nil
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

// View renders the installation progress screen. The supplied dimensions are
// authoritative: ScreenManager owns the current terminal size, while the
// dimensions retained on App can briefly lag during a resize.
func (s *progressScreen) View(width, height int) string {
	a := s.App()
	if width <= 0 || height <= 0 {
		return ""
	}
	// Below the minimum full-layout footprint, render a truthful one-line state
	// instead of asking lipgloss to place an oversized bordered panel.
	if width < 30 || height < 14 {
		label := "Installing..."
		if a.installComplete {
			label = "Incomplete"
			if a.installOutcome == installationOutcomeSucceeded && a.installSummaryFacts.outcome == installationOutcomeSucceeded {
				label = "Complete"
			} else if a.installOutcome == installationOutcomeReplanRequired && a.installSummaryFacts.outcome == installationOutcomeReplanRequired {
				label = "Prerequisites complete — replan required"
			} else if a.installOutcome == installationOutcomeConfigurationReviewRequired && a.installSummaryFacts.outcome == installationOutcomeConfigurationReviewRequired {
				label = "Tools installed — configuration review required"
			}
		}
		return PlaceWithBackground(width, height, truncateVisible(label, width))
	}

	title := TitleStyle.Render("Installing...")
	succeeded := a.installComplete && a.installOutcome == installationOutcomeSucceeded &&
		a.installSummaryFacts.outcome == installationOutcomeSucceeded
	replanRequired := a.installComplete && a.installOutcome == installationOutcomeReplanRequired &&
		a.installSummaryFacts.outcome == installationOutcomeReplanRequired
	configurationReviewRequired := a.installComplete && a.installOutcome == installationOutcomeConfigurationReviewRequired &&
		a.installSummaryFacts.outcome == installationOutcomeConfigurationReviewRequired
	if succeeded {
		title = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("✓ Installation Complete!")
	} else if replanRequired {
		title = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("✓ Prerequisites Complete — Fresh Review Required")
	} else if configurationReviewRequired {
		title = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("✓ Tools Installed — Configuration Review Required")
	} else if a.installComplete {
		title = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).Render("! Installation Incomplete")
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
	phaseKind := a.installSummaryFacts.phaseKind
	if phaseKind == "" && a.pendingInstallPlan != nil {
		if phase, phased := a.pendingInstallPlan.phase(); phased {
			phaseKind = phase.Kind()
		}
	}
	switch phaseKind {
	case operation.InstallPhasePrerequisite:
		displaySteps = []struct{ name string }{{"Installing Node/npm prerequisites"}}
	case operation.InstallPhaseNPM:
		displaySteps = []struct{ name string }{{"Installing reviewed npm tools"}}
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
	failed := a.installComplete && a.installOutcome == installationOutcomeFailed
	currentPhase := 0
	if succeeded || replanRequired || configurationReviewRequired {
		currentPhase = len(displaySteps)
	} else if a.installStep > 0 {
		// Scale the raw step counter proportionally onto the display list.
		scaled := int(float64(a.installStep) / float64(totalSteps) * float64(len(displaySteps)))
		if scaled >= len(displaySteps) {
			scaled = len(displaySteps) - 1
		}
		currentPhase = scaled
	}

	phaseCount := 5
	containerPadY := 1
	if height <= 20 {
		phaseCount = 2
		containerPadY = 0
	} else if height >= 34 {
		phaseCount = len(displaySteps)
	}
	phaseStart := currentPhase - phaseCount + 1
	if succeeded || replanRequired || configurationReviewRequired {
		phaseStart = len(displaySteps) - phaseCount
	}
	phaseStart = clampInt(phaseStart, 0, maxInt(0, len(displaySteps)-phaseCount))
	phaseEnd := min(len(displaySteps), phaseStart+phaseCount)

	phaseLines := make([]string, 0, phaseEnd-phaseStart)
	for i := phaseStart; i < phaseEnd; i++ {
		st := displaySteps[i]
		var status string
		var style lipgloss.Style

		switch {
		case failed && i < currentPhase:
			status = "•"
			style = lipgloss.NewStyle().Foreground(ColorTextMuted)
		case failed && i == currentPhase:
			status = "✗"
			style = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
		case (succeeded || replanRequired || configurationReviewRequired) && i < currentPhase:
			status = "✓"
			style = lipgloss.NewStyle().Foreground(ColorGreen)
		case i < currentPhase:
			status = "•"
			style = lipgloss.NewStyle().Foreground(ColorTextMuted)
		case i == currentPhase && a.installRunning:
			status = "▶"
			style = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		default:
			status = "○"
			style = lipgloss.NewStyle().Foreground(ColorTextMuted)
		}
		phaseLines = append(phaseLines, style.Render(fmt.Sprintf("  %s %s", status, st.name)))
	}
	stepList := strings.Join(phaseLines, "\n")

	// Progress fraction derived from the ACTUAL planned phase count, so the bar
	// reflects real completion rather than the fixed display list length.
	progressPercent := float64(a.installStep) / float64(totalSteps)
	if succeeded || replanRequired || configurationReviewRequired {
		progressPercent = 1.0
	} else if progressPercent >= 1.0 {
		// Only a sealed successful attempt may render a full bar. A failed,
		// running, stale, or otherwise unknown attempt keeps its terminal phase
		// visibly incomplete.
		progressPercent = float64(maxInt(0, totalSteps-1)) / float64(totalSteps)
	}
	if progressPercent < 0 {
		progressPercent = 0
	} else if progressPercent > 1 {
		progressPercent = 1
	}
	containerPadX := 1
	if width >= 100 {
		containerPadX = 2
	}
	contentWidth := maxInt(1, width-2-(2*containerPadX))
	progressW := min(50, contentWidth)
	progress := ProgressBar(progressPercent, progressW)

	// Output panel - show real output.
	outputCapacity := 3
	if height <= 20 {
		outputCapacity = 1
	} else if height >= 34 {
		outputCapacity = 8
	}
	panelOuterWidth := min(72, contentWidth)
	panelInnerWidth := maxInt(1, panelOuterWidth-2)
	var outputRows []string
	switch {
	case len(a.installOutput) > 0:
		start := 0
		if len(a.installOutput) > outputCapacity {
			start = len(a.installOutput) - outputCapacity
		}
		outputRows = make([]string, 0, len(a.installOutput)-start)
		for _, line := range a.installOutput[start:] {
			outputRows = append(outputRows, truncateVisible(sanitizeLogLine(line), panelInnerWidth))
		}
	case a.installRunning:
		outputRows = []string{"Starting installation..."}
	case !a.installComplete:
		outputRows = []string{"Press ENTER to start"}
	default:
		outputRows = []string{"No installer output captured"}
	}
	for len(outputRows) < outputCapacity {
		outputRows = append(outputRows, "")
	}
	outputLines := strings.Join(outputRows, "\n")
	if len(a.installOutput) == 0 {
		outputLines = lipgloss.NewStyle().Foreground(ColorTextMuted).Render(outputLines)
	}

	output := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(panelInnerWidth).
		Height(outputCapacity).
		Render(outputLines)

	var help string
	switch {
	case a.installComplete:
		help = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[ENTER] Continue")
	case a.installRunning:
		help = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Installation in progress...")
	default:
		help = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[ENTER] Start    [ESC] Back")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		stepList,
		"",
		progress,
		"",
		output,
		"",
		help,
	)

	container := ContainerStyle.
		Padding(containerPadY, containerPadX).
		Width(maxInt(1, width-2)).
		Render(content)
	return PlaceWithBackground(width, height, container)
}
