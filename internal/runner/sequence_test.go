//go:build darwin || linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func completedSequenceCommand(lines int, terminal error) *StreamingCmd {
	output, done := make(chan string, lines), make(chan error, 1)
	for i := range lines {
		output <- fmt.Sprint(i)
	}
	close(output)
	done <- terminal
	close(done)
	return &StreamingCmd{Output: output, Done: done}
}

func waitSequenceResult(t *testing.T, command *StreamingCmd) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("sequence failed to finish")
		return nil
	}
}

func TestSequenceWaitsForTerminalBeforeNextPhase(t *testing.T) {
	output, terminal := make(chan string), make(chan error, 1)
	close(output)
	second := make(chan struct{}, 1)
	command, err := RunSequentialStreaming(context.Background(),
		SequentialStreamingPhase{Name: "update", Start: func(context.Context) (*StreamingCmd, error) {
			return &StreamingCmd{Output: output, Done: terminal}, nil
		}},
		SequentialStreamingPhase{Name: "install", Start: func(context.Context) (*StreamingCmd, error) {
			second <- struct{}{}
			return completedSequenceCommand(0, nil), nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-second:
		t.Error("next phase started before first terminal completion")
	case <-time.After(50 * time.Millisecond):
	}
	terminal <- nil
	close(terminal)
	if err := waitSequenceResult(t, command); err != nil || len(second) != 1 {
		t.Fatalf("terminal result=%v next phases=%d", err, len(second))
	}
}

func TestSequenceCancellationWaitsForCleanupAndStopsLaterPhases(t *testing.T) {
	output, terminal := make(chan string), make(chan error, 1)
	close(output)
	cancelled := make(chan struct{})
	var once sync.Once
	second := make(chan struct{}, 1)
	command, err := RunSequentialStreaming(context.Background(),
		SequentialStreamingPhase{Name: "update", Start: func(context.Context) (*StreamingCmd, error) {
			return &StreamingCmd{Output: output, Done: terminal, cancel: func() { once.Do(func() { close(cancelled) }) }}, nil
		}},
		SequentialStreamingPhase{Name: "install", Start: func(context.Context) (*StreamingCmd, error) {
			second <- struct{}{}
			return completedSequenceCommand(0, nil), nil
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	command.Cancel()
	command.Cancel()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("active phase was not cancelled")
	}
	select {
	case <-command.Done:
		t.Error("sequence reported completion before phase cleanup")
	case <-time.After(50 * time.Millisecond):
	}
	cleanupErr := errors.New("phase cleanup failure")
	terminal <- cleanupErr
	close(terminal)
	for range 2 {
		if err := waitSequenceResult(t, command); !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupErr) || len(second) != 0 {
			t.Fatalf("cancellation=%v later phases=%d", err, len(second))
		}
	}
}

func TestSequenceStartAndTerminalFailuresStopLaterPhases(t *testing.T) {
	for _, failure := range []string{"pre-cancel", "first-start", "first-terminal", "second-start", "overflow"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "pre-cancel" {
				cancel()
			}
			failureErr := errors.New("phase failure")
			starts := make(chan string, 3)
			phases := make([]SequentialStreamingPhase, 0, 3)
			for _, name := range []string{"first", "second", "third"} {
				phases = append(phases, SequentialStreamingPhase{Name: name, Start: func(context.Context) (*StreamingCmd, error) {
					starts <- name
					if failure == name+"-start" {
						return nil, failureErr
					}
					if failure == "first-terminal" && name == "first" {
						return completedSequenceCommand(0, failureErr), nil
					}
					lines := 0
					if failure == "overflow" {
						lines = 2 * streamingOutputCapacity
					}
					return completedSequenceCommand(lines, nil), nil
				}})
			}
			command, err := RunSequentialStreaming(ctx, phases...)
			if command != nil {
				err = waitSequenceResult(t, command)
			}
			want, count := failureErr, 1
			switch failure {
			case "pre-cancel":
				want, count = context.Canceled, 0
			case "second-start":
				count = 2
			case "overflow":
				want = errStreamingOutputOverflow
			}
			if !errors.Is(err, want) || len(starts) != count {
				t.Fatalf("failure=%v started=%d, want %v/%d", err, len(starts), want, count)
			}
		})
	}
}

func TestSequenceContinuouslyDrainsVerbosePhase(t *testing.T) {
	ack := make(chan struct{})
	command, err := RunSequentialStreaming(context.Background(), SequentialStreamingPhase{Name: "verbose update", Start: func(context.Context) (*StreamingCmd, error) {
		output, terminal := make(chan string), make(chan error, 1)
		go func() {
			for i := range 400 {
				output <- fmt.Sprintf("line %d", i)
				<-ack
			}
			close(output)
			terminal <- nil
			close(terminal)
		}()
		return &StreamingCmd{Output: output, Done: terminal}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for line := range command.Output {
		if !strings.HasPrefix(line, "line ") {
			t.Errorf("unexpected forwarded output %q", line)
		}
		count++
		ack <- struct{}{}
	}
	if err := waitSequenceResult(t, command); err != nil || count != 400 {
		t.Fatalf("verbose output=%d error=%v", count, err)
	}
}
