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

AST-based security scanning for Go code.

```bash
# Install
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Basic scan
gosec ./...

# Generate SARIF report
gosec -fmt=sarif -out=results.sarif ./...
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

Create `.github/workflows/security.yml`:

```yaml
name: Security Scanning

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  schedule:
    - cron: '0 0 * * 1'  # Weekly on Mondays

jobs:
  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Run go vet
        run: go vet ./...

      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@latest

      - name: Run govulncheck
        run: govulncheck ./...

      - name: Install gosec
        run: go install github.com/securego/gosec/v2/cmd/gosec@latest

      - name: Run gosec
        run: gosec -fmt=sarif -out=results.sarif ./...
        continue-on-error: true

      - name: Upload SARIF results
        uses: github/codeql-action/upload-sarif@v2
        if: always()
        with:
          sarif_file: results.sarif
```

## Makefile Integration

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

The following patterns are intentionally allowed:

1. **G204 (Subprocess Launch)**: Required for package manager integration
2. **G104 (Unhandled Errors)**: Some intentional fire-and-forget operations

## References

- [Go Security Best Practices](https://golang.org/doc/security/best-practices)
- [govulncheck Documentation](https://go.dev/blog/vuln)
- [gosec Rules](https://github.com/securego/gosec#available-rules)
