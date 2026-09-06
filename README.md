# dotfiles

[![CI](https://github.com/tekierz/dotfiles/actions/workflows/ci.yml/badge.svg)](https://github.com/tekierz/dotfiles/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tekierz/dotfiles?display_name=tag&sort=semver)](https://github.com/tekierz/dotfiles/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A cross-platform terminal environment management platform with **16 customizable themes**.

Sets up a consistent, beautiful terminal experience across macOS, Linux (Arch/Debian), and Raspberry Pi. Features an interactive TUI for installation and configuration, or use CLI commands directly.

> [!IMPORTANT]
> This application installs packages and changes user configuration files.
> Review the generated plan before applying it, run the application as your
> normal user rather than through `sudo`, and keep an independent machine
> backup.

## Quick Start

### Homebrew (Recommended)

```bash
brew tap tekierz/tap
brew install dotfiles
dotfiles
```

### Manual Installation

```bash
git clone https://github.com/tekierz/dotfiles.git
cd dotfiles
make build
./bin/dotfiles
```

`make build` creates `./bin/dotfiles` locally; the compiled binary is a build artifact and is not shipped in the repository.

## Commands

| Command | Description |
|---------|-------------|
| `dotfiles` | Launch main menu TUI |
| `dotfiles install` | Run installation wizard |
| `dotfiles manage` | Configure installed tools |
| `dotfiles hotkeys` | View keybindings cheatsheet |
| `dotfiles update` | Check for package updates |
| `dotfiles status [--json]` | Show current configuration, or versioned installation health JSON |
| `dotfiles plan --json --tool <id>...` | Print a deterministic, read-only installation plan for explicit tools |
| `dotfiles apply --yes --plan-hash <hash> --tool <id>...` | Freshly replan and apply the exact reviewed install-only authority |
| `dotfiles support --json` | Print one bounded, redacted support document to stdout for review |
| `dotfiles doctor [--json]` | Diagnose which build is running, PATH collisions, Homebrew ownership, and stale legacy binaries |
| `dotfiles doctor repair [--json]` | Preview or quarantine one ownership-proven stale `~/.local/bin/dotfiles` entry |
| `dotfiles theme list` | List available themes |
| `dotfiles theme set <name>` | Set theme (run `dotfiles install` to apply) |
| `dotfiles config <tool>` | Configure a specific tool |
| `dotfiles user <name>` | Switch to / manage a user profile |
| `dotfiles users` | List all user profiles |
| `dotfiles backups` | List configuration backups |
| `dotfiles restore [name]` | Restore a backup (opens TUI picker if no name) |
| `dotfiles version` / `dotfiles --version` | Show version information |
| `dotfiles uninstall` | Restore backups and show conservative manual uninstall guidance; automatic deletion is disabled |

`dotfiles plan --json` never infers dashboard defaults. Repeat `--tool` for each
requested registry ID. Ready and no-change plans exit 0; blocked or missing
intent exits 2 with one JSON object. To apply a ready plan, repeat the exact
tool set and pass its `plan_hash` with explicit `--yes`. Apply collects a fresh
snapshot and proceeds only if the complete private authority hash still
matches; it never reads plan JSON or infers defaults. This v1 path installs or
repairs reviewed packages only and does not write application configuration.

### Review support output before sharing

`dotfiles support --json` prints one redacted JSON document to stdout. It does
not create an archive or file, and it does not upload, transmit, or attach the
output. The document contains bounded build facts, the public installation
health document, reduced executable-provenance findings, and up to 20 recent
operation summaries. It omits usernames, hostnames, paths, environment values,
credentials, config contents, logs, raw errors, operation IDs, plan hashes, and
exact activity timestamps.

Inspect the document before deciding whether to save or share it:

```bash
dotfiles support --json | less
```

A complete document exits 0. A partial document still writes valid JSON and
exits 2; invalid syntax also exits 2 but writes no JSON. Projection or output
failure exits 1 with only a generic error. `doctor --json`, raw operation
journals, configuration files, and logs are diagnostic/private inputs and are
not designed as share-safe artifacts.

### Diagnosing stale local builds

If `dotfiles` behaves differently across terminals or appears to be missing newer features, run:

```bash
dotfiles doctor
dotfiles doctor --json
```

Doctor reports the executable currently running, every `dotfiles` match reachable through `PATH`, static version/build hints, Homebrew's managed executable, and stale `dotfiles-tui` or `dotfiles-setup` candidates. It is read-only and does not execute discovered `dotfiles` binaries or modify files. JSON output uses a stable schema and omits timestamps so repeated runs against unchanged state are deterministic, but raw Doctor JSON is not the reviewed support-sharing format.

If Doctor finds the exact historical `~/.local/bin/dotfiles` shadow, run
`dotfiles doctor repair --json` to preview a deterministic repair plan. Repair
is offered only when embedded Go metadata proves project ownership and the
candidate differs from the running and Homebrew-managed executables. Interactive
repair requires typing `quarantine`; automation requires both `--yes` and the
fresh `--plan-hash`. The original bytes and permission mode are retained in a
private quarantine with a recovery manifest. Legacy binary names remain
diagnostic-only.

## What It Installs & Configures

| Tool | Description |
|------|-------------|
| **zsh** | Shell with configurable navigation, syntax highlighting, autosuggestions |
| **tmux** | Terminal multiplexer with powerline status bar |
| **Ghostty** | Modern terminal emulator |
| **eza** | Modern `ls` replacement with icons |
| **yazi** | Terminal file manager |
| **zoxide** | Smarter `cd` command |
| **fzf** | Fuzzy finder |
| **bat** | `cat` with syntax highlighting |
| **delta** | Beautiful git diffs |
| **btop** | System monitor |
| **fastfetch** | System info display |
| **neovim** | Editor (Kickstart.nvim) |
| **sshh** | Quick SSH connection manager |
| **macmon** | macOS system monitor (macOS only) |

### Disk & Network Analysis Tools

| Tool | Description |
|------|-------------|
| **ncdu** | Interactive disk usage analyzer |
| **duf** | Modern `df` replacement with colors |
| **dust** | Intuitive `du` with visual bars |
| **bandwhich** | Real-time bandwidth by process |
| **gping** | Ping with a live graph |
| **doggo** | Modern DNS client (better `dig`) |
| **trippy** | Visual traceroute + ping |

### macOS Quality-of-Life Apps (optional, macOS only)

| App | Description |
|-----|-------------|
| **Rectangle** | Window snapping & management |
| **Raycast** | Spotlight replacement with superpowers |
| **IINA** | Modern video player |
| **AppCleaner** | Clean app uninstallation |

### Raspberry Pi Support

The Go application detects Debian-family Raspberry Pi systems through the same
platform layer used for Debian and Ubuntu. Systems with less than 1 GiB of memory
automatically omit tools marked as heavy. Package availability still varies by
architecture, so review the immutable install plan before applying it.

## Features

### Interactive TUI

The TUI provides a visual interface for all operations:

- **Installation wizard** with deep-dive configuration for each tool
- **Dual-pane management** for configuring installed tools
- **Hotkey reference** with searchable keybindings
- **Package updates** with streaming logs
- **Theme switching** with live preview
- **Mouse and keyboard** navigation

### Themes

All tools share a unified color scheme. Choose from 16 themes:

| Theme | Description |
|-------|-------------|
| `neon-seapunk` | Cyberpunk neon vibes |
| `catppuccin-mocha` | Warm dark theme (default) |
| `catppuccin-macchiato` | Medium-dark variant |
| `catppuccin-frappe` | Muted dark variant |
| `catppuccin-latte` | Light theme |
| `dracula` | Popular purple-tinted dark theme |
| `gruvbox-dark` | Retro warm dark theme |
| `gruvbox-light` | Retro warm light theme |
| `nord` | Arctic, bluish dark theme |
| `tokyo-night` | Tokyo cityscape inspired |
| `solarized-dark` | Precision dark theme |
| `solarized-light` | Precision light theme |
| `monokai` | Sublime Text classic |
| `rose-pine` | Soft, muted dark theme |
| `one-dark` | Atom's dark theme |
| `everforest` | Green nature inspired |

**Switch themes anytime:**

```bash
dotfiles theme set dracula  # Set theme to Dracula
dotfiles theme set nord     # Set theme to Nord
dotfiles theme list         # Show all themes
dotfiles status             # Show current settings
dotfiles status --json      # Print deterministic, redacted installation health JSON v1
```

> Setting a theme saves it to your config; run `dotfiles install` to apply it across all tools.

Themes apply consistently across:
- Terminal (Ghostty)
- Tmux status bar
- fzf fuzzy finder
- Yazi file manager
- Git diffs (delta)

### Navigation Styles

Choose between two navigation styles:

#### Emacs/Mac Style (default, beginner-friendly)

| Tool | Navigation |
|------|------------|
| Zsh | `Ctrl-a/e` start/end, `Alt-b/f` word nav, `Ctrl-x Ctrl-e` edit in nvim |
| Tmux | Arrow keys for pane navigation, `Alt-Arrow` without prefix |
| Yazi | Yazi defaults plus prepended `Ctrl-p/n` movement, `Ctrl-b/f` leave/enter, and `Space` selection |
| Nvim | Arrow keys work alongside standard vim keys |

#### Vim Style

| Tool | Navigation |
|------|------------|
| Zsh | `Esc` for normal mode, `hjkl` navigation, `Ctrl-e` edit in nvim |
| Tmux | `hjkl` pane navigation, `Alt-hjkl` without prefix |
| Yazi | Yazi's built-in default bindings; dotfiles writes only its compatibility/style header |
| Nvim | Full vim keybindings |

### Multi-User Support

Multiple people can share the same machine with their own theme and navigation preferences:

```bash
dotfiles --Pratik              # Quick switch to Pratik's settings
dotfiles --NickMC              # Quick switch to NickMC's settings
dotfiles user TimPike          # Switch to TimPike (creates profile if new)
dotfiles users                 # List all user profiles
```

### Backup & Restore

Accepted installation changes require a plan-scoped rollback backup before
mutation. Convenience backups can also be listed and restored directly:

```bash
dotfiles backups              # List available backups
dotfiles restore              # Open backup picker (TUI)
dotfiles restore 20240102_143052  # Restore specific backup
```

User-created convenience backups are stored in
`${XDG_CONFIG_HOME:-~/.config}/dotfiles/backups/` with timestamps. Mandatory plan rollback points
live in the private operation-state backup area and are validated internally;
they are not presented as ordinary user-managed backup sessions.

Run `dotfiles` directly as the target user, never through `sudo`. Backup capture
refuses symlinks, foreign-owned files/directories, and group/world-writable
source or destination ancestors. Current backups preserve regular-file bytes,
directory structure, and POSIX owner/group/other `rwx` bits. They do not
preserve ACLs, extended attributes, file flags, hard-link topology, or
timestamps; keep an independent machine backup when those attributes matter.

### Custom Utilities

| Command | Description |
|---------|-------------|
| `dotfiles` | Main management interface |
| `hk` | Hotkey reference cheatsheet |
| `caff` | Toggle system sleep (like Caffeine) |
| `y` | Yazi file manager (cd on exit) |

`sshh` (Quick SSH connection manager) is a small helper maintained and
distributed as part of this repository. It is installed to
`~/.local/bin/sshh` by default; it is not fetched from, or guaranteed to be
command-compatible with, the separate `tekierz/sshh` repository or Homebrew
formula. The fail-closed uninstall command retains helpers until ownership
manifests and anchored removal are implemented.

### Shell Aliases

```bash
# File listing (eza)
ls      # eza with icons
ll      # long format with git status
la      # show hidden files
lt      # tree view

# Navigation
cd      # zoxide (smart jump)

# Disk analysis
df      # duf (colorful disk free)
du      # dust (visual disk usage)
diskuse # ncdu (interactive analyzer)

# Network analysis
ping    # gping (graphical ping)
dig     # doggo (modern DNS)
trace   # trippy (visual traceroute)
bandwidth # bandwhich (bandwidth monitor)
```

## Configuration Files

After running, configs are placed in:

| File | Purpose |
|------|---------|
| `~/.zshrc` | Zsh configuration |
| `~/.tmux.conf` or `${XDG_CONFIG_HOME:-~/.config}/tmux/tmux.conf` | Active Tmux configuration; existing native settings are preserved outside a managed block |
| `${XDG_CONFIG_HOME:-~/.config}/ghostty/config.ghostty` (and legacy `config`) | Ghostty XDG candidates; the reviewed plan freezes the active candidate before writing its managed block |
| `~/Library/Application Support/com.mitchellh.ghostty/config.ghostty` (and legacy `config`) | Higher-precedence macOS candidates; the last existing candidate in Ghostty's source order becomes the frozen target |
| `${YAZI_CONFIG_HOME}` when set and absolute; else `${XDG_CONFIG_HOME}/yazi` when XDG is set and absolute; else `~/.config/yazi/` | Exact Yazi directory frozen by the reviewed plan; set relative overrides fail closed |
| `~/.gitconfig` | Native Git configuration, preserved with one bounded managed include |
| `~/.config/dotfiles/git/config` | Product-owned Git/delta settings loaded by that include |
| `${XDG_CONFIG_HOME:-~/.config}/dotfiles/global.json` | Versioned global theme, navigation, active-user, animation, and backup preferences |
| `${XDG_CONFIG_HOME:-~/.config}/dotfiles/tools/manage.json` | Versioned dashboard settings state used by Manage |
| `${XDG_CONFIG_HOME:-~/.config}/dotfiles/users/` | User profile settings |
| `~/.sshh` | SSH hosts for sshh |

Git and Ghostty preserve native content outside their bounded managed sections.
Recovery is provided by the reviewed operation backup; writers do not create
ambiguous sibling `*.dotfiles.bak` files.

## Migrating from `dotfiles-setup`

The former Bash installer has been retired and removed from active distribution.
It is unsupported, is not installed by `make install`, and must not be executed
from historical raw URLs. Existing users should install the Go application, run
`dotfiles doctor`, and follow the conservative migration guidance in
[docs/legacy-migration.md](docs/legacy-migration.md).

## Post-Install

1. Restart your terminal or `source ~/.zshrc`
2. Run `tmux` to start tmux
3. Run `nvim` to install plugins
4. Run `p10k configure` to customize prompt
5. Run `dotfiles hotkeys` to see all hotkeys
6. Run `dotfiles status` to see current theme/navigation
7. Run `sshh edit` to add SSH hosts

## Requirements

- **macOS**: Homebrew
- **Arch Linux**: pacman, paru (for AUR)
- **Debian/Ubuntu**: apt (some tools need Homebrew)
- **Build from source**: Go version declared in [`go.mod`](go.mod)

Windows is not supported. Package and architecture availability is determined
by upstream package managers and can differ between distributions and releases.

## Privacy and Network Access

The `dotfiles` application has no telemetry service, user account, or automatic
support upload. Its configuration, operation journals, backups, and diagnostic
data remain on the local machine unless you explicitly copy or share them.

Install and update operations invoke third-party package managers and tools such
as Homebrew, apt, pacman, Git, and npm. Those subprocesses may contact their own
upstream services and are governed by their respective privacy and security
policies. Review an installation plan before approving it.

Only `dotfiles support --json` is designed as a bounded, redacted
support-sharing projection. Always inspect even that output before sharing it.
Raw logs, configuration, operation journals, backups, and `doctor --json` can
contain private diagnostic information.

## Security and Support

For security vulnerabilities, follow the private reporting process in
[SECURITY.md](SECURITY.md). Please do not disclose suspected vulnerabilities in
a public issue.

For reproducible defects and feature requests, use
[GitHub Issues](https://github.com/tekierz/dotfiles/issues). Community support is
provided on a best-effort basis; this project does not include a service-level
agreement or commercial support commitment.

## Limitations

- The application targets macOS, Arch-family Linux, Debian-family Linux, and
  Raspberry Pi systems; other operating systems and distributions are
  unsupported.
- Package availability and third-party behavior are outside this project's
  control.
- Rollback preserves the documented regular-file and POSIX-mode subset, not
  every filesystem attribute. See [Backup & Restore](#backup--restore).
- The application does not manage or sanitize credentials belonging to package
  managers, Git hosts, npm, MCP servers, or installed tools.
- Theme and product names belong to their respective owners. Their inclusion
  identifies compatibility or inspiration and does not imply endorsement.

## Contributing

Bug reports, focused feature proposals, documentation improvements, and code
contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) and follow
the [Code of Conduct](CODE_OF_CONDUCT.md) before opening a pull request.

## License

Copyright (c) 2025-2026 Pratik (tekierz). Released under the
[MIT License](LICENSE).

Compiled releases include third-party open-source components under their own
licenses. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
