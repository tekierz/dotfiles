package runner_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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
		if waitErr == nil || waitErr.Error() != "exact streaming output overflow" {
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

func waitExactPID(t *testing.T, path string) int {
	t.Helper()
	// Starting the child can be delayed when the full race-enabled suite is
	// competing for CPU. This setup allowance does not widen the two-second
	// cancellation bound asserted by the test itself.
	deadline := time.Now().Add(5 * time.Second)
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
