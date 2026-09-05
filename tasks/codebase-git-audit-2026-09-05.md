# Codebase and Git repository audit — 2026-09-05

**Verdict: this checkout is not ready for release or unrestricted install/update/restore use.** Its automated baseline is substantially stronger than the public default branch, but confirmed intent, lifecycle, recovery, and distribution defects remain. Passing tests do not cover the failing interaction sequences reproduced below. Do not restart the remediation from scratch: a newer local product branch already contains many relevant fixes.

This is an audit, not a remediation or release approval. No production source, dependency files, Git refs, index, commits, hooks, remote settings, or installed tools were changed. Only this report, its evidence directory, and the audit section of `tasks/todo.md` were written. Pre-existing changes to `tasks/lessons.md` and untracked `tasks/pi-agent-integration-spec.md` were preserved.

## Scope and exact baseline

| Item | Observed state |
|---|---|
| Primary checkout | `release-remediation`, `d77c0f790a3694df21dfc8e784eef66dfb2614b2` |
| Last checkout commit | July 14, 2026, “Stable - close G3F todo policy projection” |
| Baseline inventory | 413 tracked paths; 348 Go files: 165 production, 183 tests; 57,658 production Go lines |
| Public default branch | `main`, `bb3cbf9e60573ecb1dbd0c375e1e6c55a260864f`, confirmed through GitHub API |
| Local divergence | Checkout is 278 commits ahead of remote `main`, 96 ahead of remote `release-remediation`; local `main` is one commit behind remote `main` |
| Newer product branch | `feature/local-dogfood-product-2`, `890c446`, contains 35 commits after this checkout |
| Release preparation worktree | `feature/open-source-release-readiness` also starts at `890c446`; 34 tracked modifications and 10 untracked status entries |
| Repository footprint | 65 local branches, 37 worktrees, 23 stashes; ~31.28 MiB loose objects plus ~14.47 MiB packed objects |
| Host / toolchains | macOS arm64; default Go 1.27.1; authoritative checks rerun with module-pinned Go 1.25.6 |

All tracked paths were inventoried. Three independent reviews inspected execution/operation safety, backup/configuration/filesystem boundaries, and CLI/TUI behavior, while the primary review covered Git, automation, release/distribution, documentation, dependencies, and verification. Review concentrated on reachable and security-sensitive paths; this is **not** a claim of exhaustive line-by-line review of every test/document or every possible filesystem interleaving. Other worktrees were inventoried and compared selectively, not fully audited or tested. The uncommitted release-preparation candidate requires its own verification.

Evidence: [inventory](audit-work/2026-09-05/tracked-inventory.tsv), [execution review](audit-work/2026-09-05/execution-review.md), [recovery review](audit-work/2026-09-05/recovery-review.md), [product review](audit-work/2026-09-05/product-review.md), [remote evidence](audit-work/2026-09-05/remote-evidence.json).

## Prioritized findings

Priorities describe impact and remediation order, not CVSS scores. P1 means high-impact correctness or release blocker; P2 means material correctness, reliability, or hardening work; P3 means latent or lower-impact work. No critical remote exploit was established.

### P1-1 — Backup confirmation can restore or delete a different backup

**Source:** `internal/ui/screen_backups.go:174`, `:219`, `:253`.

Select A, press Enter or `d`, scroll to B while the confirmation remains open, then press `y`. The prompt still names A, but execution reads the mutable `backupIndex` and uses B. This can overwrite current configuration from an unintended backup or delete the wrong recovery copy. The handler probe confirmed prompt `reviewed-A` and execution target `unreviewed-B`; deliberately invalid catalog authority prevented actual restore/delete in the reproduction.

**Action:** freeze the exact name/catalog authority when confirmation begins and execute that snapshot. This is already addressed in source on `890c446` (`screen_backups.go:187`, `:190`, `:227`); integrate and verify that fix rather than reimplementing it here.

### P1-2 — Updates discard package-manager ownership

**Source:** `internal/ui/installation.go:1996`, `:2002`, `:2026`; `internal/pkg/update.go:28`.

Update discovery preserves each package's `InstalledBy`, but execution reduces selected records to names and re-detects one default manager. On a host with Linuxbrew and apt/paru, a package shown from one provider can be updated through another. The intended copy remains outdated while another copy changes, potentially with a success result. Both selected updates and the UI's `a` action reach this path. Confirmed by source tracing; no real upgrades were run.

**Action:** retain provider identity through confirmation, privilege handling, execution, and postchecks. The newer product branch contains provider binding/routing/surface commits; review those candidates first.

### P1-3 — Tag publication bypasses important quality gates

**Source:** `.github/workflows/release.yml:37`, `:46`, `:58`; `.github/workflows/ci.yml:3`, `:40`, `:50`, `:69`.

The tag workflow runs module checks, vet, tests/race, and formatting, but omits golangci-lint, Staticcheck, govulncheck, ShellCheck, and the macOS runtime test job. Normal CI only triggers for main/master pushes and pull requests; a tag can publish without those omitted checks passing for its commit. This matters now: the exact pinned vulnerability gate fails in this audit.

**Action:** share one complete quality-gate workflow across PR/main/tag and make publication depend on it. Keep release-tool and product-build toolchains explicit; do not infer product toolchain upgrades from installing a newer release tool. Preserve the existing draft/checksum/SBOM/attestation safeguards.

### P1-4 — Remote branch protection expects a test job this candidate no longer emits

**Evidence:** live GitHub protection API on September 5; `.github/workflows/ci.yml:75`.

Required checks are `Lint`, `Test`, and `Build (ubuntu-latest)`. This checkout emits `Test (ubuntu-latest)` and `Test (macos-latest)`, not `Test`. A PR adopting this workflow can remain blocked on a nonexistent required context. Security and macOS build/test are not included in the required set; strict up-to-date checking and administrator enforcement are disabled. The old public-main workflow still has the singular `Test`, so the name mismatch is specifically a candidate-integration defect, not proof that current main PRs already hang.

**Action:** reconcile exact emitted check names with protection in the same integration rollout, preferably using a stable aggregate required job. Remote settings were inspected only; nothing was changed.

### P1-5 — Homebrew still distributes old code and deletes binaries by basename

**Evidence:** current [external formula](https://github.com/tekierz/homebrew-tap/blob/main/Formula/dotfiles.rb), saved [formula snapshot](audit-work/2026-09-05/homebrew-formula.rb.txt).

The formula still builds tag `v2.0.1`. Its install method unconditionally unlinks existing `dotfiles-tui` and `dotfiles-setup` paths under the Homebrew prefix without proving ownership. A matching unrelated executable can be removed. Its caveat also advertises obsolete `theme --list` syntax. Current local safety work is not what `brew install` delivers.

**Action:** remove unowned deletion, publish/verify the chosen candidate, update immutable source/checksum and docs, then exercise clean install, upgrade, and uninstall. The configured SHA256 was observed, not independently verified against a downloaded tarball. No Homebrew installation was performed.

### P2-1 — Cancellation can return while package-manager descendants continue mutating

**Source:** `internal/runner/bash.go:101`, `:135`; `internal/installapply/recipe.go:157`; `internal/installapply/service.go:220`.

The live general runner kills only the direct child. A spawned helper can retain output pipes, continue writing, and delay `Done`; the apply wrapper can return cancellation and release its operation lock first. A fake child wrote a marker after cancellation, while completion initially remained blocked. This is separate from the exact npm runner's stronger process-group behavior.

**Action:** adopt bounded process-tree teardown and wait for terminal process state before releasing serialization. The newer branch includes lifecycle/adaptor/privileged-supervisor work; verify its complete cancellation boundary.

### P2-2 — Manual restore drops the selected catalog's source authority

**Source:** `internal/backup/backup.go:651`; `internal/ui/app.go:669`; `cmd/dotfiles/main.go:683`.

Callers validate a catalog entry, then pass only a pathname to `Restore`, which reads manifest/payloads again independently. A synchronization process or another same-user writer can change a later payload during restore and have those changed bytes accepted. A deterministic two-file probe changed the second source during the first destination write: restore succeeded with changed content, even though the original catalog entry correctly rejected the change afterward.

**Action:** consume opaque catalog authority throughout restore. This is an integrity/selection defect, not a demonstrated privilege escalation. Stronger plan rollback already exists, and the newer branch contains catalog-authorized manual restore work.

### P2-3 — Listing backups retains all backup contents in memory

**Source:** `internal/backup/catalog.go:63`, `:83`; `internal/safefile/directory_unix.go:222`.

Catalog creation snapshots every backup recursively, checks a second snapshot, and retains full payloads in returned authority objects. Restoring one backup or running retention first loads all backups. Four 4 MiB payloads retained 16,779,880 extra heap bytes after GC in the probe. Memory exhaustion is workload-dependent, but growth is unbounded and particularly problematic for Raspberry Pi users.

**Action:** bound catalog metadata and hashing, capture selected payloads on demand, and impose explicit budgets where content snapshots remain necessary. `890c446` is a bounded-backup-catalog candidate; review and test it rather than duplicating it.

### P2-4 — APT receipts confuse desired selection with actual installed state

**Source:** `internal/pkg/apt.go:325`, `:112`, `:134`; `internal/tools/installation_health.go:144`.

`dpkg --get-selections` indicates desired action. The batch list accepts `install` even when a package is only unpacked, and omits installed packages marked `hold`. Individual status checks also reject `hold ok installed`. Fake dpkg fixtures reproduced both false negatives for held packages and false positives for unpacked ones. The fresh installation collector treats the batch result as authoritative, so plans can skip broken installs or offer unnecessary installs.

**Action:** use one batch `dpkg-query` over actual status/error state and preserve hold policy. [Debian's dpkg documentation](https://manpages.debian.org/trixie/dpkg/dpkg.1.en.html) distinguishes selection and package states. Actual Debian package mutation was not run.

### P2-5 — Navigation loses async results and can strand backup operations

**Source:** `internal/ui/screen_backups.go:88`, `:169`, `:253`; `screen_update.go:62`, `:93`; `screen_users.go:232`, `:265`; `screen_mainmenu.go:58`.

Read and backup-operation completions are handled only by the currently active screen. Leaving Updates before its result arrives leaves `updateChecking=true`, no result, and no retry on re-entry. Backups and Users have the same read-result pattern. More seriously, backup keyboard navigation is blocked while running but mouse tab navigation is allowed; completion is then dropped and `backupRunning` remains true, preventing further keyboard actions even after returning. Independent batched navigation/load commands also permit arrival-order races.

**Action:** reduce App-owned completions globally with operation/generation identity, and make modal mouse/keyboard behavior consistent. Two distinct handler probes confirmed lost update results and stranded backup state. Both remain unresolved in the newer branch by source comparison.

### P2-6 — New User silently overwrites an existing profile

**Source:** `internal/ui/screen_users.go:334`, `:123`.

The New flow checks name syntax, then invokes an upsert helper with default settings. Creating an existing Alice changed its saved `dracula/vim` settings to `catppuccin-mocha/emacs` in a temporary-home reproduction. There is no warning; the CLI's add flow at least asks about overwrite.

**Action:** separate create from save and reject existing names atomically. Remains unresolved on `890c446`.

### P2-7 — Switching profiles leaves the running App's settings stale

**Source:** `internal/ui/screen_users.go:306`; `internal/config/user.go:197`; `internal/ui/install_plan.go:485`.

Switch persists the new profile but only reloads the user list. App/context theme and navigation retain old values. The probe observed disk `dracula/vim` with App/context still `catppuccin-mocha/emacs`. A later installer plan copies those stale values into global configuration.

**Action:** return and apply the switched profile to shared state and dependent caches. Remains unresolved on `890c446`.

### P2-8 — Theme save errors close the only screen displaying the error

**Source:** `internal/ui/screen_themepicker.go:70`; `internal/ui/app.go:120`.

Persistence stores a failure in `themeStatus`, then Enter unconditionally quits the standalone picker or navigates away. A malformed-global-config probe produced both “Failed to save theme” state and `tea.QuitMsg`. The selected value was not saved, but failure is not presented.

**Action:** have persistence return success/error and remain in the picker on failure. Remains unresolved on `890c446`.

### P2-9 — Updates has no height-bounded package viewport

**Source:** `internal/ui/screen_update.go:352`, `:416`.

Every result is rendered; cursor movement does not scroll a bounded window. With 35 package results, the view measured 80×48 in an 80×24 terminal. Selection and help can be offscreen while actions remain enabled. This was a handler/layout measurement, not a terminal screenshot.

**Action:** reserve fixed header/help space and implement a cursor-following viewport. Remains unresolved on `890c446`.

### P2-10 — Advertised npm integrations remain deliberately blocked

**Source:** `internal/installapply/recipe.go:49`, `:63`; `docs/tools.md:76`.

Docs describe Codex and Pi as available install-only integrations, but the shared executor rejects every recipe containing `InstallStepNPMGlobal` before mutation. This is intentional fail-closed behavior, not a bypass vulnerability. Selectability and recipe presence therefore do not establish working installation.

**Action:** either expose truthful blocked status or integrate the reviewed fresh, separately confirmed npm phases. The newer branch already contains phase coordination, execution, CLI/wizard, and integration-truth commits. Do not remove the guard independently.

### P2-11 — The pinned toolchain fails current vulnerability scanning

**Source:** `go.mod:3`; [scanner output](audit-work/2026-09-05/govulncheck-pinned.log.txt).

Exact Go 1.25.6 scanning exits 3 for **GO-2026-4602 / CVE-2026-27139**, with symbol traces through `safefile.listDirectoryStates` and `Journal.TerminalBackupPaths`. The official advisory limits the issue to metadata access escaping an `os.Root`; it does **not** permit file-content reads/writes outside that root. No production `os.OpenRoot` use was found here, so a project-specific exploit was not established. The scan nevertheless fails the current security gate. Fixes first appeared in Go 1.25.8 and 1.26.1. [Official advisory](https://pkg.go.dev/vuln/GO-2026-4602).

**Action:** choose a maintained, scanner-compatible patched toolchain and rerun the exact candidate gates. Do not cite July's clean scan as current evidence. The scan also listed unreachable imported/module vulnerabilities; those are not counted as additional proven application vulnerabilities.

### P2-12 — Remote tool/MCP code is not fully bound to immutable revisions

**Source:** `internal/config/claude.go:43`, `:63`; `internal/tools/neovim.go:480`; `internal/tools/tmux.go:205`.

MCP configs use unversioned npm selectors or `@latest`, while Neovim presets and TPM clone moving default-branch tips. A stable user selection therefore need not resolve to the same code at execution or later MCP startup. The allowlisted repository URL controls origin selection, not immutable content. No compromised upstream was alleged or executed.

**Action:** bind reviewed versions/commit identities and disclose unavoidable dynamic dependency behavior. The newer branch contains pinned MCP and artifact-authority commits; review them first.

### P3-1 — APT manager-wide streaming has a latent undrained-channel deadlock

**Source:** `internal/pkg/apt.go:406`; `internal/runner/bash.go:94`, `:141`.

`UpdateAllStreaming` waits for the preliminary update command without consuming its 100-line channel. A fake apt producing 120 lines blocked until context cancellation. **Correction to the July 13 audit:** both current Updates handlers pass `all=false`; no live `true` caller was found. The defect remains in the API but must not be presented as a reproduced current-dashboard Update All freeze.

**Action:** remove the dead API or sequence/drain its phases before reusing it. The newer branch contains apt phase sequencing work.

## Git, distribution, and maintainability assessment

The Git object database is structurally healthy: `git fsck --full --no-reflogs` returned successfully with 131 dangling-object notices and no other diagnostics. Dangling objects and stashes are recovery/history artifacts, not corruption; do not prune them during integration. Six worktrees had changes, including this checkout after the audit plan update. Several otherwise clean product worktrees only held untracked build outputs; the old `.claude/worktrees/audit-remediation` tree held 26 status entries.

The central repository risk is fragmented delivery. GitHub reports no open PRs and no releases. Its latest visible CI run is [July 4 main CI](https://github.com/tekierz/dotfiles/actions/runs/28698640818), for `bb3cbf9`, not this checkout or `890c446`. Remote `release-remediation` still points to `b30b9c6`. The two remote tags, `v2.0.1` and `v2.0.2`, point to the same commit `9067528`; version numbers alone do not distinguish source changes. The public-main workflow still makes lint/staticcheck/govulncheck nonblocking, unlike this remediation candidate.

The 35 newer product commits include runner lifecycle/supervision, opaque restore, bounded reads/catalogs, update routing, npm phases, pinned artifacts/MCPs, sshh hardening, and Deep Dive viewport work. The separate release-preparation worktree already contains CI/release, secret scanning, CodeQL/dependency review, licensing/policy/docs, and fixture changes. These are existing candidates, **not accepted fixes or verified remote delivery**. Inventory and reconcile them before adding duplicate implementation branches. Do not delete any worktree or stash until unique work is accounted for.

Architecture has useful separation now: headless planning/apply, public projections, operation journals/locks, filesystem authority, health collection, and native config import are explicit packages. The main remaining cohesion issue is state and lifecycle ownership across TUI handlers. `installation.go` is 2,256 lines, `manage_dualpane.go` 1,560, `install_plan.go` 1,500, and `app.go` 1,349. File size is not itself a defect, but the confirmed stale-state and dropped-message failures identify concrete extraction boundaries: shared async reducers, profile application, and provider-preserving execution.

Other lower-priority observations:

- Tests under default Go 1.27.1 failed doctor-repair fixture assumptions and npm fixture behavior; installed lint/scanner tools could not read that toolchain's format/language. Pinned Go 1.25.6 passed after the sandbox-only runner restriction was removed. Distinguish supported-toolchain policy from broad Go-version claims.
- Failure diagnostics in `doctor_repair_test.go:22` format a struct containing full binary bytes; the initial failed run produced an approximately 193 MB log. `npm_execution_identity_start_test.go:256` can print a fixture's inherited environment on failure. Use bounded field-level diagnostics and minimal fixture environments; saved audit evidence excludes environment values and binary dumps.
- Unvalidated manually edited Manage enums can flow into tmux/Lua syntax. Normal UI inputs are constrained; no lower-trust route was established, so this is input-validation hardening, not a separate code-execution finding.
- `scripts/install-hooks.sh` assumes `.git` is a directory and therefore does not support linked worktrees; lint/vulnerability hook checks are only warnings. CI must remain authoritative. No hooks were installed by this audit.
- Root agent docs retain stale size/tool-count descriptions and historical paths; `LinuxLocalTesting/Notes` contains old implementation claims without a clear archival boundary. `.golangci.bck.yml` is still tracked. Reconcile active instructions/docs after selecting the product baseline.
- A five-family credential-pattern scan found no matches in 2,689 text blobs reachable from local refs/stashes (45,457,784 bytes). It skipped 23 blobs over 2 MB and did not scan ignored/unreachable content, entropy, or custom secret formats. This is useful negative evidence, **not** a comprehensive credential-clearance claim. [Scan scope](audit-work/2026-09-05/secret-pattern-scan.json).

## Verification results and limits

| Check | Current result |
|---|---|
| Module download integrity; `go mod tidy -diff` | Pass |
| Pinned build and `go vet ./...` | Pass |
| gofmt; ShellCheck on `scripts/install-hooks.sh` | Pass |
| golangci-lint 2.5.0; Staticcheck 0.7.0 with Go 1.25.6 | Pass |
| `go test ./...`; `go test -race ./...`, pinned toolchain | Pass after sandbox-restricted runner test was rerun outside sandbox; other same-session passing package results reused from cache |
| Exact pinned govulncheck 1.1.4 | **Fail: one standard-library symbol-level advisory; applicability limit above** |
| Static cross-builds | Pass: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 |
| Isolated CLI smoke | Help/version/theme list/backups/status JSON succeed; support emits valid partial JSON with exit 2 in minimal environment; missing plan intent emits JSON/2; invalid apply and unknown-tool plan fail without mutation |
| Defect probes | Seven product handler assertions reproduce incorrect behavior; two backup probes and three fake-process/package probes reproduce their stated defects |
| Git integrity / whitespace | fsck and diff whitespace checks pass |
| Live GitHub | Metadata, refs, tags, protection, rulesets, CI history, PRs/releases, and external Homebrew formula read successfully |

Saved [verification manifest](audit-work/2026-09-05/verification.json), [test output](audit-work/2026-09-05/tests-unsandboxed.log.txt), [race output](audit-work/2026-09-05/race-unsandboxed.log.txt), and [CLI results](audit-work/2026-09-05/cli-smoke.json) distinguish initial environment failures from final gates. Audit probes were injected using Go overlays or copied source in temporary directories; no production/test files were added to the compiled tree. See [probe instructions](audit-work/2026-09-05/README.md).

Not performed: actual package installation/update, destructive owner-account restore, privilege interaction, interactive terminal screenshots/mouse hardware checks, Debian/Arch/Raspberry Pi runtime tests, Homebrew lifecycle/hash-download validation, full GoReleaser/Syft snapshot, published artifact/attestation verification, machine-enforced slice-transition shell tests, or a fresh full audit of `890c446` plus its uncommitted release changes. GoReleaser/Syft were unavailable on PATH. Sibling `homebrew-tap` and `sshh` checkouts were absent, so compatibility evidence is limited to the remote formula and embedded utility/source review. Cross-compilation is not Linux runtime or release-package verification.

## Recommended remediation order

1. **Choose and reconcile the intended integration candidate.** Compare `d77c0f7`, `890c446`, and the uncommitted release-preparation tree. Preserve unique changes and recovery artifacts; update the active plan to reflect actual branch-local versus integrated work. This audit authorizes no merges, commits, pushes, or cleanup.
2. **Adopt and verify existing safety candidates.** Prioritize bound backup confirmation/restore, bounded catalogs, process lifecycle/supervision, and provider-aware updates. Run the preserved probes against the selected combined tree.
3. **Fix the remaining reproduced product defects.** Centralize async result handling, protect New User, apply switched profile state, keep theme errors visible, and bound Updates layout. Repair APT receipt semantics. Reconcile npm and artifact behavior with user-facing claims.
4. **Close exact-candidate automation and distribution gates.** Select a patched compatible toolchain; align required checks and tag gates; validate the existing release-preparation changes; remove unowned formula deletion; obtain a current PR/CI baseline before promotion.
5. **Run disposable-system and release rehearsals.** Exercise interrupted installs, cancellation, restore/drift, user switching, terminal resizing, Homebrew upgrade/uninstall, and real Linux manager behavior. Verify the actual signed/attested/checksummed artifacts and owner-hardware behavior before publishing.

The report is complete as an evidence-bounded audit. Release readiness remains blocked by the findings and unexecuted deployment gates above.
