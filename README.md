# dotfiles

A cross-platform terminal environment management platform with **16 customizable themes**.

Sets up a consistent, beautiful terminal experience across macOS, Linux (Arch/Debian), and Raspberry Pi. Features an interactive Go TUI for installation and configuration, or use CLI commands directly.

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

## Commands

| Command | Description |
|---------|-------------|
| `dotfiles` | Launch main menu TUI |
| `dotfiles install` | Run installation wizard |
| `dotfiles manage` | Configure installed tools |
| `dotfiles hotkeys` | View keybindings cheatsheet |
| `dotfiles update` | Check for package updates |
| `dotfiles status` | Show current configuration |
| `dotfiles theme --list` | List available themes |
| `dotfiles theme set <name>` | Set theme (run `dotfiles install` to apply) |
| `dotfiles config <tool>` | Configure a specific tool |
| `dotfiles user <name>` | Switch to / manage a user profile |
| `dotfiles users` | List all user profiles |
| `dotfiles backups` | List configuration backups |
| `dotfiles restore [name]` | Restore a backup (opens TUI picker if no name) |
| `dotfiles version` | Show version information |
| `dotfiles uninstall` | Remove dotfiles and restore original config |

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
| **ripgrep** | Fast recursive search |
| **fd** | Fast, friendly `find` replacement |
| **fswatch** | Cross-platform file change monitor |
| **btop** | System monitor |
| **neovim** | Editor (Kickstart.nvim) |
| **lazygit** | Git terminal UI |
| **lazydocker** | Docker terminal UI (macOS/Arch) |
| **glow** | Markdown viewer |
| **Claude Code** | CLI plus MCP configuration |
| **Tailscale** | Mesh VPN |
| **Sunshine** | Game streaming host |
| **Moonlight** | Game streaming client |

### macOS Quality-of-Life Apps (optional, macOS only)

| App | Description |
|-----|-------------|
| **Rectangle** | Window snapping & management |
| **Raycast** | Spotlight replacement with superpowers |
| **Zen Browser** | Privacy-focused browser |
| **Cursor** | AI code editor |
| **LM Studio** | Local LLM desktop app |
| **OBS Studio** | Video recording and streaming |
| **IINA** | Modern video player |
| **AppCleaner** | Clean app uninstallation |

### Raspberry Pi Support

The Go app detects Raspberry Pi systems and trims heavy tools on low-memory models.

| Model | Notes |
|-------|------|
| **Pi 5** | Full toolset where packages are available |
| **Pi 4** | Full toolset where packages are available |
| **Pi Zero 2** | Lightweight mode skips heavy tools |

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
dotfiles theme --list       # Show all themes
dotfiles status             # Show current settings
```

> Setting a theme saves it to your config; run `dotfiles install` to apply it across all tools.

Themes apply consistently across:
- Terminal (Ghostty)
- Tmux status bar
- fzf fuzzy finder
- Yazi file manager
- Git diffs (delta)
- Bat syntax highlighting

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

All existing configs are backed up before modification. Fully reversible installation:

```bash
dotfiles backups              # List available backups
dotfiles restore              # Open backup picker (TUI)
dotfiles restore 20240102_143052  # Restore specific backup
```

Backups are stored in `~/.config/dotfiles/backups/` with timestamps.

### Custom Utilities

| Command | Description |
|---------|-------------|
| `dotfiles` | Main management interface |
| `hk` | Hotkey reference cheatsheet |
| `caff` | Toggle system sleep (like Caffeine) |
| `sshh` | Quick SSH connection manager backed by `~/.sshh` |
| `y` | Yazi file manager (cd on exit) |

### Shell Aliases

```bash
# File listing (eza)
ls      # eza with icons
ll      # long format with git status
la      # show hidden files
lt      # tree view

# Navigation
cd      # zoxide (smart jump)
```

## Configuration Files

After running, configs are placed in:

| File | Purpose |
|------|---------|
| `~/.zshrc` | Zsh configuration |
| `~/.tmux.conf` | Tmux configuration |
| `~/.config/ghostty/config` | Ghostty terminal |
| `~/.config/yazi/` | Yazi file manager |
| `~/.config/bat/config` | Bat configuration |
| `~/.gitconfig` | Git with delta |
| `~/.config/dotfiles/settings` | Theme, navigation, and active user |
| `~/.config/dotfiles/users/` | User profile settings |
| `~/.sshh` | SSH hosts for sshh |

## Post-Install

1. Restart your terminal or `source ~/.zshrc`
2. Run `tmux` to start tmux
3. Run `nvim` to install plugins
4. Run `p10k configure` to customize prompt
5. Run `dotfiles hotkeys` to see all hotkeys
6. Run `dotfiles status` to see current theme/navigation
7. Run `sshh edit` to add SSH hosts

## Requirements

- **macOS**: Homebrew (installed automatically)
- **Arch Linux**: pacman, paru (for AUR)
- **Debian/Ubuntu**: apt (some tools need Homebrew)

## License

MIT License - see [LICENSE](LICENSE)

## Related

- [sshh](https://github.com/tekierz/sshh) - Quick SSH connection manager
