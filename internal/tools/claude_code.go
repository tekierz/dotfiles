package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/safefile"
)

// ClaudeCodeTool represents Claude Code CLI
type ClaudeCodeTool struct {
	BaseTool
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
			configPaths: []string{
				filepath.Join(home, ".claude", "settings.json"),
			},
			// UI metadata
			uiGroup:        UIGroupCLITools,
			configScreen:   32, // ScreenConfigClaudeCode - has dedicated MCP config screen
			defaultEnabled: false,
		},
	}
}

// IsInstalled checks if claude command is available (npm global install)
func (t *ClaudeCodeTool) IsInstalled() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// PackageMetadataIsAuthoritative tells dashboard planning that package receipt
// presence is not authoritative for this tool. Node/npm are prerequisites, not
// the installed product; the `claude` binary produced by the custom npm step is
// the source of truth. This optional policy is intentionally outside Tool so
// ordinary package-backed tools do not need boilerplate.
func (t *ClaudeCodeTool) PackageMetadataIsAuthoritative() bool {
	return false
}

// Install installs Node (which provides npm) via the system package manager and
// then installs the Claude Code CLI globally with npm. The base package map only
// pulls in Node; the `claude` binary that IsInstalled looks for comes from the
// npm package, so installing Node alone is not enough.
func (t *ClaudeCodeTool) Install(mgr pkg.PackageManager) error {
	return t.InstallWithContextForPlatform(context.Background(), mgr, pkg.DetectPlatform(), nil)
}

// InstallWithContext performs the custom npm phase with the caller's context
// and streams its output. This prevents Ctrl+C from leaving an orphaned npm
// install and avoids buffering unbounded CombinedOutput in memory.
func (t *ClaudeCodeTool) InstallWithContext(ctx context.Context, mgr pkg.PackageManager, emitLine func(string)) error {
	return t.InstallWithContextForPlatform(ctx, mgr, pkg.DetectPlatform(), emitLine)
}

// InstallWithContextForPlatform installs both the package-manager prerequisite
// and the custom npm product against the caller's platform snapshot. The TUI
// uses this form so a Pi plan cannot drift to the host platform while executing
// Node/npm, while the legacy Install methods above retain their existing API.
func (t *ClaudeCodeTool) InstallWithContextForPlatform(ctx context.Context, mgr pkg.PackageManager, platform pkg.Platform, emitLine func(string)) error {
	// Ensure Node/npm is present first.
	if err := t.InstallForPlatform(mgr, platform); err != nil {
		return fmt.Errorf("failed to install Node.js (required for Claude Code): %w", err)
	}

	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found after installing Node.js; cannot install Claude Code CLI: %w", err)
	}

	cmd, err := runner.RunStreaming(ctx, npmPath, "install", "-g", "@anthropic-ai/claude-code")
	if err != nil {
		return fmt.Errorf("failed to start npm install -g @anthropic-ai/claude-code: %w", err)
	}
	for line := range cmd.Output {
		if emitLine != nil {
			emitLine(line)
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("npm install -g @anthropic-ai/claude-code failed: %w", err)
	}
	return nil
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
