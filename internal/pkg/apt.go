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
	aptPath string
}

// NewAptManager creates a new apt manager
func NewAptManager() *AptManager {
	path, _ := exec.LookPath("apt")
	return &AptManager{aptPath: path}
}

func (a *AptManager) Name() string {
	return "apt"
}

func (a *AptManager) IsAvailable() bool {
	return a.aptPath != ""
}

func (a *AptManager) Install(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"apt", "install", "-y"}
	args = append(args, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", args...)
	defer cancel()
	return cmd.Run()
}

func (a *AptManager) Uninstall(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"apt", "remove", "-y"}
	args = append(args, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", args...)
	defer cancel()
	return cmd.Run()
}

func (a *AptManager) IsInstalled(pkg string) bool {
	return a.IsInstalledContext(context.Background(), pkg)
}

func (a *AptManager) IsInstalledContext(ctx context.Context, pkg string) bool {
	// `dpkg -s` exits 0 even for a removed-but-not-purged package (status
	// "deinstall ok config-files"), which would falsely report it installed.
	// Query the Status field directly and require "install ok installed",
	// matching the filter ListInstalled uses (C11).
	cmd, cancel := packageCommandWithContext(ctx, packageQueryTimeout, "dpkg-query", "-W", "-f=${Status}", pkg)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(out.String()) == "install ok installed"
}

func (a *AptManager) GetVersion(pkg string) (string, error) {
	// `dpkg -s` exits 0 and reports a Version for a removed-but-not-purged
	// package (status "deinstall ok config-files"), which contradicts
	// IsInstalled. Query Status and Version together and only report a version
	// when the package is actually installed ("install ok installed"), matching
	// IsInstalled's filter exactly.
	cmd, cancel := packageCommand(packageQueryTimeout, "dpkg-query", "-W", "-f=${Status}\t${Version}", pkg)
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("package %s not installed", pkg)
	}

	parts := strings.SplitN(strings.TrimSpace(out.String()), "\t", 2)
	if strings.TrimSpace(parts[0]) != "install ok installed" {
		return "", fmt.Errorf("package %s not installed", pkg)
	}
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("could not find version for %s", pkg)
	}

	return strings.TrimSpace(parts[1]), nil
}

func (a *AptManager) CheckOutdated() ([]Package, error) {
	// CheckOutdated is a read-only query and may run from non-interactive code
	// paths (e.g. the async update-check command, where Bubble Tea owns the TTY).
	// Do NOT run `sudo apt update` here: with cached credentials it forces a
	// surprising network refresh of every repo index, and without them sudo
	// blocks on the controlling terminal. The repo index is refreshed inside
	// Update/UpdateAll/UpdateAllStreaming, which is where the network side effect
	// belongs. We report against the already-synced local cache.
	cmd, cancel := packageCommand(packageQueryTimeout, "apt", "list", "--upgradable")
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

	// Best-effort index refresh so a targeted upgrade can't 404 on a stale index.
	// sudo -n never blocks on an invisible password prompt (the install below may
	// still prompt). Output and errors are intentionally discarded to the null
	// device: this can run while the TUI owns the terminal, so writing to
	// os.Stderr would splatter the alt-screen, and any genuine failure surfaces
	// through the install below.
	refreshCmd, refreshCancel := packageCommand(packageRefreshTimeout, "sudo", "-n", "apt", "update")
	_ = refreshCmd.Run()
	refreshCancel()

	// Install specific packages (will upgrade if already installed)
	args := []string{"apt", "install", "-y"}
	args = append(args, packages...)
	cmd, cancel := packageCommand(packageMutationTimeout, "sudo", args...)
	defer cancel()
	return cmd.Run()
}

func (a *AptManager) UpdateAll() error {
	// Update package lists
	updateCmd, updateCancel := packageCommand(packageRefreshTimeout, "sudo", "apt", "update")
	defer updateCancel()
	if err := updateCmd.Run(); err != nil {
		return err
	}

	// Upgrade all packages
	upgradeCmd, upgradeCancel := packageCommand(packageMutationTimeout, "sudo", "apt", "upgrade", "-y")
	defer upgradeCancel()
	return upgradeCmd.Run()
}

func (a *AptManager) Search(query string) ([]Package, error) {
	cmd, cancel := packageCommand(packageQueryTimeout, "apt", "search", query)
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

// getInstalledVersions returns a map of all installed package names to their versions
// using a single dpkg-query command instead of individual dpkg -s calls per package.
// This eliminates the N+1 query problem that caused 5-25 second startup delays.
func (a *AptManager) getInstalledVersions() (map[string]string, error) {
	cmd, cancel := packageCommand(packageQueryTimeout, "dpkg-query", "-W", "-f=${Package}\t${Version}\n")
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	versions := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 {
			versions[parts[0]] = parts[1]
		}
	}

	return versions, nil
}

func (a *AptManager) ListInstalled() ([]Package, error) {
	// Get all versions in a single batch query (avoids N+1 problem)
	versions, err := a.getInstalledVersions()
	if err != nil {
		return nil, err
	}

	// Get list of installed packages
	cmd, cancel := packageCommand(packageQueryTimeout, "dpkg", "--get-selections")
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
		if len(parts) >= 2 && parts[1] == "install" {
			name := parts[0]
			// Look up version from pre-fetched map instead of individual GetVersion call
			version := versions[name]
			packages = append(packages, Package{
				Name:           name,
				CurrentVersion: version,
				InstalledBy:    "apt",
			})
		}
	}

	return packages, nil
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

	args := []string{"install", "-y"}
	args = append(args, packages...)
	return runner.RunStreamingWithSudo(ctx, a.aptPath, args...)
}

// UpdateStreaming updates packages with real-time output streaming
func (a *AptManager) UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	// Refresh the package index before the targeted upgrade so it can't 404 on a
	// stale index (every other apt write path does an `apt update` first). sudo -n
	// keeps this best-effort refresh from ever blocking on an invisible password
	// prompt, and deriving it from ctx lets teardown cancel it. Output and errors
	// are intentionally discarded to the null device: this MUST be invoked off the
	// Bubble Tea UI goroutine (streamingUpdateCmd builds it inside its worker) so
	// the refresh never blocks the event loop, and while the TUI owns the terminal
	// writing to os.Stderr would splatter the alt-screen. Any genuine failure
	// surfaces through the streamed install below, which owns the upgrade and may
	// still prompt for sudo.
	_ = exec.CommandContext(ctx, "sudo", "-n", "apt", "update").Run()

	args := []string{"install", "-y"}
	args = append(args, packages...)
	return runner.RunStreamingWithSudo(ctx, a.aptPath, args...)
}

// UpdateAllStreaming updates all packages with real-time output streaming
// This runs apt update && apt upgrade -y sequentially without shell injection risk
func (a *AptManager) UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error) {
	// Run update first using safe exec.Command (no shell interpolation)
	updateCmd, err := runner.RunStreamingWithSudo(ctx, a.aptPath, "update")
	if err != nil {
		return nil, fmt.Errorf("apt update failed: %w", err)
	}
	// Wait for update to complete before running upgrade
	if err := updateCmd.Wait(); err != nil {
		return nil, fmt.Errorf("apt update failed: %w", err)
	}
	// Then run upgrade using safe exec.Command
	return runner.RunStreamingWithSudo(ctx, a.aptPath, "upgrade", "-y")
}
