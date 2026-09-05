# Current checkout product/CLI/TUI audit — 2026-09-05

Scope: read-only review of cmd/dotfiles and internal/ui, relevant instructions, configuration/profile adapters, and user documentation. No production files or Git state changed. Findings below concern the current working checkout, not history. These are deterministic handler-level Go reproductions through an overlay, not a manual terminal session or real package installation. The overlay introduces an in-memory extra test file; temporary profiles/config files are confined to test HOME. Backup selection repro deliberately uses invalid catalog authority, proving the chosen name before any restore or delete can mutate files.

## Verification

Command:

```sh
GOCACHE=/tmp/dotfiles-audit-2026-09-05/go-cache go test -overlay=/tmp/dotfiles-product-overlay.json ./internal/ui -run '^TestAudit' -count=1 -v
```

Evidence: `/tmp/dotfiles-product-repro_test.go`, `/tmp/dotfiles-product-overlay.json`, `/tmp/dotfiles-product-repro-output.txt`. All seven behavioral assertions intentionally FAIL on current code; these are newly constructed audit probes, not failures from the existing suite. Go 1.27.1 darwin/arm64. Root owns baseline suite/static/race verification separately.

## Findings

### PRODUCT-1 — P1 — Backup confirmation can operate on a different backup than the named prompt

Primary location: `internal/ui/screen_backups.go:174-186`, with the missing modal mouse guard at `253-276` and fixed prompt text at `219-231`.

Trigger: Select backup A, press Enter (restore) or d (delete), scroll the mouse wheel to backup B while the confirmation is open, then press y. The displayed status still says `Restore backup 'A'?` / `Delete backup 'A'?`, but y reads the current mutable `backupIndex` and passes B to the operation. Mouse wheel/list selection is allowed in confirmation mode even though keyboard selection is blocked.

Impact: User confirmation for A can restore B over current files or irreversibly delete B. This is an authorization/intent mismatch on destructive actions.

Reproduction: `TestAuditBackupConfirmationTarget`: prompt=`Restore backup 'reviewed-A'? (y/n)`; command result name=`unreviewed-B`. Validation then refuses our intentionally empty catalog, so no real backup was touched. The same shared selection branch dispatches delete.

Fix direction: Snapshot the exact CatalogEntry selected when entering confirmation and execute that bound entry; block selection/navigation while confirming/running and revalidate the snapshot at execution. Add keyboard and mouse confirmation tests.

### PRODUCT-2 — P2 — Mouse navigation during backup operations loses completion and permanently blocks backup actions

Primary location: `internal/ui/screen_backups.go:253-261`; completion handling is screen-local at `88-151`, whereas keyboard input is locked by `169-171`.

Trigger: Start create, restore, or delete, click another management tab before completion, let the operation finish, then return. The mouse handler does not check `backupRunning`. `App.Update` delegates the completion to the new screen, which drops it.

Impact: `backupRunning` remains true indefinitely; all keyboard actions, including refresh, remain blocked on Backups. The operation may have changed disk state, but its result and warnings are lost. Restarting the app is required to restore the screen.

Reproduction: `TestAuditBackupRunningAllowsMouseNavigation` clicks tab 1 while `backupRunning=true`, delivers a restore completion while away, returns, and observes `backupRunning=true` and empty status. The test injects completion and does not perform a restore.

Fix direction: Centralize mutating backup completion reduction at the App level (or prohibit all navigation while running), with tests for mouse tab transitions and both success/error completions.

### PRODUCT-3 — P2 — Background read results disappear on navigation, leaving loading flags stuck

Primary locations: `internal/ui/screen_update.go:62-66,93-98`; matching patterns in `screen_backups.go:49-53,74-86` and `screen_users.go:232-235,265-277`. Entry also batches navigation and load independently in `screen_mainmenu.go:58-71` and `state_helpers.go:37-55`.

Trigger: Open Updates, leave before the async update check completes, wait, return. The result is handled only by the originating active screen. Completion on another screen is discarded, while the already-set loading guard prevents re-entry from retrying. Backups and Users have the same lifecycle pattern. Independent `tea.Batch` navigation/load commands also permit a fast load result to arrive before the first navigation even without user input.

Impact: Updates/Backups show loading indefinitely, or Users shows a stale/empty list, until the user explicitly presses refresh. Updates' loading screen does not advertise refresh. Ordinary tab navigation should not invalidate a completed query.

Reproduction: `TestAuditDroppedUpdateResult`: after query completion away and re-entry, `checking=true done=false results=0 retry=false`. Other two loaders and initial batch-order case are verified by the same source pattern, not separately executed in this probe.

Fix direction: Reduce App-owned async read results globally, with generation IDs if refreshing can overlap; initiate destination work in screen Init rather than racing navigation against it.

### PRODUCT-4 — P2 — New User silently overwrites an existing profile

Primary location: `internal/ui/screen_users.go:334-343`; persistence helper `123-144`.

Trigger: In Users press n, type an existing profile name, and press Enter. Input validates syntax only, then calls the same upsert helper used by Save, using default settings. There is no existence check or overwrite confirmation. The CLI's equivalent `addUser` explicitly asks before overwriting (`cmd/dotfiles/main.go:969+`).

Impact: Creating an already-existing user resets its saved theme/navigation/keyboard preferences without warning. The normal New User affordance unexpectedly performs an overwrite.

Reproduction: `TestAuditCreateExistingUser` creates Alice with dracula/vim, sends New Alice through the handler, executes the command in test HOME, then reloads Alice as catppuccin-mocha/emacs.

Fix direction: Separate create from save, reject an existing username on New, and make non-overwrite creation atomic where supported.

### PRODUCT-5 — P2 — Switching users does not apply the new profile to the running TUI

Primary location: `internal/ui/screen_users.go:306-312`; persisted update occurs in `internal/config/user.go:197-208`. App settings are loaded only in `app.go:486-495`.

Trigger: Start under default profile settings, switch in Users to a profile with a different theme/navigation style, then continue to Hotkeys/configuration/install in the same TUI session.

Impact: Disk global config records the new profile, but App.theme, App.navStyle, and ScreenContext still hold the previous values. Live theme and hotkey reference remain for the old profile. Subsequent installation planning explicitly copies those stale App values into planned global config (`install_plan.go:485-488`) and can revert the profile settings as part of an accepted install. Restarting the TUI is currently needed to load the switched profile.

Reproduction: `TestAuditUserSwitchInMemorySettings` executes a real switch in temp HOME and feeds its result through App.Update: `persisted=dracula/vim app=catppuccin-mocha/emacs context=catppuccin-mocha/emacs`.

Fix direction: Apply a successful switch to shared App/context state and refresh dependent caches. Capture the applied profile in the async result to avoid a second ambiguous load.

### PRODUCT-6 — P2 — Theme save errors close the picker instead of presenting the failure

Primary location: `internal/ui/screen_themepicker.go:70-80`; error is stored by `app.go:120-130` but only rendered inside the picker (`screen_themepicker.go:217-221`).

Trigger: Open the standalone theme picker and press Enter when global config is invalid or cannot be saved. `persistTheme` sets themeStatus, then the handler unconditionally quits (CLI) or navigates to MainMenu.

Impact: The selected theme is not saved, but the only error display is removed immediately. CLI exits normally; main menu can keep showing the unsaved live preview, implying success. The intended error handling is unreachable in the failed-save flow.

Reproduction: `TestAuditThemeSaveFailureQuits` uses a malformed global.json in test HOME; records `Failed to save theme: failed to parse global config ...`, followed by `tea.QuitMsg`.

Fix direction: Return an error/success value from persistence and remain on the picker after failure; only close on success.

### PRODUCT-7 — P2 — Package updates list has no viewport and overflows ordinary terminals

Primary location: `internal/ui/screen_update.go:352-386` (unbounded all-results rendering); layout return `416-420` uses Place, which pads but does not crop/scroll.

Trigger: Open Updates with more outdated packages than available screen rows. View always renders every result; cursor changes do not affect a viewport and no page/scroll input applies to the package list.

Impact: Top rows/header or bottom rows/help fall outside the terminal rendering area, depending on terminal behavior. Users cannot reliably see which package keyboard Enter/Space will act on. This occurs at standard 80x24 dimensions with a normal collection of tracked packages; it is not limited to extremely tiny terminals.

Reproduction: `TestAuditUpdateListViewport` with 35 package results and a WindowSizeMsg of 80x24 yields a view of 80x48. This is layout measurement, not a visual terminal run.

Fix direction: Calculate a height-bounded viewport that follows updateIndex, preserve header/help, and test early/late cursor positions and resize behavior.

## Additional audit observations / boundaries

- CLI support, public plan/apply, restore and uninstall have substantial explicit validation and bounded/transactional implementations; no new high-confidence issue found in their inspected entry routing. This is not a security certification of the deeper operation engine (reviewed separately by root/other agent).
- CLI legacy convenience commands (update check, backups, theme usage) often print errors and return success rather than returning RunE errors. Static examples are `cmd/dotfiles/main.go:558-580` and `609-613`; not independently reproduced here and lower priority than the confirmed user-visible findings.
- Mandatory plan rollback backups intentionally live outside the ordinary backup picker inventory, and README explicitly documents this; do not report that split as a defect.
- Existing regression tests cover many handler paths, but several only deliver async messages while their original screen is active. That does not prove lifetime correctness across navigation. These probes specifically exercise the omitted transitions.
- No real install/update, Homebrew mutation, privilege prompt, interactive TUI screenshot, Linux hardware test, or cross-platform runtime verification was performed by this sub-audit.
