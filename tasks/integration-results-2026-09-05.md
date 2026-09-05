# September integration results

The integration program combines the 35 newer product commits, the saved release-preparation work, and bounded audit repairs on `release-remediation`, through the reviewed `integration/2026-09-05-candidate` branch. The original audit describes `d77c0f7`; use the [current finding disposition](audit-work/2026-09-05/finding-disposition.md) alongside it. The validated product source is `f7ff93feddbdf616b9b44784ba95a90159b7d0be`; subsequent documentation records the evidence.

## What was saved

All six dirty worktrees were preserved before changing integration source. Three contained source/documentation and three contained generated binaries only. The [dirty-worktree analysis](dirty-worktree-analysis-2026-09-05.md) explains what was useful, superseded, unsafe to import, or generated. Original worktrees and all 23 stashes remain intact.

97 explicitly dated `archive/2026-09-05/…` refs were published without force; every remote SHA matched the manifest on recheck. Private recovery in `.git/recovery/2026-09-05-integration/` also contains a verified Git bundle, original patches, working-file archives and build outputs. Generated binaries are recovery evidence, not accepted source or release artifacts.

The historical audit tree was not merged wholesale: its pathname deletion approach would regress modern filesystem authority. Relative-HOME validation, live Manage context, first-frame routing and the repository-designated AUR capability rule were adapted into the current implementation with regression tests.

## What changed

- Backup restore/delete confirmation captures immutable catalog authority; listing retains bounded metadata and captures payloads only for selected operations.
- APT receipt queries use actual installed/error state, preserve held installed packages and multiarch identity, and support cancellation.
- Install and Update cancellation waits for terminal cleanup; provider identity stays attached to discovery, selection and execution. APT phases drain and revalidate captured executables before continuing.
- App-owned async generations prevent stale or duplicated Updates, Backups and Users results from corrupting current state.
- New User is create-only; explicit Save remains separate. Switching profiles updates live App/context/cache state. Failed theme saves remain visible and retryable.
- Updates follow the cursor within available height. CLI start screens render correctly and initialize their final handler exactly once.
- Go 1.26.8 and compatible pinned analyzers replace the vulnerable baseline. Tags depend on the complete reusable CI gate; stable aggregate check names match the existing main protection contexts.
- Repository and global instructions require `gpt-6-astra` for every subagent, with reasoning adjusted to task complexity.

See the 18-row disposition for exact commits, source/tests and practical limits. A fix in this branch does not mean users have received it through Homebrew.

## Verification record

Final source passed local Go 1.26.8 normal/race suites, Darwin/Linux lint, vet and Staticcheck. Module integrity/tidiness, workflow validation, ShellCheck and source secret scanning also passed. Refreshed govulncheck reports no reachable vulnerabilities; one module advisory affects code not called by this program. [Final-source CI run 33989632246](https://github.com/tekierz/dotfiles/actions/runs/33989632246) passed every job, including Release Gate. [CodeQL](https://github.com/tekierz/dotfiles/actions/runs/33989634951) also passed. The final privileged Linux harness executed all eight cases successfully in 4.17s.

[Linux/macOS CI run 33988814713](https://github.com/tekierz/dotfiles/actions/runs/33988814713) passed both platforms' normal/race suites and builds, Security, and the actual isolated eight-case privileged supervisor harness. Its lint failures were platform conversion/input analyzer findings subsequently repaired and verified with Linux and Darwin lint. The harness marker `DOTFILES_PRIVILEGED_CI_EXECUTED cases=8` proves execution, not a skipped test. The subsequent supervisor ordering/deadline findings were fixed in f7ff93f and accepted by that final-source run.

Initial failed test runs are retained as evidence: unsuitable temporary-directory ownership was corrected in the local harness; inherited XDG/Yazi and host npm/node assumptions were reproduced and fixed in tests. Product authority checks were retained.

## Remaining release decisions

1. Adopt the prepared [Homebrew ownership repair](audit-work/2026-09-05/homebrew-proposal/README.md). Its current v2.0.1 archive hash is verified and temporary formula tests pass 15 assertions. It has not been published to the tap. The future product release needs its own immutable source/hash and isolated clean-install, upgrade, uninstall and rollback trials.
2. Run real owner/platform acceptance: interactive TUI and backup recovery, Debian/Arch/macOS package transitions, Raspberry Pi resource behavior, npm integration installation/authentication, and pinned remote/MCP startup. Safe fixtures, cross-builds and ephemeral Linux root tests do not substitute for these trials.
3. Resolve repository policy before publication. Dependency Review currently fails because GitHub's dependency graph is disabled, not because it found a new dependency advisory. Enabling that setting is separately pending user input. Existing main protection requires Lint, Test and Build (ubuntu-latest), with strict freshness disabled. Release Gate enforcement and other security settings remain explicit owner decisions.
4. Promote reviewed source to main, select a release version, run the exact tag gate, and publish only after the above gates. Main and release tags were not changed. Developer ID signing/notarization requires separate credentials and policy.

No owner package install, privileged update, destructive worktree cleanup, force push or release publication was performed during this program.

## Unpublished release rehearsal and artifact review

`make release` completed using pinned GoReleaser 2.17.0 and Syft 1.44.0 on source f7ff93f. Four Darwin/Linux amd64/arm64 archives, one source archive, five SPDX 2.3 SBOMs and all ten SHA256 entries were verified. Architecture, wrapped paths, binary 0755 / docs 0644, license/notice contents and retired-product absence passed. Seven Darwin arm64 CLI checks passed with isolated HOME/config and no host package managers on PATH. The generated `0.0.1-next` version is a local snapshot label, not a selected release version. Nothing was uploaded as a release or attested.

Evidence: [release rehearsal](audit-work/2026-09-05/release-rehearsal.json), [CLI smoke](audit-work/2026-09-05/final-cli-smoke.json), [exact source CI](audit-work/2026-09-05/final-source-ci.json), [local checks](audit-work/2026-09-05/final-local-verification.json).

The HTML update was browser-checked at 1440, 390 and 320 px with no page overflow, all 18 disposition rows, expansion and an exported brief containing current integration status. Original audit interaction/print evidence remains dated separately; the guide preserves that original snapshot instead of rewriting old findings as if they never existed.
