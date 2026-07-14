# Release-Remediation Workflow Guardrails

These controls are mandatory for every remaining release-remediation slice. They
exist to keep review findings from silently expanding an uncommitted change set.

## Slice contract

Before production edits, root records and freezes:

- one outcome and one authority boundary;
- one delivery surface: headless CLI, Manage, or wizard;
- included behavior and explicit deferrals;
- an exact writable path allowlist and one Terra writer;
- the invariant/test matrix, verification commands, and intended commit message.

The state machine is:

`planned -> contract-frozen -> tests-red -> implemented -> reviewed -> verified -> committed`

Active work is never marked complete. A task checkbox becomes `[x]` only in the
commit that delivers its verified implementation. Verification evidence is bound
to that commit candidate and expires after any production edit.

Contracts use schema v2. Every red-test path is declared by a repeatable `test=`
entry that exactly matches one unique `allow=` path; filename conventions grant no
test authority. Paths containing TAB, CR, or LF are invalid. Transitions are adjacent
and isolated: a slice may change only to the next state, and a new `planned` slice may
start only after the previous slice is `committed`. Scope, budgets, exceptions,
allowlists, ignores, and tests freeze when `contract-frozen` is entered.

## Hard scope ceilings

The default ceiling is six production files, sixteen total files, or 800 changed
lines. Exceeding any one ceiling stops production edits. Sol must split the work or
root must record a before-the-fact exception in the active scope contract.

A new package, authority domain, execution surface, or architectural dependency is
always a separately planned slice. A slice may not evade these limits through tiny
intermediate commits that do not form independently reviewable semantic units.

The checked contract is `tasks/current-slice.scope`. Contract transitions are committed
alone with `bash scripts/check-slice-scope.sh --contract-candidate`; the checker compares
the staged contract to the committed predecessor. At `contract-frozen`, staged red tests
use `--test-candidate` and may contain only explicit `test=` paths. Normal payload
candidates use `--candidate` only at `verified`. Allowlisted checker/control changes use
`--guardrail-candidate`; closure-ledger appends use `--ledger-candidate`. The one-time
initial guardrail checkpoint retains `--install` only as historical bootstrap support. Run
`make slice-check` before and after each implementation handoff and before every
verification run. Any path outside the allowlist, exceeded budget, malformed contract,
mixed index/worktree payload, ignored staged path, or unrecognized Git status fails closed.

Before the verified scope commit, root computes the exact candidate digest. That commit
contains exactly one `Candidate-SHA256` trailer; the digest never appears in the mutable
scope file. Candidate verification rebuilds a temporary Git index from HEAD, applies Git
filters and file modes with `git add`, hashes the direct NUL-delimited raw manifest, and
compares it to the trailer. Hash tool preference is `sha256sum`, `shasum -a 256`, then
OpenSSL. Staged payload and working tree must match exactly.

Normal closure order is: verified scope commit, payload commit, one append-only row in
`tasks/slice-commit-ledger.tsv`, then the committed-scope transition. A normal ledger row
binds the verified contract parent, payload commit, and candidate digest. Rewrites,
removals, non-parent ancestry, allowlist escape, or digest mismatch fail closed.

Two bootstrap exceptions are closed and non-precedential:

- G1's red test used schema v1 and the old checker; authority was its exact single path
  plus independent review.
- G1 upgrades the checker that verifies it. Independent review and behavior from the
  index-extracted checker are evidence, not a self-authenticating trust root. Its later
  ledger row uses `self-upgrade-not-self-authenticating`; the first post-G1 slice is the
  first normal v1 closure row.

## Agent ownership

- Sol owns the contract, scope decisions, and final read-only review.
- One Terra implementer owns each writable path allowlist.
- Adversarial agents are read-only.
- Parallel writers are allowed only on disjoint path allowlists.
- Root alone updates the active plan, stages, verifies, and commits.
- No agent may touch user-owned untracked files.

If writers overlap, all affected work pauses for reconciliation and both focused
test sets are rerun. An agent with no persisted checkpoint or useful report for 15
minutes is interrupted and replaced; two missed checkpoints require replacement.

## Review and re-planning limits

One contract review occurs before production work and one delta review after
implementation. A third review cycle, or a second cycle that discovers a new
production area, forces a new child slice. Security findings are never waived: they
are fixed inside the frozen contract or moved to an explicit blocking slice.

Stop and re-plan immediately when:

- an edit requires a path outside the allowlist;
- a scope ceiling is reached;
- an unrelated test fails;
- another writer changes an owned path;
- the same blocker appears in two status reports; or
- verification requires behavior recorded as deferred.

## Commit exit gate

Before starting another slice:

- all writers are idle;
- `make slice-check` passes and user-owned files remain untouched;
- formatting and `git diff --check` pass;
- focused tests and affected-package race tests pass;
- full tests, vet, pinned lint, and Staticcheck pass;
- Sol and the adversarial reviewer report no sustained blocker; and
- residual risks and deferred work are recorded.

Once this gate is green, commit immediately. Optional improvements become named
follow-up slices instead of extending the verified diff. Full integration gates are
rerun after combining any parallel lanes.

After exact staging, `make slice-check-candidate` must also pass. It rejects unstaged
scoped work and measures the staged bytes from the index, preventing an unstaged
revert from hiding a different commit payload from tests or review.

## Status reporting

Status reports contain only:

- the current named slice and state;
- the most recent commit;
- passed gates and the exact remaining blocker;
- the next bounded commit; and
- active agents and their owned lanes.

Use committed/active/blocked slice counts. Do not report a percentage unless it is
derived from a fixed, named slice list.
