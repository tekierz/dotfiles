# `dotfiles plan --json` v1 contract

Status: implemented on 2026-07-12 through the deterministic read-only command;
hash-bound noninteractive apply remains a separate deferred slice.

## Decision

Directly wrapping the current TUI planner is **No-Go**. Planning and accepted execution
authority are private to `internal/ui`, while `operation.Plan` contains raw targets,
observation sources, backup paths, and other fields that are not a safe public document.
The command must use a neutral headless planning service shared by Cobra and Bubble Tea.

Version 1 is intentionally install-only and accepts explicit repeated tool intent:

```text
dotfiles plan --json --tool codex --tool pi
```

There are no implicit defaults, no `App` construction, no TUI state, and no profile input.
An omitted `--tool` is an explicit `intent_required` outcome rather than permission to infer
the dashboard defaults. Configuration planning can join the public contract only after the
neutral planner has parity tests for its private authority capture.

## Dependency order

```text
public projection tests
  -> neutral headless explicit install-only planner + TUI parity
  -> complete private authority fingerprint
  -> plan --json command
  -> hash-bound noninteractive apply
  -> owner-hardware verification
  -> installation-profile loading
```

Profiles are last. A profile will eventually be another explicit intent source; it may not
bypass planning, preview, authority hashing, backups, journaling, revalidation, or rollback.

## Invocation and side-effect boundary

- `--json` and at least one repeated `--tool <registry-id>` are the only v1 intent surface.
- Tool IDs are normalized, deduplicated, and stable-sorted before planning. Unknown IDs are
  blocked; an empty normalized set produces `intent_required`.
- The command detects platform once, detects the package manager once, and collects exactly
  one generation-tagged installation snapshot.
- `intent_required` and syntactically invalid intent are resolved before constructing the
  collector or planner and therefore perform no installation probes.
- The planner may make bounded, anchored reads needed to capture accepted private authority.
  It performs no second installation snapshot, network/auth/service probes, prompts, TUI
  initialization, sudo, locks, state bootstrap, backups, journal writes, config writes, or
  filesystem creation.
- Unknown, unsupported, stale, error, environment-mismatched, or recipe-drifted evidence for
  any selected tool fails closed. No supported subset is silently planned.

## Public JSON envelope

Every nonfatal JSON outcome is one versioned object:

```json
{
  "schema_version": 1,
  "kind": "dotfiles.plan",
  "status": "ready",
  "platform": "macos",
  "manager": "brew",
  "intent": {
    "source": "explicit_tools",
    "tools": ["codex", "pi"],
    "digest": "sha256"
  },
  "snapshot": {
    "schema_version": 1,
    "generation": 1,
    "public_digest": "sha256"
  },
  "authority": {
    "plan_hash": "sha256",
    "public_digest": "sha256"
  },
  "capabilities": {
    "installation": "planned",
    "config": "not_planned",
    "service": "not_collected",
    "auth": "not_collected",
    "apply": "hash_required"
  },
  "summary": {
    "apply": 2,
    "skip": 0,
    "blocked": 0,
    "backup_targets": 0
  },
  "actions": []
}
```

Allowed statuses are `ready`, `no_changes`, `blocked`, and `intent_required`.

- `intent_required` has an empty `intent.tools` and `actions`, and omits snapshot and private
  authority digests because collection and planning did not run.
- `blocked` contains only bounded public decision codes/reasons and redacted public evidence.
  It omits `plan_hash` and every private digest.
- `ready` includes the complete private-authority `plan_hash` and advertises apply as
  `hash_required`. This declares that only an exact fresh-plan hash match can authorize apply;
  it does not by itself register or promise availability of the apply command.
- `no_changes` and `blocked` omit `plan_hash` and advertise apply as `not_available`.
- `intent_required` retains its zero capability object because collection and planning did not
  run.
- The private installation snapshot digest is never published. `snapshot.public_digest`
  binds only the redacted public snapshot projection.
- `authority.public_digest` is SHA-256 over the canonical public document with that field
  omitted. It is an integrity/addressability value and never authorizes apply.
- Timestamps and operation IDs are absent. Planning creates no operation, and clock/randomness
  cannot change deterministic bytes.

## Public action projection

Actions preserve accepted execution order and include an explicit zero-based `ordinal`.
They may expose:

- action ID, kind, tool ID, description, disposition, bounded reason code/reason;
- ownership and reversibility;
- redacted observation facts limited to `exists` and `managed`;
- a validated install recipe projection: schema, platform, manager, typed detector, exact
  reviewed provider/package/cask/argument arrays, authentication expectation, risk, and the
  independent recipe digest.

They never expose:

- raw `operation.Plan` JSON or an `acceptedTarget`;
- absolute or relative paths, observation sources, backup paths, usernames, HOME/XDG values,
  original bytes, config contents, parent chains, directory snapshots, modes, or private
  state/snapshot digests;
- desired-config digests, existing-content digests, or path-derived digests that could
  fingerprint private local state;
- credentials, environment values, command output, raw errors, or control/bidi characters.

Targets and backups use deterministic document-local references such as `target-0001`.
Path hashes are not published because predictable paths can be recovered by dictionary attack.
All arrays are non-null. Struct field order and accepted action order define canonical output.
All strings pass one shared field-aware path/credential/control-character redactor before the
public digest is computed.

## Complete private authority fingerprint

Noninteractive apply must not accept an operation-document hash alone. The public `plan_hash`
is derived from a private canonical fingerprint that binds:

- operation document hash and normalized explicit intent digest;
- installation snapshot schema/generation/platform/manager/private digest;
- every selected install recipe digest and detector observation;
- target kind, relative private identity, existence, content revision/digest, and permissions;
- directory-root snapshot digest and accepted parent-chain identity;
- accepted operation-state plan and rollback/backup coverage.

Only the resulting digest is public. Private fields remain process-local. Apply later rebuilds
one fresh plan from the same explicit intent, compares the exact supplied hash, and executes
only the newly captured private authority after a match. Public JSON is never deserialized into
execution authority.

## Output and exit contract

| Exit | Output contract |
|---|---|
| `0` | One newline-terminated `ready` or `no_changes` JSON object on stdout; stderr empty. |
| `2` | One newline-terminated `intent_required` or `blocked` JSON object when JSON mode can represent the domain outcome; stderr empty. Invalid Cobra syntax uses bounded stderr and empty stdout. |
| `1` | Fatal collector/planner/marshal/write failure; no valid success document, bounded generic stderr (`plan collection failed`), never raw errors. |

The complete document is projected, redacted, canonicalized, digested, and marshaled in memory
before one writer call. A short or failed write returns exit 1 and is never reported as success.
No userspace API can mathematically retract bytes after an operating-system partial write, so
the contract promises atomic preparation and failure reporting—not impossible rollback of bytes
already accepted by the OS.

## Tests-first acceptance matrix

### Slice 1 — public projection

- Exact schema/key golden and one trailing newline.
- Non-null arrays, accepted action order, explicit ordinals, stable tool normalization.
- Repeated projection is byte-identical across clock/random changes.
- Public digest independently recomputes; public changes alter it.
- Sensitive-only fixture changes neither leak nor change public bytes/digest.
- Desired-config and existing-content digests are absent even when the private plan contains
  them; only safe existence/ownership enums cross the projection boundary.
- Redaction matrix covers HOME/XDG, absolute/relative paths, usernames, tokens, API keys,
  authorization headers, credential assignments, raw dial errors, controls, and bidi text.
- Clone tests prove mutating actions, recipes, observations, or projections cannot mutate the
  accepted private plan.

### Slice 2 — neutral explicit install-only planner

- Cobra and TUI adapters produce identical actions, recipes, snapshot binding, and private
  authority for the same explicit tool intent.
- Duplicate/reordered `--tool` input produces one canonical intent and deterministic plan.
- Empty intent returns `intent_required` without calling platform, manager, collector, planner,
  config loader, HOME resolver, or clock.
- Unknown tool, unavailable tool, Unknown/Unsupported presence, stale/error snapshot, collector
  mismatch, recipe drift, and platform/manager drift fail closed without a partial plan.
- Collector/platform/manager call counts are exactly one; no network/auth/service probe occurs.
- HOME/XDG/state inventory, contents, modes, and symlink topology are unchanged.

### Slice 3 — private authority fingerprint

- Identical intent and authority reproduce the same hash across time/processes.
- File bytes, existence, mode, target kind, directory root, parent chain, state plan, snapshot,
  recipe, detector, platform, manager, or intent drift changes the hash.
- Redacted public-equivalent plans can still have different private hashes.
- Blocked/intent-required documents omit the hash and all private digests.

### Slice 4 — command and exits

- Registered command grammar, repeated local `--tool`, no positional args, and no prompts.
- Exit/output matrix above, exact stdout/stderr separation, no raw error leakage.
- Marshal completes before the writer is called; short writer and failing writer return exit 1.
- Ready/no-change output is deterministic and contains no timestamps/operation IDs.

### Slice 5 — apply precursor

- Public JSON cannot reconstruct or mutate private authority.
- Fresh replan plus exact hash is required; missing/wrong/expired hash performs no mutation.
- Drift blocks before lock/journal/backup/product mutation.
- Matching apply preserves current lock, journal, mandatory rollback point, revalidation,
  cancellation, and rollback behavior.

## Commit boundaries

1. `Safety - define public plan projection`
2. `Architecture - extract explicit headless install planning`
3. `Safety - bind complete private plan authority`
4. `Feature - add deterministic plan JSON`
5. `Feature - add hash-bound noninteractive apply`
6. `Feature - load reviewed installation profiles` only after owner-hardware verification
