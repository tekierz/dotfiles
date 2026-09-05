# Tools Reference

This document describes the current Go application tool surface. The source of
truth is the registry in `internal/tools/`; install and configuration flows read
from that registry rather than from a shell installer script.

## Go CLI

Run `dotfiles` with no arguments to open the main TUI.

| Command | Description |
|---------|-------------|
| `dotfiles install` | Launch the installation wizard |
| `dotfiles manage` | Configure installed tools |
| `dotfiles update [check]` | Check for package updates |
| `dotfiles theme --list` | List themes |
| `dotfiles theme set <name>` | Save the active theme |
| `dotfiles config <tool>` | Configure one supported tool |
| `dotfiles status` | Print current settings |
| `dotfiles backups` | List backups |
| `dotfiles restore [backup-name]` | Restore a backup |
| `dotfiles hotkeys [--tool <name>]` | View hotkeys |
| `dotfiles user add <name>` | Create a user profile |
| `dotfiles user delete <name> [--force]` | Delete a user profile |
| `dotfiles users` | List user profiles |
| `dotfiles --<Username>` | Switch to an existing user profile |
| `dotfiles uninstall [flags]` | Restore previous configs and remove installed files |
| `dotfiles version` | Print version information |

## Core Terminal Stack

| Tool | Purpose |
|------|---------|
| `zsh` | Shell configuration, prompt, navigation style, aliases |
| `tmux` | Terminal multiplexer with generated status/theme config |
| `ghostty` | Terminal emulator config |
| `neovim` | Editor config, currently Kickstart.nvim-oriented |
| `git` | Git defaults and delta integration |
| `yazi` | Terminal file manager |
| `fzf` | Fuzzy finder integration |

## CLI Utilities

| Tool | Purpose |
|------|---------|
| `bat` | Syntax-highlighting `cat` replacement |
| `eza` | Modern `ls` replacement |
| `zoxide` | Smart directory jumping |
| `ripgrep` | Fast recursive search |
| `fd` | Fast file finding |
| `delta` | Syntax-highlighting pager for git diffs |
| `fswatch` | Cross-platform file change monitor |
| `btop` | System monitor |
| `glow` | Terminal markdown viewer |
| `lazygit` | Git terminal UI |
| `lazydocker` | Docker terminal UI on supported platforms |
| `claude-code` | Claude Code CLI and MCP configuration |
| `tailscale` | Mesh VPN |
| `sunshine` | Game streaming host |
| `moonlight` | Game streaming client |

## GUI Apps

These are optional desktop apps exposed through the GUI Apps screen where the
current platform supports package installation or detection.

| App | Purpose |
|-----|---------|
| Rectangle | Window snapping and management |
| Raycast | Launcher and productivity tooling |
| Zen Browser | Privacy-focused browser |
| Cursor | AI code editor |
| LM Studio | Local LLM desktop app |
| OBS Studio | Video recording and streaming |
| IINA | Video player |
| AppCleaner | macOS app cleanup |

## Embedded Utility Scripts

The Go installer writes these small helper scripts to the user's local bin
directory:

| Script | Purpose |
|--------|---------|
| `hk` | Hotkey cheatsheet |
| `caff` | Toggle sleep prevention on macOS/Linux |
| `sshh` | Quick SSH host menu backed by `~/.sshh` |

## Themes And Navigation

The Go app manages 16 themes and two navigation styles (`emacs`, `vim`). Theme
changes are saved globally and applied by generated configs for supported tools.

Use:

```bash
dotfiles theme --list
dotfiles theme set dracula
```

## Platform Notes

- macOS uses Homebrew packages and casks where available.
- Arch uses pacman/paru where package metadata is defined.
- Debian/Ubuntu uses apt packages where available.
- Raspberry Pi systems are detected by the Go package manager layer; low-memory
  models skip heavy tools.

## Legacy Installer Status

The Bash and PowerShell installers have been removed from this repository. The
Go app still removes stale `dotfiles-tui` and `dotfiles-setup` binaries during
install/uninstall so older machines can migrate cleanly.
