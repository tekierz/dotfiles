package pkg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
	cmd := exec.Command(b.brewPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func (b *BrewManager) Uninstall(packages ...string) error {
	if len(packages) == 0 {
		return nil
	}

	args := append([]string{"uninstall"}, packages...)
	cmd := exec.Command(b.brewPath, args...)
	return cmd.Run()
}

func (b *BrewManager) IsInstalled(pkg string) bool {
	cmd := exec.Command(b.brewPath, "list", pkg)
	return cmd.Run() == nil
}

func (b *BrewManager) GetVersion(pkg string) (string, error) {
	cmd := exec.Command(b.brewPath, "info", "--json=v2", pkg)
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
	cmd := exec.Command(b.brewPath, "outdated", "--json=v2")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var outdated struct {
		Formulae []struct {
			Name               string `json:"name"`
			InstalledVersions  []string `json:"installed_versions"`
			CurrentVersion     string `json:"current_version"`
		} `json:"formulae"`
		Casks []struct {
			Name             string `json:"name"`
			InstalledVersion string `json:"installed_version"`
			CurrentVersion   string `json:"current_version"`
		} `json:"casks"`
	}

	if err := json.Unmarshal(out.Bytes(), &outdated); err != nil {
		return nil, err
	}

	var packages []Package

	for _, f := range outdated.Formulae {
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
	cmd := exec.Command(b.brewPath, args...)
	return cmd.Run()
}

func (b *BrewManager) UpdateAll() error {
	cmd := exec.Command(b.brewPath, "upgrade")
	return cmd.Run()
}

func (b *BrewManager) Search(query string) ([]Package, error) {
	cmd := exec.Command(b.brewPath, "search", query)
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
	cmd := exec.Command(b.brewPath, "list", "--versions")
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
				InstalledBy:    "brew",
			})
		}
	}

	return packages, nil
}
