# dotfiles-setup

A cross-platform terminal environment setup script with **13 customizable themes**.

Sets up a consistent, beautiful terminal experience across macOS, Linux (Arch/Debian), and Windows (via WSL). Switch themes anytime with the `dotfiles` command.

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
| **Stats** | System monitor in menu bar |
| **AltTab** | Windows-style alt-tab switcher |
| **MonitorControl** | Control external monitor brightness |
| **Mos** | Smooth scrolling for external mouse |
| **Karabiner-Elements** | Keyboard customization |
| **IINA** | Modern video player |
| **The Unarchiver** | Archive extraction |
| **AppCleaner** | Clean app uninstallation |
| **mas** | Mac App Store CLI |
| **trash** | Move files to trash from CLI |

### Raspberry Pi Support

Optimized configurations for different Pi models:

| Model | Flag | Notes |
|-------|------|-------|
| **Pi 5** | `--raspi5` | Full toolset, all features |
| **Pi 4** | `--raspi` | Full toolset |
| **Pi Zero 2** | `--raspizero2` | Lightweight (skips yazi, btop) |

Raspberry Pi installs via apt + manual builds for modern tools not in repos.

## Installation

### Quick Install (Recommended)

```bash
# macOS/Linux
curl -fsSL https://raw.githubusercontent.com/tekierz/dotfiles/main/bin/dotfiles-setup | bash

# With all macOS apps
curl -fsSL https://raw.githubusercontent.com/tekierz/dotfiles/main/bin/dotfiles-setup | bash -s -- --macos-apps

# Raspberry Pi
curl -fsSL https://raw.githubusercontent.com/tekierz/dotfiles/main/bin/dotfiles-setup | bash -s -- --raspi

# Raspberry Pi Zero 2 (lightweight)
curl -fsSL https://raw.githubusercontent.com/tekierz/dotfiles/main/bin/dotfiles-setup | bash -s -- --raspizero2
```

### Homebrew

```bash
brew tap tekierz/tap
brew install dotfiles-setup
dotfiles-setup
```

### Manual

```bash
git clone https://github.com/tekierz/dotfiles.git
cd dotfiles
chmod +x bin/dotfiles-setup
./bin/dotfiles-setup
```

### Command Line Options

| Option | Description |
|--------|-------------|
| `--theme <name>` | Set color theme (default: catppuccin-mocha) |
| `--emacs` | Emacs/Mac-style navigation (default, beginner-friendly) |
| `--vim` | Vim-style navigation (hjkl everywhere) |
| `--macos-apps` | Install all optional macOS quality-of-life apps |
| `--raspi` | Raspberry Pi mode (auto-detect model) |
| `--raspi5` | Raspberry Pi 5 optimizations |
| `--raspizero2` | Raspberry Pi Zero 2 (lightweight mode) |
| `--restore [name]` | Restore configs from backup |
| `--list-backups` | List available backup sessions |
| `--no-backup` | Skip creating backups (not recommended) |
| `-y, --yes` | Skip confirmation prompts |
| `-h, --help` | Show help |

## Features

### Themes

All tools share a unified color scheme. Choose from 13 themes during install or switch anytime:

| Theme | Description |
|-------|-------------|
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

**Switch themes anytime:**

```bash
dotfiles theme dracula      # Switch to Dracula
dotfiles theme nord         # Switch to Nord
dotfiles list-themes        # Show all themes
dotfiles status             # Show current settings
```

Themes apply consistently across:
- Terminal (Ghostty)
- Tmux status bar
- fzf fuzzy finder
- Yazi file manager
- Git diffs (delta)
- Bat syntax highlighting

### Navigation Styles

Choose between two navigation styles with `--emacs` (default) or `--vim`:

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

**Create user profiles:**

```bash
# Interactive setup (guided theme/nav selection)
dotfiles user add Caitlyn

# Or specify options directly
dotfiles user add EvanGB --theme nord --nav vim
dotfiles user add KCMarsh --theme dracula --nav emacs
```

**Manage profiles:**

```bash
dotfiles users                 # List all profiles with active marker
dotfiles user delete OldUser   # Remove a profile
```

User profiles are stored in `~/.config/dotfiles/users/` and switching applies changes immediately.

### Backup & Restore

All existing configs are backed up before modification. Fully reversible installation:

```bash
# Restore to pre-installation state
dotfiles-setup --restore

# Or use the CLI post-install
dotfiles backups              # List available backups
dotfiles restore              # Restore most recent
dotfiles restore 20240102_143052  # Restore specific backup
```

Backups are stored in `~/.config/dotfiles/backups/` with timestamps.

### Custom Utilities

| Command | Description |
|---------|-------------|
| `dotfiles` | Switch themes/navigation, manage backups |
| `hk` | Hotkey reference cheatsheet |
| `caff` | Toggle system sleep (like Caffeine) |
| `sshh` | Quick SSH connection manager |
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
5. Run `hk` to see all hotkeys
6. Run `dotfiles status` to see current theme/navigation
7. Run `sshh edit` to add SSH hosts

## Requirements

- **macOS**: Homebrew (installed automatically)
- **Arch Linux**: pacman, paru (for AUR)
- **Debian/Ubuntu**: apt (some tools need Homebrew)

## Screenshots

![Terminal](https://raw.githubusercontent.com/tekierz/dotfiles/main/screenshots/terminal.png)

## License

MIT License - see [LICENSE](LICENSE)

## Related

- [sshh](https://github.com/tekierz/sshh) - Quick SSH connection manager
