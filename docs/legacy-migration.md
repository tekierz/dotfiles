# Migrating from the retired Bash installer

`dotfiles-setup` was a separate Bash installer and embedded management CLI. It
had independent install, configuration, backup, restore, and version behavior
that drifted from the Go application. It is retired, unsupported, and removed
from active source and packaging. Git history retains the source for audit and
forensics; it is not a supported installation route.

Do not execute old `raw.githubusercontent.com/.../dotfiles-setup` URLs or pipe
them into a shell. Historical content is mutable at branch URLs and the retired
installer does not meet the current operation-plan, provenance, or rollback
requirements.

## Conservative migration

1. Preserve the machine before changing it. Make an independent backup of your
   home-directory configuration files and the entire
   `~/.config/dotfiles/backups/` directory.
2. Install the supported Go application with Homebrew or a verified release
   artifact. Do not overwrite an unknown executable manually.
3. Run `dotfiles doctor` (or `dotfiles doctor --json`) and inspect every reported
   `dotfiles`, `dotfiles-tui`, and `dotfiles-setup` path.
4. Preview the Go application's exact operation plan. Confirm that every target
   and observed native configuration source belongs to the intended account.
5. Apply on a disposable account or VM first when the old installer managed
   important configuration. Verify the result before migrating a primary user.
6. Remove a stale legacy executable only after verifying its path and ownership.
   `dotfiles doctor` deliberately does not delete it.

Do not assume legacy and current backup manifests are interchangeable. Use the
program version that created a backup only in an isolated recovery environment,
or restore individual files manually after reviewing their paths and contents.
Do not point the current restore command at an unverified historical manifest.

The separately maintained Homebrew tap must distribute only the Go `dotfiles`
formula. Any old `dotfiles-setup` formula should be disabled or removed rather
than redirected to historical source. Its supported formula must also avoid
deleting legacy-named executables solely by basename; ownership must be proved
before cleanup.
