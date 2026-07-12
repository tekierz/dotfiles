package ui

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

// ==========================
// Manage Screen (Dual Pane)
// ==========================
//
// Design goals:
// - Always render in a predictable full-screen layout (no centering) so mouse hit-testing is simple.
// - Dual-pane by default: left = tools, right = settings for selected tool.
// - Keyboard first, but mouse friendly (click to select, wheel to scroll, click to toggle/adjust).
//
// Notes:
// - This UI edits a persistent ManageConfig stored at:
//   ~/.config/dotfiles/tools/manage.json
//   via internal/config's generic JSON helpers.
//
// - The underlying "apply config to actual tool config files" is a separate concern; here we focus
//   on the management experience + storing preferences.

const (
	managePaneTools    = 0
	managePaneSettings = 1
)

// manageFieldKind describes how a setting should be rendered and edited.
type manageFieldKind int

const (
	manageFieldText manageFieldKind = iota
	manageFieldToggle
	manageFieldNumber
	manageFieldOption
)

// manageField is a single editable field in the right pane.
//
// This is intentionally "small" and pointer-based so we can edit values without lots of boilerplate.
// For more complex types (slices, maps), add explicit handlers later.
type manageField struct {
	key         string
	label       string
	description string
	kind        manageFieldKind

	// One of these will be set depending on kind.
	str  *string
	b    *bool
	n    *int
	unit string

	// For option fields (kind == manageFieldOption).
	options         []string
	unknownReadOnly bool

	// For numeric fields (kind == manageFieldNumber).
	min  int
	max  int
	step int

	// Optional exact validation for text fields. The editor keeps focus and
	// leaves the model unchanged when validation fails.
	validateText   func(string) error
	readOnlyReason string
}

// manageItem is a tool entry in the left pane.
type manageItem struct {
	id           string
	name         string
	icon         string
	description  string
	category     tools.Category
	installed    bool
	configurable bool
	presence     health.Presence
	installable  health.Installability
	observed     bool
}

func (item manageItem) installationTruth() (health.Presence, health.Installability) {
	if !item.observed {
		return health.PresenceUnknown, health.InstallabilityUnknown
	}
	return item.presence, item.installable
}

func (item manageItem) installationAction() string {
	presence, installability := item.installationTruth()
	switch presence {
	case health.PresencePresent:
		return "none"
	case health.PresencePartial:
		if installability == health.InstallabilitySupported {
			return "repair"
		}
	case health.PresenceMissing:
		if installability == health.InstallabilitySupported {
			return "install"
		}
	case health.PresenceUnknown:
		return "blocked"
	}
	return "blocked"
}

func (item manageItem) installationLabel() string {
	presence, installability := item.installationTruth()
	switch presence {
	case health.PresencePresent:
		return "installed"
	case health.PresencePartial:
		switch installability {
		case health.InstallabilitySupported:
			return "partial — repair"
		case health.InstallabilityUnsupported:
			return "partial — unavailable"
		case health.InstallabilityUnknown:
			return "partial — availability unknown"
		}
	case health.PresenceMissing:
		return "not installed"
	case health.PresenceUnknown:
		return "status unknown"
	}
	return "status unknown"
}

// manageLayout captures all geometry needed for consistent rendering and mouse hit-testing.
type manageLayout struct {
	w int
	h int

	headerH int
	footerH int
	bodyY   int
	bodyH   int

	gap int

	leftX int
	leftW int

	rightX int
	rightW int

	// Panel internals (we keep these constants in sync with render styles).
	border int
	padX   int
	padY   int

	// List and fields areas (absolute coordinates in terminal space).
	leftListY int
	leftListH int

	rightListY int
	rightListH int

	// Optional widget area (e.g., globe) in the bottom of the right pane.
	rightGlobeY int
	rightGlobeH int
}

func (l manageLayout) maxToolsScroll(itemsLen int) int {
	return maxInt(0, itemsLen-l.leftListH)
}

func (l manageLayout) maxFieldsScroll(fieldsLen int) int {
	return maxInt(0, fieldsLen-l.rightListH)
}

func (l manageLayout) inLeftList(x, y int) bool {
	if x < l.leftX || x >= l.leftX+l.leftW {
		return false
	}
	return y >= l.leftListY && y < l.leftListY+l.leftListH
}

func (l manageLayout) inRightList(x, y int) bool {
	if x < l.rightX || x >= l.rightX+l.rightW {
		return false
	}
	return y >= l.rightListY && y < l.rightListY+l.rightListH
}

func (a *App) manageLayout() manageLayout {
	// Header/footer heights are kept fixed for consistent mouse mapping.
	const headerH = 3
	const footerH = 2

	bodyY := headerH
	bodyH := a.height - headerH - footerH
	if bodyH < 5 {
		bodyH = 5
	}

	gap := 1

	// Default split: 1/3 tools, 2/3 details.
	leftW := clampInt(a.width/3, 26, 42)
	minRight := 38
	if a.width-leftW-gap < minRight {
		leftW = maxInt(22, a.width-minRight-gap)
	}
	rightW := maxInt(0, a.width-leftW-gap)

	// Panel styling constants (must match render functions).
	border := 1
	padX := 1
	padY := 1

	// Left panel: title line + subtitle line + blank line.
	leftHeaderLines := 3
	leftInnerY := bodyY + border + padY
	leftInnerH := bodyH - (border * 2) - (padY * 2)
	leftListY := leftInnerY + leftHeaderLines
	leftListH := maxInt(1, leftInnerH-leftHeaderLines)

	// Right panel: title line + subtitle line + blank line.
	rightHeaderLines := 3
	rightInnerY := bodyY + border + padY
	rightInnerH := bodyH - (border * 2) - (padY * 2)
	rightListY := rightInnerY + rightHeaderLines
	rightFieldsH := maxInt(1, rightInnerH-rightHeaderLines) // total area under header

	// Reserve space for a small animated widget (globe) when there's enough room.
	rightListH := rightFieldsH
	rightGlobeH := 0
	rightGlobeY := 0
	if a.animationsEnabled && rightW >= 56 && rightFieldsH >= 18 {
		globeH := 10
		if rightFieldsH >= 22 {
			globeH = 12
		}
		rightGlobeH = globeH
		rightListH = maxInt(1, rightFieldsH-rightGlobeH-1) // 1 line gap above globe
		rightGlobeY = rightListY + rightListH + 1
	}

	layout := manageLayout{
		w: a.width,
		h: a.height,

		headerH: headerH,
		footerH: footerH,
		bodyY:   bodyY,
		bodyH:   bodyH,

		gap: gap,

		leftX:  0,
		leftW:  leftW,
		rightX: leftW + gap,
		rightW: rightW,

		border: border,
		padX:   padX,
		padY:   padY,

		leftListY: leftListY,
		leftListH: leftListH,

		rightListY: rightListY,
		rightListH: rightListH,

		rightGlobeY: rightGlobeY,
		rightGlobeH: rightGlobeH,
	}
	if a.compactManageSinglePaneActive() {
		// Compact Manage is a single full-width pane. Keep the legacy vertical
		// anchors, but make rendering and hit-testing share the visible X span.
		layout.gap = 0
		layout.rightGlobeY = 0
		layout.rightGlobeH = 0
		if a.managePane == managePaneTools {
			layout.leftX = 0
			layout.leftW = layout.w
			layout.leftListY = layout.rightListY
			layout.leftListH = layout.rightListH
			layout.rightX = layout.w
			layout.rightW = 0
		} else {
			layout.leftW = 0
			layout.rightX = 0
			layout.rightW = layout.w
		}
	}
	return layout
}

func (a *App) compactManageSinglePaneActive() bool {
	if a == nil || a.width > 80 {
		return false
	}
	if a.managePane == managePaneTools {
		return true
	}
	items := a.manageItems()
	return len(items) > 0 && a.manageIndex >= 0 && a.manageIndex < len(items) && items[a.manageIndex].id == "yazi"
}

func (a *App) manageEnsureToolsVisible(layout manageLayout, itemsLen int) {
	if itemsLen <= 0 {
		a.manageIndex = 0
		a.manageToolsScroll = 0
		return
	}

	a.manageIndex = clampInt(a.manageIndex, 0, itemsLen-1)
	maxScroll := layout.maxToolsScroll(itemsLen)
	a.manageToolsScroll = clampInt(a.manageToolsScroll, 0, maxScroll)

	// Keep selection within [scroll, scroll+visible).
	if a.manageIndex < a.manageToolsScroll {
		a.manageToolsScroll = a.manageIndex
	} else if a.manageIndex >= a.manageToolsScroll+layout.leftListH {
		a.manageToolsScroll = a.manageIndex - layout.leftListH + 1
	}
	a.manageToolsScroll = clampInt(a.manageToolsScroll, 0, maxScroll)
}

func (a *App) manageEnsureFieldsVisible(layout manageLayout, fieldsLen int) {
	if fieldsLen <= 0 {
		a.configFieldIndex = 0
		a.manageFieldsScroll = 0
		return
	}

	a.configFieldIndex = clampInt(a.configFieldIndex, 0, fieldsLen-1)
	maxScroll := layout.maxFieldsScroll(fieldsLen)
	a.manageFieldsScroll = clampInt(a.manageFieldsScroll, 0, maxScroll)

	if a.configFieldIndex < a.manageFieldsScroll {
		a.manageFieldsScroll = a.configFieldIndex
	} else if a.configFieldIndex >= a.manageFieldsScroll+layout.rightListH {
		a.manageFieldsScroll = a.configFieldIndex - layout.rightListH + 1
	}
	a.manageFieldsScroll = clampInt(a.manageFieldsScroll, 0, maxScroll)
}

func (a *App) manageItems() []manageItem {
	toolSource := a.manageToolSource
	if toolSource == nil {
		toolSource = func() []tools.Tool { return tools.GetRegistry().All() }
	}
	all := slices.Clone(toolSource())
	typed := a.installationSnapshot.Digest() != ""

	// Prefer a stable, human-friendly ordering (category → name).
	categoryOrder := map[tools.Category]int{
		tools.CategoryShell:     0,
		tools.CategoryTerminal:  1,
		tools.CategoryEditor:    2,
		tools.CategoryFile:      3,
		tools.CategoryGit:       4,
		tools.CategoryContainer: 5,
		tools.CategoryUtility:   6,
		tools.CategoryApp:       7,
	}
	sort.SliceStable(all, func(i, j int) bool {
		ci := categoryOrder[all[i].Category()]
		cj := categoryOrder[all[j].Category()]
		if ci != cj {
			return ci < cj
		}
		return all[i].Name() < all[j].Name()
	})

	// Install cache should be populated asynchronously via startInstallCacheLoad().
	// If not ready yet, initialize empty map to avoid nil panics during loading.
	if a.manageInstalled == nil {
		a.manageInstalled = make(map[string]bool, len(all))
	}

	// Before the first typed observation only, retain the legacy platform filter.
	// Typed snapshots already bind installability to their accepted platform and
	// Manage must never rediscover host truth while rendering or handling keys.
	if !typed {
		platform := pkg.PlatformUnknown
		if a.manageDetectPlatform != nil {
			platform = a.manageDetectPlatform()
		}
		if platform != pkg.PlatformUnknown {
			filtered := make([]tools.Tool, 0, len(all))
			for _, t := range all {
				installed := a.manageInstalled[t.ID()]
				supported := installerAvailable(t, platform)
				if installed || supported {
					filtered = append(filtered, t)
				}
			}
			all = filtered
		}
	}

	// Add a global section at the top.
	items := []manageItem{
		{
			id:           "global",
			name:         "Global",
			icon:         "󰒓",
			description:  "UI + platform preferences",
			category:     "global",
			installed:    true,
			configurable: true,
			presence:     health.PresencePresent,
			installable:  health.InstallabilityUnsupported,
			observed:     true,
		},
	}

	for _, t := range all {
		icon := t.Icon()
		if icon == "" {
			icon = fallbackToolIcon(t.ID(), t.Category())
		}

		presence := health.PresenceUnknown
		installability := health.InstallabilityUnknown
		observed := false
		installed := a.manageInstalled[t.ID()]
		if typed {
			if observation, ok := a.installationSnapshot.Tool(t.ID()); ok {
				presence = observation.Presence()
				installability = observation.Installability()
				observed = true
			}
			installed = presence == health.PresencePresent
		} else if installed {
			presence = health.PresencePresent
			observed = true
		}

		items = append(items, manageItem{
			id:           t.ID(),
			name:         t.Name(),
			icon:         icon,
			description:  t.Description(),
			category:     t.Category(),
			installed:    installed,
			configurable: t.HasConfig(),
			presence:     presence,
			installable:  installability,
			observed:     observed,
		})
	}

	return items
}

func fallbackToolIcon(id string, cat tools.Category) string {
	// Reasonable defaults when a tool doesn't specify an icon.
	// Prefer category icons so the UI stays visually consistent.
	switch cat {
	case tools.CategoryShell:
		return ""
	case tools.CategoryTerminal:
		return ""
	case tools.CategoryEditor:
		return ""
	case tools.CategoryFile:
		return "󰉋"
	case tools.CategoryGit:
		return ""
	case tools.CategoryContainer:
		return ""
	case tools.CategoryUtility:
		return "󰘚"
	case tools.CategoryApp:
		return "󰏇"
	}

	// ID-based fallback (for unknown categories like "global").
	if strings.Contains(id, "git") {
		return ""
	}
	return "󰈚"
}

func (a *App) manageFieldsFor(itemID string) []manageField {
	cfg := a.manageConfig
	if cfg == nil {
		return nil
	}

	switch itemID {
	case "global":
		return []manageField{
			{
				key:         "theme",
				label:       "Theme",
				description: "Controls generated tool configs (installer) and visual accents",
				kind:        manageFieldOption,
				str:         &a.theme,
				options:     config.AvailableThemes,
			},
			{
				key:         "nav",
				label:       "Navigation",
				description: "Default navigation style throughout the TUI",
				kind:        manageFieldOption,
				str:         &a.navStyle,
				options:     []string{"emacs", "vim"},
			},
			{
				key:         "animations",
				label:       "Animations",
				description: "Enable animated UI elements (headers, globe, spinners)",
				kind:        manageFieldToggle,
				b:           &a.animationsEnabled,
			},
		}

	case "ghostty":
		return []manageField{
			{key: "font_family", label: "Font Family", description: "Terminal font family", kind: manageFieldText, str: &cfg.GhosttyFontFamily},
			{key: "font_size", label: "Font Size", description: "Font size (pt)", kind: manageFieldNumber, n: &cfg.GhosttyFontSize, min: 8, max: 32, step: 1, unit: "pt"},
			{key: "opacity", label: "Opacity", description: "Background opacity (%)", kind: manageFieldNumber, n: &cfg.GhosttyOpacity, min: 0, max: 100, step: 5, unit: "%"},
			{key: "blur", label: "Blur Radius", description: "Background blur (platform dependent)", kind: manageFieldNumber, n: &cfg.GhosttyBlurRadius, min: 0, max: 100, step: 1},
			{key: "cursor", label: "Cursor Style", description: "Cursor shape", kind: manageFieldOption, str: &cfg.GhosstyCursorStyle, options: []string{"block", "bar", "underline"}},
			{key: "scrollback", label: "Scrollback Limit", description: "Maximum scrollback storage in bytes", kind: manageFieldNumber, n: &cfg.GhosttyScrollbackLines, min: 1_000_000, max: 100_000_000, step: 1_000_000, unit: " bytes"},
			{key: "decor", label: "Window Decorations", description: "Show native window decorations", kind: manageFieldToggle, b: &cfg.GhosttyWindowDecorations},
			{key: "confirm_close", label: "Confirm Close", description: "Prompt before closing window", kind: manageFieldToggle, b: &cfg.GhosttyConfirmClose},
			{key: "tab_bindings", label: "Tab Bindings", description: "Modifier used for managed tab shortcuts", kind: manageFieldOption, str: &cfg.GhosttyTabBindings, options: []string{"super", "ctrl", "ctrl-shift"}},
		}

	case "tmux":
		return []manageField{
			{key: "prefix", label: "Prefix Key", description: "Leader key for tmux commands", kind: manageFieldOption, str: &cfg.TmuxPrefix, options: []string{"C-a", "C-b", "C-Space"}},
			{key: "split_binds", label: "Split Bindings", description: "Keys used for horizontal and vertical splits", kind: manageFieldOption, str: &cfg.TmuxSplitBinds, options: []string{"percent", "pipes"}},
			{key: "base", label: "Base Index", description: "Start window/pane numbering at", kind: manageFieldNumber, n: &cfg.TmuxBaseIndex, min: 0, max: 10, step: 1},
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.TmuxMouseMode},
			{key: "status_pos", label: "Status Position", description: "Status bar placement", kind: manageFieldOption, str: &cfg.TmuxStatusPosition, options: []string{"top", "bottom"}},
			{key: "pane_border", label: "Pane Border", description: "Pane border style", kind: manageFieldOption, str: &cfg.TmuxPaneBorderStyle, options: []string{"single", "double", "heavy", "simple"}},
			{key: "history", label: "History Limit", description: "Scrollback lines per pane", kind: manageFieldNumber, n: &cfg.TmuxHistoryLimit, min: 1000, max: 200000, step: 1000, unit: " lines"},
			{key: "escape", label: "Escape Time", description: "Escape timing for key chords", kind: manageFieldNumber, n: &cfg.TmuxEscapeTime, min: 0, max: 1000, step: 5, unit: "ms"},
			{key: "resize", label: "Aggressive Resize", description: "Aggressively resize panes on window changes", kind: manageFieldToggle, b: &cfg.TmuxAggressiveResize},
			// TPM (Plugin Manager) settings
			{key: "tpm_enabled", label: "TPM Enabled", description: "Enable Tmux Plugin Manager", kind: manageFieldToggle, b: &cfg.TmuxTPMEnabled},
			{key: "plugin_sensible", label: "tmux-sensible", description: "Sensible default settings", kind: manageFieldToggle, b: &cfg.TmuxPluginSensible},
			{key: "plugin_resurrect", label: "tmux-resurrect", description: "Save and restore sessions", kind: manageFieldToggle, b: &cfg.TmuxPluginResurrect},
			{key: "plugin_continuum", label: "tmux-continuum", description: "Automatic session saving", kind: manageFieldToggle, b: &cfg.TmuxPluginContinuum},
			{key: "plugin_yank", label: "tmux-yank", description: "Enhanced clipboard support", kind: manageFieldToggle, b: &cfg.TmuxPluginYank},
			{key: "continuum_save", label: "Auto-save Interval", description: "Minutes between auto-saves", kind: manageFieldNumber, n: &cfg.TmuxContinuumSaveMin, min: 5, max: 60, step: 5, unit: " min"},
			{key: "continuum_restore", label: "Auto-restore", description: "Restore sessions on tmux start", kind: manageFieldToggle, b: &cfg.TmuxContinuumRestore},
		}

	case "zsh":
		return []manageField{
			{key: "hist_size", label: "History Size", description: "Maximum history entries", kind: manageFieldNumber, n: &cfg.ZshHistorySize, min: 1000, max: 500000, step: 1000, unit: " entries"},
			{key: "hist_dups", label: "Ignore Duplicates", description: "Don't store duplicated history entries", kind: manageFieldToggle, b: &cfg.ZshHistoryIgnoreDups},
			{key: "autocd", label: "Auto CD", description: "Allow entering directories without typing cd", kind: manageFieldToggle, b: &cfg.ZshAutoCD},
			{key: "correct", label: "Auto Correction", description: "Suggest corrections for commands", kind: manageFieldToggle, b: &cfg.ZshCorrection},
			{key: "menu", label: "Completion Menu", description: "Use menu selection for completions", kind: manageFieldToggle, b: &cfg.ZshCompletionMenu},
			{key: "syntax", label: "Syntax Highlight", description: "Syntax highlighting in shell", kind: manageFieldToggle, b: &cfg.ZshSyntaxHighlight},
			{key: "autosug", label: "Auto Suggestions", description: "Inline suggestions from history", kind: manageFieldToggle, b: &cfg.ZshAutosuggestions},
		}

	case "neovim":
		return []manageField{
			// "numbers" is the single control: relative also enables relativenumber.
			{key: "numbers", label: "Line Numbers", description: "Absolute/relative/none (relative shows relativenumber)", kind: manageFieldOption, str: &cfg.NeovimLineNumbers, options: []string{"absolute", "relative", "none"}},
			{key: "tab", label: "Tab Width", description: "Indent width", kind: manageFieldNumber, n: &cfg.NeovimTabWidth, min: 2, max: 8, step: 1, unit: " spaces"},
			{key: "expand", label: "Expand Tab", description: "Use spaces instead of tabs", kind: manageFieldToggle, b: &cfg.NeovimExpandTab},
			{key: "wrap", label: "Line Wrap", description: "Soft wrap long lines", kind: manageFieldToggle, b: &cfg.NeovimWrap},
			{key: "cursor", label: "Cursor Line", description: "Highlight current line", kind: manageFieldToggle, b: &cfg.NeovimCursorLine},
			{key: "clip", label: "Clipboard", description: "Clipboard integration", kind: manageFieldOption, str: &cfg.NeovimClipboard, options: []string{"unnamedplus", "unnamed", "none"}},
			{key: "undo", label: "Undo File", description: "Persistent undo on disk", kind: manageFieldToggle, b: &cfg.NeovimUndoFile},
		}

	case "git":
		return []manageField{
			{key: "branch", label: "Default Branch", description: "Default init branch name", kind: manageFieldOption, str: &cfg.GitDefaultBranch, options: []string{"main", "master", "develop"}},
			{key: "setup_remote", label: "Auto Setup Remote", description: "Auto-create tracking remotes on push", kind: manageFieldToggle, b: &cfg.GitAutoSetupRemote},
			{key: "rebase", label: "Pull Rebase", description: "Prefer rebase on git pull", kind: manageFieldToggle, b: &cfg.GitPullRebase},
			{key: "diff", label: "Diff Tool", description: "Default diff tool", kind: manageFieldOption, str: &cfg.GitDiffTool, options: []string{"delta", "difftastic", "vimdiff", "nvimdiff"}},
			{key: "merge", label: "Merge Tool", description: "Default merge tool", kind: manageFieldOption, str: &cfg.GitMergeTool, options: []string{"vimdiff", "nvimdiff", "meld"}},
			{key: "creds", label: "Credential Helper", description: "Credential helper backend", kind: manageFieldOption, str: &cfg.GitCredentialHelper, options: []string{"store", "cache", "osxkeychain", "none"}},
			{key: "sign", label: "Sign Commits", description: "Require signed commits", kind: manageFieldToggle, b: &cfg.GitSignCommits},
			{key: "delta_side", label: "Delta Side-by-Side", description: "Render Delta diffs in two columns", kind: manageFieldToggle, b: &cfg.GitDeltaSideBySide},
			{key: "alias_st", label: "Alias st", description: "Manage git st = status", kind: manageFieldToggle, b: &cfg.GitAliasStatus},
			{key: "alias_co", label: "Alias co", description: "Manage git co = checkout", kind: manageFieldToggle, b: &cfg.GitAliasCheckout},
			{key: "alias_br", label: "Alias br", description: "Manage git br = branch", kind: manageFieldToggle, b: &cfg.GitAliasBranch},
			{key: "alias_ci", label: "Alias ci", description: "Manage git ci = commit", kind: manageFieldToggle, b: &cfg.GitAliasCommit},
			{key: "alias_lg", label: "Alias lg", description: "Manage compact graph-log alias", kind: manageFieldToggle, b: &cfg.GitAliasLogGraph},
		}

	case "yazi":
		return []manageField{
			{key: "keymap", label: "Keymap", description: yaziManageFieldDescription(cfg, "keymap"), kind: manageFieldOption, str: &cfg.YaziKeymap, options: []string{"vim", "emacs"}, readOnlyReason: yaziUIFieldBlockReason(a, "keymap")},
			{key: "hidden", label: "Show Hidden", description: yaziManageFieldDescription(cfg, "hidden"), kind: manageFieldToggle, b: &cfg.YaziShowHidden, readOnlyReason: yaziUIFieldBlockReason(a, "hidden")},
			{key: "preview_mode", label: "Preview Mode", description: yaziManageFieldDescription(cfg, "preview_mode"), kind: manageFieldOption, str: &cfg.YaziPreviewMode, options: []string{"auto", "always", "never"}, readOnlyReason: yaziUIFieldBlockReason(a, "preview_mode")},
			{key: "sort_by", label: "Sort By", description: yaziManageFieldDescription(cfg, "sort_by"), kind: manageFieldOption, str: &cfg.YaziSortBy, options: []string{"alphabetical", "modified", "size", "natural"}, readOnlyReason: yaziUIFieldBlockReason(a, "sort_by")},
			{key: "sort_rev", label: "Sort Reverse", description: yaziManageFieldDescription(cfg, "sort_rev"), kind: manageFieldToggle, b: &cfg.YaziSortReverse, readOnlyReason: yaziUIFieldBlockReason(a, "sort_rev")},
			{key: "linemode", label: "Line Mode", description: yaziManageFieldDescription(cfg, "linemode"), kind: manageFieldOption, str: &cfg.YaziLineMode, options: []string{"size", "permissions", "mtime", "none"}, readOnlyReason: yaziUIFieldBlockReason(a, "linemode")},
			{key: "scrolloff", label: "Scroll Offset", description: yaziManageFieldDescription(cfg, "scrolloff"), kind: manageFieldNumber, n: &cfg.YaziScrollOff, min: 0, max: 20, step: 1, unit: " lines", readOnlyReason: yaziUIFieldBlockReason(a, "scrolloff")},
		}

	case "fzf":
		return []manageField{
			{key: "opts", label: "Additional fzf Flags", description: "Literal fzf flags stored as data; shell code is never evaluated", kind: manageFieldText, str: &cfg.FzfDefaultOpts},
			{key: "height", label: "Height", description: "Height percentage for fzf UI", kind: manageFieldNumber, n: &cfg.FzfHeight, min: 20, max: 100, step: 5, unit: "%"},
			{key: "layout", label: "Layout", description: "Layout mode", kind: manageFieldOption, str: &cfg.FzfLayout, options: []string{"reverse", "default", "reverse-list"}},
			{key: "border", label: "Border Style", description: "Border style for fzf window", kind: manageFieldOption, str: &cfg.FzfBorderStyle, options: []string{"rounded", "sharp", "bold", "none"}},
			{key: "preview", label: "Preview", description: "Enable preview pane", kind: manageFieldToggle, b: &cfg.FzfPreview},
			{key: "preview_window", label: "Preview Window", description: "Preview placement/size", kind: manageFieldOption, str: &cfg.FzfPreviewWindow, options: []string{"right:50%", "up:50%", "down:50%"}},
		}

	case "lazygit":
		readOnlyReason := lazyGitManageUIBlockReason(a)
		return []manageField{
			{key: "side_fraction", label: "Panel Fraction", description: "Exact global panel-width fraction (0 through 1); per-repo config may override it", kind: manageFieldText, str: &cfg.LazyGitSidePanelWidth, validateText: tools.ValidateLazyGitSidePanelWidth, readOnlyReason: readOnlyReason},
			{key: "mouse", label: "Mouse Events", description: "Enable global mouse events; per-repo config may override it", kind: manageFieldToggle, b: &cfg.LazyGitMouseEvents, readOnlyReason: readOnlyReason},
			{key: "color_preset", label: "Color Preset", description: "Standard or light high contrast; custom native colors remain read-only", kind: manageFieldOption, str: &cfg.LazyGitColorPreset, options: []string{"standard", "light-high-contrast"}, unknownReadOnly: true, readOnlyReason: readOnlyReason},
			{key: "pager_preset", label: "Pager Preset", description: "Builtin or Delta dark pager (requires git-delta); custom native pagers remain read-only", kind: manageFieldOption, str: &cfg.LazyGitPagerPreset, options: []string{"builtin", "delta"}, unknownReadOnly: true, readOnlyReason: readOnlyReason},
		}

	case "lazydocker":
		return []manageField{
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.LazyDockerMouseMode},
			{key: "tail", label: "Logs Tail", description: "How many log lines to show", kind: manageFieldNumber, n: &cfg.LazyDockerLogsTail, min: 10, max: 2000, step: 10, unit: " lines"},
		}

	case "btop":
		return []manageField{
			{key: "theme", label: "Theme", description: "btop theme name", kind: manageFieldOption, str: &cfg.BtopTheme, options: []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}},
			{key: "rate", label: "Update Rate", description: "Refresh interval", kind: manageFieldNumber, n: &cfg.BtopUpdateMs, min: 250, max: 10000, step: 250, unit: "ms"},
			{key: "temp", label: "Show Temp", description: "Show CPU temperature", kind: manageFieldToggle, b: &cfg.BtopShowTemp},
			{key: "scale", label: "Temp Scale", description: "Celsius/Fahrenheit", kind: manageFieldOption, str: &cfg.BtopTempScale, options: []string{"celsius", "fahrenheit"}},
			{key: "graph", label: "Graph Symbol", description: "Graph rendering symbol set", kind: manageFieldOption, str: &cfg.BtopGraphSymbol, options: []string{"braille", "block", "tty"}},
			{key: "boxes", label: "Shown Boxes", description: "Which panels to show", kind: manageFieldText, str: &cfg.BtopShownBoxes},
		}

	case "glow":
		return []manageField{
			{key: "style", label: "Style", description: "8 built-ins; imported custom paths are read-only until explicitly replaced", kind: manageFieldOption, str: &cfg.GlowStyle, options: []string{"auto", "ascii", "dark", "dracula", "tokyo-night", "light", "notty", "pink"}},
			{key: "pager", label: "Use Pager", description: "Page CLI file rendering; $PAGER selects the command", kind: manageFieldOption, str: &cfg.GlowPager, options: []string{"auto", "never"}},
			{key: "width", label: "Width", description: "Maximum render width; 0 is Auto (max 120; fallback 80)", kind: manageFieldNumber, n: &cfg.GlowWidth, min: 0, max: math.MaxInt, step: 1, unit: " chars"},
			{key: "mouse", label: "Mouse", description: "Enable mouse support in the Glow TUI", kind: manageFieldToggle, b: &cfg.GlowMouse},
			{key: "all", label: "Show All Files", description: "Include hidden and ignored files in the Glow TUI", kind: manageFieldToggle, b: &cfg.GlowAll},
			{key: "line_numbers", label: "Line Numbers", description: "Show source line numbers in the Glow TUI", kind: manageFieldToggle, b: &cfg.GlowShowLineNumbers},
			{key: "preserve_newlines", label: "Preserve Newlines", description: "Preserve newlines in the TUI; v2.1.2 CLI always preserves them", kind: manageFieldToggle, b: &cfg.GlowPreserveNewLines},
		}

	case "claude-code":
		return []manageField{
			{key: "mcp_context7", label: "Context7", description: "Documentation lookup for any library (recommended)", kind: manageFieldToggle, b: &cfg.ClaudeCodeMCPContext7},
			{key: "mcp_taskmaster", label: "Task Master", description: "AI-driven task management", kind: manageFieldToggle, b: &cfg.ClaudeCodeMCPTaskMaster},
			{key: "mcp_github", label: "GitHub", description: "GitHub integration and automation", kind: manageFieldToggle, b: &cfg.ClaudeCodeMCPGitHub},
			{key: "mcp_supabase", label: "Supabase", description: "Supabase database integration", kind: manageFieldToggle, b: &cfg.ClaudeCodeMCPSupabase},
			{key: "mcp_convex", label: "Convex", description: "Convex backend integration", kind: manageFieldToggle, b: &cfg.ClaudeCodeMCPConvex},
			{key: "mcp_puppeteer", label: "Puppeteer", description: "Browser automation and testing", kind: manageFieldToggle, b: &cfg.ClaudeCodeMCPPuppeteer},
			{key: "mcp_sequential", label: "Seq. Thinking", description: "Enhanced reasoning chains", kind: manageFieldToggle, b: &cfg.ClaudeCodeMCPSequentialThinking},
		}
	}

	return nil
}

func (a *App) renderManageHeader(width int) string {
	tabs := RenderTabBar(ScreenManage, width)

	subText := "Dual-pane config editor • Click, scroll, and tweak everything"
	if notice := a.manageInstallationNotice(); notice != "" {
		subText = notice
	} else if a.animationsEnabled {
		subText = AnimatedSpinnerDots(a.uiFrame/2) + " " + subText
	}
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(subText, width))

	divider := ShimmerDivider(maxInt(0, width), a.uiFrame, a.animationsEnabled)

	// Keep this exactly 3 lines (see manageLayout.headerH).
	return lipgloss.JoinVertical(lipgloss.Left, tabs, sub, divider)
}

func (a *App) manageInstallationNotice() string {
	if a.installationSnapshotError != "" {
		return installationSnapshotUnavailable + " • stale"
	}
	if a.installationSnapshotStale {
		return "stale installation status"
	}
	return ""
}

func (a *App) renderManageFooter(width int, items []manageItem, fields []manageField) string {
	// Hint line: short and consistent.
	hints := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"Tab switch pane • ↑↓ move • ←→ adjust • Space toggle • Enter edit • I install • ? hotkeys • S save • Esc back • q quit",
	)

	// Status line: either save feedback, or focused field description.
	statusText := a.manageStatus
	if statusText == "" && len(items) > 0 && items[clampInt(a.manageIndex, 0, len(items)-1)].id == "lazygit" && lazyGitManageUIBlockReason(a) != "" {
		statusText = lazyGitManageUIBlockReason(a)
	}
	if statusText == "" && a.managePane == managePaneSettings && len(fields) > 0 {
		idx := clampInt(a.configFieldIndex, 0, len(fields)-1)
		if fields[idx].description != "" {
			statusText = fields[idx].description
		}
	}

	if statusText == "" {
		statusText = " "
	}
	status := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(statusText, width))

	// Keep this exactly 2 lines (see manageLayout.footerH).
	return lipgloss.JoinVertical(lipgloss.Left, hints, status)
}

func (a *App) renderManageToolsPanel(layout manageLayout, items []manageItem) string {
	borderColor := ColorBorder
	if a.managePane == managePaneTools {
		borderColor = ColorCyan
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		// lipgloss applies borders after Width/Height, so subtract 2 to target an
		// exact outer size for predictable layouts and mouse hit-testing.
		Width(maxInt(1, layout.leftW-2)).
		Height(maxInt(1, layout.bodyH-2))

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("TOOLS")
	toolCount := maxInt(0, len(items)-1) // exclude "Global"
	installedCount := 0
	for _, it := range items {
		if it.id == "global" {
			continue
		}
		if it.installed {
			installedCount++
		}
	}
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(fmt.Sprintf("%d installed • %d tools", installedCount, toolCount))

	innerW := maxInt(0, layout.leftW-(layout.border*2)-(layout.padX*2))
	tagStyle := lipgloss.NewStyle().Foreground(ColorText).Background(ColorOverlay).Padding(0, 1)

	var lines []string
	for i := a.manageToolsScroll; i < len(items) && len(lines) < layout.leftListH; i++ {
		it := items[i]
		focused := i == a.manageIndex

		cursor := "  "
		nameStyle := lipgloss.NewStyle().Foreground(ColorText)
		if it.id != "global" && !it.installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		}
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			nameStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		}

		status := StatusDot("pending")
		if it.id == "global" {
			status = lipgloss.NewStyle().Foreground(ColorCyan).Render("●")
		} else if it.installed {
			status = StatusDot("success")
		}

		icon := it.icon
		if icon != "" {
			icon += " "
		}

		// Right-aligned category tag (helps scanning without changing selection mapping).
		cat := strings.ToUpper(string(it.category))
		if it.id == "global" {
			cat = "GLOBAL"
		}
		tag := tagStyle.Render(cat)

		left := fmt.Sprintf("%s%s %s%s", cursor, status, icon, nameStyle.Render(it.name))
		// Small visual hint that settings exist.
		if it.id != "global" && it.configurable {
			left += lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  ")
		}

		leftW := ansi.StringWidth(left)
		tagW := ansi.StringWidth(tag)
		// Keep at least 1 space between left content and tag.
		availLeft := innerW - tagW - 1
		if availLeft < 0 {
			availLeft = 0
		}
		if leftW > availLeft {
			left = truncateVisible(left, availLeft)
			leftW = ansi.StringWidth(left)
		}
		spaces := innerW - leftW - tagW
		if spaces < 1 {
			spaces = 1
		}

		line := left + strings.Repeat(" ", spaces) + tag
		lines = append(lines, truncateVisible(line, innerW))
	}

	// Pad list to keep the panel stable.
	for len(lines) < layout.leftListH {
		lines = append(lines, "")
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		sub,
		"",
		strings.Join(lines, "\n"),
	)

	return panel.Render(content)
}

func (a *App) renderManageSettingsPanel(layout manageLayout, items []manageItem, fields []manageField) string {
	borderColor := ColorBorder
	if a.managePane == managePaneSettings {
		borderColor = ColorCyan
	}

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		// lipgloss applies borders after Width/Height, so subtract 2 to target an
		// exact outer size for predictable layouts and mouse hit-testing.
		Width(maxInt(1, layout.rightW-2)).
		Height(maxInt(1, layout.bodyH-2))

	if len(items) == 0 {
		return panel.Render(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No tools found"))
	}

	item := items[a.manageIndex]

	title := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true).Render("SETTINGS")
	statusBadge := ""
	if item.id != "global" {
		badgeColor := ColorMuted
		switch item.presence {
		case health.PresencePresent:
			badgeColor = ColorGreen
		case health.PresencePartial:
			badgeColor = ColorYellow
		case health.PresenceMissing, health.PresenceUnknown:
			badgeColor = ColorMuted
		}
		statusBadge = " " + RenderBadge(strings.ToUpper(item.installationLabel()), ColorBg, badgeColor)
		statusBadge += a.nativeImportBadge(item.id)
		if item.id == "lazygit" && a.nativeConfigState.LazyGit.RepoOverridesPossible {
			statusBadge += " " + RenderBadge("REPO OVERRIDES", ColorBg, ColorYellow)
		}
	}
	metaName := item.name
	if item.icon != "" {
		metaName = item.icon + " " + metaName
	}
	meta := lipgloss.NewStyle().Foreground(ColorTextBright).Bold(true).Render(metaName) +
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  "+item.description) + statusBadge
	if item.id == "yazi" {
		// Source ownership is primary metadata; keep it before the descriptive
		// tail so narrow full layouts cannot truncate the truth badge.
		meta = lipgloss.NewStyle().Foreground(ColorTextBright).Bold(true).Render(metaName) + statusBadge +
			lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  "+item.description)
	}

	innerW := maxInt(0, layout.rightW-(layout.border*2)-(layout.padX*2))

	// Field list lines (fixed height for stable layout).
	visibleFieldLines := layout.rightListH
	fieldCapacity := visibleFieldLines
	if a.manageEditing && (a.manageEditField != nil || a.manageEditNumber != nil) && fieldCapacity > 0 {
		// Reserve the first line for the editor, but keep overall height stable.
		fieldCapacity--
	}

	var fieldLines []string
	if len(fields) == 0 {
		// No explicit fields for this tool. Show a helpful placeholder plus an
		// install hint.
		msgStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		strong := lipgloss.NewStyle().Foreground(ColorText).Bold(true)

		if item.id == "global" {
			fieldLines = append(fieldLines, msgStyle.Render("No global settings available."))
		} else {
			if item.configurable {
				fieldLines = append(fieldLines, msgStyle.Render("No manager UI fields yet (tool has config)."))
			} else {
				fieldLines = append(fieldLines, msgStyle.Render("No configurable settings for this tool."))
			}

			switch item.installationAction() {
			case "install":
				fieldLines = append(fieldLines, strong.Render("Press I to install"))
			case "repair":
				fieldLines = append(fieldLines, strong.Render("Press I to repair"))
			case "none":
				fieldLines = append(fieldLines, msgStyle.Render("Installed — press S to save global prefs"))
			default:
				fieldLines = append(fieldLines, msgStyle.Render("Installation action blocked"))
			}
		}
	} else {
		for i := a.manageFieldsScroll; i < len(fields) && len(fieldLines) < fieldCapacity; i++ {
			f := fields[i]
			focused := (a.managePane == managePaneSettings) && (i == a.configFieldIndex)
			applied := manageFieldIsApplied(item.id, f.key)
			fieldLines = append(fieldLines, truncateVisible(renderManageFieldLine(f, focused, applied), innerW))
		}
	}
	for len(fieldLines) < fieldCapacity {
		fieldLines = append(fieldLines, "")
	}

	var fieldsBlock string
	if a.manageEditing && (a.manageEditField != nil || a.manageEditNumber != nil) && visibleFieldLines > 0 {
		fieldsBlock = strings.Join(append([]string{a.renderManageInlineEditor(innerW)}, fieldLines...), "\n")
	} else {
		fieldsBlock = strings.Join(fieldLines, "\n")
	}

	// Exactly 3 header lines before the fields area (matches manageLayout.rightHeaderLines).
	actionLine := ""
	if item.id == "lazygit" && lazyGitManageUIBlockReason(a) != "" {
		text := "READ-ONLY"
		if a.nativeConfigState.LazyGit.RepoOverridesPossible {
			text += " • REPO OVERRIDES"
		}
		actionLine = lipgloss.NewStyle().Foreground(ColorYellow).Render(text + " • " + lazyGitManageUIBlockReason(a))
	} else if item.id == "lazygit" {
		switch {
		case a.manageConfig.LazyGitPagerPreset == "delta":
			reason := lazyGitDeltaAvailabilityReason(a)
			if reason == "" {
				actionLine = lipgloss.NewStyle().Foreground(ColorGreen).Render("Delta installed • uses delta --dark --paging=never")
			} else {
				actionLine = lipgloss.NewStyle().Foreground(ColorYellow).Render(reason)
			}
		case a.nativeConfigState.LazyGit.RepoOverridesPossible:
			actionLine = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Global defaults; repository config may override them")
		case item.installationAction() == "blocked":
			actionLine = lipgloss.NewStyle().Foreground(ColorYellow).Render("Installation action blocked")
		}
	} else if item.id != "global" && item.installationAction() == "install" {
		actionLine = lipgloss.NewStyle().Foreground(ColorYellow).Render("I: install this tool/app")
	} else if item.id != "global" && item.installationAction() == "repair" {
		actionLine = lipgloss.NewStyle().Foreground(ColorYellow).Render("I: repair this tool/app")
	} else if item.id != "global" && item.installationAction() == "blocked" {
		actionLine = lipgloss.NewStyle().Foreground(ColorYellow).Render("Installation action blocked")
	} else if item.id != "global" && len(fields) == 0 {
		actionLine = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No editable fields in manager yet")
	}

	contentLines := []string{
		title,
		truncateVisible(meta, innerW),
		truncateVisible(actionLine, innerW),
		fieldsBlock,
	}

	// Optional animated widget area (globe) at the bottom.
	if a.animationsEnabled && layout.rightGlobeH > 0 {
		globeW := min(30, maxInt(20, innerW/2))
		globe := RenderMiniGlobe(globeW, layout.rightGlobeH, a.uiFrame)
		globePlaced := lipgloss.Place(innerW, layout.rightGlobeH, lipgloss.Right, lipgloss.Center, globe)
		contentLines = append(contentLines, "", globePlaced)
	}

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		contentLines...,
	)

	return panel.Render(content)
}

func (a *App) nativeImportBadge(toolID string) string {
	var (
		sources []tools.ConfigImportSource
		fields  int
		errText string
	)
	switch toolID {
	case "git":
		sources, fields, errText = a.nativeConfigState.Git.Sources, len(a.nativeConfigState.Git.Fields), a.nativeConfigState.GitError
	case "ghostty":
		sources, fields, errText = a.nativeConfigState.Ghostty.Sources, len(a.nativeConfigState.Ghostty.Fields), a.nativeConfigState.GhosttyError
	case "tmux":
		sources, fields, errText = a.nativeConfigState.Tmux.Sources, len(a.nativeConfigState.Tmux.Fields), a.nativeConfigState.TmuxError
	case "btop":
		sources, fields, errText = a.nativeConfigState.Btop.Sources, len(a.nativeConfigState.Btop.Fields), a.nativeConfigState.BtopError
	case "glow":
		sources, fields, errText = a.nativeConfigState.Glow.Sources, len(a.nativeConfigState.Glow.Fields), a.nativeConfigState.GlowError
	case "lazygit":
		sources, fields, errText = a.nativeConfigState.LazyGit.Sources, len(a.nativeConfigState.LazyGit.Fields), a.nativeConfigState.LazyGitError
		if errText == "" {
			errText = a.nativeConfigState.LazyGit.ReadOnlyReason
		}
	case "yazi":
		imported := a.nativeConfigState.Yazi
		if a.nativeConfigState.PreferenceError != "" || a.nativeConfigState.YaziError != "" || imported.Main.Ownership == tools.YaziOwnershipMalformed || imported.Keymap.Ownership == tools.YaziOwnershipMalformed {
			return " " + RenderBadge("IMPORT BLOCKED", ColorBg, ColorYellow)
		}
		managed, native := false, false
		for _, observation := range []tools.YaziFileObservation{imported.Main, imported.Keymap} {
			switch observation.Ownership {
			case tools.YaziOwnershipExactCurrent, tools.YaziOwnershipExactHistorical:
				managed = true
			case tools.YaziOwnershipNative:
				native = true
			case tools.YaziOwnershipMalformed:
				// Defensive fail-closed handling if the precheck above changes.
				return " " + RenderBadge("IMPORT BLOCKED", ColorBg, ColorYellow)
			case tools.YaziOwnershipMissing, "":
				// Missing sources contribute no badge.
			}
		}
		switch {
		case native && managed:
			return " " + RenderBadge("NATIVE SOURCE", ColorBg, ColorCyan) + " " + RenderBadge("MANAGED SOURCE", ColorBg, ColorCyan)
		case native:
			return " " + RenderBadge("NATIVE SOURCE", ColorBg, ColorCyan)
		case managed:
			return " " + RenderBadge("MANAGED SOURCE", ColorBg, ColorCyan)
		default:
			return ""
		}
	default:
		return ""
	}
	if errText != "" || a.nativeConfigState.PreferenceError != "" {
		return " " + RenderBadge("IMPORT BLOCKED", ColorBg, ColorYellow)
	}
	if fields == 0 {
		return ""
	}
	managed := false
	native := false
	for _, source := range sources {
		if !source.Active {
			continue
		}
		managed = managed || source.Managed
		native = native || !source.Managed
	}
	label := "NATIVE SOURCE"
	if managed && native {
		label = "NATIVE + MANAGED"
	} else if managed {
		label = "MANAGED SOURCE"
	}
	return " " + RenderBadge(label, ColorBg, ColorCyan)
}

func (a *App) renderCompactManageYazi(layout manageLayout, fields []manageField) string {
	rows := make([]string, layout.h)
	put := func(y int, value string) {
		if y < 0 || y >= len(rows) {
			return
		}
		rows[y] = ansi.Truncate(sanitizeLogLine(value), layout.w, "…")
	}
	putWrapped := func(start, limit int, value string) {
		wrapped := strings.Split(ansi.Wrap(sanitizeLogLine(value), layout.w, " /•:-"), "\n")
		for index, line := range wrapped {
			if start+index >= limit {
				break
			}
			put(start+index, line)
		}
	}

	focus := clampInt(a.configFieldIndex, 0, len(fields)-1)
	blockedReason := ""
	if len(fields) > 0 {
		blockedReason = fields[focus].readOnlyReason
	}
	put(0, compactManageTabLine)
	if notice := a.manageInstallationNotice(); notice != "" {
		put(1, notice)
	} else if blockedReason != "" {
		putWrapped(1, layout.bodyY, "Read-only: "+blockedReason)
	} else {
		put(1, "Manage terminal tools • Yazi")
	}
	put(layout.bodyY, "YAZI SETTINGS"+ansi.Strip(a.nativeImportBadge("yazi")))

	if theme := compactYaziThemeObservationText(a); theme != "" {
		putWrapped(layout.bodyY+1, layout.bodyY+3, theme)
	}
	if provenance := yaziFocusedObservationText(a, focus); provenance != "" {
		putWrapped(layout.bodyY+3, layout.rightListY, provenance)
	} else if len(fields) > 0 {
		put(layout.bodyY+3, yaziManageFieldDescription(a.manageConfig, fields[focus].key))
	}

	for index := a.manageFieldsScroll; index < len(fields) && index-a.manageFieldsScroll < layout.rightListH; index++ {
		field := fields[index]
		focused := a.managePane == managePaneSettings && index == focus
		cursor := "  "
		if focused {
			cursor = "▸ "
		}
		value := ""
		switch field.kind {
		case manageFieldToggle:
			if field.b != nil && *field.b {
				value = "ON"
			} else {
				value = "OFF"
			}
		case manageFieldNumber:
			if field.n != nil {
				value = fmt.Sprintf("%d%s", *field.n, field.unit)
			}
		case manageFieldOption, manageFieldText:
			if field.str != nil {
				value = sanitizeLogLine(*field.str)
			}
		}
		value = sanitizeLogLine(value)
		marker := ""
		if field.readOnlyReason != "" {
			marker = " (read-only)"
		}
		prefix := cursor + field.label + ": "
		if marker != "" {
			value = ansi.Truncate(value, max(1, layout.w-lipgloss.Width(prefix)-lipgloss.Width(marker)), "…")
		}
		put(layout.rightListY+(index-a.manageFieldsScroll), prefix+value+marker)
	}

	items := a.manageItems()
	uninstalled := len(items) > 0 && a.manageIndex >= 0 && a.manageIndex < len(items) && !items[a.manageIndex].installed
	if a.manageStatus != "" {
		put(layout.h-2, a.manageStatus)
	} else if uninstalled {
		put(layout.h-2, "I install")
	}
	help := "Tab tools ↑↓ ←→ Space S save Esc back q quit"
	if blockedReason != "" {
		help = "Tab tools ↑↓ focused read-only S save Esc back q quit"
	}
	if layout.w >= 80 {
		help += " • ? hotkeys"
	}
	put(layout.h-1, help)
	return strings.Join(rows, "\n")
}

const compactManageTabLine = "1 Manage  2 Users  3 Hotkeys  4 Update  5 Backups"

func detectCompactManageTabClick(x int) Screen {
	labels := []struct {
		text   string
		screen Screen
	}{
		{"1 Manage", ScreenManage},
		{"2 Users", ScreenUsers},
		{"3 Hotkeys", ScreenHotkeys},
		{"4 Update", ScreenUpdate},
		{"5 Backups", ScreenBackups},
	}
	start := 0
	for _, label := range labels {
		end := start + lipgloss.Width(label.text)
		if x >= start && x < end {
			return label.screen
		}
		start = end + 2
	}
	return 0
}

func (a *App) renderCompactManageTools(layout manageLayout, items []manageItem) string {
	rows := make([]string, layout.h)
	put := func(y int, value string) {
		if y >= 0 && y < len(rows) {
			rows[y] = ansi.Truncate(sanitizeLogLine(value), layout.w, "…")
		}
	}
	put(0, compactManageTabLine)
	put(1, a.manageInstallationNotice())
	put(layout.bodyY, "TOOLS • SETTINGS via Tab")
	for index := a.manageToolsScroll; index < len(items) && index-a.manageToolsScroll < layout.leftListH; index++ {
		cursor := "  "
		if index == a.manageIndex {
			cursor = "▸ "
		}
		status := items[index].installationLabel()
		put(layout.leftListY+(index-a.manageToolsScroll), fmt.Sprintf("%s%s • %s", cursor, items[index].name, status))
	}
	if a.manageStatus != "" {
		put(layout.h-2, a.manageStatus)
	}
	put(layout.h-1, "Tab settings • ↑↓ move • Enter settings • Esc back • q quit")
	return strings.Join(rows, "\n")
}

// renderManageFieldLine renders one settings row. applied=false means the field is
// editable but is NOT wired to any generator (manageNotAppliedFields); a clear
// "(not applied)" marker is appended so the user is never misled into thinking the
// save wrote it — the P1-B honesty requirement.
func renderManageFieldLine(f manageField, focused, applied bool) string {
	line := renderManageFieldLineBase(f, focused)
	if !applied {
		line += " " + lipgloss.NewStyle().Foreground(ColorYellow).Render("(not applied)")
	}
	return line
}

func renderManageFieldLineBase(f manageField, focused bool) string {
	// Left label column.
	labelStyle := lipgloss.NewStyle().Foreground(ColorText).Width(18)
	valueStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	cursor := "  "

	if focused {
		cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
		labelStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Width(18)
		valueStyle = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	}
	if f.readOnlyReason != "" {
		value := "—"
		switch f.kind {
		case manageFieldToggle:
			value = "OFF"
			if f.b != nil && *f.b {
				value = "ON"
			}
		case manageFieldText, manageFieldOption:
			if f.str != nil && *f.str != "" {
				value = *f.str
			}
			if f.kind == manageFieldOption {
				switch f.key + ":" + value {
				case "color_preset:custom", "pager_preset:custom":
					value = "Custom"
				case "color_preset:standard":
					value = "Standard"
				case "color_preset:light-high-contrast":
					value = "Light High Contrast"
				case "pager_preset:builtin":
					value = "Builtin"
				case "pager_preset:delta":
					value = "Delta (dark)"
				}
			}
		case manageFieldNumber:
			if f.n != nil {
				value = fmt.Sprintf("%d%s", *f.n, f.unit)
			}
		}
		return fmt.Sprintf("%s%s %s %s", cursor, labelStyle.Render(f.label), valueStyle.Render(value), lipgloss.NewStyle().Foreground(ColorYellow).Render("(read-only)"))
	}

	switch f.kind {
	case manageFieldToggle:
		val := lipgloss.NewStyle().Foreground(ColorRed).Render("OFF")
		if f.b != nil && *f.b {
			val = lipgloss.NewStyle().Foreground(ColorGreen).Render("ON")
		}
		return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), val)

	case manageFieldNumber:
		if f.n == nil {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
		}
		leftArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("◀")
		rightArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("▶")
		if focused {
			leftArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("◀")
			rightArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("▶")
		}
		display := fmt.Sprintf("%d%s", *f.n, f.unit)
		if f.key == "width" && f.min == 0 && *f.n == 0 {
			display = "Auto (max 120; fallback 80)"
		}
		val := valueStyle.Render(display)
		return fmt.Sprintf("%s%s %s %s %s", cursor, labelStyle.Render(f.label), leftArrow, val, rightArrow)

	case manageFieldOption:
		if f.str == nil || len(f.options) == 0 {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
		}
		if f.unknownReadOnly && !oneOf(*f.str, f.options...) {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), lipgloss.NewStyle().Foreground(ColorYellow).Render("Custom (read-only)"))
		}
		leftArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("◀")
		rightArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("▶")
		if focused {
			leftArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("◀")
			rightArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("▶")
		}
		display := *f.str
		if f.key == "pager" {
			switch *f.str {
			case "auto":
				display = "Enabled"
			case "never":
				display = "Disabled"
			}
		}
		val := valueStyle.Render(display)
		return fmt.Sprintf("%s%s %s %s %s", cursor, labelStyle.Render(f.label), leftArrow, val, rightArrow)

	case manageFieldText:
		if f.str == nil {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
		}
		val := *f.str
		if val == "" {
			val = "—"
		}
		return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render(val))
	}

	return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
}

func (a *App) renderManageInlineEditor(width int) string {
	// Single-line editor used for string fields.
	//
	// Important: this must remain ONE LINE so the fields pane layout and mouse
	// hit-testing remain stable.
	if width <= 0 {
		return ""
	}

	// Render a caret by inserting a solid block at the current position.
	runes := []rune(a.manageEditValue)
	cur := clampInt(a.manageEditCursor, 0, len(runes))
	left := string(runes[:cur])
	right := string(runes[cur:])

	plain := fmt.Sprintf("EDIT %s: %s%s%s", a.manageEditFieldKey, left, "█", right)
	plain = truncatePlain(plain, width)

	return lipgloss.NewStyle().
		Foreground(ColorTextBright).
		Background(ColorOverlay).
		Render(plain)
}

func (a *App) manageFieldMutationBlocked(field manageField) bool {
	if field.readOnlyReason == "" {
		return false
	}
	a.manageStatus = field.label + " is read-only: " + field.readOnlyReason
	return true
}

func (a *App) manageStartEditing(field manageField) {
	if a.manageFieldMutationBlocked(field) {
		return
	}
	if field.kind != manageFieldText && field.kind != manageFieldNumber {
		return
	}
	a.manageEditing = true
	a.manageEditField = nil
	a.manageEditNumber = nil
	a.manageEditValidate = nil
	if field.kind == manageFieldText {
		if field.str == nil {
			return
		}
		a.manageEditField = field.str
		a.manageEditValidate = field.validateText
		a.manageEditValue = *field.str
	} else {
		if field.n == nil {
			return
		}
		a.manageEditNumber = field.n
		a.manageEditMin = field.min
		a.manageEditMax = field.max
		a.manageEditValue = strconv.Itoa(*field.n)
	}
	a.manageEditFieldKey = field.label
	a.manageEditCursor = utf8.RuneCountInString(a.manageEditValue)
}

func (a *App) manageCommitEditing() bool {
	if !a.manageEditing {
		return false
	}
	if a.manageEditField != nil {
		if a.manageEditValidate != nil {
			if err := a.manageEditValidate(a.manageEditValue); err != nil {
				a.manageStatus = err.Error()
				return false
			}
		}
		*a.manageEditField = a.manageEditValue
	} else if a.manageEditNumber != nil {
		n, err := strconv.ParseInt(strings.TrimSpace(a.manageEditValue), 10, strconv.IntSize)
		if err != nil || n < int64(a.manageEditMin) || n > int64(a.manageEditMax) {
			a.manageStatus = "Enter a valid value within the field range"
			return false
		}
		*a.manageEditNumber = int(n)
	} else {
		return false
	}
	a.manageEditing = false
	a.manageEditField = nil
	a.manageEditNumber = nil
	a.manageEditFieldKey = ""
	a.manageEditValidate = nil
	return true
}

func (a *App) manageCancelEditing() {
	a.manageEditing = false
	a.manageEditField = nil
	a.manageEditNumber = nil
	a.manageEditFieldKey = ""
	a.manageEditValidate = nil
	a.manageEditValue = ""
	a.manageEditCursor = 0
}

// Utility helpers local to this file.

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cycleStringOption(opts []string, current string, forward bool) string {
	if len(opts) == 0 {
		return current
	}
	for i, o := range opts {
		if o == current {
			if forward {
				return opts[(i+1)%len(opts)]
			}
			return opts[(i-1+len(opts))%len(opts)]
		}
	}
	return opts[0]
}

// truncateVisible truncates a string to a visible width, being conservative with ANSI sequences.
// We use lipgloss.Width which accounts for ANSI, and rune-based slicing as a best-effort.
func truncateVisible(s string, width int) string {
	if width <= 0 {
		return ""
	}
	// ANSI-safe truncation (won't break escape sequences).
	return ansi.Truncate(s, width, "…")
}

func sanitizeLogLine(s string) string {
	stripped := ansi.Strip(s)
	var b strings.Builder
	b.Grow(len(stripped))
	for _, r := range stripped {
		switch {
		case r == '\t':
			b.WriteRune(' ')
		case r < 0x20:
			continue
		case r == 0x7f:
			continue
		case r >= 0x80 && r <= 0x9f:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func truncatePlain(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}
