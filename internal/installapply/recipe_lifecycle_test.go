package installapply

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
)

func TestStreamingCancellationWaitsForTerminalAndPreservesCleanupError(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(context.Context, *runner.StreamingCmd) error
	}{
		{"output", func(ctx context.Context, command *runner.StreamingCmd) error { return streamOutput(ctx, command, nil) }},
		{"completion", waitStreaming},
	} {
		for _, deadline := range []bool{false, true} {
			name := test.name + "/cancel"
			if deadline {
				name = test.name + "/deadline"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				if deadline {
					cancel()
					ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				}
				defer cancel()
				cancel()
				output, done := make(chan string), make(chan error, 1)
				command := &runner.StreamingCmd{Output: output, Done: done}
				returned := make(chan error, 1)
				go func() { returned <- test.run(ctx, command) }()
				var result error
				early := false
				select {
				case result = <-returned:
					early = true
					t.Error("cancellation returned before terminal completion")
				case <-time.After(50 * time.Millisecond):
				}
				cleanupErr := errors.New("terminal cleanup failure")
				close(output)
				done <- cleanupErr
				close(done)
				if !early {
					select {
					case result = <-returned:
					case <-time.After(time.Second):
						t.Fatal("did not return after terminal completion")
					}
				}
				if !errors.Is(result, ctx.Err()) || !errors.Is(result, cleanupErr) {
					t.Fatalf("result=%v, want cancellation %v and cleanup failure", result, ctx.Err())
				}
			})
		}
	}
}

// This adapter honors the streaming contract: cancellation starts cleanup,
// while terminal publication waits for the test to finish that cleanup.
type gatedRecipeManager struct {
	*pkg.MockPackageManager
	started      chan struct{}
	cancelled    chan struct{}
	finish       chan struct{}
	finishOnce   sync.Once
	outputClosed bool
	starts       int
}

func newGatedRecipeManager(manager *pkg.MockPackageManager, outputClosed bool) *gatedRecipeManager {
	return &gatedRecipeManager{MockPackageManager: manager, started: make(chan struct{}), cancelled: make(chan struct{}), finish: make(chan struct{}), outputClosed: outputClosed}
}

func (manager *gatedRecipeManager) complete() {
	manager.finishOnce.Do(func() { close(manager.finish) })
}

func (manager *gatedRecipeManager) InstallStreaming(ctx context.Context, _ ...string) (*runner.StreamingCmd, error) {
	manager.starts++
	output, done := make(chan string), make(chan error, 1)
	if manager.outputClosed {
		close(output)
	}
	close(manager.started)
	go func() {
		<-ctx.Done()
		close(manager.cancelled)
		<-manager.finish
		if !manager.outputClosed {
			close(output)
		}
		done <- ctx.Err()
		close(done)
	}()
	return &runner.StreamingCmd{Output: output, Done: done}, nil
}

func (manager *gatedRecipeManager) InstallCasksStreaming(ctx context.Context, _ ...string) (*runner.StreamingCmd, error) {
	return manager.InstallStreaming(ctx)
}

func TestExecuteRecipeCancellationWaitsBeforeReturningOrStartingNextStep(t *testing.T) {
	for _, cask := range []bool{false, true} {
		name := "package"
		if cask {
			name = "cask"
		}
		t.Run(name, func(t *testing.T) {
			manager := newGatedRecipeManager(pkg.NewMockPackageManager(), cask)
			manager.ManagerName = "brew"
			identity, _ := applyManagerIdentity(t, "gated-brew", "exit 0")
			if err := manager.SetExecutableIdentity(identity); err != nil {
				t.Fatal(err)
			}
			step := operation.InstallStep{Kind: operation.InstallStepPackageManager, Provider: "brew", Packages: []string{"node"}}
			if cask {
				step = operation.InstallStep{Kind: operation.InstallStepHomebrewCask, Provider: "brew", Casks: []string{"ghostty"}}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer manager.complete()
			returned := make(chan error, 1)
			go func() { returned <- ExecuteRecipe(ctx, reviewedExecutionRecipe(step, step), manager, identity, nil) }()
			select {
			case <-manager.started:
			case err := <-returned:
				t.Fatalf("recipe returned before starting: %v", err)
			case <-time.After(time.Second):
				t.Fatal("recipe did not start")
			}
			cancel()
			<-manager.cancelled
			select {
			case err := <-returned:
				t.Fatalf("recipe returned before cleanup completed: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			manager.complete()
			select {
			case err := <-returned:
				if !errors.Is(err, context.Canceled) || manager.starts != 1 {
					t.Fatalf("result=%v starts=%d", err, manager.starts)
				}
			case <-time.After(time.Second):
				t.Fatal("recipe did not finish after cleanup")
			}
		})
	}
}
