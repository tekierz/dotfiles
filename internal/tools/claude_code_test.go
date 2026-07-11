package tools

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
)

func TestApplyConfigWithMCPsTrackedPreservesCustomServers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".claude.json")
	custom := config.MCPServer{Type: "stdio", Command: "custom-mcp", Args: []string{"serve"}}
	seed := map[string]any{
		"model": "keep-me",
		"mcpServers": map[string]config.MCPServer{
			"custom-local": custom,
			"github":       config.AllMCPServers()["github"],
		},
	}
	data, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	evidence, err := NewClaudeCodeTool().ApplyConfigWithMCPsTracked(map[string]bool{
		"context7": true,
		"github":   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Revision.Tracked() || evidence.Path != path {
		t.Fatalf("mutation evidence = %+v", evidence)
	}
	loaded, err := config.LoadClaudeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := loaded.MCPServers["custom-local"]; !ok || got.Command != custom.Command {
		t.Fatalf("custom MCP server was lost: %+v", loaded.MCPServers)
	}
	if _, ok := loaded.MCPServers["github"]; ok {
		t.Fatalf("explicitly disabled built-in MCP remained: %+v", loaded.MCPServers)
	}
	if _, ok := loaded.MCPServers["context7"]; !ok {
		t.Fatalf("enabled built-in MCP was not added: %+v", loaded.MCPServers)
	}
}

func TestApplyConfigWithMCPsTrackedReturnsTransientLoadFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wantErr := errors.New("transient descriptor read failure")
	tool := NewClaudeCodeTool()

	evidence, err := tool.applyConfigWithMCPsTracked(map[string]bool{"context7": true}, func() (*config.ClaudeConfig, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("applyConfigWithMCPsTracked error = %v, want transient load failure", err)
	}
	if evidence.Path != "" || evidence.Revision.Tracked() || evidence.Directory != nil {
		t.Fatalf("failed load returned mutation evidence: %+v", evidence)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(statErr) {
		t.Fatalf("failed load created or replaced Claude config: %v", statErr)
	}
}
