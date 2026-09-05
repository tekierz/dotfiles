# Go-Only Deployability Cleanup Spec

Branch: `worktree-audit-remediation`
Status: Implemented with automated verification; manual TUI walkthrough still recommended

## Problem

Before this cleanup, the project had two installers with overlapping ownership:

- The Go app (`cmd/dotfiles`, `internal/*`) is the product we want to ship.
- The legacy Bash installer embedded its own mini `dotfiles` CLI and duplicated theme, restore, utility, and config logic.

Recent audits found that the Bash path is the largest source of drift, stale tests, and deployment ambiguity. Keeping it alive forces the repo to maintain duplicated behavior even though the Go app already owns the current TUI, CLI, tool registry, config writers, backup/restore flow, update flow, and utility install flow.

## Goals

- Make the Go app the single deployable product.
- Remove legacy installer drift from the codebase instead of repeatedly hardening duplicate Bash logic.
- Keep the upgrade path friendly for existing users by cleaning up old installed binaries.
- Leave the repository easier to review: fewer generated artifacts, fewer stale tests, fewer diary-style task notes, and clearer release commands.
- Close the current PR blockers before merge.

## Non-Goals

- Do not rewrite the Go TUI architecture again.
- Do not add a new shell installer to replace the old Bash installer.
- Do not change the Homebrew tap repository in this branch unless explicitly coordinated.
- Do not remove compatibility migrations for user data, such as hotkey favorite migration or legacy backup restore support.

## Product Decisions

1. The canonical install command is Homebrew:

   ```bash
   brew tap tekierz/tap
   brew install dotfiles
   ```

2. The canonical manual build path is:

   ```bash
   make build
   ./bin/dotfiles
   ```

3. `dotfiles install` is the only first-run installer UX.

4. Legacy installed binaries remain cleanup targets:
   - `dotfiles-setup`
   - `dotfiles-tui`

5. Legacy source files are not cleanup targets; they should be removed from the repo surface.

## Scope

### Remove

- `bin/dotfiles-setup`
- `bin/dotfiles-setup.ps1`
- `Formula/dotfiles-setup.rb`
- Bash-heredoc drift tests:
  - `cmd/dotfiles/embedded_cli_drift_test.go`
  - `cmd/dotfiles/embedded_cli_restore_test.go`
- README legacy curl install section.
- Docs sections that describe the Bash installer as a supported path.
- Task/todo claims that the Bash installer remains part of active remediation.

### Keep

- `internal/ui/installation.go` utility installation for `hk`, `caff`, and `sshh`.
- `cleanupOldInstallations()` so current Go installs remove old `dotfiles-setup` and `dotfiles-tui` binaries.
- `cmd/dotfiles` uninstall cleanup for stale installed binary names.
- Legacy backup restore compatibility in `internal/backup`, because users may already have old backups.
- Hotkey favorite migration, because it migrates user data.

## Required Behavioral Fixes

These should land with, or before, the legacy removal.

### P1: Manage Save Completion

`manageSavedMsg` is handled only by `manageScreen.Update`, but users can tab or escape away while `saveManageConfigCmd()` is in flight. If the result arrives after navigation, `snapshotManageBaseline()` is skipped and the scoped-save baseline remains stale.

Required fix:

- Handle `manageSavedMsg` globally in `App.Update`, similar to Backups/Users operation completions, or block navigation while saving.
- Prefer global handling so async completion semantics are consistent across management screens.
- Add regression coverage for "save then tab away, completion still updates status and baseline".

### P1/P2: Destructive Path Containment

`ConfigDir()` accepts `XDG_CONFIG_HOME` and `HOME` as-is. Those paths feed destructive deletes in uninstall and backup cleanup.

Required fix:

- Reject or ignore relative `XDG_CONFIG_HOME`.
- Require `ConfigDir()` to return an absolute path or `ErrNoConfigDir`.
- Add a shared safe removal helper for config-owned deletes:
  - canonicalize base
  - canonicalize deepest existing target parent
  - require target to remain under base
  - refuse empty, relative, root, or out-of-tree targets
- Cover uninstall config removal, backup delete, and backup cleanup.

### P2: Restore Source Symlink Escape

Destination path hardening exists, but backup source reads can still follow symlinks inside a backup session.

Required fix:

- In Go restore, reject source symlinks with `Lstat`, or compare `EvalSymlinks(src)` against canonical `backupDir`.
- Preserve legacy manifest compatibility, but do not follow source symlinks out of the backup tree.
- Add tests for source symlink escape in manifest-backed and legacy fallback paths if both are still supported.

### P2: `caff` Single Source

The Bash installer and Go installer currently emit different PID paths. Removing Bash eliminates one drift vector, but the Go utility still needs to be canonical.

Required fix:

- Keep only `internal/scripts.CaffScript`.
- Use one private runtime path consistently.
- Test `status`, `off`, numeric PID validation, process-name validation, and mode-700 directory behavior.

### P3: CLI Start Screen Routing

`SetStartScreen()` updates `a.screen`, but `NewApp()` already navigated the screen manager to the constructor default. Direct CLI routes can render one wrong frame before `Init` lands.

Required fix:

- Make `SetStartScreen()` retarget the screen manager immediately when it exists, or move initial navigation out of `NewApp()`.
- Add a direct-route render test for `manage`, `hotkeys`, and one `config <tool>` path.

### P3: Manage Global Context Sync

Manage Global edits mutate `a.theme` and `a.navStyle`, but do not mirror into `ScreenContext`. Screens that read context can show stale values.

Required fix:

- When Manage changes theme/nav, update both App fields and ScreenContext fields.
- Keep `SetTheme()`/palette sync behavior consistent with theme picker.

## Deployment Shape

### Makefile

Update targets:

- Remove `SETUP_SCRIPT`.
- `install` installs only `bin/dotfiles`.
- `run-theme` uses `theme --list`, or remove the target if redundant.
- `clean` may continue to remove `bin/dotfiles` if the binary remains tracked until a later decision.

### Tracked Binary

Current branch evidence shows `bin/dotfiles` can become stale and misleading. It reports an old VCS revision and dirty state.

Recommended decision:

- Stop tracking `bin/dotfiles`.
- Add `bin/dotfiles` to `.gitignore`.
- Build artifacts in CI under temp paths instead of relying on checked-in binaries.

If the binary must remain tracked for distribution, add a verification gate that fails when `go version -m bin/dotfiles` does not match the branch HEAD and expected Go version.

### Homebrew

The checked-in `Formula/dotfiles-setup.rb` has a placeholder SHA and installs the removed Bash script. It is not a deployable formula.

Required cleanup:

- Delete `Formula/dotfiles-setup.rb` from this repo.
- Document that the supported formula lives in `tekierz/homebrew-tap`.
- Coordinate tap updates separately if the tap still references legacy script files.

## Documentation Updates

README should describe:

- Homebrew install
- `dotfiles` CLI subcommands
- `dotfiles install` first-run flow
- `dotfiles manage`, `dotfiles hotkeys`, `dotfiles backups`, `dotfiles users`
- No curl-to-Bash install path

`docs/tools.md` should describe:

- Go registry-managed tools only
- Optional external tools such as `sshh` clearly
- No legacy-only tools unless moved to an "historical/removed" appendix

`tasks/todo.md` should become a current plan, not a merge diary. Keep historical audit detail in `tasks/deep-audit-findings.md`.

## Implementation Plan

1. Fix current Go PR blockers:
   - global `manageSavedMsg`
   - config-dir absolute/safe removal
   - restore source symlink containment
   - Manage context sync
   - direct start-screen routing

2. Remove legacy Bash surface:
   - delete Bash installer
   - delete formula
   - delete heredoc drift tests
   - update Makefile
   - update docs

3. Normalize utility ownership:
   - keep `internal/scripts` as the only utility script source
   - ensure `caff` path and tests are canonical

4. Clean deploy artifacts:
   - decide tracked binary policy
   - update `.gitignore` and CI/build docs if untracking

5. Re-run full verification.

## Verification Gates

Automated:

```bash
gofmt -l .
git diff --check
go build ./...
go vet ./...
go test ./...
go test -race ./...
golangci-lint run --timeout=5m
staticcheck ./...
govulncheck ./...
```

CLI smoke:

```bash
go build -o /tmp/dotfiles ./cmd/dotfiles
/tmp/dotfiles --help
/tmp/dotfiles version
/tmp/dotfiles theme --list
/tmp/dotfiles backups
```

Manual:

- Launch `dotfiles install`, navigate to summary, do not run install unless intentionally testing writes.
- Launch `dotfiles manage`, save a global change, immediately navigate away, confirm completion still applies.
- Launch `dotfiles backups`, create/delete a backup, confirm safe containment.
- Verify no docs mention `curl ... bin/dotfiles-setup | bash`.
- Verify `rg "dotfiles-setup"` only finds upgrade cleanup/uninstall compatibility notes or this spec/todo audit trail, not supported install paths.

## PR Readiness Criteria

The branch is PR-ready when:

- The Go app is the only supported deployable artifact.
- Legacy Bash installer files and Bash-specific drift tests are gone.
- Existing users still get stale installed binaries cleaned up.
- All required behavioral fixes above are covered by tests.
- Automated verification passes.
- Documentation no longer advertises removed install paths.
- The tracked binary policy is explicit and enforced.
