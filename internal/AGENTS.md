# Internal Packages

This directory contains the core Go packages for the dotfiles TUI application.

## Package Overview

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `config/` | Configuration loading/saving | `config.go` |
| `hotkeys/` | Hotkey definitions for tools | `hotkeys.go` |
| `pkg/` | Package manager abstraction | `manager.go`, `brew.go`, `pacman.go`, `apt.go` |
| `runner/` | Bash script execution | `runner.go` |
| `tools/` | Tool registry and definitions | `registry.go`, `tool.go` |
| `ui/` | Bubble Tea TUI application | `app.go`, `screens.go`, `styles.go` |

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
