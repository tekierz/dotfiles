package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClaudeConfig represents the subset of Claude Code configuration this tool
// owns: the user-scope MCP server map. It deliberately models ONLY mcpServers
// so the rest of the file is treated as opaque and preserved on save.
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

// AllMCPServers returns all available MCP server configurations
func AllMCPServers() map[string]MCPServer {
	return map[string]MCPServer{
		"context7": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@upstash/context7-mcp"},
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
		"supabase": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@supabase/mcp-server-supabase"},
		},
		"convex": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "convex@latest", "mcp", "start"},
		},
		"puppeteer": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-puppeteer"},
		},
		"sequential-thinking": {
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
		},
	}
}

// claudeConfigPath returns the path to Claude Code's user-scope config file.
// User-scope MCP servers live in ~/.claude.json (NOT ~/.claude/settings.json,
// which holds model/permissions/hooks/statusLine and must never be clobbered).
func claudeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

// LoadClaudeConfig loads the MCP server map from ~/.claude.json. Only the
// mcpServers key is extracted; all other keys in the file are left untouched
// and will be preserved by SaveClaudeConfig.
func LoadClaudeConfig() (*ClaudeConfig, error) {
	path, err := claudeConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ClaudeConfig{MCPServers: make(map[string]MCPServer)}, nil
		}
		return nil, err
	}

	// Decode into a generic map so we only read the mcpServers key and ignore
	// (without discarding) everything else.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	cfg := &ClaudeConfig{MCPServers: make(map[string]MCPServer)}
	if servers, ok := raw["mcpServers"]; ok {
		if err := json.Unmarshal(servers, &cfg.MCPServers); err != nil {
			return nil, err
		}
		if cfg.MCPServers == nil {
			cfg.MCPServers = make(map[string]MCPServer)
		}
	}
	return cfg, nil
}

// SaveClaudeConfig writes the MCP server map to ~/.claude.json using a
// read-modify-write that preserves all other keys in the file. The existing
// file is backed up to ~/.claude.json.bak before writing, and the write is
// atomic (temp file + rename) so an interrupted save cannot truncate the file.
func SaveClaudeConfig(cfg *ClaudeConfig) error {
	path, err := claudeConfigPath()
	if err != nil {
		return err
	}

	// Read existing content into a generic map so unrelated keys (model,
	// permissions, hooks, statusLine, projects, etc.) survive the round-trip.
	raw := make(map[string]json.RawMessage)
	existing, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if readErr == nil {
		if err := json.Unmarshal(existing, &raw); err != nil {
			return err
		}
		if raw == nil {
			raw = make(map[string]json.RawMessage)
		}
		// Back up the existing file before overwriting it.
		if err := writeFileAtomic(path+".bak", existing, 0600); err != nil {
			return err
		}
	}

	// Set only the key we own.
	servers := cfg.MCPServers
	if servers == nil {
		servers = make(map[string]MCPServer)
	}
	encoded, err := json.Marshal(servers)
	if err != nil {
		return err
	}
	raw["mcpServers"] = encoded

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0600)
}
