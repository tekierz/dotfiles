# Comprehensive Current-State Audit — 2026-07-13

## Executive verdict

**NO-GO for a release, Homebrew recommendation, owner-hardware Apply/Save gate,
friends-and-family beta, or broader deployment.**

The current branch has unusually strong automated coverage and several carefully
designed safety boundaries. Fresh module, format, build, vet, lint, static-analysis,
vulnerability, test, race, CLI, release-configuration, cross-build, SBOM, and checksum
checks all passed locally. That evidence does not close the release:

- Debian Update All contains a statically confirmed output-channel deadlock.
- multi-manager update results are executed through one detected manager;
- npm-backed integrations are advertised but deliberately rejected by Apply;
- a release tag can publish without the full lint/security gate;
- the public Homebrew formula is stale and still performs unsafe basename deletion;
- current HEAD has no remote CI or PR evidence and is far ahead of both tracked refs;
- the tracked owner-hardware, cross-platform, distribution, and deployment gates remain open.

No production code was changed during this audit.

## Audit scope and method

The audit covered:

- repository and Git baseline;
- CLI and TUI architecture and reachability;
- install planning, Apply, execution identity, cancellation, and operation journals;
- configuration ownership, safe-file primitives, backup, restore, and uninstall;
- package-manager detection, update checking, and update execution;
- embedded utility scripts;
- support-output redaction and secret exposure;
- test coverage and the complete local automated gate;
- CI, tag release, GoReleaser, SBOM, checksum, and attestation design;
- current remote GitHub status and branch protection;
- public homebrew-tap and sshh cross-repository contracts;
- documentation, roadmap, and workflow-state consistency.

This was a source/static review plus local automation and read-only remote inspection.
It was not a substitute for the manual terminal, VM, owner-hardware, or staged deployment
matrices listed under Unverified gates.

## Baseline

| Item | Current state |
|------|---------------|
| Audited HEAD | 6cc21093d40507d8da8cd71bf21061fb298fd53a |
| Branch | release-remediation |
| Tracking branch | 74 commits ahead of origin/release-remediation, 0 behind |
| Main relationship | 161 commits ahead of origin/main, 0 behind |
| Latest tags | v2.0.2 and v2.0.1; both point to 906752872794a759ecb32406c60cb8af166fb9af |
| Distance from latest tag | current HEAD is 245 commits after v2.0.2 |
| Initial worktree | untracked tasks/pi-agent-integration-spec.md; otherwise production-clean |
| Audit documentation edits | tasks/todo.md plus this report |
| Repository inventory | 390 tracked/unignored files |
| Production Go | 57,456 lines under cmd and internal, excluding tests |
| Test Go | 61,689 lines |
| Test inventory | 234 test files, 1,588 Test functions, 4 benchmarks |
| Module toolchain | Go 1.25.6 |
| Host toolchain | Go 1.26.1 |
| Registered tools | 36 |

The current HEAD has no PR, commit status, or GitHub Actions run. The newest remote CI
success is for an older main commit, not this branch.

## Severity summary

| Severity | Count | Meaning |
|----------|------:|---------|
| Critical | 0 | No confirmed arbitrary remote compromise or uncontrolled destructive path |
| High | 5 | Release blocker, supported-platform hang/misrouting, or distribution-control failure |
| Medium | 12 | Confirmed security, integrity, product-contract, or operational defect |
| Low/design | 6 | Maintainability, coverage, or residual trust-boundary risk |

## High findings

### H1 — Debian Update All can deadlock before output is drained

internal/pkg/apt.go:399-415 starts a streamed apt update and immediately calls Wait.
The legacy streaming runner has a 100-line output channel; its scanner goroutines block
when the channel is full, and Done is not sent until those goroutines exit
(internal/runner/bash.go:128-158). Wait reads only Done (lines 94-97).

The TUI cannot begin draining until UpdateAllStreaming returns
(internal/ui/installation.go:2158-2188). Once apt update emits more than 100 lines, the
cycle is:

apt output -> full channel -> scanner blocked -> Done never closes -> Wait never returns
-> TUI never receives the stream.

Even below the deadlock threshold, the apt-update output is discarded. This is a
supported Debian/Pi operation and blocks release.

### H2 — multi-manager updates can run through the wrong manager

CheckAllUpdates intentionally queries every available manager and preserves Package.InstalledBy
(internal/pkg/update.go:18-50). AllManagers can return more than one manager
(internal/pkg/manager.go:213-228).

The TUI later calls one DetectManager, drops InstalledBy, converts all selected packages to
names, and calls that manager once (internal/ui/installation.go:1996-2027). A Linux system
with Linuxbrew plus apt or paru can therefore display an update from one manager and send it
to another. Tests cover provenance-preserving detection but not provenance-bound execution.

### H3 — tag publication bypasses the complete CI/security gate

.github/workflows/release.yml:3-6 runs on any version tag and publishes at lines 108-121.
Its source gate runs module verification, tidy, vet, tests, race, and formatting only
(lines 37-44). It omits golangci-lint/gosec, Staticcheck, govulncheck, and ShellCheck,
which live only in .github/workflows/ci.yml.

This is not closed by external policy:

- the repository has no GitHub rulesets;
- main protection requires only Lint, Test, and Build (ubuntu-latest);
- the Security job is not required;
- required checks are not strict and administrators are not enforced;
- tag creation is not constrained to a CI-verified main commit.

### H4 — public Homebrew distribution is stale and performs unsafe deletion

The authoritative homebrew-tap Formula/dotfiles.rb currently:

- targets v2.0.1 while v2.0.2 is the latest tag and current work is 245 commits newer;
- unconditionally unlinks HOMEBREW_PREFIX/bin/dotfiles-tui and dotfiles-setup by basename,
  without proving formula ownership;
- documents the invalid command dotfiles theme --list;
- tells users those legacy binaries are automatically cleaned up.

This independently confirms the open blocker in tasks/todo.md. The formula must not be
recommended until ownership-safe cleanup, metadata, commands, install, upgrade, rollback,
and uninstall behavior are verified.

The sshh formula also uses the mutable refs/heads/main.tar.gz URL with a fixed hash. It will
break when sshh/main moves and violates the project's immutable release-source requirement.

### H5 — advertised npm-backed integrations are deliberately non-executable

docs/tools.md:76-82 says Codex, OpenCode, and Pi are available through the reviewed
installer and describes review before execution. The shared executor pre-scans recipes and
returns ErrNPMExecutionAuthorityRequired for every npm step before mutation
(internal/installapply/recipe.go:25-27,49-66,77-78,106-107).

The tracked roadmap correctly says npm execution is disabled and public phase projection is
unfinished (tasks/todo.md:98-103). The safety behavior is appropriately fail-closed, but the
public product claim is false and the integrations are not currently usable end to end.

## Medium findings

### M1 — sshh permits SSH option and ProxyCommand injection

The embedded sshh add path rejects only pipe and newline characters
(internal/scripts/scripts.go:337-355). Connect runs ssh with the saved connection as the
first argument and no end-of-options boundary (lines 324-330 and 371-380).

A saved connection beginning with -oProxyCommand= is still parsed by ssh as an option even
though it is one quoted argv. Host syntax, leading dash/control characters, and port range
must be validated. This is especially important because the same helper accepts new entries.

### M2 — manual restore validates one catalog object and restores another pathname

Catalog listing captures and validates exact directory snapshot/parent authority
(internal/backup/catalog.go:92-114). CLI and TUI validate that CatalogEntry, then call
backup.Restore with a mutable path (cmd/dotfiles/main.go:683-695 and
internal/ui/app.go:669-672). Restore reopens the path rather than consuming the accepted
catalog authority.

A same-user concurrent replacement can therefore restore different, still structurally valid
bytes than the user reviewed. Symlink/traversal defenses remain strong, so this is an
integrity/authority gap rather than privilege escalation.

### M3 — cancellation does not reliably terminate process trees or config-phase Git work

Legacy streaming uses exec.CommandContext without a process group
(internal/runner/bash.go:101-106); Cancel stops the direct child only. Package-manager,
sudo, maintainer-script, or hook descendants may survive.

Installer config actions do not consistently check the install context, while tmux TPM and
Neovim clone operations create Background-based timeouts instead of using the install
context (internal/tools/tmux.go:193-210 and internal/tools/neovim.go:465-483). The exact npm
runner already demonstrates process-group-safe handling, but the general installer path does
not yet meet the TUI's no-orphan claim.

### M4 — remote configuration payloads remain mutable and unpinned

TPM and Neovim presets shallow-clone repository HEAD. The reviewed plan binds desired local
configuration state, not downloaded bytes. Those repositories can supply executable editor
or plugin code after review.

Default Claude MCP entries likewise use unversioned npx packages, with Convex explicitly at
latest (internal/config/claude.go:37-75). Context7 is enabled/recommended by default and the
resulting command executes later outside the reviewed package executor.

### M5 — invalid CLI grammar and failures commonly exit zero

Runtime checks against the fresh binary confirmed exit 0 for:

- invalid theme syntax;
- extra arguments to theme list;
- missing config target;
- extra arguments to backups;
- a nonexistent quick-switch user.

Source review also shows update garbage launches the TUI and update-check/backup-read helpers
cannot propagate failures through Cobra. Public noninteractive commands need explicit Args
contracts and deterministic nonzero exits.

### M6 — sshh add cannot bootstrap its own configuration

The script exits before dispatch when ~/.sshh is absent or contains no valid hosts
(internal/scripts/scripts.go:277-303). The advertised add implementation appears later
(lines 337-358), so it cannot create the first entry. Go coverage reports the scripts package
at 100%, but tests validate embedded bytes/dispatch rather than executing this behavior.

### M7 — uninstall restore can ignore a valid recovery point

restoreLatestUninstallBackup chooses the lexicographically greatest raw directory, not the
newest valid catalog entry (cmd/dotfiles/main.go:801-850). If the newest-looking directory is
partial or invalid, restore stops there and never tries an older valid backup. Deletion is
still disabled, so this fails closed, but recovery does not behave as promised.

### M8 — README, updater ownership, and registry describe different products

README.md:103-132 advertises fastfetch, macmon, ncdu, duf, dust, bandwhich, gping, doggo, and
trippy as installed/configured tools. They are absent from the 36-tool registry
(internal/tools/registry.go:48-95). internal/pkg/update.go retains a third legacy managed
package list containing several of them.

The runtime registry is documented as the source of truth, so installation claims and updater
ownership must be generated from or reconciled with it.

### M9 — Deep Dive mouse navigation cannot reach Continue

Keyboard navigation allows the synthetic Continue index at len(menuItems)
(internal/ui/screen_deepdivemenu.go:78-90). Mouse-wheel navigation stops at
len(items)-1 (lines 109-120). The full list is rendered without a viewport, so small terminal
overflow compounds the mouse reachability problem.

### M10 — the mandatory remediation state machine is stale and weakly enforced

tasks/current-slice.scope still identifies npm-private-phase-authority as contract-frozen,
while HEAD is the implementation commit Security - bind npm identity to private plans.
tasks/todo.md also calls that slice the next work.

scripts/check-slice-scope.sh validates that state is an allowed word but does not require a
commit candidate to be verified/committed. The machine check can therefore pass while the
tracked state is demonstrably pre-implementation.

### M11 — release tooling silently escapes the pinned Go toolchain

The module and setup-go pin Go 1.25.6. GoReleaser v2.17.0 requires Go 1.26.4 and Syft
v1.44.0 requires Go 1.25.8. A strict Go 1.25.6 install fails; default GOTOOLCHAIN=auto
silently downloads a newer compiler.

The application gates do pass under exact Go 1.25.6, but release tooling needs its own
explicit toolchain pin or compatible tool versions so the release is reproducible by policy,
not by an implicit download.

### M12 — some legacy readers and streams have boundedness/error gaps

Legacy streaming ignores scanner.Err after its 1 MiB line limit and can truncate output while
reporting only the process exit. Several tool/profile/hotkey config reads and backup manifest
reads use unbounded os.ReadFile/io.ReadAll paths. These are primarily same-user availability
risks, but they are inconsistent with the bounded, descriptor-anchored public-support and
operation-journal code.

## Low/design findings

1. Claude Code tool metadata points at ~/.claude/settings.json while the MCP writer correctly
   operates on ~/.claude.json.
2. CLAUDE.md and nested AGENTS.md files contain stale tool counts, line counts, removed async
   messages, obsolete config APIs/files, and outdated screen-addition guidance.
3. ScreenContext still exposes the complete mutable App, so the screen migration improved
   dispatch without fully isolating state or effects.
4. Tool-to-screen routing uses raw integer screen IDs across packages. Runtime parity tests
   catch drift, but the contract remains brittle.
5. Theme names/defaults exist in three packages; internal/theme has 0% direct coverage.
6. Running ordinary and race suites concurrently caused two runner timing tests to miss their
   two-second deadlines once. Sequential CI-parity runs and 20 focused repetitions passed, so
   this is stress sensitivity, not a confirmed gate failure.

## Verification results

### Passed against current source

| Gate | Result |
|------|--------|
| go mod verify | Pass |
| go mod tidy -diff | Pass |
| gofmt -l | Pass, no files |
| go vet ./... | Pass |
| go build / make build | Pass |
| go test ./... | Pass sequentially |
| go test -race ./... | Pass |
| Exact Go 1.25.6 build/vet/test/race | Pass |
| internal/runner repeated 20 times | Pass |
| golangci-lint 2.5.0 | Pass, 0 issues |
| Staticcheck 0.7.0 | Pass |
| govulncheck 1.1.4 | Pass, 0 reachable vulnerabilities |
| ShellCheck 0.11.0 | Pass |
| hardcoded-secret/private-key scan | No tracked secret match |
| CLI help/status/backups/theme/version smoke | Pass |
| GoReleaser 2.17.0 config | Pass |
| isolated current-HEAD release snapshot | Pass |
| 4 platform archives + source archive | Pass |
| 5 SPDX SBOMs | Pass |
| artifact checksums | Pass |
| current Darwin arm64 snapshot version | Pass: 0.0.1-next |

Govulncheck also reported 4 advisories in imported packages and 17 in required modules, but
no called vulnerable symbols.

### Coverage

| Package | Statements |
|---------|-----------:|
| cmd/dotfiles | 68.1% |
| internal/backup | 78.3% |
| internal/config | 74.9% |
| internal/health | 93.1% |
| internal/hotkeys | 100.0% |
| internal/installapply | 82.0% |
| internal/installplan | 91.2% |
| internal/operation | 80.7% |
| internal/pkg | 62.1% |
| internal/planpublic | 88.7% |
| internal/runner | 79.4% |
| internal/safefile | 71.8% |
| internal/scripts | 100.0% Go coverage, embedded shell behavior not exercised |
| internal/theme | 0.0% |
| internal/tools | 79.3% |
| internal/ui | 75.1% |

## Strengths

- Plan/apply is explicit, hash-bound, freshly replanned, detector-revalidated, locked, and
  journaled.
- Install recipes are typed and do not admit a generic remote-script/shell step.
- npm execution fails closed before prerequisite mutation.
- Manager and npm executable identity observations are path-private and drift-detecting, with
  remaining spawn/transitive boundaries documented rather than overstated.
- Safe-file writes, plan backups, and automatic rollback use descriptor anchoring, no-follow
  operations, exact revisions, parent identity, ownership/mode checks, and atomic replacement.
- Uninstall automatic deletion is deliberately disabled.
- Support JSON uses an allowlist projection, bounded reads/records/size, generic errors, and
  one-document output; it neither writes nor uploads.
- No tracked secret/private-key material was found.
- CI runs tests and race checks on both Ubuntu and macOS.
- Release artifacts are static cross-platform builds with source archive, SBOMs, checksum
  verification, attestations, and draft-until-verified publication.
- Screen factory coverage is exhaustive and fails loudly on unmapped screens.
- Test volume and adversarial coverage around plan, filesystem, support, health, and authority
  boundaries are materially stronger than a typical dotfiles project.

## Unverified gates

The following must not be inferred from local automated success:

- manual installer, Manage, Hotkeys, mouse, keyboard, animation, aesthetic, glyph, and
  40x14/60x18/80x24/120x40 terminal review;
- Debian/Ubuntu, Arch/paru, Raspberry Pi, Intel macOS, and non-Ghostty terminal behavior;
- Debian apt Update All after correction;
- package mutation, sudo, cancellation, and process-tree behavior on disposable systems;
- owner-hardware install/apply and Glow path checks;
- Homebrew clean install, upgrade, rollback, uninstall, ownership migration, and formula hash;
- local sibling-checkout compatibility (the sibling repos were absent; remote source only was
  inspected);
- macOS signing/notarization and published artifact/attestation verification;
- friends/family and mock-enterprise deployment gates;
- current-HEAD GitHub Actions, because the branch is not pushed and has no PR/run.

## Recommended remediation order

1. Fix and regression-test the Debian Update All deadlock, including high-volume output and
   cancellation.
2. Bind every update action to InstalledBy and test mixed-manager hosts.
3. Make release publication consume the complete CI/security gate and constrain tag source.
4. Repair homebrew-tap ownership cleanup, version/commands, immutable sshh source, and formula
   install/upgrade/rollback tests.
5. Either complete npm phase authority/projection or label Codex/OpenCode/Pi as blocked on all
   public surfaces.
6. Close sshh option validation, restore authority, and process-tree cancellation gaps.
7. Reconcile README/updater/registry contracts and enforce deterministic CLI grammar/exits.
8. Advance and enforce the remediation state machine, then run current-HEAD remote CI.
9. Complete the manual platform, owner-hardware, Homebrew, and staged deployment gates.

Only after those steps should the project reconsider a release or beta verdict.
