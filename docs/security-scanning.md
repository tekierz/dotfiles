# Security Scanning

This document describes how to integrate security scanning tools into the dotfiles project.

## Quick Start

Run all security checks locally:

```bash
# Install tools
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Run checks
go vet ./...
govulncheck ./...
gosec ./...
```

## Tools Overview

### 1. govulncheck - Vulnerability Detection

Scans Go binaries and source code for known vulnerabilities in dependencies.

```bash
# Install
go install golang.org/x/vuln/cmd/govulncheck@latest

# Scan current module
govulncheck ./...

# Scan with JSON output
govulncheck -json ./...
```

### 2. go vet - Static Analysis

Built-in static analysis that catches common mistakes.

```bash
go vet ./...
```

### 3. gosec - Security Linting

AST-based security scanning for Go code. In this repo, gosec is **not** installed or run as a standalone CI step. It runs only as one enabled sub-linter inside golangci-lint, configured in `.golangci.yml` (the `gosec` linter). Running it standalone is purely a local/optional convenience:

```bash
# Optional local-only install + standalone scan (NOT part of CI)
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...
```

**Common Rules:**
- G101: Hardcoded credentials
- G102: Bind to all interfaces
- G104: Unhandled errors
- G107: URL provided as taint input
- G201-G203: SQL injection
- G301-G307: File permission issues
- G401-G406: Crypto issues

## GitHub Actions Workflow

Security and lint checks live in the existing `.github/workflows/ci.yml` (there is **no** standalone `security.yml`). CI triggers on `push` to `main`/`master` and on all `pull_request` events — there is no cron/weekly schedule.

Two jobs are relevant:

### `security` job

Runs govulncheck only. Note `continue-on-error: true`, so a vulnerability finding does **not** block merges:

```yaml
  security:
    name: Security
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Run govulncheck
        continue-on-error: true  # May have issues with latest Go
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
```

### `lint` job

Runs formatting and static analysis. The gofmt check and `go vet` **hard-fail** the build; golangci-lint (which includes the gosec sub-linter) and staticcheck both use `continue-on-error: true` and do not block merges:

```yaml
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - name: Check formatting
        run: |
          if [ -n "$(gofmt -l .)" ]; then
            echo "::error::Code is not formatted. Run 'gofmt -w .'"
            gofmt -l .
            exit 1
          fi

      - name: Run go vet
        run: go vet ./...

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v4
        continue-on-error: true  # May not support latest Go yet
        with:
          version: latest
          args: --timeout=5m

      - name: Run staticcheck
        uses: dominikh/staticcheck-action@v1
        continue-on-error: true  # May not support latest Go yet
        with:
          version: latest
          install-go: false
```

> Note: there is no gosec install/run step, no SARIF generation, and no `github/codeql-action/upload-sarif` step in this repo. gosec coverage comes solely from golangci-lint.

## Makefile Integration

The current `Makefile` does **not** define `security`, `vet`, `vuln`, or `gosec` targets. The only relevant target is `make lint`, which runs golangci-lint (including the gosec sub-linter):

```makefile
# Lint code (requires golangci-lint)
lint:
	golangci-lint run
```

The following targets are a **suggested/optional** addition (not currently present in the Makefile) if you want one-shot local security checks:

```makefile
.PHONY: security vet vuln gosec

security: vet vuln gosec

vet:
	go vet ./...

vuln:
	@which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

gosec:
	@which gosec > /dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec ./...
```

## Security Best Practices Applied

### Path Traversal Prevention

The `restoreBackup` function validates paths before restoration:

```go
dstPath := filepath.Clean(filepath.Join(home, relPath))
if !strings.HasPrefix(dstPath, home+string(os.PathSeparator)) && dstPath != home {
    // Skip - path traversal detected
    continue
}
```

### Shell Injection Prevention

Package manager commands use `exec.Command` with separate arguments instead of `bash -c`:

```go
// Good - safe
cmd := exec.Command("apt", "update")

// Bad - vulnerable to injection
cmd := exec.Command("bash", "-c", "apt " + userInput)
```

### File Permissions

- Config directories: 0700 (owner read/write/execute only)
- Config files: 0600 (owner read/write only)
- Never expose sensitive data in world-readable files

### Input Validation

- Validate all user input before use
- Sanitize file paths with `filepath.Clean`
- Use parameterized commands where possible

## Known Exclusions

The following gosec rules are intentionally excluded in `.golangci.yml` (`gosec.excludes`):

1. **G104 (Unhandled Errors)**: Audit errors not checked — too noisy for a TUI with intentional fire-and-forget operations
2. **G304 (File Path as Taint Input)**: Expected for a dotfiles tool that reads/writes user-supplied config paths

## References

- [Go Security Best Practices](https://golang.org/doc/security/best-practices)
- [govulncheck Documentation](https://go.dev/blog/vuln)
- [gosec Rules](https://github.com/securego/gosec#available-rules)
