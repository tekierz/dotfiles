package ui

import "fmt"

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
// Every live, navigable screen is mapped below. An unmapped screen is a
// programming error (e.g. a new screen added without a factory case), so Create
// panics rather than silently returning nil — failing loud in dev/test instead
// of rendering a blank screen in production.
func (f *Factory) Create(id Screen, ctx *ScreenContext) ScreenHandler {
	// ScreenError needs f.data.Error, so it is not a uniform constructor(ctx)
	// call and stays out of the map-driven lookups below.
	if id == ScreenError {
		return NewErrorScreen(ctx, f.data.Error)
	}
	if h, ok := factoryCoreScreens[id]; ok {
		return h(ctx)
	}
	if h, ok := factoryConfigScreens[id]; ok {
		return h(ctx)
	}
	panic(fmt.Sprintf("screen factory: no handler for screen %d — every live screen must be mapped", id))
}

// factoryCoreScreens maps the non-config screens whose constructor is a uniform
// func(*ScreenContext) ScreenHandler.
var factoryCoreScreens = map[Screen]func(*ScreenContext) ScreenHandler{
	ScreenSummary:      func(ctx *ScreenContext) ScreenHandler { return NewSummaryScreen(ctx) },
	ScreenAnimation:    func(ctx *ScreenContext) ScreenHandler { return NewAnimationScreen(ctx) },
	ScreenProgress:     func(ctx *ScreenContext) ScreenHandler { return NewProgressScreen(ctx) },
	ScreenUsers:        func(ctx *ScreenContext) ScreenHandler { return NewUsersScreen(ctx) },
	ScreenWelcome:      func(ctx *ScreenContext) ScreenHandler { return NewWelcomeScreen(ctx) },
	ScreenThemePicker:  func(ctx *ScreenContext) ScreenHandler { return NewThemePickerScreen(ctx) },
	ScreenNavPicker:    func(ctx *ScreenContext) ScreenHandler { return NewNavPickerScreen(ctx) },
	ScreenFileTree:     func(ctx *ScreenContext) ScreenHandler { return NewFileTreeScreen(ctx) },
	ScreenMainMenu:     func(ctx *ScreenContext) ScreenHandler { return NewMainMenuScreen(ctx) },
	ScreenManage:       func(ctx *ScreenContext) ScreenHandler { return NewManageScreen(ctx) },
	ScreenDeepDiveMenu: func(ctx *ScreenContext) ScreenHandler { return NewDeepDiveMenuScreen(ctx) },
	ScreenHotkeys:      func(ctx *ScreenContext) ScreenHandler { return NewHotkeysScreen(ctx) },
	ScreenBackups:      func(ctx *ScreenContext) ScreenHandler { return NewBackupsScreen(ctx) },
	ScreenUpdate:       func(ctx *ScreenContext) ScreenHandler { return NewUpdateScreen(ctx) },
}

// factoryConfigScreens maps the per-tool config screens whose constructor is a
// uniform func(*ScreenContext) ScreenHandler.
var factoryConfigScreens = map[Screen]func(*ScreenContext) ScreenHandler{
	ScreenConfigGhostty:      func(ctx *ScreenContext) ScreenHandler { return NewConfigGhosttyScreen(ctx) },
	ScreenConfigTmux:         func(ctx *ScreenContext) ScreenHandler { return NewConfigTmuxScreen(ctx) },
	ScreenConfigZsh:          func(ctx *ScreenContext) ScreenHandler { return NewConfigZshScreen(ctx) },
	ScreenConfigNeovim:       func(ctx *ScreenContext) ScreenHandler { return NewConfigNeovimScreen(ctx) },
	ScreenConfigGit:          func(ctx *ScreenContext) ScreenHandler { return NewConfigGitScreen(ctx) },
	ScreenConfigYazi:         func(ctx *ScreenContext) ScreenHandler { return NewConfigYaziScreen(ctx) },
	ScreenConfigFzf:          func(ctx *ScreenContext) ScreenHandler { return NewConfigFzfScreen(ctx) },
	ScreenConfigUtilities:    func(ctx *ScreenContext) ScreenHandler { return NewConfigUtilitiesScreen(ctx) },
	ScreenConfigMacApps:      func(ctx *ScreenContext) ScreenHandler { return NewConfigMacAppsScreen(ctx) },
	ScreenConfigCLITools:     func(ctx *ScreenContext) ScreenHandler { return NewConfigCLIToolsScreen(ctx) },
	ScreenConfigGUIApps:      func(ctx *ScreenContext) ScreenHandler { return NewConfigGUIAppsScreen(ctx) },
	ScreenConfigCLIUtilities: func(ctx *ScreenContext) ScreenHandler { return NewConfigCLIUtilitiesScreen(ctx) },
	ScreenConfigLazyGit:      func(ctx *ScreenContext) ScreenHandler { return NewConfigLazyGitScreen(ctx) },
	ScreenConfigBtop:         func(ctx *ScreenContext) ScreenHandler { return NewConfigBtopScreen(ctx) },
	ScreenConfigGlow:         func(ctx *ScreenContext) ScreenHandler { return NewConfigGlowScreen(ctx) },
	ScreenConfigClaudeCode:   func(ctx *ScreenContext) ScreenHandler { return NewConfigClaudeCodeScreen(ctx) },
}

// CreateFactory returns a ScreenFactory function for use with ScreenManager.
func (f *Factory) CreateFactory() ScreenFactory {
	return func(id Screen, ctx *ScreenContext) ScreenHandler {
		return f.Create(id, ctx)
	}
}
