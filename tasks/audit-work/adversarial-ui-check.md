# Adversarial verification of the primary UI/dashboard audit

Date: 2026-07-09
Primary report checked: `tasks/audit-work/primary-ui-dashboard.md`
Scope: every one of the primary report's 32 findings, its settings matrix,
architecture/new-repository recommendation, and rename estimate

## Executive verdict

The primary UI audit is directionally strong and its no-go release conclusion is
correct. The most important call graphs—missing core package installation, default
rather than observed configuration state, theme-wide config application, fail-open
backup, screen-local async state, save ordering, install-status semantics, and
unviewported layouts—are real.

It is not fully accurate:

- `M-04` is refuted. Saved Hotkeys aliases have a real consumer in the Zsh
  generator and focused tests prove it.
- The settings matrix incorrectly describes Zsh as a full-file writer and counts
  the unused Neovim `Plugins` field as consumed.
- Several findings duplicate the independently audited tools/core defects and should
  not inflate the repository-wide total.
- The UI-specific rename occurrence count is 91, not 89, in current production Go
  files.
- The primary report missed a new critical failure: the dashboard bypasses custom
  `Tool.Install` methods. Claude Code therefore installs Node but not Claude Code.
- The report understates the navigation-style feature gap: selecting Emacs/Vim does
  not drive the Zsh/tmux/Yazi generators and can contradict their compiled defaults.

Verdicts for the primary report's 32 findings:

| Verdict | Count |
|---|---:|
| Confirmed | 22 |
| Partial | 4 |
| Refuted | 1 |
| Duplicate | 5 |
| **Total checked** | **32** |

After removing duplicates/refuted claims, correcting severities, and adding three
omissions, this lane retains **29 findings**:

| Corrected severity | Count |
|---|---:|
| Critical | 3 |
| High | 11 |
| Medium | 7 |
| Low | 8 |
| **Total** | **29** |

The release remains a **no-go** for friends/family and mock-enterprise use. The new
custom-installer bypass must be fixed alongside the primary critical planning and
configuration-ownership failures.

## Verification method

For each finding I followed the reachable chain from screen input/`Init`, through
Bubble Tea commands/messages, App state reduction, package/config operations, and
tests. I cross-checked the tools/core primary and adversarial reports to distinguish
UI-specific reachability from shared defects.

Safe focused diagnostics executed:

```text
go test ./internal/tools -run \
  'TestGenerateZshConfigEmitsSavedHotkeyAliases|TestWriteSavedAliasesQuotesAndFiltersUnsafeNames' \
  -count=1
PASS (0.435s)

go test ./internal/ui -run \
  'Test(BatchInstalledPackages|ManageScreenGolden|ManageSaveUsesSnapshotDuringConcurrentMutation|ScreenManager)' \
  -count=1
PASS (0.273s)
```

The full macOS UI-suite failure was not needlessly repeated; both the UI primary and
tools audit independently reproduced the same stale Glow path expectation. Static
inspection confirmed the UI integration tests also invoke the real Neovim preset
clone path.

## Per-finding verdicts

### Critical findings

| ID | Verdict | Corrected severity | Independent evidence/correction |
|---|---|---:|---|
| `C-01` | **CONFIRMED** | Critical | `startInstallation` derives package work only from `collectSelectedTools` (`internal/ui/installation.go:53-65`). That collector iterates only `CLITools`, `GUIApps`, `CLIUtilities`, and macOS apps (`installation.go:684-724`); Ghostty, tmux, Zsh, Neovim, Git, Yazi, and FZF are not in those group maps. The sole package loop consumes only this slice (`installation.go:238-318`), while all seven are configured later (`installation.go:340-433`). `install_flow_test.go:42-45,221-260` explicitly codifies them as configuration-only “always core.” A clean machine can finish without core executables. |
| `C-02` | **DUPLICATE** | Critical in canonical tools/core findings | The UI reachability is confirmed: `NewApp` loads global JSON and `manage.json`, never live application config (`internal/ui/app.go:422-463`); standalone derives from the same desired model (`app.go:1020-1030`); writers are invoked at `config_apply.go:287-318`. But the destructive within-tool ownership defect is already canonical as tools `F01/F02` and adversarial core `CORE-M05`. UI scoping protects other tools, not unknown keys inside the selected writer. Do not count this again repository-wide. |
| `C-03` | **CONFIRMED** | Critical | A theme difference returns all ten generator IDs plus Claude (`internal/ui/manage_save_scope.go:117-132`); `applyChangedManageTools` executes all with no installed/managed check (`manage_save_scope.go:147-163`). Manage saves desired/global JSON then real files without backup (`manage_dualpane.go:95-146`). The Git generator reachability is direct (`config_apply.go:300-302`), and `manage_save_scope_test.go:73-85` locks in all-tool application. Claude is especially unjustified: the seven-key map is nonempty even when all values are false, so theme-only save calls MCP mutation (`config_apply.go:320-327`). |
| `C-04` | **DUPLICATE** | High | The facts stand: backup failure is only logged (`internal/ui/installation.go:201-219`), execution proceeds, and `defaultBackupFiles` has only six entries (`app.go:735-745`) despite a larger mutation surface. This is the same canonical defect as adversarial core `ADV-CORE-H01` plus `CORE-M02`, and the tools whole-file writer finding. Severity is High as a recoverability failure; the actual destructive writes are already Critical elsewhere. |

### High findings

| ID | Verdict | Corrected severity | Independent evidence/correction |
|---|---|---:|---|
| `H-01` | **CONFIRMED** | High | The worker intentionally runs utilities plus tmux/Ghostty/Zsh/Neovim/Git/Yazi/FZF configuration even when no tool is selected (`internal/ui/installation.go:234-237,320-433`). This is distinct from `C-01`: one omits package installation; this one mutates unwanted/absent tools without selection or ownership. |
| `H-02` | **CONFIRMED** | High | File Tree reconstructs selection maps independently (`internal/ui/screen_filetree.go:87-142`) instead of rendering execution actions. It statically labels paths new/backed-up (`screen_filetree.go:175-211`), always lists the installed binary (`:224`), omits core package actions, and cannot distinguish merge/replace/create. It is materially different from the worker plan. |
| `H-03` | **CONFIRMED** | High | `ScreenManager.Update` delegates every non-navigation message only to the current handler (`internal/ui/screen_manager.go:77-102`). App-level handling covers install cache and selected streaming messages only (`app.go:903-939`). Backup results remain local (`screen_backups.go:73-137`) while navigation/quit stays available (`:60-68,178-185,230-235`); update-check results are local (`screen_update.go:92-99`) while tabs remain available (`:132-144`); user results are local (`screen_users.go:266-315`) while `usersLoaded` is set before completion (`:226-235`). The primary missed that `manageSavedMsg` is also local (`screen_manage.go:105-115`) and tab/Esc/q remain available after launching the async save (`screen_manage.go:334-363`). A completed save can therefore lose its baseline/status reducer too. |
| `H-04` | **CONFIRMED** | High | `saveManageConfigCmd` captures a value but has no single-flight/generation ID (`internal/ui/manage_dualpane.go:95-146`). Every success snapshots the then-current live config, not the persisted snapshot (`screen_manage.go:105-114`). The existing race test proves memory isolation for one command (`manage_save_race_test.go:9-65`), not completion order. Two writes can finish B then A while both completions baseline the live B. |
| `H-05` | **CONFIRMED** | Medium | Batch enumeration is followed by `t.IsInstalled()` for every negative (`internal/ui/cache.go:75-110`; synchronous duplicate at `:151-190`). For BaseTool negatives this repeats package-manager probes; custom apps may perform multiple binary/Flatpak/AppImage/desktop/bundle probes. Fallback is necessary for external installs, so the fix must classify probe strategy rather than blindly trust every negative. The 13–22 second observed latency is credible, but this is performance/availability rather than data loss, so Medium is more proportionate. |
| `H-06` | **DUPLICATE** | High in canonical tools finding | `map[string]bool`, all-package bundle semantics, and INSTALLED/NOT INSTALLED rendering are confirmed (`internal/ui/cache.go:59-72,79-125`; `manage_dualpane.go:817-824`). The same root and local Zsh example are tools `F06`; merge there rather than count twice. |
| `H-07` | **CONFIRMED** | High | Manage fixes header/footer to 3/2 rows (`internal/ui/manage_dualpane.go:224-262`) but the footer is an untruncated 119-character line (`:647-682`), the left pane bottoms out near 26 columns (`:237-243`), the installed subtitle can wrap (`:699-713`), and category tags consume name width (`:742-772`). `RenderTabBar` constructs five full pills then applies width (`styles.go:418-465`), which wraps rather than provides a compact form. Existing 80x24 tests assert substrings, not line/height bounds (`screen_golden_test.go:1453-1488,1545-1575`). The reported clipping is consistent with the render math. |
| `H-08` | **CONFIRMED** | High | Deep Dive centers every row plus headers/footer (`screen_deepdivemenu.go:146-288`), Theme Picker renders all themes, and long Neovim/tmux screens have no vertical viewport. File Tree, Backups, Update, and Users likewise render unbounded lists. `config_mouse_test.go` skipping nonvisible labels does not prove visibility. Focus can move to off-screen rows at 80x24. |
| `H-09` | **CONFIRMED** | High | Enter/`a` return an async sudo/start command while `updateRunning` stays false (`internal/ui/screen_update.go:163-203`); it becomes true only on the later `updateStartMsg` (`streaming.go:55-65`). Multiple key messages can enqueue overlapping package operations before that message arrives. An operation ID and synchronous pending state are absent. |
| `H-10` | **CONFIRMED** | High | Desired defaults are registry-driven (`internal/ui/deepdive.go:8-23,264-281`), but group screens use literal inventories (`screen_config_clitools.go:24-50`, `screen_config_cliutilities.go:18-41`, `screen_config_guiapps.go:18-39`, `screen_config_macapps.go:18-37`). This is worse than ordinary maintenance drift: a newly registered default-enabled AI tool enters the hidden map and `collectSelectedTools` installs it even if the literal screen never shows it. With the planned default-on agents, that is a consent defect, so High stands. |
| `H-11` | **CONFIRMED** | Medium | Summary is unconditional “Installation Complete,” static backup path, and static Zsh/tmux/Neovim/p10k/hk steps (`internal/ui/screen_summary.go:52-81`). Error `s` routes there (`screen_error.go:34-49`). The user did explicitly choose Skip after seeing the error, so Medium is more proportionate than High, but the resulting green outcome is still false. |
| `H-12` | **CONFIRMED** | High | Shared help says `enter/esc back`, but standalone `back()` applies and quits for either (`internal/ui/screen_config_base.go:56-103`); `q` quits without saving. Claude duplicates it (`screen_config_claudecode.go:50-61,82-98`). Because several writers replace full files and errors are only printed to stderr before quit (`config_apply.go:13-52`), conventional Escape causes a high-impact unexpected save. |
| `H-13` | **PARTIAL** | High | The red macOS test root duplicates tools `F08`: tests hardcode `.config/glow/glow.yml` while production correctly uses `~/Library/Preferences/glow/glow.yml` (`install_flow_test.go:90-93`, `manage_field_apply_test.go:147-150`, `standalone_config_scope_test.go:43-45`; `internal/tools/glow.go:99-114`). The UI-specific non-hermetic point remains independent: `drainWorker` calls the real worker (`install_flow_test.go:12-24`), whose default Kickstart preset reaches real `git clone` when no `init.lua` exists (`internal/tools/neovim.go:338-360`). Retain one High UI test-architecture finding, but do not recount the Glow root. |

### Medium findings

| ID | Verdict | Corrected severity | Independent evidence/correction |
|---|---|---:|---|
| `M-01` | **DUPLICATE** | Merge into `H-06` | Error/unknown is collapsed to false because `installCacheDoneMsg` has only a bool map and batch failure returns nil (`internal/ui/messages.go:48-51`; `cache.go:26-39,87-109`). This is one missing structured-state contract, not a separate repository finding. |
| `M-02` | **CONFIRMED** | Medium | Keyboard permits index `len(items)` for Continue (`screen_deepdivemenu.go:78-90`); wheel-down stops at `len(items)-1` (`:109-121`). Click/keyboard alternatives do not make wheel behavior correct. |
| `M-03` | **PARTIAL** | Medium | The wizard exposes `auto/less/more/none` and passes it directly (`screen_config_glow.go:31-53,81-89`; `config_apply.go:260-267`). The generator's boolean check treats only `never` as off (`internal/tools/glow.go:70-71`), so `none` incorrectly enables paging and `more` is indistinguishable from `less`. The YAML itself remains schema-valid; “invalid” overstates the bug. Manage normalization does not repair the wizard path (`config_apply.go:371-384`). |
| `M-04` | **REFUTED** | Removed; replacement below | The assertion that no consumer exists is false. The UI persists `UserHotkeys.Aliases` (`screen_hotkeys.go:821-837`); Zsh loads the active user's hotkeys and emits sorted, validated, shell-quoted aliases (`internal/tools/zsh.go:143-164,291-326`). Both focused alias tests pass. The feature has UX/apply-timing defects, recorded as `ADV-UI-M01`, but it is not dead-end. |
| `M-05` | **CONFIRMED** | Low | Every Hotkeys `View` calls `refreshHotkeysCurrentUser` (`screen_hotkeys.go:451-462`), which reads global config (`:693-707`). The default global UI tick runs every 80 ms (`app.go:22-26,508-525,890-900`), producing roughly 12.5 reads/s while this screen is open. This is wasteful but bounded to one screen and user config, so Low. |
| `M-06` | **CONFIRMED** | Low | Manage/Hotkeys prepend a spinner even when no operation runs (`manage_dualpane.go:632-644`; `screen_hotkeys.go:852-862`), and `uiTickMsg` re-arms indefinitely while animations are enabled (`app.go:890-900`). It can imply loading and consumes redraw/battery, but users can disable animation, so Low. |
| `M-07` | **DUPLICATE** | Medium in canonical core finding | Corrupt-profile omission, Linux creation default, contradictory help, no viewport, and no live App theme/nav refresh are confirmed (`screen_users.go:92-121,325-343,306-313,451-467,876-904`). The central profile-vs-user semantics and `KeyboardStyle` no-op are already canonical as core `CORE-M03`; fold the UI manifestations there. |
| `M-08` | **CONFIRMED** | **High** | The picker promises Emacs line editing and Vim modal editing (`screen_navpicker.go:110-130`), but UI-wide searches show `a.navStyle` mainly affects Hotkeys categories and persistence; it is not fed into Zsh/tmux/Yazi builders. Worse, App defaults to `emacs` (`app.go:422-426`) while `NewDeepDiveConfig` defaults Yazi to `vim` (`deepdive.go:247-250`). Most screens accept arrows and j/k independent of selection. This is a central advertised feature that does not configure the terminal environment, so elevate to High. |
| `M-09` | **PARTIAL** | Low | Theme/style values are package globals (`styles.go:37-79,150-215`). `SetTheme` serializes writers but render reads are not under that lock (`styles.go:81-119`), so concurrent Apps/renders are not isolated. Production normally runs one Bubble Tea App, so the report's multi-session framing is prospective; retain as Low architectural/test debt. |
| `M-10` | **CONFIRMED** | Medium | Every utility install calls cleanup (`installation.go:514-515`), which removes four project-looking paths solely by basename/existence and ignores errors (`:652-681`). This is separate code from core uninstall's broader ownership defect, though the fix should share an install manifest. |
| `M-11` | **CONFIRMED** | Low | `copyFile` defers `destFile.Close` and returns only `io.Copy` error (`installation.go:631-650`); `installBinary` can then chmod/rename the candidate (`:598-628`). Close/fsync/directory durability failures are not propagated. The condition is rarer than the primary severity implies, so Low. |
| `M-12` | **PARTIAL** | Low | Nerd Font private-use icons and color-heavy state are real (`styles.go:418-465` and screen headers). However, several screens also use text badges/legends, animations can be disabled, and terminal color libraries may degrade output. No explicit ASCII glyph mode, contrast suite, or full no-color test exists. Retain the remote/first-run accessibility concern at Low rather than Medium. |

### Low/documentation findings

| ID | Verdict | Corrected severity | Independent evidence/correction |
|---|---|---:|---|
| `L-01` | **CONFIRMED** | Low | `internal/ui/AGENTS.md` line counts and architecture facts are stale: `app.go` is 1,191 lines, `deps.go` 32, `animation.go` does not exist, more than two messages are globally reduced, and the one-subprocess cache claim conflicts with `cache.go:105-109`. |
| `L-02` | **CONFIRMED** | Low | `screen.go:5-13,75-77` still narrates an incremental migration while UI instructions say it is complete; bug-ID history dominates several production comments. This is maintainability noise, not a runtime defect. |
| `L-03` | **CONFIRMED** | Low | `GhosstyCursorStyle` persists through model/translation (`manage_config.go:10,121`; `config_apply.go:418`). `DeepDiveConfig`, `ManageConfig`, and `toolDeepDiveFields` duplicate schemas and change detection (`config_apply.go:387-406`; `manage_save_scope.go:14-80`). Compatibility is needed before fixing persisted names. |

## New omissions

### ADV-UI-C01 — Dashboard installation bypasses custom tool installers

**Severity: Critical**

`ClaudeCodeTool.Install` is the only current custom installer. It installs Node and
then runs `npm install -g @anthropic-ai/claude-code`
(`internal/tools/claude_code.go:53-72`). The `claude` binary checked by
`IsInstalled` comes from that npm step (`claude_code.go:47-56`).

Neither dashboard path calls it:

- The wizard gets the tool from the registry, resolves only its package names, then
  directly calls `mgr.InstallStreaming(pkgs...)`
  (`internal/ui/installation.go:254-310`).
- Manage repeats the same bypass (`installation.go:733-777`).

For Claude, `pkgs` is only Node/npm. Selecting “Install Claude Code” can install Node,
report the tool phase successful, and apply MCP JSON while never creating `claude`.
Any planned Pi/Cursor/OpenCode/Hermes custom curl/npm installer would also be skipped.

**Fix:** the install planner/executor must dispatch the tool's install strategy, not
reconstruct installation from its package metadata. Extend the Tool contract with a
context-aware streaming install operation/result or model install actions in the
manifest. Add a regression tool whose custom install sentinel must execute through
both wizard and Manage.

### ADV-UI-M01 — Alias creation is consumed, but prefilled and activated misleadingly

**Severity: Medium**

The refuted primary finding hid the real UX defects:

- Pressing `a` pre-fills the shell command with `item.Keys`, a key chord such as a
  tmux shortcut, not an executable command (`screen_hotkeys.go:248-260`).
- Save only writes `hotkeys.json` (`screen_hotkeys.go:821-837`); it does not regenerate
  Zsh or tell the user when the alias becomes active.
- The alias is consumed only on the next Zsh generation/apply
  (`internal/tools/zsh.go:143-164,291-326`).
- There is no list/edit/delete flow for saved aliases.

**Fix:** start the command blank (or add real command metadata), validate before save,
show saved aliases with edit/delete, apply the managed Zsh section immediately with a
diff/backup, and report that a new shell is required.

### ADV-UI-M02 — File Tree can start installation while async detection is pending, causing synchronous duplicate probes

**Severity: Medium**

File Tree shows a loading frame when `installCacheLoading`, but its `Update` accepts
Enter unconditionally and navigates to Progress (`screen_filetree.go:45-65,70-74`).
Progress `Init` calls `startInstallation` (`screen_progress.go:57-77`), which calls
`collectSelectedTools`; that calls synchronous `ensureInstallCache`
(`installation.go:53-54,684-688`; `cache.go:151-205`).

If the async preload has not finished, the UI goroutine performs the same slow
package/custom probes synchronously while the original command is still running.
There is no visible progressing frame during the block.

**Fix:** disable Enter until cache completion or make planning await/reuse the one
in-flight observation command. The planner should consume an immutable observation
snapshot and never trigger detection synchronously from an input handler.

## Corrected settings matrix assessment

The primary matrix is mostly useful, but four rows need material correction:

| Tool | Verdict on primary row | Correction |
|---|---|---|
| Ghostty | Confirmed | 9 generator fields; Deep Dive exposes 7, Manage 8, neither imports live state. Whole-file ownership and higher-precedence macOS config remain critical. |
| tmux | Confirmed | 16 generator fields; Deep Dive dynamically exposes up to 13, Manage 15 and omits split bindings. A Manage save can reset its hidden split default. |
| Zsh | **Partial** | It is not a full-file clobberer. `WriteZshConfig` merges a bounded managed section and preserves content outside markers (`internal/tools/zsh.go:353-380`). Manage can still reset hidden fields *inside* that section because translation begins with compiled defaults, but arbitrary user shell content outside the section survives. |
| Neovim | **Partial** | Ten fields are modeled, but only nine are consumed: `Plugins` is declared and never read. Manage's overlay is safer; installer preset behavior is separate and can clone/replace a config directory lacking `init.lua` after creating a timestamped sibling backup (`internal/tools/neovim.go:338-385`). |
| Git | Confirmed | Nine modeled inputs cannot round-trip identity, LFS, includes, arbitrary aliases, or credential/signing policy. Whole `.gitconfig` replacement remains the highest-risk tool gap. |
| Yazi | Confirmed | Seven inputs split 3/5 across Deep Dive/Manage; writer replaces three files and backup covers only one. |
| FZF | Confirmed | Six generator fields; Deep Dive exposes three, Manage all six. Raw options also enter sourced shell code, covered by tools `F14`. |
| LazyGit | Confirmed | Four fields; Deep Dive omits paging, Manage exposes all; full file is replaced. |
| LazyDocker | Confirmed | Two Manage-only preferences have no writer, but the UI does explicitly render `(not applied)` using `manageNotAppliedFields` (`manage_field_apply.go:17-49`; `manage_dualpane.go:929-938`). Incomplete, but not silently claimed applied. |
| Btop | Confirmed | Six fields; Deep Dive exposes four, Manage six; backup omits the output. |
| Glow | **Partial** | Four generator fields; Deep Dive exposes three and Manage four. Current test failures are caused by stale macOS path expectations, not proven output-schema divergence. Pager semantics are separately wrong for wizard `none`. |
| Claude Code | Confirmed, plus worse install defect | Seven MCP toggles plus install selection; theme-only apply can mutate MCPs. `ADV-UI-C01` additionally proves installation never invokes the custom npm step. |

The primary information-architecture recommendation—observed vs desired provenance,
Essentials/Advanced layers, explicit ownership, source paths, diff, and no dead
settings—is confirmed. It aligns with the tools capability-manifest recommendation.

## Cross-report duplicate map

| UI finding | Canonical repository finding |
|---|---|
| `C-02` baseline/default + within-tool clobber | tools `F01/F02`; adversarial core `CORE-M05` |
| `C-04` fail-open/incomplete rollback | adversarial core `CORE-M02` and `ADV-CORE-H01` |
| `H-06` all-package boolean “installed” state | tools `F06` |
| `M-01` detection error collapsed to false | merge into UI `H-06` structured-state finding |
| `M-07` local profiles presented as users | core `CORE-M03` |
| `H-13` Glow test failure portion only | tools `F08`; retain UI's independent network-side-effect test finding |

The UI audit should remain the canonical source for `C-01`, `C-03`, confirmation
fidelity, screen-local operation state, concurrent saves, 80x24/viewport defects,
duplicate update start, hidden planned-tool catalogs, outcome messaging, and
standalone save semantics.

## Architecture, repository, and rename verdicts

### Architecture/new repository

**Confirmed. Keep and refactor this repository.** The proposed seams—typed
observations, versioned desired state, immutable plan, ownership/backup manifest,
operation IDs, structured results, and registry-owned descriptors—address the actual
call-graph failures. A clean rewrite would not remove config precedence, package
provenance, rollback, or terminal geometry complexity.

A new repository makes sense only for a genuinely separate multi-host control plane
(fleet service, tenant identity, remote agents, policy distribution, secrets, audit
storage). The local agent/TUI should then remain a library/client boundary rather
than be discarded.

### Rename

**Partial correction:** the UI-specific difficulty of **7/10** is reasonable, but the
current exact production-UI count is **91** case-insensitive `dotfiles` occurrences
across 26 non-test Go files, not 89. More importantly, repository-wide config,
binary, module, Homebrew, backup, and legacy compatibility makes the canonical score
**8/10**, as documented in the tools/release audit.

The primary sequencing advice is sound: centralize product identity, keep an old CLI
shim, dual-read/migrate config with rollback, preserve old backup/managed markers,
and do not combine rename and ownership migration as one irreversible action.

## Corrected release order

1. Fix core install planning (`C-01`) and custom installer dispatch
   (`ADV-UI-C01`); test clean machines with missing core tools and Claude.
2. Stop theme-wide and per-tool destructive writes until observed-state import,
   ownership, diff, complete backup, and fail-closed apply exist.
3. Replace boolean installation with structured observations and remove synchronous/
   duplicate detection.
4. Move all async results to an App operation coordinator with IDs; make save/update
   starts single-flight.
5. Make confirmation, progress, summary, and backup all render the same immutable
   plan/result.
6. Fix explicit Save/Cancel semantics and make 80x24 a tested supported baseline with
   viewports/compact tabs.
7. Drive group catalogs and navigation behavior from the tool/settings descriptors
   before implementing planned AI tools.
8. Make macOS tests green and hermetic; separate real network/install integration
   tests behind an explicit suite.

No production source was modified during this adversarial check.
