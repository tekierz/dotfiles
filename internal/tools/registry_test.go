package tools

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	// Should have tools registered
	tools := r.All()
	if len(tools) == 0 {
		t.Error("expected tools to be registered")
	}

	// Should have at least core tools
	coreTools := []string{"zsh", "ghostty", "tmux", "neovim", "yazi", "git", "fzf"}
	for _, id := range coreTools {
		if _, ok := r.Get(id); !ok {
			t.Errorf("expected tool %q to be registered", id)
		}
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	// Get existing tool
	tool, ok := r.Get("zsh")
	if !ok {
		t.Fatal("expected to find zsh tool")
	}
	if tool.ID() != "zsh" {
		t.Errorf("ID() = %q, want %q", tool.ID(), "zsh")
	}
	if tool.Name() != "Zsh" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "Zsh")
	}

	// Get non-existent tool
	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent tool")
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry()

	tools := r.All()
	if len(tools) < 10 {
		t.Errorf("expected at least 10 tools, got %d", len(tools))
	}

	// Verify sorted by name
	for i := 1; i < len(tools); i++ {
		if tools[i-1].Name() > tools[i].Name() {
			t.Errorf("tools not sorted: %s > %s", tools[i-1].Name(), tools[i].Name())
		}
	}
}

func TestRegistry_ByCategory(t *testing.T) {
	r := NewRegistry()

	// Test shell category
	shellTools := r.ByCategory(CategoryShell)
	if len(shellTools) == 0 {
		t.Error("expected shell tools")
	}
	for _, tool := range shellTools {
		if tool.Category() != CategoryShell {
			t.Errorf("expected shell category, got %v", tool.Category())
		}
	}

	// Test terminal category
	terminalTools := r.ByCategory(CategoryTerminal)
	if len(terminalTools) == 0 {
		t.Error("expected terminal tools")
	}

	// Test utility category
	utilityTools := r.ByCategory(CategoryUtility)
	if len(utilityTools) < 5 {
		t.Errorf("expected at least 5 utility tools, got %d", len(utilityTools))
	}
}

func TestRegistry_Register(t *testing.T) {
	r := &Registry{
		tools: make(map[string]Tool),
	}

	// Register a tool
	tool := &mockTool{id: "test", name: "Test Tool"}
	r.Register(tool)

	// Verify registered
	got, ok := r.Get("test")
	if !ok {
		t.Fatal("expected to find registered tool")
	}
	if got.Name() != "Test Tool" {
		t.Errorf("Name() = %q, want %q", got.Name(), "Test Tool")
	}
}

// mockTool is a simple Tool implementation for testing
type mockTool struct {
	id          string
	name        string
	description string
	icon        string
	category    Category
	packages    map[pkg.Platform][]string
	installed   bool
}

func (t *mockTool) ID() string                           { return t.id }
func (t *mockTool) Name() string                         { return t.name }
func (t *mockTool) Description() string                  { return t.description }
func (t *mockTool) Icon() string                         { return t.icon }
func (t *mockTool) Category() Category                   { return t.category }
func (t *mockTool) Packages() map[pkg.Platform][]string  { return t.packages }
func (t *mockTool) IsInstalled() bool                    { return t.installed }
func (t *mockTool) Install(mgr pkg.PackageManager) error { return nil }
func (t *mockTool) ConfigPaths() []string                { return nil }
func (t *mockTool) HasConfig() bool                      { return false }
func (t *mockTool) GenerateConfig(theme string) string   { return "" }
func (t *mockTool) ApplyConfig(theme string) error       { return nil }
func (t *mockTool) IsHeavy() bool                        { return false }

func TestCategoryConstants(t *testing.T) {
	// Verify all category constants are distinct
	categories := map[Category]bool{
		CategoryShell:     true,
		CategoryTerminal:  true,
		CategoryEditor:    true,
		CategoryFile:      true,
		CategoryGit:       true,
		CategoryContainer: true,
		CategoryUtility:   true,
		CategoryApp:       true,
	}

	if len(categories) != 8 {
		t.Errorf("expected 8 distinct categories, got %d", len(categories))
	}
}

func TestToolProperties(t *testing.T) {
	r := NewRegistry()

	// Test a few specific tools
	tests := []struct {
		id       string
		name     string
		category Category
	}{
		{"zsh", "Zsh", CategoryShell},
		{"ghostty", "Ghostty", CategoryTerminal},
		{"tmux", "Tmux", CategoryTerminal},
		{"neovim", "Neovim", CategoryEditor},
		{"yazi", "Yazi", CategoryFile},
		{"git", "Git", CategoryGit},
		{"fzf", "fzf", CategoryUtility},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			tool, ok := r.Get(tt.id)
			if !ok {
				t.Fatalf("expected to find %q", tt.id)
			}

			if tool.Name() != tt.name {
				t.Errorf("Name() = %q, want %q", tool.Name(), tt.name)
			}
			if tool.Category() != tt.category {
				t.Errorf("Category() = %v, want %v", tool.Category(), tt.category)
			}
			if tool.Description() == "" {
				t.Error("Description() should not be empty")
			}
			// Icon is optional for some tools
			_ = tool.Icon()
		})
	}
}

func TestToolPackages(t *testing.T) {
	r := NewRegistry()

	// Tools should have packages defined for at least one platform
	for _, tool := range r.All() {
		packages := tool.Packages()
		if len(packages) == 0 {
			// Some tools (like apps) may have no packages
			continue
		}

		hasPackages := false
		for _, pkgs := range packages {
			if len(pkgs) > 0 {
				hasPackages = true
				break
			}
		}

		// Skip app category - they may have no packages
		if tool.Category() == CategoryApp {
			continue
		}

		if !hasPackages {
			t.Errorf("tool %s should have packages for at least one platform", tool.ID())
		}
	}
}

func TestToolConfigPaths(t *testing.T) {
	r := NewRegistry()

	// Tools with HasConfig() should have ConfigPaths()
	for _, tool := range r.All() {
		if tool.HasConfig() {
			paths := tool.ConfigPaths()
			if len(paths) == 0 {
				t.Errorf("tool %s HasConfig() but no ConfigPaths()", tool.ID())
			}
		}
	}
}
