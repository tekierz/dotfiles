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

// PacmanManager implements PackageManager for Arch Linux (pacman/paru).
type PacmanManager struct {
	pacmanPath string
	useParu    bool // Use paru for AUR support
}

// NewPacmanManager creates a new pacman manager.
func NewPacmanManager(preferParu bool) *PacmanManager {
	pm := &PacmanManager{}

	if preferParu {
		if path, err := exec.LookPath("paru"); err == nil {
			pm.pacmanPath = path
			pm.useParu = true
			return pm
		}
	}

	if path, err := exec.LookPath("pacman"); err == nil {
		pm.pacmanPath = path
	}

	return pm
}

func (p *PacmanManager) Name() string {
	if p.useParu {
		return "paru"
	}
	return "pacman"
}

func (p *PacmanManager) IsAvailable() bool {
	return p.pacmanPath != ""
}

func (p *PacmanManager) Install(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"-S", "--noconfirm", "--needed"}
	args = append(args, packages...)

	var cmd *exec.Cmd
	if p.useParu {
		cmd = exec.Command(p.pacmanPath, args...)
	} else {
		cmd = exec.Command("sudo", append([]string{p.pacmanPath}, args...)...)
	}

	return cmd.Run()
}

func (p *PacmanManager) Uninstall(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"-R", "--noconfirm"}
	args = append(args, packages...)
	cmd := exec.Command("sudo", append([]string{p.pacmanPath}, args...)...)
	return cmd.Run()
}

func (p *PacmanManager) IsInstalled(pkg string) bool {
	cmd := exec.Command(p.pacmanPath, "-Q", pkg)
	return cmd.Run() == nil
}

func (p *PacmanManager) GetVersion(pkg string) (string, error) {
	cmd := exec.Command(p.pacmanPath, "-Q", pkg)
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
		aurCmd := exec.Command(p.pacmanPath, "-Qua")
		var aurOut bytes.Buffer
		aurCmd.Stdout = &aurOut
		// `pacman -Qua` exits non-zero when there are no foreign updates, so
		// the error is intentionally ignored and we parse whatever it emits.
		_ = aurCmd.Run()

		packages = append(packages, parsePacmanUpdates(aurOut.String(), "aur")...)
	}

	return packages, nil
}

// classifyCheckupdatesErr interprets the exit status of `checkupdates`. The
// helper exits 2 specifically when there are no updates available, which is not
// an error for us; exit 0 (err == nil) means updates were found. Any other
// nonzero exit (e.g. a stale/locked temp DB or mirror error) is a genuine
// failure that must be surfaced rather than silently reported as "up to date".
func classifyCheckupdatesErr(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return nil
	}
	return fmt.Errorf("checkupdates failed: %w", err)
}

// checkOfficialUpdates returns the raw "name oldver -> newver" lines describing
// pending official-repo updates. It prefers the `checkupdates` helper (from the
// optional pacman-contrib package, which queries a private sync DB without
// root), and falls back to `pacman -Qu` when checkupdates is not installed so a
// missing optional dependency does not silently report "up to date".
func (p *PacmanManager) checkOfficialUpdates() (string, error) {
	if _, err := exec.LookPath("checkupdates"); err == nil {
		cmd := exec.Command("checkupdates")
		var out bytes.Buffer
		cmd.Stdout = &out

		// checkupdates exits 2 when there are no updates (not an error for us)
		// and 0 when updates are available. Any other exit code is a genuine
		// failure (e.g. a stale temp DB) that should be surfaced.
		if err := classifyCheckupdatesErr(cmd.Run()); err != nil {
			return "", err
		}
		return out.String(), nil
	}

	// Fallback: `pacman -Qu` reads the local sync DB and works without the
	// pacman-contrib package. It exits non-zero when there are no updates, so
	// distinguish that (empty output) from a real failure.
	cmd := exec.Command(p.pacmanPath, "-Qu")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		// `pacman -Qu` returns exit code 1 with empty output when nothing is
		// outdated; treat empty output as "no updates" rather than an error.
		if strings.TrimSpace(out.String()) == "" {
			return "", nil
		}
		return "", fmt.Errorf("pacman -Qu failed: %w", err)
	}
	return out.String(), nil
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

	args := []string{"-S", "--noconfirm"}
	args = append(args, packages...)

	var cmd *exec.Cmd
	if p.useParu {
		cmd = exec.Command(p.pacmanPath, args...)
	} else {
		cmd = exec.Command("sudo", append([]string{p.pacmanPath}, args...)...)
	}

	return cmd.Run()
}

func (p *PacmanManager) UpdateAll() error {
	var cmd *exec.Cmd
	if p.useParu {
		cmd = exec.Command(p.pacmanPath, "-Syu", "--noconfirm")
	} else {
		cmd = exec.Command("sudo", p.pacmanPath, "-Syu", "--noconfirm")
	}
	return cmd.Run()
}

func (p *PacmanManager) Search(query string) ([]Package, error) {
	cmd := exec.Command(p.pacmanPath, "-Ss", query)
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
	cmd := exec.Command(p.pacmanPath, "-Q")
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

// NeedsSudo returns true for both pacman and paru since they need sudo for package installation.
func (p *PacmanManager) NeedsSudo() bool {
	// Both pacman and paru need sudo to be cached. paru handles calling sudo internally,
	// but still requires credentials to be cached or a terminal for prompting.
	return true
}

// InstallStreaming installs packages with real-time output streaming.
func (p *PacmanManager) InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	if p.useParu {
		// paru should NOT be run with sudo - it handles sudo internally
		// Running with sudo causes permission issues with AUR builds
		args := []string{"-S", "--noconfirm", "--needed", "--skipreview", "--noprovides", "--removemake"}
		args = append(args, packages...)
		return runner.RunStreaming(ctx, p.pacmanPath, args...)
	}

	// pacman needs sudo
	args := []string{"-S", "--noconfirm", "--needed"}
	args = append(args, packages...)
	return runner.RunStreamingWithSudo(ctx, p.pacmanPath, args...)
}

// UpdateStreaming updates packages with real-time output streaming.
func (p *PacmanManager) UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	if p.useParu {
		// paru should NOT be run with sudo - it handles sudo internally
		args := []string{"-S", "--noconfirm", "--skipreview", "--noprovides"}
		args = append(args, packages...)
		return runner.RunStreaming(ctx, p.pacmanPath, args...)
	}

	// pacman needs sudo
	args := []string{"-S", "--noconfirm"}
	args = append(args, packages...)
	return runner.RunStreamingWithSudo(ctx, p.pacmanPath, args...)
}

// UpdateAllStreaming updates all packages with real-time output streaming.
func (p *PacmanManager) UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error) {
	if p.useParu {
		// paru should NOT be run with sudo - it handles sudo internally
		return runner.RunStreaming(ctx, p.pacmanPath, "-Syu", "--noconfirm", "--skipreview", "--noprovides")
	}
	return runner.RunStreamingWithSudo(ctx, p.pacmanPath, "-Syu", "--noconfirm")
}
