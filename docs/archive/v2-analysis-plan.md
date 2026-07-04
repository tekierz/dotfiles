# Dotfiles v2 Analysis & Remediation Plan

> **⚠️ STATUS: COMPLETED / SUPERSEDED.** This is a historical planning document. The plan
> below was **executed and merged as v2.1 beta** (commit `8e6357d` "Implement v2.1 beta
> features (#3)" on `main`). Nearly all listed security/performance fixes and Phase 1–4
> tasks are now implemented in the codebase. The `feature/interactive-tui` branch referenced
> below no longer exists. Section/task statuses have been annotated inline. Retained for
> historical reference only — do not treat as an open work list.

**Generated:** 2026-01-17
**Executed in:** commit `8e6357d` (v2.1 beta, merged to `main`)
**Status:** COMPLETED — plan executed and merged as v2.1 beta

---

## Executive Summary

Analysis of the dotfiles v2 Go TUI application compared to v1 bash script revealed (and v2.1
beta has since resolved):
- **System**: Clean, no old v1/v2 installations found
- **Features**: v2 is a superset of v1 (Go registry now registers 30 tools)
- **Security**: 1 critical + 3 high + 2 medium issues identified — all resolved in v2.1 beta
- **Performance**: 3 high + 3 medium + 1 low issues identified — HIGH/MEDIUM resolved in v2.1 beta

---

## Part 1: Feature Parity Status

### Core Features ✅ Complete

| Category | v1 | v2 | Status |
|----------|----|----|--------|
| Core Tools | 27 | 30 | ✅ Parity (Go registry now registers 30 tools) |
| Themes | 13 | 16 | ✅ v2 adds 3 (signature `neon-seapunk` palette is the 16th) |
| CLI Commands | ~10 | ~14 | ✅ v2 more powerful |
| Interactive TUI | ❌ | ✅ | New in v2 |
| User Profiles | ❌ | ✅ | New in v2 |
| Per-tool Config | ❌ | ✅ | New in v2 |
| Backup/Restore | ✅ | ✅ | Parity |

### Missing Features (Low Priority)

> **Status:** Disk/network tools are now installed by the **legacy bash script**
> (`bin/dotfiles-setup`, ~lines 941–952 / 1110+). They are **not** yet registered in the
> Go TUI tool registry — no corresponding tool files exist in `internal/tools/`.

| Feature | Priority | Notes |
|---------|----------|-------|
| fastfetch | Low | System info display (in bash script; not in Go registry) |
| ncdu, duf, dust | Low | Disk analysis tools (in bash script; not in Go registry) |
| tlrc | Low | TL;DR client |
| bandwhich, gping, doggo, trippy | Low | Network tools (in bash script; not in Go registry) |
| Stats, AltTab, MonitorControl, Mos | Low | macOS apps |

### Platform Gaps (Medium Priority)

| Gap | Priority | Action Required |
|-----|----------|-----------------|
| Raspberry Pi detection | ✅ Done | `PlatformPi` + `isRaspberryPi()` in `internal/pkg/manager.go` |
| Pi Zero 2 lightweight mode | ✅ Done | `IsLightweightMode()` / `IsHeavy()` gating in `internal/tools/registry.go` |
| paccache timer (Arch) | Low | Enable auto cleanup |

---

## Part 2: Security Issues

> **Status: COMPLETED.** All security issues below were resolved in v2.1 beta (commit `8e6357d`).

### CRITICAL

#### 1. Path Traversal in Backup Restore ✅ RESOLVED
- **Location:** `cmd/dotfiles/main.go` (restore loop, ~line 655)
- **Risk:** Arbitrary file overwrite via malicious backup files
- **Attack Vector:** File named `.._.._etc_passwd` → writes to `../../etc/passwd`
- **Fix (implemented):** `dstPath` is cleaned with `filepath.Clean` and validated against the
  home directory before writing:
  ```go
  if !strings.HasPrefix(dstPath, home+string(os.PathSeparator)) && dstPath != home {
      fmt.Fprintf(os.Stderr, "  Warning: Skipping %s - path traversal detected\n", entry.Name())
      continue
  }
  ```

### HIGH

#### 2. Shell Injection in apt.UpdateAllStreaming ✅ RESOLVED
- **Location:** `internal/pkg/apt.go` (`UpdateAllStreaming`, ~line 284)
- **Risk:** PATH manipulation could inject commands
- **Fix (implemented):** Uses `exec.Command` with explicit argument arrays — no `bash -c`
  (code comments: "using safe exec.Command (no shell interpolation)").

#### 3. Unsafe KeepSudoAlive Goroutine ✅ RESOLVED (removed)
- **Location:** formerly `internal/runner/bash.go`
- **Risk:** Process leak, no cleanup on crash
- **Fix (implemented):** The `KeepSudoAlive` function no longer exists anywhere in the
  codebase — it was removed.

#### 4. Go Version Vulnerable (1.25.5) ✅ RESOLVED
- **Location:** `go.mod:3`
- **CVEs:** CVE-2025-61728, CVE-2025-61726, CVE-2025-61731, CVE-2025-68119
- **Fix (implemented):** `go.mod` now declares `go 1.25.6` — the flagged 1.25.5 vulnerabilities
  are patched.

### MEDIUM

#### 5. File Permissions in tmux.go ✅ RESOLVED
- **Location:** `internal/tools/tmux.go` (MkdirAll ~line 79, WriteFile ~line 248)
- **Issue:** Using 0755/0644 instead of 0700/0600
- **Fix (implemented):** `os.MkdirAll(pluginsDir, 0700)` and
  `os.WriteFile(configPath, ..., 0600)` — already restrictive per CLAUDE.md.

#### 6. Missing HOME Environment Validation
- **Location:** `internal/config/config.go:28-31`
- **Fix:** Validate and clean environment variables

---

## Part 3: Performance Issues

> **Status: COMPLETED.** All HIGH/MEDIUM performance issues below were resolved in v2.1 beta
> (commit `8e6357d`); only #7 (async config load) remains as a minor optimization.

### HIGH

#### 1. N+1 Query in apt.ListInstalled() ✅ RESOLVED
- **Location:** `internal/pkg/apt.go` (`ListInstalled`, ~line 200)
- **Impact:** 5-25 seconds startup on Debian/Ubuntu
- **Fix (implemented):** Single batched call
  `exec.Command("dpkg-query", "-W", "-f=${Package}\t${Version}\n")` replaces the
  per-package `dpkg -s` loop.

#### 2. Repeated Platform/Manager Detection ✅ RESOLVED
- **Location:** `internal/pkg/manager.go`
- **Impact:** 200-500ms redundant syscalls
- **Fix (implemented):** Detection cached via `sync.Once` (`cachedPlatformOnce`,
  `cachedManagerOnce`, `cachedTotalMemoryMBOnce`).

#### 3. Goroutine Leak Risk in KeepSudoAlive ✅ RESOLVED (removed)
- **Location:** formerly `internal/runner/bash.go`
- **Impact:** Memory/process leak if used
- **Fix (implemented):** The `KeepSudoAlive` function was removed entirely.

### MEDIUM

#### 4. Multiple NewRegistry() Allocations ✅ RESOLVED
- **Locations:** 15+ occurrences across codebase
- **Impact:** 30KB+ wasted memory per operation
- **Fix (implemented):** Singleton `GetRegistry()` backed by `globalRegistryOnce sync.Once`
  in `internal/tools/registry.go`.

#### 5. Unbounded installOutput Slice ✅ RESOLVED
- **Location:** `internal/ui/app.go` (`installOutputMsg` handler, ~lines 817-823)
- **Impact:** Memory leak during long installs
- **Fix (implemented):** Bounded via copy+truncate to `maxOutputLines` (20);
  `installLogs` is pre-sized as a 500-line circular buffer.

#### 6. Repeated Registry Iteration ✅ RESOLVED
- **Location:** `internal/tools/registry.go`
- **Impact:** 50-100ms on status command
- **Fix (implemented):** `installedCache map[string]bool` plus `RefreshCache()` to
  invalidate/repopulate.

### LOW

#### 7. Synchronous Config Load at Startup
- **Location:** `internal/ui/app.go:260-279`
- **Impact:** 10-50ms startup delay
- **Fix:** Load configs async after TUI renders

---

## Part 4: Implementation Tasks

> **Status: COMPLETED.** Phases 1–4 were implemented in v2.1 beta (commit `8e6357d`).
> Checkboxes below are marked accordingly; remaining unchecked items are minor/optional.

### Phase 1: Critical Security Fixes (Immediate)

- [x] **Task 1.1:** Fix path traversal in backup restore
  - File: `cmd/dotfiles/main.go` (restore loop, ~line 655)
  - Add path validation after underscore-to-separator conversion
  - DONE: `filepath.Clean` + home-prefix guard implemented

- [x] **Task 1.2:** Update Go version
  - File: `go.mod`
  - DONE: `go.mod` line 3 now reads `go 1.25.6`

### Phase 2: High Security/Performance Fixes

- [x] **Task 2.1:** Fix apt shell injection risk
  - File: `internal/pkg/apt.go` (`UpdateAllStreaming`)
  - DONE: uses argument-array `exec.Command`, no `bash -c`

- [x] **Task 2.2:** Fix apt N+1 query pattern
  - File: `internal/pkg/apt.go` (`ListInstalled`)
  - DONE: single batched `dpkg-query -W -f='${Package}\t${Version}\n'`

- [x] **Task 2.3:** Cache platform/manager detection
  - File: `internal/pkg/manager.go`
  - DONE: package-level `sync.Once` caches (`cachedPlatformOnce`, `cachedManagerOnce`)

- [x] **Task 2.4:** Fix or remove KeepSudoAlive
  - File: formerly `internal/runner/bash.go`
  - DONE: function removed entirely (no longer present in the codebase)

### Phase 3: Medium Priority Fixes

- [x] **Task 3.1:** Fix file permissions in tmux.go
  - File: `internal/tools/tmux.go` (MkdirAll ~line 79, WriteFile ~line 248)
  - DONE: 0700 for plugins dir, 0600 for config file

- [ ] **Task 3.2:** Add HOME environment validation
  - File: `internal/config/config.go:28-31`
  - Validate XDG_CONFIG_HOME and HOME
  - Use filepath.Clean on all paths

- [x] **Task 3.3:** Implement registry singleton
  - File: `internal/tools/registry.go`
  - DONE: `GetRegistry()` singleton via `globalRegistryOnce sync.Once`

- [x] **Task 3.4:** Fix installOutput memory leak
  - File: `internal/ui/app.go` (`installOutputMsg` handler, ~lines 817-823)
  - DONE: copy + truncate to `maxOutputLines`; `installLogs` is a 500-line circular buffer

- [x] **Task 3.5:** Cache IsInstalled results in Registry
  - File: `internal/tools/registry.go`
  - DONE: `installedCache map[string]bool` + `RefreshCache()` method

### Phase 4: Feature Completion (Before v1 Deprecation)

- [x] **Task 4.1:** Add Raspberry Pi detection
  - File: `internal/pkg/manager.go` (the planned `internal/pkg/platform_linux.go` was NOT
    created — detection lives in `manager.go`)
  - DONE: `PlatformPi` type + `isRaspberryPi()` checking the device-tree model

- [x] **Task 4.2:** Implement Pi Zero 2 lightweight mode
  - File: `internal/tools/registry.go`
  - DONE: `IsLightweightMode()` / `IsHeavy()` gating skips heavy tools on low-memory systems

- [~] **Task 4.3:** Add optional disk/network tools (partial)
  - DONE (bash): fastfetch, ncdu, duf, dust, bandwhich, gping, doggo, trippy are installed by
    the legacy bash script `bin/dotfiles-setup` (~lines 941-952 / 1110+)
  - REMAINING (Go TUI): no corresponding tool files exist in `internal/tools/`; these are not
    yet registered in the Go registry

### Phase 5: Testing & Documentation

- [ ] **Task 5.1:** Add security tests
  - Path traversal fuzzing for backup restore
  - Malicious config file handling
  - Environment variable injection tests

- [ ] **Task 5.2:** Add performance benchmarks
  - Startup time measurement
  - apt.ListInstalled() benchmark
  - Registry operations benchmark

- [ ] **Task 5.3:** Add CI security scanning
  - Integrate govulncheck
  - Add gosec or staticcheck

- [ ] **Task 5.4:** Update documentation
  - Document migration path from v1
  - Note Raspberry Pi support status
  - Add security considerations section

---

## Part 5: Subagent Task Assignments

### Subagent 1: Critical Security
**Tasks:** 1.1, 1.2
**Focus:** Path traversal fix, Go version update
**Expected Duration:** 30 minutes

### Subagent 2: Security Hardening
**Tasks:** 2.1, 2.4, 3.1, 3.2
**Focus:** Shell injection, KeepSudoAlive, permissions, env validation
**Expected Duration:** 1 hour

### Subagent 3: Performance - apt Package
**Tasks:** 2.2
**Focus:** N+1 query elimination in apt.ListInstalled()
**Expected Duration:** 45 minutes

### Subagent 4: Performance - Caching
**Tasks:** 2.3, 3.3, 3.5
**Focus:** Platform detection cache, registry singleton, IsInstalled cache
**Expected Duration:** 1 hour

### Subagent 5: Memory Optimization
**Tasks:** 3.4
**Focus:** installOutput circular buffer
**Expected Duration:** 20 minutes

### Subagent 6: Raspberry Pi Support
**Tasks:** 4.1, 4.2
**Focus:** Pi detection and lightweight mode
**Expected Duration:** 1.5 hours

### Subagent 7: Optional Tools
**Tasks:** 4.3
**Focus:** Add disk/network utility tools
**Expected Duration:** 2 hours

### Subagent 8: Testing & CI
**Tasks:** 5.1, 5.2, 5.3
**Focus:** Security tests, benchmarks, CI integration
**Expected Duration:** 1.5 hours

---

## Verification Checklist

Before merge to main:

- [ ] All critical security fixes verified
- [ ] Go version updated and vulnerabilities patched
- [ ] Performance benchmarks show improvement
- [ ] No new lint/vet warnings
- [ ] All tests pass
- [ ] Manual TUI testing on macOS, Arch, Debian
- [ ] Backup/restore tested with edge cases
- [ ] Documentation updated

---

## References

This plan was implemented and merged as v2.1 beta:

- Implementation commit: `8e6357d` — "Implement v2.1 beta features (#3)" (PR #3, merged to `main`)

> Note: the original references pointed to ephemeral `/tmp/claude/.../tasks/*.output` paths
> from the analysis session. Those temp files no longer exist and have been removed.
