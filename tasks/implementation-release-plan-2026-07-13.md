# Implementation and Release Program — 2026-07-13

Status: **canonical execution plan; implementation not started**

Audit input: [release-audit-2026-07-13.md](release-audit-2026-07-13.md)

This plan replaces the old linear remediation checklist as the source of truth for
remaining implementation and release work. The older checklist remains below the active
summary in tasks/todo.md as historical evidence only.

## 1. Outcome

Deliver one release candidate that:

- closes every confirmed High and Medium finding from the 2026-07-13 audit;
- makes package, cask, and npm install intent truthful and executable end to end;
- never mutates a prerequisite and npm payload under one stale confirmation;
- preserves unowned configuration and restores the exact backup the user selected;
- has deterministic CLI grammar, exits, public JSON, journals, and cancellation;
- uses one complete quality gate for PR, main, rehearsal, and tag builds;
- ships immutable, checksummed, attested artifacts and ownership-safe Homebrew formulas;
- passes disposable-platform, owner Apply, owner Save, Homebrew lifecycle, and canary gates.

Default target: **v2.2.0**. If compatibility review finds an unavoidable breaking change to
a contract already shipped in v2.0.2, the target becomes v3.0.0. Existing tags are never
moved or reused.

The project remains No-Go until every release gate in section 10 is closed against the
same exact candidate.

## 2. Scope control

### In this release

- streaming/process-tree kernel, Debian Update All, and multi-manager routing;
- immutable restore authority and valid uninstall recovery selection;
- sshh bootstrap and SSH-option safety;
- bounded structured reads;
- pinned remote configuration payloads and MCP commands;
- deterministic CLI contracts and registry/update/docs parity;
- Deep Dive viewport and mouse reachability;
- complete two-phase npm planning, execution, CLI, wizard, and Manage truth;
- reusable CI, explicit toolchains, protected draft/publish, artifact verification;
- dotfiles and sshh Homebrew formula remediation;
- manual platform, owner Apply/Save, and staged deployment evidence.

### Explicitly deferred

These cannot expand a release slice unless a named gate proves they are required:

- product rename;
- adding more integrations or making Cursor Agent/Hermes automatically installable;
- plugin system, profiles/import-export, Windows support, or a full settings redesign;
- broad ScreenContext/App decomposition or typed-screen refactor;
- custom Glow style editing;
- theme palette consolidation, cosmetic deduplication, and micro-performance cleanup;
- Bubble Tea/Lip Gloss v2 migration;
- production-enterprise claims.

Cursor Agent and Hermes remain discovery-only. AI tools remain opt-in. Automatic legacy
binary deletion remains prohibited.

## 3. Parallel execution model

The repository guardrails still apply to every slice:

planned -> contract-frozen -> tests-red -> implemented -> reviewed -> verified -> committed

Implementation uses isolated Git worktrees and slice branches. Git creation, commits, pushes,
and PRs require explicit user authorization when execution begins.

With four available agent slots:

1. Root owns integration, the canonical plan, verification, and Git actions.
2. One Sol agent freezes and reviews up to two disjoint slices.
3. Terra A owns writer worktree A.
4. Terra B owns writer worktree B.

After implementation, the two Terra slots stop writing and become independent adversarial
review slots for the opposite slice. Root integrates only after both reviews and the exact
candidate gates pass. There are never more than two simultaneous writers.

Each worktree has its own branch-local tasks/current-slice.scope. Parallel slices must have
disjoint production and test allowlists. A shared path forces serialization. Root never
resolves overlapping writes by hand; the affected slices are re-planned.

### Pipeline cadence

- Sol freezes red-test contracts for the next pair while root verifies the previous pair.
- Terra A and Terra B implement concurrently.
- Reviews run concurrently after both writers stop.
- Root integrates in dependency order and runs the merge barrier.
- Evidence expires after any production edit.

## 4. Dependency graph

~~~mermaid
flowchart TD
  G0["G0 Plan and state reset"] --> G1["G1 Enforce slice-state gate"]
  G1 --> RK["RK Runner kernel"]
  G1 --> BA["BA Backup authority"]
  G1 --> SH["SH sshh contract"]
  RK --> RS["RS Sequential streaming"]
  RS --> AP["AP Debian Update All"]
  RS --> UR["UR Manager-bound routing"]
  BA --> BC["BC CLI/TUI restore consumers"]
  BC --> BU["BU Valid uninstall recovery"]
  RK --> CX["CX Installer cancellation"]
  G1 --> BR["BR Bounded readers"]
  MB1 --> SP["SP Remote payload pins"]
  MB1 --> NP1["NP1 npm phase domain"]
  NP1 --> NP2["NP2 Public plan projection"]
  RK --> NP3["NP3 Exact npm executor"]
  NP2 --> NP4["NP4 Fresh phased coordinator"]
  NP3 --> NP4
  NP4 --> NP5["NP5 CLI phase contract"]
  NP4 --> NP6["NP6 Wizard phase UX"]
  NP5 --> NP7["NP7 Manage and E2E truth"]
  NP6 --> NP7
  UR --> PO["PO Registry/update ownership"]
  NP7 --> DT["DT Product/docs truth"]
  PO --> DT
  MB1 --> CI1["CI1 Reusable full quality gate"]
  CI1 --> CI2["CI2 Explicit toolchains"]
  CI2 --> CI3["CI3 Tag provenance and protected publish"]
  AP --> MB1["MB1 Safety barrier"]
  BU --> MB1
  SH --> MB1
  CX --> MB1
  BR --> MB1
  MB1 --> NP7
  DT --> MB2["MB2 Product candidate barrier"]
  CI3 --> MB2
  MB2 --> QA["Platform and owner gates"]
  QA --> TAG["Signed tag and verified draft"]
  TAG --> TAP["Immutable Homebrew formulas"]
  TAP --> CANARY["Owner canary and limited beta"]
~~~

## 5. Slice catalog

Likely paths are planning allowlists, not authorization to edit them. Sol must freeze the
exact list before each slice.

### Governance

| ID | Outcome | Likely paths | Depends on |
|----|---------|--------------|------------|
| G0 | Reconcile stale npm scope, mark the old roadmap historical, and install this fixed catalog | tasks/current-slice.scope, tasks/todo.md, this plan | none |
| G1 | Candidate checker accepts production payload only in verified state; add transition tests and a commit ledger | scripts/check-slice-scope.sh, new script tests, Makefile, tasks/workflow-guardrails.md | G0 |
| G2 | Establish two authorized worktrees, draft PR, and remote CI baseline | Git/external state only | G1 and user authorization |

### Runtime and update safety

| ID | Outcome | Likely paths | Depends on |
|----|---------|--------------|------------|
| RK1 | Shared Unix process-group lifecycle; idempotent Cancel; scanner/long-line errors become terminal; undrained output fails boundedly | internal/runner/bash.go, exact_streaming.go, new process-group helper, runner tests | G1 |
| RS1 | Typed sequential streaming primitive continuously drains ordered phases and prevents later starts after failure/cancel | new internal/runner/sequence.go and tests | RK1 |
| AP1 | apt update then upgrade use RS1; both outputs are visible; 1,000-line and cancel cases cannot deadlock | internal/pkg/apt.go and focused tests | RS1 |
| UR1 | Add execution-provider authority distinct from display provenance; stamp it from the discovering manager | internal/pkg/manager.go, update.go, new route tests | G1 |
| UR2 | Group selected updates by provider, execute only through the bound manager, recheck through the same manager, preserve selection order | internal/ui/installation.go, streaming.go, screen_update.go, routing tests | UR1, RS1 |
| CX1 | Pass install context through every config action and TPM/Neovim operation; cancellation kills clone helpers and triggers rollback/journal cancellation | internal/ui/installation.go, internal/tools/tmux.go, neovim.go, tests | RK1 |

RK1 intentionally absorbs process-group, overflow, and scanner behavior into one runner-kernel
contract. AP1 does not raise channel capacity or discard apt output.

### Backup and bounded input

| ID | Outcome | Likely paths | Depends on |
|----|---------|--------------|------------|
| BA1 | RestoreCatalogEntry consumes the exact opaque catalog snapshot/parent authority; mutable Name/Path cannot redirect bytes | internal/backup/catalog.go, backup.go, tests; safefile only if snapshot extraction is required | G1 |
| BC1 | CLI restore consumes the accepted entry | cmd/dotfiles/main.go and restore tests | BA1 |
| BC2 | TUI Backups retains and consumes the accepted entry | internal/ui/app.go, screen_backups.go, tests | BA1 |
| BU1 | Uninstall chooses the newest valid catalog by timestamp/tie-break, never raw lexicographic directory order | cmd/dotfiles/main.go, catalog tests | BC1 |
| BR1 | Define byte/count budgets and add bounded, no-follow structured read primitive | internal/safefile bounded read files/tests, policy doc | G1 |
| BR2 | Apply limits to global/tool/profile/hotkeys/Claude/native config reads | internal/config files, selected importers, tests | BR1 |
| BR3 | Apply manifest, entry-count, file-count, per-file, and total backup limits before allocation/mutation | internal/backup and safefile snapshot-budget tests | BR1, BA1 |

Suggested initial budgets are 1 MiB per product JSON document, a separately justified
Claude/native-config limit, and explicit backup manifest/tree budgets. Exact values are
frozen by BR1 tests.

### Embedded helper and supply chain

| ID | Outcome | Likely paths | Depends on |
|----|---------|--------------|------------|
| SH1 | Black-box execute sshh; help/add work without config; first file is 0600; strict name/destination/port grammar; leading SSH options and controls rejected; defense-in-depth end-of-options | internal/scripts/scripts.go and behavior tests | G1 |
| SP1 | Define immutable remote-artifact descriptor bound into plan/action digest and public risk vocabulary | internal/operation plan/artifact files and tests | G1 |
| SP2 | TPM and Neovim use immutable archive+SHA or full commit verification, stage before commit, and expose the pin in review | internal/tools/tmux.go, neovim.go, UI plan/action tests | SP1, CX1 |
| SP3 | Pin every generated Claude MCP npm package version, remove latest/bare packages, default unsupported servers off, and correct metadata to ~/.claude.json | internal/config/claude.go, internal/tools/claude_code.go, Claude UI/tests | SP1 |

Pins change the plan digest. Wrong bytes never enter the live namespace. Pinned npm versions
still disclose that transitive registry/runtime integrity is not fully content-addressed.

### CLI, UI, and product ownership

| ID | Outcome | Likely paths | Depends on |
|----|---------|--------------|------------|
| CL1 | Table-driven Cobra Args and deterministic exits for update/theme/config/backups/users and runtime failures | cmd/dotfiles/main.go and CLI contract tests | BU1 |
| PO1 | Replace legacy static updater ownership with a registry-derived platform package set and pure filter | internal/tools managed package projection, registry.go, internal/pkg/update.go, tests | UR1 |
| PO2 | CLI and TUI update checks consume the same projection | cmd/dotfiles/main.go, internal/ui/cache.go, tests | PO1, CL1 |
| UI1 | Deep Dive uses a viewport; keyboard, wheel, click, and Continue are reachable at 40x14, 60x18, 80x24, 120x40 | internal/ui/screen_deepdivemenu.go and mouse/golden tests | npm UI slice NP6 |
| DT1 | README, tools reference, pre-PR skill, CLAUDE/AGENTS docs, and update inventory match the runtime registry and actual capabilities | README.md, docs/tools.md, relevant AGENTS/CLAUDE/skill files, parity tests | PO2, NP7, SP3 |

Theme consolidation and broad UI refactors are not part of these slices.

### Two-phase npm installation

The selected contract completes npm execution rather than repeatedly stopping at truthful
blocking.

#### Public behavior

- Plan schema advances to v2 before release.
- A plan contains one mutation authority class: manager/cask prerequisite or npm.
- When Node/npm prerequisites are missing, phase 1 contains only prerequisite actions.
- Successful phase-1 Apply records terminal phase_complete, returns exit 3, and prints one
  fixed replan-required line. It grants no phase-2 authority.
- The user reruns plan with the same explicit tool set.
- Fresh health and executable observations create a pure npm phase with a new hash.
- Phase 2 revalidates the exact npm/Node chain immediately before StartStreaming.
- Successful final Apply returns 0 only after the declared tool detector passes.
- TUI never chains automatically: phase 2 requires a fresh preview and confirmation.
- Stale, mixed, forged, drifted, blocked, cancelled, and no-change cases mutate nothing.

| ID | Outcome | Likely paths | Depends on |
|----|---------|--------------|------------|
| NP1 | Phase domain and invariants: requested tools, phase kind/index, remaining intent, exactly one authority class | internal/operation/plan.go, internal/installplan/service.go/session.go, tests | G0 |
| NP2 | Redacted planpublic v2 projection with prerequisite, npm, blocked, no-change, and replan-required vocabulary | internal/planpublic, cmd plan JSON tests/docs | NP1 |
| NP3 | Execute pure npm recipe only through accepted NPMExecutionIdentity; exact args/env/cwd, drift, overflow, cancel, and postcondition tests | internal/installapply/recipe.go, internal/pkg/npm_execution_identity.go, tests | RK1, NP1 |
| NP4 | Fresh phased coordinator: phase 1 journal/apply, exit-3 terminal state, fresh replan, phase-2 hash/identity, no automatic continuation | internal/installapply/service.go, internal/installplan, operation journal tests | NP2, NP3 |
| NP5 | Headless plan/apply v2 CLI and exact stream/exit grammar | cmd/dotfiles/plan_json.go, apply.go, tests | NP4 |
| NP6 | Wizard preview/confirm/progress/summary for phase 1 and fresh phase 2; no false full success | internal/ui/install_plan.go, installation.go, progress/summary tests | NP4 |
| NP7 | Manage/Deep Dive capability truth and end-to-end Claude Code, Codex, Pi, OpenCode, and T3 matrix | internal/ui Manage/deep-dive tests, docs capability tests | NP5, NP6 |

OpenCode package-manager and T3 cask routes remain single phase. Claude Code, Codex, and Pi
exercise the phased npm contract. No authentication occurs during Apply.

### CI and release construction

| ID | Outcome | Likely paths | Depends on |
|----|---------|--------------|------------|
| CI1 | One reusable full-quality workflow called by PR, main, rehearsal, and tag: module/tidy/format/vet/lint/gosec/static/vuln/ShellCheck/test/race/build/smoke/release-check | .github/workflows and policy tests | G1 |
| CI2 | Product Go 1.25.6 and release-tool Go 1.26.5 are separate explicit setup steps; GOTOOLCHAIN=local; GoReleaser 2.17.0 and Syft 1.44.0 captured | workflows, Makefile, docs/security-scanning.md | CI1 |
| CI3 | Strict SemVer, annotated/signed tag, exact green main ancestor, version uniqueness, minimal permissions, draft build, protected publish approval | release workflow and tests/docs | CI2 |
| CI4 | Two isolated snapshot builds compare hashes; native smoke on Darwin/Linux amd64/arm64; archive layout, source, SPDX, checksums, signatures/attestations verified | release scripts/workflow, .goreleaser.yml, tests | CI3 |
| CI5 | Require stable full-gate job names in branch protection; require up-to-date PR; disallow direct/force main pushes; require Security | GitHub repository settings and evidence | CI1, user authorization |

Direct macOS downloads require signed/notarized evidence. If Apple credentials are unavailable,
the release must not advertise that direct path; Homebrew remains the supported macOS route.

### External repositories

Each item uses its own repository PR and must not share a writer with dotfiles.

| ID | Repository | Outcome | Depends on |
|----|------------|---------|------------|
| DS1 | sshh | Black-box script/completion tests, preserve config grammar, create a new immutable annotated/signed tag if the intended tag is absent | SH1 |
| DS2 | homebrew-tap | sshh formula uses immutable tag URL/hash and exact version; upgrade preserves config | DS1 |
| DH1 | homebrew-tap | dotfiles formula structural rewrite removes basename deletion, installs release archives by OS/arch, tests exact version/help, fixes caveats | CI4 structure; final hashes deferred |
| DH2 | homebrew-tap | Fill final version/URLs/hashes only after verified public assets; no behavior change in this slice | release publication, DH1 |
| DH3 | homebrew-tap | CI lifecycle: clean install, v2.0.1 upgrade, unknown collision preservation, PATH shadow diagnosis, uninstall, reinstall, rollback, sshh coexistence | DS2, DH2 |

## 6. Parallel implementation waves

Two entries joined by plus run simultaneously. Every pair is followed by concurrent
cross-review and root integration.

| Wave | Parallel pair | Barrier |
|------|---------------|---------|
| 0 | G0, then G1 | Canonical plan and state checker green |
| 1A | RK1 + BA1 | Runner and restore authority foundations |
| 1B | RS1 + SH1 | Sequential stream and embedded helper |
| 1C | AP1 + BC1 | Debian adapter and CLI restore |
| 1D | UR1 + BC2 | Provider domain and TUI restore |
| 1E | UR2 + BU1 | TUI update routing and uninstall recovery |
| 1F | CX1 + BR1 | Cancellation propagation and bounded-read policy |
| 1G | BR2 + BR3 | Config and backup input limits |
| MB1 | Full safety merge barrier | High/Medium runtime and recovery contracts green |
| 2A | SP1 + CL1 | Payload policy and CLI grammar |
| 2B | SP2 + PO1 | Pinned repos and registry ownership |
| 2C | SP3 + PO2 | Pinned MCP/metadata and surface adoption |
| 2D | NP1 + CI1 | npm phase domain and reusable CI |
| 2E | NP2 + CI2 | Public projection and explicit toolchains |
| 2F | NP3 + CI3 | Exact executor and tag/publish controls |
| 2G | NP4 + CI4 | Phased coordinator and artifact harness |
| 2H | NP5 + NP6 | CLI and wizard adapters |
| 2I | NP7 + UI1 | Integration truth and viewport |
| 2J | DT1 + CI5 | Documentation convergence and remote policy |
| MB2 | Full product/release merge barrier | One exact green engineering candidate |
| 3A | DS1 + DH1 | Separate sshh source and tap structural work |
| 3B | DS2 + platform test reservations | Immutable sshh formula and QA readiness |
| 3C | Manual platform lanes in parallel | Same candidate hash on all systems |
| MB3 | Candidate evidence barrier | Eligible for destructive owner gates |

Serialization rules:

- cmd/dotfiles/main.go: BC1 -> BU1 -> CL1 -> PO2 -> NP5.
- internal/ui/installation.go: UR2 -> CX1 -> SP2 -> NP6.
- internal/backup files: BA1 -> BR3.
- release workflow: CI1 -> CI2 -> CI3 -> CI4.
- README/docs/tools: capability implementation -> DT1.

## 7. Slice acceptance contracts

Every slice must:

- persist red tests before production changes: an independent slice must fail on the audited
  baseline, while a dependent slice must fail on its declared parent candidate;
- prove zero subprocess/mutation on every rejected authority case;
- include hostile replacement, cancellation, overflow, or malformed-input tests relevant to
  its boundary;
- repeat focused tests at least 20 times where timing/race behavior is involved;
- run affected packages under race;
- receive review from an agent that did not implement it;
- record residual risks without inflating success claims;
- pass make slice-check before and after handoff and make slice-check-candidate after staging;
- commit immediately after its candidate gate, without unrelated planning files.

The commit exit gate is:

- gofmt and git diff --check;
- focused tests and affected-package race;
- go mod verify and tidy-diff when dependencies are touched;
- full go test and go test -race;
- go vet, pinned golangci-lint, Staticcheck;
- govulncheck and ShellCheck when relevant;
- Sol and adversarial review with no sustained blocker.

## 8. Merge barriers

### MB1 — safety substrate

Requires RK1, RS1, AP1, UR1-UR2, CX1, BA1, BC1-BC2, BU1, BR1-BR3, and SH1:

- 1,000-line apt Update All cannot deadlock;
- update/cancel process trees terminate;
- mixed Linuxbrew/apt/paru routes remain provider-bound;
- restore cannot be redirected after selection;
- uninstall finds the newest valid catalog;
- sshh first entry works and SSH options are rejected;
- oversized/symlinked structured inputs fail before mutation;
- the runtime, recovery, helper, cancellation, and bounded-input substrate is green.

Run disposable Debian and mixed-manager tests before advancing.

### MB2 — product truth

Requires SP1-SP3, CL1, PO1-PO2, UI1, NP1-NP7, DT1, and all local automated gates:

- CLI, TUI, registry, docs, plan JSON, Apply, and journal agree;
- prerequisite Apply never implies final tool success;
- phase 2 always has a new observation, hash, preview, and confirmation;
- package-only, cask, npm, no-change, blocked, drift, cancel, and failure paths are deterministic;
- AI agents remain opt-in and no authentication is attempted.

### MB3 — release construction

Requires CI1-CI5 and a remote green candidate:

- PR, main, rehearsal, and tag call the same full gate;
- exact product and release toolchains are captured;
- tag must be signed/annotated and on green main;
- draft assets reproduce twice and run natively;
- signatures/attestations, SBOMs, and both checksum manifests verify.

### MB4 — distribution

Requires DS1-DS2 and DH1-DH3:

- formulas use immutable assets and exact hashes;
- no unknown basename is deleted;
- clean install, upgrade, reinstall, uninstall, rollback, and coexistence pass;
- public assets redownload and verify anonymously.

## 9. Manual and destructive QA

### Platform matrix

Use pairwise coverage:

- Apple Silicon macOS + Ghostty: complete flow;
- Intel macOS + iTerm2: install/manage/upgrade;
- Debian amd64 + Alacritty: apt/update/cancel;
- Arch amd64 + Ghostty or xterm: pacman/paru plus Linuxbrew routing;
- Raspberry Pi arm64 + plain xterm/SSH: bounded status/install/update.

At 60x18, 80x24, and 120x40, plus targeted 40x14 error/progress cases, verify Installer,
Deep Dive, Manage, Save/Cancel, Updates, Backups/Restore, Hotkeys, mouse, keyboard, scrolling,
Ctrl+C, low color/plain ASCII/Nerd Font fallback, reduced motion, and sudo behavior.

### Owner Apply gate

Run tasks/install-apply-owner-test.md on a disposable VM or spare Mac with a proven external
restore point. Include formula, cask, phased npm, partial repair, no-change, stale/wrong hash,
cancellation, fatal fault, durable journal-before-mutation, exact streams/exits, and snapshot
restore after every row.

### Owner Save gate

Only after Apply passes:

- exercise every current config writer against realistic native/unmanaged fixtures;
- change one visible field and prove unknown bytes survive;
- verify target, preview, backup scope, modes, journal, idempotence, and rollback;
- inject symlink swap, stale revision, permission denial, disk full, cancellation, and writer
  validation failure;
- prove theme touches only explicitly managed artifacts;
- prove uninstall does not erase unowned files.

## 10. Release gates

| Gate | Exit evidence | Promotion |
|------|---------------|-----------|
| RG0 Plan freeze | G0/G1 committed; fixed catalog; no stale active state | Implementation may begin |
| RG1 Safety | MB1 plus disposable Debian/mixed-manager evidence | npm/product work may integrate |
| RG2 Engineering candidate | MB2, remote PR green, exact candidate dossier | Platform/manual QA |
| RG3 Release construction | MB3 and duplicate reproducible snapshots | Owner destructive tests |
| RG4 Owner alpha | Apply and Save pass; snapshot recovery verified | Signed tag may be created |
| RG5 Draft release | signed annotated tag, full tag gate, verified draft assets | Protected publish approval |
| RG6 Distribution | MB4 Homebrew lifecycle and anonymous asset verification | Owner canary |
| RG7 Owner canary | one bounded primary-account tool, 24-72 quiet hours, one-hour rollback SLO | Limited beta |
| RG8 Limited beta | 3-5 opt-in users, staged one at a time, quiet window, zero severe incidents | Mock enterprise |
| RG9 Mock enterprise | disposable policy/fleet/proxy/offline/least-privilege/audit matrix | Mock-enterprise claim only |

Any production change after RG2 invalidates RG2 and all later evidence.

## 11. Release cut sequence

1. Freeze features and complete G0/G1.
2. With explicit authorization, create isolated slice worktrees and a draft PR.
3. Execute Waves 1 and 2 with pairwise implementation/review.
4. Pass MB1 and MB2.
5. Push the exact candidate and require the full remote PR gate.
6. Merge through protected main; main CI must pass on the merge SHA.
7. Run duplicate snapshot builds and the platform/manual matrix.
8. Pass disposable Owner Apply, then disposable Owner Save.
9. Finalize v2.2.0 release notes, support policy, known limitations, and rollback commands.
10. Create a signed annotated tag on the exact green main commit.
11. Tag workflow reruns the full gate and builds a draft.
12. Verify native execution, version, layout, SBOMs, checksums, signatures, and attestations.
13. Approve the protected publish job.
14. Redownload and verify public assets anonymously.
15. Complete DH2 with final immutable URLs and hashes.
16. Run and merge DH3 Homebrew lifecycle tests.
17. Verify public clean install, upgrade, rollback, uninstall, and sshh coexistence.
18. Run the owner canary for 24-72 hours.
19. Enroll limited beta users one at a time.
20. Consider mock-enterprise only after its separate gate.

## 12. Evidence dossier

Keep one checksummed private dossier outside repository and guest snapshots:

- candidate version, commit, tree, branch, clean-state proof;
- slice ledger and focused/full gate outputs;
- PR, CI, review, and main-run identifiers;
- product/release tool versions and module sums;
- artifact, SBOM, signature, attestation, and checksum inventory;
- platform/manual checklist and screenshots;
- dotfiles, sshh, and homebrew-tap commit/tag identifiers;
- Homebrew lifecycle results;
- Owner Apply/Save and snapshot-restore results;
- accepted limitations, approvers, stop decisions, and rollback commands.

Only a redacted summary may be shared. Raw Doctor JSON, journals, paths, host/user identity,
configuration, environment, credentials, and tokens stay private.

## 13. Immediate stop and rollback rules

Stop and re-plan on:

- any path outside a frozen allowlist or any ceiling breach;
- writer overlap or a production edit during verification;
- an independent slice whose red test does not fail on the audited baseline, or a dependent
  slice whose red test does not fail on its declared parent candidate;
- mutation after stale, blocked, cancelled, no-change, or invalid input;
- unowned-byte deletion or loss of unknown config;
- process, output channel, or UI state that cannot terminate boundedly;
- provider, action, phase, hash, detector, or journal mismatch;
- private path/digest/error/output leakage;
- ambiguous candidate, unexpected toolchain, missing required CI, or non-main tag;
- artifact, formula hash, signature, SBOM, checksum, or attestation mismatch;
- snapshot or byte-identical restore failure.

Before tag, fix through a new reviewed candidate. A failed draft version is abandoned, never
moved. After publication, keep/revert the tap to the previous immutable formula and cut a new
patch. During beta, stop enrollment, restore affected hosts, preserve private evidence, and
resume only from a new candidate.

## 14. Definition of done

The program is complete only when:

- every slice is committed with a candidate-bound verification record;
- all audit High/Medium findings are closed or explicitly accepted by the user as release
  limitations;
- local, remote, platform, owner, artifact, distribution, and canary evidence all point to
  the same release commit;
- README, CLI/TUI behavior, public JSON, support guidance, formulas, and release notes agree;
- the final public Homebrew install is repeatable and rollback is proven;
- tasks/todo.md records the release and moves remaining deferrals to a post-release backlog.
