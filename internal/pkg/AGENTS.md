# Package Manager Abstraction

Cross-platform package manager interface.

## Key Files

| File | Purpose |
|------|---------|
| `manager.go` | PackageManager interface and platform detection |
| `brew.go` | Homebrew implementation (macOS) |
| `pacman.go` | Pacman/Paru implementation (Arch Linux) |
| `apt.go` | APT implementation (Debian/Ubuntu) |
| `update.go` | Update checking utilities |

## PackageManager Interface

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

2. Add to `DetectManager()` in `manager.go`:

```go
case PlatformNewPlatform:
    if mgr := NewNewMgrManager(); mgr.IsAvailable() {
        return mgr
    }
```

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

```go
// Check all managed packages for updates
updates, err := pkg.CheckDotfilesUpdates()

// Check specific package
outdated, err := mgr.CheckOutdated()
```
