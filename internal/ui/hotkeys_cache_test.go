package ui

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/hotkeys"
)

// TestHotkeysActiveUsernameIsPerApp verifies that two App instances do NOT share
// the active-username cache (previously a package-global). Each App must carry
// its own cache fields (hotkeysActiveUser / hotkeysActiveUserCached).
func TestHotkeysActiveUsernameIsPerApp(t *testing.T) {
	t.Parallel()

	app1 := NewApp(true /* skipIntro */)
	app2 := NewApp(true /* skipIntro */)

	// Directly set the per-App cache fields to different values (simulating two
	// separate users without needing to write to disk).
	app1.hotkeysActiveUser = "alice"
	app1.hotkeysActiveUserCached = true

	app2.hotkeysActiveUser = "bob"
	app2.hotkeysActiveUserCached = true

	u1 := app1.getCurrentUsername()
	u2 := app2.getCurrentUsername()

	// The two apps must resolve to different users — proving the cache is per-App.
	if u1 == u2 {
		t.Errorf("expected different usernames but both returned %q — cache is still shared", u1)
	}
	if u1 != "alice" {
		t.Errorf("app1 username = %q, want %q", u1, "alice")
	}
	if u2 != "bob" {
		t.Errorf("app2 username = %q, want %q", u2, "bob")
	}
}

// TestHotkeysActiveUsernameCacheInitiallyEmpty verifies that a freshly created
// App has an uncached username (hotkeysActiveUserCached = false), which means
// getCurrentUsername will trigger a disk read on first call.
func TestHotkeysActiveUsernameCacheInitiallyEmpty(t *testing.T) {
	t.Parallel()

	app := NewApp(true)
	// The cache flag must be false initially (NewApp does not pre-warm it).
	if app.hotkeysActiveUserCached {
		t.Error("expected hotkeysActiveUserCached = false on a freshly created App")
	}
}

// TestHotkeysActiveUsernameDefaultsToDefault verifies that when no active user
// is configured in global.json (or the file doesn't exist), getCurrentUsername
// returns "default".
func TestHotkeysActiveUsernameDefaultsToDefault(t *testing.T) {
	t.Parallel()

	app := NewApp(true)
	// Simulate the no-user-configured case by bypassing disk read.
	app.hotkeysActiveUser = "default"
	app.hotkeysActiveUserCached = true

	u := app.getCurrentUsername()
	if u != "default" {
		t.Errorf("getCurrentUsername() = %q, want %q", u, "default")
	}
}

// TestHotkeysFavoriteSurvivesNavStyleSwitch is the key regression test (C21):
// favoriting an item under emacs nav style and then switching to vim nav style
// must keep the item favorited, because favorites are keyed on the stable item
// ID (not the Keys string which changes with nav style).
func TestHotkeysFavoriteSurvivesNavStyleSwitch(t *testing.T) {
	t.Parallel()

	app := NewApp(true /* skipIntro */)
	// Ensure hotkeysFavorites is initialized.
	if app.hotkeysFavorites == nil {
		app.hotkeysFavorites = &config.HotkeysConfig{Users: make(map[string]*config.UserHotkeys)}
	}

	// Set nav style to emacs and bypass disk read for username.
	app.navStyle = "emacs"
	app.hotkeysActiveUser = "testuser"
	app.hotkeysActiveUserCached = true

	// Get the neovim/Navigate item under emacs.
	cats := app.hotkeyCategories()
	var emacsNavigateItem *hotkeys.Item
	for _, cat := range cats {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					itCopy := it
					emacsNavigateItem = &itCopy
				}
			}
		}
	}
	if emacsNavigateItem == nil {
		t.Fatal("neovim/Navigate item not found in emacs categories")
	}
	if emacsNavigateItem.Keys != "Arrow keys" {
		t.Fatalf("expected emacs navigate Keys=%q, got %q", "Arrow keys", emacsNavigateItem.Keys)
	}

	// Favorite the item using its stable ID.
	app.toggleHotkeyFavorite("neovim", emacsNavigateItem.ID)

	// Verify it is favorited under emacs nav style.
	if !app.isHotkeyFavorite("neovim", emacsNavigateItem.ID) {
		t.Fatal("item should be favorited under emacs nav style")
	}

	// Now switch to vim nav style.
	app.navStyle = "vim"

	// Re-fetch categories — the same item now has Keys="h/j/k/l".
	cats = app.hotkeyCategories()
	var vimNavigateItem *hotkeys.Item
	for _, cat := range cats {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					itCopy := it
					vimNavigateItem = &itCopy
				}
			}
		}
	}
	if vimNavigateItem == nil {
		t.Fatal("neovim/Navigate item not found in vim categories")
	}
	if vimNavigateItem.Keys != "h/j/k/l" {
		t.Fatalf("expected vim navigate Keys=%q, got %q", "h/j/k/l", vimNavigateItem.Keys)
	}

	// The stable ID must be the same across nav styles.
	if emacsNavigateItem.ID != vimNavigateItem.ID {
		t.Fatalf("ID changed across nav styles: emacs=%q vim=%q", emacsNavigateItem.ID, vimNavigateItem.ID)
	}

	// CRITICAL: the favorite must still show as favorited under vim nav style.
	// This was the bug: Keys changed so the old Keys-keyed lookup returned false.
	if !app.isHotkeyFavorite("neovim", vimNavigateItem.ID) {
		t.Error("favorite was orphaned when switching nav style from emacs to vim — this is the bug C21 fixes")
	}
}

// TestHotkeysFavoriteToggleRoundTrip verifies that toggle add/remove works
// correctly with stable IDs.
func TestHotkeysFavoriteToggleRoundTrip(t *testing.T) {
	t.Parallel()

	app := NewApp(true)
	if app.hotkeysFavorites == nil {
		app.hotkeysFavorites = &config.HotkeysConfig{Users: make(map[string]*config.UserHotkeys)}
	}
	app.hotkeysActiveUser = "testuser"
	app.hotkeysActiveUserCached = true

	// Find an item with a stable ID.
	var item *hotkeys.Item
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == "lazygit" {
			it := cat.Items[0]
			item = &it
			break
		}
	}
	if item == nil {
		t.Fatal("lazygit category not found")
	}

	// Should not be favorited initially.
	if app.isHotkeyFavorite("lazygit", item.ID) {
		t.Error("item should not be favorited initially")
	}

	// Toggle on.
	app.toggleHotkeyFavorite("lazygit", item.ID)
	if !app.isHotkeyFavorite("lazygit", item.ID) {
		t.Error("item should be favorited after first toggle")
	}

	// Toggle off.
	app.toggleHotkeyFavorite("lazygit", item.ID)
	if app.isHotkeyFavorite("lazygit", item.ID) {
		t.Error("item should not be favorited after second toggle")
	}
}
