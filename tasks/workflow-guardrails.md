# Release-Remediation Workflow Guardrails

Current September execution is governed by [integration-plan-2026-09-05.md](integration-plan-2026-09-05.md), including its explicit existing-candidate import exception. The state-machine/ledger procedure below remains the historical July protocol. New fixes retain bounded ownership, review, and verification under the current plan. All subagents use Astra.

These controls are mandatory for every remaining release-remediation slice. They
exist to keep review findings from silently expanding an uncommitted change set.

## Slice contract

Before production edits, root records and freezes:

- one outcome and one authority boundary;
- one delivery surface: headless CLI, Manage, or wizard;
- included behavior and explicit deferrals;
- an exact writable path allowlist and one Astra implementer writer;
- the invariant/test matrix, verification commands, and intended commit message.

The state machine is:

`planned -> contract-frozen -> tests-red -> implemented -> reviewed -> verified -> committed`

Active work is never marked complete. A task checkbox becomes `[x]` only in the
commit that delivers its verified implementation. Verification evidence is bound
to that commit candidate and expires after any production edit.

## Hard scope ceilings

The default ceiling is six production files, sixteen total files, or 800 changed
lines. Exceeding any one ceiling stops production edits. Astra reviewer must split the work or
root must record a before-the-fact exception in the active scope contract.

A new package, authority domain, execution surface, or architectural dependency is
always a separately planned slice. A slice may not evade these limits through tiny
intermediate commits that do not form independently reviewable semantic units.

The checked contract is `tasks/current-slice.scope`. Contract changes are committed
alone before production work; the checker rejects self-modification during a slice.
After staging only that file, root uses
`bash scripts/check-slice-scope.sh --contract-candidate`. The one-time initial guardrail
checkpoint instead uses `bash scripts/check-slice-scope.sh --install`, which rejects
staged product work even when recovering around an existing unstaged slice. Run
`make slice-check` before and after each implementation handoff and before every
verification run. Any path outside the allowlist, exceeded budget, malformed contract,
mixed index/worktree payload, ignored staged path, or unrecognized Git status fails closed.

The executable workflow uses hardcoded Make targets; environment variables never select
a weaker mode:

- `make slice-check-test` runs the policy harness.
- `make slice-check-contract-candidate` checks the staged scope-only transition.
- `make slice-check-test-candidate` checks the isolated red-test commit.
- `make slice-check-candidate-digest` prints the exact `Candidate-SHA256: <digest>` trailer
  from a staged reviewed-to-verified scope and an unstaged homogeneous payload.
- After committing that verified scope with the printed trailer, stage the unchanged payload
  and run either `make slice-check-candidate` or `make slice-check-guardrail-candidate`.
- `make slice-check-ledger-candidate` checks the isolated final ledger row.

Digest calculation uses disposable index and object storage and must not mutate the real
index, refs, object database, status, or locks. The ledger path is reserved to its dedicated
mode. Each ordinary closure appends one `normal-v1` row with reason `-`, a complete adjacent
scope-only state chain, a nonempty payload, and exact digest evidence. A new planned slice
requires its predecessor's unique final closure and a slice ID never previously reserved.

## Agent ownership

- Astra reviewer owns the contract, scope decisions, and final read-only review.
- One Astra implementer owns each writable path allowlist.
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
- Astra reviewer and the adversarial reviewer report no sustained blocker; and
- residual risks and deferred work are recorded.

Once this gate is green, commit immediately. Optional improvements become named
follow-up slices instead of extending the verified diff. Full integration gates are
rerun after combining any parallel lanes.

After exact staging, the matching hardcoded candidate target must also pass. It rejects unstaged
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
