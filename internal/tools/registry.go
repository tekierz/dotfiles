package tools

import (
	"sort"
	"sync"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// Global singleton registry with sync.Once for thread-safe lazy initialization.
// This avoids creating new registries and registering all 27 tools each time
// NewRegistry() would otherwise be called (15+ times across the codebase).
var (
	globalRegistry     *Registry
	globalRegistryOnce sync.Once
)

// Registry holds all registered tools
type Registry struct {
	tools map[string]Tool

	// installedCache caches IsInstalled() results to avoid repeated subprocess calls.
	// Key is tool ID, value is installation status.
	installedCache map[string]bool
	cachePopulated bool
	cacheMu        sync.RWMutex
}

// GetRegistry returns the global singleton registry.
// Use this for normal operations. Use NewRegistry() only for tests that need fresh registries.
func GetRegistry() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry()
	})
	return globalRegistry
}

// NewRegistry creates a new tool registry with all tools registered.
// Use GetRegistry() for normal operations. This is primarily for tests that need fresh registries.
func NewRegistry() *Registry {
	r := &Registry{
		tools:          make(map[string]Tool),
		installedCache: make(map[string]bool),
	}

	// Register all tools
	// Shell tools
	r.Register(NewZshTool())

	// Terminal tools
	r.Register(NewGhosttyTool())
	r.Register(NewTmuxTool())

	// Editor tools
	r.Register(NewNeovimTool())

	// File manager tools
	r.Register(NewYaziTool())

	// Git tools
	r.Register(NewGitTool())
	r.Register(NewLazyGitTool())
	r.Register(NewDeltaTool())

	// Container tools
	r.Register(NewLazyDockerTool())

	// Utility tools
	r.Register(NewFzfTool())
	r.Register(NewBatTool())
	r.Register(NewEzaTool())
	r.Register(NewZoxideTool())
	r.Register(NewRipgrepTool())
	r.Register(NewFdTool())
	r.Register(NewBtopTool())
	r.Register(NewGlowTool())
	r.Register(NewFswatchTool())
	r.Register(NewClaudeCodeTool())

	// GUI Apps
	r.Register(NewZenBrowserTool())
	r.Register(NewCursorTool())
	r.Register(NewLMStudioTool())
	r.Register(NewOBSTool())
	r.Register(NewRectangleTool())
	r.Register(NewRaycastTool())
	r.Register(NewIINATool())
	r.Register(NewAppCleanerTool())

	return r
}

// Register adds a tool to the registry
func (r *Registry) Register(t Tool) {
	r.tools[t.ID()] = t
}

// Get returns a tool by ID
func (r *Registry) Get(id string) (Tool, bool) {
	t, ok := r.tools[id]
	return t, ok
}

// All returns all registered tools sorted by name
func (r *Registry) All() []Tool {
	var tools []Tool
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// ByCategory returns tools in a specific category
func (r *Registry) ByCategory(cat Category) []Tool {
	var tools []Tool
	for _, t := range r.tools {
		if t.Category() == cat {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// ensureCache populates the installed cache if not already done
func (r *Registry) ensureCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if r.cachePopulated {
		return
	}

	for _, t := range r.tools {
		r.installedCache[t.ID()] = t.IsInstalled()
	}
	r.cachePopulated = true
}

// isInstalledCached returns cached installation status for a tool
func (r *Registry) isInstalledCached(id string) bool {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()
	return r.installedCache[id]
}

// RefreshCache invalidates and repopulates the installed cache
func (r *Registry) RefreshCache() {
	r.cacheMu.Lock()
	r.cachePopulated = false
	r.installedCache = make(map[string]bool)
	r.cacheMu.Unlock()

	r.ensureCache()
}

// InvalidateCache clears the cache without repopulating
func (r *Registry) InvalidateCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cachePopulated = false
	r.installedCache = make(map[string]bool)
}

// Installed returns all installed tools (uses cache)
func (r *Registry) Installed() []Tool {
	r.ensureCache()

	var tools []Tool
	for _, t := range r.tools {
		if r.isInstalledCached(t.ID()) {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// NotInstalled returns tools that are not installed (uses cache)
func (r *Registry) NotInstalled() []Tool {
	r.ensureCache()

	var tools []Tool
	for _, t := range r.tools {
		if !r.isInstalledCached(t.ID()) {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// NotInstalledForPlatform returns tools that are not installed and available for current platform (uses cache)
func (r *Registry) NotInstalledForPlatform() []Tool {
	r.ensureCache()

	platform := pkg.DetectPlatform()
	var tools []Tool
	for _, t := range r.tools {
		// Check if tool has packages for this platform (or "all")
		pkgs := t.Packages()[platform]
		if len(pkgs) == 0 {
			pkgs = t.Packages()["all"]
		}
		// Skip tools with no packages for this platform
		if len(pkgs) == 0 {
			continue
		}
		if !r.isInstalledCached(t.ID()) {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// CountForPlatform returns the number of tools available for the current platform
func (r *Registry) CountForPlatform() int {
	platform := pkg.DetectPlatform()
	count := 0
	for _, t := range r.tools {
		pkgs := t.Packages()[platform]
		if len(pkgs) == 0 {
			pkgs = t.Packages()["all"]
		}
		if len(pkgs) > 0 {
			count++
		}
	}
	return count
}

// IsLightweightMode returns true if running on a low-memory system (< 1GB RAM)
// where heavy tools should be skipped.
func (r *Registry) IsLightweightMode() bool {
	return pkg.IsLowMemorySystem(1024)
}

// AllForSystem returns all tools appropriate for the current system.
// On low-memory systems (< 1GB), heavy tools are excluded.
func (r *Registry) AllForSystem() []Tool {
	lightweight := r.IsLightweightMode()
	var tools []Tool
	for _, t := range r.tools {
		if lightweight && t.IsHeavy() {
			continue
		}
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// NotInstalledForSystem returns tools not installed and appropriate for the system.
// On low-memory systems (< 1GB), heavy tools are excluded.
func (r *Registry) NotInstalledForSystem() []Tool {
	r.ensureCache()

	platform := pkg.DetectPlatform()
	lightweight := r.IsLightweightMode()
	var tools []Tool
	for _, t := range r.tools {
		// Skip heavy tools on low-memory systems
		if lightweight && t.IsHeavy() {
			continue
		}
		// Check if tool has packages for this platform (or "all")
		pkgs := t.Packages()[platform]
		if len(pkgs) == 0 {
			pkgs = t.Packages()["all"]
		}
		// Skip tools with no packages for this platform
		if len(pkgs) == 0 {
			continue
		}
		if !r.isInstalledCached(t.ID()) {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// HeavyTools returns all tools marked as resource-heavy
func (r *Registry) HeavyTools() []Tool {
	var tools []Tool
	for _, t := range r.tools {
		if t.IsHeavy() {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// Configurable returns tools that have configuration options
func (r *Registry) Configurable() []Tool {
	var tools []Tool
	for _, t := range r.tools {
		if t.HasConfig() {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// InstallAll installs all tools using the provided package manager
func (r *Registry) InstallAll(mgr pkg.PackageManager) error {
	for _, t := range r.tools {
		if err := t.Install(mgr); err != nil {
			return err
		}
	}
	// Invalidate cache after installations
	r.InvalidateCache()
	return nil
}

// InstallByCategory installs all tools in a category
func (r *Registry) InstallByCategory(mgr pkg.PackageManager, cat Category) error {
	for _, t := range r.ByCategory(cat) {
		if err := t.Install(mgr); err != nil {
			return err
		}
	}
	// Invalidate cache after installations
	r.InvalidateCache()
	return nil
}

// ApplyAllConfigs applies configs for all configurable tools
func (r *Registry) ApplyAllConfigs(theme string) error {
	for _, t := range r.Configurable() {
		if err := t.ApplyConfig(theme); err != nil {
			return err
		}
	}
	return nil
}

// Categories returns all unique categories
func (r *Registry) Categories() []Category {
	seen := make(map[Category]bool)
	var cats []Category
	for _, t := range r.tools {
		if !seen[t.Category()] {
			seen[t.Category()] = true
			cats = append(cats, t.Category())
		}
	}
	return cats
}

// Count returns the total number of registered tools
func (r *Registry) Count() int {
	return len(r.tools)
}

// InstalledCount returns the number of installed tools (uses cache)
func (r *Registry) InstalledCount() int {
	r.ensureCache()

	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	count := 0
	for _, installed := range r.installedCache {
		if installed {
			count++
		}
	}
	return count
}
