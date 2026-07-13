# `dotfiles support --json` v1 contract

Status: approved for implementation after adversarial review on 2026-07-12.

This contract is the implementation gate for the open redacted support-bundle item in
[`tasks/todo.md`](todo.md). The todo item remains incomplete until the command, public
projection, read-only journal collection, documentation, and acceptance tests below are all
implemented and verified.

## Decision

Version 1 is exactly one JSON document written to stdout:

```text
dotfiles support --json
```

It does not create a ZIP, tar archive, directory, or other file. It does not upload, transmit,
copy, or automatically attach the document anywhere. There is no `--output` flag in v1. A user
can inspect stdout before deciding whether and how to save or share it.

JSON is the smallest secure first format because it has deterministic bytes, no archive entry
names or extraction behavior, no filesystem permission or timestamp metadata, and no automatic
artifact write. Archive and upload workflows require separate threat models and are deferred.

The support document is a new allowlisted public projection. It must never recursively redact
and serialize a private struct. In particular, neither the current doctor report nor an
operation journal record is safe to marshal directly.

## Command and side-effect boundary

- `--json` is required; v1 has no human-output mode and accepts no positional arguments.
- After syntax/flag validation and before collection, the command creates its own
  `signal.NotifyContext` for `os.Interrupt` and `SIGTERM`, derived from the Cobra context, and
  defers the returned stop function exactly once. Production uses the real signal constructor;
  tests inject a constructor seam and assert one construction and one stop call.
- Collection is read-only. It may inspect the running build, collect one installation-health
  snapshot, perform the existing bounded Homebrew ownership probe, and read an existing private
  operation journal through anchored filesystem APIs.
- Collection must not prompt, acquire a mutating lock, bootstrap state, create a directory or
  file, write a journal record, mutate configuration, install a package, contact an upload
  service, or initialize the TUI.
- Missing operation state or a missing operation-journal directory is a valid `not_present`
  result and must not cause the state namespace to be created.
- The full public document is validated, digested, marshaled, size-checked, and newline-terminated
  in memory before the single stdout write.

## Public JSON envelope

The exact v1 envelope is:

```json
{
  "schema_version": 1,
  "kind": "dotfiles.support",
  "outcome": "complete",
  "authority": {
    "public_digest": "sha256"
  },
  "build": {
    "collection": "collected",
    "version": "2.1.2",
    "version_source": "compiled",
    "go_version": "go1.x",
    "os": "darwin",
    "arch": "arm64",
    "vcs_state": "clean"
  },
  "installation": {
    "collection": "collected",
    "reason_code": "",
    "document": {}
  },
  "provenance": {
    "collection": "collected",
    "reason_code": "",
    "running_ownership": "homebrew",
    "path_match_count": 1,
    "legacy_binary_count": 0,
    "homebrew_installed": true,
    "finding_count": 0,
    "truncated": false,
    "findings": []
  },
  "operations": {
    "collection": "collected",
    "reason_code": "",
    "truncated": false,
    "records": []
  },
  "capabilities": {
    "build": "collected",
    "installation": "collected",
    "provenance": "collected",
    "operations": "collected",
    "config": "not_collected",
    "service": "not_collected",
    "auth": "not_collected"
  }
}
```

The v1 vocabulary is closed. No other string is accepted:

- `outcome`: `complete` or `partial`;
- build `collection`: exactly `collected`;
- build `version_source`: `compiled`, `development`, or `unknown`;
- installation and provenance `collection`: `collected` or `unavailable`;
- operations `collection`: `collected`, `not_present`, or `unavailable`;
- build `vcs_state`: `clean`, `modified`, or `unavailable`;
- provenance `running_ownership`: `homebrew` or `unverified`;
- operation status: `running`, `succeeded`, `failed`, or `cancelled`;
- action-count keys: `pending`, `succeeded`, `failed`, and `skipped`;
- rollback status when present: `succeeded`, `incomplete`, or `failed`;
- all uncollected capability values: exactly `not_collected`.

An unavailable installation section uses `document: null`. All arrays are non-null, including
empty findings and operation records. A `reason_code` is empty for collected/not-present data.
The exhaustive unavailable mapping is:

| Section | Private outcome | Public reason code |
|---|---|---|
| installation | context cancelled during/before collection | `collection_cancelled` |
| installation | any collector, snapshot-validation, or status-projection error | `status_unavailable` |
| provenance | context cancelled during/before collection | `collection_cancelled` |
| provenance | collector/subprocess failure | `provenance_unavailable` |
| provenance | unknown ownership/finding vocabulary or inconsistent report | `provenance_invalid` |
| provenance | PATH/legacy input exceeds its collection bound | `provenance_limit_exceeded` |
| operations | context cancelled during/before collection | `collection_cancelled` |
| operations | state anchor/read failure not classified as invalid content | `journal_unavailable` |
| operations | invalid entry, record, schema, authority, revision, or concurrent drift | `journal_invalid` |
| operations | entry, per-record, or aggregate byte bound exceeded | `journal_limit_exceeded` |

There is no generic `unknown` reason and raw errors never cross the projection boundary.
`outcome=complete` if every section is collected or operations is `not_present`; otherwise it
is `partial`. Capabilities exactly mirror section collection values, except config/service/auth
remain `not_collected`.

Section shapes are also closed. Collected installation requires a non-null validated
`dotfiles.status` document; unavailable installation requires null. Unavailable provenance has
empty ownership, zero counts, `homebrew_installed=false`, `finding_count=0`, `truncated=false`,
and `findings=[]`. Unavailable/not-present operations has `truncated=false` and `records=[]`.
Collected provenance and operations require internally consistent fields described below.

## Allowlisted data sources and fields

### Build

Build collection cannot make the document partial. It uses only process-local release/runtime
metadata and always emits `collection: collected`. The build section may contain only:

- product version;
- closed version-source classification;
- Go version;
- GOOS and GOARCH;
- the closed `vcs_state` enum, without the revision.

The product version accepts exact `dev`, which maps to `version:"dev"` and
`version_source:"development"`, or exact numeric `MAJOR.MINOR.PATCH`, with each component 1-5
digits and no leading zero except the value zero, which maps to `version_source:"compiled"`.
The latter is a sanitized compiled claim, not proof of release provenance. Prerelease/build
suffixes and every other value become `version:"unknown"` and `version_source:"unknown"` in v1.
Go version accepts a 1-64 byte safe ASCII runtime token matching
`[0-9A-Za-z][0-9A-Za-z.+-]*`; otherwise it becomes `unknown`. GOOS and GOARCH accept 1-32
lowercase ASCII letters/digits/underscore only; an unexpected value becomes `unknown`.

VCS data comes only from `debug.ReadBuildInfo` settings. Exactly one `vcs.modified` equal to
`true` maps to `modified`; exactly one equal to `false` maps to `clean`; missing, malformed,
duplicate, or unreadable settings map to `unavailable`. `vcs.revision` is deliberately omitted:
v1 has no offline rule that proves a revision belongs to the claimed release, and an arbitrary
revision would add fingerprinting without sufficient support value. `vcs.time` and all other
build settings are ignored. This fallback is safe and deterministic, not a partial collection
error.

It does not contain hostname, username, working directory, executable path, environment, build
path, command line, or generated-at time.

### Installation

The installation section embeds the existing validated and redacted `dotfiles.status` v1
document produced from exactly one shared installation-health collection. It must reuse the
same field-aware redaction and public-digest behavior as `dotfiles status --json`; it must not
embed the private health snapshot or its private digest.

Implementation should split status construction from output so both commands consume the same
typed public document. Serializing a second independently maintained status shape is rejected.

### Provenance

The existing doctor collector may be reused, but the current `doctor --json` report may not be
embedded. It contains absolute paths, resolved paths, symlink targets, Homebrew prefixes, modes,
and raw probe/inspection errors.

The support-specific provenance projection may contain only:

- a fixed running-ownership enum;
- PATH match and legacy-binary counts;
- Homebrew installed boolean;
- findings reduced to bounded severity/code pairs from a fixed vocabulary.

The ownership mapping is exact: `Homebrew formula tekierz/tap/dotfiles` maps to `homebrew`, and
`running executable; ownership not yet verified` maps to `unverified`. Any other value makes
the provenance section unavailable with `provenance_invalid`.

The only accepted finding pairs are:

| Severity | Code |
|---|---|
| `warning` | `path-missing` |
| `warning` | `path-shadowing` |
| `warning` | `multiple-path-matches` |
| `warning` | `version-mismatch` |
| `info` | `build-toolchain-mismatch` |
| `info` | `running-not-on-path` |
| `warning` | `homebrew-not-running` |
| `warning` | `legacy-binary` |
| `info` | `homebrew-unavailable` |

An unknown code, wrong severity/code pairing, negative count, or inconsistent Homebrew state
maps the whole section to `provenance_invalid`; entries are never silently discarded. The
support collector accepts at most 1,024 PATH matches and 1,024 legacy-binary rows. Exceeding
either bound maps to `provenance_limit_exceeded` before projection.

Counts must equal the corresponding collected slice lengths. `homebrew_installed=true` requires
the exact formula `tekierz/tap/dotfiles`, an empty probe error, and internally consistent managed
executable evidence. `homebrew_installed=false` requires empty managed-executable evidence; a
probe error may be present privately but is never published. Any contradiction maps to
`provenance_invalid`.

`finding_count` is the exact number of validated findings before publication. Findings are
sorted by severity then code, the first 256 are published, and `truncated` is true exactly when
`finding_count > 256`. Truncation remains a truthful collected section, not a partial outcome.

Doctor paths, names derived from user paths, ownership prose, finding summaries, inspection
errors, and probe errors are omitted rather than generically redacted.

### Operations

At most the newest 20 validated journal records may be projected. Each public record has this
exact shape:

```json
{
  "ordinal": 0,
  "status": "succeeded",
  "actions": {
    "pending": 0,
    "succeeded": 2,
    "failed": 0,
    "skipped": 0
  },
  "backup_recorded": false,
  "rollback": null,
  "duration_ms": 1250
}
```

`rollback` is either null or exactly
`{"status":"succeeded","restored":0,"removed":0,"skipped":0,"warnings":0}`.
`duration_ms` is null for running records and a nonnegative integer for terminal records. The
duration is calculated with overflow-safe UTC subtraction from persisted `StartedAt` and
`FinishedAt`; an unrepresentable millisecond duration is `journal_invalid`. Action and rollback
counts are nonnegative and action counts sum exactly to the private action-result length.

The allowlisted record facts are therefore only:

- operation status;
- counts of pending, succeeded, failed, and skipped actions;
- whether a backup was recorded;
- rollback presence, status, and restored/removed/skipped/warning counts;
- terminal duration in milliseconds, derived only from persisted start/finish times.

`operations.truncated` is true exactly when more than 20 fully validated candidates exist and
the first 20 were selected; otherwise it is false.

The projection omits operation ID, plan hash, absolute start/finish times, backup path, action
IDs, action summaries, rollback summary, warnings, raw JSON, and filenames. Running records do
not calculate duration against the current clock.

## Redaction and omission boundary

The public projection uses an allowlist first and leaf-value redaction only as a final boundary
for fields already approved for publication. It must not traverse arbitrary maps or structs.
Typed private fields cannot be made safe merely by changing suspicious strings to
`[redacted]`.

The document must never expose:

- HOME, XDG roots, usernames, hostname, PATH entries, executable paths, symlink targets,
  Homebrew prefix, backup paths, config paths, working directory, or environment values;
- config contents, credentials, tokens, authorization headers, authentication state, service
  state, command output, logs, or raw errors;
- operation IDs, plan hashes, private snapshot/state digests, exact activity timestamps, or
  journal filenames;
- journal summaries, warnings, action IDs, or raw journal records;
- archive metadata, because v1 creates no archive.

The existing status redactor is reusable for approved installation leaf fields. The stricter
plan-public validator must remain reject-on-unsafe and must not be replaced by a general support
redactor. If code reuse requires extraction, use a small `internal/publicsafe` leaf-redaction
package rather than broad recursive sanitization.

## Determinism, digest, and limits

- Identical collected public inputs produce byte-identical output.
- There is no generated-at timestamp, random identifier, current-time duration, locale-dependent
  prose, nondeterministic map order, or filesystem enumeration order in the document.
- Findings are stable-sorted by severity then code using ascending bytewise comparison. Equal
  pairs preserve the private collector order; no private tie-break value is published.
- Journal candidates are decoded before selection and sorted by `StartedAt` descending, then
  `OperationID` descending, then exact filename descending, all by bytewise comparison after
  UTC timestamp comparison. Because candidate validation requires
  `filename == OperationID + ".json"`, the filename tie-break is normally equal but remains an
  explicit total-order rule. The first 20 become public ordinals 0-19.
- At most 20 operation summaries are published.
- At most 256 provenance findings are published.
- Journal enumeration consumes at most 1,025 entry names: 1,024 allowed names plus one sentinel.
  If the sentinel exists, collection returns `journal_limit_exceeded` immediately, without
  stat, read, or decode of any candidate. At most 1,024 accepted candidates are subsequently
  examined.
- Each journal record is at most 1 MiB and aggregate journal record bytes read across initial
  decoding and final revision revalidation are at most 32 MiB. A bounded anchored reader must
  stop at limit+1; checking size metadata before an unbounded read is insufficient.
- The complete encoded document, including its trailing newline, must not exceed 1 MiB.
- `authority.public_digest` is lowercase SHA-256 over the canonical public document with only
  that digest value omitted. It is an integrity/addressability value and grants no execution
  authority.
- The output has exactly one trailing newline and is prepared before one writer call. A short
  or failed write is fatal and is never reported as success.

## Read-only journal requirements

`operation.DefaultJournal()` is prohibited for support collection because it calls state
bootstrap and can create missing directories. The operation package needs a new read-only
open/list boundary that:

1. Captures the existing operation-state plan without creation.
2. Returns `not_present` when the state or operations directory is missing.
3. Binds the existing operations directory and parents through anchored authority.
4. Accepts as a candidate only a real regular file whose basename exactly matches
   `YYYYMMDDTHHMMSS.NNNNNNNNNZ-<16 lowercase hex>.json`. Directories, symlinks, special files,
   hidden/temp files, wrong extensions, and every other entry make the whole section
   `journal_invalid`; they are not ignored.
   The state namespace and operations directory must both be real directories with exact mode
   `0700`; every candidate must be a real regular file with exact mode `0600`. Any file-type or
   mode mismatch is `journal_invalid`.
5. Performs a names-only bounded directory pass that consumes at most limit+1 (1,025) names.
   The 1,025th name is a sentinel: its presence returns `journal_limit_exceeded` before any
   candidate stat/read/decode. If at most 1,024 names exist, validates every name/type/mode and
   then reads every accepted candidate needed for exact StartedAt ordering through a new bounded
   anchored reader: 1 MiB per record and 32 MiB aggregate. An API that materializes the entire
   directory before applying this limit is prohibited.
6. Rejects duplicate filenames and requires each decoded `OperationID` to equal the filename
   stem exactly. The raw `StartedAt` string, and `FinishedAt` when present, must be canonical
   `time.RFC3339Nano` ending in `Z`; parse-and-remarshal must reproduce the exact bytes.
   `StartedAt` must be nonzero and its UTC
   `20060102T150405.000000000Z` rendering must equal the timestamp prefix encoded in the
   operation ID; all existing `validateRecord` invariants still apply.
7. Decodes with `json.Decoder.DisallowUnknownFields`, rejects duplicate object keys at every
   nesting level, requires exactly one top-level object, and requires a second decode to return
   `io.EOF`. Trailing whitespace is allowed; trailing values/garbage are not. Unsupported
   schemas and malformed timestamps are `journal_invalid`.
8. Revalidates the exact sorted name/type set and every decoded candidate revision after
   collection so an unlocked concurrent replacement cannot produce a mixed accepted snapshot.
9. Returns typed internal errors that the public command maps to the exhaustive reason codes
   above without publishing paths or error text.

The support command must not acquire the normal journal write lock: acquiring or bootstrapping
that lock would violate the read-only command contract.

## Exit and output contract

| Exit | Output contract |
|---|---|
| `0` | One newline-terminated `complete` JSON document on stdout; stderr empty. `operations.collection=not_present` is still complete. |
| `2` | One newline-terminated `partial` JSON document on stdout when one or more sections are unavailable; stderr empty. |
| `1` | Projection, marshal, size-limit, or write failure; no valid success document and only bounded generic `support collection failed` stderr. |
| `2` | Invalid Cobra syntax or missing required `--json`; bounded syntax stderr and empty stdout. |

Collection order is fixed: build, installation, provenance, operations. Build is unconditional
and process-local: it is projected before any context-cancellation check and is always accepted.
The first context check is immediately before installation. Installation, provenance, and
operations collectors are each called at most once, with another check immediately after the
active collector and before the next one. Cancellation already present at command entry
therefore preserves build, does not invoke any of the three collectors, and marks installation,
provenance, and operations `unavailable`/`collection_cancelled`. Cancellation observed during a
collector marks that active section and every later section cancelled, skips later calls,
preserves earlier accepted sections, and writes one valid `partial` document with exit 2.
Cancellation after operations is accepted does not rewrite any section and does not turn a
complete document partial. Parent-context cancellation, `os.Interrupt`, and `SIGTERM` follow
this same mapping. No cancellation or collector error text is published.

No userspace API can retract bytes after an operating-system partial write. The contract
therefore promises atomic preparation and correct failure reporting, not impossible rollback of
bytes already accepted by the writer.

## Tests-first acceptance matrix

### Projection and canonical output

- Exact schema, key order, field vocabulary, non-null arrays, and one trailing newline.
- Independently recomputed public digest matches.
- Identical public inputs are byte-identical across clock/random/locale changes.
- A legitimate public change alters the public digest.
- Sensitive-only doctor, status, or journal changes produce identical public bytes/digest.
- Mutation of source structs or returned projections cannot mutate an accepted document.
- Every out-of-vocabulary outcome, collection state, reason, ownership, VCS state, finding
  severity/code pair, operation status, action status, or rollback status is rejected.
- Malformed release/VCS settings exercise the exact deterministic build fallbacks and never
  leak arbitrary build-setting values.

### Redaction and omission

- Canary matrix covers Unix/Windows/UNC/HOME/XDG paths, usernames, hostnames, PATH, backup
  paths, tokens, API keys, authorization headers, environment assignments, raw dial errors,
  controls, bidi text, and oversized strings.
- No operation ID, plan hash, absolute timestamp, private digest, journal filename, action ID,
  summary, warning, raw doctor error, or forbidden key crosses the boundary.
- Current raw doctor and journal structs cannot be passed directly to the public marshaler.

### Read-only collection

- Missing state and missing operations directory return `not_present` and create nothing.
- Filesystem inventories, modes, and contents before/after collection are identical.
- Symlink, mode, traversal, malformed record, unexpected entry, concurrent replacement, and
  directory-graft fixtures fail closed without reading outside the trusted root.
- Status, doctor, and journal collection are each attempted at most once.
- The fixed build -> installation -> provenance -> operations order is asserted. Cancellation
  already present at entry still emits the unconditional build, calls no collector, and marks
  all three data sections `collection_cancelled`. Cancellation before, during, and after each
  collector preserves earlier sections, marks the active/later sections cancelled, skips later
  calls, and emits partial JSON.
- The injected signal-context seam proves construction occurs only after valid syntax and that
  its stop function is deferred and invoked exactly once on complete, partial, and fatal paths.
- The only subprocess permitted is the existing fixed `brew --prefix tekierz/tap/dotfiles`
  ownership probe with its existing timeout and no auto-update behavior.

### Bounds and ordering

- More than 20 valid records publishes exactly the newest 20 in deterministic order and sets
  `truncated=true`.
- A directory with exactly 1,024 names proceeds to candidate validation. A 1,025th name returns
  `journal_limit_exceeded` after exactly 1,025 name reads and before every stat, record read, or
  JSON decode; no additional names are consumed.
- More than 256 findings publishes exactly 256, preserves the exact pre-truncation
  `finding_count`, and sets provenance `truncated=true` without making the document partial.
- Journal ordering is independently checked as StartedAt descending, OperationID descending,
  filename descending before the newest 20 are selected.
- Per-record and aggregate byte limits are enforced by the anchored reader before unbounded
  allocation; strict decoding rejects duplicate/unknown keys, multiple values, and trailing
  garbage while accepting trailing whitespace followed by EOF.
- Encoded output over 1 MiB fails before the writer is called.
- Running operations contain no duration derived from the current clock.

### Command and exits

- Command registration, required local `--json`, no positional arguments, no prompts, and no
  file/output/upload flags.
- Complete, partial, not-present, fatal, syntax, cancellation, short-writer, and failed-writer
  exit/stdout/stderr matrix.
- Marshal and size validation complete before the single writer call.
- Generic errors contain no raw collector, filesystem, command, or writer detail.

## Dependency and commit order

Implementation must remain linear at the security boundaries, with review after each logical
commit. No implementation step may begin while this contract remains NO-GO:

1. **Architecture - extract shared public status construction**
   - Separate validated status-document construction from marshaling/writing without changing
     `status --json` bytes or exits.
2. **Safety - add read-only operation journal summaries**
   - Implement non-bootstrapping anchored enumeration, revision revalidation, limits, and
     private typed summaries. No public command yet.
3. **API - add validated support projection**
   - Add the allowlisted envelope, enums, cloning, canonical digest, redaction/omission tests,
     and size enforcement as a pure boundary.
4. **Feature - register deterministic support JSON**
   - Compose status, provenance, and operation summaries; implement complete/partial/fatal exit
     mapping and atomic output.
5. **Docs - publish support sharing guidance**
   - Document that support JSON is designed for review/sharing, while raw `doctor --json` and
     private journal files are not share-safe artifacts.
6. **Test - complete adversarial support verification**
   - Run the full test/race/static/security checklist and owner-hardware read-only verification
     before marking the todo support-bundle item complete.

Archive creation, `--output`, automatic attachment, upload, config collection, service/auth
collection, and full logs are separate future designs and must not be added implicitly to v1.
