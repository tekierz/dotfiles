# dotfiles v3.0.0

This major release makes the Go terminal manager the supported application and
introduces reviewed installation plans, configuration recovery, and verifiable
release artifacts.

## Breaking changes and migration

- The Bash `bin/dotfiles-setup` and PowerShell `bin/dotfiles-setup.ps1` installers
  have been removed. Windows is unsupported. Use the Go `dotfiles` application
  on macOS or supported Linux distributions; do not run retired installers from
  historical raw URLs.
- Before migrating, preserve your configuration and the entire
  `~/.config/dotfiles/backups/` directory independently. Install through
  `tekierz/tap/dotfiles` or a verified release artifact, then run
  `dotfiles doctor` to inspect executable ownership and PATH collisions.
  Doctor does not delete legacy-named executables. Review the
  [migration guide](https://github.com/tekierz/dotfiles/blob/v3.0.0/docs/legacy-migration.md)
  before applying changes or restoring historical backups; legacy manifests
  are not automatically interchangeable.
- `dotfiles uninstall` restores an available backup and provides manual cleanup
  guidance. It retains binaries, packages, helpers, and dotfiles state;
  `--force` skips confirmation and does not enable automatic deletion.

## Installation and configuration

- `dotfiles plan --json --tool <id>` collects an explicit installation plan.
  `dotfiles apply --yes --plan-hash <hash> --tool <id>` collects fresh observations
  and applies only the same reviewed authority. Repeat the exact tool selection.
  This CLI path installs or repairs reviewed packages; it does not apply themes
  or write application configuration.
- Node prerequisites and npm global installation use separate phases. When a
  prerequisite phase completes, apply exits 3 and requires a fresh plan before
  the npm phase. Codex and Pi installations use the pinned package arguments
  shown in their plans; installation does not authenticate either tool.
- Configuration changes use reviewed targets and bounded recovery data.
  Backups preserve supported file bytes, directory structure, and POSIX modes;
  they do not preserve every filesystem attribute or replace a machine backup.
  Package-only operations do not claim a filesystem rollback point.
- Backup listing and restore validate the selected recovery data. Incomplete
  restore is reported, and recovery data remains available after uninstall.

## Support and downloads

`dotfiles support --json` writes one bounded, redacted document to stdout for
your review. It does not create an archive or upload anything. Inspect it before
sharing; Doctor JSON, raw journals, configuration files, and logs remain private
diagnostic material outside this sharing format.

Downloads include macOS and Linux archives for amd64 and arm64, a source archive,
SPDX SBOMs, SHA256 checksums, and GitHub build provenance. Standalone macOS
archives are not Developer ID signed or notarized. See the
[verification instructions](https://github.com/tekierz/dotfiles/blob/v3.0.0/docs/releasing.md).

Hosted ARM64 checks do not establish physical Raspberry Pi compatibility.
Temporary-home recovery and credential-free tool/MCP startup do not establish
whole-disk recovery, owner-terminal behavior, or authenticated integration
acceptance; those remain separate release gates.
