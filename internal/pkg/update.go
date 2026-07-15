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
		provider := ExecutionProviderForManager(mgr)
		if provider == "" {
			managerErrs = append(managerErrs, fmt.Errorf("%s: unsupported update provider", mgr.Name()))
			continue
		}
		packages, err := mgr.CheckOutdated()
		if err != nil {
			// Keep checking other managers, but surface this failure to callers.
			managerErrs = append(managerErrs, fmt.Errorf("%s: %w", mgr.Name(), err))
			continue
		}
		for _, discovered := range packages {
			discovered.provider = provider
			allPackages = append(allPackages, discovered)
		}
	}

	// Deduplicate exact duplicates only, keyed on Name+InstalledBy+Provider. This removes
	// the paru/pacman double-listing artifact without erasing a package's source
	// manager: two records with the same name but different InstalledBy (e.g. a
	// "pacman" vs an "aur" entry) are legitimately distinct and must both survive
	// so each can be routed via the manager that can actually upgrade it.
	seen := make(map[string]bool)
	var deduped []Package
	for _, p := range allPackages {
		key := p.Name + "\x00" + p.InstalledBy + "\x00" + string(p.ExecutionProvider())
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

// DotfilesPackages is retained for source compatibility with integrations that
// used the former static updater inventory. Runtime product checks use the tool
// registry's managed platform projection instead.
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

// DotfilesDebianPackages is the retained Debian/Pi compatibility inventory.
// Runtime product checks derive Debian names from the tool registry.
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

// CheckDotfilesUpdates preserves the former public static-inventory behavior.
// Product callers should pass the runtime registry projection to
// CheckManagedUpdates.
func CheckDotfilesUpdates() ([]Package, error) {
	// Pick the allow-list appropriate for the current platform so that
	// Debian-renamed packages (fd-find etc.) are recognised correctly.
	var allowList []string
	switch DetectPlatform() {
	case PlatformMacOS, PlatformArch, PlatformUnknown:
		allowList = DotfilesPackages
	case PlatformDebian, PlatformPi:
		allowList = DotfilesDebianPackages
	}

	return CheckManagedUpdates(allowList)
}

// CheckManagedUpdates checks all available providers and returns only packages
// owned by the supplied runtime product projection.
func CheckManagedUpdates(managedPackages []string) ([]Package, error) {
	allUpdates, err := CheckAllUpdates()
	return FilterManagedUpdates(allUpdates, managedPackages), err
}

// FilterManagedUpdates is the pure managed-package filter shared by CLI and
// TUI update checks. It preserves discovery order and provider authority.
func FilterManagedUpdates(updates []Package, managedPackages []string) []Package {
	managed := make(map[string]struct{}, len(managedPackages))
	for _, name := range managedPackages {
		if name != "" {
			managed[name] = struct{}{}
		}
	}
	filtered := make([]Package, 0, len(updates))
	for _, update := range updates {
		if _, ok := managed[update.Name]; ok {
			filtered = append(filtered, update)
		}
	}
	return filtered
}
