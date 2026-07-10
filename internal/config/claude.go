package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tekierz/dotfiles/internal/safefile"
)

var claudeConfigSaveMu sync.Mutex

// claudeConfigAfterBackupHook is package-private test instrumentation for a
// non-cooperating writer that changes ~/.claude.json after the backup commits.
var claudeConfigAfterBackupHook func(path string) error

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
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		return nil, fmt.Errorf("resolve Claude config path: %w", err)
	}

	data, revision, err := safefile.ReadWithin(root, rel)
	if err != nil {
		return nil, err
	}
	if !revision.Exists() {
		return &ClaudeConfig{MCPServers: make(map[string]MCPServer)}, nil
	}

	// Decode into a generic map so we only read the mcpServers key and ignore
	// (without discarding) everything else.
	raw, err := decodeJSONObject(data)
	if err != nil {
		return nil, fmt.Errorf("parse Claude config: %w", err)
	}

	cfg := &ClaudeConfig{MCPServers: make(map[string]MCPServer)}
	if servers, ok := raw["mcpServers"]; ok {
		if bytes.Equal(bytes.TrimSpace(servers), []byte("null")) {
			return nil, errors.New("parse Claude config: mcpServers must not be null")
		}
		if err := json.Unmarshal(servers, &cfg.MCPServers); err != nil {
			return nil, fmt.Errorf("parse Claude config mcpServers: %w", err)
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
func SaveClaudeConfig(cfg *ClaudeConfig) (returnErr error) {
	if cfg == nil {
		return errors.New("claude config is nil")
	}
	claudeConfigSaveMu.Lock()
	defer claudeConfigSaveMu.Unlock()

	path, err := claudeConfigPath()
	if err != nil {
		return err
	}
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		return fmt.Errorf("resolve Claude config path: %w", err)
	}
	lockRel := ".dotfiles-claude-config.lock"
	release, err := safefile.AcquireLockWithin(root, lockRel, 0600)
	if err != nil {
		return fmt.Errorf("lock Claude config: %w", err)
	}
	anyCommit := false
	defer func() {
		if err := release(); err != nil {
			releaseErr := fmt.Errorf("release Claude config lock: %w", err)
			if anyCommit {
				returnErr = &safefile.CommittedError{Operation: "release Claude config lock", Err: errors.Join(returnErr, releaseErr)}
				return
			}
			returnErr = errors.Join(returnErr, releaseErr)
		}
	}()

	// Read existing content into a generic map so unrelated keys (model,
	// permissions, hooks, statusLine, projects, etc.) survive the round-trip.
	raw := make(map[string]json.RawMessage)
	existing, revision, err := safefile.ReadWithin(root, rel)
	if err != nil {
		return err
	}
	if revision.Exists() {
		raw, err = decodeJSONObject(existing)
		if err != nil {
			return fmt.Errorf("parse existing Claude config: %w", err)
		}
		if servers, ok := raw["mcpServers"]; ok && bytes.Equal(bytes.TrimSpace(servers), []byte("null")) {
			return errors.New("parse existing Claude config: mcpServers must not be null")
		}
		// Back up the existing file before overwriting it.
		if err := safefile.ReplaceWithin(root, rel+".bak", existing, 0600); err != nil {
			var committed interface{ Committed() bool }
			if errors.As(err, &committed) && committed.Committed() {
				anyCommit = true
			}
			return err
		}
		anyCommit = true
		if claudeConfigAfterBackupHook != nil {
			if err := claudeConfigAfterBackupHook(path); err != nil {
				return claudeConfigFailure(true, "post-backup test hook", err)
			}
		}
	}

	// Set only the key we own.
	servers := cfg.MCPServers
	if servers == nil {
		servers = make(map[string]MCPServer)
	}
	encoded, err := json.Marshal(servers)
	if err != nil {
		return claudeConfigFailure(anyCommit, "marshal Claude MCP servers after backup", err)
	}
	raw["mcpServers"] = encoded

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return claudeConfigFailure(anyCommit, "marshal Claude config after backup", err)
	}
	_, currentRevision, err := safefile.ReadWithin(root, rel)
	if err != nil {
		return claudeConfigFailure(anyCommit, "verify Claude source revision after backup", err)
	}
	if currentRevision != revision {
		return claudeConfigFailure(anyCommit, "verify Claude source revision after backup", fmt.Errorf("%w: Claude config changed since it was read", safefile.ErrRevisionChanged))
	}
	if err := safefile.ReplaceWithin(root, rel, data, 0600); err != nil {
		var committed interface{ Committed() bool }
		if errors.As(err, &committed) && committed.Committed() {
			anyCommit = true
		}
		return claudeConfigFailure(anyCommit, "replace Claude config after backup", err)
	}
	anyCommit = true
	committed, finalRevision, err := safefile.ReadWithin(root, rel)
	if err != nil {
		return &safefile.CommittedError{Operation: "read committed Claude config", Err: err}
	}
	if !finalRevision.Exists() || finalRevision.Permissions() != 0600 || !bytes.Equal(committed, data) {
		return &safefile.CommittedError{Operation: "verify committed Claude config", Err: safefile.ErrRevisionChanged}
	}
	return nil
}

func claudeConfigFailure(committed bool, operation string, err error) error {
	if !committed {
		return err
	}
	return &safefile.CommittedError{Operation: operation, Err: err}
}
