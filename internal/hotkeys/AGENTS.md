# Hotkeys Package

Hotkey definitions for terminal tools.

## Key Files

| File | Purpose |
|------|---------|
| `hotkeys.go` | Hotkey categories and definitions |

## Data Structures

```go
type Item struct {
    Keys        string // Key combination (e.g., "Prefix + |")
    Description string // What it does
}

type Category struct {
    ID    string // Stable identifier (e.g., "tmux")
    Name  string // Display name (e.g., "Tmux")
    Icon  string // Nerd Font icon
    Items []Item // List of hotkey/cheatsheet entries
}
```

## Getting Hotkeys

```go
categories := hotkeys.Categories(navStyle) // navStyle: "vim" or "emacs"

for _, cat := range categories {
    fmt.Printf("%s %s\n", cat.Icon, cat.Name)
    for _, it := range cat.Items {
        fmt.Printf("  %s - %s\n", it.Keys, it.Description)
    }
}
```

## Navigation Style

`Categories` takes a navigation style and returns bindings adjusted for it:

```go
type NavStyle string

const (
    NavVim   NavStyle = "vim"
    NavEmacs NavStyle = "emacs"
)
```

`Categories(navStyle)` runs the input through `normalizeNavStyle`: only "vim"
(case-insensitive) maps to `NavVim`; any other/unknown value falls back to
`NavEmacs`. A handful of bindings vary by style:

| Binding | vim | emacs (default) |
|---------|-----|-----------------|
| Tmux pane navigation | `Alt-h/j/k/l` | `Alt-Arrow` |
| Zsh category title | `Zsh (vim mode)` | `Zsh (emacs/Mac-style)` |
| Yazi navigation | `h/j/k/l` | `Arrow keys` |
| Yazi toggle hidden files | `.` | `Ctrl-h` |
| Neovim navigation | `h/j/k/l` | `Arrow keys` |

All other bindings are identical across styles.

## Adding New Hotkeys

Edit `Categories(navStyle string)` in `hotkeys.go`:

```go
{
    ID:   "newtool",
    Name: "New Tool",
    Icon: "",
    Items: []Item{
        {"Ctrl-x", "Do something"},
        {"Ctrl-y", "Do something else"},
    },
},
```

## Current Categories

22 categories, in source order:

- Tmux - Terminal multiplexer
- Zsh - Shell (title varies by nav style)
- Yazi - File manager
- fzf - Fuzzy finder
- Ghostty - Terminal emulator
- Neovim - Text editor
- LazyGit - Git TUI
- eza - Modern ls
- zoxide - Smart cd
- Dotfiles - This CLI
- Git - Version control
- Delta - Diff viewer
- LazyDocker - Docker TUI
- Glow - Markdown renderer
- bat - cat with syntax highlighting
- btop - Resource monitor
- ripgrep - Fast search
- fd - Fast find
- Claude Code - AI coding agent
- Tailscale - Mesh VPN
- Sunshine - Game streaming server
- Moonlight - Game streaming client
