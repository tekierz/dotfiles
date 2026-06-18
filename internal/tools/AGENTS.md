# Tools Package

Tool registry system for managing terminal tools.

## Key Files

| File | Purpose |
|------|---------|
| `tool.go` | Tool interface, BaseTool implementation, and the shared `writeToolConfig()` helper |
| `registry.go` | Registry for tool registration and querying |
| `simple_tools.go` | Data table of pure-metadata "simple" tools (bat, eza, zoxide, ripgrep, fd, fswatch, delta, lazydocker) built via `newSimpleTool(spec)` |
| Individual files | One file per tool with custom behavior (zsh.go, ghostty.go, etc.) |

## Tool Interface

```go
type Tool interface {
    ID() string                              // Unique identifier (e.g., "zsh")
    Name() string                            // Display name (e.g., "Zsh")
    Description() string                     // Short description
    Icon() string                            // Nerd Font icon
    Category() Category                      // Tool category
    Packages() map[pkg.Platform][]string     // Platform-specific packages
    IsInstalled() bool                       // Check if installed
    Install(mgr pkg.PackageManager) error    // Install the tool
    ConfigPaths() []string                   // Config file paths
    HasConfig() bool                         // Has configurable options

    // Resource requirements
    IsHeavy() bool                           // Whether tool needs significant resources (skipped on low-memory systems)

    // UI metadata for installer screens (added in v2.1 beta)
    UIGroup() UIGroup                        // Which installer group (empty = dedicated screen)
    ConfigScreen() int                       // Which config screen constant (0 = part of group screen)
    DefaultEnabled() bool                    // Default enabled state in installer
    PlatformFilter() pkg.Platform            // Empty for all platforms, or specific platform
}
```

> The interface no longer carries `GenerateConfig(theme)`/`ApplyConfig(theme)`.
> Config writing happens through package-level `WriteXConfig(cfg, theme)`
> functions (e.g. `WriteZshConfig`, `WriteGhosttyConfig`) used by the installer;
> they share a `writeToolConfig(path, content)` helper in `tool.go`.

## Categories

```go
const (
    CategoryShell     Category = "shell"
    CategoryTerminal  Category = "terminal"
    CategoryEditor    Category = "editor"
    CategoryFile      Category = "file"
    CategoryGit       Category = "git"
    CategoryContainer Category = "container"
    CategoryUtility   Category = "utility"
    CategoryApp       Category = "app"
)
```

## UI Groups

`UIGroup` (added in v2.1 beta) determines which installer section a tool appears in:

```go
const (
    UIGroupNone         UIGroup = ""              // Has a dedicated config screen
    UIGroupCLITools     UIGroup = "cli-tools"
    UIGroupCLIUtilities UIGroup = "cli-utilities"
    UIGroupGUIApps      UIGroup = "gui-apps"
    UIGroupMacApps      UIGroup = "macos-apps"
    UIGroupUtilities    UIGroup = "utilities"     // Shell scripts (hk, caff, sshh)
)
```

## Adding a New Tool

If the tool is pure metadata (a package + optional config path, default
`IsInstalled`/`Install` behavior, no custom config writer), add an entry to the
`simpleTools` table in `simple_tools.go` — `newSimpleTool(spec)` builds the
`BaseTool` and `registerSimpleTools()` registers it automatically; no new file is
needed.

If the tool needs custom behavior (its own config writer, `IsInstalled`, or
install logic), give it its own file:

1. Create `newtool.go`:

```go
package tools

import (
    "os"
    "path/filepath"

    "github.com/tekierz/dotfiles/internal/pkg"
)

type NewToolTool struct {
    BaseTool
}

func NewNewToolTool() *NewToolTool {
    home, _ := os.UserHomeDir()
    return &NewToolTool{
        BaseTool: BaseTool{
            id:          "newtool",
            name:        "New Tool",
            description: "Description here",
            icon:        "",
            category:    CategoryUtility,
            packages: map[pkg.Platform][]string{
                pkg.PlatformMacOS:  {"newtool"},
                pkg.PlatformArch:   {"newtool"},
                pkg.PlatformDebian: {"newtool"},
            },
            configPaths: []string{
                filepath.Join(home, ".config", "newtool", "config"),
            },
            // UI metadata (drives installer group placement)
            uiGroup:        UIGroupCLIUtilities,
            configScreen:   0, // 0 = part of group screen (no dedicated config screen)
            defaultEnabled: true,
        },
    }
}
```

2. Register in `registry.go`:

```go
func NewRegistry() *Registry {
    // ...
    r.Register(NewNewToolTool())
    return r
}
```

> Note: `NewRegistry()` builds and registers all tools and is primarily for tests
> that need a fresh registry. For normal operations use the global singleton
> `GetRegistry()`, which lazily constructs a single shared `Registry` via `sync.Once`.

## Registry Methods

```go
tools.GetRegistry()      // Global singleton (recommended for normal use)
tools.NewRegistry()      // Fresh registry (primarily for tests)

registry.All()                    // All tools sorted by name
registry.ByCategory(cat)          // Tools in specific category
registry.Installed()              // Currently installed tools
registry.NotInstalled()           // Not installed tools
registry.Configurable()           // Tools with config options
registry.Count()                  // Total tool count
registry.InstalledCount()         // Installed count

// Platform/system-aware queries (v2.1)
registry.AllForSystem()           // All tools, excluding heavy ones on low-memory systems
registry.NotInstalledForSystem()  // Not installed + platform/system appropriate
registry.NotInstalledForPlatform()// Not installed + available for current platform
registry.CountForPlatform()       // Tool count for current platform
registry.HeavyTools()             // Tools marked resource-heavy

// Install cache management (v2.1)
registry.RefreshCache()           // Invalidate and repopulate the installed cache
registry.InvalidateCache()        // Clear the cache without repopulating
```

## Platform Packages

Specify different package names per platform:

```go
packages: map[pkg.Platform][]string{
    pkg.PlatformMacOS:  {"ghostty"},           // Homebrew
    pkg.PlatformArch:   {"ghostty-git"},       // AUR
    pkg.PlatformDebian: {"ghostty"},           // apt
}
```
