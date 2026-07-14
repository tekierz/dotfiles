//go:build darwin || linux

package runner

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	errInvalidStreamingLifecycle = errors.New("invalid streaming lifecycle")
	errStreamingScanFailure      = errors.New("streaming scan failure")
	errStreamingOutputOverflow   = errors.New("streaming output overflow")
)

type streamingLifecycleDeps struct {
	newExitObserver func(int) (streamingExitObserver, error)
	signalGroup     func(int, syscall.Signal) error
	waitCommand     func(*exec.Cmd) error
	after           func(time.Duration) <-chan time.Time
	pipeCloseDelay  time.Duration
}

func defaultStreamingLifecycleDeps() streamingLifecycleDeps {
	return streamingLifecycleDeps{
		newExitObserver: newStreamingExitObserver,
		signalGroup:     func(pgid int, signal syscall.Signal) error { return syscall.Kill(-pgid, signal) },
		waitCommand:     func(command *exec.Cmd) error { return command.Wait() },
		after:           time.After,
		pipeCloseDelay:  100 * time.Millisecond,
	}
}

type streamingLifecycle struct {
	output   chan string
	done     chan error
	complete chan struct{}
	cancel   func(error)
	result   error
}

func (lifecycle *streamingLifecycle) Output() <-chan string { return lifecycle.output }
func (lifecycle *streamingLifecycle) Done() <-chan error    { return lifecycle.done }
func (lifecycle *streamingLifecycle) Cancel()               { lifecycle.cancel(context.Canceled) }
func (lifecycle *streamingLifecycle) Wait() error           { <-lifecycle.complete; return lifecycle.result }
func startStreamingLifecycle(ctx context.Context, command *exec.Cmd) (*streamingLifecycle, error) {
	return startStreamingLifecycleWithDeps(ctx, command, defaultStreamingLifecycleDeps())
}
func startStreamingLifecycleWithDeps(ctx context.Context, command *exec.Cmd, deps streamingLifecycleDeps) (*streamingLifecycle, error) {
	if ctx == nil || command == nil || command.Process != nil || command.ProcessState != nil {
		return nil, errInvalidStreamingLifecycle
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, err
	}
	closePipes := func() { _ = stdout.Close(); _ = stderr.Close() }
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		closePipes()
		return nil, err
	}
	pgid := command.Process.Pid
	observer, err := deps.newExitObserver(pgid)
	if err != nil {
		_ = deps.signalGroup(pgid, syscall.SIGKILL)
		closePipes()
		_ = deps.waitCommand(command)
		return nil, err
	}
	lifecycle := &streamingLifecycle{output: make(chan string, 100), done: make(chan error, 1), complete: make(chan struct{})}
	trigger := make(chan struct{})
	var causeMu sync.Mutex
	causeSet, firstCause := false, error(nil)
	setCause := func(cause error) {
		causeMu.Lock()
		if !causeSet {
			causeSet, firstCause = true, cause
			close(trigger)
		} else if firstCause == nil && cause != nil {
			firstCause = cause
		}
		causeMu.Unlock()
	}
	lifecycle.cancel = setCause
	readersDone, readers := make(chan struct{}), new(sync.WaitGroup)
	readers.Add(2)
	readPipe := func(pipe io.ReadCloser) {
		defer readers.Done()
		defer func() { _ = pipe.Close() }()
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case lifecycle.output <- scanner.Text():
			case <-trigger:
				return
			default:
				setCause(errStreamingOutputOverflow)
				return
			}
		}
		if scanner.Err() != nil {
			select {
			case <-trigger:
			default:
				setCause(errStreamingScanFailure)
			}
		}
	}
	go readPipe(stdout)
	go readPipe(stderr)
	go func() { readers.Wait(); close(readersDone) }()
	observerDone := make(chan struct{})
	go func() {
		setCause(observer.Wait())
		close(observerDone)
	}()
	go func() {
		select {
		case <-ctx.Done():
			setCause(context.Cause(ctx))
		case <-lifecycle.complete:
		}
	}()
	go func() {
		<-trigger
		causeMu.Lock()
		natural := firstCause == nil
		causeMu.Unlock()
		if natural && ctx.Done() != nil {
			select {
			case <-ctx.Done():
				setCause(context.Cause(ctx))
			case <-deps.after(deps.pipeCloseDelay):
			}
		}
		signalErr := deps.signalGroup(pgid, syscall.SIGKILL)
		for errors.Is(signalErr, syscall.EINTR) {
			signalErr = deps.signalGroup(pgid, syscall.SIGKILL)
		}
		if errors.Is(signalErr, syscall.ESRCH) {
			signalErr = nil
		}
		select {
		case <-readersDone:
		case <-deps.after(deps.pipeCloseDelay):
			closePipes()
			<-readersDone
		}
		close(lifecycle.output)
		for range lifecycle.output {
		}
		<-observerDone
		closeErr, waitErr := observer.Close(), deps.waitCommand(command)
		naturalWait := naturalWaitError(waitErr)
		if errors.Is(signalErr, syscall.EPERM) && (waitErr == nil || naturalWait != nil) {
			signalErr = nil
		}
		causeMu.Lock()
		result := firstCause
		causeMu.Unlock()
		for _, candidate := range []error{signalErr, closeErr, naturalWait} {
			if result == nil {
				result = candidate
			}
		}
		lifecycle.result = result
		lifecycle.done <- result
		close(lifecycle.done)
		close(lifecycle.complete)
	}()
	return lifecycle, nil
}
func naturalWaitError(err error) error {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ProcessState == nil {
		return err
	}
	status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
	if ok && status.Signaled() && status.Signal() == syscall.SIGKILL {
		return nil
	}
	return err
}
