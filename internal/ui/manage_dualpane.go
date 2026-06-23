package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
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
	options []string

	// For numeric fields (kind == manageFieldNumber).
	min  int
	max  int
	step int
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
}

// manageSavedMsg is emitted after a save attempt.
type manageSavedMsg struct{ err error }

// manageInstallDoneMsg is emitted after attempting to install a tool/app.
type manageInstallDoneMsg struct {
	toolID string
	err    error
}

func (a *App) saveManageConfigCmd() tea.Cmd {
	// Capture by value (pointer is stable) and run file I/O in a command.
	cfg := a.manageConfig
	theme := a.theme
	nav := a.navStyle
	animationsEnabled := a.animationsEnabled

	// Compute the set of tools to apply BEFORE the async closure runs, by diffing
	// the live config against the baseline captured at load / last save. Scoping
	// the apply to only the changed tools is the data-loss fix (P1-A2): a
	// Ghostty-only edit must not rewrite ~/.tmux.conf, ~/.zshrc, ~/.gitconfig, etc.
	// from manage.json defaults (overwriting any hand edits). A theme change is
	// cross-cutting (all generated colors depend on it) and intentionally
	// re-applies every tool — see changedManageTools.
	baseline := a.manageConfigBaseline
	changed := changedManageTools(&baseline, cfg, a.manageConfigBaselineTheme, theme)

	return func() tea.Msg {
		if err := config.SaveToolConfig("manage", cfg); err != nil {
			return manageSavedMsg{err: err}
		}

		// Also persist global theme/nav so installer + CLI stay in sync.
		g, err := config.LoadGlobalConfig()
		if err != nil {
			g = config.DefaultGlobalConfig()
		}
		g.Theme = theme
		g.NavStyle = nav
		g.DisableAnimations = !animationsEnabled

		if err := config.SaveGlobalConfig(g); err != nil {
			return manageSavedMsg{err: err}
		}

		// Apply the saved preferences to the REAL tool config files (C12), but ONLY
		// for the tools the user actually changed. Before C12 the Manage editor only
		// persisted manage.json + global prefs and claimed "Saved ✓" while no
		// generator ever ran; the first C12 pass over-corrected by re-applying ALL
		// tools every save (clobbering unrelated configs). This scoped apply funnels
		// through applyOneToolConfig — the same scoped writer the standalone
		// `dotfiles config <tool>` editor uses — so the two paths cannot drift, and
		// it is a PURE file write (no TPM/Neovim clone; that stays at install time).
		if errs := applyChangedManageTools(changed, manageConfigToDeepDive(cfg), theme); len(errs) > 0 {
			return manageSavedMsg{err: firstErrorSummary(errs)}
		}

		return manageSavedMsg{err: nil}
	}
}

// checkSudoAndInstallCmd checks if sudo is needed and either prompts or starts install.
func (a *App) checkSudoAndInstallCmd(toolID string) tea.Cmd {
	return func() tea.Msg {
		mgr := pkg.DetectManager()
		if mgr == nil {
			return manageInstallDoneMsg{toolID: toolID, err: fmt.Errorf("no package manager detected")}
		}

		// Check if sudo is needed and not cached
		if mgr.NeedsSudo() && !runner.CheckSudoCached() {
			return manageSudoRequiredMsg{toolID: toolID}
		}

		// Sudo not needed or already cached - start streaming install
		return manageStartInstallMsg{toolID: toolID}
	}
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

	return manageLayout{
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
	reg := tools.GetRegistry()
	all := reg.All()
	platform := pkg.DetectPlatform()

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

	// Filter by platform support: hide tools/apps that can't be installed on this
	// OS, but keep anything already installed.
	//
	// This is especially important for GUI apps: don't show macOS-only apps on
	// Linux and vice versa.
	if platform != pkg.PlatformUnknown {
		filtered := make([]tools.Tool, 0, len(all))
		for _, t := range all {
			installed := a.manageInstalled[t.ID()]
			supported := toolHasPackagesForPlatform(t, platform)
			if installed || supported {
				filtered = append(filtered, t)
			}
		}
		all = filtered
	}

	// Add a global section at the top.
	items := []manageItem{
		{
			id:           manageItemGlobal,
			name:         "Global",
			icon:         "󰒓",
			description:  "UI + platform preferences",
			category:     manageItemGlobal,
			installed:    true,
			configurable: true,
		},
	}

	for _, t := range all {
		icon := t.Icon()
		if icon == "" {
			icon = fallbackToolIcon(t.ID(), t.Category())
		}

		items = append(items, manageItem{
			id:           t.ID(),
			name:         t.Name(),
			icon:         icon,
			description:  t.Description(),
			category:     t.Category(),
			installed:    a.manageInstalled[t.ID()],
			configurable: t.HasConfig(),
		})
	}

	return items
}

func toolHasPackagesForPlatform(t tools.Tool, platform pkg.Platform) bool {
	pkgs := t.Packages()[platform]
	if len(pkgs) == 0 {
		pkgs = t.Packages()["all"]
	}
	return len(pkgs) > 0
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

	// ID-based fallback (for unknown categories like manageItemGlobal).
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
	case manageItemGlobal:
		return []manageField{
			{
				key:         manageFieldTheme,
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
				options:     []string{navEmacs, navVim},
			},
			{
				key:         manageFieldAnims,
				label:       "Animations",
				description: "Enable animated UI elements (headers, globe, spinners)",
				kind:        manageFieldToggle,
				b:           &a.animationsEnabled,
			},
		}

	case toolGhostty:
		return []manageField{
			{key: "font_family", label: "Font Family", description: "Terminal font family", kind: manageFieldText, str: &cfg.GhosttyFontFamily},
			{key: "font_size", label: "Font Size", description: "Font size (pt)", kind: manageFieldNumber, n: &cfg.GhosttyFontSize, min: 8, max: 32, step: 1, unit: "pt"},
			{key: "opacity", label: "Opacity", description: "Background opacity (%)", kind: manageFieldNumber, n: &cfg.GhosttyOpacity, min: 0, max: 100, step: 5, unit: "%"},
			{key: "blur", label: "Blur Radius", description: "Background blur (platform dependent)", kind: manageFieldNumber, n: &cfg.GhosttyBlurRadius, min: 0, max: 40, step: 1},
			{key: "cursor", label: "Cursor Style", description: "Cursor shape", kind: manageFieldOption, str: &cfg.GhosstyCursorStyle, options: []string{"block", "bar", "underline"}},
			{key: "scrollback", label: "Scrollback", description: "Scrollback history lines", kind: manageFieldNumber, n: &cfg.GhosttyScrollbackLines, min: 1000, max: 200000, step: 1000, unit: " lines"},
			{key: "decor", label: "Window Decorations", description: "Show native window decorations", kind: manageFieldToggle, b: &cfg.GhosttyWindowDecorations},
			{key: "confirm_close", label: "Confirm Close", description: "Prompt before closing window", kind: manageFieldToggle, b: &cfg.GhosttyConfirmClose},
		}

	case toolTmux:
		return []manageField{
			{key: "prefix", label: "Prefix Key", description: "Leader key for tmux commands", kind: manageFieldOption, str: &cfg.TmuxPrefix, options: []string{"C-a", "C-b", "C-Space"}},
			{key: "base", label: "Base Index", description: "Start window/pane numbering at", kind: manageFieldNumber, n: &cfg.TmuxBaseIndex, min: 0, max: 10, step: 1},
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.TmuxMouseMode},
			{key: "status_pos", label: "Status Position", description: "Status bar placement", kind: manageFieldOption, str: &cfg.TmuxStatusPosition, options: []string{tmuxStatusTop, "bottom"}},
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

	case toolNeovim:
		return []manageField{
			// "numbers" is the single control: relative also enables relativenumber.
			{key: "numbers", label: "Line Numbers", description: "Absolute/relative/none (relative shows relativenumber)", kind: manageFieldOption, str: &cfg.NeovimLineNumbers, options: []string{"absolute", "relative", "none"}},
			{key: keyTab, label: "Tab Width", description: "Indent width", kind: manageFieldNumber, n: &cfg.NeovimTabWidth, min: 2, max: 8, step: 1, unit: " spaces"},
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
			{key: "diff", label: "Diff Tool", description: "Default diff tool", kind: manageFieldOption, str: &cfg.GitDiffTool, options: []string{"delta", "difftastic", "vimdiff"}},
			{key: "merge", label: "Merge Tool", description: "Default merge tool", kind: manageFieldOption, str: &cfg.GitMergeTool, options: []string{"vimdiff", "nvimdiff", "meld"}},
			{key: "creds", label: "Credential Helper", description: "Credential helper backend", kind: manageFieldOption, str: &cfg.GitCredentialHelper, options: []string{"store", "cache", "osxkeychain"}},
			{key: "sign", label: "Sign Commits", description: "Require signed commits", kind: manageFieldToggle, b: &cfg.GitSignCommits},
		}

	case "yazi":
		return []manageField{
			{key: "hidden", label: "Show Hidden", description: "Show dotfiles by default", kind: manageFieldToggle, b: &cfg.YaziShowHidden},
			{key: "sort_by", label: "Sort By", description: "Sort order", kind: manageFieldOption, str: &cfg.YaziSortBy, options: []string{"alphabetical", "modified", "size", "natural"}},
			{key: "sort_rev", label: "Sort Reverse", description: "Reverse sort direction", kind: manageFieldToggle, b: &cfg.YaziSortReverse},
			{key: "linemode", label: "Line Mode", description: "Line metadata style", kind: manageFieldOption, str: &cfg.YaziLineMode, options: []string{"size", "permissions", "mtime", "none"}},
			{key: "scrolloff", label: "Scroll Offset", description: "Keep N items visible above/below cursor", kind: manageFieldNumber, n: &cfg.YaziScrollOff, min: 0, max: 20, step: 1, unit: " lines"},
		}

	case "fzf":
		return []manageField{
			{key: "opts", label: "Default Opts", description: "Extra CLI options passed to fzf", kind: manageFieldText, str: &cfg.FzfDefaultOpts},
			{key: "height", label: "Height", description: "Height percentage for fzf UI", kind: manageFieldNumber, n: &cfg.FzfHeight, min: 20, max: 100, step: 5, unit: "%"},
			{key: "layout", label: "Layout", description: "Layout mode", kind: manageFieldOption, str: &cfg.FzfLayout, options: []string{"reverse", optionDefault, "reverse-list"}},
			{key: "border", label: "Border Style", description: "Border style for fzf window", kind: manageFieldOption, str: &cfg.FzfBorderStyle, options: []string{"rounded", "sharp", "bold", "none"}},
			{key: "preview", label: "Preview", description: "Enable preview pane", kind: manageFieldToggle, b: &cfg.FzfPreview},
			{key: "preview_window", label: "Preview Window", description: "Preview placement/size", kind: manageFieldOption, str: &cfg.FzfPreviewWindow, options: []string{"right:50%", "up:50%", "down:50%"}},
		}

	case toolLazygit:
		return []manageField{
			{key: "side", label: "Side-by-Side Diff", description: "Use side-by-side diffs", kind: manageFieldToggle, b: &cfg.LazyGitSideBySide},
			{key: "paging", label: "Paging", description: "Paging backend", kind: manageFieldOption, str: &cfg.LazyGitPaging, options: []string{"delta", "diff-so-fancy", optionNever}},
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.LazyGitMouseMode},
			{key: "gui_theme", label: "GUI Theme", description: "GUI theme selection", kind: manageFieldOption, str: &cfg.LazyGitGuiTheme, options: []string{"auto", "light", "dark"}},
		}

	case "lazydocker":
		return []manageField{
			{key: "mouse", label: "Mouse Mode", description: "Enable mouse interactions", kind: manageFieldToggle, b: &cfg.LazyDockerMouseMode},
			{key: "tail", label: "Logs Tail", description: "How many log lines to show", kind: manageFieldNumber, n: &cfg.LazyDockerLogsTail, min: 10, max: 2000, step: 10, unit: " lines"},
		}

	case toolBtop:
		return []manageField{
			{key: manageFieldTheme, label: "Theme", description: "btop theme name", kind: manageFieldOption, str: &cfg.BtopTheme, options: []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}},
			{key: "rate", label: "Update Rate", description: "Refresh interval", kind: manageFieldNumber, n: &cfg.BtopUpdateMs, min: 250, max: 10000, step: 250, unit: "ms"},
			{key: "temp", label: "Show Temp", description: "Show CPU temperature", kind: manageFieldToggle, b: &cfg.BtopShowTemp},
			{key: "scale", label: "Temp Scale", description: "Celsius/Fahrenheit", kind: manageFieldOption, str: &cfg.BtopTempScale, options: []string{"celsius", "fahrenheit"}},
			{key: "graph", label: "Graph Symbol", description: "Graph rendering symbol set", kind: manageFieldOption, str: &cfg.BtopGraphSymbol, options: []string{"braille", "block", "tty"}},
			{key: "boxes", label: "Shown Boxes", description: "Which panels to show", kind: manageFieldText, str: &cfg.BtopShownBoxes},
		}

	case "glow":
		return []manageField{
			{key: "style", label: "Style", description: "Style theme for Glow", kind: manageFieldOption, str: &cfg.GlowStyle, options: []string{"auto", "dark", "light", "notty"}},
			{key: "pager", label: "Pager", description: "Pager program", kind: manageFieldOption, str: &cfg.GlowPager, options: []string{"auto", "less", optionNever}},
			{key: "width", label: "Width", description: "Max render width", kind: manageFieldNumber, n: &cfg.GlowWidth, min: 40, max: 240, step: 5, unit: " chars"},
			{key: "mouse", label: "Mouse", description: "Enable mouse support in Glow", kind: manageFieldToggle, b: &cfg.GlowMouse},
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
	if a.animationsEnabled {
		subText = AnimatedSpinnerDots(a.uiFrame/2) + " " + subText
	}
	sub := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(truncateVisible(subText, width))

	divider := ShimmerDivider(maxInt(0, width), a.uiFrame, a.animationsEnabled)

	// Keep this exactly 3 lines (see manageLayout.headerH).
	return lipgloss.JoinVertical(lipgloss.Left, tabs, sub, divider)
}

func (a *App) renderManageFooter(width int, items []manageItem, fields []manageField) string {
	// Hint line: short and consistent.
	hints := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"Tab switch pane • ↑↓ move • ←→ adjust • Space toggle • Enter edit • I install • ? hotkeys • S save • Esc back • q quit",
	)

	// Status line: either save feedback, or focused field description.
	statusText := a.manageStatus
	if a.manageInstalling {
		name := a.manageInstallID
		for _, it := range items {
			if it.id == a.manageInstallID {
				name = it.name
				break
			}
		}
		if a.animationsEnabled {
			statusText = fmt.Sprintf("%s Installing %s…", AnimatedSpinnerDots(a.uiFrame), name)
		} else {
			statusText = fmt.Sprintf("Installing %s…", name)
		}
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
		if it.id == manageItemGlobal {
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
		if it.id != manageItemGlobal && !it.installed {
			nameStyle = lipgloss.NewStyle().Foreground(ColorTextMuted)
		}
		if focused {
			cursor = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render("▸ ")
			nameStyle = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
		}

		status := StatusDot(statusPending)
		if it.id == manageItemGlobal {
			status = lipgloss.NewStyle().Foreground(ColorCyan).Render(glyphDotFilled)
		} else if it.installed {
			status = StatusDot("success")
		}

		icon := it.icon
		if icon != "" {
			icon += " "
		}

		// Right-aligned category tag (helps scanning without changing selection mapping).
		cat := strings.ToUpper(string(it.category))
		if it.id == manageItemGlobal {
			cat = "GLOBAL"
		}
		tag := tagStyle.Render(cat)

		left := fmt.Sprintf("%s%s %s%s", cursor, status, icon, nameStyle.Render(it.name))
		// Small visual hint that settings exist.
		if it.id != manageItemGlobal && it.configurable {
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

	// If installing, show log panel instead of settings
	if a.manageInstalling || len(a.installLogs) > 0 {
		return a.renderManageLogPanel(layout, items)
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
	meta := renderManageSettingsMeta(item)

	innerW := maxInt(0, layout.rightW-(layout.border*2)-(layout.padX*2))

	// Field list lines (fixed height for stable layout).
	visibleFieldLines := layout.rightListH
	fieldCapacity := visibleFieldLines
	if a.manageEditing && a.manageEditField != nil && fieldCapacity > 0 {
		// Reserve the first line for the editor, but keep overall height stable.
		fieldCapacity--
	}

	var fieldLines []string
	if len(fields) == 0 {
		// No explicit fields for this tool. Show a helpful placeholder plus an
		// install hint.
		fieldLines = renderManageFieldPlaceholder(item)
	} else {
		fieldLines = a.renderManageFieldRows(fields, item, fieldCapacity, innerW)
	}
	for len(fieldLines) < fieldCapacity {
		fieldLines = append(fieldLines, "")
	}

	var fieldsBlock string
	if a.manageEditing && a.manageEditField != nil && visibleFieldLines > 0 {
		fieldsBlock = strings.Join(append([]string{a.renderManageInlineEditor(innerW)}, fieldLines...), "\n")
	} else {
		fieldsBlock = strings.Join(fieldLines, "\n")
	}

	// Exactly 3 header lines before the fields area (matches manageLayout.rightHeaderLines).
	actionLine := manageSettingsActionLine(item, fields)

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

// renderManageSettingsMeta builds the settings-panel meta header line: the tool
// name (with optional icon), its description, and an install-status badge.
func renderManageSettingsMeta(item manageItem) string {
	statusBadge := ""
	if item.id != manageItemGlobal {
		if item.installed {
			statusBadge = " " + RenderBadge("INSTALLED", ColorBg, ColorGreen)
		} else {
			statusBadge = " " + RenderBadge("NOT INSTALLED", ColorText, ColorMuted)
		}
	}
	metaName := item.name
	if item.icon != "" {
		metaName = item.icon + " " + metaName
	}
	return lipgloss.NewStyle().Foreground(ColorTextBright).Bold(true).Render(metaName) +
		lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  "+item.description) +
		statusBadge
}

// manageSettingsActionLine returns the action hint shown above the fields area
// (install hint for uninstalled tools, or a "no fields" note).
func manageSettingsActionLine(item manageItem, fields []manageField) string {
	if item.id != manageItemGlobal && !item.installed {
		return lipgloss.NewStyle().Foreground(ColorYellow).Render("I: install this tool/app")
	}
	if item.id != manageItemGlobal && len(fields) == 0 {
		return lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No editable fields in manager yet")
	}
	return ""
}

// renderManageFieldPlaceholder builds the placeholder field lines shown when a
// tool has no explicit manager fields (a status/help message plus, for tools,
// an install hint and the platform package names).
func renderManageFieldPlaceholder(item manageItem) []string {
	msgStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	strong := lipgloss.NewStyle().Foreground(ColorText).Bold(true)

	var fieldLines []string
	if item.id == manageItemGlobal {
		fieldLines = append(fieldLines, msgStyle.Render("No global settings available."))
		return fieldLines
	}

	if item.configurable {
		fieldLines = append(fieldLines, msgStyle.Render("No manager UI fields yet (tool has config)."))
	} else {
		fieldLines = append(fieldLines, msgStyle.Render("No configurable settings for this tool."))
	}

	if !item.installed {
		fieldLines = append(fieldLines, strong.Render("Press I to install"))
	} else {
		fieldLines = append(fieldLines, msgStyle.Render("Installed — press S to save global prefs"))
	}

	// Show package names for this platform (best-effort).
	if t, ok := tools.GetRegistry().Get(item.id); ok {
		platform := pkg.DetectPlatform()
		pkgs := t.Packages()[platform]
		if len(pkgs) == 0 {
			pkgs = t.Packages()["all"]
		}
		if len(pkgs) > 0 {
			pkgLine := msgStyle.Render("Packages: ") + strong.Render(strings.Join(pkgs, ", "))
			fieldLines = append(fieldLines, pkgLine)
		}
	}
	return fieldLines
}

// renderManageFieldRows renders the visible settings rows (from the current
// scroll offset, up to fieldCapacity), truncated to innerW.
func (a *App) renderManageFieldRows(fields []manageField, item manageItem, fieldCapacity, innerW int) []string {
	var fieldLines []string
	for i := a.manageFieldsScroll; i < len(fields) && len(fieldLines) < fieldCapacity; i++ {
		f := fields[i]
		focused := (a.managePane == managePaneSettings) && (i == a.configFieldIndex)
		applied := manageFieldIsApplied(item.id, f.key)
		fieldLines = append(fieldLines, truncateVisible(renderManageFieldLine(f, focused, applied), innerW))
	}
	return fieldLines
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
		val := valueStyle.Render(fmt.Sprintf("%d%s", *f.n, f.unit))
		return fmt.Sprintf("%s%s %s %s %s", cursor, labelStyle.Render(f.label), leftArrow, val, rightArrow)

	case manageFieldOption:
		if f.str == nil || len(f.options) == 0 {
			return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(f.label), valueStyle.Render("—"))
		}
		leftArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("◀")
		rightArrow := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("▶")
		if focused {
			leftArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("◀")
			rightArrow = lipgloss.NewStyle().Foreground(ColorCyan).Render("▶")
		}
		val := valueStyle.Render(*f.str)
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

func (a *App) manageStartEditing(field manageField) {
	if field.kind != manageFieldText || field.str == nil {
		return
	}

	a.manageEditing = true
	a.manageEditField = field.str
	a.manageEditFieldKey = field.label
	a.manageEditValue = *field.str
	a.manageEditCursor = utf8.RuneCountInString(a.manageEditValue)
}

func (a *App) manageCommitEditing() {
	if !a.manageEditing || a.manageEditField == nil {
		return
	}
	*a.manageEditField = a.manageEditValue
	a.manageEditing = false
	a.manageEditField = nil
	a.manageEditFieldKey = ""
}

func (a *App) manageCancelEditing() {
	a.manageEditing = false
	a.manageEditField = nil
	a.manageEditFieldKey = ""
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

// renderManageLogPanel renders the log panel when installing/updating.
func (a *App) renderManageLogPanel(layout manageLayout, items []manageItem) string {
	borderColor := ColorCyan
	if !a.manageInstalling {
		borderColor = ColorBorder
	}

	// Get the tool name for the title
	toolName := "Install"
	for _, it := range items {
		if it.id == a.manageInstallID {
			toolName = it.name
			break
		}
	}

	// Build title with status
	var title string
	if a.manageInstalling {
		spinner := AnimatedSpinnerDots(a.uiFrame)
		if !a.animationsEnabled {
			spinner = "..."
		}
		title = fmt.Sprintf("INSTALLING %s %s", strings.ToUpper(toolName), spinner)
	} else {
		title = fmt.Sprintf("INSTALL LOG: %s", strings.ToUpper(toolName))
	}

	// Build the styled log panel to match the dual-pane layout.
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		Width(maxInt(1, layout.rightW-2)).
		Height(maxInt(1, layout.bodyH-2))

	// Build content
	innerWidth := maxInt(0, layout.rightW-4)
	innerHeight := maxInt(0, layout.bodyH-6)

	// Title line
	titleStyle := lipgloss.NewStyle().Foreground(ColorNeonPink).Bold(true)
	titleLine := titleStyle.Render(title)

	// Calculate visible log range
	visibleLines := innerHeight
	totalLines := len(a.installLogs)

	var logLines []string
	if totalLines == 0 {
		// Empty state
		if a.manageInstalling {
			logLines = append(logLines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Waiting for output..."))
		} else {
			logLines = append(logLines, lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No logs"))
		}
	} else {
		// Calculate range (scroll from bottom)
		endIdx := totalLines - a.installLogScroll
		if endIdx > totalLines {
			endIdx = totalLines
		}
		if endIdx < 0 {
			endIdx = 0
		}
		startIdx := endIdx - visibleLines
		if startIdx < 0 {
			startIdx = 0
		}

		for i := startIdx; i < endIdx; i++ {
			line := a.installLogs[i]
			if lipgloss.Width(line) > innerWidth {
				line = truncateVisible(line, innerWidth)
			}
			logLines = append(logLines, line)
		}
	}

	// Pad to fill height
	for len(logLines) < visibleLines {
		logLines = append([]string{""}, logLines...)
	}

	// Footer with hints
	var footerText string
	if a.manageInstalling {
		footerText = "Installing..."
	} else if len(a.installLogs) > 0 {
		footerText = "C: clear • ↑↓: scroll"
	}
	footer := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(footerText)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		titleLine,
		"",
		strings.Join(logLines, "\n"),
		"",
		footer,
	)

	return panel.Render(content)
}
