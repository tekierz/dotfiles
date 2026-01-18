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
type Runner struct {
	ScriptPath string
	OS         string
	Theme      string
	NavStyle   string
	SkipBackup bool
}

// NewRunner creates a new bash runner
func NewRunner() *Runner {
	return &Runner{
		ScriptPath: "bin/dotfiles-setup",
		Theme:      "catppuccin-mocha",
		NavStyle:   "emacs",
	}
}

// NeedsSudo returns true if the current OS requires sudo for package installation
func NeedsSudo() bool {
	// Check if we're on Linux (macOS uses Homebrew which doesn't need sudo)
	cmd := exec.Command("uname", "-s")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "Linux"
}

// CheckSudoCached returns true if sudo credentials are already cached
func CheckSudoCached() bool {
	cmd := exec.Command("sudo", "-n", "true")
	return cmd.Run() == nil
}

// CacheSudoCredentials prompts for sudo password and caches credentials
// This should be called with stdin/stdout connected to the terminal
func CacheSudoCredentials() error {
	cmd := exec.Command("sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// DetectOS runs the detect_os function and returns the result
func (r *Runner) DetectOS() (string, error) {
	cmd := exec.Command("bash", "-c", fmt.Sprintf(`
		source %s
		detect_os
	`, r.ScriptPath))

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// RunFunction executes a specific function from the bash script
func (r *Runner) RunFunction(funcName string, args ...string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	// Build the bash command
	bashCmd := fmt.Sprintf(`
		export OS=%q
		export THEME=%q
		export NAV_STYLE=%q
		export SKIP_PROMPTS=true
		export SKIP_BACKUP=%t
		source %s
		%s %s
	`, r.OS, r.Theme, r.NavStyle, r.SkipBackup, r.ScriptPath, funcName, strings.Join(args, " "))

	cmd := exec.Command("bash", "-c", bashCmd)
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return nil, nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		return nil, nil, nil, err
	}

	return cmd, stdout, stderr, nil
}

// StreamOutput reads output from a pipe and sends it to a channel
func StreamOutput(pipe io.ReadCloser, ch chan<- OutputLine, source string) {
	defer pipe.Close()
	scanner := bufio.NewScanner(pipe)

	for scanner.Scan() {
		line := scanner.Text()
		ch <- OutputLine{
			Text:   line,
			Type:   classifyLine(line),
			Source: source,
		}
	}
}

// classifyLine determines the type of output line
func classifyLine(line string) OutputType {
	// Remove ANSI escape codes for classification
	cleaned := stripANSI(line)

	switch {
	case strings.Contains(cleaned, "━━━━━━"):
		return OutputHeader
	case strings.HasPrefix(cleaned, "▶") || strings.Contains(line, "▶"):
		return OutputStep
	case strings.HasPrefix(cleaned, "✓") || strings.Contains(line, "✓"):
		return OutputSuccess
	case strings.HasPrefix(cleaned, "⚠") || strings.Contains(line, "⚠"):
		return OutputWarning
	case strings.HasPrefix(cleaned, "✗") || strings.Contains(line, "✗"):
		return OutputError
	default:
		return OutputNormal
	}
}

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Skip escape sequence
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !isANSITerminator(s[i]) {
					i++
				}
				if i < len(s) {
					i++ // Skip terminator
				}
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

func isANSITerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// RunSetup runs the full installation process
func (r *Runner) RunSetup() (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	args := []string{
		"--yes",
		"--theme", r.Theme,
		"--" + r.NavStyle,
	}
	if r.SkipBackup {
		args = append(args, "--no-backup")
	}

	cmd := exec.Command(r.ScriptPath, args...)
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdout.Close()
		return nil, nil, nil, err
	}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
		return nil, nil, nil, err
	}

	return cmd, stdout, stderr, nil
}

// ListThemes returns the list of available themes
func (r *Runner) ListThemes() []string {
	return []string{
		"catppuccin-mocha",
		"catppuccin-latte",
		"catppuccin-frappe",
		"catppuccin-macchiato",
		"dracula",
		"gruvbox-dark",
		"gruvbox-light",
		"nord",
		"tokyo-night",
		"solarized-dark",
		"solarized-light",
		"monokai",
		"rose-pine",
	}
}

// ListBackups runs the list_backups function
func (r *Runner) ListBackups() ([]string, error) {
	cmd := exec.Command(r.ScriptPath, "--list-backups")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	var backups []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "20") { // Backup names start with timestamp
			parts := strings.Fields(line)
			if len(parts) > 0 {
				backups = append(backups, parts[0])
			}
		}
	}

	return backups, nil
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
		stdout.Close()
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdout.Close()
		stderr.Close()
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
		defer pipe.Close()
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
