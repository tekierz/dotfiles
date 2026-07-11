# AI integration source verification — 2026-07-10

This note re-validates install and trust assumptions before the planned AI tools
enter the release registry. Upstream install methods are intentionally treated as
mutable facts and must be checked again for every tagged release.

## Verified current sources

| Integration | Current official install surface | Detection | Configuration / trust notes |
|---|---|---|---|
| OpenAI Codex CLI | OpenAI documents the standalone macOS/Linux installer at `https://chatgpt.com/codex/install.sh`; official examples also use `npm install -g @openai/codex@latest`. Homebrew currently publishes the `codex` cask. | `codex` | Authentication is interactive after install. Never confuse the official scoped npm package with the unrelated unscoped package. |
| Cursor Agent | Cursor documents `curl https://cursor.com/install -fsS | bash`. | `cursor-agent` | The installer is mutable remote code and the CLI is currently described as beta. It has full write access in non-interactive mode. |
| OpenCode | OpenCode recommends `curl -fsSL https://opencode.ai/install | bash`, `npm install -g opencode-ai`, or `brew install anomalyco/tap/opencode`; Arch also publishes `opencode`. | `opencode` | Global JSON/JSONC and TUI configs merge with remote, project, inline, and enterprise-managed sources. Managed macOS policy has highest precedence. |
| Pi | Pi documents `npm install -g --ignore-scripts @earendil-works/pi-coding-agent` or its standalone installer. | `pi` | The npm path deliberately disables lifecycle scripts. Pi has project trust but no built-in execution sandbox; extensions run with user authority. |
| T3 Code | Homebrew publishes the `t3-code` cask for macOS 12+. | app bundle `T3 Code` | GUI integration, not a PATH CLI. |
| Hermes Agent | Nous documents a mutable shell installer; current docs also describe PyPI installs followed by post-install dependency setup. | `hermes` | Installs a large agent/runtime stack and can add browser, gateway, messaging, scheduling, memory, and autonomous tool access. Opt-in/high-risk only. |

Primary references:

- https://developers.openai.com/codex/cli/
- https://developers.openai.com/codex/auth/
- https://docs.cursor.com/en/cli/installation
- https://opencode.ai/docs
- https://opencode.ai/docs/config
- https://pi.dev/docs/latest
- https://pi.dev/docs/latest/security
- https://formulae.brew.sh/cask/t3-code
- https://github.com/NousResearch/hermes-agent/blob/main/website/docs/getting-started/installation.md

## Release architecture decision

All six integrations remain opt-in. Package-manager or lifecycle-script-disabled npm
routes are preferred. A mutable remote installer must not be silently piped into a
shell by a normal package action. Before Cursor Agent or the shell-install form of
Hermes can be enabled, the operation plan needs an explicit remote-installer action
that displays the vendor, URL, fetched artifact digest, network/egress implications,
and irreversible scope; execution must consume those exact reviewed bytes.

Settings adapters must preserve upstream precedence instead of claiming one local
file is the complete effective state. OpenCode enterprise-managed policy, project
config, environment overrides, Pi project trust/extensions, and agent authentication
are health/provenance facts, not ordinary booleans. Credentials remain application-
owned and must never be copied into dotfiles-managed JSON or operation journals.
