package pkg

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// PacmanManager implements PackageManager for Arch Linux (pacman/paru)
type PacmanManager struct {
	pacmanPath string
	useParu    bool // Use paru for AUR support
}

// NewPacmanManager creates a new pacman manager
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

	// Check official repo updates
	cmd := exec.Command("checkupdates")
	var out bytes.Buffer
	cmd.Stdout = &out

	// checkupdates returns exit code 2 if no updates, 0 if updates available
	cmd.Run() // Ignore error, check output

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
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
					InstalledBy:    "pacman",
				})
			}
		}
	}

	// Check AUR updates if using paru
	if p.useParu {
		aurCmd := exec.Command(p.pacmanPath, "-Qua")
		var aurOut bytes.Buffer
		aurCmd.Stdout = &aurOut
		aurCmd.Run() // Ignore error

		aurLines := strings.Split(strings.TrimSpace(aurOut.String()), "\n")
		for _, line := range aurLines {
			if line == "" {
				continue
			}
			parts := strings.Split(line, " -> ")
			if len(parts) == 2 {
				nameParts := strings.Fields(parts[0])
				if len(nameParts) >= 2 {
					packages = append(packages, Package{
						Name:           nameParts[0],
						CurrentVersion: nameParts[1],
						LatestVersion:  strings.TrimSpace(parts[1]),
						Outdated:       true,
						InstalledBy:    "aur",
					})
				}
			}
		}
	}

	return packages, nil
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

	var packages []Package
	lines := strings.Split(out.String(), "\n")
	for i := 0; i < len(lines); i += 2 {
		if lines[i] == "" {
			continue
		}
		// First line: repo/name version
		parts := strings.Fields(lines[i])
		if len(parts) >= 2 {
			nameParts := strings.Split(parts[0], "/")
			name := parts[0]
			if len(nameParts) == 2 {
				name = nameParts[1]
			}

			desc := ""
			if i+1 < len(lines) {
				desc = strings.TrimSpace(lines[i+1])
			}

			packages = append(packages, Package{
				Name:        name,
				Description: desc,
				InstalledBy: "pacman",
			})
		}
	}

	return packages, nil
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
