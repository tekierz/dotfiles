package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/safefile"
)

// ClaudeCodeTool represents Claude Code CLI
type ClaudeCodeTool struct {
	BaseTool
}

func (t *ClaudeCodeTool) InstallRecipe(environment InstallEnvironment) (operation.InstallRecipe, error) {
	if environment.Manager == "" {
		return operation.InstallRecipe{}, fmt.Errorf("installing Claude Code requires a package manager for Node.js")
	}
	return operation.InstallRecipe{
		SchemaVersion: operation.CurrentInstallRecipeSchemaVersion,
		ToolID:        t.ID(),
		Platform:      string(environment.Platform),
		Manager:       environment.Manager,
		Steps: []operation.InstallStep{
			{Kind: operation.InstallStepPackageManager, Provider: environment.Manager, Packages: PackagesForPlatform(t.Packages(), environment.Platform)},
			{Kind: operation.InstallStepNPMGlobal, Provider: "npm", Args: []string{"install", "-g", "@anthropic-ai/claude-code"}},
		},
		Detector:       operation.InstallDetector{Kind: operation.InstallDetectorBinary, Values: []string{"claude"}},
		Authentication: "interactive provider login",
		Risk:           "downloads and executes npm package lifecycle code",
	}, nil
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
			configPaths: []string{filepath.Join(home, ".claude.json")},
			// UI metadata
			uiGroup:        UIGroupCLITools,
			configScreen:   32, // ScreenConfigClaudeCode - has dedicated MCP config screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if claude command is available (npm global install)
func (t *ClaudeCodeTool) IsInstalled() bool {
	return directInstallationDetected(t)
}

func (t *ClaudeCodeTool) IsInstalledOutsidePackageManager(DirectInstallationObservation) bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func (t *ClaudeCodeTool) InstallationDirectAlternatives(DirectInstallationObservation) []health.DirectAlternative {
	return recipeBinaryHealth("claude")
}

// PackageMetadataIsAuthoritative tells dashboard planning that package receipt
// presence is not authoritative for this tool. Node/npm are prerequisites, not
// the installed product; the `claude` binary produced by the custom npm step is
// the source of truth. This optional policy is intentionally outside Tool so
// ordinary package-backed tools do not need boilerplate.
func (t *ClaudeCodeTool) PackageMetadataIsAuthoritative() bool {
	return false
}

// Install deliberately rejects the legacy unreviewed installation route.
// Claude Code installation requires the accepted two-phase recipe coordinator.
func (t *ClaudeCodeTool) Install(mgr pkg.PackageManager) error {
	return recipeBackedInstallError(t.ID())
}

// InstallWithContext remains as a fail-closed compatibility boundary for old
// dispatchers. It grants no package-manager or npm mutation authority.
func (t *ClaudeCodeTool) InstallWithContext(context.Context, pkg.PackageManager, func(string)) error {
	return recipeBackedInstallError(t.ID())
}

// InstallWithContextForPlatform also fails closed. The platform-aware method is
// retained so legacy type assertions cannot fall through to BaseTool and install
// Node while falsely reporting Claude Code as complete.
func (t *ClaudeCodeTool) InstallWithContextForPlatform(context.Context, pkg.PackageManager, pkg.Platform, func(string)) error {
	return recipeBackedInstallError(t.ID())
}

// ApplyConfigWithMCPs applies MCP server configuration with specific MCP selections
func (t *ClaudeCodeTool) ApplyConfigWithMCPs(enabledMCPs map[string]bool) error {
	_, err := t.ApplyConfigWithMCPsTracked(enabledMCPs)
	return err
}

func (t *ClaudeCodeTool) ApplyConfigWithMCPsTracked(enabledMCPs map[string]bool) (MutationEvidence, error) {
	return t.applyConfigWithMCPsTracked(enabledMCPs, config.LoadClaudeConfig)
}

// ApplyConfigWithMCPsAtRevisionTracked applies MCP selection to a plan-accepted source.
func (t *ClaudeCodeTool) ApplyConfigWithMCPsAtRevisionTracked(enabledMCPs map[string]bool, accepted safefile.Revision) (MutationEvidence, error) {
	if !accepted.Tracked() {
		return MutationEvidence{}, fmt.Errorf("%w: accepted Claude revision is untracked", safefile.ErrRevisionChanged)
	}
	revision, err := config.ApplyClaudeMCPSelectionAtRevisionTracked(enabledMCPs, accepted)
	if err != nil {
		return MutationEvidence{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return MutationEvidence{}, err
	}
	return MutationEvidence{Path: filepath.Join(home, ".claude.json"), Revision: revision}, nil
}

func (t *ClaudeCodeTool) ApplyConfigWithMCPsAtAuthorityTracked(enabledMCPs map[string]bool, accepted safefile.Revision, parents *safefile.ParentChain) (MutationEvidence, error) {
	return t.ApplyConfigWithMCPsAtBoundAuthorityTracked(enabledMCPs, accepted, parents, operation.DefaultLocker)
}

func (t *ClaudeCodeTool) ApplyConfigWithMCPsAtBoundAuthorityTracked(enabledMCPs map[string]bool, accepted safefile.Revision, parents *safefile.ParentChain, locker operation.Locker) (MutationEvidence, error) {
	if !accepted.Tracked() {
		return MutationEvidence{}, fmt.Errorf("%w: accepted Claude revision is untracked", safefile.ErrRevisionChanged)
	}
	if !parents.Tracked() || locker == nil {
		return MutationEvidence{}, fmt.Errorf("%w: accepted Claude authority is incomplete", safefile.ErrParentChanged)
	}
	revision, err := config.ApplyClaudeMCPSelectionAtBoundAuthorityTracked(enabledMCPs, accepted, parents, locker)
	if err != nil {
		return MutationEvidence{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return MutationEvidence{}, err
	}
	return MutationEvidence{Path: filepath.Join(home, ".claude.json"), Revision: revision, Parents: parents}, nil
}

func (t *ClaudeCodeTool) applyConfigWithMCPsTracked(enabledMCPs map[string]bool, load func() (*config.ClaudeConfig, error)) (MutationEvidence, error) {
	cfg, err := load()
	if err != nil {
		return MutationEvidence{}, fmt.Errorf("load Claude config before MCP update: %w", err)
	}

	applyMCPSelection(cfg, enabledMCPs)

	revision, err := config.SaveClaudeConfigTracked(cfg)
	if err != nil {
		return MutationEvidence{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return MutationEvidence{}, err
	}
	return MutationEvidence{Path: filepath.Join(home, ".claude.json"), Revision: revision}, nil
}

func applyMCPSelection(cfg *config.ClaudeConfig, enabledMCPs map[string]bool) {
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
}
