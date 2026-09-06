# Detection Helper Functions

Helper functions in `internal/tools/apps.go` for detecting installed applications.

## hasMacOSApp

Check if a .app bundle exists in /Applications or ~/Applications.

```go
func hasMacOSApp(names ...string) bool
```

**Usage:**
```go
if hasMacOSApp("Raycast") {
    return true
}
// Multiple possible names
if hasMacOSApp("LM Studio", "LMStudio") {
    return true
}
```

**Checks:**
- `/Applications/*.app`
- `~/Applications/*.app`
- Case-insensitive matching

## hasAppImage

Check if an AppImage exists in common Linux locations.

```go
func hasAppImage(patterns ...string) bool
```

**Usage:**
```go
if hasAppImage("cursor", "Cursor") {
    return true
}
```

**Checks:**
- `~/Applications/`
- `~/.local/bin/`
- `/opt/`
- `/usr/local/bin/`

**Matches:**
- Files containing pattern AND ending in `.appimage` or containing `appimage`
- Example: `Cursor-0.45.11-x86_64.AppImage`

## hasDesktopEntry

Check if a .desktop file exists for the application.

```go
func hasDesktopEntry(names ...string) bool
```

**Usage:**
```go
if hasDesktopEntry("zen-browser", "zen") {
    return true
}
```

**Checks:**
- `~/.local/share/applications/`
- `~/.local/share/flatpak/exports/share/applications/`
- `/usr/share/applications/`
- `/usr/local/share/applications/`
- `/var/lib/flatpak/exports/share/applications/`

**Also checks:**
- Filename matches (e.g., `zen-browser.desktop`)
- Exec= field in AppImage entries (e.g., `appimagekit_xxx-Cursor.desktop`)

## isFlatpakInstalled

Check if a Flatpak application is installed.

```go
func isFlatpakInstalled(appIDs ...string) bool
```

**Usage:**
```go
if isFlatpakInstalled("io.github.nicothin.zen_browser", "app.zen_browser.zen") {
    return true
}
```

**How it works:**
- Runs `flatpak info <appID>` for each ID
- Returns true if any command succeeds

## Best Practices

### Order of Checks

For cross-platform apps, check in this order:

```go
func (t *MyTool) IsInstalled() bool {
    // 1. Check command in PATH
    if _, err := exec.LookPath("mytool"); err == nil {
        return true
    }

    // 2. Check Linux-specific (if applicable)
    if isFlatpakInstalled("com.example.mytool") {
        return true
    }
    if hasAppImage("mytool", "MyTool") {
        return true
    }
    if hasDesktopEntry("mytool") {
        return true
    }

    // 3. Check macOS app bundle
    if hasMacOSApp("MyTool") {
        return true
    }

    // 4. Fall back to package manager
    return t.BaseTool.IsInstalled()
}
```

### For macOS-Only Apps

```go
func (t *MacOnlyTool) IsInstalled() bool {
    // Check app bundle first (faster, more common)
    if hasMacOSApp("MacOnlyTool") {
        return true
    }
    // Fall back to Homebrew
    return t.BaseTool.IsInstalled()
}
```

### Common App Bundle Names

| Tool | App Bundle Names |
|------|-----------------|
| Raycast | `Raycast` |
| Rectangle | `Rectangle` |
| IINA | `IINA` |
| AppCleaner | `AppCleaner` |
| Cursor | `Cursor` |
| OBS | `OBS`, `OBS Studio` |
| LM Studio | `LM Studio`, `LMStudio` |
| Zen Browser | `Zen Browser`, `Zen` |

### Common Flatpak IDs

| Tool | Flatpak ID(s) |
|------|--------------|
| Zen Browser | `io.github.nicothin.zen_browser`, `app.zen_browser.zen` |
| OBS | `com.obsproject.Studio` |

### Finding Flatpak IDs

```bash
# List installed flatpaks
flatpak list --app

# Search for an app
flatpak search zen-browser
```

### Finding Desktop Entry Names

```bash
# List desktop files
ls ~/.local/share/applications/
ls /usr/share/applications/

# Find by pattern
find ~/.local/share/applications -name "*cursor*"
```

## Edge Cases

### Empty Arguments

All helper functions handle empty arguments gracefully:

```go
hasMacOSApp()           // returns false
hasAppImage()           // returns false
hasDesktopEntry()       // returns false
isFlatpakInstalled()    // returns false
```

### Non-Existent Directories

Functions silently skip directories that don't exist:
- No error if `/Applications` doesn't exist on Linux
- No error if `~/Applications` doesn't exist

### Case Sensitivity

- `hasMacOSApp` - Case-insensitive matching
- `hasAppImage` - Case-insensitive matching
- `hasDesktopEntry` - Case-insensitive filename, case-sensitive Exec= field
