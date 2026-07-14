package runner

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	ErrStreamingOutputScan      = errors.New("streaming output scan failed")
	ErrStreamingOutputUndrained = errors.New("streaming output was not drained")
	errInvalidStreamingContext  = errors.New("invalid streaming context")
)

const (
	streamingOutputCapacity = 100
	streamingMaxLineBytes   = 1024 * 1024
	streamingPipeCloseDelay = time.Second
)

type streamingLifecycle struct {
	ctx      context.Context
	pgid     int
	output   chan string
	done     chan error
	complete chan struct{}
	stop     chan struct{}
	readers  [2]*os.File

	mu           sync.Mutex
	fault        error
	contextCause error
	result       error
	finished     bool
	closingPipes bool
	killOnce     sync.Once
}

func startStreamingLifecycle(ctx context.Context, command *exec.Cmd) (*StreamingCmd, error) {
	if ctx == nil {
		return nil, errInvalidStreamingContext
	}
	if err := ctx.Err(); err != nil {
		return nil, streamingContextCause(ctx)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		return nil, err
	}
	closePipes := func() {
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		_ = stderrRead.Close()
		_ = stderrWrite.Close()
	}
	command.Stdout, command.Stderr = stdoutWrite, stderrWrite
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		closePipes()
		return nil, err
	}
	_ = stdoutWrite.Close()
	_ = stderrWrite.Close()
	pgid := command.Process.Pid
	lifecycle := &streamingLifecycle{
		ctx: ctx, pgid: pgid, output: make(chan string, streamingOutputCapacity),
		done: make(chan error, 1), complete: make(chan struct{}), stop: make(chan struct{}),
		readers: [2]*os.File{stdoutRead, stderrRead},
	}
	stream := &StreamingCmd{Cmd: command, Output: lifecycle.output, Done: lifecycle.done, stream: lifecycle}
	lifecycle.run(command)
	return stream, nil
}

func (l *streamingLifecycle) run(command *exec.Cmd) {
	var streams sync.WaitGroup
	streams.Add(len(l.readers))
	for _, reader := range l.readers {
		go func(source *os.File) {
			defer streams.Done()
			defer func() { _ = source.Close() }()
			scanner := bufio.NewScanner(source)
			scanner.Buffer(make([]byte, 0, 64*1024), streamingMaxLineBytes)
			for scanner.Scan() {
				if l.abnormal() {
					return
				}
				select {
				case l.output <- scanner.Text():
				default:
					if !l.abnormal() {
						l.fail(ErrStreamingOutputUndrained)
					}
					return
				}
			}
			if scanner.Err() != nil && !l.pipesClosing() {
				l.fail(ErrStreamingOutputScan)
			}
		}(reader)
	}
	streamsDone := make(chan struct{})
	go func() {
		streams.Wait()
		close(l.output)
		close(streamsDone)
	}()
	processDone := make(chan error, 1)
	go func() {
		waitErr := command.Wait()
		l.killGroup()
		processDone <- waitErr
	}()
	go l.watchContext()
	go func() {
		waitErr := <-processDone
		timer := time.NewTimer(streamingPipeCloseDelay)
		select {
		case <-streamsDone:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			l.closeReaders()
			<-streamsDone
		}
		l.finish(waitErr)
	}()
}

func (l *streamingLifecycle) cancel(cause error) {
	l.mu.Lock()
	accepted := !l.finished && l.fault == nil && l.contextCause == nil
	if accepted {
		l.contextCause = cause
	}
	l.mu.Unlock()
	if accepted {
		l.killGroup()
	}
}

func (l *streamingLifecycle) fail(fault error) {
	l.mu.Lock()
	accepted := !l.finished && l.fault == nil && l.contextCause == nil
	if accepted {
		l.fault = fault
	}
	l.mu.Unlock()
	if accepted {
		l.killGroup()
	}
}

func (l *streamingLifecycle) abnormal() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.finished || l.fault != nil || l.contextCause != nil
}

func (l *streamingLifecycle) watchContext() {
	select {
	case <-l.ctx.Done():
		l.cancel(streamingContextCause(l.ctx))
	case <-l.stop:
	}
}

func (l *streamingLifecycle) finish(processErr error) {
	if l.ctx.Err() != nil {
		l.cancel(streamingContextCause(l.ctx))
	}
	l.mu.Lock()
	if l.fault != nil {
		l.result = l.fault
	} else if l.contextCause != nil {
		l.result = l.contextCause
	} else {
		l.result = processErr
	}
	l.finished = true
	result := l.result
	l.mu.Unlock()
	l.done <- result
	close(l.done)
	close(l.complete)
	close(l.stop)
}

func (l *streamingLifecycle) wait() error {
	<-l.complete
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.result
}

func (l *streamingLifecycle) killGroup() {
	l.killOnce.Do(func() {
		if err := syscall.Kill(-l.pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return
		}
	})
}

func (l *streamingLifecycle) closeReaders() {
	l.mu.Lock()
	l.closingPipes = true
	l.mu.Unlock()
	for _, reader := range l.readers {
		_ = reader.Close()
	}
}

func (l *streamingLifecycle) pipesClosing() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closingPipes
}

func streamingContextCause(ctx context.Context) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return ctx.Err()
}
