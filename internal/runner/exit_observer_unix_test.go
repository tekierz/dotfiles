//go:build darwin || linux

package runner

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
)

func TestNativeStreamingExitObserverDoesNotReapLeader(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 7")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	observer, err := newStreamingExitObserver(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		t.Fatalf("create observer: %v", err)
	}
	defer func() { _ = observer.Close() }()
	if err := observer.Wait(); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		t.Fatalf("observe leader exit: %v", err)
	}
	if cmd.ProcessState != nil {
		t.Fatal("observer reaped the leader through exec.Cmd.Wait")
	}
	var exitErr *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("leader was not waitable with its real status after observation: %v", err)
	}
}

func TestStreamingLifecycleSignalsAnchoredGroupBeforeSoleWait(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	deps := defaultStreamingLifecycleDeps()
	signals := 0
	deps.signalGroup = func(pgid int, signal syscall.Signal) error {
		if cmd.ProcessState != nil {
			t.Error("process group was signaled after exec.Cmd.Wait reaped the anchor")
		}
		if signal != syscall.SIGKILL {
			t.Errorf("cleanup signal=%v, want SIGKILL", signal)
		}
		signals++
		return syscall.Kill(-pgid, signal)
	}
	command, err := startStreamingLifecycleWithDeps(context.Background(), cmd, deps)
	if err != nil {
		t.Fatalf("start lifecycle: %v", err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("natural completion: %v", err)
	}
	if signals != 1 {
		t.Fatalf("cleanup signals=%d, want exactly one before Wait", signals)
	}
}

func TestStreamingLifecycleSignalErrorPolicy(t *testing.T) {
	tests := []struct {
		name      string
		failures  []error
		wantError error
		wantCalls int
	}{
		{name: "retry-eintr", failures: []error{syscall.EINTR, syscall.ESRCH}, wantCalls: 2},
		{name: "esrch-benign", failures: []error{syscall.ESRCH}, wantCalls: 1},
		{name: "eperm-terminal", failures: []error{syscall.EPERM}, wantError: syscall.EPERM, wantCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", "exit 0")
			calls := 0
			deps := defaultStreamingLifecycleDeps()
			deps.newExitObserver = func(int) (streamingExitObserver, error) {
				return &fakeExitObserver{wait: func() error { return nil }}, nil
			}
			deps.signalGroup = func(int, syscall.Signal) error {
				result := test.failures[calls]
				calls++
				return result
			}
			command, err := startStreamingLifecycleWithDeps(context.Background(), cmd, deps)
			if err != nil {
				t.Fatalf("start lifecycle: %v", err)
			}
			waitErr := command.Wait()
			if !errors.Is(waitErr, test.wantError) || calls != test.wantCalls {
				t.Fatalf("terminal error=%v calls=%d, want error=%v calls=%d", waitErr, calls, test.wantError, test.wantCalls)
			}
		})
	}
}

func TestStreamingLifecycleObserverFailuresStillCleanAnchoredGroup(t *testing.T) {
	observerErr := errors.New("observer wait failed")
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	signals := 0
	deps := defaultStreamingLifecycleDeps()
	deps.newExitObserver = func(int) (streamingExitObserver, error) {
		return &fakeExitObserver{wait: func() error { return observerErr }}, nil
	}
	deps.signalGroup = func(int, syscall.Signal) error {
		signals++
		if cmd.ProcessState != nil {
			t.Error("observer failure cleanup ran after reap")
		}
		return syscall.ESRCH
	}
	command, err := startStreamingLifecycleWithDeps(context.Background(), cmd, deps)
	if err != nil {
		t.Fatalf("start lifecycle: %v", err)
	}
	if waitErr := command.Wait(); !errors.Is(waitErr, observerErr) || signals != 1 {
		t.Fatalf("terminal error=%v signals=%d, want observer error and one cleanup", waitErr, signals)
	}
}

func TestStreamingLifecycleObserverSetupFailureCleansAndReaps(t *testing.T) {
	setupErr := errors.New("observer setup failed")
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	signals := 0
	deps := defaultStreamingLifecycleDeps()
	deps.newExitObserver = func(int) (streamingExitObserver, error) { return nil, setupErr }
	deps.signalGroup = func(pgid int, signal syscall.Signal) error {
		signals++
		if cmd.ProcessState != nil {
			t.Error("observer setup cleanup ran after reap")
		}
		return syscall.Kill(-pgid, signal)
	}
	command, err := startStreamingLifecycleWithDeps(context.Background(), cmd, deps)
	if command != nil || !errors.Is(err, setupErr) || signals != 1 || cmd.ProcessState == nil {
		t.Fatalf("command=%v error=%v signals=%d state=%v, want cleaned/reaped setup failure", command, err, signals, cmd.ProcessState)
	}
}

func TestDefaultStreamingLifecycleDependenciesAreComplete(t *testing.T) {
	deps := defaultStreamingLifecycleDeps()
	if deps.newExitObserver == nil || deps.signalGroup == nil || deps.pipeCloseDelay <= 0 {
		t.Fatalf("incomplete default lifecycle dependencies: %+v", deps)
	}
}

type fakeExitObserver struct {
	wait  func() error
	close func() error
}

func (observer *fakeExitObserver) Wait() error {
	return observer.wait()
}

func (observer *fakeExitObserver) Close() error {
	if observer.close != nil {
		return observer.close()
	}
	return nil
}
