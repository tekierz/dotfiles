package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()

	if cfg.Theme != "catppuccin-mocha" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "catppuccin-mocha")
	}
	if cfg.NavStyle != "emacs" {
		t.Errorf("NavStyle = %q, want %q", cfg.NavStyle, "emacs")
	}
	if cfg.ActiveUser != "" {
		t.Errorf("ActiveUser = %q, want empty", cfg.ActiveUser)
	}
	if cfg.DisableAnimations {
		t.Error("DisableAnimations should be false by default")
	}
	if cfg.SchemaVersion != CurrentGlobalConfigSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", cfg.SchemaVersion, CurrentGlobalConfigSchemaVersion)
	}
}

func TestConfigDir(t *testing.T) {
	// Test with XDG_CONFIG_HOME set
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir := ConfigDir()
	expected := "/custom/config/dotfiles"
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}

	// Test without XDG_CONFIG_HOME (uses HOME)
	os.Unsetenv("XDG_CONFIG_HOME")
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/home/testuser")
	defer os.Setenv("HOME", origHome)

	dir = ConfigDir()
	expected = "/home/testuser/.config/dotfiles"
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}
}

func TestConfigDirIgnoresRelativeXDGPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	want := filepath.Join(home, ".config", "dotfiles")
	if got := ConfigDir(); got != want {
		t.Fatalf("ConfigDir() = %q, want HOME fallback %q", got, want)
	}
}

func TestToolsDir(t *testing.T) {
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	dir := ToolsDir()
	expected := "/custom/config/dotfiles/tools"
	if dir != expected {
		t.Errorf("ToolsDir() = %q, want %q", dir, expected)
	}
}

func TestGlobalConfigCRUD(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Load when file doesn't exist - should return defaults
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if cfg.Theme != "catppuccin-mocha" {
		t.Errorf("Theme = %q, want default", cfg.Theme)
	}

	// Save modified config
	cfg.Theme = "dracula"
	cfg.NavStyle = "vim"
	cfg.DisableAnimations = true

	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("SaveGlobalConfig failed: %v", err)
	}

	// Reload and verify
	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if loaded.Theme != "dracula" {
		t.Errorf("Theme = %q, want %q", loaded.Theme, "dracula")
	}
	if loaded.NavStyle != "vim" {
		t.Errorf("NavStyle = %q, want %q", loaded.NavStyle, "vim")
	}
	if !loaded.DisableAnimations {
		t.Error("DisableAnimations should be true")
	}
}

func TestGlobalConfigCreatesMissingExternalXDGRootSafely(t *testing.T) {
	workspace := t.TempDir()
	home := filepath.Join(workspace, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	xdg := filepath.Join(workspace, "external-xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("load missing external-XDG config: %v", err)
	}
	cfg.Theme = "dracula"
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("save external-XDG config: %v", err)
	}
	path := filepath.Join(xdg, "dotfiles", "global.json")
	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "dracula" {
		t.Fatalf("saved theme = %q, want dracula", loaded.Theme)
	}
	for _, dir := range []string{xdg, filepath.Join(xdg, "dotfiles")} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %04o, want 0700", dir, info.Mode().Perm())
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("global.json mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestGlobalConfigSupportsAbsoluteXDGWithoutHOME(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	if err := os.Mkdir(xdg, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", "")

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig with XDG only: %v", err)
	}
	cfg.Theme = "dracula"
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("SaveGlobalConfig with XDG only: %v", err)
	}
	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "dracula" {
		t.Fatalf("saved XDG-only theme = %q, want dracula", loaded.Theme)
	}
}

func TestGlobalConfigLockNameIsStableAndTargetSpecific(t *testing.T) {
	first := globalConfigLockRel("dotfiles/global.json")
	if first != globalConfigLockRel("dotfiles/global.json") {
		t.Fatal("global config lock name is not stable")
	}
	if first == globalConfigLockRel("other/global.json") {
		t.Fatal("distinct config targets share one lock name")
	}
	if strings.Contains(first, "/") || !strings.HasPrefix(first, ".dotfiles-global-config-") {
		t.Fatalf("unsafe global config lock name %q", first)
	}
}

func TestLoadGlobalConfigMigratesPartialLegacyJSONOntoDefaults(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	path := filepath.Join(ConfigDir(), "global.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dracula","nav_style":"vim"}`), 0o600); err != nil {
		t.Fatalf("write partial legacy config: %v", err)
	}

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if cfg.SchemaVersion != CurrentGlobalConfigSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", cfg.SchemaVersion, CurrentGlobalConfigSchemaVersion)
	}
	if cfg.Theme != "dracula" || cfg.NavStyle != "vim" {
		t.Errorf("loaded explicit legacy fields = theme %q nav %q, want dracula/vim", cfg.Theme, cfg.NavStyle)
	}
	if !cfg.AutoBackup || cfg.BackupMaxCount != 10 || cfg.BackupMaxAgeDays != 30 {
		t.Errorf("migrated backup defaults = auto %v count %d age %d, want true/10/30",
			cfg.AutoBackup, cfg.BackupMaxCount, cfg.BackupMaxAgeDays)
	}
}

func TestLoadGlobalConfigPreservesExplicitFalseAndZero(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "current schema",
			json: `{"schema_version":1,"theme":"nord","nav_style":"emacs","auto_backup":false,"backup_max_count":0,"backup_max_age_days":0}`,
		},
		{
			name: "unversioned legacy with explicit values",
			json: `{"theme":"nord","nav_style":"emacs","auto_backup":false,"backup_max_count":0,"backup_max_age_days":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cleanup := setupTestConfigDir(t)
			defer cleanup()

			if err := os.WriteFile(filepath.Join(ConfigDir(), "global.json"), []byte(tt.json), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			cfg, err := LoadGlobalConfig()
			if err != nil {
				t.Fatalf("LoadGlobalConfig failed: %v", err)
			}
			if cfg.AutoBackup || cfg.BackupMaxCount != 0 || cfg.BackupMaxAgeDays != 0 {
				t.Errorf("explicit values changed: auto %v count %d age %d, want false/0/0",
					cfg.AutoBackup, cfg.BackupMaxCount, cfg.BackupMaxAgeDays)
			}
		})
	}
}

func TestSaveGlobalConfigStampsSchemaWithoutMutatingCaller(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg := &GlobalConfig{Theme: "nord", NavStyle: "vim"}
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("SaveGlobalConfig failed: %v", err)
	}
	if cfg.SchemaVersion != 0 {
		t.Fatalf("SaveGlobalConfig mutated caller schema to %d", cfg.SchemaVersion)
	}

	data, err := os.ReadFile(filepath.Join(ConfigDir(), "global.json"))
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parse saved config: %v", err)
	}
	if got := strings.TrimSpace(string(saved["schema_version"])); got != "1" {
		t.Errorf("saved schema_version = %q, want 1", got)
	}
	if got := strings.TrimSpace(string(saved["theme"])); got != `"nord"` {
		t.Errorf("saved theme = %q, want nord", got)
	}
}

func TestSaveGlobalConfigAllowsLegacyStructLiteralToCreateMissingFile(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg := &GlobalConfig{ActiveUser: "alice"}
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("SaveGlobalConfig legacy literal failed: %v", err)
	}
	if cfg.SchemaVersion != 0 || cfg.Theme != "" || cfg.NavStyle != "" {
		t.Fatalf("SaveGlobalConfig unexpectedly mutated public caller fields: %#v", cfg)
	}

	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if loaded.SchemaVersion != CurrentGlobalConfigSchemaVersion || loaded.Theme != "catppuccin-mocha" || loaded.NavStyle != "emacs" {
		t.Errorf("legacy literal normalized to schema/theme/nav %d/%q/%q, want 1/catppuccin-mocha/emacs",
			loaded.SchemaVersion, loaded.Theme, loaded.NavStyle)
	}
	if loaded.ActiveUser != "alice" {
		t.Errorf("ActiveUser = %q, want alice", loaded.ActiveUser)
	}
}

func TestGlobalConfigPreservesUnknownCurrentSchemaFields(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	path := filepath.Join(ConfigDir(), "global.json")
	original := []byte(`{
  "schema_version": 1,
  "theme": "nord",
  "nav_style": "emacs",
  "auto_backup": true,
  "backup_max_count": 10,
  "backup_max_age_days": 30,
  "enterprise_policy": {"locked": true, "groups": ["friends", "family"]},
  "new_boolean": false
}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write config with unknown fields: %v", err)
	}

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	cfg.Theme = "dracula"
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("SaveGlobalConfig failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	var saved map[string]json.RawMessage
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parse saved config: %v", err)
	}
	if got := strings.TrimSpace(string(saved["theme"])); got != `"dracula"` {
		t.Errorf("saved theme = %s, want dracula", got)
	}
	if got := strings.TrimSpace(string(saved["new_boolean"])); got != "false" {
		t.Errorf("unknown boolean = %q, want false", got)
	}
	var policy struct {
		Locked bool     `json:"locked"`
		Groups []string `json:"groups"`
	}
	if err := json.Unmarshal(saved["enterprise_policy"], &policy); err != nil {
		t.Fatalf("parse preserved enterprise_policy: %v", err)
	}
	if !policy.Locked || len(policy.Groups) != 2 || policy.Groups[0] != "friends" || policy.Groups[1] != "family" {
		t.Errorf("preserved enterprise_policy = %#v", policy)
	}

	// A second load/save proves the opaque fields remain attached across normal
	// read-modify-write cycles rather than surviving only the first migration.
	reloaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatalf("second LoadGlobalConfig failed: %v", err)
	}
	reloaded.NavStyle = "vim"
	if err := SaveGlobalConfig(reloaded); err != nil {
		t.Fatalf("second SaveGlobalConfig failed: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read second saved config: %v", err)
	}
	saved = nil
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("parse second saved config: %v", err)
	}
	if _, ok := saved["enterprise_policy"]; !ok {
		t.Error("enterprise_policy was dropped on second save")
	}
	if _, ok := saved["new_boolean"]; !ok {
		t.Error("new_boolean was dropped on second save")
	}
}

func TestSaveGlobalConfigRejectsInvalidInputBeforeCreatingDirectories(t *testing.T) {
	tests := []struct {
		name string
		cfg  *GlobalConfig
		want string
	}{
		{name: "nil", cfg: nil, want: "config is nil"},
		{name: "negative schema", cfg: &GlobalConfig{SchemaVersion: -1}, want: "unsupported global config schema_version -1"},
		{name: "future schema", cfg: &GlobalConfig{SchemaVersion: 2}, want: "unsupported global config schema_version 2"},
		{name: "invalid theme", cfg: &GlobalConfig{SchemaVersion: 1, Theme: "not-a-theme", NavStyle: "emacs"}, want: "invalid global config theme"},
		{name: "invalid nav", cfg: &GlobalConfig{SchemaVersion: 1, Theme: "nord", NavStyle: "arrows"}, want: "invalid global config nav_style"},
		{name: "invalid active user", cfg: &GlobalConfig{SchemaVersion: 1, Theme: "nord", NavStyle: "emacs", ActiveUser: "../admin"}, want: "invalid global config active_user"},
		{name: "negative count", cfg: &GlobalConfig{SchemaVersion: 1, Theme: "nord", NavStyle: "emacs", BackupMaxCount: -1}, want: "backup_max_count"},
		{name: "negative age", cfg: &GlobalConfig{SchemaVersion: 1, Theme: "nord", NavStyle: "emacs", BackupMaxAgeDays: -1}, want: "backup_max_age_days"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			xdg := filepath.Join(root, "not-created")
			t.Setenv("XDG_CONFIG_HOME", xdg)
			t.Setenv("HOME", root)

			err := SaveGlobalConfig(tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("SaveGlobalConfig error = %v, want containing %q", err, tt.want)
			}
			if _, statErr := os.Stat(xdg); !os.IsNotExist(statErr) {
				t.Fatalf("invalid save created config directory, stat error = %v", statErr)
			}
		})
	}
}

func TestLoadGlobalConfigRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "malformed JSON", content: `{"theme":`, want: "failed to parse global config"},
		{name: "non-object JSON", content: `null`, want: "expected JSON object"},
		{name: "wrong field type", content: `{"auto_backup":"yes"}`, want: "failed to parse global config"},
		{name: "wrong schema type", content: `{"schema_version":"1"}`, want: "schema_version"},
		{name: "null schema", content: `{"schema_version":null}`, want: "must not be null"},
		{name: "fractional schema", content: `{"schema_version":1.5}`, want: "schema_version"},
		{name: "future schema", content: `{"schema_version":2}`, want: "unsupported global config schema_version 2"},
		{name: "negative schema", content: `{"schema_version":-1}`, want: "unsupported global config schema_version -1"},
		{name: "invalid theme", content: `{"theme":"not-a-theme"}`, want: "invalid global config theme"},
		{name: "invalid nav", content: `{"nav_style":"arrows"}`, want: "invalid global config nav_style"},
		{name: "invalid active user", content: `{"active_user":"../admin"}`, want: "invalid global config active_user"},
		{name: "negative count", content: `{"backup_max_count":-1}`, want: "backup_max_count"},
		{name: "negative age", content: `{"backup_max_age_days":-1}`, want: "backup_max_age_days"},
		{name: "duplicate schema", content: `{"schema_version":1,"schema_version":0}`, want: "duplicate global config key"},
		{name: "case-folded schema", content: `{"schema_version":1,"SCHEMA_VERSION":1}`, want: "noncanonical global config key"},
		{name: "unicode case-folded schema", content: "{\"ſchema_version\":1}", want: "noncanonical global config key"},
		{name: "case-folded theme", content: `{"Theme":"nord"}`, want: "noncanonical global config key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cleanup := setupTestConfigDir(t)
			defer cleanup()

			if err := os.WriteFile(filepath.Join(ConfigDir(), "global.json"), []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write invalid config: %v", err)
			}
			_, err := LoadGlobalConfig()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadGlobalConfig error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadGlobalConfigRejectsNullForEveryKnownScalar(t *testing.T) {
	keys := []string{
		"schema_version",
		"theme",
		"nav_style",
		"active_user",
		"disable_animations",
		"auto_backup",
		"backup_max_count",
		"backup_max_age_days",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			_, cleanup := setupTestConfigDir(t)
			defer cleanup()

			content := fmt.Sprintf(`{%q:null}`, key)
			if err := os.WriteFile(filepath.Join(ConfigDir(), "global.json"), []byte(content), 0o600); err != nil {
				t.Fatalf("write null config: %v", err)
			}
			_, err := LoadGlobalConfig()
			if err == nil || !strings.Contains(err.Error(), "must not be null") {
				t.Fatalf("LoadGlobalConfig error = %v, want null rejection", err)
			}
		})
	}
}

func TestGlobalConfigCASRejectsStaleLoadedWriter(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	initial := DefaultGlobalConfig()
	if err := SaveGlobalConfig(initial); err != nil {
		t.Fatalf("create initial global config: %v", err)
	}
	first, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	stale, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}

	first.Theme = "dracula"
	if err := SaveGlobalConfig(first); err != nil {
		t.Fatalf("save first writer: %v", err)
	}
	stale.NavStyle = "vim"
	if err := SaveGlobalConfig(stale); !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("stale SaveGlobalConfig error = %v, want ErrGlobalConfigConflict", err)
	}

	got, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "dracula" || got.NavStyle != "emacs" {
		t.Fatalf("stale writer changed config to theme/nav %q/%q", got.Theme, got.NavStyle)
	}
}

func TestGlobalConfigCASSerializesConcurrentWriters(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	if err := SaveGlobalConfig(DefaultGlobalConfig()); err != nil {
		t.Fatal(err)
	}
	first, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	first.Theme = "dracula"
	second.Theme = "nord"

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, cfg := range []*GlobalConfig{first, second} {
		wg.Add(1)
		go func(cfg *GlobalConfig) {
			defer wg.Done()
			<-start
			errs <- SaveGlobalConfig(cfg)
		}(cfg)
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrGlobalConfigConflict):
			conflicts++
		default:
			t.Fatalf("concurrent save returned unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results = %d success/%d conflicts, want 1/1", successes, conflicts)
	}
}

func TestGlobalConfigConcurrentSavesOfSamePointerAreSerialized(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Theme = "dracula"

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- SaveGlobalConfig(cfg)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("same-pointer concurrent save failed: %v", err)
		}
	}
}

func TestGlobalConfigReservationRejectsStaleWriterBeforeCallback(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	if err := SaveGlobalConfig(DefaultGlobalConfig()); err != nil {
		t.Fatal(err)
	}
	stale, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	fresh.Theme = "dracula"
	if err := SaveGlobalConfig(fresh); err != nil {
		t.Fatal(err)
	}

	called := false
	err = SaveGlobalConfigWithReservedRevision(stale, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("reservation error = %v, want ErrGlobalConfigConflict", err)
	}
	if called {
		t.Fatal("reserved callback ran before stale revision was rejected")
	}
}

func TestGlobalConfigReservationRejectsAndPreservesEditDuringCallback(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	if err := SaveGlobalConfig(DefaultGlobalConfig()); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Theme = "dracula"
	replacement := []byte(`{"schema_version":999,"future_only":true}`)
	path := filepath.Join(ConfigDir(), "global.json")

	err = SaveGlobalConfigWithReservedRevision(cfg, func() error {
		tmp := path + ".noncooperating-callback"
		if err := os.WriteFile(tmp, replacement, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	})
	if !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("reservation error = %v, want ErrGlobalConfigConflict", err)
	}
	var committed interface{ Committed() bool }
	if errors.As(err, &committed) && committed.Committed() {
		t.Fatalf("callback conflict incorrectly reported as committed: %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(replacement) {
		t.Fatalf("callback replacement overwritten: got %s, want %s", got, replacement)
	}
}

func TestGlobalConfigDetectsNonCooperatingPostWriteReplacement(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	replacement := []byte(`{"schema_version":999,"future_only":true}`)
	globalConfigAfterWriteHook = func(path string) error {
		tmp := path + ".noncooperating"
		if err := os.WriteFile(tmp, replacement, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	}
	defer func() { globalConfigAfterWriteHook = nil }()

	err = SaveGlobalConfig(cfg)
	if !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("post-write replacement error = %v, want ErrGlobalConfigConflict", err)
	}
	var committed interface{ Committed() bool }
	if !errors.As(err, &committed) || !committed.Committed() {
		t.Fatalf("post-write replacement error = %T %v, want committed error", err, err)
	}
	got, readErr := os.ReadFile(filepath.Join(ConfigDir(), "global.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(replacement) {
		t.Fatalf("non-cooperating replacement overwritten: got %s, want %s", got, replacement)
	}
}

func TestGlobalConfigCASRejectsFutureSchemaCreatedAfterMissingLoad(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	staleDefault, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ConfigDir(), "global.json")
	future := []byte(`{"schema_version":999,"future_only":true}`)
	if err := os.WriteFile(path, future, 0o600); err != nil {
		t.Fatalf("create future config: %v", err)
	}

	staleDefault.Theme = "dracula"
	if err := SaveGlobalConfig(staleDefault); !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("SaveGlobalConfig error = %v, want ErrGlobalConfigConflict", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(future) {
		t.Fatalf("future config was overwritten: got %s, want %s", got, future)
	}
}

func TestGlobalConfigCASRejectsFutureSchemaReplacingLoadedFile(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	if err := SaveGlobalConfig(DefaultGlobalConfig()); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ConfigDir(), "global.json")
	future := []byte(`{"schema_version":999,"theme":"future-theme","future_only":true}`)
	if err := os.WriteFile(path, future, 0o600); err != nil {
		t.Fatalf("replace with future config: %v", err)
	}

	loaded.Theme = "dracula"
	if err := SaveGlobalConfig(loaded); !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("SaveGlobalConfig error = %v, want ErrGlobalConfigConflict", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(future) {
		t.Fatalf("future replacement was overwritten: got %s, want %s", got, future)
	}
}

func TestGlobalConfigCASRejectsSameContentReplacement(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	initial := DefaultGlobalConfig()
	if err := SaveGlobalConfig(initial); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ConfigDir(), "global.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	temporary := path + ".replacement"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatal(err)
	}

	loaded.Theme = "dracula"
	if err := SaveGlobalConfig(loaded); !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("same-content replacement error = %v, want ErrGlobalConfigConflict", err)
	}
}

func TestGlobalConfigCASRefreshesRevisionAfterSuccessfulSave(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Theme = "dracula"
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("first save: %v", err)
	}
	cfg.NavStyle = "vim"
	if err := SaveGlobalConfig(cfg); err != nil {
		t.Fatalf("second save with refreshed revision: %v", err)
	}
	loaded, err := LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme != "dracula" || loaded.NavStyle != "vim" {
		t.Fatalf("saved theme/nav = %q/%q, want dracula/vim", loaded.Theme, loaded.NavStyle)
	}
}

func TestGlobalConfigCASRejectsUntrackedStructLiteralOverExistingFile(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	if err := SaveGlobalConfig(DefaultGlobalConfig()); err != nil {
		t.Fatal(err)
	}
	err := SaveGlobalConfig(&GlobalConfig{Theme: "dracula", NavStyle: "vim"})
	if !errors.Is(err, ErrGlobalConfigConflict) {
		t.Fatalf("untracked replacement error = %v, want ErrGlobalConfigConflict", err)
	}
}

func TestIsValidTheme(t *testing.T) {
	tests := []struct {
		theme string
		valid bool
	}{
		{"catppuccin-mocha", true},
		{"dracula", true},
		{"nord", true},
		{"neon-seapunk", true},
		{"invalid-theme", false},
		{"", false},
		{"DRACULA", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			got := IsValidTheme(tt.theme)
			if got != tt.valid {
				t.Errorf("IsValidTheme(%q) = %v, want %v", tt.theme, got, tt.valid)
			}
		})
	}
}

func TestAvailableThemes(t *testing.T) {
	// Ensure we have a reasonable number of themes
	if len(AvailableThemes) < 10 {
		t.Errorf("expected at least 10 themes, got %d", len(AvailableThemes))
	}

	// Ensure no duplicates
	seen := make(map[string]bool)
	for _, theme := range AvailableThemes {
		if seen[theme] {
			t.Errorf("duplicate theme: %s", theme)
		}
		seen[theme] = true
	}
}

// TestToolConfig is a simple config for testing
type TestToolConfig struct {
	Option1 string `json:"option1"`
	Option2 int    `json:"option2"`
}

func DefaultTestToolConfig() *TestToolConfig {
	return &TestToolConfig{
		Option1: "default",
		Option2: 42,
	}
}

func TestToolConfigCRUD(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	// Load when file doesn't exist - should return defaults
	cfg, err := LoadToolConfig("testtool", DefaultTestToolConfig)
	if err != nil {
		t.Fatalf("LoadToolConfig failed: %v", err)
	}
	if cfg.Option1 != "default" {
		t.Errorf("Option1 = %q, want %q", cfg.Option1, "default")
	}
	if cfg.Option2 != 42 {
		t.Errorf("Option2 = %d, want %d", cfg.Option2, 42)
	}

	// Save modified config
	cfg.Option1 = "modified"
	cfg.Option2 = 100

	if err := SaveToolConfig("testtool", cfg); err != nil {
		t.Fatalf("SaveToolConfig failed: %v", err)
	}

	// Reload and verify
	loaded, err := LoadToolConfig("testtool", DefaultTestToolConfig)
	if err != nil {
		t.Fatalf("LoadToolConfig failed: %v", err)
	}
	if loaded.Option1 != "modified" {
		t.Errorf("Option1 = %q, want %q", loaded.Option1, "modified")
	}
	if loaded.Option2 != 100 {
		t.Errorf("Option2 = %d, want %d", loaded.Option2, 100)
	}
}

func TestToolConfigNameRejectsPathTraversal(t *testing.T) {
	_, cleanup := setupTestConfigDir(t)
	defer cleanup()

	cfg := DefaultTestToolConfig()
	for _, name := range []string{"", "../escape", "nested/tool", "Tool"} {
		t.Run(name, func(t *testing.T) {
			if err := SaveToolConfig(name, cfg); err == nil {
				t.Fatalf("SaveToolConfig(%q) unexpectedly succeeded", name)
			}
			if _, err := LoadToolConfig(name, DefaultTestToolConfig); err == nil {
				t.Fatalf("LoadToolConfig(%q) unexpectedly succeeded", name)
			}
		})
	}
}
