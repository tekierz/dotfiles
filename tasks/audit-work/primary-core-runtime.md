# Primary Core Runtime Audit

Date: 2026-07-09
Partition: `cmd/dotfiles/`, `internal/backup/`, `internal/config/`, `internal/hotkeys/`, `internal/pkg/`, `internal/runner/`, `internal/scripts/`, `internal/theme/`, `go.mod`, `go.sum`
Method: manual line-by-line review of every tracked file in the partition, including all tests and applicable `AGENTS.md` files; repository-wide call-site searches where necessary to validate reachability; focused build/test/static/dependency checks. No production source was modified.

## Executive conclusion

**Release disposition for this partition: no-go for friends/family or mock-enterprise deployment; suitable only for developer-controlled local testing after the five high-severity issues are fixed.**

The strongest parts are the restore traversal defenses, literal-argv subprocess execution, atomic JSON writes, Claude JSON preservation, and the recent Pacman full-sync update correction. Those are substantive safeguards, not cosmetic ones. However, five high-impact failures remain:

1. restoring over an existing credential file does not tighten its permissions;
2. Debian/Pi “update all” can deadlock before the upgrade starts;
3. uninstall can delete unrelated binaries from `/usr/local/bin` based only on their names;
4. “update all” expands a managed-package view into a full system/package-manager upgrade;
5. older global configs silently disable the default backup policy on load.

The configuration architecture also explains the user-observed mismatch between installed applications and dashboard settings: the core models are narrow desired-state structs, not adapters that detect and import existing application state. There is no schema version, provenance, migration registry, ownership/merge policy, validation layer, or dry-run plan. `AppsConfig` and `UtilitiesConfig` are unused prototype residue, while `OpenCode` exists only as a dead boolean field.

I do **not** recommend starting a new repository. The package boundaries are salvageable and several security fixes are already good. A new repository would discard useful test history without fixing the state-model problem. Create a vNext architecture inside this repository, behind interfaces and schema migrations, then remove legacy paths after parity tests.

Core-partition rename difficulty: **8/10 (hard but tractable before public release)**. The name is embedded in the Cobra command (`cmd/dotfiles/main.go:27`), output/help text, config directory (`internal/config/config.go:74-90`), package allow-list (`internal/pkg/update.go:62-100`), hotkey IDs/commands (`internal/hotkeys/hotkeys.go:246-255`), Go module/import path (`go.mod:1`), backup paths, binary names, and uninstall behavior (`cmd/dotfiles/main.go:789-817`). A safe rename needs compatibility shims for the old binary, config path, Homebrew formula, backups, and module path—not a global search/replace.

## Verification performed

- `go test -count=1 -cover` on all eight assigned Go packages: pass.
  - `cmd/dotfiles`: 21.0%
  - `internal/backup`: 71.2%
  - `internal/config`: 75.6%
  - `internal/hotkeys`: 100.0%
  - `internal/pkg`: 25.4%
  - `internal/runner`: 72.2%
  - `internal/scripts`: 100.0% Go statements (misleadingly high because most behavior is embedded shell text)
  - `internal/theme`: 0.0%
- `go test -race -count=1` on all assigned packages: pass.
- `go vet` on all assigned packages: pass.
- `go mod verify`: pass (`all modules verified`).
- `go list -m -u all`: direct dependencies are current at the time of the check, but many transitives have newer releases; notably `golang.org/x/text v0.3.8 -> v0.40.0`, `golang.org/x/sys v0.36.0 -> v0.47.0`, and several 2022-era transitive modules.
- Official `govulncheck` v1.6.0: **0 reachable symbol vulnerabilities** in the assigned packages. It still reported four imported-package and seventeen required-module/stdlib advisories not reached by the scanned call graph. The local compiler was `go1.26.1`; the current advisories shown by the scanner were fixed across Go 1.26.2–1.26.5, so release artifacts should be rebuilt with a patched toolchain.
- Focused CLI probes confirmed that `dotfiles theme nonsense`, `dotfiles backups unexpected`, and `dotfiles --DoesNotExist` all exit successfully despite invalid input or failed intent.
- A fresh local macOS timing of `./bin/dotfiles status` took **22.23 seconds real** (11.70s user, 3.94s sys) for a 30-tool registry; help/version were immediate.

## Findings

### High

#### CORE-H01 — Restore does not apply the recorded mode to an existing file

- Kind: security / bug
- Evidence: `internal/backup/backup.go:149-161`; the destination is opened with `os.O_CREATE` and the recorded `mode`, but Go applies that mode only when creating a file. There is no `Chmod` after writing. `internal/backup/backup_test.go` and `cmd/dotfiles/restore_test.go:130-157` test a newly created destination, not replacement of an existing permissive destination.
- Trigger: restore a manifest entry recorded as `0600` over an existing `0644`/`0666` file.
- Impact: contents are restored but the old permissive mode remains. Credentials or tokens can remain group/world-readable while the CLI reports a successful secure restore.
- Fix: after a successful write, call `f.Chmod(mode.Perm())` before close/sync; add tests for replacing `0644` with `0600` and for chmod failure accounting.

#### CORE-H02 — Apt `UpdateAllStreaming` can deadlock on ordinary output

- Kind: concurrency / bug
- Evidence: `internal/runner/bash.go:119-149` gives output a 100-line buffer and does not call `cmd.Wait()` until both scanner goroutines finish; scanners block when the buffer fills. `internal/pkg/apt.go:318-325` calls `updateCmd.Wait()` without draining `updateCmd.Output`.
- Trigger: `apt update` emits more than 100 stdout/stderr lines (common with many repositories/locales) during Debian/Pi update-all.
- Impact: the child blocks on full pipes, scanner goroutines block on the full channel, and `Wait()` never returns; the TUI appears hung and the upgrade never begins.
- Fix: make draining intrinsic to `Wait` or expose a result stream that cannot block process reaping. For the sequential apt operation, forward the update phase into the returned stream rather than synchronously waiting on an undrained command. Add a >100-line regression test with a timeout.

#### CORE-H03 — Uninstall deletes binaries it does not own

- Kind: security / destructive bug
- Evidence: `cmd/dotfiles/main.go:789-817` deletes every path named `dotfiles`, `dotfiles-tui`, `dotfiles-setup`, `hk`, `caff`, or `sshh` under both `~/.local/bin` and `/usr/local/bin`; ownership, symlink target, checksum, install manifest, and package-manager provenance are never checked.
- Trigger: a user has any unrelated executable with one of those generic names, then runs uninstall (especially `--force`).
- Impact: unrelated software can be deleted. On systems where `/usr/local/bin` is writable/elevated, this is an unsafe broad destructive action.
- Fix: record every installed artifact and checksum in a versioned install manifest; remove only matching owned artifacts. For package-manager installations, invoke/report the package-manager uninstall flow instead of deleting its links. Never touch `/usr/local/bin` by basename alone.

#### CORE-H04 — Managed update view can become an unannounced full-system upgrade

- Kind: feature / architecture / deployment risk
- Evidence: `internal/pkg/update.go:137-165` deliberately filters checks to `DotfilesPackages`; `internal/pkg/apt.go:314-327` runs `apt upgrade -y`; `internal/pkg/pacman.go:463-469` runs full `-Syu`; `internal/pkg/brew.go:225-248,353-365` obtains all Homebrew outdated names, not the managed allow-list. The update UI call sites use the manager-level `UpdateAllStreaming` after loading managed updates.
- Trigger: choose “update all” after seeing the dotfiles-managed list.
- Impact: unrelated OS/Homebrew/AUR packages are upgraded, potentially causing service restarts, kernel changes, breaking upgrades, or enterprise change-control violations.
- Fix: split APIs into explicit `UpdateManaged(names)` and `UpdateSystem()` capabilities. The default dashboard action must pass exactly the displayed package identities, including source manager. Put full-system upgrade behind a separately named, strongly confirmed operation with dry-run output.

#### CORE-H05 — Partial/older global configs silently disable backup defaults

- Kind: migration / safety bug
- Evidence: `internal/config/config.go:63-71` defaults `AutoBackup=true`, count `10`, age `30`; `internal/config/config.go:178-183` unmarshals an existing JSON file into a zero-value `GlobalConfig` instead of overlaying defaults. `LoadToolConfig` correctly uses default-then-overlay at `internal/config/config.go:132-140`, showing the intended migration pattern. No partial-global test exists.
- Trigger: upgrade from a version whose `global.json` predates any backup field, or load a manually partial file.
- Impact: missing booleans/integers become `false/0/0`, silently disabling automatic backup and retention exactly during an upgrade where rollback is important.
- Fix: initialize with `DefaultGlobalConfig()` before unmarshal; introduce `schema_version` plus ordered migrations and validation; add fixtures for every released schema.

### Medium

#### CORE-M01 — Directory restore is destructive and non-transactional

- Kind: bug / architecture
- Evidence: `internal/backup/backup.go:289-301` verifies readability, then `os.RemoveAll(dstPath)` before copying the backup tree directly into place.
- Trigger: disk-full, permission, race, symlink-creation, or I/O failure after the live directory has been removed.
- Impact: restore reports a skipped entry but leaves the user's live directory deleted or partially reconstructed.
- Fix: restore into a sibling staging directory, fsync/verify it, preserve the old destination as a rollback rename, then atomically swap where supported; otherwise use a journaled copy plan.

#### CORE-M02 — Backup creation can report success while silently omitting files

- Kind: bug / deployment risk
- Evidence: `internal/backup/create.go:30-47` silently continues on every stat/read/write error and returns success if any one candidate succeeds; it does not return per-path failures. It writes data files before the manifest at `internal/backup/create.go:42-61`, leaving an unrestorable orphan directory if the manifest write fails.
- Trigger: one candidate is unreadable, races away, collides/fails to write, while another candidate succeeds.
- Impact: the UI can claim a rollback point while critical files are absent. A manifest failure leaves debris that backup listing can present as a backup even though restore refuses it.
- Fix: return a structured result with captured/skipped/error paths; make auto-backup fail closed on any selected path that existed but was not captured; build in a temporary directory and rename only after a durable manifest is written.

#### CORE-M03 — “User profiles” do not actually isolate or apply a full environment

- Kind: feature completeness / architecture
- Evidence: profiles store `KeyboardStyle` (`internal/config/user.go:14-21`), but `ApplyUserProfile` applies only theme, nav, and active name (`internal/config/user.go:183-195`). Per-tool files are globally rooted in `ToolsDir()` (`internal/config/config.go:92-95`) rather than per profile; only hotkeys are keyed per user (`internal/config/hotkeys.go:11-20`).
- Trigger: switch users expecting keyboard, tool settings, app selections, or utilities to change.
- Impact: the dashboard presents a stronger profile abstraction than exists. Friends/family share global tool state; mock-enterprise personas cannot be modeled safely.
- Fix: either rename the feature to “theme/navigation presets” now or implement true profile-scoped desired state with inheritance, active-profile resolution, and migration of existing global tool configs.

#### CORE-M04 — Persisted configuration is not validated

- Kind: bug / quality
- Evidence: `LoadGlobalConfig` and `LoadToolConfig` unmarshal without range/enum validation (`internal/config/config.go:116-140,163-183`); `LoadUserProfile` accepts any stored field values (`internal/config/user.go:77-97`); `SaveUserProfile` validates only `Name` (`internal/config/user.go:100-126`); `ApplyUserProfile` trusts the object (`internal/config/user.go:183-195`). Tool models include unconstrained font size, opacity, keymaps, branches, and layouts (`internal/config/tool.go:3-51`).
- Trigger: corrupt/manual/old config, unsupported enum, negative/out-of-range number, or a future field migration.
- Impact: invalid desired state reaches renderers/generators and can produce incorrect dashboard values or broken app configs.
- Fix: give each schema `Validate/Normalize`, preserve an error with field path, and reject or explicitly repair invalid persisted state. Add JSON Schema/exported diagnostics for enterprise automation.

#### CORE-M05 — Config models cannot safely represent/import existing app state; two are dead prototypes

- Kind: architecture / feature completeness / prototype residue
- Evidence: tool structs model only a few hand-selected values (`internal/config/tool.go:3-51`) and carry no schema version, source/provenance, “managed vs observed” state, unknown-field preservation, or merge policy. Repo-wide reference checking found `AppsConfig` and `UtilitiesConfig` (`internal/config/tool.go:53-89`) only in their own declarations; `UtilitiesConfig.OpenCode` at line 85 has no reader/writer/installer.
- Trigger: open the dashboard for an already configured app, save a subset, add/remove fields in later releases, or expect OpenCode/app selections to work.
- Impact: defaults can be shown as if they are observed settings; unknown/existing settings cannot be reconciled safely; dead fields imply features that do not exist.
- Fix: define an integration contract such as `Detect`, `ReadObserved`, `Plan(desired, observed)`, `Apply(plan)`, `Rollback`; store versioned desired state separately from observed snapshots; preserve unowned app config keys/lines; delete dead structs or wire them end to end.

#### CORE-M06 — CLI grammar and exit codes are not automation-safe

- Kind: CLI UX / feature
- Evidence: most Cobra commands use manual length checks and no `Args` validators (`cmd/dotfiles/main.go:64-169,181-273`), so extra args are ignored or change behavior. Invalid theme syntax only prints usage (`cmd/dotfiles/main.go:90-98`); update-check errors return normally (`cmd/dotfiles/main.go:521-539`); a nonexistent quick-switch user prints a message and returns success (`cmd/dotfiles/main.go:320-335`). Probes confirmed zero exit status for invalid theme syntax, extra backup args, and nonexistent quick switch.
- Trigger: typo, extra positional argument, no package manager, failed update query, or nonexistent profile in a script.
- Impact: CI/MDM scripts see success when intent failed; `update nonsense` opens an interactive TUI instead of rejecting input.
- Fix: use real subcommands (`update check`, `theme list/set`) and Cobra `ExactArgs/NoArgs/MaximumNArgs`; return errors from helpers rather than calling `os.Exit`; define stable exit codes and `--json`, `--non-interactive`, `--dry-run`, `--yes` behavior.

#### CORE-M07 — Manager discovery checks every executable, not the detected platform

- Kind: cross-platform bug / architecture
- Evidence: `DetectManager` is platform-gated (`internal/pkg/manager.go:125-157`), but `AllManagers` appends brew, paru/pacman, and apt solely by PATH availability (`internal/pkg/manager.go:160-177`). `CheckAllUpdates` runs all of them (`internal/pkg/update.go:18-36`). The CLI first prints the one detected manager, then calls the all-manager check via `CheckDotfilesUpdates` (`cmd/dotfiles/main.go:524-532`).
- Trigger: a development workstation/container has foreign package-manager commands in PATH.
- Impact: surprising commands, unrelated errors/results, and misleading “Using brew” output while apt/pacman are also queried.
- Fix: default to managers supported for the detected platform; make multi-manager discovery explicit and capability-driven, and report every queried source.

#### CORE-M08 — Package operations are not reliably unattended or diagnosable

- Kind: feature / enterprise readiness
- Evidence: synchronous installs discard stdout/stderr (`internal/pkg/brew.go:33-42`; apt and pacman return only `cmd.Run` errors at `internal/pkg/apt.go:32-51`, `internal/pkg/pacman.go:49-75`); no synchronous method accepts context/timeout. Apt uses `-y` but not an explicit noninteractive policy (`internal/pkg/apt.go:37-40,158-161,172-173`). Streaming commands detach stdin (`internal/runner/bash.go:92-98`) and assume sudo was pre-cached.
- Trigger: config-file prompt, restart prompt, sudo expiry, network stall, lock contention, or package-manager failure.
- Impact: hangs or opaque `exit status 1`, unsuitable for MDM/mock-enterprise rollout and difficult for friends/family to recover from.
- Fix: context on every operation, structured command/result with captured diagnostics, explicit interactive/noninteractive modes, package-manager-specific prompt policy, timeout/cancel escalation, and machine-readable progress/audit records.

#### CORE-M09 — Paru uninstall is run as root despite the implementation's own rule

- Kind: cross-platform bug
- Evidence: `internal/pkg/pacman.go:57-62` correctly avoids sudo for Paru install, but uninstall always invokes `sudo <paru-path>` (`internal/pkg/pacman.go:67-75`). The streaming implementation documents why Paru must not run as root (`internal/pkg/pacman.go:432-437`).
- Trigger: uninstall an AUR package through a `PacmanManager{useParu:true}`.
- Impact: AUR helper refusal, wrong ownership/cache effects, or failed uninstall.
- Fix: branch uninstall exactly like install/update; add fake-executable argv tests for both Paru and Pacman paths.

#### CORE-M10 — `sshh add` cannot bootstrap its own config

- Kind: feature / CLI UX bug
- Evidence: `internal/scripts/scripts.go:277-283` exits immediately when `~/.sshh` is missing; argument handling, including `add`, begins only at `internal/scripts/scripts.go:324-369`.
- Trigger: a first-time user runs the documented `sshh add "Name" "user@host" [port]` without manually creating `~/.sshh`.
- Impact: the advertised onboarding command always fails at the moment it is most needed.
- Fix: parse `help/add/edit` before requiring an existing file; create it atomically with `0600`; validate port and destination; add end-to-end shell tests for missing-file bootstrap.

#### CORE-M11 — `caff` can orphan a process while falsely reporting success

- Kind: bug / cross-platform feature
- Evidence: runtime-directory creation/permission failures are ignored (`internal/scripts/scripts.go:114-117`); Linux unconditionally assumes `systemd-inhibit` (`internal/scripts/scripts.go:198-206`); after a bounded readiness loop, PID-file write errors and child failure are ignored and “ON” is printed (`internal/scripts/scripts.go:207-224`).
- Trigger: unwritable/missing runtime base, non-systemd Linux/WSL/container, absent inhibitor binary, or child exits before readiness.
- Impact: false status, an untracked/orphan inhibitor, or no inhibition despite success output.
- Fix: fail on runtime setup; capability-detect providers; require the child to remain alive and match expected command; atomically persist PID and abort/kill on failure. Test full start/stop scripts, not only extracted helpers.

#### CORE-M12 — Emacs Yazi hidden-file binding is wrong

- Kind: feature / docs bug
- Evidence: `yaziHidden` starts as `.` at `internal/hotkeys/hotkeys.go:86-90` and the Emacs branch never changes it at lines 91-96. `internal/hotkeys/AGENTS.md` says Emacs should be `Ctrl-h`, and the static fallback says `Ctrl-h` at `internal/scripts/scripts.go:47-50`. Tests cover Yazi navigation but omit hidden-file binding (`internal/hotkeys/hotkeys_test.go:104-114`).
- Trigger: select Emacs navigation and consult the dynamic hotkey dashboard.
- Impact: incorrect instructions and direct drift between the dashboard, static `hk`, and documentation.
- Fix: assign `yaziHidden = "Ctrl-h"` in the Emacs branch and add the missing flip test.

#### CORE-M13 — “Stable” favorite IDs change when copy text changes

- Kind: maintainability / migration bug
- Evidence: item IDs are derived from user-facing descriptions (`internal/hotkeys/hotkeys.go:44-70`). Current tests prove stability only across nav style, not across releases (`internal/hotkeys/hotkeys_stable_id_test.go`).
- Trigger: edit a description for clarity, punctuation, capitalization, or localization.
- Impact: persisted favorites stop matching; migration cannot infer the old identity reliably.
- Fix: declare explicit immutable action IDs separate from display strings and add a versioned alias map for historical IDs.

#### CORE-M14 — Atomic writes still lose concurrent updates

- Kind: concurrency / architecture
- Evidence: JSON saves are atomic at the file-replacement level (`internal/config/config.go:16-48`), but read-modify-write operations have no file lock or compare-and-swap. Claude does a separate read, backup write, and final write (`internal/config/claude.go:122-157`); hotkeys/global/profile saves likewise have no inter-process coordination.
- Trigger: two CLI/TUI processes change the same config concurrently, or management automation races a user session.
- Impact: last writer silently overwrites the other's valid update; `.claude.json.bak` can also represent an intermediate writer rather than the user's original.
- Fix: advisory lock per config, revision/etag in the schema, re-read/merge under lock, and conflict errors. Race-detector success does not cover cross-process lost updates.

#### CORE-M15 — Default MCP definitions execute unpinned third-party code noninteractively

- Kind: security / enterprise readiness
- Evidence: all server definitions use `npx -y` and unversioned packages; Convex explicitly uses `convex@latest` (`internal/config/claude.go:24-61`). `-y` suppresses install confirmation.
- Trigger: enable a server and later launch Claude Code/MCP on a networked machine.
- Impact: package behavior can change without a dotfiles release; a compromised/upstream-breaking latest package is fetched and executed. This violates reproducibility/change-control expectations.
- Fix: pin reviewed versions and integrity/provenance, surface network/code-execution consent, support enterprise allow-lists/offline mirrors, and ship a controlled upgrade workflow.

#### CORE-M16 — Tests miss the riskiest command and migration paths

- Kind: test quality / repo health
- Evidence: measured coverage is 21.0% for CLI and 25.4% for `internal/pkg`; `internal/theme` has no tests. Apt tests exercise only name/sudo plus copied benchmark parsers (`internal/pkg/apt_bench_test.go`), update tests mirror production filtering/dedup instead of invoking it (`internal/pkg/update_test.go:13-58,151-190`), and runner tests always drain before `Wait` (`internal/runner/bash_test.go:14-22`). Scripts' 100% Go statement metric mostly measures `GetScript`, not shell branches.
- Trigger: changes in uninstall, actual command argv, output backpressure, partial schema migration, first-run shell flows, or theme registry.
- Impact: current green tests did not catch H01-H05 or several medium findings.
- Fix: table-driven fake executables for every package-manager method, subprocess CLI exit-code tests, partial-schema fixtures, restore failure injection, >100-line streaming tests, shell end-to-end tests, and theme registry invariants.

#### CORE-M17 — `status` performs sequential per-tool package-manager subprocesses

- Kind: performance / CLI UX / architecture
- Evidence: the CLI synchronously asks the registry for installed and not-installed tools at `cmd/dotfiles/main.go:466-471`. Registry cache population loops over every tool serially and calls `t.IsInstalled()` (`internal/tools/registry.go:127-140`). Base tools then check every package one by one (`internal/tools/tool.go:129-157`), and on macOS each check starts a new `brew list <pkg>` process (`internal/pkg/brew.go:55-58`). GUI tools add command/filesystem/app probes before often falling back to that same package check (for example `internal/tools/apps.go:41-67,97-114`). Measured fresh on the local mac: **22.23s real** for 30 registry tools; help/version were immediate.
- Trigger: run `dotfiles status` with a cold registry on a normal Homebrew mac, especially where tools have multiple packages or GUI fallbacks.
- Impact: a supposedly quick, script-friendly read command feels hung and becomes impractical for shell prompts, health checks, MDM inventory, or repeated validation. The cache helps only within the short-lived process, so every CLI invocation pays again.
- Fix: build one immutable detection snapshot per command from batched `brew list --versions` plus `brew list --cask` and command/app-bundle probes; evaluate tools against in-memory sets, parallelize only bounded non-package probes, add context/timeouts, and expose `status --json`. Set a cold-start performance budget (for example <1s locally) and benchmark it in CI with fake package-manager fixtures.

### Low

#### CORE-L01 — Generic tool config name can escape `tools/`

- Kind: security hardening
- Evidence: `LoadToolConfig` and `SaveToolConfig` directly join caller-controlled `toolName + ".json"` (`internal/config/config.go:116-122,143-150`) without requiring a single safe component.
- Trigger: a future caller passes `../name` or an absolute-like platform-specific value. Current CLI validates against a fixed UI map, so this is not presently a direct user exploit.
- Impact: internal callers can read/write another JSON path under the broader config directory.
- Fix: validate a strict tool ID or safe-base containment, and test traversal strings.

#### CORE-L02 — Streaming result API has misleading/dead contracts

- Kind: quality / prototype residue
- Evidence: `OutputLine`, `OutputType`, and `Runner` are declared but unused (`internal/runner/bash.go:14-39`); actual output is an untyped merged string channel (`internal/runner/bash.go:70-76`). Scanner errors are discarded (`internal/runner/bash.go:126-139`). `Wait` reads once from a channel and returns zero-value `nil` on a second call after close (`internal/runner/bash.go:85-88,145-149`).
- Trigger: long line >1 MiB, code calling `Wait` twice, or UI needing stderr attribution.
- Impact: truncated output can look successful; repeated wait loses the original error; structured types falsely suggest capabilities the API does not provide.
- Fix: one structured event stream, store a stable terminal result, propagate scanner errors, make `Wait` idempotent, and delete dead types.

#### CORE-L03 — Theme registries/defaults are duplicated and can drift

- Kind: architecture / maintainability
- Evidence: `internal/config/config.go:206-224` separately defines theme names; `internal/theme/names.go:3-21` and `internal/theme/palettes.go:25-266` define them again. Global default is Catppuccin (`internal/config/config.go:63-71`), while unknown palette fallback is Neon Seapunk (`internal/theme/theme.go:3-18`). There are no theme-package tests.
- Trigger: add/rename/reorder a theme or load an invalid name.
- Impact: CLI list, validation, UI order, and rendered fallback can disagree.
- Fix: make `internal/theme` the sole registry and expose names/default/validation from it; test exact key/name equality and color format/contrast.

#### CORE-L04 — Applicable package documentation is stale

- Kind: docs
- Evidence: `internal/config/AGENTS.md:8-14,40-63` and `internal/AGENTS.md` describe nonexistent `defaults.go`, `AllToolConfigs`, `LoadAllToolConfigs`, `SaveAllToolConfigs`, and nine `Default*Config` functions. Repo-wide searches found none. `cmd/dotfiles/main.go:721-723` says utilities include `y`, while the actual removal list contains `sshh` at lines 789-797. `internal/hotkeys/AGENTS.md` documents the binding that production gets wrong (M12).
- Trigger: contributor follows the prescribed extension path or user reviews uninstall preview.
- Impact: wasted implementation effort, incorrect assumptions about persistence, and lower trust in release documentation.
- Fix: generate API inventories/help from code where possible; update AGENTS after the config refactor; add doc assertions for command/help and utility lists.

#### CORE-L05 — Dependency/toolchain hygiene needs a repeatable release gate

- Kind: security / repo health
- Evidence: `go.mod:3` declares Go 1.25.6; local artifacts/tests used Go 1.26.1. Official scanning found zero reachable symbol vulnerabilities but 4 imported-package/17 module or stdlib advisories, with current stdlib fixes requiring Go 1.26.2–1.26.5. `go list -m -u all` also showed numerous old transitives, including `golang.org/x/text v0.3.8` and 2022-era x/* modules.
- Trigger: release built on whatever Go happens to be installed, without a recorded SBOM/scanner/toolchain version.
- Impact: rebuilds differ and may retain fixed stdlib/module issues even when application call-graph scanning currently says unreachable.
- Fix: pin/record the release toolchain at a patched version, generate SBOM/provenance, run `govulncheck` in CI, and periodically refresh dependencies through tested direct-dependency upgrades rather than blindly forcing transitives.

## CLI UX and discoverability assessment

The CLI is approachable for an interactive owner but not yet a stable automation surface:

- Good: clear top-level verbs, direct `status/backups/users/theme list/update check`, helpful user-facing text, literal argv execution, and a quick-switch shortcut.
- Blocking gaps: command grammar is ad hoc instead of Cobra subcommands/validators; helpers mix printing, process exit, and domain logic; errors frequently return status 0; no `--json`, quiet, dry-run, plan, or deterministic noninteractive contract; `--force` conflates confirmation skipping with unattended safety; update scope is ambiguous; user profiles over-promise isolation.
- Recommendation: make commands thin adapters over context-aware services returning typed results/errors. Add global `--output human|json`, `--non-interactive`, `--dry-run`, `--yes`, and stable exit-code documentation. Reserve TUI launch for truly zero-argument interactive commands and reject accidental extra input.

## Configuration/API extensibility recommendation

Adopt a three-layer model before adding Pi agent, Codex, Cursor CLI, OpenCode, or T3 Code:

1. **Observed state** — read installed binary/app bundle, version, config location(s), and parsed owned/unowned settings without mutation.
2. **Desired state** — versioned, validated product configuration, profile-scoped where applicable, with explicit defaults and migrations.
3. **Plan/result** — a reviewable list of file/package/process changes, backups, conflicts, warnings, and rollback tokens; the TUI and CLI consume the same plan.

Each integration should declare platform/package-manager capabilities and implement detect/read/plan/apply/verify/rollback. Preserve unowned settings structurally or through managed blocks. This resolves both the existing-app settings mismatch and the long-term explosion of bespoke screen/save logic.

## Suggested deployment gates

Before developer local testing:

- fix H01-H05;
- add regression tests that fail on each;
- rebuild with patched Go and rerun `go test -race`, `go vet`, and `govulncheck`.

Before friends/family:

- fix M01-M13 and M17 at minimum;
- provide dry-run/rollback and explicit update scope;
- run clean-install, upgrade-from-2.0.x, existing-config import, uninstall, and restore drills on macOS Intel/Apple Silicon, Debian/Ubuntu, Arch, and Pi.

Before mock enterprise:

- finish M14-M16 plus structured noninteractive/JSON output, logging/provenance, pinned MCP packages, least-privilege policy, schema migration tests, offline/failure injection, and a supported platform/version matrix.

## Refuted concerns / confirmed safeguards

- **Restore destination path traversal:** refuted for reviewed paths. `safeJoin`, absolute-home containment, intermediate-parent symlink resolution, and final-component `O_NOFOLLOW` are layered at `internal/backup/backup.go:110-201,242-317`. Tests cover lexical and symlinked-parent escape.
- **Backup source traversal by CLI name:** refuted. `isValidBackupName` rejects empty, absolute, separator, and parent components at `cmd/dotfiles/main.go:624-647`; source items are checked against the selected backup directory at `internal/backup/backup.go:284-287,605-612`.
- **Lossy underscore decode of manifestless backups:** refuted. `internal/backup/backup.go:401-416` refuses any non-empty manifestless backup; tests cover it.
- **Shell injection through package arguments:** refuted. Package managers and runner use `exec.Command`/literal argv, not a shell; runner tests exercise metacharacters.
- **Claude save clobbers unrelated top-level JSON:** refuted. `internal/config/claude.go:122-157` preserves raw unrelated keys and backs up existing content; tests verify model/permissions/hooks/statusLine preservation.
- **Half-written JSON on ordinary process interruption:** substantially mitigated. `writeFileAtomic` writes, chmods, syncs, closes, then same-directory renames (`internal/config/config.go:16-48`). Inter-process lost update remains M14; directory fsync is not provided.
- **Pacman partial-upgrade regression:** refuted. selected updates use `-Syu` (`internal/pkg/pacman.go:291-314`) and tests pin that argv.
- **Removed-but-not-purged apt packages reported installed:** refuted. status must equal `install ok installed` (`internal/pkg/apt.go:54-90`).
- **Reachable known Go vulnerabilities in this partition:** none reported by official `govulncheck` at audit time; broader module/toolchain hygiene remains L05.

## Coverage manifest

Selection command: `git ls-files cmd/dotfiles internal/backup internal/config internal/hotkeys internal/pkg internal/runner internal/scripts internal/theme go.mod go.sum`.

Assigned file count from Git: **46**. Manifest file count below: **46**.
Assigned line total from `wc -l`: **10,660**. Manifest line total below: **10,660**.
Result: **assigned total equals actual manifest total; every line was read.**

The repository-root `AGENTS.md` (191 lines) was also read as an applicable parent instruction file; it is not included in the 46-file partition total because it is outside the assigned selection.

| # | File | Lines |
|---:|---|---:|
| 1 | `cmd/dotfiles/AGENTS.md` | 129 |
| 2 | `cmd/dotfiles/embedded_cli_restore_test.go` | 199 |
| 3 | `cmd/dotfiles/main.go` | 1,088 |
| 4 | `cmd/dotfiles/restore_test.go` | 247 |
| 5 | `cmd/dotfiles/task16_theme_list_test.go` | 34 |
| 6 | `go.mod` | 31 |
| 7 | `go.sum` | 53 |
| 8 | `internal/backup/backup.go` | 633 |
| 9 | `internal/backup/backup_test.go` | 317 |
| 10 | `internal/backup/create.go` | 65 |
| 11 | `internal/backup/create_test.go` | 61 |
| 12 | `internal/config/AGENTS.md` | 217 |
| 13 | `internal/config/claude.go` | 158 |
| 14 | `internal/config/claude_test.go` | 280 |
| 15 | `internal/config/config.go` | 234 |
| 16 | `internal/config/config_test.go` | 237 |
| 17 | `internal/config/hotkeys.go` | 256 |
| 18 | `internal/config/hotkeys_migration_test.go` | 296 |
| 19 | `internal/config/hotkeys_path_test.go` | 89 |
| 20 | `internal/config/loadtoolconfig_test.go` | 74 |
| 21 | `internal/config/tool.go` | 89 |
| 22 | `internal/config/user.go` | 224 |
| 23 | `internal/config/user_test.go` | 351 |
| 24 | `internal/hotkeys/AGENTS.md` | 108 |
| 25 | `internal/hotkeys/hotkeys.go` | 415 |
| 26 | `internal/hotkeys/hotkeys_stable_id_test.go` | 191 |
| 27 | `internal/hotkeys/hotkeys_test.go` | 202 |
| 28 | `internal/pkg/AGENTS.md` | 110 |
| 29 | `internal/pkg/apt.go` | 328 |
| 30 | `internal/pkg/apt_bench_test.go` | 148 |
| 31 | `internal/pkg/brew.go` | 366 |
| 32 | `internal/pkg/brew_test.go` | 213 |
| 33 | `internal/pkg/manager.go` | 268 |
| 34 | `internal/pkg/manager_test.go` | 311 |
| 35 | `internal/pkg/mock_manager.go` | 176 |
| 36 | `internal/pkg/pacman.go` | 470 |
| 37 | `internal/pkg/pacman_test.go` | 165 |
| 38 | `internal/pkg/update.go` | 166 |
| 39 | `internal/pkg/update_test.go` | 191 |
| 40 | `internal/runner/bash.go` | 165 |
| 41 | `internal/runner/bash_test.go` | 256 |
| 42 | `internal/scripts/scripts.go` | 398 |
| 43 | `internal/scripts/scripts_test.go` | 332 |
| 44 | `internal/theme/names.go` | 21 |
| 45 | `internal/theme/palettes.go` | 266 |
| 46 | `internal/theme/theme.go` | 32 |

## Finding count

- Critical: 0
- High: 5
- Medium: 17
- Low: 5
- Total: 27
