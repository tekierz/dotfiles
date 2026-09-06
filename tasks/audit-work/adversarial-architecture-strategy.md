# Adversarial Architecture, Deployment, and Strategy Audit

Date: 2026-07-09
Repository state: `release-remediation` at `7ddcb99`, dirty only from audit/task artifacts at inspection time
Purpose: independent cross-cutting review of the primary audit reports against current code, documentation, worktree, and local installation evidence. This report deliberately prioritizes systemic release risk over finding count.

## Decision in one page

**Keep this repository and refactor it in place. Do not start a clean-room replacement. Do not add the planned AI tools yet. Retire the legacy Bash product from active distribution. If a real multi-host enterprise control plane is later desired, create that as a separate product that consumes this repository's local agent/library.**

Current deployment disposition:

| Target | Disposition now | Why |
|---|---|---|
| Read-only exploration (`help`, `version`, source audit) | Acceptable | No configuration mutation, although `status` is misleading/slow. |
| Disposable macOS user/VM | Limited engineering test only | Useful for reproducing installer/plan defects; assume the environment will be discarded. |
| Owner's existing macOS account | **No-go for Apply/Save/install** | Existing app and Git settings can be replaced without truthful import, complete backup, or transactional rollback. |
| Friends/family | **No-go** | No safe upgrade/provenance path, incomplete rollback, failing macOS suite, weak support diagnostics. |
| Mock enterprise | **No-go and wrong product framing today** | Local profiles are not policy/RBAC/tenancy; there is no noninteractive plan/apply contract, audit trail, offline/proxy policy, MDM conflict model, or supply-chain release. |

The release-critical problem is not “missing checkboxes.” It is that observation, desired state, execution, ownership, backup, and UI preview are not one coherent transaction. The dashboard constructs preferences, execution reconstructs a different plan, writers often replace whole native config files, backup covers only a subset, and package/service mutations are not rollbackable at all.

## Corrections to the existing audit emphasis

The primary reports contain strong evidence, but a release plan should incorporate these adversarial corrections:

1. **The current dashboard does not call manager-wide `UpdateAllStreaming`.** The apt output deadlock is real but latent; the current Updates screen passes displayed package names through the selected-package path. It is not one of the immediate top five release blockers. Arch's selected update still invokes `-Syu`, but the screen discloses that full-system behavior. Evidence: `internal/ui/screen_update.go:163-202`, `internal/ui/installation.go:813-867`, `internal/pkg/apt.go:316-327`.
2. **The dynamic Emacs Yazi hidden-file key is consistent with the generated config (`.`).** The stale `Ctrl-h` claim belongs to static help/docs drift, not a separate production binding bug. Evidence: `internal/hotkeys/hotkeys.go:86-96,150-156`, `internal/tools/yazi.go:158-205`, `internal/scripts/scripts.go:47-50`.
3. **Paru uninstall is a real but currently unused API bug.** It should not outrank active whole-file config loss or fail-open backup behavior.
4. **Missing Codex/Cursor Agent/OpenCode/Pi/T3/Hermes integrations are not release blockers for the existing product.** Implementing the current “ready-to-implement” spec literally would add default-on mutable remote code and more bespoke screens to an unsafe substrate. The spec must be changed from ready to blocked-on-platform architecture.
5. **Rename work is not the first implementation milestone.** Decide the brand now, introduce identity/storage indirection during the safety refactor, then execute the compatibility migration before public beta. A direct rename today would multiply an already broken ownership/migration surface.
6. **The immediate release blockers are systemic:** clean installs omit core packages; existing config is not imported and is replaced; Git is especially destructive; backup fails open and misses many writes; config writes are non-atomic/symlink-following; global config lacks migrations; binary provenance is split across Homebrew and self-copied installs.

## New or under-prioritized cross-cutting findings

### ARCH-C01 — There is no single immutable plan connecting preview, execution, backup, and rollback

- Severity: Critical
- Evidence:
  - Package collection omits core terminal tools while later phases configure them (`internal/ui/installation.go:53-65,234-318,340-433`).
  - The File Tree independently reconstructs an inaccurate preview (`internal/ui/screen_filetree.go:87-211`).
  - Backup is a fixed six-file list rather than derived from execution (`internal/ui/app.go:735-745`).
  - Manage and standalone config call writers without the install backup path (`internal/ui/manage_dualpane.go:95-146`, `internal/ui/config_apply.go:287-350`).
- Impact: the user approves one operation, the executor performs another, and rollback covers a third. No amount of extra confirmation copy can make this safe.
- Architectural fix: a pure planner must emit the one action graph consumed by preview, backup, executor, summary, logs, tests, and rollback. Every action needs target, observed hash/state, owner, risk, reversibility class, required privilege, and validation/health checks.

### ARCH-C02 — The product lacks a configuration ownership and reconciliation model

- Severity: Critical
- Evidence:
  - Current Manage/standalone state loads product JSON/defaults, not native application config (`internal/ui/app.go:422-463,1020-1030`).
  - Whole-file writers replace Ghostty, tmux, Git, all three Yazi files, fzf, LazyGit, btop, and Glow (`internal/tools/tool.go:168-183` and the writer call map in `internal/ui/config_apply.go:287-318`).
  - Git's generator cannot preserve identity, LFS filters, includes, arbitrary aliases, or enterprise credential/signing policy (`internal/tools/git.go:222-251`).
  - Every struct in `internal/config/tool.go:3-89` is declaration-only; live state is duplicated in UI/tools models. There is no shared versioned domain schema.
- Impact: “save one setting” means “replace the file with the subset this build knows,” and the UI cannot distinguish current detected value, product default, desired value, or unsupported/unmanaged value.
- Architectural fix: per-integration adapters must discover precedence, parse observed state, preserve unknown/unowned state, reconcile validated desired state, preview a semantic/raw diff, and prefer native includes/managed blocks. Whole-file ownership is allowed only for newly created product-owned files or explicit adoption with a complete preview.

### ARCH-C03 — “Fully reversible” is not a technically achievable current contract

- Severity: Critical
- Evidence:
  - README promises all existing configs are backed up and installation is fully reversible (`README.md:197-207`).
  - Backup covers only six home-relative files and installation proceeds after failure (`internal/ui/app.go:735-745`, `internal/ui/installation.go:201-224`).
  - Many written files are absent from backup, including Yazi keymap/theme, FZF, LazyGit, btop, Glow, Claude MCP state, and broader Neovim state.
  - Backup/restore does not model package installs/upgrades, global npm state, services, authentication/pairing, firewall changes, cloned repositories, or package-manager side effects.
  - Multi-file writes have no transaction; directory restore deletes the live destination before copying (`internal/backup/backup.go:289-301`).
- Impact: even a perfect file restore cannot return a machine to its prior system/package/service state. The promise misstates the product's threat and rollback model.
- Architectural fix: immediately change the promise to “file changes are backed up where listed; package/system changes are disclosed and may require manual rollback.” The planner must label every action `reversible`, `best-effort inverse`, or `non-reversible`, journal exact before/after state, and refuse destructive file actions without a durable verified backup.

### ARCH-H01 — The Go product self-replicates into a second installation channel

- Severity: High
- Evidence:
  - Homebrew is documented as the recommended owner (`README.md:7-15`).
  - During normal installation, `installUtilities` resolves the running executable and copies it to `~/.local/bin/dotfiles` unconditionally (`internal/ui/installation.go:495-531`).
  - `make install` instead places the Go binary and legacy Bash executable in `/usr/local/bin` (`Makefile:41-45`).
  - The legacy script contains yet another embedded management CLI and old version identities (`bin/dotfiles-setup:2831-2846`).
  - Current local evidence: shell resolution finds `/opt/homebrew/bin/dotfiles` and `~/.local/bin/dotfiles`; they report `2.0.1` and `2.0.0-dev`, while the checkout build reports `7ddcb99`.
- Impact: PATH order determines the product and feature set; Homebrew upgrade does not update the self-copied binary; support reports cannot identify provenance from the command name; uninstall/rename becomes unsafe. This is a direct architectural cause of the user's “old build/missing features” experience.
- Fix: package managers own the main binary. Split utility-script installation from self-installation; never copy a Homebrew-managed executable. Add `doctor` provenance/PATH collision detection and an explicit, separately named user-prefix install command only if needed. Migrate/remove old copies only through an ownership manifest or user-confirmed path-specific action.

### ARCH-H02 — The 4,346-line Bash installer remains a second supported product

- Severity: High
- Evidence: it has independent version/config/theme/platform/backup/restore/install logic (`bin/dotfiles-setup:1-31`), embeds a second management CLI at lines 2831+, remains installed by `make install` (`Makefile:41-45`), and already emits obsolete Yazi schema. README still mixes its tools/features with Go behavior (`README.md:48-108`).
- Impact: every schema/security/platform fix needs duplicate implementation and QA. Current docs and local binaries cannot answer which product a tester ran.
- Fix: immediately remove it from default/recommended installation and `make install`. Preserve the last script in a tagged EOL release or `legacy/README` migration reference, not as a live updater. Do not move it into a new active repo—that would institutionalize dual support.

### ARCH-H03 — There is no versioned state/upgrade/downgrade protocol

- Severity: High
- Evidence:
  - Global/tool/profile schemas have no `schema_version`; only hotkey IDs have a one-off migration (`internal/config/config.go`, `internal/config/user.go`, `internal/config/hotkeys.go`).
  - Older partial global JSON silently disables backup defaults because it unmarshals into zero values (`internal/config/config.go:63-71,163-183`).
  - Current tags stop at `v2.0.2`; Makefile comments require a nonexistent local `v2.1.2` tag (`Makefile:8-11`); checkout builds report a hash.
  - `tasks/todo.md:20-27` asserts green/state-of-main evidence that the current macOS suite disproves.
- Impact: upgrades cannot deterministically migrate state, downgrade cannot know compatibility, and task prose is mistaken for release evidence.
- Fix: version every persisted root, keep ordered idempotent migrations with N-1/N-2 fixtures, record writer version and ownership revision, refuse unsupported downgrade with export instructions, and test Homebrew upgrade from every supported prior release/config layout.

### ARCH-H04 — Operations have ephemeral UI state, not an auditable lifecycle

- Severity: High for mock-enterprise; Medium for local alpha
- Evidence: install logs are kept only in `App.installOutput`, capped at 20 lines (`internal/ui/app.go:253,427`, `internal/ui/screen_progress.go:18,130-197`). Several async results are screen-local and can be dropped after navigation (`internal/ui/screen_manager.go:77-102` and backup/update/user handlers). No tracked production code implements `doctor`, an operation journal, request IDs, support bundle, or `XDG_STATE_HOME`; `dotfiles doctor` exists only as an unchecked archived plan item (`docs/archive/beta.plan.md:124`).
- Impact: support cannot reconstruct what changed, long operations can complete without visible ownership, retries are not safely correlated, and enterprise-like change evidence is impossible.
- Fix: central operation coordinator with durable operation IDs, plan hash, state transitions, timing, before/after hashes, backup link, warnings, and terminal result. Store non-secret records under `XDG_STATE_HOME/<brand>/operations`; expose `doctor --json` and a redacted support bundle.

### ARCH-H05 — Product scope and capability semantics are incoherent

- Severity: High strategy risk
- Evidence: the same dashboard groups full config writers, install-only CLI packages, GUI app detectors, local preference profiles, VPN/streaming packages, MCP configuration, and planned autonomous agents under generic tool/install/config language. `HasConfig` means only that a path exists for some entries; “installed” can mean every bundled dependency receipt; Tailscale/Sunshine/Moonlight stop before service/auth readiness; GUI apps are detection/install only.
- Impact: users interpret presence as integration and “installed” as usable. Feature count grows while trust, ownership, and readiness meanings erode.
- Fix: explicitly scope v3 as a **single-host terminal/workstation environment reconciler**. Every tool declares capability badges: `Detect`, `Install`, `Adopt`, `Configure`, `Theme`, `Health`, `Auth`, `Service`, with risk class and support level. Do not market install-only entries as integrations.

### ARCH-H06 — The planned AI-agent spec is unsafe and structurally obsolete

- Severity: High if implemented; no current runtime severity
- Evidence: `tasks/new-tools-spec.md:1-27` calls the work ready, recommends five tools enabled by default, and proposes multiple `curl | bash/sh` or global npm routes. The checklist adds one bespoke file per tool and hard-coded UI changes (`tasks/new-tools-spec.md:445-457`).
- Impact: mutable remote code execution, unexpected auth/data egress, package/binary identity collisions, and more hard-wired UI drift. Friends/family may install powerful agents they did not request; enterprise simulation cannot approve provenance.
- Fix: mark the spec superseded/blocked. All AI agents default off. Model provider/version/digest, executable identity, permissions, auth presence (never secret values), data egress, sandbox, update channel, proxy/offline policy, and health through the capability manifest before implementing any one agent.

### ARCH-H07 — CLI and TUI are presentation layers without a shared automation contract

- Severity: High for mock-enterprise; Medium for local use
- Evidence: domain helpers print and call `os.Exit`; commands lack Cobra argument validators and return zero on several failures (`cmd/dotfiles/main.go:64-169,320-340,521-539`). Installer/config/update orchestration lives inside the 23k-line UI package rather than reusable application services. There is no JSON plan/apply output, dry-run artifact, stable exit-code contract, or noninteractive policy.
- Impact: automation cannot reliably distinguish no-op, partial, conflict, validation error, privilege required, or completed-with-nonreversible-changes. The TUI and CLI will continue to reconstruct behavior separately.
- Fix: extract use-case services returning typed results/errors; both Cobra and Bubble Tea call them. Required contracts: `plan --json`, `apply --plan <id> --non-interactive`, `status/doctor --json`, deterministic exit codes, no TUI when stdin/stdout is non-TTY, and explicit consent for privilege/network/nonreversible actions.

### ARCH-H08 — The release pipeline is not a deployable supply chain

- Severity: High
- Evidence: `make release` invokes snapshot GoReleaser but no `.goreleaser.yml` or release workflow is tracked (`Makefile:88-91`; tracked workflows are CI and Claude only). Tests/race run on Ubuntu; macOS only builds and runs help (`.github/workflows/ci.yml:99-137`). Current full `go test ./...` fails `TestGlowConfigPathUsesUserConfigDir` on the target Mac. Homebrew-tap and sibling compatibility could not be verified in this workspace.
- Impact: no canonical artifact, checksum/SBOM/signature/provenance, formula update proof, upgrade test, or target-platform quality gate.
- Fix: signed/tagged release workflow with Linux/macOS amd64+arm64 artifacts, checksums, SBOM and provenance; macOS full tests; clean Homebrew install/upgrade/uninstall test from 2.0.1; formula repository verification as a blocking cross-repo gate.

## Threat model and rollback model

The current app is a local user process but crosses into higher-risk domains through sudo, package managers, shell startup files, global npm, app configs, and eventually services/auth. The threat model should cover mistakes and competing managers as seriously as hostile input.

| Threat/failure | Assets at risk | Current mitigation | Material gap |
|---|---|---|---|
| Benign edit of an adopted config | Git identity/LFS/includes, Ghostty keys/security, tmux/Yazi/FZF behavior | Install-time subset backup | No import/ownership; Manage save has no backup; whole-file replacement. |
| Power loss/disk full/process kill | All generated configs | Atomic writer only in config JSON; some binary installs | Tool writer truncates in place; multi-file operations not transactional; backup can fail open. |
| Symlink/externally managed path | Files outside intended target; MDM-managed configs | Restore has strong no-follow/parent guards | Common tool writer follows final symlink and has no external-management policy. |
| Concurrent CLI/TUI/process | Desired state and native config | In-process race tests; atomic rename for some JSON | No inter-process lock/revision; save completions can reorder; last writer wins. |
| Upstream/supply-chain drift | Executables and data accessible to agents | HTTPS/package managers; govulncheck CI | Legacy mutable scripts, unpinned `npx -y`, proposed curl-to-shell, floating action majors, no release provenance. |
| Restricted/offline/proxied host | Availability and recoverability | Some streaming cancellation | Assumes network, package repos, interactive/cached sudo; no offline manifest, proxy contract, timeout policy, or diagnostics. |
| MDM/policy ownership | Enterprise config and compliance | None | No policy-vs-user precedence, ownership detection, conflict/fail-closed behavior, or audit evidence. |
| Support incident | User trust and recovery time | TUI's last 20 log lines | No doctor, durable journal, provenance, active-config precedence view, or redacted bundle. |

Rollback must be explicit per action class:

| Action class | Required contract |
|---|---|
| Product-owned file create/update | Exact before hash/content/mode, staged validation, atomic replace, exact inverse. |
| Adopted native config | Parser/import, unknown preservation, semantic diff, immediate backup, conflict hash, managed include/block preferred. |
| Directory/preset clone | Stage completely, validate, rename/swap, retain previous path until health succeeds. |
| Package install | Record provider/version and whether this operation introduced it; optional best-effort uninstall only when ownership/dependency safety is proven. Never call it fully transactional. |
| Package update | Non-reversible by default; disclose exact packages and manager, never bundle into “config rollback.” |
| Service/auth/firewall/pairing | Provider-specific inverse or explicitly non-reversible/manual steps; no secret values in journal. |
| Binary/config-name migration | Dual-read/one-write migration with sentinel/version, rollback copy, PATH/provenance checks, and old-binary compatibility window. |

## Recommended target architecture

Refactor around a small safety kernel rather than rewriting screens first:

```text
Tool/Integration Catalog
  metadata, platforms, providers, capabilities, risk, config sources
                 |
Observation ---- Desired State / Policy
  provenance      schema version + migrations + validation
       \             /
        Reconciliation Planner
        immutable action graph + diff + reversibility
                 |
     Ownership / Lock / Backup / Journal
                 |
              Executor
 file | package | service adapters; stage/validate/commit/health/rollback
                 |
   CLI JSON/human adapters     TUI renderer
```

Recommended internal boundaries:

- `catalog`: declarative integration manifests and capability/risk metadata.
- `observe`: platform facts, package/app/binary/config probes with provenance and timing.
- `state`: versioned desired config, migrations, profile/policy resolution, ownership manifest, revisions.
- `plan`: pure reconciliation and stable serializable action graph.
- `execute`: operation coordinator, locks, safe writer, package/service adapters, journal and rollback.
- `integrations/<tool>`: parser/merger/validator/health logic only where a manifest is insufficient.
- `app`: reusable use cases (`PlanInstall`, `ApplyPlan`, `Doctor`, `RestoreOperation`).
- `cmd` and `ui`: presentation adapters; neither constructs independent mutation logic.

State placement should also be separated:

- `XDG_CONFIG_HOME/<brand>`: user-desired product configuration only.
- `XDG_STATE_HOME/<brand>`: operations, ownership, migrations, backup indexes, non-secret audit history.
- `XDG_CACHE_HOME/<brand>`: detection/package snapshots with TTL and provenance.
- Native keychain/provider mechanisms: secrets. Never copy plaintext secret values into product logs/support bundles.

## Repository strategy

### Keep and refactor — recommended

Why:

- The registry, package abstractions, backup hardening, theme data, UI primitives, streaming/cancellation work, and regression history are valuable.
- Current failures are architectural seams that a rewrite would have to solve again: observed-vs-desired state, platform probes, ownership, package provenance, rollback, and release migration.
- A new repo does not remove the obligation to migrate the existing Homebrew binary, `~/.local/bin` copy, config directory, profiles, and backups.
- Preserving history is especially valuable because prior “fixed” checklists have repeatedly missed active alternate paths.

### Split only at real trust/product boundaries

- **Legacy Bash:** retire/freeze; do not create another maintained product repo.
- **Homebrew tap:** keep separate as distribution metadata, but make cross-repo tests a release gate.
- **Future enterprise control plane:** new repo/service only if requirements become remote fleet inventory, policy distribution, centralized identity/RBAC, secrets, event storage, and remote execution. It should speak a versioned protocol to this local agent; it should not fork its configuration logic.
- **Plugin ecosystem:** defer until the action/permission/provenance model exists. A plugin API before the safety kernel would expose unsafe writers as extension points.

Starting a replacement repo now is rated **2/10 strategically**; in-place safety refactor is **9/10**.

## Naming and rename blast radius

### Exact tracked counts

Measured against the current 195 tracked files using `git grep`:

- **123 tracked files** contain `dotfiles` case-insensitively.
- **987 matching lines** and **1,168 matched occurrences** contain the name case-insensitively.
- **71 tracked files / 109 lines** contain the exact Go module path `github.com/tekierz/dotfiles`.
- **18 files / 33 lines** contain the literal `.config/dotfiles`; 14 contain the tilde form `~/.config/dotfiles`. This is a lower bound because production also builds the path from separate components.
- **18 files** contain `bin/dotfiles`.
- **16 files** contain `dotfiles-setup`.
- **90 tracked files** match the migration-critical union of module path, config path, binary path, legacy setup name, Brew install/uninstall, or repository URL patterns.

### Difficulty score

- Display-only brand change: **3/10**.
- New CLI name while preserving `dotfiles` as an alias: **6/10**.
- Safe end-to-end rename of product, binary, package, repo/module, config/state, backups, release assets, and uninstall behavior: **8.5/10**.

The hard part is compatibility, not text replacement. A safe sequence is:

1. Choose the brand and verify CLI/package/repository/domain availability before code changes.
2. Introduce a centralized `ProductIdentity`/path resolver while preserving current names.
3. Land schema migrations, ownership manifest, operation journal, and `doctor` first.
4. Ship the new executable alongside a deprecated `dotfiles` shim; diagnose PATH collisions and exact binary provenance.
5. Dual-read old/new config and state roots, migrate atomically once, then one-write to the new root; retain read/rollback compatibility.
6. Preserve old managed markers/includes and backup manifests indefinitely or migrate them idempotently.
7. Transition Homebrew formula/repo/release assets with upgrade/uninstall tests from 2.0.1 and the `~/.local/bin` copy.
8. Change the Go module path last, after repository redirects and release compatibility exist.

Make the brand decision now, but execute the visible rename after the safety/migration substrate and before friends/family public beta. Do not combine config adoption and name migration in one opaque mutation.

## Observability, doctor, and supportability contract

Before friends/family, add:

- `dotfiles doctor [--json]` (under the eventual new name) reporting build provenance, all PATH candidates and versions, platform/arch, package-manager sources, timed tool probes with reasons, active config precedence, parse/ownership state, backup integrity, free space, network/proxy status, sudo capability, service/auth readiness, and migration status.
- `plan --json` with exact actions, targets, existing hashes/modes, privilege/network needs, reversibility, and warnings.
- Durable `operations/<id>.json` with plan hash, start/end/version, action outcomes, timings, backup ID, before/after hashes, and redacted errors.
- `support-bundle` that includes doctor and operation metadata but excludes file contents, command history, tokens, MCP env values, hostnames, emails, SSH destinations, and secrets by default.
- `restore --operation <id>` for the exact file actions of one operation, rather than only a timestamp directory whose relationship to the action is implicit.

Recommended stable exit classes: success/no change; success with warnings/nonreversible actions; validation/conflict; unsupported; privilege required; network/provider failure; partial apply with rollback complete; partial apply with manual recovery required. Exact integer values can be chosen later but must be documented and tested.

## Staged roadmap and explicit gates

### Stage 0 — Stop the line and make current claims truthful

- Disable or hide whole-file Apply/Save for adopted Git/Ghostty/tmux/Yazi/FZF/LazyGit/btop/Glow configs until safe ownership exists.
- Disable theme-wide regeneration.
- Remove “fully reversible” and install/integration claims that the code cannot meet.
- Stop default installation of `bin/dotfiles-setup`; stop Go self-copy of the main binary.
- Mark `tasks/new-tools-spec.md` blocked/superseded and default every future AI agent off.
- Fix the current macOS Glow test and require full tests on macOS.

**Gate 0:** `go test ./...`, race, vet, lint, and vulnerability checks green on Linux and macOS; no supported path can whole-file replace an adopted config without explicit preview/consent; one canonical binary provenance per install method.

### Stage 1 — Build the safety kernel

- Extract observation, desired state, planner, executor, and operation coordinator from UI code.
- Version schemas and implement migrations from 2.0.1/2.0.2-era layouts.
- Implement safe atomic/no-follow writer, inter-process lock/revisions, exact plan-derived backups, journal, and typed results.
- Add CLI JSON/dry-run/noninteractive/exit contracts and `doctor`.
- Classify every existing integration by capability and ownership.

**Gate 1:** preview, execution, backup, summary, and rollback consume the same serialized plan; 100% of file mutations have a verified inverse or are blocked; package/system actions are explicitly classified nontransactional.

### Stage 2 — Migrate current integrations, starting with risk

Order: Git include fragment; Ghostty precedence/managed include; tmux; Yazi three-file model; FZF safe argv/quoting; LazyGit/btop/Glow; Zsh managed block; Neovim overlays; Claude MCP merge. Import observed current values and preserve unsupported state.

**Gate 2:** on fixtures with unknown comments/keys/sections, changing one modeled field preserves every unowned semantic value; repeated apply is byte-stable outside managed regions; forced disk/permission/symlink/concurrency failures leave either old or new valid state and a truthful operation record.

### Stage 3 — Release and rename substrate

- Establish tagged reproducible release workflow, checksums, SBOM, signatures/provenance, and Homebrew formula automation/tests.
- Centralize identity/paths; add new-name shim and dual-read migration only after the brand is selected.
- Test upgrade from Homebrew 2.0.1, `~/.local/bin` 2.0.0-dev, and current config/backups; detect/remove only owned old artifacts.

**Gate 3:** clean install, upgrade, downgrade refusal/export, uninstall, and rollback pass on macOS arm64/amd64 and Linux arm64/amd64; installed binary reports tag, source, checksum, and active path; no stale PATH candidate is silently used.

### Stage 4 — Owner hardware alpha

First use a disposable macOS account, then the owner's preconfigured account only after Stage 2. Exercise 60x18, 80x24, and 120x40 terminals; cold status/dashboard performance; offline/denied-sudo/locked-package-manager/disk-full/cancel scenarios; existing Ghostty/Git/tmux/Yazi/FZF state; exact operation restore.

**Gate 4:** all planned actions and diffs manually reviewed; rollback restores file content and mode exactly; status/doctor completes within a defined budget (target <1s from cached package snapshot, progressive UI <2s cold); zero unexplained writes.

### Stage 5 — Friends/family limited deployment

- Ship a signed prerelease with AI agents off and install-only capabilities clearly labeled.
- Provide one-command doctor/support bundle and documented recovery/uninstall.
- Test fresh and adopted machines across supported macOS/Linux/Pi versions; define the exact support matrix instead of “Arch/Debian/Pi” generically.

**Gate 5:** every tester can identify version/provenance, export a redacted support record, restore the exact prior file state, and upgrade to the next prerelease without manual config surgery. No critical/high open issue in a reachable supported path.

### Stage 6 — Mock-enterprise simulation

- Define policy-vs-user precedence, MDM-managed file detection, restricted accounts, proxy/offline mirrors, noninteractive privilege, package allowlists, secrets boundaries, and immutable audit export.
- Pin CI actions/dependencies and enforce artifact provenance/vulnerability policy.
- Add AI agents one at a time only through approved provider/version/digest/auth/egress policies.

**Gate 6:** deterministic `plan --json`/apply under no TTY; no interactive sudo; no mutable remote scripts; externally managed conflicts fail closed; SBOM/provenance and audit records are reviewable; multi-account tests prove permissions and state isolation. Local “Users” profiles must not be presented as enterprise identity/RBAC.

## Final strategic recommendation

This project is worth saving, but the next development cycle should be a **safety-and-state architecture release**, not a feature release. The cleanest product story is:

> A single-host terminal/workstation reconciler that detects what is present, adopts only what the user authorizes, shows an exact plan, applies validated managed changes, and can prove what happened.

Once that statement is true, adding agents, GUI applications, Pi hardware paths, themes, a new name, and eventually an enterprise control plane becomes substantially easier. Adding them before it is true increases both the mutation surface and the number of migrations that must later be repaired.

## Verification notes

- Current `go test ./...` fails on macOS at `internal/tools/TestGlowConfigPathUsesUserConfigDir`; this independently refutes release-green task prose.
- Current branch/tag evidence: `release-remediation`, commit `7ddcb99`; tags visible locally are `v2.0.2` and `v2.0.1`; checkout binary reports `7ddcb99`.
- Current command provenance: `/opt/homebrew/bin/dotfiles` reports 2.0.1, `~/.local/bin/dotfiles` reports 2.0.0-dev, checkout `./bin/dotfiles` reports `7ddcb99`.
- No production source was modified by this audit.
