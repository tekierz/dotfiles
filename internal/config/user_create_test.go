package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func createProfileTestHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
}

func TestCreateUserProfilePreservesExistingBytes(t *testing.T) {
	for _, content := range []string{`{"name":"Alice","theme":"dracula","nav_style":"vim"}`, `not valid json`} {
		t.Run(content, func(t *testing.T) {
			createProfileTestHome(t)
			if err := os.MkdirAll(UsersDir(), 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(UsersDir(), "Alice.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if err := CreateUserProfile(DefaultUserProfile("Alice")); !errors.Is(err, ErrUserExists) {
				t.Fatalf("duplicate error=%v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte(content)) {
				t.Fatal("existing bytes changed")
			}
		})
	}
}

func TestCreateUserProfileConcurrentOneWinner(t *testing.T) {
	createProfileTestHome(t)
	const count = 16
	start := make(chan struct{})
	results := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; results <- CreateUserProfile(DefaultUserProfile("Alice")) }()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrUserExists) {
			t.Errorf("unexpected creator error: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
	profile, err := LoadUserProfile("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if profile.CreatedAt == "" || profile.UpdatedAt == "" {
		t.Fatal("created timestamps missing")
	}
	for _, path := range []string{UsersDir(), filepath.Join(UsersDir(), "Alice.json")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0600)
		if info.IsDir() {
			want = 0700
		}
		if info.Mode().Perm() != want {
			t.Errorf("mode %s=%o want %o", path, info.Mode().Perm(), want)
		}
	}
}

func TestCreateUserProfileProcessHelper(t *testing.T) {
	if os.Getenv("DOTFILES_CREATE_PROFILE_HELPER") != "1" {
		return
	}
	err := CreateUserProfile(DefaultUserProfile("Alice"))
	if errors.Is(err, ErrUserExists) {
		fmt.Println("PROFILE_ALREADY_EXISTS")
		return
	}
	if os.Getenv("DOTFILES_CREATE_PROFILE_COLD") == "1" && errors.Is(err, safefile.ErrDirectoryChanged) {
		fmt.Println("PROFILE_BOOTSTRAP_CONFLICT")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("PROFILE_CREATED")
}

func TestCreateUserProfileProcessesOneWinner(t *testing.T) {
	for _, cold := range []bool{false, true} {
		t.Run(fmt.Sprintf("cold=%v", cold), func(t *testing.T) {
			createProfileTestHome(t)
			if cold {
				t.Setenv("DOTFILES_CREATE_PROFILE_COLD", "1")
			} else {
				t.Setenv("DOTFILES_CREATE_PROFILE_COLD", "0")
				if err := CreateUserProfile(DefaultUserProfile("Seed")); err != nil {
					t.Fatal(err)
				}
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			const count = 4
			commands := make([]*exec.Cmd, count)
			outputs := make([]bytes.Buffer, count)
			for i := range commands {
				commands[i] = exec.CommandContext(ctx, executable, "-test.run=^TestCreateUserProfileProcessHelper$")
				commands[i].Env = append(os.Environ(), "DOTFILES_CREATE_PROFILE_HELPER=1", "GOMAXPROCS=2")
				commands[i].Stdout = &outputs[i]
				commands[i].Stderr = &outputs[i]
				if err := commands[i].Start(); err != nil {
					t.Fatal(err)
				}
			}
			winners := 0
			for i, cmd := range commands {
				if err := cmd.Wait(); err != nil {
					t.Fatalf("creator %d failed: %v: %s", i, err, outputs[i].String())
				}
				switch {
				case strings.Contains(outputs[i].String(), "PROFILE_CREATED"):
					winners++
				case strings.Contains(outputs[i].String(), "PROFILE_ALREADY_EXISTS"):
				case cold && strings.Contains(outputs[i].String(), "PROFILE_BOOTSTRAP_CONFLICT"):
					// The existing namespace bootstrap can refuse a concurrent creator
					// before locking. It must fail closed without replacing the winner.
					t.Log("cold namespace bootstrap refused a concurrent creator")
				default:
					t.Fatalf("creator %d omitted outcome", i)
				}
			}
			if winners != 1 {
				t.Fatalf("process winners=%d", winners)
			}
			if _, err := LoadUserProfile("Alice"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreateUserProfileDoesNotChangeExplicitSave(t *testing.T) {
	createProfileTestHome(t)
	profile := DefaultUserProfile("Alice")
	if err := CreateUserProfile(profile); err != nil {
		t.Fatal(err)
	}
	created := profile.CreatedAt
	profile.Theme = "dracula"
	profile.NavStyle = "vim"
	if err := SaveUserProfile(profile); err != nil {
		t.Fatal(err)
	}
	got, err := LoadUserProfile("Alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "dracula" || got.NavStyle != "vim" || got.CreatedAt != created {
		t.Fatalf("explicit save changed semantics: %+v", got)
	}
	if err := DeleteUserProfile("Alice"); err != nil {
		t.Fatal(err)
	}
	if err := CreateUserProfile(DefaultUserProfile("Alice")); err != nil {
		t.Fatalf("create after delete: %v", err)
	}
}

func TestCreateUserProfileRejectsInvalidAndSymlinkPaths(t *testing.T) {
	for _, kind := range []string{"invalid", "leaf", "parent"} {
		t.Run(kind, func(t *testing.T) {
			createProfileTestHome(t)
			outside := t.TempDir()
			target := filepath.Join(outside, "Alice.json")
			if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			profile := DefaultUserProfile("Alice")
			switch kind {
			case "invalid":
				profile.Name = "../Alice"
			case "leaf":
				if err := os.MkdirAll(UsersDir(), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(UsersDir(), "Alice.json")); err != nil {
					t.Fatal(err)
				}
			case "parent":
				if err := os.MkdirAll(ConfigDir(), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, UsersDir()); err != nil {
					t.Fatal(err)
				}
			}
			if err := CreateUserProfile(profile); err == nil {
				t.Fatal("unsafe profile path accepted")
			}
			after, err := os.ReadFile(target)
			if err != nil || string(after) != "unchanged" {
				t.Fatal("outside file changed")
			}
		})
	}
}
