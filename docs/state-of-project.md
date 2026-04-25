# State of the Project — Jumping-Off Report

**Generated:** 2026-04-25
**Branch:** `claude/project-analysis-report-bn2Eh`
**Baseline:** v2.1 release (commit `8e6357d`)
**Goal:** Make the application performant, secure, and ready for open-sourcing.

---

## 0. TL;DR

The project is in **good shape technically** — most of the issues called out in the previous `docs/v2-analysis-plan.md` have been resolved in v2.1. The remaining work splits into four buckets:

| Bucket | Status | Effort |
|--------|--------|--------|
| Critical security fixes (new findings) | 🔴 Blocking open-source release | ~1 day |
| Open-source governance (CONTRIBUTING, SECURITY, release pipeline) | 🔴 Blocking open-source release | ~1 day |
| Test coverage — runner & UI core paths | 🟡 Should ship before 1.0 | ~3–5 days |
| Code-structure refactors (`Update()`, deepdive duplication) | 🟡 Maintainability — pre-community | ~2–3 days |

If you do nothing else, address the **two new shell-injection findings in `internal/runner/bash.go`** and add the **standard OSS files** before flipping the repo public.

---

## 1. Where The Project Stands Today

### Codebase scale
- **79 Go files**, ~24,300 LOC
- **2 direct dependencies** (`bubbletea`, `lipgloss`) — minimal, well-maintained
- **Go 1.25.6** — current, no known CVEs against it
- `go vet`, `gofmt`, `goimports` all clean
- All tests pass; no flakes or skips

### v2-analysis-plan items — verified status
| # | Issue | Location | Status in v2.1 |
|---|-------|----------|----------------|
| 1 | Path traversal in restore | `cmd/dotfiles/main.go:650` | ✅ Fixed (clean + prefix check) |
| 2 | Shell injection in `apt.UpdateAllStreaming` | `internal/pkg/apt.go:284` | ✅ Fixed (argv arrays via `RunStreamingWithSudo`) |
| 3 | `KeepSudoAlive` goroutine | `internal/runner/bash.go` | ✅ Removed; replaced with `CacheSudoCredentials` |
| 4 | `tmux.go` perms 0755/0644 | `internal/tools/tmux.go:79,248` | ✅ Now 0700/0600 |
| 5 | `HOME` env validation | `internal/config/config.go` | ✅ XDG → HOME → `os.UserHomeDir` fallback |
| 6 | Apt N+1 query | `internal/pkg/apt.go:196` | ✅ Single `dpkg-query` batch call |
| 7 | Repeated platform detection | `internal/tools/tool.go` | ✅ `sync.Once` cache in `pkg.DetectPlatform` |
| 8 | Multiple `NewRegistry()` | (15+ sites) | ✅ Singleton via `GetRegistry()` |
| 9 | Unbounded install output | `internal/ui/app.go:248` | ✅ Circular buffer (max 20 / 500 lines) |
| 10 | Sync config load | `internal/ui/app.go:260` | ✅ Best-effort + async cache load |
| 11 | Go vulnerable version | `go.mod` | ✅ Bumped to 1.25.6 |

**Bottom line:** the previous plan was executed competently. The new findings below are mostly issues that weren't on the original list.

---

## 2. Security — New Findings That Block Open-Source Release

### 🔴 CRITICAL — Shell injection in `runner.RunFunction`
- **File:** `internal/runner/bash.go:94-107`
- **What:** `args` are joined with `strings.Join(args, " ")` and inlined into a `bash -c` script, so any caller who passes user-influenced arguments hands the user a remote-exec primitive against `bash`.
- **Risk:** Every `RunFunction(name, userInput...)` call site is potentially exploitable. This is a TUI that runs as the invoking user, so the blast radius is limited to that user — but it still includes `sudo` access if cached.
- **Fix:** Pass arguments as a real `argv` (`exec.Command(bashPath, "-c", scriptBody, "--", args...)`) or write args to a tempfile / env var. Don't string-concatenate.

### 🔴 HIGH — Unquoted `ScriptPath` in `bash -c` body
- **File:** `internal/runner/bash.go:80-83`
- **What:** `r.ScriptPath` is interpolated into a `source %s` string without `%q`. Any path containing whitespace or shell metacharacters breaks (and on a hostile path, executes).
- **Fix:** `fmt.Sprintf("source %q\n", r.ScriptPath)` and the same treatment everywhere `ScriptPath` is interpolated into a shell string.

### 🟡 MEDIUM — Backup directory & restored-file permissions
- **Files:** `cmd/dotfiles/main.go:665,677`, `internal/ui/app.go:487`
- **What:** Backup parent dirs are created with `0755` and restored files written with `0644`. The original CLAUDE.md rule says configs get `0700`/`0600`. Some users back up `.zshrc` containing tokens.
- **Fix:** `0700` for the backup dir, preserve original mode when restoring (stash it during backup), or fall back to `0600`.

### 🟡 MEDIUM — Whole environment passed through to subprocesses
- **File:** `internal/runner/bash.go:107,206,295`
- **What:** `cmd.Env = os.Environ()` blindly inherits every variable, including `LD_PRELOAD`, `PYTHONPATH`, etc. Limited risk for a user-run CLI, but it's an easy hardening win and a checkbox security reviewers look for.
- **Fix:** Allow-list `PATH`, `HOME`, `USER`, `LANG`, `TERM`, `SHELL`, `XDG_*`, `SUDO_*`. Drop the rest.

### Lower priority / acceptable
- `git clone` for TPM uses HTTPS — git defaults are fine; no change needed.
- `0755` on user-installed scripts in `~/.local/bin` is conventional for executables.

> **Action:** Land the two CRITICAL/HIGH items, the perm tightening, and the env allow-list before going public. Then run `gosec ./...` and `govulncheck ./...` in CI on every PR (the security workflow described in `docs/security-scanning.md` should be created — see §6).

---

## 3. Performance — Mostly Solved, A Few Sharp Edges Left

The expensive issues in v2-analysis-plan are gone. What's left is small.

### 🟡 MEDIUM — Cache invalidation on every navigation
- **File:** `internal/ui/app.go:856`
- **Symptom:** `manageInstalledReady` is cleared on a wide set of message types, forcing a re-batch on the next render and adding 50–200ms.
- **Fix:** Invalidate only on `postInstallMsg` / `postUninstallMsg`. Treat plain navigation as cache-friendly.

### 🟡 MEDIUM — `BaseTool.IsInstalled()` can reach the manage render path
- **File:** `internal/tools/tool.go:108-125`
- **Symptom:** When the cache hasn't populated, the manage screen render can call `mgr.IsInstalled(pkg)` per tool — 100–500ms with 27 tools.
- **Fix:** Don't render the tool-status column until `manageInstalledReady`. Show a spinner / "checking…" placeholder and finalize when the cache lands. (The async path already exists; just guard the render.)

### 🟢 LOW — Redundant `pkg.DetectPlatform()` calls inside registry loops
- **File:** `internal/tools/registry.go:209,233,275`
- **Symptom:** Cached, so <1ms — but unnecessary work in a tight loop.
- **Fix:** Hoist `platform := pkg.DetectPlatform()` out of the loop body.

### 🟢 LOW — Animation grid re-allocated each frame
- **File:** `internal/ui/animation.go:237-269`
- **Symptom:** ~1ms × 72 intro frames. Imperceptible.
- **Fix (optional):** Reuse the rune grid via a `sync.Pool` or instance field.

> **Action:** Items 1–2 are easy wins. The rest is polish you can defer.

---

## 4. Code Structure — The Real Maintainability Risk

The codebase is well-formatted and 100% godoc-commented on the packages we sampled, but several files are large enough to fight back when you try to extend them.

### File-size leaderboard
| File | Lines | Notes |
|------|-------|-------|
| `internal/ui/manage_dualpane.go` | 1,741 | Dual-pane manage UI; mixes input handling, field editing, and rendering |
| `internal/ui/screens_deepdive.go` | 1,649 | 30+ near-identical `renderConfig*()` functions |
| `internal/ui/app.go` | 1,461 | The state machine; `Update()` is 395 lines / 70+ cases |
| `internal/ui/hotkeys_dualpane.go` | 1,013 | Hotkey browser UI |
| `internal/ui/screens_manage.go` | 827 | Additional manage screens |

### Structural issues
1. **`App.Update()` is the architectural bottleneck.** 70+ `case` branches across backup, update, user, install, animation, screen-nav messages all interleaved. Splitting by message family into `handleBackupMsg`, `handleUpdateMsg`, `handleInstallMsg`, etc. would shrink the cyclomatic complexity dramatically.
2. **`screens_deepdive.go` has 30+ `renderConfig<Tool>()` clones.** Each repeats: title → field label → control → repeat. Replace with a declarative `[]FieldSpec` per tool plus a generic renderer. Eliminates ~500–700 lines.
3. **`createBackupCmd()` and `autoBackupIfEnabled()`** in `app.go` differ only by the timestamp suffix. Merge with a `createBackup(auto bool)` helper.
4. **Screen-constant explosion.** 51 `Screen*` constants in `app.go:29-80`, two added per new tool. A registry-driven approach (tool → screens) would let new tools plug in without touching the enum.
5. **`App` struct has 40+ fields** (`app.go:117-237`). Many are screen-local. Move screen-specific state into screen objects (the `ScreenManager` pattern is partly there — finish it).

### Duplication / dead weight
- `Formula/` and `LinuxLocalTesting/` directories live in the repo. The Homebrew formula has its own repo (`tekierz/homebrew-tap`); `LinuxLocalTesting/Notes` looks like personal scratch. Move out before public.
- No TODO/FIXME/XXX/HACK comments found anywhere — that's genuinely impressive. Don't introduce them in a refactor.

> **Action:** Treat refactor #1 (`Update()`) and #2 (deepdive factory) as the two highest-leverage cleanups before contributors arrive. They will dominate any "I want to add tool X" PR otherwise.

---

## 5. Test Coverage — The Biggest Open Risk

| Package | LOC | Coverage | Comment |
|---------|-----|----------|---------|
| `internal/config` | 979 | **53.7%** | Solid foundation |
| `internal/pkg` | 1,512 | 7.7% | Mock exists; real impls untested |
| `internal/tools` | 3,470 | 9.1% | Registry tested, individual tools not |
| `cmd/dotfiles` | 1,062 | 5.0% | Restore tested; rest of CLI not |
| `internal/ui` | 13,983 | **1.7%** | The bulk of the code is untested |
| `internal/runner` | 365 | **0.0%** | **Security-critical, zero tests** |
| `internal/hotkeys` | 375 | 0.0% | Data-driven, lower risk |
| `internal/scripts` | 300 | 0.0% | Static templates, lower risk |
| **Overall** | ~24k | **~8.8%** | Industry norm is 60–80% |

### What to test, in priority order
1. **`internal/runner/bash.go`** — every shell-execution path. Cover the new fix for `RunFunction` argv handling, sudo cache behavior, OS detection parsing, exit-code propagation. Without these, a regression silently ships exploitable code.
2. **CLI command surface (`cmd/dotfiles`)** — `install`, `manage`, `update`, `theme`, `backups`, `restore`, `uninstall`, `status`. Smoke tests with the existing mock package manager. These are the user-visible contract.
3. **Path-traversal regression** — explicit tests covering `..`, absolute paths, symlinks, and unicode tricks against backup restore.
4. **Package-manager implementations** — `apt.go`, `brew.go`, `pacman.go` with fake binaries on `PATH` (use `t.TempDir()` + shell-script stubs).
5. **Tool registry** — table-driven test that every tool is reachable, has at least one platform mapping, and produces the expected install command.
6. **`internal/ui` golden tests** — capture rendered output for the 5–10 most-trafficked screens. This catches accidental layout regressions cheaply (Bubble Tea's `tea.Model` makes this easy).

> **Action:** The coverage gap on `internal/runner` is the test gap that most directly affects security. Land it next to the bash.go fixes.

---

## 6. Open-Source Readiness Punch List

### 🔴 Must have before flipping public
- [ ] **CONTRIBUTING.md** — branch model, commit style (see CLAUDE.md rules), how to run `make build` / tests, expectation for tests on new tools.
- [ ] **CODE_OF_CONDUCT.md** — Contributor Covenant 2.1 is the standard.
- [ ] **SECURITY.md** — vulnerability disclosure address, supported versions, response SLA. Mention the GitHub private security advisory flow.
- [ ] **CHANGELOG.md** — at minimum a `## v2.1` entry summarising fixes; future commits follow Keep-a-Changelog.
- [ ] **`.github/ISSUE_TEMPLATE/`** — `bug_report.md`, `feature_request.md` (and a `config.yml` to disable blank issues).
- [ ] **`.github/PULL_REQUEST_TEMPLATE.md`** — checklist that mirrors the pre-PR skill.
- [ ] **GoReleaser config (`.goreleaser.yml`) + release workflow** — Makefile already references GoReleaser; add the actual config and a `release.yml` triggered on `v*` tags. Produces signed checksums for macOS/Linux/Pi.
- [ ] **Move `Formula/` to the homebrew-tap repo** and **delete `LinuxLocalTesting/`** (or move it under `docs/`).
- [ ] **Decide on the Claude Code workflows.** `claude.yml` and `claude-code-review.yml` may be confusing for outside contributors — either document them or restrict them to contributors with access.

### 🟡 Strong defaults to add
- [ ] **`.github/CODEOWNERS`** — at minimum `* @tekierz` until others join.
- [ ] **`.github/dependabot.yml`** — gomod weekly + actions weekly.
- [ ] **`.github/workflows/security.yml`** — `govulncheck`, `gosec`, `staticcheck`. The doc in `docs/security-scanning.md` already describes it.
- [ ] **README badges** — CI status, latest release, license, Go version.
- [ ] **Branch protection on `main`** — require CI + at least one review.
- [ ] **`docs/ARCHITECTURE.md`** — TUI flow, package-manager abstraction, config layering, how a new tool plugs in.
- [ ] **`docs/CONTRIBUTING_TOOLS.md`** — concrete walkthrough: "Adding tool X in 5 steps" referencing CLAUDE.md.

### 🟢 Nice to have
- [ ] `CITATION.cff`, `FUNDING.yml`
- [ ] macOS notarization in the release pipeline (Apple Developer cert required)
- [ ] Cosign-signed checksums in releases
- [ ] Reproducible-build instructions

---

## 7. Suggested 2-Week Plan

### Week 1 — security + governance (release-blocking)
- **Day 1–2:** Land the two `runner/bash.go` shell-injection fixes; tighten backup permissions; allow-list env vars; add unit tests for each fix.
- **Day 3:** Author CONTRIBUTING / CODE_OF_CONDUCT / SECURITY / CHANGELOG. Add issue + PR templates. Configure dependabot.
- **Day 4:** Create `.goreleaser.yml` and `release.yml`; cut a `v2.1.1` test release in a private fork.
- **Day 5:** Move `Formula/` and `LinuxLocalTesting/` out of the repo. Add `security.yml` workflow. Add README badges. Configure branch protection.

### Week 2 — coverage + structure (pre-community)
- **Day 6–7:** `internal/runner` test suite (target 70%+).
- **Day 8:** CLI smoke tests for all `cmd/dotfiles` subcommands.
- **Day 9–10:** Refactor `App.Update()` into per-family handlers. No behavior change, golden-test the manage + install screens around the change.
- **Day 11–12:** Replace `screens_deepdive.go` clones with a declarative renderer; add a couple of new tools through the new path to prove it.
- **Day 13:** Re-audit. Run `gosec`, `govulncheck`, `golangci-lint` clean. Tag `v2.2.0` and flip the repo public.

---

## 8. Open Questions To Decide

1. **Public or limited release first?** A "soft launch" via the existing Homebrew tap to a small group catches issues before the wider GitHub audience.
2. **Do you want a contributor license agreement / DCO?** MIT + DCO sign-off (`git commit -s`) is the lightest option and is enforceable in CI.
3. **What's the support promise?** SECURITY.md needs to commit to a window (e.g., "we patch the latest minor and previous minor").
4. **Telemetry?** Currently none. If you ever want install/feature metrics, decide *before* community PRs land it ad-hoc.
5. **Formula maintenance.** The tap repo's formula version pin is presumably bumped manually — wire it into the release workflow so a tag updates the tap automatically.

---

## 9. Appendix — Specific File:Line Reference Sheet

Security:
- `internal/runner/bash.go:80-83` — quote `ScriptPath`
- `internal/runner/bash.go:94-107` — replace `strings.Join(args, " ")` with argv passthrough
- `internal/runner/bash.go:107,206,295` — env allow-list
- `cmd/dotfiles/main.go:665` — backup dir to `0700`
- `cmd/dotfiles/main.go:677` — preserve / tighten restored file mode
- `internal/ui/app.go:487` — same as above

Performance:
- `internal/ui/app.go:856` — narrow cache invalidation triggers
- `internal/tools/tool.go:108-125` — guard with `manageInstalledReady`
- `internal/tools/registry.go:209,233,275` — hoist `DetectPlatform()` from loops

Refactor:
- `internal/ui/app.go:706` — split `Update()` by message family
- `internal/ui/screens_deepdive.go:197-1550` — declarative deepdive renderer
- `internal/ui/app.go:516-703` — merge backup helpers
- `internal/ui/app.go:29-80` — consider screen-registry instead of enum

Tests to add first:
- `internal/runner/bash_test.go` — full coverage of the security fixes
- `cmd/dotfiles/main_test.go` — CLI subcommand smoke tests
- `internal/pkg/{apt,brew,pacman}_test.go` — fake-binary integration tests
- `internal/ui/golden/*.txt` — golden snapshots for top screens
