# Internal Packages

This directory contains the core Go packages for the dotfiles TUI application.

## Package Overview

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `backup/` | Shared backup/restore (traversal-safe restore, manifest parsing) | `backup.go` |
| `config/` | Configuration loading/saving (all saves use `writeFileAtomic`) | `config.go`, `user.go`, `claude.go` (Claude Code MCP config in `~/.claude.json`, v2.1), `defaults.go`, `tool.go`, `hotkeys.go` |
| `hotkeys/` | Hotkey definitions for tools | `hotkeys.go` |
| `pkg/` | Package manager abstraction | `manager.go`, `brew.go`, `pacman.go`, `apt.go`, `update.go`, `mock_manager.go` |
| `runner/` | Bash script execution | `bash.go` |
| `scripts/` | Embedded utility scripts | `scripts.go` (hk, caff, sshh) |
| `testutil/` | Common test helpers/utilities | `testutil.go` |
| `tools/` | Tool registry and definitions | `registry.go`, `tool.go`, `simple_tools.go` (declarative table of pure-metadata tools), `apps.go` |
| `ui/` | Bubble Tea TUI application (~14,600 lines, excluding tests; every screen is a `ScreenHandler` in a `screen_*.go` file dispatched by the `ScreenManager`) | `app.go`, `screen_manager.go`, `screen_factory.go`, `manage_dualpane.go`, `styles.go` |

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
