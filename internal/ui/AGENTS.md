# UI Package

TUI implementation using Bubble Tea (Elm architecture: Model-Update-View).

Every screen is a `ScreenHandler` (defined in `screen.go`) implemented in package
`ui` in a `screen_*.go` file. There is no longer a `ui/screens/` subpackage; the
ScreenHandler/ScreenManager migration is complete and is the live dispatch.
`App.Update` delegates to the `ScreenManager` after global tick/cache handling
and App-owned Updates, Backups and Users result reduction. Request generations
reject stale and duplicate results even after tab navigation. `App.View` delegates to
`ScreenManager.View()`. The old giant `App.Update`/`App.View` switches and the
`handleWizardKey`/`handleManagementKey`/`handleKey`/`handleMouse` dispatch are
gone, along with the files `input_deepdive.go`, `input_wizard.go`,
`input_management.go`, `screens.go`, `screens_management.go`, `screens_manage.go`,
and `hotkeys_dualpane.go`.

## Key Files

| File | Purpose | Lines |
|------|---------|-------|
| `app.go` | Main App model, `Update()`/`View()` (delegate to ScreenManager), global message handling, `NewApp`/`initScreenManager`, Screen constants, state fields | ~980 |
| `screen.go` | `ScreenHandler` interface, `ScreenContext` (holds unexported `app *App` for shared state), base implementations | ~170 |
| `screen_manager.go` | Screen lifecycle/navigation management | ~200 |
| `screen_factory.go` | App-owned `Factory` building the `ScreenFactory` map | ~110 |
| `screen_manage.go` | Manage screen handler with tool actions | ~725 |
| `screen_hotkeys.go` | Hotkey viewer handler (dual-pane layout + favorites filter) | ~1140 |
| `screen_users.go` | User profile management handler | ~845 |
| `screen_update.go` | Update-checking handler | ~550 |
| `screen_backups.go` | Backup listing/restore handler | ~455 |
| `screen_config_*.go` | Per-tool config screen handlers (ghostty, tmux, zsh, neovim, git, yazi, fzf, btop, glow, lazygit, lazydocker, claudecode, clitools, cliutilities, guiapps, macapps, apps, utilities); `screen_config_base.go` holds shared config-screen logic | ~60-290 each |
| `screen_welcome.go`, `screen_themepicker.go`, `screen_navpicker.go`, `screen_filetree.go`, `screen_deepdivemenu.go`, `screen_progress.go`, `screen_summary.go`, `screen_error.go`, `screen_animation.go`, `screen_mainmenu.go` | Wizard/management screen handlers | varies |
| `manage_dualpane.go` | Dual-pane management UI layout with mouse support | ~1195 |
| `screens_deepdive.go` | Shared deep-dive config rendering helpers | ~330 |
| `styles.go` | Lipgloss color palette and style definitions | ~810 |
| `deepdive.go` | DeepDiveConfig struct and menu items | ~370 |
| `deps.go` | Dependency injection interfaces | ~210 |
| `cache.go` | Async install-cache loading + update checking (`loadInstallCacheCmd`, `startInstallCacheLoad`, `checkUpdatesCmd`) | ~185 |
| `messages.go` | Message/command types (`uiTickMsg`, `installCacheDoneMsg`, etc.) | ~155 |
| `installation.go` | Install execution and progress handling | ~700 |
| `manage_config.go` | Inline manage-config editing helpers | ~220 |
| `animation.go` | Intro animation logic (`generateLogo`) | ~120 |
| `widget_globe.go` | ASCII globe widget | ~175 |
| `state_helpers.go` | State transition helper functions | ~120 |
| `input_mouse.go` | Mouse hit detection (manual coordinate math) | ~50 |
| `toolscreens.go` | Tool/config-screen mapping helpers | ~70 |
| `deps_test.go` | Mock implementations for testing | ~200 |

**Total: ~14,600 lines** (`.go` files excluding tests; ~17,400 including tests)

## Screen Navigation

Screens are defined as constants in `app.go`:

```go
const (
    ScreenAnimation Screen = iota
    ScreenWelcome
    ScreenThemePicker
    ScreenNavPicker
    // ... remaining screens
)
```

Each constant maps to a `ScreenHandler` via the `ScreenFactory` built in
`screen_factory.go`. Navigate by returning `NavigateTo(ScreenName)` from a
handler's `Update`; the `ScreenManager` runs the target handler's `Init()`.
Startup is separate: `NewApp` and pre-Init `SetStartScreen` prepare only;
`App.Init` initializes the final handler exactly once and retains its command.
Do not call handler Init from View or discard its asynchronous command.

## Adding a New Screen

1. Add a `Screen` constant in `app.go` (the `Screen = iota` block).
2. Create `screen_newscreen.go` implementing the `ScreenHandler` interface
   (`Init`/`Update`/`View`); reach shared App state through `ScreenContext.app`.
3. Register the handler in the `ScreenFactory` map (`screen_factory.go`).

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
- The intro animation is the `ScreenAnimation` handler (`screen_animation.go`),
  which advances `a.animFrame` on each `tickMsg` and renders the frame
- `generateLogo(progress float64)` (animation.go) - builds the animated logo
- `ASCIILogo()` (styles.go) - returns the static logo
- Frame updates via `tickMsg`; the global `uiFrame` counter (driven by the
  app-wide `uiTickMsg`) feeds spinners and manager widgets

## Mouse Support

Mouse handling uses manual coordinate math (no bubblezone/zone library):
- Mouse events arrive as `tea.MouseMsg` and are routed to the active
  `ScreenHandler`'s `Update` by the `ScreenManager` (there is no top-level
  `handleMouse` dispatch anymore)
- Events are converted with `tea.MouseEvent(msg)` to read `m.X` / `m.Y`
- Hit detection compares those coordinates against rendered positions
  (e.g. `detectTabClick(x int)` in `input_mouse.go`)
- See `input_mouse.go` and per-screen handlers (e.g. `configListNav.handleMouse`
  in `screen_config_base.go`), with `manage_dualpane.go` for layout

## Async Patterns

Long-running operations use Bubble Tea's message-based async pattern:

### Install Cache Loading

`loadInstallCacheCmd()` and `startInstallCacheLoad()` are defined in `cache.go`;
`installCacheDoneMsg` is defined in `messages.go`. The state fields
(`installCacheLoading`, `manageInstalledReady`, `manageInstalled`) live on `App`,
and `App.Update` handles `installCacheDoneMsg` globally (in `app.go`) before
delegating to the `ScreenManager`. App-owned request reducers also handle
Updates, Backups and Users results before screen dispatch.

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

- **Claude Code MCP configuration**: the `ScreenConfigClaudeCode` handler
  (`screen_config_claudecode.go`). MCP server selections persist via the
  `ClaudeCodeMCPs` config field (deepdive.go).
- **Backup management screen**: full backup listing/restore UI (the
  `ScreenBackups` handler in `screen_backups.go`).
- **Hotkey favorites**: per-user favorites handled in `screen_hotkeys.go`
  (`isHotkeyFavorite` / `toggleHotkeyFavorite`). In the hotkey viewer, `f`
  toggles favorite on the selected item and `F` toggles the favorites-only
  filter mode (`a.hotkeysFavoritesOnly`).
