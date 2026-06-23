package config

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/hotkeys"
)

// Test fixtures: the neovim category ID and the "Navigate" item description
// exercised throughout the favorites-migration tests.
const (
	catIDNeovim  = "neovim"
	itemNavigate = "Navigate"
)

// TestMigrateLegacyFavorites_EmacsKey verifies that an emacs-style Keys string
// stored in the old format is correctly migrated to the stable item ID.
func TestMigrateLegacyFavorites_EmacsKey(t *testing.T) {
	t.Parallel()

	// Simulate a favorites map in the OLD format: keyed on Keys string (emacs style).
	// Under emacs, neovim "Navigate" has Keys="Arrow keys".
	userHotkeys := &UserHotkeys{
		Favorites: map[string][]string{
			catIDNeovim: {"Arrow keys"}, // old-format emacs key
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	// After migration, the favorites for catIDNeovim should contain the stable ID,
	// not the raw Keys string.
	favs := userHotkeys.Favorites[catIDNeovim]
	if len(favs) == 0 {
		t.Fatal("favorites for 'neovim' are empty after migration")
	}

	// Find the stable ID for neovim/Navigate
	var wantID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == catIDNeovim {
			for _, it := range cat.Items {
				if it.Description == itemNavigate {
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
			catIDNeovim: {"h/j/k/l"}, // old-format vim key
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites[catIDNeovim]
	if len(favs) == 0 {
		t.Fatal("favorites for 'neovim' are empty after migration")
	}

	// The stable ID must be the same regardless of which nav style the old key came from.
	var emacsID, vimID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == catIDNeovim {
			for _, it := range cat.Items {
				if it.Description == itemNavigate {
					emacsID = it.ID
				}
			}
		}
	}
	for _, cat := range hotkeys.Categories(navStyleVim) {
		if cat.ID == catIDNeovim {
			for _, it := range cat.Items {
				if it.Description == itemNavigate {
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
			catIDNeovim: {"Arrow keys", "h/j/k/l"}, // both styles of the same item
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites[catIDNeovim]
	// Should collapse to exactly ONE entry (the stable ID).
	var wantID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == catIDNeovim {
			for _, it := range cat.Items {
				if it.Description == itemNavigate {
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
		if cat.ID == catIDNeovim {
			for _, it := range cat.Items {
				if it.Description == itemNavigate {
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
			catIDNeovim: {wantID},
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites[catIDNeovim]
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
			catIDNeovim: {"some-completely-unknown-key"},
		},
	}

	MigrateLegacyFavorites(userHotkeys)

	favs := userHotkeys.Favorites[catIDNeovim]
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

// TestMigrateLegacyFavorites_ReportsChangeOnlyWhenRewritten verifies the changed
// bool: a legacy-key migration reports true, while an already-migrated (or empty)
// map reports false so callers can skip an unnecessary disk write on every launch.
func TestMigrateLegacyFavorites_ReportsChangeOnlyWhenRewritten(t *testing.T) {
	t.Parallel()

	var navigateID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == catIDNeovim {
			for _, it := range cat.Items {
				if it.Description == itemNavigate {
					navigateID = it.ID
				}
			}
		}
	}
	if navigateID == "" {
		t.Fatal("could not determine stable ID")
	}

	// 1) Legacy key -> must report a change.
	legacy := &UserHotkeys{Favorites: map[string][]string{catIDNeovim: {"Arrow keys"}}}
	if !MigrateLegacyFavorites(legacy) {
		t.Error("MigrateLegacyFavorites on legacy-keyed favorites = false, want true")
	}

	// 2) Already migrated -> must report no change.
	already := &UserHotkeys{Favorites: map[string][]string{catIDNeovim: {navigateID}}}
	if MigrateLegacyFavorites(already) {
		t.Error("MigrateLegacyFavorites on already-migrated favorites = true, want false")
	}

	// 3) Empty favorites -> no change.
	empty := &UserHotkeys{Favorites: map[string][]string{}}
	if MigrateLegacyFavorites(empty) {
		t.Error("MigrateLegacyFavorites on empty favorites = true, want false")
	}

	// 4) nil receiver -> no change.
	if MigrateLegacyFavorites(nil) {
		t.Error("MigrateLegacyFavorites(nil) = true, want false")
	}

	// 5) De-dup collapse -> a change (two entries become one).
	dup := &UserHotkeys{Favorites: map[string][]string{catIDNeovim: {"Arrow keys", "h/j/k/l"}}}
	if !MigrateLegacyFavorites(dup) {
		t.Error("MigrateLegacyFavorites on duplicate-collapsing favorites = false, want true")
	}
}

// TestIsFavoriteUsesStableID verifies that IsFavorite checks against item ID not Keys.
func TestIsFavoriteUsesStableID(t *testing.T) {
	t.Parallel()

	var navigateID string
	for _, cat := range hotkeys.Categories("emacs") {
		if cat.ID == catIDNeovim {
			for _, it := range cat.Items {
				if it.Description == itemNavigate {
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
			catIDNeovim: {navigateID},
		},
	}

	// IsFavorite must return true when given the stable ID.
	if !userHotkeys.IsFavorite(catIDNeovim, navigateID) {
		t.Errorf("IsFavorite(%q, %q) = false, want true", catIDNeovim, navigateID)
	}

	// IsFavorite must return false for the old Keys string (no longer the key).
	if userHotkeys.IsFavorite(catIDNeovim, "Arrow keys") {
		t.Errorf("IsFavorite(%q, %q) = true, want false — should not match old Keys string", catIDNeovim, "Arrow keys")
	}
}
