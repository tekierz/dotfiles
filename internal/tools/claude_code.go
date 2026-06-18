package tools

import (
	"fmt"
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
				// Node provides npm, which is used to install the Claude Code
				// CLI itself (see Install below). The `claude` binary is what
				// IsInstalled checks for.
				pkg.PlatformMacOS:  {"node"},
				pkg.PlatformArch:   {"nodejs", "npm"},
				pkg.PlatformDebian: {"nodejs", "npm"},
			},
			configPaths: []string{
				filepath.Join(home, ".claude", "settings.json"),
			},
			// UI metadata
			uiGroup:        UIGroupCLITools,
			configScreen:   32, // ScreenConfigClaudeCode - has dedicated MCP config screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if claude command is available (npm global install)
func (t *ClaudeCodeTool) IsInstalled() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// Install installs Node (which provides npm) via the system package manager and
// then installs the Claude Code CLI globally with npm. The base package map only
// pulls in Node; the `claude` binary that IsInstalled looks for comes from the
// npm package, so installing Node alone is not enough.
func (t *ClaudeCodeTool) Install(mgr pkg.PackageManager) error {
	// Ensure Node/npm is present first.
	if err := t.BaseTool.Install(mgr); err != nil {
		return fmt.Errorf("failed to install Node.js (required for Claude Code): %w", err)
	}

	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found after installing Node.js; cannot install Claude Code CLI: %w", err)
	}

	cmd := exec.Command(npmPath, "install", "-g", "@anthropic-ai/claude-code")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install -g @anthropic-ai/claude-code failed: %w: %s", err, out)
	}
	return nil
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
