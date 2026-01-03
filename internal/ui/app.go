package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
)

// App is the main application model
type App struct {
	screen        Screen
	skipIntro     bool
	width         int
	height        int
	animationDone bool

	// Animation state
	animFrame   int
	animTicker  *time.Ticker

	// User selections
	theme       string
	navStyle    string
	deepDive    bool

	// Error state
	lastError   error
}

// NewApp creates a new application instance
func NewApp(skipIntro bool) *App {
	app := &App{
		skipIntro: skipIntro,
		theme:     "catppuccin-mocha",
		navStyle:  "emacs",
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
	}

	return a, nil
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		if a.screen == ScreenWelcome {
			return a, tea.Quit
		}
		// Allow q to quit from welcome, but not other screens
		if a.screen == ScreenAnimation {
			a.screen = ScreenWelcome
			return a, nil
		}

	case "enter":
		return a.handleEnter()

	case "tab":
		if a.screen == ScreenWelcome {
			a.deepDive = !a.deepDive
		}

	case "left", "h":
		if a.screen == ScreenWelcome {
			a.deepDive = false
		}

	case "right", "l":
		if a.screen == ScreenWelcome {
			a.deepDive = true
		}

	case "esc":
		// Skip animation
		if a.screen == ScreenAnimation {
			a.screen = ScreenWelcome
			return a, nil
		}
		// Go back
		if a.screen > ScreenWelcome {
			a.screen--
		}
	}

	return a, nil
}

func (a *App) handleEnter() (tea.Model, tea.Cmd) {
	switch a.screen {
	case ScreenAnimation:
		a.screen = ScreenWelcome
	case ScreenWelcome:
		a.screen = ScreenThemePicker
	case ScreenThemePicker:
		a.screen = ScreenNavPicker
	case ScreenNavPicker:
		a.screen = ScreenFileTree
	case ScreenFileTree:
		a.screen = ScreenProgress
	case ScreenProgress:
		a.screen = ScreenSummary
	case ScreenSummary:
		return a, tea.Quit
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
	default:
		return "Unknown screen"
	}
}
