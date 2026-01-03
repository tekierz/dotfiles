# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **dotfiles-setup**: a cross-platform terminal environment setup script (~3,200 lines of bash) that creates a consistent terminal experience across macOS, Linux (Arch/Debian), and Raspberry Pi. It installs and configures zsh, tmux, Ghostty, neovim, yazi, and other terminal tools with unified theming.

## Repository Structure

```
bin/dotfiles-setup          # Main setup script (all logic is here)
bin/dotfiles-setup.ps1      # PowerShell variant (not maintained)
Formula/dotfiles-setup.rb   # Homebrew formula for distribution
docs/tools.md               # Detailed tool reference
README.md                   # User documentation
```

## Key Concepts

- **13 themes** with dynamic color loading via `load_theme_colors()` - all tools share unified colors
- **Two navigation styles**: emacs (default, beginner-friendly) and vim
- **Multi-user profiles**: stored in `~/.config/dotfiles/users/`, each with theme/nav preferences
- **Platform detection**: macOS (Homebrew), Arch (pacman/paru), Debian (apt), Raspberry Pi (optimized)
- **CLI interface**: `dotfiles` command for theme switching, user management, status
- **Backup & restore**: all existing configs backed up before modification, fully reversible

## Backup System

The script creates timestamped backups in `~/.config/dotfiles/backups/` before modifying any config file:

```bash
dotfiles-setup --list-backups    # List available backups
dotfiles-setup --restore         # Restore most recent backup
dotfiles-setup --restore <name>  # Restore specific backup
dotfiles-setup --no-backup       # Skip backups (not recommended)

# Post-install via CLI:
dotfiles backups                 # List backups
dotfiles restore                 # Restore from backup
```

Each backup session contains:
- Copies of all modified files with original permissions preserved
- A manifest file tracking what existed before installation
- Files that didn't exist are recorded so they can be removed on restore

## Working with the Setup Script

The main script at `bin/dotfiles-setup` contains:
- Theme definitions (color variables for each of 13 themes)
- Platform-specific installation functions
- Configuration file generation (zshrc, tmux.conf, ghostty config, etc.)
- User profile management
- Desktop environment shortcut configuration

When modifying:
- Test changes on multiple platforms when touching platform-specific code
- Preserve backward compatibility with existing user configurations
- The script uses `set -euo pipefail` - handle errors appropriately
- Use `safe_read_setting()` for reading config files - never use `source` on user-editable files
- Use `backup_file()` before overwriting any existing config

## Security Considerations

- **Never use `source` on user profiles** - use `safe_read_setting()` to extract specific values
- **Validate all user input** - `validate_username()` prevents path traversal
- **File permissions**: config directories get 700, settings files get 600
- **Theme validation**: themes are checked against whitelist before use

## Git Commit Rules

- Never add "Generated with Claude Code" tags to commits
- Never add "Co-Authored-By: Claude" lines to commits
- Keep commit messages clean and concise
- When asked to "push", only commit - user handles `git push` (SSH auth required)
