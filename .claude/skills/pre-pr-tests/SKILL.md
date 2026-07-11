---
name: pre-pr-tests
description: Generates comprehensive pre-PR test checklist for dotfiles TUI. Use before merging feature branches to main, when running tests, creating test plans, or doing QA. Includes manual tests, automatic tests, security checks, performance tests, aesthetic review, and cross-repo compatibility with sshh and homebrew-tap.
---

# Pre-PR Testing Checklist for Dotfiles TUI

Run these tests before merging any feature branch to main.

## Related Repositories

These repos are interconnected and may need updates together:

| Repo | Location | Purpose |
|------|----------|---------|
| **dotfiles** | `~/Desktop/Projects/dotfiles` | Main project (this repo) |
| **sshh** | `~/Desktop/Projects/sshh` | SSH connection manager utility |
| **homebrew-tap** | `~/Desktop/Projects/homebrew-tap` | Homebrew formulas for distribution |

> Paths above reflect this checkout's base (`~/Desktop/Projects/`). Adjust to wherever you cloned the repos.

### Integration Flow
```
dotfiles: homebrew-tap/Formula/dotfiles.rb -> Go `dotfiles` binary
sshh:     homebrew-tap/Formula/sshh.rb     -> independent `sshh` utility
```

Both formulas must use immutable release sources and verified SHA256 hashes.

---

## Automatic Tests (Run These First)

Copy and run this test script. It requires `golangci-lint`, `staticcheck`,
`govulncheck`, and `shellcheck` on `PATH`; CI pins the authoritative tool
versions in `.github/workflows/ci.yml`.

> These local checks mirror the GitHub Actions CI workflow (`.github/workflows/ci.yml`, which runs lint/security/test/build jobs). Install the pre-commit hooks once with `bash scripts/install-hooks.sh` so `gofmt`/`go vet` run automatically before each commit.

```bash
#!/bin/bash
set -e
echo "=== PRE-PR AUTOMATED TESTS ==="

# 1. Module integrity
echo -e "\n[1/15] Verifying module downloads..."
go mod verify
echo "GO MOD VERIFY: PASS"

# 2. Module-file cleanliness
echo -e "\n[2/15] Checking go mod tidy diff..."
go mod tidy -diff
echo "GO MOD TIDY: PASS"

# 3. Build Check
echo -e "\n[3/15] Building..."
make clean && make build
echo "BUILD: PASS"

# 4. Go Vet
echo -e "\n[4/15] Running go vet..."
go vet ./...
echo "GO VET: PASS"

# 5. Lint (golangci-lint via make lint; matches CI lint job)
echo -e "\n[5/15] Running golangci-lint..."
make lint
echo "LINT: PASS"

# 6. Staticcheck
echo -e "\n[6/15] Running staticcheck..."
staticcheck ./...
echo "STATICCHECK: PASS"

# 7. Unit Tests
echo -e "\n[7/15] Running go test..."
go test ./...
echo "GO TEST: PASS"

# 8. Race Tests (blocking in CI on both Ubuntu and macOS)
echo -e "\n[8/15] Running race tests..."
go test -race ./...
echo "GO TEST RACE: PASS"

# 9. Format Check
echo -e "\n[9/15] Checking gofmt..."
UNFORMATTED=$(gofmt -l . 2>/dev/null)
if [ -n "$UNFORMATTED" ]; then
    echo "GOFMT: FAIL - Unformatted files:"
    echo "$UNFORMATTED"
    exit 1
fi
echo "GOFMT: PASS"

# 10. Known Go vulnerabilities
echo -e "\n[10/15] Running govulncheck..."
govulncheck ./...
echo "GOVULNCHECK: PASS"

# 11. Shell analysis (repository-maintained development scripts)
echo -e "\n[11/15] Running shellcheck..."
shellcheck scripts/install-hooks.sh
echo "SHELLCHECK: PASS"

# 12. Security: Check for hardcoded secrets
echo -e "\n[12/15] Security scan..."
if grep -RInE "password[[:space:]]*=[[:space:]]*[\"']" --include="*.go" ./internal ./cmd 2>/dev/null | grep -v "Password string"; then
    echo "SECURITY: WARNING - Potential hardcoded password found"
fi
if grep -RInE "api_key[[:space:]]*=[[:space:]]*[\"']" --include="*.go" ./internal ./cmd 2>/dev/null; then
    echo "SECURITY: WARNING - Potential hardcoded API key found"
fi
echo "SECURITY: PASS (manual review recommended)"

# 13. CLI Commands Test
echo -e "\n[13/15] Testing CLI commands..."
./bin/dotfiles --help > /dev/null
echo "  --help: OK"
./bin/dotfiles status > /dev/null 2>&1
echo "  status: OK"
./bin/dotfiles backups > /dev/null 2>&1
echo "  backups: OK"
./bin/dotfiles theme list > /dev/null 2>&1
echo "  theme list: OK"
echo "CLI COMMANDS: PASS"

# 14. Binary Size Check
echo -e "\n[14/15] Binary size..."
SIZE=$(du -h ./bin/dotfiles | awk '{print $1}')
echo "  Binary size: $SIZE"
echo "BINARY SIZE: INFO"

# 15. Startup Time (Bash TIMEFORMAT works on both macOS and Linux)
echo -e "\n[15/15] Startup time..."
TIMEFORMAT='  Startup: %3R seconds'
time ./bin/dotfiles --help > /dev/null
echo "STARTUP TIME: INFO"

echo -e "\n=== AUTOMATED TESTS COMPLETE ==="
```

---

## Manual Tests (Run in Separate Terminal)

### Test 1: TUI Installer Flow

```bash
./bin/dotfiles install
```

**Checklist:**
- [ ] Intro animation plays smoothly (no flickering)
- [ ] Logo renders correctly with colors
- [ ] Press Enter to continue works
- [ ] Theme selection screen shows all 16 themes
- [ ] Arrow keys navigate theme list
- [ ] Enter selects theme
- [ ] Navigation style selection works (emacs/vim)
- [ ] Deep Dive menu shows all tools
- [ ] v2.1 tools appear in the list: Tailscale, Sunshine, Moonlight
- [ ] Selecting Claude Code opens its MCP config screen and applies MCP servers (context7 enabled by default) on install
- [ ] Each tool config screen opens correctly
- [ ] Esc/Backspace returns to previous screen
- [ ] Summary screen shows correct selections
- [ ] Tab cycles between options

### Test 2: Management UI

```bash
./bin/dotfiles manage
```

**Checklist:**
- [ ] Tool list displays with correct icons
- [ ] v2.1 tools listed: Tailscale, Sunshine, Moonlight, Claude Code (MCP)
- [ ] Installed/Not Installed status is accurate
- [ ] Arrow keys navigate tool list
- [ ] Enter opens tool configuration
- [ ] Config fields are editable
- [ ] Tab switches between panes (if dual-pane)
- [ ] Mouse clicks work on items
- [ ] Esc returns to previous view
- [ ] Scroll works for long lists

### Test 3: Hotkey Viewer

```bash
./bin/dotfiles hotkeys
```

**Checklist:**
- [ ] All tool categories display
- [ ] Hotkeys are readable
- [ ] Navigation works (up/down/left/right)
- [ ] Category switching works
- [ ] Icons render correctly
- [ ] `f` toggles favorite on the selected hotkey
- [ ] `F` toggles favorites-only filter mode (from both panes)
- [ ] Favorites persist per-user across runs (saved via `config.SaveHotkeysConfig`)

### Test 4: Animation & Performance

```bash
./bin/dotfiles --skip-intro  # Should skip animation
./bin/dotfiles               # Should show animation
```

**Checklist:**
- [ ] `--skip-intro` actually skips animation
- [ ] Animation is smooth (60fps feel)
- [ ] No visual artifacts during transitions
- [ ] Memory doesn't spike during animation
- [ ] CPU usage is reasonable

### Test 5: Keyboard Navigation

**Test all these keys in various screens:**

| Key | Expected Behavior |
|-----|------------------|
| `↑` / `k` | Move selection up |
| `↓` / `j` | Move selection down |
| `←` / `h` | Go back / previous |
| `→` / `l` | Enter / confirm |
| `Enter` | Confirm selection |
| `Esc` | Go back / cancel |
| `Tab` | Cycle focus |
| `q` | Quit (where applicable) |
| `Ctrl+C` | Force quit |

### Test 6: Mouse Navigation

**Checklist:**
- [ ] Click on menu items selects them
- [ ] Click on buttons activates them
- [ ] Scroll wheel scrolls lists
- [ ] No ghost clicks or missed clicks
- [ ] Hover effects work (if any)

---

## Aesthetic Cohesion Review

Open each screen and verify visual consistency:

### Color Palette Check

The neon-seapunk palette should be consistent:

| Element | Expected Color |
|---------|---------------|
| Primary accent | `#00F5D4` (seafoam cyan) |
| Secondary accent | `#F15BB5` (hot pink) |
| Purple accent | `#9B5DE5` (electric purple) |
| Background | `#070B1A` (deep ocean) |
| Surface | `#0F1633` (elevated) |
| Text | `#E6F1FF` (cool white) |
| Muted text | `#97A7C7` (slate) |

**Checklist:**
- [ ] All screens use the same color palette
- [ ] No jarring color mismatches
- [ ] Selected items are clearly highlighted
- [ ] Disabled items are visually distinct
- [ ] Borders are consistent style

### Layout Check

- [ ] Text is properly aligned
- [ ] Padding is consistent
- [ ] No text overflow/truncation issues
- [ ] Headers are properly sized
- [ ] Footer/help text is readable

### Icon Check

- [ ] Nerd Font icons render correctly
- [ ] Icons are aligned with text
- [ ] No missing/placeholder icons

---

## Security Checklist

Manual review of these areas:

- [ ] No secrets in code (API keys, passwords)
- [ ] File operations validate paths (no traversal)
- [ ] User input is sanitized before shell execution
- [ ] Config files have appropriate permissions (600/700)
- [ ] No `eval` or unsafe `exec.Command` with user input
- [ ] Backup/restore doesn't overwrite system files

---

## Platform Testing (If Possible)

| Platform | Tests |
|----------|-------|
| **Arch Linux** | All manual tests above |
| **macOS** | Installer, Homebrew detection, theme |
| **Debian/Ubuntu** | APT detection, basic flow |
| **Terminal: Ghostty** | Full visual test |
| **Terminal: iTerm2** | Color rendering |
| **Terminal: Alacritty** | Basic functionality |

---

## Cross-Repository Compatibility Tests

### Automated Cross-Repo Check Script

Run this from `~/Desktop/Projects/`:

```bash
#!/bin/bash
echo "=== CROSS-REPO COMPATIBILITY CHECK ==="
cd ~/Desktop/Projects || exit 1

# 1. Check all repos exist
echo -e "\n[1/5] Checking repositories..."
for repo in dotfiles sshh homebrew-tap; do
    if [ -d "$repo" ]; then
        echo "  $repo: EXISTS"
    else
        echo "  $repo: MISSING - Clone from github.com/tekierz/$repo"
        exit 1
    fi
done

# 2. Check git status of all repos
echo -e "\n[2/5] Git status..."
for repo in dotfiles sshh homebrew-tap; do
    DIRTY=$(git -C $repo status --porcelain 2>/dev/null | wc -l)
    BRANCH=$(git -C $repo branch --show-current 2>/dev/null)
    if [ "$DIRTY" -gt 0 ]; then
        echo "  $repo ($BRANCH): $DIRTY uncommitted changes"
    else
        echo "  $repo ($BRANCH): clean"
    fi
done

# 3. Version check
echo -e "\n[3/5] Version numbers..."
SSHH_VER=$(grep -m1 'VERSION=' sshh/bin/sshh 2>/dev/null | cut -d'"' -f2 || echo "unknown")
# Primary distribution: the Go-binary 'dotfiles' formula (brew install dotfiles)
TAP_DOTFILES_VER=$(grep -m1 'version' homebrew-tap/Formula/dotfiles.rb 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
TAP_SSHH_VER=$(grep -m1 'version' homebrew-tap/Formula/sshh.rb 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
echo "  sshh script: v$SSHH_VER"
echo "  homebrew-tap dotfiles (Go binary) formula: v$TAP_DOTFILES_VER"
echo "  homebrew-tap sshh formula: v$TAP_SSHH_VER"

# 4. Check SHA256 hashes are present (can't verify without release)
echo -e "\n[4/5] Homebrew formula SHA256 hashes..."
DOTFILES_SHA=$(grep -m1 'sha256' homebrew-tap/Formula/dotfiles.rb 2>/dev/null | grep -oE '[a-f0-9]{64}' || echo "missing")
SSHH_SHA=$(grep -m1 'sha256' homebrew-tap/Formula/sshh.rb 2>/dev/null | grep -oE '[a-f0-9]{64}' || echo "missing")
echo "  dotfiles (Go binary): ${DOTFILES_SHA:0:16}..."
echo "  sshh: ${SSHH_SHA:0:16}..."

# 5. Check for breaking changes in sshh config format
echo -e "\n[5/5] sshh config format compatibility..."
if grep -q "pipe-delimited" sshh/README.md 2>/dev/null || grep -q '|' sshh/bin/sshh 2>/dev/null; then
    echo "  Config format: pipe-delimited (Name|user@host|port|key)"
fi

echo -e "\n=== CROSS-REPO CHECK COMPLETE ==="
```

### Manual Cross-Repo Compatibility Checklist

**When updating dotfiles:**

- [ ] Verify `brew install tekierz/tap/sshh` still works
- [ ] Check DeepDive utilities screen includes sshh toggle
- [ ] Verify sshh appears in `dotfiles status` output

**When updating sshh:**

- [ ] Update version number in `bin/sshh`
- [ ] Create new git tag: `git tag v1.x.x && git push --tags`
- [ ] Update homebrew-tap SHA256 hash (see below)
- [ ] Test installation: `brew reinstall sshh`

**When updating homebrew-tap:**

After publishing a tagged dotfiles or sshh release:

```bash
# 1. Get the primary Go-binary formula SHA256 for the release tag
DOTFILES_TAG=vX.Y.Z
curl -sL "https://github.com/tekierz/dotfiles/archive/refs/tags/${DOTFILES_TAG}.tar.gz" | shasum -a 256

# 2. Get new SHA256 for sshh
SSHH_TAG=vX.Y.Z
curl -sL "https://github.com/tekierz/sshh/archive/refs/tags/${SSHH_TAG}.tar.gz" | shasum -a 256

# 3. Update formulas in homebrew-tap
# Edit: ~/Desktop/Projects/homebrew-tap/Formula/dotfiles.rb       # primary Go-binary formula
# Edit: ~/Desktop/Projects/homebrew-tap/Formula/sshh.rb

# 4. Commit and push
cd ~/Desktop/Projects/homebrew-tap || exit 1
git add -A && git commit -m "Update SHA256 for [package] v[version]"
git push
```

### Key Files to Check

| Change Type | Files to Update |
|-------------|-----------------|
| sshh version bump | `sshh/bin/sshh`, `homebrew-tap/Formula/sshh.rb` |
| dotfiles version bump (Go binary) | `homebrew-tap/Formula/dotfiles.rb` (primary, `brew install dotfiles`) |
| Tool registry | `dotfiles/internal/tools/registry.go`, `dotfiles/internal/ui/deepdive.go` |

### Breaking Change Detection

Watch for these breaking changes:

| Component | Breaking Change | Impact |
|-----------|-----------------|--------|
| sshh config format | Change from pipe-delimited | Users lose saved hosts |
| sshh CLI flags | Changed/removed flags | `dotfiles` utility workflows may break |
| Homebrew formula URL | Changed repo structure | `brew install` fails |
| dotfiles utilities | Removed sshh reference | sshh not installed on macOS |

---

## Pre-Merge Checklist

Before creating PR:

- [ ] All automated tests pass
- [ ] `go mod verify` and `go mod tidy -diff` pass
- [ ] `make lint` (golangci-lint) passes
- [ ] `staticcheck ./...` passes with no allowlist
- [ ] `go test ./...` and `go test -race ./...` pass
- [ ] `govulncheck ./...` and `shellcheck scripts/install-hooks.sh` pass
- [ ] Pre-commit hooks installed (`bash scripts/install-hooks.sh`)
- [ ] CI workflow (`.github/workflows/ci.yml`) is green on the branch
- [ ] Manual TUI tests pass
- [ ] No visual regressions
- [ ] Keyboard navigation works
- [ ] Mouse navigation works
- [ ] Colors are consistent
- [ ] CLI commands work correctly
- [ ] Documentation is updated
- [ ] Commit messages are clean
- [ ] Branch is rebased on main (if needed)
- [ ] Cross-repo compatibility verified
- [ ] homebrew-tap SHA256 hashes updated (if needed)

---

## Quick Test Commands Reference

```bash
# Build
make build

# Run TUI
./bin/dotfiles
./bin/dotfiles install
./bin/dotfiles manage
./bin/dotfiles hotkeys

# CLI Tests
./bin/dotfiles status
./bin/dotfiles backups
./bin/dotfiles theme list
./bin/dotfiles --help

# Static Analysis
go mod verify
go mod tidy -diff
go vet ./...
gofmt -l .
golangci-lint run
staticcheck ./...
go test ./...
go test -race ./...
govulncheck ./...
shellcheck scripts/install-hooks.sh

# Security Grep
grep -rn "password" --include="*.go" ./internal ./cmd
grep -rn "exec.Command" --include="*.go" ./internal ./cmd

# Cross-Repo Commands (run from ~/Desktop/Projects/)
cd ~/Desktop/Projects || exit 1
git -C dotfiles status
git -C sshh status
git -C homebrew-tap status

# Check versions (the primary Go binary version is tag-derived)
grep VERSION sshh/bin/sshh
grep version homebrew-tap/Formula/*.rb

# Test sshh independently
~/Desktop/Projects/sshh/bin/sshh --help
```
