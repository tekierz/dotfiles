package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNavigateMsg(t *testing.T) {
	// Test NavigateTo
	cmd := NavigateTo(ScreenMainMenu)
	if cmd == nil {
		t.Fatal("NavigateTo should return a command")
	}

	msg := cmd()
	navMsg, ok := msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if navMsg.To != ScreenMainMenu {
		t.Errorf("To = %v, want %v", navMsg.To, ScreenMainMenu)
	}
	if navMsg.PushBack {
		t.Error("PushBack should be false")
	}

	// Test NavigatePush
	cmd = NavigatePush(ScreenManage)
	msg = cmd()
	navMsg, ok = msg.(NavigateMsg)
	if !ok {
		t.Fatalf("expected NavigateMsg, got %T", msg)
	}
	if navMsg.To != ScreenManage {
		t.Errorf("To = %v, want %v", navMsg.To, ScreenManage)
	}
	if !navMsg.PushBack {
		t.Error("PushBack should be true")
	}
}

func TestNavigateBackMsg(t *testing.T) {
	cmd := NavigateBack()
	if cmd == nil {
		t.Fatal("NavigateBack should return a command")
	}

	msg := cmd()
	_, ok := msg.(NavigateBackMsg)
	if !ok {
		t.Fatalf("expected NavigateBackMsg, got %T", msg)
	}
}

func TestBaseScreen(t *testing.T) {
	ctx := &ScreenContext{
		Theme:             "dracula",
		NavStyle:          "vim",
		AnimationsEnabled: true,
		Width:             120,
		Height:            40,
		UIFrame:           5,
	}

	screen := &BaseScreen{}

	// Test with nil context
	if screen.Width() != 80 {
		t.Errorf("Width() = %d, want 80 (default)", screen.Width())
	}
	if screen.Height() != 24 {
		t.Errorf("Height() = %d, want 24 (default)", screen.Height())
	}
	if screen.Theme() != "catppuccin-mocha" {
		t.Errorf("Theme() = %q, want default", screen.Theme())
	}
	if screen.NavStyle() != "emacs" {
		t.Errorf("NavStyle() = %q, want default", screen.NavStyle())
	}
	if !screen.AnimationsEnabled() {
		t.Error("AnimationsEnabled() should default to true")
	}
	if screen.UIFrame() != 0 {
		t.Errorf("UIFrame() = %d, want 0 (default)", screen.UIFrame())
	}

	// Set context
	screen.SetContext(ctx)

	// Test with context
	if screen.Context() != ctx {
		t.Error("Context() should return set context")
	}
	if screen.Width() != 120 {
		t.Errorf("Width() = %d, want 120", screen.Width())
	}
	if screen.Height() != 40 {
		t.Errorf("Height() = %d, want 40", screen.Height())
	}
	if screen.Theme() != "dracula" {
		t.Errorf("Theme() = %q, want %q", screen.Theme(), "dracula")
	}
	if screen.NavStyle() != "vim" {
		t.Errorf("NavStyle() = %q, want %q", screen.NavStyle(), "vim")
	}
	if !screen.AnimationsEnabled() {
		t.Error("AnimationsEnabled() should be true")
	}
	if screen.UIFrame() != 5 {
		t.Errorf("UIFrame() = %d, want 5", screen.UIFrame())
	}
}

func TestScreenContext(t *testing.T) {
	deps := NewTestDependencies()
	ctx := NewScreenContext(deps)

	if ctx.Deps != deps {
		t.Error("Deps should match")
	}
	if ctx.Theme != "catppuccin-mocha" {
		t.Errorf("Theme = %q, want default", ctx.Theme)
	}
	if ctx.NavStyle != "emacs" {
		t.Errorf("NavStyle = %q, want default", ctx.NavStyle)
	}
	if !ctx.AnimationsEnabled {
		t.Error("AnimationsEnabled should be true by default")
	}
	if ctx.Width != 80 {
		t.Errorf("Width = %d, want 80", ctx.Width)
	}
	if ctx.Height != 24 {
		t.Errorf("Height = %d, want 24", ctx.Height)
	}
}

// mockScreenHandler is a minimal ScreenHandler for testing
type mockScreenHandler struct {
	BaseScreen
	screenID     Screen
	initCalled   bool
	updateCalled bool
	viewCalled   bool
	lastMsg      tea.Msg
}

func (s *mockScreenHandler) ID() Screen {
	return s.screenID
}

func (s *mockScreenHandler) Init() tea.Cmd {
	s.initCalled = true
	return nil
}

func (s *mockScreenHandler) Update(msg tea.Msg) (ScreenHandler, tea.Cmd) {
	s.updateCalled = true
	s.lastMsg = msg
	return s, nil
}

func (s *mockScreenHandler) View(width, height int) string {
	s.viewCalled = true
	return "mock view"
}
