# Adversarial Core Runtime Verification

Date: 2026-07-09
Primary report checked: `tasks/audit-work/primary-core-runtime.md`
Scope: independent verification of every H/M/L finding against the current source, production call graph, and tests. Production source was not modified.

## Bottom line

The primary report found many real defects, but its two most prominent update conclusions are not accurate for the current dashboard:

- The Updates screen never requests `all=true`. Both production calls to `checkSudoAndUpdateCmd` pass `false` (`internal/ui/screen_update.go:185`, `internal/ui/screen_update.go:202`), so `streamingUpdateAllCmd` and manager `UpdateAllStreaming` are unreachable from the current screen.
- Pressing `a` copies the exact displayed `updateResults` and routes them through the per-package path (`internal/ui/screen_update.go:188-202`). Apt and Homebrew therefore receive displayed package names. Arch still uses `-Syu`, but the screen explicitly says that a full system upgrade will ride along (`internal/ui/screen_update.go:180-185`, `internal/ui/screen_update.go:194-202`).

The apt streaming deadlock itself is real but latent: `AptManager.UpdateAllStreaming` waits without draining (`internal/pkg/apt.go:316-327`), while the runner can block after its 100-line buffer fills (`internal/runner/bash.go:119-149`). It is not a current-dashboard release blocker unless that dead branch is reconnected.

The primary report also missed a more immediate release blocker: backup coverage does not match the mutation surface, Manage/standalone config saves do not take a backup, and installation continues after a backup failure. The app can therefore overwrite existing application settings without a valid rollback point.

## Verdict summary

Verdicts for the 27 primary findings:

| Verdict | Count |
|---|---:|
| CONFIRMED | 21 |
| PARTIAL | 4 |
| REFUTED | 1 |
| DUPLICATE | 1 |
| Total checked | 27 |

Corrected retained findings, including three new omissions and excluding the refuted/duplicate findings:

| Severity | Count |
|---|---:|
| Critical | 0 |
| High | 6 |
| Medium | 14 |
| Low | 8 |
| Total | 28 |

Severity changes from the primary report:

- `CORE-H02`: High -> Low (real latent API deadlock, unreachable from current production UI).
- `CORE-H04`: removed (current UI does not call update-all and does pass displayed packages).
- `CORE-M02`: Medium -> High (silent partial capture plus install-continuation means rollback can be knowingly absent before destructive writes).
- `CORE-M05`: Medium -> High (the limitation is already a concrete clobber path, not only a future architecture concern).
- `CORE-M09`: Medium -> Low (real argv bug in an otherwise unused uninstall method).
- `CORE-M12`: removed as a separate production bug; the generated Yazi keymap actually uses `.`, so only the static documentation is stale and is covered by `CORE-L04`.

## Per-finding verdicts

| ID | Verdict | Corrected severity | Independent evidence and correction |
|---|---|---:|---|
| CORE-H01 | CONFIRMED | High | `noFollowWrite` opens an existing destination with `O_TRUNC` and supplies `mode` only to `OpenFile`; it never calls `Chmod` (`internal/backup/backup.go:149-161`). `TestRestorePreservesMode` creates a new destination (`cmd/dotfiles/restore_test.go:133-157`), so it cannot catch replacement of an existing `0644` file. |
| CORE-H02 | PARTIAL | Low | The deadlock mechanism is correct: the output channel is capped at 100 and `cmd.Wait` occurs only after both scanners finish (`internal/runner/bash.go:119-149`); apt calls `Wait` before consuming `Output` (`internal/pkg/apt.go:316-327`). Reachability is wrong: the only screen calls pass `all=false` (`internal/ui/screen_update.go:185`, `internal/ui/screen_update.go:202`), and repository-wide call search found no `all:true` construction. `streamingUpdateAllCmd` is retained dead code at `internal/ui/installation.go:952-1013`. |
| CORE-H03 | CONFIRMED | High | Uninstall loops over six basenames in both `~/.local/bin` and `/usr/local/bin` and calls `os.Remove` after only `os.Stat` (`cmd/dotfiles/main.go:785-817`). No manifest, target, checksum, or package ownership is checked. |
| CORE-H04 | REFUTED | Removed | The claimed call path is stale. `a` copies every displayed result and calls `checkSudoAndUpdateCmd(packagesToUpdate, false)` (`internal/ui/screen_update.go:188-202`); Enter does the same for selected/current results (`internal/ui/screen_update.go:163-185`). `streamingUpdateCmd` passes those names to `UpdateStreaming` (`internal/ui/installation.go:813-867`). Arch's `-Syu` behavior is explicitly disclosed before execution (`internal/ui/screen_update.go:180-185`, `internal/ui/screen_update.go:194-202`). The unused manager-wide APIs remain poor contracts, but they do not produce the reported dashboard behavior. |
| CORE-H05 | CONFIRMED | High | Defaults enable backup and retention (`internal/config/config.go:63-71`), but existing JSON is decoded into `var cfg GlobalConfig` (`internal/config/config.go:163-183`). Missing fields become `false/0/0`. Tool config correctly uses default-then-overlay (`internal/config/config.go:132-140`), and no partial-global fixture exists in `internal/config/config_test.go:111-147`. |
| CORE-M01 | CONFIRMED | Medium | Directory restore verifies readability, then destroys the live destination with `RemoveAll` and copies directly into place (`internal/backup/backup.go:289-301`). There is no staging/swap/rollback transaction. |
| CORE-M02 | CONFIRMED | High | Create silently continues on every stat/read/write failure and succeeds when any candidate succeeds (`internal/backup/create.go:30-64`). Data files precede the manifest (`internal/backup/create.go:42-61`), while backup listing accepts every directory regardless of manifest (`cmd/dotfiles/main.go:561-613`). More seriously, installation logs a backup error and continues (`internal/ui/installation.go:201-224`). The primary severity was too low. |
| CORE-M03 | PARTIAL | Medium | The concrete incompleteness is confirmed: `KeyboardStyle` is persisted (`internal/config/user.go:14-21`) but `ApplyUserProfile` applies only theme, nav, and active name (`internal/config/user.go:183-195`); tool JSON remains global under `ToolsDir` (`internal/config/config.go:92-95`), while hotkeys alone are keyed by user (`internal/config/hotkeys.go:11-20`). The phrase “full environment” is stronger than the package documentation, which describes per-user themes/navigation, but the dead keyboard field and lack of settings isolation are real. |
| CORE-M04 | CONFIRMED | Medium | Global/tool loads unmarshal without normalization or validation (`internal/config/config.go:116-140`, `internal/config/config.go:163-183`); profile load is likewise unchecked (`internal/config/user.go:77-97`), and profile save validates only the name (`internal/config/user.go:100-126`). |
| CORE-M05 | CONFIRMED | High | The architecture concern is already destructive behavior. Live writers replace whole Ghostty, tmux, Git, Yazi, FZF, LazyGit, Btop, and Glow files (`internal/tools/ghostty.go:174-187`, `internal/tools/tmux.go:291-300`, `internal/tools/git.go:242-251`, `internal/tools/yazi.go:210-237`) with narrow generated models and no observed-state merge. Manage invokes these writers (`internal/ui/config_apply.go:287-318`) without importing existing settings. Also, the primary report understated prototype residue: repository-wide references show **every type** in `internal/config/tool.go:3-89` is declaration-only, not merely `AppsConfig` and `UtilitiesConfig`; live models are separate types in `internal/tools` and `internal/ui`. Escalated because saving a small supported subset can clobber real existing settings today. |
| CORE-M06 | CONFIRMED | Medium | Most commands lack Cobra `Args`; theme invalid syntax prints usage and returns (`cmd/dotfiles/main.go:79-100`), update errors return success (`cmd/dotfiles/main.go:520-539`), and the pre-Cobra quick-switch path returns success for a valid nonexistent user (`cmd/dotfiles/main.go:317-335`). Only user add/delete use `ExactArgs` (`cmd/dotfiles/main.go:204-239`). |
| CORE-M07 | CONFIRMED | Medium | `DetectManager` is platform-gated (`internal/pkg/manager.go:125-157`), but `AllManagers` enables every executable found in PATH (`internal/pkg/manager.go:160-177`), and `CheckAllUpdates` queries them all (`internal/pkg/update.go:18-36`). The CLI prints only the detected manager before that all-manager query (`cmd/dotfiles/main.go:520-532`). A separate mutation-routing defect is recorded below as `ADV-CORE-M01`. |
| CORE-M08 | PARTIAL | Medium | Synchronous manager methods do discard diagnostics and have no context (`internal/pkg/brew.go:33-52`, `internal/pkg/apt.go:32-51`, `internal/pkg/pacman.go:49-75`), apt has no explicit debconf/noninteractive policy, and there are no timeouts. However, the current installer/update UI predominantly uses context-aware streaming commands and separately acquires sudo, so “all package operations” overstates current reachability. Enterprise/unattended reliability remains incomplete. |
| CORE-M09 | CONFIRMED | Low | Paru install/update correctly avoid sudo (`internal/pkg/pacman.go:57-62`, `internal/pkg/pacman.go:452-460`), but uninstall always runs `sudo <paru>` (`internal/pkg/pacman.go:67-75`). Repository-wide call search found no production caller of `PackageManager.Uninstall`; only mock tests call uninstall. The bug is real but currently latent, so Low is more accurate. |
| CORE-M10 | CONFIRMED | Medium | The script exits if `~/.sshh` is absent (`internal/scripts/scripts.go:277-283`) before argument handling at `internal/scripts/scripts.go:324-369`. It also exits when an existing file contains zero hosts (`internal/scripts/scripts.go:300-303`), so even `touch ~/.sshh && sshh add ...` cannot bootstrap the first record. The embedded `hk` help advertises `sshh add` (`internal/scripts/scripts.go:91-96`). |
| CORE-M11 | CONFIRMED | Medium | Runtime-dir setup errors are ignored (`internal/scripts/scripts.go:114-117`); non-macOS blindly launches `systemd-inhibit` (`internal/scripts/scripts.go:198-206`); after the bounded loop it writes a pidfile and prints success without proving the child survived or the write succeeded (`internal/scripts/scripts.go:207-224`). Tests exercise extracted PID validation, not the complete start path (`internal/scripts/scripts_test.go:96-294`). |
| CORE-M12 | DUPLICATE | Removed; fold into L04 | The dynamic dashboard is consistent with the file this project generates: `GenerateYaziKeymap` binds hidden toggle to `.` for both styles (`internal/tools/yazi.go:158-205`), and `Categories` shows `.` (`internal/hotkeys/hotkeys.go:86-96`, `internal/hotkeys/hotkeys.go:150-156`). The stale pieces are the static `hk` fallback (`internal/scripts/scripts.go:47-50`) and `internal/hotkeys/AGENTS.md:57-63`, so this is documentation drift already covered by `CORE-L04`, not a production binding bug. |
| CORE-M13 | CONFIRMED | Medium | IDs are generated from display descriptions (`internal/hotkeys/hotkeys.go:44-72`). Tests prove stability across nav style, uniqueness, and format (`internal/hotkeys/hotkeys_stable_id_test.go:8-191`), but no alias/version mapping preserves an ID after description wording changes. |
| CORE-M14 | CONFIRMED | Medium | Atomic replacement protects file integrity but not a read-modify-write transaction (`internal/config/config.go:16-48`). Claude separately reads, backs up, mutates, and writes (`internal/config/claude.go:122-157`); global/hotkey/profile saves have no lock or revision. Two processes can validly race and lose one update. |
| CORE-M15 | CONFIRMED | Medium | All MCP definitions invoke unversioned npm packages via `npx -y`; Convex explicitly requests latest (`internal/config/claude.go:24-61`). Context7 is enabled by default in the live deep-dive config (`internal/ui/deepdive.go:308`). This is reproducibility and consent debt for mock-enterprise use. |
| CORE-M16 | CONFIRMED | Medium | Independent run of the primary package set reproduced: CLI 21.0%, backup 71.2%, config 75.6%, hotkeys 100.0%, pkg 25.4%, runner 72.2%, scripts 100.0%, theme 0.0%; all tests passed. The runner helper always drains output before `Wait` (`internal/runner/bash_test.go:14-22`), package tests do not execute most real argv paths, and the update tests copy production algorithms instead of always invoking them (`internal/pkg/update_test.go:13-58`, `internal/pkg/update_test.go:151-190`). |
| CORE-M17 | CONFIRMED | Medium | `status` calls registry `Installed`/`NotInstalledForPlatform` (`cmd/dotfiles/main.go:466-471`); registry population serially calls every `Tool.IsInstalled` (`internal/tools/registry.go:127-140`); base tools issue one manager check per package (`internal/tools/tool.go:129-157`). The TUI has a batched snapshot path (`internal/ui/cache.go:26-57`), but the CLI registry path does not use it. |
| CORE-L01 | CONFIRMED | Low | Generic load/save form `filepath.Join(ToolsDir(), toolName+".json")` with no component validation (`internal/config/config.go:116-122`, `internal/config/config.go:143-150`). Current live caller uses constant `manage` (`internal/ui/manage_dualpane.go:115-117`), so this is hardening rather than a current CLI exploit. |
| CORE-L02 | CONFIRMED | Low | `OutputLine`, `OutputType`, and `Runner` are unused (`internal/runner/bash.go:14-39`), while real output is an untyped merged string channel (`internal/runner/bash.go:70-76`). Scanner errors are dropped (`internal/runner/bash.go:126-139`), and a second `Wait` receive returns the closed-channel zero value (`internal/runner/bash.go:85-88`). |
| CORE-L03 | CONFIRMED | Low | Theme identity/order/defaults are duplicated across config (`internal/config/config.go:63-71`, `internal/config/config.go:206-234`), theme (`internal/theme/names.go:3-21`, `internal/theme/theme.go:3-18`), and a third UI metadata list (`internal/ui/app.go:77-99`). Config defaults to Catppuccin while unknown palette lookup defaults to Neon Seapunk. `internal/theme` has no tests. |
| CORE-L04 | CONFIRMED | Low | Config docs name nonexistent `defaults.go`, `AllToolConfigs`, `LoadAllToolConfigs`, and `SaveAllToolConfigs` (`internal/config/AGENTS.md:5-14`, `internal/config/AGENTS.md:48-96`); they also name nonexistent `DefaultMCPServers` (`internal/config/AGENTS.md:186-191`). Parent docs repeat `defaults.go` (`internal/AGENTS.md:7-17`). Uninstall preview says `y`, while removal uses `sshh` (`cmd/dotfiles/main.go:720-723`, `cmd/dotfiles/main.go:789-797`). The Yazi static binding drift from `CORE-M12` belongs here. |
| CORE-L05 | PARTIAL | Low | Code-backed hygiene concern is real: `go.mod:3` declares Go 1.25.6 and retains notably old transitives including `golang.org/x/text v0.3.8` and `x/sys v0.36.0` (`go.mod:29-30`); the local audit ran on Go 1.26.1. However, the repository does record CI's Go selection via `go-version-file: go.mod` (`.github/workflows/ci.yml:18-21`, `:89-97`, `:105-117`, `:128-134`) and runs govulncheck. The primary's exact advisory counts/fixed-version claims are not reproducible from repository artifacts, and the installed environment has no `govulncheck` binary. Retain the repeatable-release/SBOM concern, but do not treat the ephemeral scanner details as independently verified. |

## New findings omitted by the primary report

### ADV-CORE-H01 — Backup/rollback coverage does not match the configuration mutation surface

- Severity: High
- Evidence:
  - Manual and automatic backups capture only six fixed files (`internal/ui/app.go:735-745`).
  - Installation treats a failed backup as a warning and proceeds (`internal/ui/installation.go:201-224`).
  - The same installation then writes utilities and configs for tmux, Claude, Ghostty, Zsh, Neovim, Git, Yazi, FZF, LazyGit, Btop, and Glow (`internal/ui/installation.go:332-466`).
  - Yazi alone writes three files, but only `yazi.toml` is in the backup list (`internal/tools/yazi.go:210-237`). FZF, LazyGit, Btop, Glow, Claude, and Neovim's preferences overlay are outside that list (`internal/tools/fzf.go:211-220`, `internal/tools/lazygit.go:161-170`, `internal/tools/btop.go:208-223`, `internal/tools/glow.go:88-108`, `internal/config/claude.go:116-157`, `internal/tools/neovim.go:431-453`).
  - Manage saves and standalone tool config use real writers without any backup call (`internal/ui/manage_dualpane.go:95-146`, `internal/ui/config_apply.go:287-350`). Repository-wide search finds `autoBackupIfEnabled` only in the install worker (`internal/ui/installation.go:205`).
- Impact: “backup created” does not mean the operation can be rolled back. Existing keymaps, themes, application-specific settings, MCP state, and config files can be replaced with no captured original. This combines directly with `CORE-M05`'s full-file writers and is release-blocking for existing-config testing.
- Fix: derive backup/plan scope from the exact integration mutation plan; require every existing destination to be durably captured before any write; abort on backup failure; provide the same transactional backup for Manage and standalone config saves; verify rollback end to end for every writer.

### ADV-CORE-M01 — Multi-manager update records are always mutated through the single detected manager

- Severity: Medium
- Evidence:
  - `Package` records preserve `InstalledBy` (`internal/pkg/manager.go:26-34`), and `CheckAllUpdates` intentionally collects packages from every available manager (`internal/pkg/update.go:18-59`).
  - The screen preserves the records, but `streamingUpdateCmd` chooses one `DetectManager`, strips records to names, and calls that manager once (`internal/ui/installation.go:813-824`, `internal/ui/installation.go:838-867`). `InstalledBy` is never consulted.
  - The test comment claims distinct sources survive “so each is routed to the manager that can actually upgrade it,” but the test only copies the dedupe algorithm and never tests routing (`internal/pkg/update_test.go:9-35`).
- Trigger: Linux with both Homebrew and apt/pacman commands available (a supported/likely development arrangement), where a managed-name update originates from the non-default manager.
- Impact: selecting a Homebrew update on Debian can call `apt install <same-name>` instead, potentially installing/updating the wrong package and leaving the displayed package untouched. Manager attribution is displayed data, not an enforced mutation route.
- Fix: group selected records by source-manager identity and call each matching manager with only its records, or intentionally restrict the query to the detected manager. Test actual routing, not a copied dedupe loop.

### ADV-CORE-L01 — Lossy flat backup names can collide and corrupt two manifest entries

- Severity: Low (latent with today's six fixed candidates; higher once backup scope expands)
- Evidence: `EncodeName` replaces every separator with `_` and explicitly documents lossiness (`internal/backup/backup.go:32-40`). `Create` uses that value as the sole storage filename with no collision check (`internal/backup/create.go:42-52`), and restore recomputes the same name for each manifest entry (`internal/backup/backup.go:380-393`).
- Trigger: capture both `a/b` and `a_b` (or any equivalent separator/underscore collision).
- Impact: the later write overwrites the earlier payload; both manifest entries restore the same bytes while reporting success.
- Fix: store payloads under collision-free IDs (hash/UUID or mirrored escaped tree) recorded explicitly in the manifest; reject duplicate storage IDs; add collision round-trip tests before expanding backup coverage.

## Corrected release interpretation

The current core is still not ready for friends/family or mock-enterprise deployment, but not because the live dashboard calls `UpdateAllStreaming`: it does not. The immediate blockers are restore permission preservation, unsafe uninstall ownership, global-config migration, silent/incomplete backup behavior, narrow full-file config clobbering, and backup scope that does not cover those writes.

For development ordering:

1. Make config application plan-based and ownership-aware; preserve unowned settings.
2. Make backup scope derive from the plan, fail closed, and cover Manage/standalone saves.
3. Fix restore mode enforcement and uninstall ownership.
4. Add schema migrations/validation before adding new integrations.
5. Remove the dead `all=true`/`UpdateAllStreaming` path or redesign it before reuse; fix its runner backpressure first if retained.
6. Route multi-manager update records by their recorded source.
