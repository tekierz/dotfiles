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
	preCanceledPath := filepath.Join(t.TempDir(), "must-not-exist")

	badPipe := lifecycleCommand(t, "exit", "0")
	badPipe.Stdout = io.Discard
	for _, test := range []struct {
		name          string
		ctx           context.Context
		cmd           *exec.Cmd
		want          error
		wantAny       bool
		mustNotCreate string
	}{
		{name: "nil-context", ctx: nilContext, cmd: lifecycleCommand(t, "exit", "0"), want: errInvalidStreamingLifecycle},
		{name: "nil-command", ctx: context.Background(), want: errInvalidStreamingLifecycle},
		{name: "already-started", ctx: context.Background(), cmd: started, want: errInvalidStreamingLifecycle},
		{name: "pre-canceled", ctx: canceled, cmd: lifecycleCommand(t, "touch", preCanceledPath), want: context.Canceled, mustNotCreate: preCanceledPath},
		{name: "pipe-setup", ctx: context.Background(), cmd: badPipe, wantAny: true},
		{name: "start", ctx: context.Background(), cmd: exec.CommandContext(context.Background(), filepath.Join(t.TempDir(), "missing")), wantAny: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := startStreamingLifecycle(test.ctx, test.cmd)
			if stream != nil || err == nil || !test.wantAny && !errors.Is(err, test.want) {
				t.Fatalf("start returned stream=%v error=%v, want %v", stream, err, test.want)
			}
			if test.mustNotCreate != "" {
				if test.cmd.Process != nil {
					t.Fatalf("pre-canceled command started process %d", test.cmd.Process.Pid)
				}
				if _, statErr := os.Stat(test.mustNotCreate); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("pre-canceled command touched %q: %v", test.mustNotCreate, statErr)
				}
			}
		})
	}
}

func TestStreamingLifecycleSetupFailureReapsStartedChildOnce(t *testing.T) {
	observerErr := errors.New("observer setup")
	var signals, waits atomic.Int32
	deps := defaultStreamingLifecycleDeps()
	deps.newExitObserver = func(int) (streamingExitObserver, error) { return nil, observerErr }
	deps.signalGroup = func(pgid int, signal syscall.Signal) error {
		signals.Add(1)
		return syscall.Kill(-pgid, signal)
	}
	deps.waitCommand = func(command *exec.Cmd) error {
		waits.Add(1)
		return command.Wait()
	}
	stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
	if stream != nil || !errors.Is(err, observerErr) {
		t.Fatalf("setup returned stream=%v error=%v", stream, err)
	}
	if signals.Load() != 1 || waits.Load() != 1 {
		t.Fatalf("setup cleanup signals=%d waits=%d, want 1/1", signals.Load(), waits.Load())
	}
}

func TestStreamingLifecycleNaturalExitAndNonzeroStatus(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
		want int
	}{
		{name: "success", code: "0"},
		{name: "nonzero", code: "17", want: 17},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := startStreamingLifecycle(context.Background(), lifecycleCommand(t, "exit", test.code))
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			for range stream.Output() {
			}
			waitErr := stream.Wait()
			var exitErr *exec.ExitError
			if test.want == 0 && waitErr != nil || test.want != 0 && (!errors.As(waitErr, &exitErr) || exitErr.ExitCode() != test.want) {
				t.Fatalf("Wait error=%v, want exit %d", waitErr, test.want)
			}
		})
	}
}

func TestStreamingLifecycleOwnsNaturalOrphanAndCancellation(t *testing.T) {
	for _, mode := range []string{"natural-orphan", "cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			command, pidFile := lifecycleTreeCommand(t, "group")
			ctx, expire := context.WithCancelCause(context.Background())
			defer expire(context.Canceled)
			stream, err := startStreamingLifecycle(ctx, command)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			pid := waitLifecyclePID(t, pidFile)
			if mode == "cancel" {
				stream.Cancel()
			} else if mode == "deadline" {
				expire(context.DeadlineExceeded)
			}
			waitErr := awaitLifecycleDone(t, stream.Done())
			if mode == "cancel" && !errors.Is(waitErr, context.Canceled) || mode == "deadline" && !errors.Is(waitErr, context.DeadlineExceeded) || mode == "natural-orphan" && waitErr != nil {
				t.Fatalf("terminal error=%v", waitErr)
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
			stream, err := startStreamingLifecycle(context.Background(), lifecycleHelperCommand(t, test.mode))
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if strings.Contains(test.name, "scan") {
				for range stream.Output() {
				}
			}
			waitErr := awaitLifecycleDone(t, stream.Done())
			if !errors.Is(waitErr, test.want) || errors.Is(waitErr, context.Canceled) {
				t.Fatalf("terminal error=%v, want exclusive %v", waitErr, test.want)
			}
		})
	}
}

func TestStreamingLifecycleBoundsEscapedPipeHolder(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel-%v", cancel), func(t *testing.T) {
			command, pidFile := lifecycleTreeCommand(t, "escape")
			deps := defaultStreamingLifecycleDeps()
			deps.pipeCloseDelay = 25 * time.Millisecond
			timerRequested := make(chan time.Duration, 1)
			timerRelease := make(chan time.Time, 1)
			deps.after = func(delay time.Duration) <-chan time.Time {
				timerRequested <- delay
				return timerRelease
			}
			stream, err := startStreamingLifecycleWithDeps(context.Background(), command, deps)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			pid := waitLifecyclePID(t, pidFile)
			if cancel {
				stream.Cancel()
			}
			select {
			case delay := <-timerRequested:
				if delay != deps.pipeCloseDelay {
					t.Fatalf("pipe-close delay=%v, want %v", delay, deps.pipeCloseDelay)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("bounded pipe-close timer was not requested")
			}
			select {
			case result := <-stream.Done():
				t.Fatalf("completed before pipe-close bound: %v", result)
			default:
			}
			timerRelease <- time.Now()
			waitErr := awaitLifecycleDone(t, stream.Done())
			if cancel && !errors.Is(waitErr, context.Canceled) || !cancel && waitErr != nil {
				t.Fatalf("escaped-holder result: %v", waitErr)
			}
			assertLifecycleOutputClosed(t, stream.Output())
			if waitLifecycleProcessGone(pid, 0) {
				t.Fatal("escaped holder was signaled outside the owned process group")
			}
		})
	}
}

func TestStreamingLifecycleJoinsBoundedReaderBeforeWaitReturns(t *testing.T) {
	command, pidFile := lifecycleTreeCommand(t, "escape")
	deps := defaultStreamingLifecycleDeps()
	deps.pipeCloseDelay = 25 * time.Millisecond
	timerRequested := make(chan time.Duration, 1)
	timerRelease := make(chan time.Time, 1)
	deps.after = func(delay time.Duration) <-chan time.Time {
		timerRequested <- delay
		return timerRelease
	}
	stream, err := startStreamingLifecycleWithDeps(context.Background(), command, deps)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := waitLifecyclePID(t, pidFile)
	select {
	case delay := <-timerRequested:
		if delay != deps.pipeCloseDelay {
			t.Fatalf("pipe-close delay=%v, want %v", delay, deps.pipeCloseDelay)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bounded pipe-close timer was not requested")
	}
	waitResult := lifecycleCall(stream.Wait)
	select {
	case result := <-waitResult:
		t.Fatalf("Wait completed before bounded reader teardown: %v", result)
	default:
	}
	timerRelease <- time.Now()
	if waitErr := awaitLifecycleDone(t, waitResult); waitErr != nil {
		t.Fatalf("Wait after bounded reader teardown: %v", waitErr)
	}
	assertLifecycleOutputClosed(t, stream.Output())
	if waitLifecycleProcessGone(pid, 0) {
		t.Fatal("escaped holder was signaled outside the owned process group")
	}
}

func TestStreamingLifecycleFirstCauseIsSynchronous(t *testing.T) {
	observerErr := errors.New("observer wait")
	for _, observerFirst := range []bool{false, true} {
		name := map[bool]string{false: "cancel-first", true: "observer-first"}[observerFirst]
		t.Run(name, func(t *testing.T) {
			observer := newControlledLifecycleObserver(observerErr)
			cleanupEntered, releaseCleanup := make(chan struct{}), make(chan struct{})
			var cleanupOnce sync.Once
			deps := defaultStreamingLifecycleDeps()
			deps.newExitObserver = func(int) (streamingExitObserver, error) { return observer, nil }
			deps.signalGroup = func(pgid int, signal syscall.Signal) error {
				cleanupOnce.Do(func() { close(cleanupEntered) })
				<-releaseCleanup
				return syscall.Kill(-pgid, signal)
			}
			stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			awaitLifecycleSignal(t, observer.entered)
			if observerFirst {
				close(observer.release)
				awaitLifecycleSignal(t, cleanupEntered)
				awaitLifecycleSignal(t, lifecycleAction(stream.Cancel))
			} else {
				awaitLifecycleSignal(t, lifecycleAction(stream.Cancel))
				awaitLifecycleSignal(t, cleanupEntered)
				close(observer.release)
			}
			close(releaseCleanup)
			waitErr := stream.Wait()
			want := error(context.Canceled)
			if observerFirst {
				want = observerErr
			}
			if !errors.Is(waitErr, want) {
				t.Fatalf("Wait error=%v, want first cause %v", waitErr, want)
			}
		})
	}
}

func TestStreamingLifecycleCleanupOrderAndSignalErrors(t *testing.T) {
	observerErr := errors.New("observer wait")
	closeErr := errors.New("observer close")
	overCallErr := errors.New("unexpected signal over-call")
	for _, test := range []struct {
		name       string
		observer   error
		close      error
		signals    []error
		want       error
		wantEvents []string
	}{
		{name: "eintr-esrch", signals: []error{syscall.EINTR, syscall.ESRCH}, wantEvents: []string{"observer-wait", "signal", "signal", "observer-close", "cmd-wait"}},
		{name: "cleanup-eperm", signals: []error{syscall.EPERM}, want: syscall.EPERM, wantEvents: []string{"observer-wait", "signal", "observer-close", "cmd-wait"}},
		{name: "observer-precedes-cleanup", observer: observerErr, signals: []error{syscall.EPERM}, want: observerErr, wantEvents: []string{"observer-wait", "signal", "observer-close", "cmd-wait"}},
		{name: "close-after-cleanup", close: closeErr, signals: []error{syscall.ESRCH}, want: closeErr, wantEvents: []string{"observer-wait", "signal", "observer-close", "cmd-wait"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			log := &lifecycleEventLog{}
			observer := &loggedLifecycleObserver{waitErr: test.observer, closeErr: test.close, log: log}
			deps := defaultStreamingLifecycleDeps()
			deps.newExitObserver = func(int) (streamingExitObserver, error) { return observer, nil }
			var attempts atomic.Int32
			deps.signalGroup = func(pgid int, signal syscall.Signal) error {
				log.add("signal")
				attempt := int(attempts.Add(1)) - 1
				if attempt >= len(test.signals) {
					return overCallErr
				}
				err := test.signals[attempt]
				if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM) {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				}
				return err
			}
			deps.waitCommand = func(command *exec.Cmd) error { log.add("cmd-wait"); return command.Wait() }
			stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			waitErr := stream.Wait()
			if test.want == nil && waitErr != nil || test.want != nil && !errors.Is(waitErr, test.want) {
				t.Fatalf("Wait error=%v, want %v", waitErr, test.want)
			}
			if got := log.snapshot(); !slices.Equal(got, test.wantEvents) {
				t.Fatalf("events=%q, want %q", got, test.wantEvents)
			}
			if got := int(attempts.Load()); got != len(test.signals) {
				t.Fatalf("signal attempts=%d, want %d", got, len(test.signals))
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
		go func() {
			defer cancels.Done()
			stream.Cancel()
		}()
	}
	first, ok := awaitLifecycleValue(t, stream.Done())
	if !ok || !errors.Is(first, context.Canceled) {
		t.Fatalf("Done result=%v open=%v", first, ok)
	}
	for index := 0; index < cap(results); index++ {
		if result := awaitLifecycleDone(t, results); !errors.Is(result, context.Canceled) {
			t.Fatalf("cached Wait result=%v", result)
		}
	}
	cancelsJoined := make(chan struct{})
	go func() {
		cancels.Wait()
		close(cancelsJoined)
	}()
	awaitLifecycleSignal(t, cancelsJoined)
	if _, ok = awaitLifecycleValue(t, stream.Done()); ok {
		t.Fatal("Done published more than one result")
	}
	stream.Cancel()
	if !errors.Is(stream.Wait(), context.Canceled) || waits.Load() != 1 {
		t.Fatalf("post-completion Wait=%v wait calls=%d", stream.Wait(), waits.Load())
	}
}

type controlledLifecycleObserver struct {
	waitErr  error
	entered  chan struct{}
	release  chan struct{}
	closeOne sync.Once
}

func newControlledLifecycleObserver(waitErr error) *controlledLifecycleObserver {
	return &controlledLifecycleObserver{waitErr: waitErr, entered: make(chan struct{}), release: make(chan struct{})}
}

func (observer *controlledLifecycleObserver) Wait() error {
	close(observer.entered)
	<-observer.release
	return observer.waitErr
}
func (observer *controlledLifecycleObserver) Close() error {
	observer.closeOne.Do(func() {})
	return nil
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

type loggedLifecycleObserver struct {
	waitErr, closeErr error
	log               *lifecycleEventLog
}

func (observer *loggedLifecycleObserver) Wait() error {
	observer.log.add("observer-wait")
	return observer.waitErr
}
func (observer *loggedLifecycleObserver) Close() error {
	observer.log.add("observer-close")
	return observer.closeErr
}

func lifecycleCommand(t *testing.T, mode string, args ...string) *exec.Cmd {
	t.Helper()
	return exec.CommandContext(context.Background(), os.Args[0], append([]string{"-test.run=^TestStreamingLifecycleProcessHelper$", "--", "lifecycle-helper", mode}, args...)...)
}

func lifecycleHelperCommand(t *testing.T, mode string) *exec.Cmd {
	return lifecycleCommand(t, mode, "1000")
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
	case "sleep":
		delay, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(delay) * time.Second)
	case "touch":
		_ = os.WriteFile(args[1], nil, 0o600)
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
		if err := child.Start(); err != nil {
			os.Exit(120)
		}
		if err := os.WriteFile(args[1], []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			os.Exit(121)
		}
	}
}

func awaitLifecycleSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle signal timeout")
	}
}

func lifecycleAction(action func()) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		action()
		close(done)
	}()
	return done
}

func lifecycleCall(call func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- call() }()
	return done
}

func assertLifecycleOutputClosed(t *testing.T, output <-chan string) {
	t.Helper()
	select {
	case value, ok := <-output:
		if ok {
			t.Fatalf("output remained readable after terminal publication: %q", value)
		}
	default:
		t.Fatal("output remained open after terminal publication")
	}
}

func awaitLifecycleDone(t *testing.T, result <-chan error) error {
	value, _ := awaitLifecycleValue(t, result)
	return value
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
	t.Fatal("descendant pid not recorded")
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
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) != nil
}
