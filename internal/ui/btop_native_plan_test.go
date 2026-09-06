package ui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func newNativeBtopTestApp(t *testing.T, content string) (*App, string) {
	t.Helper()
	home := withTempHome(t)
	path := filepath.Join(home, ".config", "btop", "btop.conf")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(true)
	seedTypedReadyInstallCache(t, app, map[string]bool{})
	app.manageInstalledReady = true
	app.manageInstalled = map[string]bool{}
	return app, home
}

func TestBtopNativeAdoptionPlansAndAppliesThroughStandaloneAndManage(t *testing.T) {
	native := "# keep this native comment\nproc_sorting = \"memory\"\ncolor_theme = \"catppuccin-mocha\"\nupdate_ms = 2000\ngraph_symbol = \"braille\"\nshown_boxes = \"cpu mem net proc\"\nshow_coretemp = true\ntemp_scale = \"celsius\"\n"
	for _, mode := range []string{"standalone", "manage"} {
		t.Run(mode, func(t *testing.T) {
			app, home := newNativeBtopTestApp(t, native)
			var plan *installPlan
			if mode == "standalone" {
				app.startScreen = ScreenConfigBtop
				app.deepDiveConfig.BtopUpdateMs = 2500
				var err error
				plan, err = buildStandaloneConfigPlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				result := executeStandaloneConfigPlanResult(context.Background(), plan, defaultStandaloneConfigRuntime())
				if result.err != nil || !result.applied {
					t.Fatalf("standalone result = %+v", result)
				}
			} else {
				app.manageConfig.BtopUpdateMs = 2500
				managePlan, err := buildManageSavePlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				plan = managePlan.plan
				result := executeManageSavePlanResult(context.Background(), managePlan, defaultManageSaveRuntime())
				if result.err != nil || !result.applied {
					t.Fatalf("Manage result = %+v", result)
				}
			}
			action := planActionByID(t, plan, "config:btop")
			if action.Disposition != operation.DispositionApply || action.Ownership != operation.OwnershipManagedSet || !strings.Contains(action.Description, "generated theme") {
				t.Fatalf("btop action = %+v", action)
			}
			configRel := ".config/btop/btop.conf"
			themeRel := ".config/btop/themes/catppuccin-mocha.theme"
			if action.TargetOwnership[configRel] != operation.OwnershipManagedFragment || action.TargetOwnership[themeRel] != operation.OwnershipManagedFile {
				t.Fatalf("btop target ownership = %v", action.TargetOwnership)
			}
			if !slices.Contains(action.BackupTargets, configRel) || !slices.Contains(action.BackupTargets, themeRel) {
				t.Fatalf("btop backups = %v", action.BackupTargets)
			}
			got, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(configRel)))
			if err != nil || !bytes.HasPrefix(got, []byte("# keep this native comment\nproc_sorting = \"memory\"\n")) || bytes.Count(got, []byte("# >>> dotfiles managed btop settings >>>")) != 1 || !bytes.Contains(got, []byte("update_ms = 2500")) {
				t.Fatalf("adopted btop config = %q, %v", got, err)
			}
			reloaded := NewApp(true)
			reloaded.startScreen = ScreenConfigBtop
			reloaded.deepDiveConfig.BtopUpdateMs++
			reviewed, err := buildStandaloneConfigPlan(reloaded, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			managed := false
			for _, observation := range planActionByID(t, reviewed, "config:btop").Observations {
				if observation.Source == configRel {
					managed = observation.Managed
				}
			}
			if !managed {
				t.Fatal("exact btop managed section was not reported as managed in reviewed observation")
			}
		})
	}
}

func TestBtopPlanRevalidatesStaleNativeThemeAndAllowsExplicitReplacement(t *testing.T) {
	safe := "color_theme = \"catppuccin-mocha\"\nupdate_ms = 2000\ngraph_symbol = \"braille\"\nshown_boxes = \"cpu mem\"\nshow_coretemp = true\ntemp_scale = \"celsius\"\n"
	for _, mode := range []string{"standalone", "manage", "install"} {
		t.Run(mode+" blocks unrelated save", func(t *testing.T) {
			app, home := newNativeBtopTestApp(t, safe)
			path := filepath.Join(home, ".config", "btop", "btop.conf")
			if err := os.WriteFile(path, []byte(strings.Replace(safe, "catppuccin-mocha", "my-private-theme", 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			var action operation.Action
			switch mode {
			case "standalone":
				app.startScreen = ScreenConfigBtop
				app.deepDiveConfig.BtopUpdateMs++
				plan, err := buildStandaloneConfigPlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan, "config:btop")
			case "manage":
				app.manageConfig.BtopUpdateMs++
				plan, err := buildManageSavePlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan.plan, "config:btop")
			case "install":
				app.deepDiveConfig.CLITools["btop"] = true
				plan, err := buildInstallPlan(app, registryRuntime(pkg.PlatformMacOS, app.manageInstalled), time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan, "config:btop")
			}
			if action.Disposition != operation.DispositionBlocked || !strings.Contains(action.Reason, "explicit replacement") {
				t.Fatalf("stale custom theme action = %+v", action)
			}
		})

		t.Run(mode+" allows explicit theme replacement", func(t *testing.T) {
			app, home := newNativeBtopTestApp(t, safe)
			path := filepath.Join(home, ".config", "btop", "btop.conf")
			if err := os.WriteFile(path, []byte(strings.Replace(safe, "catppuccin-mocha", "my-private-theme", 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			var action operation.Action
			switch mode {
			case "standalone":
				app.startScreen = ScreenConfigBtop
				app.deepDiveConfig.BtopTheme = "nord"
				plan, err := buildStandaloneConfigPlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan, "config:btop")
			case "manage":
				app.manageConfig.BtopTheme = "nord"
				plan, err := buildManageSavePlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan.plan, "config:btop")
			case "install":
				app.deepDiveConfig.CLITools["btop"] = true
				app.deepDiveConfig.BtopTheme = "nord"
				plan, err := buildInstallPlan(app, registryRuntime(pkg.PlatformMacOS, app.manageInstalled), time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan, "config:btop")
			}
			if action.Disposition != operation.DispositionApply {
				t.Fatalf("explicit replacement action = %+v", action)
			}
		})
	}
}

func TestBtopExternalXDGPlanIsExplicitlyBlocked(t *testing.T) {
	home := withTempHome(t)
	external := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", external)
	app := NewApp(true)
	app.startScreen = ScreenConfigBtop
	plan, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	action := planActionByID(t, plan, "config:btop")
	if action.Disposition != operation.DispositionBlocked || !strings.Contains(action.Reason, "outside HOME") {
		t.Fatalf("external XDG action = %+v (home %s)", action, home)
	}
}

func TestInitialUnsupportedBtopThemeCanBeExplicitlyResolved(t *testing.T) {
	custom := "color_theme = \"my-private-theme\"\nupdate_ms = 2000\ngraph_symbol = \"braille\"\nshown_boxes = \"cpu mem\"\nshow_coretemp = true\ntemp_scale = \"celsius\"\n"
	for _, mode := range []string{"standalone", "manage"} {
		t.Run(mode, func(t *testing.T) {
			app, _ := newNativeBtopTestApp(t, custom)
			if app.nativeConfigState.BtopError == "" || !app.nativeConfigState.BtopThemeUnsupported {
				t.Fatalf("initial custom theme state = %+v", app.nativeConfigState)
			}
			var action operation.Action
			if mode == "standalone" {
				app.startScreen = ScreenConfigBtop
				app.deepDiveConfig.BtopTheme = "nord"
				plan, err := buildStandaloneConfigPlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan, "config:btop")
			} else {
				app.manageConfig.BtopTheme = "nord"
				plan, err := buildManageSavePlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan.plan, "config:btop")
			}
			if action.Disposition != operation.DispositionApply {
				t.Fatalf("explicit initial-theme resolution action = %+v", action)
			}
		})
	}
}

func TestBtopMixedOwnershipIsRenderedInEveryReviewSurface(t *testing.T) {
	native := "color_theme = \"catppuccin-mocha\"\nupdate_ms = 2000\ngraph_symbol = \"braille\"\nshown_boxes = \"cpu mem\"\nshow_coretemp = true\ntemp_scale = \"celsius\"\n"
	app, _ := newNativeBtopTestApp(t, native)
	app.startScreen = ScreenConfigBtop
	app.deepDiveConfig.BtopUpdateMs++
	standalone, err := buildStandaloneConfigPlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app.standaloneConfigPlan = standalone
	ctx := NewTestScreenContext()
	ctx.app = app
	standaloneView := NewConfigSaveConfirmScreen(ctx).View(120, 80)

	app.manageConfig.BtopUpdateMs++
	manage, err := buildManageSavePlan(app, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app.pendingManageSavePlan = manage
	manageView := NewManageSaveConfirmScreen(ctx).View(120, 80)

	app.deepDiveConfig.CLITools["btop"] = true
	install, err := buildInstallPlan(app, registryRuntime(pkg.PlatformMacOS, app.manageInstalled), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app.pendingInstallPlan = install
	installView := NewFileTreeScreen(ctx).View(140, 240)

	for name, view := range map[string]string{"standalone": standaloneView, "manage": manageView, "install": installView} {
		for _, want := range []string{"managed_set", ".config/btop/btop.conf: managed_fragment", ".config/btop/themes/catppuccin-mocha.theme: managed_file"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%s review omits %q:\n%s", name, want, view)
			}
		}
	}
}

func TestGlobalThemeChangeExplicitlyReplacesAutoBtopTheme(t *testing.T) {
	native := "color_theme = \"catppuccin-mocha\"\nupdate_ms = 2000\ngraph_symbol = \"braille\"\nshown_boxes = \"cpu mem\"\nshow_coretemp = true\ntemp_scale = \"celsius\"\n"
	for _, mode := range []string{"standalone", "manage", "install"} {
		t.Run(mode, func(t *testing.T) {
			app, _ := newNativeBtopTestApp(t, native)
			app.theme = "nord"
			var action operation.Action
			switch mode {
			case "standalone":
				app.startScreen = ScreenConfigBtop
				plan, err := buildStandaloneConfigPlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan, "config:btop")
			case "manage":
				app.manageConfig.BtopUpdateMs++
				plan, err := buildManageSavePlan(app, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan.plan, "config:btop")
			case "install":
				app.deepDiveConfig.CLITools["btop"] = true
				plan, err := buildInstallPlan(app, registryRuntime(pkg.PlatformMacOS, app.manageInstalled), time.Now())
				if err != nil {
					t.Fatal(err)
				}
				action = planActionByID(t, plan, "config:btop")
			}
			if action.Disposition != operation.DispositionApply || !slices.Contains(action.BackupTargets, ".config/btop/themes/nord.theme") {
				t.Fatalf("global-theme replacement action = %+v", action)
			}
		})
	}
}

func TestBtopAmbiguousOrUnsupportedNativeImportBlocksEveryPlan(t *testing.T) {
	for name, native := range map[string]string{
		"malformed":         "update_ms 2000\n",
		"missing theme":     "update_ms = 2000\n",
		"unsupported theme": "color_theme = \"my-private-theme\"\nupdate_ms = 2000\n",
	} {
		t.Run(name, func(t *testing.T) {
			app, _ := newNativeBtopTestApp(t, native)
			if app.nativeConfigState.BtopError == "" {
				t.Fatal("native btop ambiguity was not surfaced")
			}
			app.startScreen = ScreenConfigBtop
			standalone, err := buildStandaloneConfigPlan(app, time.Now())
			if err != nil || !standalone.hasBlocked() {
				t.Fatalf("standalone blocked=%v err=%v", standalone != nil && standalone.hasBlocked(), err)
			}
			app.manageConfig.BtopUpdateMs++
			manage, err := buildManageSavePlan(app, time.Now())
			if err != nil || !manage.plan.hasBlocked() {
				t.Fatalf("Manage blocked=%v err=%v", manage != nil && manage.plan.hasBlocked(), err)
			}
			app.deepDiveConfig.CLITools["btop"] = true
			runtime := registryRuntime(pkg.PlatformMacOS, app.manageInstalled)
			install, err := buildInstallPlan(app, runtime, time.Now())
			if err != nil || planActionByID(t, install, "config:btop").Disposition != operation.DispositionBlocked {
				t.Fatalf("installer btop action blocked=%v err=%v", install != nil && install.hasBlocked(), err)
			}
		})
	}
}
