# Tools Reference

The supported `dotfiles` product is the Go application. Its runtime registry in
`internal/tools/registry.go` is the source of truth for available tools; the
dashboard filters that registry by operating system, package availability, and
low-memory constraints before it builds an install plan.

The retired `dotfiles-setup` Bash product had a different tool list and behavior.
It is no longer distributed or supported. See [Legacy installer migration](legacy-migration.md)
if an older machine still resolves that command.

## Registered tools

The registry currently contains 36 tools. Availability is determined from each
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
| LazyGit | Terminal Git interface with bounded, ownership-safe global presets |
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
| Codex | macOS install-only OpenAI coding CLI; version-pinned npm recipe |
| OpenCode | Install-only coding agent; Homebrew tap or Arch package recipe |
| Pi | macOS install-only coding agent; version-pinned npm recipe with lifecycle scripts disabled |
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
| T3 Code | macOS install-only coding-agent frontend; exact Homebrew cask action |

The Cursor desktop application is distinct from Cursor Agent CLI. Codex,
OpenCode, and Pi are now available as opt-in, install-only integrations through
the reviewed installer plan; the dashboard shows their exact source, arguments,
detector, authentication expectation, and risk before execution. Their settings
are not yet managed. T3 Code is available on macOS through the narrow reviewed
Homebrew-cask action. Automatic installation for Cursor Agent and Hermes remains
gated because both require a verified-artifact design for mutable vendor scripts.

Cursor Agent is discoverable as an observation-only integration. Its current
installation status is `verified artifact support pending`, so the dashboard
offers no automatic install and does not claim ownership of Cursor credentials
or configuration.

Hermes Agent is discoverable as an observation-only integration. Its current
installation status is `verified artifact and architecture support pending`, so
the dashboard offers no automatic install and does not claim installed state, configuration, credentials, platform support, or architecture support.

Codex and Pi are currently offered only on macOS. Stock Linux global-npm
permissions and Pi's Node 22.19+ requirement need an explicit user-owned runtime
and PATH contract before those platforms can be advertised safely.

## CLI commands

Run `dotfiles` with no arguments to launch the TUI.

| Command | Description |
|---------|-------------|
| `dotfiles install` | Launch the installation wizard |
| `dotfiles manage` | Open the installed-tool dashboard |
| `dotfiles update [check]` | Check for or apply tool updates |
| `dotfiles config <tool>` | Open configuration for one tool |
| `dotfiles hotkeys [--tool <name>]` | View keybindings |
| `dotfiles status [--json]` | Print current product configuration, or installation health JSON v1 |
| `dotfiles plan --json --tool <id>...` | Print a deterministic install-only plan; no defaults or apply behavior |
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
| `~/.tmux.conf` or `${XDG_CONFIG_HOME:-~/.config}/tmux/tmux.conf` | Active native tmux config preserved outside one bounded managed section |
| `${XDG_CONFIG_HOME:-~/.config}/ghostty/config.ghostty` (and legacy `config`) | Ghostty XDG candidates; the reviewed plan freezes the active candidate before mutation |
| `~/Library/Application Support/com.mitchellh.ghostty/config.ghostty` (and legacy `config`) | Higher-precedence macOS candidates; the last existing source in Ghostty's order is the frozen target |
| `${YAZI_CONFIG_HOME}` when set and absolute; else `${XDG_CONFIG_HOME}/yazi` when XDG is set and absolute; else `~/.config/yazi/` | Exact Yazi directory frozen by the reviewed plan; set relative overrides fail closed |
| `~/.config/bat/config` | bat settings |
| `~/.config/btop/btop.conf` | btop settings |
| Active LazyGit `config.yml` selected by `CONFIG_DIR`, XDG, or platform user-config precedence | LazyGit settings; the reviewed plan shows the exact writable target |
| `~/.config/lazydocker/config.yml` | LazyDocker settings |
| Active `glow.{yaml,yml}` under `GLOW_CONFIG_HOME`, `XDG_CONFIG_HOME/glow`, or Glow's platform user-config directory | Glow settings; the first Viper-compatible source wins |
| `~/.claude.json` | Claude Code user-scope MCP servers |
| `~/.gitconfig` | Native Git configuration with a bounded managed include |
| `~/.config/dotfiles/git/config` | Product-owned Git and diff settings |
| `${XDG_CONFIG_HOME:-~/.config}/dotfiles/global.json` | Versioned global theme, navigation, active-user, animation, and backup preferences |
| `${XDG_CONFIG_HOME:-~/.config}/dotfiles/tools/manage.json` | Versioned dashboard settings state used by Manage |
| `${XDG_CONFIG_HOME:-~/.config}/dotfiles/users/` | Product user profiles |
| `~/.sshh` | SSH host entries used by `sshh` |

Git and Ghostty adoption preserves existing native content inside bounded
managed sections. Recovery comes from the reviewed operation backup; writers do
not create or overwrite ambiguous sibling `*.dotfiles.bak` files.

Yazi management is pinned to the v26.5.6 configuration schema. `yazi.toml`,
`keymap.toml`, and `theme.toml` are classified independently: a missing file or
the exact current product form is writable, while native, malformed, or exact
historical content remains read-only with its reason exposed. Vim mode preserves
Yazi's default keymap and writes only the product compatibility/style header;
Emacs mode prepends five bounded navigation/selection bindings without replacing
upstream defaults. The installer reviews and writes all three files as separate
actions. Standalone Yazi saves review only main and keymap, while Manage reviews
only the affected main/keymap file or files. Ordinary standalone and Manage
saves never synthesize or rewrite `theme.toml`.

Glow management is pinned to Glow v2.1.2. It exposes `style`, `mouse`, `pager`,
`width`, `all`, `showLineNumbers`, and `preserveNewLines` in one trailing managed
YAML block while preserving unknown flat scalar settings byte-for-byte. Adoption
rejects complex, duplicate, malformed, or unsupported YAML and blocks when a
higher-precedence non-YAML source or environment setting override is active.
The eight built-in styles are writable. Existing custom style strings are
imported and displayed read-only; choose a built-in before saving other Glow
changes. Width `0` is Glow's automatic mode (maximum 120, fallback 80).

LazyGit management is pinned to LazyGit v0.62.1 (`f2788e4`) and deliberately
models four global concepts: side-panel width fraction, mouse events, one of two
color presets, and the builtin-or-Delta pager preset. The Delta pager may be
selected only when Git Delta is selected in the installer or already installed
for Manage and standalone saves. Product-generated files are managed as exact
whole files; arbitrary native YAML and custom color or pager shapes are imported
for display but remain read-only.

LazyGit target discovery follows upstream precedence:

1. `LG_CONFIG_FILE` source chains are observed and hydrated read-only; this
   release does not rewrite a chain of configuration files.
2. `CONFIG_DIR`, when set to an absolute path, is the exact config directory.
3. `XDG_CONFIG_HOME/lazygit/config.yml` is used when XDG is explicitly set.
4. Without overrides, macOS uses
   `~/Library/Application Support/lazygit/config.yml`; Unix uses
   `~/.config/lazygit/config.yml`.

The historical `jesseduffield/lazygit/config.yml` fallback can still be read for
native hydration, but it is intentionally read-only; migrate it to the modern
directory before dashboard writes. Relative overrides and active targets outside
the user's home directory fail closed. Repository-local `.git/lazygit.yml` or a
parent `.lazygit.yml` can override global values, so the dashboard discloses that
its four settings are global defaults rather than guaranteed per-repository
effective values.
