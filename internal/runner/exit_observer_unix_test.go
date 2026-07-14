//go:build darwin || linux

package runner

import (
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestStreamingExitObserverRejectsInvalidOrMissingProcess(t *testing.T) {
	for _, pid := range []int{-1, 0, 1 << 30} {
		observer, err := newStreamingExitObserver(pid)
		if observer != nil {
			_ = observer.Close()
		}
		if err == nil || observer != nil {
			t.Fatalf("pid %d returned observer=%v error=%v", pid, observer, err)
		}
	}
}

func TestStreamingExitObserverBlocksUntilRealExit(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	observer, err := newStreamingExitObserver(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("create observer: %v", err)
	}
	defer func() { _ = observer.Close() }()
	result := make(chan error, 1)
	go func() { result <- observer.Wait() }()
	select {
	case err := <-result:
		t.Fatalf("Wait returned before process exit: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("observe killed process: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not observe real process exit")
	}
}

func TestStreamingExitObserverPreservesExactWaitStatus(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 7")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	observer, err := newStreamingExitObserver(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		t.Fatalf("create observer: %v", err)
	}
	defer func() { _ = observer.Close() }()
	if err := observer.Wait(); err != nil {
		t.Fatalf("observe exit: %v", err)
	}
	if cmd.ProcessState != nil {
		t.Fatal("observer consumed exec.Cmd wait authority")
	}
	var exitErr *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("sole cmd.Wait error=%v, want exact exit 7", err)
	}
}

func TestStreamingExitObserverCloseIsIdempotentAndUnblocksWait(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	observer, err := newStreamingExitObserver(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := observer.Close(); err != nil {
		t.Fatalf("idempotent Close: %v", err)
	}
	result := make(chan error, 1)
	go func() { result <- observer.Wait() }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Wait after Close reported a process exit")
		}
	case <-time.After(time.Second):
		t.Fatal("Wait remained blocked on a released observer")
	}
}
