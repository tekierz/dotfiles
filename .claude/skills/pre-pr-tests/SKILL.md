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
| **dotfiles** | `~/projects/dotfiles` | Main project (this repo) |
| **sshh** | `~/projects/sshh` | SSH connection manager utility |
| **homebrew-tap** | `~/projects/homebrew-tap` | Homebrew formulas for distribution |

### Integration Flow
```
dotfiles-setup (bash script)
    └── installs sshh via: brew install tekierz/tap/sshh
                                    │
homebrew-tap/Formula/sshh.rb ───────┘
    └── SHA256 hash points to: github.com/tekierz/sshh/archive/refs/tags/v*.tar.gz
```

---

## Automatic Tests (Run These First)

Copy and run this test script:

```bash
#!/bin/bash
set -e
echo "=== PRE-PR AUTOMATED TESTS ==="

# 1. Build Check
echo -e "\n[1/7] Building..."
make clean && make build
echo "BUILD: PASS"

# 2. Go Vet
echo -e "\n[2/7] Running go vet..."
go vet ./...
echo "GO VET: PASS"

# 3. Format Check
echo -e "\n[3/7] Checking gofmt..."
UNFORMATTED=$(gofmt -l ./internal ./cmd 2>/dev/null)
if [ -n "$UNFORMATTED" ]; then
    echo "GOFMT: FAIL - Unformatted files:"
    echo "$UNFORMATTED"
    exit 1
fi
echo "GOFMT: PASS"

# 4. Security: Check for hardcoded secrets
echo -e "\n[4/7] Security scan..."
if grep -rn "password\s*=\s*[\"']" --include="*.go" ./internal ./cmd 2>/dev/null | grep -v "Password string"; then
    echo "SECURITY: WARNING - Potential hardcoded password found"
fi
if grep -rn "api_key\s*=\s*[\"']" --include="*.go" ./internal ./cmd 2>/dev/null; then
    echo "SECURITY: WARNING - Potential hardcoded API key found"
fi
echo "SECURITY: PASS (manual review recommended)"

# 5. CLI Commands Test
echo -e "\n[5/7] Testing CLI commands..."
./bin/dotfiles --help > /dev/null && echo "  --help: OK"
./bin/dotfiles status > /dev/null 2>&1 && echo "  status: OK"
./bin/dotfiles backups > /dev/null 2>&1 && echo "  backups: OK"
./bin/dotfiles theme --list > /dev/null 2>&1 && echo "  theme --list: OK"
echo "CLI COMMANDS: PASS"

# 6. Binary Size Check
echo -e "\n[6/7] Binary size..."
SIZE=$(ls -lh ./bin/dotfiles | awk '{print $5}')
echo "  Binary size: $SIZE"
echo "BINARY SIZE: INFO"

# 7. Startup Time
echo -e "\n[7/7] Startup time..."
START=$(date +%s%N)
timeout 2 ./bin/dotfiles --help > /dev/null 2>&1 || true
END=$(date +%s%N)
ELAPSED=$(( (END - START) / 1000000 ))
echo "  Startup: ${ELAPSED}ms"
if [ "$ELAPSED" -gt 500 ]; then
    echo "STARTUP TIME: WARNING - Slow startup (>500ms)"
else
    echo "STARTUP TIME: PASS"
fi

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
- [ ] Theme selection screen shows all 13 themes
- [ ] Arrow keys navigate theme list
- [ ] Enter selects theme
- [ ] Navigation style selection works (emacs/vim)
- [ ] Deep Dive menu shows all tools
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

Run this from `~/projects/`:

```bash
#!/bin/bash
echo "=== CROSS-REPO COMPATIBILITY CHECK ==="
cd ~/projects

# 1. Check all repos exist
echo -e "\n[1/6] Checking repositories..."
for repo in dotfiles sshh homebrew-tap; do
    if [ -d "$repo" ]; then
        echo "  $repo: EXISTS"
    else
        echo "  $repo: MISSING - Clone from github.com/tekierz/$repo"
        exit 1
    fi
done

# 2. Check git status of all repos
echo -e "\n[2/6] Git status..."
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
echo -e "\n[3/6] Version numbers..."
DOTFILES_VER=$(grep -m1 'VERSION=' dotfiles/bin/dotfiles-setup 2>/dev/null | cut -d'"' -f2 || echo "unknown")
SSHH_VER=$(grep -m1 'VERSION=' sshh/bin/sshh 2>/dev/null | cut -d'"' -f2 || echo "unknown")
TAP_DOTFILES_VER=$(grep -m1 'version' homebrew-tap/Formula/dotfiles-setup.rb 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
TAP_SSHH_VER=$(grep -m1 'version' homebrew-tap/Formula/sshh.rb 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
echo "  dotfiles script: v$DOTFILES_VER"
echo "  sshh script: v$SSHH_VER"
echo "  homebrew-tap dotfiles formula: v$TAP_DOTFILES_VER"
echo "  homebrew-tap sshh formula: v$TAP_SSHH_VER"

# 4. Check sshh installation reference in dotfiles
echo -e "\n[4/6] Integration references..."
if grep -q "tekierz/tap/sshh" dotfiles/bin/dotfiles-setup 2>/dev/null; then
    echo "  dotfiles -> sshh tap reference: FOUND"
else
    echo "  dotfiles -> sshh tap reference: MISSING"
fi

# 5. Check SHA256 hashes are present (can't verify without release)
echo -e "\n[5/6] Homebrew formula SHA256 hashes..."
DOTFILES_SHA=$(grep -m1 'sha256' homebrew-tap/Formula/dotfiles-setup.rb 2>/dev/null | grep -oE '[a-f0-9]{64}' || echo "missing")
SSHH_SHA=$(grep -m1 'sha256' homebrew-tap/Formula/sshh.rb 2>/dev/null | grep -oE '[a-f0-9]{64}' || echo "missing")
echo "  dotfiles-setup: ${DOTFILES_SHA:0:16}..."
echo "  sshh: ${SSHH_SHA:0:16}..."

# 6. Check for breaking changes in sshh config format
echo -e "\n[6/6] sshh config format compatibility..."
if grep -q "pipe-delimited" sshh/README.md 2>/dev/null || grep -q '|' sshh/bin/sshh 2>/dev/null; then
    echo "  Config format: pipe-delimited (Name|user@host|port|key)"
fi

echo -e "\n=== CROSS-REPO CHECK COMPLETE ==="
```

### Manual Cross-Repo Compatibility Checklist

**When updating dotfiles:**

- [ ] Check if sshh installation command changed
- [ ] Verify `brew install tekierz/tap/sshh` still works
- [ ] Check DeepDive utilities screen includes sshh toggle
- [ ] Verify sshh appears in `dotfiles status` output

**When updating sshh:**

- [ ] Update version number in `bin/sshh`
- [ ] Create new git tag: `git tag v1.x.x && git push --tags`
- [ ] Update homebrew-tap SHA256 hash (see below)
- [ ] Test installation: `brew reinstall sshh`

**When updating homebrew-tap:**

After pushing changes to dotfiles or sshh:

```bash
# 1. Get new SHA256 for dotfiles-setup
curl -sL https://github.com/tekierz/dotfiles/archive/refs/tags/v1.0.1.tar.gz | sha256sum

# 2. Get new SHA256 for sshh
curl -sL https://github.com/tekierz/sshh/archive/refs/tags/v1.1.0.tar.gz | sha256sum

# 3. Update formulas in homebrew-tap
# Edit: ~/projects/homebrew-tap/Formula/dotfiles-setup.rb
# Edit: ~/projects/homebrew-tap/Formula/sshh.rb

# 4. Commit and push
cd ~/projects/homebrew-tap
git add -A && git commit -m "Update SHA256 for [package] v[version]"
git push
```

### Key Files to Check

| Change Type | Files to Update |
|-------------|-----------------|
| sshh version bump | `sshh/bin/sshh`, `homebrew-tap/Formula/sshh.rb` |
| dotfiles version bump | `dotfiles/bin/dotfiles-setup`, `homebrew-tap/Formula/dotfiles-setup.rb` |
| sshh install method | `dotfiles/bin/dotfiles-setup` (grep for "sshh") |
| Tool registry | `dotfiles/internal/tools/registry.go`, `dotfiles/internal/ui/deepdive.go` |

### Breaking Change Detection

Watch for these breaking changes:

| Component | Breaking Change | Impact |
|-----------|-----------------|--------|
| sshh config format | Change from pipe-delimited | Users lose saved hosts |
| sshh CLI flags | Changed/removed flags | dotfiles install scripts break |
| Homebrew formula URL | Changed repo structure | `brew install` fails |
| dotfiles utilities | Removed sshh reference | sshh not installed on macOS |

---

## Pre-Merge Checklist

Before creating PR:

- [ ] All automated tests pass
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
./bin/dotfiles theme --list
./bin/dotfiles --help

# Static Analysis
go vet ./...
gofmt -l ./internal ./cmd

# Security Grep
grep -rn "password" --include="*.go" ./internal ./cmd
grep -rn "exec.Command" --include="*.go" ./internal ./cmd

# Cross-Repo Commands (run from ~/projects/)
cd ~/projects
git -C dotfiles status
git -C sshh status
git -C homebrew-tap status

# Check versions
grep VERSION dotfiles/bin/dotfiles-setup
grep VERSION sshh/bin/sshh
grep version homebrew-tap/Formula/*.rb

# Test sshh independently
~/projects/sshh/bin/sshh --help
```
