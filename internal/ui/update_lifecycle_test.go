package ui

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
)

type lifecycleUpdateManager struct {
	*pkg.MockPackageManager
	start func(context.Context) (*runner.StreamingCmd, error)
}

func (m lifecycleUpdateManager) UpdateStreaming(ctx context.Context, _ ...string) (*runner.StreamingCmd, error) {
	return m.start(ctx)
}

func (m lifecycleUpdateManager) UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error) {
	return m.start(ctx)
}

func TestUpdateWorkersCancellationWaitsForTerminal(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, outputMode := range []string{"blocked-forward", "silent", "closed"} {
			name := "targeted/" + outputMode
			if all {
				name = "all/" + outputMode
			}
			t.Run(name, func(t *testing.T) {
				output := make(chan string)
				done := make(chan error, 1)
				started := make(chan context.Context, 1)
				cleanupErr := errors.New("cleanup failed")
				var closeOutput sync.Once
				closeLines := func() { closeOutput.Do(func() { close(output) }) }
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { closeLines(); done <- cleanupErr; close(done) }) }
				defer release()
				manager := lifecycleUpdateManager{MockPackageManager: pkg.NewMockPackageManager(), start: func(ctx context.Context) (*runner.StreamingCmd, error) {
					started <- ctx
					return &runner.StreamingCmd{Output: output, Done: done}, nil
				}}
				a := &App{}
				if all {
					a.streamingUpdateAllCmdWithManager(manager)
				} else {
					a.streamingUpdateCmdWithResolver([]pkg.Package{{Name: "one"}}, func(pkg.ExecutionProvider) pkg.PackageManager { return manager })
				}
				defer a.streamCancel()
				ctx := <-started
				switch outputMode {
				case "blocked-forward":
					// An unbuffered 65th send proves the worker filled its 64-line queue
					// and is forwarding the next line when cancellation arrives.
					for range 65 {
						output <- "progress"
					}
				case "closed":
					closeLines()
				}
				a.streamCancel()
				<-ctx.Done()
				terminal := make(chan updateStreamMsg, 1)
				closed := make(chan struct{})
				go func() {
					defer close(closed)
					for msg := range a.updateStream {
						if msg.done {
							terminal <- msg
						}
					}
				}()
				select {
				case <-closed:
					t.Error("worker closed before terminal cleanup completed")
				case <-time.After(40 * time.Millisecond):
				}
				release()
				select {
				case <-closed:
				case <-time.After(2 * time.Second):
					t.Fatal("worker did not finish after terminal cleanup")
				}
				select {
				case msg := <-terminal:
					if !errors.Is(msg.err, context.Canceled) || !errors.Is(msg.err, cleanupErr) {
						t.Errorf("terminal error lost cancellation or cleanup: %v", msg.err)
					}
					if !all && (len(msg.results) != 1 || msg.results[0].Package.Name != "one" || msg.results[0].Success || !errors.Is(msg.results[0].Error, context.Canceled)) {
						t.Errorf("cancelled package result: %+v", msg.results)
					}
				default:
					t.Error("worker dropped terminal cancellation event")
				}
			})
		}
	}
}

func TestUpdateWorkerCancellationStopsLaterProvider(t *testing.T) {
	packages, _ := providerExecutableFixture(t) // Discovery executes only read-only fixture scripts.
	started := make(chan context.Context, 1)
	output := make(chan string)
	done := make(chan error, 1)
	var calls atomic.Int32
	manager := lifecycleUpdateManager{MockPackageManager: pkg.NewMockPackageManager(), start: func(ctx context.Context) (*runner.StreamingCmd, error) {
		calls.Add(1)
		started <- ctx
		return &runner.StreamingCmd{Output: output, Done: done}, nil
	}}
	a := &App{}
	a.streamingUpdateCmdWithResolver(packages, func(pkg.ExecutionProvider) pkg.PackageManager { return manager })
	ctx := <-started
	a.streamCancel()
	<-ctx.Done()
	close(output)
	done <- context.Canceled
	close(done)
	var terminal updateStreamMsg
	for msg := range a.updateStream {
		if msg.done {
			terminal = msg
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("started %d providers after cancellation", calls.Load())
	}
	if len(terminal.results) != len(packages) || !errors.Is(terminal.err, context.Canceled) {
		t.Fatalf("incomplete cancellation result: %+v", terminal)
	}
	for _, result := range terminal.results {
		if result.Success || !errors.Is(result.Error, context.Canceled) {
			t.Errorf("cancelled result: %+v", result)
		}
	}
}

func TestUpdateCompletionRetainedWithoutConsumer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := make(chan updateStreamMsg, 64)
	for range cap(stream) {
		stream <- updateStreamMsg{line: "old progress"}
	}
	published := make(chan struct{})
	go func() {
		publishUpdateCompletion(ctx, stream, updateStreamMsg{done: true, err: ctx.Err()})
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("cancelled completion blocked on abandoned progress queue")
	}
	for range cap(stream) - 1 {
		if msg := <-stream; msg.done {
			t.Fatal("terminal event displaced remaining progress")
		}
	}
	if msg := <-stream; !msg.done || !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("terminal result lost: %+v", msg)
	}
}

func TestUpdateWorkerSuccessDrainsOutput(t *testing.T) {
	output := make(chan string, 3)
	for _, line := range []string{"first", "second", "third"} {
		output <- line
	}
	close(output)
	done := make(chan error, 1)
	done <- nil
	close(done)
	manager := lifecycleUpdateManager{MockPackageManager: pkg.NewMockPackageManager(), start: func(context.Context) (*runner.StreamingCmd, error) {
		return &runner.StreamingCmd{Output: output, Done: done}, nil
	}}
	a := &App{}
	a.streamingUpdateAllCmdWithManager(manager)
	defer a.streamCancel()
	var lines []string
	var terminal bool
	for msg := range a.updateStream {
		if msg.done {
			terminal = true
			if msg.err != nil {
				t.Fatal(msg.err)
			}
		} else {
			lines = append(lines, msg.line)
		}
	}
	if !terminal || len(lines) != 3 || lines[0] != "first" || lines[1] != "second" || lines[2] != "third" {
		t.Fatalf("stream order: %v, terminal=%v", lines, terminal)
	}
}
