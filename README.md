# dotfiles

A cross-platform terminal environment management platform with **16 customizable themes**.

Sets up a consistent, beautiful terminal experience across macOS, Linux (Arch/Debian), and Raspberry Pi. Features an interactive TUI for installation and configuration, or use CLI commands directly.

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
| `dotfiles status` | Show current configuration |
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

### Diagnosing stale local builds

If `dotfiles` behaves differently across terminals or appears to be missing newer features, run:

```bash
dotfiles doctor
dotfiles doctor --json
```

Doctor reports the executable currently running, every `dotfiles` match reachable through `PATH`, safe static version/build hints, Homebrew's managed executable, and stale `dotfiles-tui` or `dotfiles-setup` candidates. It is read-only and does not execute discovered `dotfiles` binaries or modify files. JSON output uses a stable schema and omits timestamps so repeated runs against unchanged state are deterministic.

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
| Yazi | Arrow keys, `Ctrl-c/x/v` copy/cut/paste, `F2` rename |
| Nvim | Arrow keys work alongside standard vim keys |

#### Vim Style

| Tool | Navigation |
|------|------------|
| Zsh | `Esc` for normal mode, `hjkl` navigation, `Ctrl-e` edit in nvim |
| Tmux | `hjkl` pane navigation, `Alt-hjkl` without prefix |
| Yazi | `hjkl` navigation, `y/x/p` yank/cut/paste, `r` rename |
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
`~/.config/dotfiles/backups/` with timestamps. Mandatory plan rollback points
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

`sshh` (Quick SSH connection manager) is bundled with dotfiles and installed to `~/.local/bin/sshh` by default. The fail-closed uninstall command retains helpers until ownership manifests and anchored removal are implemented.

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
| `~/.tmux.conf` | Tmux configuration; existing native settings are preserved outside a managed block |
| `${XDG_CONFIG_HOME:-~/.config}/ghostty/config.ghostty` (and legacy `config`) | Ghostty terminal; existing settings are preserved outside a managed block |
| `~/Library/Application Support/com.mitchellh.ghostty/config.ghostty` (and legacy `config`) | Higher-precedence Ghostty sources on macOS; the latest existing source is updated |
| `~/.config/yazi/` | Yazi file manager |
| `~/.gitconfig` | Native Git configuration, preserved with one bounded managed include |
| `~/.config/dotfiles/git/config` | Product-owned Git/delta settings loaded by that include |
| `~/.config/dotfiles/settings` | Theme, navigation, and active user |
| `~/.config/dotfiles/users/` | User profile settings |
| `~/.sshh` | SSH hosts for sshh |

On first Git or Ghostty adoption, the existing native file is copied byte-for-byte
to a sibling `*.dotfiles.bak` file. An existing different backup is never overwritten.

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

## License

MIT License - see [LICENSE](LICENSE)

## Related

- [sshh](https://github.com/tekierz/sshh) - Quick SSH connection manager
