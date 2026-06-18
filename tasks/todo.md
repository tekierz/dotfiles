# Audit Remediation Plan

Branch: `worktree-audit-remediation`. Worktree-isolated, logical local commits per chunk.
Source: comprehensive audit (71 confirmed findings) + user direction (maximal scope).

## Decisions (from user)
- Scope: **everything**, including large refactors (tool-file consolidation, splitting giant files).
- Claude MCP: **fix properly** (correct file, merge-not-overwrite, real package names, install CLI).
- Abandoned refactor: **complete the migration** (all screens onto ScreenManager/handler + DI).
- Bash script: **fix everything**.

## Phase 0 — Docs + Deps (DONE)
- [x] Documentation accuracy across 13 files + registry comment — commit `68392dd`
- [x] `go mod tidy` indirect labeling — commit `a145a58`

## Phase A — Non-UI correctness + security (parallel by package)
- [x] config/: claude.go MCP rewrite (CRITICAL data loss), atomic config writes, EnsureDirs/ConfigDir guards — also extracted shared `internal/backup` package
- [x] pkg/: apt/pacman/brew CheckOutdated robustness, update.go dedup + aur manager
- [x] tools/: claude_code installs CLI, Pi platform package fallback, cask IsInstalled, neovim LSP name
- [x] runner/ + scripts/: command-injection hardening, script/PID perms, remove vestigial dead code
- [x] cmd/: backup/restore path safety + perms, uninstall restore-failure handling, cmd/installer removed
- [x] bin/dotfiles-setup: rm -rf-before-clone, chsh /etc/shells guard, stale zshrc blocks, dead code

## Phase B — UI correctness bugs (pre-migration, in current structure) — DONE
- [x] installation.go data race — converted to channel + listen-Cmd message passing (verified `go test -race`)
- [x] install-cache reload on Manage; renderProgress/installStep coupling
- [x] mouse hit-detection offsets (deepdive/users/theme picker)
- [x] favorites re-parse-per-frame perf; delete-active-user orphaning
- [x] cursor bound; NewScreenContext default clobber; deepdive index bounds

### Deferred follow-ups (revisit in C/D/F)
- [ ] Interactive install cancellation (context.CancelFunc + Esc handler) — install-flow agent flagged as larger
- [ ] backup.go: harden symlink-escape guard with EvalSymlinks on parent (lexical HasPrefix today)
- [ ] input_deepdive: deeper fix = ordered registry accessor keyed by stable ID (bounds-check landed)

## Phase C — Complete ScreenManager migration — DONE
- [x] Golden/characterization tests added per batch
- [x] Foundation: handlers in package ui, ScreenContext.app, screens/ subpackage absorbed, tool magic-int validated
- [x] Migrated all live screens (wizard, deep-dive/config x19, hotkeys/backups/update, Manage, progress/users/animation)
- [x] ScreenManager always-on; legacy View/key/mouse dispatch removed
- [x] Fixed migration-introduced bugs: global uiTick + installCacheDoneMsg dropped in managed mode
- [x] Deleted 12 dead ScreenManageX screens + dead render clusters (~1,316 lines)
- NOTE: screen count dropped (12 dead removed) — re-sync docs in Phase F (AGENTS.md still says 45 screens)

## Phase D — Verbosity / duplication / dead code (post-migration) — DONE
- [x] Consolidate 8 boilerplate tool files into a data table (simple_tools.go)
- [x] Remove dead Tool config schema (GenerateConfig/ApplyConfig); dedup WriteXConfig boilerplate
- [x] Big files shrank via migration (app.go 1461->977; ui ~15.9k->14.6k); dead code removed
- [x] #35 discarded log-panel computation removed; #33/#70/#71 resolved by migration

## Phase E — Tests — DONE
- [x] runner (69.6%), scripts (100%), hotkeys (100%) unit tests; security behavior pinned
- [x] golden/characterization tests added per migration batch; tool-metadata snapshot test

## Phase F — Final verification — DONE
- [x] build / vet / test / race / gofmt all green; golangci-lint errcheck cleared
- [x] binary smoke (version, theme list) works; docs re-synced to final state
- [x] dependency currency assessed: core deps already latest stable; blanket -u reverted
      (uncoordinated charmbracelet render-stack skew, no CVE benefit)

## Review
30 logical commits on worktree-audit-remediation. 120 files changed (+13.7k/-8.8k).
All 71 confirmed audit findings addressed (fixed, or resolved by removal/migration).
Deferred (recommended follow-ups, not blocking):
- Interactive install cancellation (Esc during install) — larger UX change
- backup.go: harden symlink-escape with EvalSymlinks on parent dir (edge case)
- CI: make golangci-lint/staticcheck/govulncheck blocking (currently continue-on-error)
- Stop tracking the built bin/dotfiles binary
- Coordinated bubbletea/lipgloss v2 migration for full dep currency (separate initiative)
