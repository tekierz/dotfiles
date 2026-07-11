package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

var claudeConfigSaveMu sync.Mutex

// claudeConfigBeforeCommitHook is package-private test instrumentation for a
// non-cooperating writer that replaces ~/.claude.json after the merge read.
var claudeConfigBeforeCommitHook func(path string) error

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
// read-modify-write that preserves all other keys in the file. The replacement
// is atomic and revision-bound. The caller's reviewed backup/rollback workflow
// owns recovery; this merge never overwrites an unowned ~/.claude.json.bak.
func SaveClaudeConfig(cfg *ClaudeConfig) (returnErr error) {
	_, err := SaveClaudeConfigTracked(cfg)
	return err
}

func SaveClaudeConfigTracked(cfg *ClaudeConfig) (committedRevision safefile.Revision, returnErr error) {
	return saveClaudeConfigAtRevisionTracked(cfg, nil, nil, operation.DefaultLocker, nil)
}

// SaveClaudeConfigAtRevisionTracked applies the merge only while the source
// still matches a reviewed plan's exact accepted revision.
func SaveClaudeConfigAtRevisionTracked(cfg *ClaudeConfig, accepted safefile.Revision) (safefile.Revision, error) {
	return saveClaudeConfigAtRevisionTracked(cfg, &accepted, nil, operation.DefaultLocker, nil)
}

// ApplyClaudeMCPSelectionAtRevisionTracked performs the accepted read,
// preservation merge, MCP selection update, and CAS while one lock is held.
// Custom MCP definitions not owned by this product remain untouched.
func ApplyClaudeMCPSelectionAtRevisionTracked(enabled map[string]bool, accepted safefile.Revision) (safefile.Revision, error) {
	selection := make(map[string]bool, len(enabled))
	for name, value := range enabled {
		selection[name] = value
	}
	return saveClaudeConfigAtRevisionTracked(nil, &accepted, nil, operation.DefaultLocker, selection)
}

func ApplyClaudeMCPSelectionAtAuthorityTracked(enabled map[string]bool, accepted safefile.Revision, parents *safefile.ParentChain) (safefile.Revision, error) {
	return ApplyClaudeMCPSelectionAtBoundAuthorityTracked(enabled, accepted, parents, operation.DefaultLocker)
}

func ApplyClaudeMCPSelectionAtBoundAuthorityTracked(enabled map[string]bool, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (safefile.Revision, error) {
	if !parents.Tracked() || locker == nil {
		return safefile.Revision{}, fmt.Errorf("%w: Claude config authority is incomplete", safefile.ErrParentChanged)
	}
	selection := make(map[string]bool, len(enabled))
	for name, value := range enabled {
		selection[name] = value
	}
	return saveClaudeConfigAtRevisionTracked(nil, &accepted, parents, locker, selection)
}

func saveClaudeConfigAtRevisionTracked(cfg *ClaudeConfig, accepted *safefile.Revision, parents *safefile.ParentChain, locker operation.Locker, selection map[string]bool) (committedRevision safefile.Revision, returnErr error) {
	if cfg == nil && selection == nil {
		return safefile.Revision{}, errors.New("claude config is nil")
	}
	claudeConfigSaveMu.Lock()
	defer claudeConfigSaveMu.Unlock()

	path, err := claudeConfigPath()
	if err != nil {
		return safefile.Revision{}, err
	}
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		return safefile.Revision{}, fmt.Errorf("resolve Claude config path: %w", err)
	}
	release, err := locker("claude-config", path)
	if err != nil {
		return safefile.Revision{}, fmt.Errorf("lock Claude config: %w", err)
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
	var existing []byte
	var revision safefile.Revision
	if accepted != nil && parents != nil {
		existing, revision, err = safefile.ReadWithinAuthorized(root, rel, parents)
	} else {
		existing, revision, err = safefile.ReadWithin(root, rel)
	}
	if err != nil {
		return safefile.Revision{}, err
	}
	if accepted != nil && revision != *accepted {
		return safefile.Revision{}, fmt.Errorf("%w: Claude config changed after plan acceptance", safefile.ErrRevisionChanged)
	}
	if revision.Exists() {
		raw, err = decodeJSONObject(existing)
		if err != nil {
			return safefile.Revision{}, fmt.Errorf("parse existing Claude config: %w", err)
		}
		if servers, ok := raw["mcpServers"]; ok && bytes.Equal(bytes.TrimSpace(servers), []byte("null")) {
			return safefile.Revision{}, errors.New("parse existing Claude config: mcpServers must not be null")
		}
	}

	// Set only the key we own. Accepted selection updates derive their source
	// map from the exact bytes read above, after authority was proven under lock.
	var servers map[string]MCPServer
	if selection != nil {
		servers = make(map[string]MCPServer)
		if encodedServers, ok := raw["mcpServers"]; ok {
			if err := json.Unmarshal(encodedServers, &servers); err != nil {
				return safefile.Revision{}, fmt.Errorf("parse existing Claude config mcpServers: %w", err)
			}
		}
		for name, enabled := range selection {
			if server, owned := AllMCPServers()[name]; owned && enabled {
				servers[name] = server
			} else if owned {
				delete(servers, name)
			}
		}
	} else {
		servers = cfg.MCPServers
	}
	if servers == nil {
		servers = make(map[string]MCPServer)
	}
	encoded, err := json.Marshal(servers)
	if err != nil {
		return safefile.Revision{}, err
	}
	raw["mcpServers"] = encoded

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return safefile.Revision{}, err
	}
	if claudeConfigBeforeCommitHook != nil {
		if err := claudeConfigBeforeCommitHook(path); err != nil {
			return safefile.Revision{}, fmt.Errorf("before Claude config commit: %w", err)
		}
	}
	var finalRevision safefile.Revision
	if accepted != nil && parents != nil {
		finalRevision, err = safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(root, rel, revision, parents, data, 0600)
	} else if accepted != nil {
		finalRevision, err = safefile.ReplaceWithinRevisionNoCreateTracked(root, rel, revision, data, 0600)
	} else {
		finalRevision, err = safefile.ReplaceWithinRevisionTracked(root, rel, revision, data, 0600)
	}
	if err != nil {
		var committed interface{ Committed() bool }
		if errors.As(err, &committed) && committed.Committed() {
			anyCommit = true
		}
		return safefile.Revision{}, err
	}
	anyCommit = true
	return finalRevision, nil
}
