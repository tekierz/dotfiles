package ui

// installationOutcome is the closed lifecycle for one reviewed installation
// attempt. Its zero value is deliberately pending so an unset or stale value
// can never be interpreted as success.
type installationOutcome uint8

const (
	installationOutcomePending installationOutcome = iota
	installationOutcomeRunning
	installationOutcomeSucceeded
	installationOutcomeFailed
)

func (o installationOutcome) String() string {
	switch o {
	case installationOutcomePending:
		return "pending"
	case installationOutcomeRunning:
		return "running"
	case installationOutcomeSucceeded:
		return "succeeded"
	case installationOutcomeFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// installationSummaryFacts is the attempt-scoped summary snapshot. Plan facts
// are copied before execution and the terminal outcome/operation ID are sealed
// before any post-install cache refresh can discard or replace the reviewed
// pendingInstallPlan.
type installationSummaryFacts struct {
	outcome             installationOutcome
	planHash            string
	actionCount         int
	rollbackTargetCount int
	operationID         string
}

func installationFactsForPlan(plan *installPlan) installationSummaryFacts {
	facts := installationSummaryFacts{outcome: installationOutcomeRunning}
	if plan == nil {
		return facts
	}
	facts.planHash = plan.hash()
	facts.actionCount = len(plan.actions())
	facts.rollbackTargetCount = len(plan.backupTargets())
	return facts
}

// prepareInstallationReview clears all attempt identity and invalidates the old
// plan so Retry must collect/review fresh evidence before another mutation.
func (a *App) prepareInstallationReview() {
	if a == nil {
		return
	}
	a.installRunning = false
	a.installComplete = false
	a.installOutcome = installationOutcomePending
	a.installSummaryFacts = installationSummaryFacts{}
	a.lastError = nil
	a.lastOperationID = ""
	a.invalidatePendingInstallPlan()
}

// stageInstallationAttempt clears stale identity while retaining the reviewed
// plan. This is used before sudo authentication, which can fail before
// startInstallation has a chance to initialize the running attempt.
func (a *App) stageInstallationAttempt(plan *installPlan) {
	if a == nil {
		return
	}
	a.installRunning = false
	a.installComplete = false
	a.installOutcome = installationOutcomePending
	a.installSummaryFacts = installationFactsForPlan(plan)
	a.installSummaryFacts.outcome = installationOutcomePending
	a.lastError = nil
	a.lastOperationID = ""
}

func (a *App) beginInstallationAttempt(plan *installPlan) {
	a.installOutcome = installationOutcomeRunning
	a.installSummaryFacts = installationFactsForPlan(plan)
}

// finishInstallationAttempt seals the immutable facts consumed by Summary.
// It may be called before startInstallation (for example a failed sudo prompt),
// so it falls back to the currently reviewed plan only when no plan facts were
// captured at start.
func (a *App) finishInstallationAttempt(outcome installationOutcome) {
	if a == nil {
		return
	}
	a.installRunning = false
	a.installComplete = true
	a.installOutcome = outcome
	facts := a.installSummaryFacts
	if facts.planHash == "" && facts.actionCount == 0 && facts.rollbackTargetCount == 0 {
		facts = installationFactsForPlan(a.pendingInstallPlan)
	}
	facts.outcome = outcome
	facts.operationID = a.lastOperationID
	a.installSummaryFacts = facts
}
