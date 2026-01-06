package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/tools"
)

// ManageConfig holds detailed management configuration for all tools
type ManageConfig struct {
	// Ghostty detailed settings
	GhosttyFontFamily        string
	GhosttyFontSize          int
	GhosttyOpacity           int
	GhosttyBlurRadius        int
	GhosstyCursorStyle       string
	GhosttyScrollbackLines   int
	GhosttyWindowDecorations bool
	GhosttyConfirmClose      bool

	// Tmux detailed settings
	TmuxPrefix           string
	TmuxBaseIndex        int
	TmuxMouseMode        bool
	TmuxStatusPosition   string
	TmuxPaneBorderStyle  string
	TmuxHistoryLimit     int
	TmuxEscapeTime       int
	TmuxAggressiveResize bool

	// Tmux TPM settings
	TmuxTPMEnabled       bool
	TmuxPluginSensible   bool
	TmuxPluginResurrect  bool
	TmuxPluginContinuum  bool
	TmuxPluginYank       bool
	TmuxContinuumSaveMin int
	TmuxContinuumRestore bool

	// Zsh detailed settings
	ZshHistorySize       int
	ZshHistoryIgnoreDups bool
	ZshAutoCD            bool
	ZshCorrection        bool
	ZshCompletionMenu    bool
	ZshSyntaxHighlight   bool
	ZshAutosuggestions   bool

	// Neovim detailed settings
	NeovimLineNumbers string
	NeovimRelativeNum bool
	NeovimTabWidth    int
	NeovimExpandTab   bool
	NeovimWrap        bool
	NeovimCursorLine  bool
	NeovimClipboard   string
	NeovimUndoFile    bool

	// Git detailed settings
	GitDefaultBranch    string
	GitAutoSetupRemote  bool
	GitPullRebase       bool
	GitDiffTool         string
	GitMergeTool        string
	GitCredentialHelper string
	GitSignCommits      bool

	// Yazi detailed settings
	YaziShowHidden  bool
	YaziSortBy      string
	YaziSortReverse bool
	YaziLineMode    string
	YaziScrollOff   int

	// FZF detailed settings
	FzfDefaultOpts   string
	FzfHeight        int
	FzfLayout        string
	FzfBorderStyle   string
	FzfPreview       bool
	FzfPreviewWindow string

	// LazyGit detailed settings
	LazyGitSideBySide bool
	LazyGitPaging     string
	LazyGitMouseMode  bool
	LazyGitGuiTheme   string

	// LazyDocker detailed settings
	LazyDockerMouseMode bool
	LazyDockerLogsTail  int

	// Btop detailed settings
	BtopTheme       string
	BtopUpdateMs    int
	BtopShowTemp    bool
	BtopTempScale   string
	BtopGraphSymbol string
	BtopShownBoxes  string

	// Glow detailed settings
	GlowStyle string
	GlowPager string
	GlowWidth int
	GlowMouse bool
}

// NewManageConfig creates a new management config with defaults
func NewManageConfig() *ManageConfig {
	return &ManageConfig{
		// Ghostty
		GhosttyFontFamily:        "JetBrainsMono Nerd Font",
		GhosttyFontSize:          14,
		GhosttyOpacity:           100,
		GhosttyBlurRadius:        0,
		GhosstyCursorStyle:       "block",
		GhosttyScrollbackLines:   10000,
		GhosttyWindowDecorations: true,
		GhosttyConfirmClose:      true,

		// Tmux
		TmuxPrefix:           "C-a",
		TmuxBaseIndex:        1,
		TmuxMouseMode:        true,
		TmuxStatusPosition:   "bottom",
		TmuxPaneBorderStyle:  "single",
		TmuxHistoryLimit:     50000,
		TmuxEscapeTime:       10,
		TmuxAggressiveResize: true,

		// Tmux TPM
		TmuxTPMEnabled:       true,
		TmuxPluginSensible:   true,
		TmuxPluginResurrect:  true,
		TmuxPluginContinuum:  false,
		TmuxPluginYank:       true,
		TmuxContinuumSaveMin: 15,
		TmuxContinuumRestore: true,

		// Zsh
		ZshHistorySize:       10000,
		ZshHistoryIgnoreDups: true,
		ZshAutoCD:            true,
		ZshCorrection:        true,
		ZshCompletionMenu:    true,
		ZshSyntaxHighlight:   true,
		ZshAutosuggestions:   true,

		// Neovim
		NeovimLineNumbers: "absolute",
		NeovimRelativeNum: true,
		NeovimTabWidth:    4,
		NeovimExpandTab:   true,
		NeovimWrap:        false,
		NeovimCursorLine:  true,
		NeovimClipboard:   "unnamedplus",
		NeovimUndoFile:    true,

		// Git
		GitDefaultBranch:    "main",
		GitAutoSetupRemote:  true,
		GitPullRebase:       true,
		GitDiffTool:         "delta",
		GitMergeTool:        "vimdiff",
		GitCredentialHelper: "store",
		GitSignCommits:      false,

		// Yazi
		YaziShowHidden:  false,
		YaziSortBy:      "alphabetical",
		YaziSortReverse: false,
		YaziLineMode:    "size",
		YaziScrollOff:   5,

		// FZF
		FzfDefaultOpts:   "",
		FzfHeight:        40,
		FzfLayout:        "reverse",
		FzfBorderStyle:   "rounded",
		FzfPreview:       true,
		FzfPreviewWindow: "right:50%",

		// LazyGit
		LazyGitSideBySide: true,
		LazyGitPaging:     "delta",
		LazyGitMouseMode:  true,
		LazyGitGuiTheme:   "auto",

		// LazyDocker
		LazyDockerMouseMode: true,
		LazyDockerLogsTail:  100,

		// Btop
		BtopTheme:       "auto",
		BtopUpdateMs:    2000,
		BtopShowTemp:    true,
		BtopTempScale:   "celsius",
		BtopGraphSymbol: "braille",
		BtopShownBoxes:  "cpu mem net proc",

		// Glow
		GlowStyle: "auto",
		GlowPager: "less",
		GlowWidth: 80,
		GlowMouse: true,
	}
}

// manageTool represents a tool in the manage list
type manageTool struct {
	id     string
	name   string
	icon   string
	screen Screen
}

// getManageTools returns the list of manageable tools
func getManageTools() []manageTool {
	return []manageTool{
		{"ghostty", "Ghostty", "", ScreenManageGhostty},
		{"tmux", "Tmux", "", ScreenManageTmux},
		{"zsh", "Zsh", "", ScreenManageZsh},
		{"neovim", "Neovim", "", ScreenManageNeovim},
		{"git", "Git", "", ScreenManageGit},
		{"yazi", "Yazi", "󰉋", ScreenManageYazi},
		{"fzf", "FZF", "", ScreenManageFzf},
		{"lazygit", "LazyGit", "", ScreenManageLazyGit},
		{"lazydocker", "LazyDocker", "", ScreenManageLazyDocker},
		{"btop", "Btop", "", ScreenManageBtop},
		{"glow", "Glow", "", ScreenManageGlow},
	}
}

// renderManageDetailed renders the detailed manage screen with tool selection
func (a *App) renderManageDetailed() string {
	registry := tools.NewRegistry()
	manageTools := getManageTools()

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa"))
	title := titleStyle.Render("  Manage Tools")

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render(fmt.Sprintf("%d tools available • Select to configure", len(manageTools)))

	var lines []string

	for i, mt := range manageTools {
		tool, exists := registry.Get(mt.id)

		cursor := "  "
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

		if i == a.manageIndex {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("▸ ")
			nameStyle = nameStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
		}

		status := "○ Not installed"
		if exists && tool.IsInstalled() {
			status = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("● Installed")
		}

		line := fmt.Sprintf("%s%s %s  %s",
			cursor,
			mt.icon,
			nameStyle.Render(fmt.Sprintf("%-12s", mt.name)),
			statusStyle.Render(status))
		lines = append(lines, line)
	}

	toolList := strings.Join(lines, "\n")

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • Enter Configure • Esc Back • q Quit")

	content := fmt.Sprintf("\n\n%s\n%s\n\n%s\n\n%s",
		title, subtitle, toolList, footer)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// Helper for management config screens
func renderManageTitle(icon, name, desc string) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Italic(true)

	return lipgloss.JoinVertical(lipgloss.Center,
		titleStyle.Render(fmt.Sprintf("%s  %s Configuration", icon, name)),
		descStyle.Render(desc),
	)
}

func renderManageField(label string, value string, focused bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4")).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

	cursor := "  "
	if focused {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("▸ ")
		labelStyle = labelStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
		valueStyle = valueStyle.Foreground(lipgloss.Color("#89b4fa"))
	}

	return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(label), valueStyle.Render(value))
}

func renderManageToggle(label string, value bool, focused bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4")).Width(20)

	cursor := "  "
	if focused {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("▸ ")
		labelStyle = labelStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
	}

	toggle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Render("○ OFF")
	if value {
		toggle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("● ON")
	}

	return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(label), toggle)
}

func renderManageNumber(label string, value int, unit string, focused bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4")).Width(20)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

	cursor := "  "
	if focused {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("▸ ")
		labelStyle = labelStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
		valueStyle = valueStyle.Foreground(lipgloss.Color("#89b4fa"))
	}

	return fmt.Sprintf("%s%s ◀ %s ▶", cursor, labelStyle.Render(label), valueStyle.Render(fmt.Sprintf("%d%s", value, unit)))
}

func renderManageOption(label string, value string, options []string, focused bool) string {
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4")).Width(20)

	cursor := "  "
	if focused {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("▸ ")
		labelStyle = labelStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
	}

	var parts []string
	for _, opt := range options {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))
		if opt == value {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true)
		}
		parts = append(parts, style.Render(opt))
	}

	return fmt.Sprintf("%s%s %s", cursor, labelStyle.Render(label), strings.Join(parts, " │ "))
}

// =====================================
// Ghostty Management Config
// =====================================

func (a *App) renderManageGhostty() string {
	title := renderManageTitle("", "Ghostty", "GPU-accelerated terminal emulator")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageField("Font Family", cfg.GhosttyFontFamily, a.configFieldIndex == 0))
	lines = append(lines, renderManageNumber("Font Size", cfg.GhosttyFontSize, "pt", a.configFieldIndex == 1))
	lines = append(lines, renderManageNumber("Opacity", cfg.GhosttyOpacity, "%", a.configFieldIndex == 2))
	lines = append(lines, renderManageNumber("Blur Radius", cfg.GhosttyBlurRadius, "", a.configFieldIndex == 3))
	lines = append(lines, renderManageOption("Cursor Style", cfg.GhosstyCursorStyle, []string{"block", "bar", "underline"}, a.configFieldIndex == 4))
	lines = append(lines, renderManageNumber("Scrollback", cfg.GhosttyScrollbackLines, " lines", a.configFieldIndex == 5))
	lines = append(lines, renderManageToggle("Window Decorations", cfg.GhosttyWindowDecorations, a.configFieldIndex == 6))
	lines = append(lines, renderManageToggle("Confirm Close", cfg.GhosttyConfirmClose, a.configFieldIndex == 7))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(60).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// Tmux Management Config
// =====================================

func (a *App) renderManageTmux() string {
	title := renderManageTitle("", "Tmux", "Terminal multiplexer")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageOption("Prefix Key", cfg.TmuxPrefix, []string{"C-a", "C-b", "C-Space"}, a.configFieldIndex == 0))
	lines = append(lines, renderManageNumber("Base Index", cfg.TmuxBaseIndex, "", a.configFieldIndex == 1))
	lines = append(lines, renderManageToggle("Mouse Mode", cfg.TmuxMouseMode, a.configFieldIndex == 2))
	lines = append(lines, renderManageOption("Status Position", cfg.TmuxStatusPosition, []string{"top", "bottom"}, a.configFieldIndex == 3))
	lines = append(lines, renderManageOption("Pane Border", cfg.TmuxPaneBorderStyle, []string{"single", "double", "heavy", "simple"}, a.configFieldIndex == 4))
	lines = append(lines, renderManageNumber("History Limit", cfg.TmuxHistoryLimit, " lines", a.configFieldIndex == 5))
	lines = append(lines, renderManageNumber("Escape Time", cfg.TmuxEscapeTime, "ms", a.configFieldIndex == 6))
	lines = append(lines, renderManageToggle("Aggressive Resize", cfg.TmuxAggressiveResize, a.configFieldIndex == 7))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(65).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// Zsh Management Config
// =====================================

func (a *App) renderManageZsh() string {
	title := renderManageTitle("", "Zsh", "Z shell configuration")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageNumber("History Size", cfg.ZshHistorySize, " entries", a.configFieldIndex == 0))
	lines = append(lines, renderManageToggle("Ignore Duplicates", cfg.ZshHistoryIgnoreDups, a.configFieldIndex == 1))
	lines = append(lines, renderManageToggle("Auto CD", cfg.ZshAutoCD, a.configFieldIndex == 2))
	lines = append(lines, renderManageToggle("Auto Correction", cfg.ZshCorrection, a.configFieldIndex == 3))
	lines = append(lines, renderManageToggle("Completion Menu", cfg.ZshCompletionMenu, a.configFieldIndex == 4))
	lines = append(lines, renderManageToggle("Syntax Highlighting", cfg.ZshSyntaxHighlight, a.configFieldIndex == 5))
	lines = append(lines, renderManageToggle("Auto Suggestions", cfg.ZshAutosuggestions, a.configFieldIndex == 6))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(55).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// Neovim Management Config
// =====================================

func (a *App) renderManageNeovim() string {
	title := renderManageTitle("", "Neovim", "Hyperextensible text editor")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageOption("Line Numbers", cfg.NeovimLineNumbers, []string{"absolute", "relative", "none"}, a.configFieldIndex == 0))
	lines = append(lines, renderManageToggle("Relative Numbers", cfg.NeovimRelativeNum, a.configFieldIndex == 1))
	lines = append(lines, renderManageNumber("Tab Width", cfg.NeovimTabWidth, " spaces", a.configFieldIndex == 2))
	lines = append(lines, renderManageToggle("Expand Tab", cfg.NeovimExpandTab, a.configFieldIndex == 3))
	lines = append(lines, renderManageToggle("Line Wrap", cfg.NeovimWrap, a.configFieldIndex == 4))
	lines = append(lines, renderManageToggle("Cursor Line", cfg.NeovimCursorLine, a.configFieldIndex == 5))
	lines = append(lines, renderManageOption("Clipboard", cfg.NeovimClipboard, []string{"unnamedplus", "unnamed", "none"}, a.configFieldIndex == 6))
	lines = append(lines, renderManageToggle("Undo File", cfg.NeovimUndoFile, a.configFieldIndex == 7))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(65).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// Git Management Config
// =====================================

func (a *App) renderManageGit() string {
	title := renderManageTitle("", "Git", "Version control configuration")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageOption("Default Branch", cfg.GitDefaultBranch, []string{"main", "master", "develop"}, a.configFieldIndex == 0))
	lines = append(lines, renderManageToggle("Auto Setup Remote", cfg.GitAutoSetupRemote, a.configFieldIndex == 1))
	lines = append(lines, renderManageToggle("Pull Rebase", cfg.GitPullRebase, a.configFieldIndex == 2))
	lines = append(lines, renderManageOption("Diff Tool", cfg.GitDiffTool, []string{"delta", "difftastic", "vimdiff"}, a.configFieldIndex == 3))
	lines = append(lines, renderManageOption("Merge Tool", cfg.GitMergeTool, []string{"vimdiff", "nvimdiff", "meld"}, a.configFieldIndex == 4))
	lines = append(lines, renderManageOption("Credential Helper", cfg.GitCredentialHelper, []string{"store", "cache", "osxkeychain"}, a.configFieldIndex == 5))
	lines = append(lines, renderManageToggle("Sign Commits", cfg.GitSignCommits, a.configFieldIndex == 6))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(65).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// Yazi Management Config
// =====================================

func (a *App) renderManageYazi() string {
	title := renderManageTitle("󰉋", "Yazi", "Terminal file manager")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageToggle("Show Hidden", cfg.YaziShowHidden, a.configFieldIndex == 0))
	lines = append(lines, renderManageOption("Sort By", cfg.YaziSortBy, []string{"alphabetical", "modified", "size", "natural"}, a.configFieldIndex == 1))
	lines = append(lines, renderManageToggle("Sort Reverse", cfg.YaziSortReverse, a.configFieldIndex == 2))
	lines = append(lines, renderManageOption("Line Mode", cfg.YaziLineMode, []string{"size", "permissions", "mtime", "none"}, a.configFieldIndex == 3))
	lines = append(lines, renderManageNumber("Scroll Offset", cfg.YaziScrollOff, " lines", a.configFieldIndex == 4))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(65).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// FZF Management Config
// =====================================

func (a *App) renderManageFzf() string {
	title := renderManageTitle("", "FZF", "Fuzzy finder configuration")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageNumber("Height", cfg.FzfHeight, "%", a.configFieldIndex == 0))
	lines = append(lines, renderManageOption("Layout", cfg.FzfLayout, []string{"reverse", "default", "reverse-list"}, a.configFieldIndex == 1))
	lines = append(lines, renderManageOption("Border Style", cfg.FzfBorderStyle, []string{"rounded", "sharp", "bold", "none"}, a.configFieldIndex == 2))
	lines = append(lines, renderManageToggle("Preview", cfg.FzfPreview, a.configFieldIndex == 3))
	lines = append(lines, renderManageOption("Preview Window", cfg.FzfPreviewWindow, []string{"right:50%", "up:50%", "down:50%"}, a.configFieldIndex == 4))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(65).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// LazyGit Management Config
// =====================================

func (a *App) renderManageLazyGit() string {
	title := renderManageTitle("", "LazyGit", "Terminal UI for Git")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageToggle("Side-by-Side Diff", cfg.LazyGitSideBySide, a.configFieldIndex == 0))
	lines = append(lines, renderManageOption("Paging", cfg.LazyGitPaging, []string{"delta", "diff-so-fancy", "never"}, a.configFieldIndex == 1))
	lines = append(lines, renderManageToggle("Mouse Mode", cfg.LazyGitMouseMode, a.configFieldIndex == 2))
	lines = append(lines, renderManageOption("GUI Theme", cfg.LazyGitGuiTheme, []string{"auto", "light", "dark"}, a.configFieldIndex == 3))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(60).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// LazyDocker Management Config
// =====================================

func (a *App) renderManageLazyDocker() string {
	title := renderManageTitle("", "LazyDocker", "Terminal UI for Docker")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageToggle("Mouse Mode", cfg.LazyDockerMouseMode, a.configFieldIndex == 0))
	lines = append(lines, renderManageNumber("Logs Tail", cfg.LazyDockerLogsTail, " lines", a.configFieldIndex == 1))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(55).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// Btop Management Config
// =====================================

func (a *App) renderManageBtop() string {
	title := renderManageTitle("", "Btop", "System resource monitor")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageOption("Theme", cfg.BtopTheme, []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}, a.configFieldIndex == 0))
	lines = append(lines, renderManageNumber("Update Rate", cfg.BtopUpdateMs, "ms", a.configFieldIndex == 1))
	lines = append(lines, renderManageToggle("Show Temp", cfg.BtopShowTemp, a.configFieldIndex == 2))
	lines = append(lines, renderManageOption("Temp Scale", cfg.BtopTempScale, []string{"celsius", "fahrenheit"}, a.configFieldIndex == 3))
	lines = append(lines, renderManageOption("Graph Symbol", cfg.BtopGraphSymbol, []string{"braille", "block", "tty"}, a.configFieldIndex == 4))
	lines = append(lines, renderManageField("Shown Boxes", cfg.BtopShownBoxes, a.configFieldIndex == 5))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(70).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}

// =====================================
// Glow Management Config
// =====================================

func (a *App) renderManageGlow() string {
	title := renderManageTitle("", "Glow", "Markdown renderer")
	cfg := a.manageConfig

	var lines []string
	lines = append(lines, renderManageOption("Style", cfg.GlowStyle, []string{"auto", "dark", "light", "notty"}, a.configFieldIndex == 0))
	lines = append(lines, renderManageOption("Pager", cfg.GlowPager, []string{"less", "more", "none"}, a.configFieldIndex == 1))
	lines = append(lines, renderManageNumber("Width", cfg.GlowWidth, " chars", a.configFieldIndex == 2))
	lines = append(lines, renderManageToggle("Mouse", cfg.GlowMouse, a.configFieldIndex == 3))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#45475a")).
		Padding(1, 2).
		Width(55).
		Render(content)

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • ←→ Adjust • Space Toggle • Esc Back")

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, "", box, "", help))
}
