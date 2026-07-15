package operation

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const dogfoodTraceEnv = "DOTFILES_DOGFOOD_TRACE"

type TracePhase string

const (
	TraceInstall TracePhase = "install"
	TraceUpdate  TracePhase = "update"
	TraceRestore TracePhase = "restore"
)

type TraceOutcome string

const (
	TraceRunning   TraceOutcome = "running"
	TraceSucceeded TraceOutcome = "succeeded"
	TracePartial   TraceOutcome = "partial"
	TraceFailed    TraceOutcome = "failed"
	TraceCancelled TraceOutcome = "cancelled"
)

type TraceCounts struct {
	Attempted int
	Succeeded int
	Failed    int
	Skipped   int
	Warnings  int
}

var traceMu sync.Mutex

// Trace records one opt-in, fixed-field dogfood event. Callers cannot attach
// paths, command output, environment values, credentials, or file contents.
func Trace(phase TracePhase, outcome TraceOutcome, counts TraceCounts) {
	if os.Getenv(dogfoodTraceEnv) != "1" || !validTracePhase(phase) || !validTraceOutcome(outcome) {
		return
	}
	counts.Attempted = boundedTraceCount(counts.Attempted)
	counts.Succeeded = boundedTraceCount(counts.Succeeded)
	counts.Failed = boundedTraceCount(counts.Failed)
	counts.Skipped = boundedTraceCount(counts.Skipped)
	counts.Warnings = boundedTraceCount(counts.Warnings)
	traceMu.Lock()
	defer traceMu.Unlock()
	_, _ = fmt.Fprintf(os.Stderr,
		"dotfiles-trace time=%s phase=%s outcome=%s attempted=%d succeeded=%d failed=%d skipped=%d warnings=%d\n",
		time.Now().UTC().Format("2006-01-02T15:04:05Z"), phase, outcome,
		counts.Attempted, counts.Succeeded, counts.Failed, counts.Skipped, counts.Warnings)
}

func validTracePhase(phase TracePhase) bool {
	return phase == TraceInstall || phase == TraceUpdate || phase == TraceRestore
}

func validTraceOutcome(outcome TraceOutcome) bool {
	return outcome == TraceRunning || outcome == TraceSucceeded || outcome == TracePartial || outcome == TraceFailed || outcome == TraceCancelled
}

func boundedTraceCount(count int) int {
	if count < 0 {
		return 0
	}
	const maximum = 999999
	if count > maximum {
		return maximum
	}
	return count
}
