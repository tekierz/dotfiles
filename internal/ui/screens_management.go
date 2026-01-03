package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

// =====================================
// Main Menu Screen
// =====================================

func (a *App) renderMainMenu() string {
	items := GetMainMenuItems()

	// Title with gradient
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa")).
		Render("  Dotfiles Management")

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("Terminal environment management platform")

	// Menu items
	var menuLines []string
	for i, item := range items {
		cursor := "  "
		itemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

		if i == a.mainMenuIndex {
			cursor = " "
			itemStyle = itemStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
			descStyle = descStyle.Foreground(lipgloss.Color("#94e2d5"))
		}

		line := fmt.Sprintf("%s%s %s  %s",
			cursor,
			item.Icon,
			itemStyle.Render(item.Name),
			descStyle.Render(item.Description))
		menuLines = append(menuLines, line)
	}

	menu := strings.Join(menuLines, "\n")

	// Footer hints
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • Enter Select • q Quit")

	// Center everything
	content := fmt.Sprintf("\n\n%s\n%s\n\n%s\n\n%s",
		title, subtitle, menu, footer)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// =====================================
// Update Screen
// =====================================

// updatePackage represents a package that can be updated
type updatePackage struct {
	name           string
	currentVersion string
	latestVersion  string
	selected       bool
}

func (a *App) renderUpdate() string {
	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa")).
		Render("  Package Updates")

	// Get outdated packages
	mgr := pkg.DetectManager()
	if mgr == nil {
		content := fmt.Sprintf("\n\n%s\n\n%s\n\n%s",
			title,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Render("No package manager detected"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("q Quit • Esc Back"))
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
	}

	updates, err := pkg.CheckDotfilesUpdates()
	if err != nil {
		content := fmt.Sprintf("\n\n%s\n\n%s\n\n%s",
			title,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Render(fmt.Sprintf("Error: %v", err)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("q Quit • Esc Back"))
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
	}

	if len(updates) == 0 {
		content := fmt.Sprintf("\n\n%s\n\n%s\n\n%s",
			title,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render(" All packages are up to date!"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("q Quit • Esc Back"))
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
	}

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render(fmt.Sprintf("Found %d outdated package(s)", len(updates)))

	// Package list
	var pkgLines []string
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true)
	pkgLines = append(pkgLines, headerStyle.Render(fmt.Sprintf("  %-25s %-12s %-12s", "PACKAGE", "CURRENT", "LATEST")))
	pkgLines = append(pkgLines, headerStyle.Render(fmt.Sprintf("  %-25s %-12s %-12s", strings.Repeat("─", 25), strings.Repeat("─", 12), strings.Repeat("─", 12))))

	for i, p := range updates {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
		versionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387"))
		newStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))

		if i == a.updateIndex {
			cursor = " "
			style = style.Bold(true)
		}

		line := fmt.Sprintf("%s%-25s %s → %s",
			cursor,
			style.Render(p.Name),
			versionStyle.Render(p.CurrentVersion),
			newStyle.Render(p.LatestVersion))
		pkgLines = append(pkgLines, line)
	}

	packageList := strings.Join(pkgLines, "\n")

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • Enter Update All • Esc Back • q Quit")

	content := fmt.Sprintf("\n\n%s\n%s\n\n%s\n\n%s",
		title, subtitle, packageList, footer)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// =====================================
// Hotkeys Screen
// =====================================

// HotkeyCategory holds hotkeys for a tool
type HotkeyCategory struct {
	Name    string
	Icon    string
	Hotkeys []Hotkey
}

// Hotkey represents a single keyboard shortcut
type Hotkey struct {
	Keys        string
	Description string
}

// GetHotkeyCategories returns all hotkey categories
func GetHotkeyCategories() []HotkeyCategory {
	return []HotkeyCategory{
		{
			Name: "Tmux",
			Icon: "",
			Hotkeys: []Hotkey{
				{"Prefix + |", "Split pane vertically"},
				{"Prefix + -", "Split pane horizontally"},
				{"Prefix + h/j/k/l", "Navigate panes"},
				{"Prefix + H/J/K/L", "Resize panes"},
				{"Prefix + z", "Toggle pane zoom"},
				{"Prefix + c", "New window"},
				{"Prefix + n/p", "Next/previous window"},
				{"Prefix + d", "Detach session"},
				{"Prefix + [", "Enter copy mode"},
				{"Prefix + r", "Reload config"},
			},
		},
		{
			Name: "Zsh",
			Icon: "",
			Hotkeys: []Hotkey{
				{"Ctrl+R", "Search command history"},
				{"Ctrl+T", "Fuzzy find files (fzf)"},
				{"Alt+C", "Fuzzy cd to directory"},
				{"Ctrl+G", "Fuzzy find git files"},
				{"Tab", "Autocomplete"},
				{"Ctrl+A/E", "Beginning/end of line"},
				{"Ctrl+U/K", "Delete to start/end"},
				{"Ctrl+W", "Delete word backwards"},
				{"Alt+.", "Insert last argument"},
				{"!!", "Repeat last command"},
			},
		},
		{
			Name: "Neovim",
			Icon: "",
			Hotkeys: []Hotkey{
				{"Space", "Leader key"},
				{"<Leader>ff", "Find files"},
				{"<Leader>fg", "Live grep"},
				{"<Leader>fb", "Browse buffers"},
				{"<Leader>e", "File explorer"},
				{"gd", "Go to definition"},
				{"K", "Hover documentation"},
				{"<Leader>ca", "Code actions"},
				{"<Leader>rn", "Rename symbol"},
				{"[d / ]d", "Prev/next diagnostic"},
			},
		},
		{
			Name: "Yazi",
			Icon: "",
			Hotkeys: []Hotkey{
				{"h/l", "Parent/enter directory"},
				{"j/k", "Move up/down"},
				{"gg/G", "First/last item"},
				{"Space", "Toggle selection"},
				{"y", "Yank (copy)"},
				{"d", "Cut"},
				{"p", "Paste"},
				{"a", "Create file"},
				{"r", "Rename"},
				{"/", "Search"},
			},
		},
		{
			Name: "Ghostty",
			Icon: "",
			Hotkeys: []Hotkey{
				{"Cmd+T", "New tab"},
				{"Cmd+W", "Close tab"},
				{"Cmd+Shift+[/]", "Prev/next tab"},
				{"Cmd+D", "Split vertical"},
				{"Cmd+Shift+D", "Split horizontal"},
				{"Cmd+]/[", "Next/prev pane"},
				{"Cmd++/-", "Increase/decrease font"},
				{"Cmd+0", "Reset font size"},
				{"Cmd+Shift+Enter", "Toggle fullscreen"},
				{"Cmd+,", "Open config"},
			},
		},
		{
			Name: "LazyGit",
			Icon: "",
			Hotkeys: []Hotkey{
				{"Space", "Stage/unstage file"},
				{"c", "Commit"},
				{"P", "Push"},
				{"p", "Pull"},
				{"b", "Branches menu"},
				{"m", "Merge"},
				{"r", "Rebase"},
				{"/", "Search"},
				{"?", "Help"},
				{"q", "Quit"},
			},
		},
	}
}

func (a *App) renderHotkeys() string {
	categories := GetHotkeyCategories()

	// Filter if specified
	if a.hotkeyFilter != "" {
		var filtered []HotkeyCategory
		for _, cat := range categories {
			if strings.ToLower(cat.Name) == strings.ToLower(a.hotkeyFilter) {
				filtered = append(filtered, cat)
			}
		}
		if len(filtered) > 0 {
			categories = filtered
		}
	}

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa")).
		Render("  Keyboard Shortcuts")

	if len(categories) == 0 {
		content := fmt.Sprintf("\n\n%s\n\n%s",
			title,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Render("No hotkeys found"))
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
	}

	// Category tabs
	var tabItems []string
	for i, cat := range categories {
		tabStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))
		if i == a.hotkeyCategory {
			tabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#89b4fa")).
				Bold(true).
				Underline(true)
		}
		tabItems = append(tabItems, tabStyle.Render(fmt.Sprintf(" %s %s ", cat.Icon, cat.Name)))
	}
	tabs := strings.Join(tabItems, "│")

	// Current category hotkeys
	currentCat := categories[a.hotkeyCategory]
	var hotkeyLines []string

	for i, hk := range currentCat.Hotkeys {
		keyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af")).
			Bold(true).
			Width(20)
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))

		cursor := "  "
		if i == a.hotkeyCursor {
			cursor = " "
			descStyle = descStyle.Foreground(lipgloss.Color("#a6e3a1"))
		}

		line := fmt.Sprintf("%s%s %s",
			cursor,
			keyStyle.Render(hk.Keys),
			descStyle.Render(hk.Description))
		hotkeyLines = append(hotkeyLines, line)
	}

	hotkeyList := strings.Join(hotkeyLines, "\n")

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("←→ Categories • ↑↓ Scroll • Esc Back • q Quit")

	content := fmt.Sprintf("\n\n%s\n\n%s\n\n%s\n\n%s",
		title, tabs, hotkeyList, footer)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// =====================================
// Manage Screen
// =====================================

func (a *App) renderManage() string {
	registry := tools.NewRegistry()

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa")).
		Render("  Manage Tools")

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render(fmt.Sprintf("%d tools registered • %d installed", registry.Count(), registry.InstalledCount()))

	// Group by category
	categories := []tools.Category{
		tools.CategoryShell,
		tools.CategoryTerminal,
		tools.CategoryEditor,
		tools.CategoryFile,
		tools.CategoryGit,
		tools.CategoryContainer,
		tools.CategoryUtility,
	}

	var lines []string
	itemIndex := 0

	for _, cat := range categories {
		catTools := registry.ByCategory(cat)
		if len(catTools) == 0 {
			continue
		}

		// Category header
		catStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#cba6f7")).
			Bold(true)
		lines = append(lines, catStyle.Render(fmt.Sprintf("\n  %s", strings.ToUpper(string(cat)))))

		for _, tool := range catTools {
			cursor := "  "
			nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4"))
			descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))

			if itemIndex == a.manageIndex {
				cursor = " "
				nameStyle = nameStyle.Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
			}

			status := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("○")
			if tool.IsInstalled() {
				status = lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Render("●")
			}

			line := fmt.Sprintf("%s%s %s %s  %s",
				cursor,
				status,
				tool.Icon(),
				nameStyle.Render(tool.Name()),
				descStyle.Render(tool.Description()))
			lines = append(lines, line)
			itemIndex++
		}
	}

	toolList := strings.Join(lines, "\n")

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("↑↓ Navigate • Enter Configure • Esc Back • q Quit")

	content := fmt.Sprintf("\n\n%s\n%s\n%s\n\n%s",
		title, subtitle, toolList, footer)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content)
}

// =====================================
// Backups Screen
// =====================================

func (a *App) renderBackups() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa")).
		Render("  Backups")

	// TODO: Read actual backups from ~/.config/dotfiles/backups/
	message := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("Backup management coming soon...\n\nUse CLI: dotfiles backups")

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Render("Esc Back • q Quit")

	content := fmt.Sprintf("\n\n%s\n\n%s\n\n%s",
		title, message, footer)

	return lipgloss.Place(a.width, a.height,
		lipgloss.Center, lipgloss.Center,
		content)
}
