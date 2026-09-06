# Primary audit: UI, dashboard, installer UX, and configuration fidelity

Audit date: 2026-07-09
Scope owner: `internal/ui/` (all tracked source, tests, and package documentation)
Method: line-by-line static review, call-chain tracing, adversarial refutation, local 80x24 observation supplied by the audit team, and targeted test execution.

## Verdict

The TUI has a substantial, attractive foundation, but it is **not ready for release or friends-and-family deployment**. The release blocker is not merely that Pi, OpenCode, Codex, Cursor Agent, and T3 Code are absent. The current dashboard can confidently present an inaccurate install plan, fail to install the core tools it configures, initialize from application defaults instead of the user's real files, and then overwrite unmodeled settings. A theme save expands that risk to every supported generator, including Git, with no Manage-path backup.

The visual system is cohesive and the code contains a large amount of thoughtful regression work: consistent seapunk theming, explicit loading/error states, scoped per-tool generator dispatch, cancelable streaming operations, mouse hit-test tests, and several race tests. Those strengths make an in-place stabilization preferable to a clean-room rewrite. They do not, however, offset the present configuration-loss and plan-integrity failures.

Release recommendation: **no-go** until C-01 through C-04 and H-01 through H-07 are resolved and proven on real, preconfigured machines.

Finding count: **4 critical, 13 high, 12 medium, 3 low/documentation**.

## Feature completeness snapshot

| Dashboard capability | Status | Release assessment |
|---|---|---|
| Quick/deep-dive install | Present but internally inconsistent | Core package installation is absent from the selected package plan while core configuration always runs. Blocker. |
| Existing-app detection | Present as one boolean | Slow on realistic Macs and cannot distinguish installed, partial, external, configured, or detection failure. High risk. |
| Manage installed tools | Broad UI, unsafe source of truth | Loads `manage.json` or defaults, not real app files. Per-tool saves regenerate modeled output and lose unmodeled settings. Blocker. |
| Theme management | Visually polished, operationally dangerous | A theme change rewrites all ten generator tools plus Claude Code, installed or not, without backup. Blocker. |
| Per-tool configuration | Consistent presentation, fragmented coverage | Wizard and Manage expose different subsets; Escape/Enter save in standalone mode without an explicit save contract. |
| Backups/restore | Functional list/confirm/status UI | Captures a fixed six-path subset and is not invoked by Manage saves; async results can be dropped on navigation. |
| Updates | Check, selection, logs, sudo, cancel | Check results can be dropped on navigation; rapid activation can start duplicate update flows. |
| Hotkeys | Useful viewer, favorites, filtering | “Add alias” only stores opaque data; nothing renders or applies it. Per-frame config reads add avoidable I/O. |
| Users/profiles | Basic local preference profiles | Not an enterprise identity/policy feature; live app state is not fully refreshed, help is contradictory, and async results are screen-local. |
| Mouse support | Considerable coverage | Hit testing is often carefully measured, but fixed-height assumptions break when narrow content wraps. |
| Responsive/tiny-terminal support | Partial | Standard 80x24 is visibly clipped; many wizard/config/list screens have no viewport. |
| Pi/OpenCode/Codex/Cursor Agent/T3 Code | Not integrated in these screens | Hard-coded group inventories and integer screen coupling make registry additions insufficient. |

## Critical findings

### C-01 — The installer does not install the core terminal tools

- **Kind:** feature completeness / install correctness
- **Evidence:** `internal/ui/installation.go:53-65` derives package work exclusively from `collectSelectedTools`; `internal/ui/installation.go:684-724` collects only `CLITools`, `GUIApps`, `CLIUtilities`, and macOS apps. Ghostty, tmux, zsh, Neovim, Git, Yazi, and FZF are absent. The only package-install loop consumes that list at `internal/ui/installation.go:234-318`. Those core tools are then configured at `internal/ui/installation.go:340-433`.
- **Trigger and impact:** Run Quick Install or Deep Dive on a fresh Mac lacking one or more core tools. The program creates/configures their files but never asks Homebrew to install their packages. The UI can finish with a nominal terminal environment whose primary executables are absent.
- **Recommendation:** Build one registry-driven `InstallPlan` containing every selected/default-enabled tool, configuration action, package action, platform resolution, ownership decision, and expected file mutation. Render and execute that exact immutable plan. Dedicated core config screens must not imply that `UIGroupNone` means “configuration only.”
- **Adversarial refutation:** `install_pi_packages_test.go:16-50` verifies only package-name resolution, not that core IDs reach the worker. `install_flow_test.go:42-45` actually codifies core items as unconditional *configuration* phases. No other package-install path exists in this partition. Finding stands.

### C-02 — Dashboard values are preferences/defaults, not detected current settings, and a scoped save loses unmodeled settings

- **Kind:** data/configuration loss / misleading UX
- **Evidence:** `internal/ui/app.go:422-463` constructs `NewDeepDiveConfig()` and `NewManageConfig()`, then loads only global JSON and `tools/manage.json`; there is no import of Ghostty, Git, tmux, zsh, Yazi, FZF, or other real config files. Standalone config similarly seeds from `manage.json` at `internal/ui/app.go:1020-1030`. The Ghostty defaults are explicit at `internal/ui/manage_config.go:110-124`. Saving one field calls the full generator at `internal/ui/config_apply.go:287-317`; Ghostty receives only the nine modeled values at `internal/ui/config_apply.go:123-136`. Manage persistence invokes that writer at `internal/ui/manage_dualpane.go:95-146`. There is no backup in this path.
- **Concrete local observation:** On the test Mac, `~/.config/dotfiles/tools/manage.json` did not exist while `~/.config/ghostty/config` did. The dashboard therefore showed JetBrains Mono / 14 / 100% / confirm-close true / scrollback 10,000 instead of the real JetBrainsMono Nerd Font / 13 / 90% / false / 67,108,864. Real unmodeled keys included padding, cursor blinking, mouse-hide behavior, window state, shell integration, clipboard protections, and custom keybinds. Saving any Ghostty field would regenerate the file without them.
- **Trigger and impact:** Open Manage on a machine with hand-maintained or externally generated configs, edit one displayed field, and press Save; or run `dotfiles config ghostty` and leave with Enter/Escape. Unmodeled settings inside that same tool are removed even though unrelated tool files are scoped out.
- **Recommendation:** Introduce explicit provenance: `Detected current`, `Desired managed`, `Default`, and `Unsupported/unmanaged`. Parse/import existing files before presenting values. Prefer managed include files or marked blocks over whole-file ownership; otherwise show a diff and require explicit ownership confirmation. Back up every file immediately before applying it.
- **Adversarial refutation:** Scoped apply (`internal/ui/manage_save_scope.go:111-163`) does protect *other tools*. It does not protect unknown keys inside the edited tool, and the baseline is the loaded/default Manage model rather than the actual file. Finding stands.

### C-03 — Changing the theme rewrites every generator, including Git and uninstalled tools, with no backup

- **Kind:** cross-tool data loss
- **Evidence:** `internal/ui/manage_save_scope.go:117-132` returns all ten generator IDs plus `claude-code` whenever the theme differs. `internal/ui/manage_save_scope.go:147-163` executes every returned writer with no installed/configured check. `internal/ui/manage_dualpane.go:105-145` persists preferences and immediately applies those generators without backup or preview. Git is a whole generator call at `internal/ui/config_apply.go:300-302`; its modeled inputs contain no `user.name`, `user.email`, LFS filters, signing key, or arbitrary sections (`internal/ui/deepdive.go:83-92`). `manage_save_scope_test.go:73-85` locks in the all-tools behavior. Claude is also appended even though theme is not an MCP concern, and its non-empty seven-key boolean map passes the length gate in `internal/ui/config_apply.go:320-327`.
- **Trigger and impact:** Change Global → Theme in Manage and press `S`. The app writes configs for absent and present tools from defaults. On a normal developer machine this can erase Git identity, LFS filters, custom aliases, credential/signing settings, and Ghostty custom keys across multiple files in one action.
- **Recommendation:** Theme selection should update a product palette and only regenerate artifacts that are both explicitly managed and theme-dependent. Use per-tool managed snippets/includes, file ownership manifests, preflight diffs, atomic transactions, and a mandatory backup. Never synthesize an absent tool's config merely because a global theme changed.
- **Adversarial refutation:** The code comment says every generated tool depends on theme, but that is not true of Claude MCP configuration, and “generator accepts a theme” does not establish user consent to own the whole destination file. Installed status is ignored. Finding stands.

### C-04 — Destructive installation proceeds after backup failure, and the advertised rollback set is incomplete

- **Kind:** recoverability / data loss
- **Evidence:** Auto-backup errors are logged and ignored at `internal/ui/installation.go:201-219`; destructive/configuration phases proceed at `internal/ui/installation.go:320-455`. The fixed backup set at `internal/ui/app.go:735-745` contains only six paths, omitting generated FZF, Btop, Glow, LazyGit, Yazi keymap/theme, broader Neovim trees, and future tools. The welcome promise says configuration is “fully reversible” (`internal/ui/screen_welcome.go:122-126`), while the File Tree labels files “backed up” unconditionally (`internal/ui/screen_filetree.go:206-211`).
- **Trigger and impact:** Backup fails due to permission, disk, manifest, or configuration error, or succeeds while missing a generated path. The install continues and can overwrite files without a usable rollback point.
- **Recommendation:** Make backup/preflight failure a hard stop unless the user explicitly confirms “continue without rollback.” Generate the backup manifest from the same immutable mutation plan; include directories/managed snippets as appropriate, verify hashes and manifest durability, and report the exact rollback coverage before confirmation.
- **Adversarial refutation:** The log accurately emits a warning and backup creation rejects zero captured files. That improves honesty after the fact but does not make proceeding safe, and the manifest is still incomplete. Finding stands.

## High findings

### H-01 — Core configuration runs unconditionally, including for absent or unwanted tools

- **Kind:** consent / idempotency
- **Evidence:** `internal/ui/installation.go:234-237` explicitly states configuration always runs. Tmux, Ghostty, zsh, Neovim, Git, Yazi, and FZF are written at `internal/ui/installation.go:340-433` without selection or installed checks. `internal/ui/install_flow_test.go:38-45` encodes this behavior.
- **Impact:** A rerun intended to install one optional utility can modify seven unrelated core configurations; fresh installs create files for applications the user does not have.
- **Recommendation:** Plan configuration independently per tool with `create`, `merge managed block`, `replace`, or `leave unmanaged` actions. Gate each action on explicit selection/ownership.
- **Refutation:** Reapplying configuration is intentional for idempotency, but idempotency is only valid for files the product owns. No ownership contract is checked. Finding stands.

### H-02 — The confirmation screen is not a faithful preview of execution

- **Kind:** information architecture / informed consent
- **Evidence:** `internal/ui/screen_filetree.go:87-142` inventories only selection maps, not core package actions. It claims installed tools' settings will update and “all tools” receive settings/themes at `internal/ui/screen_filetree.go:161-172`, while optional config writers are gated. It labels Ghostty/Yazi/Neovim paths “new” regardless of existence and zsh/tmux/Git “backed up” regardless of the backup setting/result at `internal/ui/screen_filetree.go:175-211`.
- **Impact:** The user approves a materially inaccurate plan and cannot know create-versus-replace behavior.
- **Recommendation:** Render the same `InstallPlan` the worker will execute. Show package, config, path, ownership, existing-state, diff size, backup destination, and unsupported/skipped actions.
- **Refutation:** A static summary can be conservative, but these labels are not conservative; they are false in both directions. Finding stands.

### H-03 — Screen-local async results are dropped when the user navigates away

- **Kind:** state machine / reliability
- **Evidence:** `ScreenManager.Update` sends non-global messages only to the current handler (`internal/ui/screen_manager.go:77-102`). Backups handle load/create/delete/restore results only in `internal/ui/screen_backups.go:73-137`, yet keyboard and mouse tab navigation remain available at `internal/ui/screen_backups.go:178-185,230-235`; `q` is even handled before the `backupRunning` guard at `internal/ui/screen_backups.go:60-68,152-155`. Update checks are screen-local at `internal/ui/screen_update.go:92-99` while tab changes remain available during `updateChecking` at `internal/ui/screen_update.go:132-144`. Users marks `usersLoaded=true` before the command (`internal/ui/screen_users.go:226-235`), handles results only at `internal/ui/screen_users.go:266-315`, and allows tabs at `internal/ui/screen_users.go:381-388`. The top-level global switch covers streaming update/manage messages but not these results (`internal/ui/app.go:915-939`).
- **Impact:** Navigate during a load or operation and the terminal message lands on another screen, is ignored, and leaves `backupsLoading`, `backupRunning`, `updateChecking`, or `usersLoaded` in a wedged/stale state. Completed destructive operations may receive no visible result.
- **Recommendation:** Centralize operations in an app-level coordinator with request IDs, ownership-independent result reducers, cancellation, and observable lifecycle. A screen should subscribe/render operation state, not own terminal messages.
- **Refutation:** Streaming update and Manage-install messages were correctly moved global. That fix demonstrates the failure mode but does not cover these message types. Finding stands.

### H-04 — Concurrent Manage saves can complete out of order and establish a false baseline

- **Kind:** concurrency / persistence integrity
- **Evidence:** `internal/ui/manage_dualpane.go:95-146` snapshots and launches an async save without a pending flag or generation ID. On any successful result, `internal/ui/screen_manage.go:105-114` snapshots the *current live* config, not the snapshot that actually completed.
- **Impact:** Press `S`, edit, press `S` again. If the older command finishes last, disk may contain A while the UI baselines B and reports `Saved ✓`; a subsequent save sees no diff and cannot repair the mismatch.
- **Recommendation:** Make saves single-flight or monotonically versioned. Return the persisted snapshot/version in `manageSavedMsg`; accept only the latest completion; write preferences and generated artifacts transactionally.
- **Refutation:** `manage_save_race_test.go:9-65` proves a value snapshot prevents a data race during one save. It does not test ordering, stale completions, or baseline correctness. Finding stands.

### H-05 — Install-status loading defeats its own batch optimization and took 13–22 seconds locally

- **Kind:** performance / dashboard availability
- **Evidence:** The cache performs batch enumeration at `internal/ui/cache.go:75-103`, then calls `t.IsInstalled()` for every tool not found in the batch at `internal/ui/cache.go:105-109`. A batch-negative package is normal “not installed” information, not a reason for another subprocess. `internal/ui/AGENTS.md:161-165` incorrectly claims this reduces approximately 27 subprocesses to one. The Manage screen blocks on a full-screen loading message (`internal/ui/screen_manage.go:679-691`). Audit-team PTY runs observed roughly 13–22 seconds; `dotfiles status` measured 22.23 seconds through the related status path.
- **Impact:** First launch feels hung, and the most important management screen is unavailable. More absent tools can mean more fallback probes.
- **Recommendation:** Treat a successful package-manager batch miss as authoritative for package-backed tools. Only call custom detection for direct binaries/app bundles/Flatpaks or when the batch itself failed. Parallelize bounded non-package probes, cache with provenance/TTL, and render the dashboard progressively.
- **Refutation:** Fallback is necessary for non-package installs. The registry can classify those cases; calling fallback for every negative discards the batch's main benefit. Finding stands.

### H-06 — Boolean install state mislabels partial/external/configured tools as absent

- **Kind:** status semantics / UX correctness
- **Evidence:** The cache stores only `map[string]bool` (`internal/ui/cache.go:79-125`), and `allPackagesInBatch` requires every package in a bundle (`internal/ui/cache.go:59-72`). Manage reduces that again to `installed bool` (`internal/ui/manage_dualpane.go:75-84,415-423`) and renders only INSTALLED/NOT INSTALLED (`internal/ui/manage_dualpane.go:817-824`). On the test Mac, zsh 5.9 plus autosuggestions and syntax highlighting were present, but the dashboard labeled Zsh NOT INSTALLED because optional/recommended bundle members were missing.
- **Impact:** Users lose trust in Ghostty/zsh/application status and cannot tell whether the executable, optional integrations, or only configuration is missing.
- **Recommendation:** Replace the boolean with structured states: `Installed`, `Partial`, `External`, `Configured`, `Missing`, and `Detection unavailable`, including missing components and detection source.
- **Refutation:** All-or-nothing is reasonable for a strict dependency bundle, but the UI label says the *tool* is not installed and supplies no missing-component explanation. Finding stands.

### H-07 — Standard 80x24 layout clips essential controls and corrupts fixed hit-test geometry

- **Kind:** responsiveness / accessibility
- **Evidence:** Manage assumes exactly three header and two footer rows (`internal/ui/manage_dualpane.go:224-262`) but renders an unbounded 119-character hint line at `internal/ui/manage_dualpane.go:647-682`. Its left pane is only 26 columns at width 80 (`internal/ui/manage_dualpane.go:237-243`); the installed-count subtitle is not truncated (`internal/ui/manage_dualpane.go:699-713`), and padded category tags deliberately consume the remaining width before tool names are truncated (`internal/ui/manage_dualpane.go:742-772`). Local 80x24 output wrapped `24 installed • 30 tools`, clipped the footer after `I install •`, hid Save/Esc/q, and truncated names to forms such as `Ghos…`, `Claud…`, and `Laz…`. The five-tab bar is built at full intrinsic width and merely assigned `.Width(width)` (`internal/ui/styles.go:418-465`), not responsively shortened.
- **Impact:** Primary navigation/help disappears at the most common terminal size; wrapping adds rows that the mouse layout still assumes do not exist. The dashboard's hierarchy is dominated by category badges while names lose identity.
- **Recommendation:** Add breakpoints: compact tabs, single-pane Manage below a safe width, hide/move category tags before truncating names, context-sensitive short help, and measured wrapping. Assert every rendered line width and total height at 40x10, 60x18, 80x24, 100x30, and wide sizes.
- **Refutation:** Manage has scroll offsets and clamps, and one test checks 120x30 log-panel width. Those do not constrain header/footer/subtitle output at 80x24. Finding stands.

### H-08 — Many screens render full lists without a viewport, so selected controls can be off-screen

- **Kind:** responsive UX / keyboard visibility
- **Evidence:** Deep Dive builds every category/item plus legend/help and centers the complete block (`internal/ui/screen_deepdivemenu.go:175-288`). Theme Picker prints all 16 themes (`internal/ui/screen_themepicker.go:150-231`). Neovim renders 14 focusable rows (`internal/ui/screen_config_neovim.go:16-27,63-168`), tmux up to 13 (`internal/ui/screen_config_tmux.go:19-45,151-305`), while shared config screens have no vertical viewport. File Tree (`internal/ui/screen_filetree.go:148-265`), Backups (`internal/ui/screen_backups.go:377-485`), Update (`internal/ui/screen_update.go:349-413`), and Users (`internal/ui/screen_users.go:716-889`) similarly render unbounded content/lists. Users' “tiny” fallback is only 10x5 (`internal/ui/screen_users.go:654-682`).
- **Impact:** At 80x24 or smaller, continue buttons, help, fields, backups, or users are clipped while keyboard focus keeps moving invisibly.
- **Recommendation:** Standardize a viewport component with selected-row auto-scroll, page keys, scroll indicators, and a compact fallback. Keep footer/action areas pinned.
- **Refutation:** Mouse tests use tall terminals and `config_mouse_test.go:299-312` skips labels that are not visible, which masks rather than disproves the defect. Finding stands.

### H-09 — Update actions can be started more than once before the running flag is set

- **Kind:** async race / package safety
- **Evidence:** Enter and `a` return `checkSudoAndUpdateCmd` at `internal/ui/screen_update.go:163-203`, but leave `updateRunning=false`. That flag becomes true only when the later `updateStartMsg` reaches `internal/ui/streaming.go:55-65`. Rapid Enter/`a` presses can enqueue multiple checks/prompts/starts; each flow shares the same stream/cancel fields.
- **Impact:** Duplicate package-manager operations or sudo prompts can overlap, with one stream overwriting another's cancellation handle.
- **Recommendation:** Set a synchronous `pending` operation state before returning the first command, reject subsequent starts, and carry an operation ID through sudo/start/stream messages.
- **Refutation:** The existing `updateRunning` guard is effective only after the asynchronous round trip. Finding stands.

### H-10 — Planned AI tools require edits in multiple hard-coded UI catalogs

- **Kind:** architecture / planned feature readiness
- **Evidence:** Selection defaults are registry-driven (`internal/ui/deepdive.go:8-23`), but rendered/navigable lists are duplicated literals: CLI tools at `internal/ui/screen_config_clitools.go:24-50`, CLI utilities at `internal/ui/screen_config_cliutilities.go:18-41`, GUI apps at `internal/ui/screen_config_guiapps.go:18-39`, macOS apps at `internal/ui/screen_config_macapps.go:18-37`, and helper scripts at `internal/ui/screen_config_utilities.go:18-36`. Dedicated routes require an int enum, factory case, tools-screen map, renderer, model fields, generator, translation, diff fields, and tests (`internal/ui/toolscreens.go:9-93`). Pi, OpenCode, Codex, Cursor Agent, and T3 Code are absent from these inventories.
- **Impact:** Adding a registry tool can silently make it selectable in the config map but invisible/unreachable in the UI, or visible without install/apply support.
- **Recommendation:** Define a typed, registry-owned UI descriptor/schema: group, order, fields, validation, platform, install strategy, detection strategy, config ownership, route/panel factory, and apply/import adapters. Use a shared AI-agent integration model for install/auth/provider/config rather than one raw Screen integer per product.
- **Refutation:** Startup panics catch mismatched dedicated screen integers, and the Mac-app golden test checks one inventory. They do not reconcile all group literals with the registry. Finding stands.

### H-11 — Completion and error screens overstate success

- **Kind:** outcome integrity / user guidance
- **Evidence:** Summary always displays “Installation Complete,” a backup path, and static zsh/tmux/Neovim/p10k/hk steps (`internal/ui/screen_summary.go:52-81`) regardless of installed selections, failed phases, or backup result. Error → Skip navigates to that generic success screen (`internal/ui/screen_error.go:34-49`).
- **Impact:** A partial/failed install can end in a green success screen with invalid next steps and a nonexistent rollback claim.
- **Recommendation:** Carry a structured outcome into Summary: succeeded/failed/skipped actions, actual backup, next actions derived from installed components, and “completed with errors” presentation. “Skip” must acknowledge unresolved failures.
- **Refutation:** The preceding Error screen exposes the error, but Skip explicitly moves to a contradictory success state. Finding stands.

### H-12 — Standalone config uses Escape and Enter as implicit destructive Save, while `q` silently discards

- **Kind:** command discoverability / destructive UX
- **Evidence:** Shared help says `enter/esc back` (`internal/ui/screen_config_base.go:56-61`), but in standalone mode both call `applyStandaloneConfigCmd` and quit (`internal/ui/screen_config_base.go:63-103`). `q` quits without apply at `internal/ui/screen_config_base.go:91-103`. Claude repeats the same contract (`internal/ui/screen_config_claudecode.go:50-61,82-98`). Apply errors are written to stderr and the TUI then quits (`internal/ui/config_apply.go:13-52`) rather than showing a durable error screen.
- **Impact:** Users reasonably use Escape to cancel and instead overwrite a config; the only discard action is undocumented. Save failure may disappear with the alternate screen.
- **Recommendation:** Add explicit dirty state and commands: `S Save`, `Esc Cancel/back`, `q Quit` with confirmation if dirty. Present a diff/ownership warning before the first write and retain an error screen on failure.
- **Refutation:** The behavior is documented in source comments and was added to avoid discarding edits. It is not disclosed to the user and violates conventional Escape semantics. Finding stands.

### H-13 — The UI test suite is red and slow because config-path expectations drifted and unit tests perform install side effects

- **Kind:** repository health / test architecture
- **Evidence:** `go test ./internal/ui` failed after 51.265 seconds. Failures were `TestInstallAndConfigApplyProduceSameFiles/glow`, `TestConfigGating_DeselectedToolSkipsConfig`, all four Glow cases in `TestManageAppliedFieldsRoundTrip`, and `TestStandaloneConfigScopedToOpenedTool`; all expect `.config/glow/glow.yml`, which was not created. The install-flow test invokes the real config worker and tolerates TPM/Neovim clone failures (`internal/ui/install_flow_test.go:38-100`), producing real network/clone attempts rather than injected writers. A focused race run passed, but still took 73.066 seconds: `go test -race ./internal/ui -run 'Test(ConcurrentNewAppNoRace|ConcurrentSetThemeNoRace|ManageSaveUsesSnapshotDuringConcurrentMutation|StandaloneConfigApplyDeepSnapshotNoRace)$'`.
- **Impact:** The default package gate cannot pass, is slow enough to discourage iteration, and mixes unit assertions with network/filesystem side effects. Release regressions cannot be separated from environmental noise.
- **Recommendation:** Inject every generator, package manager, clone, filesystem root, and clock. Make install-plan tests pure. Keep a small explicit integration suite behind a tag. Update Glow's canonical path once in the generator contract and assert the contract rather than duplicating path literals.
- **Refutation:** Temp HOME protects the developer's real files and race tests pass. Hermetic home paths do not remove network clones or stale expected paths, and the default command remains red. Finding stands.

## Medium findings

### M-01 — Installed-state detection failures appear as “not installed”

`installCacheDoneMsg` carries only a boolean map (`internal/ui/messages.go:48-51`); batch failure returns nil and individual probes collapse errors to false (`internal/ui/cache.go:31-39,87-109`). There is no `unknown` or error provenance. Recommend a typed result and a visible “detection unavailable” retry state. Refutation: individual fallback reduces false negatives, but cannot distinguish a failed probe from a genuine miss.

### M-02 — Deep Dive mouse wheel cannot select Continue

Keyboard permits index `len(items)` (`internal/ui/screen_deepdivemenu.go:78-90`), while wheel-down stops at `len(items)-1` (`internal/ui/screen_deepdivemenu.go:111-121`). Recommend a shared clamp over the same item model. Refutation: clicking or keyboard can reach Continue; that does not make wheel behavior consistent.

### M-03 — Glow exposes invalid/inconsistent pager vocabulary

The wizard offers `auto`, `less`, `more`, `none` and passes the selection directly (`internal/ui/screen_config_glow.go:31-53,81-89`; `internal/ui/config_apply.go:260-267`). The normalization helper documents generator vocabulary and maps `more/none`, but it is used only in Manage translation (`internal/ui/config_apply.go:371-384`). Recommend one canonical enum at the model boundary and a test for every displayed value. Refutation: Manage's `auto/less/never` path is valid; the wizard path is not normalized.

### M-04 — “Add alias” is a dead-end feature

Hotkeys starts alias entry and incorrectly pre-fills the command from a key chord (`internal/ui/screen_hotkeys.go:248-260`), then only writes the map to hotkeys JSON (`internal/ui/screen_hotkeys.go:821-837`). No UI renderer, delete/edit flow, shell generator, or command consumer reads those aliases, while the footer advertises `a add alias` (`internal/ui/screen_hotkeys.go:865-871`). Recommend either a complete custom-cheatsheet feature or a validated managed zsh-alias feature; remove the action until one exists. Refutation: persistence succeeds, but storage without any consumer is not feature completion.

### M-05 — Hotkeys reads global configuration every animation frame

`View` calls `refreshHotkeysCurrentUser` (`internal/ui/screen_hotkeys.go:451-462`), which loads config from disk (`internal/ui/screen_hotkeys.go:693-707`). The global animation loop runs every 80 ms indefinitely while animations are enabled (`internal/ui/app.go:22-26,508-525,890-900`). Recommend cache invalidation on user switch/config save or an mtime watcher. Refutation: it is only one read per frame rather than per row; approximately 12.5 reads/second while idle is still unnecessary.

### M-06 — Decorative animation looks like permanent work and drives idle redraws

Manage prepends a spinner to its static “Dual-pane config editor” subtitle (`internal/ui/manage_dualpane.go:632-644`); Hotkeys does the same (`internal/ui/screen_hotkeys.go:852-862`). `uiTick` re-arms forever at 80 ms (`internal/ui/app.go:890-900`). On hardware the header appeared to be continually loading after status detection completed. Recommend reserve spinners for actual pending operations, slow purely decorative shimmer substantially, stop ticks when no active animation, and honor reduced-motion/no-animation during first launch. Refutation: users can disable animations; default idle UI should still communicate state semantically and avoid needless CPU/battery work.

### M-07 — User profiles are local preference presets, not enterprise users, and their UX is inconsistent

Corrupt profiles are silently omitted (`internal/ui/screen_users.go:92-121`); new users default to Linux keyboard even on macOS (`internal/ui/screen_users.go:325-343`); successful switch reloads the list but does not explicitly update the live App palette/navigation state (`internal/ui/screen_users.go:175-185,306-313`). The settings pane says Enter switches user although Enter cycles a field there (`internal/ui/screen_users.go:876-882`), and the status says `q:back` while `q` quits (`internal/ui/screen_users.go:250-260,892-904`). The user list has no viewport (`internal/ui/screen_users.go:752-789`). Recommend rename to “Profiles,” apply the profile live, surface partial-load errors, choose platform defaults, fix help, and define enterprise policy/identity separately. Refutation: `ApplyUserProfile` persists the selected profile, but the current process and semantics remain unclear.

### M-08 — Navigation-style selection promises behavior the app does not implement

The picker markets Vim modal navigation and Emacs/Mac line editing (`internal/ui/screen_navpicker.go:110-130`), but most screens accept both arrows and `j/k` regardless of the selection, and the Manage editor supports arrows/home/end rather than the advertised editing models (`internal/ui/screen_manage.go:131-201`). The selected value mainly changes hotkey categories (`internal/ui/screen_hotkeys.go:674-690`). Recommend either implement a centralized keymap/modal editing abstraction or rename the setting to the narrower cheatsheet/key-label preference. Refutation: accepting both styles is friendly, but then the selection is not a meaningful navigation mode.

### M-09 — Global theme/style state prevents true per-App isolation

The screen context claims to replace App coupling but stores `app *App` and most handlers mutate the large App directly (`internal/ui/screen.go:42-82`; `internal/ui/app.go:158-354`). Theme tokens/styles are package globals (`internal/ui/styles.go:72-148`); tests cover concurrent writes but not concurrent render reads. Recommend per-App immutable theme tokens passed in render context and screen-local state/reducers. Refutation: a CLI normally runs one App, but tests, embedding, future multi-session/mock-enterprise control, and maintainability benefit from actual isolation.

### M-10 — Legacy cleanup deletes filenames without proving ownership

Every utility installation calls cleanup (`internal/ui/installation.go:514-515`), which removes four legacy-looking paths, including `/usr/local/bin`, based only on existence and silently ignores errors (`internal/ui/installation.go:652-681`). Recommend delete only manifest-recorded or hash/version-verified assets, show the plan, and require confirmation for system paths. Refutation: names are project-specific, but a filename is not an ownership proof.

### M-11 — Binary copy does not propagate destination close/durability errors

`copyFile` defers destination close and returns only `io.Copy` error (`internal/ui/installation.go:631-649`); the caller can rename without detecting a close-time/disk error (`internal/ui/installation.go:598-628`). Scripts check close but neither path fsyncs file/directory. Recommend close/sync before rename and propagate all errors. Refutation: rename is atomic for visibility, not proof the bytes were successfully flushed.

### M-12 — Accessibility depends heavily on color and Nerd Font glyphs

Tabs and numerous headers use private-use Nerd Font icons without capability fallback (`internal/ui/styles.go:418-465`), while status uses colored dots throughout. Legends and text badges help on some screens, but there is no ASCII/no-color/low-color mode or contrast validation for all 16 themes. Recommend ASCII fallback, `NO_COLOR`/terminal capability support, selected-state text independent of color, and automated WCAG-like contrast/readability checks for the palette. Refutation: this is a terminal-tool audience likely to have Nerd Fonts, but first-run and remote/mock-enterprise terminals cannot assume them.

## Low/documentation findings

### L-01 — Package documentation is materially stale

`internal/ui/AGENTS.md:18-47` reports old line counts (for example app ~980 versus 1,191; deps ~210 versus 32; total ~17,400 versus 23,433) and references nonexistent `animation.go`/`generateLogo` at `internal/ui/AGENTS.md:40,97-105`. It says App handles two global messages (`internal/ui/AGENTS.md:8-10,128-129`), while streaming terminal messages are also global (`internal/ui/app.go:915-939`). Its one-subprocess performance claim is contradicted by fallback probes. Update from generated inventory and document operation locality/ownership rules.

### L-02 — Migration/prototype commentary obscures current architecture

There are 188 source/test matches for “legacy,” “migration/migrated,” `FIX N`, or `Task N`. `internal/ui/screen.go:5-13,75-77` still describes an incremental migration while `internal/ui/AGENTS.md:5-14` declares it complete. Large comments often narrate old bug IDs instead of current invariants. Preserve rationale in ADRs/tests and make production comments explain the present contract.

### L-03 — Naming/config models contain drift signals

The typo `GhosstyCursorStyle` is a public field throughout Manage (`internal/ui/manage_config.go:10,121`; `internal/ui/config_apply.go:418`). DeepDiveConfig and ManageConfig duplicate overlapping settings and require manual translation plus a separate changed-field list (`internal/ui/config_apply.go:387-406`; `internal/ui/manage_save_scope.go:14-80`). Consolidate around typed tool-owned schemas/adapters and migrate persisted JSON keys compatibly.

## Settings exposure and apply matrix

This matrix compares **modeled inputs in the current code**, not the full upstream application's configuration surface. The key systemic issue is that generator application is usually broader than the fields visible in either dashboard, while no real-file import establishes safe starting values.

| Tool | Modeled inputs consumed by writer | Deep Dive exposes | Manage exposes | Missing/high-risk gap | Apply reality |
|---|---:|---|---|---|---|
| Ghostty | 9 | 7: family, size, opacity, blur, scrollback, cursor, tab bindings | 8: all except tab bindings; adds decorations/confirm close | No one screen exposes all 9; real local file had many unmodeled upstream keys | Whole Ghostty config writer receives only 9 modeled values; unknown keys are lost. |
| tmux | 16 | 13: prefix/splits/status/mouse/history/escape/base/TPM/plugins/interval | 15: all modeled except split bindings | Wizard lacks border/aggressive resize/auto-restore; Manage lacks split keys | Writer owns full modeled tmux file; Manage can reset its hidden split binding to default. |
| zsh | 10 concepts | Prompt, history, AutoCD, two plugin toggles | History + six booleans | Manage omits prompt/plugin list/aliases; wizard omits aliases/ignore-dups/correction/completion; arbitrary shell content is unmodeled | Full zsh writer consumes all ten and may reset hidden fields/default aliases. |
| Neovim | 10 | Preset, tab/wrap/cursor/clipboard, six LSP toggles | Seven editor options | Wizard omits plugin list/line-number/expand-tab/undo; Manage omits preset/LSP/plugins | Manage uses a user-options overlay (safer); installer can clone/move presets and needs separate ownership consent. |
| Git | 9 | Delta-side, branch, rebase, sign, credential | Branch, auto-remote, rebase, diff, merge, credential, sign | Aliases/delta are split; identity, signing key, LFS, includes, arbitrary sections are absent | Whole `.gitconfig` writer is called; highest-risk model gap. `store` is also offered without a plaintext warning. |
| Yazi | 7 | Keymap, hidden, preview | Hidden, sort/reverse, line mode, scroll offset | No one screen exposes all seven | Writer generates all modeled Yazi outputs; backup captures only `yazi.toml`, not keymap/theme. |
| FZF | 6 | Preview, height, layout | All 6 | Deep Dive omits default opts/border/preview window | Writer applies all six; backup omits its output. |
| LazyGit | 4 | Side-by-side, mouse, theme | All 4 | Wizard omits paging | Full modeled file writer; only optional selection gates installer application. |
| LazyDocker | 2 Manage preferences (1 unused field in DeepDive model) | Install checkbox only; no settings screen | Mouse + log tail | No writer exists | UI labels both “not applied,” but editable dead preferences should be removed until supported. |
| Btop | 6 | Theme, update rate, temperature, graph | All 6 | Wizard omits scale/shown boxes | Full modeled file writer; backup omits output. |
| Glow | 4 | Style, pager, width | All 4 | Wizard pager vocabulary differs; Manage is broader | Writer applies all four, but current package tests fail because path expectations and output diverge. |
| Claude Code | 7 MCP toggles plus install selection | Install + all MCPs | All MCPs, no install control | Auth/provider/security/provenance not modeled; theme should not trigger MCP writes | Separate gated apply, but the non-empty map makes all-false maps executable; theme path includes it. |

Evidence anchors: modeled fields at `internal/ui/deepdive.go:26-151` and `internal/ui/manage_config.go:3-108`; Manage controls at `internal/ui/manage_dualpane.go:466-627`; writer translations at `internal/ui/config_apply.go:123-267`; generator dispatch at `internal/ui/config_apply.go:270-349`; changed-field coupling at `internal/ui/manage_save_scope.go:14-80`.

### Recommended settings information architecture

1. Show a read-only **Current detected** summary first, including source path and detection confidence.
2. Offer a short **Essentials** set: Ghostty font/opacity/keybindings; tmux prefix/mouse/session behavior; zsh prompt/history/plugins; Neovim ownership/preset plus editor basics; Git identity/credential/signing safety; Yazi/FZF/LazyGit basic behavior.
3. Put safe but less common options under **Advanced**, grouped by upstream concept rather than dumping every key.
4. Mark every setting as `Managed`, `Inherited`, `Unmanaged`, or `Unsupported`; show which exact file/block will change.
5. Provide “Open upstream config,” “Preview diff,” and “Reset product-managed section,” not “replace my whole file.”
6. Hide or remove settings with no generator (LazyDocker) and values invalid for the installed version.
7. For AI agents, use a shared flow: install source/version → authentication status (never secret values) → provider/model defaults → permissions/sandbox → integrations/MCP → update channel. Product-specific advanced adapters can follow.

## Screen-by-screen aesthetic and UX assessment

| Screen/area | What works | What blocks polish/release |
|---|---|---|
| Intro animation | Distinctive branding, deterministic visual language, skippable path | Motion is enabled by default; hard-coded DOTFILES banner complicates rename; capability/reduced-motion fallback is weak. |
| Welcome | Clear Quick vs Deep Dive decision; narrow mouse split has regression tests | “Fully reversible” promise is false with current backup coverage/fail-open behavior. |
| Theme picker | Useful wide preview and cohesive palettes | Sixteen-row list has no viewport; changing theme through Manage has destructive cross-tool semantics; contrast is unverified. |
| Navigation picker | Simple two-choice hierarchy | Choice overpromises modal/Emacs behavior that most controls do not honor. |
| Deep Dive menu | Categories and status legend scan well | Full unscrolled menu; wheel cannot reach Continue; statuses are slow and semantically binary/partial only. |
| Per-tool config screens | Strong visual consistency, shared field geometry, broad mouse tests | Long screens clip; no explicit save/cancel/diff; settings are split differently between wizard and Manage. |
| File Tree confirmation | Tree metaphor is appropriate for a mutation preview | Data is independently reconstructed and inaccurate; create/replace/merge and backup truth are absent. |
| Progress | Streaming/cancellation work is comparatively mature; failure aggregation exists | Static “core” phases are not tied to the confirmed plan; backup failure is nonfatal. |
| Error/Summary | Clear color/status contrast and retry affordance | Skip leads to unconditional success; next steps and backup claims are not outcome-derived. |
| Main menu | Clean, understandable top-level destinations | No visible distinction between safe read-only pages and destructive/configuration actions. |
| Manage | Best product concept: searchable mental model, tool/install badges, fields, logs, keyboard/mouse | Slow blocking load, wrong source of truth, destructive saves, dense 80x24 layout, clipped help, truncated names, permanent spinner. |
| Users | Two-pane structure and empty state are understandable | These are profiles, not users; contradictory help, no scrolling, incomplete live apply, dropped async results. |
| Hotkeys | Favorites/filter/category hierarchy are useful | Alias feature has no consumer; disk read each frame; icon dependence and dense footer. |
| Update | Package selection and streaming log model are useful | Check result locality, duplicate-start window, no operation ID, long unviewported result lists. |
| Backups | Confirmation and partial-restore reporting are thoughtful | Fixed incomplete capture list, no viewport, and operations/load can outlive their handler and lose results. |
| Global tab bar | Consistent location and numeric shortcuts | Five full icon+name pills do not adapt; labels/coordinates overflow on narrow terminals. |

## Architecture and long-term strategy

### Keep the repository; refactor around contracts

Starting a new repository is **not justified for the current local/friends-and-family objective**. This package already has 23,256 Go lines, extensive behavior tests, mature cancelable streaming work, and useful UI primitives. A rewrite would discard hard-won edge-case knowledge while leaving the difficult problems—config ownership, detection provenance, package planning, rollback, platform variance—unchanged.

The recommended seam is an in-place v3 architecture:

1. **Inventory/Detection:** typed per-component observations with provenance and partial states.
2. **Desired State:** versioned user/policy intent, separate from detected files and compiled defaults.
3. **Planner:** pure reconciliation into an immutable, reviewable action graph.
4. **Ownership/Backup:** generator-owned files or marked/include fragments, mutation manifest, verified rollback.
5. **Executor:** operation IDs, cancellation, transactional/atomic file actions, structured results.
6. **Dashboard:** renders observations/plans/results and never reconstructs execution independently.
7. **Tool descriptors:** registry-driven install/detect/config/import/apply/UI metadata; custom panels only for exceptional tools.

Create a new repository only if the product becomes a separate multi-host control plane/service with remote agents, tenant identity, policy distribution, secrets management, fleet inventory, and audit/event storage. That is a different product boundary; it should consume this repo's local agent/library rather than replace it.

### Mock-enterprise caution

The current “Users” feature is local profile selection, not isolation, RBAC, tenancy, device policy, or an audit trail. Mock enterprise testing should not present it as such. Before that phase, define managed-vs-user policy precedence, noninteractive plan/apply, deterministic exit codes, immutable audit output, proxy/offline package behavior, secrets boundaries, and rollback across multiple local accounts.

## Rename difficulty score

**UI/dashboard-specific difficulty: 7/10 (moderately high).**

Within production UI code alone there are 89 case-insensitive `dotfiles` matches (including module imports), plus user-facing/path/behavior coupling: the DOTFILES animation (`internal/ui/screen_animation.go:212-213`), `~/.config/dotfiles` messaging and Manage persistence contract (`internal/ui/manage_dualpane.go:27-33`; `internal/ui/screen_summary.go:61-65`), installed binary name (`internal/ui/installation.go:529-531`), File Tree labels (`internal/ui/screen_filetree.go:175-224`), stderr prefix (`internal/ui/config_apply.go:43-50`), temp-file prefixes, backup paths, CLI examples, tests, and raw tool/screen identifiers.

The visual label is easy; the compatibility contract is not. A safe rename needs a centralized product identity package, new binary plus deprecated alias/shim, config-directory migration with rollback and dual-read/one-write rules, Homebrew/formula changes, module/import decision, backup compatibility, CLI completion/docs updates, and tests for upgrades from every supported old layout. Do not combine this migration with the config-ownership rewrite in one irreversible step.

## Release gates and recommended sequence

### Gate A — safe local testing

- Fix C-01 install planning and derive confirmation/progress/summary from it.
- Add real-file import/provenance and managed ownership boundaries; no whole-file overwrite by default.
- Make every destructive apply diffable, backed up, and fail closed.
- Disable theme-wide regeneration until managed ownership exists.
- Make `go test ./internal/ui` green and hermetic.

### Gate B — primary hardware testing

- Test clean Mac plus the owner's preconfigured Mac at 60x18, 80x24, and 120x40.
- Verify repeat runs are idempotent byte-for-byte outside managed regions.
- Inject backup, disk-full, permission, package-manager, network, and cancellation failures.
- Prove restore reverses every planned mutation.
- Capture plan/apply results and timings; target progressive dashboard availability under two seconds.

### Gate C — friends and family

- Add versioned config migrations and backward-compatible binary/config paths.
- Add explicit unsupported-platform/tool states and actionable error guidance.
- Remove incomplete actions (alias, LazyDocker settings) or finish them.
- Ship an opt-in diagnostic bundle with secrets redaction.

### Gate D — mock enterprise

- Define policy, identity, privilege, audit, proxy/offline, secrets, and fleet boundaries separately from local profiles.
- Add noninteractive plan/apply/export interfaces and deterministic machine-readable results.
- Test macOS/Linux/Pi images and restricted accounts without assuming Nerd Fonts, interactive sudo, internet, or writable standard paths.

## Adversarial check ledger

| Finding | Strongest attempted refutation | Result |
|---|---|---|
| C-01 | Core tools might be installed in an implicit utility/config phase | No package-manager call exists there; only writers/clones run. Confirmed. |
| C-02 | Scoped saves prevent clobbering | They scope by tool, not by keys/blocks within the tool; actual file is never imported. Confirmed. |
| C-03 | All generators need theme refresh | Claude does not; installed/managed status and whole-file ownership are ignored. Confirmed. |
| C-04 | Backup warnings are honest | Honest warning does not provide recovery; execution continues and manifest omits outputs. Confirmed. |
| H-03 | Global async dispatch already solved navigation loss | Only selected streaming messages are global; backup/check/user messages remain local. Confirmed. |
| H-04 | Snapshot race test proves safe saves | It proves memory isolation for one command, not ordering or correct baseline. Confirmed. |
| H-05 | Batch enumeration should make detection cheap | Every miss still invokes custom `IsInstalled`; real timing corroborates. Confirmed. |
| H-07/H-08 | Lipgloss width/height and scroll clamps make layouts responsive | Oversized content wraps/overflows; live 80x24 output clips controls and tests omit/skip the case. Confirmed. |
| H-10 | Registry is the source of truth | Defaults are registry-driven, visible ordered lists/routes are not. Confirmed. |
| H-13 | Temp HOME makes tests hermetic | It protects real files but real clones/network and stale paths remain; default tests fail. Confirmed. |

## Verification executed

- `go test ./internal/ui` — **FAIL**, 51.265 s; Glow path/output drift across four top-level tests/seven subcases.
- Focused race command for theme, Manage snapshot, and standalone deep snapshot — **PASS**, 73.066 s.
- Audit-team interactive 80x24 run — Manage footer clipped, installed-count subtitle wrapped, names truncated, and initial detection observed at ~13–22 seconds.

No production source files were modified during this audit.

## Exhaustive coverage manifest

All 87 tracked files under `internal/ui/` were read in full. Total: **23,433 lines** (23,256 Go/source-test lines plus 177 documentation lines).

| File | Lines |
|---|---:|
| `internal/ui/AGENTS.md` | 177 |
| `internal/ui/app.go` | 1191 |
| `internal/ui/backup_count_collision_test.go` | 113 |
| `internal/ui/backups_restore_test.go` | 54 |
| `internal/ui/cache.go` | 216 |
| `internal/ui/cache_test.go` | 84 |
| `internal/ui/components.go` | 9 |
| `internal/ui/config_apply.go` | 523 |
| `internal/ui/config_apply_race_test.go` | 69 |
| `internal/ui/config_apply_test.go` | 98 |
| `internal/ui/config_builders_test.go` | 363 |
| `internal/ui/config_footer_test.go` | 252 |
| `internal/ui/config_mouse_test.go` | 369 |
| `internal/ui/copyfile_test.go` | 122 |
| `internal/ui/deepdive.go` | 421 |
| `internal/ui/deps.go` | 32 |
| `internal/ui/deps_test.go` | 15 |
| `internal/ui/hotkeys_cache_test.go` | 194 |
| `internal/ui/hotkeys_migration_save_test.go` | 125 |
| `internal/ui/input_mouse.go` | 48 |
| `internal/ui/install_flow_test.go` | 261 |
| `internal/ui/install_pi_packages_test.go` | 51 |
| `internal/ui/installation.go` | 1030 |
| `internal/ui/main_test.go` | 33 |
| `internal/ui/manage_config.go` | 224 |
| `internal/ui/manage_dualpane.go` | 1237 |
| `internal/ui/manage_field_apply.go` | 50 |
| `internal/ui/manage_field_apply_test.go` | 287 |
| `internal/ui/manage_install_cancel_test.go` | 71 |
| `internal/ui/manage_save_race_test.go` | 154 |
| `internal/ui/manage_save_scope.go` | 169 |
| `internal/ui/manage_save_scope_test.go` | 157 |
| `internal/ui/messages.go` | 162 |
| `internal/ui/regression_test.go` | 423 |
| `internal/ui/screen.go` | 145 |
| `internal/ui/screen_animation.go` | 275 |
| `internal/ui/screen_backups.go` | 492 |
| `internal/ui/screen_config_base.go` | 573 |
| `internal/ui/screen_config_btop.go` | 121 |
| `internal/ui/screen_config_claudecode.go` | 211 |
| `internal/ui/screen_config_clitools.go` | 136 |
| `internal/ui/screen_config_cliutilities.go` | 119 |
| `internal/ui/screen_config_fzf.go` | 95 |
| `internal/ui/screen_config_ghostty.go` | 164 |
| `internal/ui/screen_config_git.go` | 132 |
| `internal/ui/screen_config_glow.go` | 110 |
| `internal/ui/screen_config_guiapps.go` | 117 |
| `internal/ui/screen_config_lazygit.go` | 94 |
| `internal/ui/screen_config_macapps.go` | 115 |
| `internal/ui/screen_config_neovim.go` | 168 |
| `internal/ui/screen_config_tmux.go` | 306 |
| `internal/ui/screen_config_utilities.go` | 114 |
| `internal/ui/screen_config_yazi.go` | 98 |
| `internal/ui/screen_config_zsh.go` | 179 |
| `internal/ui/screen_deepdivemenu.go` | 289 |
| `internal/ui/screen_error.go` | 103 |
| `internal/ui/screen_factory.go` | 109 |
| `internal/ui/screen_filetree.go` | 265 |
| `internal/ui/screen_golden_test.go` | 2280 |
| `internal/ui/screen_hotkeys.go` | 1153 |
| `internal/ui/screen_hotkeys_test.go` | 83 |
| `internal/ui/screen_mainmenu.go` | 198 |
| `internal/ui/screen_manage.go` | 728 |
| `internal/ui/screen_manager.go` | 112 |
| `internal/ui/screen_manager_test.go` | 238 |
| `internal/ui/screen_navpicker.go` | 150 |
| `internal/ui/screen_progress.go` | 335 |
| `internal/ui/screen_summary.go` | 103 |
| `internal/ui/screen_test.go` | 164 |
| `internal/ui/screen_themepicker.go` | 232 |
| `internal/ui/screen_update.go` | 513 |
| `internal/ui/screen_users.go` | 905 |
| `internal/ui/screen_users_tiny_test.go` | 142 |
| `internal/ui/screen_welcome.go` | 198 |
| `internal/ui/screens_deepdive.go` | 314 |
| `internal/ui/standalone_config_scope_test.go` | 58 |
| `internal/ui/standalone_mouse_test.go` | 652 |
| `internal/ui/state_helpers.go` | 114 |
| `internal/ui/streaming.go` | 214 |
| `internal/ui/styles.go` | 472 |
| `internal/ui/sudo_keepalive.go` | 72 |
| `internal/ui/sudo_keepalive_test.go` | 175 |
| `internal/ui/task16_misc_mediums_test.go` | 97 |
| `internal/ui/theme_race_test.go` | 50 |
| `internal/ui/toolscreens.go` | 94 |
| `internal/ui/toolscreens_test.go` | 97 |
| `internal/ui/widget_globe.go` | 176 |
