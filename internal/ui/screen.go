package ui

import tea "github.com/charmbracelet/bubbletea"

// ScreenHandler represents a UI screen with its own state and behavior.
// Each screen is responsible for:
// - Handling input (keyboard, mouse)
// - Rendering itself
// - Managing its own local state
// - Signaling navigation to other screens
//
// Note: This is separate from the Screen type (int enum) in app.go.
// During migration, screens will gradually implement ScreenHandler.
type ScreenHandler interface {
	// ID returns the unique identifier for this screen type
	ID() Screen

	// Init returns any initial commands to run when the screen is entered
	Init() tea.Cmd

	// Update handles a message and returns the updated screen and any commands.
	// If navigation to a different screen is needed, return a NavigateMsg.
	Update(msg tea.Msg) (ScreenHandler, tea.Cmd)

	// View renders the screen content.
	// Width and height are the available terminal dimensions.
	View(width, height int) string
}

// NavigateMsg signals that the screen wants to navigate to a different screen
type NavigateMsg struct {
	To       Screen
	PushBack bool // If true, current screen is saved for back navigation
}

// NavigateTo creates a NavigateMsg to go to a screen
func NavigateTo(id Screen) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{To: id, PushBack: false}
	}
}

// NavigatePush creates a NavigateMsg that saves current screen for back navigation
func NavigatePush(id Screen) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{To: id, PushBack: true}
	}
}

// NavigateBackMsg signals to go back to the previous screen
type NavigateBackMsg struct{}

// NavigateBack returns a command to go back to the previous screen
func NavigateBack() tea.Cmd {
	return func() tea.Msg {
		return NavigateBackMsg{}
	}
}

// ScreenContext provides shared state and dependencies to screens.
// This replaces the tight coupling to the App struct.
type ScreenContext struct {
	// Dependencies (injected)
	Deps *Dependencies

	// Shared state that screens can read/modify
	Theme             string
	NavStyle          string
	AnimationsEnabled bool

	// Dimensions
	Width  int
	Height int

	// UI animation frame counter
	UIFrame int
}

// BaseScreen provides common functionality for screen implementations.
// Embed this in concrete screen types to get default implementations.
type BaseScreen struct {
	ctx *ScreenContext
}

// Context returns the screen context
func (s *BaseScreen) Context() *ScreenContext {
	return s.ctx
}

// SetContext sets the screen context (called by ScreenManager)
func (s *BaseScreen) SetContext(ctx *ScreenContext) {
	s.ctx = ctx
}

// Width returns the screen width from context
func (s *BaseScreen) Width() int {
	if s.ctx == nil {
		return 80
	}
	return s.ctx.Width
}

// Height returns the screen height from context
func (s *BaseScreen) Height() int {
	if s.ctx == nil {
		return 24
	}
	return s.ctx.Height
}

// Theme returns the current theme from context
func (s *BaseScreen) Theme() string {
	if s.ctx == nil {
		return "catppuccin-mocha"
	}
	return s.ctx.Theme
}

// NavStyle returns the current nav style from context
func (s *BaseScreen) NavStyle() string {
	if s.ctx == nil {
		return "emacs"
	}
	return s.ctx.NavStyle
}

// AnimationsEnabled returns whether animations are enabled
func (s *BaseScreen) AnimationsEnabled() bool {
	if s.ctx == nil {
		return true
	}
	return s.ctx.AnimationsEnabled
}

// UIFrame returns the current UI animation frame
func (s *BaseScreen) UIFrame() int {
	if s.ctx == nil {
		return 0
	}
	return s.ctx.UIFrame
}

// ScreenFactory creates screen instances by ID.
// This allows the ScreenManager to create screens lazily.
type ScreenFactory func(id Screen, ctx *ScreenContext) ScreenHandler

// ContextSetter is implemented by screens that need context injection
type ContextSetter interface {
	SetContext(ctx *ScreenContext)
}
