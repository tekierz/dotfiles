# Release process

Tagged releases are built by `.github/workflows/release.yml` from the exact tag
commit. The workflow rejects malformed semantic-version tags and tags whose
commit is not contained in `main`. It invokes the same reusable CI workflow as pull requests and integration
pushes: module, formatting, vet, golangci-lint, ShellCheck, Staticcheck,
govulncheck, Gitleaks, workflow/release validation, and normal/race tests and
binary smoke checks on both Ubuntu and macOS. Publication requires that
complete gate to succeed at the exact tag commit. Release tools are installed at pinned versions, not moving
`latest` selectors.

## Artifacts

Every release produces static `darwin` and `linux` binaries for `amd64` and
`arm64`, wrapped in deterministic `tar.gz` archives with the project license,
README, third-party notices, and dependency license texts. It also publishes a
source archive and one SPDX JSON SBOM per archive/source artifact. The workflow
creates `dotfiles_release_checksums.txt` over all five archives and all five
SBOMs, verifies it, and records GitHub provenance for every named artifact.
GoReleaser keeps the release in draft until those checks and attestations
succeed.

The Go binaries and their archives use the commit timestamp, `-trimpath`, and
an empty Go build ID so repeated builds from the same source and toolchain are
byte-identical. SBOM documents carry generator metadata; their exact bytes are
covered by the release checksum and attestation rather than described as
reproducible across separate generation runs.

Consumers can verify a downloaded archive with:

```bash
sha256sum --check dotfiles_release_checksums.txt
gh attestation verify dotfiles_VERSION_OS_ARCH.tar.gz \
  --repo tekierz/dotfiles
```

On macOS, use `shasum -a 256 -c dotfiles_release_checksums.txt` when GNU `sha256sum` is
not installed.

## Local rehearsal

Install GoReleaser v2.17.0 and Syft v1.44.0, then run:

```bash
make release-check
make release
```

This creates an unpublished snapshot under `dist/`. A release candidate is not
accepted merely because the cross-build succeeds: the pre-PR checklist, owner-
hardware test matrix, and Homebrew install/upgrade/rollback trial must also pass.

## Remaining distribution gates

- The external `tekierz/homebrew-tap` is a hard tag gate. As verified on
  2026-07-16, its `Formula/dotfiles.rb` still targets v2.0.1 and unconditionally
  unlinks `dotfiles-tui` and `dotfiles-setup` executables by basename during
  install. Remove that unowned deletion, update the formula from the published
  archive checksum, correct its stale feature/command documentation, and test a
  clean install plus an upgrade from the last supported version.
- The repository's current owner settings do not yet provide a complete public
  release boundary. Require the stable `Release Gate` check with strict
  up-to-date branches, enable private vulnerability reporting, secret scanning
  and push protection, and enable the dependency graph plus Dependabot security
  updates before publishing.
- GitHub provenance is configured, but Apple Developer ID signing/notarization is
  not possible until release credentials and an ownership policy are provisioned.
  Do not describe standalone macOS archives as notarized before that gate exists.
- Keep the retired Bash installer absent from source, packaging, CI, and active
  installation docs. `TestLegacyInstallerIsNotDistributed` enforces the local
  distribution invariant.

## September toolchain baseline

`go.mod` pins Go 1.26.8, a supported patched release verified against the
[Go release history](https://go.dev/doc/devel/release#go1.26.0).
The pinned golangci-lint v2.11.4 supports Go 1.26; see the
[upstream changelog](https://golangci-lint.run/docs/product/changelog/).
Local verification must use the pinned toolchain and matching analysis tools.
