# Manual Test Plan — `worktree-audit-remediation` (macOS)

> **⚠️ STATUS: ARCHIVED 2026-07-03.** This test plan targeted the `worktree-audit-remediation`
> branch, which has been merged to `main`. For current pre-release testing use the
> `pre-pr-tests` skill checklist (`.claude/skills/pre-pr-tests/SKILL.md`).

Focused on what this branch changed. Most of it is **safe** (navigate + quit).
A few steps **modify your real machine** — those are marked ⚠️ with how to stay safe.

## 0. Build & launch the branch binary

```bash
cd ~/Desktop/Projects/dotfiles/.claude/worktrees/audit-remediation
make build
./bin/dotfiles            # main menu
```

Quick automated sanity (should all pass):
```bash
go build ./... && go vet ./... && go test ./... && gofmt -l internal cmd
```

---

## 1. Smoke test — the screen migration (highest priority) — SAFE

Every screen now runs through the new ScreenManager. Just walk the whole app and watch for **panics, blank screens, frozen animation, or wrong screen**. Quit any time with `q` / `Ctrl+C`.

- [ ] `./bin/dotfiles` → intro animation plays and **advances on its own** to the welcome screen (don't `--skip-intro` this first run — we want to see the intro tick).
- [ ] Animated header/spinner keeps moving on later screens (this specifically regressed mid-migration and was fixed — confirm it animates, doesn't freeze after the first screen).
- [ ] `./bin/dotfiles --skip-intro` → lands straight on the wizard, no animation.
- [ ] From the main menu, open **every** entry and `Esc` back out: Install, Manage, Update, Hotkeys, Backups, Users. No crashes, each renders.
- [ ] Resize the terminal while on a few screens — layout reflows, no garbage.

---

## 2. Wizard / installer navigation — SAFE (don't finish the install)

```bash
./bin/dotfiles install
```
> ⚠️ Navigate freely, but **do not proceed past the final Summary → Install step** unless you actually want it to install/overwrite your dotfiles. Browsing the wizard is read-only; the Progress screen is where real changes happen.

- [ ] Theme picker shows **16** themes; ↑/↓ moves; selecting one **live-previews** the colors immediately.
- [ ] Nav-style picker toggles emacs/vim.
- [ ] Deep Dive menu lists tools; open several config screens (Ghostty, Zsh, Neovim, **Claude Code**), edit fields, `Esc` back. Field cursor up/down + left/right adjust works.
- [ ] Checkbox-list screens (CLI tools, GUI apps, macOS apps) toggle items.
- [ ] On macOS the **macOS Apps** screen appears; cursor lands on the right tool when you click (mouse hit-detection was fixed).

---

## 3. Manage screen — SAFE to browse

```bash
./bin/dotfiles manage
```
- [ ] Tool list shows accurate **Installed / Not Installed** badges (it queries brew).
- [ ] Tab switches panes; arrow keys scroll; inline field edit works (`Enter` to edit, type, `Enter` to save, `Esc` to cancel).
- [ ] Mouse: click a tool in the left list, click the tab bar — selects the right thing.
- [ ] (Optional ⚠️ installs a package) Install a small not-installed tool from here and confirm: the **install log streams live**, and when it finishes the badge flips to **Installed without leaving the screen** (this cache-refresh was a fix).

---

## 4. Backups & restore — create is SAFE, restore is ⚠️

```bash
./bin/dotfiles backups          # CLI list — safe
```
- [ ] In the TUI Backups screen, **create a backup** (safe — writes to `~/.config/dotfiles/backups/<timestamp>/`). Confirm it appears in the list.
- [ ] Inspect it: `ls ~/.config/dotfiles/backups/*/` — there's a `manifest.txt` and files named with `_` separators.
- [ ] ⚠️ **Restore** overwrites your real dotfiles. Only test if you just made a backup to restore from. After restore, confirm your `~/.zshrc` etc. are intact (path-mapping + permissions are preserved now).
- [ ] Delete a backup from the screen — list refreshes.

---

## 5. ⚠️ Claude Code MCP config — the critical fix (back up first!)

This was the dangerous bug: it used to **wipe your `~/.claude/settings.json`**. The fix writes MCP servers to `~/.claude.json` (merge-preserving) instead. **Test deliberately:**

```bash
# Back up BOTH first
cp ~/.claude/settings.json ~/.claude/settings.json.testbak 2>/dev/null
cp ~/.claude.json ~/.claude.json.testbak 2>/dev/null
```
- [ ] Trigger the Claude Code MCP apply (select Claude Code in the installer/deep-dive and let it apply its config, or run the manage Claude Code screen).
- [ ] **`~/.claude/settings.json` is UNCHANGED** — `diff ~/.claude/settings.json ~/.claude/settings.json.testbak` shows nothing. (This is the whole point.)
- [ ] `~/.claude.json` still has all your existing keys (projects, history, etc.) **plus** a `mcpServers` entry — `diff` shows only additions, and a `~/.claude.json.bak` was created.
- [ ] MCP package name is `@upstash/context7-mcp` (grep `~/.claude.json` for it), not `@context7/mcp`.
- [ ] Restore your originals if you want: `mv ~/.claude.json.testbak ~/.claude.json` etc.

---

## 6. Hotkeys & Users — SAFE

```bash
./bin/dotfiles hotkeys
```
- [ ] Categories + items render; ↑/↓/←/→ navigate panes.
- [ ] `f` favorites the selected hotkey; `F` toggles favorites-only filter; favorites **persist** across relaunch (cached now, not re-read per frame — should feel snappy with no flicker).

```bash
./bin/dotfiles users
```
- [ ] List + settings pane render; create a user, switch users.
- [ ] Delete the **currently active** user → it doesn't leave a phantom active user (the active user is cleared/reassigned). Confirm `dotfiles status` / relaunch behaves.

---

## 7. CLI commands — SAFE

```bash
./bin/dotfiles version
./bin/dotfiles status
./bin/dotfiles theme list            # NOTE: 'list', not '--list' (doc fix)
./bin/dotfiles theme set dracula     # saves theme; run 'dotfiles install' to apply
./bin/dotfiles update check          # prints outdated packages (shouldn't hang on a no-TTY sudo)
./bin/dotfiles --help
```
- [ ] `theme list` shows 16 themes; `theme --list` should error (flag removed — confirms the doc form).
- [ ] `update check` returns promptly and doesn't block on `sudo` (brew path on Mac).

---

## What to watch for (regressions this branch could introduce)

- **Navigation dead-ends / blank screen** after entering a screen → migration dispatch issue.
- **Frozen header animation** after the first screen → the global-tick fix (should NOT happen).
- **Stuck "loading…" install status** that never resolves → the install-cache global fix (should NOT happen).
- **Install logs not streaming** or the app hanging during install → install-flow message passing.
- **Any `panic:`** in the terminal → capture the stack trace and the screen you were on.

If something's off, note the exact screen + key/mouse action, and (if it panicked) the trace — that's enough to pinpoint it.
