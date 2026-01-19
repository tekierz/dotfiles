---
name: add-tool
description: This skill should be used when the user asks to "add a new tool", "create a tool", "add an app", "register a tool", "implement a new tool", or wants to extend the dotfiles tool registry with new CLI tools, GUI apps, or macOS-only apps.
---

# Adding New Tools to Dotfiles

This skill guides the creation of new tools for the dotfiles TUI installer. Tools represent CLI utilities, GUI applications, or macOS-specific apps that can be installed and configured through the TUI.

## Quick Overview

Adding a new tool requires:
1. Create tool file in `internal/tools/`
2. Register in `internal/tools/registry.go`
3. Add tests in `internal/tools/*_test.go`

## Tool Categories

| Category | UIGroup | Examples |
|----------|---------|----------|
| CLI Tools | `UIGroupCLITools` | lazygit, lazydocker |
| CLI Utilities | `UIGroupCLIUtilities` | bat, eza, fzf, btop |
| GUI Apps (cross-platform) | `UIGroupGUIApps` | cursor, obs, zen-browser |
| macOS Apps | `UIGroupMacApps` | raycast, rectangle, iina |
| Shell Scripts | `UIGroupUtilities` | hk, caff, sshh |
| Core Tools | `UIGroupNone` | ghostty, tmux, zsh, neovim |

## Step 1: Create Tool File

Create `internal/tools/newtool.go`:

```go
package tools

import (
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
            id:          "newtool",           // Unique identifier
            name:        "NewTool",           // Display name
            description: "Brief description",
            icon:        "󰊠",                 // Nerd font icon
            category:    CategoryUtility,     // See categories below
            packages: map[pkg.Platform][]string{
                pkg.PlatformMacOS:  {"newtool"},
                pkg.PlatformArch:   {"newtool"},
                pkg.PlatformDebian: {"newtool"},
            },
            configPaths: []string{
                filepath.Join(home, ".config", "newtool", "config"),
            },
            // UI metadata
            uiGroup:        UIGroupCLIUtilities,
            configScreen:   0,     // 0 = part of group screen
            defaultEnabled: true,  // Enabled by default in installer
            platformFilter: "",    // Empty = all platforms
        },
    }
}
```

## Step 2: Platform-Specific Considerations

### macOS-Only Apps

Set `platformFilter` to restrict to macOS:

```go
platformFilter: pkg.PlatformMacOS,
uiGroup:        UIGroupMacApps,
```

### Apps with Custom IsInstalled

Override `IsInstalled()` for apps that may be installed outside package managers:

```go
func (t *NewToolTool) IsInstalled() bool {
    // Check macOS app bundle
    if hasMacOSApp("NewTool") {
        return true
    }
    // Check command availability
    if _, err := exec.LookPath("newtool"); err == nil {
        return true
    }
    // Fall back to package manager
    return t.BaseTool.IsInstalled()
}
```

For Linux apps, also check:
- `hasDesktopEntry("newtool")` - .desktop files
- `hasAppImage("newtool")` - AppImage binaries
- `isFlatpakInstalled("com.example.newtool")` - Flatpak

## Step 3: Register the Tool

Add to `internal/tools/registry.go` in `NewRegistry()`:

```go
// In the appropriate section based on category
r.Register(NewNewToolTool())
```

## Step 4: Add Tests

Add tests to verify platform filtering and IsInstalled behavior:

```go
// In internal/tools/newtool_test.go or apps_test.go
func TestNewToolTool_PlatformFilter(t *testing.T) {
    tool := NewNewToolTool()
    // For cross-platform tools
    if tool.PlatformFilter() != "" {
        t.Error("NewTool should be cross-platform")
    }
    // OR for macOS-only
    if tool.PlatformFilter() != pkg.PlatformMacOS {
        t.Error("NewTool should be macOS-only")
    }
}
```

## Categories Reference

| Constant | Use For |
|----------|---------|
| `CategoryShell` | Shell environments (zsh) |
| `CategoryTerminal` | Terminal emulators, multiplexers |
| `CategoryEditor` | Text editors, IDEs |
| `CategoryFile` | File managers |
| `CategoryGit` | Git tools |
| `CategoryContainer` | Docker/container tools |
| `CategoryUtility` | CLI utilities |
| `CategoryApp` | GUI applications |

## Checklist

Before committing:

- [ ] Tool file created with all required fields
- [ ] Registered in registry.go
- [ ] Platform filter set correctly (empty for cross-platform, `pkg.PlatformMacOS` for macOS-only)
- [ ] UIGroup matches tool type
- [ ] IsInstalled override added if app may be installed outside package manager
- [ ] Tests added for platform filter and IsInstalled
- [ ] `go build ./...` passes
- [ ] `go test ./internal/tools/...` passes

## Additional Resources

### Reference Files

- **`references/tool-interface.md`** - Complete Tool interface documentation
- **`references/helper-functions.md`** - Detection helper functions (hasMacOSApp, hasAppImage, etc.)

### Examples

- **`examples/cli-utility.go`** - Simple CLI utility template
- **`examples/macos-app.go`** - macOS-only app with IsInstalled override
