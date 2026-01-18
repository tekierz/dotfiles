package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
	"github.com/tekierz/dotfiles/internal/scripts"
	"github.com/tekierz/dotfiles/internal/tools"
)

const (
	introAnimationFrames = 72
	introAnimationTick   = 70 * time.Millisecond
	uiTick               = 80 * time.Millisecond
)

// Screen represents different screens in the wizard
type Screen int

const (
	ScreenAnimation Screen = iota
	ScreenWelcome
	ScreenThemePicker
	ScreenNavPicker
	ScreenFileTree
	ScreenProgress
	ScreenSummary
	ScreenError
	// Deep dive screens
	ScreenDeepDiveMenu
	ScreenConfigGhostty
	ScreenConfigTmux
	ScreenConfigZsh
	ScreenConfigNeovim
	ScreenConfigGit
	ScreenConfigYazi
	ScreenConfigFzf
	ScreenConfigUtilities
	ScreenConfigMacApps
	// Management platform screens (new)
	ScreenMainMenu
	ScreenManage
	ScreenUpdate
	ScreenHotkeys
	ScreenBackups
	ScreenUsers
	ScreenConfigApps
	// Additional config screens
	ScreenConfigCLITools
	ScreenConfigGUIApps
	ScreenConfigCLIUtilities // bat, eza, zoxide, ripgrep, fd, delta, fswatch
	// Individual CLI tool config screens (installer)
	ScreenConfigLazyGit
	ScreenConfigLazyDocker
	ScreenConfigBtop
	ScreenConfigGlow
	// Management config screens (detailed)
	ScreenManageGhostty
	ScreenManageTmux
	ScreenManageZsh
	ScreenManageNeovim
	ScreenManageGit
	ScreenManageYazi
	ScreenManageFzf
	ScreenManageLazyGit
	ScreenManageLazyDocker
	ScreenManageBtop
	ScreenManageGlow
	ScreenConfigClaudeCode
)

// Available themes
var themes = []struct {
	name  string
	desc  string
	color string
}{
	{"catppuccin-mocha", "Dark, warm pastels", "#89b4fa"},
	{"catppuccin-latte", "Light, warm pastels", "#1e66f5"},
	{"catppuccin-frappe", "Muted, cozy dark", "#8caaee"},
	{"catppuccin-macchiato", "Dark, punchy contrast", "#8aadf4"},
	{"dracula", "Dark with vibrant purples", "#bd93f9"},
	{"gruvbox-dark", "Retro warm browns", "#83a598"},
	{"gruvbox-light", "Warm paper-like tones", "#076678"},
	{"nord", "Arctic cool blues", "#88c0d0"},
	{"tokyo-night", "Rich purples and blues", "#7aa2f7"},
	{"solarized-dark", "Low contrast dark", "#268bd2"},
	{"solarized-light", "Low contrast light", "#268bd2"},
	{"monokai", "Classic vibrant", "#66d9ef"},
	{"rose-pine", "Soft muted pinks", "#c4a7e7"},
	{"one-dark", "Atom's dark theme", "#61afef"},
	{"everforest", "Green nature inspired", "#a7c080"},
	{"neon-seapunk", "Neon cyberpunk vibes", "#00F5D4"},
}

func (a *App) syncThemeIndex() {
	for i, t := range themes {
		if t.name == a.theme {
			a.themeIndex = i
			SetTheme(a.theme) // Apply theme colors for live preview
			return
		}
	}
}

// App is the main application model
type App struct {
	screen        Screen
	startScreen   Screen // Initial screen to show (for CLI routing)
	skipIntro     bool
	width         int
	height        int
	animationDone bool

	// Screen manager for migrated screens (nil during transition)
	screenMgr *ScreenManager

	// Animation state
	animFrame        int
	animTicker       *time.Ticker
	postIntroScreen  Screen // where to land after the intro animation
	uiFrame          int    // global animation frame counter (manager widgets, spinners, etc.)
	manageInstalling bool
	manageInstallID  string

	// User selections
	themeIndex int
	theme      string
	navStyle   string
	// animationsEnabled controls non-essential UI animations (headers/widgets).
	// When false, we render static UI to reduce motion/jank and CPU usage.
	animationsEnabled bool
	deepDive          bool

	// Deep dive state (installer)
	deepDiveMenuIndex int
	deepDiveConfig    *DeepDiveConfig
	configFieldIndex  int // Currently focused field in config screens
	macAppIndex       int // Currently focused app in macOS screen
	utilityIndex      int // Currently focused utility
	cliToolIndex      int // Currently focused CLI tool
	guiAppIndex       int // Currently focused GUI app
	cliUtilityIndex   int // Currently focused CLI utility (bat, eza, etc.)

	// Management state (detailed config)
	manageConfig *ManageConfig
	managePane   int // 0 = tools pane, 1 = settings pane (ScreenManage)
	// Cached install status for tools to avoid running package-manager checks every render.
	manageInstalled      map[string]bool
	manageInstalledReady bool
	installCacheLoading  bool // Currently loading cache asynchronously
	// Manage screen scrolling
	manageToolsScroll  int
	manageFieldsScroll int
	// Inline editing state (used by ScreenManage)
	manageEditing      bool
	manageEditValue    string
	manageEditCursor   int
	manageEditField    *string
	manageEditFieldKey string // human label for the field being edited
	manageStatus       string // transient status line (save result, etc.)

	// Installation state
	installStep     int
	installOutput   []string
	installRunning  bool
	installComplete bool
	installCmd      *exec.Cmd
	runner          *runner.Runner

	// Management platform state (new)
	mainMenuIndex        int                   // Main menu cursor
	manageIndex          int                   // Manage screen cursor
	updateIndex          int                   // Update screen cursor
	hotkeyFilter         string                // Filter hotkeys by tool
	hotkeyCursor         int                   // Hotkeys screen cursor
	hotkeyCategory       int                   // Current category in hotkeys
	hotkeysPane          int                   // 0 = categories, 1 = items
	hotkeyCatScroll      int                   // Category list scroll
	hotkeyItemScroll     int                   // Item list scroll
	hotkeysReturn        Screen                // Screen to return to when leaving hotkeys
	hotkeysFavorites     *config.HotkeysConfig // User hotkey favorites config
	hotkeysFavoritesOnly bool                  // Filter to show only favorites
	backupIndex          int                   // Backup selection cursor
	backups              []BackupEntry
	backupsLoaded        bool
	backupsLoading       bool
	backupConfirmMode    bool
	backupConfirmType    string // "restore" or "delete"
	backupStatus         string // Status message for backup operations
	backupRunning        bool   // Currently running a backup operation
	backupError          error  // Error from backup operation

	// Users screen state
	usersItems      []userItem // Cached user list
	usersIndex      int        // Selected user index
	usersPane       int        // 0 = list pane, 1 = settings pane
	usersFieldIndex int        // Selected field in settings pane
	usersLoaded     bool       // Whether users have been loaded
	usersCreating   bool       // In "new user" input mode
	usersDeleting   bool       // In "confirm delete" mode
	usersNewName    string     // New user name being typed
	usersStatus     string     // Status message

	// Update screen async state
	updateChecking  bool          // Currently checking for updates
	updateCheckDone bool          // Check completed (use cached results)
	updateResults   []pkg.Package // Cached update results
	updateError     error         // Error from update check
	updateRunning   bool          // Currently running an update operation
	updateStatus    string        // Status message for current update operation
	updateSelected  map[int]bool  // Selected packages for batch update

	// Install/Update log streaming state
	installLogs          []string // Circular buffer of log lines (max 500)
	installLogScroll     int      // Scroll position in log buffer (0 = bottom)
	installLogAutoScroll bool     // Auto-scroll to bottom during active install

	// Error state
	lastError error
}

// AppOption configures optional App parameters
type AppOption func(*App)

// WithScreenFactory sets the screen factory for the ScreenManager
func WithScreenFactory(factory ScreenFactory) AppOption {
	return func(a *App) {
		if factory != nil {
			deps := NewDependencies()
			ctx := NewScreenContext(deps)
			ctx.Theme = a.theme
			ctx.NavStyle = a.navStyle
			ctx.AnimationsEnabled = a.animationsEnabled
			a.screenMgr = NewScreenManager(ctx, factory)
		}
	}
}

// NewApp creates a new application instance
func NewApp(skipIntro bool, opts ...AppOption) *App {
	app := &App{
		skipIntro:            skipIntro,
		theme:                "catppuccin-mocha",
		navStyle:             "emacs",
		animationsEnabled:    true,
		runner:               runner.NewRunner(),
		installOutput:        make([]string, 0, 100),
		deepDiveConfig:       NewDeepDiveConfig(),
		manageConfig:         NewManageConfig(),
		managePane:           0,
		postIntroScreen:      ScreenWelcome,
		hotkeysReturn:        ScreenMainMenu,
		updateSelected:       make(map[int]bool),
		installLogs:          make([]string, 0, 500),
		installLogAutoScroll: true,
	}

	// Best-effort: load persisted global settings (theme + nav) if available.
	if cfg, err := config.LoadGlobalConfig(); err == nil && cfg != nil {
		if cfg.Theme != "" {
			app.theme = cfg.Theme
		}
		if cfg.NavStyle != "" {
			app.navStyle = cfg.NavStyle
		}
		app.animationsEnabled = !cfg.DisableAnimations
	}

	// Keep the theme picker cursor in sync with the persisted theme.
	app.syncThemeIndex()

	// Apply the theme colors to the UI
	SetTheme(app.theme)

	// Best-effort: load persisted management settings for deep-dive manager UI.
	if cfg, err := config.LoadToolConfig("manage", NewManageConfig); err == nil && cfg != nil {
		app.manageConfig = cfg
	}

	// Best-effort: load hotkeys favorites config.
	if hkCfg, err := config.LoadHotkeysConfig(); err == nil && hkCfg != nil {
		app.hotkeysFavorites = hkCfg
	} else {
		app.hotkeysFavorites = &config.HotkeysConfig{Users: make(map[string]*config.UserHotkeys)}
	}

	if skipIntro {
		app.screen = ScreenWelcome
		app.animationDone = true
	} else {
		app.screen = ScreenAnimation
	}

	// Apply options (e.g., screen factory)
	for _, opt := range opts {
		opt(app)
	}

	return app
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{}
	if a.animationsEnabled {
		cmds = append(cmds, tickUI())
	}
	if a.screen == ScreenAnimation {
		cmds = append(cmds, tickAnimation(), checkDurdraw())
	}
	// Start update check if starting directly on Update screen
	if a.screen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
		a.updateChecking = true
		cmds = append(cmds, checkUpdatesCmd())
	}
	// Start install cache loading if starting directly on Manage or Hotkeys screen
	if a.screen == ScreenManage || a.screen == ScreenHotkeys {
		if cmd := a.startInstallCacheLoad(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// tickMsg is sent on each animation frame
type tickMsg time.Time

// uiTickMsg is a global tick for small UI animations (spinners/widgets).
type uiTickMsg time.Time

// durdrawAvailableMsg indicates if durdraw is available
type durdrawAvailableMsg bool

// animationDoneMsg indicates the animation has finished
type animationDoneMsg struct{}

// installOutputMsg carries output from the installation
type installOutputMsg struct {
	line runner.OutputLine
}

// installDoneMsg indicates installation completed
type installDoneMsg struct {
	err     error
	context string // last few lines of output for error context
}

// installStartMsg triggers installation start
type installStartMsg struct{}

// sudoRequiredMsg indicates sudo is needed
type sudoRequiredMsg struct{}

// sudoCachedMsg indicates sudo credentials are ready
type sudoCachedMsg struct {
	err error
}

// updateCheckDoneMsg indicates the async update check completed
type updateCheckDoneMsg struct {
	updates []pkg.Package
	err     error
}

// installCacheDoneMsg indicates the async install cache loading completed
type installCacheDoneMsg struct {
	installed map[string]bool
}

// updateRunDoneMsg indicates an update operation completed
type updateRunDoneMsg struct {
	results []pkg.UpdateResult
	err     error
}

// installLogMsg carries a single log line from streaming install/update
type installLogMsg struct {
	line string
}

// manageSudoRequiredMsg indicates sudo is needed before manage install
type manageSudoRequiredMsg struct {
	toolID string
}

// updateSudoRequiredMsg indicates sudo is needed before update
type updateSudoRequiredMsg struct {
	packages []pkg.Package
	all      bool
}

// manageStartInstallMsg triggers streaming install after sudo is cached
type manageStartInstallMsg struct {
	toolID string
}

// updateStartMsg triggers streaming update after sudo is cached
type updateStartMsg struct {
	packages []pkg.Package
	all      bool
}

// BackupEntry represents a backup in the list
type BackupEntry struct {
	Name      string
	Timestamp time.Time
	FileCount int
	Size      int64 // bytes
	Path      string
}

// backupsLoadedMsg indicates the async backup list loading completed
type backupsLoadedMsg struct {
	backups []BackupEntry
	err     error
}

// backupRestoreDoneMsg indicates a restore operation completed
type backupRestoreDoneMsg struct {
	name  string
	count int
	err   error
}

// backupDeleteDoneMsg indicates a delete operation completed
type backupDeleteDoneMsg struct {
	name string
	err  error
}

// backupCreateDoneMsg indicates a new backup was created
type backupCreateDoneMsg struct {
	name string
	err  error
}

func tickAnimation() tea.Cmd {
	return tea.Tick(introAnimationTick, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func tickUI() tea.Cmd {
	return tea.Tick(uiTick, func(t time.Time) tea.Msg {
		return uiTickMsg(t)
	})
}

func checkDurdraw() tea.Cmd {
	return func() tea.Msg {
		available := DetectDurdraw()
		return durdrawAvailableMsg(available)
	}
}

// checkUpdatesCmd starts an async update check
func checkUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		updates, err := pkg.CheckDotfilesUpdates()
		return updateCheckDoneMsg{updates: updates, err: err}
	}
}

// loadInstallCacheCmd loads installation status for all tools asynchronously
// This uses batch checking where supported (brew list --versions) for better performance
func loadInstallCacheCmd() tea.Cmd {
	return func() tea.Msg {
		reg := tools.GetRegistry()
		all := reg.All()
		installed := make(map[string]bool, len(all)+3)

		// Try batch checking first (much faster than individual checks)
		mgr := pkg.DetectManager()
		platform := pkg.DetectPlatform()

		var installedPkgs map[string]bool
		if mgr != nil {
			// Get all installed packages in one call
			pkgList, err := mgr.ListInstalled()
			if err == nil {
				installedPkgs = make(map[string]bool, len(pkgList))
				for _, p := range pkgList {
					installedPkgs[p.Name] = true
				}
			}
		}

		// Check each tool
		for _, t := range all {
			found := false
			if installedPkgs != nil {
				// Use batch result - check if primary package is installed
				pkgs := t.Packages()[platform]
				if len(pkgs) == 0 {
					pkgs = t.Packages()["all"]
				}
				if len(pkgs) > 0 {
					if installedPkgs[pkgs[0]] {
						installed[t.ID()] = true
						found = true
					}
				}
			}
			// Fall back to IsInstalled() for tools not found via package manager
			// (e.g., flatpaks, AppImages, direct binaries)
			if !found {
				installed[t.ID()] = t.IsInstalled()
			}
		}

		// Check utility scripts in ~/.local/bin (always fast - just file existence)
		home := os.Getenv("HOME")
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		if home != "" {
			binDir := filepath.Join(home, ".local", "bin")
			for _, util := range []string{"hk", "caff", "sshh"} {
				_, err := os.Stat(filepath.Join(binDir, util))
				installed[util] = err == nil
			}
		}

		return installCacheDoneMsg{installed: installed}
	}
}

// runUpdateCmd updates specific packages
func runUpdateCmd(packages []pkg.Package) tea.Cmd {
	return func() tea.Msg {
		results := pkg.UpdatePackages(packages)
		return updateRunDoneMsg{results: results}
	}
}

// runUpdateAllCmd updates all outdated packages
func runUpdateAllCmd() tea.Cmd {
	return func() tea.Msg {
		err := pkg.UpdateAllPackages()
		return updateRunDoneMsg{err: err}
	}
}

// checkSudoAndUpdateCmd checks if sudo is needed and either prompts or starts update
func checkSudoAndUpdateCmd(packages []pkg.Package, all bool) tea.Cmd {
	return func() tea.Msg {
		mgr := pkg.DetectManager()
		if mgr == nil {
			return updateRunDoneMsg{err: fmt.Errorf("no package manager detected")}
		}

		// Check if sudo is needed and not cached
		if mgr.NeedsSudo() && !runner.CheckSudoCached() {
			return updateSudoRequiredMsg{packages: packages, all: all}
		}

		// Sudo not needed or already cached - start streaming update
		return updateStartMsg{packages: packages, all: all}
	}
}

// loadBackupsCmd loads the list of available backups asynchronously
func loadBackupsCmd() tea.Cmd {
	return func() tea.Msg {
		backupDir := filepath.Join(config.ConfigDir(), "backups")
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			if os.IsNotExist(err) {
				return backupsLoadedMsg{backups: []BackupEntry{}}
			}
			return backupsLoadedMsg{err: err}
		}

		var backups []BackupEntry
		for _, entry := range entries {
			if entry.IsDir() {
				info, _ := entry.Info()
				path := filepath.Join(backupDir, entry.Name())
				count := countBackupFiles(path)
				size := calcDirSize(path)

				timestamp := time.Time{}
				if info != nil {
					timestamp = info.ModTime()
				}

				backups = append(backups, BackupEntry{
					Name:      entry.Name(),
					Timestamp: timestamp,
					FileCount: count,
					Size:      size,
					Path:      path,
				})
			}
		}

		// Sort by timestamp descending (newest first)
		sort.Slice(backups, func(i, j int) bool {
			return backups[i].Timestamp.After(backups[j].Timestamp)
		})

		return backupsLoadedMsg{backups: backups}
	}
}

// countBackupFiles counts files in a backup directory
func countBackupFiles(path string) int {
	count := 0
	filepath.Walk(path, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

// calcDirSize calculates the total size of files in a directory
func calcDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// formatBytes formats a byte count into a human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// restoreBackupCmd restores files from a backup
func restoreBackupCmd(backup BackupEntry) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(backup.Path)
		if err != nil {
			return backupRestoreDoneMsg{name: backup.Name, err: err}
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return backupRestoreDoneMsg{name: backup.Name, err: err}
		}

		restored := 0
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "manifest.txt" {
				continue
			}

			srcPath := filepath.Join(backup.Path, entry.Name())
			// Backup files are named with underscores replacing slashes
			relPath := strings.ReplaceAll(entry.Name(), "_", string(os.PathSeparator))
			dstPath := filepath.Clean(filepath.Join(home, relPath))

			// Security: Prevent path traversal attacks
			if !strings.HasPrefix(dstPath, home+string(os.PathSeparator)) && dstPath != home {
				continue
			}

			// Ensure destination directory exists
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				continue
			}

			// Read source and write to destination
			data, err := os.ReadFile(srcPath)
			if err != nil {
				continue
			}

			if err := os.WriteFile(dstPath, data, 0600); err != nil {
				continue
			}

			restored++
		}

		return backupRestoreDoneMsg{name: backup.Name, count: restored, err: nil}
	}
}

// deleteBackupCmd deletes a backup directory
func deleteBackupCmd(backup BackupEntry) tea.Cmd {
	return func() tea.Msg {
		err := os.RemoveAll(backup.Path)
		return backupDeleteDoneMsg{name: backup.Name, err: err}
	}
}

// createBackupCmd creates a new backup of current dotfiles
func createBackupCmd() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return backupCreateDoneMsg{err: err}
		}

		// Create backup directory with timestamp
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		backupDir := filepath.Join(config.ConfigDir(), "backups", timestamp)
		if err := os.MkdirAll(backupDir, 0700); err != nil {
			return backupCreateDoneMsg{err: err}
		}

		// Files to backup (relative to home)
		filesToBackup := []string{
			".zshrc",
			".tmux.conf",
			".config/nvim/init.lua",
			".config/ghostty/config",
			".config/yazi/yazi.toml",
			".gitconfig",
		}

		backedUp := []string{}
		for _, relPath := range filesToBackup {
			srcPath := filepath.Join(home, relPath)
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				continue
			}

			data, err := os.ReadFile(srcPath)
			if err != nil {
				continue
			}

			// Replace path separators with underscores for flat storage
			safeName := strings.ReplaceAll(relPath, string(os.PathSeparator), "_")
			dstPath := filepath.Join(backupDir, safeName)

			if err := os.WriteFile(dstPath, data, 0600); err != nil {
				continue
			}

			backedUp = append(backedUp, relPath)
		}

		// Write manifest
		manifest := strings.Join(backedUp, "\n")
		manifestPath := filepath.Join(backupDir, "manifest.txt")
		os.WriteFile(manifestPath, []byte(manifest), 0600)

		return backupCreateDoneMsg{name: timestamp, err: nil}
	}
}

// Update handles messages
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window resize for screen manager
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		a.width = wsm.Width
		a.height = wsm.Height
		if a.screenMgr != nil {
			a.screenMgr.SetSize(wsm.Width, wsm.Height)
		}
		return a, nil
	}

	// Delegate to screen manager for navigation messages and migrated screens
	if a.screenMgr != nil {
		if cmd, handled := a.screenMgr.Update(msg); handled {
			// Sync legacy screen field with manager's current legacy screen
			if a.screenMgr.IsLegacyMode() {
				a.screen = a.screenMgr.LegacyScreen()
			}
			return a, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKey(msg)

	case tea.MouseMsg:
		return a.handleMouse(msg)

	case tickMsg:
		if a.screen == ScreenAnimation {
			// If we don't have a window size yet, don't advance frames. This prevents
			// the intro from "fast-forwarding" on terminals that deliver WindowSizeMsg
			// a little late.
			if a.width == 0 || a.height == 0 {
				return a, tickAnimation()
			}

			a.animFrame++
			// Animation runs for a short burst and then transitions to the wizard.
			if a.animFrame >= introAnimationFrames {
				a.animationDone = true
				a.screen = a.postIntroScreen
				// Trigger update check if transitioning to Update screen
				if a.postIntroScreen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
					a.updateChecking = true
					return a, checkUpdatesCmd()
				}
				// Trigger install cache load if transitioning to Manage or Hotkeys screen
				if a.postIntroScreen == ScreenManage || a.postIntroScreen == ScreenHotkeys {
					if cmd := a.startInstallCacheLoad(); cmd != nil {
						return a, cmd
					}
				}
				return a, nil
			}
			return a, tickAnimation()
		}

	case uiTickMsg:
		if !a.animationsEnabled {
			return a, nil
		}
		a.uiFrame++
		if a.screenMgr != nil {
			a.screenMgr.IncrementUIFrame()
		}
		return a, tickUI()

	case durdrawAvailableMsg:
		// Store durdraw availability if needed
		return a, nil

	case animationDoneMsg:
		a.animationDone = true
		a.screen = a.postIntroScreen
		// Trigger update check if transitioning to Update screen
		if a.postIntroScreen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return a, checkUpdatesCmd()
		}
		// Trigger install cache load if transitioning to Manage or Hotkeys screen
		if a.postIntroScreen == ScreenManage || a.postIntroScreen == ScreenHotkeys {
			if cmd := a.startInstallCacheLoad(); cmd != nil {
				return a, cmd
			}
		}
		return a, nil

	case sudoRequiredMsg:
		// Need to prompt for sudo - use tea.Exec to exit alt screen
		return a, tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
			return sudoCachedMsg{err: err}
		})

	case sudoCachedMsg:
		if msg.err != nil {
			a.lastError = msg.err
			a.screen = ScreenError
			return a, nil
		}
		// Sudo cached successfully, start installation
		return a, a.startInstallation()

	case installStartMsg:
		// Check if we need sudo first
		if runner.NeedsSudo() && !runner.CheckSudoCached() {
			return a, func() tea.Msg { return sudoRequiredMsg{} }
		}
		return a, a.startInstallation()

	case installOutputMsg:
		a.installOutput = append(a.installOutput, msg.line.Text)
		// Keep only the last 20 lines for display (using copy to avoid memory leak)
		const maxOutputLines = 20
		if len(a.installOutput) > maxOutputLines {
			copy(a.installOutput, a.installOutput[len(a.installOutput)-maxOutputLines:])
			a.installOutput = a.installOutput[:maxOutputLines]
		}
		// Update step based on output type
		if msg.line.Type == runner.OutputStep {
			a.installStep++
		}
		return a, nil

	case installDoneMsg:
		a.installRunning = false
		a.installComplete = true
		if msg.err != nil {
			// Include context in error message for better debugging
			if msg.context != "" {
				a.lastError = fmt.Errorf("%v\n\nOutput:\n%s", msg.err, msg.context)
			} else {
				a.lastError = msg.err
			}
			a.screen = ScreenError
		}
		return a, nil

	case manageSavedMsg:
		if msg.err != nil {
			a.manageStatus = fmt.Sprintf("Save failed: %v", msg.err)
		} else {
			a.manageStatus = "Saved ✓"
		}
		return a, nil

	case manageInstallDoneMsg:
		a.manageInstalling = false
		a.manageInstallID = ""
		a.manageInstalledReady = false // refresh install status cache
		if msg.err != nil {
			a.manageStatus = fmt.Sprintf("Install failed: %v", msg.err)
		} else {
			a.manageStatus = "Installed ✓"
		}
		return a, nil

	case updateCheckDoneMsg:
		a.updateChecking = false
		a.updateCheckDone = true
		a.updateResults = msg.updates
		a.updateError = msg.err
		return a, nil

	case userLoadedMsg:
		if msg.err != nil {
			a.usersStatus = fmt.Sprintf("Load failed: %v", msg.err)
		} else {
			a.usersItems = msg.users
			a.usersStatus = ""
		}
		return a, nil

	case userSavedMsg:
		if msg.err != nil {
			a.usersStatus = fmt.Sprintf("Save failed: %v", msg.err)
		} else {
			a.usersStatus = fmt.Sprintf("Saved %s ✓", msg.name)
			// Reload user list
			return a, loadUsersCmd()
		}
		return a, nil

	case userDeletedMsg:
		if msg.err != nil {
			a.usersStatus = fmt.Sprintf("Delete failed: %v", msg.err)
		} else {
			a.usersStatus = fmt.Sprintf("Deleted %s", msg.name)
			// Reload user list and adjust index
			if a.usersIndex > 0 {
				a.usersIndex--
			}
			return a, loadUsersCmd()
		}
		return a, nil

	case userSwitchedMsg:
		if msg.err != nil {
			a.usersStatus = fmt.Sprintf("Switch failed: %v", msg.err)
		} else {
			a.usersStatus = fmt.Sprintf("Switched to %s ✓", msg.name)
			// Reload user list to update active indicator
			return a, loadUsersCmd()
		}
		return a, nil

	case installCacheDoneMsg:
		a.manageInstalled = msg.installed
		a.manageInstalledReady = true
		a.installCacheLoading = false
		return a, nil

	case updateRunDoneMsg:
		a.updateRunning = false
		a.installLogAutoScroll = false // Allow user to scroll through logs
		if msg.err != nil {
			a.updateStatus = fmt.Sprintf("Update failed: %v", msg.err)
		} else {
			// Count successes and failures
			successes := 0
			failures := 0
			for _, r := range msg.results {
				if r.Success {
					successes++
				} else {
					failures++
				}
			}
			if failures > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d, failed %d", successes, failures)
			} else if successes > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d package(s) ✓", successes)
			} else {
				a.updateStatus = "Update complete ✓"
			}
			// Clear selections and refresh the package list
			a.updateSelected = make(map[int]bool)
			a.updateCheckDone = false
			a.updateChecking = true
			return a, checkUpdatesCmd()
		}
		return a, nil

	case installLogMsg:
		a.appendInstallLog(msg.line)
		return a, nil

	case manageSudoRequiredMsg:
		// Need to prompt for sudo before manage install
		return a, tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
			if err != nil {
				return manageInstallDoneMsg{toolID: msg.toolID, err: err}
			}
			// Sudo cached, now start the streaming install
			return manageStartInstallMsg{toolID: msg.toolID}
		})

	case updateSudoRequiredMsg:
		// Need to prompt for sudo before update
		return a, tea.Exec(sudoPromptCmd(), func(err error) tea.Msg {
			if err != nil {
				return updateRunDoneMsg{err: err}
			}
			// Sudo cached, now start the streaming update
			return updateStartMsg{packages: msg.packages, all: msg.all}
		})

	case manageStartInstallMsg:
		// Start the streaming install (sudo already cached)
		a.clearInstallLogs()
		a.manageInstalling = true
		a.manageInstallID = msg.toolID
		return a, a.streamingInstallToolCmd(msg.toolID)

	case updateStartMsg:
		// Start the streaming update (sudo already cached)
		a.clearInstallLogs()
		a.updateRunning = true
		if msg.all {
			return a, a.streamingUpdateAllCmd()
		}
		return a, a.streamingUpdateCmd(msg.packages)

	case manageInstallWithLogsMsg:
		// Install completed with logs
		a.manageInstalling = false
		a.installLogAutoScroll = false
		// Append all logs
		for _, line := range msg.logs {
			a.appendInstallLog(line)
		}
		// Update install status
		if msg.err != nil {
			a.manageStatus = fmt.Sprintf("Install failed: %v", msg.err)
		} else {
			a.manageStatus = "Installed successfully ✓"
			// Refresh install status cache
			a.manageInstalledReady = false
		}
		return a, nil

	case updateWithLogsMsg:
		// Update completed with logs
		a.updateRunning = false
		a.installLogAutoScroll = false
		// Append all logs
		for _, line := range msg.logs {
			a.appendInstallLog(line)
		}
		// Process results
		if msg.err != nil {
			a.updateStatus = fmt.Sprintf("Update failed: %v", msg.err)
		} else {
			successes := 0
			failures := 0
			for _, r := range msg.results {
				if r.Success {
					successes++
				} else {
					failures++
				}
			}
			if failures > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d, failed %d", successes, failures)
			} else if successes > 0 {
				a.updateStatus = fmt.Sprintf("Updated %d package(s) ✓", successes)
			} else {
				a.updateStatus = "Update complete ✓"
			}
			// Clear selections and refresh the package list
			a.updateSelected = make(map[int]bool)
			a.updateCheckDone = false
			a.updateChecking = true
			return a, checkUpdatesCmd()
		}
		return a, nil

	case backupsLoadedMsg:
		a.backupsLoading = false
		a.backupsLoaded = true
		if msg.err != nil {
			a.backupError = msg.err
			a.backups = []BackupEntry{}
		} else {
			a.backups = msg.backups
			a.backupError = nil
		}
		return a, nil

	case backupRestoreDoneMsg:
		a.backupRunning = false
		a.backupConfirmMode = false
		if msg.err != nil {
			a.backupStatus = fmt.Sprintf("Restore failed: %v", msg.err)
		} else {
			a.backupStatus = fmt.Sprintf("Restored %d files from %s", msg.count, msg.name)
		}
		return a, nil

	case backupDeleteDoneMsg:
		a.backupRunning = false
		a.backupConfirmMode = false
		if msg.err != nil {
			a.backupStatus = fmt.Sprintf("Delete failed: %v", msg.err)
		} else {
			a.backupStatus = fmt.Sprintf("Deleted backup: %s", msg.name)
			// Adjust index if needed
			if a.backupIndex > 0 && a.backupIndex >= len(a.backups)-1 {
				a.backupIndex--
			}
			// Refresh backup list
			a.backupsLoaded = false
			a.backupsLoading = true
			return a, loadBackupsCmd()
		}
		return a, nil

	case backupCreateDoneMsg:
		a.backupRunning = false
		if msg.err != nil {
			a.backupStatus = fmt.Sprintf("Backup failed: %v", msg.err)
		} else {
			a.backupStatus = fmt.Sprintf("Created backup: %s", msg.name)
			// Refresh backup list
			a.backupsLoaded = false
			a.backupsLoading = true
			return a, loadBackupsCmd()
		}
		return a, nil
	}

	return a, nil
}

// appendInstallLog adds a line to the install log buffer (max 500 lines)
func (a *App) appendInstallLog(line string) {
	const maxLogLines = 500
	a.installLogs = append(a.installLogs, line)
	// Use copy to avoid memory leak from reslicing
	if len(a.installLogs) > maxLogLines {
		copy(a.installLogs, a.installLogs[len(a.installLogs)-maxLogLines:])
		a.installLogs = a.installLogs[:maxLogLines]
	}
	// Auto-scroll to bottom if enabled
	if a.installLogAutoScroll {
		a.installLogScroll = 0 // 0 = bottom in our scroll model
	}
}

// clearInstallLogs clears the log buffer and resets scroll
func (a *App) clearInstallLogs() {
	a.installLogs = make([]string, 0, 500)
	a.installLogScroll = 0
	a.installLogAutoScroll = true
}

func (a *App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case ScreenMainMenu:
		return a.handleMainMenuMouse(msg)
	case ScreenManage:
		return a.handleManageMouse(msg)
	case ScreenHotkeys:
		return a.handleHotkeysMouse(msg)
	case ScreenUsers:
		return a.handleUsersMouse(msg)
	case ScreenBackups:
		return a.handleBackupsMouse(msg)
	case ScreenUpdate:
		return a.handleTabBarMouse(msg)
	case ScreenWelcome:
		return a.handleWelcomeMouse(msg)
	case ScreenThemePicker:
		return a.handleThemePickerMouse(msg)
	case ScreenNavPicker:
		return a.handleNavPickerMouse(msg)
	case ScreenDeepDiveMenu:
		return a.handleDeepDiveMenuMouse(msg)
	case ScreenFileTree, ScreenSummary:
		return a.handleSummaryMouse(msg)
	case ScreenConfigGhostty, ScreenConfigTmux, ScreenConfigZsh, ScreenConfigNeovim,
		ScreenConfigGit, ScreenConfigYazi, ScreenConfigFzf, ScreenConfigUtilities,
		ScreenConfigMacApps, ScreenConfigApps, ScreenConfigCLITools, ScreenConfigGUIApps,
		ScreenConfigCLIUtilities, ScreenConfigLazyGit, ScreenConfigLazyDocker,
		ScreenConfigBtop, ScreenConfigGlow:
		return a.handleConfigScreenMouse(msg)
	default:
		return a, nil
	}
}

// handleTabBarMouse handles mouse clicks on the tab bar for screens that use it
func (a *App) handleTabBarMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Only handle left clicks
	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Tab bar is at Y=0 (first line)
	if m.Y != 0 {
		return a, nil
	}

	// Check if click is on a tab
	if screen, cmd := a.detectTabClick(m.X); screen != 0 {
		a.screen = screen
		return a, cmd
	}

	return a, nil
}

// detectTabClick determines which tab was clicked based on X position
// Returns the target screen and any command to run, or (0, nil) if no tab clicked
func (a *App) detectTabClick(x int) (Screen, tea.Cmd) {
	tabs := GetManagementTabs()
	if len(tabs) == 0 {
		return 0, nil
	}

	// All screens now use unified RenderTabBar format: "N 󰒓 Name" with Padding(0,1)
	var tabWidths []int
	for i, tab := range tabs {
		content := fmt.Sprintf("%d %s %s", i+1, tab.Icon, tab.Name)
		// Padding(0, 1) = 1 space each side = 2 total
		width := lipgloss.Width(content) + 2
		tabWidths = append(tabWidths, width)
	}

	// Tab bar is left-aligned (Width renders left-aligned by default)
	// Account for 1-char separator " " between tabs
	currentX := 0
	for i, tab := range tabs {
		endX := currentX + tabWidths[i]

		if x >= currentX && x < endX {
			// Don't switch if already on this screen
			if tab.Screen == a.screen {
				return 0, nil
			}

			// Start async update check when switching to Update screen
			if tab.Screen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
				a.updateChecking = true
				return tab.Screen, checkUpdatesCmd()
			}

			return tab.Screen, nil
		}

		// Move past tab width + separator (1 char)
		currentX = endX + 1
	}

	return 0, nil
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global quit handlers
	if key == "ctrl+c" {
		return a, tea.Quit
	}

	// 'q' quits from any screen except during installation
	if key == "q" && !a.installRunning && !(a.screen == ScreenManage && a.manageEditing) {
		return a, tea.Quit
	}

	// Screen-specific key handling
	switch a.screen {
	case ScreenAnimation:
		// Any key skips animation
		a.animationDone = true
		a.screen = a.postIntroScreen
		// Trigger update check if transitioning to Update screen
		if a.postIntroScreen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return a, checkUpdatesCmd()
		}
		// Trigger install cache load if transitioning to Manage or Hotkeys screen
		if a.postIntroScreen == ScreenManage || a.postIntroScreen == ScreenHotkeys {
			if cmd := a.startInstallCacheLoad(); cmd != nil {
				return a, cmd
			}
		}
		return a, nil

	case ScreenWelcome:
		switch key {
		case "enter":
			if a.deepDive {
				a.screen = ScreenDeepDiveMenu
				// Start async install cache load for deep dive menu
				if cmd := a.startInstallCacheLoad(); cmd != nil {
					return a, cmd
				}
			} else {
				a.screen = ScreenThemePicker
			}
		case "tab", "left", "right", "h", "l":
			a.deepDive = !a.deepDive
		}

	case ScreenThemePicker:
		switch key {
		case "up", "k":
			if a.themeIndex > 0 {
				a.themeIndex--
				a.theme = themes[a.themeIndex].name
				SetTheme(a.theme) // Apply theme immediately for live preview
			}
		case "down", "j":
			if a.themeIndex < len(themes)-1 {
				a.themeIndex++
				a.theme = themes[a.themeIndex].name
				SetTheme(a.theme) // Apply theme immediately for live preview
			}
		case "enter":
			a.screen = ScreenNavPicker
		case "esc":
			a.screen = ScreenWelcome
		}

	case ScreenNavPicker:
		switch key {
		case "left", "right", "h", "l", "tab":
			if a.navStyle == "emacs" {
				a.navStyle = "vim"
			} else {
				a.navStyle = "emacs"
			}
		case "enter":
			a.screen = ScreenFileTree
		case "esc":
			a.screen = ScreenThemePicker
		}

	case ScreenFileTree:
		switch key {
		case "enter":
			a.screen = ScreenProgress
			return a, func() tea.Msg { return installStartMsg{} }
		case "esc":
			a.screen = ScreenNavPicker
		}

	case ScreenProgress:
		switch key {
		case "enter":
			// Only advance if installation is complete
			if !a.installRunning {
				a.screen = ScreenSummary
			}
		}

	case ScreenSummary:
		switch key {
		case "enter", "q":
			return a, tea.Quit
		}

	case ScreenError:
		switch key {
		case "r":
			// Retry - go back to progress
			a.screen = ScreenProgress
		case "s":
			// Skip - continue to summary
			a.screen = ScreenSummary
		case "q":
			return a, tea.Quit
		case "esc":
			a.screen = ScreenFileTree
		}

	// Deep dive menu navigation
	case ScreenDeepDiveMenu:
		menuItems := GetFilteredDeepDiveMenuItems()
		maxIdx := len(menuItems) // +1 for "Continue" option
		switch key {
		case "up", "k":
			if a.deepDiveMenuIndex > 0 {
				a.deepDiveMenuIndex--
			}
		case "down", "j":
			if a.deepDiveMenuIndex < maxIdx {
				a.deepDiveMenuIndex++
			}
		case "enter":
			if a.deepDiveMenuIndex == maxIdx {
				// "Continue to Installation" selected
				a.screen = ScreenThemePicker
			} else {
				// Navigate to specific config screen
				a.screen = menuItems[a.deepDiveMenuIndex].Screen
			}
		case "esc":
			a.screen = ScreenWelcome
		}

	// Ghostty config
	// Fields: 0=font family, 1=font size, 2=opacity, 3=blur, 4=scrollback, 5=cursor style, 6=tab bindings
	case ScreenConfigGhostty:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 6 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 0: // Font family
				opts := []string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"}
				a.deepDiveConfig.GhosttyFontFamily = cycleOption(opts, a.deepDiveConfig.GhosttyFontFamily, false)
			case 1: // Font size
				if a.deepDiveConfig.GhosttyFontSize > 8 {
					a.deepDiveConfig.GhosttyFontSize--
				}
			case 2: // Opacity
				if a.deepDiveConfig.GhosttyOpacity > 0 {
					a.deepDiveConfig.GhosttyOpacity -= 5
				}
			case 3: // Blur radius
				if a.deepDiveConfig.GhosttyBlurRadius > 0 {
					a.deepDiveConfig.GhosttyBlurRadius -= 5
				}
			case 4: // Scrollback lines
				opts := []string{"1000", "5000", "10000", "50000", "100000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.GhosttyScrollbackLines)
				result := cycleOption(opts, current, false)
				a.deepDiveConfig.GhosttyScrollbackLines = atoi(result, 10000)
			case 5: // Cursor style
				opts := []string{"block", "bar", "underline"}
				a.deepDiveConfig.GhosttyCursorStyle = cycleOption(opts, a.deepDiveConfig.GhosttyCursorStyle, false)
			case 6: // Tab bindings
				opts := []string{"super", "ctrl", "alt"}
				a.deepDiveConfig.GhosttyTabBindings = cycleOption(opts, a.deepDiveConfig.GhosttyTabBindings, false)
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 0: // Font family
				opts := []string{"JetBrains Mono", "Fira Code", "Hack", "Menlo", "Monaco"}
				a.deepDiveConfig.GhosttyFontFamily = cycleOption(opts, a.deepDiveConfig.GhosttyFontFamily, true)
			case 1: // Font size
				if a.deepDiveConfig.GhosttyFontSize < 32 {
					a.deepDiveConfig.GhosttyFontSize++
				}
			case 2: // Opacity
				if a.deepDiveConfig.GhosttyOpacity < 100 {
					a.deepDiveConfig.GhosttyOpacity += 5
				}
			case 3: // Blur radius
				if a.deepDiveConfig.GhosttyBlurRadius < 100 {
					a.deepDiveConfig.GhosttyBlurRadius += 5
				}
			case 4: // Scrollback lines
				opts := []string{"1000", "5000", "10000", "50000", "100000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.GhosttyScrollbackLines)
				result := cycleOption(opts, current, true)
				a.deepDiveConfig.GhosttyScrollbackLines = atoi(result, 10000)
			case 5: // Cursor style
				opts := []string{"block", "bar", "underline"}
				a.deepDiveConfig.GhosttyCursorStyle = cycleOption(opts, a.deepDiveConfig.GhosttyCursorStyle, true)
			case 6: // Tab bindings
				opts := []string{"super", "ctrl", "alt"}
				a.deepDiveConfig.GhosttyTabBindings = cycleOption(opts, a.deepDiveConfig.GhosttyTabBindings, true)
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Tmux config
	// Fields: 0=prefix, 1=splits, 2=status, 3=mouse, 4=history, 5=escape, 6=base, 7=TPM toggle
	// If TPM enabled: 8=sensible, 9=resurrect, 10=continuum, 11=yank, 12=interval (if continuum)
	case ScreenConfigTmux:
		// Calculate max field based on TPM state
		maxField := 7 // Fields 0-7 (basic settings + TPM toggle)
		if a.deepDiveConfig.TmuxTPMEnabled {
			maxField = 11 // + plugin toggles (8-11)
			if a.deepDiveConfig.TmuxPluginContinuum {
				maxField = 12 // + continuum interval
			}
		}

		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
				// Skip hidden fields when TPM is disabled
				if !a.deepDiveConfig.TmuxTPMEnabled && a.configFieldIndex > 7 {
					a.configFieldIndex = 7
				}
				// Skip interval field when continuum is disabled
				if a.deepDiveConfig.TmuxTPMEnabled && !a.deepDiveConfig.TmuxPluginContinuum && a.configFieldIndex == 12 {
					a.configFieldIndex = 11
				}
			}
		case "down", "j":
			if a.configFieldIndex < maxField {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			fwd := key == "right" || key == "l"
			switch a.configFieldIndex {
			case 0: // Prefix
				opts := []string{"ctrl-a", "ctrl-b", "ctrl-space"}
				a.deepDiveConfig.TmuxPrefix = cycleOption(opts, a.deepDiveConfig.TmuxPrefix, fwd)
			case 1: // Split binds
				if a.deepDiveConfig.TmuxSplitBinds == "pipes" {
					a.deepDiveConfig.TmuxSplitBinds = "percent"
				} else {
					a.deepDiveConfig.TmuxSplitBinds = "pipes"
				}
			case 2: // Status bar
				if a.deepDiveConfig.TmuxStatusBar == "top" {
					a.deepDiveConfig.TmuxStatusBar = "bottom"
				} else {
					a.deepDiveConfig.TmuxStatusBar = "top"
				}
			case 4: // History limit
				opts := []string{"10000", "25000", "50000", "100000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.TmuxHistoryLimit)
				a.deepDiveConfig.TmuxHistoryLimit = atoi(cycleOption(opts, current, fwd), 50000)
			case 5: // Escape time
				opts := []string{"0", "10", "50", "100"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.TmuxEscapeTime)
				a.deepDiveConfig.TmuxEscapeTime = atoi(cycleOption(opts, current, fwd), 10)
			case 6: // Base index
				if a.deepDiveConfig.TmuxBaseIndex == 0 {
					a.deepDiveConfig.TmuxBaseIndex = 1
				} else {
					a.deepDiveConfig.TmuxBaseIndex = 0
				}
			case 12: // Continuum interval
				if a.deepDiveConfig.TmuxTPMEnabled && a.deepDiveConfig.TmuxPluginContinuum {
					if fwd {
						if a.deepDiveConfig.TmuxContinuumSaveMin < 60 {
							a.deepDiveConfig.TmuxContinuumSaveMin += 5
						}
					} else {
						if a.deepDiveConfig.TmuxContinuumSaveMin > 5 {
							a.deepDiveConfig.TmuxContinuumSaveMin -= 5
						}
					}
				}
			}
		case " ":
			switch a.configFieldIndex {
			case 3: // Mouse mode
				a.deepDiveConfig.TmuxMouseMode = !a.deepDiveConfig.TmuxMouseMode
			case 7: // TPM enabled
				a.deepDiveConfig.TmuxTPMEnabled = !a.deepDiveConfig.TmuxTPMEnabled
			case 8: // tmux-sensible
				a.deepDiveConfig.TmuxPluginSensible = !a.deepDiveConfig.TmuxPluginSensible
			case 9: // tmux-resurrect
				a.deepDiveConfig.TmuxPluginResurrect = !a.deepDiveConfig.TmuxPluginResurrect
			case 10: // tmux-continuum
				a.deepDiveConfig.TmuxPluginContinuum = !a.deepDiveConfig.TmuxPluginContinuum
			case 11: // tmux-yank
				a.deepDiveConfig.TmuxPluginYank = !a.deepDiveConfig.TmuxPluginYank
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Zsh config
	// Fields: 0-3=prompts, 4=history, 5=autocd, 6=syntax, 7=autosuggestions, 8-12=plugins
	case ScreenConfigZsh:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 12 { // 4 prompts + 4 shell options + 5 plugins - 1
				a.configFieldIndex++
			}
		case "left", "right", "h", "l", " ":
			fwd := key == "right" || key == "l"
			if a.configFieldIndex < 4 {
				// Prompt style selection
				opts := []string{"p10k", "starship", "pure", "minimal"}
				a.deepDiveConfig.ZshPromptStyle = opts[a.configFieldIndex]
			} else if a.configFieldIndex == 4 {
				// History size
				opts := []string{"1000", "5000", "10000", "50000"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.ZshHistorySize)
				a.deepDiveConfig.ZshHistorySize = atoi(cycleOption(opts, current, fwd), 10000)
			} else if a.configFieldIndex == 5 {
				// Auto CD
				a.deepDiveConfig.ZshAutoCD = !a.deepDiveConfig.ZshAutoCD
			} else if a.configFieldIndex == 6 {
				// Syntax highlighting
				a.deepDiveConfig.ZshSyntaxHighlight = !a.deepDiveConfig.ZshSyntaxHighlight
			} else if a.configFieldIndex == 7 {
				// Autosuggestions
				a.deepDiveConfig.ZshAutosuggestions = !a.deepDiveConfig.ZshAutosuggestions
			} else {
				// Plugin toggle
				plugins := []string{"zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions", "fzf-tab", "zsh-history-substring-search"}
				pluginIdx := a.configFieldIndex - 8
				if pluginIdx >= 0 && pluginIdx < len(plugins) {
					togglePlugin(&a.deepDiveConfig.ZshPlugins, plugins[pluginIdx])
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Neovim config
	// Fields: 0-3=configs, 4=tabwidth, 5=wrap, 6=cursorline, 7=clipboard, 8-13=LSPs
	case ScreenConfigNeovim:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 13 { // 4 configs + 4 editor settings + 6 LSPs - 1
				a.configFieldIndex++
			}
		case "left", "right", "h", "l", " ":
			fwd := key == "right" || key == "l"
			if a.configFieldIndex < 4 {
				// Config preset selection
				opts := []string{"kickstart", "lazyvim", "nvchad", "custom"}
				a.deepDiveConfig.NeovimConfig = opts[a.configFieldIndex]
			} else if a.configFieldIndex == 4 {
				// Tab width
				opts := []string{"2", "4", "8"}
				current := fmt.Sprintf("%d", a.deepDiveConfig.NeovimTabWidth)
				a.deepDiveConfig.NeovimTabWidth = atoi(cycleOption(opts, current, fwd), 4)
			} else if a.configFieldIndex == 5 {
				// Wrap
				a.deepDiveConfig.NeovimWrap = !a.deepDiveConfig.NeovimWrap
			} else if a.configFieldIndex == 6 {
				// Cursor line
				a.deepDiveConfig.NeovimCursorLine = !a.deepDiveConfig.NeovimCursorLine
			} else if a.configFieldIndex == 7 {
				// Clipboard
				opts := []string{"unnamedplus", "unnamed", "none"}
				a.deepDiveConfig.NeovimClipboard = cycleOption(opts, a.deepDiveConfig.NeovimClipboard, fwd)
			} else {
				// LSP toggle
				lsps := []string{"lua_ls", "pyright", "tsserver", "gopls", "rust_analyzer", "clangd"}
				lspIdx := a.configFieldIndex - 8
				if lspIdx >= 0 && lspIdx < len(lsps) {
					togglePlugin(&a.deepDiveConfig.NeovimLSPs, lsps[lspIdx])
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Git config
	// Fields: 0=delta, 1=branch, 2=rebase, 3=sign, 4=credential
	case ScreenConfigGit:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 4 {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			fwd := key == "right" || key == "l"
			switch a.configFieldIndex {
			case 1: // Default branch
				opts := []string{"main", "master", "develop"}
				a.deepDiveConfig.GitDefaultBranch = cycleOption(opts, a.deepDiveConfig.GitDefaultBranch, fwd)
			case 4: // Credential helper
				opts := []string{"cache", "store", "osxkeychain", "none"}
				a.deepDiveConfig.GitCredentialHelper = cycleOption(opts, a.deepDiveConfig.GitCredentialHelper, fwd)
			}
		case " ":
			switch a.configFieldIndex {
			case 0: // Delta side-by-side
				a.deepDiveConfig.GitDeltaSideBySide = !a.deepDiveConfig.GitDeltaSideBySide
			case 2: // Pull rebase
				a.deepDiveConfig.GitPullRebase = !a.deepDiveConfig.GitPullRebase
			case 3: // Sign commits
				a.deepDiveConfig.GitSignCommits = !a.deepDiveConfig.GitSignCommits
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Yazi config
	case ScreenConfigYazi:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			switch a.configFieldIndex {
			case 0:
				if a.deepDiveConfig.YaziKeymap == "vim" {
					a.deepDiveConfig.YaziKeymap = "emacs"
				} else {
					a.deepDiveConfig.YaziKeymap = "vim"
				}
			case 2:
				opts := []string{"auto", "always", "never"}
				a.deepDiveConfig.YaziPreviewMode = cycleOption(opts, a.deepDiveConfig.YaziPreviewMode, key == "right" || key == "l")
			}
		case " ":
			if a.configFieldIndex == 1 {
				a.deepDiveConfig.YaziShowHidden = !a.deepDiveConfig.YaziShowHidden
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// FZF config
	case ScreenConfigFzf:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 1:
				if a.deepDiveConfig.FzfHeight > 20 {
					a.deepDiveConfig.FzfHeight -= 10
				}
			case 2:
				opts := []string{"reverse", "default", "reverse-list"}
				a.deepDiveConfig.FzfLayout = cycleOption(opts, a.deepDiveConfig.FzfLayout, false)
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 1:
				if a.deepDiveConfig.FzfHeight < 100 {
					a.deepDiveConfig.FzfHeight += 10
				}
			case 2:
				opts := []string{"reverse", "default", "reverse-list"}
				a.deepDiveConfig.FzfLayout = cycleOption(opts, a.deepDiveConfig.FzfLayout, true)
			}
		case " ":
			if a.configFieldIndex == 0 {
				a.deepDiveConfig.FzfPreview = !a.deepDiveConfig.FzfPreview
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// macOS Apps config
	case ScreenConfigMacApps:
		apps := []string{"rectangle", "raycast", "stats", "alt-tab", "monitor-control", "mos", "karabiner", "iina", "the-unarchiver", "appcleaner"}
		switch key {
		case "up", "k":
			if a.macAppIndex > 0 {
				a.macAppIndex--
			}
		case "down", "j":
			if a.macAppIndex < len(apps)-1 {
				a.macAppIndex++
			}
		case " ":
			app := apps[a.macAppIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[app] {
				a.deepDiveConfig.MacApps[app] = !a.deepDiveConfig.MacApps[app]
			}
		case "esc", "enter":
			a.macAppIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Utilities config
	case ScreenConfigUtilities:
		utilities := []string{"hk", "caff", "sshh"}
		switch key {
		case "up", "k":
			if a.utilityIndex > 0 {
				a.utilityIndex--
			}
		case "down", "j":
			if a.utilityIndex < len(utilities)-1 {
				a.utilityIndex++
			}
		case " ":
			util := utilities[a.utilityIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[util] {
				a.deepDiveConfig.Utilities[util] = !a.deepDiveConfig.Utilities[util]
			}
		case "esc", "enter":
			a.utilityIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// CLI Tools config
	case ScreenConfigCLITools:
		tools := []string{"lazygit", "lazydocker", "btop", "glow", "claude-code"}
		switch key {
		case "up", "k":
			if a.cliToolIndex > 0 {
				a.cliToolIndex--
			}
		case "down", "j":
			if a.cliToolIndex < len(tools)-1 {
				a.cliToolIndex++
			}
		case " ":
			tool := tools[a.cliToolIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[tool] {
				a.deepDiveConfig.CLITools[tool] = !a.deepDiveConfig.CLITools[tool]
			}
		case "esc", "enter":
			a.cliToolIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// GUI Apps config
	case ScreenConfigGUIApps:
		apps := []string{"zen-browser", "cursor", "lm-studio", "obs"}
		switch key {
		case "up", "k":
			if a.guiAppIndex > 0 {
				a.guiAppIndex--
			}
		case "down", "j":
			if a.guiAppIndex < len(apps)-1 {
				a.guiAppIndex++
			}
		case " ":
			app := apps[a.guiAppIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[app] {
				a.deepDiveConfig.GUIApps[app] = !a.deepDiveConfig.GUIApps[app]
			}
		case "esc", "enter":
			a.guiAppIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// CLI Utilities config (bat, eza, zoxide, ripgrep, fd, delta, fswatch)
	case ScreenConfigCLIUtilities:
		utilities := []string{"bat", "eza", "zoxide", "ripgrep", "fd", "delta", "fswatch"}
		switch key {
		case "up", "k":
			if a.cliUtilityIndex > 0 {
				a.cliUtilityIndex--
			}
		case "down", "j":
			if a.cliUtilityIndex < len(utilities)-1 {
				a.cliUtilityIndex++
			}
		case " ":
			util := utilities[a.cliUtilityIndex]
			// Don't allow toggling if already installed
			if !a.manageInstalled[util] {
				a.deepDiveConfig.CLIUtilities[util] = !a.deepDiveConfig.CLIUtilities[util]
			}
		case "esc", "enter":
			a.cliUtilityIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// LazyGit config
	case ScreenConfigLazyGit:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			if a.configFieldIndex == 2 {
				opts := []string{"auto", "dark", "light"}
				a.deepDiveConfig.LazyGitTheme = cycleOption(opts, a.deepDiveConfig.LazyGitTheme, key == "right" || key == "l")
			}
		case " ":
			switch a.configFieldIndex {
			case 0:
				a.deepDiveConfig.LazyGitSideBySide = !a.deepDiveConfig.LazyGitSideBySide
			case 1:
				a.deepDiveConfig.LazyGitMouseMode = !a.deepDiveConfig.LazyGitMouseMode
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// LazyDocker config
	case ScreenConfigLazyDocker:
		switch key {
		case " ":
			a.deepDiveConfig.LazyDockerMouseMode = !a.deepDiveConfig.LazyDockerMouseMode
		case "esc", "enter":
			a.screen = ScreenDeepDiveMenu
		}

	// Btop config
	case ScreenConfigBtop:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 3 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}
				a.deepDiveConfig.BtopTheme = cycleOption(opts, a.deepDiveConfig.BtopTheme, false)
			case 1:
				if a.deepDiveConfig.BtopUpdateMs > 500 {
					a.deepDiveConfig.BtopUpdateMs -= 500
				}
			case 3:
				opts := []string{"braille", "block", "tty"}
				a.deepDiveConfig.BtopGraphType = cycleOption(opts, a.deepDiveConfig.BtopGraphType, false)
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dracula", "gruvbox", "nord", "tokyo-night"}
				a.deepDiveConfig.BtopTheme = cycleOption(opts, a.deepDiveConfig.BtopTheme, true)
			case 1:
				if a.deepDiveConfig.BtopUpdateMs < 10000 {
					a.deepDiveConfig.BtopUpdateMs += 500
				}
			case 3:
				opts := []string{"braille", "block", "tty"}
				a.deepDiveConfig.BtopGraphType = cycleOption(opts, a.deepDiveConfig.BtopGraphType, true)
			}
		case " ":
			if a.configFieldIndex == 2 {
				a.deepDiveConfig.BtopShowTemp = !a.deepDiveConfig.BtopShowTemp
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Glow config
	case ScreenConfigGlow:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 2 {
				a.configFieldIndex++
			}
		case "left", "h":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dark", "light", "notty"}
				a.deepDiveConfig.GlowStyle = cycleOption(opts, a.deepDiveConfig.GlowStyle, false)
			case 1:
				opts := []string{"auto", "less", "more", "none"}
				a.deepDiveConfig.GlowPager = cycleOption(opts, a.deepDiveConfig.GlowPager, false)
			case 2:
				if a.deepDiveConfig.GlowWidth > 40 {
					a.deepDiveConfig.GlowWidth -= 10
				}
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 0:
				opts := []string{"auto", "dark", "light", "notty"}
				a.deepDiveConfig.GlowStyle = cycleOption(opts, a.deepDiveConfig.GlowStyle, true)
			case 1:
				opts := []string{"auto", "less", "more", "none"}
				a.deepDiveConfig.GlowPager = cycleOption(opts, a.deepDiveConfig.GlowPager, true)
			case 2:
				if a.deepDiveConfig.GlowWidth < 200 {
					a.deepDiveConfig.GlowWidth += 10
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Main menu navigation
	case ScreenMainMenu:
		items := GetMainMenuItems()
		switch key {
		case "up", "k":
			if a.mainMenuIndex > 0 {
				a.mainMenuIndex--
			}
		case "down", "j":
			if a.mainMenuIndex < len(items)-1 {
				a.mainMenuIndex++
			}
		case "enter":
			targetScreen := items[a.mainMenuIndex].Screen
			a.screen = targetScreen
			// Start async operations for screens that need it
			switch targetScreen {
			case ScreenManage:
				if cmd := a.startInstallCacheLoad(); cmd != nil {
					return a, cmd
				}
			case ScreenBackups:
				if !a.backupsLoading && !a.backupsLoaded {
					a.backupsLoading = true
					return a, loadBackupsCmd()
				}
			case ScreenUpdate:
				if !a.updateChecking && !a.updateCheckDone {
					a.updateChecking = true
					return a, checkUpdatesCmd()
				}
			case ScreenUsers:
				if !a.usersLoaded {
					a.usersLoaded = true
					return a, loadUsersCmd()
				}
			}
		}

	// Update screen navigation
	case ScreenUpdate:
		// Start async update check if not already running or done
		if !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			return a, checkUpdatesCmd()
		}
		// Don't allow actions while update is running
		if a.updateRunning {
			return a, nil
		}
		// Handle tab navigation first
		if handled, cmd := a.handleTabNavigationWithCmd(key); handled {
			return a, cmd
		}
		switch key {
		case "up", "k":
			if a.updateIndex > 0 {
				a.updateIndex--
			}
		case "down", "j":
			a.updateIndex++
		case " ": // Toggle selection for batch update
			if len(a.updateResults) > 0 && a.updateIndex < len(a.updateResults) {
				if a.updateSelected[a.updateIndex] {
					delete(a.updateSelected, a.updateIndex)
				} else {
					a.updateSelected[a.updateIndex] = true
				}
			}
		case "enter": // Update selected or current package
			if len(a.updateResults) > 0 && !a.updateChecking && !a.updateRunning {
				var packagesToUpdate []pkg.Package
				if len(a.updateSelected) > 0 {
					// Update selected packages
					for idx := range a.updateSelected {
						if idx < len(a.updateResults) {
							packagesToUpdate = append(packagesToUpdate, a.updateResults[idx])
						}
					}
				} else if a.updateIndex < len(a.updateResults) {
					// Update current package
					packagesToUpdate = append(packagesToUpdate, a.updateResults[a.updateIndex])
				}
				if len(packagesToUpdate) > 0 {
					a.clearInstallLogs()
					a.updateStatus = fmt.Sprintf("Updating %d package(s)...", len(packagesToUpdate))
					return a, checkSudoAndUpdateCmd(packagesToUpdate, false)
				}
			}
		case "a": // Update all packages
			if len(a.updateResults) > 0 && !a.updateChecking && !a.updateRunning {
				a.clearInstallLogs()
				a.updateStatus = "Updating all packages..."
				return a, checkSudoAndUpdateCmd(nil, true)
			}
		case "r": // Refresh updates
			a.updateCheckDone = false
			a.updateChecking = true
			a.updateResults = nil
			a.updateError = nil
			a.updateStatus = ""
			a.updateSelected = make(map[int]bool)
			a.clearInstallLogs()
			return a, checkUpdatesCmd()
		case "c", "C": // Clear logs
			if !a.updateRunning && len(a.installLogs) > 0 {
				a.clearInstallLogs()
				a.updateStatus = "Logs cleared"
			}
		case "pgup", "ctrl+u": // Scroll logs up
			if len(a.installLogs) > 0 {
				a.installLogScroll += 10
				maxScroll := CalculateMaxLogScroll(len(a.installLogs), a.height-14)
				if a.installLogScroll > maxScroll {
					a.installLogScroll = maxScroll
				}
				a.installLogAutoScroll = false
			}
		case "pgdown", "ctrl+d": // Scroll logs down
			if len(a.installLogs) > 0 {
				a.installLogScroll -= 10
				if a.installLogScroll < 0 {
					a.installLogScroll = 0
				}
			}
		case "esc":
			a.screen = ScreenMainMenu
		}

	// Hotkeys screen navigation
	case ScreenHotkeys:
		return a.handleHotkeysKey(msg)

	// Manage screen navigation
	case ScreenManage:
		return a.handleManageKey(msg)

	// Management config screens
	case ScreenManageGhostty:
		maxFields := 7
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageTmux:
		maxFields := 7
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageZsh:
		maxFields := 6
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageNeovim:
		maxFields := 7
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageGit:
		maxFields := 6
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageYazi:
		maxFields := 4
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageFzf:
		maxFields := 4
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageLazyGit:
		maxFields := 3
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageLazyDocker:
		maxFields := 1
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageBtop:
		maxFields := 5
		a.handleManageNavigation(key, maxFields, ScreenManage)

	case ScreenManageGlow:
		maxFields := 3
		a.handleManageNavigation(key, maxFields, ScreenManage)

	// Users screen navigation
	case ScreenUsers:
		return a.handleUsersKey(msg)

	// Backups screen navigation
	case ScreenBackups:
		// Start async backup loading if not already running or done
		if !a.backupsLoading && !a.backupsLoaded {
			a.backupsLoading = true
			return a, loadBackupsCmd()
		}
		// Don't allow actions while backup operation is running
		if a.backupRunning {
			return a, nil
		}
		// Handle confirmation mode
		if a.backupConfirmMode {
			switch key {
			case "y", "Y":
				if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
					a.backupRunning = true
					backup := a.backups[a.backupIndex]
					if a.backupConfirmType == "restore" {
						return a, restoreBackupCmd(backup)
					} else if a.backupConfirmType == "delete" {
						return a, deleteBackupCmd(backup)
					}
				}
				a.backupConfirmMode = false
			case "n", "N", "esc":
				a.backupConfirmMode = false
				a.backupStatus = ""
			}
			return a, nil
		}
		// Handle tab navigation first
		if handled, cmd := a.handleTabNavigationWithCmd(key); handled {
			return a, cmd
		}
		switch key {
		case "up", "k":
			if a.backupIndex > 0 {
				a.backupIndex--
			}
		case "down", "j":
			if len(a.backups) > 0 && a.backupIndex < len(a.backups)-1 {
				a.backupIndex++
			}
		case "enter": // Restore selected backup
			if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
				a.backupConfirmMode = true
				a.backupConfirmType = "restore"
				a.backupStatus = fmt.Sprintf("Restore backup '%s'? (y/n)", a.backups[a.backupIndex].Name)
			}
		case "d", "D": // Delete selected backup
			if len(a.backups) > 0 && a.backupIndex < len(a.backups) {
				a.backupConfirmMode = true
				a.backupConfirmType = "delete"
				a.backupStatus = fmt.Sprintf("Delete backup '%s'? (y/n)", a.backups[a.backupIndex].Name)
			}
		case "n", "N": // Create new backup
			a.backupRunning = true
			a.backupStatus = "Creating backup..."
			return a, createBackupCmd()
		case "r", "R": // Refresh backup list
			a.backupsLoaded = false
			a.backupsLoading = true
			a.backupStatus = ""
			a.backupError = nil
			return a, loadBackupsCmd()
		case "esc":
			a.screen = ScreenMainMenu
		}
	}

	return a, nil
}

// View renders the UI
func (a *App) View() string {
	// Try screen manager for migrated screens first
	if a.screenMgr != nil && !a.screenMgr.IsLegacyMode() {
		if view := a.screenMgr.View(); view != "" {
			return view
		}
	}

	switch a.screen {
	case ScreenAnimation:
		return a.renderAnimation()
	case ScreenWelcome:
		return a.renderWelcome()
	case ScreenThemePicker:
		return a.renderThemePicker()
	case ScreenNavPicker:
		return a.renderNavPicker()
	case ScreenFileTree:
		return a.renderFileTree()
	case ScreenProgress:
		return a.renderProgress()
	case ScreenSummary:
		return a.renderSummary()
	case ScreenError:
		return a.renderError()
	// Deep dive screens
	case ScreenDeepDiveMenu:
		return a.renderDeepDiveMenu()
	case ScreenConfigGhostty:
		return a.renderConfigGhostty()
	case ScreenConfigTmux:
		return a.renderConfigTmux()
	case ScreenConfigZsh:
		return a.renderConfigZsh()
	case ScreenConfigNeovim:
		return a.renderConfigNeovim()
	case ScreenConfigGit:
		return a.renderConfigGit()
	case ScreenConfigYazi:
		return a.renderConfigYazi()
	case ScreenConfigFzf:
		return a.renderConfigFzf()
	case ScreenConfigMacApps:
		return a.renderConfigMacApps()
	case ScreenConfigUtilities:
		return a.renderConfigUtilities()
	case ScreenConfigCLITools:
		return a.renderConfigCLITools()
	case ScreenConfigGUIApps:
		return a.renderConfigGUIApps()
	case ScreenConfigLazyGit:
		return a.renderConfigLazyGit()
	case ScreenConfigLazyDocker:
		return a.renderConfigLazyDocker()
	case ScreenConfigBtop:
		return a.renderConfigBtop()
	case ScreenConfigGlow:
		return a.renderConfigGlow()
	case ScreenConfigCLIUtilities:
		return a.renderConfigCLIUtilities()
	// Management platform screens
	case ScreenMainMenu:
		return a.renderMainMenu()
	case ScreenManage:
		return a.renderManageDualPane()
	case ScreenManageGhostty:
		return a.renderManageGhostty()
	case ScreenManageTmux:
		return a.renderManageTmux()
	case ScreenManageZsh:
		return a.renderManageZsh()
	case ScreenManageNeovim:
		return a.renderManageNeovim()
	case ScreenManageGit:
		return a.renderManageGit()
	case ScreenManageYazi:
		return a.renderManageYazi()
	case ScreenManageFzf:
		return a.renderManageFzf()
	case ScreenManageLazyGit:
		return a.renderManageLazyGit()
	case ScreenManageLazyDocker:
		return a.renderManageLazyDocker()
	case ScreenManageBtop:
		return a.renderManageBtop()
	case ScreenManageGlow:
		return a.renderManageGlow()
	case ScreenUpdate:
		return a.renderUpdate()
	case ScreenHotkeys:
		return a.renderHotkeysDualPane()
	case ScreenUsers:
		return a.renderUsersDualPane()
	case ScreenBackups:
		return a.renderBackups()
	default:
		return "Unknown screen"
	}
}

// startInstallation begins the installation process using the Go-based package manager
func (a *App) startInstallation() tea.Cmd {
	if a.installRunning {
		return nil
	}

	a.installRunning = true
	a.installStep = 0
	a.installOutput = []string{}

	// Save theme and nav style before installation
	a.saveInstallerConfig()

	// Collect all selected tools from deep dive config
	selectedTools := a.collectSelectedTools()

	return func() tea.Msg {
		if len(selectedTools) == 0 {
			a.installOutput = append(a.installOutput, "No tools selected for installation")
			return installDoneMsg{err: nil}
		}

		// Detect package manager
		mgr := pkg.DetectManager()
		if mgr == nil {
			return installDoneMsg{err: fmt.Errorf("no package manager detected")}
		}

		platform := pkg.DetectPlatform()
		reg := tools.GetRegistry()

		a.installOutput = append(a.installOutput, fmt.Sprintf("Installing %d tools using %s...", len(selectedTools), mgr.Name()))

		var lastErr error
		successCount := 0
		for _, toolID := range selectedTools {
			a.installStep++
			a.installOutput = append(a.installOutput, fmt.Sprintf("▶ Installing %s...", toolID))

			t, ok := reg.Get(toolID)
			if !ok {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Unknown tool: %s", toolID))
				continue
			}

			// Skip if already installed
			if t.IsInstalled() {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✓ %s already installed", toolID))
				successCount++
				continue
			}

			// Get packages for this platform
			pkgs := t.Packages()[platform]
			if len(pkgs) == 0 {
				pkgs = t.Packages()["all"]
			}
			if len(pkgs) == 0 {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ No packages for %s on this platform", toolID))
				continue
			}

			// Install using streaming command
			ctx := context.Background()
			cmd, err := mgr.InstallStreaming(ctx, pkgs...)
			if err != nil {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✗ Failed to start install: %v", err))
				lastErr = err
				continue
			}

			// Collect output
			for line := range cmd.Output {
				a.installOutput = append(a.installOutput, "  "+line)
				// Keep last 20 lines for display (using copy to avoid memory leak)
				const maxOutputLines = 20
				if len(a.installOutput) > maxOutputLines {
					copy(a.installOutput, a.installOutput[len(a.installOutput)-maxOutputLines:])
					a.installOutput = a.installOutput[:maxOutputLines]
				}
			}

			if err := cmd.Wait(); err != nil {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✗ Failed to install %s: %v", toolID, err))
				lastErr = err
			} else {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ✓ %s installed successfully", toolID))
				successCount++
			}
		}

		if successCount == len(selectedTools) {
			a.installOutput = append(a.installOutput, fmt.Sprintf("\n✓ All %d tools installed successfully!", successCount))
		} else {
			a.installOutput = append(a.installOutput, fmt.Sprintf("\n✓ Installed %d/%d tools", successCount, len(selectedTools)))
		}

		// Install dotfiles binary and utilities to ~/.local/bin
		a.installStep++
		a.installOutput = append(a.installOutput, "\n▶ Installing dotfiles utilities...")
		if err := installUtilities(a.deepDiveConfig.Utilities); err != nil {
			a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Failed to install utilities: %v", err))
			lastErr = err
		} else {
			a.installOutput = append(a.installOutput, "  ✓ Utilities installed to ~/.local/bin")
		}

		// Configure tmux with TPM plugins
		a.installStep++
		a.installOutput = append(a.installOutput, "\n▶ Configuring tmux...")
		tmuxCfg := tools.TmuxConfig{
			Prefix:           a.deepDiveConfig.TmuxPrefix,
			SplitBinds:       a.deepDiveConfig.TmuxSplitBinds,
			StatusBar:        a.deepDiveConfig.TmuxStatusBar,
			MouseMode:        a.deepDiveConfig.TmuxMouseMode,
			TPMEnabled:       a.deepDiveConfig.TmuxTPMEnabled,
			PluginSensible:   a.deepDiveConfig.TmuxPluginSensible,
			PluginResurrect:  a.deepDiveConfig.TmuxPluginResurrect,
			PluginContinuum:  a.deepDiveConfig.TmuxPluginContinuum,
			PluginYank:       a.deepDiveConfig.TmuxPluginYank,
			ContinuumSaveMin: a.deepDiveConfig.TmuxContinuumSaveMin,
		}
		if err := tools.SetupTPM(tmuxCfg, a.theme); err != nil {
			a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Failed to configure tmux: %v", err))
			lastErr = err
		} else {
			a.installOutput = append(a.installOutput, "  ✓ Tmux configured with ~/.tmux.conf")
			if tmuxCfg.TPMEnabled {
				if tools.IsTPMInstalled() {
					a.installOutput = append(a.installOutput, "  ✓ TPM plugins ready (run prefix+I in tmux to install)")
				} else {
					a.installOutput = append(a.installOutput, "  ⚠ TPM installed but plugins pending")
				}
			}
		}

		// Apply Claude Code MCP configuration if claude-code was selected
		if a.deepDiveConfig.CLITools["claude-code"] || a.deepDiveConfig.Utilities["claude-code"] {
			a.installStep++
			a.installOutput = append(a.installOutput, "\n▶ Configuring Claude Code MCP servers...")
			claudeTool := tools.NewClaudeCodeTool()
			if err := claudeTool.ApplyConfig(a.theme); err != nil {
				a.installOutput = append(a.installOutput, fmt.Sprintf("  ⚠ Failed to configure Claude MCP: %v", err))
				lastErr = err
			} else {
				a.installOutput = append(a.installOutput, "  ✓ Claude Code MCP servers configured (context7 enabled)")
			}
		}

		// Build context from last few output lines for error display
		var context string
		if lastErr != nil && len(a.installOutput) > 0 {
			start := 0
			if len(a.installOutput) > 8 {
				start = len(a.installOutput) - 8
			}
			context = strings.Join(a.installOutput[start:], "\n")
		}

		return installDoneMsg{err: lastErr, context: context}
	}
}

// installUtilities copies the dotfiles binary and shell utilities to ~/.local/bin
func installUtilities(utilities map[string]bool) error {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
	}

	binDir := filepath.Join(home, ".local", "bin")

	// Create ~/.local/bin if it doesn't exist
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", binDir, err)
	}

	// Clean up legacy binaries from previous installations
	cleanupOldInstallations()

	// Get the path to the currently running executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot get executable path: %w", err)
	}

	// Resolve any symlinks to get the real path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	// Copy the binary to ~/.local/bin/dotfiles
	destPath := filepath.Join(binDir, "dotfiles")
	// Remove existing binary first to avoid "text file busy" error
	// (Linux allows deleting a running binary, but not overwriting it)
	_ = os.Remove(destPath)
	if err := copyFile(execPath, destPath); err != nil {
		return fmt.Errorf("cannot copy binary: %w", err)
	}

	// Make it executable
	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}

	// Install selected utility scripts
	for name, enabled := range utilities {
		if !enabled {
			continue
		}
		script := scripts.GetScript(name)
		if script == "" {
			continue
		}
		scriptPath := filepath.Join(binDir, name)
		if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
			return fmt.Errorf("cannot write %s: %w", name, err)
		}
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// cleanupOldInstallations removes legacy binaries from previous installations.
// This handles the transition from separate dotfiles-tui/dotfiles-setup to unified dotfiles.
func cleanupOldInstallations() (removed []string) {
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return nil
	}

	// Locations where old binaries might exist
	locations := []string{
		filepath.Join(home, ".local", "bin", "dotfiles-tui"),
		filepath.Join(home, ".local", "bin", "dotfiles-setup"),
		"/usr/local/bin/dotfiles-tui",
		"/usr/local/bin/dotfiles-setup",
	}

	for _, path := range locations {
		if _, err := os.Stat(path); err == nil {
			// Binary exists, try to remove it
			if err := os.Remove(path); err == nil {
				removed = append(removed, filepath.Base(path))
			}
			// Silently ignore removal errors (permission issues, etc.)
		}
	}

	return removed
}

// collectSelectedTools gathers all tool IDs selected in deep dive config
func (a *App) collectSelectedTools() []string {
	// Ensure we have install status cached
	a.ensureInstallCache()

	var selected []string

	// CLI Tools (lazygit, lazydocker, btop, glow, claude-code)
	for id, enabled := range a.deepDiveConfig.CLITools {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
		}
	}

	// GUI Apps (zen-browser, cursor, lm-studio, obs)
	for id, enabled := range a.deepDiveConfig.GUIApps {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
		}
	}

	// CLI Utilities (bat, eza, zoxide, ripgrep, fd, delta, fswatch)
	for id, enabled := range a.deepDiveConfig.CLIUtilities {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
		}
	}

	// Note: Utilities (hk, caff, sshh) are shell scripts handled by installUtilities()
	// They don't go through the package manager

	// macOS Apps (rectangle, raycast, stats, etc.)
	for id, enabled := range a.deepDiveConfig.MacApps {
		if enabled && !a.manageInstalled[id] {
			selected = append(selected, id)
		}
	}

	return selected
}

// handleManageNavigation handles common navigation for management config screens
func (a *App) handleManageNavigation(key string, maxFields int, backScreen Screen) {
	switch key {
	case "up", "k":
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
	case "down", "j":
		if a.configFieldIndex < maxFields {
			a.configFieldIndex++
		}
	case "esc":
		a.configFieldIndex = 0
		a.screen = backScreen
	}
}

// handleTabNavigation handles number key shortcuts for tab navigation
// Returns (handled, command) - command may be nil even if handled
func (a *App) handleTabNavigationWithCmd(key string) (bool, tea.Cmd) {
	tabs := GetManagementTabs()
	var targetScreen Screen
	switch key {
	case "1":
		if len(tabs) > 0 {
			targetScreen = tabs[0].Screen
		}
	case "2":
		if len(tabs) > 1 {
			targetScreen = tabs[1].Screen
		}
	case "3":
		if len(tabs) > 2 {
			targetScreen = tabs[2].Screen
		}
	case "4":
		if len(tabs) > 3 {
			targetScreen = tabs[3].Screen
		}
	default:
		return false, nil
	}

	if targetScreen == 0 {
		return false, nil
	}

	a.screen = targetScreen

	// Start async operations when switching to certain screens
	if targetScreen == ScreenUpdate && !a.updateChecking && !a.updateCheckDone {
		a.updateChecking = true
		return true, checkUpdatesCmd()
	}

	if targetScreen == ScreenManage {
		if cmd := a.startInstallCacheLoad(); cmd != nil {
			return true, cmd
		}
	}

	if targetScreen == ScreenUsers && !a.usersLoaded {
		a.usersLoaded = true
		return true, loadUsersCmd()
	}

	if targetScreen == ScreenBackups && !a.backupsLoading && !a.backupsLoaded {
		a.backupsLoading = true
		return true, loadBackupsCmd()
	}

	return true, nil
}

// handleTabNavigation handles number key shortcuts for tab navigation
// Returns true if the key was handled
func (a *App) handleTabNavigation(key string) bool {
	handled, _ := a.handleTabNavigationWithCmd(key)
	return handled
}

// cycleOption cycles through options forward or backward
func cycleOption(opts []string, current string, forward bool) string {
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

// atoi converts a string to int with a default value
func atoi(s string, defaultVal int) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return defaultVal
	}
	return n
}

// togglePlugin adds or removes a plugin from the list
func togglePlugin(plugins *[]string, plugin string) {
	for i, p := range *plugins {
		if p == plugin {
			*plugins = append((*plugins)[:i], (*plugins)[i+1:]...)
			return
		}
	}
	*plugins = append(*plugins, plugin)
}

// execCommand wraps exec.Cmd to implement tea.ExecCommand
type execCommand struct {
	*exec.Cmd
}

func (e execCommand) SetStdin(r io.Reader)  { e.Cmd.Stdin = r }
func (e execCommand) SetStdout(w io.Writer) { e.Cmd.Stdout = w }
func (e execCommand) SetStderr(w io.Writer) { e.Cmd.Stderr = w }

// sudoPromptCmd returns a command that prompts for sudo credentials
func sudoPromptCmd() tea.ExecCommand {
	// Use a script that shows a nice message then prompts for sudo
	cmd := exec.Command("bash", "-c", `
		echo ""
		echo "┌────────────────────────────────────────────┐"
		echo "│  Installation requires administrator       │"
		echo "│  privileges to install system packages.    │"
		echo "└────────────────────────────────────────────┘"
		echo ""
		if sudo -v; then
			echo ""
			echo "✓ Authentication successful"
			echo ""
			echo "Press any key to continue..."
			read -n 1
			exit 0
		else
			echo ""
			echo "✗ Authentication failed"
			echo ""
			echo "Press any key to return to the TUI..."
			read -n 1
			exit 1
		fi
	`)
	return execCommand{cmd}
}

// manageInstallWithLogsMsg carries install result with collected logs
type manageInstallWithLogsMsg struct {
	toolID string
	logs   []string
	err    error
}

// updateWithLogsMsg carries update result with collected logs
type updateWithLogsMsg struct {
	logs    []string
	results []pkg.UpdateResult
	err     error
}

// streamingInstallToolCmd returns a command that installs a tool with output collection
func (a *App) streamingInstallToolCmd(toolID string) tea.Cmd {
	return func() tea.Msg {
		reg := tools.GetRegistry()
		t, ok := reg.Get(toolID)
		if !ok {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("unknown tool: %s", toolID)}
		}

		mgr := pkg.DetectManager()
		if mgr == nil {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("no package manager detected")}
		}

		// Get packages for this platform
		platform := pkg.DetectPlatform()
		pkgs := t.Packages()[platform]
		if len(pkgs) == 0 {
			pkgs = t.Packages()["all"]
		}
		if len(pkgs) == 0 {
			return manageInstallWithLogsMsg{toolID: toolID, err: fmt.Errorf("no packages defined for %s", toolID)}
		}

		// Start streaming install
		ctx := context.Background()
		cmd, err := mgr.InstallStreaming(ctx, pkgs...)
		if err != nil {
			return manageInstallWithLogsMsg{toolID: toolID, err: err}
		}

		// Collect all output
		var logs []string
		for line := range cmd.Output {
			logs = append(logs, line)
		}

		// Wait for completion
		err = cmd.Wait()
		return manageInstallWithLogsMsg{toolID: toolID, logs: logs, err: err}
	}
}

// streamingUpdateCmd returns a command that updates packages with output collection
func (a *App) streamingUpdateCmd(packages []pkg.Package) tea.Cmd {
	return func() tea.Msg {
		mgr := pkg.DetectManager()
		if mgr == nil {
			return updateWithLogsMsg{err: fmt.Errorf("no package manager detected")}
		}

		var pkgNames []string
		for _, p := range packages {
			pkgNames = append(pkgNames, p.Name)
		}

		ctx := context.Background()
		cmd, err := mgr.UpdateStreaming(ctx, pkgNames...)
		if err != nil {
			return updateWithLogsMsg{err: err}
		}

		// Collect all output
		var logs []string
		for line := range cmd.Output {
			logs = append(logs, line)
		}

		err = cmd.Wait()

		// Build results
		var results []pkg.UpdateResult
		for _, p := range packages {
			results = append(results, pkg.UpdateResult{
				Package: p,
				Success: err == nil,
				Error:   err,
			})
		}
		return updateWithLogsMsg{logs: logs, results: results, err: err}
	}
}

// streamingUpdateAllCmd returns a command that updates all packages with output collection
func (a *App) streamingUpdateAllCmd() tea.Cmd {
	return func() tea.Msg {
		mgr := pkg.DetectManager()
		if mgr == nil {
			return updateWithLogsMsg{err: fmt.Errorf("no package manager detected")}
		}

		ctx := context.Background()
		cmd, err := mgr.UpdateAllStreaming(ctx)
		if err != nil {
			return updateWithLogsMsg{err: err}
		}

		// Collect all output
		var logs []string
		for line := range cmd.Output {
			logs = append(logs, line)
		}

		err = cmd.Wait()
		return updateWithLogsMsg{logs: logs, err: err}
	}
}

// saveInstallerConfig saves theme and nav style during installer flow
func (a *App) saveInstallerConfig() {
	g, err := config.LoadGlobalConfig()
	if err != nil {
		g = config.DefaultGlobalConfig()
	}
	g.Theme = a.theme
	g.NavStyle = a.navStyle
	g.DisableAnimations = !a.animationsEnabled

	// Save synchronously since we're about to start installation
	_ = config.SaveGlobalConfig(g)
}

// SetStartScreen sets the initial screen to display (for CLI routing)
func (a *App) SetStartScreen(screen Screen) {
	a.startScreen = screen
	// Always land on the requested screen after the intro.
	a.postIntroScreen = screen

	// Starting explicitly at the animation means "intro → welcome".
	if screen == ScreenAnimation {
		a.postIntroScreen = ScreenWelcome
		// If animations are disabled (reduce motion) or the caller requested
		// skipping the intro, go straight to welcome.
		if a.skipIntro || !a.animationsEnabled {
			a.screen = ScreenWelcome
			a.animationDone = true
			return
		}

		a.screen = ScreenAnimation
		a.animFrame = 0
		a.animationDone = false
		return
	}

	// If the caller requested skipping the intro, go straight to the screen.
	if a.skipIntro || !a.animationsEnabled {
		a.screen = screen
		a.animationDone = true
		return
	}

	// Default behavior: play the intro and then transition to the requested screen.
	a.screen = ScreenAnimation
	a.animFrame = 0
	a.animationDone = false
}

// SetHotkeyFilter sets the tool filter for hotkeys screen
func (a *App) SetHotkeyFilter(tool string) {
	a.hotkeyFilter = tool
}

// GetToolConfigScreen returns the screen constant for a tool name
func GetToolConfigScreen(tool string) (Screen, bool) {
	screens := map[string]Screen{
		"ghostty":   ScreenConfigGhostty,
		"tmux":      ScreenConfigTmux,
		"zsh":       ScreenConfigZsh,
		"neovim":    ScreenConfigNeovim,
		"git":       ScreenConfigGit,
		"yazi":      ScreenConfigYazi,
		"fzf":       ScreenConfigFzf,
		"apps":      ScreenConfigApps,
		"utilities": ScreenConfigUtilities,
	}

	screen, ok := screens[tool]
	return screen, ok
}

// MainMenuItem represents an item in the main menu
type MainMenuItem struct {
	Name        string
	Description string
	Icon        string
	Screen      Screen
}

// GetMainMenuItems returns the main menu items for the management platform
func GetMainMenuItems() []MainMenuItem {
	return []MainMenuItem{
		{
			Name:        "Install",
			Description: "Full installation wizard",
			Icon:        "󰆓",
			Screen:      ScreenWelcome,
		},
		{
			Name:        "Manage",
			Description: "Configure installed tools",
			Icon:        "󰒓",
			Screen:      ScreenManage,
		},
		{
			Name:        "Update",
			Description: "Check and install updates",
			Icon:        "󰚰",
			Screen:      ScreenUpdate,
		},
		{
			Name:        "Theme",
			Description: "Change color theme",
			Icon:        "󰔎",
			Screen:      ScreenThemePicker,
		},
		{
			Name:        "Hotkeys",
			Description: "View keyboard shortcuts",
			Icon:        "󰌌",
			Screen:      ScreenHotkeys,
		},
		{
			Name:        "Backups",
			Description: "View and restore backups",
			Icon:        "󰁯",
			Screen:      ScreenBackups,
		},
	}
}

// ScreenToolIDs maps deep dive screens to their corresponding tool IDs
// This is the single source of truth for screen-to-tool mapping
var ScreenToolIDs = map[Screen][]string{
	ScreenConfigGhostty:      {"ghostty"},
	ScreenConfigTmux:         {"tmux"},
	ScreenConfigZsh:          {"zsh"},
	ScreenConfigNeovim:       {"neovim"},
	ScreenConfigGit:          {"git"},
	ScreenConfigYazi:         {"yazi"},
	ScreenConfigFzf:          {"fzf"},
	ScreenConfigLazyGit:      {"lazygit"},
	ScreenConfigLazyDocker:   {"lazydocker"},
	ScreenConfigBtop:         {"btop"},
	ScreenConfigGlow:         {"glow"},
	ScreenConfigCLIUtilities: {"bat", "eza", "zoxide", "ripgrep", "fd", "delta", "fswatch"},
	ScreenConfigGUIApps:      {"zen-browser", "cursor", "lm-studio", "obs"},
	ScreenConfigMacApps:      {"rectangle", "raycast", "iina", "appcleaner"},
	ScreenConfigCLITools:     {"lazygit", "lazydocker", "btop", "glow", "claude-code"},
	ScreenConfigUtilities:    {"hk", "caff", "sshh"}, // shell scripts, not package manager installs
}

// ensureInstallCache populates the install status cache if not already done.
// This is kept for synchronous contexts (like collectSelectedTools before install).
// For UI rendering, use startInstallCacheLoad() and check installCacheLoading.
func (a *App) ensureInstallCache() {
	if a.manageInstalledReady {
		return
	}

	reg := tools.GetRegistry()
	all := reg.All()

	if a.manageInstalled == nil {
		a.manageInstalled = make(map[string]bool, len(all)+3) // +3 for utilities
	}

	// Try batch checking first (much faster)
	mgr := pkg.DetectManager()
	platform := pkg.DetectPlatform()

	var installedPkgs map[string]bool
	if mgr != nil {
		pkgList, err := mgr.ListInstalled()
		if err == nil {
			installedPkgs = make(map[string]bool, len(pkgList))
			for _, p := range pkgList {
				installedPkgs[p.Name] = true
			}
		}
	}

	for _, t := range all {
		found := false
		if installedPkgs != nil {
			pkgs := t.Packages()[platform]
			if len(pkgs) == 0 {
				pkgs = t.Packages()["all"]
			}
			if len(pkgs) > 0 {
				if installedPkgs[pkgs[0]] {
					a.manageInstalled[t.ID()] = true
					found = true
				}
			}
		}
		// Fall back to IsInstalled() for tools not found via package manager
		// (e.g., flatpaks, AppImages, direct binaries)
		if !found {
			a.manageInstalled[t.ID()] = t.IsInstalled()
		}
	}

	// Check utility scripts in ~/.local/bin
	home := os.Getenv("HOME")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home != "" {
		binDir := filepath.Join(home, ".local", "bin")
		for _, util := range []string{"hk", "caff", "sshh"} {
			_, err := os.Stat(filepath.Join(binDir, util))
			a.manageInstalled[util] = err == nil
		}
	}

	a.manageInstalledReady = true
}

// startInstallCacheLoad begins async cache loading if not already loading or ready.
// Returns a command to start loading, or nil if cache is ready/loading.
func (a *App) startInstallCacheLoad() tea.Cmd {
	if a.manageInstalledReady || a.installCacheLoading {
		return nil
	}
	a.installCacheLoading = true
	return loadInstallCacheCmd()
}

// isInstallCacheReady returns true if the cache is ready, false if loading or not started
func (a *App) isInstallCacheReady() bool {
	return a.manageInstalledReady
}

// getDeepDiveItemStatus returns the install status for a deep dive menu item
// Returns "installed" (blue) if all installed, "partial" (yellow) if partially installed, "pending" (grey) if not
func (a *App) getDeepDiveItemStatus(item DeepDiveMenuItem) string {
	toolIDs, ok := ScreenToolIDs[item.Screen]
	if !ok || len(toolIDs) == 0 {
		// No tool mapping (e.g., utilities) - show as pending
		return "pending"
	}

	installedCount := 0
	for _, id := range toolIDs {
		if a.manageInstalled[id] {
			installedCount++
		}
	}

	if installedCount == len(toolIDs) {
		return "installed" // All installed (blue)
	} else if installedCount > 0 {
		return "partial" // Partially installed (yellow)
	}
	return "pending" // None installed (grey)
}

// handleWelcomeMouse handles mouse clicks on the welcome screen
func (a *App) handleWelcomeMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle left click to toggle deep dive option or proceed
	if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		// The welcome screen has two boxes side by side at the bottom
		// Left box: Quick Install, Right box: Deep Dive
		centerY := a.height / 2
		centerX := a.width / 2

		// If clicking in lower half where the options are
		if m.Y > centerY {
			if m.X < centerX {
				a.deepDive = false
			} else {
				a.deepDive = true
			}
		}
	}

	return a, nil
}

// handleThemePickerMouse handles mouse clicks on the theme picker screen
func (a *App) handleThemePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle scroll wheel for navigation (works anywhere on screen)
	switch m.Button {
	case tea.MouseButtonWheelUp:
		if a.themeIndex > 0 {
			a.themeIndex--
			a.theme = themes[a.themeIndex].name
			SetTheme(a.theme)
		}
		return a, nil
	case tea.MouseButtonWheelDown:
		if a.themeIndex < len(themes)-1 {
			a.themeIndex++
			a.theme = themes[a.themeIndex].name
			SetTheme(a.theme)
		}
		return a, nil
	}

	// Handle left clicks
	if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
		// The content is centered using PlaceWithBackground + ContainerStyle
		// Container has Padding(1, 2) and Border, then title + empty line + themes
		// Calculate approximate content area
		containerH := len(themes) + 6 // title + empty + themes + empty + help + padding
		containerW := 60              // approximate
		startY := (a.height - containerH) / 2
		startX := (a.width - containerW) / 2

		// Theme list starts after: container border (1) + padding (1) + title (1) + empty (1)
		listStartY := startY + 4

		// Check if click is in theme list area
		if m.Y >= listStartY && m.Y < listStartY+len(themes) && m.X >= startX {
			themeIdx := m.Y - listStartY
			if themeIdx >= 0 && themeIdx < len(themes) {
				a.themeIndex = themeIdx
				a.theme = themes[themeIdx].name
				SetTheme(a.theme)
				return a, nil
			}
		}
	}

	return a, nil
}

// handleNavPickerMouse handles mouse clicks on the nav picker screen
func (a *App) handleNavPickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Nav picker has two boxes side by side (or stacked on narrow terminals)
	// Calculate approximate positions
	centerX := a.width / 2
	centerY := a.height / 2

	if a.width >= 78 {
		// Side by side layout
		// Emacs box is on the left, Vim box is on the right
		if m.X < centerX {
			a.navStyle = "emacs"
		} else {
			a.navStyle = "vim"
		}
	} else {
		// Stacked layout - Emacs on top, Vim on bottom
		if m.Y < centerY {
			a.navStyle = "emacs"
		} else {
			a.navStyle = "vim"
		}
	}

	return a, nil
}

// handleDeepDiveMenuMouse handles mouse clicks on the deep dive menu
func (a *App) handleDeepDiveMenuMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle scroll wheel
	if m.Button == tea.MouseButtonWheelUp {
		if a.deepDiveMenuIndex > 0 {
			a.deepDiveMenuIndex--
		}
		return a, nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		items := GetDeepDiveMenuItems()
		if a.deepDiveMenuIndex < len(items)-1 {
			a.deepDiveMenuIndex++
		}
		return a, nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Menu items are in a centered container
	items := GetDeepDiveMenuItems()
	contentHeight := len(items) + 10 // items + headers + padding
	startY := (a.height - contentHeight) / 2
	listStartY := startY + 4 // After title and instructions

	// Account for category headers (they take up a line but aren't clickable)
	clickableY := listStartY
	for i, item := range items {
		if item.Category != "" {
			clickableY++ // Category header takes a line
		}
		if m.Y == clickableY {
			a.deepDiveMenuIndex = i
			// Double-click or single click to enter
			a.configFieldIndex = 0
			a.screen = item.Screen
			return a, nil
		}
		clickableY++
	}

	return a, nil
}

// handleConfigScreenMouse handles mouse clicks on config screens
func (a *App) handleConfigScreenMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m := tea.MouseEvent(msg)

	// Handle scroll wheel for field navigation
	if m.Button == tea.MouseButtonWheelUp {
		if a.configFieldIndex > 0 {
			a.configFieldIndex--
		}
		return a, nil
	}
	if m.Button == tea.MouseButtonWheelDown {
		// Get max fields for current screen
		maxFields := a.getConfigScreenMaxFields()
		if a.configFieldIndex < maxFields-1 {
			a.configFieldIndex++
		}
		return a, nil
	}

	if m.Action != tea.MouseActionPress || m.Button != tea.MouseButtonLeft {
		return a, nil
	}

	// Config screens have fields listed vertically
	// Approximate click detection based on Y position
	contentHeight := a.getConfigScreenMaxFields() + 8
	startY := (a.height - contentHeight) / 2
	fieldStartY := startY + 4 // After title

	if m.Y >= fieldStartY {
		fieldIdx := m.Y - fieldStartY
		maxFields := a.getConfigScreenMaxFields()
		if fieldIdx >= 0 && fieldIdx < maxFields {
			a.configFieldIndex = fieldIdx
		}
	}

	return a, nil
}

// getConfigScreenMaxFields returns the number of fields for the current config screen
func (a *App) getConfigScreenMaxFields() int {
	switch a.screen {
	case ScreenConfigGhostty:
		return 7
	case ScreenConfigTmux:
		if a.deepDiveConfig.TmuxTPMEnabled {
			return 13
		}
		return 8
	case ScreenConfigZsh:
		return 13
	case ScreenConfigNeovim:
		return 14
	case ScreenConfigGit:
		return 5
	case ScreenConfigYazi:
		return 3
	case ScreenConfigFzf:
		return 3
	case ScreenConfigMacApps:
		return len(a.deepDiveConfig.MacApps)
	case ScreenConfigGUIApps:
		return 4
	case ScreenConfigCLITools:
		return 5
	case ScreenConfigCLIUtilities:
		return 7
	case ScreenConfigUtilities:
		return 3
	default:
		return 10
	}
}

// handleSummaryMouse handles mouse clicks on the summary screen
func (a *App) handleSummaryMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Summary screen doesn't need special mouse handling yet
	// Left click could be used to trigger install in the future
	return a, nil
}
