package ui

import tea "github.com/charmbracelet/bubbletea"

// ScreenManager handles screen transitions and maintains navigation history.
// It provides a clean separation between the App's Bubble Tea model and
// individual screen implementations.
type ScreenManager struct {
	// Current screen (nil if using legacy screen handling)
	current ScreenHandler

	// Screen factory for creating new screens
	factory ScreenFactory

	// Navigation history for back navigation
	history []ScreenHandler

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
		history:    make([]ScreenHandler, 0, 10),
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
			return sm.navigateToHandler(handler, false)
		}
	}

	// Fall back to legacy mode
	sm.legacyScreen = screenID
	sm.legacyMode = true
	sm.current = nil
	return nil
}

// NavigateWithPush changes to a new screen, saving the current for back navigation
func (sm *ScreenManager) NavigateWithPush(screenID Screen) tea.Cmd {
	// Save current to history
	if sm.current != nil {
		sm.history = append(sm.history, sm.current)
	}

	return sm.Navigate(screenID)
}

// navigateToHandler switches to a managed screen handler
func (sm *ScreenManager) navigateToHandler(handler ScreenHandler, pushCurrent bool) tea.Cmd {
	// Save current to history if requested
	if pushCurrent && sm.current != nil {
		sm.history = append(sm.history, sm.current)
	}

	// Inject context if the screen supports it
	if setter, ok := handler.(ContextSetter); ok {
		setter.SetContext(sm.ctx)
	}

	sm.current = handler
	sm.legacyMode = false

	// Return the screen's init command
	return handler.Init()
}

// NavigateBack goes back to the previous screen in history
func (sm *ScreenManager) NavigateBack() tea.Cmd {
	if len(sm.history) == 0 {
		// No history - go to main menu in legacy mode
		sm.legacyScreen = ScreenMainMenu
		sm.legacyMode = true
		sm.current = nil
		return nil
	}

	// Pop from history
	prev := sm.history[len(sm.history)-1]
	sm.history = sm.history[:len(sm.history)-1]

	// Restore context if needed
	if setter, ok := prev.(ContextSetter); ok {
		setter.SetContext(sm.ctx)
	}

	sm.current = prev
	sm.legacyMode = false

	return nil
}

// ClearHistory clears the navigation history
func (sm *ScreenManager) ClearHistory() {
	sm.history = sm.history[:0]
}

// Update handles a message for the current screen.
// Returns the model, command, and whether the message was handled.
func (sm *ScreenManager) Update(msg tea.Msg) (tea.Cmd, bool) {
	// Handle navigation messages
	switch m := msg.(type) {
	case NavigateMsg:
		if m.PushBack {
			return sm.NavigateWithPush(m.To), true
		}
		return sm.Navigate(m.To), true

	case NavigateBackMsg:
		return sm.NavigateBack(), true
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

// HistoryDepth returns the current navigation history depth
func (sm *ScreenManager) HistoryDepth() int {
	return len(sm.history)
}
