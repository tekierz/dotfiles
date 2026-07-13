# Owner-hardware install/apply verification

Status: required release gate. Run only in a disposable macOS VM or on a spare Mac
whose disk can be restored from a known snapshot. A second user on the owner's primary
Mac is **not** sufficient isolation. Do not test Manage Save, standalone Save, installer
theme application, or `theme set` on a primary account.

## 1. Preconditions and evidence

- Record macOS version, architecture, Homebrew version/prefix, Node/npm versions, free
  space, and VM/spare-Mac identity. Confirm a tested VM/APFS/full-disk restore point.
- Use a non-admin test account where practical. Record every pre-existing package, cask,
  binary, app bundle, and npm global package involved below. Do not uninstall a user's
  pre-existing software merely to manufacture a Missing state.
- Create an evidence directory on a host-shared or external volume that is outside both the
  repository and the guest snapshot being restored. Verify a test file survives one restore
  before beginning. Save `brew list --formula`,
  `brew list --cask`, `npm list -g --depth=0`, `dotfiles status --json`, and hashes/modes
  of pre-existing files below `$HOME`, `XDG_CONFIG_HOME`, and `XDG_STATE_HOME` that the
  test may touch. Redact evidence before sharing it.
- Set a dedicated absolute state root, create only its parent, and make it private:

  ```bash
  export OWNER_EVIDENCE="/Volumes/OwnerEvidence/dotfiles-owner-evidence"
  export XDG_STATE_HOME="$HOME/dotfiles-owner-state"
  mkdir -p "$OWNER_EVIDENCE" "$XDG_STATE_HOME"
  chmod 700 "$OWNER_EVIDENCE" "$XDG_STATE_HOME"
  ```

## 2. Prove the tested binary

Build from the exact reviewed workspace and invoke the absolute build output throughout;
do not use an unqualified `dotfiles` from `PATH`.

```bash
make build
export DOTFILES_BIN="$PWD/bin/dotfiles"
test -x "$DOTFILES_BIN"
shasum -a 256 "$DOTFILES_BIN" | tee "$OWNER_EVIDENCE/binary.sha256"
"$DOTFILES_BIN" version | tee "$OWNER_EVIDENCE/version.txt"
"$DOTFILES_BIN" doctor --json >"$OWNER_EVIDENCE/doctor.json"
type -a dotfiles | tee "$OWNER_EVIDENCE/path.txt"
```

STOP if the workspace, branch/commit, binary hash, executable reported by Doctor, or
Homebrew/PATH ownership is ambiguous. Record `git status --short` and `git rev-parse HEAD`
without adding the evidence directory to the repository.

## 3. Install matrix

Run each row from a fresh VM snapshot or restore the snapshot between rows. Choose the
ordinary formula only after confirming it is absent; `glow` is the default candidate.
For every row, save stdout and stderr separately, verify the plan recipe exactly, extract
only `authority.plan_hash`, then repeat the identical tool set at apply time.

```bash
TOOL=glow
"$DOTFILES_BIN" plan --json --tool "$TOOL" \
  >"$OWNER_EVIDENCE/$TOOL.plan.json" 2>"$OWNER_EVIDENCE/$TOOL.plan.stderr"
PLAN_HASH="$(jq -er '.authority.plan_hash' "$OWNER_EVIDENCE/$TOOL.plan.json")"
"$DOTFILES_BIN" apply --yes --plan-hash "$PLAN_HASH" --tool "$TOOL" \
  >"$OWNER_EVIDENCE/$TOOL.apply.stdout" 2>"$OWNER_EVIDENCE/$TOOL.apply.stderr"
```

| Case | Tool ID | Reviewed route to verify before apply | Required postcondition |
|---|---|---|---|
| Ordinary Homebrew formula | `glow` (or another recorded absent formula) | one `package_manager` step, provider `brew`, expected formula only | receipt present and detector succeeds |
| OpenAI Codex CLI | `codex` | Homebrew `node`, then npm global `@openai/codex@0.144.1` | `codex` resolves; do not authenticate |
| Pi coding agent | `pi` | Homebrew `node`, then npm global `--ignore-scripts @earendil-works/pi-coding-agent@0.80.3` | `pi` resolves; do not authenticate |
| OpenCode | `opencode` | Homebrew formula `anomalyco/tap/opencode` | `opencode` resolves; do not authenticate |
| T3 Code app | `t3-code` | one `homebrew_cask` step for cask `t3-code` | real `T3 Code.app` detector succeeds; do not sign in |

Each initial plan must be `ready`, exit 0, produce exactly one newline-terminated JSON
object on stdout, and leave stderr empty. Apply must exit 0, leave stderr empty, and emit
exactly one line of the form:

```text
installation applied: operation=<validated-id> plan_hash=<full-hash> succeeded=<n> failed=0
```

Verify the full echoed hash equals the reviewed hash. Do not accept a partial install, a
different provider/package/cask/npm argument, a prompt, a request for JSON/stdin authority,
or authentication as part of this gate.

## 4. State and failure matrix

| Scenario | Procedure | Expected result |
|---|---|---|
| `no_changes` | Re-run `plan --json` for a successfully installed row. | Exit 0; one `no_changes` JSON object; no `plan_hash`; apply capability `not_available`; no mutation. |
| Partial repair | On a fresh snapshot, install only a strict subset of the `zsh` Homebrew receipts, confirm `status --json` reports `zsh` as `partial`, then plan/apply `--tool zsh`. | Plan is `ready`; apply installs the reviewed missing coverage; final status is `present`. If the collector does not report `partial`, STOP rather than forcing repair. |
| Stale hash, still ready | Plan one Missing tool. Before apply, create the previously missing private state namespace below the dedicated `XDG_STATE_HOME` with mode 0700 (`mkdir -p "$XDG_STATE_HOME/dotfiles"/{operations,locks,backups,staging}; chmod 700 "$XDG_STATE_HOME/dotfiles" "$XDG_STATE_HOME/dotfiles"/*`), without installing the tool. Apply the old hash. | Exit 2; stdout empty; stderr exactly `installation plan changed; run dotfiles plan --json again`; package/npm/cask remains untouched. |
| Became no-change | Plan one Missing tool, install it independently through the exact reviewed provider, then apply the old hash. | Exit 2; stdout empty; stderr exactly `installation plan is not ready; run dotfiles plan --json again`; no second mutation. |
| Wrong/syntactically invalid hash | Change one hex digit, then separately try uppercase/short input. | Well-formed wrong hash: exit 2 with the changed-plan line and no mutation. Malformed hash: exit 2 with `invalid apply request`; no planning or mutation. |
| Cancellation | For an otherwise isolated Missing row, start apply and send SIGINT only after its running journal record is visible. Use a bounded polling harness; if installation finishes first, restore the snapshot and mark the cancellation case untested. | Exit 130; stdout empty; stderr exactly `installation apply cancelled`; terminal journal status `cancelled`; incomplete actions skipped/failed truthfully; lock is released. |
| Fatal operation | Exercise only through a reversible VM fault such as temporarily making the isolated state root unavailable before a fresh run. | Exit 1; stdout empty; stderr exactly `installation apply failed`; no raw path/error/credential text and no unjournaled package mutation. |

Also verify omitted `--yes`, omitted tools, positional arguments, `--json`, and
`--yes=false` all exit 2 with empty stdout and exactly `invalid apply request` on stderr.
Blocked plan outcomes exit 2 with one public JSON object on stdout and empty stderr; fatal
planning exits 1 with empty stdout and `plan collection failed` on stderr.

## 5. Journal and side-effect checks

For every apply that crossed the product/package/npm/cask mutation boundary, inspect only the
dedicated state root. The reviewed operation may bootstrap its private state namespace before
the running journal is written; no product/package/npm/cask mutation may occur until that
running record is durable:

- one operation record exists under `dotfiles/operations`; it moved from `running` to one
  terminal status and binds the full reviewed plan hash;
- action count/order and succeeded/failed/skipped results match the plan and CLI counts;
- package-only operations record the package-only/no-filesystem-rollback warning, with no
  fabricated backup or rollback point;
- state directories are mode 0700 and records are private regular files; there are no
  symlink traversals, abandoned operation locks, raw command output, paths, credentials,
  tokens, or authentication material in public streams or the journal;
- config/theme files and dashboard preferences are byte-for-byte unchanged. This gate is
  install-only and grants no authority to test Save or theme application.

Capture the final `status --json`, package/cask/npm inventories, journal JSON, exit codes,
separate streams, relevant modes, and binary hash for each row. Record failures verbatim in
private evidence, then redact paths and identifiers in any shared report.

## 6. Cleanup and verdict

Restore the VM/spare-Mac snapshot after every destructive row and again at the end. Snapshot
restore is the authoritative cleanup; ad-hoc uninstall is not evidence of rollback. If a
snapshot cannot be restored and verified against the baseline inventories/hashes, the gate
is incomplete and the machine must not be reused for another row or deployment tier.

Immediate **STOP / No-Go** conditions:

- any test is attempted on the owner's primary account or without a verified restore point;
- binary/path provenance is ambiguous, or a different build runs than the one hashed;
- stale, wrong, blocked, no-change, cancelled, or fatal input performs product mutation;
- provider, package, cask, npm version/arguments, detector, or tool set differs from the
  reviewed plan, or detector drift does not block;
- apply performs product/package/npm/cask mutation before a durable running journal record,
  succeeds without its postcondition, loses/releases the lock incorrectly, or reports false
  backup/rollback coverage; private state bootstrap alone is not product mutation;
- an exit code/stream differs from this matrix, output is partial/multiple, or private/raw
  errors, paths, credentials, environment values, or tokens escape;
- config/theme/dashboard files change, authentication is attempted, an unexpected privilege
  escalation occurs, or snapshot cleanup cannot return the host to the recorded baseline.

Passing this matrix closes only the install/apply owner-hardware subgate and permits continued
local-alpha remediation. It does not advance the product to any deployment tier or authorize
friends/family deployment, mock-enterprise testing, primary-account Save/Theme tests, or
installation-profile implementation by itself.
