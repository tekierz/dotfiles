# Pre-Release Adversarial Review — dotfiles

**Ref reviewed:** `main` @ `7806117` · **Date:** 2026-07-04
**Method:** dynamic multi-agent workflow — 3 promise-inventory mappers (UI/CLI+docs/bash) →
12 adversarial finders across security, quality, and feature-completeness → every candidate
adversarially verified by an independent reviewer → a completeness critic named 4 coverage
gaps, each closed by a targeted finder. 98 agents, 0 errors. Models: opus-4.8 (x-high) as
finder/verifier, gpt-5.5 (x-high) via codex as targeted researcher, fable as critic/synthesis.

**Result: 67 verified findings — 0 critical, 3 high, 20 medium, 44 low. 34 are incomplete
features** (advertised behavior that does not work), the rest are quality/security defects in
working code. Promise inventory covered: 87 UI, 53 CLI/doc, 42 installer claims.

> **Bottom line:** No data-loss or critical-severity issues survived — the P0/P1/P2 remediation
> holds. But the product's headline promise, *"16 themes with unified colors across all tools,"*
> is substantially **not delivered**: the file manager, terminal, multiplexer, and git-diff
> theming are all dead or wrong. This is a **cohesive incomplete-feature problem, not a pile of
> unrelated bugs** — and it is the single most important thing to fix before showing anyone.

---

## The dominant theme: "unified theming" is mostly unwired

Seven findings (all 3 highs + 4 mediums) form one story. The wizard lets the user pick a theme
with a live preview, the install summary promises theme files, and the README claims *"themes
apply consistently across the terminal, editor, file manager, git diffs."* In reality:

- **yazi (file manager) — 3 HIGH, and this is the worst of them.** The generator writes
  `yazi.toml`, `keymap.toml`, and `theme.toml` under a `[manager]` section. Current yazi (26.x,
  what brew/pacman install today) **renamed that section to `[mgr]` and silently ignores
  `[manager]`.** A verifier reproduced this on the installed yazi 26.5.6 with a differential
  parse test: bad values under `[mgr]` are rejected, the same values under `[manager]` launch
  clean and are discarded. Consequence: **every** yazi manager setting (sort, hidden files,
  linemode, ratio), **every** generated keybinding (the entire vim/emacs keymap), and **all**
  theme colors are inert. yazi is the *one* tool whose per-theme palette is actually computed —
  and none of it reaches the program.
- **Ghostty & tmux themes are written as comments only** (`ghostty.go:63`, `tmux.go:120`). The
  selected theme name is emitted as a `# comment`; no color/palette/status-bar directive is
  written, so both keep their default colors. Ghostty's install summary also promises a
  `themes/dotfiles-theme` file that is never created.
- **git-diff (delta) theme is hardcoded to `Dracula`** (`git.go:156`) regardless of selection,
  so diffs never match the rest of the environment.

**Recommendation:** treat theming as one coordinated fix, not seven tickets. The yazi
section-rename is a hard blocker (the feature is 100% dead on current yazi); ghostty/tmux/delta
are "write the actual colors, not a comment." A single focused pass closes all seven.

## Incomplete features beyond theming (advertised, doesn't work)

- **Neovim LSP checkboxes are a no-op** (`neovim.go:92`) — the deep-dive screen (subtitle
  literally *"Editor configuration and LSP"*) collects Lua/Python/TS/Go/Rust/C++ selections into
  `NeovimConfig.LSPs`, but the generator writes no lspconfig/mason/server setup at all.
- **`dotfiles uninstall` doesn't remove `sshh`** (`main.go:796`) — the help text promises it; the
  removal loop targets `hk`/`caff`/`y` and leaves `~/.local/bin/sshh` behind while hunting a `y`
  binary that was never installed. Compounded by **README contradicting itself** (`README.md:219`
  says sshh is "not bundled or managed by dotfiles" while the installer bundles it by default).
- **Debian/Raspberry Pi are second-class in the bash installer** — a cluster of platform gaps:
  `bat`/`fd` install as Debian's renamed `batcat`/`fdfind` and are never symlinked, so every
  `command -v bat` guard fails and the bat config + fzf preview + fzf fd-commands silently die
  while setup reports success (`:1390`); zsh-syntax-highlighting/autosuggestions are cloned to a
  path the zshrc never sources on Pi and not installed at all on Debian (`:1330`); Powerlevel10k
  is only installed/sourced on macOS (`:3982`); Starship and Pure prompt styles are selectable
  in the TUI but never installed (`zsh.go:181/204`).
- **Hotkey cheatsheet advertises binds that don't exist** — the tmux cheatsheet promises
  `Prefix+H/J/K/L` resize and (vim mode) `Alt-h/j/k/l` navigation, but `tmux.go` binds neither
  (`hotkeys.go:110/111`); the `hk` table advertises Ghostty tab shortcuts that aren't wired.
- **Tailscale is advertised in the deep-dive menu but has no checkbox** on the CLI Utilities
  screen (`screen_config_cliutilities.go:19`) — it can never be selected/installed via that path.

> The good news: the specific dead-UI class fixed in P1 (zsh plugins, p10k on macOS, fzf
> sourcing, mac-apps list, hotkeys input, manage-panel input) **held up** — verifiers confirmed
> those fixes. The findings above are *siblings* the earlier sweep didn't reach, plus the
> platform-specific bash paths that the Go-side P1 work didn't touch.

## Security

No critical or high security findings. The surviving items are all **medium/low and concentrated
in the legacy bash restore path** — and they trace directly to the consolidation you just did:

- **Bash restore is missing the traversal/symlink hardening that lives only on `archive/audit-remediation`** (commit `3a7a71b`, which is *not* an ancestor of `main`). Three linked mediums:
  session name isn't validated so `--restore ../../tmp/evil` reads an attacker manifest
  (`:989`); the manifest *source* path is never validated so a foreign manifest copies arbitrary
  files into `$HOME` → config overwrite → code execution on next shell (`:1025`); and
  `is_safe_restore_path` uses a lexical prefix check a symlinked intermediate dir escapes, so
  `cp`/`rm` can be redirected outside `$HOME` (`:970`). The Go restore path *has* the
  `isValidBackupName` guard; the bash path doesn't. **Port `3a7a71b` from the archive branch.**
- Lower-severity: `delete_user` rm's a username-derived path with no validation (`:3191`); bash
  config writes use default umask instead of the 0600/0700 policy the Go side enforces (`:916`);
  unpinned `curl|bash` for Homebrew/zoxide with no checksum (`:1136`); a Go restore TOCTOU
  between the Lstat symlink check and WriteFile (`backup.go:143`).

The injection lane came back **clean** — the prior audit's claimed shell-injection findings in
`internal/runner/bash.go` did **not** reproduce at this ref (independently re-derived).

## Quality (working code, real bugs)

- **Concurrent map read/write in standalone config apply** (`config_apply.go:44`) — the one
  finding that can *crash*: `dotfiles config claude-code`, on Enter, hands the live
  `ClaudeCodeMCPs` map to a worker goroutine (via a shallow `*deepDiveConfig` copy) while the
  still-active screen can toggle it. Go's race detector aborts on concurrent map access. The
  earlier `manage`-save snapshot fix did **not** cover this second standalone-apply path.
- **Package-manager display/action mismatches** — `brew` "Update all" runs a system-wide
  `brew upgrade`, not the dotfiles-filtered list shown (`brew.go:299`); apt's targeted
  `UpdateStreaming` skips the `apt update` refresh every other apt path does, so a selected
  upgrade can 404 on a stale index (`apt.go:296`); apt `GetVersion` reports removed-but-not-purged
  packages as installed (`apt.go:70`); pacman's `-Qu` fallback treats any error as "no updates"
  (`pacman.go:160`).
- **16 silent-failure findings** — mostly persistence errors swallowed with `_ =`: theme/nav
  preference save (`installation.go:846`), hotkey favorite + alias saves (`screen_hotkeys.go`),
  active-user deletion leaving an orphaned marker (`screen_users.go:158`), backup-retention
  cleanup ignoring `RemoveAll` failure so the max-count/age policy silently never runs
  (`app.go:792`). Individually low; collectively they mean the app reports success it can't back.

## What the verifiers threw out (the review self-corrected)

11 candidates were **refuted** — evidence the verification layer worked, and confirmation that
several of *your recent fixes are solid*: the pacman `-Syu` full-upgrade (intentional + now
disclosed), the apt non-blocking refresh, the `manage`-save snapshot isolation, and brew
`--greedy` handling were all challenged and cleared.

---

## Recommended sequencing before manual testing

1. **Theming pass (the 3 HIGH + 4 theme mediums)** — yazi `[manager]`→`[mgr]` across all three
   files, plus emit real colors for ghostty/tmux/delta. Without this, the flagship feature is a
   demo that doesn't work; a tester will hit it in the first five minutes.
2. **Port the bash-restore hardening** from `archive/audit-remediation` `3a7a71b` (3 mediums, and
   it's a code-exec path).
3. **Fix the standalone-config concurrent map** (`config_apply.go:44`) — it's a crash.
4. **The incomplete-feature mediums** — neovim LSP, uninstall/sshh + README, the Debian/Pi
   installer cluster. These are "advertised, doesn't work" and will read as broken to a new user.
5. Low-severity silent-failures and the umask/permission bash items can batch into a cleanup pass.

The full finding list follows. `feat` = incomplete advertised feature; `bug` = defect in working
code. Every row was adversarially verified (CONFIRMED unless noted).

## HIGH
| Sev | Kind | Location | Finding |
|---|---|---|---|
| HIGH | feat | `internal/tools/yazi.go:66` | yazi.toml is written with a [manager] section that yazi 26.x silently ignores, so no manager settings (sort, hidden files, linemode, scrolloff, ratio) ever apply |
| HIGH | feat | `internal/tools/yazi.go:155` | keymap.toml is written under a [manager] section that yazi 26.x ignores, so all generated vim/emacs navigation keybindings are dead |
| HIGH | feat | `internal/tools/yazi_theme.go:260` | yazi theme.toml is generated under the pre-rename [manager] section (also [select], separator_open/close), which current yazi ignores — the file-manager pane is left unthemed even though yazi is the ONE tool whose theme colors are actually computed |

## MEDIUM
| Sev | Kind | Location | Finding |
|---|---|---|---|
| MEDIUM | bug | `README.md:219` | README states `sshh` is 'not bundled or managed by dotfiles', but the installer bundles and installs it by default and the manage/cache code tracks it. |
| MEDIUM | bug | `bin/dotfiles-setup:970` | Bash is_safe_restore_path uses a lexical HasPrefix check that a symlinked intermediate directory escapes, redirecting cp/rm outside $HOME |
| MEDIUM | bug | `bin/dotfiles-setup:989` | Bash restore_backup does not validate the session name for path traversal, unlike the Go path's isValidBackupName guard |
| MEDIUM | bug | `bin/dotfiles-setup:1025` | Bash restore_backup never validates the manifest SOURCE path, so a tampered/foreign manifest copies arbitrary files into $HOME (config overwrite -> code execution) |
| MEDIUM | feat | `bin/dotfiles-setup:1330` | zsh-syntax-highlighting and zsh-autosuggestions are never loaded on Raspberry Pi (cloned into the oh-my-zsh custom path the zshrc never sources) and are neither installed nor cloned at all on Debian, so the advertised syntax-highlighting/autosuggestions feature is dead on both platforms. |
| MEDIUM | feat | `bin/dotfiles-setup:1390` | On Debian and Raspberry Pi, bat and fd are installed under Debian's renamed binaries (batcat/fdfind) and never symlinked, so every `command -v bat`/`command -v fd` guard fails and the bat config, fzf bat-preview, and fzf fd file/dir commands are all silently disabled while setup_bat still prints success. |
| MEDIUM | feat | `cmd/dotfiles/main.go:796` | `dotfiles uninstall` promises to remove the `sshh` utility it installed but never does, leaving ~/.local/bin/sshh behind while trying to delete a nonexistent `y` binary. |
| MEDIUM | feat | `internal/hotkeys/hotkeys.go:110` | Under vim nav style the tmux cheatsheet advertises 'Alt-h/j/k/l' to navigate panes, but tmux.go always binds Alt-Arrow only |
| MEDIUM | feat | `internal/hotkeys/hotkeys.go:111` | tmux cheatsheet advertises 'Prefix + H/J/K/L' to resize panes, but the generated ~/.tmux.conf binds no resize keys and tmux has no such default |
| MEDIUM | bug | `internal/pkg/apt.go:296` | apt UpdateStreaming (the targeted path the TUI actually calls for selected updates) omits the `apt update` refresh that every other apt update path performs, so a targeted upgrade runs against a possibly-stale index and can fail to fetch the candidate .deb. |
| MEDIUM | bug | `internal/pkg/brew.go:299` | Updates screen 'a' (Update all) runs a full system-wide `brew upgrade`, not the dotfiles-filtered list it displays |
| MEDIUM | feat | `internal/tools/ghostty.go:62` | Selected theme is never applied to Ghostty, and the install summary lists a 'themes/dotfiles-theme' file that is never created. |
| MEDIUM | feat | `internal/tools/ghostty.go:63` | Ghostty's selected theme is written only as a comment; no color/palette/theme directive is emitted, so the terminal never adopts the chosen theme. |
| MEDIUM | feat | `internal/tools/git.go:156` | Delta git-diff syntax theme is hardcoded to `Dracula` regardless of the selected dotfiles theme, so theme selection does not apply to git diffs. |
| MEDIUM | feat | `internal/tools/neovim.go:92` | Neovim 'LSP Servers' checkboxes (Lua/Python/TypeScript-JS/Go/Rust/C-C++) are a no-op: the selected servers are never written to any config. |
| MEDIUM | feat | `internal/tools/tmux.go:120` | Tmux's selected theme is written only as a comment; no status-bar or pane colors are emitted, so tmux appearance never reflects the chosen theme. |
| MEDIUM | bug | `internal/ui/config_apply.go:44` | applyStandaloneConfigCmd iterates a.deepDiveConfig.ClaudeCodeMCPs on a worker goroutine while the still-active config screen can toggle the same map (fatal concurrent map read/write) |

## LOW
| Sev | Kind | Location | Finding |
|---|---|---|---|
| LOW | bug | `bin/dotfiles-setup:780` | run_pkg records all package failures as 'optional', including network prerequisites (git/curl/wget) that later steps depend on |
| LOW | bug | `bin/dotfiles-setup:916` | Bash safe_write_config and every `mkdir -p ~/.config/...` create tool config files and directories at the default umask (0644/0755), never applying the documented 0700/0600 policy |
| LOW | bug | `bin/dotfiles-setup:1136` | Unpinned remote code executed via `bash -c "$(curl ...)"` (Homebrew) and `curl ... \| bash` (zoxide) with no checksum/signature verification |
| LOW | feat | `bin/dotfiles-setup:1139` | Intel-Mac Homebrew PATH fallback is dead code: `eval ""` always returns 0 so the `\|\| eval .../usr/local/bin/brew shellenv` never runs |
| LOW | bug | `bin/dotfiles-setup:1140` | Homebrew install is never verified: on curl failure it prints a false 'Homebrew installed' success; on installer non-zero exit the bare command aborts the whole run |
| LOW | feat | `bin/dotfiles-setup:1175` | The macOS 'Installing Nerd Fonts' step taps the removed homebrew/cask-fonts, which now errors on every current Homebrew, recording a spurious package failure in the summary. |
| LOW | bug | `bin/dotfiles-setup:1254` | Bare `sudo apt update` (and every sudo call lacks `-n`/upfront `sudo -v`) aborts the whole installer under set -e and can hang/fail in a TTY-less curl\|bash run |
| LOW | bug | `bin/dotfiles-setup:1383` | The Debian branch never installs git, but setup_nvim git-clones kickstart.nvim and setup_git requires git; on a minimal Debian without git preinstalled the neovim setup fails and git configuration is skipped. |
| LOW | feat | `bin/dotfiles-setup:2305` | The hk static hotkey table advertises Ghostty shortcuts (Super+1-9 tab switching, Super+{/} prev/next tab, Ctrl+Shift+, reload, Ctrl+Shift+n new window) that setup_ghostty never writes and that are not Ghostty defaults on Linux, so the DE step that 'frees Super+1-9 for Ghostty' leaves those keys bound to nothing. |
| LOW | bug | `bin/dotfiles-setup:3191` | delete_user removes a username-derived path with no validate_username call, allowing path traversal to delete arbitrary *.settings files |
| LOW | feat | `bin/dotfiles-setup:3982` | Powerlevel10k is only installed and sourced on macOS; on Arch/Debian/Pi it is neither installed nor sourced, so the advertised next-step `4. p10k configure` fails with command-not-found on every Linux target. |
| LOW | bug | `internal/backup/backup.go:143` | Go noFollowWrite has a TOCTOU between the Lstat symlink check and WriteFile, allowing a raced symlink to redirect the write |
| LOW | bug | `internal/backup/backup.go:546` | Backup restore recreates directories and files using the mode captured in the backup, so a legacy/bash backup can reintroduce looser-than-policy permissions on restore |
| LOW | feat | `internal/hotkeys/hotkeys.go:146` | Zsh cheatsheet advertises 'Ctrl-g' to fuzzy-find git files, but no generated config binds Ctrl-g and it is not a shell/fzf default |
| LOW | feat | `internal/hotkeys/hotkeys.go:157` | Under emacs nav style (the default) the Yazi cheatsheet advertises 'Ctrl-h' to toggle hidden files, but the keymap generator only ever binds '.' and Yazi's default is also '.' |
| LOW | bug | `internal/pkg/apt.go:70` | apt GetVersion uses `dpkg -s`, which exits 0 and returns a Version for removed-but-not-purged packages, contradicting IsInstalled and reporting a version for a package that is not installed. |
| LOW | bug | `internal/pkg/brew.go:103` | `brew outdated --greedy` spends network/latency enumerating auto-updating casks that CheckDotfilesUpdates always discards, and a single failing cask check can hide upgradeable formulae |
| LOW | bug | `internal/pkg/pacman.go:160` | pacman checkOfficialUpdates fallback treats any `pacman -Qu` failure with empty stdout as 'no updates', masking genuine errors (DB lock, corrupt sync DB). |
| LOW | feat | `internal/pkg/update.go:66` | `dotfiles` formula is absent from the update allow-list, so `dotfiles update` can never update the dotfiles binary itself |
| LOW | bug | `internal/runner/bash.go:172` | Sudo and package-manager helpers are resolved through the inherited PATH rather than an absolute path in the streaming runner |
| LOW | bug | `internal/scripts/scripts.go:167` | caff start() has a check-then-act TOCTOU race: concurrent `caff on` invocations each pass the status check and spawn their own caffeinate/systemd-inhibit, but only the last PID is recorded in the PIDFILE, orphaning the other inhibitor which `caff off` can never stop |
| LOW | bug | `internal/scripts/scripts.go:284` | `sshh add` writes records via `echo "$1 \| $2 \| ${3:-22}"` with no sanitization of `\|` or newlines, so name/host arguments can corrupt existing fields or forge entirely new host entries |
| LOW | feat | `internal/tools/btop.go:61` | btop/glow global theme selection is dropped — btop.conf color_theme comes from a separate cfg.Theme (default 'Default'), not the chosen palette |
| LOW | feat | `internal/tools/fzf.go:60` | fzf generator drops the selected theme — no --color option added to FZF_DEFAULT_OPTS |
| LOW | feat | `internal/tools/neovim.go:97` | Neovim generator drops the selected theme entirely — no colorscheme is ever emitted, so none of the 16 themes affect neovim colors |
| LOW | bug | `internal/tools/neovim.go:289` | Neovim config-apply reports success even when the init.lua require-line that actually loads the user's preferences fails to write |
| LOW | feat | `internal/tools/zsh.go:169` | The Go-TUI-generated ~/.zshrc omits most of the documented shell aliases (df→duf, du→dust, diskuse→ncdu, ping→gping, dig→doggo, trace→trippy, bandwidth→bandwhich, cd→zoxide, lt→eza). |
| LOW | feat | `internal/tools/zsh.go:181` | Zsh 'Starship' prompt style is selectable but starship is never installed by any tool, so the generated init line is a no-op and no prompt is applied. |
| LOW | feat | `internal/tools/zsh.go:204` | Zsh 'Pure' prompt style is selectable but the pure prompt package is never installed, so it silently falls back to a bare minimal prompt. |
| LOW | bug | `internal/ui/app.go:792` | Backup retention cleanup ignores RemoveAll failures, so the max-count/max-age policy can silently never take effect |
| LOW | feat | `internal/ui/config_apply.go:226` | README/docs claim themes apply to `bat` and that ~/.config/bat/config is written, but the Go TUI has no bat config/theme generator. |
| LOW | bug | `internal/ui/installation.go:511` | Installer writes hk/caff/sshh with symlink-following, non-atomic os.WriteFile, unlike the atomic temp+rename used for the binary; a pre-existing ~/.local/bin/{hk,caff,sshh} symlink causes the installer to overwrite the symlink's target file instead of replacing the link |
| LOW | bug | `internal/ui/installation.go:775` | Batch `brew upgrade` failure marks every package in the batch as failed, mis-reporting packages that actually upgraded |
| LOW | bug | `internal/ui/installation.go:846` | Installer silently discards the error from saving the user's theme/nav-style/animation preferences |
| LOW | feat | `internal/ui/screen_config_cliutilities.go:19` | Tailscale is advertised as a CLI Utility in the deep-dive menu but has no checkbox on the CLI Utilities screen, so it can never be selected/installed via the wizard. |
| LOW | feat | `internal/ui/screen_filetree.go:194` | Install summary shows a 'bat/ └── config (new)' file that the installer never writes. |
| LOW | bug | `internal/ui/screen_hotkeys.go:741` | Toggling a hotkey favorite swallows the persistence error, so the star appears set but is not saved |
| LOW | bug | `internal/ui/screen_hotkeys.go:824` | Saving a hotkey alias swallows the persistence error and reports no failure |
| LOW | bug | `internal/ui/screen_users.go:158` | Deleting the active user swallows ClearActiveUser error, leaving a stale active-user marker that orphans hotkey/favorites data |
