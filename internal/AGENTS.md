# Internal Packages

This directory contains the core Go packages for the dotfiles TUI application.

## Package Overview

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `config/` | Configuration loading/saving | `config.go`, `user.go` |
| `hotkeys/` | Hotkey definitions for tools | `hotkeys.go` |
| `pkg/` | Package manager abstraction | `manager.go`, `brew.go`, `pacman.go`, `apt.go` |
| `runner/` | Bash script execution | `bash.go` |
| `scripts/` | Embedded utility scripts | `scripts.go` (hk, caff, sshh) |
| `tools/` | Tool registry and definitions | `registry.go`, `tool.go`, `apps.go` |
| `ui/` | Bubble Tea TUI application (~12,600 lines) | `app.go`, `screens.go`, `styles.go` |

## Architecture

```
cmd/dotfiles/main.go (CLI entry point)
         │
         ▼
    internal/ui/app.go (TUI application)
         │
    ┌────┴────┬────────────┐
    ▼         ▼            ▼
internal/ internal/   internal/
config/   tools/      pkg/
```

## Adding New Features

1. **New tool**: Add to `tools/` package, register in `registry.go`
2. **New screen**: Add Screen constant in `ui/app.go`, implement render function
3. **New config option**: Add to `config/config.go`, update load/save functions
4. **New package manager**: Implement `PackageManager` interface in `pkg/`
