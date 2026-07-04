package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// configZshScreen is the migrated ScreenHandler for the Zsh config screen.
// Navigation + back are inherited from configFieldNav. The legacy Zsh handler
// treated left/right/h/l/space identically (a single case), which maps directly
// onto the shared adjust callback.
//
// Fields: 0-3=prompt radios, 4=history, 5=autocd, 6-7=plugins.
type configZshScreen struct {
	configFieldNav
}

// NewConfigZshScreen creates a new Zsh config screen handler.
func NewConfigZshScreen(ctx *ScreenContext) *configZshScreen {
	s := &configZshScreen{}
	s.id = ScreenConfigZsh
	s.maxField = func(*App) int { return 7 } // 4 prompts + 2 shell options + 2 plugins - 1
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
	default: // Plugin toggle
		plugins := []string{"zsh-autosuggestions", "zsh-syntax-highlighting"}
		pluginIdx := idx - 6
		if pluginIdx >= 0 && pluginIdx < len(plugins) {
			plugin := plugins[pluginIdx]
			switch plugin {
			case "zsh-autosuggestions":
				enabled := !cfg.ZshAutosuggestions
				cfg.ZshAutosuggestions = enabled
				setZshPluginSelected(&cfg.ZshPlugins, plugin, enabled)
			case "zsh-syntax-highlighting":
				enabled := !cfg.ZshSyntaxHighlight
				cfg.ZshSyntaxHighlight = enabled
				setZshPluginSelected(&cfg.ZshPlugins, plugin, enabled)
			}
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
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(55))
	fieldIdx := 0

	rec.write(sectionHeaderStyle.Render("Prompt Style"))
	rec.write("\n")
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
		rec.field(fieldIdx)
		focused := a.configFieldIndex == fieldIdx
		selected := cfg.ZshPromptStyle == p.value
		rec.write(renderRadioOption(p.label, p.desc, selected, focused))
		rec.write("\n")
		fieldIdx++
	}

	rec.write(sectionHeaderStyle.Render("Shell Options"))
	rec.write("\n")

	rec.field(fieldIdx)
	historyFocused := a.configFieldIndex == fieldIdx
	rec.write(renderFieldLabel("History Size", historyFocused))
	rec.write(renderOptionSelector(
		[]string{"1000", "5000", "10000", "50000"},
		[]string{"1K", "5K", "10K", "50K"},
		fmt.Sprintf("%d", cfg.ZshHistorySize),
		historyFocused,
	))
	rec.write("\n")
	fieldIdx++

	rec.field(fieldIdx)
	autoCDFocused := a.configFieldIndex == fieldIdx
	rec.write(renderCheckbox("Auto CD (cd into directories)", cfg.ZshAutoCD, autoCDFocused))
	rec.write("\n")
	fieldIdx++

	rec.write(sectionHeaderStyle.Render("Plugins"))
	rec.write("\n")
	plugins := []struct {
		id   string
		name string
	}{
		{"zsh-autosuggestions", "Auto-suggestions"},
		{"zsh-syntax-highlighting", "Syntax highlighting"},
	}
	for _, p := range plugins {
		rec.field(fieldIdx)
		focused := a.configFieldIndex == fieldIdx
		enabled := zshPluginEnabledInConfig(cfg, p.id)
		rec.write(renderCheckbox(p.name, enabled, focused))
		rec.write("\n")
		fieldIdx++
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return PlaceWithBackground(
		width, height,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}

func zshPluginSelected(plugins []string, plugin string) bool {
	for _, selected := range plugins {
		if selected == plugin {
			return true
		}
	}
	return false
}

func zshPluginEnabledInConfig(cfg *DeepDiveConfig, plugin string) bool {
	switch plugin {
	case "zsh-autosuggestions":
		return cfg.ZshAutosuggestions
	case "zsh-syntax-highlighting":
		return cfg.ZshSyntaxHighlight
	default:
		return zshPluginSelected(cfg.ZshPlugins, plugin)
	}
}

func setZshPluginSelected(plugins *[]string, plugin string, enabled bool) {
	next := (*plugins)[:0]
	for _, selected := range *plugins {
		if selected == plugin {
			continue
		}
		next = append(next, selected)
	}
	if enabled {
		next = append(next, plugin)
	}
	*plugins = next
}
