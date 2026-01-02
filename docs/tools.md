# Tools Reference

This document lists all tools installed by `dotfiles-setup`, organized by platform. Tools marked with **[Default]** are installed automatically; those marked with **[Optional]** require user confirmation or flags.

---

## Core Tools (All Platforms)

These tools form the foundation of the terminal environment and are installed on all supported platforms.

### Shell & Prompt

| Tool | Description |
|------|-------------|
| **zsh** | Modern shell with better scripting, completion, and plugin support. Replaces bash as the default interactive shell. |
| **zsh-syntax-highlighting** | Highlights commands as you type—green for valid commands, red for errors. Catches typos before you hit Enter. |
| **zsh-autosuggestions** | Shows ghost text predictions based on your command history. Press `→` or `Ctrl-f` to accept. |
| **powerlevel10k** | Fast, highly customizable prompt theme with git status, execution time, and context indicators. |

### File Navigation & Management

| Tool | Description |
|------|-------------|
| **eza** | Modern replacement for `ls` with colors, icons, git status integration, and tree view. Aliased to `ls`, `ll`, `la`, `lt`. |
| **yazi** | Terminal file manager with vim-like navigation, image previews, and bulk operations. Use `y` or `yazi` to launch; `q` exits to current directory. |
| **zoxide** | Smarter `cd` that learns your habits. Type `cd proj` to jump to `/home/user/projects` if you've been there before. |
| **fzf** | Fuzzy finder for files, history, and more. `Ctrl-r` for history search, `Ctrl-t` for file search, `Alt-c` for directory jump. |

### Text & Code

| Tool | Description |
|------|-------------|
| **neovim** | Modern vim with better defaults, Lua configuration, and async plugin support. Configured with Kickstart.nvim for a sensible starting point. |
| **bat** | `cat` replacement with syntax highlighting, line numbers, and git integration. Themed to match your selected color scheme. |
| **ripgrep** | Blazingly fast `grep` replacement that respects `.gitignore`. Used by fzf and many editor plugins. |
| **fd** | Fast, user-friendly `find` replacement. Simpler syntax: `fd pattern` instead of `find . -name '*pattern*'`. |
| **git-delta** | Beautiful git diffs with syntax highlighting, line numbers, and side-by-side view. Automatically configured in `.gitconfig`. |

### Terminal & Multiplexing

| Tool | Description |
|------|-------------|
| **tmux** | Terminal multiplexer for persistent sessions, split panes, and window management. Essential for remote work—sessions survive disconnects. |
| **Ghostty** | Modern, GPU-accelerated terminal emulator with excellent font rendering. Configured with your selected theme and transparent background. |

### System & Utilities

| Tool | Description |
|------|-------------|
| **btop** | Beautiful system monitor showing CPU, memory, disks, network, and processes. Much nicer than `top` or `htop`. |
| **fastfetch** | Fast system information display showing OS, kernel, packages, memory, and more. Runs on shell startup. |
| **tlrc** | Rust client for tldr pages—simplified, practical man pages with examples. `tldr tar` is much friendlier than `man tar`. |

### Clipboard (Platform-Specific)

| Tool | Platform | Description |
|------|----------|-------------|
| **pbcopy/pbpaste** | macOS | Built-in clipboard commands. No installation needed. |
| **wl-clipboard** | Linux (Wayland) | Provides `wl-copy` and `wl-paste` for Wayland sessions (GNOME, KDE Plasma on Wayland). |
| **xclip** | Linux (X11) | Clipboard access for X11 sessions. Fallback for older systems or X11-based desktops. |

> **Note:** tmux is configured to auto-detect which clipboard tool is available and use it for mouse selection copy and middle-click paste.

### Custom Utilities

These small scripts are installed to `~/.local/bin/`:

| Tool | Description |
|------|-------------|
| **hk** | Hotkey reference cheatsheet. Displays keybindings for tmux, zsh, yazi, fzf, and other tools in a nicely formatted table. |
| **caff** | Caffeine toggle to prevent system sleep. `caff on` keeps your machine awake; `caff off` restores normal behavior. Works on both macOS and Linux. |
| **sshh** | Quick SSH connection manager. Store frequently-used hosts in `~/.sshh` and connect with `sshh 1` or via interactive menu. |
| **dotfiles** | Theme and user management CLI. Switch themes with `dotfiles theme dracula`, manage user profiles with `dotfiles --Username`. |

### Fonts

| Font | Description |
|------|-------------|
| **JetBrains Mono Nerd Font** | Primary monospace font with programming ligatures and complete icon coverage for file managers and prompts. |
| **Iosevka Nerd Font** | Alternative narrow font option, useful for fitting more content on screen. |

---

## macOS

### Default Tools

All [Core Tools](#core-tools-all-platforms) plus:

| Tool | Description |
|------|-------------|
| **Homebrew** | Package manager for macOS. Installed automatically if not present. Used to install all other tools. |
| **macmon** | macOS system monitor showing CPU, GPU, memory, and thermals. Like btop but with Apple Silicon-specific metrics. |

### Optional: Quality-of-Life Apps

Install with `--macos-apps` flag or answer "yes" when prompted.

#### Always Installed (when opting in)

| Tool | Type | Description |
|------|------|-------------|
| **Rectangle** | [Optional] | Window snapping and management. `Ctrl-Opt-←/→` for half-screen, `Ctrl-Opt-↑` for maximize. Free alternative to Magnet. |
| **Raycast** | [Optional] | Spotlight replacement with clipboard history, snippets, window management, and extensible commands. |
| **Stats** | [Optional] | Menu bar system monitor showing CPU, memory, disk, network, and battery with customizable widgets. |

#### Individually Prompted

These are prompted one-by-one unless using `--macos-apps -y`:

| Tool | Type | Description |
|------|------|-------------|
| **AltTab** | [Optional] | Windows-style `Alt-Tab` with window previews. Shows all windows, not just apps. |
| **MonitorControl** | [Optional] | Control external monitor brightness and volume using keyboard keys or menu bar. |
| **Mos** | [Optional] | Smooth scrolling for external mice. Makes scroll wheels feel like trackpad. |
| **Karabiner-Elements** | [Optional] | Powerful keyboard customization. Remap keys, create complex modifications, fix non-Mac keyboards. |
| **IINA** | [Optional] | Modern video player built on mpv. Clean interface, good codec support, Picture-in-Picture. |
| **The Unarchiver** | [Optional] | Extract any archive format. Handles zip, rar, 7z, tar, and dozens more. |
| **AppCleaner** | [Optional] | Thorough app uninstaller that removes preferences, caches, and support files. |
| **mas** | [Optional] | Mac App Store CLI. Install and update App Store apps from terminal. |
| **trash** | [Optional] | Move files to Trash from command line instead of permanent deletion. |
| **switchaudio-osx** | [Optional] | Switch audio input/output devices from command line. |

---

## Arch Linux

### Default Tools

All [Core Tools](#core-tools-all-platforms) plus:

| Tool | Source | Description |
|------|--------|-------------|
| **pacman-contrib** | [Default] | Pacman utilities including `paccache` for cleaning old packages. Auto-enabled timer keeps cache from growing indefinitely. |
| **Ghostty** | [Default] | Installed from AUR via `paru` if available. |

### Requirements

- **pacman**: System package manager (included with Arch)
- **paru**: AUR helper for installing community packages (recommended but optional)

---

## Debian / Ubuntu

### Default Tools

| Tool | Status | Description |
|------|--------|-------------|
| **zsh** | [Default] | Shell |
| **tmux** | [Default] | Terminal multiplexer |
| **neovim** | [Default] | Editor |
| **fzf** | [Default] | Fuzzy finder |
| **bat** | [Default] | Syntax-highlighted cat (may be `batcat` on older versions) |
| **ripgrep** | [Default] | Fast grep |
| **fd-find** | [Default] | Fast find (binary may be `fdfind`) |
| **btop** | [Default] | System monitor |

### Limitations

Some tools require manual installation or Homebrew on Linux:

| Tool | Notes |
|------|-------|
| **eza** | Not in default repos; may need Homebrew or cargo |
| **yazi** | Requires cargo or manual binary install |
| **zoxide** | Installed via curl script |
| **git-delta** | May need Homebrew |
| **Ghostty** | Manual installation required |

---

## Raspberry Pi

Optimized configurations for different Pi models with resource-appropriate tool selection.

### All Models (Default)

| Tool | Description |
|------|-------------|
| **zsh** | Shell |
| **tmux** | Terminal multiplexer |
| **neovim** | Editor |
| **fzf** | Fuzzy finder |
| **bat** | Syntax-highlighted cat |
| **ripgrep** | Fast grep |
| **fd-find** | Fast find |
| **git, curl, wget** | Basic utilities |
| **htop** | Lightweight system monitor |
| **zoxide** | Smart cd (installed via script) |

### Pi 4 / Pi 5 (Additional)

| Tool | Description |
|------|-------------|
| **btop** | Full system monitor (needs more RAM than htop) |
| **eza** | Modern ls replacement |
| **yazi** | Terminal file manager (requires cargo build) |

### Pi Zero 2 W (Lightweight Mode)

Uses `--raspizero2` flag. Skips resource-heavy tools:

| Skipped | Reason |
|---------|--------|
| **yazi** | Too memory-intensive for 512MB RAM |
| **btop** | Uses htop instead |
| **eza** | Uses standard ls |

---

## Installation Flags Reference

| Flag | Description |
|------|-------------|
| `--macos-apps` | Install all optional macOS quality-of-life apps without prompting |
| `--raspi` | Raspberry Pi mode (auto-detects model) |
| `--raspi5` | Force Raspberry Pi 5 optimizations |
| `--raspizero2` | Lightweight mode for Pi Zero 2 W |
| `--theme <name>` | Set initial theme (default: catppuccin-mocha) |
| `--emacs` | Use emacs/Mac-style navigation (default) |
| `--vim` | Use vim-style navigation |
| `-y, --yes` | Skip all confirmation prompts |

---

## Post-Install Configuration

After installation, these files contain your tool configurations:

| File | Purpose |
|------|---------|
| `~/.zshrc` | Zsh configuration with aliases, functions, and plugin loading |
| `~/.tmux.conf` | Tmux configuration with theme and keybindings |
| `~/.config/ghostty/config` | Terminal emulator settings |
| `~/.config/yazi/` | File manager configuration |
| `~/.config/bat/config` | Bat theme settings |
| `~/.gitconfig` | Git configuration with delta |
| `~/.config/dotfiles/settings` | Current theme, navigation style, and active user |
| `~/.sshh` | SSH hosts for quick connect |
