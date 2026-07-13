# Release-Readiness Plan

## Audit remediation program — 2026-07-10

The 2026-07-09 audit is the source of truth. Remediation is staged so safety and
observability land before new integrations or broad UI work.

### Mandatory execution guardrails

Every remaining slice follows `tasks/workflow-guardrails.md` and the machine-checked
`tasks/current-slice.scope`. The required state machine is `planned -> contract-frozen ->
tests-red -> implemented -> reviewed -> verified -> committed`; active work is never checked
complete. Run `make slice-check` at handoffs and before verification, then run
`make slice-check-candidate` after exact staging. The default hard ceiling is six production
files, sixteen total files, or 800 changed lines; crossing any ceiling stops edits until Sol
splits the slice or root records a before-the-fact exception.

Current slice: `manager-executable-private-authority` — **verified**, scope frozen under a
one-time recovery exception. Sol and Terra report GO; full test/race, vet, pinned lint/static,
vulnerability, formatting, diff, and scope gates pass. No npm/interpreter, helper-executable,
settings, profile, rename, or aesthetic behavior entered this commit.

### Batch 1 — reachable safety blockers (complete)

- [x] Replace dashboard package-only execution with a tool-aware install path so core
      terminal tools and custom installers (currently Claude Code) actually run.
- [x] Add focused wizard/Manage tests proving core tools enter the plan and custom install
      behavior cannot be bypassed.
- [x] Replace truncating/symlink-following tool config writes with one atomic, no-follow,
      mode-enforcing primitive and migrate every current writer to it.
- [x] Add failure-injection tests for existing mode, symlink refusal, and old-or-new atomicity.
- [x] Make global-config loading default-overlay/version-ready so partial older JSON cannot
      silently disable backup/retention defaults.
- [x] Enforce recorded file modes when restoring over an existing destination.
- [x] Keep generated fzf options inert when sourced, including hostile values.
- [x] Disable basename/path-based uninstall deletion until exact ownership and anchored
      recursive removal exist; propagate backup inspection and restore failures.
- [x] Run independent adversarial reviews over all safety patches and resolve every sustained
      blocker before accepting the batch.
- [x] Run focused tests, full Go tests/race, vet, formatting, lint/security checks, and
      documentation diff validation.

### Batch 2 — ownership, plan, and rollback kernel

- [ ] Introduce typed observations and one immutable action plan consumed by confirmation,
      backup, execution, summary, and rollback.
- [ ] Import native current values and preserve unknown/unowned settings; move Git to a
      managed include and define safe ownership for Ghostty/tmux/Yazi/LazyGit/btop/Glow.
- [ ] Derive backup scope from the action plan, fail closed, and add the same verified
      backup/preview behavior to Manage and standalone saves.
- [ ] Stop theme-wide regeneration of absent/unmanaged tools and preserve modeled settings
      omitted by Manage.
- [ ] Add schema versions, ordered migrations, ownership revisions, operation IDs, and a
      durable non-secret journal.
- [x] Stop self-copying the Homebrew-owned main binary.
- [x] Add executable provenance, stale-build detection, and PATH-collision diagnostics.
- [x] Add an explicitly reviewed repair flow for stale PATH entries; diagnostics remain
      deliberately read-only until ownership can be proven.

### Batch 3 — truthful UX, settings platform, and CLI contracts

- [ ] Replace boolean install state with structured package/binary/config/service/auth health.
  - [x] Add the immutable, generation-tagged installation-health domain and collector.
  - [x] Adopt one atomic installation snapshot in Manage and install planning: reject stale
        async completions, render Present/Partial/Missing/Unknown truthfully, and fail closed
        when planning from unknown, unsupported, stale, or environment-mismatched evidence.
  - [x] Adopt the same fresh, generation-bound installation snapshot in the CLI-tools
        deep-dive selector: render Present/Partial/Missing/Unknown and installability states
        textually, fail closed on loading/stale/error or generation mismatch, keep Cursor
        Agent/Hermes discovery-only and Claude Code context-only, and bound keyboard/mouse
        views at 60x18, 80x24, and 120x40.
- [ ] Bind exact executable observations into reviewed package and npm installation authority.
  - [x] Add an independently reviewed, bounded, path-redacted executable-identity observation
        primitive with deterministic drift revalidation; do not claim it closes the final
        revalidation-to-exec race.
  - [x] Adopt the primitive in Brew, Apt, Pacman, and Paru construction without widening the
        package-manager interface, and route manager invocations through the observed path.
  - [x] Bind accepted package-manager identities into private plan authority and revalidate
        immediately before mutation while documenting the remaining spawn boundary.
  - [ ] Add the equivalent npm executable/interpreter-chain authority before enabling npm
        recipes for owner-hardware Apply testing.
- [ ] Add current-source/provenance, Essentials/Advanced/raw layers, diff, and capability badges.
- [ ] Make 60x18/80x24/120x40 layouts responsive with viewports, compact tabs, glyph/color
      fallbacks, reduced motion, and explicit Save/Cancel semantics.
- [ ] Centralize async operations with IDs/single-flight reducers and eliminate synchronous
      detection from input handlers.
- [x] Make installer terminal outcomes and summaries fail closed: retry/escape require a fresh
      reviewed plan, failed/skipped/unknown attempts never render success, and sealed plan,
      rollback-scope, and operation facts survive post-attempt cache refreshes at 40x14,
      60x18, and 80x24.
- [ ] Add `doctor`, `plan --json`, noninteractive apply, deterministic exit codes, and a
      redacted support bundle.
  - [x] Add deterministic, versioned `status --json` v1 directly from the installation-health
        collector, with redacted public evidence, explicit uncollected capabilities, and stable
        success/failure exit behavior. Plan, apply, and the bounded support JSON contract are
        complete; the broader CLI/release and install/apply deployment gates remain open.
  - [x] Implement the reviewed `plan --json` contract in
        `tasks/plan-json-contract.md`: public projection tests first, then a neutral headless
        install-only planner accepting explicit repeated `--tool` intent with no defaults or
        `App`, followed by a complete private authority fingerprint and the Cobra command.
        Directly marshaling `operation.Plan` or wrapping the private TUI planner is prohibited.
  - [x] Add hash-bound noninteractive package apply after the complete authority fingerprint:
        require explicit tools/confirmation/hash, fresh replan, detector revalidation, lock,
        running/terminal journal, cancellation, and bounded deterministic exits. Package-only
        actions truthfully record that no filesystem rollback point exists.
  - [x] Register the reviewed `dotfiles support --json` v1 command on the validated public
        projection and read-only journal summary boundary, with fixed collection order,
        cancellation precedence, closed partial reason codes, one-write output, and deterministic
        complete/partial/fatal exits.
  - [x] Publish review-before-sharing guidance and exact stdout-only scope in command help,
        README, the tool reference, and maintainer CLI guidance. Raw `doctor --json`, operation
        journals, configuration, and logs are explicitly not share-safe substitutes.
  - [x] Complete the final adversarial/static verification and owner-hardware read-only gate:
        the fresh `6fff1ac` binary emitted byte-identical complete documents, passed framing and
        forbidden-field/path checks, and left the existing operation-state directory metadata
        unchanged. Full test/race/vet, pinned lint/static/vulnerability tools, GoReleaser,
        ShellCheck, module checks, and release-target cross-compiles pass. The broader parent
        CLI/release and install/apply deployment gates remain open.
- [ ] Generate settings/help/hotkeys/docs/tests from tool manifests where practical.

### Batch 4 — integrations, distribution, and deployment gates

- [ ] Finish current install-only integrations or label them honestly.
- [ ] Add Codex, Cursor Agent, OpenCode, Pi, T3 Code, and Hermes only after the safety kernel,
      all opt-in with provenance/auth/permission/egress policy.
- [x] Retire the legacy Bash product from active distribution: remove its source and
      bespoke tests, stop `make install` from distributing it, delete active execution
      instructions, and retain a conservative migration guide.
- [ ] Remove the public Homebrew formula's basename-only deletion of old
      `dotfiles-tui`/`dotfiles-setup` executables and correct its stale v2.0.1 metadata
      and legacy feature claims before recommending tap upgrades.
- [ ] Make Linux and macOS tests fully blocking; add reproducible signed artifacts,
      checksums, SBOM/provenance, and Homebrew upgrade/rollback tests.
- [ ] Complete owner-hardware, friends/family, and mock-enterprise gates from
      `tasks/pre-deployment-audit-2026-07-09.md`. The active install/apply owner gate is
      `tasks/install-apply-owner-test.md` and must pass on a disposable VM or spare Mac before
      advancing to either broader deployment tier.
- [ ] Execute a compatibility-first product rename (difficulty 8/10) before broader beta.

### Batch 1 review

- **Accepted safety changes:** descriptor-anchored filesystem operations; atomic generated
  config writes; tool-aware install execution; versioned/serialized state; atomic fail-closed
  file restore; inert fzf shell options; and non-destructive uninstall behavior.
- **Independent review:** each safety area received adversarial review and re-review after
  fixes. The final uninstall review exercised compiled CLI exit codes as well as source/tests.
- **Verification:** `go test ./...`, `go test -race ./...`, `go vet ./...`, formatting and
  diff checks, golangci-lint (0 issues), Staticcheck (0 issues), ShellCheck, govulncheck
  (0 reachable vulnerabilities), build/CLI smoke tests, and cross-platform compile/tests.
- **Deliberate fail-closed limits:** directory restore/removal and automatic uninstall
  deletion remain disabled until descriptor-anchored recursion and exact ownership records
  exist. These are unfinished release requirements, not silent feature claims.
- **Release verdict after Batch 1:** still **No-Go** for owner-hardware Apply/Save testing,
  friends/family, or mock-enterprise deployment. Batches 2–4 remain active release work.

### Batch 2 progress

- Theme-only Manage saves now persist desired theme state without scheduling any tool
  generators. Cross-tool theme application remains confined to the reviewed installer plan,
  preventing absent or unadopted application configs from being synthesized from defaults.
- Tmux now owns one exact managed section instead of the complete `~/.tmux.conf`; native
  settings and comments remain byte-for-byte intact, legacy product-owned files migrate to
  the bounded form, and ambiguous marker layouts fail closed.
- Manage now exposes tmux split-binding style instead of silently restoring the hidden
  compiled default whenever another tmux field is saved.
- Tmux discovery now follows upstream's user-config precedence and binds both planning and
  writing to the same active legacy/XDG source; the reload hotkey targets that source, and
  unsafe relative XDG paths or symlinked candidates fail closed.
- Manage and standalone tmux saves cannot silently adopt an existing native source: first
  ownership requires the reviewed installer plan and its mandatory rollback point.
- Native preference schema v2 now has explicit ordered migration semantics: v1 retains
  Git/Ghostty intent while permitting first tmux hydration, prototype files permit one-time
  adoption, and malformed or future versions fail closed.
- Tmux now has read-only native/XDG import with line-level native-vs-managed provenance for
  every representable generator field. Dynamic/conditional/targeted syntax, conflicting
  composite settings, and malformed markers block installer and direct saves; native files
  without plugin declarations no longer inherit compiled TPM defaults.
- Btop and Glow now validate persisted values at the writer boundary before any filesystem
  mutation, blocking directive/YAML injection and invalid ranges/enums. Btop per-tool theme
  overrides now plan, create, and reference the same exact theme artifact.
- Existing Yazi, LazyGit, btop, and Glow generator settings are now exposed consistently in
  both Manage and standalone editors, eliminating cross-surface resets from hidden modeled
  fields; LazyGit's width control is labeled for what it actually changes.
- Yazi now pins generation/import to the v26.5.6 schema and classifies `yazi.toml`,
  `keymap.toml`, and `theme.toml` independently. Missing and exact-current product forms are
  writable; native, malformed, and exact-historical files remain read-only with per-file
  provenance and reasons preserved through planning.
- Yazi keymap generation is non-destructive: Vim mode retains upstream defaults with a
  compatibility/style header only, while Emacs mode prepends five bounded bindings. Import
  recognizes current and historical product forms without granting historical bytes write
  authority.
- Reviewed installer execution now uses three deterministic Yazi actions with independent
  evidence, journal, and rollback scope. Standalone saves are limited to main plus keymap;
  Manage schedules only the affected main/keymap files; ordinary saves do not synthesize a
  theme file.
- Reviewed execution freezes the selected Yazi directory, active Ghostty source candidate,
  global `global.json`, and Manage `tools/manage.json` paths at plan time. Environment drift
  cannot redirect those writes after confirmation.
- The LazyGit safety slice now pins compatibility to v0.62.1 and manages only four bounded
  global concepts in an exact product-owned file. Missing or exact current product files can
  be planned and written at the active CONFIG_DIR/XDG/platform path; arbitrary, malformed,
  custom, legacy-fallback, external, and `LG_CONFIG_FILE` sources hydrate read-only and block
  mutation with their reason preserved in installer, Manage, and standalone previews.
- LazyGit planning, backup, execution, and rollback authority now bind the same dynamic target,
  and the Delta pager requires a proven installer selection or cached installed dependency.
  Repository-local LazyGit configuration may still override these global defaults and is
  disclosed as such rather than treated as writable dashboard state.
- The Batch 2 theme/settings item remains open until omitted modeled settings are preserved
  and Manage/standalone saves have reviewed preview, backup, and rollback parity.

### Batch 3 progress

- `dotfiles status --json` now publishes a deterministic v1 installation-health document
  directly from the shared collector. It binds the requested platform, manager, generation,
  and current health schema; exposes typed package/direct evidence with explicit
  config/service/auth `not_collected` capability markers; redacts paths and credentials before
  computing its public digest; and fails atomically with a stable nonzero contract. The default
  human-readable `dotfiles status` output remains compatible.
- Manage and reviewed installation now consume one generation-bound, immutable installation
  snapshot. Missing and Partial tools produce exact install/repair plans; stale, unknown,
  unsupported, environment-mismatched, or recipe-drifted evidence fails closed. Package-only
  Manage plans retain journal/lock authority without pretending to have filesystem rollback,
  while mixed filesystem plans still require a verified backup. The former direct Manage
  install worker and its pre-probe dispatch route are removed.
- Install actions now publish a versioned, non-secret recipe in the immutable operation
  plan: platform, package manager, exact package/npm arguments, typed detector,
  authentication expectation, and risk are hash-bound and visible in the confirmation UI.
- Reviewed wizard execution is derived only from the immutable plan actions. It rejects
  missing/extra/duplicate selections, recipe digest drift, platform/manager drift, and
  detector-state drift before mutation; recipe-backed tools never dispatch their legacy
  arbitrary `Install` method.
- Package, binary, and app-bundle detection now follows the accepted typed recipe instead
  of decorative metadata or arbitrary `Tool.IsInstalled` behavior. Package-receipt
  detectors must refer to packages installed by the same recipe.
- Mutable vendor shell scripts are deliberately unrepresentable. Manage refuses direct
  installation for recipe-backed tools and routes users to the reviewed installer flow.
- The neutral executable-identity foundation now captures a bounded, path-private, immutable
  Darwin/Linux snapshot over invocation and canonical paths, mode, size, device/inode, and
  content digest. Deterministic race tests cover alias/parent retargeting, atomic replacement,
  growth during hashing, and pre-open FIFO/symlink swaps; nonblocking no-follow descriptor
  opening prevents those swaps from hanging. Sol and Terra independently accepted the slice,
  and focused/full-package/race/vet/pinned lint/static checks pass. This is drift observation,
  not spawn authority: npm/interpreter identity and the final revalidation-to-exec race remain
  open.
- Brew, Apt, Pacman, and Paru now capture one identity-only executable source at construction;
  absolute/clean/observable lookup results are required, unsafe Paru candidates cannot silently
  downgrade to Pacman, and automatic Arch detection performs no outer retry. Every current
  direct, sudo-child, and streaming manager route uses the captured invocation path, while
  zero or malformed identities block both manager and auxiliary subprocesses. Paru uninstall
  and all other Paru mutation routes remain direct. `sudo`, `dpkg`, `dpkg-query`,
  `checkupdates`, script interpreters, and post-construction drift are explicitly still outside
  this slice's authority.
- Accepted headless, package-only Manage, and full-wizard plans now bind the exact manager
  executable identity into private authority whenever a reviewed package-manager/cask step
  requires it; npm-only and config-only plans retain no manager identity. Apply and TUI
  execution reject same-name executable substitution, revalidate before manager-backed
  detectors and every manager mutation, run an exact all-action detector preflight before the
  first package/npm/cask mutation, and repeat a monotonic per-action check: false-to-true
  transitions skip without mutation as already satisfied, while every other drift fails closed.
  The documented revalidation-to-spawn race remains.
- Remaining provenance work includes npm executable/interpreter-chain identity and broader
  Manage plan/preview parity.
- Nonblocking hardening remains: app-bundle/cask plans can create private operation-state
  bookkeeping before same-path manager drift is refused. No package/cask mutation occurs;
  a later slice may revalidate required manager identity before state bootstrap to avoid that
  state churn.
- Codex and Pi now use version-pinned, macOS-only npm recipes; OpenCode uses only
  its reviewed Homebrew/Arch routes; T3 Code uses an exact typed Homebrew-cask
  action. Unsupported platform rows are disabled instead of poisoning the plan.
- `dotfiles plan --json` now shares the neutral explicit install planner with the TUI. It
  accepts only repeated explicit `--tool` intent, collects one installation snapshot,
  publishes the complete private-authority fingerprint only as `plan_hash`, emits one
  deterministic redacted document, and implements stable exit codes 0, 2, and 1. Public JSON
  cannot reconstruct private execution authority.
- `dotfiles apply --yes --plan-hash <hash> --tool <id>...` now rebuilds that private authority
  from one fresh snapshot and executes only an exact hash match. Syntax, stale/no-change,
  cancellation, operational failure, and post-mutation output failure have bounded exit and
  stream contracts; package operations are locked and journaled, with detector checks both
  before state bootstrap and immediately before execution.
- Installation progress now treats the active screen dimensions as authoritative and uses
  compact, detailed, or expanded phase/output windows at 60x18, 80x24, and 120x40. Long
  installer output is sanitized and visibly bounded, while only matching live and sealed
  success facts may render a completed title, check marks, or a full progress bar. Focused,
  full-UI, race, and pinned static checks pass; broader responsive-screen work remains open.

### Current product-gap checkpoint — 2026-07-12

- [x] Give the Manage left selector a clear, accessible application taxonomy so terminal,
      editor, AI-agent, utility, service, and system-management tools are distinguishable at
      60x18, 80x24, and 120x40 without relying on color or optional glyph support alone.
- [x] Diagnose and repair Pi installation reachability from Manage while preserving the
      immutable typed-recipe plan, confirmation, journal, and execution authority.
- [x] Reconcile the intended new-tool registry against the deep-dive installer and make every
      supported integration selectable with truthful unsupported/install-only states.
- [x] Add focused tests for Manage taxonomy, Pi plan reachability, and deep-dive registry
      completeness before production changes; require adversarial review and responsive-view
      evidence before accepting each slice.
- [ ] Specify installation-profile loading, validation, versioning, portability, preview, and
      failure behavior now, but defer implementation until installer and integration behavior
      has passed owner-hardware verification.
  - Profiles remain explicitly deferred until deterministic `plan --json`, hash-bound apply,
    and owner-hardware install/repair/drift/rollback verification are complete. A future profile
    is an explicit intent source and may not bypass the reviewed plan/apply authority chain.

The supported deep-dive registry is now exact for Codex, OpenCode, Pi, T3 Code, Cursor
desktop, and the existing service/app integrations. Cursor Agent CLI is registered as a
distinct discovery-only integration with unknown presence and unsupported installation;
the dashboard must not offer an install action until its artifact, architecture,
authentication, and ownership contracts are reviewed. Hermes discovery-only is now
registered with unknown/unsupported health and must not be presented as installable.
Installation profiles remain deferred under the unchecked installation-profile item above.

## Comprehensive pre-deployment audit — 2026-07-09

- [x] Inventory every tracked file and record exact coverage.
- [x] Review all Go production code and tests for correctness, security, architecture,
      maintainability, and feature completeness.
- [x] Review the TUI/CLI promises and every dashboard surface against implemented behavior.
- [x] Audit aesthetics, responsive terminal UX, navigation, CLI/hotkey ergonomics,
      discoverability, accessibility, and perceived polish.
- [x] Build a per-tool matrix of settings supported upstream, modeled internally,
      exposed in the dashboard, and actually written by generators.
- [x] Review shell installers, CI, build/release configuration, and cross-platform behavior.
- [x] Review every tracked document for accuracy, drift, prototype residue, and planned work.
- [x] Run independent adversarial verification and a completeness pass over all findings.
- [x] Run automated build, vet, format, race, lint/security, and CLI diagnostics where available.
- [x] Produce `tasks/pre-deployment-audit-2026-07-09.md` with prioritized findings,
      deployment readiness, repository strategy, cleanup plan, and rename difficulty score.

### Audit review

- **Verdict:** No-Go for Save/Apply/install on an existing account, friends/family, or
  mock enterprise. Read-only use and disposable-user/VM engineering tests only.
- **Coverage:** all 195 tracked baseline paths / 51,980 lines, followed by three
  independent finding checks and one cross-cutting architecture/strategy review.
- **Primary blockers:** incomplete and custom-installer-bypassing install plans;
  non-importing full-file config writers; theme-wide resets; incomplete/fail-open
  backup; competing binary owners; failing macOS tests; legacy/release drift.
- **Strategy:** keep and refactor this repo; retire the legacy Bash product; create a
  separate repository only for a future genuine multi-host enterprise control plane.
- **Rename:** 8/10 for a compatibility-safe product/data/distribution migration.
- **Report:** `tasks/pre-deployment-audit-2026-07-09.md`; detailed evidence is under
  `tasks/audit-work/`.
- **Audit baseline:** no production changes were made by the audit itself. The remediation
  commits and current release status are tracked in the program and review above.
- [x] Local audit-environment cleanup: restored Homebrew developer mode to off.

> The 2026-07-09 audit supersedes the older green-state claim and severity counts below.
> Completed historical checkboxes are not current release evidence.

**Created:** 2026-07-03 · **Target:** first public release
**Source:** full-codebase audit re-verified against `main` — details, evidence, and exact
locations for every item are in `tasks/release-audit-2026-07-03.md`.
**State of main:** build/vet/test/race/gofmt green; govulncheck clean. Two prior
remediation cycles fixed 62 of 160 audit findings; the items below are what remains.

Severity counts remaining: **3 critical, 26 high, 37 medium, 32 low.**
P0+P1 are release blockers; P2 should ship but won't eat data; P3+ is post-release.

---

## P0 — Data-loss bugs (must fix before any release)

- [x] **Self-destructing binary**: `installUtilities` (internal/ui/installation.go) deletes
      the currently running `dotfiles` binary before copying the replacement; if the copy
      fails the user has no binary. Write to temp + rename, never remove-then-copy.
- [x] **Backup format schism (cluster)**: bash script writes directory-format backups; Go
      restore silently skips them (`internal/backup/backup.go` fallback drops dirs and does
      lossy `_`→`/` name mapping). Consequences to fix together:
      - [x] `dotfiles uninstall` "restores" a bash-format backup as a no-op, then
            `RemoveAll`s the backups directory — unrecoverable loss (cmd/dotfiles/main.go).
            Uninstall must refuse to delete backups it could not actually restore.
      - [x] TUI restore reports success on backups it silently skipped.
      - [x] Either teach Go restore the bash manifest format, or migrate/refuse loudly.
- [x] **Bash restore half-aborts**: resolved by retiring and removing the unsupported
      Bash product; no current restore path executes its duplicated logic.
- [x] **neovim preset destroys config + its only backup** (internal/tools/neovim.go):
      re-running the preset flow moves the user's config to a fixed backup path,
      clobbering the previous backup, then a failed clone leaves nothing. Timestamped
      backups + clone-to-temp-then-swap.
- [x] **Git config clobber (residual)**: Go `WriteGitConfig` preserves native settings;
      the clobbering Bash implementation was removed with the retired product.

## P1 — Release blockers (broken promises, versioning, dead gates)

Versioning / distribution:
- [x] Align version to the release tag everywhere: `Makefile` still hardcodes
      `VERSION = 2.0.1`; cmd/dotfiles/main.go fallback constant stale. Single-source from
      git tag via ldflags.
- [x] Stop tracking the compiled `bin/dotfiles` ELF binary in git (it ships stale —
      currently reports 2.0.1). Add to .gitignore; adjust Makefile/README accordingly.
- [x] Delete or fix the stale in-repo `Formula/dotfiles-setup.rb` (placeholder sha256);
      the real formula lives in the homebrew-tap repo — having both invites drift.
- [x] Decide the fate of `bin/dotfiles-setup.ps1` for the initial release: it is
      undocumented-in-flow, version "1.0.0", pipes `get.scoop.sh` over plain HTTP to
      `Invoke-Expression`, clobbers `$PROFILE` with no backup, silently weakens execution
      policy, suppresses all install failures, and prints "Windows Terminal configured"
      for a scheme it never writes. **Recommendation: remove it from the initial release**
      (or mark experimental + fix the four HIGHs). Full list in the audit report.

CI / quality gates:
- [x] `.golangci.yml` is incompatible with the current golangci-lint, and the CI lint step
      is `continue-on-error` — linting is silently dead. Fix config, make it blocking.
- [x] Make the CI Security job able to fail: `govulncheck` and `staticcheck` are
      `continue-on-error` (race gate is already blocking).

Features that don't do what the UI says:
- [x] Zsh "Plugins" checkboxes are dead UI — five plugins selectable, none ever wired
      into the generated .zshrc (internal/ui/screen_config_zsh.go + internal/tools/zsh.go).
      Wire them or remove the section.
- [x] Default "p10k" prompt style generates a .zshrc that never loads Powerlevel10k and
      nothing installs it — new users get a broken prompt out of the box (internal/tools/zsh.go).
- [x] fzf config screen is a no-op: generated config file is never sourced (internal/tools/fzf.go).
- [x] macOS Apps screen offers 6 apps that don't exist in the tool registry (screen list drift).
- [x] tmux percent-style split bindings are bound then immediately unbound (internal/tools/tmux.go).
- [x] Hotkeys aliases: input can't accept `h`, `l`, or space; saved aliases are never
      consumed by anything; and `q` while typing an alias instantly quits the TUI
      (internal/ui/screen_hotkeys.go — last unguarded quit path).
- [x] Manage screen: keyboard input while the install-log panel is shown still mutates
      hidden settings fields (mouse path was fixed; keyboard + misleading "↑↓: scroll"
      footer remain) (internal/ui/screen_manage.go).

Retired Bash installer (historical findings; source and execution docs removed):
- [x] `--list-backups` prints one entry then exits 1 (`((count++))` under `set -e`).
- [x] `setup_utilities` writes a legacy bash CLI to `~/.local/bin/dotfiles`, shadowing or
      clobbering the Go binary of the same name.
- [x] Generated CLI calls `sed_i` which is never defined inside the heredoc (runtime crash).
- [x] KDE systems without `kwriteconfig` abort the entire install under `set -e`.
- [x] Raspberry Pi model detection lost in a `$( )` subshell — Pi-specific setup never runs.
- [x] Embedded CLI theme table drifted: 13 themes vs 16 (frappe/macchiato silently get mocha).

## P2 — Should fix before release (correctness/security, non-data-loss)

- [x] `sudo apt update` runs synchronously inside CheckOutdated — TUI hangs on the sudo
      password prompt (internal/pkg/apt.go; partial mitigation exists).
- [x] pacman `Update()` installs from a stale sync DB — reported updates don't apply
      (internal/pkg/pacman.go; use the checkupdates-db pattern or a full -Sy transaction guard).
- [x] `CheckAllUpdates` swallows all per-manager errors — total failure renders as
      "everything up to date" (internal/pkg/update.go).
- [x] ManageConfig saved from a worker goroutine while the UI goroutine can still write
      through field pointers — data race / torn JSON (internal/ui/manage_dualpane.go:95-113).
- [x] Raw package-manager output rendered to the terminal without stripping ANSI/control
      sequences (update results path) — escape-sequence injection surface.
- [x] caff pidfile in world-writable /tmp is predictable — cross-user process-kill;
      the Go copy was fixed and the duplicated retired-script copy was removed.
- [x] App-detection substring matching produces false "installed" (internal/tools/apps.go).
- [x] Hotkeys config path ignores `XDG_CONFIG_HOME`, diverging from ConfigDir().
- [x] Blocking package-manager subprocess calls inside `View()` via ensureInstallCache
      (six screen_config_*.go call sites) — move to async Cmd like the rest of the app.
- [x] yazi theme.toml declared-but-never-written; PreviewMode ignored. glow config written
      to a path glow doesn't read on macOS.
- [x] bash: package install failures silenced then "installation complete" reported.

## P3 — Cleanups (post-release acceptable; tracked so they don't rot)

- [ ] Extract shared per-theme palette: internal/tools/yazi_theme.go re-declares 16-theme
      hex colors that overlap ~9 fields with internal/ui/styles.go ThemePalettes; move the
      shared table to a low-level package both can import (ui imports tools, so the
      palette must live below both to avoid a cycle).
- [ ] caff `is_caffeine` uses `ps -p PID -o comm=` with no fallback — not portable to
      busybox ps (narrow: busybox systems rarely run systemd-inhibit; macOS uses real ps).
- [ ] Micro-perf (flagged in review, low): matchesToken re-normalizes the wanted name per
      directory entry; sanitizeLogLine allocates twice per line; installOutput appends
      sanitize inline at two sites instead of a funnel helper like appendInstallLog.
- [ ] Dedupe: ensureInstallCache vs loadInstallCacheCmd (~55 dup lines); favorites-filter
      block ×5; dual-pane layout engine ×3; styles defined twice (var block vs
      updateStyles — GradientCyber already drifted); backup/restore/cache flow triplets;
      bash outer-script vs generated-CLI function copies.
- [ ] Test quality: mockScreenHandler records but never asserts message forwarding;
      renderFileTree tests partially non-hermetic/tautological (some fixed on main).
- [ ] All remaining LOW items — enumerated with locations in the audit report.
- [ ] Deferred from prior remediation: interactive install cancellation (Esc);
      EvalSymlinks hardening in backup path guard; coordinated bubbletea/lipgloss v2
      dependency migration.
- [ ] Retire the dormant accepted-triple Yazi writer APIs after the compatibility harness
      confirms no supported caller still depends on aggregate three-file authority.

## Pre-release checklist (run in order, after P0–P2 land)

- [x] `go build ./... && go vet ./... && gofmt -l . && go test -race ./...` all clean
- [x] golangci-lint clean with the repaired config; CI fully blocking
- [x] `govulncheck ./...` re-run clean
- [ ] Run the `pre-pr-tests` skill checklist (manual TUI pass on macOS + one Linux)
- [x] Automated macOS Glow path parity: planner, importer, registry, and writer resolve
      the same go-app-paths/Viper-precedence target with revision-bound tests.
- [ ] Manual owner-hardware confirmation remains: run `glow config` and verify the
      active macOS path and resulting behavior before release.
- [ ] Add pinned Glamour v0.10.0 style-schema/renderer validation, then enable custom
      Glow style editing. Until then, imported custom styles remain read-only.
- [ ] Fresh-machine install test: Homebrew tap and signed release artifact paths; then
      uninstall and verify configs restored byte-identical (this exercises the P0 backup fixes)
- [ ] Tag release; verify `dotfiles --version` matches the tag; update homebrew-tap
      formula sha256; verify `brew install` from the tap
- [ ] README final pass: supported install instructions match release artifacts and the
      Homebrew tap; verify no retired-installer execution path is advertised

## Post-release backlog (carried forward)

- New AI CLI tools — the older `tasks/new-tools-spec.md` is stale and must not be treated as
  implementation authority; current official-source verification and the reviewed recipe
  boundary govern safe routes, while mutable-script-only tools remain blocked.
- Features from archived beta.plan: config export/import, tool dependency graph,
  `dotfiles doctor`, plugin system, theme customization guide, user guide
- Integration/E2E test suite for install/uninstall; UI snapshot tests; CI platform matrix
- Windows support decision (native PowerShell done right, or WSL-only officially)
