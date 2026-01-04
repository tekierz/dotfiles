package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/ui"
)

var (
	skipIntro bool
	version   = "2.0.0-dev"
)

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "dotfiles",
	Short: "Terminal environment management",
	Long: `Dotfiles is a unified terminal environment management platform.

It provides installation, configuration, and updates for your terminal
tools including zsh, tmux, neovim, yazi, ghostty, and more.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default: launch TUI main menu
		launchTUI(ui.ScreenMainMenu)
	},
}

// installCmd launches the installation wizard
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Launch installation wizard",
	Run: func(cmd *cobra.Command, args []string) {
		if skipIntro {
			launchTUI(ui.ScreenWelcome)
		} else {
			launchTUI(ui.ScreenAnimation)
		}
	},
}

// manageCmd launches the tool management screen
var manageCmd = &cobra.Command{
	Use:   "manage",
	Short: "Manage tool configurations",
	Run: func(cmd *cobra.Command, args []string) {
		launchTUI(ui.ScreenManage)
	},
}

// updateCmd handles package updates
var updateCmd = &cobra.Command{
	Use:   "update [check]",
	Short: "Check and install package updates",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) > 0 && args[0] == "check" {
			// CLI mode: print outdated packages
			checkUpdates()
		} else {
			// TUI mode: interactive update screen
			launchTUI(ui.ScreenUpdate)
		}
	},
}

// themeCmd handles theme operations
var themeCmd = &cobra.Command{
	Use:   "theme [set <name>]",
	Short: "View or change theme",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			// No args: launch TUI picker
			launchTUI(ui.ScreenThemePicker)
			return
		}

		if args[0] == "set" && len(args) > 1 {
			// Direct set
			setTheme(args[1])
		} else if args[0] == "list" {
			// List available themes
			listThemes()
		} else {
			fmt.Println("Usage: dotfiles theme [set <name>|list]")
		}
	},
}

// configCmd handles per-tool configuration
var configCmd = &cobra.Command{
	Use:   "config <tool>",
	Short: "Configure a specific tool",
	Long: `Configure a specific tool. Without flags, launches TUI.

Available tools: ghostty, tmux, zsh, neovim, git, yazi, fzf, apps, utilities`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			// No tool specified: show help
			cmd.Help()
			return
		}

		// Check for flags (direct set mode)
		// For now, launch TUI for the specific tool
		launchToolConfig(args[0])
	},
}

// hotkeysCmd launches the hotkey viewer
var hotkeysCmd = &cobra.Command{
	Use:     "hotkeys",
	Aliases: []string{"hk"},
	Short:   "View hotkey reference",
	Run: func(cmd *cobra.Command, args []string) {
		tool, _ := cmd.Flags().GetString("tool")
		if tool != "" {
			launchHotkeysFiltered(tool)
		} else {
			launchTUI(ui.ScreenHotkeys)
		}
	},
}

// statusCmd shows current status
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current configuration status",
	Run: func(cmd *cobra.Command, args []string) {
		showStatus()
	},
}

// backupsCmd lists available backups
var backupsCmd = &cobra.Command{
	Use:   "backups",
	Short: "List available backups",
	Run: func(cmd *cobra.Command, args []string) {
		listBackups()
	},
}

// restoreCmd restores from backup
var restoreCmd = &cobra.Command{
	Use:   "restore [backup-name]",
	Short: "Restore from a backup",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			// TUI mode: select backup
			launchTUI(ui.ScreenBackups)
		} else {
			// CLI mode: restore specific backup
			restoreBackup(args[0])
		}
	},
}

// versionCmd shows version
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dotfiles version %s\n", version)
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVar(&skipIntro, "skip-intro", false, "Skip intro animation")

	// Hotkeys flags
	hotkeysCmd.Flags().String("tool", "", "Filter hotkeys by tool (tmux, zsh, neovim, etc.)")

	// Add subcommands
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(manageCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(themeCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(hotkeysCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(backupsCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// launchTUI launches the TUI at a specific screen
func launchTUI(screen ui.Screen) {
	app := ui.NewApp(skipIntro)
	app.SetStartScreen(screen)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// launchToolConfig launches TUI for a specific tool config
func launchToolConfig(tool string) {
	app := ui.NewApp(true) // Skip intro for direct config access

	screen, ok := ui.GetToolConfigScreen(tool)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown tool: %s\n", tool)
		fmt.Println("Available: ghostty, tmux, zsh, neovim, git, yazi, fzf, apps, utilities")
		os.Exit(1)
	}

	app.SetStartScreen(screen)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// launchHotkeysFiltered launches hotkey viewer filtered to a tool
func launchHotkeysFiltered(tool string) {
	app := ui.NewApp(true)
	app.SetStartScreen(ui.ScreenHotkeys)
	app.SetHotkeyFilter(tool)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// setTheme sets the theme directly via CLI
func setTheme(theme string) {
	if !config.IsValidTheme(theme) {
		fmt.Fprintf(os.Stderr, "Invalid theme: %s\n", theme)
		fmt.Println("Available themes:")
		for _, t := range config.AvailableThemes {
			fmt.Printf("  %s\n", t)
		}
		os.Exit(1)
	}

	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	cfg.Theme = theme
	if err := config.SaveGlobalConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Theme set to: %s\n", theme)
	fmt.Println("Run 'dotfiles install' to apply the new theme to all tools.")
}

// listThemes prints available themes
func listThemes() {
	cfg, _ := config.LoadGlobalConfig()
	current := cfg.Theme

	fmt.Println("Available themes:")
	for _, t := range config.AvailableThemes {
		if t == current {
			fmt.Printf("  * %s (current)\n", t)
		} else {
			fmt.Printf("    %s\n", t)
		}
	}
}

// showStatus prints current configuration status
func showStatus() {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Dotfiles Status")
	fmt.Println("===============")
	fmt.Printf("Theme:      %s\n", cfg.Theme)
	fmt.Printf("Navigation: %s\n", cfg.NavStyle)
	fmt.Printf("Config dir: %s\n", config.ConfigDir())

	// TODO: Show installed tools, outdated packages, etc.
}

// checkUpdates prints outdated packages (CLI mode)
func checkUpdates() {
	fmt.Println("Checking for updates...")

	mgr := pkg.DetectManager()
	if mgr == nil {
		fmt.Println("No package manager detected.")
		return
	}

	fmt.Printf("Using %s package manager\n\n", mgr.Name())

	updates, err := pkg.CheckDotfilesUpdates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking updates: %v\n", err)
		return
	}

	if len(updates) == 0 {
		fmt.Println("All packages are up to date!")
		return
	}

	fmt.Printf("Found %d outdated package(s):\n\n", len(updates))
	fmt.Printf("%-25s %-15s %-15s\n", "PACKAGE", "CURRENT", "LATEST")
	fmt.Printf("%-25s %-15s %-15s\n", "-------", "-------", "------")
	for _, p := range updates {
		fmt.Printf("%-25s %-15s %-15s\n", p.Name, p.CurrentVersion, p.LatestVersion)
	}
	fmt.Println()
	fmt.Println("Run 'dotfiles update' for interactive update selection.")
}

// listBackups prints available backups
func listBackups() {
	fmt.Println("Available backups:")
	// TODO: Read from ~/.config/dotfiles/backups/
	fmt.Println("Backup listing not yet implemented.")
}

// restoreBackup restores a specific backup
func restoreBackup(name string) {
	fmt.Printf("Restoring backup: %s\n", name)
	// TODO: Implement backup restore
	fmt.Println("Backup restore not yet implemented.")
}
