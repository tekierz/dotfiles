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

const (
	streamingOutputCapacity = 128
	streamingPipeGrace      = 50 * time.Millisecond
	streamingTokenLimit     = 1024 * 1024
)

type streamingLifecycleDeps struct {
	newExitObserver func(int) (streamingExitObserver, error)
	signalGroup     func(int, syscall.Signal) error
	signalLeader    func(int, syscall.Signal) error
	waitCommand     func(*exec.Cmd) error
	onWorkerExit    func(string)
}

func defaultStreamingLifecycleDeps() streamingLifecycleDeps {
	return streamingLifecycleDeps{
		newExitObserver: newStreamingExitObserver,
		signalGroup: func(pgid int, signal syscall.Signal) error {
			return signalStreamingGroup(pgid, signal, syscall.Kill)
		},
		signalLeader: syscall.Kill,
		waitCommand:  func(command *exec.Cmd) error { return command.Wait() },
		onWorkerExit: func(string) {},
	}
}

type streamingLifecycle struct {
	command  *exec.Cmd
	output   chan string
	done     chan error
	finished chan struct{}
	cancel   context.CancelCauseFunc
	result   error
}

type completedStreamingExitObserver struct{}

func (completedStreamingExitObserver) Wait() error  { return nil }
func (completedStreamingExitObserver) Close() error { return nil }

func (lifecycle *streamingLifecycle) Output() <-chan string { return lifecycle.output }
func (lifecycle *streamingLifecycle) Done() <-chan error    { return lifecycle.done }
func (lifecycle *streamingLifecycle) Cancel()               { lifecycle.cancel(context.Canceled) }
func (lifecycle *streamingLifecycle) Wait() error {
	<-lifecycle.finished
	return lifecycle.result
}

func startStreamingLifecycle(ctx context.Context, command *exec.Cmd) (*streamingLifecycle, error) {
	return startStreamingLifecycleWithDeps(ctx, command, defaultStreamingLifecycleDeps())
}

func startStreamingLifecycleWithDeps(ctx context.Context, command *exec.Cmd, deps streamingLifecycleDeps) (*streamingLifecycle, error) {
	if ctx == nil || command == nil || command.Process != nil || command.SysProcAttr != nil {
		return nil, errInvalidStreamingLifecycle
	}
	if err := context.Cause(ctx); err != nil {
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
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	observer, err := deps.newExitObserver(command.Process.Pid)
	if errors.Is(err, syscall.ESRCH) {
		observer, err = completedStreamingExitObserver{}, nil
	}
	if err != nil {
		delivered, cleanupErr := terminateStreamingProcess(command.Process.Pid, deps)
		_ = stdout.Close()
		_ = stderr.Close()
		waitErr := deps.waitCommand(command)
		if delivered && isStreamingSIGKILL(waitErr) {
			waitErr = nil
		}
		return nil, joinStreamingErrors(err, cleanupErr, waitErr)
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	lifecycle := &streamingLifecycle{
		command: command, output: make(chan string, streamingOutputCapacity),
		done: make(chan error, 1), finished: make(chan struct{}), cancel: cancel,
	}
	go lifecycle.run(runCtx, observer, stdout, stderr, deps)
	return lifecycle, nil
}

type streamingCause struct {
	err     error
	natural bool
}

func (lifecycle *streamingLifecycle) run(ctx context.Context, observer streamingExitObserver, stdout, stderr io.ReadCloser, deps streamingLifecycleDeps) {
	causes := make(chan streamingCause, 4)
	stopWorkers := make(chan struct{})
	stopReaders := make(chan struct{})
	var workers, readers sync.WaitGroup

	workers.Add(2)
	go func() {
		defer workers.Done()
		defer deps.onWorkerExit("observer")
		err := observer.Wait()
		causes <- streamingCause{err: err, natural: err == nil}
	}()
	go func() {
		defer workers.Done()
		defer deps.onWorkerExit("context")
		select {
		case <-ctx.Done():
			causes <- streamingCause{err: context.Cause(ctx)}
		case <-stopWorkers:
		}
	}()

	readPipe := func(pipe io.ReadCloser) {
		defer readers.Done()
		defer func() { _ = pipe.Close() }()
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(make([]byte, 64*1024), streamingTokenLimit)
		for scanner.Scan() {
			select {
			case lifecycle.output <- scanner.Text():
			case <-stopReaders:
				return
			default:
				causes <- streamingCause{err: errStreamingOutputOverflow}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case causes <- streamingCause{err: errors.Join(errStreamingScanFailure, err)}:
			case <-stopReaders:
			}
		}
	}
	readers.Add(2)
	go readPipe(stdout)
	go readPipe(stderr)

	cause := <-causes
	delivered, cleanupErr := terminateStreamingProcess(lifecycle.command.Process.Pid, deps)
	// A successful leader fallback after native group EPERM cannot have delivered
	// to a leader the no-reap observer already proved exited. Preserve its natural status.
	if cause.natural && cleanupErr == syscall.EPERM { //nolint:errorlint // Only a lone native EPERM identifies the observed-leader fallback.
		delivered, cleanupErr = false, nil
	}
	closeErr := observer.Close()
	close(stopWorkers)
	workers.Wait()
	lifecycle.cancel(context.Canceled)

	readersDone := make(chan struct{})
	go func() { readers.Wait(); close(readersDone) }()
	select {
	case <-readersDone:
	case <-time.After(streamingPipeGrace):
		close(stopReaders)
		_ = stdout.Close()
		_ = stderr.Close()
		<-readersDone
	}
	close(lifecycle.output)
	waitErr := deps.waitCommand(lifecycle.command)
	if !cause.natural && delivered && isStreamingSIGKILL(waitErr) {
		waitErr = nil
	}
	lifecycle.result = joinStreamingErrors(cause.err, cleanupErr, closeErr, waitErr)
	lifecycle.done <- lifecycle.result
	close(lifecycle.done)
	close(lifecycle.finished)
}

func signalStreamingGroup(pgid int, signal syscall.Signal, kill func(int, syscall.Signal) error) error {
	if pgid <= 0 || kill == nil {
		return errInvalidStreamingLifecycle
	}
	return kill(-pgid, signal)
}

func terminateStreamingProcess(pid int, deps streamingLifecycleDeps) (bool, error) {
	groupErr := retryStreamingSignal(func() error { return deps.signalGroup(pid, syscall.SIGKILL) })
	switch {
	case groupErr == nil:
		return true, nil
	case errors.Is(groupErr, syscall.ESRCH):
		return false, nil
	case !errors.Is(groupErr, syscall.EPERM):
		return false, groupErr
	}
	leaderErr := retryStreamingSignal(func() error { return deps.signalLeader(pid, syscall.SIGKILL) })
	delivered := leaderErr == nil
	if errors.Is(leaderErr, syscall.ESRCH) {
		return false, nil
	}
	return delivered, joinStreamingErrors(groupErr, leaderErr)
}

func retryStreamingSignal(signal func() error) error {
	for {
		err := signal()
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

func isStreamingSIGKILL(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ProcessState == nil {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && status.Signal() == syscall.SIGKILL
}

func joinStreamingErrors(errs ...error) error {
	var present []error
	for _, err := range errs {
		if err != nil {
			present = append(present, err)
		}
	}
	if len(present) == 1 {
		return present[0]
	}
	return errors.Join(present...)
}
