package screens

import (
	"github.com/tekierz/dotfiles/internal/ui"
)

// ScreenData holds any data needed to create a screen.
// This is used to pass context-specific information to screens during creation.
type ScreenData struct {
	// Error is set when creating the error screen
	Error error
}

// Factory creates screen handlers based on screen ID.
// This is used by ScreenManager for navigation.
type Factory struct {
	data *ScreenData
}

// NewFactory creates a new screen factory.
func NewFactory() *Factory {
	return &Factory{
		data: &ScreenData{},
	}
}

// SetError sets the error for the next error screen creation.
func (f *Factory) SetError(err error) {
	f.data.Error = err
}

// Create returns a ScreenHandler for the given screen ID.
// Returns nil if the screen is not migrated yet (falls back to legacy).
func (f *Factory) Create(id ui.Screen, ctx *ui.ScreenContext) ui.ScreenHandler {
	switch id {
	case ui.ScreenError:
		return NewErrorScreen(ctx, f.data.Error)
	case ui.ScreenSummary:
		return NewSummaryScreen(ctx)
	default:
		// Not migrated yet - return nil to use legacy handling
		return nil
	}
}

// CreateFactory returns a ScreenFactory function for use with ScreenManager.
func (f *Factory) CreateFactory() ui.ScreenFactory {
	return func(id ui.Screen, ctx *ui.ScreenContext) ui.ScreenHandler {
		return f.Create(id, ctx)
	}
}
