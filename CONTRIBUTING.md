# Contributing

Thank you for helping improve `dotfiles`.

## Before You Start

- Search existing issues and pull requests before proposing duplicate work.
- Use an issue for a significant feature or behavior change before investing in
  an implementation.
- Keep changes focused. Avoid unrelated cleanup in the same pull request.
- Never include credentials, machine-specific configuration, private logs, or
  personal data.
- Security vulnerabilities must follow [SECURITY.md](SECURITY.md), not the
  public issue tracker.

Participation in this project is governed by
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Development Setup

The supported product is the Go application in `cmd/dotfiles`.

Requirements:

- the Go version declared in `go.mod`;
- Git;
- Make for the documented convenience targets; and
- a supported macOS or Linux environment for manual platform testing.

Build and run:

```bash
go mod download
make build
./bin/dotfiles --help
```

Useful checks:

```bash
go test ./...
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
```

See `Makefile` and `docs/security-scanning.md` for the complete automated
checks. Platform-dependent changes should also be exercised manually on each
affected operating system.

## Project Guidelines

- Preserve the explicit plan/apply boundary for mutating operations.
- Run the application as the target user, not through `sudo`.
- Do not source user shell profiles to discover settings.
- Use exact argument vectors rather than shell command construction.
- Treat paths, package-manager output, configuration, and detected executables
  as untrusted input.
- Keep support-sharing output bounded and redacted.
- Prefer small changes with clear failure behavior and useful diagnostics.

Read the nearest `AGENTS.md` or package guidance before changing an internal
subsystem.

## Pull Requests

A good pull request:

1. explains the user-visible problem and solution;
2. identifies affected platforms;
3. includes focused tests for changed behavior;
4. documents manual verification and remaining limitations;
5. updates public documentation when behavior changes; and
6. updates `THIRD_PARTY_NOTICES.md` and `LICENSES/` when linked dependencies
   or their license terms change; and
7. leaves generated binaries, local configuration, and secrets out of Git.

Use a clean, descriptive commit history. Maintainers may ask for changes,
squashing, or additional platform evidence before merging.

By submitting a contribution, you agree that it may be distributed under the
project's [MIT License](LICENSE).

## Reporting Bugs

Use the bug report form and include:

- the `dotfiles version` output;
- operating system, distribution, and architecture;
- the exact command or TUI flow;
- expected and actual behavior; and
- minimal, redacted logs or `dotfiles support --json` output.

Inspect all diagnostic material before posting it. Do not attach raw
configuration, backups, operation journals, or `doctor --json`.
