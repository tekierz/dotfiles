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

	// Deduplicate exact duplicates only, keyed on Name+InstalledBy. This removes
	// the paru/pacman double-listing artifact without erasing a package's source
	// manager: two records with the same name but different InstalledBy (e.g. a
	// "pacman" vs an "aur" entry) are legitimately distinct and must both survive
	// so UpdatePackages() routes each via the manager that can actually upgrade it.
	seen := make(map[string]bool)
	var deduped []Package
	for _, p := range allPackages {
		key := p.Name + "\x00" + p.InstalledBy
		if !seen[key] {
			seen[key] = true
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
		// AUR-sourced packages can only be upgraded by an AUR helper. If paru is
		// not actually available, NewPacmanManager(true) silently falls back to
		// plain pacman (useParu==false), whose `sudo pacman -S <aurpkg>` cannot
		// build AUR targets. Return nil in that case so UpdatePackages reports a
		// clear "package manager aur not available" error instead of dispatching
		// a command that is guaranteed to fail on AUR packages.
		mgr := NewPacmanManager(true)
		if !mgr.useParu {
			return nil
		}
		return mgr
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

// DotfilesPackages is the canonical dotfiles package allow-list using macOS/Arch
// names. On Debian/Pi several packages use different names (e.g. fd -> fd-find);
// use DotfilesDebianPackages for that platform and CheckDotfilesUpdates for
// platform-aware filtering.
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

// DotfilesDebianPackages is the Debian/Pi variant of the allow-list, using the
// stock Debian package names. Packages unavailable in stock repos are omitted
// so the filter does not match phantom updates (glow, lazygit, lazydocker).
var DotfilesDebianPackages = []string{
	// Core shell
	"zsh",
	"zsh-syntax-highlighting",
	"zsh-autosuggestions",
	// zsh-completions not packaged separately on Debian

	// Terminal tools
	"tmux",
	"neovim",
	"fzf",
	"ripgrep",
	"fd-find", // Debian name for fd
	"bat",
	"eza",
	"zoxide",
	"btop",
	// glow omitted: not in stock Debian repos (Charm keyring required)

	// Git tools
	"git",
	"git-delta",
	// lazygit omitted: not in stock Debian repos
	// lazydocker omitted: not in stock Debian repos

	// Utilities
	"fastfetch",
	"ncdu",
	"fswatch",
	// tlrc, duf, dust omitted: not in stock Debian repos
}

// CheckDotfilesUpdates checks for updates only for dotfiles-managed packages.
// On Debian/Pi, Debian-specific package names (e.g., fd-find instead of fd)
// are used for filtering so renamed packages are not silently dropped.
func CheckDotfilesUpdates() ([]Package, error) {
	allUpdates, err := CheckAllUpdates()
	if err != nil {
		return nil, err
	}

	// Pick the allow-list appropriate for the current platform so that
	// Debian-renamed packages (fd-find etc.) are recognised correctly.
	var allowList []string
	switch DetectPlatform() {
	case PlatformDebian, PlatformPi:
		allowList = DotfilesDebianPackages
	default:
		allowList = DotfilesPackages
	}

	dotfilesSet := make(map[string]bool, len(allowList))
	for _, pkg := range allowList {
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
