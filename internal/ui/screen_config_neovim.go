package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configNeovimScreen is the migrated ScreenHandler for the Neovim config screen.
// Like Zsh, left/right/h/l/space are treated identically and map onto the shared
// adjust callback.
//
// Fields: 0-3=config presets, 4=tabwidth, 5=wrap, 6=cursorline, 7=clipboard,
// 8-13=LSPs.
type configNeovimScreen struct {
	configFieldNav
}

// NewConfigNeovimScreen creates a new Neovim config screen handler.
func NewConfigNeovimScreen(ctx *ScreenContext) *configNeovimScreen {
	s := &configNeovimScreen{}
	s.id = ScreenConfigNeovim
	s.maxField = func(*App) int { return 13 } // 4 configs + 4 editor settings + 6 LSPs - 1
	s.adjust = neovimAdjust
	s.SetContext(ctx)
	return s
}

func neovimAdjust(a *App, _ string, fwd bool) {
	cfg := a.deepDiveConfig
	idx := a.configFieldIndex
	switch {
	case idx < 4: // Config preset selection
		opts := []string{"kickstart", "lazyvim", "nvchad", "custom"}
		cfg.NeovimConfig = opts[idx]
	case idx == 4: // Tab width
		opts := []string{"2", "4", "8"}
		current := fmt.Sprintf("%d", cfg.NeovimTabWidth)
		cfg.NeovimTabWidth = atoi(cycleOption(opts, current, fwd), 4)
	case idx == 5: // Wrap
		cfg.NeovimWrap = !cfg.NeovimWrap
	case idx == 6: // Cursor line
		cfg.NeovimCursorLine = !cfg.NeovimCursorLine
	case idx == 7: // Clipboard
		opts := []string{"unnamedplus", "unnamed", "none"}
		cfg.NeovimClipboard = cycleOption(opts, cfg.NeovimClipboard, fwd)
	default: // LSP toggle
		lsps := []string{"lua_ls", "pyright", "tsserver", "gopls", "rust_analyzer", "clangd"}
		lspIdx := idx - 8
		if lspIdx >= 0 && lspIdx < len(lsps) {
			togglePlugin(&cfg.NeovimLSPs, lsps[lspIdx])
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configNeovimScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Neovim configuration screen.
func (s *configNeovimScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "Neovim", "Editor configuration and LSP")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

	content.WriteString(sectionHeaderStyle.Render("Configuration"))
	content.WriteString("\n")
	configs := []struct {
		value string
		label string
		desc  string
	}{
		{"kickstart", "Kickstart.nvim", "Minimal, well-documented"},
		{"lazyvim", "LazyVim", "Full IDE experience"},
		{"nvchad", "NvChad", "Beautiful and fast"},
		{"custom", "Keep existing", "Don't modify config"},
	}
	for _, c := range configs {
		focused := a.configFieldIndex == fieldIdx
		selected := cfg.NeovimConfig == c.value
		content.WriteString(renderRadioOption(c.label, c.desc, selected, focused))
		content.WriteString("\n")
		fieldIdx++
	}

	content.WriteString(sectionHeaderStyle.Render("Editor Settings"))
	content.WriteString("\n")

	tabFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Tab Width", tabFocused))
	content.WriteString(renderOptionSelector(
		[]string{"2", "4", "8"},
		[]string{"2", "4", "8"},
		fmt.Sprintf("%d", cfg.NeovimTabWidth),
		tabFocused,
	))
	content.WriteString("\n")
	fieldIdx++

	wrapFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Line Wrapping", cfg.NeovimWrap, wrapFocused))
	content.WriteString("\n")
	fieldIdx++

	cursorFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Highlight Cursor Line", cfg.NeovimCursorLine, cursorFocused))
	content.WriteString("\n")
	fieldIdx++

	clipboardFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("Clipboard", clipboardFocused))
	content.WriteString(renderOptionSelector(
		[]string{"unnamedplus", "unnamed", "none"},
		[]string{"System (+)", "Selection (*)", "None"},
		cfg.NeovimClipboard,
		clipboardFocused,
	))
	content.WriteString("\n")
	fieldIdx++

	content.WriteString(sectionHeaderStyle.Render("LSP Servers"))
	content.WriteString("\n")
	lsps := []struct {
		id   string
		name string
	}{
		{"lua_ls", "Lua"},
		{"pyright", "Python"},
		{"tsserver", "TypeScript/JS"},
		{"gopls", "Go"},
		{"rust_analyzer", "Rust"},
		{"clangd", "C/C++"},
	}
	for _, l := range lsps {
		focused := a.configFieldIndex == fieldIdx
		enabled := false
		for _, el := range cfg.NeovimLSPs {
			if el == l.id {
				enabled = true
				break
			}
		}
		content.WriteString(renderCheckbox(l.name, enabled, focused))
		content.WriteString("\n")
		fieldIdx++
	}

	box := configBoxStyle.Width(a.deepDiveBoxWidth(55)).Render(content.String())
	help := HelpStyle.Render("↑↓ navigate • space/enter select • esc back")

	return PlaceWithBackground(
		width, height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
