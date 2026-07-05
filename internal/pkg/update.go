package pkg

import (
	"errors"
	"fmt"
	"sort"
)

var allManagers = AllManagers

// UpdateResult represents the result of an update operation
type UpdateResult struct {
	Package Package
	Success bool
	Error   error
}

// CheckAllUpdates checks for updates across all available package managers
func CheckAllUpdates() ([]Package, error) {
	var allPackages []Package
	var managerErrs []error

	managers := allManagers()
	if len(managers) == 0 {
		return nil, fmt.Errorf("no package managers available")
	}

	for _, mgr := range managers {
		packages, err := mgr.CheckOutdated()
		if err != nil {
			// Keep checking other managers, but surface this failure to callers.
			managerErrs = append(managerErrs, fmt.Errorf("%s: %w", mgr.Name(), err))
			continue
		}
		allPackages = append(allPackages, packages...)
	}

	// Deduplicate exact duplicates only, keyed on Name+InstalledBy. This removes
	// the paru/pacman double-listing artifact without erasing a package's source
	// manager: two records with the same name but different InstalledBy (e.g. a
	// "pacman" vs an "aur" entry) are legitimately distinct and must both survive
	// so each can be routed via the manager that can actually upgrade it.
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

	return allPackages, errors.Join(managerErrs...)
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
	"dotfiles",
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

	return filtered, err
}
