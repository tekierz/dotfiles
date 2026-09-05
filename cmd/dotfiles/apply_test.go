package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/installapply"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

const applyTestOperationID = "20260712T220000.000000000Z-0123456789abcdef"

func TestApplyCommandGrammarAndLocalFlags(t *testing.T) {
	command := newApplyCommand(applyCommandRuntime{})
	if command.Use != "apply" || command.Args == nil {
		t.Fatalf("identity=%q args=%v", command.Use, command.Args)
	}
	for name, defaultValue := range map[string]string{"yes": "false", "plan-hash": "", "tool": "[]"} {
		flag := command.Flags().Lookup(name)
		if flag == nil || flag.DefValue != defaultValue {
			t.Fatalf("flag %s=%#v", name, flag)
		}
		if command.InheritedFlags().Lookup(name) != nil {
			t.Fatalf("flag %s escaped apply command", name)
		}
	}
	if command.Flags().Lookup("json") != nil {
		t.Fatal("apply unexpectedly accepts JSON mode")
	}
}

func TestApplyCommandCanonicalRequestAndSingleWriteSuccess(t *testing.T) {
	hash := strings.Repeat("a", 64)
	writes := &planTestWriter{}
	applyCalls, signalCalls, stopCalls := 0, 0, 0
	command := newApplyCommand(applyCommandRuntime{
		apply: func(ctx context.Context, request installapply.Request) (installapply.Result, error) {
			applyCalls++
			if ctx == nil || !reflect.DeepEqual(request.RawTools, []string{"git", "zsh"}) || request.ExpectedHash != hash {
				t.Fatalf("request=%+v ctx=%v", request, ctx)
			}
			return installapply.Result{
				OperationID: applyTestOperationID,
				PlanHash:    hash,
				Status:      operation.StatusSucceeded,
				Succeeded:   2,
				Next:        installapply.NextComplete,
			}, nil
		},
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			signalCalls++
			return parent, func() { stopCalls++ }
		},
	})
	command.SetOut(writes)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--yes", "--plan-hash", hash, "--tool", " zsh ", "--tool", "git", "--tool", "git"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "installation applied: operation=" + applyTestOperationID + " plan_hash=" + hash + " succeeded=2 failed=0\n"
	if writes.writes != 1 || writes.String() != want || applyCalls != 1 || signalCalls != 1 || stopCalls != 1 {
		t.Fatalf("output=%q writes=%d calls apply/signal/stop=%d/%d/%d", writes.String(), writes.writes, applyCalls, signalCalls, stopCalls)
	}
}

func TestApplyCommandSyntaxStopsBeforeRuntime(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, args := range [][]string{
		{"--plan-hash", hash, "--tool", "git"},
		{"--yes=false", "--plan-hash", hash, "--tool", "git"},
		{"--yes", "--tool", "git"},
		{"--yes", "--plan-hash", "ABC", "--tool", "git"},
		{"--yes", "--plan-hash", hash},
		{"--yes", "--plan-hash", hash, "--tool", "Git"},
		{"--yes", "--plan-hash", hash, "--tool", "git", "positional"},
		{"--yes", "--plan-hash", hash, "--tool", "git", "--json"},
	} {
		command := newApplyCommand(applyCommandRuntime{
			apply: func(context.Context, installapply.Request) (installapply.Result, error) {
				t.Fatalf("runtime called for %v", args)
				return installapply.Result{}, nil
			},
			signalContext: func(context.Context) (context.Context, context.CancelFunc) {
				t.Fatalf("signal seam called for %v", args)
				return nil, nil
			},
		})
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		command.SetArgs(args)
		assertApplyExit(t, command.Execute(), 2, applySyntaxMessage)
	}
}

func TestApplyCommandErrorExitAndRedactionMapping(t *testing.T) {
	hash := strings.Repeat("a", 64)
	tests := []struct {
		name    string
		err     error
		result  installapply.Result
		code    int
		message string
	}{
		{name: "hash", err: errors.Join(installapply.ErrPlanHashMismatch, errors.New("/Users/private token=SECRET")), code: 2, message: applyHashMessage},
		{name: "not ready", err: errors.Join(installapply.ErrPlanNotReady, errors.New("/Users/private token=SECRET")), code: 2, message: applyNotReadyMessage},
		{name: "invalid", err: errors.Join(installapply.ErrInvalidApplyRequest, errors.New("/Users/private token=SECRET")), code: 2, message: applySyntaxMessage},
		{name: "cancelled", err: errors.Join(installapply.ErrApplyFailed, context.Canceled, errors.New("/Users/private token=SECRET")), code: 130, message: applyCancelledMessage},
		{name: "deadline", err: context.DeadlineExceeded, code: 1, message: applyFailedMessage},
		{name: "failure", err: errors.Join(installapply.ErrApplyFailed, errors.New("/Users/private token=SECRET")), code: 1, message: applyFailedMessage},
		{name: "invalid success", result: installapply.Result{OperationID: "bad\nSECRET", PlanHash: hash, Status: operation.StatusSucceeded, Succeeded: 1, Next: installapply.NextComplete}, code: 1, message: applyFailedMessage},
		{name: "unicode operation id", result: installapply.Result{OperationID: "operation-α", PlanHash: hash, Status: operation.StatusSucceeded, Succeeded: 1, Next: installapply.NextComplete}, code: 1, message: applyFailedMessage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stops := 0
			command := newApplyCommand(applyCommandRuntime{
				apply:         func(context.Context, installapply.Request) (installapply.Result, error) { return tc.result, tc.err },
				signalContext: func(parent context.Context) (context.Context, context.CancelFunc) { return parent, func() { stops++ } },
			})
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(io.Discard)
			command.SetArgs([]string{"--yes", "--plan-hash", hash, "--tool", "git"})
			err := command.Execute()
			assertApplyExit(t, err, tc.code, tc.message)
			if output.Len() != 0 || stops != 1 || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "/Users") {
				t.Fatalf("output=%q stops=%d error=%q", output.String(), stops, err)
			}
		})
	}
}

func TestApplyCommandPropagatesCancelledSignalContext(t *testing.T) {
	hash := strings.Repeat("a", 64)
	applyCalls, stops := 0, 0
	command := newApplyCommand(applyCommandRuntime{
		apply: func(ctx context.Context, _ installapply.Request) (installapply.Result, error) {
			applyCalls++
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("apply context error=%v", ctx.Err())
			}
			return installapply.Result{}, ctx.Err()
		},
		signalContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			cancel()
			return ctx, func() { stops++ }
		},
	})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--yes", "--plan-hash", hash, "--tool", "git"})
	assertApplyExit(t, command.Execute(), 130, applyCancelledMessage)
	if applyCalls != 1 || stops != 1 {
		t.Fatalf("apply calls=%d stop calls=%d", applyCalls, stops)
	}
}

func TestApplyCommandSignalStopRunsWhenContextSeamFails(t *testing.T) {
	hash := strings.Repeat("a", 64)
	stops := 0
	command := newApplyCommand(applyCommandRuntime{
		apply: func(context.Context, installapply.Request) (installapply.Result, error) {
			t.Fatal("apply called with nil signal context")
			return installapply.Result{}, nil
		},
		signalContext: func(context.Context) (context.Context, context.CancelFunc) {
			return nil, func() { stops++ }
		},
	})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--yes", "--plan-hash", hash, "--tool", "git"})
	assertApplyExit(t, command.Execute(), 1, applyFailedMessage)
	if stops != 1 {
		t.Fatalf("signal stop calls=%d want=1", stops)
	}
}

func TestApplyCommandSuccessWriterFailuresUsePostMutationMessage(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, writer := range []*planTestWriter{{short: true}, {err: errors.New("SECRET writer failure")}} {
		command := newApplyCommand(applyCommandRuntime{
			apply: func(context.Context, installapply.Request) (installapply.Result, error) {
				return installapply.Result{
					OperationID: applyTestOperationID,
					PlanHash:    hash,
					Status:      operation.StatusSucceeded,
					Succeeded:   1,
					Next:        installapply.NextComplete,
				}, nil
			},
			signalContext: func(parent context.Context) (context.Context, context.CancelFunc) { return context.WithCancel(parent) },
		})
		command.SetOut(writer)
		command.SetErr(io.Discard)
		command.SetArgs([]string{"--yes", "--plan-hash", hash, "--tool", "git"})
		assertApplyExit(t, command.Execute(), 1, applyOutputFailedMessage)
		if writer.writes != 1 {
			t.Fatalf("writer calls=%d", writer.writes)
		}
	}
}

func TestDefaultApplyRuntimeFreshPlanHashMismatchCreatesNoState(t *testing.T) {
	runtime, calls := planRuntimeFixture(t, health.PresenceMissing)
	previous := planRuntime
	planRuntime = runtime
	t.Cleanup(func() { planRuntime = previous })
	home := os.Getenv("HOME")
	stateDir := filepath.Join(home, ".local", "state", "dotfiles")
	result, err := defaultApplyCommandRuntime().apply(context.Background(), installapply.Request{RawTools: []string{"zsh"}, ExpectedHash: strings.Repeat("f", 64)})
	if !errors.Is(err, installapply.ErrPlanHashMismatch) || result != (installapply.Result{}) {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if _, statErr := os.Lstat(stateDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("hash mismatch mutated state path %s: %v", stateDir, statErr)
	}
	if calls.registry != 1 || calls.platform != 1 || calls.manager != 1 || calls.collect != 1 || calls.describe != 1 || calls.capture != 1 || calls.clock != 1 ||
		calls.generation != 1 || calls.collectedPlatform != pkg.PlatformMacOS || calls.collectedManager != "brew" {
		t.Fatalf("fresh apply calls=%+v", calls)
	}
}

func TestRegisteredApplyCommandAndRootExitContract(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"apply"})
	if err != nil || found != applyCmd {
		t.Fatalf("registered=%p want=%p err=%v", found, applyCmd, err)
	}
	hash := strings.Repeat("a", 64)
	tests := []struct {
		name   string
		args   []string
		result installapply.Result
		err    error
		code   int
		stdout string
		stderr string
	}{
		{name: "success", args: []string{"apply", "--yes", "--plan-hash", hash, "--tool", "git"}, result: installapply.Result{OperationID: applyTestOperationID, PlanHash: hash, Status: operation.StatusSucceeded, Succeeded: 1, Next: installapply.NextComplete}, code: 0, stdout: "installation applied: operation=" + applyTestOperationID + " plan_hash=" + hash + " succeeded=1 failed=0\n"},
		{name: "syntax", args: []string{"apply", "--plan-hash", hash, "--tool", "git"}, code: 2, stderr: applySyntaxMessage + "\n"},
		{name: "hash", args: []string{"apply", "--yes", "--plan-hash", hash, "--tool", "git"}, err: installapply.ErrPlanHashMismatch, code: 2, stderr: applyHashMessage + "\n"},
		{name: "not ready", args: []string{"apply", "--yes", "--plan-hash", hash, "--tool", "git"}, err: installapply.ErrPlanNotReady, code: 2, stderr: applyNotReadyMessage + "\n"},
		{name: "cancel", args: []string{"apply", "--yes", "--plan-hash", hash, "--tool", "git"}, err: context.Canceled, code: 130, stderr: applyCancelledMessage + "\n"},
		{name: "failed", args: []string{"apply", "--yes", "--plan-hash", hash, "--tool", "git"}, err: errors.New("SECRET"), code: 1, stderr: applyFailedMessage + "\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			previous := applyRuntime
			applyRuntime = applyCommandRuntime{
				apply:         func(context.Context, installapply.Request) (installapply.Result, error) { return tc.result, tc.err },
				signalContext: func(parent context.Context) (context.Context, context.CancelFunc) { return context.WithCancel(parent) },
			}
			t.Cleanup(func() { applyRuntime = previous; resetActualApplyCommandForTest(t) })
			resetActualApplyCommandForTest(t)
			var stdout, stderr bytes.Buffer
			if code := executeRoot(tc.args, &stdout, &stderr); code != tc.code || stdout.String() != tc.stdout || stderr.String() != tc.stderr {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func assertApplyExit(t *testing.T, err error, code int, message string) {
	t.Helper()
	var exit *commandExitError
	if !errors.As(err, &exit) || exit.code != code || exit.silent || exit.Error() != message {
		t.Fatalf("error=%#v want code=%d message=%q", err, code, message)
	}
}

func resetActualApplyCommandForTest(t *testing.T) {
	t.Helper()
	for _, name := range []string{"yes", "plan-hash"} {
		if flag := applyCmd.Flags().Lookup(name); flag != nil {
			if err := flag.Value.Set(flag.DefValue); err != nil {
				t.Fatal(err)
			}
			flag.Changed = false
		}
	}
	if flag := applyCmd.Flags().Lookup("tool"); flag != nil {
		replacer, ok := flag.Value.(interface{ Replace([]string) error })
		if !ok {
			t.Fatal("apply --tool flag cannot be reset")
		}
		if err := replacer.Replace(nil); err != nil {
			t.Fatal(err)
		}
		flag.Changed = false
	}
	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(io.Discard)
}
