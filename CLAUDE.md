# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **dotfiles**: a cross-platform terminal environment management platform that creates a consistent terminal experience across macOS, Linux (Arch/Debian), and Raspberry Pi. It includes:

- **Go TUI Application** (`cmd/dotfiles/`) - Interactive installer and management platform using Bubble Tea
- **Legacy Bash Script** (`bin/dotfiles-setup`) - Original setup script (~3,500 lines of bash)

The Go application provides installation, configuration, and updates for zsh, tmux, Ghostty, neovim, yazi, and 25+ other terminal tools with unified theming. v2.1 added Tailscale (VPN), Sunshine/Moonlight (game streaming), and Claude Code (MCP configuration).

## Repository Structure

```
cmd/
  dotfiles/              # Go CLI entry point (Cobra + Bubble Tea)
  installer/             # Standalone installer entry point (main.go)
internal/
  config/                # Configuration loading/saving (JSON)
  hotkeys/               # Hotkey definitions for tools
  pkg/                   # Package manager abstraction (brew/pacman/apt)
  runner/                # Bash script execution
  scripts/               # Embedded utility scripts (hk, caff, sshh)
  testutil/              # Shared test helpers (testutil.go)
  tools/                 # Tool registry (30 tools)
  ui/                    # Bubble Tea TUI (~14,600 lines)
bin/
  dotfiles               # Built Go binary
  dotfiles-setup         # Legacy bash script
  dotfiles-setup.ps1     # Windows PowerShell setup script
docs/
  tools.md               # Detailed tool reference
  beta.plan              # Planned improvements for next release
  v2-analysis-plan.md    # v2 analysis / planning notes
  security-scanning.md   # Security scanning reference
```

## Homebrew Distribution

The project is distributed via a single Homebrew formula:

```bash
brew tap tekierz/tap
brew install dotfiles
```

The formula is maintained in the separate [homebrew-tap](https://github.com/tekierz/homebrew-tap) repository.

## Go Application Architecture

### Key Packages

| Package | Purpose |
|---------|---------|
| `internal/ui/` | Bubble Tea TUI (Model-Update-View pattern) |
| `internal/tools/` | Tool registry with 30 tools |
| `internal/pkg/` | Package manager abstraction |
| `internal/config/` | JSON configuration management |
| `internal/hotkeys/` | Hotkey definitions |
| `internal/scripts/` | Embedded shell scripts |

### Screen Navigation

The TUI uses screen-based navigation with 33 screens (the `Screen` enum in
`internal/ui/app.go`):
- Wizard: Intro, ThemeSelect, NavStyle, DeepDive, Summary
- Management: MainMenu, Manage, Update, Hotkeys, Backups
- Config: Per-tool configuration screens

Each screen is a `ScreenHandler` implemented in package `ui` in a `screen_*.go`
file (e.g. `screen_welcome.go`, `screen_manage.go`, `screen_config_*.go`).
`App.Update` delegates to the `ScreenManager` (`uiTickMsg` and
`installCacheDoneMsg` are handled globally first); `App.View` delegates to
`ScreenManager.View()`. `NewApp` always wires the `ScreenManager` via
`initScreenManager()`.

### Async Patterns

The TUI uses Bubble Tea's message-based async pattern for long-running operations:

**Install Cache Loading** (`internal/ui/cache.go`):
- `loadInstallCacheCmd()` - Async command to check all tool installation status
- Uses batch package manager queries (`brew list --versions`) for performance
- Shows loading spinner while cache populates
- State: `installCacheLoading`, `manageInstalledReady`, `manageInstalled`

**Update Checking**:
- `checkUpdatesCmd()` - Async command to check for outdated packages
- State: `updateChecking`, `updateCheckDone`, `updateResults`

### Styling

Neon-seapunk color palette defined in `internal/ui/styles.go`:
- Primary: `#00F5D4` (cyan), `#F15BB5` (magenta)
- Background: `#070B1A` (deep ocean)

## CLI Commands

```bash
dotfiles                  # Launch TUI main menu
dotfiles install          # Launch TUI installer
dotfiles manage           # Launch TUI management
dotfiles hotkeys          # Launch TUI hotkey viewer
dotfiles config <tool>    # Configure a specific tool
dotfiles status           # Print status (CLI)
dotfiles backups          # List backups (CLI)
dotfiles restore <name>   # Restore backup (CLI)
dotfiles theme list       # List themes (CLI)
dotfiles theme set <name> # Set theme directly (CLI)
dotfiles user [name]      # Manage users (add/delete subcommands)
dotfiles users            # List users (CLI)
dotfiles update           # Check for updates
dotfiles version          # Print version
dotfiles uninstall        # Remove dotfiles and restore config
```

## Key Concepts

- **16 themes** with unified colors across all tools
- **Two navigation styles**: emacs (default) and vim
- **Platform detection**: macOS (Homebrew), Arch (pacman/paru), Debian (apt)
- **Tool registry**: Interface-based tool definitions with platform-specific packages
- **Backup & restore**: Timestamped backups in `~/.config/dotfiles/backups/`
- **Legacy cleanup**: `cleanupOldInstallations()` removes old dotfiles-tui/dotfiles-setup binaries

## Development

### Building

```bash
make build          # Build Go binary to bin/dotfiles
make clean          # Clean build artifacts
go build ./...      # Compile check
go vet ./...        # Static analysis
```

### Adding New Tools

1. Create `internal/tools/newtool.go` implementing `Tool` interface
2. Register in `internal/tools/registry.go`
3. Add deep dive config screen if needed

### Adding New Screens

1. Add Screen constant in `internal/ui/app.go`
2. Add case in `View()` method
3. Add key handling in `Update()` method
4. Create render function

### Testing

```bash
go test ./...           # Run all tests
go test ./internal/ui/  # Run UI tests only
go test -v ./...        # Verbose output
```

Test infrastructure includes:
- Mock package manager (`internal/pkg/mock_manager.go`)
- Mock config provider (`internal/ui/deps_test.go`)
- Screen handler tests (`internal/ui/screen_test.go`)

### Pre-PR Checklist

See `.claude/skills/pre-pr-tests/SKILL.md` for comprehensive testing checklist including:
- Automated tests (build, vet, fmt, security scan)
- Manual TUI testing
- Cross-repository compatibility checks

### Performance Considerations

- Use async loading for operations that involve subprocess calls
- Batch package manager queries where possible (e.g., `brew list` once vs per-package)
- Cache results in App struct with `*Ready` flags
- Show loading spinners during async operations

## Security Considerations

- **Never use `source` on user profiles** - use `safe_read_setting()` in bash
- **Validate all user input** in config parsing
- **File permissions**: config directories get 700, settings files get 600

## Git Commit Rules

- Never add "Generated with Claude Code" tags to commits
- Never add "Co-Authored-By: Claude" lines to commits
- Keep commit messages clean and concise
- When asked to "push", only commit - user handles `git push` (SSH auth required)
