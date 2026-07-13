package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tekierz/dotfiles/internal/runner"
)

// PacmanManager implements PackageManager for Arch Linux (pacman/paru)
type PacmanManager struct {
	useParu  bool // Use paru for AUR support
	identity ExecutableIdentity
	state    executableResolutionState
}

// NewPacmanManager creates a new pacman manager
func NewPacmanManager(preferParu bool) *PacmanManager {
	return newPacmanManager(preferParu, exec.LookPath)
}

func newPacmanManager(preferParu bool, lookup executableLookup) *PacmanManager {
	manager := &PacmanManager{}

	if preferParu {
		resolution := resolveManagerExecutable("paru", lookup)
		if resolution.state == executableResolutionValid {
			manager.identity = resolution.identity
			manager.state = resolution.state
			manager.useParu = true
			return manager
		}
		if resolution.state != executableResolutionMissing {
			manager.state = resolution.state
			return manager
		}
	}

	resolution := resolveManagerExecutable("pacman", lookup)
	manager.identity = resolution.identity
	manager.state = resolution.state
	return manager
}

func (p *PacmanManager) ExecutableIdentity() (ExecutableIdentity, bool) {
	if p == nil || p.state != executableResolutionValid || !validExecutableIdentity(p.identity) {
		return ExecutableIdentity{}, false
	}
	return p.identity, true
}

func (p *PacmanManager) executablePath() string {
	identity, ok := p.ExecutableIdentity()
	if !ok {
		return ""
	}
	return identity.invocationPath
}

func (p PacmanManager) String() string {
	if p.useParu {
		return "paru_manager"
	}
	return "pacman_manager"
}

func (p PacmanManager) GoString() string {
	return p.String()
}

func (p *PacmanManager) Name() string {
	if p.useParu {
		return "paru"
	}
	return "pacman"
}

func (p *PacmanManager) IsAvailable() bool {
	return p.executablePath() != ""
}

func (p *PacmanManager) Install(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	if p.executablePath() == "" {
		return errPackageManagerUnavailable
	}

	args := []string{"-S", "--noconfirm", "--needed"}
	args = append(args, packages...)

	if p.useParu {
		cmd, cancel := packageCommand(packageMutationTimeout, p.executablePath(), args...)
		defer cancel()
		return cmd.Run()
	}
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", append([]string{p.executablePath()}, args...)...)
	defer cancel()
	return cmd.Run()
}

func (p *PacmanManager) Uninstall(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	if p.executablePath() == "" {
		return errPackageManagerUnavailable
	}

	args := []string{"-R", "--noconfirm"}
	args = append(args, packages...)
	if p.useParu {
		cmd, cancel := packageCommand(packageMutationTimeout, p.executablePath(), args...)
		defer cancel()
		return cmd.Run()
	}
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", append([]string{p.executablePath()}, args...)...)
	defer cancel()
	return cmd.Run()
}

func (p *PacmanManager) IsInstalled(pkg string) bool {
	if p.executablePath() == "" {
		return false
	}
	return p.IsInstalledContext(context.Background(), pkg)
}

func (p *PacmanManager) IsInstalledContext(ctx context.Context, pkg string) bool {
	if p.executablePath() == "" {
		return false
	}
	cmd, cancel := packageCommandWithContext(ctx, packageQueryTimeout, p.executablePath(), "-Q", pkg)
	defer cancel()
	return cmd.Run() == nil
}

func (p *PacmanManager) GetVersion(pkg string) (string, error) {
	if p.executablePath() == "" {
		return "", errPackageManagerUnavailable
	}
	cmd, cancel := packageCommand(packageQueryTimeout, p.executablePath(), "-Q", pkg)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("package %s not installed", pkg)
	}

	// Output format: "package-name version"
	parts := strings.Fields(out.String())
	if len(parts) >= 2 {
		return parts[1], nil
	}

	return "", fmt.Errorf("could not parse version for %s", pkg)
}

func (p *PacmanManager) CheckOutdated() ([]Package, error) {
	if p.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	// Use checkupdates for official repos (safer, doesn't require root)
	var packages []Package

	// Check official repo updates.
	officialOut, err := p.checkOfficialUpdates()
	if err != nil {
		return nil, err
	}
	packages = append(packages, parsePacmanUpdates(officialOut, "pacman")...)

	// Check AUR updates if using paru
	if p.useParu {
		aurCmd, cancel := packageCommand(packageQueryTimeout, p.executablePath(), "-Qua")
		defer cancel()
		var aurOut bytes.Buffer
		aurCmd.Stdout = &aurOut
		// `pacman -Qua` exits non-zero when there are no foreign updates, so
		// the error is intentionally ignored and we parse whatever it emits.
		_ = aurCmd.Run()

		packages = append(packages, parsePacmanUpdates(aurOut.String(), "aur")...)
	}

	return packages, nil
}

// checkOfficialUpdates returns the raw "name oldver -> newver" lines describing
// pending official-repo updates. It prefers the `checkupdates` helper (from the
// optional pacman-contrib package, which queries a private sync DB without
// root), and falls back to `pacman -Qu` when checkupdates is not installed so a
// missing optional dependency does not silently report "up to date".
func (p *PacmanManager) checkOfficialUpdates() (string, error) {
	if p.executablePath() == "" {
		return "", errPackageManagerUnavailable
	}
	if _, err := exec.LookPath("checkupdates"); err == nil {
		cmd, cancel := packageCommand(packageRefreshTimeout, "checkupdates")
		defer cancel()
		var out bytes.Buffer
		cmd.Stdout = &out

		// checkupdates exits 2 when there are no updates (not an error for us)
		// and 0 when updates are available. Any other exit code is a genuine
		// failure (e.g. a stale temp DB) that should be surfaced.
		err := cmd.Run()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
				return "", nil
			}
			return "", fmt.Errorf("checkupdates failed: %w", err)
		}
		return out.String(), nil
	}

	// Fallback: `pacman -Qu` reads the local sync DB and works without the
	// pacman-contrib package. It exits 1 with empty stdout when nothing is
	// outdated, and may still print benign diagnostics to stderr. Other
	// failures (DB lock, corrupt sync DB) also exit non-zero and write
	// diagnostics to stderr and/or use a different exit code. Distinguish the
	// genuine no-updates case from a real error so failures aren't silently
	// reported as "up to date".
	cmd, cancel := packageCommand(packageQueryTimeout, p.executablePath(), "-Qu")
	defer cancel()
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return classifyPacmanQuResult(exitCode, out.String(), errBuf.String(), err)
	}
	return out.String(), nil
}

func classifyPacmanQuResult(exitCode int, stdout, stderr string, runErr error) (string, error) {
	if runErr == nil {
		return stdout, nil
	}

	stderrText := filterPacmanLocalNewerWarnings(stderr)
	if hasPacmanDatabaseFailure(stderrText) {
		return "", pacmanQuFailure(runErr, stderrText)
	}

	if exitCode == 1 {
		if hasPacmanErrorMarker(stderrText) {
			return "", pacmanQuFailure(runErr, stderrText)
		}
		if strings.TrimSpace(stdout) == "" {
			return "", nil
		}
		return stdout, nil
	}

	return "", pacmanQuFailure(runErr, stderrText)
}

func pacmanQuFailure(err error, stderr string) error {
	if stderr != "" {
		return fmt.Errorf("pacman -Qu failed: %w: %s", err, stderr)
	}
	return fmt.Errorf("pacman -Qu failed: %w", err)
}

func filterPacmanLocalNewerWarnings(stderr string) string {
	var remaining []string
	for _, line := range strings.Split(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isPacmanLocalNewerWarning(trimmed) {
			continue
		}
		remaining = append(remaining, trimmed)
	}
	return strings.Join(remaining, "\n")
}

func hasPacmanDatabaseFailure(stderr string) bool {
	lower := strings.ToLower(stderr)
	markers := []string{
		"could not lock database",
		"unable to lock database",
		"/var/lib/pacman/db.lck",
		"failed to synchronize all databases",
		"invalid or corrupted database",
		"invalid or corrupt database",
		"corrupted database",
		"corrupt database",
		"database is incorrect version",
		"could not parse package description file",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}

	return (strings.Contains(lower, "database file") && strings.Contains(lower, "does not exist")) ||
		(strings.Contains(lower, "could not open file") && strings.Contains(lower, "/var/lib/pacman/sync/"))
}

func hasPacmanErrorMarker(stderr string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "error:") {
			return true
		}
	}
	return false
}

func isPacmanLocalNewerWarning(line string) bool {
	if !strings.HasPrefix(line, "warning: ") {
		return false
	}

	message := strings.TrimPrefix(line, "warning: ")
	localMarker := ": local ("
	localIndex := strings.Index(message, localMarker)
	if localIndex <= 0 {
		return false
	}

	return strings.Contains(message[localIndex+len(localMarker):], ") is newer than ")
}

// parsePacmanUpdates parses "name oldver -> newver" lines (as produced by
// checkupdates, `pacman -Qu`, and `pacman -Qua`) into Package records tagged
// with the given source manager ("pacman" or "aur").
func parsePacmanUpdates(output, installedBy string) []Package {
	var packages []Package
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Format: "package oldver -> newver"
		parts := strings.Split(line, " -> ")
		if len(parts) == 2 {
			nameParts := strings.Fields(parts[0])
			if len(nameParts) >= 2 {
				packages = append(packages, Package{
					Name:           nameParts[0],
					CurrentVersion: nameParts[1],
					LatestVersion:  strings.TrimSpace(parts[1]),
					Outdated:       true,
					InstalledBy:    installedBy,
				})
			}
		}
	}
	return packages
}

func (p *PacmanManager) Update(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	if p.executablePath() == "" {
		return errPackageManagerUnavailable
	}

	args := pacmanUpdateArgs(nil, packages)

	if p.useParu {
		cmd, cancel := packageCommand(packageMutationTimeout, p.executablePath(), args...)
		defer cancel()
		return cmd.Run()
	}
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", append([]string{p.executablePath()}, args...)...)
	defer cancel()
	return cmd.Run()
}

func pacmanUpdateArgs(extraFlags []string, packages []string) []string {
	// Use -Syu for selected updates: checkupdates reads a private fresh DB, while
	// -S would use stale sync DBs and bare -Sy risks a partial upgrade.
	args := []string{"-Syu", "--noconfirm"}
	args = append(args, extraFlags...)
	return append(args, packages...)
}

func (p *PacmanManager) UpdateAll() error {
	if p.executablePath() == "" {
		return errPackageManagerUnavailable
	}
	if p.useParu {
		cmd, cancel := packageCommand(packageMutationTimeout, p.executablePath(), "-Syu", "--noconfirm")
		defer cancel()
		return cmd.Run()
	}
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", p.executablePath(), "-Syu", "--noconfirm")
	defer cancel()
	return cmd.Run()
}

func (p *PacmanManager) Search(query string) ([]Package, error) {
	if p.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	cmd, cancel := packageCommand(packageQueryTimeout, p.executablePath(), "-Ss", query)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parsePacmanSearch(out.String()), nil
}

// parsePacmanSearch parses `pacman -Ss` / `paru -Ss` output into Package
// records. The output format is line-based: header lines have the shape
// "repo/name version [flags...]" (first field contains '/') and description
// lines start with whitespace. The old stride-by-2 approach broke when
// output contained leading blank lines or when paru emitted extra lines.
// This parser detects headers by the presence of '/' in the first field and
// accumulates description lines until the next header.
func parsePacmanSearch(output string) []Package {
	var packages []Package
	var current *Package

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// A header line's first field contains '/' (e.g. "core/linux 6.9.0-1")
		fields := strings.Fields(line)
		if len(fields) >= 1 && strings.Contains(fields[0], "/") {
			// Flush previous package before starting a new one
			if current != nil {
				packages = append(packages, *current)
			}

			nameParts := strings.SplitN(fields[0], "/", 2)
			name := fields[0]
			if len(nameParts) == 2 {
				name = nameParts[1]
			}
			current = &Package{
				Name:        name,
				InstalledBy: "pacman",
			}
			continue
		}

		// A description line starts with whitespace; accumulate it
		if current != nil && len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			desc := strings.TrimSpace(line)
			if current.Description == "" {
				current.Description = desc
			} else {
				current.Description += " " + desc
			}
		}
	}

	// Flush last package
	if current != nil {
		packages = append(packages, *current)
	}

	return packages
}

func (p *PacmanManager) ListInstalled() ([]Package, error) {
	if p.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	cmd, cancel := packageCommand(packageQueryTimeout, p.executablePath(), "-Q")
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var packages []Package
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			packages = append(packages, Package{
				Name:           parts[0],
				CurrentVersion: parts[1],
				InstalledBy:    "pacman",
			})
		}
	}

	return packages, nil
}

// NeedsSudo returns true for both pacman and paru since they need sudo for package installation
func (p *PacmanManager) NeedsSudo() bool {
	// Both pacman and paru need sudo to be cached. paru handles calling sudo internally,
	// but still requires credentials to be cached or a terminal for prompting.
	return true
}

// InstallStreaming installs packages with real-time output streaming
func (p *PacmanManager) InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}
	if p.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}

	if p.useParu {
		// paru should NOT be run with sudo - it handles sudo internally
		// Running with sudo causes permission issues with AUR builds
		args := []string{"-S", "--noconfirm", "--needed", "--skipreview", "--noprovides", "--removemake"}
		args = append(args, packages...)
		return runner.RunStreaming(ctx, p.executablePath(), args...)
	}

	// pacman needs sudo
	args := []string{"-S", "--noconfirm", "--needed"}
	args = append(args, packages...)
	return runner.RunStreamingWithSudo(ctx, p.executablePath(), args...)
}

// UpdateStreaming updates packages with real-time output streaming
func (p *PacmanManager) UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}
	if p.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}

	if p.useParu {
		// paru should NOT be run with sudo - it handles sudo internally
		args := pacmanUpdateArgs([]string{"--skipreview", "--noprovides"}, packages)
		return runner.RunStreaming(ctx, p.executablePath(), args...)
	}

	// pacman needs sudo
	args := pacmanUpdateArgs(nil, packages)
	return runner.RunStreamingWithSudo(ctx, p.executablePath(), args...)
}

// UpdateAllStreaming updates all packages with real-time output streaming
func (p *PacmanManager) UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error) {
	if p.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	if p.useParu {
		// paru should NOT be run with sudo - it handles sudo internally
		return runner.RunStreaming(ctx, p.executablePath(), "-Syu", "--noconfirm", "--skipreview", "--noprovides")
	}
	return runner.RunStreamingWithSudo(ctx, p.executablePath(), "-Syu", "--noconfirm")
}
