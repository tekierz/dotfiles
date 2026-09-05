# Repository preservation and sync — 2026-09-05

Integration branch: `release-remediation`, initially `d77c0f7`. `main` and release tags remain separate promotion boundaries.

## Verified preservation

All six originally dirty worktrees were saved before integration: tracked/staged binary patches, working-file archives and available generated build outputs, plus a verified Git bundle containing all refs and reflog-reachable history. Private recovery is `.git/recovery/2026-09-05-integration/`; files are private to the owner, with SHA-256 verification metadata. Original worktrees and 23 stashes remain intact.

97 archive refs were pushed atomically without force and independently compared with `git ls-remote`: **97 exact commit matches**. Names are visibly historical under `archive/2026-09-05/branches/`, `remote-before/`, `stashes/`, and `worktrees/`. The [ref manifest](audit-work/2026-09-05/archive-refs.json) gives exact identities; [publication evidence](audit-work/2026-09-05/archive-publication.json) records the check.

Gitleaks 8.30.1 scanned 695 reachable commits with no findings using the saved release-preparation configuration (one exact fixture-hash exception). This is a detector result, not proof that history contains no sensitive material. Generated untracked binaries and browser scratch remain in private recovery.

## Source dispositions

See the [dirty-worktree analysis](dirty-worktree-analysis-2026-09-05.md). Three source snapshots are published:

- `archive/2026-09-05/worktrees/integration-root` — `2b831471fc1bc3f5173eb5b12e6eeff367c8d2e5`
- `archive/2026-09-05/worktrees/open-source-release-readiness` — `bb1f472c455724a8b7bbd307c1e7f2b191ac509e`
- `archive/2026-09-05/worktrees/audit-remediation` — `776a8cf5ad5c75d4357acfcfac77175ae1aff2f7`

The isolated `integration/2026-09-05-candidate` starts from the release-preparation snapshot, which includes the 35 newer product commits. Its acceptance is in progress; publication as an archive does not mean release readiness.

## Instruction policy

Repository `AGENTS.md` is a symlink to `CLAUDE.md`, so one canonical edit updates both. All subagents use `gpt-6-astra` with task-appropriate reasoning. Global `~/.codex/AGENTS.md` and `~/.claude/CLAUDE.md` were also updated; their prior contents are saved in private recovery. Historical worktree instruction files remain preserved history; the current user instruction overrides their old model routing.
