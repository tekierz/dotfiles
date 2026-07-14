//go:build darwin || linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStreamingEntryPointsOwnProcessGroup(t *testing.T) {
	for _, entry := range streamingTestEntries() {
		for _, outcome := range []string{"cancel", "deadline", "natural"} {
			t.Run(entry.name+"/"+outcome, func(t *testing.T) {
				ctx, expire := context.WithCancelCause(context.Background())
				mode := map[bool]string{false: "group", true: "orphan"}[outcome == "natural"]
				command, pid := startStreamingTree(t, entry, ctx, mode)
				if outcome == "cancel" {
					command.Cancel()
				} else if outcome == "deadline" {
					expire(context.DeadlineExceeded)
				}
				waitErr := awaitStreamingResult(t, command.Done)
				requireStreaming(t, !(outcome == "cancel" && !errors.Is(waitErr, context.Canceled) || outcome == "deadline" && !errors.Is(waitErr, context.DeadlineExceeded) || outcome == "natural" && waitErr != nil), "%s terminal error=%v", outcome, waitErr)
				requireStreaming(t, streamingProcessGone(pid, time.Second), "owned descendant %d survived %s", pid, outcome)
			})
		}
	}
}
func TestStreamingEntryPointsClassifyReaderFailures(t *testing.T) {
	for _, entry := range streamingTestEntries() {
		for _, source := range []string{"stdout", "stderr"} {
			t.Run(entry.name+"/scan-"+source, func(t *testing.T) {
				command := startStreamingHelper(t, entry, context.Background(), "long-"+source)
				drainStreamingOutput(command)
				assertStreamingErrorClass(t, awaitStreamingResult(t, command.Done), "scan")
			})
			t.Run(entry.name+"/undrained-"+source, func(t *testing.T) {
				command := startStreamingHelper(t, entry, context.Background(), "lines-"+source, "1000")
				assertStreamingErrorClass(t, awaitStreamingResult(t, command.Done), "undrained")
			})
		}
	}
}
func TestStreamingEntryPointsCacheWaitDoneAndCancel(t *testing.T) {
	for _, entry := range streamingTestEntries() {
		t.Run(entry.name, func(t *testing.T) {
			command := startStreamingHelper(t, entry, context.Background(), "delay", "30000")
			drainStreamingOutput(command)
			waits := make(chan error, 8)
			for index := 0; index < cap(waits); index++ {
				go func() { waits <- command.Wait() }()
			}
			for index := 0; index < 8; index++ {
				go command.Cancel()
			}
			result, ok := awaitStreamingValue(t, command.Done)
			requireStreaming(t, ok && errors.Is(result, context.Canceled), "Done result=%v ok=%v", result, ok)
			for index := 0; index < cap(waits); index++ {
				requireStreaming(t, errors.Is(awaitStreamingResult(t, waits), context.Canceled), "Wait result changed")
			}
			_, ok = awaitStreamingValue(t, command.Done)
			requireStreaming(t, !ok, "Done published more than one value")
			command.Cancel()
			requireStreaming(t, errors.Is(command.Wait(), context.Canceled), "post-Cancel Wait changed")
		})
	}
}
func TestStreamingWithSudoFailsClosed(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\n: > \"$SUDO_MARKER\"\n"), 0o700)
	requireStreaming(t, err == nil, "write fake sudo: %v", err)
	t.Setenv("PATH", dir)
	t.Setenv("SUDO_MARKER", marker)
	command, err := RunStreamingWithSudo(context.Background(), "/bin/true")
	requireStreaming(t, command == nil && err != nil, "sudo fallback command=%v error=%v", command, err)
	_, err = os.Stat(marker)
	requireStreaming(t, os.IsNotExist(err), "sudo subprocess started: %v", err)
}
func TestStreamingLifecycleDependencyFailures(t *testing.T) {
	observerFailure, closeFailure := errors.New("observer wait"), errors.New("observer close")
	tests := []struct {
		name, mode, cause, class string
		wait, close              error
		signals                  []error
		want                     error
	}{
		{name: "eintr-esrch", signals: []error{syscall.EINTR, syscall.ESRCH}},
		{name: "cleanup", signals: []error{syscall.EPERM}, want: syscall.EPERM},
		{name: "observer-first", wait: observerFailure, signals: []error{syscall.EPERM}, want: observerFailure},
		{name: "close", close: closeFailure, signals: []error{syscall.ESRCH}, want: closeFailure},
		{name: "cancel-first", cause: "cancel", wait: observerFailure, signals: []error{syscall.EPERM}, want: context.Canceled},
		{name: "deadline-first", cause: "deadline", wait: observerFailure, signals: []error{syscall.EPERM}, want: context.DeadlineExceeded},
		{name: "scan-first", mode: "long-stdout", class: "scan", wait: observerFailure, signals: []error{syscall.EPERM}},
		{name: "undrained-first", mode: "lines-stdout", class: "undrained", wait: observerFailure, signals: []error{syscall.EPERM}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, expire := context.WithCancelCause(context.Background())
			cmd, ready, release := exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 0"), "", ""
			if test.mode != "" {
				cmd, ready, release = newStreamingTreeCommand(t, test.mode)
			}
			events, gate := make(chan string, len(test.signals)+2), make(chan struct{})
			deps := defaultStreamingLifecycleDeps()
			deps.newExitObserver = func(int) (streamingExitObserver, error) {
				return &fakeLifecycleObserver{test.wait, test.close, gate, events, func() { requireStreaming(t, cmd.ProcessState == nil, "observer closed after cmd.Wait") }}, nil
			}
			deps.signalGroup = func(pgid int, signal syscall.Signal) error {
				requireStreaming(t, cmd.ProcessState == nil, "cleanup after cmd.Wait")
				events <- "signal"
				if test.cause != "" || test.class != "" {
					_ = syscall.Kill(-pgid, signal)
				}
				return test.signals[len(events)-2]
			}
			command, err := startStreamingLifecycleWithDeps(ctx, cmd, deps)
			requireStreaming(t, err == nil, "start: %v", err)
			if cause := map[string]error{"cancel": context.Canceled, "deadline": context.DeadlineExceeded}[test.cause]; cause != nil {
				expire(cause)
			}
			if ready != "" {
				_ = waitStreamingPID(t, ready)
				_ = os.WriteFile(release, nil, 0o600)
			}
			close(gate)
			result := command.Wait()
			if test.class != "" {
				assertStreamingErrorClass(t, result, test.class)
			} else {
				requireStreaming(t, errors.Is(result, test.want), "result=%v want=%v", result, test.want)
			}
			requireStreaming(t, <-events == "wait", "observer Wait was not first")
			for range test.signals {
				requireStreaming(t, <-events == "signal", "cleanup order changed")
			}
			requireStreaming(t, <-events == "close", "observer Close was not last")
			command.Cancel()
			replay := command.Wait()
			if test.class != "" {
				assertStreamingErrorClass(t, replay, test.class)
			} else {
				requireStreaming(t, errors.Is(replay, test.want), "replay=%v want=%v", replay, test.want)
			}
			requireStreaming(t, len(events) == 0, "lifecycle teardown repeated")
		})
	}
}
func TestStreamingLifecycleBoundsEscapedPipeHolder(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel-%v", cancel), func(t *testing.T) {
			mode := map[bool]string{false: "escape", true: "escape-block"}[cancel]
			cmd, pidFile, release := newStreamingTreeCommand(t, mode)
			deps := defaultStreamingLifecycleDeps()
			deps.pipeCloseDelay = 40 * time.Millisecond
			command, err := startStreamingLifecycleWithDeps(context.Background(), cmd, deps)
			requireStreaming(t, err == nil, "start escaped holder: %v", err)
			drainStreamingOutput(command)
			pid := waitStreamingPID(t, pidFile)
			started := time.Now()
			if cancel {
				command.Cancel()
			} else {
				_ = os.WriteFile(release+".leader", nil, 0o600)
			}
			result := awaitStreamingResult(t, command.Done)
			elapsed := time.Since(started)
			requireStreaming(t, elapsed >= deps.pipeCloseDelay && elapsed <= time.Second, "bounded reader elapsed=%v delay=%v", elapsed, deps.pipeCloseDelay)
			requireStreaming(t, !(cancel && !errors.Is(result, context.Canceled) || !cancel && result != nil), "terminal result=%v", result)
			requireStreaming(t, !streamingProcessGone(pid, 0), "escaped holder died before cleanup")
			_ = os.WriteFile(release, nil, 0o600)
			requireStreaming(t, streamingProcessGone(pid, time.Second), "escaped holder leaked after cleanup")
		})
	}
}

type streamingTestEntry struct {
	name  string
	start func(context.Context, string, ...string) (*StreamingCmd, error)
}

func streamingTestEntries() []streamingTestEntry {
	return []streamingTestEntry{{"RunStreaming", RunStreaming}, {"RunExactStreaming", func(ctx context.Context, path string, args ...string) (*StreamingCmd, error) {
		return RunExactStreaming(ctx, ExactStreamingRequest{Path: path, Args: args, Env: []string{"PATH=/usr/bin:/bin"}, Dir: "/"})
	}}}
}
func startStreamingHelper(t *testing.T, entry streamingTestEntry, ctx context.Context, mode string, args ...string) *StreamingCmd {
	executable, _ := os.Executable()
	arguments := []string{"-test.run=^TestStreamingLifecycleProcessHelper$", "--", "streaming-helper", mode}
	arguments = append(arguments, args...)
	command, err := entry.start(ctx, executable, arguments...)
	requireStreaming(t, err == nil, "start %s/%s: %v", entry.name, mode, err)
	return command
}
func startStreamingTree(t *testing.T, entry streamingTestEntry, ctx context.Context, mode string) (*StreamingCmd, int) {
	cmd, pidFile, _ := newStreamingTreeCommand(t, mode)
	command, err := entry.start(ctx, cmd.Path, cmd.Args[1:]...)
	requireStreaming(t, err == nil, "start tree: %v", err)
	return command, waitStreamingPID(t, pidFile)
}
func newStreamingTreeCommand(t *testing.T, mode string) (*exec.Cmd, string, string) {
	dir := t.TempDir()
	pidFile, release := filepath.Join(dir, "pid"), filepath.Join(dir, "release")
	executable, _ := os.Executable()
	cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestStreamingLifecycleProcessHelper$", "--", "streaming-helper", mode, pidFile, release)
	t.Cleanup(func() {
		_ = os.WriteFile(release, nil, 0o600)
		if pid := readStreamingPID(pidFile); pid > 1 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return cmd, pidFile, release
}
func TestStreamingLifecycleProcessHelper(t *testing.T) {
	index := slices.Index(os.Args, "streaming-helper")
	if index < 0 {
		return
	}
	signal.Ignore(syscall.SIGPIPE)
	args := os.Args[index+1:]
	switch args[0] {
	case "long-stdout", "long-stderr":
		output := os.Stdout
		if args[0] == "long-stderr" {
			output = os.Stderr
		}
		_, _ = fmt.Fprintln(output, strings.Repeat("x", 1024*1024+1))
		if len(args) > 2 {
			_ = os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0o600)
			waitStreamingRelease(args[2])
		}
	case "lines-stdout", "lines-stderr":
		output := os.Stdout
		if args[0] == "lines-stderr" {
			output = os.Stderr
		}
		count, _ := strconv.Atoi(args[1])
		for line := 0; line < count; line++ {
			_, _ = fmt.Fprintf(output, "line-%d\n", line)
		}
		if len(args) > 2 {
			_ = os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0o600)
			waitStreamingRelease(args[2])
		}
	case "delay":
		delay, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(delay) * time.Millisecond)
	case "group", "orphan", "escape", "escape-block":
		executable, _ := os.Executable()
		child := exec.CommandContext(context.Background(), executable, "-test.run=^TestStreamingLifecycleProcessHelper$", "--", "streaming-helper", "hold", args[1], args[2])
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if strings.HasPrefix(args[0], "escape") {
			child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		}
		err := child.Start()
		requireStreaming(t, err == nil, "start helper child: %v", err)
		if args[0] == "group" {
			_ = child.Wait()
		} else {
			_ = waitStreamingPID(t, args[1])
			if strings.HasPrefix(args[0], "escape") {
				waitStreamingRelease(map[bool]string{false: args[2], true: args[2] + ".leader"}[args[0] == "escape"])
			}
		}
	case "hold":
		err := os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0o600)
		requireStreaming(t, err == nil, "write helper pid: %v", err)
		waitStreamingRelease(args[2])
	}
}
func waitStreamingRelease(path string) {
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func drainStreamingOutput(command *StreamingCmd) {
	go func() {
		for range command.Output {
		}
	}()
}
func awaitStreamingResult(t *testing.T, result <-chan error) error {
	value, _ := awaitStreamingValue(t, result)
	return value
}
func awaitStreamingValue(t *testing.T, result <-chan error) (error, bool) {
	select {
	case value, ok := <-result:
		return value, ok
	case <-time.After(3 * time.Second):
		t.Fatal("streaming result timeout")
	}
	return nil, false
}
func waitStreamingPID(t *testing.T, path string) int {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pid := readStreamingPID(path); pid > 1 {
			return pid
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("pid not recorded")
	return 0
}
func readStreamingPID(path string) int {
	data, _ := os.ReadFile(path)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}
func streamingProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) != nil
}
func assertStreamingErrorClass(t *testing.T, err error, class string) {
	opposite := "scan"
	if class == "scan" {
		opposite = "undrained"
	}
	text := strings.ToLower(fmt.Sprint(err))
	if err == nil || !strings.Contains(text, class) || strings.Contains(text, opposite) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("terminal error=%v, want exclusive %s", err, class)
	}
}
func requireStreaming(t *testing.T, condition bool, format string, args ...any) {
	if !condition {
		t.Fatalf(format, args...)
	}
}

type fakeLifecycleObserver struct {
	waitErr, closeErr error
	gate              <-chan struct{}
	events            chan<- string
	closeCheck        func()
}

func (observer *fakeLifecycleObserver) Wait() error {
	<-observer.gate
	observer.events <- "wait"
	return observer.waitErr
}
func (observer *fakeLifecycleObserver) Close() error {
	observer.events <- "close"
	observer.closeCheck()
	return observer.closeErr
}
