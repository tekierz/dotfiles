package tools

import (
	"sort"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// ManagedPackagesForPlatform returns the deterministic package-name projection
// owned by the runtime tool registry for one platform. It performs no package
// manager queries and excludes tools with no package on that platform.
func (r *Registry) ManagedPackagesForPlatform(platform pkg.Platform) []string {
	if r == nil {
		return nil
	}
	managed := make(map[string]struct{})
	for _, tool := range r.All() {
		for _, name := range PackagesForPlatform(tool.Packages(), platform) {
			if name != "" {
				managed[name] = struct{}{}
			}
		}
	}
	packages := make([]string, 0, len(managed))
	for name := range managed {
		packages = append(packages, name)
	}
	sort.Strings(packages)
	return packages
}
