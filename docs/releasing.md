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

- The ownership-only `tekierz/homebrew-tap` repair merged on 2026-09-05 as
  `bc3dd989` and passed post-merge hosted acceptance. It still distributes v2.0.1.
  Candidate `7214af5` passed clean install, v2.0.1 upgrade and exact older-package
  rollback in hosted macOS acceptance. After publication update its immutable
  source/checksum. Candidate formula builds must explicitly set
  `ENV["GOTOOLCHAIN"] = "go1.26.8"` because Homebrew's build environment otherwise
  selects its installed Go version.
- Main now requires GitHub Actions' stable `Release Gate` and GitHub Advanced Security's
  `CodeQL` findings check with strict freshness
  and administrator enforcement. Private vulnerability reporting, secret scanning,
  push protection, dependency graph/alerts, and Dependabot security updates are
  enabled and verified. Preserve those controls during promotion.
- Owner terminal/font/mouse, whole-disk snapshot recovery, physical Raspberry Pi
  and authenticated integration acceptance remain required observations. Hosted
  Linux ARM64 userspace and synthetic-home recovery do not certify those gates.
- GitHub provenance is configured, but Apple Developer ID signing/notarization is
  not possible until release credentials and an ownership policy are provisioned.
  Do not describe standalone macOS archives as notarized before that gate exists.
- Keep the retired Bash installer absent from source, packaging, CI, and active
  installation docs. `TestLegacyInstallerIsNotDistributed` enforces the local
  distribution invariant.

## Prepared v3.0.0 publication sequence

The current owner request authorizes publication. Draft promotion PR #6 remains
open because physical Pi, owner terminal, tested whole-disk recovery and
authenticated integration observations are unavailable. Record those results
against the candidate before proceeding.

1. Complete the remaining acceptance and check the final PR head against current
   main. Require fresh Release Gate and CodeQL findings checks and all disposable
   platform acceptance jobs. Mark PR #6 ready and merge its exact reviewed head.
2. Dispatch `platform-acceptance.yml` with `--ref main` and wait for all five jobs
   on the promoted commit. The push trigger covers integration branches, so a
   main promotion needs this explicit dispatch. Recheck main's exact commit.
3. Create and push only `v3.0.0` at that tested main commit. The existing Release
   workflow checks the exact tag, uses
   [authored notes](release-notes-v3.0.0.md), and publishes only after CI,
   checksums and GitHub provenance succeed. Future tags require their own
   `docs/release-notes-vVERSION.md`; a missing file fails the release build.
4. Download the published five archives and five SBOMs, verify the complete
   checksum manifest and GitHub attestations, and check the actual binaries'
   reported version. Inspect the published notes and asset inventory.
5. Update the tap formula to version 3.0.0 and the precise immutable source URL
   selected for that release. Hash those exact downloaded bytes: GitHub's tag
   tarball and GoReleaser's custom source archive have different content and
   interchangeable hashes must not be assumed. Preserve the merged ownership
   repair and explicit Go toolchain setting. Run and review the tap's hosted
   formula tests and actual install/upgrade/rollback before merging its update.

The local clean-clone rehearsal at `7214af5` verified four platform binaries,
five archives, five SPDX documents, all ten checksums, licenses, source contents,
and `vcs.modified=false`. It is unpublished evidence. Go 1.26.8 omits VCS build
metadata from linked worktrees with a `.git` pointer file; use an ordinary clean
clone for this provenance-sensitive rehearsal.

## September toolchain baseline

`go.mod` pins Go 1.26.8, a supported patched release verified against the
[Go release history](https://go.dev/doc/devel/release#go1.26.0).
The pinned golangci-lint v2.11.4 supports Go 1.26; see the
[upstream changelog](https://golangci-lint.run/docs/product/changelog/).
Local verification must use the pinned toolchain and matching analysis tools.
