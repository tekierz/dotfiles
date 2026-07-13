package runner

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

var (
	errInvalidExactStreamingRequest = errors.New("invalid exact streaming request")
	errExactStreamingOutputOverflow = errors.New("exact streaming output overflow")
)

// ExactStreamingRequest describes one process without PATH or shell lookup.
type ExactStreamingRequest struct {
	Path string
	Args []string
	Env  []string
	Dir  string
}

// RunExactStreaming validates and clones one exact process request before it
// starts the process and streams its stdout and stderr.
func RunExactStreaming(ctx context.Context, request ExactStreamingRequest) (*StreamingCmd, error) {
	if ctx == nil {
		return nil, errInvalidExactStreamingRequest
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validExactPath(request.Path) || !validExactPath(request.Dir) || !validExactStrings(request.Args) || !validExactEnvironment(request.Env) {
		return nil, errInvalidExactStreamingRequest
	}
	args := append([]string(nil), request.Args...)
	environment := append([]string(nil), request.Env...)
	childContext, cancel := context.WithCancel(ctx)
	// #nosec G204 -- Path is validated as an exact clean absolute path; Args are literal argv.
	command := exec.CommandContext(childContext, request.Path, args...)
	command.Env = environment
	command.Dir = request.Dir
	command.Stdin = nil
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return killExactProcessGroup(command) }
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		cancel()
		return nil, err
	}
	if err := command.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		cancel()
		return nil, err
	}
	output := make(chan string, 100)
	done := make(chan error, 1)
	var outcome sync.Mutex
	overflowed := false
	overflow := func() {
		outcome.Lock()
		first := !overflowed
		overflowed = true
		outcome.Unlock()
		if first {
			cancel()
			_ = killExactProcessGroup(command)
		}
	}
	var streams sync.WaitGroup
	streams.Add(2)
	copyOutput := func(source io.ReadCloser) {
		defer streams.Done()
		defer func() { _ = source.Close() }()
		scanner := bufio.NewScanner(source)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case output <- scanner.Text():
			case <-childContext.Done():
				return
			default:
				overflow()
				return
			}
		}
		if scanner.Err() != nil && childContext.Err() == nil {
			overflow()
		}
	}
	go copyOutput(stdout)
	go copyOutput(stderr)
	go func() {
		streams.Wait()
		close(output)
		waitErr := command.Wait()
		outcome.Lock()
		didOverflow := overflowed
		outcome.Unlock()
		if didOverflow {
			waitErr = errExactStreamingOutputOverflow
		} else if childContext.Err() != nil {
			waitErr = childContext.Err()
		}
		done <- waitErr
		close(done)
	}()
	return &StreamingCmd{Cmd: command, Output: output, Done: done, cancel: cancel}, nil
}

func killExactProcessGroup(command *exec.Cmd) error {
	if command == nil || command.Process == nil || command.Process.Pid <= 0 {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func validExactPath(value string) bool {
	return value != "" && !strings.ContainsRune(value, 0) && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func validExactStrings(values []string) bool {
	for _, value := range values {
		if strings.ContainsRune(value, 0) {
			return false
		}
	}
	return true
}

func validExactEnvironment(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key, _, ok := strings.Cut(value, "=")
		folded := strings.ToLower(key)
		if !ok || key == "" || strings.ContainsRune(value, 0) {
			return false
		}
		if _, duplicate := seen[folded]; duplicate {
			return false
		}
		seen[folded] = struct{}{}
	}
	return true
}
