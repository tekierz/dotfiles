package tools

import (
	"os/exec"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// ClaudeCodeTool represents Claude Code CLI
type ClaudeCodeTool struct {
	BaseTool
}

// NewClaudeCodeTool creates a new Claude Code tool
func NewClaudeCodeTool() *ClaudeCodeTool {
	return &ClaudeCodeTool{
		BaseTool: BaseTool{
			id:          "claude-code",
			name:        "Claude Code",
			description: "AI-powered coding assistant",
			icon:        "󰚩",
			category:    CategoryUtility,
			packages: map[pkg.Platform][]string{
				// npm package - installed via: npm install -g @anthropic-ai/claude-code
				pkg.PlatformMacOS:  {"node"},
				pkg.PlatformArch:   {"nodejs", "npm"},
				pkg.PlatformDebian: {"nodejs", "npm"},
			},
			configPaths: []string{},
		},
	}
}

// IsInstalled checks if claude command is available (npm global install)
func (t *ClaudeCodeTool) IsInstalled() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}
