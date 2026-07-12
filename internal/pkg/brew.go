package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tekierz/dotfiles/internal/runner"
)

// BrewManager implements PackageManager for Homebrew
type BrewManager struct {
	brewPath string
}

// NewBrewManager creates a new Homebrew manager
func NewBrewManager() *BrewManager {
	path, _ := exec.LookPath("brew")
	return &BrewManager{brewPath: path}
}

func (b *BrewManager) Name() string {
	return "brew"
}

func (b *BrewManager) IsAvailable() bool {
	return b.brewPath != ""
}

func (b *BrewManager) Install(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := append([]string{"install"}, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, b.brewPath, args...)
	defer cancel()
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (b *BrewManager) Uninstall(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := append([]string{"uninstall"}, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, b.brewPath, args...)
	defer cancel()
	return cmd.Run()
}

func (b *BrewManager) IsInstalled(pkg string) bool {
	return b.IsInstalledContext(context.Background(), pkg)
}

func (b *BrewManager) IsInstalledContext(ctx context.Context, pkg string) bool {
	cmd, cancel := packageCommandWithContext(ctx, packageQueryTimeout, b.brewPath, "list", pkg)
	defer cancel()
	return cmd.Run() == nil
}

func (b *BrewManager) GetVersion(pkg string) (string, error) {
	cmd, cancel := packageCommand(packageQueryTimeout, b.brewPath, "info", "--json=v2", pkg)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", err
	}

	var info struct {
		Formulae []struct {
			Versions struct {
				Stable string `json:"stable"`
			} `json:"versions"`
			Installed []struct {
				Version string `json:"version"`
			} `json:"installed"`
		} `json:"formulae"`
		Casks []struct {
			Version   string `json:"version"`
			Installed string `json:"installed"`
		} `json:"casks"`
	}

	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		return "", err
	}

	// Check formulae first
	if len(info.Formulae) > 0 && len(info.Formulae[0].Installed) > 0 {
		return info.Formulae[0].Installed[0].Version, nil
	}
	// Check casks
	if len(info.Casks) > 0 && info.Casks[0].Installed != "" {
		return info.Casks[0].Installed, nil
	}

	return "", fmt.Errorf("package %s not installed", pkg)
}

func (b *BrewManager) CheckOutdated() ([]Package, error) {
	// --greedy includes auto-updating casks that would otherwise be skipped
	// by `brew outdated` (they report as up-to-date without this flag).
	cmd, cancel := packageCommand(packageQueryTimeout, b.brewPath, "outdated", "--json=v2", "--greedy")
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parseBrewOutdated(out.Bytes())
}

// CheckOutdatedNonGreedy reports outdated packages WITHOUT `--greedy`, so
// auto-updating and `version :latest` casks are excluded. Those casks always
// appear under `brew outdated --greedy` because brew cannot track their version,
// which makes the greedy list useless as a "did this upgrade actually fail?"
// oracle: they would be reported outdated forever, regardless of upgrade state.
// The non-greedy list contains only packages whose version brew can verify, so a
// package that REMAINS in it after an upgrade genuinely failed. recheckOutdatedNames
// uses this after a partial-batch failure to keep formulae (and version-tracked
// casks) honest while not falsely marking the auto-updaters brew cannot judge.
// Parsing/filtering is shared with CheckOutdated via parseBrewOutdated.
func (b *BrewManager) CheckOutdatedNonGreedy() ([]Package, error) {
	cmd, cancel := packageCommand(packageQueryTimeout, b.brewPath, "outdated", "--json=v2")
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parseBrewOutdated(out.Bytes())
}

// parseBrewOutdated parses the JSON emitted by `brew outdated --json=v2
// --greedy` into Package records. It is extracted from CheckOutdated so that
// unit tests can exercise the real parsing and filtering logic without
// shelling out to brew.
//
// Rules:
//   - Pinned formulae are excluded (brew upgrade refuses them and would fail
//     the whole batch).
//   - Casks whose current_version is empty are excluded (some auto-updating
//     casks report "" even with --greedy; including them produces an
//     unactionable record).
func parseBrewOutdated(data []byte) ([]Package, error) {
	var outdated struct {
		Formulae []struct {
			Name              string   `json:"name"`
			InstalledVersions []string `json:"installed_versions"`
			CurrentVersion    string   `json:"current_version"`
			Pinned            bool     `json:"pinned"`
		} `json:"formulae"`
		Casks []struct {
			Name             string `json:"name"`
			InstalledVersion string `json:"installed_version"`
			CurrentVersion   string `json:"current_version"`
		} `json:"casks"`
	}

	if err := json.Unmarshal(data, &outdated); err != nil {
		return nil, err
	}

	var packages []Package

	for _, f := range outdated.Formulae {
		// Skip pinned formulae: `brew upgrade <name>` refuses to move a pinned
		// formula and exits non-zero when one is named explicitly. Reporting it
		// as outdated would route it into Update() and fail the entire upgrade
		// batch it shares, so it must not appear in the actionable update list.
		if f.Pinned {
			continue
		}
		currentVer := ""
		if len(f.InstalledVersions) > 0 {
			currentVer = f.InstalledVersions[0]
		}
		packages = append(packages, Package{
			Name:           f.Name,
			CurrentVersion: currentVer,
			LatestVersion:  f.CurrentVersion,
			Outdated:       true,
			InstalledBy:    "brew",
		})
	}

	for _, c := range outdated.Casks {
		// Guard against auto-updating casks that report an empty current_version
		// (brew returns "" for some auto-update casks even with --greedy).
		if c.CurrentVersion == "" {
			continue
		}
		packages = append(packages, Package{
			Name:           c.Name,
			CurrentVersion: c.InstalledVersion,
			LatestVersion:  c.CurrentVersion,
			Outdated:       true,
			InstalledBy:    "brew-cask",
		})
	}

	return packages, nil
}

func (b *BrewManager) Update(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := append([]string{"upgrade"}, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, b.brewPath, args...)
	defer cancel()
	return cmd.Run()
}

// outdatedPackageNames returns the names of the outdated packages dotfiles
// tracks and displays (formulae + casks from CheckOutdated, with pinned formulae
// and empty-version casks already filtered out). "Update all" upgrades exactly
// this set by name instead of running a bare `brew upgrade`, which would sweep in
// every outdated formula and cask on the machine — a surprise full-system upgrade
// from the Updates screen's "update all". Naming the packages explicitly also
// forces the greedy auto-updating casks CheckOutdated surfaces (bare `brew
// upgrade` skips them), so the action matches the list on screen.
func (b *BrewManager) outdatedPackageNames() ([]string, error) {
	outdated, err := b.CheckOutdated()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(outdated))
	for _, p := range outdated {
		names = append(names, p.Name)
	}
	return names, nil
}

func (b *BrewManager) UpdateAll() error {
	names, err := b.outdatedPackageNames()
	if err != nil {
		return fmt.Errorf("failed to determine outdated packages: %w", err)
	}
	if len(names) == 0 {
		return nil
	}

	args := append([]string{"upgrade"}, names...)
	cmd, cancel := packageCommand(packageMutationTimeout, b.brewPath, args...)
	defer cancel()
	return cmd.Run()
}

func (b *BrewManager) Search(query string) ([]Package, error) {
	cmd, cancel := packageCommand(packageQueryTimeout, b.brewPath, "search", query)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var packages []Package
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		if line != "" && !strings.HasPrefix(line, "==>") {
			packages = append(packages, Package{
				Name:        strings.TrimSpace(line),
				InstalledBy: "brew",
			})
		}
	}

	return packages, nil
}

func (b *BrewManager) ListInstalled() ([]Package, error) {
	// Scope formulae explicitly. An unscoped list can traverse casks and fail on
	// unrelated untrusted taps even after emitting otherwise valid formula data.
	// Casks are captured independently by ListInstalledCasks.
	cmd, cancel := packageCommand(packageQueryTimeout, b.brewPath, "list", "--formula", "--versions")
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parseBrewInstalledFormulae(out.Bytes()), nil
}

func parseBrewInstalledFormulae(data []byte) []Package {
	var packages []Package
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			packages = append(packages, Package{
				Name:           parts[0],
				CurrentVersion: parts[1],
				InstalledBy:    "brew",
			})
		}
	}

	return packages
}

// ListInstalledCasks returns all installed Homebrew casks as their cask tokens.
// This is a single `brew list --cask` call, used to batch cask detection so that
// cask-backed tools (e.g. sunshine, tailscale) don't each shell out individually.
func (b *BrewManager) ListInstalledCasks() ([]string, error) {
	cmd, cancel := packageCommand(packageQueryTimeout, b.brewPath, "list", "--cask")
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var casks []string
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		token := strings.TrimSpace(line)
		if token != "" {
			casks = append(casks, token)
		}
	}

	return casks, nil
}

// NeedsSudo returns false for Homebrew (doesn't require sudo)
func (b *BrewManager) NeedsSudo() bool {
	return false
}

// InstallStreaming installs packages with real-time output streaming
func (b *BrewManager) InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	args := append([]string{"install"}, packages...)
	return runner.RunStreaming(ctx, b.brewPath, args...)
}

func (b *BrewManager) InstallCasksStreaming(ctx context.Context, casks ...string) (*runner.StreamingCmd, error) {
	if len(casks) == 0 {
		return nil, fmt.Errorf("no casks specified")
	}
	for _, cask := range casks {
		if !validCaskToken(cask) {
			return nil, fmt.Errorf("invalid Homebrew cask token %q", cask)
		}
	}
	args := append([]string{"install", "--cask"}, casks...)
	return runner.RunStreaming(ctx, b.brewPath, args...)
}

func validCaskToken(value string) bool {
	if value == "" || value[0] == '-' {
		return false
	}
	for _, r := range value {
		allowed := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("+._@-", r)
		if !allowed {
			return false
		}
	}
	return true
}

// UpdateStreaming updates packages with real-time output streaming
func (b *BrewManager) UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	args := append([]string{"upgrade"}, packages...)
	return runner.RunStreaming(ctx, b.brewPath, args...)
}

// UpdateAllStreaming upgrades only the outdated packages dotfiles tracks/displays
// (see outdatedPackageNames), streaming output live. It deliberately does NOT run
// a bare `brew upgrade`, which would upgrade every outdated formula and cask on
// the system — the user must not get a surprise full-system upgrade from the
// Updates screen's "update all".
func (b *BrewManager) UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error) {
	names, err := b.outdatedPackageNames()
	if err != nil {
		return nil, fmt.Errorf("failed to determine outdated packages: %w", err)
	}
	if len(names) == 0 {
		// Nothing outdated: return a no-op rather than falling through to a bare
		// `brew upgrade`. The caller treats a nil command as a clean completion.
		return nil, nil
	}

	args := append([]string{"upgrade"}, names...)
	return runner.RunStreaming(ctx, b.brewPath, args...)
}
