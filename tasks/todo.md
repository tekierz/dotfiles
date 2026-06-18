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
- [ ] config/: claude.go MCP rewrite (CRITICAL data loss), atomic config writes, EnsureDirs/ConfigDir guards
- [ ] pkg/: apt/pacman/brew CheckOutdated robustness, update.go dedup + aur manager
- [ ] tools/: claude_code installs CLI, Pi platform package fallback, cask IsInstalled, neovim LSP name
- [ ] runner/ + scripts/: command-injection hardening, script/PID perms, remove vestigial dead code
- [ ] cmd/: backup/restore path safety + perms, uninstall restore-failure handling, cmd/installer
- [ ] bin/dotfiles-setup: rm -rf-before-clone, chsh /etc/shells guard, stale zshrc blocks, dead code

## Phase B — UI correctness bugs (pre-migration, in current structure)
- [ ] installation.go data race (goroutine mutates App state)
- [ ] install-cache reload on Manage; renderProgress/installStep coupling
- [ ] mouse hit-detection offsets (deepdive/users/theme picker)
- [ ] favorites re-parse-per-frame perf; delete-active-user orphaning
- [ ] ctx theme/navStyle sync; misc low UI logic bugs

## Phase C — Complete ScreenManager migration (sequential, high-risk)
- [ ] Characterization (golden) tests for representative screens first
- [ ] Migrate all 45 screens to ScreenHandler; wire Dependencies/Provider DI
- [ ] Make ScreenManager the real dispatch path; remove legacy switch
- [ ] Move dead screens/ subpackage into the live path; fix ErrorScreen/SummaryScreen theme

## Phase D — Verbosity / duplication / dead code (post-migration)
- [ ] Consolidate ~30 near-identical tool files into a data table
- [ ] Extract repeated render/style boilerplate; dedup per-tool render blocks
- [ ] Split 1,400–1,700-line files; remove remaining dead code
- [ ] input_deepdive mirrored arms, animation MatrixRain, etc.

## Phase E — Tests
- [ ] runner/, scripts/, hotkeys/ unit tests (security-sensitive)
- [ ] regression tests for fixed bugs

## Phase F — Final verification
- [ ] build / vet / test / gofmt / golangci-lint all green
- [ ] Review full diff; summary report

## Review
(to be filled in as phases complete)
