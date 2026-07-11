package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// OutputLine represents a line of output from the bash script
type OutputLine struct {
	Text   string
	Type   OutputType
	Source string // stdout or stderr
}

// OutputType categorizes output lines
type OutputType int

const (
	OutputNormal OutputType = iota
	OutputHeader
	OutputStep
	OutputSuccess
	OutputWarning
	OutputError
)

// Runner executes bash functions and captures output
type Runner struct{}

const probeTimeout = 5 * time.Second

// NewRunner creates a new bash runner
func NewRunner() *Runner {
	return &Runner{}
}

// NeedsSudo returns true if the current OS requires sudo for package installation
func NeedsSudo() bool {
	// Check if we're on Linux (macOS uses Homebrew which doesn't need sudo)
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "uname", "-s")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "Linux"
}

// CheckSudoCached returns true if sudo credentials are already cached
func CheckSudoCached() bool {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "-n", "true")
	return cmd.Run() == nil
}

// RefreshSudo extends the cached sudo timestamp WITHOUT prompting. It runs
// `sudo -n -v`, which refreshes the credential cache when it is still valid and
// fails (rather than prompting) once it has expired. It is used by the install
// keep-alive loop so a long non-interactive install does not hit an expired
// sudo timestamp mid-run (C16). stdin is left detached so it can never block
// waiting for a password.
func RefreshSudo() error {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "-n", "-v")
	cmd.Stdin = nil
	return cmd.Run()
}

// StreamingCmd wraps an exec.Cmd with real-time output streaming
type StreamingCmd struct {
	Cmd    *exec.Cmd
	Output <-chan string
	Done   <-chan error
	cancel context.CancelFunc
}

// Cancel stops the running command
func (s *StreamingCmd) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Wait blocks until the command completes and returns the error (if any)
func (s *StreamingCmd) Wait() error {
	return <-s.Done
}

// RunStreaming executes a command and streams output line-by-line
// Returns a StreamingCmd that provides channels for output and completion
func RunStreaming(ctx context.Context, name string, args ...string) (*StreamingCmd, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	// Connect stdin to /dev/null to prevent commands from hanging waiting for input
	cmd.Stdin = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		cancel()
		return nil, fmt.Errorf("start command: %w", err)
	}

	outputCh := make(chan string, 100)
	doneCh := make(chan error, 1)

	// Stream stdout and stderr concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	streamPipe := func(pipe io.ReadCloser) {
		defer wg.Done()
		defer func() { _ = pipe.Close() }()
		scanner := bufio.NewScanner(pipe)
		// Increase buffer size for long lines (package manager output can be verbose)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case outputCh <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}

	go streamPipe(stdout)
	go streamPipe(stderr)

	// Wait for command to finish and close channels
	go func() {
		wg.Wait()
		close(outputCh)
		doneCh <- cmd.Wait()
		close(doneCh)
	}()

	return &StreamingCmd{
		Cmd:    cmd,
		Output: outputCh,
		Done:   doneCh,
		cancel: cancel,
	}, nil
}

// RunStreamingWithSudo executes a command with sudo and streams output
// The sudo credentials should be cached before calling this function
func RunStreamingWithSudo(ctx context.Context, name string, args ...string) (*StreamingCmd, error) {
	sudoArgs := append([]string{name}, args...)
	return RunStreaming(ctx, "sudo", sudoArgs...)
}
