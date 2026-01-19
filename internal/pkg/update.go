package pkg

import (
	"fmt"
	"sort"
)

// UpdateResult represents the result of an update operation
type UpdateResult struct {
	Package Package
	Success bool
	Error   error
}

// CheckAllUpdates checks for updates across all available package managers
func CheckAllUpdates() ([]Package, error) {
	var allPackages []Package

	managers := AllManagers()
	if len(managers) == 0 {
		return nil, fmt.Errorf("no package managers available")
	}

	for _, mgr := range managers {
		packages, err := mgr.CheckOutdated()
		if err != nil {
			// Log error but continue with other managers
			continue
		}
		allPackages = append(allPackages, packages...)
	}

	// Deduplicate packages by name only (same package from different sources is still same package)
	seen := make(map[string]bool)
	var deduped []Package
	for _, p := range allPackages {
		if !seen[p.Name] {
			seen[p.Name] = true
			deduped = append(deduped, p)
		}
	}
	allPackages = deduped

	// Sort by name for consistent display
	sort.Slice(allPackages, func(i, j int) bool {
		return allPackages[i].Name < allPackages[j].Name
	})

	return allPackages, nil
}

// UpdatePackages updates specific packages using the appropriate manager
func UpdatePackages(packages []Package) []UpdateResult {
	var results []UpdateResult

	// Group packages by manager
	byManager := make(map[string][]string)
	for _, pkg := range packages {
		byManager[pkg.InstalledBy] = append(byManager[pkg.InstalledBy], pkg.Name)
	}

	// Update each group
	for managerName, pkgNames := range byManager {
		mgr := getManagerByName(managerName)
		if mgr == nil {
			for _, name := range pkgNames {
				results = append(results, UpdateResult{
					Package: Package{Name: name, InstalledBy: managerName},
					Success: false,
					Error:   fmt.Errorf("package manager %s not available", managerName),
				})
			}
			continue
		}

		err := mgr.Update(pkgNames...)
		for _, name := range pkgNames {
			results = append(results, UpdateResult{
				Package: Package{Name: name, InstalledBy: managerName},
				Success: err == nil,
				Error:   err,
			})
		}
	}

	return results
}

// UpdateAllPackages updates all outdated packages
func UpdateAllPackages() error {
	managers := AllManagers()
	if len(managers) == 0 {
		return fmt.Errorf("no package managers available")
	}

	var errs []error
	for _, mgr := range managers {
		if err := mgr.UpdateAll(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", mgr.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("update errors: %v", errs)
	}

	return nil
}

// getManagerByName returns the appropriate manager for a given name
func getManagerByName(name string) PackageManager {
	switch name {
	case "brew", "brew-cask":
		return NewBrewManager()
	case "pacman":
		return NewPacmanManager(false)
	case "paru", "aur":
		return NewPacmanManager(true)
	case "apt":
		return NewAptManager()
	}
	return nil
}

// InstallPackage installs a single package using the detected manager
func InstallPackage(name string) error {
	mgr := DetectManager()
	if mgr == nil {
		return fmt.Errorf("no package manager available")
	}
	return mgr.Install(name)
}

// InstallPackages installs multiple packages using the detected manager
func InstallPackages(names ...string) error {
	mgr := DetectManager()
	if mgr == nil {
		return fmt.Errorf("no package manager available")
	}
	return mgr.Install(names...)
}

// IsPackageInstalled checks if a package is installed
func IsPackageInstalled(name string) bool {
	mgr := DetectManager()
	if mgr == nil {
		return false
	}
	return mgr.IsInstalled(name)
}

// DotfilesPackages returns the list of packages managed by dotfiles
var DotfilesPackages = []string{
	// Core shell
	"zsh",
	"zsh-syntax-highlighting",
	"zsh-autosuggestions",
	"zsh-completions",

	// Terminal tools
	"tmux",
	"neovim",
	"fzf",
	"ripgrep",
	"fd",
	"bat",
	"eza",
	"zoxide",
	"yazi",
	"btop",
	"glow",

	// Git tools
	"git",
	"git-delta",
	"lazygit",
	"lazydocker",

	// Utilities
	"fastfetch",
	"tlrc",
	"ncdu",
	"duf",
	"dust",
	"fswatch",
}

// CheckDotfilesUpdates checks for updates only for dotfiles-managed packages
func CheckDotfilesUpdates() ([]Package, error) {
	allUpdates, err := CheckAllUpdates()
	if err != nil {
		return nil, err
	}

	// Filter to only dotfiles packages
	dotfilesSet := make(map[string]bool)
	for _, pkg := range DotfilesPackages {
		dotfilesSet[pkg] = true
	}

	var filtered []Package
	for _, pkg := range allUpdates {
		if dotfilesSet[pkg.Name] {
			filtered = append(filtered, pkg)
		}
	}

	return filtered, nil
}
