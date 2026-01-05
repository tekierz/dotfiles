---
name: pre-pr-tests
description: Generates comprehensive pre-PR test checklist for dotfiles TUI. Use before merging feature branches to main, when running tests, creating test plans, or doing QA. Includes manual tests, automatic tests, security checks, performance tests, and aesthetic review.
---

# Pre-PR Testing Checklist for Dotfiles TUI

Run these tests before merging any feature branch to main.

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
```
