# CLI Entry Point

Cobra-based CLI with TUI integration.

## Key Files

| File | Purpose |
|------|---------|
| `main.go` | CLI commands and TUI launcher |

## Command Structure

```
dotfiles                    # Launch TUI main menu
dotfiles install            # Launch TUI installer
dotfiles manage             # Launch TUI management
dotfiles hotkeys            # Launch TUI hotkey viewer (alias: hk)
dotfiles update             # Launch interactive TUI update screen
dotfiles update check       # Print outdated packages (CLI)
dotfiles status             # Print status (CLI)
dotfiles plan --json --tool <id> # Print deterministic explicit install plan JSON
dotfiles doctor [--json]    # Diagnose executable provenance and PATH collisions
dotfiles doctor repair      # Preview/confirm ownership-proven stale binary quarantine
dotfiles backups            # List backups (CLI)
dotfiles restore            # Launch TUI backup selector (ScreenBackups)
dotfiles restore <name>     # Restore a specific backup (CLI)
dotfiles theme              # Launch TUI theme picker
dotfiles theme list         # List themes (CLI)
dotfiles theme set <name>   # Set theme directly (CLI)
dotfiles config <tool>      # Configure a tool (ghostty, tmux, zsh, neovim, git, yazi, fzf, apps, utilities)
dotfiles version            # Print version information
dotfiles uninstall          # Restore backups and show safe manual cleanup guidance
dotfiles user               # Show current active user
dotfiles user [name]        # Switch to a user profile (prompts to create if new)
dotfiles user add <name>    # Create a new user profile
dotfiles user delete <name> # Delete a user profile (alias: rm, remove)
dotfiles users              # List all user profiles
dotfiles --<Username>       # Quick switch to an existing user profile (e.g. dotfiles --Pratik)
dotfiles --skip-intro       # Skip intro animation (persistent flag)
```

## Adding a New Command

1. Create command variable:

```go
var newCmd = &cobra.Command{
    Use:   "newcmd",
    Short: "Short description",
    Long:  `Longer description with examples.`,
    Run: func(cmd *cobra.Command, args []string) {
        // Implementation
    },
}
```

2. Register in `init()`:

```go
func init() {
    rootCmd.AddCommand(newCmd)
}
```

## Launching TUI

`NewApp` requires a `skipIntro bool`. The ScreenManager is always wired
internally by `NewApp` (via `initScreenManager`), which builds an App-owned
`ui.Factory` mapping each `Screen` to its `ScreenHandler` — there is no opt-in
`WithScreenFactory` flag anymore.

```go
func launchTUI(screen ui.Screen) {
    app := ui.NewApp(skipIntro)
    app.SetStartScreen(screen)

    p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
    if _, err := p.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
        os.Exit(1)
    }
}
```

`NewApp` is defined as `func NewApp(skipIntro bool, opts ...AppOption) *App`; the
variadic `opts` is currently unused (no `With*` options exist), so callers pass
only `skipIntro`. `launchToolConfig` and `launchHotkeysFiltered` follow the same
pattern, passing `true` for `skipIntro`.

## Flags

```go
var skipIntro bool

func init() {
    // Global persistent flag
    rootCmd.PersistentFlags().BoolVar(&skipIntro, "skip-intro", false, "Skip intro animation")

    // Per-command flags
    hotkeysCmd.Flags().String("tool", "", "Filter hotkeys by tool (tmux, zsh, neovim, etc.)")

    uninstallCmd.Flags().Bool("keep-config", false, "Keep ~/.config/dotfiles directory")
    uninstallCmd.Flags().Bool("keep-binaries", false, "Keep installed binaries")
    uninstallCmd.Flags().Bool("no-restore", false, "Skip restoring backups")
    uninstallCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

    userAddCmd.Flags().String("theme", "", "Theme name (e.g., catppuccin-mocha)")
    userAddCmd.Flags().String("nav", "", "Navigation style: emacs or vim")
    userAddCmd.Flags().String("keyboard", "", "Keyboard style: macos or linux")
    userDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")
}
```

`--skip-intro` is the only persistent (global) flag; everything else is scoped to
its specific subcommand.

## CLI vs TUI

- **CLI mode**: Print output and exit (status, backups, theme list, update check)
- **TUI mode**: Launch interactive Bubble Tea program

Pattern for hybrid commands:

```go
Run: func(cmd *cobra.Command, args []string) {
    if listFlag {
        printList()  // CLI mode
        return
    }
    launchTUI(ui.ScreenSomething)  // TUI mode
}
```
