package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// UserProfile represents a user configuration profile
type UserProfile struct {
	Name          string `json:"name"`
	Theme         string `json:"theme"`
	NavStyle      string `json:"nav_style"`      // "emacs" or "vim"
	KeyboardStyle string `json:"keyboard_style"` // "macos" or "linux"
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// usernameRegex validates username format
// Must start with letter, followed by 0-31 alphanumeric/underscore/hyphen chars
var usernameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,31}$`)

// ValidNavStyles are the allowed navigation styles
var ValidNavStyles = []string{"emacs", "vim"}

// ValidKeyboardStyles are the allowed keyboard styles
var ValidKeyboardStyles = []string{"macos", "linux"}

// UsersDir returns the users directory path
func UsersDir() string {
	return filepath.Join(ConfigDir(), "users")
}

// ValidateUsername checks if a username is valid
func ValidateUsername(name string) error {
	if name == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if !usernameRegex.MatchString(name) {
		return fmt.Errorf("invalid username %q: must start with a letter, contain only alphanumeric characters, underscores, or hyphens, and be at most 32 characters", name)
	}
	return nil
}

// IsValidNavStyle checks if a navigation style is valid
func IsValidNavStyle(style string) bool {
	for _, s := range ValidNavStyles {
		if s == style {
			return true
		}
	}
	return false
}

// IsValidKeyboardStyle checks if a keyboard style is valid
func IsValidKeyboardStyle(style string) bool {
	for _, s := range ValidKeyboardStyles {
		if s == style {
			return true
		}
	}
	return false
}

// UserExists checks if a user profile exists
func UserExists(name string) bool {
	path := filepath.Join(UsersDir(), name+".json")
	_, err := os.Stat(path)
	return err == nil
}

// LoadUserProfile loads a user profile from disk
func LoadUserProfile(name string) (*UserProfile, error) {
	if err := ValidateUsername(name); err != nil {
		return nil, err
	}

	path := filepath.Join(UsersDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("user %q does not exist", name)
		}
		return nil, fmt.Errorf("failed to read user profile: %w", err)
	}

	var profile UserProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &profile, nil
}

// SaveUserProfile saves a user profile to disk
func SaveUserProfile(profile *UserProfile) error {
	if err := ValidateUsername(profile.Name); err != nil {
		return err
	}

	if err := EnsureDirs(); err != nil {
		return err
	}

	// Update timestamp
	profile.UpdatedAt = time.Now().Format(time.RFC3339)
	if profile.CreatedAt == "" {
		profile.CreatedAt = profile.UpdatedAt
	}

	path := filepath.Join(UsersDir(), profile.Name+".json")
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal user profile: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write user profile: %w", err)
	}

	return nil
}

// DeleteUserProfile removes a user profile from disk
func DeleteUserProfile(name string) error {
	if err := ValidateUsername(name); err != nil {
		return err
	}

	if !UserExists(name) {
		return fmt.Errorf("user %q does not exist", name)
	}

	path := filepath.Join(UsersDir(), name+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete user profile: %w", err)
	}

	return nil
}

// ListUserProfiles returns all user profile names, sorted alphabetically
func ListUserProfiles() ([]string, error) {
	if err := EnsureDirs(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(UsersDir())
	if err != nil {
		return nil, fmt.Errorf("failed to read users directory: %w", err)
	}

	var users []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			users = append(users, strings.TrimSuffix(name, ".json"))
		}
	}

	sort.Strings(users)
	return users, nil
}

// DefaultUserProfile returns a new profile with default settings
func DefaultUserProfile(name string) *UserProfile {
	return &UserProfile{
		Name:          name,
		Theme:         "catppuccin-mocha",
		NavStyle:      "emacs",
		KeyboardStyle: "linux",
	}
}

// ApplyUserProfile applies a user profile's settings to the global config
func ApplyUserProfile(profile *UserProfile) error {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return err
	}

	cfg.Theme = profile.Theme
	cfg.NavStyle = profile.NavStyle
	cfg.ActiveUser = profile.Name

	return SaveGlobalConfig(cfg)
}

// GetActiveUser returns the currently active user profile, if any
func GetActiveUser() (*UserProfile, error) {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return nil, err
	}

	if cfg.ActiveUser == "" {
		return nil, nil
	}

	if !UserExists(cfg.ActiveUser) {
		return nil, nil
	}

	return LoadUserProfile(cfg.ActiveUser)
}

// ClearActiveUser clears the active user setting
func ClearActiveUser() error {
	cfg, err := LoadGlobalConfig()
	if err != nil {
		return err
	}

	cfg.ActiveUser = ""
	return SaveGlobalConfig(cfg)
}
