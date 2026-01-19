package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/runner"
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
	ScreenManageClaudeCode
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
	// Hotkeys alias editing state
	hotkeysAddingAlias  bool   // Currently adding an alias
	hotkeysAliasName    string // Alias name being entered
	hotkeysAliasCommand string // Command the alias maps to
	hotkeysAliasField   int    // 0 = name, 1 = command
	hotkeysAliasCursor  int    // Cursor position in current field
	backupIndex         int    // Backup selection cursor
	backups             []BackupEntry
	backupsLoaded       bool
	backupsLoading      bool
	backupConfirmMode   bool
	backupConfirmType   string // "restore" or "delete"
	backupStatus        string // Status message for backup operations
	backupRunning       bool   // Currently running a backup operation
	backupError         error  // Error from backup operation

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
	// Preload install cache immediately on startup for faster Deep Dive/Manage transitions
	// By loading during intro animation, cache is ready when user navigates to those screens
	if cmd := a.startInstallCacheLoad(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
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

		// Run backup cleanup based on settings
		cleanupBackups()

		return backupCreateDoneMsg{name: timestamp, err: nil}
	}
}

// cleanupBackups removes old backups based on global config settings
func cleanupBackups() {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return
	}

	backupsDir := filepath.Join(config.ConfigDir(), "backups")
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return
	}

	type backupInfo struct {
		name    string
		modTime time.Time
	}

	var backups []backupInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupInfo{
			name:    entry.Name(),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})

	now := time.Now()
	for i, backup := range backups {
		shouldDelete := false

		// Delete if exceeds max count (and max count is set)
		if cfg.BackupMaxCount > 0 && i >= cfg.BackupMaxCount {
			shouldDelete = true
		}

		// Delete if exceeds max age (and max age is set)
		if cfg.BackupMaxAgeDays > 0 {
			age := now.Sub(backup.modTime)
			if age > time.Duration(cfg.BackupMaxAgeDays)*24*time.Hour {
				shouldDelete = true
			}
		}

		if shouldDelete {
			backupPath := filepath.Join(backupsDir, backup.name)
			os.RemoveAll(backupPath)
		}
	}
}

// autoBackupIfEnabled creates a backup if auto-backup is enabled in settings
func autoBackupIfEnabled() error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	if !cfg.AutoBackup {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Create backup directory with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05") + "_auto"
	backupDir := filepath.Join(config.ConfigDir(), "backups", timestamp)
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return err
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

	// Run cleanup after creating backup
	cleanupBackups()

	return nil
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
		ScreenConfigBtop, ScreenConfigGlow, ScreenConfigClaudeCode:
		return a.handleConfigScreenMouse(msg)
	default:
		return a, nil
	}
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

	// Delegate to screen-specific handlers
	switch a.screen {
	// Wizard screens
	case ScreenAnimation, ScreenWelcome, ScreenThemePicker, ScreenNavPicker,
		ScreenFileTree, ScreenProgress, ScreenSummary, ScreenError:
		return a.handleWizardKey(msg)

	// Management screens
	case ScreenMainMenu, ScreenManage, ScreenUpdate, ScreenHotkeys, ScreenBackups, ScreenUsers,
		ScreenManageGhostty, ScreenManageTmux, ScreenManageZsh, ScreenManageNeovim,
		ScreenManageGit, ScreenManageYazi, ScreenManageFzf, ScreenManageLazyGit,
		ScreenManageLazyDocker, ScreenManageBtop, ScreenManageGlow, ScreenManageClaudeCode:
		return a.handleManagementKey(msg)

	// Deep dive screens
	case ScreenDeepDiveMenu, ScreenConfigGhostty, ScreenConfigTmux, ScreenConfigZsh,
		ScreenConfigNeovim, ScreenConfigGit, ScreenConfigYazi, ScreenConfigFzf,
		ScreenConfigMacApps, ScreenConfigUtilities, ScreenConfigCLITools,
		ScreenConfigGUIApps, ScreenConfigCLIUtilities, ScreenConfigLazyGit,
		ScreenConfigLazyDocker, ScreenConfigBtop, ScreenConfigGlow, ScreenConfigClaudeCode:
		return a.handleDeepDiveKey(msg)
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
	case ScreenConfigClaudeCode:
		return a.renderConfigClaudeCode()
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
	case ScreenManageClaudeCode:
		return a.renderManageClaudeCode()
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

// buildScreenToolIDs generates the screen to tool ID mapping from the registry.
// This uses the tool registry as the single source of truth.
func buildScreenToolIDs() map[Screen][]string {
	result := make(map[Screen][]string)

	// Map UIGroup to their group screens
	groupScreens := map[tools.UIGroup]Screen{
		tools.UIGroupCLITools:     ScreenConfigCLITools,
		tools.UIGroupCLIUtilities: ScreenConfigCLIUtilities,
		tools.UIGroupGUIApps:      ScreenConfigGUIApps,
		tools.UIGroupMacApps:      ScreenConfigMacApps,
	}

	for _, t := range tools.GetRegistry().All() {
		// Tools with dedicated screens (UIGroupNone with a configScreen set)
		if t.UIGroup() == tools.UIGroupNone && t.ConfigScreen() != 0 {
			screen := Screen(t.ConfigScreen())
			result[screen] = append(result[screen], t.ID())
		}

		// Tools in group screens
		if screen, ok := groupScreens[t.UIGroup()]; ok {
			result[screen] = append(result[screen], t.ID())
		}
	}

	// Add utilities (shell scripts not in registry)
	result[ScreenConfigUtilities] = []string{"hk", "caff", "sshh"}

	return result
}

// ScreenToolIDs maps deep dive screens to their corresponding tool IDs
// Generated from tool registry - single source of truth
var ScreenToolIDs = buildScreenToolIDs()
