# UI Package

TUI implementation using Bubble Tea (Elm architecture: Model-Update-View).

## Key Files

| File | Purpose | Lines |
|------|---------|-------|
| `app.go` | Main App model, Update(), View(), message handlers | ~1400 |
| `screens.go` | Wizard screen rendering (intro, theme, nav, summary) | ~500 |
| `screens_deepdive.go` | Deep dive config screens for installer | ~1100 |
| `screens_management.go` | Management platform screens | ~450 |
| `manage_dualpane.go` | Dual-pane management UI with mouse support | ~1500 |
| `hotkeys_dualpane.go` | Hotkey viewer dual-pane layout | ~550 |
| `styles.go` | Lipgloss color palette and style definitions | ~430 |
| `deepdive.go` | DeepDiveConfig struct and menu items | ~290 |
| `animation.go` | ASCII art animations for intro | varies |

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
