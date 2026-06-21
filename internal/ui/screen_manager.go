package ui

import tea "github.com/charmbracelet/bubbletea"

// ScreenManager handles screen transitions between screen handlers.
// It provides a clean separation between the App's Bubble Tea model and
// individual screen implementations.
type ScreenManager struct {
	// Current screen (nil only before the first Navigate)
	current ScreenHandler

	// Screen factory for creating new screens
	factory ScreenFactory

	// Shared context passed to all screens
	ctx *ScreenContext
}

// NewScreenManager creates a new screen manager with the given context.
// current is nil until the first Navigate; callers Navigate the start screen at
// construction time so the first rendered frame is never blank.
func NewScreenManager(ctx *ScreenContext, factory ScreenFactory) *ScreenManager {
	return &ScreenManager{
		ctx:     ctx,
		factory: factory,
	}
}

// Current returns the current screen handler (nil before the first Navigate).
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

// Navigate changes to a new screen by creating its handler via the factory.
// The factory maps every navigable screen and panics on an unmapped one, so a
// successful Navigate always lands on a real handler.
func (sm *ScreenManager) Navigate(screenID Screen) tea.Cmd {
	handler := sm.factory(screenID, sm.ctx)
	if handler == nil {
		// Defensive: the factory is expected to panic on unmapped screens, but a
		// nil handler from a custom/test factory must also fail loud rather than
		// leaving the manager in an inert state.
		panic("screen manager: factory returned nil handler — every navigable screen must be mapped")
	}
	return sm.navigateToHandler(handler)
}

// navigateToHandler switches to a managed screen handler
func (sm *ScreenManager) navigateToHandler(handler ScreenHandler) tea.Cmd {
	// Inject context if the screen supports it
	if setter, ok := handler.(ContextSetter); ok {
		setter.SetContext(sm.ctx)
	}

	sm.current = handler

	// Return the screen's init command
	return handler.Init()
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
