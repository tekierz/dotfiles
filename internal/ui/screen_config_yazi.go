package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/tools"

	tea "github.com/charmbracelet/bubbletea"
)

// configYaziScreen is the migrated ScreenHandler for the Yazi config screen.
// Navigation + back are inherited from configFieldNav.
//
// Fields: 0=keymap, 1=show hidden, 2=preview mode, 3=sort order,
// 4=reverse sort, 5=line metadata, 6=scroll offset.
type configYaziScreen struct {
	configFieldNav
}

// NewConfigYaziScreen creates a new Yazi config screen handler.
func NewConfigYaziScreen(ctx *ScreenContext) *configYaziScreen {
	s := &configYaziScreen{}
	s.id = ScreenConfigYazi
	s.maxField = func(*App) int { return 6 }
	s.adjust = yaziAdjust
	s.SetContext(ctx)
	return s
}

func yaziUIFieldKey(index int) string {
	switch index {
	case 0:
		return "keymap"
	case 1:
		return "hidden"
	case 2:
		return "preview_mode"
	case 3:
		return "sort_by"
	case 4:
		return "sort_rev"
	case 5:
		return "linemode"
	case 6:
		return "scrolloff"
	default:
		return ""
	}
}

func yaziUIFieldID(index int) string {
	switch index {
	case 0:
		return tools.YaziFieldKeymap
	case 1:
		return tools.YaziFieldShowHidden
	case 2:
		return tools.YaziFieldPreviewMode
	case 3:
		return tools.YaziFieldSortBy
	case 4:
		return tools.YaziFieldSortReverse
	case 5:
		return tools.YaziFieldLineMode
	case 6:
		return tools.YaziFieldScrollOff
	default:
		return ""
	}
}

func yaziUIOwningObservation(a *App, index int) tools.YaziFileObservation {
	if a == nil {
		return tools.YaziFileObservation{}
	}
	if index == 0 {
		return a.nativeConfigState.Yazi.Keymap
	}
	if index >= 1 && index <= 6 {
		return a.nativeConfigState.Yazi.Main
	}
	return tools.YaziFileObservation{}
}

func compactYaziDisplayPath(path string) string {
	if path == "" {
		return ""
	}
	cleanPath := filepath.Clean(path)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cleanPath
	}
	home = filepath.Clean(home)
	if cleanPath == home {
		return "~"
	}
	if strings.HasPrefix(cleanPath, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(cleanPath, home)
	}
	return cleanPath
}

func compactYaziDisplayText(value string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		home = filepath.Clean(home)
		separator := string(filepath.Separator)
		switch {
		case value == home:
			value = "~"
		case home != separator:
			value = strings.ReplaceAll(value, home+separator, "~"+separator)
		}
	}
	return sanitizeLogLine(value)
}

func yaziFocusedObservationText(a *App, index int) string {
	if a == nil {
		return ""
	}
	if fieldID := yaziUIFieldID(index); fieldID != "" {
		if provenance, ok := a.nativeConfigState.Yazi.Fields[fieldID]; ok {
			label := "Observed at "
			if a.nativeConfigState.PreferenceError != "" {
				label = "Observed only (not applied): "
			}
			return fmt.Sprintf("%s%s:%d • %s • %s",
				label,
				sanitizeLogLine(compactYaziDisplayPath(provenance.Path)),
				provenance.Line,
				sanitizeLogLine(provenance.Key),
				sanitizeLogLine(string(provenance.Scope)),
			)
		}
	}
	observation := yaziUIOwningObservation(a, index)
	if observation.Path == "" {
		return ""
	}
	return fmt.Sprintf("Observed file: %s • %s",
		sanitizeLogLine(compactYaziDisplayPath(observation.Path)),
		sanitizeLogLine(string(observation.Ownership)),
	)
}

func yaziThemeObservationText(a *App) string {
	if a == nil {
		return ""
	}
	theme := a.nativeConfigState.Yazi.Theme
	if theme.Kind != tools.YaziFileKindTheme || theme.Path == "" || theme.Ownership == "" {
		return ""
	}
	parts := []string{
		"Theme file: " + sanitizeLogLine(compactYaziDisplayPath(theme.Path)),
		sanitizeLogLine(string(theme.Ownership)),
		"display-only",
	}
	if theme.ReadOnlyReason != "" {
		parts = append(parts, compactYaziDisplayText(theme.ReadOnlyReason))
	}
	if theme.Error != "" {
		parts = append(parts, compactYaziDisplayText(theme.Error))
	}
	return strings.Join(parts, " • ")
}

func renderYaziReadOnlyLiteral(value string, focused bool, width int) string {
	valueStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	if focused {
		valueStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	}
	line := "    " + valueStyle.Render(sanitizeLogLine(value)) + " " +
		lipgloss.NewStyle().Foreground(ColorYellow).Render("(read-only)")
	return lipgloss.NewStyle().Width(width).Render(line)
}

func yaziAdjust(a *App, key string, fwd bool) {
	if yaziUIFieldBlockReason(a, yaziUIFieldKey(a.configFieldIndex)) != "" {
		return
	}
	cfg := a.deepDiveConfig
	switch key {
	case "left", "right", "h", "l":
		switch a.configFieldIndex {
		case 0:
			if cfg.YaziKeymap == "vim" {
				cfg.YaziKeymap = "emacs"
			} else {
				cfg.YaziKeymap = "vim"
			}
		case 2:
			opts := []string{"auto", "always", "never"}
			cfg.YaziPreviewMode = cycleOption(opts, cfg.YaziPreviewMode, fwd)
		case 3:
			opts := []string{"alphabetical", "modified", "size", "natural"}
			cfg.YaziSortBy = cycleOption(opts, cfg.YaziSortBy, fwd)
		case 5:
			opts := []string{"size", "permissions", "mtime", "none"}
			cfg.YaziLineMode = cycleOption(opts, cfg.YaziLineMode, fwd)
		case 6:
			if fwd && cfg.YaziScrollOff < 20 {
				cfg.YaziScrollOff++
			} else if !fwd && cfg.YaziScrollOff > 0 {
				cfg.YaziScrollOff--
			}
		}
	case " ":
		switch a.configFieldIndex {
		case 1:
			cfg.YaziShowHidden = !cfg.YaziShowHidden
		case 4:
			cfg.YaziSortReverse = !cfg.YaziSortReverse
		}
	}
}

// Update delegates to the shared field-navigation handler.
func (s *configYaziScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	return s, s.handleMsg(msg)
}

// View renders the Yazi configuration screen.
func (s *configYaziScreen) View(width, height int) string {
	a := s.App()
	title := renderConfigTitle("󰉋", "Yazi", "File manager settings")
	blockedReason := yaziUIFieldBlockReason(a, yaziUIFieldKey(a.configFieldIndex))

	cfg := a.deepDiveConfig
	rec := newFieldLayoutRecorder(a.deepDiveBoxWidth(50))

	rec.field(0)
	keymapFocused := a.configFieldIndex == 0
	rec.write(renderFieldLabel("Keymap Style", keymapFocused))
	if yaziUIFieldBlockReason(a, "keymap") != "" {
		rec.write(renderYaziReadOnlyLiteral(cfg.YaziKeymap, keymapFocused, rec.innerWidth))
	} else {
		rec.write(renderOptionSelector(
			[]string{"vim", "emacs"},
			[]string{"Vim (hjkl)", "Emacs (arrows)"},
			cfg.YaziKeymap,
			keymapFocused,
		))
	}
	rec.write("\n\n")

	rec.field(1)
	hiddenFocused := a.configFieldIndex == 1
	rec.write(renderFieldLabel("Show Hidden Files", hiddenFocused))
	if yaziUIFieldBlockReason(a, "hidden") != "" {
		rec.write(renderYaziReadOnlyLiteral(map[bool]string{true: "ON", false: "OFF"}[cfg.YaziShowHidden], hiddenFocused, rec.innerWidth))
	} else {
		rec.write(renderToggle(cfg.YaziShowHidden, hiddenFocused))
	}
	rec.write("\n\n")

	rec.field(2)
	previewFocused := a.configFieldIndex == 2
	rec.write(renderFieldLabel("File Preview", previewFocused))
	if yaziUIFieldBlockReason(a, "preview_mode") != "" {
		rec.write(renderYaziReadOnlyLiteral(cfg.YaziPreviewMode, previewFocused, rec.innerWidth))
	} else {
		rec.write(renderOptionSelector(
			[]string{"auto", "always", "never"},
			[]string{"Auto", "Always", "Never"},
			cfg.YaziPreviewMode,
			previewFocused,
		))
	}
	rec.write("\n\n")

	rec.field(3)
	sortFocused := a.configFieldIndex == 3
	rec.write(renderFieldLabel("Sort By", sortFocused))
	if yaziUIFieldBlockReason(a, "sort_by") != "" {
		rec.write(renderYaziReadOnlyLiteral(cfg.YaziSortBy, sortFocused, rec.innerWidth))
	} else {
		rec.write(renderOptionSelector(
			[]string{"alphabetical", "modified", "size", "natural"},
			[]string{"Alphabetical", "Modified", "Size", "Natural"},
			cfg.YaziSortBy,
			sortFocused,
		))
	}
	rec.write("\n\n")

	rec.field(4)
	reverseFocused := a.configFieldIndex == 4
	rec.write(renderFieldLabel("Reverse Sort", reverseFocused))
	if yaziUIFieldBlockReason(a, "sort_rev") != "" {
		rec.write(renderYaziReadOnlyLiteral(map[bool]string{true: "ON", false: "OFF"}[cfg.YaziSortReverse], reverseFocused, rec.innerWidth))
	} else {
		rec.write(renderToggle(cfg.YaziSortReverse, reverseFocused))
	}
	rec.write("\n\n")

	rec.field(5)
	lineModeFocused := a.configFieldIndex == 5
	rec.write(renderFieldLabel("Line Metadata", lineModeFocused))
	if yaziUIFieldBlockReason(a, "linemode") != "" {
		rec.write(renderYaziReadOnlyLiteral(cfg.YaziLineMode, lineModeFocused, rec.innerWidth))
	} else {
		rec.write(renderOptionSelector(
			[]string{"size", "permissions", "mtime", "none"},
			[]string{"Size", "Permissions", "Modified", "None"},
			cfg.YaziLineMode,
			lineModeFocused,
		))
	}
	rec.write("\n\n")

	rec.field(6)
	scrollFocused := a.configFieldIndex == 6
	rec.write(renderFieldLabel("Scroll Offset", scrollFocused))
	if yaziUIFieldBlockReason(a, "scrolloff") != "" {
		rec.write(renderYaziReadOnlyLiteral(fmt.Sprintf("%d lines", cfg.YaziScrollOff), scrollFocused, rec.innerWidth))
	} else {
		offsetStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if scrollFocused {
			offsetStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		}
		rec.write(fmt.Sprintf("    ◀ %s ▶", offsetStyle.Render(fmt.Sprintf("%d lines", cfg.YaziScrollOff))))
	}

	box := configBoxStyle.Width(rec.boxWidth).Render(rec.String())
	help := s.footer()
	notices := make([]string, 0, 3)
	if blockedReason != "" {
		notice := lipgloss.NewStyle().
			Foreground(ColorYellow).
			Width(rec.innerWidth).
			Render("Read-only: " + sanitizeLogLine(blockedReason))
		notices = append(notices, notice)
	}
	if observationText := yaziFocusedObservationText(a, a.configFieldIndex); observationText != "" {
		noticeWidth := width - 4
		if noticeWidth < 1 {
			noticeWidth = 1
		}
		notices = append(notices, lipgloss.NewStyle().Foreground(ColorTextMuted).Width(noticeWidth).Render(observationText))
	}
	if themeText := yaziThemeObservationText(a); themeText != "" {
		noticeWidth := width - 4
		if noticeWidth < 1 {
			noticeWidth = 1
		}
		notices = append(notices, lipgloss.NewStyle().Foreground(ColorTextMuted).Width(noticeWidth).Render(themeText))
	}
	if len(notices) > 0 {
		content := lipgloss.JoinVertical(lipgloss.Center, title, "", box, strings.Join(notices, "\n"), "", help)
		a.configFieldLayout = rec.finalizeComposed(width, height, content, box, lipgloss.Height(title)+1)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
	}
	a.configFieldLayout = rec.finalize(width, height, title, box, help)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help),
	)
}
