# Tools Reference

The supported `dotfiles` product is the Go application. Its runtime registry in
`internal/tools/registry.go` is the source of truth for available tools; the
dashboard filters that registry by operating system, package availability, and
low-memory constraints before it builds an install plan.

The retired `dotfiles-setup` Bash product had a different tool list and behavior.
It is no longer distributed or supported. See [Legacy installer migration](legacy-migration.md)
if an older machine still resolves that command.

## Registered tools

The registry currently contains 30 tools. Availability is determined from each
tool's platform package metadata; seeing a tool here does not promise that every
package manager or CPU architecture provides it.

### Shell, terminal, editor, and file management

| Tool | Dashboard role |
|------|----------------|
| Zsh | Interactive shell configuration |
| Ghostty | Terminal configuration and keybindings |
| tmux | Multiplexer configuration and keybindings |
| Neovim | Editor installation and configuration |
| Yazi | Terminal file manager configuration |

### Git and command-line utilities

| Tool | Dashboard role |
|------|----------------|
| Git | Git defaults and bounded delta/difftastic integration |
| Git Delta | Syntax-highlighted Git diffs |
| LazyGit | Terminal Git interface |
| LazyDocker | Terminal Docker interface |
| fzf | Fuzzy finding and shell integration |
| bat | Syntax-highlighted file viewing |
| eza | Directory listing |
| zoxide | Directory jumping |
| ripgrep | Recursive text search |
| fd | File search |
| btop | Resource monitoring |
| Glow | Markdown rendering |
| fswatch | File-change monitoring |

### Services and integrations

| Tool | Dashboard role |
|------|----------------|
| Claude Code | AI coding CLI and user-scope MCP configuration |
| Tailscale | Mesh VPN client |
| Sunshine | Game-streaming host |
| Moonlight | Game-streaming client |

These integrations are not equivalent in completeness. Review the dashboard's
capability and health information, the exact install plan, and any authentication
or service setup required by the vendor before applying changes.

### Graphical applications

| Tool | Dashboard role |
|------|----------------|
| Zen Browser | Browser installation |
| Cursor | Desktop editor installation |
| LM Studio | Local model application installation |
| OBS Studio | Recording and streaming application installation |
| Rectangle | macOS window management |
| Raycast | macOS launcher |
| IINA | macOS media player |
| AppCleaner | macOS application removal utility |

The planned Cursor Agent CLI is distinct from the Cursor desktop application in
this table. Codex, Cursor Agent, OpenCode, Pi, T3 Code, and Hermes remain gated by
the release safety plan until their provenance, authentication, permission, and
remote-artifact policies are implemented.

## CLI commands

Run `dotfiles` with no arguments to launch the TUI.

| Command | Description |
|---------|-------------|
| `dotfiles install` | Launch the installation wizard |
| `dotfiles manage` | Open the installed-tool dashboard |
| `dotfiles update [check]` | Check for or apply tool updates |
| `dotfiles config <tool>` | Open configuration for one tool |
| `dotfiles hotkeys [--tool <name>]` | View keybindings |
| `dotfiles status` | Print current product configuration |
| `dotfiles doctor [--json]` | Diagnose executable provenance and PATH collisions |
| `dotfiles doctor repair [--json]` | Preview a stale user-local binary repair; confirmation is required to apply |
| `dotfiles theme list` | List themes |
| `dotfiles theme set <name>` | Select the product theme |
| `dotfiles backups` | List product backups |
| `dotfiles restore [backup-name]` | Restore a product backup |
| `dotfiles user add <name>` | Create a user profile |
| `dotfiles user delete <name>` | Delete a user profile |
| `dotfiles users` | List user profiles |
| `dotfiles version` / `dotfiles --version` | Print build version |
| `dotfiles uninstall` | Restore backups and print conservative manual removal guidance |

`dotfiles doctor` is read-only. It reports the running executable, ordered
`PATH` matches, static build metadata, Homebrew ownership hints, and stale legacy
binary names without executing or deleting discovered binaries.

`dotfiles doctor repair` is a separate, fail-closed mutation path limited to
`~/.local/bin/dotfiles`. It refuses symlinks, hardlinks, unknown builds, the
running executable, and Homebrew-owned or byte-identical candidates. A preview
is bound to a SHA-256 plan hash; noninteractive apply requires `--yes` and
`--plan-hash <hash>`. Successful repair preserves the exact binary as mode 0600
and records its original mode plus restore guidance in a private manifest.
`dotfiles-tui` and `dotfiles-setup` are never changed by this command.

## Managed configuration locations

Paths can vary by platform and native application precedence. The operation plan
shown before apply is authoritative for a particular machine.

| Path | Purpose |
|------|---------|
| `~/.zshrc` | Zsh integration |
| `~/.tmux.conf` | tmux settings |
| `${XDG_CONFIG_HOME:-~/.config}/ghostty/config.ghostty` | XDG Ghostty settings |
| `~/Library/Application Support/com.mitchellh.ghostty/config.ghostty` | macOS Ghostty settings |
| `~/.config/yazi/` | Yazi settings |
| `~/.config/bat/config` | bat settings |
| `~/.config/btop/btop.conf` | btop settings |
| `~/.config/lazygit/config.yml` | LazyGit settings |
| `~/.config/lazydocker/config.yml` | LazyDocker settings |
| `~/.config/glow/glow.yml` | Glow settings on platforms using the XDG path |
| `~/.claude.json` | Claude Code user-scope MCP servers |
| `~/.gitconfig` | Native Git configuration with a bounded managed include |
| `~/.config/dotfiles/git/config` | Product-owned Git and diff settings |
| `~/.config/dotfiles/settings` | Product theme, navigation, and active user |
| `~/.config/dotfiles/users/` | Product user profiles |
| `~/.sshh` | SSH host entries used by `sshh` |

Git and Ghostty adoption preserves existing native content inside bounded
managed sections. Recovery comes from the reviewed operation backup; writers do
not create or overwrite ambiguous sibling `*.dotfiles.bak` files.
