# Configuration Package

User configuration management with JSON storage.

## Key Files

| File | Purpose |
|------|---------|
| `config.go` | GlobalConfig, directory helpers (ConfigDir/ToolsDir/EnsureDirs), generic tool-config load/save (LoadToolConfig/SaveToolConfig), AllToolConfigs aggregation, theme list, and the shared `writeFileAtomic` helper (temp+rename) used by all config saves |
| `tool.go` | Tool config structs (GhosttyConfig, TmuxConfig, ZshConfig, NeovimConfig, GitConfig, YaziConfig, FzfConfig, AppsConfig, UtilitiesConfig) |
| `defaults.go` | Default config functions (Default*Config) |
| `user.go` | UserProfile management (multi-user support) |
| `claude.go` | Claude Code MCP config (ClaudeConfig/MCPServer) in `~/.claude.json` with read-merge-preserve + `.bak` backup + atomic write; MCP server defaults |
| `hotkeys.go` | Per-user hotkey favorites & aliases (HotkeysConfig/UserHotkeys) |
| `config_test.go` | Config tests |
| `user_test.go` | User profile tests |

## Config Directory

Default: `~/.config/dotfiles/`

```go
config.ConfigDir() // Returns config directory path
```

## Global Config

Stored in `~/.config/dotfiles/global.json`:

```go
type GlobalConfig struct {
    Theme             string `json:"theme"`                          // Current theme name
    NavStyle          string `json:"nav_style"`                      // "emacs" or "vim"
    ActiveUser        string `json:"active_user,omitempty"`          // Active user profile name
    DisableAnimations bool   `json:"disable_animations,omitempty"`
    AutoBackup        bool   `json:"auto_backup"`                    // Backup before changes
    BackupMaxCount    int    `json:"backup_max_count"`               // Max backups to keep (0 = unlimited)
    BackupMaxAgeDays  int    `json:"backup_max_age_days"`            // Prune older backups (0 = keep forever)
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
    FontSize    int    `json:"font_size"`
    Opacity     int    `json:"opacity"`
    TabBindings string `json:"tab_bindings"`
}

// Load all tool configs
cfgs, err := config.LoadAllToolConfigs()

// Save all tool configs
err := config.SaveAllToolConfigs(cfgs)
```

## Adding New Config Options

1. Add field to the appropriate struct in `tool.go`:

```go
type NewToolConfig struct {
    OptionName string `json:"option_name"`
    // ...
}
```

2. Add to the `AllToolConfigs` struct in `config.go` (fields are pointer types):

```go
type AllToolConfigs struct {
    Ghostty   *GhosttyConfig   `json:"ghostty"`
    Tmux      *TmuxConfig      `json:"tmux"`
    Zsh       *ZshConfig       `json:"zsh"`
    Neovim    *NeovimConfig    `json:"neovim"`
    Git       *GitConfig       `json:"git"`
    Yazi      *YaziConfig      `json:"yazi"`
    Fzf       *FzfConfig       `json:"fzf"`
    Apps      *AppsConfig      `json:"apps"`
    Utilities *UtilitiesConfig `json:"utilities"`
    NewTool   *NewToolConfig   `json:"new_tool"` // Add here
}
```

3. Add a `Default*Config()` function in `defaults.go`, then wire it into
   `LoadAllToolConfigs()`/`SaveAllToolConfigs()`. `LoadToolConfig` returns the
   default only when the file does not exist (`os.IsNotExist`).

## Config Validation

Tool/global configs are NOT validated on load and there is no `Validate()`
method. `LoadToolConfig`/`LoadGlobalConfig` only return defaults when the file
is absent (`os.IsNotExist`); on-disk values are unmarshalled as-is without
range checks or fallback.

Discrete value validation exists separately as standalone helpers (not applied
automatically during config load):

```go
config.IsValidTheme(theme)          // checks against AvailableThemes
config.IsValidNavStyle(style)       // "emacs" or "vim"
config.IsValidKeyboardStyle(style)  // "macos" or "linux"
config.ValidateUsername(name)       // format rules below
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

// Existence / validation helpers
exists := config.UserExists("username")
ok := config.IsValidNavStyle("emacs")       // ValidNavStyles = {emacs, vim}
ok := config.IsValidKeyboardStyle("linux")   // ValidKeyboardStyles = {macos, linux}
```

`DefaultUserProfile(name)` defaults: Theme `catppuccin-mocha`, NavStyle `emacs`,
KeyboardStyle `linux`.

## Username Validation

Usernames must:
- Start with a letter
- Contain only letters, numbers, underscores, hyphens
- Be 1-32 characters long

```go
err := config.ValidateUsername("myuser")
```

## Claude Code / MCP Config

Manages user-scope MCP server entries in `~/.claude.json` (NOT
`~/.claude/settings.json`, which holds model/permissions/hooks/statusLine and
must never be clobbered). `SaveClaudeConfig` does a read-merge-preserve: it reads
the existing file into a generic map, replaces only the `mcpServers` key, leaves
all other keys untouched, writes a `~/.claude.json.bak` backup of the prior
contents, then writes atomically (temp file + rename) via `writeFileAtomic`
(file 0600). `LoadClaudeConfig` extracts only `mcpServers` and returns an empty
map if the file is missing.

```go
type ClaudeConfig struct {
    MCPServers map[string]MCPServer `json:"mcpServers"`
}

type MCPServer struct {
    Type    string            `json:"type"`
    Command string            `json:"command,omitempty"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
}

cfg, err := config.LoadClaudeConfig()   // empty map if file missing
err := config.SaveClaudeConfig(cfg)
defaults := config.DefaultMCPServers()  // {context7 -> npx -y @upstash/context7-mcp}
all := config.AllMCPServers()           // context7, task-master, github,
                                        // supabase, convex, puppeteer,
                                        // sequential-thinking
```

## Hotkeys Config

Per-user hotkey favorites and aliases, stored in
`~/.config/dotfiles/hotkeys.json`. Keyed by the global config's ActiveUser.

```go
type HotkeysConfig struct {
    Users map[string]*UserHotkeys `json:"users"`
}

type UserHotkeys struct {
    Favorites map[string][]string `json:"favorites"` // category_id -> item_keys
    Aliases   map[string]string   `json:"aliases"`   // alias -> command
}

cfg, err := config.LoadHotkeysConfig()
err := config.SaveHotkeysConfig(cfg)

u := cfg.GetUserHotkeys("username")     // get-or-create, mutations persist
cfg.SetUserHotkeys("username", u)
fav := u.IsFavorite(categoryID, itemKey)
u.ToggleFavorite(categoryID, itemKey)
n := u.GetFavoriteCount()
```
