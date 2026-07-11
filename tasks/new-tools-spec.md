# New AI Coding CLI Tools — Implementation Spec

Status: **superseded prototype — do not implement from this document**. Its
default-on policy, mutable-script examples, OpenCode lineage, and execution model
predate the immutable reviewed-recipe boundary. Current authority is
`tasks/ai-integrations-source-verification-2026-07-10.md`, the release plan, and
the typed recipes in source. This file remains only as historical design context.

This spec adds six AI tools to the dotfiles installer. Five are CLI agents (one,
`hermes`, is opt-in with a warning); one (`t3-code`) is a macOS GUI app modeled as
a Homebrew cask. The five CLI agents are modeled after
`internal/tools/claude_code.go` (npm/curl custom install + custom `IsInstalled`
via `exec.LookPath`), NOT the `simpleTools` data table (that table is brew-formula
only — see `internal/tools/simple_tools.go`). `t3-code` is modeled after the
cask-style GUI apps (`internal/tools/sunshine.go` / `NewCursorTool()` in `apps.go`):
a `packages` map carrying the cask name + custom `IsInstalled` via `hasMacOSApp`.

---

## 1. Per-tool summary table

| User said | Canonical name | macOS install (preferred) | Detect binary | Prereq | Install model | Default | Confidence |
|-----------|----------------|---------------------------|---------------|--------|---------------|---------|------------|
| "codex cli" | OpenAI Codex CLI | `brew install --cask codex` (or `npm i -g @openai/codex`) | `codex` | ChatGPT login or OpenAI API key | cask **or** npm | **enabled** | HIGH |
| "cursor cli" | Cursor Agent (Cursor CLI) | `curl https://cursor.com/install -fsSL \| bash` | `cursor-agent` | Cursor account login | curl-script | enabled | HIGH |
| "open-code" | opencode | `brew install opencode` (or `curl -fsSL https://opencode.ai/install \| bash`) | `opencode` | LLM provider API key | brew formula **or** curl | enabled | HIGH |
| "pi" | Pi Coding Agent (pi.dev) | `curl -fsSL https://pi.dev/install.sh \| sh` (or `npm i -g --ignore-scripts @earendil-works/pi-coding-agent`) | `pi` | Node/npm; `/login` or provider key (e.g. `ANTHROPIC_API_KEY`) | curl or npm | enabled | **HIGH (confirmed)** |
| "t3" | T3 Code (t3.codes) | `brew install --cask t3-code` | macOS app bundle "T3 Code" (`hasMacOSApp`) | none (cask self-contained) | brew cask (GUI app) | enabled (macOS-only) | **HIGH (confirmed)** |
| "hermes" | Hermes Agent (Nous Research) | `curl -fsSL https://hermes-agent.nousresearch.com/install.sh \| bash` | `hermes` | installer bundles Python 3.11/Node/uv/ripgrep/ffmpeg | curl-script | **NOT default + WARNING** | **HIGH (confirmed)** |

---

## 2. Per-tool detail

### Codex CLI — HIGH confidence
- Canonical: **OpenAI Codex CLI** — "Lightweight coding agent that runs in your terminal."
- Repo: https://github.com/openai/codex ; npm: https://www.npmjs.com/package/@openai/codex
- macOS install: `brew install --cask codex` (cask wraps the Rust binary OpenAI ships)
  OR `npm install -g @openai/codex`. **Package is `@openai/codex` — the unscoped
  `codex` npm package is an unrelated 2012 project; do not use it.**
- Linux: `npm install -g @openai/codex`, or download the binary from GitHub Releases. No known apt/pacman package.
- Detect: `codex` on PATH.
- Prereq: ChatGPT login (Plus/Pro/etc.) recommended, or OpenAI API key. npm route needs Node.
- Recommended default: **enabled**.

### Cursor CLI (Cursor Agent) — HIGH confidence
- Canonical: **Cursor Agent CLI** — Cursor's terminal coding agent (beta).
- URL: https://cursor.com/cli ; blog: https://cursor.com/blog/cli
- macOS/Linux install: `curl https://cursor.com/install -fsSL | bash`
- Detect: **`cursor-agent`** on PATH (NOT `cursor` — that's the GUI editor, which
  this repo already has as `NewCursorTool()` in registry.go). Verify with
  `cursor-agent --version`.
- Prereq: Cursor account login. curl script self-contains the binary (no Node needed).
- Recommended default: **enabled**.

### opencode — HIGH confidence
- Canonical: **opencode** — open-source AI coding agent built for the terminal (TUI).
- URL: https://opencode.ai/docs/ ; repo: https://github.com/sst/opencode ; npm: `opencode-ai`
- macOS install: `brew install opencode` (Homebrew formula) OR
  `curl -fsSL https://opencode.ai/install | bash` OR `npm i -g opencode-ai@latest`.
- Linux: `curl -fsSL https://opencode.ai/install | bash`; Arch: `paru -S opencode-bin`.
- Detect: `opencode` on PATH.
- Prereq: at least one LLM provider API key (supports 75+ providers).
- NOTE on naming: two GitHub orgs exist — **`sst/opencode`** (current, active,
  `opencode-ai` npm, `opencode.ai` site) and the older `opencode-ai/opencode`. Use
  the `sst` lineage (the `opencode.ai` install script / `opencode-ai` npm package).
- Modeling note: because `brew install opencode` works, opencode is the ONE new
  tool that *could* go in the `simpleTools` table (brew formula, `defaultEnabled:
  true`, `uiGroup: UIGroupCLITools`). But to keep behavior identical across macOS
  (brew) and Linux (curl), prefer the custom-file pattern with curl fallback. Pick
  one and be consistent — see "Open questions".
- Recommended default: **enabled**.

### pi — CONFIRMED (HIGH confidence)
- Confirmed by user: this is the **Pi coding agent at https://pi.dev** — a minimal
  terminal coding harness that extends through TypeScript modules for tools,
  commands, and custom interfaces.
- Docs: https://pi.dev/docs/latest ; install info confirmed from the official docs.
- macOS install (preferred — sidesteps the npm package name): verbatim from docs:
  ```
  curl -fsSL https://pi.dev/install.sh | sh
  ```
  npm route (verbatim from docs — note the **`--ignore-scripts`** flag and the
  **`@earendil-works/pi-coding-agent`** package; the earlier `@mariozechner/...`
  guess is WRONG, the official docs use the `@earendil-works` scope):
  ```
  npm install -g --ignore-scripts @earendil-works/pi-coding-agent
  ```
- Detect: **`pi`** on PATH. (Caution: `pi` is a short, collision-prone binary name,
  but it is the confirmed binary; `exec.LookPath("pi")` is correct.)
- Prereq: Node.js/npm (for the npm route); authentication via `/login` for
  subscription providers, or set a provider API key such as `ANTHROPIC_API_KEY`
  before first run. The curl route bundles its runtime — no separate Node install
  needed, matching the curl-script pattern (§3b).
- Recommended default: **enabled**.
- Modeling: prefer the **curl-script** pattern (§3b) — `packages` empty, custom
  `Install()` runs `curl -fsSL https://pi.dev/install.sh | sh`. (If you choose the
  npm route instead, use §3a but the install args are
  `["install", "-g", "--ignore-scripts", "@earendil-works/pi-coding-agent"]`.)

### t3 — CONFIRMED (HIGH confidence) — macOS GUI app via Homebrew cask
- Confirmed by user: **t3 is a desktop/web app and the user WANTS it included** as
  an install option. It is **T3 Code** — "Minimal GUI for AI code agents" — a macOS
  desktop GUI front-end for AI coding agents. Homepage: https://t3.codes/
- A **scriptable install DOES exist**: an official Homebrew cask `t3-code`
  (https://formulae.brew.sh/cask/t3-code, current version 0.0.27). No `.dmg`-only
  fallback is needed.
  ```
  brew install --cask t3-code
  ```
- Platform: **macOS-only** (requires macOS 12+; Apple Silicon + Intel). There is no
  Linux/Windows package — model it as macOS-focused (GUI app).
- This is a **GUI app, not a terminal CLI** — there is no binary on PATH. Detect via
  the macOS app bundle, exactly like sunshine.go/the other GUI apps:
  `hasMacOSApp("T3 Code")`, then fall back to the package-manager check.
- Modeling: clone the cask-style GUI-app pattern (`internal/tools/sunshine.go`, or
  `NewCursorTool()` in `apps.go`) — see §3d. Put the cask name in `packages` under
  `pkg.PlatformMacOS` only; `uiGroup: UIGroupGUIApps` (or `UIGroupMacApps`, since
  it's macOS-only — match whichever group the installer uses for macOS-only casks);
  `category: CategoryApp`. Because it's macOS-only, set the registration's
  `platformFilter: pkg.PlatformMacOS` so it's hidden on Linux (see how the
  `platformFilter: pkg.PlatformMacOS` MacApps entries are wired in registry_test.go).
- Recommended default: **enabled** (user wants it). It's optional like the other GUI
  apps; if you'd rather not auto-check it, `defaultEnabled: false` matches sunshine's
  posture — but the user asked for it as an install option, so `true` is reasonable.
- NOTE: do not confuse with the unofficial community wrapper
  `chrisdesrochers/t3chat-desktop` (a t3.chat web wrapper) — that's a different
  product. Use the official `t3-code` cask (t3.codes).

### hermes — CONFIRMED — opt-in + WARNING (HIGH confidence)
- Confirmed from the repo README (https://github.com/nousresearch/hermes-agent):
  **Hermes Agent** by **Nous Research** — a self-improving autonomous AI agent.
- macOS/Linux install (verbatim from README):
  ```
  curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash
  ```
  After install, reload the shell: `source ~/.zshrc` (or `~/.bashrc`).
- Detect: **`hermes`** on PATH (CONFIRMED — the README states you start it with
  `hermes`). `exec.LookPath("hermes")` is correct.
- Prereq: the **installer handles dependencies automatically** — it pulls in
  Python 3.11, Node.js, `uv`, ripgrep, and ffmpeg (uses system Git on macOS/Linux).
  No separate provider key required to install.
- **What it does / why a warning (confirmed capabilities from the README):** Hermes
  is a self-improving autonomous agent that: creates and self-improves its own
  skills after complex tasks; maintains an agent-curated persistent memory across
  sessions (closed learning loop, session search); reaches external messaging
  platforms (Telegram, Discord, Slack, WhatsApp, Signal) through a single gateway;
  spawns isolated subagents for parallel delegation; runs a built-in cron scheduler
  for unattended daily/weekly automation; and runs anywhere including VPS, GPU
  clusters, and serverless backends (Modal, Daytona) with persistence. It is
  model-agnostic (200+ models). That combination — autonomous self-modification,
  persistent memory, unattended scheduled execution, and reach into chat platforms
  and remote compute — is exactly what makes it powerful and risky for an
  unattended/default install, hence opt-in only. The install method itself is also
  a `curl | bash` pipe to a third-party host.
- Recommended default: **NOT enabled** + warning (see §5).

---

## 3. Code pattern — what each new tool file looks like

Clone `internal/tools/claude_code.go`. Each new tool is its own file
`internal/tools/<id>.go` with a struct embedding `BaseTool`, a `NewXTool()`
constructor, a custom `IsInstalled()` using `exec.LookPath`, and a custom
`Install()`.

### 3a. npm-based tool (Codex npm route, pi npm route) — closest to claude_code.go

```go
package tools

import (
	"fmt"
	"os/exec"

	"github.com/tekierz/dotfiles/internal/pkg"
)

type CodexTool struct{ BaseTool }

func NewCodexTool() *CodexTool {
	return &CodexTool{BaseTool: BaseTool{
		id:          "codex",
		name:        "Codex CLI",
		description: "OpenAI's terminal coding agent",
		icon:        "󰚩", // pick a nerd-font glyph; reuse claude-code's if unsure
		category:    CategoryUtility,
		packages: map[pkg.Platform][]string{ // Node provides npm for the install
			pkg.PlatformMacOS:  {"node"},
			pkg.PlatformArch:   {"nodejs", "npm"},
			pkg.PlatformDebian: {"nodejs", "npm"},
		},
		uiGroup:        UIGroupCLITools,
		defaultEnabled: true,
	}}
}

func (t *CodexTool) IsInstalled() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

func (t *CodexTool) Install(mgr pkg.PackageManager) error {
	if err := t.BaseTool.Install(mgr); err != nil { // ensure node/npm
		return fmt.Errorf("failed to install Node.js (required for Codex CLI): %w", err)
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found after installing Node.js: %w", err)
	}
	cmd := exec.Command(npmPath, "install", "-g", "@openai/codex")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install -g @openai/codex failed: %w: %s", err, out)
	}
	return nil
}
```

NOTE: For Codex you may prefer the **cask** route on macOS (`brew install --cask
codex`) and npm only on Linux. If so, branch in `Install()` on
`pkg.DetectPlatform() == pkg.PlatformMacOS` and call `mgr` cask install vs npm.
Simplest consistent option is the npm route on all platforms (above).

### 3b. curl-script tool (Cursor, opencode-via-curl, pi-via-curl, hermes)

Same shape, but `packages` is empty (no Node needed) and `Install()` runs the
vendor script. Run the script through the user's shell:

```go
func (t *CursorAgentTool) IsInstalled() bool {
	_, err := exec.LookPath("cursor-agent")
	return err == nil
}

func (t *CursorAgentTool) Install(mgr pkg.PackageManager) error {
	// curl https://cursor.com/install -fsSL | bash
	cmd := exec.Command("bash", "-c", "curl https://cursor.com/install -fsSL | bash")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cursor-agent install script failed: %w: %s", err, out)
	}
	return nil
}
```

Security note (CLAUDE.md): we already forbid `source`-ing user profiles; piping a
remote script to `bash` is the vendor's official method here, but call it out in
the tool's description and (for hermes) in the warning. `packages` empty ⇒
`BaseTool.Install` is a no-op, so the curl tool's custom `Install` is the only path.

IMPORTANT (no command injection): the `bash -c` string above must contain ONLY a
hardcoded constant vendor URL — never interpolate user input into it. The URLs
here are fixed literals, so there is no injection surface. If you ever need a
dynamic value, drop the shell and pass args directly:
`exec.Command("curl", "-fsSL", url)` piped programmatically, never
`exec.Command("bash", "-c", "curl ... "+userVar)`. A `go vet`/linter or the
security hook will (correctly) flag any `bash -c` that concatenates a variable.

### 3c. brew-formula tool (opencode, if you choose the brew route)

If modeling opencode purely as a brew formula on macOS, it can be a `simpleTools`
entry instead of a file:
```go
{
	id: "opencode", name: "opencode",
	description: "Open-source terminal AI coding agent",
	icon: "󰚩", category: CategoryUtility,
	packages: map[pkg.Platform][]string{
		pkg.PlatformMacOS: {"opencode"},
		// Arch: paru -S opencode-bin (not an apt/pacman core pkg — may need custom Install)
	},
	uiGroup: UIGroupCLITools, defaultEnabled: true,
},
```
But `IsInstalled` for a simpleTool checks the package manager, not the binary —
acceptable on macOS-brew. For Linux-curl parity, prefer the custom file (§3b).

### 3d. cask GUI-app tool (t3-code) — clone `sunshine.go` / `NewCursorTool()`

t3-code is a macOS GUI app, NOT a CLI. Do NOT clone claude_code.go for it. Clone
`internal/tools/sunshine.go` (or `NewCursorTool()` in `apps.go`): the cask name goes
in `packages` under `pkg.PlatformMacOS` only, and `IsInstalled` checks the macOS app
bundle via `hasMacOSApp` before falling back to the package manager. No `exec.LookPath`
(no binary on PATH).

```go
package tools

import (
	"os/exec"

	"github.com/tekierz/dotfiles/internal/pkg"
)

type T3CodeTool struct{ BaseTool }

func NewT3CodeTool() *T3CodeTool {
	return &T3CodeTool{BaseTool: BaseTool{
		id:          "t3-code",
		name:        "T3 Code",
		description: "Minimal GUI for AI code agents (macOS)",
		icon:        "󰚩", // pick a nerd-font glyph
		category:    CategoryApp,
		packages: map[pkg.Platform][]string{
			pkg.PlatformMacOS: {"t3-code"}, // Homebrew cask; macOS-only
		},
		configPaths:    []string{},
		uiGroup:        UIGroupGUIApps, // or UIGroupMacApps — match the macOS-only-cask group
		configScreen:   0,
		defaultEnabled: true, // user wants it; sunshine uses false — either is fine
	}}
}

// IsInstalled detects the macOS app bundle (GUI app, no binary on PATH),
// mirroring sunshine.go's out-of-band detection before the pkg-manager fallback.
func (t *T3CodeTool) IsInstalled() bool {
	if _, err := exec.LookPath("t3-code"); err == nil { // unlikely, but cheap
		return true
	}
	if hasMacOSApp("T3 Code") {
		return true
	}
	return t.BaseTool.IsInstalled()
}
```

Register with `platformFilter: pkg.PlatformMacOS` (macOS-only) — see how the
`platformFilter: pkg.PlatformMacOS` MacApps entries are wired so the tool is hidden
on Linux. `BaseTool.Install` will `brew install --cask t3-code` via the macOS
package manager from the `packages` map — no custom `Install()` needed.

---

## 4. Registry wiring

In `internal/tools/registry.go`, inside `NewRegistry()`, add to the "Utility
tools" block (next to `NewClaudeCodeTool()`):

```go
r.Register(NewCodexTool())
r.Register(NewCursorAgentTool())
r.Register(NewOpenCodeTool())   // or add to simpleTools table instead
r.Register(NewPiTool())         // CONFIRMED: pi.dev curl installer, binary `pi`
r.Register(NewHermesTool())     // CONFIRMED opt-in: defaultEnabled:false + warning
```

t3-code is a macOS GUI app — register it with the other GUI-app/cask tools (next to
`NewSunshineTool()` / the MacApps casks), with `platformFilter: pkg.PlatformMacOS`:
```go
r.Register(NewT3CodeTool())     // CONFIRMED: brew cask t3-code, macOS-only GUI app
```

`NewHermesTool()` IS registered (so it appears in the installer list); its opt-in
status is expressed via `defaultEnabled: false` + warning, not by omitting it.

No hotkeys entry needed (these are interactive agents, not hotkey scripts — the
`internal/hotkeys` registry is for the hk/caff/sshh-style bindings). UI group:
`UIGroupCLITools` for all five agents (they're CLI tools, mirroring claude-code).

ID-collision check: the GUI Cursor editor tool is `NewCursorTool()` (likely id
`"cursor"`). Use id `"cursor-agent"` (binary `cursor-agent`) for the CLI to avoid
collision. Verify `NewCursorTool()`'s id in `internal/tools/cursor.go` before wiring.

Update the "30 tools" / count references in CLAUDE.md and any tests that assert a
fixed registry count (search: `Registry.Count`, hardcoded counts in
`internal/tools/*_test.go` and `internal/ui/*_test.go`).

---

## 5. Modeling hermes's opt-in + warning

Two parts: (a) default off, (b) a visible warning before/at install.

**(a) Default off:** `defaultEnabled: false` in `NewHermesTool()` — same mechanism
Tailscale/Sunshine/claude-code already use (they're `false`). This makes the
installer leave the checkbox unselected by default.

**(b) Warning surface.** The `Tool` interface has no warning field today. Pick the
lowest-impact option:

- **Option A (recommended, smallest):** Put the warning in `Description()` with a
  leading marker, e.g.
  `description: "⚠ Autonomous self-improving agent (Nous Research) — runs shell commands & connects to chat platforms. Opt-in."`
  The installer already renders `Description()` next to each tool, so no UI change
  is needed. Lowest risk, ships immediately.

- **Option B (cleaner, more work):** Add an optional `Warning() string` to the
  `Tool` interface (default `""` in `BaseTool`, overridden in `HermesTool`), then
  in the CLI-tools group renderer (`internal/ui/screen_*` that lists CLITools —
  find the handler rendering `UIGroupCLITools`) show the warning in the danger
  color (`#F15BB5` magenta or a red from `internal/ui/styles.go`) and, ideally, a
  confirmation step before install. Requires touching the screen handler + styles
  + interface; do this only if the user wants an explicit confirm gate.

Recommend Option A for v1; note Option B as a follow-up if a confirm-gate is wanted.

**Warning text to use (rationale):**
> Hermes Agent is an autonomous, self-improving AI agent that executes real
> terminal commands and can connect to external platforms (Telegram/Discord/Slack)
> and remote backends (SSH/Docker/Modal). It installs via a `curl | bash` script
> from a third-party host. Enable only if you understand and accept this autonomy.

---

## 6. Open questions to confirm with the user

**RESOLVED (no longer open):**
- ~~**pi**~~ — CONFIRMED as the pi.dev Pi coding agent. Install
  `curl -fsSL https://pi.dev/install.sh | sh` (or npm
  `@earendil-works/pi-coding-agent --ignore-scripts`), binary `pi`. Default enabled.
- ~~**t3**~~ — CONFIRMED. User wants it: T3 Code macOS GUI app via the official
  Homebrew cask `t3-code` (t3.codes). A scriptable install exists
  (`brew install --cask t3-code`); macOS-only, modeled as a cask GUI app (§3d).
- ~~**hermes binary name**~~ — CONFIRMED `hermes` on PATH (per README); opt-in with
  warning retained.
- codex / cursor / opencode — already HIGH confidence; no identity questions.

**Still open (implementation choices, not blockers):**
1. **Codex install route:** cask (`brew install --cask codex`, no Node) vs npm
   (`@openai/codex`, needs Node). Pick one for consistency.
2. **opencode install route:** brew formula (simpleTools entry) vs curl/npm custom
   file. Affects whether it's a data-table tool or its own file.
3. **pi install route:** curl installer (§3b, recommended — no Node needed) vs npm
   (§3a, `@earendil-works/pi-coding-agent --ignore-scripts`). Both confirmed valid.
4. **hermes warning UX:** Option A (warning in description, ships now) vs Option B
   (interface `Warning()` + confirm gate). Confirm desired strength.

---

## 7. Could-not-confirm list

All previously-unconfirmed items are now RESOLVED (verified against official
docs/README, 2026-06-18):

- ~~**t3 as a CLI:**~~ RESOLVED — it's a macOS GUI app (T3 Code, t3.codes), installed
  via the official Homebrew cask `t3-code`. Modeled as a cask GUI app (§3d), not a CLI.
- ~~**pi npm package name:**~~ RESOLVED — official docs use
  `@earendil-works/pi-coding-agent` (with `--ignore-scripts`); curl installer
  `https://pi.dev/install.sh` is the preferred route. The `@mariozechner/...` guess
  was incorrect.
- ~~**hermes binary name:**~~ RESOLVED — README confirms the binary is `hermes`.

(Nothing remains unconfirmed for pi / t3 / hermes. Remaining items in §6 are
implementation-route choices, not unknowns.)

---

## 8. Files to touch (checklist)

- [ ] `internal/tools/codex.go` (new — §3a)
- [ ] `internal/tools/cursor_agent.go` (new — §3b)
- [ ] `internal/tools/opencode.go` (new — §3b) OR add to `internal/tools/simple_tools.go` (§3c)
- [ ] `internal/tools/pi.go` (new — §3b curl preferred / §3a npm; CONFIRMED pi.dev, binary `pi`)
- [ ] `internal/tools/t3_code.go` (new — §3d cask GUI app, macOS-only; CONFIRMED cask `t3-code`)
- [ ] `internal/tools/hermes.go` (new — §3b, defaultEnabled:false + warning §5; CONFIRMED binary `hermes`)
- [ ] `internal/tools/registry.go` — register the new tools (§4), incl. `NewT3CodeTool()` with `platformFilter: pkg.PlatformMacOS`
- [ ] `CLAUDE.md` + any tests asserting tool counts — bump the count (§4)
- [ ] (Option B only) `internal/ui/` CLITools group renderer + `Tool.Warning()` + styles (§5b)
- [ ] Verify `internal/tools/cursor.go` id to avoid collision with `cursor-agent` (§4)
- [ ] `go build ./... && go vet ./... && go test ./...` before PR
