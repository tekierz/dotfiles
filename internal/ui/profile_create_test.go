package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/config"
)

func TestNewUserPreservesExistingProfile(t *testing.T) {
	withTempHome(t)
	profile := config.DefaultUserProfile("Alice")
	profile.Theme = "dracula"
	profile.NavStyle = "vim"
	if err := config.SaveUserProfile(profile); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.UsersDir(), "Alice.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(true)
	a.usersLoaded = true
	a.screenMgr.Navigate(ScreenUsers)
	a.usersCreating = true
	a.usersNewName = "Alice"
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("New command missing")
	}
	a.Update(cmd())
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("New overwrote the existing profile")
	}
	if !strings.Contains(a.usersStatus, "already exists") {
		t.Fatalf("duplicate error not visible: %q", a.usersStatus)
	}
}

func TestNewUserCreatesAndExplicitSaveUpdates(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	a.usersLoaded = true
	a.screenMgr.Navigate(ScreenUsers)
	a.usersCreating = true
	a.usersNewName = "Alice"
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("New command missing")
	}
	_, refresh := a.Update(cmd())
	if refresh == nil {
		t.Fatalf("create failed: %s", a.usersStatus)
	}
	a.Update(refresh())
	profile, err := config.LoadUserProfile("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Theme != "catppuccin-mocha" || profile.NavStyle != "emacs" {
		t.Fatalf("wrong create defaults: %+v", profile)
	}
	done := saveUserCmd("Alice", "dracula", "vim", "macos")().(userSavedMsg)
	if done.err != nil {
		t.Fatal(done.err)
	}
	updated, err := config.LoadUserProfile("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Theme != "dracula" || updated.NavStyle != "vim" || updated.CreatedAt != profile.CreatedAt {
		t.Fatal("explicit Save no longer updates existing profile")
	}
}
