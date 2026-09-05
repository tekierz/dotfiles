package ui

import tea "github.com/charmbracelet/bubbletea"

// ScreenManager handles screen transitions between screen handlers.
// It provides a clean separation between the App's Bubble Tea model and
// individual screen implementations.
type ScreenManager struct {
	// Current screen (nil until preparation or navigation).
	current            ScreenHandler
	currentInitialized bool

	// Screen factory for creating new screens
	factory ScreenFactory

	// Shared context passed to all screens
	ctx *ScreenContext
}

// NewScreenManager creates a new screen manager with the given context.
// current is nil until preparation or navigation selects a handler.
func NewScreenManager(ctx *ScreenContext, factory ScreenFactory) *ScreenManager {
	return &ScreenManager{
		ctx:     ctx,
		factory: factory,
	}
}

// Current returns the selected screen handler, even before its Init runs.
func (sm *ScreenManager) Current() ScreenHandler {
	return sm.current
}

// Context returns the shared screen context
func (sm *ScreenManager) Context() *ScreenContext {
	return sm.ctx
}

// SetSize updates the context dimensions
func (sm *ScreenManager) SetSize(width, height int) {
	sm.ctx.Width = width
	sm.ctx.Height = height
}

// IncrementUIFrame increments the UI animation frame counter
func (sm *ScreenManager) IncrementUIFrame() {
	sm.ctx.UIFrame++
}

// prepare selects a handler for rendering without beginning its async work.
// Startup can retarget this selection until App.Init owns the final Init command.
func (sm *ScreenManager) prepare(screenID Screen) {
	handler := sm.factory(screenID, sm.ctx)
	if handler == nil {
		panic("screen manager: factory returned nil handler — every navigable screen must be mapped")
	}
	if setter, ok := handler.(ContextSetter); ok {
		setter.SetContext(sm.ctx)
	}
	sm.current = handler
	sm.currentInitialized = false
}

// initCurrent transfers this handler's one initialization command to the caller.
func (sm *ScreenManager) initCurrent() tea.Cmd {
	if sm.current == nil || sm.currentInitialized {
		return nil
	}
	sm.currentInitialized = true
	return sm.current.Init()
}

// Navigate changes screens and returns its initialization command immediately.
// Normal event-loop navigation keeps its existing lifecycle behavior.
func (sm *ScreenManager) Navigate(screenID Screen) tea.Cmd {
	sm.prepare(screenID)
	return sm.initCurrent()
}

// Update handles a message for the current screen.
// Returns the command and whether the message was handled.
func (sm *ScreenManager) Update(msg tea.Msg) (tea.Cmd, bool) {
	// Handle navigation messages
	if m, ok := msg.(NavigateMsg); ok {
		return sm.Navigate(m.To), true
	}

	// Before the first Navigate there is no screen to delegate to.
	if sm.current == nil {
		return nil, false
	}

	// Delegate to current screen
	nextScreen, cmd := sm.current.Update(msg)

	// Check if screen changed
	if nextScreen != sm.current {
		// Inject context if needed
		if setter, ok := nextScreen.(ContextSetter); ok {
			setter.SetContext(sm.ctx)
		}
		sm.current = nextScreen
		sm.currentInitialized = true
	}

	return cmd, true
}

// View renders the current screen.
// Returns empty string before the first Navigate.
func (sm *ScreenManager) View() string {
	if sm.current == nil {
		return ""
	}
	return sm.current.View(sm.ctx.Width, sm.ctx.Height)
}
