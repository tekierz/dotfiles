# Tools, Generated Configs, Legacy Installer, Documentation, and Release Audit

**Audit lane:** `/root/audit_tools_docs_release`
**Repository state inspected:** `release-remediation`, commit `7ddcb99` (`7ddcb99-dirty`)
**Date:** 2026-07-09
**Coverage:** 62 tracked files, 17,887 baseline lines (corrected manifest at end)
**Finding count:** 2 Critical, 14 High, 16 Medium, 4 Low = **36 findings**

## Scope and method

This lane covered every tracked file assigned to it: all of `internal/tools/**`, the
4,346-line legacy installer, root/build/CI/lint files, README and project guidance,
all tracked docs/tasks, and both tracked skill trees. It deliberately excluded the
other audit lanes (`cmd/dotfiles`, most non-tools `internal/**`, and `go.mod/go.sum`).

The file set came from `git ls-files` with the excluded lane prefixes removed. Each
assigned file was read in full, including tests, examples, archived documents, and
embedded Bash heredocs. Structural searches were then used to cross-check every
writer, registered tool, config path, stale interface reference, command, and rename
surface. Adversarial checks included:

- `go test ./internal/tools` on the local macOS host (failed; finding F08).
- `shellcheck -x bin/dotfiles-setup scripts/install-hooks.sh` (warnings; no parser
  errors).
- Inspection of the host's real Ghostty and Git state without exposing values.
- Direct invocation of the installed Glow and Ghostty binaries to validate actual
  config resolution and current option names.
- Current upstream documentation checks for Ghostty, Yazi, Claude Code, and current
  Debian package availability.
- A refutation pass for every finding: each item below states what was checked that
  could have made the issue benign.

The `pre-pr-tests` skill influenced the release-readiness checks, especially manual
TUI, aesthetics, security, performance, and cross-repository expectations. The skill
itself has two divergent tracked copies; that is finding F28. The expected sibling
`homebrew-tap` and `sshh` repositories were not present at the documented paths, so
formula/SHA/cross-repo compatibility remains unverified rather than silently assumed.

## Executive conclusion

**Do not release this build to friends/family or mock-enterprise machines yet.** The
Go application has a coherent foundation and useful test coverage, but its central
configuration contract is unsafe: the dashboard usually does not import what an
installed application is actually configured to do, then many Save/Apply paths
replace that application's whole config. The local Ghostty mismatch is therefore not
an old-build symptom alone; it is reproducible from current source. Git is the most
dangerous example because a theme/config apply can erase identity, Git LFS filters,
includes, and custom aliases.

Release posture by area:

| Area | Rating | Why |
|---|---:|---|
| Feature presence | 5/10 | 30 tools exist, but the six planned AI tools are not registered and many existing entries are install-only. |
| Existing-app adoption | 2/10 | Most settings are not imported; Ghostty can also be overridden by a second, higher-precedence macOS config. |
| Data safety | 2/10 | Whole-file, non-atomic, symlink-following writes are the common path. |
| Cross-platform truthfulness | 4/10 | Package metadata and legacy Yazi schema have drifted; actual macOS tests are absent in CI. |
| Architecture/maintainability | 5/10 | The registry is a reasonable seam, but config/UI/docs are hand-wired and do not scale. |
| Documentation health | 4/10 | Multiple active-looking sources contradict current behavior and each other. |
| Release engineering | 3/10 | CI exists, but no real release config/workflow, provenance, package integration test, or cross-repo verification is present. |
| Local test readiness | 4/10 | Safe only in a disposable account/VM after backups and before any config Apply action. |

The right strategy is **stabilize this repository, not start over**. Preserve its
history, tests, registry, package-manager abstractions, and TUI. Replace the config
ownership/state model, quarantine or retire the legacy installer, then add planned
tools through a capability schema. A new repo would be justified only if the product
is intentionally re-scoped into an enterprise endpoint-management system or the Go
TUI is being replaced wholesale.

## Release-blocking findings

### F01 — Critical — Dashboard Save/Apply can erase unmodeled settings

**Evidence:** `writeToolConfig` calls `os.WriteFile` directly
(`internal/tools/tool.go:168-183`). Whole-file writers feed it generated content for
Ghostty (`ghostty.go:174-188`), tmux (`tmux.go:291-300`), Git (`git.go:242-252`),
Yazi's three files (`yazi.go:210-237`), fzf (`fzf.go:211-220`), btop
(`btop.go:208-224`), LazyGit (`lazygit.go:161-170`), and Glow (`glow.go:88-96`).
Those generators have no read/import/merge pass. Zsh is the notable good exception:
it merges a bounded managed section (`zsh.go:353-380`). Neovim Manage uses an
overlay (`neovim.go:402-453`).

**Trigger:** open Manage/config with an already customized app, change one exposed
field or apply a theme, and save.

**Impact:** every setting not represented by the generator disappears. On the local
machine, Ghostty contains padding, mouse-hide, window-state, shell-detection,
clipboard/paste-protection, and custom keybind settings that the dashboard cannot
represent. The current dashboard's eight Ghostty controls cannot round-trip them.

**Refutation attempted:** installation backup code can protect some install-time
writes, but standalone Manage/config and theme application still call writers; a
backup does not make displayed defaults truthful, preserve post-backup edits, make
writes atomic, or prevent a user from accepting a destructive diff they never saw.

**Fix:** introduce explicit ownership modes (`managed fragment`, `merge-capable`,
`full replacement with explicit consent`). Import existing state before rendering,
show a semantic/raw diff, back up immediately before every destructive apply, and
prefer native include files or bounded managed markers. No full replacement should
be the default for an adopted install.

### F02 — Critical — Git writer can erase identity, LFS, credentials, includes, and aliases

**Evidence:** `GenerateGitConfig` emits only the modeled core/pull/push/credential,
signing, delta/diff/merge, and five allow-listed aliases; the alias switch is at
`internal/tools/git.go:222-237`. `WriteGitConfig` then replaces `~/.gitconfig` at
`git.go:242-252`. It emits no `[user]`, `[filter "lfs"]`, `[include]`, arbitrary
credential sections, or unknown aliases. The local global config contains
`user.name`, `user.email`, Git LFS filters, and custom `last`/`unstage` aliases, none
of which the generator can reproduce.

**Trigger:** edit any Git field or perform a global theme apply that includes Git.

**Impact:** commits lose/alter identity behavior, Git LFS can stop functioning, and
private enterprise include/credential/signing policy can be removed. This is a
release-stopping data-loss and workflow-breakage defect.

**Refutation attempted:** the active task plan says identity is preserved, but no
preservation/read/merge code exists in this writer. The absence was checked across
all assigned production files; preservation is not delegated elsewhere in the tools
package.

**Fix:** never own `~/.gitconfig`. Write a dedicated managed include such as
`~/.config/<brand>/gitconfig`, add/remove one idempotent `include.path` entry with a
Git-aware editor, and leave identity, LFS, credentials, and unknown sections alone.

## High-severity findings

### F03 — High — Shared writer is non-atomic, follows symlinks, and does not enforce promised modes

**Evidence:** `internal/tools/tool.go:175-183` uses `MkdirAll` plus `os.WriteFile`.
`os.WriteFile(..., 0600)` applies the mode only when creating a file; an existing
0644 file stays 0644. It truncates in place and follows a final-component symlink.
The local Ghostty file is 0644, directly refuting the comment's asserted 0600
postcondition (`tool.go:168-174`). Legacy `safe_write_config` similarly uses
`mkdir -p` and `echo >` without chmod (`bin/dotfiles-setup:925-940`).

**Trigger:** any config write, especially interruption/disk-full, an existing
permissive file, or a symlinked config.

**Impact:** partial/empty configs, writes outside the expected config root, and false
security guarantees. Mock-enterprise deployments cannot rely on documented modes.

**Refutation attempted:** parent directories are created with 0700, but `MkdirAll`
does not tighten an existing directory and does not address final-file behavior.

**Fix:** `lstat`/no-follow policy, create a same-directory temp file, write+fsync,
chmod explicitly, validate generated syntax, atomic rename, then fsync the directory.

### F04 — High — Ghostty install detection and config precedence misrepresent local state

**Evidence:** `NewGhosttyTool` has only a package map and XDG config path
(`internal/tools/ghostty.go:32-55`); unlike GUI app tools, it has no `exec.LookPath`
or `/Applications/Ghostty.app` probe. It always writes
`~/.config/ghostty/config` (`ghostty.go:174-183`). Current official Ghostty config
loading reads XDG first and macOS Application Support afterward, so
`~/Library/Application Support/com.mitchellh.ghostty/config[.ghostty]` can override
the generated file ([official loading order](https://ghostty.org/docs/config)).

**Trigger:** Ghostty was installed outside the currently detected Brew receipt, or
the macOS Application Support config exists.

**Impact:** an installed app appears absent and/or dashboard changes appear not to
work because a later config wins. This matches the reported local symptom.

**Refutation attempted:** XDG is a supported Ghostty location and `brew list ghostty`
works for a normal cask receipt, but neither fact covers manual/app-bundle installs or
the documented later-precedence file.

**Fix:** probe executable, bundle, and package receipt; discover both config roots;
show the active precedence chain; adopt a managed `config-file` fragment at the
highest-precedence location or update the actual active file without clobbering it.

### F05 — High — Generated Ghostty blur option is not current schema

**Evidence:** `internal/tools/ghostty.go:80-82` writes
`background-blur-radius = N`. The current option is `background-blur`; the former is
absent from Ghostty's current [configuration reference](https://ghostty.org/docs/config/reference).

**Trigger:** select a nonzero blur radius.

**Impact:** generated config contains an unknown key and blur does not work (and may
cause config diagnostics). It is direct evidence that hand-maintained generators
have drifted from upstream.

**Refutation attempted:** `window-decoration=true/false` was also checked and remains
accepted for backward compatibility; only the blur key is asserted invalid here.

**Fix:** emit `background-blur`, validate against the installed Ghostty version, and
add a generated-config smoke test using `ghostty +validate-config`/equivalent.

### F06 — High — “Installed” means every bundled dependency has a package receipt

**Evidence:** `allPackagesInstalled` requires every package
(`internal/tools/tool.go:129-158`). This makes Zsh depend on all bundled plugins,
Yazi on all preview dependencies, Git on package-manager Git rather than Apple's
system binary, and most simple tools blind to binaries installed via another source.

**Trigger:** an app is manually installed, available on PATH, installed as a cask or
AppImage, or missing one optional dependency.

**Impact:** false “not installed” state, duplicate install attempts, and the exact
“already installed apps aren't reflected” experience reported by the user.

**Refutation attempted:** the all-package rule intentionally makes the installer
repair partial bundles (`tool.go:129-133`), but that is a dependency-health signal,
not truthful primary-app installation state.

**Fix:** model primary executable/app probes separately from required and optional
dependencies; report `installed`, `dependency degraded`, `configured`, and `managed`
as distinct dimensions with reasons.

### F07 — High — Claude Code advertises and checks the wrong config path

**Evidence:** `internal/tools/claude_code.go:20-38` registers
`~/.claude/settings.json`, while `ApplyConfigWithMCPs` delegates to the config package
(`claude_code.go:75-103`) whose persisted MCP file is `~/.claude.json`. The repository
docs also refer to `.claude.json`.

**Trigger:** HasConfig/status/backup/config-discovery for Claude Code.

**Impact:** readiness and backup surfaces inspect the wrong file; users may be told
Claude is unconfigured while the application is configured, or the real file may be
missed by backup.

**Refutation attempted:** Claude itself also has a settings file, but the feature
implemented here is specifically MCP mutation through `LoadClaudeConfig`/
`SaveClaudeConfig`, so that second path does not describe what this tool writes.

**Fix:** obtain paths from the same config package used by apply; label this feature
“Claude Code MCP servers” rather than comprehensive Claude settings management.

### F08 — High — The current test suite fails on macOS and CI does not run tests there

**Evidence:** `go test ./internal/tools` fails
`TestGlowConfigPathUsesUserConfigDir`: the test expects `os.UserConfigDir()` at
`internal/tools/glow_test.go:10-32`, while the correct macOS implementation uses
`~/Library/Preferences/glow/glow.yml` (`glow.go:99-114`). CI runs tests/race only on
Ubuntu (`.github/workflows/ci.yml:99-117`); its macOS matrix only builds and runs
`--help` (`ci.yml:119-137`).

**Trigger:** run the test suite on a Mac—the product's primary local target.

**Impact:** release claims of green tests are false on the target platform, and
platform-specific config/detection defects escape CI.

**Refutation attempted:** the implementation was verified against the installed Glow
binary and is correct; changing production to satisfy the test would regress Glow.

**Fix:** make the expected path platform-aware/inject the resolver, and run normal
tests on macOS plus Linux/ARM compile and package-integration jobs.

### F09 — High — Public legacy installer generates obsolete Yazi configuration

**Evidence:** the legacy theme uses `[manager]` and `[select]`
(`bin/dotfiles-setup:438-511`); initial Yazi config and both keymaps use `[manager]`
(`bin/dotfiles-setup:2202-2239`, `:2288-2291`); its embedded updater repeats
`[manager]`/`[select]` (`:3540-3582`). Current Yazi uses `[mgr]` and the newer theme
schema ([official current configuration](https://yazi-rs.github.io/docs/configuration/overview/)).
The Go generator has already migrated to `[mgr]` (`internal/tools/yazi.go:167-205`).

**Trigger:** install via the documented legacy Bash route or switch a theme with its
embedded management CLI on current Yazi.

**Impact:** keymaps/theme sections are ignored or partially ineffective; a prominent
fallback distribution path is functionally stale.

**Refutation attempted:** current Yazi was checked rather than assuming the rename;
the current Go generator and upstream docs agree. This is not an issue with the Go
writer's section names.

**Fix:** remove legacy from the recommended/public install path immediately; if kept
for migration, patch and test it against a pinned current Yazi and clearly label its
support horizon.

### F10 — High — The legacy installer is a second, drifting product

**Evidence:** `bin/dotfiles-setup` is 4,346 lines and embeds another management CLI
starting at `:2831`. Outer version is `1.0.1` (`:11`); embedded version is `1.1.0`
(`:2843`); intended Go release is `2.1.2` (`Makefile:8-11`). Theme generation,
restore, aliases, hotkey help, platform setup, and config mutation are duplicated.

**Trigger:** any feature, schema, security, theme, or documentation change lands in
only one implementation—as already happened with Yazi.

**Impact:** permanent feature drift, doubled QA surface, confusing support reports,
and inability to state what “dotfiles” installs without first identifying the path.

**Refutation attempted:** the script is labeled legacy in portions of README/docs,
but it remains installed by `make install` (`Makefile:41-45`), is described as
available, and contains a live updater; it is not a frozen archival artifact.

**Fix:** stop installing it by default; move it to a clearly frozen `legacy/` or a
separate migration-only artifact, remove duplicated embedded management logic, and
make the Go application the only supported product.

### F11 — High — Planned AI-agent installs are not enterprise-safe and are mostly default-on

**Evidence:** the six tools are only a spec, not implementation
(`tasks/new-tools-spec.md:18-27`, `:445-457`). Cursor Agent, OpenCode fallback, Pi,
and Hermes propose remote `curl | bash/sh` execution; Codex/Pi alternatives use
global npm. Five tools are recommended enabled by default; Hermes is the only
default-off warning despite all agents involving authentication/data egress. The T3
sample says it will be platform-filtered but omits `platformFilter` from the actual
constructor (`new-tools-spec.md:292-327`).

**Trigger:** implement the spec literally and deploy to friends/family or mock
enterprise machines.

**Impact:** unpinned remote code execution, mutable dependencies, unexpected agent
installation, credential/data-egress policy violations, and platform leakage.

**Refutation attempted:** the spec tells implementers to re-verify packages and warns
about Hermes; it does not add checksum/signature verification, version policy,
consent gates, or equivalent warnings for the other agents.

**Fix:** default every AI agent off; require explicit consent with provenance,
permissions, data-egress/auth summary; prefer signed packages/releases; pin version
and digest; validate the binary identity (especially collision-prone `pi`); support
proxy/offline policy and an allowlist. Implement only after F01/F06 are fixed.

### F12 — High — There is no reproducible release pipeline

**Evidence:** `Makefile:88-91` invokes `goreleaser release --snapshot --clean`, but
there is no tracked `.goreleaser.yml` and no release workflow. CI only builds current
host binaries. Latest repository tags found locally are `v2.0.2`/`v2.0.1`, while the
Makefile says releases must come from `v2.1.2` (`Makefile:8-11`). No checksums, SBOM,
signature/provenance, Homebrew formula integration test, upgrade test, or artifact
installation test is present in this lane.

**Trigger:** attempt a local/friends release or run `make release`.

**Impact:** non-reproducible/unverifiable artifacts and ambiguous versioning; mock
enterprise evaluation cannot establish provenance or rollback.

**Refutation attempted:** CI's build matrix proves source compiles on Ubuntu/macOS,
but it is not a packaging/release process and `--snapshot` is explicitly not a
published release.

**Fix:** add a pinned GoReleaser/release workflow, version/tag policy, macOS/Linux
amd64+arm64 artifacts, checksums, SBOM, signing/provenance, Homebrew formula update
and clean-machine install/upgrade/uninstall tests.

### F13 — High — The blocking lint job globally suppresses most substantive findings

**Evidence:** `.golangci.yml:91-167` excludes diagnostics by generic message text for
all errcheck, wrapped-error mistakes, exhaustive switches, complexity, several
gosec rules (including command execution and permissions), nilerr, noctx, misspell,
and more. Most rules have no path scope. CI calls this config as a blocking gate
(`.github/workflows/ci.yml:34-38`).

**Trigger:** introduce a new unchecked error, command-without-context, permission
issue, nil error return, or excluded security pattern anywhere.

**Impact:** CI remains green while high-value defects accumulate; the gate's label
overstates its protection.

**Refutation attempted:** Staticcheck has a tighter explicit allowlist
(`ci.yml:40-81`), but it does not replace errcheck/gosec/noctx/errorlint coverage.

**Fix:** baseline findings by exact file/rule, remediate in bounded batches, forbid
global text exclusions, and ratchet the baseline so new instances fail.

### F14 — High — fzf “advanced options” are written verbatim into sourced shell code

**Evidence:** `FzfConfig.DefaultOpts` is documented as appended verbatim
(`internal/tools/fzf.go:15-22`). `GenerateFzfConfig` builds a sourceable Zsh file and
interpolates shell strings; the result is sourced automatically from generated Zsh
(`zsh.go:190-192`), then written at `fzf.go:211-220`.

**Trigger:** a quote, command substitution, newline, or malicious value enters the
saved JSON/UI field (or the config is altered by another process) and Zsh starts.

**Impact:** syntax breakage or command execution during every shell startup. This is
especially undesirable in shared/mock-enterprise profiles.

**Refutation attempted:** the local user is generally authorized to edit their own
shell config, but the dashboard claims structured configuration and expands the
trust boundary to imported/persisted data; accidental quotes alone are enough to
break startup.

**Fix:** model options as a validated string array, shell-quote each token, reject
newlines/substitution syntax, and parse-test the generated file with `zsh -n`.

### F15 — High — Neovim exposes a no-op Plugins field and only one real colorscheme mapping

**Evidence:** `NeovimConfig.Plugins` is declared at
`internal/tools/neovim.go:15-31` but is never read by the generator/writer. Theme
mapping handles only `tokyo-night` (`neovim.go:296-303`); other dark themes overwrite
four highlight groups, while light themes intentionally do nothing
(`neovim.go:228-245`). LSP output assumes Mason APIs (`:257-290`) without a
capability check in this package.

**Trigger:** choose plugin options or expect one of the advertised 16 themes to be
applied consistently to Neovim.

**Impact:** dashboard selections can be inert; themes are partial and can conflict
with preset colorschemes. The aesthetic promise is materially incomplete.

**Refutation attempted:** the comments admit full per-theme theming is out of scope
(`:235-237`) and the overlay is intentionally safer than replacement. That explains
the limitation but does not make the exposed field or global theme claim true.

**Fix:** remove/disable Plugins until implemented; make preset capabilities explicit;
map supported theme plugins per preset or label Neovim theming as “accent overlay”;
validate Mason presence and report degraded LSP setup rather than assuming it.

### F16 — High — Active documentation and agent skills can generate wrong release work

**Evidence:** the available `.agents` pre-PR skill says 13 themes, uses the removed
`theme --list` syntax, old `~/projects` locations, and legacy formula assumptions
(`.agents/skills/pre-pr-tests/SKILL.md:16-18`, `:75`, `:114`, `:266-318`). The newer
`.claude` copy says 16 themes/current commands. The add-tool reference documents
removed `GenerateConfig`/`ApplyConfig` interface methods
(`.claude/skills/add-tool/references/tool-interface.md:21-25`, `:153-211`), and its
example recommends overrides that no longer compile
(`examples/cli-utility.go:56-65`).

**Trigger:** an agent follows the cataloged skill to test a release or add the six
planned tools.

**Impact:** false test failures/omissions and new code built against a removed API;
the automation intended to improve consistency instead amplifies drift.

**Refutation attempted:** the `.claude` pre-PR copy is newer, but the current skill
catalog selected `.agents`, proving the stale copy is operational, not archival.

**Fix:** maintain one canonical skill source, generate/symlink mirrors, add a CI drift
check, and rewrite add-tool guidance around the current Tool/capability model.

## Medium-severity findings

### F17 — Medium — Registry platform APIs ignore part of their own platform contract

**Evidence:** `AllForSystem` filters only heavy tools, not package availability or
`PlatformFilter` (`internal/tools/registry.go:240-254`).
`NotInstalledForSystem` checks packages but not `PlatformFilter` (`:257-283`).
`InstallAll` and `InstallByCategory` call every selected tool without platform or
lightweight filtering (`:300-321`).

**Trigger/impact:** callers outside current UI filtering can show/install irrelevant
tools; planned macOS-only T3 is especially exposed. **Refutation:** some UI screens
may filter independently, but the registry methods are public and named as the
authoritative system-aware APIs. **Fix:** one `AvailableOn(SystemFacts)` predicate
used by every listing/install path and tested for all platforms.

### F18 — Medium — Registry silently overwrites duplicate IDs and blocks reads during probes

**Evidence:** `Register` assigns a map key with no collision error
(`internal/tools/registry.go:90-93`). `ensureCache` holds the exclusive mutex while
calling every tool's subprocess-heavy `IsInstalled` serially (`:127-140`).

**Trigger/impact:** a new ID collision silently removes a tool; cache refresh can
freeze status readers and scales poorly as six agents are added. **Refutation:** the
current registry tests count tools, which may catch some collisions, but not a future
same-count replacement; UI has a separate async layer, but this registry contract
still blocks. **Fix:** collision error/panic during construction; snapshot tools,
probe concurrently with a bound, then atomically publish results with timestamps.

### F19 — Medium — Debian package support comments have aged out

**Evidence:** LazyGit omits Debian (`internal/tools/lazygit.go:35-40`) and Glow omits
it (`glow.go:40-45`) because comments say they are not in stock repositories.
Current Debian stable package search includes both `lazygit` and `glow` (as well as
`git-delta`, eza, and zoxide).

**Trigger/impact:** Debian/Pi users cannot select/install packages that now exist.
**Refutation:** Yazi was also checked and does not appear in Debian stable, so its
omission is not generalized as wrong. **Fix:** CI-generated package availability
matrix per supported distro/version; avoid timeless comments about mutable repos.

### F20 — Medium — Glow UI has three pager values but only two generated outcomes

**Evidence:** `GlowConfig.Pager` documents `auto`, `less`, `never`
(`internal/tools/glow.go:13-18`), but generator emits a boolean based only on
`!= "never"` (`:70-71`). `auto` style is forced to `dark` (`:63-68`) even for a
selected light global theme.

**Trigger/impact:** “auto” and “less” are indistinguishable; light themes produce an
unexpected dark renderer. **Refutation:** current Glow's YAML pager is boolean, so
the generator may be schema-valid; the defect is the UI/model promise, not merely
serialization. **Fix:** expose the actual boolean or implement pager command through
the supported mechanism; derive style from theme luminance and preserve custom
styles.

### F21 — Medium — Yazi “always preview” is only zero image delay and all three files are replaced

**Evidence:** `always` only maps `image_delay` to zero
(`internal/tools/yazi.go:115-129`); `never` replaces `previewers` with an empty array
(`:77-110`). The writer replaces `yazi.toml`, `keymap.toml`, and `theme.toml`
(`:210-237`), including openers/rules/plugins the UI does not model.

**Trigger/impact:** selecting preview mode does not provide the advertised general
semantics, while any save destroys custom opener/plugin/keymap configuration.
**Refutation:** comments correctly acknowledge Yazi has no single toggle, and emitted
`[mgr]` schema is current; that does not make the labels or ownership safe. **Fix:**
rename to precise image-preview behavior, use Yazi flavor/merge facilities where
possible, and preserve/open a diff for unmodeled sections.

### F22 — Medium — `HasConfig` can mean “a path exists,” not “this app manages config”

**Evidence:** simple metadata gives Bat and ripgrep config paths
(`internal/tools/simple_tools.go:34-49`, `:79-92`) and LazyDocker a config path while
explicitly having no generator (`:136-155`). `BaseTool.HasConfig` derives from paths,
so status/configuration language can imply management despite no writer/screen.

**Trigger/impact:** existing files look like configured features and docs claim
post-install config/theme work that does not happen. **Refutation:** LazyDocker's
comment intentionally suppresses `dotfiles config`; the semantic conflation remains
in the common interface. **Fix:** separate `ObservedConfigPaths`, `CanImport`,
`CanConfigure`, and `Ownership` capabilities.

### F23 — Medium — Tailscale/Sunshine/Moonlight stop at package installation

**Evidence:** `internal/tools/tailscale.go`, `sunshine.go`, and `moonlight.go` contain
metadata/detection but no post-install state machine, service health, authentication,
pairing, firewall, or readiness checks.

**Trigger/impact:** the dashboard can report success while VPN/streaming is unusable;
mock-enterprise Tailscale tests cannot represent ACL/tag/device-auth posture.
**Refutation:** package installation may be the intentionally narrow scope, but docs
and dashboard feature grouping do not consistently say “install only.” **Fix:** label
install-only explicitly, or add provider-specific guided post-install/health steps
with no secret persistence.

### F24 — Medium — GUI app entries are install/detect only, not integrations

**Evidence:** Zen, Cursor GUI, LM Studio, OBS, Rectangle, Raycast, IINA, and AppCleaner
in `internal/tools/apps.go` have empty config paths and detection/install metadata;
no preference import/export exists. This is distinct from the planned Cursor Agent.

**Trigger/impact:** users reasonably interpret dashboard presence as settings
integration; already-installed settings are not reflected because none are read.
**Refutation:** comments say the zero screen is a group screen, so the code is not
pretending to have a per-app editor; user-facing docs still need an explicit
capability label. **Fix:** show badges such as `Install only`, `Detect`, `Configure`,
`Health`; do not call install-only entries integrations.

### F25 — Medium — README mixes Go and legacy features and contains hotkey drift

**Evidence:** README's main “What It Installs & Configures” table includes fastfetch,
sshh, macmon, and disk/network tools not in the Go registry
(`README.md:48-77`); legacy scoping appears only later. It claims themes apply
consistently to a subset (`:155-162`) and “All existing configs are backed up…Fully
reversible” (`:197-207`). Its Emacs Yazi row says Ctrl-c/x/v and F2
(`:168-175`), but the Go generator emits C-p/n/b/f navigation and y/x/p/r operations
(`internal/tools/yazi.go:179-203`).

**Trigger/impact:** local testers expect missing features/hotkeys and overestimate
backup safety. **Refutation:** the legacy installer really does provide some listed
tools/hotkeys; the problem is product-path ambiguity, not invented features. **Fix:**
separate a capability matrix by Go vs legacy, then retire legacy; generate hotkey
docs from the same definitions as config output.

### F26 — Medium — `docs/tools.md` package and configuration claims are inaccurate

**Evidence:** it calls LazyGit/LazyDocker/Glow “all platforms”
(`docs/tools.md:316-324`) while code omits Debian; LM Studio says Arch package
`lm-studio` (`:327-334`) while code uses `lmstudio-bin`
(`internal/tools/apps.go:123-138`). Elsewhere the doc claims config generation for
install-only Bat/LazyDocker and uses the wrong macOS Glow path.

**Trigger/impact:** testers choose unsupported flows and maintainers update the wrong
package/path. **Refutation:** some discrepancies reflect upstream availability moving
after code, which is exactly why mutable data should be generated/verified. **Fix:**
generate registry/package/config capability tables from a manifest and date-stamp
external availability checks.

### F27 — Medium — Security scanning documentation describes old non-blocking CI

**Evidence:** `docs/security-scanning.md:64-135` says govulncheck,
golangci-lint, and Staticcheck use `continue-on-error`, older actions, and `latest`.
Current CI makes them blocking and pins tool versions (`.github/workflows/ci.yml:31-97`).

**Trigger/impact:** release reviewers misunderstand what gates are enforced and may
follow obsolete remediation instructions. **Refutation:** the doc correctly says
there is no standalone security workflow/SARIF, but its core job behavior is stale.
**Fix:** update from current workflow or generate this section; add a docs/CI drift
test.

### F28 — Medium — Duplicate skill trees have already diverged

**Evidence:** `.agents/skills/pre-pr-tests/SKILL.md` is 448 lines and
`.claude/skills/pre-pr-tests/SKILL.md` is 481 lines with conflicting command, theme,
path, and Homebrew formula guidance. See F16 for exact representative lines.

**Trigger/impact:** different agents produce different QA results. **Refutation:**
both were read; this is not line-ending or metadata-only drift. **Fix:** one canonical
file plus generated mirrors/checksum enforcement.

### F29 — Medium — Active release task claims are already false on the target Mac

**Evidence:** `tasks/todo.md` reports tests/race/gofmt green and records release
readiness work, but F08 reproduces a current tools test failure. Historical audit
documents have status banners, while `LinuxLocalTesting/Notes:34-70` still presents a
now-resolved ApplyConfig architecture gap as “Remaining Issues.”

**Trigger/impact:** a reviewer treats task prose as verification evidence and skips
fresh tests. **Refutation:** the task result may have been true on Linux at the time;
it is not cross-platform proof. **Fix:** record command, OS/arch, commit SHA, date,
and output artifact; archive resolved Notes and auto-expire stale test attestations.

### F30 — Medium — Legacy restore reports success even when restores fail

**Evidence:** outer restore prints success then only warns when `errors > 0` and
returns normally (`bin/dotfiles-setup:1194-1203`). The embedded CLI does the same
(`:4045-4049`).

**Trigger/impact:** automation and users receive a zero/success completion despite
partial rollback. **Refutation:** a visible warning is printed interactively, but it
does not provide a failing exit status. **Fix:** return nonzero on any error and emit
a machine-readable per-path result; test forced permission failures.

### F31 — Medium — Legacy install path executes mutable remote code/dependencies

**Evidence:** the script contains unpinned Homebrew/zoxide install scripts, branch
clones, and package/tool installs without release digests; `--no-backup` permits
destructive replacement. `shellcheck` found no syntax errors but there are no tracked
integration tests for this 4,346-line path.

**Trigger/impact:** remote compromise/upstream drift or non-repeatable enterprise
deployment. **Refutation:** HTTPS and package managers reduce casual tampering but do
not provide artifact identity/reproducibility. **Fix:** retire the path; otherwise pin
versions/commits/digests, download then verify, never pipe to a shell, and publish an
offline manifest.

### F32 — Medium — Generator boundaries lack consistent validation

**Evidence:** Ghostty directly emits font size, family, opacity, cursor, and
scrollback (`internal/tools/ghostty.go:58-100`); tmux/btop/lazygit/glow similarly
trust model values. Some helpers validate enumerations (good examples:
`yazi.go:131-155`, Zsh alias validation at `zsh.go:330-350`), but there is no common
schema or parse validation before replacement.

**Trigger/impact:** corrupt/stale JSON, future UI bugs, or imported values generate
invalid/unusable configs (for example opacity 0 or size 0). **Refutation:** current UI
adjusters may clamp many fields; writers are also called by tests/other flows and
must enforce invariants at the boundary. **Fix:** typed field schema with enum/range
validation plus per-tool parser/CLI validation before atomic commit.

## Low-severity findings

### F33 — Low — Stale `.golangci.bck.yml` can be used accidentally

**Evidence:** `.golangci.bck.yml` is a 95-line older config beside the 220-line active
`.golangci.yml`. **Trigger/impact:** local/automation invocation with the backup file
produces different lint results. **Refutation:** default golangci-lint ignores it.
**Fix:** delete it from tracked source or archive it outside the repo with provenance.

### F34 — Low — Root project guidance is duplicated byte-for-byte

**Evidence:** `AGENTS.md` and `CLAUDE.md` are each 191 lines and byte-identical.
They already contain stale size facts (legacy script described as ~3,700 lines vs
4,346). **Trigger/impact:** future edits drift and double review burden.
**Refutation:** identical content avoids current behavioral conflict. **Fix:** one
canonical document with a generated mirror/check.

### F35 — Low — Release action dependencies use floating major tags

**Evidence:** `.github/workflows/ci.yml` uses `actions/checkout@v4`,
`actions/setup-go@v5`, and `golangci/golangci-lint-action@v7`; Claude workflows also
use major tags. **Trigger/impact:** upstream tag movement changes trusted CI code,
which mock-enterprise reviews commonly reject. **Refutation:** major tags are standard
for many projects and permissions are mostly scoped; they are still mutable.
**Fix:** pin full commit SHAs and use a dependency updater.

### F36 — Low — Shellcheck warnings signal maintainability debt in the legacy path

**Evidence:** `shellcheck -x` reports unused variables, quoted regex RHS, unsafe HOME
pattern removal, declaration/assignment coupling, `ls` parsing, and unquoted
`gsettings` key use (SC2034, SC2076, SC2295, SC2155, SC2012, SC2086), plus intentional
subshell-HOME warnings. **Trigger/impact:** edge-case path/word-splitting defects and
noise that hides new warnings. **Refutation:** no Shellcheck error-level/parser
failure occurred. **Fix:** clear/suppress each warning locally with rationale, then
make Shellcheck a blocking CI job if legacy remains.

## Per-tool settings and completeness matrix

The correct goal is not to expose every upstream knob. The installed Ghostty 1.2.3
reports roughly **634 documented default keys**, while Manage exposes eight Ghostty
controls and the struct has nine fields. A scalable UX needs:

1. **Common:** curated safe controls with clear ownership and imported current value.
2. **Advanced:** searchable schema-driven fields relevant to the installed version.
3. **Raw/import:** view active source chain, preserve unknown keys, and edit a managed
   fragment or native raw file with validation/diff.
4. **Health:** installation source, primary binary/app, dependencies, active config,
   parse status, service/auth readiness.

| Registered tool | What is currently modeled/generated | Important upstream/current-state gap | Ownership/completeness verdict |
|---|---|---|---|
| Zsh | Prompt, 2 plugins, history, autocd/correction/completion, six alias toggles, navigation-derived content | PATH/env modules, broad history policy, prompt dependency lifecycle, arbitrary plugins; `.zshenv` is listed but not written | **Best current pattern:** bounded managed section preserves user content; medium feature coverage |
| Ghostty | Font, size, opacity, blur, tabs, scrollback, cursor, decoration, confirm-close; hardcoded theme/shell/blink/GTK values | Local padding, mouse hide, window state, shell detection, clipboard/paste protection, keybinds; macOS second config; invalid blur key | **Unsafe full replacement; low settings coverage; status inaccurate** |
| tmux | Prefix/splits/status/mouse/index/borders/history/escape/resize and TPM plugins | Copy mode/clipboard, terminal features, shell, formats, user keybinds, session behavior | **Unsafe full replacement; medium coverage** |
| Neovim | Preset, LSPs, tabs/wrap/cursor/clipboard/numbers/undo; overlay | Plugins field no-op, Mason assumptions, only one colorscheme mapping, preset-specific integration | **Overlay is safer; feature/theme completeness low-medium** |
| Yazi | Keymap, hidden, preview approximation, sort, linemode, scrolloff, theme | User openers/rules/tasks/mouse/plugins/flavors; “always” semantics; legacy schema broken | **Unsafe 3-file replacement; medium coverage** |
| Git | Branch/pull/push/credential/sign toggle/diff/merge/five aliases/delta | Identity, signing key/format, LFS, includes, credential sections, arbitrary aliases, rerere/prune/safe-directory | **Critical unsafe replacement; low coverage for a global config** |
| LazyGit | Side-by-side, mouse, light/dark, pager | Confirmations, updates, branch defaults, services, keybinds/custom commands, signing/editor | **Unsafe replacement; low coverage** |
| fzf | Preview, height/layout, border, window, raw options, theme | Binds, multi/history/search scheme/commands; raw value is shell code risk | Managed-owned file but full replacement; medium coverage if raw field is removed/typed |
| btop | Theme, update, temperature, graph, scale, boxes | Process sort/tree, disks, net interface/scales, GPU, logging, vim/mouse behavior | **Unsafe replacement; low-medium coverage** |
| Glow | Style, pager, width, mouse | 3-value pager collapses to boolean; auto always dark; platform path docs drift | **Unsafe replacement; UI semantics incomplete** |
| Claude Code | MCP server selection only | Tool path points at settings file while apply owns `.claude.json`; no core model/permissions/hooks/enterprise policy | Label **MCP-only**; merge behavior is preferable but path/readiness wrong |
| Tailscale | Package/detection | Login, daemon/service, device authorization, ACL tags, exit-node/DNS, health | **Install only, not deployment-ready integration** |
| Sunshine | Package/app detection | Service, web setup, encoder, firewall, credentials, pairing, health | **Install only** |
| Moonlight | Package/app detection | Host discovery/pairing, resolution/codec/controller/network validation | **Install only** |
| bat | Package + observed config path | No writer/import/theme install despite docs implications | **Install/detect only; `HasConfig` misleading** |
| eza | Package | Alias behavior lives in Zsh, no standalone preferences | **Install only** |
| zoxide | Package | Shell initialization/alias is hardcoded in Zsh, no status/import | **Install plus implicit Zsh integration** |
| ripgrep | Package + observed config path | No writer/import/config editor | **Install/detect only; `HasConfig` misleading** |
| fd | Package | No configuration/integration beyond install | **Install only** |
| fswatch | Package | No configuration | **Install only** |
| Delta | Package | Config is embedded in destructive Git writer; no independent adoption | **Install plus unsafe indirect Git config** |
| LazyDocker | Package + observed path | No generator/import; Debian omitted | **Install only; explicit code comment but docs overstate** |
| Zen Browser | Package/app/flatpak/AppImage detection | No settings/profile integration | **Install/detect only** |
| Cursor GUI | Package/app/AppImage detection | No editor settings/extensions/profile integration; distinct from planned `cursor-agent` | **Install/detect only** |
| LM Studio | Package/app detection | No model path/runtime/server settings or health | **Install/detect only** |
| OBS Studio | Package/app detection | No profiles/scenes/encoder/plugin integration | **Install/detect only** |
| Rectangle | macOS package/app detection | No preferences import/export | **Install/detect only** |
| Raycast | macOS package/app detection | No settings/extensions/hotkeys integration | **Install/detect only** |
| IINA | macOS package/app detection | No preferences | **Install/detect only** |
| AppCleaner | macOS package/app detection | No preferences | **Install/detect only** |

### Planned but absent

OpenAI Codex CLI, Cursor Agent, OpenCode, Pi Coding Agent, T3 Code, and Hermes are not
registered. They exist only in `tasks/new-tools-spec.md`. Their absence is therefore
not evidence of an older local build: **the current source does not implement them**.
Do not add them as six new bespoke files/screens following the present pattern. First
add the schema/capability and safe-install policy below, then model them as opt-in
install/auth integrations.

## Recommended target architecture

The current split—one custom struct, custom generator, hand-built UI screen, registry
metadata, tests, and prose docs per tool—will not scale to upstream settings breadth or
the planned agent set. Use a data-driven capability manifest with escape hatches:

```text
ToolManifest
  id, displayName, category, riskClass
  supportedPlatforms / architecture / minimumVersions
  providers[]: package, cask, release artifact, manual
  probes[]: executable, app bundle, package receipt, service
  dependencies[]: required | optional, plus reason
  configSources[]: path, precedence, format, ownership, version applicability
  fields[]: upstreamKey, type, enum/range, default, common/advanced, secret flag
  importer / merger / writer / validator (custom adapter only where needed)
  postInstallSteps[] and healthChecks[]
  auth/dataEgress/permissions disclosure
  docs metadata and hotkeys/aliases
```

Generate common/advanced screens, help tables, docs, status reasons, and table-driven
tests from that manifest. Keep custom adapters for Zsh section merging, Neovim preset
overlays, Git include editing, and Claude MCP JSON. This preserves flexibility without
forcing every upstream setting into a form.

Config lifecycle should be:

```text
discover sources -> identify active precedence -> import known + preserve unknown
-> render current value/provenance -> edit validated model -> preview semantic/raw diff
-> immediate backup -> validate generated candidate -> atomic commit -> health check
```

The dashboard should never use “installed” as a catch-all. Display separate states:
`available`, `primary installed`, `dependencies`, `config found`, `adopted/managed`,
`valid`, and `service/auth ready`.

## Aesthetics, ergonomics, hotkeys, and discoverability

The generated terminal configs have a recognizable 16-theme visual strategy and the
direct Ghostty/tmux/fzf/Yazi palettes are internally coherent. The strongest aesthetic
work is in the theme palette reuse and bounded Zsh/fzf integration. The visible quality
problems are architectural rather than merely cosmetic:

- A “unified theme” is not unified when Glow forces auto to dark and Neovim maps only
  Tokyo Night, with other themes reduced to a few highlight overrides.
- The dashboard can make an existing terminal look less polished by deleting padding,
  keybindings, clipboard policy, window state, and other unmodeled Ghostty choices.
- Hardcoded cursor blinking, shell integration, and platform-irrelevant GTK settings
  substitute generator opinion for imported user state.
- Install-only apps look like integrated features unless capability badges explain the
  difference.
- README's Emacs Yazi keys describe the legacy script, while the Go generator uses a
  different map. Hotkey help/docs/config generation need one source of truth.
- Git exposes only five aliases and deletes unknown aliases; Zsh is better because it
  imports saved hotkey aliases and uses a managed section. Reuse that ownership model.

For local testers, add a “Why does this say not installed?” details view showing every
probe and missing optional dependency, and a “Which config is active?” view showing
path precedence. Those two screens would resolve most ambiguity better than more
checkboxes.

## Repo strategy and cleanup decision

### Keep this repository (recommended)

Reasons to retain it:

- The Go registry, package abstractions, config package, themes, backup work, TUI, and
  tests are substantial reusable assets.
- Git history documents several real remediation rounds and prevents regressions.
- The central defects are replaceable seams (ownership/state/release), not proof the
  entire application language or interaction model is wrong.
- Starting over would reproduce the hardest work—cross-platform detection, backup,
  config precedence, packaging—and likely carry the same conceptual bug into a clean
  history.

Required cleanup:

1. Freeze/remove the legacy installer from supported distribution and default install.
2. Replace config ownership/write primitives before adding features.
3. Introduce capability/status schema and generate docs/tests/screens where practical.
4. Collapse duplicate skills/docs and archive stale planning material.
5. Establish a real signed/checksummed release pipeline and clean-machine matrix.

Start a new repo only if “mock enterprise” becomes the actual product boundary—central
fleet policy, remote execution, audit logs, MDM, multi-tenant auth—or if replacing the
Go TUI entirely. Those are separate products, not a cleanup tactic.

## Rename difficulty: 8/10

**Code-only display rename:** about 4/10.
**Safe product/distribution/data rename:** **8/10**.

Evidence from the whole tracked repository: 118 tracked files contain `dotfiles`; 100
Go module/import occurrences use `github.com/tekierz/dotfiles`; 90 tracked files touch
high-impact command/config/repository/distribution strings such as `bin/dotfiles`,
`~/.config/dotfiles`, backup paths, Brew commands, or GitHub URLs. Blast radius includes:

- Go module/import path and repository URL.
- Executable/subcommand references, shell aliases, generated comments and managed
  markers.
- `~/.config/dotfiles`, backups/manifests, users/settings, and migration discovery.
- Homebrew formula/tap, release assets, install docs, CI/actions, and sibling repos.
- The public legacy script and its embedded duplicate CLI.
- Skills, tasks, screenshots/help text, and tests with tool-count/brand assumptions.

Recommended no-break migration:

1. Choose a brand whose CLI/package/domain are available; inventory sibling repos and
   formula ownership first.
2. Change display name first while keeping `dotfiles` executable/config directory.
3. Add the new executable name with a deprecation shim/alias for `dotfiles` for at
   least one release.
4. Dual-read old/new config roots; atomically copy once with a migration version and
   rollback; write only the new root after confirmed migration.
5. Recognize old and new managed markers/includes indefinitely or migrate them
   idempotently.
6. Publish a Homebrew transition (`conflicts_with`/`replaces`/alias as appropriate),
   preserve repo redirects, and test upgrade/uninstall from the last old-name release.
7. Change Go module path last, when the repository and release redirects exist.

Do the rename after safe config ownership but before public beta; otherwise every
installed tester adds another persisted-data and package migration case.

## Staged release gates

### Gate 0 — Destructive-path freeze

- Disable/hide Git and whole-file Save/Apply on adopted configs, or require an explicit
  replacement preview until safe mergers land.
- Fix Ghostty active-source detection and blur key.
- Fix the macOS Glow test and add macOS tests to CI.
- Remove legacy Bash from recommended/default install.

### Gate 1 — Safe local alpha (disposable user/VM)

- Atomic/no-follow writer with explicit chmod and per-tool validation.
- Import/provenance/diff/backup for every current writer.
- Separate installed/dependency/config/service states.
- Truthful capability badges and generated docs/hotkeys.
- Full macOS clean-user install, adopt-existing, theme-switch, restore, uninstall tests.

### Gate 2 — Friends/family limited test

- Linux Debian/Arch and macOS Intel/Apple Silicon matrix; Pi/ARM compile and at least
  one hardware smoke test.
- Signed/checksummed release artifacts and Homebrew upgrade/rollback tests.
- Crash/interruption, symlink, permission, space-in-path, partial-package, and offline
  tests.
- Telemetry remains opt-in/non-secret; provide a redacted diagnostic bundle.

### Gate 3 — Mock enterprise

- No default AI-agent installs; package/provenance allowlist and offline/proxy support.
- Auth/service readiness without storing secrets; configuration policy/audit trail.
- Pinned GitHub Actions and dependencies, SBOM/provenance, vulnerability policy.
- Multi-user permissions, MDM-managed config detection, least-privilege installer, and
  explicit conflict behavior for externally managed files.

## Coverage manifest

The following is the exact assigned tracked baseline before audit-plan edits. Total:
**62 files / 17,887 lines**. The original draft overstated `tasks/todo.md` by 13
lines; this corrected total reconciles with the other two disjoint partitions and
the 51,980-line repository baseline.

| Lines | Tracked file |
|---:|---|
| 448 | `.agents/skills/pre-pr-tests/SKILL.md` |
| 170 | `.claude/skills/add-tool/SKILL.md` |
| 65 | `.claude/skills/add-tool/examples/cli-utility.go` |
| 84 | `.claude/skills/add-tool/examples/crossplatform-app.go` |
| 53 | `.claude/skills/add-tool/examples/macos-app.go` |
| 210 | `.claude/skills/add-tool/references/helper-functions.md` |
| 246 | `.claude/skills/add-tool/references/tool-interface.md` |
| 481 | `.claude/skills/pre-pr-tests/SKILL.md` |
| 137 | `.github/workflows/ci.yml` |
| 44 | `.github/workflows/claude-code-review.yml` |
| 50 | `.github/workflows/claude.yml` |
| 39 | `.gitignore` |
| 95 | `.golangci.bck.yml` |
| 220 | `.golangci.yml` |
| 191 | `AGENTS.md` |
| 191 | `CLAUDE.md` |
| 21 | `LICENSE` |
| 143 | `LinuxLocalTesting/Notes` |
| 109 | `Makefile` |
| 296 | `README.md` |
| 4,346 | `bin/dotfiles-setup` |
| 139 | `docs/archive/beta.plan.md` |
| 335 | `docs/archive/v2-analysis-plan.md` |
| 215 | `docs/security-scanning.md` |
| 358 | `docs/tools.md` |
| 38 | `internal/AGENTS.md` |
| 178 | `internal/tools/AGENTS.md` |
| 616 | `internal/tools/apps.go` |
| 199 | `internal/tools/apps_test.go` |
| 224 | `internal/tools/btop.go` |
| 103 | `internal/tools/claude_code.go` |
| 221 | `internal/tools/fzf.go` |
| 188 | `internal/tools/ghostty.go` |
| 252 | `internal/tools/git.go` |
| 123 | `internal/tools/glow.go` |
| 45 | `internal/tools/glow_test.go` |
| 171 | `internal/tools/lazygit.go` |
| 57 | `internal/tools/moonlight.go` |
| 461 | `internal/tools/neovim.go` |
| 147 | `internal/tools/neovim_test.go` |
| 356 | `internal/tools/registry.go` |
| 618 | `internal/tools/registry_test.go` |
| 189 | `internal/tools/simple_tools.go` |
| 54 | `internal/tools/sunshine.go` |
| 46 | `internal/tools/tailscale.go` |
| 326 | `internal/tools/tmux.go` |
| 50 | `internal/tools/tmux_test.go` |
| 184 | `internal/tools/tool.go` |
| 155 | `internal/tools/tool_test.go` |
| 238 | `internal/tools/yazi.go` |
| 90 | `internal/tools/yazi_test.go` |
| 389 | `internal/tools/yazi_theme.go` |
| 413 | `internal/tools/zsh.go` |
| 225 | `internal/tools/zsh_test.go` |
| 103 | `scripts/install-hooks.sh` |
| 73 | `tasks/archive/audit-remediation-todo.md` |
| 499 | `tasks/archive/deep-audit-findings.md` |
| 132 | `tasks/archive/manual-test-plan.md` |
| 457 | `tasks/new-tools-spec.md` |
| 207 | `tasks/pre-release-adversarial-review-2026-07-04.md` |
| 1,222 | `tasks/release-audit-2026-07-03.md` |
| 152 | `tasks/todo.md` |

## Verification limitations

- Sibling `homebrew-tap` and `sshh` repositories were not available at the documented
  local paths, so formula version/SHA/install and cross-repo compatibility are open.
- `golangci-lint` and `govulncheck` were not installed locally in this lane; CI config
  was audited, but those binaries were not re-run here.
- No destructive TUI Apply was performed against the user's real config. Existing
  files were inspected read-only; the loss behavior follows directly from the writer
  implementations and was corroborated by diffing generated coverage against local
  key names.
- Fast-moving AI tool install details must be re-verified against official sources at
  implementation time, as the spec itself acknowledges.
