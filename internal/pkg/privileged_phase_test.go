//go:build darwin || linux

package pkg

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/runner"
)

type privilegedPhaseCall struct {
	ctx    context.Context
	target string
	args   []string
}

func invokeManagerStream(manager PackageManager, ctx context.Context, kind string) (*runner.StreamingCmd, error) {
	switch kind {
	case "install":
		return manager.InstallStreaming(ctx, "git", "tmux")
	case "targeted":
		return manager.UpdateStreaming(ctx, "git", "tmux")
	case "all":
		return manager.UpdateAllStreaming(ctx)
	default:
		return nil, fmt.Errorf("unknown streaming test operation %q", kind)
	}
}

func awaitManagerPhase(t *testing.T, command *runner.StreamingCmd) (int, error) {
	t.Helper()
	if command == nil {
		t.Fatal("streaming command missing")
	}
	type result struct {
		lines int
		err   error
	}
	finished := make(chan result, 1)
	go func() {
		lines := 0
		for range command.Output {
			lines++
		}
		finished <- result{lines, command.Wait()}
	}()
	select {
	case got := <-finished:
		return got.lines, got.err
	case <-time.After(3 * time.Second):
		command.Cancel()
		t.Fatal("manager phase did not reach terminal completion")
		return 0, nil
	}
}

func TestAptPublicStreamingRoutesCapturedExecutableAndDrainsUpdate(t *testing.T) {
	for _, kind := range []string{"install", "targeted", "all"} {
		t.Run(kind, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "capture.log")
			t.Setenv("MANAGER_CAPTURE_LOG", logPath)
			captured := writeCapturedManagerExecutable(t, "apt")
			script, err := os.ReadFile(captured)
			if err != nil {
				t.Fatal(err)
			}
			script = append(script, []byte("if [ \"$1\" = update ]; then i=0; while [ \"$i\" -lt 120 ]; do printf 'phase-line-%s\\n' \"$i\"; i=$((i+1)); done; fi\n")...)
			if err := os.WriteFile(captured, script, 0o700); err != nil {
				t.Fatal(err)
			}
			manager := newAptManager(func(string) (string, error) { return captured, nil })
			t.Setenv("PATH", installHostileManagerPath(t, "apt", "sudo"))
			calls := make(chan privilegedPhaseCall, 2)
			manager.privilegedStreaming = func(ctx context.Context, target string, args ...string) (*runner.StreamingCmd, error) {
				calls <- privilegedPhaseCall{ctx, target, append([]string(nil), args...)}
				if target != captured {
					return nil, errors.New("factory refuses any non-fixture executable")
				}
				return runner.RunStreaming(ctx, target, args...)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command, err := invokeManagerStream(manager, ctx, kind)
			if err != nil {
				t.Fatal(err)
			}
			lines, err := awaitManagerPhase(t, command)
			if err != nil {
				t.Fatal(err)
			}
			want := [][]string{{"install", "-y", "git", "tmux"}}
			wantLines := 0
			if kind != "install" {
				wantLines = 120
				second := []string{"install", "-y", "git", "tmux"}
				if kind == "all" {
					second = []string{"upgrade", "-y"}
				}
				want = [][]string{{"update"}, second}
			}
			if lines != wantLines || len(calls) != len(want) {
				t.Fatalf("drained lines=%d calls=%d, want %d/%d", lines, len(calls), wantLines, len(want))
			}
			var logs []string
			for _, args := range want {
				call := <-calls
				if call.target != captured || !reflect.DeepEqual(call.args, args) {
					t.Fatalf("captured call=%+v want argv=%q", call, args)
				}
				line := "target:" + captured
				for _, arg := range args {
					line += " <" + arg + ">"
				}
				logs = append(logs, line)
			}
			requireManagerCaptureLog(t, logPath, logs...)
		})
	}
}

func TestAptPublicUpdateStopsAfterFirstPhaseFailureCancellationOrDrift(t *testing.T) {
	for _, kind := range []string{"targeted", "all"} {
		for _, failure := range []string{"start", "terminal", "cancel", "identity"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				captured := writeManagerExecutable(t, "apt")
				manager := newAptManager(func(string) (string, error) { return captured, nil })
				t.Setenv("PATH", installHostileManagerPath(t, "apt", "sudo"))
				calls := make(chan privilegedPhaseCall, 2)
				terminal := make(chan error, 1)
				failureErr := errors.New("first phase failed")
				manager.privilegedStreaming = func(ctx context.Context, target string, args ...string) (*runner.StreamingCmd, error) {
					calls <- privilegedPhaseCall{ctx, target, append([]string(nil), args...)}
					if failure == "start" {
						return nil, failureErr
					}
					output := make(chan string)
					close(output)
					return &runner.StreamingCmd{Output: output, Done: terminal}, nil
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				command, err := invokeManagerStream(manager, ctx, kind)
				if len(calls) != 1 {
					t.Fatalf("first phase starts=%d", len(calls))
				}
				first := <-calls
				if first.target != captured || !reflect.DeepEqual(first.args, []string{"update"}) {
					t.Fatalf("unexpected first phase=%+v", first)
				}
				want := failureErr
				if failure == "start" {
					if command != nil || !errors.Is(err, want) {
						t.Fatalf("start failure command=%v error=%v", command, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				switch failure {
				case "terminal":
					terminal <- failureErr
				case "cancel":
					command.Cancel()
					select {
					case <-first.ctx.Done():
					case <-time.After(time.Second):
						t.Fatal("cancellation did not reach active factory context")
					}
					select {
					case <-command.Done:
						t.Error("APT cancellation returned before terminal cleanup")
					case <-time.After(50 * time.Millisecond):
					}
					want = context.Canceled
					terminal <- context.Canceled
				case "identity":
					if err := os.WriteFile(captured, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
						t.Fatal(err)
					}
					want = errPackageManagerUnavailable
					terminal <- nil
				}
				close(terminal)
				if _, err := awaitManagerPhase(t, command); !errors.Is(err, want) || len(calls) != 0 {
					t.Fatalf("phase error=%v later starts=%d, want %v/no later starts", err, len(calls), want)
				}
			})
		}
	}
}

func TestPacmanAndParuPublicStreamingKeepCapturedRouting(t *testing.T) {
	for _, paru := range []bool{false, true} {
		for _, kind := range []string{"install", "targeted", "all"} {
			name := "pacman"
			if paru {
				name = "paru"
			}
			t.Run(name+"/"+kind, func(t *testing.T) {
				logPath := filepath.Join(t.TempDir(), "capture.log")
				t.Setenv("MANAGER_CAPTURE_LOG", logPath)
				captured := writeCapturedManagerExecutable(t, name)
				manager := newPacmanManager(paru, func(string) (string, error) { return captured, nil })
				t.Setenv("PATH", installHostileManagerPath(t, name, "sudo"))
				calls := make(chan privilegedPhaseCall, 1)
				manager.privilegedStreaming = func(ctx context.Context, target string, args ...string) (*runner.StreamingCmd, error) {
					calls <- privilegedPhaseCall{ctx, target, append([]string(nil), args...)}
					if paru || target != captured {
						return nil, errors.New("invalid privileged route")
					}
					return runner.RunStreaming(ctx, target, args...)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				command, err := invokeManagerStream(manager, ctx, kind)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := awaitManagerPhase(t, command); err != nil {
					t.Fatal(err)
				}
				args := []string{"-Syu", "--noconfirm"}
				if kind == "install" {
					args = []string{"-S", "--noconfirm", "--needed"}
				}
				if paru {
					args = append(args, "--skipreview", "--noprovides")
					if kind == "install" {
						args = append(args, "--removemake")
					}
				}
				if kind != "all" {
					args = append(args, "git", "tmux")
				}
				wantCalls := 1
				if paru {
					wantCalls = 0
				}
				if len(calls) != wantCalls {
					t.Fatalf("privileged calls=%d want=%d", len(calls), wantCalls)
				}
				if !paru {
					call := <-calls
					if call.target != captured || !reflect.DeepEqual(call.args, args) {
						t.Fatalf("privileged call=%+v want=%q", call, args)
					}
				}
				line := "target:" + captured
				for _, arg := range args {
					line += " <" + arg + ">"
				}
				requireManagerCaptureLog(t, logPath, line)
			})
		}
	}
}

func TestAPTPhaseClassifierRejectsUnboundFactory(t *testing.T) {
	for _, factory := range []string{"nil", "other.privilegedStreaming", "a.unreviewedFactory"} {
		expression, err := parser.ParseExpr(`runner.RunSequentialStreaming(ctx, aptStreamingPhase("first", identity, ` + factory + `, "update"), aptStreamingPhase("second", identity, a.privilegedStreaming, "upgrade", "-y"))`)
		if err != nil {
			t.Fatal(err)
		}
		call := expression.(*ast.CallExpr)
		if exactSequentialStreamingPhases(call.Args[1:], "aptStreamingPhase", "identity", "a") {
			t.Fatalf("phase classifier accepted factory %s", factory)
		}
	}
}
