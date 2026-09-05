package pkg

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tekierz/dotfiles/internal/runner"
)

// AptManager implements PackageManager for Debian/Ubuntu
type AptManager struct {
	identity ExecutableIdentity
	state    executableResolutionState
}

// NewAptManager creates a new apt manager
func NewAptManager() *AptManager {
	return newAptManager(exec.LookPath)
}

func newAptManager(lookup executableLookup) *AptManager {
	resolution := resolveManagerExecutable("apt", lookup)
	return &AptManager{identity: resolution.identity, state: resolution.state}
}

func (a *AptManager) Name() string {
	return "apt"
}

func (a *AptManager) IsAvailable() bool {
	return a.executablePath() != ""
}

func (a *AptManager) ExecutableIdentity() (ExecutableIdentity, bool) {
	if a == nil || a.state != executableResolutionValid || !validExecutableIdentity(a.identity) {
		return ExecutableIdentity{}, false
	}
	return a.identity, true
}

func (a *AptManager) executablePath() string {
	identity, ok := a.ExecutableIdentity()
	if !ok {
		return ""
	}
	return identity.invocationPath
}

func (AptManager) String() string {
	return "apt_manager"
}

func (manager AptManager) GoString() string {
	return manager.String()
}

func (a *AptManager) Install(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	if a.executablePath() == "" {
		return errPackageManagerUnavailable
	}

	args := []string{"install", "-y"}
	args = append(args, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", append([]string{a.executablePath()}, args...)...)
	defer cancel()
	return cmd.Run()
}

func (a *AptManager) Uninstall(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	if a.executablePath() == "" {
		return errPackageManagerUnavailable
	}

	args := []string{"remove", "-y"}
	args = append(args, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", append([]string{a.executablePath()}, args...)...)
	defer cancel()
	return cmd.Run()
}

func (a *AptManager) IsInstalled(pkg string) bool {
	if a.executablePath() == "" {
		return false
	}
	return a.IsInstalledContext(context.Background(), pkg)
}

func (a *AptManager) IsInstalledContext(ctx context.Context, pkg string) bool {
	if a.executablePath() == "" {
		return false
	}
	// Selection (including hold) is policy; only actual status/error state
	// determines whether a healthy installed receipt exists.
	cmd, cancel := packageCommandWithContext(ctx, packageQueryTimeout, "dpkg-query", "-W", "-f=${Status}", pkg)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	return aptStatusInstalled(out.String())
}

func (a *AptManager) GetVersion(pkg string) (string, error) {
	if a.executablePath() == "" {
		return "", errPackageManagerUnavailable
	}
	cmd, cancel := packageCommand(packageQueryTimeout, "dpkg-query", "-W", "-f=${Status}\t${Version}", pkg)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("package %s not installed", pkg)
	}

	parts := strings.SplitN(strings.TrimSpace(out.String()), "\t", 2)
	if !aptStatusInstalled(parts[0]) {
		return "", fmt.Errorf("package %s not installed", pkg)
	}
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("could not find version for %s", pkg)
	}

	return strings.TrimSpace(parts[1]), nil
}

func (a *AptManager) CheckOutdated() ([]Package, error) {
	if a.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	// CheckOutdated is a read-only query and may run from non-interactive code
	// paths (e.g. the async update-check command, where Bubble Tea owns the TTY).
	// Do NOT run `sudo apt update` here: with cached credentials it forces a
	// surprising network refresh of every repo index, and without them sudo
	// blocks on the controlling terminal. The repo index is refreshed inside
	// Update/UpdateAll/UpdateAllStreaming, which is where the network side effect
	// belongs. We report against the already-synced local cache.
	cmd, cancel := packageCommand(packageQueryTimeout, a.executablePath(), "list", "--upgradable")
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var packages []Package
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "Listing...") {
			continue
		}

		// Format: "package/source version [upgradable from: old_version]"
		// Example: "vim/jammy-updates 2:8.2.3995-1ubuntu2.7 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.3]"
		parts := strings.Fields(line)
		if len(parts) >= 4 {
			nameParts := strings.Split(parts[0], "/")
			name := nameParts[0]
			newVersion := parts[1]

			oldVersion := ""
			for i, p := range parts {
				if p == "from:" && i+1 < len(parts) {
					oldVersion = strings.TrimSuffix(parts[i+1], "]")
				}
			}

			packages = append(packages, Package{
				Name:           name,
				CurrentVersion: oldVersion,
				LatestVersion:  newVersion,
				Outdated:       true,
				InstalledBy:    "apt",
			})
		}
	}

	return packages, nil
}

func (a *AptManager) Update(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}
	if a.executablePath() == "" {
		return errPackageManagerUnavailable
	}

	// Best-effort index refresh so a targeted upgrade can't 404 on a stale index.
	// sudo -n never blocks on an invisible password prompt (the install below may
	// still prompt). Output and errors are intentionally discarded to the null
	// device: this can run while the TUI owns the terminal, so writing to
	// os.Stderr would splatter the alt-screen, and any genuine failure surfaces
	// through the install below.
	refreshCmd, refreshCancel := packageCommand(packageRefreshTimeout, "sudo", "-n", a.executablePath(), "update")
	_ = refreshCmd.Run()
	refreshCancel()

	// Install specific packages (will upgrade if already installed)
	args := []string{"install", "-y"}
	args = append(args, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", append([]string{a.executablePath()}, args...)...)
	defer cancel()
	return cmd.Run()
}

func (a *AptManager) UpdateAll() error {
	if a.executablePath() == "" {
		return errPackageManagerUnavailable
	}
	// Update package lists
	updateCmd, updateCancel := packageCommand(packageRefreshTimeout, "sudo", a.executablePath(), "update")
	defer updateCancel()
	if err := updateCmd.Run(); err != nil {
		return err
	}

	// Upgrade all packages
	upgradeCmd, upgradeCancel := packageCommand(packageMutationTimeout, "sudo", a.executablePath(), "upgrade", "-y")
	defer upgradeCancel()
	return upgradeCmd.Run()
}

func (a *AptManager) Search(query string) ([]Package, error) {
	if a.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	cmd, cancel := packageCommand(packageQueryTimeout, a.executablePath(), "search", query)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var packages []Package
	lines := strings.Split(out.String(), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" || strings.HasPrefix(line, "Sorting...") || strings.HasPrefix(line, "Full Text Search...") {
			continue
		}

		// Parse package line: "name/source version arch"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			nameParts := strings.Split(parts[0], "/")
			name := nameParts[0]

			desc := ""
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") {
				desc = strings.TrimSpace(lines[i+1])
				i++ // Skip description line
			}

			packages = append(packages, Package{
				Name:        name,
				Description: desc,
				InstalledBy: "apt",
			})
		}
	}

	return packages, nil
}

// aptStatusInstalled ignores the desired selection while requiring fully
// installed, error-free state. Pending triggers and partial installs are not
// healthy receipts, even when dpkg's desired action is install.
func aptStatusInstalled(status string) bool {
	fields := strings.Fields(status)
	return len(fields) == 3 && fields[1] == "ok" && fields[2] == "installed"
}

func (a *AptManager) ListInstalled() ([]Package, error) {
	return a.ListInstalledContext(context.Background())
}

// ListInstalledContext collects status and versions in one read-only query.
// binary:Package preserves architecture qualifiers instead of collapsing
// co-installed architectures into one version-map entry.
func (a *AptManager) ListInstalledContext(ctx context.Context) ([]Package, error) {
	if a.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	cmd, cancel := packageCommandWithContext(ctx, packageQueryTimeout, "dpkg-query", "-W", "-f=${binary:Package}\t${Status}\t${Version}\n")
	defer cancel()
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return parseAptInstalledReceipts(string(out)), nil
}

func parseAptInstalledReceipts(output string) []Package {
	var packages []Package
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || fields[0] == "" || !aptStatusInstalled(fields[1]) || strings.TrimSpace(fields[2]) == "" {
			continue
		}
		packages = append(packages, Package{
			Name: fields[0], CurrentVersion: strings.TrimSpace(fields[2]), InstalledBy: "apt",
		})
	}
	return packages
}

// NeedsSudo returns true for apt (requires sudo for package operations)
func (a *AptManager) NeedsSudo() bool {
	return true
}

// InstallStreaming installs packages with real-time output streaming
func (a *AptManager) InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}
	if a.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	if identity, ok := a.ExecutableIdentity(); !ok || identity.Revalidate() != nil {
		return nil, errPackageManagerUnavailable
	}

	args := []string{"install", "-y"}
	args = append(args, packages...)
	return runner.RunStreamingWithSudo(ctx, a.executablePath(), args...)
}

// UpdateStreaming updates packages with real-time output streaming
func (a *AptManager) UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}
	if a.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	identity, ok := a.ExecutableIdentity()
	if !ok || identity.Revalidate() != nil {
		return nil, errPackageManagerUnavailable
	}

	args := []string{"install", "-y"}
	args = append(args, packages...)
	return runner.RunSequentialStreaming(ctx,
		aptStreamingPhase("apt update", identity, "update"),
		aptStreamingPhase("apt targeted upgrade", identity, args...),
	)
}

// UpdateAllStreaming updates all packages with real-time output streaming
// This runs apt update && apt upgrade -y sequentially without shell injection risk
func (a *AptManager) UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error) {
	if a.executablePath() == "" {
		return nil, errPackageManagerUnavailable
	}
	identity, ok := a.ExecutableIdentity()
	if !ok || identity.Revalidate() != nil {
		return nil, errPackageManagerUnavailable
	}
	return runner.RunSequentialStreaming(ctx,
		aptStreamingPhase("apt update", identity, "update"),
		aptStreamingPhase("apt upgrade", identity, "upgrade", "-y"),
	)
}

func aptStreamingPhase(name string, identity ExecutableIdentity, args ...string) runner.SequentialStreamingPhase {
	argv := append([]string(nil), args...)
	return runner.SequentialStreamingPhase{
		Name: name,
		Start: func(ctx context.Context) (*runner.StreamingCmd, error) {
			if identity.Revalidate() != nil {
				return nil, errPackageManagerUnavailable
			}
			return runner.RunStreamingWithSudo(ctx, identity.invocationPath, argv...)
		},
	}
}
