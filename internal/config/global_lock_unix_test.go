//go:build darwin || linux

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

func setupPrivateGlobalConfigLockTest(t *testing.T) (*GlobalConfig, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg, filepath.Join(ConfigDir(), "global.json")
}

func TestGlobalConfigUsesPrivateStateLockWithoutSidecar(t *testing.T) {
	cfg, path := setupPrivateGlobalConfigLockTest(t)
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "global.json" {
		t.Fatalf("config directory entries = %v, want only global.json", entries)
	}
	locks := filepath.Join(os.Getenv("XDG_STATE_HOME"), "dotfiles", "locks")
	lockEntries, err := os.ReadDir(locks)
	if err != nil {
		t.Fatalf("read private state locks: %v", err)
	}
	if len(lockEntries) == 0 {
		t.Fatal("global save created no private state lock")
	}
}

func TestOrdinaryAndAcceptedGlobalConfigWritersSharePrivateLock(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		name := "ordinary"
		if accepted {
			name = "accepted"
		}
		t.Run(name, func(t *testing.T) {
			cfg, path := setupPrivateGlobalConfigLockTest(t)
			if err := SaveGlobalConfig(cfg); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadGlobalConfig()
			if err != nil {
				t.Fatal(err)
			}
			cfg.Theme = "dracula"
			root, rel, err := anchoredFilePath(path)
			if err != nil {
				t.Fatal(err)
			}
			_, revision, parents, err := safefile.ObserveFileWithin(root, rel)
			if err != nil {
				t.Fatal(err)
			}
			release, err := operation.AcquireStateLock("global-config", path)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				if accepted {
					_, err = SaveGlobalConfigAtAuthorityTracked(cfg, revision, parents)
				} else {
					err = SaveGlobalConfig(cfg)
				}
				done <- err
			}()
			select {
			case err := <-done:
				_ = release()
				t.Fatalf("writer bypassed held private lock: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("writer did not acquire released private lock")
			}
		})
	}
}

func TestGlobalConfigPrivateLockSupportsXDGOnlyState(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(workspace, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(workspace, "state"))
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "dotfiles", "locks")); err != nil {
		t.Fatalf("XDG-only private lock state: %v", err)
	}
}
