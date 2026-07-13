package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
	"github.com/tekierz/dotfiles/internal/ui"
)

var (
	skipIntro bool
	version   = "dev"
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

// configCmd handles per-tool configuration. Long is set in init() from the
// authoritative tool→screen map so help and behavior never diverge (C28).
var configCmd = &cobra.Command{
	Use:   "config <tool>",
	Short: "Configure a specific tool",
	Long: "Configure a specific tool. Without flags, launches TUI.\n\n" +
		"Available tools: " + strings.Join(ui.ConfigurableToolIDs(), ", "),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			// No tool specified: show help
			_ = cmd.Help()
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

// statusCmd shows current status.
var statusCmd = newRegisteredStatusCommand()

// planCmd prints the deterministic, install-only public plan contract.
var planCmd = newRegisteredPlanCommand()

// applyCmd executes only an exact freshly reviewed installation plan.
var applyCmd = newRegisteredApplyCommand()

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
			if _, _, err := restoreBackup(args[0]); err != nil {
				os.Exit(1)
			}
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

// uninstallCmd restores original config and reports conservative cleanup guidance.
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Restore backups and show safe manual uninstall guidance",
	Long: `Safely prepare to uninstall dotfiles and restore your original configuration.

This command can restore configuration files from the latest backup. Automatic
deletion is disabled until dotfiles has an ownership manifest, provenance checks,
and descriptor-anchored recursive removal. Binaries, helpers, packages, and the
dotfiles configuration directory are retained even when --force is used.

Remove the Homebrew-managed main binary separately with:
  brew uninstall tekierz/tap/dotfiles

Use --no-restore to skip the backup restore attempt. The --keep-config and
--keep-binaries flags remain accepted for compatibility; retention is currently
unconditional.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		keepConfig, _ := cmd.Flags().GetBool("keep-config")
		keepBinaries, _ := cmd.Flags().GetBool("keep-binaries")
		noRestore, _ := cmd.Flags().GetBool("no-restore")
		force, _ := cmd.Flags().GetBool("force")

		return runUninstall(keepConfig, keepBinaries, noRestore, force)
	},
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("dotfiles version {{.Version}}\n")

	// Global flags
	rootCmd.PersistentFlags().BoolVar(&skipIntro, "skip-intro", false, "Skip intro animation")

	// Hotkeys flags
	hotkeysCmd.Flags().String("tool", "", "Filter hotkeys by tool (tmux, zsh, neovim, etc.)")

	// Uninstall flags
	uninstallCmd.Flags().Bool("keep-config", false, "Compatibility flag; configuration is always retained")
	uninstallCmd.Flags().Bool("keep-binaries", false, "Compatibility flag; binaries and helpers are always retained")
	uninstallCmd.Flags().Bool("no-restore", false, "Skip restoring backups")
	uninstallCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt (does not enable deletion)")

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
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(backupsCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(usersCmd)
}

func main() {
	if err := validateEffectiveUser(effectiveUserID()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

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

	if code := executeRoot(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func executeRoot(args []string, stdout, stderr io.Writer) int {
	previousSilenceErrors, previousSilenceUsage := rootCmd.SilenceErrors, rootCmd.SilenceUsage
	rootCmd.SilenceErrors, rootCmd.SilenceUsage = true, true
	defer func() {
		rootCmd.SilenceErrors, rootCmd.SilenceUsage = previousSilenceErrors, previousSilenceUsage
	}()
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	if err := rootCmd.Execute(); err != nil {
		var exit *commandExitError
		if errors.As(err, &exit) {
			if !exit.silent {
				if _, writeErr := fmt.Fprintln(stderr, exit.Error()); writeErr != nil {
					return 1
				}
			}
			return exit.code
		}
		if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
			return 1
		}
		return 1
	}
	return 0
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
	app := ui.NewApp(true)

	screen, ok := ui.GetToolConfigScreen(tool)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown tool: %s\n", tool)
		fmt.Println("Available: " + strings.Join(ui.ConfigurableToolIDs(), ", "))
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
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		// Fall back to defaults so the theme list is still usable when
		// no config directory exists; no '(current)' marker will appear.
		cfg = config.DefaultGlobalConfig()
	}
	listThemesWithConfig(cfg)
}

// listThemesWithConfig prints available themes using the provided config.
// Passing nil is safe: it is treated the same as an empty config (no current
// theme is highlighted).
func listThemesWithConfig(cfg *config.GlobalConfig) {
	var current string
	if cfg != nil {
		current = cfg.Theme
	}

	fmt.Println("Available themes:")
	for _, t := range config.AvailableThemes {
		if t == current {
			fmt.Printf("  * %s (current)\n", t)
		} else {
			fmt.Printf("    %s\n", t)
		}
	}
}

// writeHumanStatus prints the legacy human-readable configuration status.
func writeHumanStatus(writer io.Writer) error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	output := make([]byte, 0, 1024)
	output = append(output, "Dotfiles Status\n===============\n"...)
	if cfg.ActiveUser != "" {
		output = fmt.Appendf(output, "User:       %s\n", cfg.ActiveUser)
	}
	output = fmt.Appendf(output, "Theme:      %s\n", cfg.Theme)
	output = fmt.Appendf(output, "Navigation: %s\n", cfg.NavStyle)
	output = fmt.Appendf(output, "Config dir: %s\n\n", config.ConfigDir())

	// Show installed tools (filtered by platform)
	registry := tools.GetRegistry()
	installed := registry.Installed()
	notInstalled := registry.NotInstalledForPlatform()

	output = fmt.Appendf(output, "Installed Tools: %d/%d\n", len(installed), registry.CountForPlatform())
	output = append(output, "─────────────────────────\n"...)

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
			categoryName := string(cat)
			categoryName = strings.ToUpper(categoryName[:1]) + categoryName[1:]
			output = fmt.Appendf(output, "  %s: %s\n", categoryName, strings.Join(names, ", "))
		}
	}

	if len(notInstalled) > 0 {
		output = append(output, '\n')
		output = fmt.Appendf(output, "Not Installed: %d tools\n", len(notInstalled))
		names := make([]string, 0, len(notInstalled))
		for _, t := range notInstalled {
			names = append(names, t.Name())
		}
		output = fmt.Appendf(output, "  %s\n", strings.Join(names, ", "))
	}
	written, err := writer.Write(output)
	if err != nil {
		return err
	}
	if written != len(output) {
		return io.ErrShortWrite
	}
	return nil
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
		// Partial results (one of several managers failed) still print.
		if len(updates) == 0 {
			fmt.Fprintf(os.Stderr, "Error checking updates: %v\n", err)
			return
		}
		fmt.Fprintf(os.Stderr, "Warning: some update checks failed: %v\n\n", err)
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

	entries, err := backup.ListCatalog(backupDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading backups: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Println("No backups found.")
		fmt.Printf("Backup directory: %s\n", backupDir)
		return
	}

	fmt.Printf("Available backups (%d):\n", len(entries))
	fmt.Println("─────────────────────────")

	for _, entry := range entries {
		fmt.Printf("  %s  (%d files, %s)\n",
			entry.Name,
			entry.FileCount,
			entry.Timestamp.Format("Jan 02 15:04"))
	}

	fmt.Println()
	fmt.Println("To restore: dotfiles restore <backup-name>")
}

// restoreBackup restores a specific backup. It returns the number of files
// successfully restored, the number of entries skipped, and a fatal error for
// failures that prevent a clean restore. The path mapping, traversal guard, and
// mode preservation are shared with the TUI via the internal/backup package so
// the two paths cannot diverge.
// isValidBackupName reports whether name is a safe single-component backup name.
// A backup lives at <config>/backups/<name>; allowing "..", absolute paths, or
// path separators would let an externally-supplied name traverse out of the
// backups directory and read an arbitrary restore SOURCE. The name must equal
// its own filepath.Base and contain no separator or parent reference.
func isValidBackupName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if filepath.IsAbs(name) {
		return false
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator) {
		return false
	}
	return name == filepath.Base(name)
}

func restoreBackup(name string) (int, int, error) {
	if !isValidBackupName(name) {
		fmt.Fprintf(os.Stderr, "Invalid backup name: %q\n", name)
		fmt.Println("Run 'dotfiles backups' to see available backups.")
		return 0, 0, fmt.Errorf("invalid backup name: %q", name)
	}

	backupDir := filepath.Join(config.ConfigDir(), "backups", name)
	catalog, err := backup.ListCatalog(filepath.Dir(backupDir))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing backup: %v\n", err)
		return 0, 0, err
	}
	var selected *backup.CatalogEntry
	for index := range catalog {
		if catalog[index].Name == name {
			selected = &catalog[index]
			break
		}
	}
	if selected == nil {
		fmt.Fprintf(os.Stderr, "Backup '%s' not found or has no valid manifest.\n", name)
		fmt.Println("Run 'dotfiles backups' to see available backups.")
		return 0, 0, os.ErrNotExist
	}
	if err := backup.ValidateCatalogEntry(*selected); err != nil {
		return 0, 0, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		return 0, 0, err
	}

	fmt.Printf("Restoring backup: %s\n", name)

	result, err := backup.Restore(backupDir, home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading backup: %v\n", err)
		return 0, 0, err
	}

	for _, relPath := range result.Restored {
		fmt.Printf("  Restored: %s\n", relPath)
	}
	for _, relPath := range result.Removed {
		fmt.Printf("  Removed: %s\n", relPath)
	}
	for item, reason := range result.Skipped {
		fmt.Fprintf(os.Stderr, "  Warning: Skipping %s - %s\n", item, reason)
	}
	for item, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "  Warning: Restored %s - %s\n", item, warning)
	}

	fmt.Printf("\nRestored %d files from backup.\n", result.Count())
	if len(result.Removed) > 0 {
		fmt.Printf("Removed %d files/directories created after the backup.\n", len(result.Removed))
	}
	if outcomeErr := restoreOutcomeError(result); outcomeErr != nil {
		return result.Count(), len(result.Skipped), outcomeErr
	}
	return result.Count(), 0, nil
}

func restoreOutcomeError(result backup.RestoreResult) error {
	var problems []string
	if len(result.Skipped) > 0 {
		problems = append(problems, fmt.Sprintf("%d backup entries could not be restored", len(result.Skipped)))
	}
	if len(result.Warnings) > 0 {
		problems = append(problems, fmt.Sprintf("%d restored backup entries have durability/verification warnings", len(result.Warnings)))
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

// runUninstall optionally restores original configuration and reports retained
// resources. Cleanup is deliberately fail-closed: without an ownership manifest,
// provenance checks, and descriptor-anchored recursive removal, no binary,
// helper, or dotfiles state directory is safe to delete automatically.
var readUninstallBackupDir = os.ReadDir

func runUninstall(keepConfig, keepBinaries, noRestore, force bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	configDir := config.ConfigDir()

	printUninstallPreview(!keepConfig || !keepBinaries, noRestore)
	confirmed, err := confirmUninstall(force)
	if err != nil || !confirmed {
		return err
	}

	var restoreErr error
	if !noRestore {
		restoreErr = restoreLatestUninstallBackup(configDir)
	}
	printUninstallGuidance(home, configDir)
	return restoreErr
}

func printUninstallPreview(deletionRequested, noRestore bool) {
	fmt.Println("Safe Dotfiles Uninstall")
	fmt.Println("=======================")
	fmt.Println()
	fmt.Println("This will:")

	if !noRestore {
		fmt.Println("  • Restore configuration files from latest backup (if available)")
	}
	fmt.Println("  • Retain binaries, helpers, packages, and the dotfiles configuration directory")
	if deletionRequested {
		fmt.Println("  • Decline automatic deletion because ownership and anchored-removal safeguards are not implemented")
	}
	fmt.Println()
}

func confirmUninstall(force bool) (bool, error) {
	if force {
		return true, nil
	}
	fmt.Print("Continue with restore and manual uninstall guidance? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read uninstall confirmation: %w", err)
	}
	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("Uninstall cancelled.")
		return false, nil
	}
	fmt.Println()
	return true, nil
}

func restoreLatestUninstallBackup(configDir string) error {
	// Restore from latest backup.
	//
	// The backups directory lives inside configDir. If restore is incomplete,
	// report the preserved safety net explicitly. Cleanup below is disabled in
	// every case, so the backups survive even after a successful restore.
	fmt.Println("Checking for backups...")
	backupDir := filepath.Join(configDir, "backups")
	entries, readErr := readUninstallBackupDir(backupDir)
	switch {
	case errors.Is(readErr, os.ErrNotExist):
		fmt.Println("No backup directory found; nothing was restored.")
		fmt.Println()
		return nil
	case readErr != nil:
		problem := fmt.Errorf("read backup directory %s: %w", backupDir, readErr)
		fmt.Fprintf(os.Stderr, "Could not inspect backups: %v\n", problem)
		fmt.Fprintf(os.Stderr, "Backup state was left untouched at: %s\n\n", backupDir)
		return problem
	}

	latestBackup := latestBackupDirectory(entries)
	if latestBackup == "" {
		fmt.Println("No backup session directories found; nothing was restored.")
		fmt.Println()
		return nil
	}
	count, skipped, restoreErr := restoreBackup(latestBackup)
	fmt.Println()
	if restoreErr == nil && skipped == 0 && count > 0 {
		return nil
	}
	if restoreErr == nil {
		restoreErr = fmt.Errorf("restore produced %d restored and %d skipped entries", count, skipped)
	}
	fmt.Fprintln(os.Stderr, "Restore did not complete successfully; keeping configuration directory so backups are preserved.")
	fmt.Fprintf(os.Stderr, "Your backups remain at: %s\n", backupDir)
	fmt.Fprintln(os.Stderr, "Re-run 'dotfiles restore <backup-name>' or remove the directory manually once recovered.")
	fmt.Fprintln(os.Stderr)
	return fmt.Errorf("restore backup %s: %w", latestBackup, restoreErr)
}

func latestBackupDirectory(entries []os.DirEntry) string {
	var latest string
	for _, entry := range entries {
		if entry.IsDir() && (latest == "" || entry.Name() > latest) {
			latest = entry.Name()
		}
	}
	return latest
}

func printUninstallGuidance(home, configDir string) {
	// Gate 0: do not infer ownership from a basename or recursively remove a
	// path-resolved config directory. Re-enable cleanup only after installation
	// records exact owned artifacts and recursive deletion is descriptor-anchored.
	fmt.Println("Automatic deletion is disabled in this release.")
	fmt.Println("No binaries, helpers, packages, or dotfiles state directory were deleted by uninstall cleanup.")
	fmt.Println()
	fmt.Println("Why: this build cannot yet prove artifact ownership and does not have anchored recursive removal.")
	fmt.Println()
	fmt.Println("Remove the Homebrew-managed main binary with:")
	fmt.Println("  brew uninstall tekierz/tap/dotfiles")
	fmt.Println()
	fmt.Println("Paths intentionally left untouched if present (verify ownership before manual removal):")
	retainedPaths := []string{
		configDir,
		filepath.Join(home, ".local", "bin", "dotfiles"),
		filepath.Join(home, ".local", "bin", "dotfiles-tui"),
		filepath.Join(home, ".local", "bin", "dotfiles-setup"),
		filepath.Join(home, ".local", "bin", "hk"),
		filepath.Join(home, ".local", "bin", "caff"),
		filepath.Join(home, ".local", "bin", "sshh"),
		"/usr/local/bin/dotfiles",
		"/usr/local/bin/dotfiles-tui",
		"/usr/local/bin/dotfiles-setup",
		"/usr/local/bin/hk",
		"/usr/local/bin/caff",
		"/usr/local/bin/sshh",
	}
	for _, path := range retainedPaths {
		if path != "" {
			fmt.Printf("  • %s\n", path)
		}
	}
	fmt.Println("Package-managed tools and external helpers such as sshh were retained; use their owning package manager.")
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
