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
	if !mgr.IsLegacyMode() {
		t.Error("should start in legacy mode")
	}
	if mgr.Current() != nil {
		t.Error("Current() should be nil initially")
	}
	if mgr.HistoryDepth() != 0 {
		t.Errorf("HistoryDepth() = %d, want 0", mgr.HistoryDepth())
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

func TestScreenManager_LegacyMode(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	// Initially in legacy mode
	if !mgr.IsLegacyMode() {
		t.Error("should start in legacy mode")
	}

	// Set legacy screen
	mgr.SetLegacyScreen(ScreenManage)

	if mgr.LegacyScreen() != ScreenManage {
		t.Errorf("LegacyScreen() = %v, want %v", mgr.LegacyScreen(), ScreenManage)
	}
	if !mgr.IsLegacyMode() {
		t.Error("should still be in legacy mode")
	}
}

func TestScreenManager_Navigate_Legacy(t *testing.T) {
	ctx := NewTestScreenContext()

	// Factory that returns nil (legacy fallback)
	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		return nil
	}

	mgr := NewScreenManager(ctx, factory)

	cmd := mgr.Navigate(ScreenHotkeys)
	if cmd != nil {
		t.Error("Navigate to legacy screen should return nil cmd")
	}

	if !mgr.IsLegacyMode() {
		t.Error("should be in legacy mode")
	}
	if mgr.LegacyScreen() != ScreenHotkeys {
		t.Errorf("LegacyScreen() = %v, want %v", mgr.LegacyScreen(), ScreenHotkeys)
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

	if mgr.IsLegacyMode() {
		t.Error("should not be in legacy mode")
	}
	if mgr.Current() != testScreen {
		t.Error("Current() should be the test screen")
	}
	if !testScreen.initCalled {
		t.Error("Init() should have been called")
	}
}

func TestScreenManager_NavigateWithPush(t *testing.T) {
	ctx := NewTestScreenContext()

	screen1 := &mockScreenHandler{screenID: ScreenError}
	screen2 := &mockScreenHandler{screenID: ScreenSummary}

	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		switch id {
		case ScreenError:
			return screen1
		case ScreenSummary:
			return screen2
		}
		return nil
	}

	mgr := NewScreenManager(ctx, factory)

	// Navigate to first screen
	mgr.Navigate(ScreenError)
	if mgr.HistoryDepth() != 0 {
		t.Errorf("HistoryDepth() = %d, want 0", mgr.HistoryDepth())
	}

	// Navigate with push
	mgr.NavigateWithPush(ScreenSummary)
	if mgr.HistoryDepth() != 1 {
		t.Errorf("HistoryDepth() = %d, want 1", mgr.HistoryDepth())
	}
	if mgr.Current() != screen2 {
		t.Error("Current() should be screen2")
	}
}

func TestScreenManager_NavigateBack(t *testing.T) {
	ctx := NewTestScreenContext()

	screen1 := &mockScreenHandler{screenID: ScreenError}
	screen2 := &mockScreenHandler{screenID: ScreenSummary}

	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		switch id {
		case ScreenError:
			return screen1
		case ScreenSummary:
			return screen2
		}
		return nil
	}

	mgr := NewScreenManager(ctx, factory)

	// Set up history
	mgr.Navigate(ScreenError)
	mgr.NavigateWithPush(ScreenSummary)

	// Go back
	mgr.NavigateBack()

	if mgr.Current() != screen1 {
		t.Error("Current() should be screen1 after back")
	}
	if mgr.HistoryDepth() != 0 {
		t.Errorf("HistoryDepth() = %d, want 0", mgr.HistoryDepth())
	}
}

func TestScreenManager_NavigateBack_NoHistory(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	// Navigate back with no history
	mgr.NavigateBack()

	if !mgr.IsLegacyMode() {
		t.Error("should be in legacy mode")
	}
	if mgr.LegacyScreen() != ScreenMainMenu {
		t.Errorf("LegacyScreen() = %v, want %v", mgr.LegacyScreen(), ScreenMainMenu)
	}
}

func TestScreenManager_ClearHistory(t *testing.T) {
	ctx := NewTestScreenContext()

	screen1 := &mockScreenHandler{screenID: ScreenError}
	screen2 := &mockScreenHandler{screenID: ScreenSummary}

	factory := func(id Screen, ctx *ScreenContext) ScreenHandler {
		switch id {
		case ScreenError:
			return screen1
		case ScreenSummary:
			return screen2
		}
		return nil
	}

	mgr := NewScreenManager(ctx, factory)

	// Build up history
	mgr.Navigate(ScreenError)
	mgr.NavigateWithPush(ScreenSummary)

	if mgr.HistoryDepth() != 1 {
		t.Errorf("HistoryDepth() = %d, want 1", mgr.HistoryDepth())
	}

	// Clear
	mgr.ClearHistory()

	if mgr.HistoryDepth() != 0 {
		t.Errorf("HistoryDepth() = %d, want 0 after clear", mgr.HistoryDepth())
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
	msg := NavigateMsg{To: ScreenError, PushBack: false}
	cmd, handled := mgr.Update(msg)

	if !handled {
		t.Error("NavigateMsg should be handled")
	}
	_ = cmd // may be nil

	if mgr.IsLegacyMode() {
		t.Error("should not be in legacy mode")
	}
}

func TestScreenManager_Update_NavigateBackMsg(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	msg := NavigateBackMsg{}
	cmd, handled := mgr.Update(msg)

	if !handled {
		t.Error("NavigateBackMsg should be handled")
	}
	_ = cmd // may be nil
}

func TestScreenManager_View_LegacyMode(t *testing.T) {
	ctx := NewTestScreenContext()
	mgr := NewScreenManager(ctx, nil)

	view := mgr.View()
	if view != "" {
		t.Error("View() should return empty string in legacy mode")
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
