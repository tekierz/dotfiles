package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/config"
)

func writeFutureGlobalConfig(t *testing.T) (string, []byte) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path := filepath.Join(config.ConfigDir(), "global.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	content := []byte(`{"schema_version":999,"theme":"future-theme","future_only":{"keep":true}}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write future global config: %v", err)
	}
	return path, content
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s was modified:\n got: %s\nwant: %s", path, got, want)
	}
}

func TestSaveInstallerConfigDoesNotOverwriteFutureGlobalSchema(t *testing.T) {
	path, original := writeFutureGlobalConfig(t)
	a := &App{theme: "dracula", navStyle: "vim", animationsEnabled: false}

	err := a.saveInstallerConfig()
	if err == nil || !strings.Contains(err.Error(), "unsupported global config schema_version 999") {
		t.Fatalf("saveInstallerConfig error = %v, want future-schema error", err)
	}
	assertFileBytes(t, path, original)
}

func TestSaveManageConfigDoesNotOverwriteFutureGlobalSchema(t *testing.T) {
	path, original := writeFutureGlobalConfig(t)
	a := &App{
		theme:                     "dracula",
		navStyle:                  "vim",
		animationsEnabled:         false,
		manageConfig:              NewManageConfig(),
		manageConfigBaseline:      *NewManageConfig(),
		manageConfigBaselineTheme: "dracula",
	}

	_, err := buildManageSavePlan(a, time.Now())
	if err == nil || !strings.Contains(err.Error(), "unsupported global config schema_version 999") {
		t.Fatalf("buildManageSavePlan error = %v, want future-schema error", err)
	}
	assertFileBytes(t, path, original)
	if _, err := os.Stat(filepath.Join(config.ToolsDir(), "manage.json")); !os.IsNotExist(err) {
		t.Fatalf("Manage save mutated manage.json before global schema validation, stat error = %v", err)
	}
}

func TestSaveCallsitesDoNotOverwriteMalformedGlobalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path := filepath.Join(config.ConfigDir(), "global.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	original := []byte(`{"schema_version":`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write malformed global config: %v", err)
	}

	installer := &App{theme: "dracula", navStyle: "vim", animationsEnabled: false}
	if err := installer.saveInstallerConfig(); err == nil || !strings.Contains(err.Error(), "failed to parse global config") {
		t.Fatalf("saveInstallerConfig error = %v, want parse error", err)
	}
	assertFileBytes(t, path, original)

	manager := &App{
		theme:                     "dracula",
		navStyle:                  "vim",
		animationsEnabled:         false,
		manageConfig:              NewManageConfig(),
		manageConfigBaseline:      *NewManageConfig(),
		manageConfigBaselineTheme: "dracula",
	}
	_, err := buildManageSavePlan(manager, time.Now())
	if err == nil || !strings.Contains(err.Error(), "failed to parse global config") {
		t.Fatalf("buildManageSavePlan error = %v, want parse error", err)
	}
	assertFileBytes(t, path, original)
}

func assertStartInstallationBlockedBeforeMutation(t *testing.T, wantError string) {
	t.Helper()
	home := os.Getenv("HOME")
	a := &App{
		theme:             "dracula",
		navStyle:          "vim",
		animationsEnabled: false,
		deepDiveConfig:    NewDeepDiveConfig(),
	}
	cmd := a.startInstallation()
	if cmd == nil {
		t.Fatal("startInstallation returned nil instead of a fatal result")
	}
	done, ok := cmd().(installDoneMsg)
	if !ok {
		t.Fatalf("startInstallation returned unexpected message type")
	}
	if done.err == nil || !strings.Contains(done.err.Error(), wantError) {
		t.Fatalf("startInstallation error = %v, want %q", done.err, wantError)
	}
	if a.installEvents != nil || a.streamCancel != nil || a.installPlannedSteps != 0 {
		t.Fatalf("installation worker/plan initialized after global-config failure: events=%v cancel=%v steps=%d", a.installEvents != nil, a.streamCancel != nil, a.installPlannedSteps)
	}
	for _, path := range []string{
		filepath.Join(home, ".local", "bin", "dotfiles"),
		filepath.Join(home, ".config", "ghostty", "config"),
		filepath.Join(home, ".tmux.conf"),
		filepath.Join(home, ".gitconfig"),
		filepath.Join(config.ConfigDir(), "backups"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("global-config failure allowed install mutation at %s (stat error %v)", path, err)
		}
	}
}

func TestStartInstallationFailsClosedOnFutureGlobalConfig(t *testing.T) {
	path, original := writeFutureGlobalConfig(t)
	assertStartInstallationBlockedBeforeMutation(t, "unsupported global config schema_version 999")
	assertFileBytes(t, path, original)
}

func TestStartInstallationFailsClosedOnMalformedGlobalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path := filepath.Join(config.ConfigDir(), "global.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"schema_version":`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	assertStartInstallationBlockedBeforeMutation(t, "failed to parse global config")
	assertFileBytes(t, path, original)
}

func TestStandaloneConfigFailsClosedBeforeToolMutation(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wantErr string
	}{
		{name: "future schema", content: []byte(`{"schema_version":999,"future_only":true}`), wantErr: "unsupported global config schema_version 999"},
		{name: "malformed", content: []byte(`{"schema_version":`), wantErr: "failed to parse global config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			globalPath := filepath.Join(config.ConfigDir(), "global.json")
			if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(globalPath, tt.content, 0o600); err != nil {
				t.Fatal(err)
			}
			toolPath := filepath.Join(home, ".gitconfig")
			sentinel := []byte("# user-owned git config\n")
			if err := os.WriteFile(toolPath, sentinel, 0o600); err != nil {
				t.Fatal(err)
			}

			errs := applyStandaloneSnapshot(ScreenConfigGit, *NewDeepDiveConfig(), "dracula")
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), tt.wantErr) {
				t.Fatalf("standalone errors = %v, want one containing %q", errs, tt.wantErr)
			}
			assertFileBytes(t, toolPath, sentinel)
			assertFileBytes(t, globalPath, tt.content)
		})
	}
}
