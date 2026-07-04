# Full-Codebase Release Audit — 2026-07-03

**Scope:** every line of the repository — 22 partitioned review units (Go, bash, PowerShell,
docs, CI) + 3 cross-cutting sweeps (duplication, terminal hardening, release readiness),
run by a 201-agent workflow. Every critical/high finding survived 3 independent adversarial
refuters; mediums survived 1. A completeness critic then closed coverage gaps
(`bin/dotfiles-setup.ps1`, tracked stale binary, govulncheck run).

**Important context:** the initial audit ran against the stale `fix/v2.1.2` checkout
(Jan 2026). All 160 findings were then **re-verified one-by-one against `main`**
(which has since had two remediation cycles). Statuses below are the verified state of
`main` at commit `c04c1d6`.

**Baseline on main:** `go build`, `go vet`, `gofmt`, `go test ./...` all green.
`govulncheck ./...`: 0 vulnerabilities affecting the code (1 in a required module, never called).

## Status summary

| Status | Count | Meaning |
|--------|-------|---------|
| still-present | 69 | defect exists on main as described |
| partial | 29 | part fixed on main, remainder detailed below |
| fixed | 56 | remediated on main since the stale branch |
| obsolete | 6 | the code/file no longer exists |

Remaining work (still-present + partial) by severity: **critical 3, high 26, medium 37, low 32**.

---


## Still present on main

### [CRITICAL/bug] internal/ui/installation.go:387 — installUtilities can delete the currently running binary and then fail the copy (self-destruct)

installUtilities resolves the running executable with os.Executable()+filepath.EvalSymlinks (lines 372-381), then unconditionally does os.Remove(destPath) at line 387 before copyFile(execPath, destPath) at line 388. There is no guard for execPath == destPath. destPath is ~/.local/bin/dotfiles, which is exactly where this function installs the binary — so on any run after the first install, if the user launches the copy in ~/.local/bin (which installUtilities itself put on their system, and which PATH commonly prefers), execPath equals destPath. os.Remove unlinks the running binary, and the subsequent os.Open(src) in copyFile fails with ENOENT because the path was just unlinked. Result: the install step errors out AND the user's dotfiles binary has been deleted from disk.

**Current location:** internal/ui/installation.go:497

**Verifier note:** installUtilities (now at line 459) resolves os.Executable()+EvalSymlinks (lines 482-491), then unconditionally removes ~/.local/bin/dotfiles and copies. If the running binary IS ~/.local/bin/dotfiles, os.Remove unlinks it and copyFile then fails opening the just-deleted source path, leaving the binary deleted. No guard was added on main.


### [HIGH/release] .golangci.yml:20 — Lint config is incompatible with current golangci-lint; combined with CI continue-on-error, linting is silently dead

The config is golangci-lint v1 format (no 'version: "2"' key) and enables 'exportloopref' (line 20), which was deprecated in v1.60 and removed in v1.62+/v2; 'gosimple' and 'stylecheck' (lines 9, 41) were also merged into staticcheck in v2. CI (.github/workflows/ci.yml:35-39) installs 'version: latest' — a v2.x binary — which refuses to load this config, so the lint step fails every run; but the step has continue-on-error: true, so the failure is invisible. 'make lint' (Makefile:74-76) and the pre-commit hook's golangci-lint step (scripts/install-hooks.sh:71-79) hit the same config error on any current install, and both treat it as non-blocking, so none of the 30 configured linters (including gosec) ever actually run anywhere.

**Current location:** .golangci.yml:20

**Verifier note:** Only change since the finding: CI switched from 'version: latest' to pinned v2.5.0 with a comment calling the step advisory. That does not fix anything — v2.5.0 still rejects the v1 config, and continue-on-error keeps the dead lint silent, so none of the ~30 configured linters (including gosec) run anywhere. Fix requires migrating the config (golangci-lint migrate) and removing continue-on-error.


### [HIGH/bug] bin/dotfiles-setup:771 — --list-backups prints only the first backup then exits 1 (((count++)) under set -e)

list_backups (called at top level from line 891 with errexit active) increments `((count++))` at line 771. With count=0, the first increment returns status 1 and errexit kills the script after printing only the first backup entry. Verified: `bash -c 'set -euo pipefail; f(){ local count=0; for x in a b c; do echo $x; ((count++)); done; }; f'` prints only 'a' and exits 1. Same bug duplicated at line 3158.

**Current location:** bin/dotfiles-setup:801 (duplicate at 3305)

**Verifier note:** Remediation only fixed the adjacent grep -c exit-status issue (the `|| true` comment at lines 798/3302); the ((count++)) errexit bug itself was not fixed. Fix: use `count=$((count+1))` or `((count++)) || true`.


### [HIGH/bug] bin/dotfiles-setup:862 — Raspberry Pi model auto-detection is lost because RASPI_MODEL is set inside a command-substitution subshell

detect_os assigns RASPI_MODEL (lines 862-871), but it is invoked as `OS=$(detect_os)` at line 886, so the assignment happens in a subshell and never reaches the parent. RASPI_MODEL therefore stays "" for any auto-detected Pi. Additionally, when `--raspi` is passed, line 883 sets OS=raspi and detect_os is never called at all, so the flag's documented behavior ('auto-detect model', help line 503) never happens. All downstream model checks (lines 902, 1035, 1052, 1068) see an empty model.

**Current location:** bin/dotfiles-setup:958 (subshell call); 924-952 (detect_os); 955-956 (--raspi override)

**Verifier note:** Downstream RASPI_MODEL checks moved to lines 974, 1107, 1113, 1124, 1140 but the root cause is identical to the original finding.


### [HIGH/bug] bin/dotfiles-setup:1632 — setup_kde_shortcuts aborts the entire installation under set -e on KDE systems without kwriteconfig

The script runs under `set -euo pipefail` (line 9, re-asserted at line 606). `setup_kde_shortcuts` does `return 1` at line 1632 when neither kwriteconfig6 nor kwriteconfig5 exists, and the `$kwrite ... 2>/dev/null` invocations at lines 1636-1637 and 1641-1642 have no `|| true` guard (unlike every gsettings/xfconf/qdbus call nearby). A non-zero status from either path propagates through the case branch in setup_de_shortcuts (line 1599) to the plain call in main() (line 3450), and errexit terminates the whole script.

**Current location:** bin/dotfiles-setup:1718

**Verifier note:** Code moved slightly (function now at lines 1707-1738, call site at 3670) but is byte-for-byte the same defect described in the finding.


### [HIGH/release] bin/dotfiles-setup:2280 — setup_utilities overwrites ~/.local/bin/dotfiles with a legacy bash CLI, clobbering/shadowing the Go binary of the same name

Line 2280 writes a 750-line bash CLI to ~/.local/bin/dotfiles — the exact name of the project's primary Go binary (bin/dotfiles, brew formula 'dotfiles'). The generated ~/.zshrc (line 1235) prepends ~/.local/bin to PATH, so on any system where the Go binary is not later in a prepended brew path (all Linux installs via pacman/apt/manual copy), the crippled legacy CLI shadows the Go TUI. Worse, if the user keeps the Go binary in ~/.local/bin itself, `cat >` at line 2280 replaces it outright (backup at line 1910 mitigates only when backups are enabled). The embedded CLI's dispatch (theme/nav/keyboard/list-themes/backups/restore/status/user/users) has no `hotkeys`, `install`, `manage`, or `update` subcommands, so the hk wrapper (lines 1929/2070) and all documented Go CLI commands stop working.

**Current location:** bin/dotfiles-setup:2366

**Verifier note:** Defect unchanged, only shifted lines (2280 -> 2366). The embedded ~750-line bash CLI still clobbers/shadows the Go `dotfiles` binary; its dispatch has no hotkeys/install/manage/update subcommands (no `hotkeys` case exists between lines 2366-3567). Mitigation is only backup_file at line 1996, same as before.


### [HIGH/bug] bin/dotfiles-setup:2846 — Generated dotfiles CLI calls sed_i, which is never defined inside the heredoc

The CLI script written to ~/.local/bin/dotfiles via the quoted heredoc (lines 2280-3377) calls sed_i in update_tmux (lines 2846-2850) and update_fzf (lines 2882-2885). sed_i() is only defined in dotfiles-setup itself at line 640, outside the heredoc, so it does not exist in the generated script. Every 'dotfiles theme <name>' invocation prints 'sed_i: command not found' repeatedly. Because the delete passes never run, update_tmux's 'cat >> ~/.tmux.conf' appends a new 15-line theme block on every theme switch without removing the old one, so ~/.tmux.conf accumulates conflicting duplicate theme blocks indefinitely. update_fzf becomes a complete no-op (it only uses sed), so fzf colors are never updated after initial setup.

**Current location:** bin/dotfiles-setup:2963

**Verifier note:** Defect unchanged, only shifted ~117 lines. Generated ~/.local/bin/dotfiles still emits "sed_i: command not found" on theme switch; update_tmux appends duplicate theme blocks to ~/.tmux.conf without deleting old ones, and update_fzf remains a no-op. Note the legacy bash script is deprecated in favor of the Go TUI, but it still ships in bin/.


### [HIGH/security] bin/dotfiles-setup.ps1:85 — Invoke-RestMethod get.scoop.sh | Invoke-Expression

Remote script piped to execution; scheme-less URI resolves to http:// (MITM-injectable).

**Current location:** bin/dotfiles-setup.ps1:85

**Verifier note:** Fix would be `Invoke-RestMethod https://get.scoop.sh | Invoke-Expression` at minimum (explicit HTTPS), ideally downloading to a file and verifying before execution.


### [HIGH/bug] bin/dotfiles-setup.ps1:175 — $PROFILE clobbered with no backup

Set-Content -Force unconditionally overwrites existing PowerShell profile; no backup.

**Current location:** bin/dotfiles-setup.ps1:175

**Verifier note:** Defect unchanged on main: any existing PowerShell profile is silently clobbered.


### [HIGH/bug] bin/dotfiles-setup.ps1:188 — Windows Terminal scheme dead code + false success

$catppuccinScheme built, never written to settings.json; prints Windows Terminal configured.

**Current location:** bin/dotfiles-setup.ps1:188-213

**Verifier note:** Defect unchanged on main. The only mitigation is the pre-existing Write-Warn at line 213 telling the user to set the scheme manually, but the scheme hash remains dead code and the success message is still false.


### [HIGH/bug] bin/dotfiles-setup.ps1:221 — git clone assumed without git installed/checked

Git.Git not in winget list, no Get-Command git guard; failure still prints success.

**Current location:** bin/dotfiles-setup.ps1:221

**Verifier note:** Defect unchanged on main: git is never installed or checked, and clone failure still prints success.


### [HIGH/release] cmd/dotfiles/main.go:22 — Version constants inconsistent with release branch/tags: binary will report 2.0.1 for a v2.1.2 release

cmd/dotfiles/main.go:22 hardcodes `version = "2.0.1"` and Makefile:10 sets `VERSION = 2.0.1` (used via -X main.version). The current branch is fix/v2.1.2 and the newest tag is v2.0.2, so both the default and Makefile-injected versions are two releases behind. Additionally the Homebrew tap formula must pass the same ldflags or `dotfiles version` reports the stale constant; the in-repo Formula/dotfiles-setup.rb does not build Go at all. Related: rootCmd.Version is never set, so `dotfiles --version` errors with 'unknown flag' even though main.go:320 special-cases the string "version" as a reserved flag name, and bin/dotfiles-setup declares VERSION="1.0.1" (line 11) while its embedded CLI says 1.1.0 (line 2287).

**Verifier note:** Nothing in the finding has been remediated on main: hardcoded 2.0.1 in both main.go and Makefile remains behind the v2.0.2 tag/v2.1.2 release intent; --version flag still unsupported; bash script version mismatch (1.0.1 vs 1.1.0, now at line 2373) persists.


### [HIGH/bug] internal/tools/neovim.go:151 — setupNeovimPreset can destroy the user's Neovim config and its only backup

When ~/.config/nvim exists but has no init.lua (e.g., a legacy init.vim setup), lines 149-153 run `os.RemoveAll(nvimDir + ".backup")` — permanently deleting any previous backup — then `os.Rename(nvimDir, backupDir)` with the error ignored. If the subsequent `git clone` (line 156) fails (offline, git missing, GitHub down), the function returns an error with ~/.config/nvim gone; nothing restores the rename and nothing tells the user their config now lives in nvim.backup. A second attempt in the same broken state that gets past the rename again would have already wiped that .backup. Additionally, init.vim users get their entire working config silently displaced by kickstart because only init.lua is checked at line 142-146.

**Current location:** internal/tools/neovim.go:186-197

**Verifier note:** Defect unchanged from the audited branch: no backup rotation safety, no rollback on clone failure, no user notification of the .backup location, and prior backups are permanently deleted before the risky operation.


### [HIGH/bug] internal/tools/tmux.go:136 — Percent-style tmux split bindings are bound and then immediately unbound

In GenerateTmuxConfig, when cfg.SplitBinds != "pipes" (the UI toggles between "pipes" and "percent", internal/ui/input_deepdive.go:152-155), the else branch emits `bind % split-window -h ...` and `bind '"' split-window -v ...` (lines 133-134), but lines 136-137 then unconditionally emit `unbind '"'` and `unbind %`. tmux processes the config top-to-bottom, so the unbind lines remove the bindings that were just created (and the built-in defaults). The unbinds were only meant for the "pipes" branch.

**Current location:** internal/tools/tmux.go:143

**Verifier note:** Code is identical to the finding: the unbind lines run for both branches instead of only the "pipes" branch, so percent-style split bindings are immediately unbound.


### [HIGH/release] internal/tools/zsh.go:147 — Default "p10k" prompt style generates a .zshrc that never loads Powerlevel10k, and nothing installs it

The p10k case (lines 147-152) writes the instant-prompt cache block and `[[ -f ~/.p10k.zsh ]] && source ~/.p10k.zsh`, but never sources powerlevel10k.zsh-theme, and the zsh package list (lines 39-41) does not include powerlevel10k on any platform (nor does internal/ui/installation.go install it). The legacy bash installer does both (bin/dotfiles-setup:945 installs it, :1225-1228 sources the theme), so this is a regression in the Go path. "p10k" is the default everywhere (internal/config/defaults.go:25, internal/ui/deepdive.go:158).

**Current location:** internal/tools/zsh.go:161

**Verifier note:** Unchanged on main: the default p10k prompt style produces a .zshrc that never loads Powerlevel10k, and the theme is neither installed nor sourced anywhere in the Go codebase.


### [HIGH/bug] internal/ui/hotkeys_dualpane.go:408 — Alias input cannot accept 'h', 'l', or space characters

In handleHotkeysAliasInput, the cases `"left", "h"` (line 408) and `"right", "l"` (line 414) intercept the letters h and l as cursor-movement keys before the default rune-insertion branch. Additionally, the default branch (line 439) only inserts when `msg.Type == tea.KeyRunes`, but bubbletea v1.3.10 reports a lone space as `Type: KeySpace` (key.go:699-701 in the vendored module), so spaces are silently dropped too.

**Current location:** internal/ui/screen_hotkeys.go:305-339 (handleAliasInput; file renamed from hotkeys_dualpane.go)

**Verifier note:** Logic moved verbatim into hotkeysScreen.handleAliasInput; no remediation applied. Aliases still cannot contain 'h', 'l', or spaces.


### [HIGH/release] internal/ui/screens_deepdive.go:499 — Zsh 'Plugins' checkbox section is dead UI - selections are never applied

renderConfigZsh (screens_deepdive.go:499-523) offers five plugin checkboxes (zsh-autosuggestions, zsh-syntax-highlighting, zsh-completions, fzf-tab, zsh-history-substring-search) stored in cfg.ZshPlugins. installation.go:209 copies this into tools.ZshConfig.Plugins, but internal/tools/zsh.go never reads the Plugins field: the generated .zshrc only honors the separate SyntaxHighlight and Autosuggestions booleans (zsh.go:91,101). So zsh-completions, fzf-tab, and zsh-history-substring-search are silently ignored, and syntax-highlighting/autosuggestions are confusingly exposed twice on the same screen (Shell Options checkboxes at lines 488/494 vs Plugins checkboxes) where only the Shell Options pair has any effect.

**Current location:** internal/ui/screen_config_zsh.go:126-142 (Plugins checkboxes); internal/tools/zsh.go:104-127 (generator)

**Verifier note:** Finding's cited file screens_deepdive.go was split into internal/ui/screen_config_zsh.go, but behavior is unchanged: five plugin checkboxes (incl. zsh-completions, fzf-tab, zsh-history-substring-search) are dead UI, and syntax-highlighting/autosuggestions remain doubly exposed with only the Shell Options booleans taking effect.


### [HIGH/bug] internal/ui/screens_deepdive.go:807 — macOS Apps screen offers 6 apps that do not exist in the tool registry (list drift)

renderConfigMacApps (screens_deepdive.go:807-822) and its keyboard handler (input_deepdive.go:411) hardcode 10 app IDs: rectangle, raycast, stats, alt-tab, monitor-control, mos, karabiner, iina, the-unarchiver, appcleaner. The registry (internal/tools/registry.go:84-91, internal/tools/apps.go) only registers 4 of these (rectangle, raycast, iina, appcleaner). The installer resolves selections via registry lookup (installation.go:66-70) and skips unknown IDs with a log-only warning. The registry already carries UIGroupMacApps metadata (used for MacApps defaults via buildToolGroupDefaults, deepdive.go:217) precisely so these lists would not be hand-maintained — the render and input lists were copy-pasted from the legacy bash script (bin/dotfiles-setup:973-1004, same 10 casks) and drifted from the Go registry.

**Current location:** internal/ui/screen_config_macapps.go:19

**Verifier note:** The screen was refactored from renderConfigMacApps in screens_deepdive.go into a ScreenHandler (screen_config_macapps.go), consolidating render+input into one macAppItems list, but the list was not reconciled with the registry — the 6 unregistered app IDs remain selectable and will be silently skipped at install time.


### [MEDIUM/release] Formula/dotfiles-setup.rb:7 — Stale in-repo Homebrew formula with placeholder sha256 contradicts documented distribution

Formula/dotfiles-setup.rb points at the v1.0.0 tarball (line 7), has `sha256 "REPLACE_WITH_ACTUAL_SHA256"` (line 8), installs only the legacy bash script, and its caveats describe the old Catppuccin-only workflow. CLAUDE.md/README state the real formula lives in the separate tekierz/homebrew-tap repo and installs the Go `dotfiles` binary. Shipping this dead formula in an initial public release is confusing and non-functional (brew would reject the placeholder checksum).

**Current location:** Formula/dotfiles-setup.rb:7-8

**Verifier note:** The stale in-repo formula was never removed despite CLAUDE.md documenting distribution via the separate tekierz/homebrew-tap repo installing the Go binary. Fix is trivial: delete the Formula/ directory.


### [MEDIUM/release] bin/dotfiles:1 — Compiled ELF binary tracked in git, ships stale

bin/dotfiles is checked in; reports old version vs current source.

**Current location:** bin/dotfiles:1

**Verifier note:** Fix would be `git rm --cached bin/dotfiles` plus adding it to .gitignore; neither remediation cycle did this.


### [MEDIUM/duplication] bin/dotfiles-setup:756 — Backup/restore and theme-color logic duplicated wholesale later in the same script, with the same bugs

list_backups (756-778) and restore_backup (782-849) are re-implemented near-identically at ~3141-3165 and ~3190-3229 ('BACKUP & RESTORE (for CLI)' section), including the same errexit-fatal `((count++))`/`((restored++))` bugs (lines 3158, 3207-3219). Likewise load_theme_colors (34-278) is duplicated as a compressed theme table at ~2370-2450, and generate_ghostty_theme/generate_yazi_theme/generate_bat_config (313-480) are duplicated as update_* functions (~2814, ~2893, ~2965). Because both copies are top-level definitions in one file, whichever was parsed last wins at call time, making behavior order-dependent and guaranteeing fixes get applied to only one copy (as the counter bug demonstrates).

**Current location:** bin/dotfiles-setup:784 vs 3287 (list_backups), 839 vs 3342 (restore_backup), 34 vs 2423 (load_theme_colors)

**Verifier note:** The wholesale duplication described in the finding remains on main, shifted by ~30-150 lines. The ((restored++)) instances appear to have been removed, but ((count++)) survives in both duplicate list_backups implementations, so the last-definition-wins/order-dependent behavior and the demonstrated same-bug-in-both-copies problem persist.


### [MEDIUM/bug] bin/dotfiles-setup:953 — All package installs discard stderr and force success, then report 'Package installation complete'

Every package-manager invocation is suffixed with `2>/dev/null || true` (brew: 928-953, 958, 973-1007; apt: 1019-1032, 1042-1044, 1140-1155; pacman: 1097-1121; paru: 1129-1134; cargo/git installs: 1054-1090). Any failure — network down, sudo denied, package renamed, disk full — is invisible, and line 1162 unconditionally prints 'Package installation complete'. The rest of the script then writes zsh/tmux/yazi configs referencing tools that were never installed.

**Verifier note:** The legacy bash script on main still suppresses stderr and forces success on every package-manager invocation (brew, pacman, apt, git clones) and then unconditionally reports "Package installation complete". Line numbers shifted (~+70) but the defect is unchanged.


### [MEDIUM/release] bin/dotfiles-setup:1845 — setup_git replaces ~/.gitconfig wholesale, discarding user.name/user.email and all existing git settings

`cat > ~/.gitconfig` at line 1845 overwrites the user's entire git configuration to install delta/theme settings and aliases. Identity ([user]), signing keys, credential helpers, includes, and custom aliases are all destroyed; the script only prints 'Remember to set your git user.name and user.email!' (line 1879). The per-file backup at line 1839 runs only when a backup session exists — with --no-backup (SKIP_BACKUP), the loss is unrecoverable.

**Current location:** bin/dotfiles-setup:1931

**Verifier note:** Code is byte-for-byte the same defect as reported, just shifted from line 1845 to 1931. With --no-backup the loss remains unrecoverable.


### [MEDIUM/duplication] bin/dotfiles-setup:2290 — Embedded CLI duplicates the theme system and has already drifted: 13 themes vs the Go app's 16

The generated ~/.local/bin/dotfiles CLI re-declares SUPPORTED_THEMES with 13 themes (line 2290) and carries a full copy of load_theme_colors (starting line 2337, comment at 2338 admits 'same as in dotfiles-setup'), duplicating the ~300-line theme color table defined earlier in this same script. The duplication has already produced drift: CLAUDE.md and the Go app advertise 16 themes, while the legacy CLI and both hk hotkey tables (lines 2041 and 2182: 'Show all 13 available themes') say 13. Users switching to one of the 3 missing themes via the legacy CLI get rejected even though the setup script/Go app support them.

**Current location:** bin/dotfiles-setup:2376

**Verifier note:** Defect intact on main, only line numbers shifted (2290->2376, 2337->2423, hk tables 2041/2182->2126/2267). Drift is now 14 bash themes vs 16 Go themes, with stale "13" text; users cannot select one-dark or neon-seapunk via the legacy CLI.


### [MEDIUM/duplication] bin/dotfiles-setup:2337 — load_theme_colors and safe_read_setting are duplicated verbatim between outer script and generated CLI

load_theme_colors is defined twice: bin/dotfiles-setup:34 (outer script, ~230 lines) and bin/dotfiles-setup:2337 (inside the DOTFILES_CLI_EOF heredoc, ~117 lines). safe_read_setting is likewise duplicated at lines 294 and 2301, and the SUPPORTED_THEMES list at lines 26 and 2290. The comment at line 2336 even admits 'same as in dotfiles-setup'. The copies have already diverged (frappe/macchiato exist only in the outer copy), demonstrating the maintenance hazard.

**Current location:** bin/dotfiles-setup:2423 (heredoc copy of load_theme_colors)

**Verifier note:** Duplication of all three items persists in the legacy bash script. Minor improvement: the two SUPPORTED_THEMES lists are currently in sync (frappe/macchiato now in both copies), so the previously observed divergence is gone, but the structural duplication hazard remains unchanged.


### [MEDIUM/security] bin/dotfiles-setup.ps1:84 — Execution policy silently weakened

Set-ExecutionPolicy RemoteSigned -Force without disclosure; never restored.

**Current location:** bin/dotfiles-setup.ps1:84

**Verifier note:** Unchanged from the audited branch. Mitigating factor: it is scoped to CurrentUser and only runs when Scoop is absent, but the policy change is still silent and permanent.


### [MEDIUM/bug] bin/dotfiles-setup.ps1:107 — winget/scoop install failures suppressed

stderr to $null, no $LASTEXITCODE check, unconditional Packages installed.

**Current location:** bin/dotfiles-setup.ps1:107

**Verifier note:** Code is unchanged from the audited version: stderr suppressed on winget/scoop installs (also scoop bucket adds at lines 112-113), no exit-code checking anywhere, and success is reported unconditionally.


### [MEDIUM/security] bin/dotfiles-setup.ps1:231 — Unverified remote sshh.ps1 downloaded and aliased

No checksum, no download error check; alias added even if download failed.

**Current location:** bin/dotfiles-setup.ps1:230

**Verifier note:** Code is byte-identical to the finding's description on current main; no remediation was applied to the PowerShell script.


### [MEDIUM/bug] cmd/dotfiles/main.go:764 — Uninstall binary list wrong: leaves sshh installed, deletes unrelated user binary `y`

Install writes utility scripts hk, caff, sshh to ~/.local/bin (internal/ui/installation.go:397-410 via scripts.GetScript, which only knows hk/caff/sshh; the install-status cache also checks exactly hk/caff/sshh at internal/ui/cache.go:74). But runUninstall's removal list (main.go:764-771) is {dotfiles, dotfiles-tui, dotfiles-setup, hk, caff, y}: `sshh` is missing so it is left behind, and `y` is never installed as a binary by this app (it exists only as a zsh function written by the legacy script, dotfiles-setup:1271). The command's own help text (main.go:257) says it removes 'hk, caff, sshh'.

**Current location:** cmd/dotfiles/main.go:774

**Verifier note:** Defect unchanged on main: uninstall leaves sshh in ~/.local/bin and /usr/local/bin while deleting an unrelated binary named "y" if the user has one.


### [MEDIUM/bug] internal/config/hotkeys.go:27 — Hotkeys config ignores XDG_CONFIG_HOME, diverging from ConfigDir()

LoadHotkeysConfig (hotkeys.go:23-27) and SaveHotkeysConfig (hotkeys.go:49-57) hand-build the path os.UserHomeDir() + ".config/dotfiles/hotkeys.json" instead of using ConfigDir() from config.go:36-50, which honors XDG_CONFIG_HOME first. Every other config file (global.json, tools/*, users/*) follows XDG; hotkeys.json does not. This is also duplicated path/mkdir logic that config.go's ConfigDir()/EnsureDirs() already provide. Any backup, restore, or uninstall logic that operates on ConfigDir() will miss or orphan hotkeys.json for XDG users.

**Current location:** internal/config/hotkeys.go:29 (load) and :59 (save)

**Verifier note:** LoadHotkeysConfig and SaveHotkeysConfig are unchanged on main: both ignore XDG_CONFIG_HOME and duplicate the mkdir/path logic ConfigDir() provides, so XDG users' hotkeys.json still diverges from the rest of the config tree.


### [MEDIUM/bug] internal/pkg/pacman.go:170 — Update() installs from stale sync DB — reported updates silently don't apply

CheckOutdated detects updates via `checkupdates`, which downloads fresh sync databases into a private temp DB and does not touch /var/lib/pacman/sync. Update() then runs `pacman -S --noconfirm <pkg>` (line 170-177) against the real, un-refreshed sync DB. pacman resolves the old version, sees it already installed (`reinstalling`), exits 0 — and UpdatePackages (update.go:76-83) marks the update Success:true. The package remains outdated. (Note the fix is not simply `-Sy <pkg>`, which creates a partial-upgrade risk on Arch; per-package it should effectively be `-Syu <pkg>` or documented as full-upgrade only.)

**Current location:** internal/pkg/pacman.go:196 (Update) and internal/pkg/pacman.go:345 (UpdateStreaming)

**Verifier note:** Defect unchanged from finding: updates reported by checkupdates are installed against the un-refreshed /var/lib/pacman/sync DB, so pacman resolves the already-installed version, reinstalls, exits 0, and the update is falsely marked successful. UpdateAll/UpdateAllStreaming (-Syu) are fine, but the per-package update flow (Update selected packages) still hits the buggy path. Fix should be `-Syu <pkg>` semantics or restricting to full-upgrade, per the finding's note.


### [MEDIUM/bug] internal/pkg/update.go:24 — CheckAllUpdates swallows all per-manager errors — total failure looks like 'up to date'

The loop at update.go:24-31 skips any manager whose CheckOutdated fails, with a comment 'Log error but continue' — but nothing is logged. If the only manager errors (e.g. brew outdated fails, apt list fails), the function returns an empty slice and nil error, indistinguishable from a clean system.

**Current location:** internal/pkg/update.go:25

**Verifier note:** Code identical in behavior to the audited version; remediation cycles did not touch this error path.


### [MEDIUM/bug] internal/tools/apps.go:419 — Loose substring matching in app-detection helpers produces false 'installed' results

hasDesktopEntry (line 419), hasAppImage (line 495), and hasMacOSApp (line 528) all use case-insensitive strings.Contains against short generic patterns. Examples: CursorTool.IsInstalled calls hasDesktopEntry("cursor", ...) — on KDE Plasma systems /usr/share/applications contains kcm_cursortheme.desktop, so Cursor IDE is reported installed on every KDE desktop. hasDesktopEntry("zen") for Zen Browser matches any .desktop filename containing "zen" (e.g. org.gnome.Zenity), and hasAppImage("zen") matches any file in /opt containing both "zen" and "appimage". A false positive removes the tool from Registry.NotInstalled*/install lists, so the user cannot install it through the TUI.

**Current location:** internal/tools/apps.go:423

**Verifier note:** All three helpers on main still use case-insensitive strings.Contains against short generic patterns. hasDesktopEntry("cursor", ...) (called from CursorTool.IsInstalled at apps.go:106) still matches KDE's kcm_cursortheme.desktop; hasDesktopEntry("zen-browser","zen") still matches org.gnome.Zenity.desktop; hasAppImage("zen",...) still matches any /opt file containing "zen"+"appimage". The Exec= fallback (hasDesktopEntryExec, apps.go:444) was added for AppImage entries but does not tighten the primary filename substring check, so the false-positive behavior is unchanged.


### [MEDIUM/bug] internal/tools/fzf.go:127 — Generated fzf config file is never sourced — the entire fzf config screen is a no-op

WriteFzfConfig writes ~/.config/fzf/fzf.zsh, but nothing references that path anywhere else in the repo: the generated ~/.zshrc (internal/tools/zsh.go) never sources it (grep for 'config/fzf' and 'fzf' in zsh.go returns nothing), and configPaths is empty (fzf.go:38) so the file is also invisible to HasConfig()/backup/uninstall. FZF_DEFAULT_OPTS, height, layout, and preview settings chosen in ScreenConfigFzf therefore never take effect.

**Current location:** internal/tools/fzf.go:42 (empty configPaths) and internal/tools/fzf.go:134-146 (WriteFzfConfig writing an unsourced file)

**Verifier note:** Defect unchanged on main: the generated fzf.zsh is never sourced by the generated .zshrc and is invisible to backup/uninstall via configPaths, so ScreenConfigFzf settings still have no runtime effect. Tests assert the file's contents but not that it is ever loaded.


### [MEDIUM/bug] internal/tools/glow.go:98 — Glow config written to a path glow does not read on macOS, and not tracked in configPaths

WriteGlowConfig always writes ~/.config/glow/glow.yml. On macOS (the project's primary platform, installed via Homebrew), glow resolves its config with os.UserConfigDir(), i.e. ~/Library/Preferences/glow/glow.yml, unless XDG_CONFIG_HOME is set — so the written file is ignored. Additionally configPaths is empty (glow.go:38), so the file the tool writes is excluded from HasConfig()/backup/uninstall. Minor related dead logic: the Pager branches at glow.go:63-69 emit 'pager: true' for both 'less' and 'auto', making the three-way Pager option a two-way one.

**Current location:** internal/tools/glow.go:95

**Verifier note:** All three aspects of the finding are unchanged on main: macOS-ignored config path, empty configPaths excluding the file from HasConfig/backup/uninstall, and the two-way pager logic for a three-way option. Tests even encode the ~/.config path as expected behavior.


### [MEDIUM/bug] internal/tools/yazi.go:41 — Yazi theme.toml declared but never written; PreviewMode setting silently ignored

configPaths (yazi.go:38-42) lists three files including theme.toml, but WriteYaziConfig (yazi.go:152-179) only writes yazi.toml and keymap.toml — despite the tool's headline feature being '16 themes with unified colors', yazi gets no theme file and GenerateYaziConfig only embeds the theme name in a comment. Separately, YaziConfig.PreviewMode is collected from the deep-dive UI (installation.go passes a.deepDiveConfig.YaziPreviewMode) but is never read by GenerateYaziConfig, so that user choice does nothing.

**Current location:** internal/tools/yazi.go:46 (theme.toml in configPaths) and internal/tools/yazi.go:174-195 (WriteYaziConfig)

**Verifier note:** Both halves of the finding persist unchanged on main: no theme.toml is ever generated/written, and the PreviewMode deep-dive setting is collected but has no effect on the emitted [preview] section.


### [MEDIUM/duplication] internal/ui/cache.go:109 — ensureInstallCache duplicates loadInstallCacheCmd almost line-for-line (~55 lines)

cache.go:22-82 (loadInstallCacheCmd) and cache.go:109-171 (ensureInstallCache) contain the same algorithm copied twice: DetectManager, DetectPlatform, ListInstalled into a map, per-tool platform-package lookup with 'all' fallback and IsInstalled() fallback, then the hk/caff/sshh ~/.local/bin stat loop. The only difference is whether results go into a fresh map returned via installCacheDoneMsg or into a.manageInstalled. Any fix to the detection logic (e.g., checking all packages of a tool instead of only pkgs[0], or adding a new utility script) must now be made in two places; they will drift.

**Current location:** internal/ui/cache.go:154

**Verifier note:** Slightly mitigated since the finding: batch lookup and package-set matching were extracted into shared helpers (batchInstalledPackages, allPackagesInBatch), so some drift risk is reduced. But the ~45-line orchestration loop (platform resolution, per-tool fallback, utility-script stat loop) remains duplicated verbatim in both functions, so changes like adding a new utility script still require edits in two places.


### [MEDIUM/bug] internal/ui/deepdive.go:343 — CLI Utilities menu advertises tailscale, but the config screen's hardcoded list omits it (registry/UI drift)

NewDeepDiveConfig builds CLIUtilities from the registry via buildToolGroupDefaults (deepdive.go:233), which yields 8 tools including tailscale (internal/tools/tailscale.go is in UIGroupCLIUtilities). The menu item description at deepdive.go:343 explicitly lists 'bat, eza, zoxide, ripgrep, fd, tailscale'. But the actual config screen handler uses a hardcoded 7-item slice without tailscale (internal/ui/input_deepdive.go:503: {"bat","eza","zoxide","ripgrep","fd","delta","fswatch"}), and the struct comment at deepdive.go:102 also lists only those 7. So tailscale exists in the config map but can never be viewed or toggled from the UI, despite being advertised in the menu. state_helpers.go:163 (getConfigScreenMaxFields returns 7 for this screen) encodes yet another count (8 positions), so three different sources disagree.

**Current location:** internal/ui/screen_config_cliutilities.go:19

**Verifier note:** The screen logic was migrated from input_deepdive.go into a ScreenHandler (screen_config_cliutilities.go), and the toggle list is now the shared cliUtilityItems slice, but the drift is unchanged: tailscale exists in DeepDiveConfig.CLIUtilities and the menu description yet is absent from the config screen. Struct comment at app.go:63 also still lists only the 7 items.


### [MEDIUM/duplication] internal/ui/hotkeys_dualpane.go:224 — Favorites-filtering block copy-pasted five times

The identical 'build displayItems by filtering allItems through isHotkeyFavorite' loop appears at lines 224-233 (handleHotkeysKey), 342-347 (the 'f' unfavorite re-filter), 577-586 (mouse wheel), 615-624 (mouse click), 663-672 (renderHotkeysDualPane), and 858-869 (renderHotkeysItemsPanel). Six near-identical copies of the same logic.

**Current location:** internal/ui/screen_hotkeys.go:108

**Verifier note:** File was renamed/consolidated from hotkeys_dualpane.go to screen_hotkeys.go, but the six near-identical favorites-filtering blocks survive intact; the render-panel copy is a slight variant that also tracks original indices.


### [MEDIUM/bug] internal/ui/hotkeys_dualpane.go:523 — Saved aliases are written to config but never consumed anywhere

hotkeysSaveAlias persists `userHotkeys.Aliases[name] = command` into hotkeys config JSON, but a repo-wide search shows nothing reads UserHotkeys.Aliases: the zsh generator (internal/tools/zsh.go) uses its own fixed map of predefined aliases, and no UI screen lists or renders saved aliases. The feature is a silent no-op beyond writing JSON.

**Current location:** internal/ui/screen_hotkeys.go:819

**Verifier note:** Code moved from hotkeys_dualpane.go to screen_hotkeys.go but the saved aliases remain a write-only no-op: nothing lists, renders, or generates shell config from UserHotkeys.Aliases.


### [MEDIUM/bug] internal/ui/manage_dualpane.go:97 — saveManageConfigCmd serializes a shared *ManageConfig from a background goroutine while the UI thread can still mutate it (data race)

The comment says "Capture by value (pointer is stable)" but `cfg := a.manageConfig` copies only the pointer. The returned tea.Cmd runs on a Bubble Tea worker goroutine and passes `cfg` to config.SaveToolConfig (JSON marshal reads every field), while the Update goroutine keeps writing to the very same struct through the field pointers held by manageField (e.g. `*f.n = ...` at line 274, `*f.b = !*f.b` at 285, `*a.manageEditField = ...` at 1545). Concurrent unsynchronized read/write of the same struct fields is a Go data race; besides tripping -race, the saved file can contain a torn mix of pre- and post-edit values.

**Current location:** internal/ui/manage_dualpane.go:95-113

**Verifier note:** Remediation added a value-copied baseline (a.manageConfigBaseline, app.go:136) for the changed-tools diff, but the config actually serialized is still the shared *ManageConfig pointer, so the data race / torn-write risk described in the finding remains.


### [MEDIUM/bug] internal/ui/screen_test.go:144 — mockScreenHandler.updateCalled and lastMsg are recorded but never asserted — message forwarding to screens is untested

grep confirms updateCalled/lastMsg are set in the mock (screen_test.go:159-160) but read by no test in the package. Consequently there is no test that ScreenManager.Update forwards a non-navigation tea.Msg (key press, tick, async result) to the current screen's Update, or that a handler returned from Update replaces the current screen. That forwarding is the ScreenManager's core job; every test in screen_manager_test.go only exercises Navigate*/View. Combined with TestScreenManager_Update_NavigateBackMsg (screen_manager_test.go:278-289), which asserts only `handled == true` and ignores the resulting state (it should verify the fall-back to legacy ScreenMainMenu), the manager's Update path is effectively unverified.

**Current location:** internal/ui/screen_test.go:141

**Verifier note:** The secondary point about TestScreenManager_Update_NavigateBackMsg is obsolete (that test and apparently NavigateBackMsg handling were removed), but the primary gap — message-forwarding and screen-replacement in ScreenManager.Update untested — remains.


### [MEDIUM/efficiency] internal/ui/screens.go:458 — renderFileTree performs blocking package-manager queries inside View()

renderFileTree calls a.ensureInstallCache() directly from the render path. When manageInstalledReady is false (e.g., the user reaches the Installation Summary before the async loadInstallCacheCmd finishes, or via a flow that never started it), ensureInstallCache (cache.go:109) synchronously runs mgr.ListInstalled() plus a per-tool t.IsInstalled() subprocess fallback for every tool not matched by the batch query. This blocks the UI thread for the duration of those subprocesses and mutates App state from View(), violating the project's own stated pattern (async loading with *Ready flags per CLAUDE.md 'Performance Considerations').

**Current location:** internal/ui/screen_filetree.go:80

**Verifier note:** The code moved from screens.go to screen_filetree.go, but the defect is unchanged: the Installation Summary View() still triggers synchronous package-manager queries and mutates App state from the render path. Same pattern also exists in View() of screen_config_clitools.go:73, screen_config_cliutilities.go:63, screen_config_utilities.go:59, screen_config_macapps.go:66, screen_config_guiapps.go:62. cache.go's own comment (lines 151-153) says View rendering should use startInstallCacheLoad() instead, but these View methods do not.


### [MEDIUM/efficiency] internal/ui/screens_deepdive.go:800 — Blocking package-manager subprocess calls inside View() via ensureInstallCache

renderConfigMacApps (line 800), renderConfigUtilities (1116), renderConfigCLITools (1184), renderConfigCLIUtilities (1255), and renderConfigGUIApps (1327) all call a.ensureInstallCache() from Bubble Tea's View path. ensureInstallCache (cache.go:109) synchronously runs mgr.ListInstalled() (e.g. `brew list --versions`) plus per-tool t.IsInstalled() fallbacks. cache.go:106-108 explicitly documents that this function is for synchronous contexts and that UI rendering must use startInstallCacheLoad(). The deep-dive menu key handler (input_deepdive.go:16-38) has no guard for installCacheLoading, so a user can enter these screens while the async load is still running, at which point the render blocks the entire event loop and duplicates the in-flight async work (both eventually write a.manageInstalled).

**Current location:** internal/ui/cache.go:154; internal/ui/screen_config_clitools.go:73 (and sibling screen_config_*.go View methods)

**Verifier note:** The monolithic screens_deepdive.go was split into per-screen files, but the blocking ensureInstallCache-in-View pattern and unguarded navigation while installCacheLoading persist unchanged; the deep-dive menu's loading spinner only affects its own View, not key handling.


### [MEDIUM/duplication] internal/ui/screens_deepdive.go:1114 — Five near-identical checkbox-group screens (~330 lines) should be one registry-driven helper; tailscale is missing from the CLI Utilities list as a result

renderConfigMacApps (798-870), renderConfigUtilities (1114-1179), renderConfigCLITools (1182-1250), renderConfigCLIUtilities (1253-1322), and renderConfigGUIApps (1325-1393) are byte-for-byte the same loop (cursor color, renderCheckboxInlineWithInstallState, yellow-installed styling, '(installed)' suffix, box+help) differing only in title, hardcoded item slice, index field, and config map. Concrete drift already exists: TailscaleTool is registered with UIGroupCLIUtilities (internal/tools/tailscale.go:29), so it appears in the CLIUtilities defaults map, but the hardcoded list at screens_deepdive.go:1262-1274 omits it — tailscale can never be selected in the Deep Dive installer.

**Current location:** internal/ui/screen_config_cliutilities.go:19-31

**Verifier note:** Navigation/toggle logic was deduplicated via configListNav, but lists are not registry-driven and the tailscale omission persists; deepdive.go's group description now even names tailscale, worsening the mismatch.


### [MEDIUM/security] internal/ui/screens_management.go:263 — Raw package-manager output rendered into the TUI without stripping ANSI/control sequences

installLogs lines come verbatim from brew/pacman/paru/apt stdout+stderr via runner.RunStreaming and are joined straight into the lipgloss layout (screens_management.go:262-268; same for a.installOutput in screens.go:705-711). These tools emit carriage returns, cursor-movement and color escapes (progress bars), which corrupt the alt-screen layout; worse, AUR package names/descriptions and .pkg output are third-party-controlled text, so arbitrary escape sequences (title set, clipboard OSC52, screen clear) flow to the user's terminal. A stripANSI helper already exists in internal/runner/bash.go:166 but is only used for line classification, never for display. truncateVisible on a line containing escapes can also cut a sequence in half.

**Current location:** internal/ui/manage_dualpane.go:1181 (also internal/ui/screen_progress.go:131,142)

**Verifier note:** Original file screens_management.go was deleted; log display moved to manage_dualpane.go but still joins raw package-manager output (brew/pacman/paru/apt, third-party-controlled text) into the lipgloss layout, and truncateVisible can still split escape sequences.


### [LOW/security] README.md:255 — README instructs users to curl | bash a 3,477-line installer from the main branch with no integrity check

Lines 253-262 recommend 'curl -fsSL .../main/bin/dotfiles-setup | bash' (three variants). Piping the moving 'main' ref straight to bash gives users no chance to inspect the script, no pinned version, and no checksum; a compromised branch or truncated download executes partially. Since the same README already offers Homebrew (checksummed, versioned) and git-clone paths, the curl|bash section is the weakest install path being advertised for the public release.

**Current location:** README.md:263

**Verifier note:** All three curl|bash variants remain, still piping the unpinned main branch to bash with no checksum or version pin. The only change is framing: they now sit under a "Legacy Bash Script" heading rather than being a primary install path, which slightly reduces prominence but does not remediate the integrity concern.


### [LOW/release] bin/dotfiles:1 — Compiled 64-bit ELF binary is tracked in git and will ship stale in the public repo

bin/dotfiles is a stripped x86-64 ELF executable committed to the repository (currently showing as modified in git status). It is a build artifact of cmd/dotfiles: it bloats every clone, goes stale relative to source between commits, is platform-specific (useless on macOS despite the cross-platform pitch), and `make clean` (Makefile:63-66) removes only INSTALLER_BIN, never DOTFILES_BIN, so it keeps getting re-committed. Shipping opaque binaries in a public source repo also invites supply-chain suspicion.

**Verifier note:** Core defect remains: the compiled Linux binary is still committed to the repo. One sub-point was fixed: `make clean` (Makefile:57-59) now removes DOTFILES_BIN (rm -f bin/dotfiles). But the binary is still tracked, not gitignored, and will keep being re-committed after builds.


### [LOW/security] bin/dotfiles-setup:1063 — curl | bash installers with errors silenced (zoxide, Homebrew)

Line 1063 pipes `curl -sS https://raw.githubusercontent.com/ajeetdsouza/zoxide/main/install.sh | bash 2>/dev/null` — remote code executed with all diagnostics hidden, fetched from a mutable branch (main) rather than a pinned tag/checksum. Line 918 does the same for Homebrew (an accepted upstream pattern, but worth noting for a security-conscious release). A partial download is mitigated by bash reading the full pipe, but there is no integrity check and no visibility if the script fails or is compromised.

**Current location:** bin/dotfiles-setup:1135

**Verifier note:** Defect unchanged on main; only line numbers moved (1063->1135, 918->990). Homebrew case remains the accepted upstream pattern; zoxide case still hides all diagnostics via 2>/dev/null.


### [LOW/bug] bin/dotfiles-setup:1192 — setup_zsh computes plugin path variables that are never used (dead, misleading code)

Lines 1191-1213 assign SYNTAX_HL_PATH, AUTOSUGG_PATH, FZF_KEYBIND_PATH, FZF_COMPLETION_PATH, and P10K_PATH (including an Intel-Mac fallback branch), but the ~/.zshrc heredoc that follows is quoted ('ZSHRC_EOF'), so none of these variables are ever expanded — the zshrc instead hardcodes every candidate path (lines 1282-1287, 1314-1325). The 23-line block is dead code and misleads: editing these variables (e.g., to support a new plugin location) has no effect on the generated config. They are also unscoped globals leaking out of the function.

**Current location:** bin/dotfiles-setup:1264

**Verifier note:** Dead code block merely shifted from ~1191 to 1264; identical content including Intel-Mac fallback branch and unscoped globals.


### [LOW/duplication] bin/dotfiles-setup:1438 — Identical clipboard shell snippet duplicated 5 times in the generated tmux.conf

The sh -c 'command -v pbcopy ... wl-copy ... xclip' clipboard-detection one-liner is repeated verbatim in five bindings (lines 1438-1439, 1440-1441, 1444-1445, 1446-1447, and the paste variant at 1450-1451). A fix to the detection logic (e.g., adding xsel support or fixing the XDG_SESSION_TYPE check) must be applied in five places.

**Current location:** bin/dotfiles-setup:1525

**Verifier note:** Same duplication as described (5 occurrences counting the paste variant), just shifted from ~line 1438 to 1524-1537. Low-severity duplication in the generated tmux.conf heredoc in the legacy bash script.


### [LOW/duplication] bin/dotfiles-setup:1917 — Two hk heredocs duplicate ~120 identical lines

The vim variant (lines 1917-2056) and emacs variant (lines 2058-2197) of the generated hk script are near-identical: the shebang/dispatch preamble and the EZA, ZOXIDE, FZF, GHOSTTY, NVIM, CAFF, SSHH, DOTFILES, and macOS APPS tables are byte-for-byte duplicates. Only the TMUX, ZSH, and YAZI tables differ. Any hotkey change must be applied twice; the '13 available themes' string (lines 2041/2182) already shows both copies drifting together from reality.

**Current location:** bin/dotfiles-setup:2003-2283

**Verifier note:** Defect merely shifted ~85 lines. The vim variant is written at line 2003 (`cat > ~/.local/bin/hk << 'HK_VIM_EOF'`) and the emacs variant at line 2144; shared EZA/ZOXIDE/FZF/GHOSTTY/NVIM/CAFF/SSHH/DOTFILES/macOS APPS tables remain byte-for-byte duplicated, with only TMUX/ZSH/YAZI tables differing. Low-severity duplication in the legacy script; the Go TUI (dotfiles hotkeys) is the preferred path per the heredoc's own dispatch preamble.


### [LOW/release] bin/dotfiles-setup.ps1:16 — ScriptVersion 1.0.0 stale; bash script has two conflicting VERSION values

ps1 says 1.0.0; bash has 1.0.1 (line 11) and 1.1.0 (line 2287).

**Current location:** bin/dotfiles-setup.ps1:16

**Verifier note:** Unchanged on main: ps1 still reports 1.0.0 while the bash script defines two conflicting VERSION values (1.0.1 at line 11, later reassigned to 1.1.0 at line 2373, which wins at runtime).


### [LOW/duplication] internal/pkg/apt_bench_test.go:44 — Benchmarks copy production parsing logic inline and have already drifted from apt.go

BenchmarkParseDpkgOutput/BenchmarkListInstalledMock (lines 23-69) and BenchmarkParseUpgradableOutput (lines 72-113) re-implement the parsing from AptManager.ListInstalled/CheckOutdated rather than calling it. They have already drifted: apt.go ListInstalled (apt.go:220-253) now does a dpkg-query batch version lookup that the benchmark does not measure, so the benchmark numbers describe code that no longer exists.

**Current location:** internal/pkg/apt_bench_test.go:23

**Verifier note:** File is unchanged from the audited version: BenchmarkParseDpkgOutput/BenchmarkListInstalledMock parse "dpkg --get-selections"-style output inline, and BenchmarkParseUpgradableOutput re-implements CheckOutdated's apt-list parsing. Production ListInstalled (apt.go:231) does a dpkg-query -W batch version lookup the benchmarks never measure, so the drift described in the finding remains.


### [LOW/bug] internal/pkg/brew.go:39 — Non-streaming Install/Uninstall/Update discard stderr, reducing failures to 'exit status 1'

brew.go Install (39-42), Uninstall, Update, UpdateAll — and the equivalents in pacman.go and apt.go — run cmd.Run() with Stdout/Stderr discarded, so the returned error is only the exit status with no output. Callers such as InstallPackage/UpdatePackages (update.go:126-141, 76-83) propagate this to the user as an opaque failure with no reason (missing formula, network error, conflict).

**Current location:** internal/pkg/brew.go:42

**Verifier note:** No CombinedOutput/stderr capture anywhere in the non-streaming Install/Uninstall/Update/UpdateAll paths of brew, pacman, or apt managers; failures still surface as opaque exit-status errors.


### [LOW/bug] internal/pkg/mock_manager.go:145 — Mock streaming methods return (nil, nil), nil-deref landmine for streaming-path tests

InstallStreaming/UpdateStreaming/UpdateAllStreaming (mock_manager.go:136-158) return a nil *runner.StreamingCmd with a nil error on success. Any UI test exercising the real install flow does `cmd, err := mgr.InstallStreaming(...); if err != nil {...}` then calls cmd.Wait()/reads cmd output — panicking with a nil pointer dereference instead of testing the flow.

**Current location:** internal/pkg/mock_manager.go:136-158

**Verifier note:** Code is unchanged from the finding: all three streaming mock methods still return a nil *runner.StreamingCmd with nil error on success, so any test that checks err then dereferences the returned command will panic. Low severity (test infrastructure only).


### [LOW/duplication] internal/scripts/scripts.go:21 — HKScript static hotkey table duplicates internal/hotkeys/hotkeys.go and has already drifted

hotkeys.go:37 declares Categories() "the single source of truth", but scripts.go:21-104 embeds a second, hand-maintained hotkey table in the hk fallback script. They already disagree: tmux pane resize is "Prefix + H/J/K/L" in hotkeys.go:69 but "Ctrl-a S-Arrow" in scripts.go:33; the hk table lists "Alt-1/2/3/4/5 Switch window" (scripts.go:30) which hotkeys.go omits entirely; yazi bindings differ substantially (scripts.go:47-59 vs hotkeys.go:113-124). One of these is wrong about the actual installed tmux/yazi config, so users get contradictory documentation depending on whether the Go binary is on PATH.

**Current location:** internal/scripts/scripts.go:21

**Verifier note:** The duplicated hand-maintained hotkey table in the hk fallback script remains on main with the same drift examples cited in the finding (tmux resize keys, Alt-window-switch, differing yazi bindings).


### [LOW/bug] internal/tools/fzf.go:102 — completion.zsh sourced without its own existence check

The generated snippet (fzf.go:102-104) checks only for /usr/share/fzf/key-bindings.zsh but then sources both key-bindings.zsh AND completion.zsh inside that branch. On distros/packages that ship key-bindings without completion (or vice versa), the unguarded source fails.

**Current location:** internal/tools/fzf.go:121

**Verifier evidence:** internal/tools/fzf.go:119-121 — the branch checks only `[[ -f /usr/share/fzf/key-bindings.zsh ]]` yet sources both key-bindings.zsh and completion.zsh inside it; completion.zsh still has no existence guard (the brew branch at lines 125-126 guards each file, the Linux branch does not).


### [LOW/bug] internal/tools/neovim.go:191 — Silent failure wiring custom options into preset init.lua

writeNeovimUserPrefs ignores the error from os.WriteFile when appending the `pcall(require, "custom.options")` line to init.lua (line 191, `_ = os.WriteFile(...)`). If the write fails (read-only init.lua, disk full), the function still returns nil and the installer reports Neovim configured, but every user preference (tab width, clipboard, wrap, etc.) written to lua/custom/options.lua is never loaded.

**Current location:** internal/tools/neovim.go:252

**Verifier note:** Code moved from ~line 191 to line 252 but the defect is unchanged: a failed append of the pcall(require, "custom.options") line to init.lua is silently ignored, so user prefs in lua/custom/options.lua would never load while the installer reports success.


### [LOW/efficiency] internal/tools/registry.go:134 — ensureCache holds the write lock across all IsInstalled subprocess scans; flatpak probed once per app ID

ensureCache (lines 134-146) takes cacheMu.Lock() and, while holding it, serially runs IsInstalled() for all ~30 tools — each of which may spawn subprocesses (package-manager queries, and apps.go isFlatpakInstalled at line 372 runs a separate `flatpak info` per candidate ID, up to 4 for Zen Browser alone) plus directory scans. Any concurrent caller of Installed()/InstalledCount()/isInstalledCached blocks for the entire multi-second scan. isFlatpakInstalled could run `flatpak list --app` once and match, and the per-tool results could be computed outside the lock and swapped in.

**Current location:** internal/tools/registry.go:128

**Verifier note:** Code is identical to the audited version: any concurrent caller of Installed()/isInstalledCached blocks on cacheMu for the entire multi-second scan, and flatpak is probed per app ID instead of one `flatpak list --app`.


### [LOW/bug] internal/tools/registry.go:333 — InstallAll/InstallByCategory abort on first failure and skip cache invalidation

InstallAll (lines 331-340) and InstallByCategory (lines 343-352) return on the first tool's Install error, so one unavailable package (e.g., an AUR package that fails to build) aborts every remaining install. Worse, the early return skips InvalidateCache(), so tools that did install before the failure are still reported as not-installed by the stale cache until process restart or manual refresh.

**Current location:** internal/tools/registry.go:301-322

**Verifier note:** Defect unchanged on main: one failing package aborts all remaining installs and leaves the install cache stale for tools installed before the failure. A minimal fix is to defer/always call InvalidateCache and collect errors instead of returning early.


### [LOW/efficiency] internal/ui/app.go:392 — loadBackupsCmd walks every backup directory tree twice

For each backup entry, countBackupFiles(path) (line 392) and calcDirSize(path) (line 393) each perform a full filepath.Walk of the same tree, doubling filesystem traversal. Both helpers also discard the Walk error entirely. For bash-script backups containing copied directories (e.g. a full nvim config tree via backup_directory), this is hundreds of stat calls done twice per backup, on every entry to the Backups screen after invalidation.

**Current location:** internal/ui/app.go:559

**Verifier note:** Defect exists exactly as described on main: double tree traversal per backup entry and ignored Walk errors. Low severity, efficiency only. Trivial fix: merge into one walk returning (count, size).


### [LOW/bug] internal/ui/hotkeys_dualpane.go:391 — Enter in alias dialog silently discards partially-entered input

On "enter", if either hotkeysAliasName or hotkeysAliasCommand is empty the save is skipped, but hotkeysCancelAlias() runs unconditionally, wiping both fields and closing the dialog with no feedback.

**Current location:** internal/ui/screen_hotkeys.go:287

**Verifier note:** Code moved from hotkeys_dualpane.go (deleted) to screen_hotkeys.go handleAliasInput; the defect logic is unchanged.


### [LOW/duplication] internal/ui/hotkeys_dualpane.go:455 — Three copies of single-line rune editing (backspace/delete/insert/cursor) in the UI

hotkeysAliasBackspace/hotkeysAliasDelete/hotkeysAliasInsertRunes (hotkeys_dualpane.go:455-515) duplicate the identical rune-splice logic twice each (once per field, name vs command), and the whole editor (incl. key handling at 382-444) duplicates the manage inline editor's editing branch (manage_dualpane.go:163-229: same esc/enter/left/right/home/end/backspace/delete/KeyRunes handling on a string+cursor pair). screen_users.go:163-198 contains a third, cruder text-input implementation (byte-based backspace, custom charset check).

**Current location:** internal/ui/screen_hotkeys.go:751-811

**Verifier note:** Defect unchanged; files were renamed/refactored (hotkeys_dualpane.go and manage_dualpane.go editors now in screen_hotkeys.go and screen_manage.go; users input now in usersScreen.handleKey) but all three independent single-line rune/byte editing implementations remain, including the per-field duplication inside each hotkeys alias helper.


### [LOW/bug] internal/ui/hotkeys_dualpane.go:854 — itemIndices mapping is built every frame but never used

renderHotkeysItemsPanel allocates and fills itemIndices (lines 854-868, including the filteredIndices branch) to map display index -> original index, but no subsequent code reads it. Dead per-frame allocation and a sign that display-index vs original-index mapping was intended but abandoned.

**Current location:** internal/ui/screen_hotkeys.go:974

**Verifier note:** Defect moved verbatim into renderHotkeysItemsPanel in screen_hotkeys.go; dead per-frame allocation remains.


### [LOW/bug] internal/ui/installation.go:580 — streamingUpdateCmd assigns one aggregate error to every package's UpdateResult

The update runs as a single streaming command for all package names (line 565), then lines 579-587 build per-package results where Success and Error are copied from the single cmd.Wait() error. If the package manager exits non-zero because one package failed, every package in the batch is reported as failed (and vice versa: a manager that exits 0 despite a partial failure marks everything successful). The per-package result structure implies granularity that does not exist.

**Current location:** internal/ui/installation.go:748-755

**Verifier note:** Code moved from ~line 580 to streamingUpdateCmd at internal/ui/installation.go:703; the per-package result construction is unchanged in behavior. Low severity: only affects granularity of reported update results.


### [LOW/efficiency] internal/ui/manage_dualpane.go:1114 — renderManageDualPane rebuilds and re-sorts the full tool list, and mutates App state, on every View frame

manageItems() fetches the registry, allocates and stable-sorts all ~28 tools, applies platform filtering, and lazily initializes a.manageInstalled — and it is called from View (line 1114) as well as from every key (line 233) and mouse (line 516) event. With animations enabled the manage screen re-renders on every UI tick, so this allocation/sort churn runs continuously. View also mutates model state (a.manageIndex at 1118, scroll fields via manageEnsureToolsVisible/manageEnsureFieldsVisible at 1124-1129, and the manageInstalled map at 838), which belongs in Update per the Elm architecture and per this repo's own CLAUDE.md guidance to cache with *Ready flags.

**Current location:** internal/ui/screen_manage.go:675 (View) calling manageItems() defined at internal/ui/manage_dualpane.go:345

**Verifier note:** Code moved from renderManageDualPane in manage_dualpane.go to the View in screen_manage.go, but the per-frame rebuild/sort and state mutation in View are unchanged; no cache/*Ready flag added. Low-severity efficiency issue.


### [LOW/security] internal/ui/screens/error.go:64 — Error screen renders raw subprocess output without stripping ANSI/control escape sequences

ErrorScreen.View uses errMsg = s.err.Error() and renders it directly. The install error is constructed in app.go:836-837 as fmt.Errorf("%v\n\nOutput:\n%s", msg.err, msg.context), where context is captured stdout/stderr from bash/package-manager subprocesses. That output can legitimately contain ANSI escape sequences (many package managers emit color codes) and, in the worst case, cursor-movement/OSC/title-set sequences. Nothing sanitizes them before they hit the terminal, and lipgloss width/border math is also broken by embedded escapes, so the error box renders corrupted. The same applies to the legacy error renderer path fed by a.lastError.

**Current location:** internal/ui/screen_error.go:63

**Verifier note:** Code moved from internal/ui/screens/error.go to internal/ui/screen_error.go, but the defect is unchanged: raw package-manager stdout/stderr (which can contain ANSI color, cursor-movement, or OSC sequences) reaches the terminal unsanitized and breaks lipgloss width/border rendering.


### [LOW/bug] scripts/install-hooks.sh:9 — Hook installer assumes .git/hooks is a directory — fails in git worktrees and ignores core.hooksPath

Line 9 hardcodes HOOKS_DIR="$REPO_ROOT/.git/hooks". In a linked worktree, $REPO_ROOT/.git is a file (gitdir pointer), so 'cat > "$HOOKS_DIR/pre-commit"' at line 14 fails with 'Not a directory' and set -e aborts. If the user has core.hooksPath configured (increasingly common with hook managers), the script writes to a location git will never execute, printing 'installed successfully' while the hook is inert. Also minor: the '2>&1 > /dev/null' redirections at lines 63/73/84 suppress stdout (where go test failures print) while showing stderr — on test failure the developer sees only the generic retry hint.

**Verifier note:** Script is byte-for-byte identical to the audited version on main; none of the three issues (worktree gitdir file, core.hooksPath, stdout-suppressing redirection order) were remediated.



## Partially fixed on main

### [CRITICAL/bug] bin/dotfiles-setup:824 — restore_backup aborts after the first restored file due to ((var++)) under set -e, leaving configs half-restored

The script runs under `set -euo pipefail` (line 9). In restore_backup, lines 824/829 use `cp ... && ((restored++)) || ((errors++))` and lines 835/837 use `... && rm -rf "$original" && ((removed++))`. Bash's `((restored++))` returns exit status 1 when the pre-increment value is 0, so on the FIRST successful restore `((restored++))` fails, `((errors++))` then also runs (spuriously counting an error) and itself returns 1 because errors was 0. Since it is the last command of the &&/|| list, errexit fires and the whole script exits with status 1 mid-loop. Verified with `bash -c 'set -euo pipefail; restored=0; errors=0; true && ((restored++)) || ((errors++)); echo done'` which exits 1 before the echo. The summary at line 842 never prints. The same broken pattern is duplicated at lines 3207-3219.

**Current location:** bin/dotfiles-setup:879, 907, 909 (first copy) and 3380, 3406, 3408 (second copy)

**Verifier note:** The most common path (restoring pre-existing files) is fixed, but restore still aborts mid-loop on the first file that must be *removed* (existed=no branch, ((removed++)) with removed=0) or the first unsafe manifest path (((errors++)) with errors=0). Both duplicated restore_backup implementations (lines 839 and 3342) have the same residual issue.


### [CRITICAL/bug] cmd/dotfiles/main.go:804 — `dotfiles uninstall` silently no-op restores bash-format backups, then RemoveAll deletes all backups — unrecoverable data loss

runUninstall picks the newest dir under ~/.config/dotfiles/backups (main.go:738-751) and calls restoreBackup(), which only understands the Go flat format: `if entry.IsDir() { continue }` (main.go:645). Bash-created backups store files in nested trees (bin/dotfiles-setup:701-735), so everything under .config/ is a directory and is skipped — 'Restored 0 files' or only top-level dotfiles. Immediately afterwards runUninstall executes os.RemoveAll(configDir) (main.go:804), which deletes the backups directory itself. The original configs the user was promised ('Restore configuration files from backup') are permanently destroyed.

**Current location:** internal/backup/backup.go:229

**Verifier note:** Critical data-loss half fixed (backups preserved on failed/zero-file restore, restore also shares hardened backup.Restore with manifest support). Remaining gap is functional: legacy bash nested backups are not restored, but the user can now recover manually since the backup dir is kept.


### [HIGH/bug] bin/dotfiles-setup:2450 — Embedded 'dotfiles' bash CLI duplicates load_theme_colors and has already drifted: catppuccin-frappe/macchiato silently apply mocha colors

bin/dotfiles-setup contains TWO full copies of the theme color table: the outer script's load_theme_colors (lines 34-278, 13 themes) and the embedded ~/.local/bin/dotfiles CLI's load_theme_colors (lines 2337-2453, only 11 themes — catppuccin-frappe and catppuccin-macchiato cases are missing and fall through to the '*' default which recursively loads catppuccin-mocha, line 2450-2451). Meanwhile SUPPORTED_THEMES in the embedded CLI (line 2290) still lists frappe and macchiato, so validation passes. Additionally the Go app defines 16 themes (internal/config/config.go:260-277 adds everforest, one-dark, neon-seapunk), so any theme table consumer in bash silently mis-themes those three too.

**Current location:** bin/dotfiles-setup:26 and 2376 vs internal/config/config.go:222-223

**Verifier note:** Core bug fixed: frappe/macchiato no longer silently fall through to mocha, and unknown themes now fail loudly rather than mis-theming. What remains: (1) the theme color table is still duplicated in full (outer script lines 34-291 and embedded CLI lines 2423-2570), so future drift risk persists; (2) the Go app supports 16 themes while bash supports 14 — one-dark and neon-seapunk (internal/config/config.go:222-223) are absent from both bash SUPPORTED_THEMES lists and color tables, so selecting them in the Go TUI leaves the bash CLI unable to apply/validate them (now an explicit error, not silent mis-theming).


### [HIGH/bug] cmd/dotfiles/main.go:645 — Go restore silently drops most files from legacy bash-created backups; uninstall then deletes all backups

Both products share ~/.config/dotfiles/backups but use incompatible formats. The bash script (bin/dotfiles-setup:673-735) stores backups as a real nested directory tree (e.g. .config/nvim/...) plus a pipe-delimited manifest. Go restore (cmd/dotfiles/main.go:644-684 and internal/ui/app.go:471-502) assumes a flat layout with underscores encoding slashes, and skips every directory entry (`if entry.IsDir() ... continue`) without any warning. On a bash-format backup it restores only top-level dotfiles (.zshrc, .tmux.conf) and silently ignores everything under .config/. Worse, `runUninstall` (main.go:735-757) picks the lexicographically greatest backup name — bash names like `20260703_120000` sort above Go names like `2026-07-03_12-00-00` because '0' > '-', so the bash backup wins — 'restores' it near-emptily, prints success, then `os.RemoveAll(configDir)` (main.go:804) permanently deletes all

**Current location:** internal/backup/backup.go:52-84 (manifest format incompatible with bin/dotfiles-setup:705-757 pipe-delimited format)

**Verifier note:** The high-severity data-loss path (uninstall deleting all backups after a near-empty restore) is fixed, and failures are now loud. What remains is a functional gap: the Go restore still cannot restore legacy bash-created backups, but it now fails safely with per-file warnings and preserves the backup directory.


### [HIGH/bug] internal/pkg/apt.go:81 — CheckOutdated runs `sudo apt update` synchronously — hangs/corrupts the TUI when sudo prompts for a password

CheckOutdated (apt.go:81-82) executes `sudo apt update` with cmd.Stdin unset. sudo prompts on /dev/tty, not stdin, so under the Bubble Tea alt-screen the password prompt is drawn invisibly and sudo blocks waiting for input while the TUI also owns the tty. This function is reached from the update-check path (internal/ui/cache.go:15 -> pkg.CheckDotfilesUpdates -> CheckAllUpdates -> AptManager.CheckOutdated), which the UI treats as a passive read operation. The error is also ignored (`// Ignore errors, best effort`), so when sudo fails non-interactively the user silently gets stale upgradable lists. AptManager.Update (line 134-135) has the same silent `sudo apt update` swallow.

**Current location:** internal/pkg/apt.go:145-146

**Verifier note:** The high-severity TUI-hang scenario (sudo prompt during the passive update-check path) is resolved. What remains is the secondary point from the finding's detail: Update() still best-effort-swallows a `sudo apt update` failure, so installs can proceed against a stale index. Update() runs from interactive install flows, so the hang risk is much lower; residual severity is low.


### [HIGH/bug] internal/tools/git.go:129 — WriteGitConfig replaces ~/.gitconfig wholesale, dropping user identity and breaking git commit

WriteGitConfig (git.go:129-143) writes a generated file directly over ~/.gitconfig. GenerateGitConfig never emits a [user] section, so the user's existing user.name, user.email, user.signingkey, [include] directives, per-remote settings, and credential config are all destroyed. The auto-backup in internal/ui/installation.go:40-46 only runs 'if enabled'. The generated config also unconditionally sets core.pager=delta and core.editor=nvim (git.go:87-89) even when the user deselected those tools, and sets commit.gpgsign=true without a signingkey when SignCommits is on.

**Current location:** internal/tools/git.go:190

**Verifier note:** Primary high-severity defect (wholesale ~/.gitconfig replacement dropping user identity) remains; only the unconditional delta-pager sub-issue was remediated. Auto-backup is still conditional ("if enabled", tool.go:182-189), so it does not mitigate the data loss by default.


### [HIGH/bug] internal/ui/app.go:472 — restoreBackupCmd silently skips directory-structured backups (bash-script format) and reports success

restoreBackupCmd assumes the Go TUI's flat naming scheme (path separators replaced by underscores) and does `if entry.IsDir() ... continue` (line 472) then `strings.ReplaceAll(entry.Name(), "_", os.PathSeparator)` (line 478). But the legacy bash installer writes backups into the SAME ~/.config/dotfiles/backups/<timestamp> directory preserving real directory structure (bin/dotfiles-setup backup_file(): `backup_path="$BACKUP_SESSION_DIR/$relative_path"; mkdir -p $(dirname ...)`). The TUI Backups screen lists those sessions, but restore skips every nested file (.config/nvim/init.lua, .config/ghostty/config, .local/bin/*) and only restores top-level dotfiles, then reports 'Restored N files' as success. Additionally, the underscore-to-slash mapping is lossy: any backed-up filename containing a literal underscore is restored to a wrong, fabricated path (each '_' becomes '/').

**Verifier note:** Substantially remediated but not fully. Fixed parts: (1) Go-created backups now write a tab-separated manifest (relpath\tmode) that drives restore losslessly, so the underscore-to-slash lossy mapping no longer affects Go TUI backups (internal/backup/create.go, backup.go ReadManifest/Restore); (2) silent success is gone — per-file failures are recorded in RestoreResult.Skipped and surfaced by the TUI (internal/ui/app.go:646-671, restoreBackupCmd returns skipped count; comment C3 says all-skipped must not report green). Still broken for legacy bash backups: bin/dotfiles-setup writes backups preserving real directory structure with a manifest.txt in an incompatible format ("# comments" + "original_path|backup_path|existed" pipe lines, absolute paths). Go ReadManifest will parse those comment/pipe lines as bogus relPaths; every entry then fails (absolute paths rejected as traversal, encoded source names not found), so restoring a bash-format backup via the TUI restores zero nested files. It now reports failures/skips instead of false success, but bash-format backups remain effectively unrestorable through the TUI, and the manifest-less fallback keeps both the IsDir-skip and lossy underscore decode.


### [HIGH/bug] internal/ui/app.go:1145 — Global 'q' quit handler fires while user is typing in text-input modes, instantly exiting the TUI

handleKey() runs `if key == "q" && !a.installRunning && !(a.screen == ScreenManage && a.manageEditing) { return a, tea.Quit }` BEFORE delegating to screen handlers. The only text-input exemption is ScreenManage's manageEditing. Other input modes on the App struct — usersCreating (screen_users.go:163-198, which accepts any lowercase letter including 'q' into the username), hotkeysAddingAlias (hotkeys_dualpane.go:204, alias name/command entry), and backupConfirmMode — never receive the 'q' keypress because the global handler quits first.

**Current location:** internal/ui/screen_hotkeys.go:57

**Verifier note:** Fix is trivial: guard the hotkeys screen's 'q' quit with `!a.hotkeysAddingAlias`. Its own comment ("no inline edits apply here") is wrong — alias entry is an inline edit on this screen. backupConfirmMode is a y/n prompt (no free text), so 'q' quitting there (screen_backups.go:65) is arguably acceptable, though it bypasses the confirm cancel.


### [HIGH/bug] internal/ui/input_wizard.go:105 — Error screen 'Retry' does not restart the installation and leads to a false 'Installation Complete!' summary

In handleWizardKey, ScreenError case 'r' (input_wizard.go:105-107) only sets a.screen = ScreenProgress. It never emits installStartMsg or calls startInstallation(). By the time the error screen is shown, installDoneMsg has already set installRunning=false and installComplete=true (app.go:832-833). So the Progress screen shows stale state, nothing re-runs, and pressing Enter there (input_wizard.go:91-94, gated only on !installRunning) advances to ScreenSummary, which unconditionally renders '✓ Installation Complete!' (internal/ui/screens/summary.go:61) even though the install failed. The migrated handler internal/ui/screens/error.go:40-42 has the identical flaw: NavigateTo(ui.ScreenProgress) navigates without restarting the install (ScreenProgress is not migrated in factory.go, so it falls back to the same legacy state). The 's' (Skip) key on the error screen leads to the same false-succe

**Current location:** internal/ui/screen_summary.go:56 and internal/ui/screen_error.go:44

**Verifier note:** Remaining piece: the 's' (Skip) key on the error screen (screen_error.go:44) still navigates straight to ScreenSummary, and SummaryScreen.View unconditionally renders "✓ Installation Complete!" (screen_summary.go:53-56) with no reference to a.lastError. So after a failed install, choosing Skip still shows a false success banner. The main high-severity retry defect is fixed; only this skip-path false-success remains (lower severity, since Skip is an explicit user choice to continue).


### [HIGH/bug] internal/ui/manage_dualpane.go:426 — Settings keys and mouse clicks silently mutate hidden fields while the install log panel is displayed

renderManageSettingsPanel (line 1315) swaps the right pane to the log panel whenever `a.manageInstalling || len(a.installLogs) > 0`, but handleManageKey's settings-pane switch (lines 426-488) and handleManageMouse's inRightList branch (lines 565-624) have no corresponding guard. While the log panel is on screen, up/down move `configFieldIndex`, left/right/space adjust option/number fields, enter toggles booleans or opens the invisible inline text editor (which then captures ALL keystrokes into a config field), and clicks in the log area toggle/cycle whatever field happens to sit at that row. None of this is visible because the fields are replaced by the log view. Worse, the log panel footer (line 1727) says "C: clear • ↑↓: scroll", actively instructing the user to press the keys that corrupt hidden settings — actual log scrolling is only bound to pgup/pgdown/ctrl+u/ctrl+d (lines 373-393)

**Current location:** internal/ui/screen_manage.go:418-480 (settings-pane key switch), internal/ui/manage_dualpane.go:1199 (footer hint)

**Verifier note:** The mouse-click path was remediated on main (handlers moved from manage_dualpane.go to screen_manage.go), but the keyboard path and the misleading footer hint described in the finding remain unfixed.


### [HIGH/bug] internal/ui/screen_users.go:426 — Users screen mouse click selects the wrong user (off-by-two) and wrong settings field

handleUsersMouse computes userIdx := msg.Y - 5 and fieldIdx := msg.Y - 5, but the actual layout from renderUsersDualPane is: tab bar (1 line, Y=0), pane header (Y=1), divider (Y=2), first list item at Y=3. So the correct offset is 3, not 5. The right pane has the same off-by-two, and additionally each selected option field injects a description line (renderUsersSettingsPane lines 630-637), which shifts subsequent fields so a fixed offset can never be right. The offsets are also wrong in a different way while usersCreating/usersDeleting is active, since those headers occupy 4 lines instead of 2 (lines 491-514).

**Current location:** internal/ui/screen_users.go:589 (fixed offset applied even during usersCreating/usersDeleting transient headers at lines 713-727)

**Verifier note:** Main off-by-two and description-line issues are fixed. Residual: a left-pane click while the create-name or delete-confirm header is shown selects the user one row above the click (header is 3 rows in those modes vs the assumed 2). Low severity; mouse handling could simply be skipped while usersCreating/usersDeleting.


### [MEDIUM/release] .github/workflows/ci.yml:60 — The entire 'Security' CI job can never fail (govulncheck is continue-on-error), and staticcheck/race steps are also non-blocking

ci.yml:60 marks govulncheck continue-on-error — it is the only check in the Security job, so the job is green even if a known CVE is found in dependencies. staticcheck (line 43) and the race-detector test run (line 83) are likewise continue-on-error. The comments say 'may not support latest Go yet', but govulncheck@latest tracks Go releases closely; this blanket suppression turns three of the workflow's checks into decoration. Note also that gosec, which docs/security-scanning.md and the pre-PR checklist advertise as part of the security posture, is not run in CI at all.

**Current location:** .github/workflows/ci.yml:71

**Verifier note:** Remains: Security job still cannot fail — govulncheck (its sole check) is continue-on-error, though now pinned to v1.1.4 with a documented "flip to blocking after clean cycles" rationale; staticcheck (and golangci-lint) also remain advisory, with comments citing Go 1.25 tooling compatibility. Fixed: the race-detector run is now a blocking step, and the docs no longer advertise gosec as part of CI security posture (it runs only as a golangci-lint sub-linter, documented). Net: the deliberate suppression is now documented and narrower, but the Security job green-regardless-of-CVEs defect is still present.


### [MEDIUM/release] Makefile:10 — Hardcoded VERSION=2.0.1 is stale (tags v2.0.2 exists, branch targets v2.1.2) so released binaries misreport their version

Makefile:10 sets VERSION = 2.0.1 and injects it via -X main.version; cmd/dotfiles/main.go:22 also hardcodes version = "2.0.1" as the fallback. Git tags already include v2.0.2 and the current branch is fix/v2.1.2, so 'dotfiles version' from a fresh 'make build' reports 2.0.1 regardless of the actual release. The version must currently be bumped in two places manually. Additionally the 'help' target text (line 100: 'build - Build the installer binary') describes the old installer, and 'clean' (lines 63-66) does not remove the primary build output bin/dotfiles.

**Current location:** Makefile:9 and cmd/dotfiles/main.go:22

**Verifier note:** Core finding (stale hardcoded version in two places) still present on main; help/clean sub-issues resolved.


### [MEDIUM/security] bin/dotfiles-setup:2205 — Generated caff script uses a predictable world-writable /tmp pidfile

The caff utility written at lines 2202-2270 stores its PID in /tmp/caffeine-$USER.pid. On a multi-user machine, any other user can pre-create that predictable filename (or a symlink) in sticky /tmp before the victim first runs caff. `echo $! > "$PIDFILE"` (line 2234) then writes through the attacker-owned file/symlink, and later `kill "$(cat "$PIDFILE")"` / `kill -0 $(cat ...)` (lines 2208, 2241) trusts attacker-controllable contents with no numeric validation, letting the attacker make the victim signal an arbitrary process they own.

**Current location:** bin/dotfiles-setup:2291

**Verifier note:** The remediation was applied only to the Go TUI's embedded CaffScript. The legacy bash path (bin/dotfiles-setup) still writes a caff script with the predictable world-writable /tmp pidfile and no PID validation, exactly as the finding describes. Fix would be to port the hardened script (or drop the legacy generator).


### [MEDIUM/duplication] internal/ui/app.go:517 — Three near-identical duplicated flows: backup create, backup restore, and install-cache load

(1) createBackupCmd (internal/ui/app.go:517-574) and autoBackupIfEnabled (app.go:639-703) duplicate ~55 lines: same filesToBackup list, same flat-encoding loop, same manifest write. (2) restoreBackupCmd (app.go:457-506) and restoreBackup (cmd/dotfiles/main.go:602-687) duplicate the restore algorithm and have already diverged (0600 vs 0644 permissions, silent vs printed errors). (3) loadInstallCacheCmd (internal/ui/cache.go:22-82) and ensureInstallCache (cache.go:109-171) duplicate ~55 lines of batch install detection including the hardcoded {hk, caff, sshh} list. Divergence in pair (2) already produced the security regression reported separately.

**Current location:** internal/ui/cache.go:77 and internal/ui/cache.go:154

**Verifier note:** Backup create/restore duplication eliminated via internal/backup package (the security-regression-prone pair is fixed). Only the install-cache duplication in cache.go remains; the two copies are structurally identical, differing only in returning a map vs writing to App state.


### [MEDIUM/duplication] internal/ui/hotkeys_dualpane.go:20 — Dual-pane layout engine duplicated three times (manage, hotkeys, users)

hotkeysLayout (hotkeys_dualpane.go:20-145) is a field-for-field copy of manageLayout (manage_dualpane.go:629-769): same struct members, same headerH/footerH/leftW clamp(w/3,26,42)/minRight=38 geometry, same maxScroll/inLeftList/inRightList helpers, same globe-reservation logic (~130 duplicated lines). screen_users.go then implements a THIRD ad-hoc dual-pane with hand-computed hit-testing offsets (handleUsersMouse uses magic 'msg.Y - 5' at lines 426/433 and width/3 split at 422) that is not derived from any layout struct and can silently disagree with what renderUsersDualPane draws (its header is 2-4 lines depending on creating/deleting state, so Y-5 misses). The right-aligned-badge list row rendering is also duplicated between renderManageToolsPanel (manage_dualpane.go:1233-1290) and renderHotkeysCategoriesPanel (hotkeys_dualpane.go:775-815).

**Current location:** internal/ui/screen_hotkeys.go:537

**Verifier note:** The correctness-adjacent part of the finding is fixed: screen_users.go no longer uses the magic 'msg.Y - 5' hit-testing; it now uses documented constants (firstRowY = usersTabBarRows + usersHeaderRows, screen_users.go:589) and explicitly compensates for the dynamic description line (C19 logic, lines 611-632). However, the core duplication remains: hotkeysLayout is still a copy of manageLayout with identical geometry and helpers (hotkeys file moved from hotkeys_dualpane.go to screen_hotkeys.go), the users screen is still a third ad-hoc dual-pane with a hand-coded a.width/3 split (screen_users.go:600) not derived from any shared layout struct, and the right-aligned-badge list-row rendering is still duplicated between the manage and hotkeys panels.


### [MEDIUM/duplication] internal/ui/input_wizard.go:12 — Animation-skip / post-intro transition block duplicated verbatim between input_wizard.go and app.go, plus the 'start update check on entering ScreenUpdate' guard repeated in four places

input_wizard.go:12-27 (key press skips intro) is byte-for-byte identical to app.go:780-793 (animationDoneMsg handler): set animationDone, switch to postIntroScreen, conditionally fire checkUpdatesCmd or startInstallCacheLoad. Separately, the 3-line idiom 'if !a.updateChecking && !a.updateCheckDone { a.updateChecking = true; return checkUpdatesCmd() }' appears at input_wizard.go:17-20, app.go:783-786, input_management.go:44-47, input_management.go:59-62, and input_mouse.go:63-66. A change to screen-entry side effects (e.g., adding a new async load) must currently be replicated in 4-5 call sites.

**Current location:** internal/ui/state_helpers.go:39

**Verifier note:** The byte-for-byte duplicated intro transition block is consolidated into postIntroTransition, and a shared startTabTargetLoad helper was added for tab navigation. However, app.go's postIntroTransition, screen_mainmenu.go's menu-select handler, and updateScreen.Init each still inline the same update-check guard instead of reusing startTabTargetLoad, so the "repeated in four places" half of the finding remains (reduced severity; each site is small and startTabTargetLoad exists as the obvious consolidation point).


### [MEDIUM/duplication] internal/ui/screens_deepdive.go:1181 — Five near-identical selection-list render functions, with item lists duplicated a second time in the input handlers

renderConfigMacApps (798-870), renderConfigUtilities (1113-1179), renderConfigCLITools (1181-1250), renderConfigCLIUtilities (1252-1322), and renderConfigGUIApps (1324-1393) are ~60-line copies of each other: identical cursor/checkbox/nameStyle/'(installed)' suffix logic, differing only in the item slice, index field, and box width. Worse, each screen's item id list exists again, hand-copied, in input_deepdive.go (lines 411, 434, 457, 480, 503) and the MCP list at screens_deepdive.go:1591-1603 is duplicated at input_deepdive.go:651. This double-maintenance already produced the claude-code drift bug (finding 1) and will produce more as tools are added. renderCheckboxInline (screens_deepdive.go:1077-1088) is also dead code, superseded by renderCheckboxInlineWithInstallState.

**Current location:** internal/ui/screen_config_cliutilities.go:58

**Verifier note:** The dangerous half of the finding (item lists duplicated between render and input handlers, which caused the claude-code drift bug) is fully resolved by the ScreenHandler refactor. What remains is cosmetic render-loop duplication across five View methods with no state to drift; severity of the residual is low.


### [MEDIUM/bug] internal/ui/screens_test.go:82 — TestRenderFileTree_PackagesToInstall asserts a tautology-adjacent either/or and can never catch a categorization bug

The test enables lazygit and bat, then only checks that the output contains 'Packages to Install' OR 'Already Installed'. It never checks which section a tool landed in, nor that the tool IDs appear at all. Because install state comes from the real host (see finding above), the author weakened the assertion to pass everywhere. A regression that puts every selected tool into the wrong bucket (e.g., inverting the a.manageInstalled[id] check at screens.go:470) would still pass this test.

**Current location:** internal/ui/screen_golden_test.go:272-285

**Verifier note:** The determinism half of the complaint is fixed (cache pre-populated). The core gap remains: inverting the a.manageInstalled[id] categorization in internal/ui/screen_filetree.go (~lines 92-135) would still pass all tests, since no test asserts which section a selected tool renders in.


### [MEDIUM/duplication] internal/ui/styles.go:421 — All 11 UI styles are defined twice: package-level var block duplicates updateStyles(), and the two copies can drift (GradientCyber already has)

styles.go:421-479 initializes ContainerStyle, TitleStyle, LogoStyle, ButtonStyle, ButtonActiveStyle, ButtonGlowStyle, StatusReadyStyle, StatusPendingStyle, StatusErrorStyle, HelpStyle, AccentBoxStyle; styles.go:320-371 (updateStyles) re-creates the exact same 11 definitions. Any style tweak must be made in two places. The drift risk is already realized for gradients: the default GradientCyber (styles.go:397-399) is a 7-stop gradient, but updateDynamicColors (styles.go:309-313) replaces it with a 3-stop gradient. Since SetTheme() is called at startup when loading saved config, the 7-stop default is only ever visible on a truly fresh run; selecting 'neon-seapunk' explicitly yields different visuals (3 stops) than the shipped default (7 stops).

**Current location:** internal/ui/styles.go:329-355 and 392-422 (duplication); 318-322 vs 370-372 (gradient drift)

**Verifier note:** Scope reduced: 6 of the original 11 duplicated styles (LogoStyle, ButtonGlowStyle, StatusReady/Pending/ErrorStyle, AccentBoxStyle) were removed from the codebase entirely (no references anywhere in internal/ui). But the core defect persists: the remaining 5 styles are still defined twice and can drift, and the GradientCyber 7-stop vs 3-stop discrepancy described in the finding is unchanged — SetTheme("neon-seapunk") still yields a 3-stop gradient differing from the shipped 7-stop default.


### [LOW/docs] CLAUDE.md:61 — Multiple stale counts and inconsistent platform claims across CLAUDE.md, README.md, and AGENTS.md files

Verified stale facts: CLAUDE.md:61 says '43 screens' but internal/ui/app.go defines 46 Screen constants (ui/AGENTS.md:36 already says 46); CLAUDE.md:10 says '~3,200 lines of bash' vs actual 3,477 (wc -l bin/dotfiles-setup); CLAUDE.md:26 and ui/AGENTS.md:24 say '~12,600 lines' of UI vs actual 14,640; internal/hotkeys/AGENTS.md:54-62 lists 7 hotkey categories while hotkeys.go defines ~20 (eza, zoxide, Git, Delta, Glow, bat, btop, ripgrep, fd, Claude Code, Tailscale, ...); internal/config/AGENTS.md:23 says global config is 'config.json' but config.go:116 writes 'global.json'; README.md:5 says platforms are 'macOS, Linux (Arch/Debian), and Windows (via WSL)' while CLAUDE.md:7 says 'macOS, Linux (Arch/Debian), and Raspberry Pi' — the two top-level docs disagree about supported platforms.

**Current location:** CLAUDE.md:10, CLAUDE.md:27

**Verifier note:** All structural inconsistencies from the finding were remediated; only the approximate line counts have re-drifted (~6-7%) as the code grew. Low-severity doc-freshness residue, not the original cross-file contradictions.


### [LOW/bug] bin/dotfiles-setup:745 — safe_write_config is dead code and would crash under --no-backup (unbound BACKUP_SESSION_DIR)

safe_write_config (739-753) is defined but never called anywhere in the script (grep shows only the definition). If it were used, line 745 tests `[[ -n "$BACKUP_SESSION_DIR" ]]` without a `${...:-}` default; BACKUP_SESSION_DIR is only assigned in init_backup_session, which main() skips when SKIP_BACKUP=true (line 3432-3434), so under `set -u` the first call with --no-backup would abort with 'unbound variable'. Every live call site instead uses the guarded pattern `[[ -n "${BACKUP_SESSION_DIR:-}" ]]` (e.g. lines 1189, 1385).

**Current location:** bin/dotfiles-setup:767

**Verifier note:** Crash defect fixed; the dead-code half of the finding remains — safe_write_config is unused and could be removed. Low severity.


### [LOW/duplication] internal/pkg/pacman.go:113 — Duplicated '"pkg old -> new"' parsing blocks in CheckOutdated

pacman.go:113-132 (checkupdates output) and pacman.go:141-159 (paru -Qua output) are byte-for-byte identical parsing loops except for the InstalledBy string. Similarly, ListInstalled parsing in brew.go:203-213 and pacman.go:243-253 is the same 'split lines, Fields, name+version' block. Both pairs should share a helper (e.g. parseUpdateLines(out, source) and parseNameVersionLines(out, source)).

**Current location:** internal/pkg/brew.go:233 and internal/pkg/pacman.go:252

**Verifier note:** The higher-value half (checkupdates vs paru -Qua parsing in CheckOutdated) was deduplicated into parsePacmanUpdates. The minor ListInstalled duplication across brew.go/pacman.go remains; low-severity style issue only.


### [LOW/bug] internal/runner/bash.go:332 — Scanner errors silently swallowed in RunStreaming/StreamOutput — output truncates with no error surfaced

In streamPipe (lines 326-339, live code used for all package-manager streaming) `scanner.Err()` is never checked: a line exceeding the 1MB limit returns bufio.ErrTooLong and the loop exits silently, dropping the rest of that pipe's output while cmd.Wait() may still return nil. The dead-code twin StreamOutput (lines 130-142) is worse: it uses the default 64KB token limit AND ignores scanner.Err(), so a single long line (e.g., a compact-JSON or progress line from a package manager) silently truncates all subsequent output.

**Current location:** internal/runner/bash.go:132

**Verifier note:** The worse half (StreamOutput with default 64KB limit) is gone, but the live streamPipe closure still silently swallows scanner errors: a line >1MB triggers bufio.ErrTooLong, the loop exits, and remaining pipe output is dropped while cmd.Wait() may still return nil.


### [LOW/bug] internal/tools/lazygit.go:78 — LazyGit pager hardcodes 'delta --dark' regardless of selected theme

GenerateLazyGitConfig always emits 'pager: delta --dark --paging=never' and the LazyGitConfig.Theme field ('auto'/'dark'/'light') is never used anywhere in the generator. With any of the project's light themes, diffs inside lazygit render with dark-optimized colors; the Theme option shown to the user is dead.

**Current location:** internal/tools/lazygit.go:157

**Verifier note:** Dead-Theme half fixed; pager still passes --dark to delta even when Theme is "light", so light-theme diffs render with dark-optimized delta colors.


### [LOW/duplication] internal/tools/tmux.go:282 — Default-config literals duplicated between GenerateConfig and ApplyConfig in three tools

The identical hardcoded default config struct is written twice per tool: tmux.go:282-293 vs 299-310 (TmuxConfig), neovim.go:216-225 vs 230-239 (NeovimConfig), zsh.go:182-201 vs 206-224 (ZshConfig). Each pair can drift silently (a default changed in GenerateConfig but not ApplyConfig would make preview and applied config disagree). Similarly, the four macOS-app IsInstalled methods in apps.go (Rectangle 249-256, Raycast 286-293, IINA 323-330, AppCleaner 360-367) are byte-identical except for the app name. Note also these Registry-level ApplyConfig defaults ignore the user's deep-dive choices, so Registry.ApplyAllConfigs (registry.go:355), if ever wired up, would clobber a customized ~/.zshrc/.tmux.conf with defaults — today it is uncalled outside tests.

**Current location:** internal/tools/apps.go:253

**Verifier note:** Only the minor "also" portion of the finding remains (duplicated hasMacOSApp-wrapping IsInstalled methods); the config-drift/clobber risk that motivated the finding is fully resolved.


### [LOW/duplication] internal/ui/app.go:83 — Theme name list maintained in 3 Go places (+2 bash); welcome screen hardcodes '13 THEMES' while 16 exist

The theme roster is defined independently in config.AvailableThemes (internal/config/config.go:260-277, 16 entries), the ui themes slice with per-theme accent hex (internal/ui/app.go:83-105, 16 entries), and ThemePalettes (internal/ui/styles.go:35+, 16 entries) — plus the two bash tables (13 and 11 entries). renderWelcome (internal/ui/screens.go:219) still renders the literal '13 THEMES', and the embedded bash CLI help says 'Show all 13 available themes' (bin/dotfiles-setup:2040) — both stale relative to the 16-theme Go roster.

**Current location:** internal/ui/app.go:77

**Verifier note:** The user-visible "13 THEMES" welcome-screen defect is fixed (screens.go was split; welcome moved to screen_welcome.go). Core duplication finding still applies: theme roster maintained independently in three Go locations plus the legacy bash script, whose help text still says 13 themes.


### [LOW/bug] internal/ui/screens_deepdive.go:1576 — Claude Code screen's install toggle is hidden at index -1 with no visual affordance, and skips the install-cache guard

renderConfigClaudeCode focuses the 'Install Claude Code' toggle only when a.configFieldIndex == -1 (lines 1576-1579), reachable solely by pressing 'up' from the first MCP row (input_deepdive.go:653-657). Nothing on screen or in the help line ('space toggle - esc back') indicates the row above the list is selectable, and unlike its sibling screens this render function never calls ensureInstallCache/checks manageInstalledReady before reading a.manageInstalled["claude-code"] (line 1578), so the '(installed)' state can render stale-false if this screen is reached before the cache loads. Given claude-code is also unreachable on the CLI Tools screen (finding 1), this obscure toggle is currently the only way to enable the tool.

**Current location:** internal/ui/screen_config_claudecode.go:152

**Verifier note:** Remaining defect: "(installed)" state can render stale-false if this screen is reached before the async install cache loads; fix is a one-line a.ensureInstallCache() at the top of View(). Note claude-code is now present on the CLI Tools screen too, so the "only way to enable the tool" claim from the original finding is obsolete.


### [LOW/duplication] internal/ui/screens_management.go:629 — handleMainMenuMouse re-implements screen-entry async bootstrap already in handleTabNavigationWithCmd

The switch at screens_management.go:629-651 (ScreenManage -> startInstallCacheLoad, ScreenBackups -> loadBackupsCmd, ScreenUpdate -> checkUpdatesCmd, ScreenUsers -> loadUsersCmd, plus guard flags) duplicates the same logic in handleTabNavigationWithCmd (internal/ui/state_helpers.go:57-78) and again in the keyboard main-menu handler. Three copies of the 'entering screen X requires starting async load Y with guard flags Z' invariant means a new screen or a changed guard only gets updated in some paths.

**Current location:** internal/ui/screen_mainmenu.go:53-72

**Verifier note:** The refactor collapsed three copies to two, but selectItem's switch (screen_mainmenu.go:53-72) still re-implements exactly the same on-enter async bootstrap as startTabTargetLoad — ScreenManage -> startInstallCacheLoad, ScreenBackups -> backupsLoading/backupsLoaded guard + loadBackupsCmd, ScreenUpdate -> updateChecking/updateCheckDone guard + checkUpdatesCmd, ScreenUsers -> usersLoaded guard + loadUsersCmd — instead of calling startTabTargetLoad(a, target). A changed guard or new screen load still needs updating in two places. Low severity; the mouse-vs-keyboard duplication specifically cited is fixed.



## Fixed on main (verified)

### [CRITICAL/bug] bin/dotfiles-setup:3219 — Two incompatible backup formats share one directory; bash restore of a Go-created backup DELETES current configs

The destructive defect (bash restore deleting configs from a Go-format manifest) is fixed by the path-safety guard. The two formats still share ~/.config/dotfiles/backups with no format marker, so bash-restoring a Go backup still fails (safely) with per-line warnings/errors rather than restoring anything — an interop/UX gap, not a data-loss bug.


### [CRITICAL/bug] internal/config/claude.go:98 — SaveClaudeConfig destroys all non-MCP settings in ~/.claude/settings.json

All three defects addressed: no data loss (merge-preserve save at claude.go:116-158), correct file path (~/.claude.json per claude.go:65-67 comment), and LoadClaudeConfig (claude.go:79-110) reads only mcpServers from a generic map. Minor residue: ApplyConfigWithMCPs (internal/tools/claude_code.go:77-80) still swallows a Load error and starts from an empty MCP map — on a transient read failure this


### [CRITICAL/bug] internal/tools/claude_code.go:61 — ApplyConfigWithMCPs destroys the user's entire ~/.claude/settings.json

All three aspects remediated: wholesale clobber (save now preserves unrelated keys), wrong file (writes to ~/.claude.json where user-scope MCP servers actually live), and error-swallow clobber. ApplyConfigWithMCPs (internal/tools/claude_code.go:77-80) still swallows LoadClaudeConfig errors, but this is now harmless: SaveClaudeConfig re-reads the file itself and returns an error on malformed JSON (


### [HIGH/docs] README.md:140 — Documented theme commands do not work: 'dotfiles theme dracula' and 'dotfiles theme --list' both fail

All three cited docs (README.md, CLAUDE.md, cmd/dotfiles/AGENTS.md) now show the positional-arg syntax the CLI actually accepts.


### [HIGH/bug] cmd/dotfiles/main.go:659 — Path-traversal guard rejects ALL files when $HOME has a trailing slash, and uninstall then deletes the backups anyway

Both halves of the finding remediated: the guard now uses filepath.Clean(home) (and the CLI delegates to internal/backup.Restore, eliminating the test-vs-production divergence noted in the finding), and uninstall no longer deletes the config dir after a failed/empty restore.


### [HIGH/bug] internal/pkg/pacman.go:106 — CheckOutdated silently reports 'no updates' when checkupdates is missing or fails

Core defect (missing/failing checkupdates collapsed into "everything up to date") is remediated. The `paru`/`pacman -Qua` AUR check (:114-119) still ignores its exit status, but this is now a deliberate, documented choice because -Qua exits non-zero when there are no foreign updates — its exit code cannot distinguish "no updates" from failure, and it only runs when paru is confirmed present.


### [HIGH/efficiency] internal/ui/hotkeys_dualpane.go:167 — Disk read + JSON parse per favorite check, per item, per animation frame

Disk read + JSON parse per favorite check eliminated by per-frame username cache and in-memory favorites config.


### [HIGH/bug] internal/ui/input_deepdive.go:456 — CLI Tools screen renders 5 rows but keyboard handler only knows 4 — claude-code row is unreachable

The old input_deepdive.go/screens_deepdive.go code is gone; the CLI Tools screen was rewritten as configCLIToolsScreen with a single source of truth. The claude-code row is now intentionally non-navigable context (documented at lines 15-19, 39-41): it is configured on its own screen (ScreenConfigClaudeCode), so it is no longer an "unreachable" defect. Tests (screen_golden_test.go:670-672, standalo


### [HIGH/bug] internal/ui/installation.go:36 — startInstallation's Cmd goroutine mutates App state (installOutput, installStep) concurrently with the UI goroutine — data race

The concurrent-mutation defect was refactored away with a channel/event architecture; the worker snapshots cfg/theme before starting (installation.go:62,104) so no App reads occur off-loop either.


### [HIGH/bug] internal/ui/installation.go:65 — startInstallation's tea.Cmd goroutine mutates App state (installOutput/installStep) concurrently with View — data race

The direct-mutation goroutine was replaced with the message/channel pattern the finding recommended, including context cancellation for teardown.


### [HIGH/bug] internal/ui/screen_users.go:469 — Reachable panic: negative strings.Repeat counts on small terminals in Users screen

Code comments at lines 663-665 and 673-674 explicitly reference this panic class, indicating a deliberate remediation.


### [HIGH/bug] internal/ui/screens_deepdive.go:1200 — CLI Tools screen renders a 'Claude Code' row the user can never reach or toggle

The original defect (handler list drifted from render list, leaving a reachable-looking but unfocusable row) no longer exists. The claude-code row is still rendered but intentionally non-navigable by design: itemIDs is built from the same cliToolItems slice (screen_config_clitools.go:47-50), the row is marked with sentinel field index -1 so clicks resolve to nothing (lines 85-89), and tests (scree


### [HIGH/duplication] internal/ui/screens_manage.go:256 — Entire legacy per-tool manage layer (~600 lines across 3 files) is unreachable dead code duplicating manage_dualpane.go

Entire legacy per-tool manage layer (the ~600 lines of unreachable renderers and key handlers) was deleted; no duplication remains.


### [HIGH/bug] internal/ui/screens_test.go:209 — renderFileTree tests are non-hermetic: they shell out to the host's real package manager

The cited test file was removed and its successors are hermetic. Note: production code still uses package-level pkg.DetectManager()/tools.GetRegistry() rather than injected deps, but the tests no longer depend on the host environment, which was the defect described.


### [MEDIUM/docs] README.md:5 — README says Windows via WSL only; native ps1 script undocumented

The README no longer omits the native PowerShell installer; the WSL-only claim is gone.


### [MEDIUM/docs] README.md:193 — 'dotfiles restore' does not 'Restore most recent' as documented — it opens the TUI backup selector

Documentation was corrected to describe the actual TUI-picker behavior; code unchanged.


### [MEDIUM/bug] bin/dotfiles-setup:583 — `--theme` as the last argument crashes with 'unbound variable' instead of the intended usage error

bin/dotfiles-setup:602 now uses "${2:-}" in both tests: `if [[ -n "${2:-}" && ! "${2:-}" =~ ^- ]]`, so under set -u a trailing --theme falls into the else branch and prints the intended usage error (lines 612-614) instead of an unbound-variable crash.


### [MEDIUM/bug] bin/dotfiles-setup:2696 — add_user argument parser infinite-loops when a flag is given without a value

The bare `shift 2` pattern remains (lines 2815/2819/2824), so a trailing flag yields an abrupt "unbound variable" crash rather than a friendly usage error — a minor UX issue, but the reported infinite-loop/CPU-spin defect is impossible under set -euo pipefail.


### [MEDIUM/bug] bin/dotfiles-setup:3207 — restore_backup reports a spurious error on every successful restore

Success path can no longer increment errors. Minor residual: `((errors++))` at lines 879/3380 on the unsafe-path skip branch would return exit 1 when errors==0 under `set -e`, but that is a different (error-branch) code path than the finding described.


### [MEDIUM/bug] bin/dotfiles-setup:3395 — setup_nvim deletes ~/.config/nvim before git clone; clone failure leaves no nvim config

Clone-then-swap pattern eliminates the destroy-before-network-op hazard; existing config is never deleted on clone failure regardless of backup settings.


### [MEDIUM/bug] cmd/dotfiles/main.go:422 — Nil-pointer panic in `dotfiles theme list` when global.json is corrupt

Nil-deref path is eliminated both at the call site (error handled with default config) and defensively in listThemesWithConfig.


### [MEDIUM/security] cmd/dotfiles/main.go:677 — Restore loosens file permissions: 0600 backups are written back as 0644 (dirs 0755)

The old os.WriteFile(dstPath, data, 0644) / MkdirAll 0755 code no longer exists anywhere in the worktree; restore now enforces 600/700 defaults and preserves original modes.


### [MEDIUM/duplication] cmd/dotfiles/restore_test.go:47 — Test reimplements the production path-safety check instead of testing it, and the copy diverges from production

Validation was centralized into internal/backup with an exported IsRestorePathSafe; the test exercises the real implementation, so both the duplication and the divergence described in the finding are resolved.


### [MEDIUM/docs] docs/security-scanning.md:93 — Security-scanning doc's example workflow is broken (Go 1.21 vs go.mod 1.25.6, deprecated codeql upload-sarif@v2) and its 'Known Exclusions' contradict the actual config

All four cited inaccuracies (Go version pin, deprecated upload-sarif@v2, wrong gosec exclusions, phantom Makefile targets/security.yml) are gone; the doc now describes the actual ci.yml-based setup and no longer shows nonexistent Makefile security targets.


### [MEDIUM/release] internal/config/claude.go:56 — AllMCPServers references npm packages that do not exist under those names

Regression tests exist: internal/config/claude_test.go:165-175 explicitly forbids the old bogus package names ("@context7/mcp", "@anthropic-ai/mcp-server-convex", "@anthropic-ai/mcp-server-puppeteer") and asserts the correct ones.


### [MEDIUM/bug] internal/pkg/update.go:153 — DotfilesPackages uses macOS/Arch names, so CheckDotfilesUpdates misses renamed Debian packages

Platform-aware allow-list resolves the Debian rename issue; packages not in stock Debian repos are deliberately omitted with comments.


### [MEDIUM/security] internal/runner/bash.go:104 — RunFunction builds a `bash -c` string with unquoted funcName/args (shell injection surface) — and the whole runner is dead, broken code

The injection-prone dead API was deleted rather than patched; remaining code uses argv-style exec with no shell interpolation.


### [MEDIUM/security] internal/scripts/scripts.go:110 — caff uses predictable world-writable /tmp pidfile — cross-user process-kill and symlink clobber

The cross-user /tmp pidfile, symlink clobber, and arbitrary-PID-kill vectors are all addressed in the current main-based worktree.


### [MEDIUM/docs] internal/ui/AGENTS.md:80 — AGENTS.md documents a mouse-zone library (zone.Mark/zone.Get) that is not a dependency and is not used anywhere

The misleading zone.Mark()/zone.Get() guidance was replaced with an accurate description of the actual manual coordinate hit-testing implementation.


### [MEDIUM/bug] internal/ui/app.go:487 — Restore loop swallows all per-file errors and reports success

All three aspects of the finding are remediated: per-file errors are no longer swallowed (collected as Skipped and surfaced in the TUI), success is not reported when files are skipped, and original file permissions are preserved via the backup manifest.


### [MEDIUM/duplication] internal/ui/app.go:639 — autoBackupIfEnabled duplicates ~50 lines of createBackupCmd verbatim

The ~50-line verbatim duplication is gone. autoBackupIfEnabled retains only its intentionally distinct bits (AutoBackup setting check, "_auto" timestamp suffix, autoBackupResult return); the capture loop, file list, manifest write, and cleanupBackups() are all shared, so drift between manual and auto backups is no longer possible.


### [MEDIUM/efficiency] internal/ui/hotkeys_dualpane.go:902 — Hotkeys favorites check does disk I/O (os.ReadFile of global.json) per visible item per render frame

The dual-pane file moved into internal/ui/screen_hotkeys.go. LoadGlobalConfig is now called at most once per frame (cache-cold fallback) rather than per row; regression tests exist in internal/ui/hotkeys_cache_test.go.


### [MEDIUM/duplication] internal/ui/input_deepdive.go:410 — Five identical toggle-list screen handlers, with tool-ID lists duplicated against the render code

The five structurally identical handlers were consolidated into configListNav, and each screen's tool-ID list now exists once, shared by input handling and rendering.


### [MEDIUM/duplication] internal/ui/manage_dualpane.go:1650 — renderManageLogPanel builds a full RenderLogPanel component per frame, then discards it and re-implements the same rendering inline

The duplicated dead work per render frame was removed; only the single inline implementation remains.


### [MEDIUM/bug] internal/ui/screens.go:696 — Progress screen's hardcoded 10-step model desyncs from actual installStep increments

renderProgress in internal/ui/screens.go was moved to internal/ui/screen_progress.go. The step counter now maps to a planned-step total computed per install, so it cannot desync/overflow as described.


### [MEDIUM/duplication] internal/ui/screens_management.go:325 — renderManage() is dead code duplicating renderManageDualPane, with per-frame subprocess pattern

The dead duplicate renderManage() with its per-frame IsInstalled()/InstalledCount() subprocess calls was removed along with the entire screens_management.go file; manage screen rendering is consolidated in manage_dualpane.go/screen_manage.go.


### [MEDIUM/bug] internal/ui/screens_management.go:704 — Backups list mouse click selects the entry above the one clicked (off-by-one)

Off-by-one corrected on main; handler moved from screens_management.go to screen_backups.go.


### [LOW/security] bin/dotfiles-setup:2234 — caff utility trusts a predictable world-writable /tmp pidfile — another local user can make `caff off` kill an arbitrary victim process

Finding cited bin/dotfiles-setup; caff generation moved to embedded Go script where both the predictable /tmp path and the unvalidated kill were remediated.


### [LOW/bug] bin/dotfiles-setup:3156 — list_backups prints mangled file count for backups where nothing matched

bin/dotfiles-setup:799 and :3303 now read: files=$(grep -c "|yes" "$manifest" 2>/dev/null || true); files=${files:-0} — the "|| echo 0" double-output is gone; grep's own "0" is captured, and ${files:-0} covers a truly empty result.


### [LOW/release] go.mod:26 — go.mod is not tidy: spf13/cobra is a direct dependency but marked '// indirect'

Module metadata is tidy on main; the misclassified indirect entries were moved to the direct block.


### [LOW/duplication] internal/config/claude.go:24 — context7 server definition duplicated between DefaultMCPServers and AllMCPServers

Minor: internal/config/AGENTS.md:188 still references config.DefaultMCPServers() in docs, but the code duplication the finding described no longer exists.


### [LOW/bug] internal/config/config.go:107 — All config saves are non-atomic; interrupted write corrupts JSON with no recovery

Verified in worktree /home/tiki/projects/dotfiles/.claude/worktrees/release-audit-docs; no remaining non-atomic writes in internal/config. Other os.WriteFile uses (backup/, tools/, ui/) are outside this finding's scope.


### [LOW/release] internal/runner/bash.go:229 — Runner is dead code shipping stale data: ListThemes has 13 themes vs the documented 16, ListBackups parses by "20" prefix

The stale 13-theme hardcoded list and fragile "20"-prefix backup parsing were deleted along with the entire unused script-scraping API; the runner package was reduced to the streaming/sudo helpers that have real callers.


### [LOW/duplication] internal/tools/ghostty.go:122 — Write<Tool>Config + duplicated default-config literals repeated across 9 tool files

No remaining GenerateConfig/ApplyConfig duplicate literals in tool files; only claude_code.go ApplyConfigWithMCPs remains, unrelated to this finding.


### [LOW/duplication] internal/tools/yazi.go:153 — Seven near-identical Write*Config functions and duplicated default-config literals

Both parts of the finding (copy-pasted write pattern and diverging default-config literals) were remediated by centralizing writeToolConfig and by refactoring to single per-tool config-mapping functions.


### [LOW/release] internal/ui/animation.go:115 — ~200 lines of animation.go are dead code containing two latent bugs (mid-rune string slicing and a divide-by-zero)

The dead code and both latent bugs (mid-rune slicing in generateLogo, divide-by-zero in NewMatrixRain) are gone with the file. internal/ui/screen_animation.go is an unrelated intro-animation screen, not the flagged code.


### [LOW/duplication] internal/ui/app.go:1017 — updateWithLogsMsg result handling duplicates updateRunDoneMsg block; intro-completion block duplicated too

Handlers moved from app.go into screen_update.go and screen_animation.go during the screen-handler refactor; the duplicated blocks no longer exist.


### [LOW/bug] internal/ui/input_deepdive.go:34 — Deep dive menu index is not re-clamped against the filtered menu before indexing

Code moved from input_deepdive.go (deleted) to screen_deepdivemenu.go in the ScreenHandler refactor. Enter (line 92) and mouse clicks (line 137-141, also range-checked) both go through the clamped selectItem.


### [LOW/bug] internal/ui/input_management.go:77 — Update screen 'down' increments the cursor without an upper bound, relying on a state-mutating clamp inside View()

Code moved from internal/ui/input_management.go to internal/ui/screen_update.go during a refactor; a defensive clamp still exists at screen_update.go:308-312 but is no longer load-bearing.


### [LOW/bug] internal/ui/input_mouse.go:129 — Theme picker mouse hit-testing is hardcoded to an approximated layout and misselects rows when the real container differs

Fix explicitly references this finding (C20) in a comment at screen_themepicker.go:143 ("unlike the legacy hand-derived len(themes)+6 / containerW=60"). Right-edge bound also added, covering the missing-X-bound part of the finding.


### [LOW/bug] internal/ui/manage_dualpane.go:123 — installToolCmd is dead code that would bypass the sudo check and streaming-log flow if ever wired up

Dead-code trap eliminated; only the sudo-checked streaming install flow remains.


### [LOW/bug] internal/ui/screen_users.go:447 — renderUsersDualPane sets usersLoaded=true in View without dispatching the load command

The Users screen was refactored into a ScreenHandler (usersScreen). The load is dispatched in Init() on every entry path, and startTabTargetLoad also kicks it for tab navigation; no render function mutates usersLoaded anymore.


### [LOW/release] internal/ui/screens.go:219 — Welcome screen hardcodes '13 THEMES' but 16 themes exist

Count is still a hardcoded literal rather than derived from len(themes), so it could drift again with future theme additions, but the reported defect (stale "13") is fixed.


### [LOW/duplication] internal/ui/screens/error.go:36 — ScreenError and ScreenSummary key handling exists in two parallel implementations that have already drifted

The duplicated/drifted legacy handlers were removed during the screen-handler migration; only one implementation per screen exists on main, so behavior no longer depends on construction path.


### [LOW/bug] internal/ui/screens/summary.go:14 — Summary screen hardcodes neon-seapunk colors instead of the theme palette the user just selected

File moved from internal/ui/screens/summary.go to internal/ui/screen_summary.go; local color constants removed in favor of SetTheme-updated package variables.


### [LOW/duplication] internal/ui/screens_manage.go:397 — Eleven near-identical renderManageX functions duplicate box/help/layout boilerplate

The duplication was removed via a full rewrite of the manage screens into a data-driven dual-pane architecture rather than by extracting a helper.



## Obsolete (code removed)

### [MEDIUM/security] internal/runner/bash.go:96 — Command injection and CWD-relative sourcing in Runner.RunFunction/DetectOS

The injection-prone dead code was deleted entirely; remaining exec paths pass args as separate argv elements, so no shell interpolation or CWD-relative sourcing remains.


### [MEDIUM/bug] internal/ui/deps_test.go:95 — MockConfigProvider.LoadUserProfile returns (nil, nil) for a missing profile, diverging from the real provider

The ConfigProvider abstraction and its mock were removed on main; screens call config.LoadUserProfile directly (internal/ui/screen_users.go:104,132,168), so the mock's (nil, nil) divergence can no longer exist.


### [LOW/release] cmd/installer/main.go:21 — Legacy cmd/installer entry point ships without the screen factory, diverging from the main binary

The redundant second TUI binary entry point no longer exists, so the screen-factory divergence and uninstall-cleanup gap it described no longer apply.


### [LOW/bug] internal/testutil/testutil.go:28 — TempConfigDir mutates process-global env with os.Setenv instead of t.Setenv

The helper and its package were deleted, so the finding no longer applies. Note: individual tests still use os.Setenv with manual restore (e.g. internal/config/user_test.go:125-135, internal/ui/main_test.go:26) rather than t.Setenv, but none call t.Parallel(), and that is outside the scope of this finding.


### [LOW/duplication] internal/ui/screens_test.go:106 — Redundant tautological guards in TestRenderFileTree_ConditionalConfigDirs

Both the test file and the renderFileTree function it exercised are absent from main, so the tautological-guard cleanliness finding no longer applies.


### [LOW/bug] internal/ui/screens_test.go:199 — TestRenderFileTree_SelectedToolsWithConfig can silently become a no-op

Finding cited TestRenderFileTree_SelectedToolsWithConfig in screens_test.go; both the test file and renderFileTree are gone on main. The replacement golden test has no vacuous-guard pattern, so the defect cannot recur in current code.

