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
dotfiles hotkeys            # Launch TUI hotkey viewer
dotfiles update             # Launch TUI update screen
dotfiles status             # Print status (CLI)
dotfiles backups            # List backups (CLI)
dotfiles restore <name>     # Restore backup (CLI)
dotfiles theme              # Theme management
dotfiles theme --list       # List themes (CLI)
dotfiles --skip-intro       # Skip intro animation
dotfiles --version          # Print version
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

```go
func launchTUI(startScreen ui.Screen) {
    app := ui.NewApp()
    app.SetStartScreen(startScreen)

    p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
    if _, err := p.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

## Flags

```go
var skipIntro bool

func init() {
    rootCmd.PersistentFlags().BoolVar(&skipIntro, "skip-intro", false, "Skip intro animation")
}
```

## CLI vs TUI

- **CLI mode**: Print output and exit (status, backups, theme --list)
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
