# Configuration Package

User configuration management with JSON storage.

## Key Files

| File | Purpose |
|------|---------|
| `config.go` | Config structs, load/save functions |

## Config Directory

Default: `~/.config/dotfiles/`

```go
config.ConfigDir() // Returns config directory path
```

## Global Config

Stored in `~/.config/dotfiles/config.json`:

```go
type GlobalConfig struct {
    Theme    string `json:"theme"`     // Current theme name
    NavStyle string `json:"nav_style"` // "emacs" or "vim"
}

// Load
cfg, err := config.LoadGlobalConfig()

// Save
err := config.SaveGlobalConfig(cfg)
```

## Tool Configs

Per-tool JSON configs in `~/.config/dotfiles/tools/`:

```go
type GhosttyConfig struct {
    FontFamily   string `json:"font_family"`
    FontSize     int    `json:"font_size"`
    Opacity      int    `json:"opacity"`
    // ...
}

// Load all tool configs
cfgs, err := config.LoadAllToolConfigs()

// Save all tool configs
err := config.SaveAllToolConfigs(cfgs)
```

## Adding New Config Options

1. Add field to appropriate struct in `config.go`:

```go
type NewToolConfig struct {
    OptionName string `json:"option_name"`
    // ...
}
```

2. Add to `AllToolConfigs` struct:

```go
type AllToolConfigs struct {
    Ghostty  GhosttyConfig  `json:"ghostty"`
    NewTool  NewToolConfig  `json:"new_tool"`  // Add here
    // ...
}
```

3. Add default in `LoadAllToolConfigs()` if file doesn't exist

## Config Validation

Configs are validated on load. Invalid values fall back to defaults.

```go
func (c *GhosttyConfig) Validate() {
    if c.FontSize < 8 || c.FontSize > 72 {
        c.FontSize = 14 // Default
    }
}
```
