package ui

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
func (f *Factory) Create(id Screen, ctx *ScreenContext) ScreenHandler {
	switch id {
	case ScreenError:
		return NewErrorScreen(ctx, f.data.Error)
	case ScreenSummary:
		return NewSummaryScreen(ctx)
	case ScreenWelcome:
		return NewWelcomeScreen(ctx)
	case ScreenThemePicker:
		return NewThemePickerScreen(ctx)
	case ScreenNavPicker:
		return NewNavPickerScreen(ctx)
	case ScreenFileTree:
		return NewFileTreeScreen(ctx)
	case ScreenMainMenu:
		return NewMainMenuScreen(ctx)
	case ScreenManage:
		return NewManageScreen(ctx)
	case ScreenDeepDiveMenu:
		return NewDeepDiveMenuScreen(ctx)
	case ScreenHotkeys:
		return NewHotkeysScreen(ctx)
	case ScreenBackups:
		return NewBackupsScreen(ctx)
	case ScreenUpdate:
		return NewUpdateScreen(ctx)
	case ScreenConfigGhostty:
		return NewConfigGhosttyScreen(ctx)
	case ScreenConfigTmux:
		return NewConfigTmuxScreen(ctx)
	case ScreenConfigZsh:
		return NewConfigZshScreen(ctx)
	case ScreenConfigNeovim:
		return NewConfigNeovimScreen(ctx)
	case ScreenConfigGit:
		return NewConfigGitScreen(ctx)
	case ScreenConfigYazi:
		return NewConfigYaziScreen(ctx)
	case ScreenConfigFzf:
		return NewConfigFzfScreen(ctx)
	case ScreenConfigUtilities:
		return NewConfigUtilitiesScreen(ctx)
	case ScreenConfigMacApps:
		return NewConfigMacAppsScreen(ctx)
	case ScreenConfigApps:
		return NewConfigAppsScreen(ctx)
	case ScreenConfigCLITools:
		return NewConfigCLIToolsScreen(ctx)
	case ScreenConfigGUIApps:
		return NewConfigGUIAppsScreen(ctx)
	case ScreenConfigCLIUtilities:
		return NewConfigCLIUtilitiesScreen(ctx)
	case ScreenConfigLazyGit:
		return NewConfigLazyGitScreen(ctx)
	case ScreenConfigLazyDocker:
		return NewConfigLazyDockerScreen(ctx)
	case ScreenConfigBtop:
		return NewConfigBtopScreen(ctx)
	case ScreenConfigGlow:
		return NewConfigGlowScreen(ctx)
	case ScreenConfigClaudeCode:
		return NewConfigClaudeCodeScreen(ctx)
	default:
		// Not migrated yet - return nil to use legacy handling
		return nil
	}
}

// CreateFactory returns a ScreenFactory function for use with ScreenManager.
func (f *Factory) CreateFactory() ScreenFactory {
	return func(id Screen, ctx *ScreenContext) ScreenHandler {
		return f.Create(id, ctx)
	}
}
