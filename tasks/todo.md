# Release-Readiness Plan

**Created:** 2026-07-03 · **Target:** first public release
**Source:** full-codebase audit re-verified against `main` — details, evidence, and exact
locations for every item are in `tasks/release-audit-2026-07-03.md`.
**State of main:** build/vet/test/race/gofmt green; govulncheck clean. Two prior
remediation cycles fixed 62 of 160 audit findings; the items below are what remains.

Severity counts remaining: **3 critical, 26 high, 37 medium, 32 low.**
P0+P1 are release blockers; P2 should ship but won't eat data; P3+ is post-release.

---

## P0 — Data-loss bugs (must fix before any release)

- [ ] **Self-destructing binary**: `installUtilities` (internal/ui/installation.go) deletes
      the currently running `dotfiles` binary before copying the replacement; if the copy
      fails the user has no binary. Write to temp + rename, never remove-then-copy.
- [ ] **Backup format schism (cluster)**: bash script writes directory-format backups; Go
      restore silently skips them (`internal/backup/backup.go` fallback drops dirs and does
      lossy `_`→`/` name mapping). Consequences to fix together:
      - [ ] `dotfiles uninstall` "restores" a bash-format backup as a no-op, then
            `RemoveAll`s the backups directory — unrecoverable loss (cmd/dotfiles/main.go).
            Uninstall must refuse to delete backups it could not actually restore.
      - [ ] TUI restore reports success on backups it silently skipped.
      - [ ] Either teach Go restore the bash manifest format, or migrate/refuse loudly.
- [ ] **Bash restore half-aborts**: `((var++))` under `set -e` still aborts
      `restore_backup` partway in one copy of the logic (bin/dotfiles-setup:~824); the
      duplicated later copy was fixed, the earlier one wasn't. Fix both / dedupe.
- [ ] **neovim preset destroys config + its only backup** (internal/tools/neovim.go):
      re-running the preset flow moves the user's config to a fixed backup path,
      clobbering the previous backup, then a failed clone leaves nothing. Timestamped
      backups + clone-to-temp-then-swap.
- [ ] **Git config clobber (residual)**: Go `WriteGitConfig` now preserves identity but
      bash `setup_git` (bin/dotfiles-setup:~1845) still replaces `~/.gitconfig` wholesale.

## P1 — Release blockers (broken promises, versioning, dead gates)

Versioning / distribution:
- [ ] Align version to the release tag everywhere: `Makefile` still hardcodes
      `VERSION = 2.0.1`; cmd/dotfiles/main.go fallback constant stale. Single-source from
      git tag via ldflags.
- [ ] Stop tracking the compiled `bin/dotfiles` ELF binary in git (it ships stale —
      currently reports 2.0.1). Add to .gitignore; adjust Makefile/README accordingly.
- [ ] Delete or fix the stale in-repo `Formula/dotfiles-setup.rb` (placeholder sha256);
      the real formula lives in the homebrew-tap repo — having both invites drift.
- [ ] Decide the fate of `bin/dotfiles-setup.ps1` for the initial release: it is
      undocumented-in-flow, version "1.0.0", pipes `get.scoop.sh` over plain HTTP to
      `Invoke-Expression`, clobbers `$PROFILE` with no backup, silently weakens execution
      policy, suppresses all install failures, and prints "Windows Terminal configured"
      for a scheme it never writes. **Recommendation: remove it from the initial release**
      (or mark experimental + fix the four HIGHs). Full list in the audit report.

CI / quality gates:
- [ ] `.golangci.yml` is incompatible with the current golangci-lint, and the CI lint step
      is `continue-on-error` — linting is silently dead. Fix config, make it blocking.
- [ ] Make the CI Security job able to fail: `govulncheck` and `staticcheck` are
      `continue-on-error` (race gate is already blocking).

Features that don't do what the UI says:
- [ ] Zsh "Plugins" checkboxes are dead UI — five plugins selectable, none ever wired
      into the generated .zshrc (internal/ui/screen_config_zsh.go + internal/tools/zsh.go).
      Wire them or remove the section.
- [ ] Default "p10k" prompt style generates a .zshrc that never loads Powerlevel10k and
      nothing installs it — new users get a broken prompt out of the box (internal/tools/zsh.go).
- [ ] fzf config screen is a no-op: generated config file is never sourced (internal/tools/fzf.go).
- [ ] macOS Apps screen offers 6 apps that don't exist in the tool registry (screen list drift).
- [ ] tmux percent-style split bindings are bound then immediately unbound (internal/tools/tmux.go).
- [ ] Hotkeys aliases: input can't accept `h`, `l`, or space; saved aliases are never
      consumed by anything; and `q` while typing an alias instantly quits the TUI
      (internal/ui/screen_hotkeys.go — last unguarded quit path).
- [ ] Manage screen: keyboard input while the install-log panel is shown still mutates
      hidden settings fields (mouse path was fixed; keyboard + misleading "↑↓: scroll"
      footer remain) (internal/ui/screen_manage.go).

Bash installer (still the documented fallback path):
- [ ] `--list-backups` prints one entry then exits 1 (`((count++))` under `set -e`).
- [ ] `setup_utilities` writes a legacy bash CLI to `~/.local/bin/dotfiles`, shadowing or
      clobbering the Go binary of the same name.
- [ ] Generated CLI calls `sed_i` which is never defined inside the heredoc (runtime crash).
- [ ] KDE systems without `kwriteconfig` abort the entire install under `set -e`.
- [ ] Raspberry Pi model detection lost in a `$( )` subshell — Pi-specific setup never runs.
- [ ] Embedded CLI theme table drifted: 13 themes vs 16 (frappe/macchiato silently get mocha).

## P2 — Should fix before release (correctness/security, non-data-loss)

- [ ] `sudo apt update` runs synchronously inside CheckOutdated — TUI hangs on the sudo
      password prompt (internal/pkg/apt.go; partial mitigation exists).
- [ ] pacman `Update()` installs from a stale sync DB — reported updates don't apply
      (internal/pkg/pacman.go; use the checkupdates-db pattern or a full -Sy transaction guard).
- [ ] `CheckAllUpdates` swallows all per-manager errors — total failure renders as
      "everything up to date" (internal/pkg/update.go).
- [ ] ManageConfig saved from a worker goroutine while the UI goroutine can still write
      through field pointers — data race / torn JSON (internal/ui/manage_dualpane.go:95-113).
- [ ] Raw package-manager output rendered to the terminal without stripping ANSI/control
      sequences (update results path) — escape-sequence injection surface.
- [ ] caff pidfile in world-writable /tmp is predictable — cross-user process-kill;
      Go copy partially fixed, generated-script copy (bin/dotfiles-setup) unchanged.
- [ ] App-detection substring matching produces false "installed" (internal/tools/apps.go).
- [ ] Hotkeys config path ignores `XDG_CONFIG_HOME`, diverging from ConfigDir().
- [ ] Blocking package-manager subprocess calls inside `View()` via ensureInstallCache
      (six screen_config_*.go call sites) — move to async Cmd like the rest of the app.
- [ ] yazi theme.toml declared-but-never-written; PreviewMode ignored. glow config written
      to a path glow doesn't read on macOS.
- [ ] bash: package install failures silenced then "installation complete" reported.

## P3 — Cleanups (post-release acceptable; tracked so they don't rot)

- [ ] Dedupe: ensureInstallCache vs loadInstallCacheCmd (~55 dup lines); favorites-filter
      block ×5; dual-pane layout engine ×3; styles defined twice (var block vs
      updateStyles — GradientCyber already drifted); backup/restore/cache flow triplets;
      bash outer-script vs generated-CLI function copies.
- [ ] Test quality: mockScreenHandler records but never asserts message forwarding;
      renderFileTree tests partially non-hermetic/tautological (some fixed on main).
- [ ] All remaining LOW items — enumerated with locations in the audit report.
- [ ] Deferred from prior remediation: interactive install cancellation (Esc);
      EvalSymlinks hardening in backup path guard; coordinated bubbletea/lipgloss v2
      dependency migration.

## Pre-release checklist (run in order, after P0–P2 land)

- [ ] `go build ./... && go vet ./... && gofmt -l . && go test -race ./...` all clean
- [ ] golangci-lint clean with the repaired config; CI fully blocking
- [ ] `govulncheck ./...` re-run clean
- [ ] Run the `pre-pr-tests` skill checklist (manual TUI pass on macOS + one Linux)
- [ ] Fresh-machine install test: brew tap path AND bash-script path; then uninstall and
      verify configs restored byte-identical (this exercises the P0 backup fixes)
- [ ] Tag release; verify `dotfiles --version` matches the tag; update homebrew-tap
      formula sha256; verify `brew install` from the tap
- [ ] README final pass: install instructions match reality; consider adding a
      commit-pinned/checksummed variant of the curl|bash instruction

## Post-release backlog (carried forward)

- New AI CLI tools — `tasks/new-tools-spec.md` (researched, ready to implement; re-verify
  package names first)
- Features from archived beta.plan: config export/import, tool dependency graph,
  `dotfiles doctor`, plugin system, theme customization guide, user guide
- Integration/E2E test suite for install/uninstall; UI snapshot tests; CI platform matrix
- Windows support decision (native PowerShell done right, or WSL-only officially)
