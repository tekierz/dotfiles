package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// Tree branch glyphs used when rendering the install summary file tree.
const (
	treeBranch = "├──"
	treeLeaf   = "└──"
)

// fileTreeScreen is the migrated ScreenHandler for the installation summary
// (file tree) screen shown before installation starts.
//
// State stays on App: it reads the deep-dive config and install cache through
// s.App(). Pressing enter navigates to the migrated progress screen, which
// triggers the install from its own Init() (the sudo check + startInstallation
// live in progressScreen.Init, so the start does not depend on the ordering of a
// separate installStartMsg relative to the NavigateMsg in a tea.Batch).
type fileTreeScreen struct {
	BaseScreen
}

// NewFileTreeScreen creates a new file tree screen handler.
func NewFileTreeScreen(ctx *ScreenContext) *fileTreeScreen {
	s := &fileTreeScreen{}
	s.SetContext(ctx)
	return s
}

// ID returns the screen identifier.
func (s *fileTreeScreen) ID() Screen { return ScreenFileTree }

// Init returns any initial commands (none on entry).
func (s *fileTreeScreen) Init() tea.Cmd { return nil }

// Update handles keyboard input for the file tree screen. (Mouse is a no-op,
// matching the legacy handleSummaryMouse for ScreenFileTree.)
func (s *fileTreeScreen) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case keyCtrlC, "q":
			return s, tea.Quit
		case keyEnter:
			// Begin installation: navigate to the migrated progress screen. The
			// install is triggered by progressScreen.Init() (which does the sudo
			// check itself), so we deliberately do NOT emit a separate
			// installStartMsg here — its ordering relative to the NavigateMsg in a
			// tea.Batch is not guaranteed, and the message would have no migrated
			// handler until Progress is active.
			return s, NavigateTo(ScreenProgress)
		case keyEsc:
			return s, NavigateTo(ScreenNavPicker)
		}
	}
	return s, nil
}

// View renders the installation summary: packages to install, already installed
// tools, and the files that will be modified.
func (s *fileTreeScreen) View(width, height int) string {
	a := s.App()

	title := TitleStyle.Render("Installation Summary")

	cfg := a.deepDiveConfig
	newStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	modStyle := lipgloss.NewStyle().Foreground(ColorYellow)
	mutedStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	textStyle := lipgloss.NewStyle().Foreground(ColorText)
	pkgStyle := lipgloss.NewStyle().Foreground(ColorCyan)

	var lines []string

	// Ensure install cache is populated
	a.ensureInstallCache()

	// Collect selected and already installed tools
	toInstall, alreadyInstalled := s.filetreeCollectTools(cfg)

	// Sort for stable display order (prevents flickering from map iteration)
	sort.Strings(toInstall)
	sort.Strings(alreadyInstalled)

	// Packages to install section
	lines = filetreeAppendInstallSections(lines, toInstall, alreadyInstalled, textStyle, pkgStyle, mutedStyle)

	// Files-to-be-modified tree (config dirs + home dotfiles + bin utilities).
	lines = filetreeAppendConfigFiles(lines, cfg, textStyle, newStyle, modStyle, mutedStyle)

	tree := strings.Join(lines, "\n")

	legend := mutedStyle.Render(
		fmt.Sprintf("  %s New    %s Modified    %s Package    %s Settings Only",
			newStyle.Render(glyphDotFilled),
			modStyle.Render(glyphDotFilled),
			pkgStyle.Render(glyphDotFilled),
			mutedStyle.Render(glyphDotFilled),
		))

	help := HelpStyle.Render("[ENTER] Start Installation    [ESC] Back")

	// Prevent the tree from overflowing narrow terminals.
	treeMaxW := maxInt(20, width-6)
	tree = lipgloss.NewStyle().MaxWidth(treeMaxW).Render(tree)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		ContainerStyle.Render(lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			tree,
			legend,
			"",
			help,
		)),
	)
}

// filetreeCollectTools partitions the enabled tools/apps into those that still
// need installing and those already installed, preserving the legacy traversal
// order (CLI tools, GUI apps, CLI utilities, utilities, then macOS apps).
func (s *fileTreeScreen) filetreeCollectTools(cfg *DeepDiveConfig) (toInstall, alreadyInstalled []string) {
	a := s.App()
	classify := func(m map[string]bool) {
		for id, enabled := range m {
			if enabled {
				if a.manageInstalled[id] {
					alreadyInstalled = append(alreadyInstalled, id)
				} else {
					toInstall = append(toInstall, id)
				}
			}
		}
	}

	classify(cfg.CLITools)     // CLI Tools
	classify(cfg.GUIApps)      // GUI Apps
	classify(cfg.CLIUtilities) // CLI Utilities
	classify(cfg.Utilities)    // Utilities
	if pkg.DetectPlatform() == pkg.PlatformMacOS {
		classify(cfg.MacApps) // macOS Apps (only on macOS)
	}
	return toInstall, alreadyInstalled
}

// filetreeAppendInstallSections appends the "Packages to Install" and "Already
// Installed" sections (each as a tree) to lines.
func filetreeAppendInstallSections(lines, toInstall, alreadyInstalled []string, textStyle, pkgStyle, mutedStyle lipgloss.Style) []string {
	if len(toInstall) > 0 {
		lines = append(lines, textStyle.Render("  Packages to Install:"))
		for i, toolID := range toInstall {
			prefix := treeBranch
			if i == len(toInstall)-1 {
				prefix = treeLeaf
			}
			lines = append(lines, textStyle.Render("  "+prefix+" ")+pkgStyle.Render(toolID))
		}
		lines = append(lines, "")
	}

	if len(alreadyInstalled) > 0 {
		lines = append(lines, mutedStyle.Render("  Already Installed (settings will update):"))
		for i, toolID := range alreadyInstalled {
			prefix := treeBranch
			if i == len(alreadyInstalled)-1 {
				prefix = treeLeaf
			}
			lines = append(lines, textStyle.Render("  "+prefix+" ")+mutedStyle.Render(toolID+" ✓"))
		}
		lines = append(lines, mutedStyle.Render("  Note: Settings and themes will be applied to all tools"))
		lines = append(lines, "")
	}
	return lines
}

// filetreeAppendConfigFiles appends the "Files to be Modified" tree: the
// ~/.config/ entries, the home dotfiles, and the ~/.local/bin/ utilities.
func filetreeAppendConfigFiles(lines []string, cfg *DeepDiveConfig, textStyle, newStyle, modStyle, mutedStyle lipgloss.Style) []string {
	// ~/.config/ section
	lines = append(lines, textStyle.Render("  Files to be Modified:"))
	lines = append(lines, textStyle.Render("  ~/.config/"))
	lines = append(lines, textStyle.Render("  ├── ")+newStyle.Render("dotfiles/")+mutedStyle.Render(" (new)"))
	lines = append(lines, textStyle.Render("  │   ├── settings"))
	lines = append(lines, textStyle.Render("  │   └── backups/"))

	// Ghostty
	lines = append(lines, textStyle.Render("  ├── ")+newStyle.Render("ghostty/"))
	lines = append(lines, textStyle.Render("  │   ├── config"))
	lines = append(lines, textStyle.Render("  │   └── themes/dotfiles-theme"))

	// Yazi
	lines = append(lines, textStyle.Render("  ├── ")+newStyle.Render("yazi/"))
	lines = append(lines, textStyle.Render("  │   ├── yazi.toml"))
	lines = append(lines, textStyle.Render("  │   ├── keymap.toml"))
	lines = append(lines, textStyle.Render("  │   └── theme.toml"))

	// Bat (if CLI utilities include bat)
	if cfg.CLIUtilities["bat"] {
		lines = append(lines, textStyle.Render("  ├── ")+newStyle.Render("bat/"))
		lines = append(lines, textStyle.Render("  │   └── config"))
	}

	// Neovim with config type
	nvimNote := ""
	switch cfg.NeovimConfig {
	case "kickstart":
		nvimNote = " (Kickstart.nvim)"
	case "lazyvim":
		nvimNote = " (LazyVim)"
	case "nvchad":
		nvimNote = " (NvChad)"
	case "custom":
		nvimNote = " (unchanged)"
	}
	lines = append(lines, textStyle.Render("  └── ")+newStyle.Render("nvim/")+mutedStyle.Render(nvimNote))

	// ~/ section
	lines = append(lines, "")
	lines = append(lines, textStyle.Render("  ~/"))
	lines = append(lines, textStyle.Render("  ├── ")+modStyle.Render(".zshrc")+mutedStyle.Render(" (backed up)"))
	lines = append(lines, textStyle.Render("  ├── ")+modStyle.Render(".tmux.conf")+mutedStyle.Render(" (backed up)"))
	lines = append(lines, textStyle.Render("  ├── ")+modStyle.Render(".gitconfig")+mutedStyle.Render(" (backed up)"))

	// ~/.local/bin/ utilities - only show enabled ones
	var binFiles []string
	if cfg.Utilities["hk"] {
		binFiles = append(binFiles, "hk")
	}
	if cfg.Utilities["caff"] {
		binFiles = append(binFiles, "caff")
	}
	if cfg.Utilities["sshh"] {
		binFiles = append(binFiles, "sshh")
	}
	binFiles = append(binFiles, "dotfiles") // Always installed

	if len(binFiles) > 0 {
		lines = append(lines, textStyle.Render("  └── .local/bin/"))
		for i, f := range binFiles {
			prefix := treeBranch
			if i == len(binFiles)-1 {
				prefix = treeLeaf
			}
			lines = append(lines, textStyle.Render("      "+prefix+" ")+newStyle.Render(f))
		}
	}
	return lines
}
