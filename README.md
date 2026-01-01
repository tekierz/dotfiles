# dotfiles-setup

A cross-platform terminal environment setup script with a **Catppuccin Mocha** theme.

Sets up a consistent, beautiful terminal experience across macOS, Linux (Arch/Debian), and Windows (via WSL).

## What It Installs & Configures

| Tool | Description |
|------|-------------|
| **zsh** | Shell with vim mode, syntax highlighting, autosuggestions |
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

## Installation

### Quick Install (Recommended)

```bash
# macOS/Linux
curl -fsSL https://raw.githubusercontent.com/tekierz/dotfiles/main/bin/dotfiles-setup | bash

# Or clone and run
git clone https://github.com/tekierz/dotfiles.git
cd dotfiles
./bin/dotfiles-setup
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

## Features

### Consistent Theme (Catppuccin Mocha)

All tools are configured with the Catppuccin Mocha color scheme for a unified look:

- Terminal (Ghostty)
- Tmux status bar
- fzf fuzzy finder
- Yazi file manager
- Git diffs (delta)
- Bat syntax highlighting
- Man pages

### Vim-Style Navigation Everywhere

| Context | Keys |
|---------|------|
| Zsh | `Esc` for normal mode, `Ctrl-e` edit in nvim |
| Tmux | `hjkl` pane navigation, `Alt-hjkl` without prefix |
| Yazi | Full vim keybindings |
| fzf | Vim-style selection |

### Custom Utilities

| Command | Description |
|---------|-------------|
| `hk` | Hotkey reference cheatsheet |
| `caff` | Toggle system sleep (like Caffeine) |
| `sshh` | Quick SSH connection manager |
| `y` | Yazi file manager (cd on exit) |

### Shell Aliases

```bash
ls      # eza with icons
ll      # long format with git status
la      # show hidden files
lt      # tree view
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
| `~/.sshh` | SSH hosts for sshh |

## Post-Install

1. Restart your terminal or `source ~/.zshrc`
2. Run `tmux` to start tmux
3. Run `nvim` to install plugins
4. Run `p10k configure` to customize prompt
5. Run `hk` to see all hotkeys
6. Run `sshh edit` to add SSH hosts

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
