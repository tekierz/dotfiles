# UI Package

TUI implementation using Bubble Tea (Elm architecture: Model-Update-View).

## Key Files

| File | Purpose | Lines |
|------|---------|-------|
| `app.go` | Main App model, Update(), View(), message handlers, Screen constants, state fields | ~1460 |
| `screens.go` | Wizard screen rendering (intro, theme, nav, summary) | ~800 |
| `screens_deepdive.go` | Deep dive config screens for installer | ~1550 |
| `screens_management.go` | Management platform screens | ~710 |
| `screens_manage.go` | Manage screen with tool actions | ~750 |
| `manage_dualpane.go` | Dual-pane management UI with mouse support | ~1730 |
| `hotkeys_dualpane.go` | Hotkey viewer dual-pane layout + favorites filter | ~1010 |
| `styles.go` | Lipgloss color palette and style definitions | ~810 |
| `deepdive.go` | DeepDiveConfig struct and menu items | ~360 |
| `screen.go` | ScreenHandler interface and base implementations | ~150 |
| `screen_manager.go` | Screen lifecycle management | ~200 |
| `screen_users.go` | User profile management screens | ~670 |
| `deps.go` | Dependency injection interfaces | ~200 |
| `cache.go` | Async install-cache loading (`loadInstallCacheCmd`, `startInstallCacheLoad`) | ~190 |
| `messages.go` | Message/command types (`tickMsg`, `installCacheDoneMsg`, etc.) | ~160 |
| `installation.go` | Install execution and progress handling | ~630 |
| `animation.go` | Intro animation logic (`generateLogo`) | ~280 |
| `widget_globe.go` | ASCII globe widget | ~180 |
| `components.go` | Shared render components | ~130 |
| `state_helpers.go` | State transition helper functions | ~180 |
| `input_wizard.go` | Key handling for wizard screens (split out of Update) | ~120 |
| `input_deepdive.go` | Key handling for deep-dive config screens | ~680 |
| `input_management.go` | Key handling for management screens | ~280 |
| `input_mouse.go` | Mouse hit detection (manual coordinate math) | ~280 |
| `deps_test.go` | Mock implementations for testing | ~200 |

**Total: ~14,400 lines** (all `.go` files; ~13,700 excluding test files)

## Screen Navigation

Screens are defined as constants in `app.go`:

```go
const (
    ScreenAnimation Screen = iota
    ScreenWelcome
    ScreenThemePicker
    ScreenNavPicker
    // ... 45 total screens
)
```

Navigate by setting `a.screen = ScreenName` in Update().

## Adding a New Screen

1. Add Screen constant in `app.go` (line ~29-80)
2. Add case in `View()` method to return render function
3. Add case in `Update()` for key handling
4. Create render function: `func (a *App) renderNewScreen() string`

## Color Palette (Neon Seapunk)

```go
ColorCyan       = "#00F5D4"  // Seafoam neon (primary accent)
ColorMagenta    = "#F15BB5"  // Hot pink (secondary accent)
ColorNeonPurple = "#9B5DE5"  // Electric purple
ColorBg         = "#070B1A"  // Deep ocean background
ColorSurface    = "#0F1633"  // Elevated surfaces
```

## Styling Patterns

```go
// Container with border
ContainerStyle.Render(content)

// Selected item
lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)

// Muted text
lipgloss.NewStyle().Foreground(ColorTextMuted)
```

## Animation System

Deterministic animations using hash functions for consistent patterns:
- `renderAnimation()` (screens.go) - renders the intro animation screen
- `generateLogo(progress float64)` (animation.go) - builds the animated logo
- `ASCIILogo()` (styles.go) - returns the static logo
- Animation frame controlled by `a.animFrame` counter
- Frame updates via `tickMsg` messages (defined in messages.go)

## Mouse Support

Mouse handling uses manual coordinate math (no bubblezone/zone library):
- Mouse events arrive as `tea.MouseMsg`, dispatched by `handleMouse()` in `app.go`
- Events are converted with `tea.MouseEvent(msg)` to read `m.X` / `m.Y`
- Hit detection compares those coordinates against rendered positions
  (e.g. `detectTabClick(x int)` in `input_mouse.go`)
- See `input_mouse.go` for per-screen mouse handlers and `manage_dualpane.go` for layout

## Async Patterns

Long-running operations use Bubble Tea's message-based async pattern:

### Install Cache Loading

`loadInstallCacheCmd()` and `startInstallCacheLoad()` are defined in `cache.go`;
`installCacheDoneMsg` is defined in `messages.go`. Only the state fields
(`installCacheLoading`, `manageInstalledReady`, `manageInstalled`) and the
`case installCacheDoneMsg` handler live in `app.go`.

```go
// State fields in App
installCacheLoading  bool           // Currently loading
manageInstalledReady bool           // Cache ready
manageInstalled      map[string]bool // Cached results

// Command
func loadInstallCacheCmd() tea.Cmd  // Async loader with batch queries

// Message
type installCacheDoneMsg struct {
    installed map[string]bool
}

// Trigger
if cmd := a.startInstallCacheLoad(); cmd != nil {
    return a, cmd
}
```

### Loading State in Render

```go
if a.installCacheLoading {
    spinner := AnimatedSpinnerDots(a.uiFrame)
    loadingText := fmt.Sprintf("%s Loading installation status...", spinner)
    return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, loadingText)
}
```

### Performance

- Uses batch package manager queries (`brew list --versions` instead of per-package)
- Reduces ~27 subprocess calls to 1
- Shows loading spinner during cache population

## v2.1 Features

- **Claude Code MCP configuration**: `ScreenConfigClaudeCode` (installer) and
  `ScreenManageClaudeCode` (management), rendered by `renderConfigClaudeCode()`
  (screens_deepdive.go). MCP server selections persist via the `ClaudeCodeMCPs`
  config field (deepdive.go).
- **Backup management screen**: full backup listing/restore UI (`ScreenBackups`).
- **Hotkey favorites**: per-user favorites persisted via `hotkeysFavorites`
  (`isHotkeyFavorite` / `toggleHotkeyFavorite` in hotkeys_dualpane.go). In the
  hotkey viewer, `f` toggles favorite on the selected item and `F` toggles the
  favorites-only filter mode (`a.hotkeysFavoritesOnly`).
