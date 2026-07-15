package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/hotkeys"
)

// HotkeysConfig stores per-user hotkey customizations
type HotkeysConfig struct {
	// Keyed by user name from global config's ActiveUser
	Users map[string]*UserHotkeys `json:"users"`
}

// UserHotkeys stores a user's hotkey customizations
type UserHotkeys struct {
	Favorites map[string][]string `json:"favorites"` // category_id -> []item_keys
	Aliases   map[string]string   `json:"aliases"`   // alias -> actual_command
}

// LoadHotkeysConfig loads hotkeys config from ConfigDir()/hotkeys.json.
// Earlier releases wrote hotkeys.json to a literal ~/.config/dotfiles path
// even when XDG_CONFIG_HOME pointed elsewhere; if the file is missing at the
// ConfigDir() location, fall back to that legacy path so saved favorites and
// aliases survive the upgrade (the next save writes the new location).
func LoadHotkeysConfig() (*HotkeysConfig, error) {
	dir := ConfigDir()
	if dir == "" {
		return nil, ErrNoConfigDir
	}
	path := filepath.Join(dir, "hotkeys.json")

	data, revision, err := readProductConfigJSON(path)
	if err != nil {
		return nil, err
	}
	if !revision.Exists() {
		if legacy := legacyHotkeysPath(); legacy != "" && legacy != path {
			legacyData, legacyRevision, legacyErr := readProductConfigJSON(legacy)
			if legacyErr != nil {
				return nil, legacyErr
			}
			if legacyRevision.Exists() {
				data, revision = legacyData, legacyRevision
			}
		}
	}
	if !revision.Exists() {
		return &HotkeysConfig{Users: make(map[string]*UserHotkeys)}, nil
	}

	var cfg HotkeysConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Users == nil {
		cfg.Users = make(map[string]*UserHotkeys)
	}
	return &cfg, nil
}

// legacyHotkeysPath returns the pre-XDG location hotkeys.json was written to,
// or "" when the home directory cannot be determined.
func legacyHotkeysPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "dotfiles", "hotkeys.json")
}

// SaveHotkeysConfig saves hotkeys config
func SaveHotkeysConfig(cfg *HotkeysConfig) error {
	dir := ConfigDir()
	if dir == "" {
		return ErrNoConfigDir
	}
	path := filepath.Join(dir, "hotkeys.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// GetUserHotkeys gets or creates hotkeys for a specific user.
// Returns a pointer to the actual map entry, so modifications persist.
func (c *HotkeysConfig) GetUserHotkeys(username string) *UserHotkeys {
	if c.Users == nil {
		c.Users = make(map[string]*UserHotkeys)
	}
	h, ok := c.Users[username]
	if !ok || h == nil {
		h = &UserHotkeys{
			Favorites: make(map[string][]string),
			Aliases:   make(map[string]string),
		}
		c.Users[username] = h
	}
	return h
}

// SetUserHotkeys updates the hotkeys for a specific user
func (c *HotkeysConfig) SetUserHotkeys(username string, h *UserHotkeys) {
	if c.Users == nil {
		c.Users = make(map[string]*UserHotkeys)
	}
	c.Users[username] = h
}

// IsFavorite checks if a hotkey item is a favorite for the user
func (u *UserHotkeys) IsFavorite(categoryID, itemKey string) bool {
	if u == nil || u.Favorites == nil {
		return false
	}
	items, ok := u.Favorites[categoryID]
	if !ok {
		return false
	}
	for _, k := range items {
		if k == itemKey {
			return true
		}
	}
	return false
}

// ToggleFavorite toggles favorite status for a hotkey item
func (u *UserHotkeys) ToggleFavorite(categoryID, itemKey string) {
	if u.Favorites == nil {
		u.Favorites = make(map[string][]string)
	}

	items := u.Favorites[categoryID]
	for i, k := range items {
		if k == itemKey {
			// Remove from favorites
			u.Favorites[categoryID] = append(items[:i], items[i+1:]...)
			return
		}
	}
	// Add to favorites
	u.Favorites[categoryID] = append(items, itemKey)
}

// GetFavoriteCount returns the total number of favorites for the user
func (u *UserHotkeys) GetFavoriteCount() int {
	if u == nil {
		return 0
	}
	count := 0
	for _, items := range u.Favorites {
		count += len(items)
	}
	return count
}

// MigrateLegacyFavorites rewrites favorites stored in the old format (keyed by
// the raw Keys string) to the new format (keyed by the stable item ID).
//
// Migration logic:
//  1. Build a lookup table: for each category, map every known Keys value under
//     EITHER nav style to that item's stable ID.
//  2. Walk the existing favorites; for each entry that is NOT already a known
//     stable ID but IS a known Keys string, replace it with the stable ID.
//  3. De-duplicate: if two old entries resolve to the same stable ID (e.g. the
//     emacs and vim Keys for the same action were both saved), emit the ID once.
//  4. Entries that cannot be matched to any known item OR stable ID are left
//     in place (not dropped) so no favorite is ever silently lost.
//
// Calling this function on an already-migrated map is safe (idempotent).
//
// It returns true only if it actually rewrote at least one favorites entry, so
// callers can persist the config exclusively when something changed (avoiding a
// disk write on every launch for an already-migrated config).
func MigrateLegacyFavorites(u *UserHotkeys) bool {
	if u == nil || len(u.Favorites) == 0 {
		return false
	}

	keysToID, knownIDs := legacyFavoriteLookups()

	changed := false
	for catID, entries := range u.Favorites {
		catKeys := keysToID[catID] // may be nil if catID is unknown

		migrated := migrateFavoriteEntries(entries, catKeys, knownIDs)
		// Report a change if the rewritten slice differs from the original (an
		// entry was remapped to a stable ID, or a duplicate was collapsed).
		if !stringSliceEqual(entries, migrated) {
			changed = true
		}
		u.Favorites[catID] = migrated
	}
	return changed
}

func legacyFavoriteLookups() (map[string]map[string]string, map[string]bool) {
	keysToID := map[string]map[string]string{}
	knownIDs := map[string]bool{}
	for _, navStyle := range []string{"emacs", "vim"} {
		for _, category := range hotkeys.Categories(navStyle) {
			if keysToID[category.ID] == nil {
				keysToID[category.ID] = map[string]string{}
			}
			for _, item := range category.Items {
				knownIDs[item.ID] = true
				keysToID[category.ID][item.Keys] = item.ID
			}
		}
	}
	return keysToID, knownIDs
}

func migrateFavoriteEntries(entries []string, keysToID map[string]string, knownIDs map[string]bool) []string {
	seen := make(map[string]bool, len(entries))
	migrated := make([]string, 0, len(entries))
	for _, entry := range entries {
		resolved := entry
		if !knownIDs[entry] {
			if id, ok := keysToID[entry]; ok {
				resolved = id
			}
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		migrated = append(migrated, resolved)
	}
	return migrated
}

// stringSliceEqual reports whether two string slices have identical length and
// element-wise contents (order-sensitive).
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
