# Release process

Tagged releases are built by `.github/workflows/release.yml` from the exact tag
commit. The workflow reruns module, formatting, vet, normal-test, and race-test
gates before publishing anything. GoReleaser and Syft are installed at pinned
versions, not moving `latest` selectors.

## Artifacts

Every release produces static `darwin` and `linux` binaries for `amd64` and
`arm64`, wrapped in deterministic `tar.gz` archives with the license and README.
It also publishes a source archive and one SPDX JSON SBOM per archive/source artifact.
The workflow creates `dotfiles_release_checksums.txt` over all five archives and all
five SBOMs, verifies it, and records GitHub provenance for every named artifact.
GoReleaser keeps the release in draft until those checks and attestations succeed.

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
  2026-07-10, its `Formula/dotfiles.rb` still targets v2.0.1 and unconditionally
  unlinks `dotfiles-tui` and `dotfiles-setup` executables by basename during
  install. Remove that unowned deletion, update the formula from the published
  archive checksum, correct its stale feature/command documentation, and test a
  clean install plus an upgrade from the last supported version.
- GitHub provenance is configured, but Apple Developer ID signing/notarization is
  not possible until release credentials and an ownership policy are provisioned.
  Do not describe standalone macOS archives as notarized before that gate exists.
- Keep the retired Bash installer absent from source, packaging, CI, and active
  installation docs. `TestLegacyInstallerIsNotDistributed` enforces the local
  distribution invariant.
