package ui

import (
	"fmt"
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

func TestNewAppAdoptsRepresentableNativeTmuxValuesBeforeFirstSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	tmux := `set -g prefix C-b
bind | split-window -h
bind - split-window -v
set -g mouse off
set -g base-index 0
setw -g pane-base-index 0
set -g status-position top
setw -g pane-border-lines double
set -g history-limit 12000
set -sg escape-time 25
setw -g aggressive-resize off
`
	path := filepath.Join(home, ".tmux.conf")
	if err := os.WriteFile(path, []byte(tmux), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	got := app.manageConfig
	if got.TmuxPrefix != "C-b" || got.TmuxSplitBinds != "pipes" || got.TmuxMouseMode || got.TmuxBaseIndex != 0 || got.TmuxStatusPosition != "top" || got.TmuxPaneBorderStyle != "double" || got.TmuxHistoryLimit != 12000 || got.TmuxEscapeTime != 25 || got.TmuxAggressiveResize || got.TmuxTPMEnabled || got.TmuxPluginSensible || got.TmuxPluginResurrect || got.TmuxPluginContinuum || got.TmuxPluginYank {
		t.Fatalf("native tmux values were not adopted safely: %+v", got)
	}
	state := app.NativeConfigState()
	if state.TmuxError != "" || !state.Applied {
		t.Fatalf("tmux native state = %+v", state)
	}
	if source := state.Tmux.Fields[tools.TmuxFieldPrefix]; source.Path != path || source.Scope != tools.ConfigValueNative {
		t.Fatalf("tmux UI provenance = %+v", source)
	}
	if badge := app.nativeImportBadge("tmux"); !strings.Contains(badge, "NATIVE SOURCE") {
		t.Fatalf("tmux provenance badge = %q", badge)
	}
}

func TestNewAppHydratesNativeBtopForManageAndStandalone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "btop", "btop.conf")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "color_theme = \"catppuccin-mocha\"\nupdate_ms = 2750\ngraph_symbol = \"block\"\nshown_boxes = \"cpu mem\"\nshow_coretemp = false\ntemp_scale = \"fahrenheit\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.BtopTheme != "auto" || got.BtopUpdateMs != 2750 || got.BtopGraphSymbol != "block" || got.BtopShownBoxes != "cpu mem" || got.BtopShowTemp || got.BtopTempScale != "fahrenheit" {
		t.Fatalf("native btop Manage hydration = %+v", got)
	}
	if got := app.deepDiveConfig; got.BtopTheme != "auto" || got.BtopUpdateMs != 2750 || got.BtopGraphType != "block" || got.BtopShownBoxes != "cpu mem" || got.BtopShowTemp || got.BtopTempScale != "fahrenheit" {
		t.Fatalf("native btop standalone hydration = %+v", got)
	}
	state := app.NativeConfigState()
	if state.BtopError != "" || len(state.Btop.Fields) != 6 || state.Btop.Fields[tools.BtopFieldUpdateMs].Path != path {
		t.Fatalf("btop native state = %+v", state.Btop)
	}
}

func TestNewAppHydratesNativeGlowSevenFieldsForManageAndStandalone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLOW_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path := filepath.Join(home, ".config", "glow", "glow.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "style: /tmp/custom.json\nmouse: true\npager: true\nwidth: 111\nall: true\nshowLineNumbers: true\npreserveNewLines: true\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	mc := app.manageConfig
	if mc.GlowStyle != "/tmp/custom.json" || !mc.GlowMouse || mc.GlowPager != "auto" || mc.GlowWidth != 111 || !mc.GlowAll || !mc.GlowShowLineNumbers || !mc.GlowPreserveNewLines {
		t.Fatalf("Manage Glow hydration=%+v", mc)
	}
	dd := app.deepDiveConfig
	if dd.GlowStyle != "/tmp/custom.json" || !dd.GlowMouse || dd.GlowPager != "auto" || dd.GlowWidth != 111 || !dd.GlowAll || !dd.GlowShowLineNumbers || !dd.GlowPreserveNewLines {
		t.Fatalf("DeepDive Glow hydration=%+v", dd)
	}
	state := app.NativeConfigState()
	if state.GlowError != "" || len(state.Glow.Fields) != 7 || state.Glow.Fields[tools.GlowFieldStyle].Path != path {
		t.Fatalf("Glow state=%+v err=%s", state.Glow, state.GlowError)
	}
	ctx := NewTestScreenContext()
	ctx.app = app
	if view := NewConfigGlowScreen(ctx).View(80, 24); !strings.Contains(view, "/tmp/custom.json") || !strings.Contains(view, "111 chars") || !strings.Contains(view, "Enabled") {
		t.Fatalf("hydrated Glow view:\n%s", view)
	}
}

func TestNewAppHydratesNativeLazyGitFourFieldsForManageAndInstaller(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	path := filepath.Join(home, ".config", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := `gui:
  sidePanelWidth: 0.42
  mouseEvents: false
  theme:
    activeBorderColor: [blue, bold]
    inactiveBorderColor: [default]
    selectedLineBgColor: [reverse]
    defaultFgColor: [black]
git:
  pagers:
    - colorArg: always
      pager: delta --dark --paging=never
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.LazyGitSidePanelWidth != "0.42" || got.LazyGitMouseEvents || got.LazyGitColorPreset != "light-high-contrast" || got.LazyGitPagerPreset != "delta" {
		t.Fatalf("Manage LazyGit hydration = %+v", got)
	}
	if got := app.deepDiveConfig; got.LazyGitSidePanelWidth != "0.42" || got.LazyGitMouseEvents || got.LazyGitColorPreset != "light-high-contrast" || got.LazyGitPagerPreset != "delta" {
		t.Fatalf("installer LazyGit hydration = %+v", got)
	}
	state := app.NativeConfigState()
	if len(state.LazyGit.Fields) != 4 || state.LazyGit.Fields[tools.LazyGitFieldSidePanelWidth].Path != path || !state.LazyGit.RepoOverridesPossible {
		t.Fatalf("LazyGit native state = %+v", state.LazyGit)
	}
	if state.LazyGit.ReadOnlyReason == "" || !strings.Contains(state.LazyGit.ReadOnlyReason, "arbitrary native") {
		t.Fatalf("arbitrary native source was not disclosed read-only: %+v", state.LazyGit)
	}
}

func TestNewAppHydratesLazyGitConfigFileChainReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	one, two := filepath.Join(home, "one.yml"), filepath.Join(home, "two.yml")
	if err := os.WriteFile(one, []byte("gui:\n  sidePanelWidth: 0.4\n  mouseEvents: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(two, []byte("git:\n  pagers: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LG_CONFIG_FILE", one+","+two)

	app := NewApp(true)
	if got := app.manageConfig; got.LazyGitSidePanelWidth != "0.4" || got.LazyGitMouseEvents || got.LazyGitPagerPreset != "builtin" {
		t.Fatalf("LG_CONFIG_FILE hydration = %+v", got)
	}
	state := app.NativeConfigState().LazyGit
	if len(state.Sources) != 2 || !state.Sources[0].Active || !state.Sources[1].Active || !strings.Contains(state.ReadOnlyReason, "LG_CONFIG_FILE") {
		t.Fatalf("LG_CONFIG_FILE state = %+v", state)
	}
}

func TestNewAppDisplaysCustomLazyGitValuesReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	path := filepath.Join(home, ".config", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := `gui:
  theme:
    activeBorderColor: [red, bold]
    inactiveBorderColor: [yellow]
git:
  pagers:
    - pager: bat --paging=never
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.LazyGitColorPreset != "custom" || got.LazyGitPagerPreset != "custom" {
		t.Fatalf("custom native values were hidden: %+v", got)
	}
	if got := app.deepDiveConfig; got.LazyGitColorPreset != "custom" || got.LazyGitPagerPreset != "custom" {
		t.Fatalf("custom installer values were hidden: %+v", got)
	}
	state := app.NativeConfigState().LazyGit
	if !strings.Contains(state.ReadOnlyReason, "custom LazyGit colors") || !strings.Contains(state.ReadOnlyReason, "custom LazyGit pagers") {
		t.Fatalf("custom read-only disclosure = %q", state.ReadOnlyReason)
	}
}

func TestLazyGitSchemaFourMigratesUnambiguousPrototypePreferencesWithoutNativeSource(t *testing.T) {
	for _, tc := range []struct {
		name, theme, paging, wantColor, wantPager string
	}{
		{"dark and delta", "dark", "delta", "standard", "delta"},
		{"light and disabled", "light", "never", "light-high-contrast", "builtin"},
		{"custom prototype pager", "auto", "diff-so-fancy", "standard", "custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("LG_CONFIG_FILE", "")
			path := filepath.Join(config.ToolsDir(), "manage.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			raw := []byte(fmt.Sprintf(`{"NativeImportSchemaVersion":4,"LazyGitTheme":%q,"LazyGitPaging":%q,"LazyGitSideBySide":false}`, tc.theme, tc.paging))
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			app := NewApp(true)
			if got := app.manageConfig; got.LazyGitColorPreset != tc.wantColor || got.LazyGitPagerPreset != tc.wantPager || got.LazyGitSidePanelWidth != "0.3333" {
				t.Fatalf("migration = %+v", got)
			}
		})
	}
}

func TestLazyGitSchemaFiveExplicitPreferencesWinForManagedWritableSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	native := filepath.Join(home, ".config", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(native), 0o700); err != nil {
		t.Fatal(err)
	}
	generated := tools.GenerateLazyGitConfig(tools.LazyGitConfig{SidePanelWidth: "0.4", MouseEvents: false, ColorPreset: "standard", PagerPreset: "builtin"}, "ignored")
	if err := os.WriteFile(native, []byte(generated), 0o600); err != nil {
		t.Fatal(err)
	}
	manage := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(manage), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manage, []byte(`{"NativeImportSchemaVersion":5,"LazyGitSidePanelWidth":"0.75","LazyGitMouseEvents":true,"LazyGitColorPreset":"light-high-contrast","LazyGitPagerPreset":"delta"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.LazyGitSidePanelWidth != "0.75" || !got.LazyGitMouseEvents || got.LazyGitColorPreset != "light-high-contrast" || got.LazyGitPagerPreset != "delta" {
		t.Fatalf("schema five explicit preferences = %+v", got)
	}
	state := app.NativeConfigState().LazyGit
	if !state.Managed || state.ReadOnlyReason != "" || !state.RepoOverridesPossible {
		t.Fatalf("managed source state = %+v", state)
	}
}

func TestLazyGitSchemaFiveExplicitPreferencesWinWhenNativeMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	manage := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(manage), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manage, []byte(`{"NativeImportSchemaVersion":5,"LazyGitSidePanelWidth":"0.75","LazyGitMouseEvents":false,"LazyGitColorPreset":"light-high-contrast","LazyGitPagerPreset":"delta"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.LazyGitSidePanelWidth != "0.75" || got.LazyGitMouseEvents || got.LazyGitColorPreset != "light-high-contrast" || got.LazyGitPagerPreset != "delta" {
		t.Fatalf("missing native source replaced explicit preferences: %+v", got)
	}
}

func TestLazyGitSchemaFiveArbitraryNativeHydrationOutranksSavedPreferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	native := filepath.Join(home, ".config", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(native), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("gui:\n  sidePanelWidth: 0.42\n  mouseEvents: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manage := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(manage), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manage, []byte(`{"NativeImportSchemaVersion":5,"LazyGitSidePanelWidth":"0.75","LazyGitMouseEvents":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	if got := app.manageConfig; got.LazyGitSidePanelWidth != "0.42" || got.LazyGitMouseEvents {
		t.Fatalf("arbitrary native source was masked by stale saved preferences: %+v", got)
	}
	if app.NativeConfigState().LazyGit.ReadOnlyReason == "" {
		t.Fatal("arbitrary native source was not disclosed read-only")
	}
}

func TestMalformedLazyGitYAMLIsVisibleAndDoesNotOverlayDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	path := filepath.Join(home, ".config", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("gui:\n  sidePanelWidth: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewApp(true)
	defaults := NewManageConfig()
	if got := app.manageConfig; got.LazyGitSidePanelWidth != defaults.LazyGitSidePanelWidth || got.LazyGitMouseEvents != defaults.LazyGitMouseEvents || got.LazyGitColorPreset != defaults.LazyGitColorPreset || got.LazyGitPagerPreset != defaults.LazyGitPagerPreset {
		t.Fatalf("malformed LazyGit YAML changed desired state: %+v", got)
	}
	if app.NativeConfigState().LazyGitError == "" {
		t.Fatal("malformed LazyGit YAML error was not exposed")
	}
}

func TestGlowSchemaFourExplicitWinsAndLegacyPagerMigratesWithoutNativeSource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		schema      int
		pager, want string
	}{{"schema four explicit", 4, "less", "less"}, {"schema three less canonicalizes", 3, "less", "auto"}, {"schema three none canonicalizes", 3, "none", "never"}} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("GLOW_CONFIG_HOME", "")
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			path := filepath.Join(config.ToolsDir(), "manage.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(fmt.Sprintf(`{"NativeImportSchemaVersion":%d,"GlowPager":%q}`, tc.schema, tc.pager)), 0o600); err != nil {
				t.Fatal(err)
			}
			app := NewApp(true)
			if app.manageConfig.GlowPager != tc.want {
				t.Fatalf("pager=%q want %q", app.manageConfig.GlowPager, tc.want)
			}
		})
	}
}

func TestGlowSchemaZeroDefaultsDoNotBlockNativeHydration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GLOW_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	native := filepath.Join(home, ".config", "glow", "glow.yml")
	if err := os.MkdirAll(filepath.Dir(native), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("style: light\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manage := filepath.Join(config.ToolsDir(), "manage.json")
	if err := os.MkdirAll(filepath.Dir(manage), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manage, []byte(`{"GlowStyle":"dark","GlowPager":"less"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	if app.manageConfig.GlowStyle != "light" || app.manageConfig.GlowPager != "never" || app.manageConfig.GlowAll != true || app.manageConfig.GlowWidth != 0 {
		t.Fatalf("schema zero native hydration=%+v", app.manageConfig)
	}
}

func TestBtopSchemaThreeExplicitPreferenceWinsAndSchemaTwoMigrates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	path := filepath.Join(home, ".config", "btop", "btop.conf")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("color_theme = \"catppuccin-mocha\"\nupdate_ms = 2750\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		schema int
		want   int
	}{{"schema two migrates", 2, 2750}, {"schema three explicit wins", 3, 5000}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			managePath := filepath.Join(config.ToolsDir(), "manage.json")
			if err := os.MkdirAll(filepath.Dir(managePath), 0o700); err != nil {
				t.Fatal(err)
			}
			raw := []byte(fmt.Sprintf(`{"NativeImportSchemaVersion":%d,"BtopUpdateMs":5000}`, tc.schema))
			if err := os.WriteFile(managePath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			app := NewApp(true)
			if app.manageConfig.BtopUpdateMs != tc.want {
				t.Fatalf("BtopUpdateMs = %d, want %d", app.manageConfig.BtopUpdateMs, tc.want)
			}
		})
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

func TestNewAppTmuxIndirectionFailsClosedAndRemainsVisible(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.WriteFile(filepath.Join(home, ".tmux.conf"), []byte("set -g prefix C-b\nsource-file ~/.tmux-extra.conf\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	if app.manageConfig.TmuxPrefix != NewManageConfig().TmuxPrefix {
		t.Fatalf("ambiguous tmux import changed defaults: %+v", app.manageConfig)
	}
	state := app.NativeConfigState()
	if state.TmuxError == "" || !strings.Contains(state.TmuxError, "indirection") {
		t.Fatalf("tmux import error was not exposed: %+v", state)
	}
	if badge := app.nativeImportBadge("tmux"); !strings.Contains(badge, "IMPORT BLOCKED") {
		t.Fatalf("blocked tmux badge = %q", badge)
	}
	if errs := applyOneToolConfig("tmux", manageConfigToDeepDive(app.manageConfig), app.theme); len(errs) == 0 {
		t.Fatal("standalone tmux apply ignored ambiguous native import")
	}
}

func TestNativeConfigStateReturnsDefensiveCopies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CONFIG_DIR", filepath.Join(home, ".config", "lazygit"))
	t.Setenv("LG_CONFIG_FILE", "")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[init]\n\tdefaultBranch = develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lazyGitPath := filepath.Join(home, ".config", "lazygit", "config.yml")
	if err := os.MkdirAll(filepath.Dir(lazyGitPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lazyGitPath, []byte("gui:\n  sidePanelWidth: 0.42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	first := app.NativeConfigState()
	delete(first.Git.Fields, tools.GitFieldDefaultBranch)
	first.Git.Sources[0].Path = "mutated"
	delete(first.LazyGit.Fields, tools.LazyGitFieldSidePanelWidth)
	first.LazyGit.Sources[0].Path = "mutated"
	first.LazyGit.Warnings = append(first.LazyGit.Warnings, "mutated")
	second := app.NativeConfigState()
	if _, ok := second.Git.Fields[tools.GitFieldDefaultBranch]; !ok || second.Git.Sources[0].Path == "mutated" {
		t.Fatal("NativeConfigState exposed mutable internal maps or slices")
	}
	if _, ok := second.LazyGit.Fields[tools.LazyGitFieldSidePanelWidth]; !ok || second.LazyGit.Sources[0].Path == "mutated" || len(second.LazyGit.Warnings) != 0 || !second.LazyGit.RepoOverridesPossible {
		t.Fatal("NativeConfigState exposed mutable LazyGit state")
	}
}
