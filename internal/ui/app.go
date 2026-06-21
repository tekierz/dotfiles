package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/backup"
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
	_ // retired: ScreenConfigApps (vestigial screen). Slot reserved so the
	// remaining iota values stay stable for the raw ConfigScreen() ints declared
	// in the tools package (see toolscreens.go / verifyToolConfigScreens).
	// Additional config screens
	ScreenConfigCLITools
	ScreenConfigGUIApps
	ScreenConfigCLIUtilities // bat, eza, zoxide, ripgrep, fd, delta, fswatch
	// Individual CLI tool config screens (installer)
	ScreenConfigLazyGit
	ScreenConfigLazyDocker
	ScreenConfigBtop
	ScreenConfigGlow
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

// persistTheme saves the currently selected theme to the global config, so a
// standalone "Change theme" from the main menu actually sticks across runs.
// On failure it records a brief human-readable message in themeStatus that the
// view layer can surface to the user instead of silently discarding the error.
func (a *App) persistTheme() {
	g, err := config.LoadGlobalConfig()
	if err != nil || g == nil {
		if err != nil {
			a.themeStatus = "Failed to save theme: " + err.Error()
		}
		return
	}
	g.Theme = a.theme
	if err := config.SaveGlobalConfig(g); err != nil {
		a.themeStatus = "Failed to save theme: " + err.Error()
		return
	}
	a.themeStatus = ""
}

// snapshotManageBaseline records the current Manage config + theme as the
// baseline the next Manage save diffs against. Called after loading at startup and
// after a successful save so each save scopes its config-file writes to only the
// tools changed since the last persisted state.
func (a *App) snapshotManageBaseline() {
	if a.manageConfig != nil {
		a.manageConfigBaseline = *a.manageConfig // value copy of a flat struct
	} else {
		a.manageConfigBaseline = *NewManageConfig()
	}
	a.manageConfigBaselineTheme = a.theme
}

// revertThemeToSaved reverts the in-session theme/preview back to the persisted
// theme (used when the user cancels a standalone theme change with Esc).
func (a *App) revertThemeToSaved() {
	g, err := config.LoadGlobalConfig()
	if err != nil || g == nil || g.Theme == "" {
		return
	}
	a.theme = g.Theme
	a.syncThemeIndex() // updates themeIndex + applies the palette
	if a.screenMgr != nil {
		a.screenMgr.Context().Theme = a.theme
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
	// screenFactory is the App-owned factory used by screenMgr. It is kept on
	// the App so transition sites can pass per-screen data (e.g. the error to
	// display) before navigating through the manager.
	screenFactory *Factory

	// Animation state
	animFrame        int
	postIntroScreen  Screen // where to land after the intro animation
	uiFrame          int    // global animation frame counter (manager widgets, spinners, etc.)
	manageInstalling bool
	manageInstallID  string

	// User selections
	themeIndex int
	theme      string
	// themeListLayout records the on-screen geometry (per-theme row extents +
	// horizontal box span) of the most recently rendered theme picker so the
	// mouse handler can map a click to the correct theme. Repopulated on every
	// themePickerScreen.View. The legacy handler hand-derived the container
	// height/width and shifted every row by ~2-3 (C20).
	themeListLayout fieldLayout
	navStyle        string
	// animationsEnabled controls non-essential UI animations (headers/widgets).
	// When false, we render static UI to reduce motion/jank and CPU usage.
	animationsEnabled bool
	deepDive          bool

	// Deep dive state (installer)
	deepDiveMenuIndex int
	// deepDiveMenuLayout records per-item geometry of the most recently rendered
	// deep-dive menu so the mouse handler maps a click to the correct row.
	// Repopulated on every deepDiveMenuScreen.View; replaces the legacy anchor
	// that ignored category-header MarginTop blanks and the box border/padding.
	deepDiveMenuLayout fieldLayout
	deepDiveConfig     *DeepDiveConfig
	configFieldIndex   int // Currently focused field in config screens

	// configFieldLayout records the on-screen geometry of the most recently
	// rendered field-config screen so the mouse handler can map a click to the
	// correct field. It is repopulated on every View of a field-config screen
	// (see fieldLayoutRecorder in screen_config_base.go). Without it, click-to-
	// select mis-maps because fields render as variable-height blocks (label +
	// control + blank, with selectors that may wrap) rather than one row each.
	configFieldLayout fieldLayout

	// configStandalone is true when the app was launched directly into a single
	// tool's config screen via `dotfiles config <tool>` (not as part of the
	// install wizard). In that mode there is no later install step to apply the
	// edits, so the config screen's back() persists the edits to the real config
	// files itself and quits, instead of returning to the deep-dive menu (C27).
	configStandalone bool
	macAppIndex      int // Currently focused app in macOS screen
	utilityIndex     int // Currently focused utility
	cliToolIndex     int // Currently focused CLI tool
	guiAppIndex      int // Currently focused GUI app
	cliUtilityIndex  int // Currently focused CLI utility (bat, eza, etc.)

	// Management state (detailed config)
	manageConfig *ManageConfig
	// manageConfigBaseline is a snapshot of manageConfig + theme as last loaded or
	// last successfully saved. The Manage save diffs the live config against this
	// to apply ONLY the tools the user actually changed, so editing one tool can
	// never rewrite another tool's config file from manage.json defaults (P1-A2).
	manageConfigBaseline      ManageConfig
	manageConfigBaselineTheme string
	managePane                int // 0 = tools pane, 1 = settings pane (ScreenManage)
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
	installStep         int
	installPlannedSteps int // total step-increments the worker will emit (set before worker starts)
	installOutput       []string
	installRunning      bool
	installComplete     bool
	installEvents       chan installEventMsg // streamed progress from the install worker goroutine
	updateStream        chan updateStreamMsg // streamed progress from the update worker goroutine
	runner              *runner.Runner
	// streamCancel cancels the context driving the currently-running install or
	// update worker (and the underlying StreamingCmd). It is retained on the App
	// so navigate-away / Ctrl+C can tear the subprocess + worker goroutines down
	// deterministically rather than orphaning them. nil when nothing streams.
	streamCancel context.CancelFunc
	// streamCmd is the StreamingCmd backing the active install/update stream, kept
	// so its subprocess can be Cancel()ed on teardown. nil when nothing streams.
	streamCmd *runner.StreamingCmd
	// sudoKeepAliveStop terminates the sudo keep-alive goroutine that refreshes
	// the credential cache for the duration of a Linux install (C16). It is set
	// by startInstallation() and invoked on BOTH normal completion (installDoneMsg)
	// and cancel/teardown (teardownStream); it blocks until the goroutine exits so
	// no keep-alive survives the install. nil when no keep-alive is running (and
	// always a no-op on non-Linux).
	sudoKeepAliveStop func()

	// Management platform state (new)
	mainMenuIndex int // Main menu cursor
	// mainMenuLayout records per-row geometry of the most recently rendered main
	// menu so the mouse handler maps a click to the correct item. Repopulated on
	// every mainMenuScreen.View; replaces the legacy hand-derived anchor that
	// ignored ContainerStyle's border/padding and HelpStyle's padding.
	mainMenuLayout   fieldLayout
	manageIndex      int    // Manage screen cursor
	updateIndex      int    // Update screen cursor
	hotkeyFilter     string // Filter hotkeys by tool
	hotkeyCursor     int    // Hotkeys screen cursor
	hotkeyCategory   int    // Current category in hotkeys
	hotkeysPane      int    // 0 = categories, 1 = items
	hotkeyCatScroll  int    // Category list scroll
	hotkeyItemScroll int    // Item list scroll
	hotkeysReturn    Screen // Screen to return to when leaving hotkeys
	themeReturn      Screen // Screen to return to when leaving the theme picker (used for Esc destination)
	// themeStandalone is true when the theme picker was launched as a
	// standalone "change theme" command (CLI `dotfiles theme` or main-menu
	// 'Theme'), as opposed to being wizard step 2. It is set/reset at every
	// picker entry so no path can inherit a stale value (C7, C8).
	themeStandalone bool
	// themeStatus holds a transient status message from the last persistTheme
	// call (empty on success, error text on failure).
	themeStatus          string
	hotkeysFavorites     *config.HotkeysConfig // User hotkey favorites config
	hotkeysFavoritesOnly bool                  // Filter to show only favorites
	// Per-App active-username cache for the hotkeys screen. Refreshed once per
	// event/frame via refreshHotkeysCurrentUser so that per-row lookups within
	// a single frame don't re-read global.json from disk.
	hotkeysActiveUser       string // cached username, or "" if not yet resolved
	hotkeysActiveUserCached bool   // true once the cache has been populated
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

// initScreenManager wires the App's Factory and ScreenManager. Every live
// screen is a migrated ScreenHandler, so the manager is mandatory and is built
// during NewApp. The App owns the Factory so transition sites can set
// per-screen data (such as the error to display) before navigating.
func (a *App) initScreenManager() {
	if a.screenFactory == nil {
		a.screenFactory = NewFactory()
	}
	deps := NewDependencies()
	ctx := NewScreenContext(deps)
	// Connect the context to this App so handlers can reach shared App
	// state (theme, deepDiveConfig, etc.) through ScreenContext.app.
	ctx.app = a
	ctx.Theme = a.theme
	ctx.NavStyle = a.navStyle
	ctx.AnimationsEnabled = a.animationsEnabled
	a.screenMgr = NewScreenManager(ctx, a.screenFactory.CreateFactory())
}

// showError transitions to the error screen: it sets the error on the factory
// and navigates through the manager so the ErrorScreen renders with the real
// error. The caller is responsible for setting a.lastError.
func (a *App) showError(err error) tea.Cmd {
	a.screenFactory.SetError(err)
	return NavigateTo(ScreenError)
}

// showSummary transitions to the summary screen through the ScreenManager.
func (a *App) showSummary() tea.Cmd {
	return NavigateTo(ScreenSummary)
}

// postIntroTransition lands on the post-intro screen after the intro animation
// finishes (or is skipped). It routes navigation through the ScreenManager so
// the post-intro screen (Welcome, MainMenu, ThemePicker) enters managed mode,
// and batches any on-enter async load the destination needs.
func (a *App) postIntroTransition() tea.Cmd {
	a.animationDone = true
	target := a.postIntroScreen

	var async tea.Cmd
	switch target {
	case ScreenUpdate:
		if !a.updateChecking && !a.updateCheckDone {
			a.updateChecking = true
			async = checkUpdatesCmd()
		}
	case ScreenManage, ScreenHotkeys:
		async = a.startInstallCacheLoad()
	}

	nav := NavigateTo(target)
	if async != nil {
		return tea.Batch(nav, async)
	}
	return nav
}

// NewApp creates a new application instance
func NewApp(skipIntro bool, opts ...AppOption) *App {
	// Fail fast if the tool->screen mapping has drifted from the tools registry
	// (e.g. a Screen iota reorder), so misroutes are caught at startup.
	verifyToolConfigScreens()

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
		themeReturn:          ScreenWelcome,
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

	// Snapshot the loaded Manage config + theme as the save baseline so the Manage
	// save can scope its config-file writes to only the tools the user changes.
	app.snapshotManageBaseline()

	// Best-effort: load hotkeys favorites config and migrate any legacy
	// Key-string-keyed favorites to the new stable-ID format.
	if hkCfg, err := config.LoadHotkeysConfig(); err == nil && hkCfg != nil {
		for _, uh := range hkCfg.Users {
			config.MigrateLegacyFavorites(uh)
		}
		// Persist the migrated config so the migration is one-time.
		_ = config.SaveHotkeysConfig(hkCfg)
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

	// Apply any options before wiring the manager so they can set fields the
	// manager's context reads (theme, nav style, animations).
	for _, opt := range opts {
		opt(app)
	}

	// Every live screen is a migrated ScreenHandler, so the ScreenManager is
	// mandatory. Wire it and eagerly enter managed mode on the start screen so
	// the first rendered frame (which Bubble Tea draws before Init's command is
	// processed) is never blank.
	app.initScreenManager()
	app.screenMgr.Navigate(app.screen)

	return app
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{}
	// Drive the start screen through the ScreenManager so the screen's Init()
	// runs. The intro animation (animationScreen.Init) issues tickAnimation();
	// the Update screen (updateScreen.Init) kicks
	// the update check; the Progress screen (progressScreen.Init) triggers the
	// install. App.Init therefore must NOT duplicate those, or they would
	// double-fire.
	cmds = append(cmds, NavigateTo(a.screen))
	if a.animationsEnabled {
		cmds = append(cmds, tickUI())
	}
	// Preload install cache immediately on startup for faster Deep Dive/Manage transitions.
	// By loading during the intro animation, the cache is ready when the user
	// navigates to those screens.
	if cmd := a.startInstallCacheLoad(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
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

// countBackupFiles counts backed-up dotfiles in a backup directory.
// The manifest (backup.ManifestName) is metadata, not a backed-up dotfile, so
// it is excluded so the displayed count matches what restore will actually write.
func countBackupFiles(path string) int {
	count := 0
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() && info.Name() != backup.ManifestName {
			count++
		}
		return nil
	})
	return count
}

// makeUniqueBackupDir returns a path inside backupsDir that does not yet exist,
// starting with baseName. If baseName is already taken, it appends _2, _3, etc.
// until a free slot is found. This prevents two backups created in the same
// second from silently overwriting each other.
func makeUniqueBackupDir(backupsDir, baseName string) string {
	candidate := filepath.Join(backupsDir, baseName)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = filepath.Join(backupsDir, fmt.Sprintf("%s_%d", baseName, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// calcDirSize calculates the total size of files in a directory
func calcDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, _ error) error {
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

// restoreBackupCmd restores files from a backup. The path mapping, traversal
// guard, and mode preservation are shared with the CLI via the
// internal/backup package so the two paths cannot diverge.
func restoreBackupCmd(b BackupEntry) tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return backupRestoreDoneMsg{name: b.Name, err: err}
		}

		result, err := backup.Restore(b.Path, home)
		if err != nil {
			return backupRestoreDoneMsg{name: b.Name, err: err}
		}

		// Surface skipped files (path-traversal rejection, write-through-symlink
		// refusal, read/mkdir errors) instead of discarding them. A restore where
		// every file is skipped must NOT report green success (C3); mirrors the
		// CLI, which prints each skipped reason.
		return backupRestoreDoneMsg{
			name:    b.Name,
			count:   result.Count(),
			skipped: len(result.Skipped),
			err:     nil,
		}
	}
}

// deleteBackupCmd deletes a backup directory
func deleteBackupCmd(b BackupEntry) tea.Cmd {
	return func() tea.Msg {
		err := os.RemoveAll(b.Path)
		return backupDeleteDoneMsg{name: b.Name, err: err}
	}
}

// createBackupCmd creates a new backup of current dotfiles
func createBackupCmd() tea.Cmd {
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return backupCreateDoneMsg{err: err}
		}

		// Derive a unique backup directory: start with the current second as the
		// name, then append _2, _3, etc. if a same-second backup already exists
		// so two rapid backups never silently overwrite each other.
		timestamp := time.Now().Format("2006-01-02_15-04-05")
		backupsDir := filepath.Join(config.ConfigDir(), "backups")
		backupDir := makeUniqueBackupDir(backupsDir, timestamp)
		backupName := filepath.Base(backupDir)

		// backup.Create is the single source of truth for the capture loop and
		// reports an honest result: an error when zero files were captured or
		// the manifest write fails, instead of silently claiming success (C4).
		if _, err := backup.Create(home, backupDir, defaultBackupFiles); err != nil {
			return backupCreateDoneMsg{err: err}
		}

		// Run backup cleanup based on settings
		cleanupBackups()

		return backupCreateDoneMsg{name: backupName, err: nil}
	}
}

// defaultBackupFiles is the fixed set of dotfiles captured by both the manual
// "create backup" action and the pre-install auto-backup. Paths are relative
// to the user's home directory.
var defaultBackupFiles = []string{
	".zshrc",
	".tmux.conf",
	".config/nvim/init.lua",
	".config/ghostty/config",
	".config/yazi/yazi.toml",
	".gitconfig",
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
	for i, bk := range backups {
		shouldDelete := false

		// Delete if exceeds max count (and max count is set)
		if cfg.BackupMaxCount > 0 && i >= cfg.BackupMaxCount {
			shouldDelete = true
		}

		// Delete if exceeds max age (and max age is set)
		if cfg.BackupMaxAgeDays > 0 {
			age := now.Sub(bk.modTime)
			if age > time.Duration(cfg.BackupMaxAgeDays)*24*time.Hour {
				shouldDelete = true
			}
		}

		if shouldDelete {
			backupPath := filepath.Join(backupsDir, bk.name)
			os.RemoveAll(backupPath)
		}
	}
}

// autoBackupResult reports what the pre-install auto-backup actually did so the
// install worker can be honest with the user (C5). enabled is false when
// auto-backup is turned off (no backup attempted, no warning).
type autoBackupResult struct {
	enabled bool // auto-backup is on in settings
	count   int  // number of files actually captured
}

// autoBackupIfEnabled creates a backup if auto-backup is enabled in settings.
// It returns a result describing whether a backup was attempted and how many
// files were captured, plus an error if the backup was attempted but failed
// (zero files captured or manifest write failed). The pre-install path overwrites
// the user's dotfiles, so it MUST NOT claim a backup exists unless at least one
// file was written and the manifest persisted (C5).
func autoBackupIfEnabled() (autoBackupResult, error) {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return autoBackupResult{}, err
	}

	if !cfg.AutoBackup {
		return autoBackupResult{enabled: false}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return autoBackupResult{enabled: true}, err
	}

	// Derive a unique backup directory (same collision logic as createBackupCmd).
	// Auto-backups use an "_auto" suffix to distinguish them visually from
	// manual backups; additional _2, _3, etc. suffixes guard against same-second
	// collisions (two installs started in the same second).
	timestamp := time.Now().Format("2006-01-02_15-04-05") + "_auto"
	backupsDir := filepath.Join(config.ConfigDir(), "backups")
	backupDir := makeUniqueBackupDir(backupsDir, timestamp)

	// backup.Create returns an error when zero files were captured or the
	// manifest write fails, so a "success" here genuinely means a rollback
	// point exists.
	count, err := backup.Create(home, backupDir, defaultBackupFiles)
	if err != nil {
		return autoBackupResult{enabled: true, count: count}, err
	}

	// Run cleanup after creating backup
	cleanupBackups()

	return autoBackupResult{enabled: true, count: count}, nil
}

// Update handles messages
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window resize for screen manager
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		a.width = wsm.Width
		a.height = wsm.Height
		a.screenMgr.SetSize(wsm.Width, wsm.Height)
		return a, nil
	}

	// uiTickMsg drives the global animation frame counter for all screens. It
	// must be handled before delegating to the manager: the manager would
	// otherwise consume the message and the tickUI() chain would never be
	// re-issued, freezing the animated header.
	if _, ok := msg.(uiTickMsg); ok {
		if !a.animationsEnabled {
			return a, nil
		}
		a.uiFrame++
		a.screenMgr.IncrementUIFrame()
		return a, tickUI()
	}

	// installCacheDoneMsg carries the result of the app-wide install-status cache,
	// which is preloaded at startup and shared by the deep-dive/manage screens. It
	// can complete while ANY screen is active, so apply it globally before
	// delegating to the manager (managed screens would otherwise drop it and the
	// cache would never mark ready).
	if m, ok := msg.(installCacheDoneMsg); ok {
		a.manageInstalled = m.installed
		a.manageInstalledReady = true
		a.installCacheLoading = false
		return a, nil
	}

	// Streaming/terminal async messages for the package-update and tool-install
	// flows are handled GLOBALLY here, before delegating, exactly like
	// installCacheDoneMsg above. Their re-arm/finalize/cache-refresh chain
	// outlives the originating screen (the worker goroutine + package-manager
	// subprocess do too), so handling them only in the originating screen's
	// Update would drop the message when the user has navigated away — wedging
	// the running flag, dropping the result, and leaking the worker + subprocess
	// (cluster A: C0, C1, C9). The screen handlers delegate to these same *App
	// methods, so behavior is identical whether or not the screen is still active.
	switch m := msg.(type) {
	case updateStreamMsg:
		return a, a.handleUpdateStreamMsg(m)
	case updateStartMsg:
		return a, a.handleUpdateStartMsg(m)
	case updateSudoRequiredMsg:
		return a, a.handleUpdateSudoRequiredMsg(m)
	case manageInstallWithLogsMsg:
		return a, a.handleManageInstallWithLogsMsg(m)
	case manageInstallDoneMsg:
		return a, a.handleManageInstallDoneMsg(m)
	case manageStartInstallMsg:
		return a, a.handleManageStartInstallMsg(m)
	case manageSudoRequiredMsg:
		return a, a.handleManageSudoRequiredMsg(m)
	}

	// Every live screen is a migrated ScreenHandler, so the ScreenManager owns
	// all input and async messages. In managed mode it handles every message
	// (returning handled=true), including the per-screen ctrl+c/q quit. Any
	// message it does not claim (e.g. before the first navigation lands) is a
	// no-op here.
	//
	// Note: the intro animation (tickMsg / animationDoneMsg),
	// the install flow (installStartMsg / sudoRequiredMsg / sudoCachedMsg /
	// installOutputMsg / installEventMsg / installDoneMsg / installLogMsg) and the
	// Users async results (userLoadedMsg / userSavedMsg / userDeletedMsg /
	// userSwitchedMsg) are all handled by their migrated ScreenHandlers via the
	// ScreenManager, which delegates every non-navigation message to the active
	// handler.
	cmd, _ := a.screenMgr.Update(msg)
	return a, cmd
}

// View renders the UI through the ScreenManager. Every live screen is a
// migrated ScreenHandler, so the manager always renders the current screen.
func (a *App) View() string {
	// Defensive: if no navigation has landed yet (no current screen), navigate to
	// the start screen so the first frame is never blank. NewApp already navigates
	// at construction, so this is belt-and-suspenders.
	if a.screenMgr.Current() == nil {
		a.screenMgr.Navigate(a.screen)
	}
	return a.screenMgr.View()
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

	// If we're being routed straight to a single tool's config screen (i.e.
	// `dotfiles config <tool>`), remember that so back() persists the edits to
	// the real config files and quits rather than bouncing to the deep-dive menu
	// (which only exists in the install wizard) and discarding them (C27).
	a.configStandalone = screenIsToolConfig(screen)

	// Seed the in-memory deep-dive config from the persisted manage.json so a
	// standalone `dotfiles config <tool>` edits (and on exit re-writes) the user's
	// SAVED preferences rather than the compiled-in defaults. Without this the
	// opened tool's other fields would silently reset to defaults on save, and the
	// scoped write would persist defaults over prior Manage customizations (FIX 1;
	// also closes the deferred T3 cross-launch-persistence gap). Uses the SAME
	// translation the Manage save path uses so the two stay in sync.
	if a.configStandalone {
		dd := manageConfigToDeepDive(a.manageConfig)
		a.deepDiveConfig = &dd
	}

	// CLI `dotfiles theme` routes directly to ScreenThemePicker. Set
	// themeStandalone=true so Enter persists+quits (the whole TUI was started
	// just for the picker) instead of advancing into the install wizard (C7).
	// themeStandalone is the explicit mode flag for standalone vs wizard.
	// Within standalone mode, themeReturn==ScreenMainMenu distinguishes an
	// in-TUI call (main menu "Theme") from a CLI call (constructor default
	// ScreenWelcome), so we leave themeReturn unchanged here.
	if screen == ScreenThemePicker {
		a.themeStandalone = true
	}

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

// GetToolConfigScreen returns the dedicated config screen for a tool ID, used by
// the `dotfiles config <tool>` CLI command.
//
// It delegates to the authoritative toolConfigScreens map so the CLI can only
// open tools that actually have a config screen (and opens the SAME screen the
// rest of the TUI uses). Previously this kept a private, divergent map that
// advertised non-existent screens (e.g. "apps", "utilities") and omitted real
// ones (lazygit, lazydocker, btop, glow, claude-code) — C28.
func GetToolConfigScreen(tool string) (Screen, bool) {
	screen, ok := toolConfigScreens[tool]
	return screen, ok
}

// ConfigurableToolIDs returns the sorted list of tool IDs that `dotfiles config`
// can open, derived from the authoritative toolConfigScreens map. Used to build
// accurate CLI help/error strings (C28).
func ConfigurableToolIDs() []string {
	ids := make([]string, 0, len(toolConfigScreens))
	for id := range toolConfigScreens {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
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
		// Tools with dedicated screens (UIGroupNone with a configScreen set).
		// Use the authoritative toolConfigScreens map (keyed by tool ID) rather
		// than converting the raw int, so the Screen constant is named
		// symbolically and stays correct if the iota is reordered.
		if t.UIGroup() == tools.UIGroupNone && t.ConfigScreen() != 0 {
			if screen, ok := toolConfigScreens[t.ID()]; ok {
				result[screen] = append(result[screen], t.ID())
			}
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
