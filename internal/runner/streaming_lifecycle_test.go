//go:build darwin || linux

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestStreamingLifecycleRejectsCallerSysProcAttrBeforeMutation(t *testing.T) {
	marker := t.TempDir() + "/started"
	command := lifecycleCommand(t, "touch", marker)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stream, err := startStreamingLifecycle(context.Background(), command)
	if stream != nil || !errors.Is(err, errInvalidStreamingLifecycle) {
		t.Fatalf("stream=%v error=%v", stream, err)
	}
	if command.Process != nil {
		t.Fatalf("rejected command started process %d", command.Process.Pid)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected command mutated marker: %v", err)
	}
}

func TestSignalStreamingGroupUsesOneNegativePGIDCallAndNativeErrno(t *testing.T) {
	want := syscall.EPERM
	var calls atomic.Int32
	err := signalStreamingGroup(41, syscall.SIGKILL, func(pid int, signal syscall.Signal) error {
		calls.Add(1)
		if pid != -41 || signal != syscall.SIGKILL {
			t.Fatalf("kill(%d, %v), want (-41, SIGKILL)", pid, signal)
		}
		return want
	})
	if err != want || calls.Load() != 1 { //nolint:errorlint // The native errno must be returned unchanged by the one-call wrapper.
		t.Fatalf("error=%v calls=%d, want native EPERM once", err, calls.Load())
	}
}

func TestTerminateStreamingProcessTracksActualDelivery(t *testing.T) {
	for _, test := range []struct {
		name            string
		group, leader   []error
		wantDelivery    bool
		wantErr         error
		wantGroupCalls  int
		wantLeaderCalls int
	}{
		{name: "group-delivered", group: []error{nil}, wantDelivery: true, wantGroupCalls: 1},
		{name: "group-gone", group: []error{syscall.ESRCH}, wantGroupCalls: 1},
		{name: "eperm-leader-esrch", group: []error{syscall.EPERM}, leader: []error{syscall.ESRCH}, wantGroupCalls: 1, wantLeaderCalls: 1},
		{name: "eperm-leader-delivered", group: []error{syscall.EPERM}, leader: []error{nil}, wantDelivery: true, wantErr: syscall.EPERM, wantGroupCalls: 1, wantLeaderCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps, groupCalls, leaderCalls := lifecycleSignalDeps(test.group, test.leader)
			delivered, err := terminateStreamingProcess(73, deps)
			if delivered != test.wantDelivery || !sameLifecycleError(err, test.wantErr) {
				t.Fatalf("delivered=%v error=%v", delivered, err)
			}
			if *groupCalls != test.wantGroupCalls || *leaderCalls != test.wantLeaderCalls {
				t.Fatalf("group calls=%d leader calls=%d", *groupCalls, *leaderCalls)
			}
		})
	}
}

func TestStreamingLifecyclePreservesNaturalExitAndSignal(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		exit int
		sig  syscall.Signal
	}{
		{name: "exit-17", args: []string{"exit", "17"}, exit: 17},
		{name: "natural-sigkill", args: []string{"sigkill"}, sig: syscall.SIGKILL},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream, err := startStreamingLifecycle(context.Background(), lifecycleCommand(t, test.args...))
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			waitErr := stream.Wait()
			var exitErr *exec.ExitError
			if !errors.As(waitErr, &exitErr) || waitErr != exitErr { //nolint:errorlint // Natural status must retain exact identity.
				t.Fatalf("error=%T %v, want exact *exec.ExitError", waitErr, waitErr)
			}
			status := exitErr.Sys().(syscall.WaitStatus)
			if test.exit != 0 && exitErr.ExitCode() != test.exit || test.sig != 0 && status.Signal() != test.sig {
				t.Fatalf("status=%v exit=%d signal=%v", status, exitErr.ExitCode(), status.Signal())
			}
		})
	}
}

func TestStreamingLifecycleSuppressesOnlyDeliveredSIGKILL(t *testing.T) {
	stream, err := startStreamingLifecycle(context.Background(), lifecycleCommand(t, "sleep", "30"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	stream.Cancel()
	waitErr := stream.Wait()
	if !errors.Is(waitErr, context.Canceled) {
		t.Fatalf("error=%v, want cancellation", waitErr)
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		t.Fatalf("delivered SIGKILL leaked wait error: %v", waitErr)
	}
	status := stream.command.ProcessState.Sys().(syscall.WaitStatus)
	if status.Signal() != syscall.SIGKILL {
		t.Fatalf("actual terminal signal=%v, want SIGKILL", status.Signal())
	}
}

func TestStreamingLifecycleJoinsObserverAndContextWorkers(t *testing.T) {
	observer := &blockingLifecycleObserver{entered: make(chan struct{}), closed: make(chan struct{})}
	exits, release := make(chan string, 2), make(chan struct{})
	deps := defaultStreamingLifecycleDeps()
	deps.newExitObserver = func(int) (streamingExitObserver, error) { return observer, nil }
	deps.onWorkerExit = func(worker string) { exits <- worker; <-release }
	stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	awaitLifecycle(t, observer.entered)
	stream.Cancel()
	workers := []string{awaitLifecycle(t, exits), awaitLifecycle(t, exits)}
	slices.Sort(workers)
	if !slices.Equal(workers, []string{"context", "observer"}) {
		t.Fatalf("worker exits=%q", workers)
	}
	waited := lifecycleResult(stream.Wait)
	select {
	case err := <-waited:
		t.Fatalf("Wait returned before workers joined: %v", err)
	default:
	}
	close(release)
	if err := awaitLifecycle(t, waited); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestStreamingLifecycleSetupRollbackRetriesAndFallsBack(t *testing.T) {
	setupErr := errors.New("observer setup")
	deps, groupCalls, leaderCalls := lifecycleSignalDeps(
		[]error{syscall.EINTR, syscall.EPERM}, []error{syscall.EINTR, nil},
	)
	deps.newExitObserver = func(int) (streamingExitObserver, error) { return nil, setupErr }
	baseLeader := deps.signalLeader
	deps.signalLeader = func(pid int, signal syscall.Signal) error {
		err := baseLeader(pid, signal)
		if err == nil {
			return syscall.Kill(pid, signal)
		}
		return err
	}
	var waits atomic.Int32
	baseWait := deps.waitCommand
	deps.waitCommand = func(command *exec.Cmd) error { waits.Add(1); return baseWait(command) }
	stream, err := startStreamingLifecycleWithDeps(context.Background(), lifecycleCommand(t, "sleep", "30"), deps)
	if stream != nil || !errors.Is(err, setupErr) || !errors.Is(err, syscall.EPERM) {
		t.Fatalf("stream=%v error=%v", stream, err)
	}
	if *groupCalls != 2 || *leaderCalls != 2 || waits.Load() != 1 {
		t.Fatalf("group=%d leader=%d waits=%d", *groupCalls, *leaderCalls, waits.Load())
	}
}

func TestStreamingLifecycleRetainsOutputAndCachesCompletion(t *testing.T) {
	stream, err := startStreamingLifecycle(context.Background(), lifecycleCommand(t, "output"))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if first, second := stream.Wait(), stream.Wait(); first != nil || second != nil {
		t.Fatalf("cached wait: %v / %v", first, second)
	}
	var output []string
	for line := range stream.Output() {
		output = append(output, line)
	}
	slices.Sort(output)
	if !slices.Equal(output, []string{"stderr", "stdout"}) {
		t.Fatalf("output=%q", output)
	}
	if done, ok := awaitLifecycleValue(t, stream.Done()); !ok || done != nil {
		t.Fatalf("Done=%v open=%v", done, ok)
	}
}

type blockingLifecycleObserver struct {
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (observer *blockingLifecycleObserver) Wait() error {
	close(observer.entered)
	<-observer.closed
	return errStreamingExitObserverClosed
}
func (observer *blockingLifecycleObserver) Close() error {
	observer.once.Do(func() { close(observer.closed) })
	return nil
}

func lifecycleSignalDeps(group, leader []error) (streamingLifecycleDeps, *int, *int) {
	deps := defaultStreamingLifecycleDeps()
	groupCalls, leaderCalls := 0, 0
	deps.signalGroup = func(int, syscall.Signal) error {
		result := group[groupCalls]
		groupCalls++
		return result
	}
	deps.signalLeader = func(int, syscall.Signal) error {
		result := leader[leaderCalls]
		leaderCalls++
		return result
	}
	return deps, &groupCalls, &leaderCalls
}

func sameLifecycleError(got, want error) bool {
	return got == nil && want == nil || want != nil && errors.Is(got, want)
}

func lifecycleCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	argv := append([]string{"-test.run=^TestStreamingLifecycleProcessHelper$", "--", "lifecycle-helper"}, args...)
	return exec.CommandContext(context.Background(), os.Args[0], argv...)
}

func TestStreamingLifecycleProcessHelper(t *testing.T) {
	for index, arg := range os.Args {
		if arg != "lifecycle-helper" {
			continue
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
		case "output":
			_, _ = fmt.Fprintln(os.Stdout, "stdout")
			_, _ = fmt.Fprintln(os.Stderr, "stderr")
		}
		os.Exit(0)
	}
}

func lifecycleResult(call func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- call() }()
	return done
}

func awaitLifecycle[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("lifecycle timeout")
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
		t.Fatal("lifecycle timeout")
	}
	return nil, false
}
