package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveClaudeConfigPreservesUnrelatedKeys is the regression test for the
// critical data-loss bug: SaveClaudeConfig must never drop keys it does not
// own (model, permissions, hooks, statusLine, etc.) in ~/.claude.json.
func TestSaveClaudeConfigPreservesUnrelatedKeys(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir failed: %v", err)
	}
	path := filepath.Join(home, ".claude.json")

	// Seed a realistic ~/.claude.json with many unrelated top-level keys.
	original := map[string]any{
		"model":       "claude-opus-4-8",
		"permissions": map[string]any{"allow": []string{"Bash(go test:*)"}},
		"statusLine":  map[string]any{"type": "command", "command": "echo hi"},
		"hooks":       map[string]any{"PreToolUse": []any{}},
		"verbose":     true,
		"mcpServers": map[string]any{
			"preexisting": map[string]any{"type": "stdio", "command": "foo"},
		},
	}
	seed, _ := json.MarshalIndent(original, "", "  ")
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	// Load (only mcpServers should be surfaced) and replace the MCP map.
	cfg, err := LoadClaudeConfig()
	if err != nil {
		t.Fatalf("LoadClaudeConfig failed: %v", err)
	}
	if _, ok := cfg.MCPServers["preexisting"]; !ok {
		t.Errorf("expected preexisting MCP server to be loaded, got %v", cfg.MCPServers)
	}
	cfg.MCPServers = AllMCPServers()
	if err := SaveClaudeConfig(cfg); err != nil {
		t.Fatalf("SaveClaudeConfig failed: %v", err)
	}

	// Re-read the raw file and verify every unrelated key survived.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse back failed: %v", err)
	}
	for _, key := range []string{"model", "permissions", "statusLine", "hooks", "verbose"} {
		if _, ok := got[key]; !ok {
			t.Errorf("key %q was dropped on save (data loss)", key)
		}
	}

	// model value should be intact.
	var model string
	if err := json.Unmarshal(got["model"], &model); err != nil {
		t.Fatalf("model not a string: %v", err)
	}
	if model != "claude-opus-4-8" {
		t.Errorf("model = %q, want %q", model, "claude-opus-4-8")
	}

	// mcpServers should be the new map (context7), not the old preexisting one.
	var servers map[string]MCPServer
	if err := json.Unmarshal(got["mcpServers"], &servers); err != nil {
		t.Fatalf("mcpServers not parseable: %v", err)
	}
	if _, ok := servers["context7"]; !ok {
		t.Errorf("context7 not present after save: %v", servers)
	}
	if _, ok := servers["preexisting"]; ok {
		t.Errorf("preexisting server should have been replaced, got %v", servers)
	}
}

// TestSaveClaudeConfigWritesToClaudeJSON verifies MCP servers are written to
// ~/.claude.json and NOT to ~/.claude/settings.json (which Claude Code uses
// for model/permissions/hooks and must not be touched).
func TestSaveClaudeConfigWritesToClaudeJSON(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	home, _ := os.UserHomeDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	claudeJSON := filepath.Join(home, ".claude.json")

	cfg := &ClaudeConfig{MCPServers: AllMCPServers()}
	if err := SaveClaudeConfig(cfg); err != nil {
		t.Fatalf("SaveClaudeConfig failed: %v", err)
	}

	if _, err := os.Stat(claudeJSON); err != nil {
		t.Errorf("expected ~/.claude.json to exist: %v", err)
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Errorf("~/.claude/settings.json should NOT be written, stat err = %v", err)
	}
}

// TestSaveClaudeConfigBacksUpExisting verifies a backup is taken before
// overwriting an existing ~/.claude.json.
func TestSaveClaudeConfigBacksUpExisting(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".claude.json")
	bak := path + ".bak"

	seed := []byte(`{"model":"sonnet","mcpServers":{}}`)
	if err := os.WriteFile(path, seed, 0600); err != nil {
		t.Fatalf("seed write failed: %v", err)
	}

	cfg := &ClaudeConfig{MCPServers: AllMCPServers()}
	if err := SaveClaudeConfig(cfg); err != nil {
		t.Fatalf("SaveClaudeConfig failed: %v", err)
	}

	bakData, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("expected backup file: %v", err)
	}
	if string(bakData) != string(seed) {
		t.Errorf("backup content = %q, want original %q", string(bakData), string(seed))
	}
}

// TestLoadClaudeConfigMissingFile verifies a missing file yields an empty map,
// not an error.
func TestLoadClaudeConfigMissingFile(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg, err := LoadClaudeConfig()
	if err != nil {
		t.Fatalf("LoadClaudeConfig failed: %v", err)
	}
	if cfg.MCPServers == nil {
		t.Error("MCPServers should be initialized, not nil")
	}
	if len(cfg.MCPServers) != 0 {
		t.Errorf("expected empty MCPServers, got %v", cfg.MCPServers)
	}
}

// TestMCPPackageNames guards against shipping known-bad npm package names.
func TestMCPPackageNames(t *testing.T) {
	// These npm packages do not exist and must never appear in args.
	bad := []string{
		"@context7/mcp",
		"@anthropic-ai/mcp-server-convex",
		"@anthropic-ai/mcp-server-puppeteer",
	}
	want := map[string]string{
		"context7":  "@upstash/context7-mcp",
		"puppeteer": "@modelcontextprotocol/server-puppeteer",
	}

	for name, server := range AllMCPServers() {
		joined := strings.Join(server.Args, " ")
		for _, b := range bad {
			if strings.Contains(joined, b) {
				t.Errorf("MCP %q references non-existent package %q (args: %v)", name, b, server.Args)
			}
		}
		if pkg, ok := want[name]; ok {
			if !strings.Contains(joined, pkg) {
				t.Errorf("MCP %q should reference %q, got args %v", name, pkg, server.Args)
			}
		}
	}

	// context7 is the default-enabled MCP; verify it uses the correct package.
	all := AllMCPServers()
	c7, ok := all["context7"]
	if !ok {
		t.Fatal("context7 missing from AllMCPServers")
	}
	if !strings.Contains(strings.Join(c7.Args, " "), "@upstash/context7-mcp") {
		t.Errorf("default context7 args = %v, want @upstash/context7-mcp", c7.Args)
	}
}

// TestWriteFileAtomic verifies the atomic writer leaves no temp file and writes
// the expected content with the requested permissions.
func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	if err := writeFileAtomic(path, []byte("hello"), 0600); err != nil {
		t.Fatalf("writeFileAtomic failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", string(data), "hello")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("perm = %v, want 0600", info.Mode().Perm())
	}

	// No leftover temp files in the directory.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}

	// Overwrite must succeed and replace content.
	if err := writeFileAtomic(path, []byte("world"), 0600); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "world" {
		t.Errorf("content after overwrite = %q, want %q", string(data), "world")
	}
}

// TestConfigDirEmptyGuard verifies Load/Save/EnsureDirs fail loudly (rather
// than silently using relative paths) when no config dir can be determined.
func TestConfigDirEmptyGuard(t *testing.T) {
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	origHome := os.Getenv("HOME")
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
		if origHome != "" {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
	}()

	os.Unsetenv("XDG_CONFIG_HOME")
	os.Unsetenv("HOME")

	// On platforms where os.UserHomeDir still resolves (e.g. via the user
	// database) ConfigDir may be non-empty; only assert the guard when the
	// precondition (empty ConfigDir) actually holds.
	if ConfigDir() != "" {
		t.Skip("ConfigDir resolved despite unset HOME; guard not exercised on this platform")
	}

	if err := EnsureDirs(); !errors.Is(err, ErrNoConfigDir) {
		t.Errorf("EnsureDirs() err = %v, want ErrNoConfigDir", err)
	}
	if _, err := LoadGlobalConfig(); !errors.Is(err, ErrNoConfigDir) {
		t.Errorf("LoadGlobalConfig() err = %v, want ErrNoConfigDir", err)
	}
	if _, err := LoadToolConfig("x", DefaultTestToolConfig); !errors.Is(err, ErrNoConfigDir) {
		t.Errorf("LoadToolConfig() err = %v, want ErrNoConfigDir", err)
	}
}
