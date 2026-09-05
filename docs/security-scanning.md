# Security and quality gates

The blocking workflow is `.github/workflows/ci.yml`. It runs on pull requests
and pushes to `main` or `master`; tagged releases rerun the core gates in
`.github/workflows/release.yml` before a draft can be published.

## Pinned tooling

CI installs tools at explicit versions:

| Tool | Version | Purpose |
|------|---------|---------|
| Go toolchain | `go.mod` | Build, test, race, vet, module verification |
| golangci-lint | v2.5.0 | Multi-linter and gosec gate |
| Staticcheck | v0.7.0 | Go 1.25-aware static analysis |
| govulncheck | v1.1.4 | Reachable Go vulnerability analysis |
| Gitleaks | v8.30.1 | Secret scanning with a reviewed narrow fixture allowlist |
| GoReleaser | v2.17.0 | Release-configuration validation |
| actionlint | v1.7.12 | GitHub Actions syntax and expression validation |
| ShellCheck | runner package | Maintained shell-script analysis |

GitHub Actions are referenced by immutable commit SHA. Dependabot checks Go
modules and Actions weekly, but updates still require normal review and CI.
CodeQL analyzes Go on pull requests, `main`, and a weekly schedule. Dependency
Review rejects pull requests that introduce moderate-or-higher known
vulnerabilities or AGPL-only dependencies.

## Local checks

Run the same core checks before opening a pull request:

```bash
go mod verify
go mod tidy -diff
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go test -race ./...
golangci-lint run --timeout=5m
staticcheck ./...
govulncheck ./...
gitleaks git --redact --no-banner .
gitleaks dir --redact --no-banner .
shellcheck scripts/*.sh tests/*.sh
actionlint .github/workflows/*.yml
goreleaser check
```

Install the pinned development tools with `make dev-deps`. GoReleaser and Syft
are intentionally separate release dependencies; their required versions are
shown by `make release-check` and pinned in the release workflow.

## What is blocking

- Formatting, module drift, vet, golangci-lint, Staticcheck, ShellCheck,
  actionlint, and GoReleaser configuration failures fail the lint job.
- A reachable vulnerability or source secret finding fails the security job.
- Build, normal tests, and race tests must pass on both Ubuntu and macOS.
- The stable `Release Gate` check succeeds only when lint, vulnerability,
  platform test, and platform build jobs all succeed.
- A tag remains a draft until exact platform archives, executable layout and
  version, SPDX SBOMs, checksums, and GitHub provenance all validate.

Repository settings must require the stable `Release Gate` check on an
up-to-date branch. GitHub secret scanning, push protection, Dependabot security
updates, the dependency graph used by Dependency Review, and private
vulnerability reporting are owner-controlled repository settings and should be
enabled before a public release.

There is no `continue-on-error` security or lint gate. Do not weaken a gate to
make a release green. A narrowly justified exclusion must identify the exact
rule and scope; broad diagnostic-text suppressions are release debt and are
tracked for removal.

## Security boundaries

### Filesystem mutation

Product writes use descriptor-anchored, no-follow operations and atomic
replacement. Plans observe exact targets and digests; existing targets require a
verified rollback point before execution. Paths are not made safe merely by
calling `filepath.Clean` or checking a string prefix.

### Process execution

Package-manager and tool invocations pass arguments directly to `exec.Command`
or the typed runner. Sourced shell fragments must encode values as inert data;
they must not interpolate user-controlled text as shell syntax.

### Configuration and secrets

Product configuration directories are private (`0700`) and state files are
private (`0600`). Plans and journals store hashes, provenance, and sanitized
summaries—not configuration contents, tokens, or environment values. AI tools
remain opt-in and must not silently run mutable remote installers.

### Supply chain

Release builds are static, use `-trimpath`, publish SHA-256 manifests and SPDX
SBOMs, and receive GitHub artifact attestations. Apple signing/notarization and
the external Homebrew formula upgrade/rollback trial remain separate hard gates;
see `docs/releasing.md`.

## Known policy decisions

golangci-lint's gosec integration currently excludes:

- `G104`, because intentionally ignored UI cleanup errors are reviewed through
  narrower error-handling checks and tests.
- `G304`, because this local configuration manager must open user-selected paths;
  safe anchored-path primitives and ownership tests are the controlling boundary.

These rule exclusions do not permit symlink following, unsafe permissions,
unchecked mutation failures, or shell execution of path content.

## References

- [Go vulnerability management](https://go.dev/doc/security/vuln/)
- [govulncheck](https://go.dev/blog/vuln)
- [GoReleaser supply-chain guidance](https://goreleaser.com/blog/supply-chain-security/)
- [GitHub artifact attestations](https://docs.github.com/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds)
