package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestInstallationOutcomeSuccessRendersCompleteSummary(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installRunning = true
	screen := NewProgressScreen(ctx)
	if _, cmd := screen.Update(installDoneMsg{}); cmd != nil {
		t.Fatalf("successful completion returned unexpected command %T", cmd())
	}
	if ctx.app.installOutcome != installationOutcomeSucceeded || ctx.app.installSummaryFacts.outcome != installationOutcomeSucceeded {
		t.Fatalf("successful completion state = outcome %q facts %q", ctx.app.installOutcome, ctx.app.installSummaryFacts.outcome)
	}
	view := NewSummaryScreen(ctx).View(80, 24)
	for _, want := range []string{"Installation Complete", "Next steps:", "dotfiles status", "dotfiles manage"} {
		if !strings.Contains(view, want) {
			t.Fatalf("successful summary missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "source ~/.zshrc") || strings.Contains(view, "p10k configure") {
		t.Fatalf("generic success summary guessed tool-specific next steps:\n%s", view)
	}
}

func TestInstallationErrorRecordsFailedOutcome(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installRunning = true
	screen := NewProgressScreen(ctx)
	_, cmd := screen.Update(installEventMsg{done: true, err: errors.New("installer failed"), context: "last output", operationID: "failed-operation"})
	if cmd == nil {
		t.Fatal("failed completion did not navigate to the error screen")
	}
	if ctx.app.installOutcome != installationOutcomeFailed || ctx.app.installSummaryFacts.outcome != installationOutcomeFailed || ctx.app.lastError == nil || ctx.app.lastOperationID != "failed-operation" {
		t.Fatalf("failed state outcome=%q facts=%q error=%v operation=%q", ctx.app.installOutcome, ctx.app.installSummaryFacts.outcome, ctx.app.lastError, ctx.app.lastOperationID)
	}
	if ctx.app.installSummaryFacts.operationID != "failed-operation" {
		t.Fatalf("failed terminal facts omitted operation ID: %+v", ctx.app.installSummaryFacts)
	}
	assertIncompleteInstallationSummary(t, NewSummaryScreen(ctx).View(80, 24))
}

func TestErrorScreenSkipForcesFailedSummary(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installOutcome = installationOutcomeSucceeded
	ctx.app.installComplete = true
	ctx.app.installSummaryFacts.outcome = installationOutcomeSucceeded
	ctx.app.lastError = errors.New("installer failed")
	screen := NewErrorScreen(ctx, ctx.app.lastError)
	_, cmd := screen.Update(keyMsg("s"))
	if cmd == nil {
		t.Fatal("skip did not navigate to summary")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || message.To != ScreenSummary {
		t.Fatalf("skip command = %#v", message)
	}
	if ctx.app.installOutcome != installationOutcomeFailed || ctx.app.installSummaryFacts.outcome != installationOutcomeFailed {
		t.Fatalf("skip state = outcome %q facts %q", ctx.app.installOutcome, ctx.app.installSummaryFacts.outcome)
	}
	assertIncompleteInstallationSummary(t, NewSummaryScreen(ctx).View(80, 24))
}

func TestInstallationRetryAndStartClearStaleFailure(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installOutcome = installationOutcomeFailed
	ctx.app.installComplete = true
	ctx.app.installSummaryFacts = installationSummaryFacts{outcome: installationOutcomeFailed, operationID: "stale-operation"}
	ctx.app.lastError = errors.New("stale failure")
	ctx.app.lastOperationID = "stale-operation"
	ctx.app.pendingInstallPlan = &installPlan{}
	screen := NewErrorScreen(ctx, ctx.app.lastError)
	_, cmd := screen.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("retry did not navigate to plan review")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || message.To != ScreenFileTree {
		t.Fatalf("retry command = %#v", message)
	}
	if ctx.app.installOutcome != installationOutcomePending || ctx.app.installComplete || ctx.app.lastError != nil || ctx.app.lastOperationID != "" || ctx.app.pendingInstallPlan != nil || ctx.app.installSummaryFacts != (installationSummaryFacts{}) {
		t.Fatalf("retry retained stale state: outcome=%q complete=%t error=%v operation=%q plan=%v facts=%+v", ctx.app.installOutcome, ctx.app.installComplete, ctx.app.lastError, ctx.app.lastOperationID, ctx.app.pendingInstallPlan != nil, ctx.app.installSummaryFacts)
	}

	ctx.app.installPlanError = errors.New("invalid plan")
	start := ctx.app.startInstallation()
	if start == nil || ctx.app.installOutcome != installationOutcomeRunning || ctx.app.installSummaryFacts.outcome != installationOutcomeRunning || ctx.app.lastError != nil || ctx.app.lastOperationID != "" {
		t.Fatalf("start state: outcome=%q facts=%q error=%v operation=%q command=%v", ctx.app.installOutcome, ctx.app.installSummaryFacts.outcome, ctx.app.lastError, ctx.app.lastOperationID, start)
	}
}

func TestErrorScreenEscapeAlsoRequiresFreshPlanReview(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installOutcome = installationOutcomeFailed
	ctx.app.installComplete = true
	ctx.app.installSummaryFacts = installationSummaryFacts{outcome: installationOutcomeFailed, operationID: "stale-operation"}
	ctx.app.lastError = errors.New("stale failure")
	ctx.app.lastOperationID = "stale-operation"
	ctx.app.pendingInstallPlan = &installPlan{installHash: "stale-plan"}

	_, cmd := NewErrorScreen(ctx, ctx.app.lastError).Update(keyMsg("esc"))
	message, ok := cmd().(NavigateMsg)
	if !ok || message.To != ScreenFileTree {
		t.Fatalf("escape command = %#v", message)
	}
	if ctx.app.installOutcome != installationOutcomePending || ctx.app.installComplete || ctx.app.lastError != nil || ctx.app.lastOperationID != "" || ctx.app.pendingInstallPlan != nil || ctx.app.installSummaryFacts != (installationSummaryFacts{}) {
		t.Fatalf("escape retained stale attempt state: outcome=%q complete=%t error=%v operation=%q plan=%v facts=%+v", ctx.app.installOutcome, ctx.app.installComplete, ctx.app.lastError, ctx.app.lastOperationID, ctx.app.pendingInstallPlan != nil, ctx.app.installSummaryFacts)
	}
}

func TestSudoFailureClearsStaleAttemptIdentityAndSealsCurrentPlan(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.pendingInstallPlan = &installPlan{installHash: "fresh-reviewed-plan"}
	ctx.app.installOutcome = installationOutcomeSucceeded
	ctx.app.installComplete = true
	ctx.app.installSummaryFacts = installationSummaryFacts{outcome: installationOutcomeSucceeded, planHash: "stale-plan", operationID: "stale-operation"}
	ctx.app.lastError = errors.New("stale error")
	ctx.app.lastOperationID = "stale-operation"

	_, cmd := NewProgressScreen(ctx).Update(sudoCachedMsg{err: errors.New("sudo authentication failed")})
	if cmd == nil || ctx.app.installOutcome != installationOutcomeFailed || ctx.app.installSummaryFacts.outcome != installationOutcomeFailed {
		t.Fatalf("sudo failure did not seal failed state: outcome=%q facts=%+v command=%v", ctx.app.installOutcome, ctx.app.installSummaryFacts, cmd)
	}
	if ctx.app.lastOperationID != "" || ctx.app.installSummaryFacts.operationID != "" || ctx.app.installSummaryFacts.planHash != "fresh-reviewed-plan" {
		t.Fatalf("sudo failure retained stale identity or lost current plan: last=%q facts=%+v", ctx.app.lastOperationID, ctx.app.installSummaryFacts)
	}
	if ctx.app.lastError == nil || !strings.Contains(ctx.app.lastError.Error(), "sudo authentication failed") {
		t.Fatalf("sudo failure error = %v", ctx.app.lastError)
	}
}

func TestInstallationSummaryFailsClosedForStaleOrUnknownOutcome(t *testing.T) {
	for _, outcome := range []installationOutcome{installationOutcomePending, installationOutcomeRunning, installationOutcome(255)} {
		t.Run(outcome.String(), func(t *testing.T) {
			ctx := newGoldenContext(t)
			ctx.app.installOutcome = outcome
			ctx.app.installComplete = true
			ctx.app.installSummaryFacts = installationSummaryFacts{outcome: outcome, operationID: "stale-operation"}
			assertIncompleteInstallationSummary(t, NewSummaryScreen(ctx).View(80, 24))
		})
	}

	ctx := newGoldenContext(t)
	ctx.app.installOutcome = installationOutcomeSucceeded
	ctx.app.installComplete = true
	ctx.app.installSummaryFacts.outcome = installationOutcomeFailed
	assertIncompleteInstallationSummary(t, NewSummaryScreen(ctx).View(80, 24))
}

func TestIncompleteInstallationSummaryRendersAtSupportedSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{40, 14}, {60, 18}, {80, 24}} {
		for _, outcome := range []installationOutcome{installationOutcomeFailed, installationOutcomePending} {
			ctx := newGoldenContext(t)
			ctx.app.installOutcome = outcome
			ctx.app.installComplete = true
			ctx.app.installSummaryFacts.outcome = outcome
			view := NewSummaryScreen(ctx).View(size.width, size.height)
			if strings.TrimSpace(view) == "" {
				t.Fatalf("%dx%d outcome %q rendered empty", size.width, size.height, outcome)
			}
			assertIncompleteInstallationSummary(t, view)
			assertRenderedWidth(t, view, size.width)
		}
	}
}

func TestInstallationSummaryKeepsEssentialFactsVisibleAtFortyByFourteen(t *testing.T) {
	for _, outcome := range []installationOutcome{installationOutcomeSucceeded, installationOutcomeFailed} {
		ctx := newGoldenContext(t)
		ctx.app.installComplete = true
		ctx.app.installOutcome = outcome
		ctx.app.installSummaryFacts = installationSummaryFacts{
			outcome: outcome, planHash: "accepted-plan-hash", actionCount: 7,
			rollbackTargetCount: 3, operationID: "operation-42",
		}
		view := NewSummaryScreen(ctx).View(40, 14)
		for _, want := range []string{"Plan:", "7 actions", "Operation:", "Rollback scope:", "[ENTER] Exit"} {
			if !strings.Contains(view, want) {
				t.Fatalf("40x14 outcome %q missing %q:\n%s", outcome, want, view)
			}
		}
		if outcome == installationOutcomeSucceeded {
			if !strings.Contains(view, "Installation Complete") || !strings.Contains(view, "dotfiles status") {
				t.Fatalf("40x14 success omitted verified title/guidance:\n%s", view)
			}
		} else {
			assertIncompleteInstallationSummary(t, view)
		}
		if got := len(strings.Split(view, "\n")); got > 14 {
			t.Fatalf("40x14 outcome %q rendered %d lines:\n%s", outcome, got, view)
		}
		assertRenderedWidth(t, view, 40)
	}
}

func TestErrorScreenActionsWrapAtSupportedSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{40, 14}, {60, 18}, {80, 24}} {
		ctx := newGoldenContext(t)
		view := NewErrorScreen(ctx, errors.New("installation failed after a long package-manager operation that needs plan review")).View(size.width, size.height)
		for _, want := range []string{"[R] Review Plan", "[S] Skip to Summary", "[Q] Quit"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d error screen missing %q:\n%s", size.width, size.height, want, view)
			}
		}
		if strings.Contains(view, "Installation Complete") {
			t.Fatalf("%dx%d error screen rendered false success:\n%s", size.width, size.height, view)
		}
		assertRenderedWidth(t, view, size.width)
		if got := len(strings.Split(view, "\n")); got > size.height {
			t.Fatalf("%dx%d error screen rendered %d lines:\n%s", size.width, size.height, got, view)
		}
	}
}

func TestFailedProgressRetainsReachedProgressWithoutSuccessFlash(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.width, ctx.app.height = 80, 24
	ctx.app.installComplete = true
	ctx.app.installOutcome = installationOutcomeFailed
	ctx.app.installPlannedSteps = 10
	ctx.app.installStep = 6
	ctx.app.installOutput = []string{"installer failed"}

	view := stripANSITest(NewProgressScreen(ctx).View(80, 24))
	for _, forbidden := range []string{"Installation Complete", "✓"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("failed progress rendered success marker %q:\n%s", forbidden, view)
		}
	}
	for _, want := range []string{"Installation Incomplete", "•", "✗", "░"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failed progress missing retained/failed marker %q:\n%s", want, view)
		}
	}
}

func TestInstallationSummaryUsesSealedAttemptFacts(t *testing.T) {
	ctx := newGoldenContext(t)
	ctx.app.installComplete = true
	ctx.app.installOutcome = installationOutcomeFailed
	ctx.app.installSummaryFacts = installationSummaryFacts{
		outcome: installationOutcomeFailed, planHash: "accepted-plan-hash", actionCount: 7,
		rollbackTargetCount: 3, operationID: "sealed-operation",
	}
	ctx.app.lastOperationID = "mutable-operation"
	ctx.app.pendingInstallPlan = nil

	view := NewSummaryScreen(ctx).View(80, 24)
	for _, want := range []string{"accepted-pla", "7 actions", "3 verified target", "sealed-operation"} {
		if !strings.Contains(view, want) {
			t.Fatalf("sealed summary missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "mutable-operation") {
		t.Fatalf("summary read mutable App identity instead of sealed facts:\n%s", view)
	}
}

func TestCurrentScreenIsPrefersManagerWithLegacyFallback(t *testing.T) {
	legacy := &App{screen: ScreenFileTree}
	if !legacy.currentScreenIs(ScreenFileTree) {
		t.Fatal("legacy screen fallback did not recognize FileTree")
	}

	ctx := newGoldenContext(t)
	ctx.app.screen = ScreenFileTree
	ctx.app.screenMgr.current = NewErrorScreen(ctx.app.screenMgr.Context(), errors.New("failed"))
	if ctx.app.currentScreenIs(ScreenFileTree) {
		t.Fatal("stale legacy screen overrode managed Error screen")
	}
	ctx.app.screenMgr.current = NewFileTreeScreen(ctx.app.screenMgr.Context())
	ctx.app.screen = ScreenWelcome
	if !ctx.app.currentScreenIs(ScreenFileTree) {
		t.Fatal("managed FileTree screen did not override stale legacy value")
	}
}

func assertIncompleteInstallationSummary(t *testing.T, view string) {
	t.Helper()
	if strings.Contains(view, "Installation Complete") || strings.Contains(view, "Next steps:") || strings.Contains(view, "dotfiles status") || strings.Contains(view, "source ~/.zshrc") {
		t.Fatalf("incomplete summary rendered success content:\n%s", view)
	}
	if !strings.Contains(view, "Installation Incomplete") {
		t.Fatalf("incomplete summary missing truthful title:\n%s", view)
	}
}

func assertRenderedWidth(t *testing.T, view string, width int) {
	t.Helper()
	for lineNumber, line := range strings.Split(stripANSITest(view), "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d width=%d exceeds %d: %q", lineNumber+1, got, width, line)
		}
	}
}
