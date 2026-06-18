package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configZshScreen is the migrated ScreenHandler for the Zsh config screen.
// Navigation + back are inherited from configFieldNav. The legacy Zsh handler
// treated left/right/h/l/space identically (a single case), which maps directly
// onto the shared adjust callback.
//
// Fields: 0-3=prompt radios, 4=history, 5=autocd, 6=syntax, 7=autosuggestions,
// 8-12=plugins.
type configZshScreen struct {
	configFieldNav
}

// NewConfigZshScreen creates a new Zsh config screen handler.
func NewConfigZshScreen(ctx *ScreenContext) *configZshScreen {
	s := &configZshScreen{}
	s.id = ScreenConfigZsh
	s.maxField = func(*App) int { return 12 } // 4 prompts + 4 shell options + 5 plugins - 1
	s.adjust = zshAdjust
	s.SetContext(ctx)
	return s
}

func zshAdjust(a *App, _ string, fwd bool) {
	cfg := a.deepDiveConfig
	idx := a.configFieldIndex
	switch {
	case idx < 4: // Prompt style selection
		opts := []string{"p10k", "starship", "pure", "minimal"}
		cfg.ZshPromptStyle = opts[idx]
	case idx == 4: // History size
		opts := []string{"1000", "5000", "10000", "50000"}
		current := fmt.Sprintf("%d", cfg.ZshHistorySize)
		cfg.ZshHistorySize = atoi(cycleOption(opts, current, fwd), 10000)
	case idx == 5: // Auto CD
		cfg.ZshAutoCD = !cfg.ZshAutoCD
	case idx == 6: // Syntax highlighting
		cfg.ZshSyntaxHighlight = !cfg.ZshSyntaxHighlight
	case idx == 7: // Autosuggestions
		cfg.ZshAutosuggestions = !cfg.ZshAutosuggestions
	default: // Plugin toggle
		plugins := []string{"zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions", "fzf-tab", "zsh-history-substring-search"}
		pluginIdx := idx - 8
		if pluginIdx >= 0 && pluginIdx < len(plugins) {
			togglePlugin(&cfg.ZshPlugins, plugins[pluginIdx])
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configZshScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Zsh configuration screen.
func (s *configZshScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("", "Zsh", "Shell prompt and plugins")

	cfg := a.deepDiveConfig
	var content strings.Builder
	fieldIdx := 0

	content.WriteString(sectionHeaderStyle.Render("Prompt Style"))
	content.WriteString("\n")
	prompts := []struct {
		value string
		label string
		desc  string
	}{
		{"p10k", "Powerlevel10k", "Feature-rich, customizable"},
		{"starship", "Starship", "Fast, minimal, cross-shell"},
		{"pure", "Pure", "Pretty, minimal, fast"},
		{"minimal", "Minimal", "Simple $ prompt"},
	}
	for _, p := range prompts {
		focused := a.configFieldIndex == fieldIdx
		selected := cfg.ZshPromptStyle == p.value
		content.WriteString(renderRadioOption(p.label, p.desc, selected, focused))
		content.WriteString("\n")
		fieldIdx++
	}

	content.WriteString(sectionHeaderStyle.Render("Shell Options"))
	content.WriteString("\n")

	historyFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderFieldLabel("History Size", historyFocused))
	content.WriteString(renderOptionSelector(
		[]string{"1000", "5000", "10000", "50000"},
		[]string{"1K", "5K", "10K", "50K"},
		fmt.Sprintf("%d", cfg.ZshHistorySize),
		historyFocused,
	))
	content.WriteString("\n")
	fieldIdx++

	autoCDFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Auto CD (cd into directories)", cfg.ZshAutoCD, autoCDFocused))
	content.WriteString("\n")
	fieldIdx++

	syntaxFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Syntax Highlighting", cfg.ZshSyntaxHighlight, syntaxFocused))
	content.WriteString("\n")
	fieldIdx++

	suggestFocused := a.configFieldIndex == fieldIdx
	content.WriteString(renderCheckbox("Auto-suggestions", cfg.ZshAutosuggestions, suggestFocused))
	content.WriteString("\n")
	fieldIdx++

	content.WriteString(sectionHeaderStyle.Render("Plugins"))
	content.WriteString("\n")
	plugins := []struct {
		id   string
		name string
	}{
		{"zsh-autosuggestions", "Auto-suggestions"},
		{"zsh-syntax-highlighting", "Syntax highlighting"},
		{"zsh-completions", "Extra completions"},
		{"fzf-tab", "FZF tab completion"},
		{"zsh-history-substring-search", "History search"},
	}
	for _, p := range plugins {
		focused := a.configFieldIndex == fieldIdx
		enabled := false
		for _, ep := range cfg.ZshPlugins {
			if ep == p.id {
				enabled = true
				break
			}
		}
		content.WriteString(renderCheckbox(p.name, enabled, focused))
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
