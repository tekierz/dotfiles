package ui

// NewTestScreenContext creates a deterministic ScreenContext for testing:
// fixed dimensions, animations disabled, and the default theme/nav style. It
// does not touch persisted config, so it is safe under the hermetic temp-HOME
// test setup.
func NewTestScreenContext() *ScreenContext {
	return &ScreenContext{
		Theme:             "catppuccin-mocha",
		NavStyle:          "emacs",
		AnimationsEnabled: false, // Disable for tests
		Width:             80,
		Height:            24,
	}
}
