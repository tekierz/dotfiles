# Hotkeys Package

Hotkey definitions for terminal tools.

## Key Files

| File | Purpose |
|------|---------|
| `hotkeys.go` | Hotkey categories and definitions |

## Data Structures

```go
type HotkeyCategory struct {
    Name    string   // Category name (e.g., "Tmux")
    Icon    string   // Nerd Font icon
    Hotkeys []Hotkey // List of hotkeys
}

type Hotkey struct {
    Key         string // Key combination (e.g., "Ctrl+a |")
    Description string // What it does
}
```

## Getting Hotkeys

```go
categories := hotkeys.GetHotkeyCategories()

for _, cat := range categories {
    fmt.Printf("%s %s\n", cat.Icon, cat.Name)
    for _, hk := range cat.Hotkeys {
        fmt.Printf("  %s - %s\n", hk.Key, hk.Description)
    }
}
```

## Adding New Hotkeys

Edit `GetHotkeyCategories()` in `hotkeys.go`:

```go
{
    Name: "New Tool",
    Icon: "",
    Hotkeys: []Hotkey{
        {Key: "Ctrl+x", Description: "Do something"},
        {Key: "Ctrl+y", Description: "Do something else"},
    },
},
```

## Current Categories

- Tmux - Terminal multiplexer
- Neovim - Text editor
- Yazi - File manager
- Zsh - Shell
- FZF - Fuzzy finder
- LazyGit - Git TUI
- LazyDocker - Docker TUI
