package ui

import (
	"fmt"
	"io"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	mainMenuIndex    int    // Main menu cursor
	manageIndex      int    // Manage screen cursor
	updateIndex      int    // Update screen cursor
	hotkeyFilter     string // Filter hotkeys by tool
	hotkeyCursor     int    // Hotkeys screen cursor
	hotkeyCategory   int    // Current category in hotkeys
	hotkeysPane      int    // 0 = categories, 1 = items
	hotkeyCatScroll  int    // Category list scroll
	hotkeyItemScroll int    // Item list scroll
	hotkeysReturn    Screen // Screen to return to when leaving hotkeys
	backupIndex      int    // Backup selection cursor

	// Update screen async state
	updateChecking  bool          // Currently checking for updates
	updateCheckDone bool          // Check completed (use cached results)
	updateResults   []pkg.Package // Cached update results
	updateError     error         // Error from update check
	updateRunning   bool          // Currently running an update operation
	updateStatus    string        // Status message for current update operation
	updateSelected  map[int]bool  // Selected packages for batch update

	// Error state
	lastError error
}

// NewApp creates a new application instance
func NewApp(skipIntro bool) *App {
	app := &App{
		skipIntro:         skipIntro,
		theme:             "catppuccin-mocha",
		navStyle:          "emacs",
		animationsEnabled: true,
		runner:            runner.NewRunner(),
		installOutput:     make([]string, 0, 100),
		deepDiveConfig:    NewDeepDiveConfig(),
		manageConfig:      NewManageConfig(),
		managePane:        0,
		postIntroScreen:   ScreenWelcome,
		hotkeysReturn:     ScreenMainMenu,
		updateSelected:    make(map[int]bool),
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

	if skipIntro {
		app.screen = ScreenWelcome
		app.animationDone = true
	} else {
		app.screen = ScreenAnimation
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
	err error
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

// updateRunDoneMsg indicates an update operation completed
type updateRunDoneMsg struct {
	results []pkg.UpdateResult
	err     error
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

// Update handles messages
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKey(msg)

	case tea.MouseMsg:
		return a.handleMouse(msg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

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
				return a, nil
			}
			return a, tickAnimation()
		}

	case uiTickMsg:
		if !a.animationsEnabled {
			return a, nil
		}
		a.uiFrame++
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
		// Keep only the last 20 lines for display
		if len(a.installOutput) > 20 {
			a.installOutput = a.installOutput[len(a.installOutput)-20:]
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
			a.lastError = msg.err
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

	case updateRunDoneMsg:
		a.updateRunning = false
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
	}

	return a, nil
}

func (a *App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch a.screen {
	case ScreenManage:
		return a.handleManageMouse(msg)
	case ScreenHotkeys:
		return a.handleHotkeysMouse(msg)
	case ScreenUpdate, ScreenBackups:
		return a.handleTabBarMouse(msg)
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
		return a, nil

	case ScreenWelcome:
		switch key {
		case "enter":
			if a.deepDive {
				a.screen = ScreenDeepDiveMenu
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
		menuItems := GetDeepDiveMenuItems()
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
	case ScreenConfigGhostty:
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
			case 0: // Font size
				if a.deepDiveConfig.GhosttyFontSize > 8 {
					a.deepDiveConfig.GhosttyFontSize--
				}
			case 1: // Opacity
				if a.deepDiveConfig.GhosttyOpacity > 0 {
					a.deepDiveConfig.GhosttyOpacity -= 5
				}
			case 2: // Tab bindings
				opts := []string{"super", "ctrl", "alt"}
				for i, o := range opts {
					if o == a.deepDiveConfig.GhosttyTabBindings && i > 0 {
						a.deepDiveConfig.GhosttyTabBindings = opts[i-1]
						break
					}
				}
			}
		case "right", "l":
			switch a.configFieldIndex {
			case 0:
				if a.deepDiveConfig.GhosttyFontSize < 32 {
					a.deepDiveConfig.GhosttyFontSize++
				}
			case 1:
				if a.deepDiveConfig.GhosttyOpacity < 100 {
					a.deepDiveConfig.GhosttyOpacity += 5
				}
			case 2:
				opts := []string{"super", "ctrl", "alt"}
				for i, o := range opts {
					if o == a.deepDiveConfig.GhosttyTabBindings && i < len(opts)-1 {
						a.deepDiveConfig.GhosttyTabBindings = opts[i+1]
						break
					}
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Tmux config
	case ScreenConfigTmux:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 3 {
				a.configFieldIndex++
			}
		case "left", "right", "h", "l":
			switch a.configFieldIndex {
			case 0: // Prefix
				opts := []string{"ctrl-a", "ctrl-b", "ctrl-space"}
				a.deepDiveConfig.TmuxPrefix = cycleOption(opts, a.deepDiveConfig.TmuxPrefix, key == "right" || key == "l")
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
			}
		case " ":
			if a.configFieldIndex == 3 {
				a.deepDiveConfig.TmuxMouseMode = !a.deepDiveConfig.TmuxMouseMode
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Zsh config
	case ScreenConfigZsh:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 8 { // 4 prompts + 5 plugins - 1
				a.configFieldIndex++
			}
		case "left", "right", "h", "l", " ":
			if a.configFieldIndex < 4 {
				// Prompt style selection
				opts := []string{"p10k", "starship", "pure", "minimal"}
				a.deepDiveConfig.ZshPromptStyle = opts[a.configFieldIndex]
			} else {
				// Plugin toggle
				plugins := []string{"zsh-autosuggestions", "zsh-syntax-highlighting", "zsh-completions", "fzf-tab", "zsh-history-substring-search"}
				pluginIdx := a.configFieldIndex - 4
				if pluginIdx < len(plugins) {
					togglePlugin(&a.deepDiveConfig.ZshPlugins, plugins[pluginIdx])
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Neovim config
	case ScreenConfigNeovim:
		switch key {
		case "up", "k":
			if a.configFieldIndex > 0 {
				a.configFieldIndex--
			}
		case "down", "j":
			if a.configFieldIndex < 9 { // 4 configs + 6 LSPs - 1
				a.configFieldIndex++
			}
		case "left", "right", "h", "l", " ":
			if a.configFieldIndex < 4 {
				opts := []string{"kickstart", "lazyvim", "nvchad", "custom"}
				a.deepDiveConfig.NeovimConfig = opts[a.configFieldIndex]
			} else {
				lsps := []string{"lua_ls", "pyright", "tsserver", "gopls", "rust_analyzer", "clangd"}
				lspIdx := a.configFieldIndex - 4
				if lspIdx < len(lsps) {
					togglePlugin(&a.deepDiveConfig.NeovimLSPs, lsps[lspIdx])
				}
			}
		case "esc", "enter":
			a.configFieldIndex = 0
			a.screen = ScreenDeepDiveMenu
		}

	// Git config
	case ScreenConfigGit:
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
			if a.configFieldIndex == 1 {
				opts := []string{"main", "master", "develop"}
				a.deepDiveConfig.GitDefaultBranch = cycleOption(opts, a.deepDiveConfig.GitDefaultBranch, key == "right" || key == "l")
			}
		case " ":
			if a.configFieldIndex == 0 {
				a.deepDiveConfig.GitDeltaSideBySide = !a.deepDiveConfig.GitDeltaSideBySide
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
			a.deepDiveConfig.MacApps[app] = !a.deepDiveConfig.MacApps[app]
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
			a.deepDiveConfig.Utilities[util] = !a.deepDiveConfig.Utilities[util]
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
			a.deepDiveConfig.CLITools[tool] = !a.deepDiveConfig.CLITools[tool]
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
			a.deepDiveConfig.GUIApps[app] = !a.deepDiveConfig.GUIApps[app]
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
			a.deepDiveConfig.CLIUtilities[util] = !a.deepDiveConfig.CLIUtilities[util]
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
			a.screen = items[a.mainMenuIndex].Screen
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
			if len(a.updateResults) > 0 && !a.updateChecking {
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
					a.updateRunning = true
					a.updateStatus = fmt.Sprintf("Updating %d package(s)...", len(packagesToUpdate))
					return a, runUpdateCmd(packagesToUpdate)
				}
			}
		case "a": // Update all packages
			if len(a.updateResults) > 0 && !a.updateChecking {
				a.updateRunning = true
				a.updateStatus = "Updating all packages..."
				return a, runUpdateAllCmd()
			}
		case "r": // Refresh updates
			a.updateCheckDone = false
			a.updateChecking = true
			a.updateResults = nil
			a.updateError = nil
			a.updateStatus = ""
			a.updateSelected = make(map[int]bool)
			return a, checkUpdatesCmd()
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

	// Backups screen navigation
	case ScreenBackups:
		// Handle tab navigation first
		if handled, cmd := a.handleTabNavigationWithCmd(key); handled {
			return a, cmd
		}
		switch key {
		case "esc":
			a.screen = ScreenMainMenu
		}
	}

	return a, nil
}

// View renders the UI
func (a *App) View() string {
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
	case ScreenBackups:
		return a.renderBackups()
	default:
		return "Unknown screen"
	}
}

// startInstallation begins the installation process
func (a *App) startInstallation() tea.Cmd {
	if a.installRunning {
		return nil
	}

	a.installRunning = true
	a.installStep = 0
	a.installOutput = []string{}

	// Configure the runner with user selections
	a.runner.Theme = a.theme
	a.runner.NavStyle = a.navStyle

	return func() tea.Msg {
		cmd, stdout, stderr, err := a.runner.RunSetup()
		if err != nil {
			return installDoneMsg{err: err}
		}
		a.installCmd = cmd

		// Create channels for output
		outputCh := make(chan runner.OutputLine, 100)

		// Start goroutines to read output
		go runner.StreamOutput(stdout, outputCh, "stdout")
		go runner.StreamOutput(stderr, outputCh, "stderr")

		// Wait for command to complete
		go func() {
			waitErr := cmd.Wait()
			close(outputCh)
			// Note: We can't easily send tea.Msg from here
			// The UI will detect completion via installRunning flag
			if waitErr != nil {
				a.lastError = waitErr
			}
			a.installRunning = false
			a.installComplete = true
		}()

		// Read output in this goroutine and return
		// Note: This is a simplified version - full async would need more work
		for line := range outputCh {
			a.installOutput = append(a.installOutput, line.Text)
			if len(a.installOutput) > 20 {
				a.installOutput = a.installOutput[len(a.installOutput)-20:]
			}
			if line.Type == runner.OutputStep {
				a.installStep++
			}
		}

		return installDoneMsg{err: nil}
	}
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
		sudo -v
		echo ""
		echo "Press any key to continue..."
		read -n 1
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
	ScreenConfigUtilities:    {}, // hk, caff, sshh are not in registry
}

// ensureInstallCache populates the install status cache if not already done
func (a *App) ensureInstallCache() {
	if a.manageInstalledReady {
		return
	}

	reg := tools.NewRegistry()
	all := reg.All()

	if a.manageInstalled == nil {
		a.manageInstalled = make(map[string]bool, len(all))
	}

	for _, t := range all {
		a.manageInstalled[t.ID()] = t.IsInstalled()
	}
	a.manageInstalledReady = true
}

// getDeepDiveItemStatus returns the install status for a deep dive menu item
// Returns "success" if installed, "warning" if partially installed (groups), "pending" if not
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
		return "success" // All installed
	} else if installedCount > 0 {
		return "warning" // Partially installed (for groups)
	}
	return "pending" // None installed
}
