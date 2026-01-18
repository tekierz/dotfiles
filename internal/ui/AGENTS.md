# UI Package

TUI implementation using Bubble Tea (Elm architecture: Model-Update-View).

## Key Files

| File | Purpose | Lines |
|------|---------|-------|
| `app.go` | Main App model, Update(), View(), message handlers | ~3100 |
| `screens.go` | Wizard screen rendering (intro, theme, nav, summary) | ~800 |
| `screens_deepdive.go` | Deep dive config screens for installer | ~1550 |
| `screens_management.go` | Management platform screens | ~450 |
| `screens_manage.go` | Manage screen with tool actions | ~750 |
| `manage_dualpane.go` | Dual-pane management UI with mouse support | ~1730 |
| `hotkeys_dualpane.go` | Hotkey viewer dual-pane layout | ~600 |
| `styles.go` | Lipgloss color palette and style definitions | ~810 |
| `deepdive.go` | DeepDiveConfig struct and menu items | ~360 |
| `screen.go` | ScreenHandler interface and base implementations | ~150 |
| `screen_manager.go` | Screen lifecycle management | ~200 |
| `screen_users.go` | User profile management screens | ~670 |
| `deps.go` | Dependency injection interfaces | ~200 |
| `deps_test.go` | Mock implementations for testing | ~200 |

**Total: ~12,600 lines**

## Screen Navigation

Screens are defined as constants in `app.go`:

```go
const (
    ScreenIntro Screen = iota
    ScreenThemeSelect
    ScreenNavStyle
    // ... 46 total screens
)
```

Navigate by setting `a.screen = ScreenName` in Update().

## Adding a New Screen

1. Add Screen constant in `app.go` (line ~23-70)
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
- `renderAnimatedLogo()` - Animated ASCII art
- Animation frame controlled by `a.animFrame` counter
- Frame updates via `tickMsg` messages

## Mouse Support

Dual-pane layouts support mouse:
- `zone.Mark()` for clickable regions
- `zone.Get()` in Update() for hit detection
- See `manage_dualpane.go` for implementation

## Async Patterns

Long-running operations use Bubble Tea's message-based async pattern:

### Install Cache Loading

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
