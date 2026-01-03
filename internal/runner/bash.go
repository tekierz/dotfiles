package runner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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
