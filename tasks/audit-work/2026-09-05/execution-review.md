# Execution, package manager, and operation audit — 2026-09-05

Read-only audit of the current `/Users/tiki7/Desktop/Projects/dotfiles` checkout. Read root-provided instructions, internal/AGENTS.md, internal/pkg/AGENTS.md, and tasks/lessons.md. Inspected internal/runner, internal/pkg, internal/installplan, internal/installapply, internal/operation plus live UI update call paths and installation receipt collection. Production files and Git state were not changed. Isolated reproductions copy current production runner/pkg Go files to `/tmp/dotfiles-execution-audit-repro`, use fake executables, and never invoke a real package mutation.

## Findings

### EXEC-1 — Medium, newly verified: APT confuses desired selection with installed state

**Primary locations:** internal/pkg/apt.go:325-338, :112, :134. Propagation: internal/tools/installation_health.go:144-157.

`ListInstalled` trusts `dpkg --get-selections` rows whose second column is `install`; this is the wanted action, not the actual installation state. A package left unpacked/unconfigured after an interrupted installation is listed as installed. Conversely, an installed package on hold is excluded. `IsInstalledContext` and `GetVersion` also reject held packages because they require the entire Status to equal `install ok installed`, excluding legitimate `hold ok installed`.

The fresh installation collector treats successful ListInstalled output as complete authority, marking omitted receipts missing and listed receipts present without secondary checks. Consequently a damaged installation can produce no-changes/skip rather than repair, while held installed tools are offered for installation and can fail postconditions even after a package-manager no-op.

**Executed reproduction:** `TestAuditAptReceiptSelectionIsNotInstalledState` against copied production code:

- Status `hold ok installed`, selection `hold`: `IsInstalled=false`, `ListInstalled=[]`.
- Status `install ok unpacked`, selection `install`: `IsInstalled=false`, `ListInstalled=[{zsh 1.0 ... apt}]`.

**Fix:** enumerate Package, Version, actual status-state and error-state in one dpkg-query batch; distinguish installed/configured, held-installed, residual-config, unpacked, and broken states. Use coherent semantics for individual queries and list queries. Do not silently change the hold policy.

Selection/state semantics independently checked against official Debian documentation: https://www.debian.org/doc/manuals/debian-faq/pkg-basics.en and https://manpages.debian.org/trixie/dpkg/dpkg.1.en.html . No corresponding finding found in the historical July audit reports searched.

### EXEC-2 — Medium, existing gap re-confirmed by runtime reproduction: cancellation leaves package-manager descendants running

**Primary locations:** internal/runner/bash.go:101-106 and :135-157. Integration: internal/installapply/recipe.go:157-159 and :173-175; internal/installapply/service.go:220.

The live general package executor uses CommandContext without a process group. Cancelling kills only the direct process. A spawned installer/helper can retain stdout/stderr and continue mutating; scanner completion and Done remain pending until those descendants close their pipes. The apply wrapper returns context cancellation immediately and can release the install-operation lock before surviving helpers finish. Users can therefore receive a cancelled result and start another operation while work from the previous operation is still active.

**Executed reproduction:** RunStreaming starts `/bin/sh` with a child that sleeps 0.3 s then writes a marker. Cancellation after the ready line leaves Done blocked at 150 ms and the child writes `mutated` after cancellation. All fixture children exit and marker is confined to a temporary directory.

**Fix:** terminate and reap the owned process tree, make pipe draining/cancellation bounded, and wait for termination before reporting terminal status or releasing serialization. The exact npm runner has process-group handling but is not what general package manager streaming invokes. Privileged subprocess teardown needs an explicit supported contract.

**Historical status:** current confirmation of July-13 M3, not a new vulnerability claim. General scanner.Err omission and undrained-output resource behavior are also still present; avoid multiplying these into separate high findings.

### EXEC-3 — High on multi-manager hosts, existing defect re-confirmed from live source: updates discard provider ownership

**Primary locations:** internal/ui/installation.go:1996-2006, :2026. Source of mixed records: internal/pkg/update.go:28-49; internal/pkg/manager.go:214-225. Live triggers: internal/ui/screen_update.go:185, :202.

CheckAllUpdates queries every available manager and deliberately preserves Name+InstalledBy records. The live selected/displayed update executor discards InstalledBy, re-detects one default manager, and invokes it with all names. On a Linux host with Homebrew plus apt/paru, a displayed update from one provider can be sent to another provider. This can upgrade/install a different copy while leaving the intended package outdated; a zero exit marks the wrong result successful. Same-name records from different managers may be sent twice.

**Fix:** partition update requests by original provider, map official pacman/AUR ownership to a compatible captured manager, and preserve provider identity through execution, auth, postchecks, and results. Reject unavailable providers instead of falling back to a different manager.

**Verification:** source tracing of currently reachable Enter and `a` handlers; no real upgrades. Historical July-13 H2 remains applicable.

### EXEC-4 — Low/latent, existing defect re-confirmed and historical severity corrected: APT manager-wide streaming waits on undrained output

**Primary locations:** internal/pkg/apt.go:406-411; internal/runner/bash.go:94-96, :128, :141-157.

UpdateAllStreaming waits for apt update before returning its stream, but never drains the update output. The 100-line channel fills and blocks scanners; Done cannot arrive because it is produced after scanners finish. Output is discarded even below the deadlock threshold.

**Executed reproduction:** fake apt prints 120 lines; UpdateAllStreaming does not return after 200 ms and returns only when its context is cancelled.

**Current reachability correction:** both actual Updates screen invocations use `checkSudoAndUpdateCmd(packages, false)` (screen_update.go:185, :202). No true caller was found. The manager-wide wrapper remains in installation.go:2142, but the current `a` path upgrades displayed package names. July-13 H1 incorrectly elevated this to a live supported-screen blocker; current evidence supports latent Low, consistent with the older adversarial-core check. Do not present it as a currently reproducible dashboard freeze without another caller.

## Other bounded observations

- Npm-backed product execution remains intentionally disabled: installapply/recipe.go rejects InstallStepNPMGlobal before mutation; installplan rejects mixed manager/npm phases and PlanFresh does not capture an npm identity. This is an existing unfinished integration, not a newly discovered executable-authority bypass. Tool/CLI auditors should assess advertised product reachability.
- Reviewed manager identity validation openly documents its remaining revalidate-to-spawn race and does not bind every transitive interpreter/helper/network byte. Do not claim those broad boundaries are cryptographically closed or report the documented limit as a new exploit without an independent trigger.
- Operational-state parent/directory authorities, private permissions, journal atomic replacement, and accepted recipe/state hashing have substantial defensive machinery. No additional concrete exploitable state/journal defect was confirmed during this bounded read-only review.
- Cross-platform package mutations, privileged process-tree termination, and real Debian held/unconfigured package fixtures were not executed on this macOS host. APT parser behavior was reproduced with exact expected command output; official Debian documentation supplied state semantics.

## Reproduction command and results

From `/tmp/dotfiles-execution-audit-repro`:

```sh
GOCACHE=/tmp/dotfiles-audit-2026-09-05/go-cache go test -v ./internal/runner ./internal/pkg -run TestAudit -count=1
```

All three isolated reproduction tests passed, meaning the asserted defects were observed. The final run reported runner 0.586 s and pkg 1.301 s. These are defect reproductions, not evidence the production behavior is correct. Parent audit owns full repository build/vet/test/race evidence.
