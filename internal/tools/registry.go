package tools

import (
	"sort"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// Registry holds all registered tools
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new tool registry with all tools registered
func NewRegistry() *Registry {
	r := &Registry{
		tools: make(map[string]Tool),
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

// Installed returns all installed tools
func (r *Registry) Installed() []Tool {
	var tools []Tool
	for _, t := range r.tools {
		if t.IsInstalled() {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// NotInstalled returns tools that are not installed
func (r *Registry) NotInstalled() []Tool {
	var tools []Tool
	for _, t := range r.tools {
		if !t.IsInstalled() {
			tools = append(tools, t)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// NotInstalledForPlatform returns tools that are not installed and available for current platform
func (r *Registry) NotInstalledForPlatform() []Tool {
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
		if !t.IsInstalled() {
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
	return nil
}

// InstallByCategory installs all tools in a category
func (r *Registry) InstallByCategory(mgr pkg.PackageManager, cat Category) error {
	for _, t := range r.ByCategory(cat) {
		if err := t.Install(mgr); err != nil {
			return err
		}
	}
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

// InstalledCount returns the number of installed tools
func (r *Registry) InstalledCount() int {
	count := 0
	for _, t := range r.tools {
		if t.IsInstalled() {
			count++
		}
	}
	return count
}
