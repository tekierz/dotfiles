# September integration and remediation plan

User authority: use `release-remediation` (starting at `d77c0f7`) as the integration branch; save all uncommitted/untracked work, publish historical branches/stashes as clearly dated archive refs, analyze dirty-worktree work, then execute the audit development sequence. Use `gpt-6-astra` for every subagent with task-appropriate reasoning effort.

## Preservation and branch policy

- Preserve every original worktree and stash before any source integration. Do not reset, clean, delete branches, move tags, or force-push.
- Private recovery bundle and working-file/build-product snapshots: `.git/recovery/2026-09-05-integration/`. Original global model instructions are saved there too.
- Publish history under `archive/2026-09-05/branches/`, `remote-before/`, `stashes/`, and `worktrees/`. Archive means saved history, not accepted code or release authority.
- Keep generated binaries/browser scratch local in the recovery archive. Durable audit source and the user's proposed Pi specification are saved in source history; the Pi proposal is not implementation scope.
- `release-remediation` is the integration branch. Public `main`, tags, publication and owner-machine mutations are separate promotion boundaries.

## Explicit transition from the July program

This user-authorized September plan governs current work. The committed G3F scope and July ledger describe historical slices; they are not a bulk-import contract. Preserve their historical claims and do not manufacture tests-red states or normal-v1 closures for the 35 existing product commits.

Before-the-fact import exception: review the existing `890c446` product delta (75 production files, +5610/-984 lines) together with its saved release-preparation snapshot `bb1f472c`. Work on an isolated candidate, with defects and weakened test coverage explicitly recorded. The large import is permitted as preservation/integration of existing work, not new uncontrolled implementation. No release-ready claim follows from import.

Current remote CI is an integration/promotion acceptance gate. Establish a draft PR and repair integration-branch triggers/shared checks while the candidate is being tested; the older G2 requirement to obtain green remote CI before importing any product source is superseded only for this isolated September candidate. Do not promote to main or publish a release until complete candidate checks pass.

New fixes remain bounded, with named ownership, focused failing tests first where behavior changes, independent review, and meaningful verification. Default per-fix limits remain six production files / sixteen total files / 800 lines; split or explicitly record a scoped exception before exceeding them. Root alone stages, commits and integrates. Agents may not modify shared owned paths concurrently.

## Ordered work

- [x] Inventory 37 worktrees and preserve six dirty trees, historical refs and stashes locally.
- [x] Create 97 dated archive refs, including three source snapshots; preserve generated artifacts locally.
- [x] Update repository/global model policy to Astra; preserve prior global instructions.
- [x] Complete dirty-worktree disposition report and record unique salvage candidates.
- [x] Scan archive source history and push dated refs plus the audit/planning checkpoint without force.
- [x] Prepare isolated combined product/release-preparation candidate; full Go1.26.8 suite passed on the tested intermediate candidate.
- [x] Fix APT receipt-state semantics independently of update execution (candidate `a8c69d0`; focused tests/package race/vet passed).
- [ ] Fix cancellation wrappers and restore meaningful privileged-streaming acceptance coverage.
- [x] Replace catalog-wide payload retention (`3dcec08`; full affected packages and focused race passed).
- [x] Fix App-owned async results, duplicate-profile creation, live profile application, theme-save errors and Updates viewport in sequential UI slices.
- [ ] Adapt useful historical context/start-screen/relative-HOME/AUR observations only after confirming current defects.
- [x] Upgrade to a supported patched analysis-compatible Go toolchain and unify PR/integration/main/tag quality gates.
- [ ] Verify the combined candidate, integrate it to release-remediation, sync it and establish current remote CI/protection evidence.
- [x] Prepare the external Homebrew ownership-safe formula change with verified artifact inputs.
- [ ] Record remaining real-platform, Homebrew, owner-hardware and publication gates; do not claim unrun gates passed.

## Initial bounded contracts

### APT receipts (independent writer)

Paths: `internal/pkg/apt.go`, `internal/pkg/apt_receipts_test.go`, `internal/pkg/manager_executable_identity_test.go`, and `internal/pkg/apt_bench_test.go` only if its obsolete batch fixture changes.

Use actual installed/error state, not desired selection. Held installed packages remain installed; unpacked, half-configured, residual config and error states do not become healthy receipts. Batch/individual queries agree, preserve multiarch identities/versions, propagate failure/cancellation, and never mutate hold state or use sudo. No runner/update-execution edits. Acceptance: targeted receipt/identity tests, package tests/race, format/vet.

### Cancellation terminal wait (safety writer after review)

Frozen paths: `internal/installapply/recipe.go`, `recipe_test.go`, `recipe_lifecycle_test.go`, and `service_lifecycle_test.go` (or existing service tests if fixture reuse requires it). Cancellation must request teardown and wait for terminal cleanup before returning/releasing the operation lock. Preserve terminal cleanup errors. Do not weaken the supervisor's trusted-path boundary to make fake-sudo tests pass. Privileged runner tests are a separate slice if they require another authority surface.

### UI lane (one writer, sequential commits)

First globally reduce Updates, Backups and Users results with generation/operation identity; maintain opaque backup confirmation. Then create-only profiles, successful-switch App/context adoption, visible theme failure, and cursor-following Updates viewport. Each slice has its own exact path allowlist and acceptance matrix before editing. Shared `app.go`/screen handlers serialize this lane.

## Evidence and review

Current audit: `tasks/codebase-git-audit-2026-09-05.md`; human guide: corresponding `.html`.

Dirty-worktree report: `tasks/dirty-worktree-analysis-2026-09-05.md`.

Candidate spot-checks already show cancellation wrappers still return before terminal wait, and per-entry backup limits still permit retention of every payload. Both findings remain open. Release-preparation also removed two real privileged-streaming assertions without equivalent production-path replacement; restore meaningful coverage before acceptance.

Existing newer fixes must be checked with acceptance assertions, not by reading a green result from defect-asserting audit probes. Some saved probes intentionally pass when a bug exists.

### UI async ownership scoped exception (before edits)

The P2-5 slice may touch eight production paths because every launch must carry generation identity: `internal/ui/app.go`, `cache.go`, `state_helpers.go`, `screen_mainmenu.go`, `streaming.go`, `screen_update.go`, `screen_backups.go`, and `screen_users.go`. Keep reducers/helpers in app.go; cap 800 changed lines and sixteen total paths. No profile/theme/viewport fixes in this slice. Agent freezes exact names against the tree before editing.

### Shared quality gate and toolchain (root writer)

Paths: `go.mod`, `Makefile`, `.github/workflows/ci.yml`, `release.yml`, `codeql.yml`, `dependency-review.yml`, `docs/releasing.md`, `docs/security-scanning.md`. Use supported Go 1.26.8 and a Go 1.26-compatible pinned linter; verify official release metadata. Make CI itself callable from tags so direct PR/integration checks retain existing required names. Publication depends on the complete reusable gate; retain tag ancestry to main. Validate actionlint, package/tool compatibility, security and complete candidate gates before acceptance. No release tag or main promotion is authorized by this patch.

### Linter migration follow-up (before edits)

New golangci-lint enables optional Staticcheck QF1012 formatting suggestions (96 sites), absent from standalone Staticcheck's normal gate. Exclude only that optional quick-fix rule while retaining all security checks and normal Staticcheck. Review each new gosec diagnostic against its exact boundary, using per-line rule-specific explanations only for false positives; fix real defects. The descriptor conversion slice owns `internal/pkg/executable_identity.go`, `npm_execution_identity.go`, `internal/safefile/anchored_unix.go`, `directory_unix.go`, `parent_chain_unix.go`, `internal/runner/exit_observer_darwin.go` (six production paths). A second slice owns command-factory false-positive explanations in runner/bash.go, tools/neovim.go, tools/tmux.go, installplan/service.go. UI diagnostics serialize with the UI writer. Security rules are not globally disabled.

### Catalog metadata ownership (safety writer)

Freeze compact recursive authority in new safefile directory_authority files; edit backup catalog/limits and tests. Aggregate caps: 16,384 accepted restore items and 8 MiB manifest text, with existing per-backup snapshot limits. Whole-list failure on budget exhaustion preserves retention safety; selected restore captures immutable bytes. Regression and replacement/mutation tests precede acceptance.

### Historical relative-HOME salvage (root writer)

Only `internal/config/config.go` and new `config_home_test.go`: ConfigDir must return unavailable when HOME is relative and no absolute XDG root exists; absolute XDG remains valid. Red regression then config tests. This adapts one useful historical dirty-tree intention without importing obsolete filesystem helpers.

### Updates viewport (root writer)

Only `internal/ui/screen_update.go` and new `update_viewport_test.go`. Derive a height-bounded visible range from current cursor and available chrome/log space, retain full package identities/selections, keep header/help/cursor visible at80x24 and smaller supported sizes, and exercise navigation/resize. No shared App fields or provider execution changes.

### Privileged execution verification slices

R1 (safety writer): private actual launcher construction in `runner/privileged_launcher.go` plus tests, preserve trusted defaults in bash.go, replace obsolete argv-only test, exercise sequence via real fake-shell phases. R2 separate Linux-only opt-in supervisor harness, isolated child processes, fake root-owned apt/pacman in ephemeral GitHub runner; no owner sudo/package execution. Root wires exact guarded test invocation. Package-manager phase wiring is a later bounded pkg slice. Per-slice limits remain six production/sixteen total/800 lines.

## Intermediate execution evidence

- `a8c69d0`: actual APT receipt state; focused/package race/vet passed.
- `85001c4`: cancellation waits for terminal cleanup; package/cask and real operation-lock contention regressions passed.
- `071dc45`: App-owned async results; full UI tests/race and stale/duplicate-generation cases passed.
- `3dcec08`: compact catalog authority; full safefile/backup/UI/CLI and focused race passed. Per-backup snapshot memory remains transient.
- `05aaf48`: create-only profiles; duplicate bytes and cooperating cross-process creators verified, config/UI race passed. Cold operation-state bootstrap can explicitly fail closed before the profile lock; same-UID noncooperating rename limitations are unchanged.
- `274f070`: relative HOME rejected; historical intention adapted with red/green regression.
- `03c0347`: Go1.26.8/shared workflow gate. Current vulnerability scan: zero reachable findings, one uncalled module advisory. Module tidy/verify and GoReleaser/actionlint checks passed. Full candidate/remote gates remain required after final edits.
- `05cbc31`: actual privileged launcher construction/sequence tests; runner/installapply/race passed. Linux supervisor execution and pkg phase-wiring verification are separate follow-ups.
- Homebrew ownership-only patch, tests and verified existing source SHA saved in `tasks/audit-work/2026-09-05/homebrew-proposal/`; no live install or tap publication.

The first upgraded all-suite run used `/tmp` fixtures with a mismatched inherited group, causing authority checks to reject them. That run is invalid environment evidence. Repeating with the owner-private TMPDIR passed the full suite; do not weaken ownership checks to accommodate an unsuitable harness directory.

### First remote CI repair (root writer)

Run33988086766 passed Security/macOS tests/both builds; Linux tests and lint failed. Narrow tests-only paths: tools/config_validation_test.go and yazi_test.go must isolate inherited XDG/Yazi roots; pkg/executable_identity_test.go must keep platform-dependent Stat_t.Dev conversion with a justified unconvert annotation. Investigate UI/manage_pi_reachability_test.go platform-bound snapshot mismatch before changing expectations. Preserve product fail-closed behavior. Regression under hostile inherited paths, focused checks, then remote rerun.

### Package phases and startup completion

Package phase scope: apt.go, pacman.go, manager_executable_identity_test.go and new privileged_phase_test.go. Private per-instance factories retain production trusted defaults, validate captured identities before every phase, exercise actual public manager methods, phase draining, failure/cancellation and identity drift. Commit af2df9d; pkg/runner/installapply, package race and vet passed.

Startup scope: app.go, screen_manager.go and start_screen_test.go. Prepare the selected handler for first render; App.Init owns its one initialization command. Commit 0dfc0fd; initial-route regressions were red, then focused startup/manager race and vet passed. Profile adoption (55c3878), theme retry (5215292) and bounded Updates viewport (9ca40e5) have focused and combined UI race evidence.

CI fixture regressions reproduced locally: inherited XDG/Yazi roots and host-dependent npm/node observation. Tests now supply isolated roots and observed native executable fixtures without running installs. The portable device type fixture uses explicit normalization, avoiding platform-specific unused lint suppression. Combined candidate full tests and lint passed; final race and remote Linux supervisor evidence are in progress.
