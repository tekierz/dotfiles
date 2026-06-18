# Package Manager Abstraction

Cross-platform package manager interface.

## Key Files

| File | Purpose |
|------|---------|
| `manager.go` | PackageManager interface and platform detection |
| `brew.go` | Homebrew implementation (macOS) |
| `pacman.go` | Pacman/Paru implementation (Arch Linux) |
| `apt.go` | APT implementation (Debian/Ubuntu) |
| `update.go` | Update/install helpers (`InstallPackage`, `InstallPackages`, `IsPackageInstalled`), `UpdateResult`, the `DotfilesPackages` managed-package list, and update-checking functions (`CheckAllUpdates`, `CheckDotfilesUpdates`) |

## PackageManager Interface

Requires the `context` and `internal/runner` packages (for the streaming methods).

```go
type PackageManager interface {
    Name() string                          // "brew", "pacman", "apt"
    IsAvailable() bool                     // Check if available on system
    Install(packages ...string) error      // Install packages
    Uninstall(packages ...string) error    // Remove packages
    IsInstalled(pkg string) bool           // Check if installed
    GetVersion(pkg string) (string, error) // Get installed version
    CheckOutdated() ([]Package, error)     // List outdated packages
    Update(packages ...string) error       // Update specific packages
    UpdateAll() error                      // Update all packages
    Search(query string) ([]Package, error)// Search for packages
    ListInstalled() ([]Package, error)     // List all installed
    NeedsSudo() bool                       // True if operations require elevated privileges
    InstallStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error)  // Install with real-time output
    UpdateStreaming(ctx context.Context, packages ...string) (*runner.StreamingCmd, error)   // Update with real-time output
    UpdateAllStreaming(ctx context.Context) (*runner.StreamingCmd, error)                     // Update all with real-time output
}
```

## Platform Detection

```go
platform := pkg.DetectPlatform() // Returns PlatformMacOS, PlatformArch, etc.
mgr := pkg.DetectManager()       // Returns appropriate PackageManager or nil
```

Platforms:
- `PlatformMacOS` - Darwin, uses Homebrew
- `PlatformArch` - Arch Linux/CachyOS, uses Pacman/Paru
- `PlatformDebian` - Debian/Ubuntu, uses APT
- `PlatformPi` - Raspberry Pi (detected via device-tree model / cpuinfo), uses APT like Debian
- `PlatformUnknown` - Unsupported

## Adding a New Package Manager

1. Create `newmgr.go`:

```go
package pkg

type NewMgrManager struct {
    mgrPath string
}

func NewNewMgrManager() *NewMgrManager {
    path, _ := exec.LookPath("newmgr")
    return &NewMgrManager{mgrPath: path}
}

func (m *NewMgrManager) Name() string { return "newmgr" }
func (m *NewMgrManager) IsAvailable() bool { return m.mgrPath != "" }
// ... implement remaining interface methods
```

2. Add the new platform constant to the `Platform` const block and to `detectPlatformImpl()` in `manager.go`.

3. Add to `detectManagerImpl()` in `manager.go` (`DetectManager()` is a cached `sync.Once` wrapper that calls `detectManagerImpl()`):

```go
case PlatformNewPlatform:
    if mgr := NewNewMgrManager(); mgr.IsAvailable() {
        return mgr
    }
```

4. Register the new manager in `AllManagers()` too. It returns every available manager, and `CheckAllUpdates()`/`UpdateAllPackages()` iterate over it (a manager missing here is invisible to those functions even if `detectManagerImpl()` returns it).

## Package Structure

```go
type Package struct {
    Name           string // Package name
    CurrentVersion string // Installed version
    LatestVersion  string // Available version
    Outdated       bool   // Needs update
    InstalledBy    string // "brew", "pacman", etc.
    Description    string // Package description
}
```

## Update Checking

`pkg.DotfilesPackages` is the canonical list of packages managed by dotfiles; `CheckDotfilesUpdates()` iterates over it.

```go
// Check all managed packages for updates (uses pkg.DotfilesPackages)
updates, err := pkg.CheckDotfilesUpdates()

// Check specific package
outdated, err := mgr.CheckOutdated()
```
