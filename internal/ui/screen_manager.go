package ui

import tea "github.com/charmbracelet/bubbletea"

// ScreenManager handles screen transitions between migrated screens.
// It provides a clean separation between the App's Bubble Tea model and
// individual screen implementations.
type ScreenManager struct {
	// Current screen (nil if using legacy screen handling)
	current ScreenHandler

	// Screen factory for creating new screens
	factory ScreenFactory

	// Shared context passed to all screens
	ctx *ScreenContext

	// Legacy mode: when true, the manager delegates to the old App handling
	// This allows incremental migration
	legacyMode bool

	// Legacy screen ID (used when legacyMode is true)
	legacyScreen Screen
}

// NewScreenManager creates a new screen manager with the given context
func NewScreenManager(ctx *ScreenContext, factory ScreenFactory) *ScreenManager {
	return &ScreenManager{
		ctx:        ctx,
		factory:    factory,
		legacyMode: true, // Start in legacy mode for gradual migration
	}
}

// SetLegacyScreen sets the legacy screen ID (for incremental migration)
func (sm *ScreenManager) SetLegacyScreen(s Screen) {
	sm.legacyScreen = s
	sm.legacyMode = true
	sm.current = nil
}

// LegacyScreen returns the current legacy screen ID
func (sm *ScreenManager) LegacyScreen() Screen {
	return sm.legacyScreen
}

// IsLegacyMode returns true if the manager is in legacy mode
func (sm *ScreenManager) IsLegacyMode() bool {
	return sm.legacyMode
}

// Current returns the current screen handler (nil if in legacy mode)
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

// Navigate changes to a new screen.
// If the screen implements ScreenHandler, it switches to managed mode.
// Otherwise, it stays in legacy mode with the screen ID.
func (sm *ScreenManager) Navigate(screenID Screen) tea.Cmd {
	// Try to create a screen handler using the factory
	if sm.factory != nil {
		if handler := sm.factory(screenID, sm.ctx); handler != nil {
			return sm.navigateToHandler(handler)
		}
	}

	// Fall back to legacy mode
	sm.legacyScreen = screenID
	sm.legacyMode = true
	sm.current = nil
	return nil
}

// navigateToHandler switches to a managed screen handler
func (sm *ScreenManager) navigateToHandler(handler ScreenHandler) tea.Cmd {
	// Inject context if the screen supports it
	if setter, ok := handler.(ContextSetter); ok {
		setter.SetContext(sm.ctx)
	}

	sm.current = handler
	sm.legacyMode = false

	// Return the screen's init command
	return handler.Init()
}

// Update handles a message for the current screen.
// Returns the model, command, and whether the message was handled.
func (sm *ScreenManager) Update(msg tea.Msg) (tea.Cmd, bool) {
	// Handle navigation messages
	if m, ok := msg.(NavigateMsg); ok {
		return sm.Navigate(m.To), true
	}

	// If in legacy mode, don't handle the message
	if sm.legacyMode || sm.current == nil {
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
// Returns empty string if in legacy mode.
func (sm *ScreenManager) View() string {
	if sm.legacyMode || sm.current == nil {
		return ""
	}
	return sm.current.View(sm.ctx.Width, sm.ctx.Height)
}
