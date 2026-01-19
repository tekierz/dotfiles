package ui

import (
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/tools"
)

// TestRenderFileTree_QuickSetupExplanation verifies that Quick Setup mode
// shows explanatory text to users about what's included.
func TestRenderFileTree_QuickSetupExplanation(t *testing.T) {
	app := newTestApp()
	app.deepDive = false // Quick Setup mode
	app.width = 80
	app.height = 40

	output := app.renderFileTree()

	// Should contain Quick Setup explanation
	if !strings.Contains(output, "Quick Setup") {
		t.Error("Quick Setup mode should show 'Quick Setup' explanation text")
	}
	if !strings.Contains(output, "Deep Dive") {
		t.Error("Quick Setup mode should mention Deep Dive as alternative")
	}
}

// TestRenderFileTree_DeepDiveNoExplanation verifies that Deep Dive mode
// does NOT show the Quick Setup explanation text.
func TestRenderFileTree_DeepDiveNoExplanation(t *testing.T) {
	app := newTestApp()
	app.deepDive = true // Deep Dive mode
	app.width = 80
	app.height = 40

	output := app.renderFileTree()

	// Should NOT contain Quick Setup explanation (it's already deep dive)
	if strings.Contains(output, "Quick Setup:") {
		t.Error("Deep Dive mode should NOT show Quick Setup explanation")
	}
}

// TestRenderFileTree_ConfigurationsToApply verifies that the summary shows
// a "Configurations to Apply" section with tool names.
func TestRenderFileTree_ConfigurationsToApply(t *testing.T) {
	app := newTestApp()
	app.width = 80
	app.height = 40

	output := app.renderFileTree()

	// Should have Configurations to Apply section
	if !strings.Contains(output, "Configurations to Apply") {
		t.Error("should show 'Configurations to Apply' section")
	}

	// Core tools should be listed (these are always configured)
	coreTools := []string{"Ghostty", "Tmux", "Zsh", "Neovim", "Git", "Yazi"}
	for _, tool := range coreTools {
		if !strings.Contains(output, tool) {
			t.Errorf("Configurations to Apply should include core tool %q", tool)
		}
	}
}

// TestRenderFileTree_PackagesToInstall verifies that selected tools
// appear in the appropriate section (either "Packages to Install" or
// "Already Installed" depending on system state).
func TestRenderFileTree_PackagesToInstall(t *testing.T) {
	app := newTestApp()
	app.width = 80
	app.height = 40
	// Enable some CLI tools
	app.deepDiveConfig.CLITools["lazygit"] = true
	app.deepDiveConfig.CLIUtilities["bat"] = true

	output := app.renderFileTree()

	// Should show one of the package sections (depends on what's installed)
	hasPackageSection := strings.Contains(output, "Packages to Install") ||
		strings.Contains(output, "Already Installed")

	if !hasPackageSection {
		t.Error("should show either 'Packages to Install' or 'Already Installed' section when tools are selected")
	}
}

// TestRenderFileTree_ConditionalConfigDirs verifies that config directories
// are shown conditionally based on selected tools.
func TestRenderFileTree_ConditionalConfigDirs(t *testing.T) {
	app := newTestApp()
	app.width = 80
	app.height = 40

	// Disable all optional tools
	app.deepDiveConfig.CLIUtilities["bat"] = false
	app.deepDiveConfig.CLITools["lazygit"] = false
	app.deepDiveConfig.CLITools["lazydocker"] = false
	app.deepDiveConfig.CLIUtilities["btop"] = false

	output := app.renderFileTree()

	// These directories should NOT appear when tools are disabled
	if strings.Contains(output, "bat/") && !app.deepDiveConfig.CLIUtilities["bat"] {
		t.Error("bat/ directory should not appear when bat is disabled")
	}
	if strings.Contains(output, "lazygit/") && !app.deepDiveConfig.CLITools["lazygit"] {
		t.Error("lazygit/ directory should not appear when lazygit is disabled")
	}

	// Now enable them
	app.deepDiveConfig.CLIUtilities["bat"] = true
	app.deepDiveConfig.CLITools["lazygit"] = true

	output = app.renderFileTree()

	// Now they should appear
	if !strings.Contains(output, "bat/") {
		t.Error("bat/ directory should appear when bat is enabled")
	}
	if !strings.Contains(output, "lazygit/") {
		t.Error("lazygit/ directory should appear when lazygit is enabled")
	}
}

// TestRenderFileTree_AlwaysShowsCoreDirectories verifies that core config
// directories always appear regardless of selection.
func TestRenderFileTree_AlwaysShowsCoreDirectories(t *testing.T) {
	app := newTestApp()
	app.width = 80
	app.height = 40

	output := app.renderFileTree()

	// Core directories should always be present
	coreDirs := []string{"dotfiles/", "ghostty/", "yazi/", "nvim/"}
	for _, dir := range coreDirs {
		if !strings.Contains(output, dir) {
			t.Errorf("core directory %q should always appear", dir)
		}
	}

	// Home directory files should always be present
	homeFiles := []string{".zshrc", ".tmux.conf", ".gitconfig"}
	for _, file := range homeFiles {
		if !strings.Contains(output, file) {
			t.Errorf("home file %q should always appear", file)
		}
	}
}

// TestRenderFileTree_NeovimConfigType verifies that the neovim config type
// is shown correctly in the summary.
func TestRenderFileTree_NeovimConfigType(t *testing.T) {
	tests := []struct {
		config   string
		expected string
	}{
		{"kickstart", "Kickstart.nvim"},
		{"lazyvim", "LazyVim"},
		{"nvchad", "NvChad"},
		{"custom", "unchanged"},
	}

	for _, tt := range tests {
		t.Run(tt.config, func(t *testing.T) {
			app := newTestApp()
			app.width = 80
			app.height = 40
			app.deepDiveConfig.NeovimConfig = tt.config

			output := app.renderFileTree()

			if !strings.Contains(output, tt.expected) {
				t.Errorf("neovim config %q should show %q in output", tt.config, tt.expected)
			}
		})
	}
}

// TestRenderFileTree_SelectedToolsWithConfig verifies that selected tools
// with config files appear in the Configurations to Apply section.
func TestRenderFileTree_SelectedToolsWithConfig(t *testing.T) {
	app := newTestApp()
	app.width = 80
	app.height = 40

	// Enable tools that have config
	app.deepDiveConfig.CLIUtilities["bat"] = true
	app.deepDiveConfig.CLITools["lazygit"] = true
	app.deepDiveConfig.CLIUtilities["btop"] = true

	output := app.renderFileTree()

	// These tools have HasConfig() = true, so they should appear
	// Note: The exact names depend on tool.Name() which we verify here
	registry := tools.GetRegistry()

	if tool, exists := registry.Get("bat"); exists && tool.HasConfig() {
		if !strings.Contains(output, tool.Name()) {
			t.Errorf("selected tool with config %q should appear in output", tool.Name())
		}
	}
}

// newTestApp creates a minimal App instance for testing render functions.
func newTestApp() *App {
	app := &App{
		width:           80,
		height:          40,
		deepDiveConfig:  NewDeepDiveConfig(),
		manageInstalled: make(map[string]bool),
	}
	return app
}
