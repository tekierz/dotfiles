# Dotfiles v2 Analysis & Remediation Plan

**Generated:** 2026-01-17
**Branch:** feature/interactive-tui
**Status:** 98% feature complete, requires security/performance fixes before production

---

## Executive Summary

Analysis of the dotfiles v2 Go TUI application compared to v1 bash script revealed:
- **System**: Clean, no old v1/v2 installations found
- **Features**: 98% complete, v2 is a superset of v1
- **Security**: 1 critical + 3 high + 2 medium issues identified
- **Performance**: 3 high + 3 medium + 1 low issues identified

---

## Part 1: Feature Parity Status

### Core Features ✅ Complete

| Category | v1 | v2 | Status |
|----------|----|----|--------|
| Core Tools | 27 | 27 | ✅ Parity |
| Themes | 13 | 16 | ✅ v2 adds 3 |
| CLI Commands | ~10 | ~14 | ✅ v2 more powerful |
| Interactive TUI | ❌ | ✅ | New in v2 |
| User Profiles | ❌ | ✅ | New in v2 |
| Per-tool Config | ❌ | ✅ | New in v2 |
| Backup/Restore | ✅ | ✅ | Parity |

### Missing Features (Low Priority)

| Feature | Priority | Notes |
|---------|----------|-------|
| fastfetch | Low | System info display |
| ncdu, duf, dust | Low | Disk analysis tools |
| tlrc | Low | TL;DR client |
| bandwhich, gping, doggo, trippy | Low | Network tools |
| Stats, AltTab, MonitorControl, Mos | Low | macOS apps |

### Platform Gaps (Medium Priority)

| Gap | Priority | Action Required |
|-----|----------|-----------------|
| Raspberry Pi detection | Medium | Add Pi model auto-detection |
| Pi Zero 2 lightweight mode | Medium | Skip heavy tools on low-memory Pi |
| paccache timer (Arch) | Low | Enable auto cleanup |

---

## Part 2: Security Issues

### CRITICAL

#### 1. Path Traversal in Backup Restore
- **Location:** `cmd/dotfiles/main.go:652`
- **Risk:** Arbitrary file overwrite via malicious backup files
- **Attack Vector:** File named `.._.._etc_passwd` → writes to `../../etc/passwd`
- **Fix Required:**
  ```go
  // Validate path stays within home directory
  dstPath = filepath.Clean(dstPath)
  if !strings.HasPrefix(dstPath, home) {
      fmt.Fprintf(os.Stderr, "  Security: Skipping path outside home: %s\n", relPath)
      continue
  }
  ```

### HIGH

#### 2. Shell Injection in apt.UpdateAllStreaming
- **Location:** `internal/pkg/apt.go:254`
- **Risk:** PATH manipulation could inject commands
- **Fix:** Use exec.Command with argument arrays instead of bash -c

#### 3. Unsafe KeepSudoAlive Goroutine
- **Location:** `internal/runner/bash.go:78-93`
- **Risk:** Process leak, no cleanup on crash
- **Fix:** Rewrite using Go's time.Ticker with context cancellation

#### 4. Go Version Vulnerable (1.25.5)
- **Location:** `go.mod:3`
- **CVEs:** CVE-2025-61728, CVE-2025-61726, CVE-2025-61731, CVE-2025-68119
- **Fix:** Update to Go 1.25.6+

### MEDIUM

#### 5. File Permissions in tmux.go
- **Location:** `internal/tools/tmux.go:75,244`
- **Issue:** Using 0755/0644 instead of 0700/0600
- **Fix:** Change to restrictive permissions per CLAUDE.md

#### 6. Missing HOME Environment Validation
- **Location:** `internal/config/config.go:28-31`
- **Fix:** Validate and clean environment variables

---

## Part 3: Performance Issues

### HIGH

#### 1. N+1 Query in apt.ListInstalled()
- **Location:** `internal/pkg/apt.go:196-221`
- **Impact:** 5-25 seconds startup on Debian/Ubuntu
- **Current:** Individual `dpkg -s` call per package
- **Fix:** Use `dpkg-query -W -f='${Package}\t${Version}\n'` for batch lookup

#### 2. Repeated Platform/Manager Detection
- **Location:** `internal/tools/tool.go:71-89`
- **Impact:** 200-500ms redundant syscalls
- **Fix:** Cache detection results in package-level singleton

#### 3. Goroutine Leak Risk in KeepSudoAlive
- **Location:** `internal/runner/bash.go:78-93`
- **Impact:** Memory/process leak if used
- **Fix:** Remove unused function or rewrite properly

### MEDIUM

#### 4. Multiple NewRegistry() Allocations
- **Locations:** 15+ occurrences across codebase
- **Impact:** 30KB+ wasted memory per operation
- **Fix:** Use singleton pattern with sync.Once

#### 5. Unbounded installOutput Slice
- **Location:** `internal/ui/app.go:248,610-615`
- **Impact:** Memory leak during long installs
- **Fix:** Use circular buffer pattern (like installLogs)

#### 6. Repeated Registry Iteration
- **Location:** `internal/tools/registry.go:104-243`
- **Impact:** 50-100ms on status command
- **Fix:** Cache IsInstalled() results with Refresh() method

### LOW

#### 7. Synchronous Config Load at Startup
- **Location:** `internal/ui/app.go:260-279`
- **Impact:** 10-50ms startup delay
- **Fix:** Load configs async after TUI renders

---

## Part 4: Implementation Tasks

### Phase 1: Critical Security Fixes (Immediate)

- [ ] **Task 1.1:** Fix path traversal in backup restore
  - File: `cmd/dotfiles/main.go:652`
  - Add path validation after underscore-to-separator conversion
  - Add test for malicious path handling

- [ ] **Task 1.2:** Update Go version
  - File: `go.mod`
  - Update from 1.25.5 to 1.25.6+
  - Run `go mod tidy`

### Phase 2: High Security/Performance Fixes

- [ ] **Task 2.1:** Fix apt shell injection risk
  - File: `internal/pkg/apt.go:254`
  - Replace bash -c with direct exec.Command calls
  - Add test for UpdateAllStreaming

- [ ] **Task 2.2:** Fix apt N+1 query pattern
  - File: `internal/pkg/apt.go:196-221`
  - Replace per-package dpkg -s with batch dpkg-query
  - Benchmark before/after

- [ ] **Task 2.3:** Cache platform/manager detection
  - File: `internal/pkg/manager.go`
  - Add package-level singleton with sync.Once
  - Update `internal/tools/tool.go` to use cached values

- [ ] **Task 2.4:** Fix or remove KeepSudoAlive
  - File: `internal/runner/bash.go:78-93`
  - Check if function is used anywhere
  - If unused: remove entirely
  - If used: rewrite with Go time.Ticker and context

### Phase 3: Medium Priority Fixes

- [ ] **Task 3.1:** Fix file permissions in tmux.go
  - File: `internal/tools/tmux.go:75,244`
  - Change 0755 → 0700 for directories
  - Change 0644 → 0600 for config files

- [ ] **Task 3.2:** Add HOME environment validation
  - File: `internal/config/config.go:28-31`
  - Validate XDG_CONFIG_HOME and HOME
  - Use filepath.Clean on all paths

- [ ] **Task 3.3:** Implement registry singleton
  - File: `internal/tools/registry.go`
  - Add sync.Once initialization
  - Update all 15+ NewRegistry() call sites

- [ ] **Task 3.4:** Fix installOutput memory leak
  - File: `internal/ui/app.go:248,610-615`
  - Use circular buffer pattern (copy + truncate)
  - Match installLogs implementation

- [ ] **Task 3.5:** Cache IsInstalled results in Registry
  - File: `internal/tools/registry.go:104-243`
  - Add installed cache map
  - Add Refresh() method to invalidate

### Phase 4: Feature Completion (Before v1 Deprecation)

- [ ] **Task 4.1:** Add Raspberry Pi detection
  - Files: `internal/pkg/manager.go`, new `internal/pkg/platform_linux.go`
  - Detect Pi model from /proc/cpuinfo or /sys/firmware
  - Add Platform type for Pi variants

- [ ] **Task 4.2:** Implement Pi Zero 2 lightweight mode
  - Files: `internal/tools/registry.go`, affected tool files
  - Skip heavy tools (yazi, btop) on low-memory Pi
  - Add memory threshold detection

- [ ] **Task 4.3:** Add optional disk/network tools
  - Files: New tool files in `internal/tools/`
  - Add: fastfetch, ncdu, duf, dust
  - Add: bandwhich, gping, doggo, trippy
  - Mark as optional with platform filtering

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

- Feature Parity Agent Output: `/tmp/claude/-home-tiki-projects-dotfiles/tasks/aed1301.output`
- Git Analysis Agent Output: `/tmp/claude/-home-tiki-projects-dotfiles/tasks/a3edc9e.output`
- Security Agent Output: `/tmp/claude/-home-tiki-projects-dotfiles/tasks/ad6bb1d.output`
- Performance Agent Output: `/tmp/claude/-home-tiki-projects-dotfiles/tasks/a08c00d.output`
