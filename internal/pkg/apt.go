package pkg

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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

func (a *AptManager) ListInstalled() ([]Package, error) {
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
			version, _ := a.GetVersion(name)
			packages = append(packages, Package{
				Name:           name,
				CurrentVersion: version,
				InstalledBy:    "apt",
			})
		}
	}

	return packages, nil
}
