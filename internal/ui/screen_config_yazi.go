package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

type yaziFieldDescriptor struct {
	key   string
	label string
}

var yaziFieldDescriptors = [...]yaziFieldDescriptor{
	{key: "keymap", label: "Keymap Style"},
	{key: "hidden", label: "Show Hidden Files"},
	{key: "preview_mode", label: "File Preview"},
	{key: "sort_by", label: "Sort By"},
	{key: "sort_rev", label: "Reverse Sort"},
	{key: "linemode", label: "Line Metadata"},
	{key: "scrolloff", label: "Scroll Offset"},
}

func yaziFieldDisplayValue(cfg tools.YaziConfig, index int) string {
	switch index {
	case 0:
		if cfg.Keymap == "emacs" {
			return "Emacs Ctrl-P/N/B/F + Space"
		}
		if cfg.Keymap == "vim" {
			return "Vim/Yazi defaults"
		}
		return cfg.Keymap
	case 1:
		if cfg.ShowHidden {
			return "ON"
		}
		return "OFF"
	case 2:
		switch cfg.PreviewMode {
		case "auto":
			return "Default image-preview delay (30ms)"
		case "always":
			return "Immediate image previews (0ms delay)"
		case "never":
			return "Disable previewers + preloaders"
		default:
			return cfg.PreviewMode
		}
	case 3:
		switch cfg.SortBy {
		case "alphabetical":
			return "Alphabetical"
		case "modified":
			return "Modified (mtime)"
		case "size":
			return "Size"
		case "natural":
			return "Natural"
		default:
			return cfg.SortBy
		}
	case 4:
		if cfg.SortReverse {
			return "ON"
		}
		return "OFF"
	case 5:
		switch cfg.LineMode {
		case "size":
			return "Size"
		case "permissions":
			return "Permissions"
		case "mtime":
			return "Modified metadata (mtime)"
		case "none":
			return "None"
		default:
			return cfg.LineMode
		}
	case 6:
		return fmt.Sprintf("%d lines", cfg.ScrollOff)
	default:
		return ""
	}
}

func yaziFieldDescription(cfg tools.YaziConfig, index int) string {
	switch index {
	case 0, 2, 3, 5:
		return yaziFieldDisplayValue(cfg, index)
	case 1:
		return "Show dotfiles by default"
	case 4:
		return "Reverse selected sort order"
	case 6:
		return "Keep entries above and below cursor"
	default:
		return ""
	}
}

func yaziManageFieldDescription(cfg *ManageConfig, key string) string {
	if cfg == nil {
		return ""
	}
	index := -1
	for candidate, descriptor := range yaziFieldDescriptors {
		if descriptor.key == key {
			index = candidate
			break
		}
	}
	if index < 0 {
		return ""
	}
	return sanitizeLogLine(yaziFieldDescription(yaziConfigFrom(manageConfigToDeepDive(cfg)), index))
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
	if index >= 0 && index < len(yaziFieldDescriptors) {
		return yaziFieldDescriptors[index].key
	}
	return ""
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

func yaziRawDisplayValue(cfg tools.YaziConfig, index int) string {
	switch index {
	case 0:
		return cfg.Keymap
	case 1:
		if cfg.ShowHidden {
			return "ON"
		}
		return "OFF"
	case 2:
		return cfg.PreviewMode
	case 3:
		return cfg.SortBy
	case 4:
		if cfg.SortReverse {
			return "ON"
		}
		return "OFF"
	case 5:
		return cfg.LineMode
	case 6:
		return fmt.Sprintf("%d lines", cfg.ScrollOff)
	default:
		return ""
	}
}

func yaziEditorFooter(a *App, blocked bool) string {
	if a != nil && a.configStandalone {
		if blocked {
			return HelpStyle.Render("↑↓ • focused read-only • enter preview • esc/q cancel")
		}
		return HelpStyle.Render("↑↓ • ←→/space change • enter preview • esc/q cancel")
	}
	if blocked {
		return HelpStyle.Render("↑↓ navigate • focused read-only • enter/esc back • q quit")
	}
	return HelpStyle.Render("↑↓ navigate • ←→/space change • enter/esc back • q quit")
}

func compactYaziThemeObservationText(a *App) string {
	if a == nil {
		return ""
	}
	theme := a.nativeConfigState.Yazi.Theme
	if theme.Kind != tools.YaziFileKindTheme || theme.Path == "" || theme.Ownership == "" {
		return ""
	}
	return fmt.Sprintf("Theme file: %s • %s • display-only",
		compactYaziDisplayPath(theme.Path), sanitizeLogLine(string(theme.Ownership)))
}

func wrapYaziCompactLine(value string, width int, style lipgloss.Style) string {
	if width < 1 {
		width = 1
	}
	return style.Render(ansi.Wrap(sanitizeLogLine(value), width, " /•:-"))
}

func (s *configYaziScreen) compactView(width, height int) string {
	a := s.App()
	cfg := yaziConfigFrom(*a.deepDiveConfig)
	focus := clampInt(a.configFieldIndex, 0, len(yaziFieldDescriptors)-1)
	const capacity = 3
	start := clampInt(focus-1, 0, len(yaziFieldDescriptors)-capacity)
	end := min(len(yaziFieldDescriptors), start+capacity)
	blockedReason := yaziUIFieldBlockReason(a, yaziUIFieldKey(focus))

	innerWidth := max(20, width-4)
	rows := make([]string, 0, capacity+2)
	rowFields := make([]int, 0, capacity)
	if start > 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("↑ more above"))
	}
	for index := start; index < end; index++ {
		descriptor := yaziFieldDescriptors[index]
		focused := index == focus
		cursor := "  "
		labelStyle := lipgloss.NewStyle().Foreground(ColorText)
		valueStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			labelStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
			valueStyle = labelStyle
		}
		blocked := yaziUIFieldBlockReason(a, descriptor.key) != ""
		value := yaziFieldDisplayValue(cfg, index)
		marker := ""
		if blocked {
			value = yaziRawDisplayValue(cfg, index)
			marker = " " + lipgloss.NewStyle().Foreground(ColorYellow).Render("(read-only)")
		}
		prefix := cursor + labelStyle.Render(descriptor.label) + ": "
		value = sanitizeLogLine(value)
		if blocked {
			valueBudget := max(1, innerWidth-lipgloss.Width(prefix)-lipgloss.Width(marker))
			value = truncateVisible(value, valueBudget)
		}
		line := prefix + valueStyle.Render(value) + marker
		rows = append(rows, truncateVisible(line, innerWidth))
		rowFields = append(rowFields, index)
	}
	if end < len(yaziFieldDescriptors) {
		rows = append(rows, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("↓ more below"))
	}
	for index := range rows {
		rows[index] = lipgloss.NewStyle().Width(innerWidth).Render(rows[index])
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(strings.Join(rows, "\n"))
	title := renderConfigTitle("󰉋", "Yazi", "File manager settings")
	parts := []string{title, box}
	if blockedReason != "" {
		parts = append(parts, wrapYaziCompactLine("Read-only: "+blockedReason, width, lipgloss.NewStyle().Foreground(ColorYellow)))
	}
	if observation := yaziFocusedObservationText(a, focus); observation != "" {
		parts = append(parts, wrapYaziCompactLine(observation, width, lipgloss.NewStyle().Foreground(ColorTextMuted)))
	}
	if theme := compactYaziThemeObservationText(a); theme != "" {
		parts = append(parts, wrapYaziCompactLine(theme, width, lipgloss.NewStyle().Foreground(ColorTextMuted)))
	}
	footer := yaziEditorFooter(a, blockedReason != "")
	parts = append(parts, footer)
	content := lipgloss.JoinVertical(lipgloss.Center, parts...)

	contentH, contentW := lipgloss.Height(content), lipgloss.Width(content)
	topPad := max(0, (height-contentH)/2)
	leftPad := max(0, (width-contentW)/2)
	boxW := lipgloss.Width(box)
	boxLeftInContent := max(0, (contentW-boxW)/2)
	boxTop := topPad + lipgloss.Height(title)
	boxContentTop := boxTop + 1
	boxLeft := leftPad + boxLeftInContent + 2
	rowOffset := 0
	if start > 0 {
		rowOffset++
	}
	layout := fieldLayout{boxLeft: boxLeft, boxRight: boxLeft + innerWidth - 1, hasXBounds: true}
	for offset, field := range rowFields {
		layout.extents = append(layout.extents, fieldExtent{index: field, startY: boxContentTop + rowOffset + offset, height: 1})
	}
	a.configFieldLayout = layout
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
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
	if width <= 80 || height <= 24 {
		return s.compactView(width, height)
	}
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
			[]string{"Vim", "Emacs"},
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
			[]string{"Default", "Immediate", "Disabled"},
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
	help := yaziEditorFooter(a, blockedReason != "")
	notices := make([]string, 0, 4)
	if description := yaziFieldDescription(yaziConfigFrom(*cfg), a.configFieldIndex); description != "" {
		notices = append(notices, lipgloss.NewStyle().Foreground(ColorTextMuted).Render(sanitizeLogLine(description)))
	}
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
