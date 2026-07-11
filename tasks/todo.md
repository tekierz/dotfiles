# Release-Readiness Plan

## Audit remediation program — 2026-07-10

The 2026-07-09 audit is the source of truth. Remediation is staged so safety and
observability land before new integrations or broad UI work.

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
- [ ] Add current-source/provenance, Essentials/Advanced/raw layers, diff, and capability badges.
- [ ] Make 60x18/80x24/120x40 layouts responsive with viewports, compact tabs, glyph/color
      fallbacks, reduced motion, and explicit Save/Cancel semantics.
- [ ] Centralize async operations with IDs/single-flight reducers and eliminate synchronous
      detection from input handlers.
- [ ] Add `doctor`, `plan --json`, noninteractive apply, deterministic exit codes, and a
      redacted support bundle.
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
      `tasks/pre-deployment-audit-2026-07-09.md`.
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
- The Batch 2 theme/settings item remains open until omitted modeled settings are preserved
  and Manage/standalone saves have reviewed preview, backup, and rollback parity.

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

## Pre-release checklist (run in order, after P0–P2 land)

- [ ] `go build ./... && go vet ./... && gofmt -l . && go test -race ./...` all clean
- [ ] golangci-lint clean with the repaired config; CI fully blocking
- [ ] `govulncheck ./...` re-run clean
- [ ] Run the `pre-pr-tests` skill checklist (manual TUI pass on macOS + one Linux)
- [ ] On macOS, confirm glow reads its config where we write it
      (`glow config` should show/edit `~/Library/Preferences/glow/glow.yml`,
      per go-app-paths User scope)
- [ ] Fresh-machine install test: Homebrew tap and signed release artifact paths; then
      uninstall and verify configs restored byte-identical (this exercises the P0 backup fixes)
- [ ] Tag release; verify `dotfiles --version` matches the tag; update homebrew-tap
      formula sha256; verify `brew install` from the tap
- [ ] README final pass: supported install instructions match release artifacts and the
      Homebrew tap; verify no retired-installer execution path is advertised

## Post-release backlog (carried forward)

- New AI CLI tools — `tasks/new-tools-spec.md` (researched, ready to implement; re-verify
  package names first)
- Features from archived beta.plan: config export/import, tool dependency graph,
  `dotfiles doctor`, plugin system, theme customization guide, user guide
- Integration/E2E test suite for install/uninstall; UI snapshot tests; CI platform matrix
- Windows support decision (native PowerShell done right, or WSL-only officially)
