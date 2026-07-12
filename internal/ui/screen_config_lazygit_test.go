package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/tools"
)

type lazyGitUIBlockCase struct {
	name, visible string
	apply         func(*App)
}

func lazyGitUIBlockCases() []lazyGitUIBlockCase {
	return []lazyGitUIBlockCase{
		{"arbitrary native", "arbitrary native YAML is read-only", func(a *App) { a.nativeConfigState.LazyGit.ReadOnlyReason = "arbitrary native YAML is read-only" }},
		{"LG chain", "LG_CONFIG_FILE chain is read-only", func(a *App) { a.nativeConfigState.LazyGit.ReadOnlyReason = "LG_CONFIG_FILE chain is read-only" }},
		{"legacy fallback", "legacy jesseduffield fallback is read-only", func(a *App) {
			a.nativeConfigState.LazyGit.ReadOnlyReason = "legacy jesseduffield fallback is read-only"
		}},
		{"read error", "native LazyGit configuration could not be imported safely: permission denied", func(a *App) { a.nativeConfigState.LazyGitError = "permission denied" }},
		{"preference error", "saved management preferences could not be read safely: malformed manage.json", func(a *App) { a.nativeConfigState.PreferenceError = "malformed manage.json" }},
		{"custom without native", "custom LazyGit colors are read-only in this release", func(a *App) {
			a.manageConfig.LazyGitColorPreset = "custom"
			a.deepDiveConfig.LazyGitColorPreset = "custom"
		}},
		{"corrupt preset without native", "custom LazyGit pagers are read-only in this release", func(a *App) {
			a.manageConfig.LazyGitPagerPreset = "corrupt-token"
			a.deepDiveConfig.LazyGitPagerPreset = "corrupt-token"
		}},
	}
}

func normalizedLazyGitView(out string) string {
	// Lipgloss may wrap immediately after a hyphen. Rejoin that visual soft wrap
	// while preserving every causal word for exact disclosure assertions.
	return strings.ReplaceAll(strings.Join(strings.Fields(stripANSITest(out)), " "), "- ", "-")
}

func assertLazyGitDeepDiveFields(t *testing.T, cfg *DeepDiveConfig, width string, mouse bool, color, pager string) {
	t.Helper()
	if cfg.LazyGitSidePanelWidth != width || cfg.LazyGitMouseEvents != mouse || cfg.LazyGitColorPreset != color || cfg.LazyGitPagerPreset != pager {
		t.Fatalf("LazyGit fields mutated: width=%q mouse=%v color=%q pager=%q", cfg.LazyGitSidePanelWidth, cfg.LazyGitMouseEvents, cfg.LazyGitColorPreset, cfg.LazyGitPagerPreset)
	}
}

func assertLazyGitManageFields(t *testing.T, cfg *ManageConfig, width string, mouse bool, color, pager string) {
	t.Helper()
	if cfg.LazyGitSidePanelWidth != width || cfg.LazyGitMouseEvents != mouse || cfg.LazyGitColorPreset != color || cfg.LazyGitPagerPreset != pager {
		t.Fatalf("LazyGit Manage fields mutated: width=%q mouse=%v color=%q pager=%q", cfg.LazyGitSidePanelWidth, cfg.LazyGitMouseEvents, cfg.LazyGitColorPreset, cfg.LazyGitPagerPreset)
	}
}

func TestLazyGitManageFractionEditorValidatesBeforeMutation(t *testing.T) {
	app := appForFields()
	field := app.manageFieldsFor("lazygit")[0]
	if field.validateText == nil {
		t.Fatal("LazyGit fraction field has no exact validator")
	}
	before := app.manageConfig.LazyGitSidePanelWidth
	app.manageStartEditing(field)
	app.manageEditValue = "1.01"
	if app.manageCommitEditing() || !app.manageEditing || app.manageConfig.LazyGitSidePanelWidth != before {
		t.Fatalf("invalid fraction escaped editor: editing=%v value=%q", app.manageEditing, app.manageConfig.LazyGitSidePanelWidth)
	}
	if !strings.Contains(app.manageStatus, "between 0 and 1") {
		t.Fatalf("validation feedback=%q", app.manageStatus)
	}
	app.manageCancelEditing()
	app.manageStartEditing(field)
	app.manageEditValue = "0.42"
	if !app.manageCommitEditing() || app.manageConfig.LazyGitSidePanelWidth != "0.42" {
		t.Fatalf("valid exact fraction was not committed: %q", app.manageConfig.LazyGitSidePanelWidth)
	}
}

func TestConfigLazyGitExactFractionEditorCommitCancelAndInputCapture(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.deepDiveConfig.LazyGitSidePanelWidth = "0.3333"
	screen := NewConfigLazyGitScreen(ctx)
	if _, cmd := screen.Update(keyMsg("e")); cmd != nil || !screen.editing {
		t.Fatal("e did not enter exact fraction editor")
	}
	original := screen.editValue
	screen.Update(keyMsg("q"))
	screen.Update(keyMsg("h"))
	screen.Update(clickAt(30, 10))
	if !screen.editing || screen.editValue != original || ctx.app.configFieldIndex != 0 {
		t.Fatalf("hotkey/mouse escaped editor: editing=%v value=%q field=%d", screen.editing, screen.editValue, ctx.app.configFieldIndex)
	}
	screen.editValue = "2"
	screen.Update(keyMsg("enter"))
	if !screen.editing || ctx.app.deepDiveConfig.LazyGitSidePanelWidth != "0.3333" || screen.editError == "" {
		t.Fatal("invalid editor value mutated config or closed editor")
	}
	screen.Update(keyMsg("esc"))
	if screen.editing || ctx.app.deepDiveConfig.LazyGitSidePanelWidth != "0.3333" {
		t.Fatal("Esc did not cancel without mutation")
	}
	screen.Update(keyMsg("e"))
	screen.editValue = ".5"
	screen.Update(keyMsg("enter"))
	if screen.editing || ctx.app.deepDiveConfig.LazyGitSidePanelWidth != ".5" {
		t.Fatalf("valid exact value not committed: %q", ctx.app.deepDiveConfig.LazyGitSidePanelWidth)
	}
	if _, cmd := screen.Update(keyMsg("enter")); cmd == nil {
		t.Fatal("Enter outside edit no longer follows normal back/preview semantics")
	}
}

func TestConfigLazyGitCustomDisclosureDeltaAndLayoutAt80x24(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.width, ctx.app.height = 80, 24
	ctx.Width, ctx.Height = 80, 24
	ctx.app.deepDiveConfig.LazyGitColorPreset = "custom"
	ctx.app.deepDiveConfig.LazyGitPagerPreset = "custom"
	ctx.app.nativeConfigState.LazyGit.ReadOnlyReason = "native YAML is read-only"
	ctx.app.nativeConfigState.LazyGit.RepoOverridesPossible = true
	screen := NewConfigLazyGitScreen(ctx)
	out := screen.View(80, 24)
	if got := lipgloss.Height(out); got > 24 {
		t.Fatalf("80x24 view height=%d\n%s", got, out)
	}
	for _, want := range []string{"Side Panel Fraction", "Mouse Events", "Color Preset", "Pager Preset", "Custom (read-only)", "native YAML is read-only", "Repository config may override"} {
		if !strings.Contains(out, want) {
			t.Errorf("compact view missing %q:\n%s", want, out)
		}
	}
	ctx.app.configFieldIndex = 2
	screen.Update(keyMsg("right"))
	if ctx.app.deepDiveConfig.LazyGitColorPreset != "custom" {
		t.Fatal("custom read-only color was changed")
	}
	ctx.app.configFieldIndex = 3
	screen.Update(keyMsg("right"))
	if ctx.app.deepDiveConfig.LazyGitPagerPreset != "custom" {
		t.Fatal("custom read-only pager was changed")
	}
}

func TestConfigLazyGitDeltaFeedbackStates(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.deepDiveConfig.LazyGitPagerPreset = "delta"
	screen := NewConfigLazyGitScreen(ctx)
	ctx.app.manageInstalledReady = false
	if out := screen.View(80, 24); !strings.Contains(out, "not ready") {
		t.Fatalf("loading feedback missing:\n%s", out)
	}
	ctx.app.manageInstalledReady = true
	ctx.app.manageInstalled["delta"] = false
	if out := screen.View(80, 24); !strings.Contains(out, "install the Delta") {
		t.Fatalf("missing dependency feedback absent:\n%s", out)
	}
	ctx.app.manageInstalled["delta"] = true
	if out := screen.View(80, 24); !strings.Contains(out, "Delta installed") || !strings.Contains(out, "--dark") {
		t.Fatalf("installed dependency feedback absent:\n%s", out)
	}
}

func TestConfigLazyGitMouseGeometryIncludesDisclosureRows(t *testing.T) {
	ctx := newDeepDiveContext(t)
	ctx.app.width, ctx.app.height = 80, 24
	ctx.Width, ctx.Height = 80, 24
	ctx.app.nativeConfigState.LazyGit.ReadOnlyReason = "native YAML is read-only"
	ctx.app.nativeConfigState.LazyGit.RepoOverridesPossible = true
	screen := NewConfigLazyGitScreen(ctx)
	out := screen.View(80, 24)
	for idx, label := range []string{"Side Panel Fraction", "Mouse Events", "Color Preset", "Pager Preset"} {
		y := labelLineY(t, out, label)
		if y < 0 {
			t.Fatalf("missing label %q", label)
		}
		ctx.app.configFieldIndex = 99
		screen.Update(clickAt(30, y))
		if ctx.app.configFieldIndex != idx {
			t.Errorf("click %q selected %d, want %d", label, ctx.app.configFieldIndex, idx)
		}
	}
}

func TestManageLazyGitCustomAndNativeDisclosuresAreReadOnly(t *testing.T) {
	ctx := newManageContext(t)
	items := ctx.app.manageItems()
	for i, item := range items {
		if item.id == "lazygit" {
			ctx.app.manageIndex = i
			break
		}
	}
	ctx.app.managePane = managePaneSettings
	ctx.app.configFieldIndex = 2
	ctx.app.manageConfig.LazyGitColorPreset = "custom"
	ctx.app.nativeConfigState.LazyGit.ReadOnlyReason = "native YAML is read-only"
	ctx.app.nativeConfigState.LazyGit.RepoOverridesPossible = true
	screen := NewManageScreen(ctx)
	out := screen.View(ctx.Width, ctx.Height)
	for _, want := range []string{"Custom (read-only)", "native YAML is read-only", "REPO OVERRIDES"} {
		if !strings.Contains(out, want) {
			t.Errorf("Manage disclosure missing %q:\n%s", want, out)
		}
	}
	screen.Update(keyMsg("right"))
	if ctx.app.manageConfig.LazyGitColorPreset != "custom" || !strings.Contains(ctx.app.manageStatus, "read-only") {
		t.Fatalf("keyboard changed custom value: %q status=%q", ctx.app.manageConfig.LazyGitColorPreset, ctx.app.manageStatus)
	}
	ctx.app.manageStatus = ""
	_ = screen.View(ctx.Width, ctx.Height)
	layout := ctx.app.manageLayout()
	screen.Update(clickAt(layout.rightX+layout.rightW-2, layout.rightListY+2))
	if ctx.app.manageConfig.LazyGitColorPreset != "custom" || !strings.Contains(ctx.app.manageStatus, "read-only") {
		t.Fatalf("mouse changed custom value: %q status=%q", ctx.app.manageConfig.LazyGitColorPreset, ctx.app.manageStatus)
	}
}

func TestManageLazyGitDeltaFeedbackStates(t *testing.T) {
	ctx := newManageContext(t)
	items := ctx.app.manageItems()
	for i, item := range items {
		if item.id == "lazygit" {
			ctx.app.manageIndex = i
			break
		}
	}
	ctx.app.managePane = managePaneSettings
	ctx.app.manageConfig.LazyGitPagerPreset = "delta"
	ctx.app.manageInstalled["lazygit"] = true
	ctx.app.nativeConfigState.LazyGit.ReadOnlyReason = ""
	screen := NewManageScreen(ctx)
	ctx.app.manageInstalledReady = false
	if out := screen.View(ctx.Width, ctx.Height); !strings.Contains(out, "not ready") {
		t.Fatalf("Manage loading feedback missing:\n%s", out)
	}
	ctx.app.manageInstalledReady = true
	ctx.app.manageInstalled["delta"] = false
	if out := screen.View(ctx.Width, ctx.Height); !strings.Contains(out, "install the Delta") {
		t.Fatalf("Manage missing feedback absent:\n%s", out)
	}
	ctx.app.manageInstalled["delta"] = true
	if out := screen.View(ctx.Width, ctx.Height); !strings.Contains(out, "Delta installed") || !strings.Contains(out, "--dark") {
		t.Fatalf("Manage installed feedback absent:\n%s", out)
	}
}

func TestLazyGitInvalidFractionFailsAllPlanPreflights(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", home+"/.config/lazygit")
	t.Setenv("LG_CONFIG_FILE", "")
	cfg := *NewDeepDiveConfig()
	cfg.LazyGitSidePanelWidth = "1.01"
	cfg.LazyGitColorPreset = "standard"
	cfg.LazyGitPagerPreset = "builtin"
	standalone, reason, err := standaloneConfigPlanSpec(home, "theme", cfg, "lazygit", false)
	if err != nil || reason != "" || !strings.Contains(standalone.preflightBlockReason, "between 0 and 1") {
		t.Fatalf("standalone invalid-fraction preflight=%q reason=%q err=%v", standalone.preflightBlockReason, reason, err)
	}
	cfg.CLITools["lazygit"] = true
	specs, err := installerConfigSpecs(home, "theme", cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	blocked := ""
	for _, spec := range specs {
		if spec.toolID == "lazygit" {
			blocked = spec.preflightBlockReason
		}
	}
	if !strings.Contains(blocked, "between 0 and 1") {
		t.Fatalf("installer invalid-fraction preflight=%q", blocked)
	}
	cfg.LazyGitSidePanelWidth = "0.42"
	cfg.LazyGitPagerPreset = "corrupt-token"
	standalone, reason, err = standaloneConfigPlanSpec(home, "theme", cfg, "lazygit", false)
	if err != nil || reason != "" || !strings.Contains(standalone.preflightBlockReason, "custom LazyGit pagers") {
		t.Fatalf("standalone invalid-preset preflight=%q reason=%q err=%v", standalone.preflightBlockReason, reason, err)
	}
	specs, err = installerConfigSpecs(home, "theme", cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	blocked = ""
	for _, spec := range specs {
		if spec.toolID == "lazygit" {
			blocked = spec.preflightBlockReason
		}
	}
	if !strings.Contains(blocked, "custom LazyGit pagers") {
		t.Fatalf("installer invalid-preset preflight=%q", blocked)
	}
	if err := tools.ValidateLazyGitSidePanelWidth("0.42"); err != nil {
		t.Fatalf("single-field validator rejected valid fraction: %v", err)
	}
}

func TestConfigLazyGitWholeSourceBlocksEveryMutationAndShowsExactCause(t *testing.T) {
	for _, tc := range lazyGitUIBlockCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = 80, 24
			ctx.Width, ctx.Height = 80, 24
			ctx.app.configStandalone = true
			ctx.app.startScreen = ScreenConfigLazyGit
			cfg := ctx.app.deepDiveConfig
			cfg.LazyGitSidePanelWidth = "0.42"
			cfg.LazyGitMouseEvents = true
			cfg.LazyGitColorPreset = "standard"
			cfg.LazyGitPagerPreset = "builtin"
			tc.apply(ctx.app)
			wantWidth, wantMouse, wantColor, wantPager := cfg.LazyGitSidePanelWidth, cfg.LazyGitMouseEvents, cfg.LazyGitColorPreset, cfg.LazyGitPagerPreset
			screen := NewConfigLazyGitScreen(ctx)
			out := screen.View(80, 24)
			visible := normalizedLazyGitView(out)
			if !strings.Contains(visible, tc.visible) || !strings.Contains(visible, "read-only") {
				t.Fatalf("causal read-only text missing %q:\n%s", tc.visible, out)
			}
			for field := 0; field < 4; field++ {
				ctx.app.configFieldIndex = field
				for _, key := range []string{"e", "left", "right", "h", "l", " "} {
					screen.Update(keyMsg(key))
					assertLazyGitDeepDiveFields(t, cfg, wantWidth, wantMouse, wantColor, wantPager)
					if screen.editing {
						t.Fatalf("blocked field %d entered editor with %q", field, key)
					}
				}
			}
			out = screen.View(80, 24)
			for _, label := range []string{"Side Panel Fraction", "Mouse Events", "Color Preset", "Pager Preset"} {
				y := labelLineY(t, out, label)
				if y < 0 {
					t.Fatalf("missing %q", label)
				}
				screen.Update(clickAt(30, y))
				assertLazyGitDeepDiveFields(t, cfg, wantWidth, wantMouse, wantColor, wantPager)
			}
			if _, cmd := screen.Update(keyMsg("enter")); cmd == nil {
				t.Fatal("blocked standalone Enter did not open reviewable preview")
			}
		})
	}
}

func TestManageLazyGitWholeSourceBlocksEveryMutationWithoutDirtyPreferences(t *testing.T) {
	for _, tc := range lazyGitUIBlockCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newManageContext(t)
			items := ctx.app.manageItems()
			for i, item := range items {
				if item.id == "lazygit" {
					ctx.app.manageIndex = i
					break
				}
			}
			ctx.app.managePane = managePaneSettings
			cfg := ctx.app.manageConfig
			cfg.LazyGitSidePanelWidth = "0.42"
			cfg.LazyGitMouseEvents = true
			cfg.LazyGitColorPreset = "standard"
			cfg.LazyGitPagerPreset = "builtin"
			tc.apply(ctx.app)
			wantWidth, wantMouse, wantColor, wantPager := cfg.LazyGitSidePanelWidth, cfg.LazyGitMouseEvents, cfg.LazyGitColorPreset, cfg.LazyGitPagerPreset
			ctx.app.manageConfigBaseline = *cfg
			ctx.app.manageConfigBaselineTheme = ctx.app.theme
			screen := NewManageScreen(ctx)
			out := screen.View(ctx.Width, ctx.Height)
			visible := normalizedLazyGitView(out)
			if !strings.Contains(visible, tc.visible) || !strings.Contains(visible, "read-only") {
				t.Fatalf("Manage cause missing %q:\n%s", tc.visible, out)
			}
			for field := 0; field < 4; field++ {
				ctx.app.configFieldIndex = field
				for _, key := range []string{"left", "right", "h", "l", " ", "enter"} {
					screen.Update(keyMsg(key))
					assertLazyGitManageFields(t, cfg, wantWidth, wantMouse, wantColor, wantPager)
					if ctx.app.manageEditing {
						t.Fatalf("blocked field %d entered editor with %q", field, key)
					}
				}
			}
			_ = screen.View(ctx.Width, ctx.Height)
			layout := ctx.app.manageLayout()
			for field := 0; field < 4; field++ {
				screen.Update(clickAt(layout.rightX+layout.rightW-2, layout.rightListY+field))
				assertLazyGitManageFields(t, cfg, wantWidth, wantMouse, wantColor, wantPager)
			}
			if changed := changedManageTools(&ctx.app.manageConfigBaseline, cfg, ctx.app.manageConfigBaselineTheme, ctx.app.theme); len(changed) != 0 {
				t.Fatalf("blocked interaction dirtied Manage scope: %v", changed)
			}
			if _, err := os.Lstat(config.ToolsDir() + "/manage.json"); !os.IsNotExist(err) {
				t.Fatalf("blocked interaction persisted manage.json: %v", err)
			}
		})
	}
}

func TestManageLazyGitNarrowRowsAndMouseGeometryWritableAndReadOnly(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		name := "writable"
		if readOnly {
			name = "read-only"
		}
		t.Run(name, func(t *testing.T) {
			ctx := newManageContext(t)
			ctx.Width, ctx.Height = 80, 24
			ctx.app.width, ctx.app.height = 80, 24
			for i, item := range ctx.app.manageItems() {
				if item.id == "lazygit" {
					ctx.app.manageIndex = i
					break
				}
			}
			ctx.app.managePane = managePaneSettings
			ctx.app.manageConfig.LazyGitSidePanelWidth = "0.42"
			ctx.app.manageConfig.LazyGitMouseEvents = true
			ctx.app.manageConfig.LazyGitColorPreset = "standard"
			ctx.app.manageConfig.LazyGitPagerPreset = "builtin"
			if readOnly {
				ctx.app.nativeConfigState.LazyGit.ReadOnlyReason = "arbitrary native YAML is read-only"
			}
			fields := ctx.app.manageFieldsFor("lazygit")
			for i, field := range fields {
				if got := lipgloss.Height(renderManageFieldLineBase(field, i == 0)); got != 1 {
					t.Fatalf("field %d renders %d rows", i, got)
				}
			}
			screen := NewManageScreen(ctx)
			out := screen.View(80, 24)
			layout := ctx.app.manageLayout()
			labels := []string{"Panel Fraction", "Mouse Events", "Color Preset", "Pager Preset"}
			for idx, label := range labels {
				y := labelLineY(t, out, label)
				if y < 0 {
					t.Fatalf("missing %q in narrow Manage view:\n%s", label, out)
				}
				screen.Update(clickAt(layout.rightX+layout.rightW-2, y))
				if ctx.app.configFieldIndex != idx {
					t.Fatalf("click %q selected %d, want %d", label, ctx.app.configFieldIndex, idx)
				}
				if readOnly {
					assertLazyGitManageFields(t, ctx.app.manageConfig, "0.42", true, "standard", "builtin")
				}
			}
			if !readOnly {
				if ctx.app.manageConfig.LazyGitSidePanelWidth != "0.42" || ctx.app.manageConfig.LazyGitMouseEvents || ctx.app.manageConfig.LazyGitColorPreset != "light-high-contrast" || ctx.app.manageConfig.LazyGitPagerPreset != "delta" {
					t.Fatalf("writable mouse mapping changed wrong fields: %+v", ctx.app.manageConfig)
				}
			}
		})
	}
}

func TestLazyGitSchemaFourDiffSoFancyMigrationBlocksBothEditorSurfaces(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONFIG_DIR", home+"/.config/lazygit")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LG_CONFIG_FILE", "")
	path := config.ToolsDir() + "/manage.json"
	if err := os.MkdirAll(config.ToolsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"NativeImportSchemaVersion":4,"LazyGitTheme":"auto","LazyGitPaging":"diff-so-fancy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	if app.manageConfig.LazyGitPagerPreset != "custom" || app.deepDiveConfig.LazyGitPagerPreset != "custom" {
		t.Fatalf("schema-four migration did not reach both surfaces: Manage=%q standalone=%q", app.manageConfig.LazyGitPagerPreset, app.deepDiveConfig.LazyGitPagerPreset)
	}
	const want = "custom LazyGit pagers are read-only in this release"
	if got := lazyGitManageUIBlockReason(app); got != want {
		t.Fatalf("Manage block reason=%q", got)
	}
	if got := lazyGitStandaloneUIBlockReason(app); got != want {
		t.Fatalf("standalone block reason=%q", got)
	}
	if app.nativeConfigState.LazyGit.ReadOnlyReason != "" {
		t.Fatalf("test unexpectedly relied on native read-only state: %q", app.nativeConfigState.LazyGit.ReadOnlyReason)
	}
}
