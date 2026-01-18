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
	cfg, err := config.LoadClaudeConfig()
	if err != nil {
		cfg = &config.ClaudeConfig{MCPServers: make(map[string]config.MCPServer)}
	}

	// Add default MCPs if not already configured
	defaults := config.DefaultMCPServers()
	for name, server := range defaults {
		if _, exists := cfg.MCPServers[name]; !exists {
			cfg.MCPServers[name] = server
		}
	}

	return config.SaveClaudeConfig(cfg)
}
