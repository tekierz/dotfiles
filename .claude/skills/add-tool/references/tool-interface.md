# Tool Interface Reference

Complete documentation for the Tool interface in `internal/tools/tool.go`.

## Interface Definition

```go
type Tool interface {
    // Identity
    ID() string          // Unique identifier (e.g., "ghostty", "lazygit")
    Name() string        // Display name (e.g., "Ghostty", "LazyGit")
    Description() string // Short description
    Icon() string        // Nerd font icon
    Category() Category  // Tool category

    // Package management
    Packages() map[pkg.Platform][]string // Platform-specific package names
    IsInstalled() bool                   // Check if tool is installed
    Install(mgr pkg.PackageManager) error

    // Configuration
    ConfigPaths() []string              // Config file paths
    HasConfig() bool                    // Whether tool has configurable options
    GenerateConfig(theme string) string // Generate config content
    ApplyConfig(theme string) error     // Write config to disk

    // Resource requirements
    IsHeavy() bool // Skip on low-memory systems (< 1GB RAM)

    // UI metadata
    UIGroup() UIGroup             // Which installer group
    ConfigScreen() int            // Config screen constant (0 = group screen)
    DefaultEnabled() bool         // Default state in installer
    PlatformFilter() pkg.Platform // Platform restriction
}
```

## BaseTool Fields

```go
type BaseTool struct {
    id          string
    name        string
    description string
    icon        string
    category    Category
    packages    map[pkg.Platform][]string
    configPaths []string
    heavyTool   bool // Skip on Pi Zero 2, etc.

    // UI metadata
    uiGroup        UIGroup
    configScreen   int
    defaultEnabled bool
    platformFilter pkg.Platform
}
```

## Platform Constants

```go
const (
    PlatformMacOS  Platform = "macos"
    PlatformArch   Platform = "arch"
    PlatformDebian Platform = "debian"
)
```

## Category Constants

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

## UIGroup Constants

```go
const (
    UIGroupNone         UIGroup = "" // Has dedicated config screen
    UIGroupCLITools     UIGroup = "cli-tools"
    UIGroupCLIUtilities UIGroup = "cli-utilities"
    UIGroupGUIApps      UIGroup = "gui-apps"
    UIGroupMacApps      UIGroup = "macos-apps"
    UIGroupUtilities    UIGroup = "utilities" // Shell scripts
)
```

## BaseTool Method Implementations

### IsInstalled (default)

```go
func (t *BaseTool) IsInstalled() bool {
    platform := pkg.DetectPlatform()
    mgr := pkg.DetectManager()
    if mgr == nil {
        return false
    }

    pkgs := t.packages[platform]
    if len(pkgs) == 0 {
        pkgs = t.packages["all"]
    }
    if len(pkgs) == 0 {
        return false
    }

    return mgr.IsInstalled(pkgs[0])
}
```

### Install (default)

```go
func (t *BaseTool) Install(mgr pkg.PackageManager) error {
    platform := pkg.DetectPlatform()
    pkgs := t.packages[platform]
    if len(pkgs) == 0 {
        pkgs = t.packages["all"]
    }
    if len(pkgs) == 0 {
        return nil // No packages for this platform
    }
    return mgr.Install(pkgs...)
}
```

### HasConfig (default)

```go
func (t *BaseTool) HasConfig() bool {
    return len(t.configPaths) > 0
}
```

## When to Override Methods

### Override IsInstalled when:

- App may be installed outside package manager (manual download, AppImage, etc.)
- App has multiple possible installation methods
- Need to check for app bundles on macOS

### Override ApplyConfig when:

- Tool has user-configurable settings from DeepDiveConfig
- Need to generate config files with theme colors
- Need to set up plugins or additional components

### Override GenerateConfig when:

- Tool config content depends on theme or user settings
- Need to template configuration files

## Examples of Custom Implementations

### IsInstalled with Multiple Sources

```go
func (t *ZenBrowserTool) IsInstalled() bool {
    // Check command
    if _, err := exec.LookPath("zen-browser"); err == nil {
        return true
    }
    if _, err := exec.LookPath("zen"); err == nil {
        return true
    }
    // Check flatpak
    if isFlatpakInstalled("io.github.nicothin.zen_browser") {
        return true
    }
    // Check AppImage
    if hasAppImage("zen", "zen-browser") {
        return true
    }
    // Check desktop entry
    if hasDesktopEntry("zen-browser", "zen") {
        return true
    }
    // Check macOS app bundle
    if hasMacOSApp("Zen Browser", "Zen") {
        return true
    }
    // Fall back to package manager
    return t.BaseTool.IsInstalled()
}
```

### ApplyConfig with Theme

```go
func (t *GhosttyTool) ApplyConfig(theme string) error {
    home, _ := os.UserHomeDir()
    configDir := filepath.Join(home, ".config", "ghostty")

    // Ensure directory exists
    if err := os.MkdirAll(configDir, 0700); err != nil {
        return err
    }

    // Generate config content
    content := t.GenerateConfig(theme)

    // Write config file
    configPath := filepath.Join(configDir, "config")
    return os.WriteFile(configPath, []byte(content), 0600)
}
```

## Registry Integration

Tools are registered in `internal/tools/registry.go`:

```go
func NewRegistry() *Registry {
    r := &Registry{
        tools:          make(map[string]Tool),
        installedCache: make(map[string]bool),
    }

    // Register tools by category
    r.Register(NewZshTool())
    r.Register(NewGhosttyTool())
    // ... etc

    return r
}
```

## Testing Requirements

Every tool should have tests for:

1. **Platform filter** - Verify correct platform restriction
2. **UIGroup** - Verify correct UI group assignment
3. **IsInstalled** - Verify doesn't panic, handles edge cases
4. **Package definition** - Verify packages defined for appropriate platforms
