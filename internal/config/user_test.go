package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shared test fixtures for the config package tests.
const (
	testUserName         = "testuser"
	themeCatppuccinMocha = "catppuccin-mocha"
	themeDracula         = "dracula"
	navStyleVim          = "vim"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		// Valid names
		{"alice", true},
		{"Bob", true},
		{"user123", true},
		{"test_user", true},
		{"test-user", true},
		{"A", true},
		{"user_123_test", true},
		{strings.Repeat("a", 32), true}, // max length

		// Invalid names
		{"", false},                      // empty
		{"123user", false},               // starts with number
		{"_user", false},                 // starts with underscore
		{"-user", false},                 // starts with hyphen
		{"user@name", false},             // invalid char
		{"user.name", false},             // invalid char
		{"user name", false},             // space
		{strings.Repeat("a", 33), false}, // too long
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.name)
			if tt.valid && err != nil {
				t.Errorf("expected %q to be valid, got error: %v", tt.name, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected %q to be invalid, but got no error", tt.name)
			}
		})
	}
}

func TestIsValidNavStyle(t *testing.T) {
	tests := []struct {
		style string
		valid bool
	}{
		{"emacs", true},
		{navStyleVim, true},
		{"Emacs", false},
		{"VIM", false},
		{"", false},
		{"nano", false},
	}

	for _, tt := range tests {
		t.Run(tt.style, func(t *testing.T) {
			got := IsValidNavStyle(tt.style)
			if got != tt.valid {
				t.Errorf("IsValidNavStyle(%q) = %v, want %v", tt.style, got, tt.valid)
			}
		})
	}
}

func TestIsValidKeyboardStyle(t *testing.T) {
	tests := []struct {
		style string
		valid bool
	}{
		{"macos", true},
		{"linux", true},
		{"MacOS", false},
		{"LINUX", false},
		{"", false},
		{"windows", false},
	}

	for _, tt := range tests {
		t.Run(tt.style, func(t *testing.T) {
			got := IsValidKeyboardStyle(tt.style)
			if got != tt.valid {
				t.Errorf("IsValidKeyboardStyle(%q) = %v, want %v", tt.style, got, tt.valid)
			}
		})
	}
}

func TestDefaultUserProfile(t *testing.T) {
	profile := DefaultUserProfile(testUserName)

	if profile.Name != testUserName {
		t.Errorf("Name = %q, want %q", profile.Name, testUserName)
	}
	if profile.Theme != themeCatppuccinMocha {
		t.Errorf("Theme = %q, want %q", profile.Theme, themeCatppuccinMocha)
	}
	if profile.NavStyle != "emacs" {
		t.Errorf("NavStyle = %q, want %q", profile.NavStyle, "emacs")
	}
	if profile.KeyboardStyle != "linux" {
		t.Errorf("KeyboardStyle = %q, want %q", profile.KeyboardStyle, "linux")
	}
}

func setupTestConfigDir(t *testing.T) func() {
	t.Helper()

	// Create temp directory
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".config", "dotfiles", "users")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create test config dir: %v", err)
	}

	// Store and replace env vars
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	origHome := os.Getenv("HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	os.Setenv("HOME", dir)

	cleanup := func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
		if origHome != "" {
			os.Setenv("HOME", origHome)
		} else {
			os.Unsetenv("HOME")
		}
	}

	return cleanup
}

func TestUserProfileCRUD(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Test Save
	profile := DefaultUserProfile(testUserName)
	profile.Theme = themeDracula
	profile.NavStyle = navStyleVim

	if err := SaveUserProfile(profile); err != nil {
		t.Fatalf("SaveUserProfile failed: %v", err)
	}

	// Test UserExists
	if !UserExists(testUserName) {
		t.Error("UserExists returned false for existing user")
	}
	if UserExists("nonexistent") {
		t.Error("UserExists returned true for non-existent user")
	}

	// Test Load
	loaded, err := LoadUserProfile(testUserName)
	if err != nil {
		t.Fatalf("LoadUserProfile failed: %v", err)
	}
	if loaded.Name != testUserName {
		t.Errorf("Name = %q, want %q", loaded.Name, testUserName)
	}
	if loaded.Theme != themeDracula {
		t.Errorf("Theme = %q, want %q", loaded.Theme, themeDracula)
	}
	if loaded.NavStyle != navStyleVim {
		t.Errorf("NavStyle = %q, want %q", loaded.NavStyle, navStyleVim)
	}
	if loaded.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}
	if loaded.UpdatedAt == "" {
		t.Error("UpdatedAt should be set")
	}

	// Test List
	users, err := ListUserProfiles()
	if err != nil {
		t.Fatalf("ListUserProfiles failed: %v", err)
	}
	if len(users) != 1 || users[0] != testUserName {
		t.Errorf("ListUserProfiles = %v, want [testuser]", users)
	}

	// Add another user
	profile2 := DefaultUserProfile("anotheruser")
	if err := SaveUserProfile(profile2); err != nil {
		t.Fatalf("SaveUserProfile failed: %v", err)
	}

	users, err = ListUserProfiles()
	if err != nil {
		t.Fatalf("ListUserProfiles failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
	// Should be sorted alphabetically
	if users[0] != "anotheruser" {
		t.Errorf("first user should be anotheruser, got %s", users[0])
	}

	// Test Delete
	if err := DeleteUserProfile(testUserName); err != nil {
		t.Fatalf("DeleteUserProfile failed: %v", err)
	}
	if UserExists(testUserName) {
		t.Error("user should not exist after delete")
	}

	// Delete non-existent should fail
	if err := DeleteUserProfile(testUserName); err == nil {
		t.Error("deleting non-existent user should fail")
	}
}

func TestLoadUserProfile_Errors(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Invalid username
	_, err := LoadUserProfile("123invalid")
	if err == nil {
		t.Error("LoadUserProfile should fail for invalid username")
	}

	// Non-existent user
	_, err = LoadUserProfile("nonexistent")
	if err == nil {
		t.Error("LoadUserProfile should fail for non-existent user")
	}
}

func TestSaveUserProfile_InvalidUsername(t *testing.T) {
	profile := &UserProfile{
		Name: "123invalid",
	}

	err := SaveUserProfile(profile)
	if err == nil {
		t.Error("SaveUserProfile should fail for invalid username")
	}
}

func TestApplyUserProfile(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Create a user profile
	profile := DefaultUserProfile(testUserName)
	profile.Theme = "nord"
	profile.NavStyle = navStyleVim

	if err := SaveUserProfile(profile); err != nil {
		t.Fatalf("SaveUserProfile failed: %v", err)
	}

	// Apply it
	if err := ApplyUserProfile(profile); err != nil {
		t.Fatalf("ApplyUserProfile failed: %v", err)
	}

	// Verify global config was updated
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}

	if cfg.Theme != "nord" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "nord")
	}
	if cfg.NavStyle != navStyleVim {
		t.Errorf("NavStyle = %q, want %q", cfg.NavStyle, navStyleVim)
	}
	if cfg.ActiveUser != testUserName {
		t.Errorf("ActiveUser = %q, want %q", cfg.ActiveUser, testUserName)
	}
}

func TestGetActiveUser(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// No active user initially
	user, err := GetActiveUser()
	if err != nil {
		t.Fatalf("GetActiveUser failed: %v", err)
	}
	if user != nil {
		t.Error("expected nil user initially")
	}

	// Create and apply a user
	profile := DefaultUserProfile("activetest")
	if err := SaveUserProfile(profile); err != nil {
		t.Fatalf("SaveUserProfile failed: %v", err)
	}
	if err := ApplyUserProfile(profile); err != nil {
		t.Fatalf("ApplyUserProfile failed: %v", err)
	}

	// Now GetActiveUser should return the user
	user, err = GetActiveUser()
	if err != nil {
		t.Fatalf("GetActiveUser failed: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Name != "activetest" {
		t.Errorf("Name = %q, want %q", user.Name, "activetest")
	}
}

func TestClearActiveUser(t *testing.T) {
	cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Set an active user
	profile := DefaultUserProfile("cleartest")
	if err := SaveUserProfile(profile); err != nil {
		t.Fatalf("SaveUserProfile failed: %v", err)
	}
	if err := ApplyUserProfile(profile); err != nil {
		t.Fatalf("ApplyUserProfile failed: %v", err)
	}

	// Clear it
	if err := ClearActiveUser(); err != nil {
		t.Fatalf("ClearActiveUser failed: %v", err)
	}

	// Verify
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if cfg.ActiveUser != "" {
		t.Errorf("ActiveUser = %q, want empty", cfg.ActiveUser)
	}
}
