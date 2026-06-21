package ui

import (
	"testing"
)

func TestNewScreenManager(t *testing.T) {
	ctx := NewTestScreenContext()
	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		return nil
	}

	mgr := NewScreenManager(ctx, factory)

	if mgr.Context() != ctx {
		t.Error("Context() should return set context")
	}
	if mgr.Current() != nil {
		t.Error("Current() should be nil before the first Navigate")
	}
}

func TestScreenManager_SetSize(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	mgr.SetSize(100, 50)

	if ctx.Width != 100 {
		t.Errorf("Width = %d, want 100", ctx.Width)
	}
	if ctx.Height != 50 {
		t.Errorf("Height = %d, want 50", ctx.Height)
	}
}

func TestScreenManager_IncrementUIFrame(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	if ctx.UIFrame != 0 {
		t.Errorf("UIFrame = %d, want 0", ctx.UIFrame)
	}

	mgr.IncrementUIFrame()
	if ctx.UIFrame != 1 {
		t.Errorf("UIFrame = %d, want 1", ctx.UIFrame)
	}

	mgr.IncrementUIFrame()
	if ctx.UIFrame != 2 {
		t.Errorf("UIFrame = %d, want 2", ctx.UIFrame)
	}
}

func TestScreenManager_Navigate_Managed(t *testing.T) {
	ctx := NewTestScreenContext()

	testScreen := &mockScreenHandler{screenID: ScreenError}

	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		if id == ScreenError {
			return testScreen
		}
		return nil
	}

	mgr := NewScreenManager(ctx, factory)

	cmd := mgr.Navigate(ScreenError)
	// Init may return nil
	_ = cmd

	if mgr.Current() != testScreen {
		t.Error("Current() should be the test screen")
	}
	if !testScreen.initCalled {
		t.Error("Init() should have been called")
	}
}

func TestScreenManager_Update_NavigateMsg(t *testing.T) {
	ctx := NewTestScreenContext()

	testScreen := &mockScreenHandler{screenID: ScreenError}

	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		if id == ScreenError {
			return testScreen
		}
		return nil
	}

	mgr := NewScreenManager(ctx, factory)

	// Send NavigateMsg
	msg := NavigateMsg{To: ScreenError}
	cmd, handled := mgr.Update(msg)

	if !handled {
		t.Error("NavigateMsg should be handled")
	}
	_ = cmd // may be nil

	if mgr.Current() != testScreen {
		t.Error("Current() should be the test screen after NavigateMsg")
	}
}

func TestScreenManager_Update_BeforeNavigate(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	// A non-navigation message before the first Navigate must not panic and
	// must report "not handled" (current is still nil).
	_, handled := mgr.Update(uiTickMsg{})
	if handled {
		t.Error("Update should report not-handled before the first Navigate")
	}
}

func TestScreenManager_View_BeforeNavigate(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	view := mgr.View()
	if view != "" {
		t.Error("View() should return empty string before the first Navigate")
	}
}

func TestScreenManager_View_ManagedMode(t *testing.T) {
	ctx := NewTestScreenContext()

	testScreen := &mockScreenHandler{screenID: ScreenError}

	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		return testScreen
	}

	mgr := NewScreenManager(ctx, factory)
	mgr.Navigate(ScreenError)

	view := mgr.View()
	if view != "mock view" {
		t.Errorf("View() = %q, want %q", view, "mock view")
	}
	if !testScreen.viewCalled {
		t.Error("View() should have been called on screen")
	}
}

// allNavigableScreens lists every Screen constant that is reachable via
// navigation. The retired ScreenConfigApps iota slot is intentionally omitted
// because it has no name and is never navigated to. If a new navigable screen is
// added it MUST appear here and in the factory; otherwise TestFactory_FailsLoud
// catches the gap.
var allNavigableScreens = []Screen{
	ScreenAnimation,
	ScreenWelcome,
	ScreenThemePicker,
	ScreenNavPicker,
	ScreenFileTree,
	ScreenProgress,
	ScreenSummary,
	ScreenError,
	ScreenDeepDiveMenu,
	ScreenConfigGhostty,
	ScreenConfigTmux,
	ScreenConfigZsh,
	ScreenConfigNeovim,
	ScreenConfigGit,
	ScreenConfigYazi,
	ScreenConfigFzf,
	ScreenConfigUtilities,
	ScreenConfigMacApps,
	ScreenMainMenu,
	ScreenManage,
	ScreenUpdate,
	ScreenHotkeys,
	ScreenBackups,
	ScreenUsers,
	ScreenConfigCLITools,
	ScreenConfigGUIApps,
	ScreenConfigCLIUtilities,
	ScreenConfigLazyGit,
	ScreenConfigLazyDocker,
	ScreenConfigBtop,
	ScreenConfigGlow,
	ScreenConfigClaudeCode,
}

// TestFactory_AllNavigableScreensMapped proves the factory returns a non-nil
// handler for every navigable screen, so the fail-loud panic can never fire for
// a real screen during normal use.
func TestFactory_AllNavigableScreensMapped(t *testing.T) {
	ctx := NewTestScreenContext()
	f := NewFactory()

	for _, id := range allNavigableScreens {
		handler := f.Create(id, ctx)
		if handler == nil {
			t.Errorf("Factory.Create(%v) = nil; every navigable screen must map to a handler", id)
		}
	}
}

// TestFactory_FailsLoudOnUnmappedScreen verifies the factory panics (fail loud)
// rather than silently returning nil when handed an unmapped Screen value.
func TestFactory_FailsLoudOnUnmappedScreen(t *testing.T) {
	ctx := NewTestScreenContext()
	f := NewFactory()

	// A Screen value far beyond every defined constant is guaranteed unmapped.
	invalid := Screen(9999)

	defer func() {
		if r := recover(); r == nil {
			t.Error("Factory.Create with an unmapped screen should panic, but it did not")
		}
	}()

	f.Create(invalid, ctx)
}

// TestScreenManager_Navigate_UnmappedPanics verifies Navigate fails loud (via
// the factory panic) for an unmapped screen instead of entering an inert state.
func TestScreenManager_Navigate_UnmappedPanics(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, NewFactory().CreateFactory())

	defer func() {
		if r := recover(); r == nil {
			t.Error("Navigate to an unmapped screen should panic, but it did not")
		}
	}()

	mgr.Navigate(Screen(9999))
}
