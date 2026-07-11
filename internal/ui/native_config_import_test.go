package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestNewAppAdoptsRepresentableNativeGitAndGhosttyValuesBeforeFirstSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	gitPath := filepath.Join(home, ".gitconfig")
	git := "[init]\n\tdefaultBranch = develop\n[pull]\n\trebase = false\n[push]\n\tautoSetupRemote = false\n" +
		"[credential]\n\thelper = osxkeychain\n[commit]\n\tgpgsign = true\n[merge]\n\ttool = nvimdiff\n[diff]\n\texternal = difft\n"
	if err := os.WriteFile(gitPath, []byte(git), 0o600); err != nil {
		t.Fatal(err)
	}
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(ghosttyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	ghostty := "font-family = Berkeley Mono\nfont-size = 17\nbackground-opacity = 0.82\nbackground-blur = 55\n" +
		"cursor-style = underline\nscrollback-limit = 75000000\nwindow-decoration = false\nconfirm-close-surface = false\n"
	if err := os.WriteFile(ghosttyPath, []byte(ghostty), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.GitDefaultBranch != "develop" || got.GitPullRebase || got.GitAutoSetupRemote ||
		got.GitCredentialHelper != "osxkeychain" || !got.GitSignCommits || got.GitMergeTool != "nvimdiff" || got.GitDiffTool != "difftastic" {
		t.Fatalf("native Git values were not adopted: %+v", got)
	}
	if got := app.manageConfig; got.GhosttyFontFamily != "Berkeley Mono" || got.GhosttyFontSize != 17 || got.GhosttyOpacity != 82 ||
		got.GhosttyBlurRadius != 55 || got.GhosstyCursorStyle != "underline" || got.GhosttyScrollbackLines != 75000000 ||
		got.GhosttyWindowDecorations || got.GhosttyConfirmClose {
		t.Fatalf("native Ghostty values were not adopted: %+v", got)
	}
	state := app.NativeConfigState()
	if !state.Applied || state.GitError != "" || state.GhosttyError != "" {
		t.Fatalf("native config state = %+v", state)
	}
	if source := state.Git.Fields[tools.GitFieldDefaultBranch]; source.Path != gitPath || source.Scope != tools.ConfigValueNative {
		t.Fatalf("Git UI provenance = %+v", source)
	}
	if source := state.Ghostty.Fields[tools.GhosttyFieldFontFamily]; source.Path != ghosttyPath || source.Scope != tools.ConfigValueNative {
		t.Fatalf("Ghostty UI provenance = %+v", source)
	}
	if _, err := os.Lstat(filepath.Join(config.ToolsDir(), "manage.json")); !os.IsNotExist(err) {
		t.Fatalf("read-only first-adoption import persisted manage.json: %v", err)
	}
}

func TestNewAppExplicitManagePreferencesOutrankNativeImports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	gitPath := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitPath, []byte("[init]\n\tdefaultBranch = develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(ghosttyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghosttyPath, []byte("font-size = 19\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := NewManageConfig()
	saved.GitDefaultBranch = "master"
	saved.GhosttyFontSize = 13
	if err := config.SaveToolConfig("manage", saved); err != nil {
		t.Fatalf("SaveToolConfig: %v", err)
	}

	app := NewApp(true)
	if app.manageConfig.GitDefaultBranch != "master" || app.manageConfig.GhosttyFontSize != 13 {
		t.Fatalf("native import overwrote explicit manage preferences: %+v", app.manageConfig)
	}
	state := app.NativeConfigState()
	if state.Applied {
		t.Fatal("native import reported adoption over explicit manage preferences")
	}
	if state.Git.Config.DefaultBranch != "develop" || state.Ghostty.Config.FontSize != 19 {
		t.Fatalf("current native state was not exposed alongside saved preferences: %+v", state)
	}
}

func TestNewAppPartialPreferencesOnlyOutrankMatchingNativeFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[init]\n\tdefaultBranch = develop\n[pull]\n\trebase = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	if err := os.MkdirAll(filepath.Dir(ghosttyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghosttyPath, []byte("font-size = 19\nkeybind = ctrl+t=new_tab\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	managePath := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(managePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managePath, []byte(`{"NativeImportSchemaVersion":1,"GitDefaultBranch":"master"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.GitDefaultBranch != "master" || got.GitPullRebase || got.GhosttyFontSize != 19 || got.GhosttyTabBindings != "ctrl" {
		t.Fatalf("partial preference/native merge = %+v", got)
	}
	if app.deepDiveConfig.GitDefaultBranch != "master" || app.deepDiveConfig.GitPullRebase || app.deepDiveConfig.GhosttyFontSize != 19 || app.deepDiveConfig.GhosttyTabBindings != "ctrl" {
		t.Fatalf("wizard did not use hydrated desired model: %+v", app.deepDiveConfig)
	}
}

func TestNewAppLegacyPrototypePreferencesPermitOneTimeNativeAdoption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[init]\n\tdefaultBranch = develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	if err := os.MkdirAll(filepath.Dir(ghosttyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghosttyPath, []byte("font-size = 19\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	managePath := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(managePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managePath, []byte(`{"GitDefaultBranch":"main","GhosttyFontSize":14}`), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if app.manageConfig.GitDefaultBranch != "develop" || app.manageConfig.GhosttyFontSize != 19 {
		t.Fatalf("legacy prototype defaults blocked native adoption: %+v", app.manageConfig)
	}
}

func TestNativePreferenceSchemaV1RetainsGitButPermitsTmuxMigration(t *testing.T) {
	withTempHome(t)
	managePath := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(managePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managePath, []byte(`{"NativeImportSchemaVersion":1,"GitDefaultBranch":"master","TmuxPrefix":"C-b"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	presence := inspectManagePreferencePresence()
	if presence.err != nil {
		t.Fatal(presence.err)
	}
	if !presence.fields["GitDefaultBranch"] {
		t.Fatal("schema v1 lost explicit Git preference")
	}
	if presence.fields["TmuxPrefix"] {
		t.Fatal("schema v1 tmux prototype default was treated as explicit")
	}
}

func TestNativePreferenceSchemaFailsClosedOnFutureOrMalformedVersion(t *testing.T) {
	for _, data := range []string{
		`{"NativeImportSchemaVersion":999}`,
		`{"NativeImportSchemaVersion":"two"}`,
		`{"NativeImportSchemaVersion":0}`,
	} {
		t.Run(data, func(t *testing.T) {
			withTempHome(t)
			managePath := filepath.Join(config.ToolsDir(), "manage.json")
			if err := os.MkdirAll(filepath.Dir(managePath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(managePath, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if presence := inspectManagePreferencePresence(); presence.err == nil {
				t.Fatalf("schema %s was accepted", data)
			}
		})
	}
}

func TestNewAppMalformedPreferenceFileBlocksNativeAdoption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[init]\n\tdefaultBranch = develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	managePath := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(managePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managePath, []byte(`{"GitDefaultBranch":`), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if app.manageConfig.GitDefaultBranch != NewManageConfig().GitDefaultBranch {
		t.Fatalf("malformed preference allowed native adoption: %+v", app.manageConfig)
	}
	if app.NativeConfigState().PreferenceError == "" {
		t.Fatal("malformed preference error was not exposed")
	}
}

func TestNewAppSkipsUnrepresentableNativeValuesButRetainsProvenance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[init]\n\tdefaultBranch = trunk\n[credential]\n\thelper = !custom-command\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ghosttyPath := filepath.Join(home, ".config", "ghostty", "config")
	if err := os.MkdirAll(filepath.Dir(ghosttyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghosttyPath, []byte("font-size = 99\nscrollback-limit = 100\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	defaults := NewManageConfig()
	if app.manageConfig.GitDefaultBranch != defaults.GitDefaultBranch || app.manageConfig.GitCredentialHelper != defaults.GitCredentialHelper ||
		app.manageConfig.GhosttyFontSize != defaults.GhosttyFontSize || app.manageConfig.GhosttyScrollbackLines != defaults.GhosttyScrollbackLines {
		t.Fatalf("unrepresentable native values escaped UI constraints: %+v", app.manageConfig)
	}
	state := app.NativeConfigState()
	if _, ok := state.Git.Fields[tools.GitFieldDefaultBranch]; !ok {
		t.Fatal("unrepresentable Git value lost provenance")
	}
	if _, ok := state.Ghostty.Fields[tools.GhosttyFieldFontSize]; !ok {
		t.Fatal("unrepresentable Ghostty value lost provenance")
	}
}

func TestNewAppNativeImportErrorsFailClosedAndRemainVisible(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	malformed := "# >>> dotfiles git include (managed)\n[include]\n\tpath = ~/.config/other\n# <<< dotfiles git include (managed)\n"
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(malformed), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if app.manageConfig.GitDefaultBranch != NewManageConfig().GitDefaultBranch {
		t.Fatalf("malformed native config changed defaults: %+v", app.manageConfig)
	}
	state := app.NativeConfigState()
	if state.GitError == "" || !strings.Contains(state.GitError, "modified dotfiles Git include") {
		t.Fatalf("Git import error was not exposed: %+v", state)
	}
}

func TestNativeConfigStateReturnsDefensiveCopies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[init]\n\tdefaultBranch = develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	first := app.NativeConfigState()
	delete(first.Git.Fields, tools.GitFieldDefaultBranch)
	first.Git.Sources[0].Path = "mutated"
	second := app.NativeConfigState()
	if _, ok := second.Git.Fields[tools.GitFieldDefaultBranch]; !ok || second.Git.Sources[0].Path == "mutated" {
		t.Fatal("NativeConfigState exposed mutable internal maps or slices")
	}
}
