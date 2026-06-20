package hotkeys

import (
	"fmt"
	"testing"
)

// TestItemIDStableAcrossNavStyle is the TDD RED test: favoring an item under emacs
// and then switching to vim must still find the same item (same ID), because the
// ID is navStyle-independent.
//
// This test WILL FAIL until Item.ID is added and populated in Categories().
func TestItemIDStableAcrossNavStyle(t *testing.T) {
	t.Parallel()

	emacsItems := Categories("emacs")
	vimItems := Categories("vim")

	// Find the neovim "Navigate" item under emacs.
	var emacsNav, vimNav *Item
	for i := range emacsItems {
		if emacsItems[i].ID == "neovim" {
			for j := range emacsItems[i].Items {
				if emacsItems[i].Items[j].Description == "Navigate" {
					it := emacsItems[i].Items[j]
					emacsNav = &it
				}
			}
		}
	}
	for i := range vimItems {
		if vimItems[i].ID == "neovim" {
			for j := range vimItems[i].Items {
				if vimItems[i].Items[j].Description == "Navigate" {
					it := vimItems[i].Items[j]
					vimNav = &it
				}
			}
		}
	}
	if emacsNav == nil {
		t.Fatal("neovim/Navigate item not found in emacs categories")
	}
	if vimNav == nil {
		t.Fatal("neovim/Navigate item not found in vim categories")
	}

	// The Keys MUST differ (that is the whole problem being fixed).
	if emacsNav.Keys == vimNav.Keys {
		t.Fatalf("expected neovim/Navigate Keys to differ between styles, both = %q", emacsNav.Keys)
	}

	// The ID MUST be the same — this is the stable handle for favorites.
	if emacsNav.ID == "" {
		t.Fatal("neovim/Navigate item has empty ID in emacs nav style")
	}
	if vimNav.ID == "" {
		t.Fatal("neovim/Navigate item has empty ID in vim nav style")
	}
	if emacsNav.ID != vimNav.ID {
		t.Fatalf("neovim/Navigate ID differs between nav styles: emacs=%q vim=%q", emacsNav.ID, vimNav.ID)
	}
}

// TestAllItemIDsUniqueAndNonEmpty asserts every item emitted by Categories has a
// unique, non-empty ID across both nav styles.
func TestAllItemIDsUniqueAndNonEmpty(t *testing.T) {
	t.Parallel()

	for _, ns := range []string{"vim", "emacs"} {
		ns := ns
		t.Run(ns, func(t *testing.T) {
			t.Parallel()
			cats := Categories(ns)
			seen := map[string]bool{}
			for _, cat := range cats {
				for _, it := range cat.Items {
					if it.ID == "" {
						t.Errorf("[%s] category %q item %q has empty ID", ns, cat.ID, it.Description)
						continue
					}
					if seen[it.ID] {
						t.Errorf("[%s] duplicate item ID %q (category %q, desc %q)", ns, it.ID, cat.ID, it.Description)
					}
					seen[it.ID] = true
				}
			}
		})
	}
}

// TestItemIDScheme validates that item IDs follow the "<categoryID>.<slug>" format.
func TestItemIDScheme(t *testing.T) {
	t.Parallel()

	cats := Categories("emacs")
	for _, cat := range cats {
		prefix := cat.ID + "."
		for _, it := range cat.Items {
			if len(it.ID) <= len(prefix) {
				t.Errorf("category %q item %q: ID %q doesn't have expected prefix %q", cat.ID, it.Description, it.ID, prefix)
				continue
			}
			if it.ID[:len(prefix)] != prefix {
				t.Errorf("category %q item %q: ID %q doesn't start with %q", cat.ID, it.Description, it.ID, prefix)
			}
		}
	}
}

// TestIDsStableAcrossNavStyleForAllFlippingItems tests all items that have
// navStyle-dependent Keys to ensure their IDs remain the same.
func TestIDsStableAcrossNavStyleForAllFlippingItems(t *testing.T) {
	t.Parallel()

	emacsItems := Categories("emacs")
	vimItems := Categories("vim")

	// Build maps: catID -> desc -> item for each nav style
	emacsMap := map[string]map[string]Item{}
	vimMap := map[string]map[string]Item{}
	for _, cat := range emacsItems {
		emacsMap[cat.ID] = map[string]Item{}
		for _, it := range cat.Items {
			emacsMap[cat.ID][it.Description] = it
		}
	}
	for _, cat := range vimItems {
		vimMap[cat.ID] = map[string]Item{}
		for _, it := range cat.Items {
			vimMap[cat.ID][it.Description] = it
		}
	}

	// For any item that exists in both nav styles with same description, the ID must be identical.
	for catID, emacsDescMap := range emacsMap {
		vimDescMap, ok := vimMap[catID]
		if !ok {
			continue
		}
		for desc, emacsItem := range emacsDescMap {
			vimItem, ok := vimDescMap[desc]
			if !ok {
				continue
			}
			if emacsItem.ID != vimItem.ID {
				t.Errorf("category %q description %q: ID differs between nav styles (emacs=%q vim=%q)",
					catID, desc, emacsItem.ID, vimItem.ID)
			}
		}
	}
}

// TestCategoriesItemCountConsistentAcrossNavStyles verifies that the number of
// items in each category doesn't change between nav styles (only Keys/ID change).
func TestCategoriesItemCountConsistentAcrossNavStyles(t *testing.T) {
	t.Parallel()

	emacsItems := Categories("emacs")
	vimItems := Categories("vim")

	if len(emacsItems) != len(vimItems) {
		t.Fatalf("category count differs: emacs=%d vim=%d", len(emacsItems), len(vimItems))
	}
	for i := range emacsItems {
		e := emacsItems[i]
		v := vimItems[i]
		if e.ID != v.ID {
			t.Errorf("category[%d] ID mismatch: emacs=%q vim=%q", i, e.ID, v.ID)
		}
		if len(e.Items) != len(v.Items) {
			t.Errorf("category %q item count: emacs=%d vim=%d", e.ID, len(e.Items), len(v.Items))
		}
	}
}

// TestItemIDFormat verifies no special characters that could break JSON keys.
func TestItemIDFormat(t *testing.T) {
	t.Parallel()

	cats := Categories("emacs")
	for _, cat := range cats {
		for _, it := range cat.Items {
			for _, ch := range it.ID {
				if ch == '"' || ch == '\\' || ch == '\n' || ch == '\r' || ch == '\t' {
					t.Errorf("item ID %q contains invalid character %q", it.ID, fmt.Sprintf("%c", ch))
				}
			}
		}
	}
}
