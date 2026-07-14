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
	pid := waitExactPID(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	started := time.Now()
	command.Cancel()
	select {
	case waitErr := <-command.Done:
		if !errors.Is(waitErr, context.Canceled) || time.Since(started) > 2*time.Second {
			t.Fatalf("group cancel error=%v elapsed=%v", waitErr, time.Since(started))
		}
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(pid, syscall.SIGKILL)
		select {
		case <-command.Done:
		case <-time.After(time.Second):
		}
		t.Fatal("cancel left a descendant holding streaming pipes")
	}
	if !waitExactProcessGone(pid, time.Second) {
		t.Fatalf("descendant %d survived owned-group cancellation", pid)
	}
}

func TestExactStreamingCancellationDoesNotKillUnrelatedProcess(t *testing.T) {
	unrelated := exec.CommandContext(context.Background(), "/bin/sleep", "30")
	if err := unrelated.Start(); err != nil {
		t.Fatalf("start unrelated process: %v", err)
	}
	t.Cleanup(func() { _ = unrelated.Process.Kill(); _, _ = unrelated.Process.Wait() })
	command, err := runner.RunExactStreaming(context.Background(), runner.ExactStreamingRequest{
		Path: "/bin/sleep", Args: []string{"30"}, Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/",
	})
	if err != nil {
		t.Fatalf("start owned process: %v", err)
	}
	command.Cancel()
	select {
	case waitErr := <-command.Done:
		if !errors.Is(waitErr, context.Canceled) {
			t.Fatalf("owned cancellation error=%v", waitErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("owned cancellation did not complete")
	}
	if err := unrelated.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("unrelated process was killed: %v", err)
	}
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
		assertStreamingClass(t, waitErr, "undrained")
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

func TestStreamingEntryPointsOwnWholeProcessGroup(t *testing.T) {
	for _, entry := range streamingEntryPoints() {
		for _, outcome := range []string{"cancel", "deadline", "natural-exit"} {
			t.Run(entry.name+"/"+outcome, func(t *testing.T) {
				ctx, expire := context.WithCancelCause(context.Background())
				mode := "descendant"
				if outcome == "natural-exit" {
					mode = "orphan"
				}
				command, pid := startStreamingDescendant(t, entry, ctx, mode)
				switch outcome {
				case "cancel":
					command.Cancel()
				case "deadline":
					expire(context.DeadlineExceeded)
				}
				waitErr := awaitStreamingDone(t, command)
				if outcome == "cancel" && !errors.Is(waitErr, context.Canceled) {
					t.Fatalf("cancel error=%v", waitErr)
				}
				if outcome == "deadline" && !errors.Is(waitErr, context.DeadlineExceeded) {
					t.Fatalf("deadline error=%v", waitErr)
				}
				if outcome == "natural-exit" && waitErr != nil {
					t.Fatalf("natural leader exit error=%v", waitErr)
				}
				if !waitExactProcessGone(pid, time.Second) {
					t.Fatalf("owned descendant %d survived %s cleanup", pid, outcome)
				}
			})
		}
	}
}

func TestStreamingEntryPointsClassifyReaderFailuresExclusively(t *testing.T) {
	for _, entry := range streamingEntryPoints() {
		for _, source := range []string{"stdout", "stderr"} {
			t.Run(entry.name+"/scan-"+source, func(t *testing.T) {
				command := startStreamingHelper(t, entry, context.Background(), "long-"+source)
				drainStreaming(command)
				assertStreamingClass(t, awaitStreamingDone(t, command), "scan")
			})
			t.Run(entry.name+"/undrained-"+source, func(t *testing.T) {
				command := startStreamingHelper(t, entry, context.Background(), "lines-"+source, "1000")
				assertStreamingClass(t, awaitStreamingDone(t, command), "undrained")
			})
		}
	}
}

func TestStreamingEntryPointsWaitDoneCancelAreStableAndConcurrent(t *testing.T) {
	for _, entry := range streamingEntryPoints() {
		t.Run(entry.name, func(t *testing.T) {
			command := startStreamingHelper(t, entry, context.Background(), "delay", "30000")
			drainStreaming(command)
			results := make(chan error, 8)
			for i := 0; i < 8; i++ {
				go func() { results <- command.Wait() }()
			}
			var callers sync.WaitGroup
			for i := 0; i < 16; i++ {
				callers.Add(1)
				go func() {
					defer callers.Done()
					for j := 0; j < 8; j++ {
						command.Cancel()
					}
				}()
			}
			callers.Wait()
			doneErr, ok := awaitStreamingValue(t, command.Done)
			if !ok || !errors.Is(doneErr, context.Canceled) {
				t.Fatalf("Done result=%v ok=%v, want one canceled result", doneErr, ok)
			}
			for i := 0; i < 8; i++ {
				if err := awaitStreamingError(t, results); !errors.Is(err, context.Canceled) {
					t.Fatalf("Wait result=%v, want stable canceled result", err)
				}
			}
			if _, ok := awaitStreamingValue(t, command.Done); ok {
				t.Fatal("Done published more than one terminal result")
			}
			command.Cancel()
		})
	}
}

func TestStreamingWithSudoFailsClosedBeforeStartingSubprocess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "sudo-started")
	path := filepath.Join(dir, "sudo")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n: > \"$SUDO_MARKER\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SUDO_MARKER", marker)
	command, err := runner.RunStreamingWithSudo(context.Background(), "/bin/true")
	if err == nil || command != nil {
		t.Fatalf("sudo ordinary-lifecycle fallback returned command=%v error=%v", command, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("sudo subprocess started before RK1P: %v", statErr)
	}
}

type streamingEntryPoint struct {
	name  string
	start func(context.Context, string, ...string) (*runner.StreamingCmd, error)
}

func streamingEntryPoints() []streamingEntryPoint {
	return []streamingEntryPoint{
		{name: "RunStreaming", start: runner.RunStreaming},
		{name: "RunExactStreaming", start: func(ctx context.Context, path string, args ...string) (*runner.StreamingCmd, error) {
			return runner.RunExactStreaming(ctx, runner.ExactStreamingRequest{Path: path, Args: args, Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/"})
		}},
	}
}

func startStreamingHelper(t *testing.T, entry streamingEntryPoint, ctx context.Context, mode string, args ...string) *runner.StreamingCmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	arguments := []string{"-test.run=^TestStreamingLifecycleHelper$", "--", "streaming-lifecycle-helper", mode}
	arguments = append(arguments, args...)
	command, err := entry.start(ctx, executable, arguments...)
	if err != nil {
		t.Fatalf("start %s helper %s: %v", entry.name, mode, err)
	}
	return command
}

func startStreamingDescendant(t *testing.T, entry streamingEntryPoint, ctx context.Context, mode string) (*runner.StreamingCmd, int) {
	t.Helper()
	dir := t.TempDir()
	pidFile, release := filepath.Join(dir, "pid"), filepath.Join(dir, "release")
	command := startStreamingHelper(t, entry, ctx, mode, pidFile, release)
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o600) })
	pid := waitExactPID(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	return command, pid
}

func TestStreamingLifecycleHelper(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "streaming-lifecycle-helper" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	args := os.Args[separator+1:]
	signal.Ignore(syscall.SIGPIPE)
	switch args[0] {
	case "long-stdout":
		_, _ = fmt.Fprintln(os.Stdout, strings.Repeat("x", 1024*1024+1))
	case "long-stderr":
		_, _ = fmt.Fprintln(os.Stderr, strings.Repeat("x", 1024*1024+1))
	case "lines-stdout", "lines-stderr":
		count, _ := strconv.Atoi(args[1])
		output := os.Stdout
		if args[0] == "lines-stderr" {
			output = os.Stderr
		}
		for i := 0; i < count; i++ {
			_, _ = fmt.Fprintf(output, "line-%d\n", i)
		}
	case "descendant", "orphan":
		executable, _ := os.Executable()
		child := exec.Command(executable, "-test.run=^TestStreamingLifecycleHelper$", "--", "streaming-lifecycle-helper", "pipe-holder", args[1], args[2])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		if args[0] == "orphan" {
			_ = waitExactPID(t, args[1])
		} else {
			_ = child.Wait()
		}
	case "pipe-holder":
		if err := os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat(args[2]); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	case "delay":
		delay, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(delay) * time.Millisecond)
	case "exit":
		os.Exit(7)
	default:
		t.Fatalf("unknown helper mode %q", args[0])
	}
}

func drainStreaming(command *runner.StreamingCmd) {
	go func() {
		for range command.Output {
		}
	}()
}

func awaitStreamingDone(t *testing.T, command *runner.StreamingCmd) error {
	t.Helper()
	err, _ := awaitStreamingValue(t, command.Done)
	return err
}

func awaitStreamingError(t *testing.T, result <-chan error) error {
	t.Helper()
	err, _ := awaitStreamingValue(t, result)
	return err
}

func awaitStreamingValue(t *testing.T, result <-chan error) (error, bool) {
	t.Helper()
	select {
	case err, ok := <-result:
		return err, ok
	case <-time.After(3 * time.Second):
		t.Fatal("streaming terminal result timed out")
	}
	return nil, false
}

func assertStreamingClass(t *testing.T, err error, class string) {
	t.Helper()
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), class) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("terminal error=%v, want exclusive %s classification", err, class)
	}
	opposite := "scan"
	if class == opposite {
		opposite = "undrained"
	}
	if strings.Contains(strings.ToLower(err.Error()), opposite) {
		t.Fatalf("terminal error=%v has both %s and %s classifications", err, class, opposite)
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
