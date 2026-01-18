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
	cmd := exec.Command("sudo", args...)
	return cmd.Run()
}

func (a *AptManager) Uninstall(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"apt", "remove", "-y"}
	args = append(args, packages...)
	cmd := exec.Command("sudo", args...)
	return cmd.Run()
}

func (a *AptManager) IsInstalled(pkg string) bool {
	cmd := exec.Command("dpkg", "-s", pkg)
	return cmd.Run() == nil
}

func (a *AptManager) GetVersion(pkg string) (string, error) {
	cmd := exec.Command("dpkg", "-s", pkg)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("package %s not installed", pkg)
	}

	// Parse dpkg output for Version line
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Version:")), nil
		}
	}

	return "", fmt.Errorf("could not find version for %s", pkg)
}

func (a *AptManager) CheckOutdated() ([]Package, error) {
	// First update package lists
	updateCmd := exec.Command("sudo", "apt", "update")
	updateCmd.Run() // Ignore errors, best effort

	// Get list of upgradable packages
	cmd := exec.Command("apt", "list", "--upgradable")
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

	// Update package lists first
	updateCmd := exec.Command("sudo", "apt", "update")
	updateCmd.Run()

	// Install specific packages (will upgrade if already installed)
	args := []string{"apt", "install", "-y"}
	args = append(args, packages...)
	cmd := exec.Command("sudo", args...)
	return cmd.Run()
}

func (a *AptManager) UpdateAll() error {
	// Update package lists
	updateCmd := exec.Command("sudo", "apt", "update")
	if err := updateCmd.Run(); err != nil {
		return err
	}

	// Upgrade all packages
	upgradeCmd := exec.Command("sudo", "apt", "upgrade", "-y")
	return upgradeCmd.Run()
}

func (a *AptManager) Search(query string) ([]Package, error) {
	cmd := exec.Command("apt", "search", query)
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
	cmd := exec.Command("dpkg-query", "-W", "-f=${Package}\t${Version}\n")
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
	cmd := exec.Command("dpkg", "--get-selections")
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
