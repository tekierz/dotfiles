package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
	"github.com/tekierz/dotfiles/internal/ui"
	"github.com/tekierz/dotfiles/internal/ui/screens"
)

var (
	skipIntro bool
	version   = "2.0.1"
)

// rootCmd is the base command
var rootCmd = &cobra.Command{
	Use:   "dotfiles",
	Short: "Terminal environment management",
	Long: `Dotfiles is a unified terminal environment management platform.

It provides installation, configuration, and updates for your terminal
tools including zsh, tmux, neovim, yazi, ghostty, and more.

Quick user switch:
  dotfiles --<Username>    Switch to user profile (e.g., dotfiles --Pratik)`,
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

// userCmd handles user operations
var userCmd = &cobra.Command{
	Use:   "user [name]",
	Short: "Switch to or manage user profile",
	Long: `Switch to a user profile or manage user profiles.

Without arguments, shows the current active user.
With a name argument, switches to that user (creates if doesn't exist).

Examples:
  dotfiles user              # Show current user
  dotfiles user Pratik       # Switch to Pratik (prompts to create if new)
  dotfiles user add Alice    # Create new user Alice
  dotfiles user delete Bob   # Delete user Bob`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			showCurrentUser()
			return
		}
		switchToUser(args[0])
	},
}

// userAddCmd creates a new user profile
var userAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create new user profile",
	Long: `Create a new user profile with optional settings.

If no flags are provided, uses default settings.
Use flags to customize the profile:
  --theme     Theme name (e.g., catppuccin-mocha, dracula)
  --nav       Navigation style: emacs or vim
  --keyboard  Keyboard style: macos or linux

Examples:
  dotfiles user add Alice
  dotfiles user add Bob --theme dracula --nav vim
  dotfiles user add Carol --keyboard macos`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		theme, _ := cmd.Flags().GetString("theme")
		nav, _ := cmd.Flags().GetString("nav")
		keyboard, _ := cmd.Flags().GetString("keyboard")
		addUser(args[0], theme, nav, keyboard)
	},
}

// userDeleteCmd deletes a user profile
var userDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete user profile",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		force, _ := cmd.Flags().GetBool("force")
		deleteUser(args[0], force)
	},
}

// usersCmd lists all users
var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "List all user profiles",
	Run: func(cmd *cobra.Command, args []string) {
		listUsers()
	},
}

// uninstallCmd removes dotfiles and restores original config
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove dotfiles and restore original configuration",
	Long: `Uninstall dotfiles completely and restore your original configuration.

This will:
  - Restore configuration files from backup (if available)
  - Remove dotfiles binaries from ~/.local/bin
  - Remove utility scripts (hk, caff, sshh)
  - Remove dotfiles configuration directory

Use --keep-config to preserve the ~/.config/dotfiles directory.
Use --keep-binaries to preserve installed binaries.
Use --no-restore to skip restoring backups.`,
	Run: func(cmd *cobra.Command, args []string) {
		keepConfig, _ := cmd.Flags().GetBool("keep-config")
		keepBinaries, _ := cmd.Flags().GetBool("keep-binaries")
		noRestore, _ := cmd.Flags().GetBool("no-restore")
		force, _ := cmd.Flags().GetBool("force")

		runUninstall(keepConfig, keepBinaries, noRestore, force)
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVar(&skipIntro, "skip-intro", false, "Skip intro animation")

	// Hotkeys flags
	hotkeysCmd.Flags().String("tool", "", "Filter hotkeys by tool (tmux, zsh, neovim, etc.)")

	// Uninstall flags
	uninstallCmd.Flags().Bool("keep-config", false, "Keep ~/.config/dotfiles directory")
	uninstallCmd.Flags().Bool("keep-binaries", false, "Keep installed binaries")
	uninstallCmd.Flags().Bool("no-restore", false, "Skip restoring backups")
	uninstallCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// User command flags
	userAddCmd.Flags().String("theme", "", "Theme name (e.g., catppuccin-mocha)")
	userAddCmd.Flags().String("nav", "", "Navigation style: emacs or vim")
	userAddCmd.Flags().String("keyboard", "", "Keyboard style: macos or linux")
	userDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// User subcommands
	userCmd.AddCommand(userAddCmd)
	userCmd.AddCommand(userDeleteCmd)

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
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(usersCmd)
}

func main() {
	// Handle --<Username> quick switch before Cobra parses flags
	// This allows "dotfiles --Alice" to work as a quick user switch
	if len(os.Args) == 2 {
		arg := os.Args[1]
		if strings.HasPrefix(arg, "--") && len(arg) > 2 && !strings.Contains(arg, "=") {
			username := arg[2:]
			// Skip if it's a known flag or looks like a help request
			if username != "help" && username != "version" && username != "skip-intro" {
				if config.ValidateUsername(username) == nil && config.UserExists(username) {
					switchToUser(username)
					return
				}
				// If username is valid but doesn't exist, show helpful message
				if config.ValidateUsername(username) == nil {
					fmt.Printf("User %q does not exist.\n", username)
					fmt.Println("Create with: dotfiles user add", username)
					return
				}
			}
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// createScreenFactory creates the screen factory for the ScreenManager
func createScreenFactory() ui.ScreenFactory {
	factory := screens.NewFactory()
	return factory.CreateFactory()
}

// launchTUI launches the TUI at a specific screen
func launchTUI(screen ui.Screen) {
	app := ui.NewApp(skipIntro, ui.WithScreenFactory(createScreenFactory()))
	app.SetStartScreen(screen)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// launchToolConfig launches TUI for a specific tool config
func launchToolConfig(tool string) {
	app := ui.NewApp(true, ui.WithScreenFactory(createScreenFactory()))

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
	app := ui.NewApp(true, ui.WithScreenFactory(createScreenFactory()))
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
	if cfg.ActiveUser != "" {
		fmt.Printf("User:       %s\n", cfg.ActiveUser)
	}
	fmt.Printf("Theme:      %s\n", cfg.Theme)
	fmt.Printf("Navigation: %s\n", cfg.NavStyle)
	fmt.Printf("Config dir: %s\n", config.ConfigDir())
	fmt.Println()

	// Show installed tools (filtered by platform)
	registry := tools.GetRegistry()
	installed := registry.Installed()
	notInstalled := registry.NotInstalledForPlatform()

	fmt.Printf("Installed Tools: %d/%d\n", len(installed), registry.CountForPlatform())
	fmt.Println("─────────────────────────")

	// Group by category
	byCategory := make(map[tools.Category][]tools.Tool)
	for _, t := range installed {
		byCategory[t.Category()] = append(byCategory[t.Category()], t)
	}

	categories := []tools.Category{
		tools.CategoryShell, tools.CategoryTerminal, tools.CategoryEditor,
		tools.CategoryFile, tools.CategoryGit, tools.CategoryContainer,
		tools.CategoryUtility, tools.CategoryApp,
	}

	// Get current platform for package details
	platform := pkg.DetectPlatform()

	for _, cat := range categories {
		if catTools, ok := byCategory[cat]; ok && len(catTools) > 0 {
			names := make([]string, 0, len(catTools))
			for _, t := range catTools {
				// For tools with multiple packages (like zsh), show them
				pkgs := t.Packages()[platform]
				if len(pkgs) == 0 {
					pkgs = t.Packages()["all"]
				}
				if len(pkgs) > 1 {
					// Show tool name with package count
					names = append(names, fmt.Sprintf("%s (+%d pkgs)", t.Name(), len(pkgs)-1))
				} else {
					names = append(names, t.Name())
				}
			}
			fmt.Printf("  %s: %s\n", strings.Title(string(cat)), strings.Join(names, ", "))
		}
	}

	if len(notInstalled) > 0 {
		fmt.Println()
		fmt.Printf("Not Installed: %d tools\n", len(notInstalled))
		names := make([]string, 0, len(notInstalled))
		for _, t := range notInstalled {
			names = append(names, t.Name())
		}
		fmt.Printf("  %s\n", strings.Join(names, ", "))
	}
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
	backupDir := filepath.Join(config.ConfigDir(), "backups")

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No backups found.")
			fmt.Printf("Backup directory: %s\n", backupDir)
			return
		}
		fmt.Fprintf(os.Stderr, "Error reading backups: %v\n", err)
		return
	}

	// Filter for directories only (backups are stored in timestamped dirs)
	var backups []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			backups = append(backups, e)
		}
	}

	if len(backups) == 0 {
		fmt.Println("No backups found.")
		return
	}

	// Sort by name (timestamps sort chronologically)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name() > backups[j].Name() // Most recent first
	})

	fmt.Printf("Available backups (%d):\n", len(backups))
	fmt.Println("─────────────────────────")

	for _, b := range backups {
		info, err := b.Info()
		if err != nil {
			fmt.Printf("  %s\n", b.Name())
			continue
		}

		// Count files in backup
		files, _ := os.ReadDir(filepath.Join(backupDir, b.Name()))
		fileCount := 0
		for _, f := range files {
			if !f.IsDir() {
				fileCount++
			}
		}

		fmt.Printf("  %s  (%d files, %s)\n",
			b.Name(),
			fileCount,
			info.ModTime().Format("Jan 02 15:04"))
	}

	fmt.Println()
	fmt.Println("To restore: dotfiles restore <backup-name>")
}

// restoreBackup restores a specific backup
func restoreBackup(name string) {
	backupDir := filepath.Join(config.ConfigDir(), "backups", name)

	info, err := os.Stat(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Backup '%s' not found.\n", name)
			fmt.Println("Run 'dotfiles backups' to see available backups.")
			return
		}
		fmt.Fprintf(os.Stderr, "Error accessing backup: %v\n", err)
		return
	}

	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "'%s' is not a valid backup directory.\n", name)
		return
	}

	// Read manifest if it exists
	manifestPath := filepath.Join(backupDir, "manifest.txt")
	manifest, _ := os.ReadFile(manifestPath)

	fmt.Printf("Restoring backup: %s\n", name)
	if len(manifest) > 0 {
		fmt.Printf("Manifest:\n%s\n", string(manifest))
	}

	// Walk backup directory and restore files
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading backup: %v\n", err)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		return
	}

	restored := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "manifest.txt" {
			continue
		}

		srcPath := filepath.Join(backupDir, entry.Name())
		// Backup files are named with underscores replacing slashes
		// e.g., ".config_dotfiles_settings.json" -> ".config/dotfiles/settings.json"
		relPath := strings.ReplaceAll(entry.Name(), "_", string(os.PathSeparator))
		dstPath := filepath.Clean(filepath.Join(home, relPath))

		// Security: Prevent path traversal attacks. A malicious backup file named
		// ".._.._etc_passwd" would be converted to "../../etc/passwd", allowing
		// arbitrary file overwrites outside the home directory. We validate that
		// the final destination path stays within the user's home directory.
		if !strings.HasPrefix(dstPath, home+string(os.PathSeparator)) && dstPath != home {
			fmt.Fprintf(os.Stderr, "  Warning: Skipping %s - path traversal detected\n", entry.Name())
			continue
		}

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "  Error creating directory for %s: %v\n", relPath, err)
			continue
		}

		// Read source and write to destination
		data, err := os.ReadFile(srcPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error reading %s: %v\n", entry.Name(), err)
			continue
		}

		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  Error writing %s: %v\n", relPath, err)
			continue
		}

		fmt.Printf("  Restored: %s\n", relPath)
		restored++
	}

	fmt.Printf("\nRestored %d files from backup.\n", restored)
}

// runUninstall removes dotfiles and optionally restores original configuration
func runUninstall(keepConfig, keepBinaries, noRestore, force bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	configDir := config.ConfigDir()

	// Show what will be done
	fmt.Println("Dotfiles Uninstaller")
	fmt.Println("====================")
	fmt.Println()
	fmt.Println("This will:")

	if !noRestore {
		fmt.Println("  • Restore configuration files from latest backup (if available)")
	}
	if !keepBinaries {
		fmt.Println("  • Remove dotfiles binaries (dotfiles, dotfiles-tui, dotfiles-setup)")
		fmt.Println("  • Remove utility scripts (hk, caff, y)")
	}
	if !keepConfig {
		fmt.Printf("  • Remove configuration directory (%s)\n", configDir)
	}
	fmt.Println()

	// Prompt for confirmation unless --force
	if !force {
		fmt.Print("Continue with uninstall? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Uninstall cancelled.")
			return
		}
		fmt.Println()
	}

	// Restore from latest backup
	if !noRestore {
		fmt.Println("Checking for backups...")
		backupDir := filepath.Join(configDir, "backups")
		if entries, err := os.ReadDir(backupDir); err == nil && len(entries) > 0 {
			// Find most recent backup (directories sorted by timestamp)
			var latestBackup string
			for _, e := range entries {
				if e.IsDir() {
					if latestBackup == "" || e.Name() > latestBackup {
						latestBackup = e.Name()
					}
				}
			}
			if latestBackup != "" {
				fmt.Printf("Restoring from backup: %s\n", latestBackup)
				restoreBackup(latestBackup)
				fmt.Println()
			}
		} else {
			fmt.Println("No backups found to restore.")
			fmt.Println()
		}
	}

	// Remove binaries
	if !keepBinaries {
		fmt.Println("Removing binaries...")

		// Binaries to remove
		binaries := []string{
			"dotfiles",
			"dotfiles-tui",
			"dotfiles-setup",
			"hk",
			"caff",
			"y",
		}

		// Locations to check
		locations := []string{
			filepath.Join(home, ".local", "bin"),
			"/usr/local/bin",
		}

		removed := 0
		for _, loc := range locations {
			for _, bin := range binaries {
				binPath := filepath.Join(loc, bin)
				if _, err := os.Stat(binPath); err == nil {
					if err := os.Remove(binPath); err != nil {
						fmt.Fprintf(os.Stderr, "  Warning: Could not remove %s: %v\n", binPath, err)
					} else {
						fmt.Printf("  Removed: %s\n", binPath)
						removed++
					}
				}
			}
		}

		if removed == 0 {
			fmt.Println("  No binaries found to remove.")
		}
		fmt.Println()
	}

	// Remove config directory
	if !keepConfig {
		fmt.Printf("Removing configuration directory: %s\n", configDir)
		if _, err := os.Stat(configDir); err == nil {
			if err := os.RemoveAll(configDir); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: Could not remove config directory: %v\n", err)
			} else {
				fmt.Println("  Configuration directory removed.")
			}
		} else {
			fmt.Println("  Configuration directory not found.")
		}
		fmt.Println()
	}

	fmt.Println("Uninstall complete!")
	fmt.Println()
	fmt.Println("Note: The following may still need manual cleanup:")
	fmt.Println("  • Shell configuration (~/.zshrc, ~/.bashrc)")
	fmt.Println("  • Tmux configuration (~/.tmux.conf)")
	fmt.Println("  • Ghostty configuration (~/.config/ghostty)")
	fmt.Println("  • Neovim configuration (~/.config/nvim)")
	fmt.Println("  • Installed packages (use your package manager)")
	fmt.Println()
	fmt.Println("To completely remove dotfiles from Homebrew:")
	fmt.Println("  brew uninstall tekierz/tap/dotfiles")
}

// showCurrentUser displays the current active user
func showCurrentUser() {
	profile, err := config.GetActiveUser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting active user: %v\n", err)
		os.Exit(1)
	}

	if profile == nil {
		fmt.Println("No active user profile.")
		fmt.Println()
		fmt.Println("Create a user profile with:")
		fmt.Println("  dotfiles user add <name>")
		return
	}

	fmt.Printf("Active User: %s\n", profile.Name)
	fmt.Printf("  Theme:    %s\n", profile.Theme)
	fmt.Printf("  Nav:      %s\n", profile.NavStyle)
	fmt.Printf("  Keyboard: %s\n", profile.KeyboardStyle)
}

// switchToUser switches to a user profile, prompting to create if it doesn't exist
func switchToUser(name string) {
	if err := config.ValidateUsername(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !config.UserExists(name) {
		fmt.Printf("User %q does not exist.\n", name)
		fmt.Print("Create new user profile? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return
		}

		// Create with defaults
		addUser(name, "", "", "")
		return
	}

	profile, err := config.LoadUserProfile(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading user profile: %v\n", err)
		os.Exit(1)
	}

	if err := config.ApplyUserProfile(profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error applying user profile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Switched to user: %s\n", profile.Name)
	fmt.Printf("  Theme:    %s\n", profile.Theme)
	fmt.Printf("  Nav:      %s\n", profile.NavStyle)
	fmt.Printf("  Keyboard: %s\n", profile.KeyboardStyle)
	fmt.Println()
	fmt.Println("Run 'dotfiles install' to apply theme changes to all tools.")
}

// addUser creates a new user profile
func addUser(name, theme, nav, keyboard string) {
	if err := config.ValidateUsername(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if config.UserExists(name) {
		fmt.Printf("User %q already exists.\n", name)
		fmt.Print("Overwrite existing profile? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	profile := config.DefaultUserProfile(name)

	// Apply provided settings
	if theme != "" {
		if !config.IsValidTheme(theme) {
			fmt.Fprintf(os.Stderr, "Invalid theme: %s\n", theme)
			fmt.Println("Available themes:")
			for _, t := range config.AvailableThemes {
				fmt.Printf("  %s\n", t)
			}
			os.Exit(1)
		}
		profile.Theme = theme
	}

	if nav != "" {
		if !config.IsValidNavStyle(nav) {
			fmt.Fprintf(os.Stderr, "Invalid nav style: %s\n", nav)
			fmt.Println("Valid options: emacs, vim")
			os.Exit(1)
		}
		profile.NavStyle = nav
	}

	if keyboard != "" {
		if !config.IsValidKeyboardStyle(keyboard) {
			fmt.Fprintf(os.Stderr, "Invalid keyboard style: %s\n", keyboard)
			fmt.Println("Valid options: macos, linux")
			os.Exit(1)
		}
		profile.KeyboardStyle = keyboard
	}

	if err := config.SaveUserProfile(profile); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving user profile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created user profile: %s\n", profile.Name)
	fmt.Printf("  Theme:    %s\n", profile.Theme)
	fmt.Printf("  Nav:      %s\n", profile.NavStyle)
	fmt.Printf("  Keyboard: %s\n", profile.KeyboardStyle)
	fmt.Println()
	fmt.Printf("Switch to this user with: dotfiles user %s\n", name)
}

// deleteUser removes a user profile
func deleteUser(name string, force bool) {
	if err := config.ValidateUsername(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !config.UserExists(name) {
		fmt.Fprintf(os.Stderr, "User %q does not exist.\n", name)
		os.Exit(1)
	}

	// Check if this is the active user
	activeProfile, _ := config.GetActiveUser()
	isActive := activeProfile != nil && activeProfile.Name == name

	if isActive {
		fmt.Printf("Warning: %q is the currently active user.\n", name)
	}

	if !force {
		fmt.Printf("Delete user profile %q? [y/N]: ", name)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	if err := config.DeleteUserProfile(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting user profile: %v\n", err)
		os.Exit(1)
	}

	// Clear active user if we deleted them
	if isActive {
		if err := config.ClearActiveUser(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not clear active user: %v\n", err)
		}
	}

	fmt.Printf("Deleted user profile: %s\n", name)
}

// listUsers displays all user profiles
func listUsers() {
	users, err := config.ListUserProfiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing users: %v\n", err)
		os.Exit(1)
	}

	if len(users) == 0 {
		fmt.Println("No user profiles found.")
		fmt.Println()
		fmt.Println("Create a user profile with:")
		fmt.Println("  dotfiles user add <name>")
		return
	}

	// Get active user for marking
	activeProfile, _ := config.GetActiveUser()
	activeName := ""
	if activeProfile != nil {
		activeName = activeProfile.Name
	}

	fmt.Printf("User Profiles (%d):\n", len(users))
	fmt.Println("─────────────────────────")

	for _, name := range users {
		profile, err := config.LoadUserProfile(name)
		if err != nil {
			fmt.Printf("  %s (error loading)\n", name)
			continue
		}

		marker := "○"
		if name == activeName {
			marker = "●"
		}

		fmt.Printf("  %s %s\n", marker, profile.Name)
		fmt.Printf("      Theme: %s, Nav: %s, Keyboard: %s\n",
			profile.Theme, profile.NavStyle, profile.KeyboardStyle)
	}

	fmt.Println()
	fmt.Println("● = active user")
}
