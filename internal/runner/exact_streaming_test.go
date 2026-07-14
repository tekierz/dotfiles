package runner_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/runner"
)

func TestExactStreamingUsesLiteralRequestAndClonesIt(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "injection-marker")
	script := exactExecutable(t, "#!/bin/sh\nprintf 'cwd=%s\\n' \"$PWD\"\nprintf 'env=%s\\n' \"$EXACT_SAFE\"\nprintf 'arg=%s\\n' \"$1\"\nprintf 'err-line\\n' >&2\n")
	payload := "$(touch " + marker + ");semi;`touch " + marker + "`"
	request := runner.ExactStreamingRequest{Path: script, Args: []string{payload}, Env: []string{"EXACT_SAFE=accepted"}, Dir: dir}
	canonicalDir, _ := filepath.EvalSymlinks(dir)
	command, err := runner.RunExactStreaming(context.Background(), request)
	if err != nil {
		t.Fatalf("start exact stream: %v", err)
	}
	request.Args[0], request.Env[0], request.Dir = "mutated", "EXACT_SAFE=mutated", "/"
	lines, waitErr := collectExactStream(command.Output, command.Done)
	if waitErr != nil {
		t.Fatalf("wait exact stream: %v", waitErr)
	}
	for _, want := range []string{"cwd=" + canonicalDir, "env=accepted", "arg=" + payload, "err-line"} {
		if !containsExactLine(lines, want) {
			t.Fatalf("output %q omits %q", lines, want)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("literal argument reached a shell: %v", err)
	}
	if !reflect.DeepEqual(command.Cmd.Args, []string{script, payload}) || !reflect.DeepEqual(command.Cmd.Env, []string{"EXACT_SAFE=accepted"}) || command.Cmd.Dir != dir {
		t.Fatalf("command did not retain cloned exact request: args=%q env=%q dir=%q", command.Cmd.Args, command.Cmd.Env, command.Cmd.Dir)
	}
}

func TestExactStreamingRejectsMalformedRequestsBeforeStart(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "side-effect-marker")
	script := exactExecutable(t, "#!/bin/sh\n: > \"$SIDE_EFFECT_MARKER\"\n")
	base := runner.ExactStreamingRequest{Path: script, Env: []string{"SIDE_EFFECT_MARKER=" + marker}, Dir: dir}
	var nilContext context.Context
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if command, err := runner.RunExactStreaming(cancelled, base); !errors.Is(err, context.Canceled) || command != nil {
		t.Fatalf("pre-cancelled request returned command=%v error=%v", command, err)
	}
	tests := []struct {
		name string
		ctx  context.Context
		req  runner.ExactStreamingRequest
	}{
		{name: "nil-context", ctx: nilContext, req: base},
		{name: "relative-path", ctx: context.Background(), req: withExactPath(base, filepath.Base(script))},
		{name: "unclean-path", ctx: context.Background(), req: withExactPath(base, dir+"/../"+filepath.Base(dir)+"/"+filepath.Base(script))},
		{name: "nul-path", ctx: context.Background(), req: withExactPath(base, script+"\x00")},
		{name: "nul-argument", ctx: context.Background(), req: withExactArgs(base, []string{"bad\x00arg"})},
		{name: "relative-dir", ctx: context.Background(), req: withExactDir(base, "relative")},
		{name: "unclean-dir", ctx: context.Background(), req: withExactDir(base, dir+"/../"+filepath.Base(dir))},
		{name: "nul-dir", ctx: context.Background(), req: withExactDir(base, dir+"\x00")},
		{name: "malformed-env", ctx: context.Background(), req: withExactEnv(base, []string{"SIDE_EFFECT_MARKER=" + marker, "MALFORMED"})},
		{name: "empty-env-key", ctx: context.Background(), req: withExactEnv(base, []string{"SIDE_EFFECT_MARKER=" + marker, "=value"})},
		{name: "nul-env", ctx: context.Background(), req: withExactEnv(base, []string{"SIDE_EFFECT_MARKER=" + marker, "BAD=value\x00"})},
		{name: "duplicate-env", ctx: context.Background(), req: withExactEnv(base, []string{"SIDE_EFFECT_MARKER=" + marker, "DUP=one", "DUP=two"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if command, err := runner.RunExactStreaming(test.ctx, test.req); err == nil || command != nil {
				t.Fatalf("malformed request returned command=%v error=%v", command, err)
			}
		})
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("malformed request started a process: %v", err)
	}
}

func TestExactStreamingPropagatesExitAndCancellation(t *testing.T) {
	t.Run("nonzero", func(t *testing.T) {
		command, err := runner.RunExactStreaming(context.Background(), runner.ExactStreamingRequest{Path: "/bin/sh", Args: []string{"-c", "printf failure; exit 7"}, Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/"})
		if err != nil {
			t.Fatalf("start nonzero command: %v", err)
		}
		lines, waitErr := collectExactStream(command.Output, command.Done)
		if waitErr == nil || !containsExactLine(lines, "failure") {
			t.Fatalf("nonzero result lines=%q error=%v", lines, waitErr)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		command, err := runner.RunExactStreaming(context.Background(), runner.ExactStreamingRequest{Path: "/bin/sleep", Args: []string{"30"}, Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/"})
		if err != nil {
			t.Fatalf("start cancellable command: %v", err)
		}
		started := time.Now()
		command.Cancel()
		_, waitErr := collectExactStream(command.Output, command.Done)
		if !errors.Is(waitErr, context.Canceled) || time.Since(started) > 2*time.Second {
			t.Fatalf("cancel result error=%v elapsed=%v", waitErr, time.Since(started))
		}
	})
}

func TestExactStreamingCancellationTerminatesPipeHoldingDescendant(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "descendant.pid")
	script := exactExecutable(t, "#!/bin/sh\n/bin/sleep 30 &\nprintf '%s\\n' \"$!\" > \"$PID_FILE\"\nwait\n")
	command, err := runner.RunExactStreaming(context.Background(), runner.ExactStreamingRequest{
		Path: script, Env: []string{"PID_FILE=" + pidFile}, Dir: dir,
	})
	if err != nil {
		t.Fatalf("start descendant command: %v", err)
	}
	t.Cleanup(func() { cleanupStreamingCommand(command) })
	pid := waitExactPID(t, pidFile)
	started := time.Now()
	command.Cancel()
	select {
	case waitErr := <-command.Done:
		if !errors.Is(waitErr, context.Canceled) || time.Since(started) > 2*time.Second {
			t.Fatalf("group cancel error=%v elapsed=%v", waitErr, time.Since(started))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel left a descendant holding streaming pipes")
	}
	if !waitExactProcessGone(pid, time.Second) {
		t.Fatalf("descendant %d survived owned-group cancellation", pid)
	}
}

func TestExactStreamingCancellationDoesNotKillUnrelatedProcess(t *testing.T) {
	assertIndependentOwnedCommands(t, streamingEntryPoints(t)[1])
}

func TestExactStreamingUndrainedOutputFailsBoundedly(t *testing.T) {
	script := exactExecutable(t, "#!/bin/sh\ni=0\nwhile [ \"$i\" -lt 1000 ]; do printf 'line-%s\\n' \"$i\"; i=$((i + 1)); done\n")
	command, err := runner.RunExactStreaming(context.Background(), runner.ExactStreamingRequest{
		Path: script, Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/",
	})
	if err != nil {
		t.Fatalf("start output overflow command: %v", err)
	}
	select {
	case waitErr := <-command.Done:
		if !errors.Is(waitErr, runner.ErrStreamingOutputUndrained) {
			t.Fatalf("overflow error=%v, want deterministic overflow", waitErr)
		}
		if errors.Is(waitErr, context.Canceled) {
			t.Fatalf("overflow was misreported as cancellation: %v", waitErr)
		}
		for range command.Output {
		}
	case <-time.After(2 * time.Second):
		command.Cancel()
		_ = command.Cmd.Process.Kill()
		select {
		case <-command.Done:
		case <-time.After(time.Second):
		}
		t.Fatal("undrained output deadlocked instead of failing boundedly")
	}
}

func TestExactStreamingCancelCompletionRaceIsBounded(t *testing.T) {
	for iteration := 0; iteration < 32; iteration++ {
		command, err := runner.RunExactStreaming(context.Background(), runner.ExactStreamingRequest{
			Path: "/usr/bin/true", Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/",
		})
		if err != nil {
			t.Fatalf("iteration %d start: %v", iteration, err)
		}
		go command.Cancel()
		for range command.Output {
		}
		select {
		case waitErr := <-command.Done:
			if waitErr != nil && !errors.Is(waitErr, context.Canceled) {
				t.Fatalf("iteration %d race error=%v", iteration, waitErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d cancel/completion race hung", iteration)
		}
	}
}

func TestStreamingEntryPointsExposeScanAndUndrainedSentinels(t *testing.T) {
	for _, entry := range streamingEntryPoints(t) {
		for _, source := range []string{"stdout", "stderr"} {
			t.Run(entry.name+"/scan-"+source, func(t *testing.T) {
				command := startStreamingHelper(t, entry, context.Background(), "long-"+source)
				drainStreaming(command)
				assertStreamingError(t, receiveError(t, command.Done), runner.ErrStreamingOutputScan)
			})
			t.Run(entry.name+"/undrained-"+source, func(t *testing.T) {
				command := startStreamingHelper(t, entry, context.Background(), "lines-"+source, "1000")
				assertStreamingError(t, receiveError(t, command.Done), runner.ErrStreamingOutputUndrained)
			})
		}
	}
}

func TestStreamingEntryPointsOwnDescendantLifecycle(t *testing.T) {
	for _, entry := range streamingEntryPoints(t) {
		t.Run(entry.name+"/cancel", func(t *testing.T) {
			command, pidFile := startOwnedDescendant(t, entry, context.Background(), "descendant")
			pid := waitExactPID(t, pidFile)
			command.Cancel()
			assertContextError(t, receiveError(t, command.Done), context.Canceled)
			assertExactProcessGone(t, pid)
		})
		t.Run(entry.name+"/deadline-after-ready", func(t *testing.T) {
			ctx, expire := context.WithCancelCause(context.Background())
			command, pidFile := startOwnedDescendant(t, entry, ctx, "descendant")
			pid := waitExactPID(t, pidFile)
			timer := time.AfterFunc(100*time.Millisecond, func() { expire(context.DeadlineExceeded) })
			defer timer.Stop()
			assertContextError(t, receiveError(t, command.Done), context.DeadlineExceeded)
			assertExactProcessGone(t, pid)
		})
		t.Run(entry.name+"/natural-parent-exit", func(t *testing.T) {
			command, pidFile := startOwnedDescendant(t, entry, context.Background(), "orphan")
			pid := waitExactPID(t, pidFile)
			if waitErr := receiveError(t, command.Done); waitErr != nil {
				t.Fatalf("successful parent exit error=%v", waitErr)
			}
			if waitErr := command.Wait(); waitErr != nil {
				t.Fatalf("successful parent replay error=%v", waitErr)
			}
			assertExactProcessGone(t, pid)
		})
	}
}

func TestStreamingEntryPointsStableTerminalResult(t *testing.T) {
	for _, entry := range streamingEntryPoints(t) {
		for _, outcome := range []string{"success", "exit", "cancel", "deadline", "scan", "undrained"} {
			t.Run(entry.name+"/"+outcome, func(t *testing.T) {
				command := startTerminalOutcome(t, entry, outcome)
				results := make(chan error, 4)
				for call := 0; call < 4; call++ {
					go func() { results <- command.Wait() }()
				}
				for call := 0; call < 4; call++ {
					assertTerminalOutcome(t, receiveError(t, results), outcome)
				}
				doneErr, ok := receiveDoneValue(t, command.Done)
				if !ok {
					t.Fatal("Done closed before publishing its single terminal result")
				}
				assertTerminalOutcome(t, doneErr, outcome)
				if _, ok = receiveDoneValue(t, command.Done); ok {
					t.Fatal("Done published more than one terminal result")
				}
				repeated := make(chan error, 1)
				go func() { repeated <- command.Wait() }()
				assertTerminalOutcome(t, receiveError(t, repeated), outcome)
				command.Cancel()
				postCancel := make(chan error, 1)
				go func() { postCancel <- command.Wait() }()
				assertTerminalOutcome(t, receiveError(t, postCancel), outcome)
				if _, ok = receiveDoneValue(t, command.Done); ok {
					t.Fatal("post-completion Cancel reopened Done")
				}
			})
		}
	}
}

func TestStreamingEntryPointsCancelRacesAndIsolation(t *testing.T) {
	for _, entry := range streamingEntryPoints(t) {
		t.Run(entry.name+"/concurrent-idempotent", func(t *testing.T) {
			command := startStreamingHelper(t, entry, context.Background(), "delay", "30000")
			drainStreaming(command)
			var callers sync.WaitGroup
			for caller := 0; caller < 16; caller++ {
				callers.Add(1)
				go func() {
					defer callers.Done()
					for call := 0; call < 8; call++ {
						command.Cancel()
					}
				}()
			}
			callers.Wait()
			assertContextError(t, command.Wait(), context.Canceled)
			command.Cancel()
		})
		for iteration := 0; iteration < 20; iteration++ {
			t.Run(fmt.Sprintf("%s/wait-done-cancel-%02d", entry.name, iteration), func(t *testing.T) {
				command := startStreamingHelper(t, entry, context.Background(), "delay", "1")
				drainStreaming(command)
				waited := make(chan error, 1)
				go func() { waited <- command.Wait() }()
				go command.Cancel()
				doneErr, ok := receiveDoneValue(t, command.Done)
				waitErr := receiveError(t, waited)
				same := doneErr == nil && waitErr == nil || errors.Is(doneErr, context.Canceled) && errors.Is(waitErr, context.Canceled)
				if !ok || !same {
					t.Fatalf("Wait/Done race diverged: wait=%v done=%v ok=%v", waitErr, doneErr, ok)
				}
				if errors.Is(waitErr, context.Canceled) {
					assertContextError(t, waitErr, context.Canceled)
					assertContextError(t, doneErr, context.Canceled)
				}
			})
		}
		t.Run(entry.name+"/independent-owned-commands", func(t *testing.T) {
			assertIndependentOwnedCommands(t, entry)
		})
	}
}

func TestStreamingEntryPointsOverflowCancelPrecedence(t *testing.T) {
	for _, entry := range streamingEntryPoints(t) {
		t.Run(entry.name+"/cancel-first", func(t *testing.T) {
			command, release := startBarrierCommand(t, entry)
			command.Cancel()
			assertContextError(t, receiveError(t, command.Done), context.Canceled)
			if err := os.WriteFile(release, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		})
		t.Run(entry.name+"/overflow-first", func(t *testing.T) {
			command := startStreamingHelper(t, entry, context.Background(), "lines-stdout", "1000")
			assertStreamingError(t, receiveError(t, command.Done), runner.ErrStreamingOutputUndrained)
			command.Cancel()
			replayed := make(chan error, 1)
			go func() { replayed <- command.Wait() }()
			assertStreamingError(t, receiveError(t, replayed), runner.ErrStreamingOutputUndrained)
		})
	}
}

func TestStreamingLifecycleHelper(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "streaming-lifecycle-helper" {
			separator = index
			break
		}
	}
	if separator < 0 {
		return
	}
	arguments := os.Args[separator+1:]
	if len(arguments) == 0 {
		t.Fatal("streaming lifecycle helper mode is missing")
	}
	signal.Ignore(syscall.SIGPIPE)
	switch arguments[0] {
	case "long-stdout":
		_, _ = fmt.Fprintln(os.Stdout, strings.Repeat("x", 1024*1024+1))
	case "long-stderr":
		_, _ = fmt.Fprintln(os.Stderr, strings.Repeat("x", 1024*1024+1))
	case "lines-stdout", "lines-stderr":
		count, _ := strconv.Atoi(arguments[1])
		output := os.Stdout
		if arguments[0] == "lines-stderr" {
			output = os.Stderr
		}
		for line := 0; line < count; line++ {
			_, _ = fmt.Fprintf(output, "line-%d\n", line)
		}
	case "descendant", "orphan", "middle":
		childMode := "middle"
		if arguments[0] == "middle" {
			childMode = "pipe-holder"
		}
		executable, _ := os.Executable()
		child := exec.Command(executable, "-test.run=^TestStreamingLifecycleHelper$", "--", "streaming-lifecycle-helper", childMode, arguments[1], arguments[2])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		if arguments[0] == "orphan" {
			_ = waitExactPID(t, arguments[1])
		} else {
			_ = child.Wait()
		}
	case "pipe-holder", "barrier":
		if err := os.WriteFile(arguments[1], []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat(arguments[2]); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	case "delay":
		delay, _ := strconv.Atoi(arguments[1])
		time.Sleep(time.Duration(delay) * time.Millisecond)
	case "exit":
		os.Exit(7)
	default:
		t.Fatalf("unknown streaming lifecycle helper mode %q", arguments[0])
	}
}

type streamingEntryPoint struct {
	name  string
	start func(context.Context, string, ...string) (*runner.StreamingCmd, error)
}

func streamingEntryPoints(_ *testing.T) []streamingEntryPoint {
	return []streamingEntryPoint{
		{name: "RunStreaming", start: runner.RunStreaming},
		{name: "RunExactStreaming", start: func(ctx context.Context, executable string, args ...string) (*runner.StreamingCmd, error) {
			return runner.RunExactStreaming(ctx, runner.ExactStreamingRequest{
				Path: executable, Args: args, Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/",
			})
		}},
	}
}

func startStreamingHelper(t *testing.T, entry streamingEntryPoint, ctx context.Context, mode string, arguments ...string) *runner.StreamingCmd {
	t.Helper()
	executable, _ := os.Executable()
	args := []string{"-test.run=^TestStreamingLifecycleHelper$", "--", "streaming-lifecycle-helper", mode}
	args = append(args, arguments...)
	command, err := entry.start(ctx, executable, args...)
	if err != nil {
		t.Fatalf("start %s helper %s: %v", entry.name, mode, err)
	}
	t.Cleanup(func() { cleanupStreamingCommand(command) })
	return command
}

func startOwnedDescendant(t *testing.T, entry streamingEntryPoint, ctx context.Context, mode string) (*runner.StreamingCmd, string) {
	t.Helper()
	dir := t.TempDir()
	pidFile, stopFile := filepath.Join(dir, "pid"), filepath.Join(dir, "stop")
	command := startStreamingHelper(t, entry, ctx, mode, pidFile, stopFile)
	t.Cleanup(func() { _ = os.WriteFile(stopFile, nil, 0o600) })
	return command, pidFile
}

func startBarrierCommand(t *testing.T, entry streamingEntryPoint) (*runner.StreamingCmd, string) {
	t.Helper()
	dir := t.TempDir()
	ready, release := filepath.Join(dir, "ready"), filepath.Join(dir, "release")
	command := startStreamingHelper(t, entry, context.Background(), "barrier", ready, release)
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o600) })
	_ = waitExactPID(t, ready)
	return command, release
}

func assertIndependentOwnedCommands(t *testing.T, entry streamingEntryPoint) {
	t.Helper()
	commandA := startStreamingHelper(t, entry, context.Background(), "delay", "30000")
	drainStreaming(commandA)
	commandB, releaseB := startBarrierCommand(t, entry)
	commandA.Cancel()
	assertContextError(t, receiveError(t, commandA.Done), context.Canceled)
	select {
	case waitErr := <-commandB.Done:
		t.Fatalf("cancel A completed B early: %v", waitErr)
	default:
	}
	if err := os.WriteFile(releaseB, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if waitErr := commandB.Wait(); waitErr != nil {
		t.Fatalf("independent B error=%v", waitErr)
	}
}

func drainStreaming(command *runner.StreamingCmd) {
	go func() {
		for range command.Output {
		}
	}()
}

func receiveDoneValue(t *testing.T, done <-chan error) (error, bool) {
	t.Helper()
	select {
	case value, ok := <-done:
		return value, ok
	case <-time.After(3 * time.Second):
		t.Fatal("terminal result timed out")
	}
	return nil, false
}

func receiveError(t *testing.T, result <-chan error) error {
	t.Helper()
	value, _ := receiveDoneValue(t, result)
	return value
}

func assertStreamingError(t *testing.T, got, want error) {
	t.Helper()
	opposite := runner.ErrStreamingOutputScan
	if errors.Is(want, opposite) {
		opposite = runner.ErrStreamingOutputUndrained
	}
	if !errors.Is(got, want) || errors.Is(got, opposite) || errors.Is(got, context.Canceled) || errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("terminal error=%v, want %v without context classification", got, want)
	}
}

func assertContextError(t *testing.T, got, want error) {
	t.Helper()
	opposite := context.Canceled
	if errors.Is(want, opposite) {
		opposite = context.DeadlineExceeded
	}
	if !errors.Is(got, want) || errors.Is(got, opposite) || errors.Is(got, runner.ErrStreamingOutputScan) || errors.Is(got, runner.ErrStreamingOutputUndrained) {
		t.Fatalf("terminal error=%v, want exclusive %v", got, want)
	}
}

func assertExactProcessGone(t *testing.T, pid int) {
	t.Helper()
	if !waitExactProcessGone(pid, time.Second) {
		t.Fatalf("owned descendant %d survived terminal cleanup", pid)
	}
}

func startTerminalOutcome(t *testing.T, entry streamingEntryPoint, outcome string) *runner.StreamingCmd {
	t.Helper()
	ctx := context.Background()
	mode, arguments, drain := "delay", []string{"1"}, true
	switch outcome {
	case "exit":
		mode = "exit"
	case "cancel":
		mode, arguments = "delay", []string{"30000"}
	case "deadline":
		ctx, _ = context.WithTimeout(ctx, 100*time.Millisecond)
		arguments = []string{"30000"}
	case "scan":
		mode = "long-stdout"
	case "undrained":
		mode, arguments, drain = "lines-stdout", []string{"1000"}, false
	}
	command := startStreamingHelper(t, entry, ctx, mode, arguments...)
	if drain {
		drainStreaming(command)
	}
	if outcome == "cancel" {
		command.Cancel()
	}
	return command
}

func assertTerminalOutcome(t *testing.T, got error, outcome string) {
	t.Helper()
	if want, ok := map[string]error{"cancel": context.Canceled, "deadline": context.DeadlineExceeded}[outcome]; ok {
		assertContextError(t, got, want)
		return
	}
	if want, ok := map[string]error{"scan": runner.ErrStreamingOutputScan, "undrained": runner.ErrStreamingOutputUndrained}[outcome]; ok {
		assertStreamingError(t, got, want)
		return
	}
	valid := outcome == "success" && got == nil
	var exitErr *exec.ExitError
	valid = valid || outcome == "exit" && errors.As(got, &exitErr) && !errors.Is(got, context.Canceled) && !errors.Is(got, context.DeadlineExceeded) && !errors.Is(got, runner.ErrStreamingOutputScan) && !errors.Is(got, runner.ErrStreamingOutputUndrained)
	if !valid {
		t.Fatalf("outcome %s got terminal error %v", outcome, got)
	}
}

func cleanupStreamingCommand(command *runner.StreamingCmd) {
	command.Cancel()
	drainStreaming(command)
	select {
	case <-command.Done:
	case <-time.After(time.Second):
	}
}

func waitExactPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 1 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("descendant pid was not recorded")
	return 0
}

func waitExactProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, syscall.Signal(0)); err != nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func exactExecutable(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exact-command")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func collectExactStream(output <-chan string, done <-chan error) ([]string, error) {
	var lines []string
	for line := range output {
		lines = append(lines, line)
	}
	return lines, <-done
}

func containsExactLine(lines []string, want string) bool {
	for _, line := range lines {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func withExactPath(request runner.ExactStreamingRequest, value string) runner.ExactStreamingRequest {
	request.Path = value
	return request
}

func withExactArgs(request runner.ExactStreamingRequest, value []string) runner.ExactStreamingRequest {
	request.Args = value
	return request
}

func withExactEnv(request runner.ExactStreamingRequest, value []string) runner.ExactStreamingRequest {
	request.Env = value
	return request
}

func withExactDir(request runner.ExactStreamingRequest, value string) runner.ExactStreamingRequest {
	request.Dir = value
	return request
}
