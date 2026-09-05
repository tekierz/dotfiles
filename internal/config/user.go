package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
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
	if ConfigDir() == "" {
		return nil, ErrNoConfigDir
	}

	path := filepath.Join(UsersDir(), name+".json")
	data, revision, err := readProductConfigJSON(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read user profile: %w", err)
	}
	if !revision.Exists() {
		return nil, fmt.Errorf("user %q does not exist", name)
	}

	var profile UserProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &profile, nil
}

// ErrUserExists reports that create-only profile persistence found an existing file.
var ErrUserExists = errors.New("user profile already exists")

var userProfileSaveMu sync.Mutex

// withUserProfileLock shares one private lock protocol across create/save/delete.
// The in-process lock also serializes namespace bootstrap for first-time saves.
func withUserProfileLock(name string, mutate func(root, rel string) error) (returnErr error) {
	userProfileSaveMu.Lock()
	defer userProfileSaveMu.Unlock()
	if err := ValidateUsername(name); err != nil {
		return err
	}
	if ConfigDir() == "" {
		return ErrNoConfigDir
	}
	path := filepath.Join(UsersDir(), name+".json")
	if err := ensureConfiguredXDGRoot(path); err != nil {
		return err
	}
	root, rel, err := anchoredFilePath(path)
	if err != nil {
		return fmt.Errorf("resolve user profile path: %w", err)
	}
	release, err := operation.DefaultLocker("user-profile", path)
	if err != nil {
		return fmt.Errorf("lock user profile: %w", err)
	}
	defer func() {
		if err := release(); err != nil {
			if returnErr == nil {
				returnErr = &safefile.CommittedError{Operation: "release user profile lock", Err: err}
			} else {
				returnErr = errors.Join(returnErr, err)
			}
		}
	}()
	return mutate(root, rel)
}

// SaveUserProfile saves a profile, retaining the explicit Save/upsert behavior.
func SaveUserProfile(profile *UserProfile) error {
	return saveUserProfile(profile, false)
}

// CreateUserProfile creates a profile only while its destination remains absent.
// Cooperating creators and explicit saves share the private per-profile lock.
func CreateUserProfile(profile *UserProfile) error {
	return saveUserProfile(profile, true)
}

func saveUserProfile(profile *UserProfile, createOnly bool) error {
	if profile == nil {
		return fmt.Errorf("user profile is nil")
	}
	return withUserProfileLock(profile.Name, func(root, rel string) error {
		var revision safefile.Revision
		var parents *safefile.ParentChain
		if createOnly {
			if err := safefile.EnsureDirectoryWithin(root, filepath.ToSlash(filepath.Dir(rel)), 0o700); err != nil {
				return err
			}
			_, observed, chain, err := safefile.ObserveFileWithinLimit(root, rel, maxProductConfigJSONBytes)
			if err != nil {
				return fmt.Errorf("inspect user profile: %w", err)
			}
			if observed.Exists() {
				return fmt.Errorf("user %q: %w", profile.Name, ErrUserExists)
			}
			revision, parents = observed, chain
		}
		profile.UpdatedAt = time.Now().Format(time.RFC3339)
		if profile.CreatedAt == "" {
			profile.CreatedAt = profile.UpdatedAt
		}
		data, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal user profile: %w", err)
		}
		if createOnly {
			_, err = safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(root, rel, revision, parents, data, 0o600)
		} else {
			err = safefile.ReplaceWithin(root, rel, data, 0o600)
		}
		if err != nil {
			return fmt.Errorf("failed to write user profile: %w", err)
		}
		return nil
	})
}

// DeleteUserProfile removes a user profile under the same mutation lock.
func DeleteUserProfile(name string) error {
	return withUserProfileLock(name, func(root, rel string) error {
		if err := safefile.RemoveWithin(root, rel); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("user %q does not exist", name)
			}
			return fmt.Errorf("failed to delete user profile: %w", err)
		}
		return nil
	})
}

// ListUserProfiles returns all user profile names, sorted alphabetically
func ListUserProfiles() ([]string, error) {
	if ConfigDir() == "" {
		return nil, ErrNoConfigDir
	}

	entries, err := os.ReadDir(UsersDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
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
