package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClaudeConfig represents Claude Code settings
type ClaudeConfig struct {
	MCPServers map[string]MCPServer `json:"mcpServers"`
}

// MCPServer defines an MCP server configuration
type MCPServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// DefaultMCPServers returns the default MCP server configurations
func DefaultMCPServers() map[string]MCPServer {
	return map[string]MCPServer{
		"context7": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@context7/mcp"},
		},
	}
}

// AllMCPServers returns all available MCP server configurations
func AllMCPServers() map[string]MCPServer {
	return map[string]MCPServer{
		"context7": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@context7/mcp"},
		},
		"task-master": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "task-master-ai"},
		},
		"github": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-github"},
		},
		"puppeteer": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@anthropic-ai/mcp-server-puppeteer"},
		},
		"sequential-thinking": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
		},
	}
}

// LoadClaudeConfig loads Claude config from ~/.claude/settings.json
func LoadClaudeConfig() (*ClaudeConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".claude", "settings.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ClaudeConfig{MCPServers: make(map[string]MCPServer)}, nil
		}
		return nil, err
	}

	var cfg ClaudeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveClaudeConfig saves Claude config to ~/.claude/settings.json
func SaveClaudeConfig(cfg *ClaudeConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
