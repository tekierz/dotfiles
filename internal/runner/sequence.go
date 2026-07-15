//go:build darwin || linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var errInvalidStreamingSequence = errors.New("invalid streaming sequence")

// SequentialStreamingPhase is one ordered process phase. Start must use one of
// the existing streaming adapters so process lifecycle and, where applicable,
// privileged-supervisor authority remain owned by those adapters.
type SequentialStreamingPhase struct {
	Name  string
	Start func(context.Context) (*StreamingCmd, error)
}

// RunSequentialStreaming starts ordered streaming phases behind one facade.
// Each active phase is continuously drained before the next phase may start.
// A phase failure, cancellation, or undrained facade prevents every later phase.
func RunSequentialStreaming(ctx context.Context, phases ...SequentialStreamingPhase) (*StreamingCmd, error) {
	if ctx == nil || len(phases) == 0 {
		return nil, errInvalidStreamingSequence
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	sequence := append([]SequentialStreamingPhase(nil), phases...)
	for _, phase := range sequence {
		if phase.Name == "" || phase.Start == nil {
			return nil, errInvalidStreamingSequence
		}
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	first, err := sequence[0].Start(runCtx)
	if err != nil {
		cancel(err)
		return nil, fmt.Errorf("%s: %w", sequence[0].Name, err)
	}
	if !validSequentialStreamingCommand(first) {
		if first != nil {
			first.Cancel()
		}
		cancel(errInvalidStreamingSequence)
		return nil, fmt.Errorf("%s: %w", sequence[0].Name, errInvalidStreamingSequence)
	}
	output := make(chan string, streamingOutputCapacity)
	done := make(chan error, 1)
	finished := make(chan struct{})
	var result error
	var resultMu sync.RWMutex

	wait := func() error {
		<-finished
		resultMu.RLock()
		defer resultMu.RUnlock()
		return result
	}
	go func() {
		defer close(output)
		defer close(finished)
		defer cancel(nil)
		terminal := runStreamingSequence(runCtx, cancel, output, sequence, first)
		resultMu.Lock()
		result = terminal
		resultMu.Unlock()
		done <- terminal
		close(done)
	}()

	return &StreamingCmd{
		Output: output,
		Done:   done,
		cancel: func() { cancel(context.Canceled) },
		wait:   wait,
	}, nil
}

func runStreamingSequence(ctx context.Context, cancel context.CancelCauseFunc, output chan<- string, phases []SequentialStreamingPhase, first *StreamingCmd) error {
	for index, phase := range phases {
		command := first
		if index != 0 {
			if err := context.Cause(ctx); err != nil {
				return err
			}
			var err error
			command, err = phase.Start(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", phase.Name, err)
			}
		}
		if !validSequentialStreamingCommand(command) {
			if command != nil {
				command.Cancel()
			}
			return fmt.Errorf("%s: %w", phase.Name, errInvalidStreamingSequence)
		}
		if err := drainStreamingPhase(ctx, cancel, output, command); err != nil {
			return fmt.Errorf("%s: %w", phase.Name, err)
		}
	}
	return nil
}

func validSequentialStreamingCommand(command *StreamingCmd) bool {
	return command != nil && command.Output != nil && command.Done != nil
}

func drainStreamingPhase(ctx context.Context, cancel context.CancelCauseFunc, output chan<- string, command *StreamingCmd) error {
	phaseOutput, phaseDone := command.Output, command.Done
	ctxDone := ctx.Done()
	var terminal error
	var forwardingErr error
	for phaseOutput != nil || phaseDone != nil {
		select {
		case line, ok := <-phaseOutput:
			if !ok {
				phaseOutput = nil
				continue
			}
			if forwardingErr != nil {
				continue
			}
			select {
			case output <- line:
			case <-ctx.Done():
				forwardingErr = context.Cause(ctx)
				command.Cancel()
			default:
				forwardingErr = errStreamingOutputOverflow
				cancel(forwardingErr)
				command.Cancel()
			}
		case err, ok := <-phaseDone:
			if ok {
				terminal = err
			}
			phaseDone = nil
		case <-ctxDone:
			if forwardingErr == nil {
				forwardingErr = context.Cause(ctx)
				command.Cancel()
			}
			ctxDone = nil
		}
	}
	return joinStreamingErrors(forwardingErr, terminal)
}
