package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/operation"
)

func TestProgressScreenResponsiveStatesFitSupportedTerminals(t *testing.T) {
	sizes := []struct {
		width  int
		height int
	}{
		{width: 60, height: 18},
		{width: 80, height: 24},
		{width: 120, height: 40},
	}
	states := []struct {
		name      string
		running   bool
		complete  bool
		outcome   installationOutcome
		title     string
		help      string
		phase     string
		phaseKind operation.InstallPhaseKind
	}{
		{name: "running", running: true, outcome: installationOutcomeRunning, title: "Installing...", help: "Installation in progress", phase: "Configuring git"},
		{name: "succeeded", complete: true, outcome: installationOutcomeSucceeded, title: "Installation Complete", help: "[ENTER] Continue", phase: "Configuring tools"},
		{name: "replan-required", complete: true, outcome: installationOutcomeReplanRequired, title: "Prerequisites Complete", help: "[ENTER] Continue", phase: "Installing Node/npm prerequisites", phaseKind: operation.InstallPhasePrerequisite},
		{name: "configuration-review-required", complete: true, outcome: installationOutcomeConfigurationReviewRequired, title: "Tools Installed", help: "[ENTER] Continue", phase: "Installing reviewed npm tools", phaseKind: operation.InstallPhaseNPM},
		{name: "failed", complete: true, outcome: installationOutcomeFailed, title: "Installation Incomplete", help: "[ENTER] Continue", phase: "Configuring git"},
	}

	for _, size := range sizes {
		for _, state := range states {
			t.Run(fmt.Sprintf("%s/%dx%d", state.name, size.width, size.height), func(t *testing.T) {
				ctx := newGoldenContext(t)
				// Deliberately disagree with the render dimensions. View's arguments
				// are the screen contract and must be authoritative.
				ctx.app.width, ctx.app.height = 180, 70
				ctx.app.installRunning = state.running
				ctx.app.installComplete = state.complete
				ctx.app.installOutcome = state.outcome
				ctx.app.installSummaryFacts.outcome = state.outcome
				ctx.app.installSummaryFacts.phaseKind = state.phaseKind
				ctx.app.installPlannedSteps = 10
				ctx.app.installStep = 6
				ctx.app.installOutput = []string{
					"older package output",
					"latest installer output",
				}

				view := NewProgressScreen(ctx).View(size.width, size.height)
				plain := stripANSITest(view)
				for _, want := range []string{state.title, state.help, "latest installer output", state.phase, "█"} {
					if !strings.Contains(plain, want) {
						t.Fatalf("%s %dx%d missing %q:\n%s", state.name, size.width, size.height, want, plain)
					}
				}
				assertProgressViewFits(t, view, size.width, size.height)

				switch state.outcome {
				case installationOutcomeSucceeded:
					if !strings.Contains(plain, "✓") || strings.Contains(plain, "░") {
						t.Fatalf("succeeded %dx%d did not render an exclusively complete bar/state:\n%s", size.width, size.height, plain)
					}
				case installationOutcomeReplanRequired, installationOutcomeConfigurationReviewRequired:
					if !strings.Contains(plain, "✓") || strings.Contains(plain, "░") {
						t.Fatalf("%s %dx%d did not render an exclusively complete phase boundary:\n%s", state.name, size.width, size.height, plain)
					}
					if strings.Contains(plain, "Installation Complete") {
						t.Fatalf("%s %dx%d rendered false full-install completion:\n%s", state.name, size.width, size.height, plain)
					}
				case installationOutcomeFailed:
					for _, forbidden := range []string{"Installation Complete", "✓", "100%"} {
						if strings.Contains(plain, forbidden) {
							t.Fatalf("failed %dx%d rendered false success %q:\n%s", size.width, size.height, forbidden, plain)
						}
					}
					for _, want := range []string{"✗", "░"} {
						if !strings.Contains(plain, want) {
							t.Fatalf("failed %dx%d missing terminal marker %q:\n%s", size.width, size.height, want, plain)
						}
					}
				case installationOutcomePending, installationOutcomeRunning:
					if !strings.Contains(plain, "░") {
						t.Fatalf("running %dx%d rendered a full progress bar:\n%s", size.width, size.height, plain)
					}
				}
			})
		}
	}
}

func TestProgressScreenSuccessFailsClosedWhenSealedFactsDisagree(t *testing.T) {
	for _, factsOutcome := range []installationOutcome{installationOutcomePending, installationOutcomeFailed} {
		ctx := newGoldenContext(t)
		ctx.app.installComplete = true
		ctx.app.installOutcome = installationOutcomeSucceeded
		ctx.app.installSummaryFacts.outcome = factsOutcome
		ctx.app.installPlannedSteps = 10
		ctx.app.installStep = 10
		ctx.app.installOutput = []string{"terminal outcome mismatch"}

		plain := stripANSITest(NewProgressScreen(ctx).View(80, 24))
		for _, forbidden := range []string{"Installation Complete", "✓", "100%"} {
			if strings.Contains(plain, forbidden) {
				t.Fatalf("sealed outcome %q rendered false success %q:\n%s", factsOutcome, forbidden, plain)
			}
		}
		for _, want := range []string{"Installation Incomplete", "░", "terminal outcome mismatch"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("sealed outcome %q missing fail-closed marker %q:\n%s", factsOutcome, want, plain)
			}
		}
	}
}

func TestProgressScreenRunningAtPlannedStepNeverRendersFull(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installRunning = true
	ctx.app.installOutcome = installationOutcomeRunning
	ctx.app.installSummaryFacts.outcome = installationOutcomeRunning
	ctx.app.installPlannedSteps = 10
	ctx.app.installStep = 10
	ctx.app.installOutput = []string{"final phase still running"}

	plain := stripANSITest(NewProgressScreen(ctx).View(60, 18))
	for _, want := range []string{"Installing...", "Installation in progress", "final phase still running", "░"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("running terminal phase missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Installation Complete") {
		t.Fatalf("running terminal phase rendered false completion:\n%s", plain)
	}
}

func TestProgressScreenTinyTerminalsDoNotOverflow(t *testing.T) {
	for _, size := range []struct{ width, height int }{{1, 1}, {11, 4}, {29, 13}} {
		ctx := newGoldenContext(t)
		ctx.app.width, ctx.app.height = 180, 70
		ctx.app.installComplete = true
		ctx.app.installOutcome = installationOutcomeFailed
		ctx.app.installSummaryFacts.outcome = installationOutcomeFailed
		view := NewProgressScreen(ctx).View(size.width, size.height)
		assertProgressViewFits(t, view, size.width, size.height)
	}
}

func TestProgressScreenBoundsLongANSIOutput(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = 200, 80
	ctx.app.installRunning = true
	ctx.app.installOutcome = installationOutcomeRunning
	ctx.app.installPlannedSteps = 10
	ctx.app.installStep = 4
	ctx.app.installOutput = []string{
		"older output",
		"\x1b[31mLATEST " + strings.Repeat("very-wide-output-", 20) + "\x1b[0m",
	}

	view := NewProgressScreen(ctx).View(60, 18)
	if !strings.Contains(stripANSITest(view), "LATEST") {
		t.Fatalf("compact progress omitted latest long output:\n%s", stripANSITest(view))
	}
	assertProgressViewFits(t, view, 60, 18)
}

func TestProgressScreenCompactShowsCurrentAndRecentPhases(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = 140, 50
	ctx.app.installRunning = true
	ctx.app.installOutcome = installationOutcomeRunning
	ctx.app.installPlannedSteps = 10
	ctx.app.installStep = 6
	ctx.app.installOutput = []string{"bounded output"}

	plain := stripANSITest(NewProgressScreen(ctx).View(60, 18))
	for _, want := range []string{"Configuring neovim", "Configuring git", "bounded output"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("compact progress omitted current/recent content %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "Installing packages") || strings.Contains(plain, "Configuring tools") {
		t.Fatalf("compact progress rendered the full phase list instead of the current window:\n%s", plain)
	}
}

func TestProgressScreenExpandedUsesRenderDimensionsNotStoredAppSize(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = 40, 14
	ctx.app.installRunning = true
	ctx.app.installOutcome = installationOutcomeRunning
	ctx.app.installPlannedSteps = 10
	ctx.app.installStep = 6
	ctx.app.installOutput = []string{
		"output one", "output two", "output three", "output four",
		"output five", "output six", "output seven", "output eight",
	}

	view := NewProgressScreen(ctx).View(120, 40)
	plain := stripANSITest(view)
	for _, want := range []string{"Installing packages", "Configuring tools", "output one", "output eight"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expanded progress omitted %q when App stored 40x14:\n%s", want, plain)
		}
	}
	assertProgressViewFits(t, view, 120, 40)
}

func assertProgressViewFits(t *testing.T, view string, width, height int) {
	t.Helper()
	plain := stripANSITest(view)
	lines := strings.Split(plain, "\n")
	if len(lines) > height {
		t.Fatalf("progress view rendered %d lines, exceeds height %d:\n%s", len(lines), height, plain)
	}
	for lineNumber, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("progress line %d width=%d exceeds %d: %q", lineNumber+1, got, width, line)
		}
	}
}
