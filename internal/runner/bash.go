package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
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
	wait   func() error
}

// Cancel stops the running command
func (s *StreamingCmd) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Wait blocks until the command completes and returns the error (if any)
func (s *StreamingCmd) Wait() error {
	if s.wait != nil {
		return s.wait()
	}
	return <-s.Done
}

// RunStreaming executes a command and streams output line-by-line
// Returns a StreamingCmd that provides channels for output and completion
func RunStreaming(ctx context.Context, name string, args ...string) (*StreamingCmd, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	// Connect stdin to /dev/null to prevent commands from hanging waiting for input
	cmd.Stdin = nil
	lifecycle, err := startStreamingLifecycle(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}
	return &StreamingCmd{
		Cmd:    cmd,
		Output: lifecycle.Output(),
		Done:   lifecycle.Done(),
		cancel: lifecycle.Cancel,
		wait:   lifecycle.Wait,
	}, nil
}

// RunStreamingWithSudo executes one accepted package-manager command through
// the privileged supervisor and streams output. The sudo credentials must be
// cached before calling this function; sudo is always invoked non-interactively.
func RunStreamingWithSudo(ctx context.Context, name string, args ...string) (*StreamingCmd, error) {
	if ctx == nil {
		return nil, errInvalidPrivilegedRequest
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	target, err := validatePrivilegedTarget(name)
	if err != nil {
		return nil, errInvalidPrivilegedRequest
	}
	sudo, err := findTrustedPrivilegedExecutable("sudo")
	if err != nil {
		return nil, errPrivilegedSupervisorUnavailable
	}
	supervisor, err := currentPrivilegedSupervisorExecutable()
	if err != nil {
		return nil, errPrivilegedSupervisorUnavailable
	}
	if !validPrivilegedArguments(args) || !validPrivilegedCommand(target, args) {
		return nil, errInvalidPrivilegedRequest
	}

	sudoArgs := []string{"-n", "--", supervisor, privilegedSupervisorDispatchArg, target}
	sudoArgs = append(sudoArgs, append([]string(nil), args...)...)
	// #nosec G204 -- sudo, supervisor, and target are exact validated absolute
	// paths; arguments remain literal argv entries and never enter a shell.
	command := exec.Command(sudo, sudoArgs...)
	command.Env = privilegedLauncherEnvironment()
	command.Dir = "/"
	control, err := command.StdinPipe()
	if err != nil {
		return nil, errPrivilegedSupervisorUnavailable
	}

	var closeOnce sync.Once
	closeControl := func() {
		closeOnce.Do(func() { _ = control.Close() })
	}
	deps := defaultStreamingLifecycleDeps()
	// The unprivileged parent cannot signal the sudo-owned process group. Closing
	// the inherited control pipe asks the root supervisor to kill and reap its
	// own exact descendant group instead.
	deps.signalGroup = func(int, syscall.Signal) error {
		closeControl()
		return nil
	}
	deps.signalLeader = func(int, syscall.Signal) error {
		closeControl()
		return nil
	}
	lifecycle, err := startStreamingLifecycleWithDeps(ctx, command, deps)
	if err != nil {
		closeControl()
		return nil, fmt.Errorf("start privileged command: %w", err)
	}
	cancel := func() {
		closeControl()
		lifecycle.Cancel()
	}
	return &StreamingCmd{
		Cmd: command, Output: lifecycle.Output(), Done: lifecycle.Done(),
		cancel: cancel, wait: lifecycle.Wait,
	}, nil
}
