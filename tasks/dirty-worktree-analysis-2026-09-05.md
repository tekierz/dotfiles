# Dirty worktree work analysis — 2026-09-05

**Save all six dirty worktrees; integrate their work selectively.** Three hold source or documentation worth retaining, and three hold only visible generated executables. Preservation protects history and original bytes. It does not mean every old implementation belongs in the current product or that an archived candidate is release-ready.

This consolidates the preservation inventory and two work-value reviews against the [September audit](codebase-git-audit-2026-09-05.md). The inventory observed 37 worktrees, six dirty worktrees, and 119 visible dirty paths before preservation. Counts below describe that original state, not the checkout after archive commits. This synthesis makes no Git changes and runs no additional product tests.

## Preservation status and meaning

The integration owner reports a private bundle and worktree tar archives under `.git/recovery/2026-09-05-integration`, plus 97 archive refs. Source/document snapshots are:

| Snapshot ref | Commit |
|---|---|
| `archive/2026-09-05/worktrees/integration-root` | `2b83147` |
| `archive/2026-09-05/worktrees/open-source-release-readiness` | `bb1f472` |
| `archive/2026-09-05/worktrees/audit-remediation` | `776a8cf` |

The integration owner subsequently published all 97 refs under `archive/2026-09-05` and verified exact remote commit matches; see [sync evidence](repository-sync-2026-09-05.md). Generated binaries remain local-only. Private recovery preserves material excluded from source snapshots, including generated artifacts. Do not confuse publishing an archive ref with accepting its contents into the product branch.

## The six worktrees

| Worktree and original branch | Dirty work | What to save and what to integrate |
|---|---|---|
| Root: `/Users/tiki7/Desktop/Projects/dotfiles`, `release-remediation` | Two tracked task-file changes; 36 untracked audit/spec/browser files | Retain audit Markdown/HTML, durable evidence, lessons and roadmap changes in source history. Keep the Pi spec as a proposed exploration, not approved implementation scope. Browser caches and generated rendering outputs belong in local recovery; retain canonical report and reproduction instructions. |
| `dotfiles-worktrees/open-source-release-readiness`, `feature/open-source-release-readiness` | 34 tracked modifications; 16 untracked files after expanding directories | Preserve all source/docs. Most changes are useful, but reconcile them after the newer product baseline and repair coverage/gate gaps before integration. Details below. |
| `.claude/worktrees/audit-remediation`, `worktree-audit-remediation` | 24 tracked changes, including deletions; two untracked documents | Preserve the full historical patch. Most code is superseded; extract four review intentions below instead of merging the old implementation wholesale. |
| `dotfiles-worktrees/bc2-tui-restore`, `feature/bc2-tui-restore-dogfood` | One untracked `dotfiles` executable | Local artifact archive only. No unique dirty source implementation was identified. Keep its branch history separately. |
| `dotfiles-worktrees/local-dogfood`, `feature/local-dogfood-2026-07-14` | Untracked `bin/dotfiles-linux-amd64` and `bin/dotfiles-linux-arm64` | Local artifact archive only. Embedded version label is `ea54a14-dogfood`; retain history and rebuild from accepted source for releases. |
| `dotfiles-worktrees/local-dogfood-product-2`, `feature/local-dogfood-product-2` | Untracked `bin/dotfiles-linux-amd64` and `bin/dotfiles-linux-arm64` | Local artifact archive only. Embedded version label is `890c446`. Its committed 35 newer product commits matter substantially; these dirty binaries add no source changes. |

The `dotfiles-worktrees/` entries above are siblings under `/Users/tiki7/Desktop/Projects/`. The nested historical worktree must be archived independently, avoiding recursive duplication inside the root archive.

## Root: preserve the audit and planning work

The unique work is the September evidence-backed code/Git audit, its navigable HTML presentation, saved probes and verification results under `tasks/audit-work/2026-09-05/`, and the updated task/lesson records. These explain both confirmed failures on `d77c0f7` and why the newer `890c446` branch should be assessed before writing duplicate fixes.

Preserve `tasks/pi-agent-integration-spec.md` for its design work while retaining its proposed status. Do not let restoring a document silently approve its roadmap. Merge task history deliberately: the current audit's failed and unexecuted gates must survive reconciliation with older all-green checklists.

`.playwright-cli/` contains local browser automation state. `output/playwright/` contains screenshots, a print-check PDF, verification JSON, and a development brief. These are generated evidence rather than product source. Local recovery retains their value without making them build inputs; canonical HTML and reproduction documentation are the durable source counterpart.

## Release-readiness: useful work with specific acceptance conditions

This tree starts at `890c446`, 35 product commits beyond the audited root `d77c0f7`. Its dirty patch is approximately 520 insertions and 186 deletions in tracked files. Applying its fixture changes directly to the older root would test the wrong contracts.

### Retain and reconcile onto the newer product baseline

- **Explicit phase and authority handling:** `apply.go` rejects prerequisite-only completion as full success; recipe/UI switch cases explicitly reject npm in the manager-only loop and replan-only public projections as execution authority. These preserve fail-closed behavior rather than unlock an alternate install route.
- **Lifecycle-aware cleanup:** unused compatibility helpers are removed; runner lint annotations explain that process lifecycle/supervisor code owns cancellation. Keep that ownership intact when reconciling the patch.
- **Fixtures for current product contracts:** pinned Git artifacts, typed health observations, scoped manager identity, bounded reads, npm prerequisite/continuation phases, and responsive phase completion displays replace stale expectations. Implicit wizard selection filters unavailable tools; explicit requests retain rejection of unknown health. Coordinator generation normalization does not imply accepting a stale outer UI snapshot.
- **Public project materials:** contributor guidance, issue/PR templates, truthful bundled-helper descriptions, license texts and third-party notices are substantive source work. Package them together with `.goreleaser.yml` timestamp and archive-content changes; reconcile notices if dependencies change.
- **Security automation groundwork:** actionlint, broader ShellCheck, Gitleaks, CodeQL, dependency review, timeout and stable aggregate jobs provide useful foundations. The Gitleaks exception is narrowly scoped to a specific fixture and rule/path, not a broad exclusion.

### Repair before treating the candidate as accepted

1. **Replace lost execution coverage.** `internal/pkg/manager_executable_identity_test.go` removes apt/pacman hostile-PATH streaming executions because the old fake sudo path cannot satisfy the new trusted supervisor. AST checks do not replace execution assertions. Add safe tests of actual phase order, captured executable/arguments, first-phase failure/cancel preventing phase two, privileged child lifetime, and absence of hostile-PATH fallback. The review found no dedicated privileged-supervisor tests; the retained sudo argv test constructs old argv locally without exercising production.
2. **Complete and align release gates.** The proposed tag workflow still lacks golangci-lint and macOS runtime gates; integration-branch pushes and supplemental workflow coverage need reconciliation. Require a shared complete gate before publication and match actual remote required contexts. Preserve ancestry-to-main as the current publication boundary unless deliberately changed.
3. **Repair the toolchain/security baseline.** The candidate retains Go 1.25.6, whose exact vulnerability scan failed in the September audit. Select a supported patched toolchain with compatible analyzers and rerun the final candidate gates; July success is not current evidence.
4. **Make reporting channels real.** Security/conduct policy points to private reporting that still needs enabling and an unspecified fallback contact. Confirm an actual private channel before presenting those instructions as usable; do not invent maintainer details.
5. **Reconcile task and distribution claims.** Preserve the July checklist as history, reopening current failed/unexecuted gates. Documentation does not repair the separate live Homebrew formula or establish that new behavior is distributed.
6. **Rehearse release packaging.** Verify license contents, source/binary archives and reproducibility from committed accepted source. Timestamp settings alone do not prove reproducible release output.

Fresh follow-up verification on the unchanged dirty release tree passed `go test ./internal/pkg ./internal/tools ./internal/ui ./cmd/dotfiles` with Go 1.25.6, with uncached package results. ShellCheck over `scripts/*.sh tests/*.sh` and whitespace checks also passed. That is useful bounded evidence, not a whole-suite/race/security/release rehearsal or proof that removed assertions remain covered.

## Historical audit-remediation: extract intentions, avoid regressions

The old tree is based on `bb813df`. Its Go-only retirement work was valuable historically, but current integration already contains stronger successors:

| Historical work | Current disposition |
|---|---|
| Remove tracked binary, stale formula and PowerShell launcher | Already represented by `ca4242f`; old Makefile would reintroduce hardcoded version 2.0.1. Archive only. |
| Retire Bash product, embedded CLI tests and rewrite docs | Superseded by `9d6eab5` and current migration/distribution tests; old registry counts and guidance are stale. Archive only. |
| Backup source symlink checks | Superseded by `dad90ae` and anchored safefile checks, with broader source/intermediate/root symlink tests. Do not replace these with the old check-then-read helper. |
| Reject relative XDG configuration path | Already covered by current config validation and tests. Archive only. |
| `SafeRemoveAllUnder` and destructive call rewiring | Do not transplant. EvalSymlinks followed by RemoveAll has a check/use gap and lacks the accepted snapshot/parent authority used by newer code. |
| Global `manageSavedMsg` handling | Flow was replaced by reviewed transactional Manage saves in `d1a738a`. Reintroducing it would target retired behavior. |
| Automatic stale-binary cleanup in the old deployment spec | Preserve as history only. It conflicts with current ownership-review/diagnose-without-deletion policy. |

Four useful intentions deserve focused review on the selected modern baseline:

1. **Absolute HOME validation.** Old `ConfigDir` changes also reject relative HOME; inspected `d77c0f7` and `890c446` only reject empty HOME. Establish caller impact and add focused tests before adapting this. No destructive escape was demonstrated; some downstream safefile consumers reject relative authority already.
2. **Manage screen context synchronization.** Theme/navigation/animation edits can leave `ScreenContext` stale. The old fix covers keyboard paths only and misses mouse navigation/animation changes. Adapt complete behavior for both inputs, alongside profile-state work, rather than copying obsolete methods.
3. **First-frame CLI routing.** `SetStartScreen` changes App fields while the manager's first render can still show the eagerly selected default screen. The old eager navigation fix discards the handler's Init command and can strand Manage cache loading. Preserve the intended first-render test, redesign initialization ownership, and verify the loader completes exactly once.
4. **AUR-only install capability without paru.** An unfinished old task identifies LM Studio's AUR recipe being handed a generic pacman manager. Source inspection found no relevant install capability gate. This is a candidate for focused plan/recipe tests and current package-repository validation, not a reproduced Arch installation defect. It is distinct from the audit's update-provider finding.

Old test-pass notes and deployment assumptions are provenance, not verification of today's product. The old lesson about re-baselining when worktree shape changes may be retained without importing an obsolete roadmap.

## Generated executables and provenance limits

The five visible untracked executables total 62,588,202 bytes. Also preserve ignored `bin/` and `dist/` output locally where present; `.DS_Store` is generated local metadata. These files are not source commits.

The dogfood Linux binaries were inspected through module metadata and SHA-256 hashing, never executed. Both sets report Go 1.26.1, CGO disabled, and the architecture implied by their names. Ignored Darwin binaries were also inspected as context. The embedded version labels match their worktree HEAD prefixes, but lack `vcs.revision`, `vcs.time`, and `vcs.modified`; linker labels do not prove exact source identity or clean builds. Preserve bytes and hashes for provenance and rebuild release artifacts from accepted source/toolchain.

## Evidence boundaries and next integration decision

This report synthesizes `/tmp/dotfiles-preservation-review.md`, `/tmp/dirty-worktree-analysis-old.md`, `/tmp/dirty-worktree-analysis-release.md`, and the durable [September audit](codebase-git-audit-2026-09-05.md). It reports those reviews' findings and the integration owner's preservation status; it does not independently verify archive contents or remote refs.

The preservation review's bounded text scan found no high-confidence credential-shaped values in visible untracked text and unstaged diffs. Three compiled binaries matched a provider-key regex without establishing an actual secret. The earlier history scan also had size/scope exclusions. Neither constitutes comprehensive secret clearance. Audit evidence includes local filesystem paths; generic logs, journals and diagnostic JSON must not be described as inherently share-safe. Run the established scanner on the publication candidate.

The correct integration order is to retain recovery and historical refs, select/reconcile the newer product baseline, adopt useful release-readiness groups with the repairs above, then verify the combined candidate against the preserved audit probes and complete gates. Remaining September product findings still require resolution; archiving their neighboring work does not close them. No package installation, destructive restore, real Linux manager run, release rehearsal, or remote-delivery verification was added by this synthesis.
