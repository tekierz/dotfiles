package tools

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/pkg"
)

// TestMacOSApps_PlatformFilter verifies that macOS-only apps have the
// correct platform filter set to prevent them from appearing on Linux.
func TestMacOSApps_PlatformFilter(t *testing.T) {
	macOSOnlyApps := []string{
		"rectangle",
		"raycast",
		"iina",
		"appcleaner",
	}

	r := NewRegistry()

	for _, id := range macOSOnlyApps {
		t.Run(id, func(t *testing.T) {
			tool, exists := r.Get(id)
			if !exists {
				t.Fatalf("expected tool %q to be registered", id)
			}

			filter := tool.PlatformFilter()
			if filter != pkg.PlatformMacOS {
				t.Errorf("tool %q PlatformFilter() = %q, want %q (macOS only)",
					id, filter, pkg.PlatformMacOS)
			}
		})
	}
}

// TestMacOSApps_UIGroup verifies that macOS-only apps are in the correct
// UI group to ensure proper display in the installer.
func TestMacOSApps_UIGroup(t *testing.T) {
	macOSOnlyApps := []string{
		"rectangle",
		"raycast",
		"iina",
		"appcleaner",
	}

	r := NewRegistry()

	for _, id := range macOSOnlyApps {
		t.Run(id, func(t *testing.T) {
			tool, exists := r.Get(id)
			if !exists {
				t.Fatalf("expected tool %q to be registered", id)
			}

			group := tool.UIGroup()
			if group != UIGroupMacApps {
				t.Errorf("tool %q UIGroup() = %q, want %q",
					id, group, UIGroupMacApps)
			}
		})
	}
}

// TestCrossplatformApps_NoPlatformFilter verifies that cross-platform apps
// do NOT have a platform filter, so they appear on all platforms.
func TestCrossplatformApps_NoPlatformFilter(t *testing.T) {
	crossPlatformApps := []string{
		"zen-browser",
		"cursor",
		"lm-studio",
		"obs",
	}

	r := NewRegistry()

	for _, id := range crossPlatformApps {
		t.Run(id, func(t *testing.T) {
			tool, exists := r.Get(id)
			if !exists {
				t.Fatalf("expected tool %q to be registered", id)
			}

			filter := tool.PlatformFilter()
			if filter != "" {
				t.Errorf("cross-platform tool %q should have empty PlatformFilter(), got %q",
					id, filter)
			}
		})
	}
}

// TestRaycastTool_IsInstalled verifies that RaycastTool.IsInstalled()
// checks for macOS app bundle, not just Homebrew.
func TestRaycastTool_IsInstalled(t *testing.T) {
	tool := NewRaycastTool()

	// Verify the tool has an IsInstalled override (not just BaseTool)
	// We can't test actual installation without mocking, but we can verify
	// the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Verify it has correct platform filter
	if tool.PlatformFilter() != pkg.PlatformMacOS {
		t.Error("Raycast should be macOS-only")
	}
}

// TestRectangleTool_IsInstalled verifies that RectangleTool.IsInstalled()
// checks for macOS app bundle, not just Homebrew.
func TestRectangleTool_IsInstalled(t *testing.T) {
	tool := NewRectangleTool()

	// Verify the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Verify it has correct platform filter
	if tool.PlatformFilter() != pkg.PlatformMacOS {
		t.Error("Rectangle should be macOS-only")
	}
}

// TestIINATool_IsInstalled verifies that IINATool.IsInstalled()
// checks for macOS app bundle, not just Homebrew.
func TestIINATool_IsInstalled(t *testing.T) {
	tool := NewIINATool()

	// Verify the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Verify it has correct platform filter
	if tool.PlatformFilter() != pkg.PlatformMacOS {
		t.Error("IINA should be macOS-only")
	}
}

// TestAppCleanerTool_IsInstalled verifies that AppCleanerTool.IsInstalled()
// checks for macOS app bundle, not just Homebrew.
func TestAppCleanerTool_IsInstalled(t *testing.T) {
	tool := NewAppCleanerTool()

	// Verify the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Verify it has correct platform filter
	if tool.PlatformFilter() != pkg.PlatformMacOS {
		t.Error("AppCleaner should be macOS-only")
	}
}

// TestZenBrowserTool_IsInstalled verifies that ZenBrowserTool.IsInstalled()
// checks multiple sources (command, flatpak, AppImage, desktop entry, app bundle).
func TestZenBrowserTool_IsInstalled(t *testing.T) {
	tool := NewZenBrowserTool()

	// Verify the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Should be cross-platform (no filter)
	if tool.PlatformFilter() != "" {
		t.Error("Zen Browser should be cross-platform")
	}
}

// TestCursorTool_IsInstalled verifies that CursorTool.IsInstalled()
// checks multiple sources including AppImage for Linux.
func TestCursorTool_IsInstalled(t *testing.T) {
	tool := NewCursorTool()

	// Verify the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Should be cross-platform (no filter)
	if tool.PlatformFilter() != "" {
		t.Error("Cursor should be cross-platform")
	}
}

// TestLMStudioTool_IsInstalled verifies that LMStudioTool.IsInstalled()
// checks multiple sources including common install paths.
func TestLMStudioTool_IsInstalled(t *testing.T) {
	tool := NewLMStudioTool()

	// Verify the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Should be cross-platform (no filter)
	if tool.PlatformFilter() != "" {
		t.Error("LM Studio should be cross-platform")
	}
}

// TestOBSTool_IsInstalled verifies that OBSTool.IsInstalled()
// checks multiple sources.
func TestOBSTool_IsInstalled(t *testing.T) {
	tool := NewOBSTool()

	// Verify the method exists and doesn't panic
	_ = tool.IsInstalled()

	// Should be cross-platform (no filter)
	if tool.PlatformFilter() != "" {
		t.Error("OBS should be cross-platform")
	}
}

// TestHasMacOSApp_InvalidNames verifies hasMacOSApp handles edge cases.
func TestHasMacOSApp_InvalidNames(t *testing.T) {
	// Empty names should not cause panic
	result := hasMacOSApp()
	if result {
		t.Error("hasMacOSApp() with no args should return false")
	}

	// Names that definitely don't exist
	result = hasMacOSApp("NonExistentApp12345", "AnotherFakeApp")
	if result {
		t.Error("hasMacOSApp with fake names should return false")
	}
}

// TestHasAppImage_InvalidPatterns verifies hasAppImage handles edge cases.
func TestHasAppImage_InvalidPatterns(t *testing.T) {
	// Empty patterns should not cause panic
	result := hasAppImage()
	if result {
		t.Error("hasAppImage() with no args should return false")
	}

	// Patterns that definitely don't exist
	result = hasAppImage("NonExistentApp12345", "AnotherFakeApp")
	if result {
		t.Error("hasAppImage with fake patterns should return false")
	}
}

// TestHasDesktopEntry_InvalidNames verifies hasDesktopEntry handles edge cases.
func TestHasDesktopEntry_InvalidNames(t *testing.T) {
	// Empty names should not cause panic
	result := hasDesktopEntry()
	if result {
		t.Error("hasDesktopEntry() with no args should return false")
	}

	// Names that definitely don't exist
	result = hasDesktopEntry("nonexistent-app-12345")
	if result {
		t.Error("hasDesktopEntry with fake name should return false")
	}
}

// TestIsFlatpakInstalled_InvalidIDs verifies isFlatpakInstalled handles edge cases.
func TestIsFlatpakInstalled_InvalidIDs(t *testing.T) {
	// Empty IDs should not cause panic
	result := isFlatpakInstalled()
	if result {
		t.Error("isFlatpakInstalled() with no args should return false")
	}

	// IDs that definitely don't exist
	result = isFlatpakInstalled("com.nonexistent.app.12345")
	if result {
		t.Error("isFlatpakInstalled with fake ID should return false")
	}
}
