package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// LoadHotkeysConfig loads hotkeys config from ~/.config/dotfiles/hotkeys.json
func LoadHotkeysConfig() (*HotkeysConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".config", "dotfiles", "hotkeys.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &HotkeysConfig{Users: make(map[string]*UserHotkeys)}, nil
		}
		return nil, err
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

// SaveHotkeysConfig saves hotkeys config
func SaveHotkeysConfig(cfg *HotkeysConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "dotfiles")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "hotkeys.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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
