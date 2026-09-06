# Recovery, configuration, and native writer audit — 2026-09-05

Scope: read-only inspection of internal/backup, internal/safefile, internal/config, native writers in internal/tools, and their restore/retention callers. Production source and Git were not modified. Read internal/AGENTS.md, internal/config/AGENTS.md, internal/tools/AGENTS.md, and tasks/lessons.md. This is a focused audit, not a proof of correctness of every filesystem interleaving.

## Verified findings

### P2 — Backup listing retains the bytes of every backup, making listing and retention scale with total stored data

Primary source: internal/backup/catalog.go:63, :83, :92-95. Supporting source: internal/safefile/directory_unix.go:222-227 and :301; internal/ui/app.go:606-615, :828, :841, :865; cmd/dotfiles/main.go:666.

ListCatalog recursively snapshots each backup, which reads every file into memory, then takes another complete snapshot for verification. It retains the original entire snapshot in each returned catalogAuthority. The TUI retains the CatalogEntry in BackupEntry, so this memory remains live. This means listing all backups or restoring one selected backup needs memory proportional to the size of ALL backups, with additional temporary copies. Retention also lists all backups before pruning them, so the mechanism intended to control accumulated data can run out of memory first.

Trigger: a legitimate large Neovim directory backup, several accumulated backups, or a large file in a backup directory. Especially material for the supported Raspberry Pi / low-memory audience. No hostile actor is needed. No allocation or total-byte budget is enforced by recursive snapshots.

Verified using isolated Go overlay TestAuditCatalogRetainsAllPayloadBytes: four 4 MiB payloads yielded 16,779,880 bytes of additional live heap after runtime.GC, for 16,777,216 payload bytes. No OOM was deliberately induced; the OOM consequence follows from unbounded linear retention and remains workload-dependent.

Suggested fix: keep bounded manifest/identity/digest metadata in the listing; stream recursive hashing rather than retain file content; capture and validate the selected entry on demand. Preserve exact deletion authority using bounded identities/digests without converting the catalog into a copy of all recovery data. Include a retained-heap regression test and explicit size/file-count limits where snapshots are unavoidable.

### P2 — Interactive and CLI restore discard catalog authority before consuming backup sources

Primary source: internal/backup/backup.go:651-658; caller boundaries internal/ui/app.go:669-672 and cmd/dotfiles/main.go:683-695. Directory sources have the equivalent independent snapshot at internal/backup/backup.go:607-615.

Both callers validate a CatalogEntry once and then pass only its string path to backup.Restore. Restore parses the manifest again and reads each payload independently without requiring the exact revisions/digests retained by the selected catalog entry. A backup changed after validation (including while earlier files are being restored) can therefore supply different bytes and still produce a successful restore. A manifest replacement in the validation-to-read gap can similarly change the selected set, including existed=no removals. The filesystem kernel prevents symlink traversal and verifies each individual read; it does not bind these reads to what was selected and validated.

Trigger: another same-user process or synchronization operation edits/replaces the backup during restore. This is an authority-continuity/data-integrity failure, not a demonstrated privilege escalation. Plan rollback already has a stronger RestoreExpectedPlan path; the issue is manual catalog restore.

Verified using isolated Go overlay TestAuditRestoreAcceptsChangedSourceAfterCatalogValidation: create a two-file backup, list and validate it, then change the second source in the first destination-write callback. Restore succeeds, has no skipped entries, and writes the changed payload to the second destination. ValidateCatalogEntry on the original entry correctly rejects the same change afterward, proving usable authority existed but was discarded.

Suggested fix: add a RestoreCatalogEntry-style API that accepts opaque selected authority and checks each manifest/source read against that authority, or consumes immutable originals captured from that authority. Keep descriptor-relative source reads and fail before writing any changed source. Add tests for manifest/root/file/directory replacement after selection and between restore items.

## Additional hardening observation (not promoted to security finding)

Manage preferences load through config.LoadToolConfig with unvalidated JSON (internal/config/config.go:348-365; internal/ui/app.go:506-513). TmuxStatusPosition flows through internal/ui/config_apply.go:414/:128 into internal/tools/tmux.go:315, which emits it as raw tmux syntax. NeovimClipboard flows through :441/:173 into internal/tools/neovim.go:223, which interpolates it inside Lua quotes without escaping. These fields are intended as enums, but newline/quote payloads can produce executable config syntax. The normal UI only cycles safe enum choices; no lower-trust input path was established, so this remains input-validation hardening for manually edited/synchronized product state rather than an independent code-execution vulnerability. Allowlist the documented enum values at load/planning/writer boundaries; extend config_validation_test.go beyond btop/Glow. This observation is static, not an executed command-injection reproduction.

## Positive evidence and limits

- Exact revision and parent-chain authority is extensively represented in safefile and plan rollback; committed mutation errors distinguish failures after namespace commit.
- Symlink/nonregular-file refusal, process ownership constraints, backup manifest modes, and failed-root cleanup have substantial adversarial coverage.
- Existing tests passed (cached): GOCACHE=/tmp/dotfiles-audit-2026-09-05/go-cache go test ./internal/backup ./internal/safefile ./internal/config ./internal/tools.
- Fresh overlay repro passed: GOCACHE=/tmp/dotfiles-audit-2026-09-05/go-cache go test -overlay /tmp/dotfiles-audit-2026-09-05/recovery/overlay.json ./internal/backup -run '^TestAudit' -v -count=1.
- Repro source: /tmp/dotfiles-audit-2026-09-05/recovery/audit_test.go. Overlay adds only a virtual test file; nothing was added to the checkout.
- No Linux runtime, actual low-memory device, live destructive restore, or hostile concurrent-process stress campaign was performed.
