# Internal Packages

This directory contains the core Go packages for the dotfiles TUI application.

## Package Overview

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `config/` | Configuration loading/saving | `config.go`, `user.go`, `claude.go` (Claude Code MCP config, v2.1), `defaults.go`, `tool.go`, `hotkeys.go` |
| `hotkeys/` | Hotkey definitions for tools | `hotkeys.go` |
| `pkg/` | Package manager abstraction | `manager.go`, `brew.go`, `pacman.go`, `apt.go`, `update.go`, `mock_manager.go` |
| `runner/` | Bash script execution | `bash.go` |
| `scripts/` | Embedded utility scripts | `scripts.go` (hk, caff, sshh) |
| `testutil/` | Common test helpers/utilities | `testutil.go` |
| `tools/` | Tool registry and definitions | `registry.go`, `tool.go`, `apps.go` |
| `ui/` | Bubble Tea TUI application (~14,400 lines; ~14,700 with the `ui/screens/` subpackage) | `app.go`, `screens.go`, `manage_dualpane.go`, `screens_deepdive.go`, `styles.go` |
| `ui/screens/` | Screen-rendering subpackage | `error.go`, `factory.go`, `summary.go` |

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
