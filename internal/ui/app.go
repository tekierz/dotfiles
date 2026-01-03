package ui

import (
	"io"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/runner"
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
)

// Available themes
var themes = []struct {
	name  string
	desc  string
	color string
}{
	{"catppuccin-mocha", "Dark, warm pastels", "#89b4fa"},
	{"catppuccin-latte", "Light, warm pastels", "#1e66f5"},
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
	animFrame  int
	animTicker *time.Ticker

	// User selections
	themeIndex int
	theme      string
	navStyle   string
	deepDive   bool

	// Deep dive state
	deepDiveMenuIndex int
	deepDiveConfig    *DeepDiveConfig
	configFieldIndex  int // Currently focused field in config screens
	macAppIndex       int // Currently focused app in macOS screen
	utilityIndex      int // Currently focused utility

	// Installation state
	installStep     int
	installOutput   []string
	installRunning  bool
	installComplete bool
	installCmd      *exec.Cmd
	runner          *runner.Runner

	// Management platform state (new)
	mainMenuIndex   int      // Main menu cursor
	manageIndex     int      // Manage screen cursor
	updateIndex     int      // Update screen cursor
	hotkeyFilter    string   // Filter hotkeys by tool
	hotkeyCursor    int      // Hotkeys screen cursor
	hotkeyCategory  int      // Current category in hotkeys
	backupIndex     int      // Backup selection cursor

	// Error state
	lastError error
}

// NewApp creates a new application instance
func NewApp(skipIntro bool) *App {
	app := &App{
		skipIntro:      skipIntro,
		theme:          "catppuccin-mocha",
		navStyle:       "emacs",
		runner:         runner.NewRunner(),
		installOutput:  make([]string, 0, 100),
		deepDiveConfig: NewDeepDiveConfig(),
	}

	if skipIntro {
		app.screen = ScreenWelcome
	} else {
		app.screen = ScreenAnimation
	}

	return app
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	if a.screen == ScreenAnimation {
		return tea.Batch(
			tickAnimation(),
			checkDurdraw(),
		)
	}
	return nil
}

// tickMsg is sent on each animation frame
type tickMsg time.Time

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

func tickAnimation() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func checkDurdraw() tea.Cmd {
	return func() tea.Msg {
		available := DetectDurdraw()
		return durdrawAvailableMsg(available)
	}
}

// Update handles messages
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKey(msg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil

	case tickMsg:
		if a.screen == ScreenAnimation {
			a.animFrame++
			// Animation runs for about 30 frames (3 seconds)
			if a.animFrame >= 30 {
				a.animationDone = true
				a.screen = ScreenWelcome
				return a, nil
			}
			return a, tickAnimation()
		}

	case durdrawAvailableMsg:
		// Store durdraw availability if needed
		return a, nil

	case animationDoneMsg:
		a.animationDone = true
		a.screen = ScreenWelcome
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
	}

	return a, nil
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global quit handler
	if key == "ctrl+c" {
		return a, tea.Quit
	}

	// Screen-specific key handling
	switch a.screen {
	case ScreenAnimation:
		// Any key skips animation
		a.screen = ScreenWelcome
		return a, nil

	case ScreenWelcome:
		switch key {
		case "q":
			return a, tea.Quit
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
			}
		case "down", "j":
			if a.themeIndex < len(themes)-1 {
				a.themeIndex++
				a.theme = themes[a.themeIndex].name
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
	a.screen = screen
	if screen != ScreenAnimation && screen != ScreenWelcome {
		a.skipIntro = true
	}
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
			Icon:        "",
			Screen:      ScreenWelcome,
		},
		{
			Name:        "Manage",
			Description: "Configure installed tools",
			Icon:        "",
			Screen:      ScreenManage,
		},
		{
			Name:        "Update",
			Description: "Check and install updates",
			Icon:        "",
			Screen:      ScreenUpdate,
		},
		{
			Name:        "Theme",
			Description: "Change color theme",
			Icon:        "",
			Screen:      ScreenThemePicker,
		},
		{
			Name:        "Hotkeys",
			Description: "View keyboard shortcuts",
			Icon:        "",
			Screen:      ScreenHotkeys,
		},
		{
			Name:        "Backups",
			Description: "View and restore backups",
			Icon:        "",
			Screen:      ScreenBackups,
		},
	}
}
