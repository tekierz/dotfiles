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
- [ ] Scan archive source history and push dated refs plus the audit/planning checkpoint without force.
- [ ] Prepare isolated combined product/release-preparation candidate and record current tests.
- [ ] Fix APT receipt-state semantics independently of update execution.
- [ ] Fix cancellation wrappers and restore meaningful privileged-streaming acceptance coverage.
- [ ] Replace catalog-wide payload retention with practical bounded listing/selected-source authority.
- [ ] Fix App-owned async results, duplicate-profile creation, live profile application, theme-save errors and Updates viewport in sequential UI slices.
- [ ] Adapt useful historical context/start-screen/relative-HOME/AUR observations only after confirming current defects.
- [ ] Upgrade to a supported patched analysis-compatible Go toolchain and unify PR/integration/main/tag quality gates.
- [ ] Verify the combined candidate, integrate it to release-remediation, sync it and establish current remote CI/protection evidence.
- [ ] Prepare the external Homebrew ownership-safe formula change with verified artifact inputs.
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
