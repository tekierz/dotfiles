package tools

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
)

// ClaudeCodeTool represents Claude Code CLI
type ClaudeCodeTool struct {
	BaseTool
}

// NewClaudeCodeTool creates a new Claude Code tool
func NewClaudeCodeTool() *ClaudeCodeTool {
	home, _ := os.UserHomeDir()
	return &ClaudeCodeTool{
		BaseTool: BaseTool{
			id:          "claude-code",
			name:        "Claude Code",
			description: "AI-powered coding assistant",
			icon:        "󰚩",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				// npm package - installed via: npm install -g @anthropic-ai/claude-code
				pkg.PlatformMacOS:  {"node"},
				pkg.PlatformArch:   {"nodejs", "npm"},
				pkg.PlatformDebian: {"nodejs", "npm"},
			},
			configPaths: []string{
				filepath.Join(home, ".claude", "settings.json"),
			},
			// UI metadata
			uiGroup:        UIGroupCLITools,
			configScreen:   43, // ScreenConfigClaudeCode - has dedicated MCP config screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if claude command is available (npm global install)
func (t *ClaudeCodeTool) IsInstalled() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// ApplyConfig applies MCP server configuration
func (t *ClaudeCodeTool) ApplyConfig(theme string) error {
	// Use default MCPs when called without specific selections
	defaults := make(map[string]bool)
	for name := range config.DefaultMCPServers() {
		defaults[name] = true
	}
	return t.ApplyConfigWithMCPs(defaults)
}

// ApplyConfigWithMCPs applies MCP server configuration with specific MCP selections
func (t *ClaudeCodeTool) ApplyConfigWithMCPs(enabledMCPs map[string]bool) error {
	cfg, err := config.LoadClaudeConfig()
	if err != nil {
		cfg = &config.ClaudeConfig{MCPServers: make(map[string]config.MCPServer)}
	}

	// Ensure MCPServers map is initialized (may be nil if settings.json exists but lacks this field)
	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]config.MCPServer)
	}

	// Get all available MCP server configurations
	allMCPs := config.AllMCPServers()

	// Add enabled MCPs
	for name, enabled := range enabledMCPs {
		if enabled {
			if server, exists := allMCPs[name]; exists {
				cfg.MCPServers[name] = server
			}
		} else {
			// Remove disabled MCPs if they exist
			delete(cfg.MCPServers, name)
		}
	}

	return config.SaveClaudeConfig(cfg)
}
