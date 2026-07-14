//go:build darwin || linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestStreamingLifecycleRejectsInvalidAndPreCanceledStarts(t *testing.T) {
	var nilContext context.Context
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	started := lifecycleCommand(t, "exit", "0")
	if err := started.Start(); err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	t.Cleanup(func() { _, _ = started.Process.Wait() })
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	preCanceled := lifecycleCommand(t, "touch", marker)
	badPipe := lifecycleCommand(t, "exit", "0")
	badPipe.Stdout = io.Discard
	for _, test := range []struct {
		name string
		ctx  context.Context
		cmd  *exec.Cmd
		want error
		any  bool
	}{
		{name: "nil-context", ctx: nilContext, cmd: lifecycleCommand(t, "exit", "0"), want: errInvalidStreamingLifecycle},
		{name: "nil-command", ctx: context.Background(), want: errInvalidStreamingLifecycle},
		{name: "already-started", ctx: context.Background(), cmd: started, want: errInvalidStreamingLifecycle},
		{name: "pre-canceled", ctx: canceled, cmd: preCanceled, want: context.Canceled},
		{name: "pipe-setup", ctx: context.Background(), cmd: badPipe, any: true},
		{name: "start", ctx: context.Background(), cmd: exec.CommandContext(context.Background(), filepath.Join(t.TempDir(), "missing")), any: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := startStreamingLifecycle(test.ctx, test.cmd)
			if stream != nil || err == nil || !test.any && !errors.Is(err, test.want) {
				t.Fatalf("stream=%v error=%v want=%v", stream, err, test.want)
			}
		})
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-canceled command ran: %v", err)
	}
	if preCanceled.Process != nil {
		t.Fatalf("pre-canceled command started process %d", preCanceled.Process.Pid)
	}
}
func TestStreamingLifecycleSetupFailureReapsStartedChildOnce(t *testing.T) {
	observerErr := errors.New("observer setup")
	var signals, waits atomic.Int32
	deps := defaultStreamingLifecycleDeps()
	deps.newExitObserver = func(int) (streamingExitObserver, error) { return nil, observerErr }
	deps.signalGroup = func(pgid int, signal syscall.Signal) error { signals.Add(1); return syscall.Kill(-pgid, signal) }
	deps.waitCommand = func(command *exec.Cmd) error { waits.Add(1); return command.Wait() }
	stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
	if stream != nil || !errors.Is(err, observerErr) || signals.Load() != 1 || waits.Load() != 1 {
		t.Fatalf("stream=%v error=%v signals=%d waits=%d", stream, err, signals.Load(), waits.Load())
	}
}
func TestStreamingLifecycleReturnsExactNaturalStatus(t *testing.T) {
	for _, test := range []struct {
		name, mode, arg string
		exit            int
		signal          syscall.Signal
	}{
		{name: "success", mode: "exit", arg: "0"},
		{name: "nonzero", mode: "exit", arg: "17", exit: 17},
		{name: "self-sigkill", mode: "sigkill", signal: syscall.SIGKILL},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := startStreamingLifecycle(context.Background(), lifecycleCommand(t, test.mode, test.arg))
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			for range stream.Output() {
			}
			waitErr := stream.Wait()
			if test.exit == 0 && test.signal == 0 && waitErr != nil {
				t.Fatalf("success error=%v", waitErr)
			}
			if test.exit != 0 || test.signal != 0 {
				var exitErr *exec.ExitError
				if !errors.As(waitErr, &exitErr) || exitErr == nil {
					t.Fatalf("error=%T %v, want exact nonnil *exec.ExitError", waitErr, waitErr)
				}
				if waitErr != exitErr { //nolint:errorlint // Exact identity rejects wrapped exit errors after errors.As.
					t.Fatalf("error=%T %v wraps *exec.ExitError instead of returning it directly", waitErr, waitErr)
				}
				if test.exit != 0 && exitErr.ExitCode() != test.exit {
					t.Fatalf("exit error=%v want=%d", waitErr, test.exit)
				}
				if test.signal != 0 && exitErr.ProcessState.Sys().(syscall.WaitStatus).Signal() != test.signal {
					t.Fatalf("signal error=%v want=%v", waitErr, test.signal)
				}
			}
		})
	}
}
func TestStreamingLifecycleRetainsBufferedOutputAfterTerminalRead(t *testing.T) {
	for _, terminal := range []string{"Wait", "Done"} {
		t.Run(terminal, func(t *testing.T) {
			stream, err := startStreamingLifecycle(context.Background(), lifecycleCommand(t, "buffered"))
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if terminal == "Wait" {
				err = stream.Wait()
			} else {
				err = awaitLifecycleResult(t, stream.Done())
			}
			if err != nil {
				t.Fatalf("terminal error: %v", err)
			}
			if lines := collectLifecycleOutput(t, stream.Output()); !slices.Equal(lines, []string{"one", "three", "two"}) {
				t.Fatalf("buffered output=%q", lines)
			}
		})
	}
}
func TestStreamingLifecycleOwnsNaturalOrphanAndCancellation(t *testing.T) {
	for _, mode := range []string{"natural", "cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			command, pidFile := lifecycleTreeCommand(t, "group")
			ctx, expire := context.WithCancelCause(context.Background())
			defer expire(context.Canceled)
			stream, err := startStreamingLifecycle(ctx, command)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			pid := waitLifecyclePID(t, pidFile)
			switch mode {
			case "cancel":
				stream.Cancel()
			case "deadline":
				expire(context.DeadlineExceeded)
			}
			waitErr := stream.Wait()
			if mode == "natural" && waitErr != nil || mode == "cancel" && !errors.Is(waitErr, context.Canceled) || mode == "deadline" && !errors.Is(waitErr, context.DeadlineExceeded) {
				t.Fatalf("%s error=%v", mode, waitErr)
			}
			if !waitLifecycleProcessGone(pid, time.Second) {
				t.Fatalf("owned descendant %d survived", pid)
			}
		})
	}
}
func TestStreamingLifecycleClassifiesReaderFailures(t *testing.T) {
	for _, test := range []struct {
		name, mode string
		want       error
	}{
		{name: "stdout-scan", mode: "long-stdout", want: errStreamingScanFailure},
		{name: "stderr-scan", mode: "long-stderr", want: errStreamingScanFailure},
		{name: "stdout-undrained", mode: "lines-stdout", want: errStreamingOutputOverflow},
		{name: "stderr-undrained", mode: "lines-stderr", want: errStreamingOutputOverflow},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := startStreamingLifecycle(context.Background(), lifecycleCommand(t, test.mode, "1000"))
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if strings.Contains(test.name, "scan") {
				go func() {
					for range stream.Output() {
					}
				}()
			}
			waitErr := stream.Wait()
			if !errors.Is(waitErr, test.want) || errors.Is(waitErr, context.Canceled) {
				t.Fatalf("error=%v want=%v", waitErr, test.want)
			}
		})
	}
}
func TestStreamingLifecycleBoundsEscapedHolderAndJoinsReaders(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		name := "natural"
		if cancel {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			command, pidFile := lifecycleTreeCommand(t, "escape")
			deps := defaultStreamingLifecycleDeps()
			deps.pipeCloseDelay = 25 * time.Millisecond
			requested := make(chan time.Duration, 1)
			release := make(chan time.Time, 1)
			deps.after = func(delay time.Duration) <-chan time.Time { requested <- delay; return release }
			stream, err := startStreamingLifecycleWithDeps(context.Background(), command, deps)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			pid := waitLifecyclePID(t, pidFile)
			if cancel {
				stream.Cancel()
			}
			if delay := awaitLifecycleResult(t, requested); delay != deps.pipeCloseDelay {
				t.Fatalf("delay=%v", delay)
			}
			waited := lifecycleCall(stream.Wait)
			select {
			case result := <-waited:
				t.Fatalf("completed before bounded close: %v", result)
			default:
			}
			release <- time.Now()
			waitErr := awaitLifecycleResult(t, waited)
			if cancel && !errors.Is(waitErr, context.Canceled) || !cancel && waitErr != nil {
				t.Fatalf("error=%v", waitErr)
			}
			if output := collectLifecycleOutput(t, stream.Output()); len(output) != 0 {
				t.Fatalf("unexpected output=%q", output)
			}
			if waitLifecycleProcessGone(pid, 0) {
				t.Fatal("escaped holder was signaled outside owned group")
			}
		})
	}
}
func TestStreamingLifecycleFirstCauseIsSynchronous(t *testing.T) {
	observerErr := errors.New("observer wait")
	for _, observerFirst := range []bool{false, true} {
		name := "cancel-first"
		if observerFirst {
			name = "observer-first"
		}
		t.Run(name, func(t *testing.T) {
			observer := &testLifecycleObserver{waitErr: observerErr, entered: make(chan struct{}), release: make(chan struct{})}
			cleanupEntered, releaseCleanup := make(chan struct{}), make(chan struct{})
			var once sync.Once
			deps := defaultStreamingLifecycleDeps()
			deps.newExitObserver = func(int) (streamingExitObserver, error) { return observer, nil }
			deps.signalGroup = func(pgid int, signal syscall.Signal) error {
				once.Do(func() { close(cleanupEntered) })
				<-releaseCleanup
				return syscall.Kill(-pgid, signal)
			}
			stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			awaitLifecycleResult(t, observer.entered)
			if observerFirst {
				close(observer.release)
				awaitLifecycleResult(t, cleanupEntered)
				awaitLifecycleResult(t, lifecycleAction(stream.Cancel))
			} else {
				awaitLifecycleResult(t, lifecycleAction(stream.Cancel))
				awaitLifecycleResult(t, cleanupEntered)
				close(observer.release)
			}
			close(releaseCleanup)
			want := context.Canceled
			if observerFirst {
				want = observerErr
			}
			if waitErr := stream.Wait(); !errors.Is(waitErr, want) {
				t.Fatalf("error=%v want=%v", waitErr, want)
			}
		})
	}
}
func TestStreamingLifecycleCleanupFallbackOrderAndErrors(t *testing.T) {
	observerErr := errors.New("observer wait")
	closeErr := errors.New("observer close")
	hiddenGroupKillErr := errors.New("fake group signal killed leader")
	invalidGroupSignalErr := errors.New("invalid group signal")
	invalidLeaderSignalErr := errors.New("invalid leader signal")
	for _, test := range []struct {
		name, mode, arg string
		observer, close error
		group, leader   []error
		want            []error
		events          []string
		live            bool
	}{
		{name: "group-eintr-esrch", mode: "exit", arg: "0", group: []error{syscall.EINTR, syscall.ESRCH}, events: []string{"observer-wait", "group", "group", "observer-close", "cmd-wait"}},
		{name: "group-eperm-leader-eintr-kill", mode: "sleep", arg: "30", group: []error{syscall.EPERM}, leader: []error{syscall.EINTR, nil}, want: []error{syscall.EPERM}, events: []string{"observer-wait", "group", "leader", "leader", "observer-close", "cmd-wait"}, live: true},
		{name: "group-eperm-leader-esrch", mode: "exit", arg: "0", group: []error{syscall.EPERM}, leader: []error{syscall.ESRCH}, want: []error{syscall.EPERM}, events: []string{"observer-wait", "group", "leader", "observer-close", "cmd-wait"}},
		{name: "observer-error-eperm", mode: "sleep", arg: "30", observer: observerErr, group: []error{syscall.EPERM}, leader: []error{nil}, want: []error{observerErr, syscall.EPERM}, events: []string{"observer-wait", "group", "leader", "observer-close", "cmd-wait"}, live: true},
		{name: "group-esrch-close-error", mode: "exit", arg: "0", close: closeErr, group: []error{syscall.ESRCH}, want: []error{closeErr}, events: []string{"observer-wait", "group", "observer-close", "cmd-wait"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			log := &lifecycleEventLog{}
			observer := &testLifecycleObserver{waitErr: test.observer, closeErr: test.close, log: log}
			deps := defaultStreamingLifecycleDeps()
			deps.newExitObserver = func(int) (streamingExitObserver, error) { return observer, nil }
			var groupCalls, leaderCalls, waits atomic.Int32
			deps.signalGroup = func(pgid int, signal syscall.Signal) error {
				log.add("group")
				index := int(groupCalls.Add(1)) - 1
				if pgid <= 0 || signal != syscall.SIGKILL || index >= len(test.group) {
					return invalidGroupSignalErr
				}
				return test.group[index]
			}
			deps.signalLeader = func(pid int, signal syscall.Signal) error {
				log.add("leader")
				index := int(leaderCalls.Add(1)) - 1
				if pid <= 0 || signal != syscall.SIGKILL || index >= len(test.leader) {
					return invalidLeaderSignalErr
				}
				if test.live {
					if err := syscall.Kill(pid, 0); err != nil {
						return fmt.Errorf("%w: pid %d: %w", hiddenGroupKillErr, pid, err)
					}
				}
				if test.leader[index] != nil {
					return test.leader[index]
				}
				return syscall.Kill(pid, signal)
			}
			deps.waitCommand = func(command *exec.Cmd) error { waits.Add(1); log.add("cmd-wait"); return command.Wait() }
			stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, test.mode, test.arg), deps)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			started := time.Now()
			waitErr := stream.Wait()
			for _, unexpected := range []error{hiddenGroupKillErr, invalidGroupSignalErr, invalidLeaderSignalErr} {
				if errors.Is(waitErr, unexpected) {
					t.Fatalf("error=%v includes unexpected cleanup failure %v", waitErr, unexpected)
				}
			}
			for _, want := range test.want {
				if !errors.Is(waitErr, want) {
					t.Fatalf("error=%v omits %v", waitErr, want)
				}
			}
			if len(test.want) == 0 && waitErr != nil {
				t.Fatalf("unexpected error=%v", waitErr)
			}
			if got := log.snapshot(); !slices.Equal(got, test.events) {
				t.Fatalf("events=%q want=%q", got, test.events)
			}
			if int(groupCalls.Load()) != len(test.group) || int(leaderCalls.Load()) != len(test.leader) || waits.Load() != 1 {
				t.Fatalf("calls group=%d leader=%d wait=%d", groupCalls.Load(), leaderCalls.Load(), waits.Load())
			}
			if test.live && time.Since(started) > 2*time.Second {
				t.Fatalf("EPERM fallback was not bounded: %v", time.Since(started))
			}
		})
	}
}
func TestStreamingLifecycleWaitDoneCancelAreOnceAndCached(t *testing.T) {
	var waits atomic.Int32
	deps := defaultStreamingLifecycleDeps()
	baseWait := deps.waitCommand
	deps.waitCommand = func(command *exec.Cmd) error { waits.Add(1); return baseWait(command) }
	stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	results := make(chan error, 8)
	var cancels sync.WaitGroup
	for index := 0; index < cap(results); index++ {
		go func() { results <- stream.Wait() }()
		cancels.Add(1)
		go func() { defer cancels.Done(); stream.Cancel() }()
	}
	first, ok := awaitLifecycleValue(t, stream.Done())
	if !ok || !errors.Is(first, context.Canceled) {
		t.Fatalf("Done=%v open=%v", first, ok)
	}
	for index := 0; index < cap(results); index++ {
		if result := awaitLifecycleResult(t, results); !errors.Is(result, context.Canceled) {
			t.Fatalf("Wait=%v", result)
		}
	}
	cancelsJoined := make(chan struct{})
	go func() { cancels.Wait(); close(cancelsJoined) }()
	awaitLifecycleResult(t, cancelsJoined)
	if _, ok = awaitLifecycleValue(t, stream.Done()); ok {
		t.Fatal("Done published twice")
	}
	stream.Cancel()
	if !errors.Is(stream.Wait(), context.Canceled) || waits.Load() != 1 {
		t.Fatalf("cached=%v waits=%d", stream.Wait(), waits.Load())
	}
}

type lifecycleEventLog struct {
	mu     sync.Mutex
	events []string
}

func (log *lifecycleEventLog) add(event string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.events = append(log.events, event)
}
func (log *lifecycleEventLog) snapshot() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.events...)
}

type testLifecycleObserver struct {
	waitErr, closeErr error
	log               *lifecycleEventLog
	entered, release  chan struct{}
}

func (observer *testLifecycleObserver) Wait() error {
	if observer.log != nil {
		observer.log.add("observer-wait")
	}
	if observer.entered != nil {
		close(observer.entered)
		<-observer.release
	}
	return observer.waitErr
}
func (observer *testLifecycleObserver) Close() error {
	if observer.log != nil {
		observer.log.add("observer-close")
	}
	return observer.closeErr
}
func lifecycleCommand(t *testing.T, mode string, args ...string) *exec.Cmd {
	t.Helper()
	arguments := append([]string{"-test.run=^TestStreamingLifecycleProcessHelper$", "--", "lifecycle-helper", mode}, args...)
	return exec.CommandContext(context.Background(), os.Args[0], arguments...)
}
func lifecycleTreeCommand(t *testing.T, mode string) (*exec.Cmd, string) {
	t.Helper()
	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	command := lifecycleCommand(t, mode, pidFile)
	t.Cleanup(func() {
		if pid := readLifecyclePID(pidFile); pid > 1 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	return command, pidFile
}
func TestStreamingLifecycleProcessHelper(t *testing.T) {
	index := slices.Index(os.Args, "lifecycle-helper")
	if index < 0 {
		return
	}
	args := os.Args[index+1:]
	switch args[0] {
	case "exit":
		code, _ := strconv.Atoi(args[1])
		os.Exit(code)
	case "sigkill":
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	case "sleep":
		delay, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(delay) * time.Second)
	case "touch":
		_ = os.WriteFile(args[1], nil, 0o600)
	case "buffered":
		_, _ = fmt.Fprintln(os.Stdout, "one\ntwo")
		_, _ = fmt.Fprintln(os.Stderr, "three")
	case "long-stdout", "long-stderr":
		output := os.Stdout
		if args[0] == "long-stderr" {
			output = os.Stderr
		}
		_, _ = fmt.Fprintln(output, strings.Repeat("x", 1024*1024+1))
	case "lines-stdout", "lines-stderr":
		output := os.Stdout
		if args[0] == "lines-stderr" {
			output = os.Stderr
		}
		count, _ := strconv.Atoi(args[1])
		for line := 0; line < count; line++ {
			_, _ = fmt.Fprintf(output, "line-%d\n", line)
		}
	case "group", "escape":
		child := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestStreamingLifecycleProcessHelper$", "--", "lifecycle-helper", "sleep", "30")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if args[0] == "escape" {
			child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		}
		if child.Start() != nil {
			os.Exit(120)
		}
		if os.WriteFile(args[1], []byte(strconv.Itoa(child.Process.Pid)), 0o600) != nil {
			os.Exit(121)
		}
	}
	os.Exit(0)
}
func collectLifecycleOutput(t *testing.T, output <-chan string) []string {
	t.Helper()
	var lines []string
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line, ok := <-output:
			if !ok {
				slices.Sort(lines)
				return lines
			}
			lines = append(lines, line)
		case <-timer.C:
			t.Fatal("lifecycle output did not close")
			return nil
		}
	}
}
func lifecycleAction(action func()) <-chan struct{} {
	done := make(chan struct{})
	go func() { action(); close(done) }()
	return done
}
func lifecycleCall(call func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- call() }()
	return done
}
func awaitLifecycleResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle result timeout")
	}
	var zero T
	return zero
}
func awaitLifecycleValue(t *testing.T, result <-chan error) (error, bool) {
	t.Helper()
	select {
	case value, ok := <-result:
		return value, ok
	case <-time.After(3 * time.Second):
		t.Fatal("lifecycle result timeout")
	}
	return nil, false
}
func waitLifecyclePID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pid := readLifecyclePID(path); pid > 1 {
			return pid
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("pid not recorded")
	return 0
}
func readLifecyclePID(path string) int {
	data, _ := os.ReadFile(path)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}
func waitLifecycleProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) != nil
}
