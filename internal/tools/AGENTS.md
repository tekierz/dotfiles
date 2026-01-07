# Tools Package

Tool registry system for managing terminal tools.

## Key Files

| File | Purpose |
|------|---------|
| `tool.go` | Tool interface and BaseTool implementation |
| `registry.go` | Registry for tool registration and querying |
| Individual files | One file per tool (zsh.go, ghostty.go, etc.) |

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
    GenerateConfig(theme string) string      // Generate config content
    ApplyConfig(theme string) error          // Apply configuration
}
```

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

## Adding a New Tool

1. Create `newtool.go`:

```go
package tools

import "github.com/tekierz/dotfiles/internal/pkg"

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

## Registry Methods

```go
registry.All()           // All tools sorted by name
registry.ByCategory(cat) // Tools in specific category
registry.Installed()     // Currently installed tools
registry.NotInstalled()  // Not installed tools
registry.Configurable()  // Tools with config options
registry.Count()         // Total tool count
registry.InstalledCount()// Installed count
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
