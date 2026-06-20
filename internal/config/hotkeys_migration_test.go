package config

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/hotkeys"
)

// TestMigrateLegacyFavorites_EmacsKey verifies that an emacs-style Keys string
// stored in the old format is correctly migrated to the stable item ID.
func TestMigrateLegacyFavorites_EmacsKey(t *testing.T) {
	t.Parallel()

	// Simulate a favorites map in the OLD format: keyed on Keys string (emacs style).
	// Under emacs, neovim "Navigate" has Keys="Arrow keys".
	userHotkeys := &UserHotkeys{
		Favorites: map[string][]string{
			"neovim": {"Arrow keys"}, // old-format emacs key
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	// After migration, the favorites for "neovim" should contain the stable ID,
	// not the raw Keys string.
	favs := userHotkeys.Favorites["neovim"]
	if len(favs) == 0 {
		t.Fatal("favorites for 'neovim' are empty after migration")
	}

	// Find the stable ID for neovim/Navigate
	var wantID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					wantID = it.ID
				}
			}
		}
	}
	if wantID == "" {
		t.Fatal("could not determine stable ID for neovim/Navigate")
	}

	found := false
	for _, fav := range favs {
		if fav == wantID {
			found = true
		}
	}
	if !found {
		t.Fatalf("stable ID %q not found in favorites after migration; got %v", wantID, favs)
	}
}

// TestMigrateLegacyFavorites_VimKey verifies that a vim-style Keys string stored
// in the old format is also correctly migrated to the stable item ID.
func TestMigrateLegacyFavorites_VimKey(t *testing.T) {
	t.Parallel()

	// Under vim, neovim "Navigate" has Keys="h/j/k/l".
	userHotkeys := &UserHotkeys{
		Favorites: map[string][]string{
			"neovim": {"h/j/k/l"}, // old-format vim key
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites["neovim"]
	if len(favs) == 0 {
		t.Fatal("favorites for 'neovim' are empty after migration")
	}

	// The stable ID must be the same regardless of which nav style the old key came from.
	var emacsID, vimID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					emacsID = it.ID
				}
			}
		}
	}
	for _, cat := range hotkeys.Categories("vim") {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					vimID = it.ID
				}
			}
		}
	}
	if emacsID != vimID {
		t.Fatalf("emacs ID %q != vim ID %q — IDs are not stable", emacsID, vimID)
	}

	found := false
	for _, fav := range favs {
		if fav == emacsID {
			found = true
		}
	}
	if !found {
		t.Fatalf("stable ID %q not found in favorites after migration; got %v", emacsID, favs)
	}
}

// TestMigrateLegacyFavorites_NoLoss verifies that both emacs and vim legacy keys
// for the SAME logical action are de-duped to the same stable ID (no double entry).
func TestMigrateLegacyFavorites_NoLoss(t *testing.T) {
	t.Parallel()

	// An emacs key and a vim key for the same item were saved (shouldn't happen, but
	// migration must be idempotent and not create duplicates).
	userHotkeys := &UserHotkeys{
		Favorites: map[string][]string{
			"neovim": {"Arrow keys", "h/j/k/l"}, // both styles of the same item
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites["neovim"]
	// Should collapse to exactly ONE entry (the stable ID).
	var wantID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					wantID = it.ID
				}
			}
		}
	}

	count := 0
	for _, fav := range favs {
		if fav == wantID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 entry for stable ID %q after migration, got %d; favs=%v", wantID, count, favs)
	}
}

// TestMigrateLegacyFavorites_AlreadyMigrated verifies that a favorites map that
// already has stable IDs is not corrupted by calling MigrateLegacyFavorites again.
func TestMigrateLegacyFavorites_AlreadyMigrated(t *testing.T) {
	t.Parallel()

	var wantID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					wantID = it.ID
				}
			}
		}
	}
	if wantID == "" {
		t.Fatal("could not determine stable ID")
	}

	// Already in the new format.
	userHotkeys := &UserHotkeys{
		Favorites: map[string][]string{
			"neovim": {wantID},
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites["neovim"]
	if len(favs) != 1 || favs[0] != wantID {
		t.Fatalf("after idempotent migration expected [%q], got %v", wantID, favs)
	}
}

// TestMigrateLegacyFavorites_UnrecognizedEntryPreserved verifies that an entry
// that cannot be matched to any known item OR stable ID is left in place (not
// silently dropped).
func TestMigrateLegacyFavorites_UnrecognizedEntryPreserved(t *testing.T) {
	t.Parallel()

	userHotkeys := &UserHotkeys{
		Favorites: map[string][]string{
			"neovim": {"some-completely-unknown-key"},
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites["neovim"]
	if len(favs) == 0 {
		t.Fatal("unrecognized entry was silently dropped — must be preserved")
	}
	found := false
	for _, fav := range favs {
		if fav == "some-completely-unknown-key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unrecognized entry 'some-completely-unknown-key' not preserved; got %v", favs)
	}
}

// TestIsFavoriteUsesStableID verifies that IsFavorite checks against item ID not Keys.
func TestIsFavoriteUsesStableID(t *testing.T) {
	t.Parallel()

	var navigateID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == "neovim" {
			for _, it := range cat.Items {
				if it.Description == "Navigate" {
					navigateID = it.ID
				}
			}
		}
	}
	if navigateID == "" {
		t.Fatal("could not find navigate ID")
	}

	userHotkeys := &UserHotkeys{
		Favorites: map[string][]string{
			"neovim": {navigateID},
		},
	}

	// IsFavorite must return true when given the stable ID.
	if !userHotkeys.IsFavorite("neovim", navigateID) {
		t.Errorf("IsFavorite(%q, %q) = false, want true", "neovim", navigateID)
	}

	// IsFavorite must return false for the old Keys string (no longer the key).
	if userHotkeys.IsFavorite("neovim", "Arrow keys") {
		t.Errorf("IsFavorite(%q, %q) = true, want false — should not match old Keys string", "neovim", "Arrow keys")
	}
}
