# Tools Reference

This document lists all tools installed by the legacy `dotfiles-setup` bash script (`bin/dotfiles-setup`), organized by platform. Tools marked with **[Default]** are installed automatically; those marked with **[Optional]** require user confirmation or flags.

> **Scope note:** Most sections below describe the **legacy bash script** tool set. The current **Go TUI** manages tools from its own registry (`internal/tools/`, 30 registered tools) which differs from this list. Some tools here (`fastfetch`, `tlrc`, `ncdu`, `duf`, `dust`, `bandwhich`, `gping`, `doggo`, `trippy`, `macmon`) are installed **only by the legacy bash script** and are not part of the Go TUI registry. Conversely, several tools managed by the Go TUI are documented in the new [Go TUI Tool Registry](#go-tui-tool-registry) section near the end.

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

### Disk Analysis

| Tool | Description |
|------|-------------|
| **ncdu** | Interactive disk usage analyzer with ncurses interface. Navigate directories, delete files, see what's eating space. Aliased to `diskuse`. |
| **duf** | Modern `df` replacement with colorful output showing disk usage per mount. Aliased to `df`. |
| **dust** | Intuitive `du` replacement written in Rust. Shows directory sizes with visual bars. Aliased to `du`. |

### Network Analysis

| Tool | Description |
|------|-------------|
| **bandwhich** | Real-time bandwidth utilization by process and connection. See exactly what's using your network. Run with `bandwidth` or `sudo bandwhich`. |
| **gping** | Ping with a live graph showing latency over time. Great for monitoring connection quality. Aliased to `ping`. |
| **doggo** | Modern DNS client (better `dig`). Clean output, supports DNS-over-TLS/HTTPS. Aliased to `dig`. |
| **trippy** | Network diagnostic tool combining traceroute and ping. Visual display of network path with latency per hop. Use `trace` or `trip`. |

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
| **dotfiles** | Theme and user management CLI. Switch themes with `dotfiles theme set dracula`, manage user profiles with `dotfiles --Username` (e.g. `dotfiles --Pratik`). |

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

> **Go TUI picker:** the Go TUI macOS app picker offers only Rectangle, Raycast, IINA, and AppCleaner. Everything else in these tables (including `mas`, `trash`, and `switchaudio-osx`) is installed only by the legacy bash script.

---

## Arch Linux

### Default Tools

All [Core Tools](#core-tools-all-platforms) plus:

| Tool | Source | Description |
|------|--------|-------------|
| **pacman-contrib** | [Default] | Pacman utilities including `paccache` for cleaning old packages. Auto-enabled timer keeps cache from growing indefinitely. |
| **Ghostty** | AUR | Installed from AUR via `paru` if available. |
| **ncdu, duf, bandwhich, gping** | [Default] | Disk and network tools from official repos. |
| **dust, dog, trippy** | AUR | Additional analysis tools from AUR (requires `paru`). |

### Requirements

- **pacman**: System package manager (included with Arch)
- **paru**: AUR helper for installing community packages (required for Ghostty, dust, dog, trippy)

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
| **ncdu** | [Default] | Interactive disk usage analyzer |
| **duf** | [Default] | Modern df replacement (Ubuntu 22.04+) |

### Limitations

Some tools require manual installation or Homebrew on Linux:

| Tool | Notes |
|------|-------|
| **eza** | Not in default repos; may need Homebrew or cargo |
| **yazi** | Requires cargo or manual binary install |
| **zoxide** | Installed via curl script |
| **git-delta** | May need Homebrew |
| **Ghostty** | Manual installation required |
| **dust, bandwhich, gping, doggo, trippy** | Network tools require cargo or Homebrew |

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

## Desktop Environment Shortcuts

The installer automatically configures desktop environment shortcuts to free up `Super+C/V/1-9` for Ghostty, making the Cmd key behave like macOS.

### KDE Plasma

| Original Shortcut | Action | Changed To |
|-------------------|--------|------------|
| `Super+V` | Show Clipboard Items | `Alt+V` |
| `Super+1-9` | Activate Task Manager Entry | Disabled |

### GNOME

| Original Shortcut | Action | Changed To |
|-------------------|--------|------------|
| `Super+1-9` | Switch to Application | Disabled |

### XFCE

Any `Super+1-9` shortcuts are removed to avoid conflicts.

### LXDE / LXQt / Headless

No changes needed—these environments don't typically bind `Super+key` combinations.

> **Note:** Log out/in after installation for DE shortcut changes to take full effect.

---

## Installation Flags Reference (Legacy bash script)

These flags apply to the **legacy `dotfiles-setup` bash script only**. They are not recognized by the Go binary (see the [Go CLI](#go-cli-subcommands--flags) section below).

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

## Go CLI Subcommands & Flags

The current **Go binary** (`dotfiles`) uses subcommands rather than the legacy flags above. Run `dotfiles` with no arguments to launch the TUI main menu.

| Command | Description |
|---------|-------------|
| `dotfiles install` | Launch the TUI installer |
| `dotfiles manage` | Launch the TUI management screen |
| `dotfiles update [check]` | Check for / apply tool updates |
| `dotfiles theme set <name>` | Set theme directly (`dotfiles theme list` lists themes; bare `dotfiles theme` opens the picker) |
| `dotfiles config <tool>` | Configure a specific tool |
| `dotfiles status` | Print status (CLI) |
| `dotfiles backups` | List backups |
| `dotfiles restore [backup-name]` | Restore a backup |
| `dotfiles hotkeys [--tool <name>]` | View hotkeys |
| `dotfiles user add <name> [--theme <t>] [--nav <emacs\|vim>] [--keyboard <macos\|linux>]` | Create a user profile |
| `dotfiles user delete <name> [--force]` | Delete a user profile |
| `dotfiles users` | List user profiles |
| `dotfiles --<Username>` | Quick-switch to an existing user profile (e.g. `dotfiles --Pratik`) |
| `dotfiles uninstall [--keep-config] [--keep-binaries] [--no-restore] [--force]` | Remove dotfiles and restore config |
| `dotfiles version` | Print version |

Global flag: `--skip-intro` skips the intro animation.

---

## Go TUI Tool Registry

The Go TUI manages tools from its own registry (`internal/tools/`). In addition to the cross-platform CLI tools shared with the legacy list (zsh, tmux, neovim, yazi, git, git-delta, fzf, bat, eza, zoxide, ripgrep, fd, btop), it registers the following tools that are **not** covered by the legacy bash script sections above.

### CLI Tools (Go TUI)

| Tool | Description | Packages |
|------|-------------|----------|
| **LazyGit** | Simple terminal UI for Git commands. | `lazygit` (all platforms) |
| **LazyDocker** | Simple terminal UI for Docker. Resource-heavy; skipped on low-memory systems (e.g. Pi Zero 2). | `lazydocker` (all platforms) |
| **Glow** | Render markdown on the CLI. | `glow` (all platforms) |
| **fswatch** | Cross-platform file change monitor. | `fswatch` (all platforms) |
| **Tailscale** | Mesh VPN for secure networking. | `tailscale` (all platforms) |
| **Sunshine** | Self-hosted game streaming server. | `sunshine` (all platforms) |
| **Moonlight** | Open-source game streaming client. | `moonlight` (macOS), `moonlight-qt` (Arch, Debian) |
| **Claude Code** | AI-powered coding assistant. Installed via npm (`@anthropic-ai/claude-code`); requires `node` (macOS) or `nodejs`+`npm` (Arch, Debian). Has a dedicated MCP configuration screen in the TUI. | `node` (macOS), `nodejs`, `npm` (Arch, Debian) |

### GUI Applications (Go TUI)

| Tool | Description | Packages |
|------|-------------|----------|
| **Zen Browser** | Privacy-focused browser based on Firefox. | `zen-browser` (macOS), `zen-browser-bin` (Arch); no Debian package |
| **Cursor** | AI-first code editor. | `cursor` (macOS), `cursor-bin` (Arch); no Debian package |
| **LM Studio** | Local LLM runner. | `lm-studio` (macOS, Arch); no Debian package |
| **OBS Studio** | Streaming and recording software. | `obs` (macOS), `obs-studio` (Arch, Debian) |

> The Go TUI also registers the macOS-only apps Rectangle, Raycast, IINA, and AppCleaner (see the [macOS Quality-of-Life Apps](#optional-quality-of-life-apps) section).

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
| `~/.config/btop/btop.conf` | btop resource monitor settings (Go TUI) |
| `~/.config/lazygit/config.yml` | LazyGit configuration (Go TUI) |
| `~/.config/lazydocker/config.yml` | LazyDocker configuration (Go TUI) |
| `~/.config/glow/glow.yml` | Glow markdown renderer settings (Go TUI) |
| `~/.claude.json` | Claude Code user-scope MCP servers (Go TUI; only the `mcpServers` key is managed) |
| `~/.gitconfig` | Git configuration with delta |
| `~/.config/dotfiles/settings` | Current theme, navigation style, and active user |
| `~/.sshh` | SSH hosts for quick connect |
