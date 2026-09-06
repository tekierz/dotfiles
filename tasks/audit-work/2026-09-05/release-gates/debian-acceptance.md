# Debian acceptance evidence

**Passed:** [Debian run 34008157813](https://github.com/tekierz/dotfiles/actions/runs/34008157813), commit `739f82f457672fffed6504bee8577498cc6cb8a7`, completed 2026-09-06 at 03:07:09 UTC. Downloaded logs/artifacts contain the required `DOTFILES_DEBIAN_PLATFORM_EXECUTED cases=8 update=no-op` marker. Independent artifact assertions verified all eight cases below.

Environment: Debian 13.6/trixie amd64 container, APT 3.0.3, on the GitHub-hosted Ubuntu 24.04 kernel. Pinned image digest: `sha256:abc9cb88a5587630d7f915f47b23b0668fe250fbfc6457aa4d52b534c1bbf73f`. Installed package: `fzf 0.60.3-1+b2`.

| Case | Verified evidence |
|---|---|
| Wrong hash | Empty stdout, exact changed-plan error; successful bootstrap asserts exit 2 and package still absent. |
| CLI plan/apply | One reviewed APT/fzf action; exact hash success line; empty stderr; one succeeded journal action. |
| Installed receipt | Native dpkg and individual/batch product queries agree on installed status/version. |
| Held receipt | `hold ok installed` remains healthy at the same version; bootstrap verifies queries retain the hold before explicit unhold. |
| Targeted update | Real production sequence logs index refresh before install; zero upgraded/installed/removed; version unchanged. |
| No changes | Follow-up plan is `no_changes`, lacks plan hash, and reports apply unavailable. |
| Stale apply | Empty stdout, exact not-ready error; successful bootstrap asserts exit 2. |
| Installed fzf configuration | `TestGeneratedOptionsAreAcceptedByInstalledFzf` passed in 0.01s; no skip. |

The journal binds the reviewed hash, contains the package-only/no-filesystem-rollback warning, and invents no backup. Bootstrap asserts its private source mode 0600. The synthetic config tree stayed empty and the shell sentinel checksum matched. Public apply/plan stderr remained empty; raw APT output exists only in the separate QA update log.

Exit codes, file modes and intermediate absent/held state are established by fail-fast assertions in the successful exact-commit bootstrap. They were not separately exported as raw exit-code files or intermediate inventories. Binary hashes are the runner's recorded hashes; binaries were not part of the downloaded evidence artifact. [Sanitized JSON](debian-acceptance.json) retains precise hashes, artifact metadata and per-case results; it omits raw user paths, hostname, operation ID and journal contents. The original artifact expires 2026-09-13.

[Shared CI 34008157778](https://github.com/tekierz/dotfiles/actions/runs/34008157778) also completed successfully at the same commit: Lint, Security, both builds, Ubuntu/macOS normal and race tests, Test aggregate and Release Gate. Its Linux privileged lifecycle step passed. [CodeQL 34008157746](https://github.com/tekierz/dotfiles/actions/runs/34008157746) passed at that commit.

This closes the Debian fzf install/receipt/held-state/targeted no-op update subgate. It does **not** prove a version-changing upgrade, real UpdateAll transaction, real cancellation/sudo-denial recovery, Arch/pacman, Darwin/Homebrew, native Debian kernel/ARM/Pi hardware, physical terminal visuals, casks/services/GPU/network behavior, npm/auth flows, owner snapshot restoration or Homebrew publication. No owner machine package/configuration mutation was used.
