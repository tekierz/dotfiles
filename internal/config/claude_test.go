package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/safefile"
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

func TestClaudeConfigRejectsAmbiguousOrNonObjectInputWithoutOverwrite(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude.json")
	tests := map[string]string{
		"null object":       `null`,
		"duplicate key":     `{"model":"one","model":"two","mcpServers":{}}`,
		"null MCP servers":  `{"model":"keep","mcpServers":null}`,
		"top-level array":   `[{"mcpServers":{}}]`,
		"trailing document": `{"mcpServers":{}} {}`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadClaudeConfig(); err == nil {
				t.Fatalf("LoadClaudeConfig accepted %s", content)
			}
			if err := SaveClaudeConfig(&ClaudeConfig{}); err == nil {
				t.Fatalf("SaveClaudeConfig accepted %s", content)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Fatalf("ambiguous Claude config changed to %s, want %s", got, content)
			}
			if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
				t.Fatalf("invalid input created backup before validation: %v", err)
			}
		})
	}
}

func TestSaveClaudeConfigDetectsSourceReplacementAfterBackup(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude.json")
	original := []byte(`{"model":"keep","mcpServers":{}}`)
	replacement := []byte(`{"model":"newer","futureOnly":true}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	claudeConfigAfterBackupHook = func(path string) error {
		temporary := path + ".noncooperating"
		if err := os.WriteFile(temporary, replacement, 0o600); err != nil {
			return err
		}
		return os.Rename(temporary, path)
	}
	defer func() { claudeConfigAfterBackupHook = nil }()

	err = SaveClaudeConfig(&ClaudeConfig{MCPServers: AllMCPServers()})
	if !errors.Is(err, safefile.ErrRevisionChanged) {
		t.Fatalf("SaveClaudeConfig error = %v, want ErrRevisionChanged", err)
	}
	var committed *safefile.CommittedError
	if !errors.As(err, &committed) {
		t.Fatalf("SaveClaudeConfig error = %T %v, want partial committed error", err, err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(replacement) {
		t.Fatalf("noncooperating Claude replacement overwritten: got %s, want %s", got, replacement)
	}
	backup, readErr := os.ReadFile(path + ".bak")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(backup) != string(original) {
		t.Fatalf("Claude backup = %s, want %s", backup, original)
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
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", "")
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

func TestWriteFileAtomicRejectsIntermediateSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Fatal(err)
	}
	// HOME must win even when XDG names the symlink itself; otherwise the
	// untrusted .config descendant would be promoted into a trusted root.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	target := filepath.Join(home, ".config", "dotfiles", "tools", "test.json")

	err := writeFileAtomic(target, []byte("do not redirect"), 0o600)
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("writeFileAtomic error = %v, want safefile.ErrSymlink", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "dotfiles", "tools", "test.json")); !os.IsNotExist(statErr) {
		t.Fatalf("symlink redirected write outside trusted root, stat error = %v", statErr)
	}
}

func TestWriteFileAtomicResolvedHomeWinsOverXDGAlias(t *testing.T) {
	workspace := t.TempDir()
	realHome := filepath.Join(workspace, "real-home")
	homeLink := filepath.Join(workspace, "home")
	outside := filepath.Join(workspace, "outside")
	for _, dir := range []string{realHome, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(realHome, homeLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	xdg := filepath.Join(realHome, ".config")
	if err := os.Symlink(outside, xdg); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", homeLink)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	err := writeFileAtomic(filepath.Join(xdg, "dotfiles", "tools", "test.json"), []byte("unsafe"), 0o600)
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("writeFileAtomic error = %v, want safefile.ErrSymlink", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "dotfiles", "tools", "test.json")); !os.IsNotExist(statErr) {
		t.Fatalf("resolved-HOME XDG alias redirected write, stat error = %v", statErr)
	}
}

func TestSaveClaudeConfigRejectsSymlinkTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	victim := filepath.Join(t.TempDir(), "victim.json")
	original := []byte(`{"model":"keep"}`)
	if err := os.WriteFile(victim, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(home, ".claude.json")); err != nil {
		t.Fatal(err)
	}

	err := SaveClaudeConfig(&ClaudeConfig{MCPServers: map[string]MCPServer{}})
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("SaveClaudeConfig error = %v, want safefile.ErrSymlink", err)
	}
	got, readErr := os.ReadFile(victim)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("Claude symlink victim changed: got %s, want %s", got, original)
	}
}

// TestConfigDirEmptyGuard verifies Load/Save fail loudly (rather
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

	if _, err := LoadGlobalConfig(); err != ErrNoConfigDir {
		t.Errorf("LoadGlobalConfig() err = %v, want ErrNoConfigDir", err)
	}
	if _, err := LoadToolConfig("x", DefaultTestToolConfig); err != ErrNoConfigDir {
		t.Errorf("LoadToolConfig() err = %v, want ErrNoConfigDir", err)
	}
}
