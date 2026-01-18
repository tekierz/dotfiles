# Configuration Package

User configuration management with JSON storage.

## Key Files

| File | Purpose |
|------|---------|
| `config.go` | GlobalConfig, tool configs, load/save functions |
| `user.go` | UserProfile management (multi-user support) |
| `user_test.go` | User profile tests |

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

## User Profiles

Multi-user support with per-user themes and navigation preferences.

Stored in `~/.config/dotfiles/users/<username>.json`:

```go
type UserProfile struct {
    Name          string `json:"name"`
    Theme         string `json:"theme"`
    NavStyle      string `json:"nav_style"`
    KeyboardStyle string `json:"keyboard_style"`
    CreatedAt     string `json:"created_at"`
    UpdatedAt     string `json:"updated_at"`
}

// CRUD operations
profile := config.DefaultUserProfile("username")
err := config.SaveUserProfile(profile)
profile, err := config.LoadUserProfile("username")
err := config.DeleteUserProfile("username")
users, err := config.ListUserProfiles()

// Active user
err := config.ApplyUserProfile(profile)  // Sets as active
user, err := config.GetActiveUser()
err := config.ClearActiveUser()
```

## Username Validation

Usernames must:
- Start with a letter
- Contain only letters, numbers, underscores, hyphens
- Be 1-32 characters long

```go
err := config.ValidateUsername("myuser")
```
