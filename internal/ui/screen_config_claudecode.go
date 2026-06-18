package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configClaudeCodeScreen is the migrated ScreenHandler for the Claude Code MCP
// configuration screen. It does not fit either shared base (configFieldNav /
// configListNav) because its focus index ranges over -1..len(mcps)-1: index -1
// is the "Install Claude Code" toggle and indices 0..6 are the MCP server
// toggles. State stays on App (a.configFieldIndex, a.deepDiveConfig). The Update
// is kept thin and mirrors the legacy handleDeepDiveKey behavior exactly.
type configClaudeCodeScreen struct {
	BaseScreen
}

// claudeCodeMCPItems is the ordered list of MCP server rows shown on this
// screen, mapped to focus indices 0..len-1.
var claudeCodeMCPItems = []struct {
	id   string
	name string
	desc string
}{
	{"context7", "Context7", "Documentation lookup for any library"},
	{"task-master", "Task Master", "AI-driven task management"},
	{"github", "GitHub", "GitHub integration and automation"},
	{"supabase", "Supabase", "Supabase database integration"},
	{"convex", "Convex", "Convex backend integration"},
	{"puppeteer", "Puppeteer", "Browser automation and testing"},
	{"sequential-thinking", "Sequential Thinking", "Enhanced reasoning chains"},
}

// NewConfigClaudeCodeScreen creates a new Claude Code config screen handler.
func NewConfigClaudeCodeScreen(ctx *ScreenContext) *configClaudeCodeScreen {
	s := &configClaudeCodeScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *configClaudeCodeScreen) ID() Screen { return ScreenConfigClaudeCode }

// Init returns any initial commands (none on entry).
func (s *configClaudeCodeScreen) Init() tea.Cmd { return nil }

// back resets the focused field and returns to the deep-dive menu.
func (s *configClaudeCodeScreen) back() tea.Cmd {
	if a := s.App(); a != nil {
		a.configFieldIndex = 0
	}
	return NavigateTo(ScreenDeepDiveMenu)
}

// toggle flips the state for the field at a.configFieldIndex: -1 toggles the
// Claude Code install flag, 0..len-1 toggle the corresponding MCP server.
func (s *configClaudeCodeScreen) toggle(a *App) {
	switch {
	case a.configFieldIndex == -1:
		a.deepDiveConfig.CLITools["claude-code"] = !a.deepDiveConfig.CLITools["claude-code"]
	case a.configFieldIndex >= 0 && a.configFieldIndex < len(claudeCodeMCPItems):
		mcp := claudeCodeMCPItems[a.configFieldIndex].id
		a.deepDiveConfig.ClaudeCodeMCPs[mcp] = !a.deepDiveConfig.ClaudeCodeMCPs[mcp]
	}
}

// Update handles keyboard and mouse input. Keyboard mirrors the legacy
// handleDeepDiveKey case for ScreenConfigClaudeCode (focus ranges -1..len-1).
func (s *configClaudeCodeScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	a := s.App()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return s, tea.Quit
		case "up", "k":
			// Allow navigating to -1 for the install toggle.
			if a.configFieldIndex > -1 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < len(claudeCodeMCPItems)-1 {
				a.configFieldIndex++
			}
		case " ":
			s.toggle(a)
		case "esc", "enter":
			return s, s.back()
		}
	case tea.MouseMsg:
		return s, s.handleMouse(msg)
	}
	return s, nil
}

// handleMouse moves the focus with the scroll wheel within the real navigable
// range (-1..len-1, where -1 is the install toggle). A left click selects the
// row under the cursor.
func (s *configClaudeCodeScreen) handleMouse(msg tea.MouseMsg) tea.Cmd {
	a := s.App()
	m := tea.MouseEvent(msg)

	if m.Button == tea.MouseButtonWheelUp {
		if a.configFieldIndex > -1 {
			a.configFieldIndex--
		}
		return nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		if a.configFieldIndex < len(claudeCodeMCPItems)-1 {
			a.configFieldIndex++
		}
		return nil
	}
	return nil
}

// View renders the Claude Code MCP configuration screen.
func (s *configClaudeCodeScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("󰚩", "Claude Code", "AI-powered coding assistant with MCP servers")

	cfg := a.deepDiveConfig
	var content strings.Builder

	// Install toggle (focus index -1).
	content.WriteString(renderFieldLabel("Install Claude Code", a.configFieldIndex == -1))
	enabled := cfg.CLITools["claude-code"]
	installed := a.manageInstalled["claude-code"]
	content.WriteString(renderCheckboxInlineWithInstallState(enabled, a.configFieldIndex == -1, installed))
	if installed {
		content.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(" (installed)"))
	}
	content.WriteString("\n\n")

	// MCP Servers header.
	mcpHeader := lipgloss.NewStyle().Foreground(ColorMagenta).Bold(true).Render("MCP Servers")
	mcpDesc := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(" (Model Context Protocol)")
	content.WriteString(mcpHeader + mcpDesc + "\n\n")

	for i, mcp := range claudeCodeMCPItems {
		focused := a.configFieldIndex == i
		mcpEnabled := cfg.ClaudeCodeMCPs[mcp.id]

		cursor := "  "
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Render("▸ ")
		}

		checkbox := "[ ]"
		if mcpEnabled {
			checkbox = lipgloss.NewStyle().Foreground(ColorGreen).Render("[✓]")
		}

		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if focused {
			nameStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
			descStyle = lipgloss.NewStyle().Foreground(ColorText)
		}

		// Show (recommended) indicator for context7.
		suffix := ""
		if mcp.id == "context7" {
			suffix = lipgloss.NewStyle().Foreground(ColorYellow).Render(" (recommended)")
		}

		content.WriteString(fmt.Sprintf("%s%s %s%s %s\n",
			cursor,
			checkbox,
			nameStyle.Render(fmt.Sprintf("%-20s", mcp.name)),
			suffix,
			descStyle.Render(mcp.desc),
		))
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(65)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space toggle • esc back")

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
