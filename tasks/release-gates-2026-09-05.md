# Release gate remediation continuation

Continue the user's accepted September integration plan from `4a4cf57`. Root owns Git; all delegated agents use Astra. Preserve original worktrees and recovery data. Main promotion, selecting a release version, publishing a tag/release and destructive owner-machine operations remain separate decisions.

## Execution order

1. Inspect the disabled dependency graph and current security settings, prepare a minimal reversible remediation, and rerun Dependency Review. Preserve before/after evidence; do not weaken checks to manufacture green results.
2. Revalidate and publish the prepared ownership-only Homebrew change as a reviewable PR against current tap source. Keep its current version/hash; do not claim this distributes the new integration code.
3. Add and run a bounded disposable platform acceptance harness after an Astra review freezes its scope. Use explicit CI opt-in and no owner package operations. Record exactly which real package-manager path was executed.
4. Run isolated PTY/UI and backup-recovery smoke where supported, then document remaining real hardware, terminals, credentials and installation transitions.
5. Update the HTML guide with runnable results, PRs and the concrete remaining release decisions.

## Current boundaries

Current code defects in the original 18-finding audit are repaired except the still-live external Homebrew behavior. The integration source, normal/race suites, CodeQL, quality gate and unpublished packaging are verified. Remaining owner settings and live-platform evidence do not become satisfied merely because the previous plan checked off “record remaining gates”.

Dependency graph changes should be limited to the necessary repository features; new paid services or broad access changes are outside this work. Main branch protection/promotion and public release remain reviewable owner decisions.

## Review

This continuation is verified through integration commit `739f82f`. Full CI and CodeQL passed. The Debian QA helper/workflow adds acceptance infrastructure without changing production behavior. The HTML guide links each result to exact evidence.

- [x] Dependency graph/alerts enabled; Dependency Review and comparison API pass.
- [x] Homebrew ownership repair published as draft PR #1, with 18 assertions and three real hosted macOS install/uninstall cycles passing at `79ef099`.
- [x] Debian 13 amd64 executes all eight real package acceptance cases at `739f82f`, including installation, held receipts and supervised no-op update.
- [x] Ten isolated Darwin PTY assertions pass against binary source `4a4cf57` (production source `f7ff93f`).
- [x] Publish [readable release guide](release-gates-2026-09-05.html), current status and bounded next sequence.

Homebrew merge is pending the user's concrete PR decision. No main promotion, release tag or product publication is implied by completion of this continuation.

## Dependency Review completed

Enabled dependency alerts and the dependency graph using GitHub's supported vulnerability-alerts API; both mutation and verification returned204. Dependency Review run33989635030 now passes. A live dependency-comparison API request also succeeds. This does not enable automatic dependency-update PRs or alter branch protection. Reference: https://docs.github.com/en/rest/repos/repos#enable-vulnerability-alerts.

## Verified evidence

- [Debian acceptance](audit-work/2026-09-05/release-gates/debian-acceptance.md): actual APT install/receipt/hold path, exact-plan rejection/success, synthetic private journal, installed-fzf configuration; update phases ran with zero version changes.
- [Homebrew acceptance](audit-work/2026-09-05/release-gates/homebrew-acceptance.json): both legacy names survive three fixture types during actual install/uninstall; version remains 2.0.1.
- [PTY assertions](audit-work/2026-09-05/release-gates/pty-acceptance.json) and [screen evidence](audit-work/2026-09-05/release-gates/pty-screens.html): five tabs, create-only profile, switch, backup create/cancel/restore/delete-cancel, resize and clean exit.
- [CI](https://github.com/tekierz/dotfiles/actions/runs/34008157778) and [CodeQL](https://github.com/tekierz/dotfiles/actions/runs/34008157746) passed on the same `739f82f` source.

## Remaining development sequence

1. Adopt the reviewed [tap repair PR #1](https://github.com/tekierz/homebrew-tap/pull/1) after the requested merge decision. This closes existing formula ownership behavior; it does not deliver the new product source.
2. Add a separately scoped disposable acceptance slice for exact release-candidate clean install, v2.0.1-to-candidate upgrade and rollback. Verify configuration preservation and binary/source identity across transitions. Extend native package coverage to Arch and real version changes; current Debian update is explicitly a no-op.
3. Schedule owner terminal/fonts/mouse and realistic backup recovery, Raspberry Pi resources/hardware, and supported npm/MCP installation/startup/authentication. Each needs explicit fixtures or suitable hardware/credentials. Automated profile and authority tests remain useful but do not substitute for these observations.
4. Review repository policy: integration is currently unprotected; main requires Lint, Test, Build (ubuntu-latest), with strict freshness off. Decide whether to require Release Gate and fresh checks and enable remaining secret protections. Dependency graph/alerts are now enabled; automatic update PRs were not enabled.
5. Once release scope and version are chosen, prepare main promotion, rebuild the exact candidate, verify release gates, publish the approved tag/assets, update the tap to immutable source/hash, and recheck distribution. Signing/notarization needs separate credentials and policy.

## Verification limits

No owner package operations occurred. Debian uses a disposable pinned-image container; Homebrew uses disposable hosted macOS and a local test tap; PTY writes only to a new private temporary home. Public tap main, dotfiles main and tags remain unchanged at this review checkpoint. Original worktrees, stashes and 97 archive refs remain preserved.
