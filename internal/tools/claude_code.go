package tools

import (
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
