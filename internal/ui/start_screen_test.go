package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/config"
)

func TestStartScreenInitialViewMatchesCLI(t *testing.T) {
	for _, screen := range []Screen{ScreenConfigZsh, ScreenManage, ScreenHotkeys} {
		t.Run(fmt.Sprint(screen), func(t *testing.T) {
			withTempHome(t)
			a := NewApp(true)
			a.SetStartScreen(screen)
			a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			view := a.View()
			if a.screenMgr.Current().ID() != screen {
				t.Fatalf("first frame screen=%v want%v", a.screenMgr.Current().ID(), screen)
			}
			if view == "" {
				t.Fatal("initial view is empty")
			}
			if a.installCacheLoading || a.installationSnapshotGeneration != 0 {
				t.Fatal("first frame started a loader without returning its command")
			}
		})
	}
}

type startupProbeScreen struct {
	ScreenHandler
	initialize func() tea.Cmd
}

func (s *startupProbeScreen) Init() tea.Cmd { return s.initialize() }

func TestStartScreenManageRetainsExactlyOneLoader(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	a.animationsEnabled = false
	a.syncSharedSettings()
	factory := a.screenMgr.factory
	initialized, executed := 0, 0
	a.screenMgr.factory = func(id Screen, ctx *ScreenContext) ScreenHandler {
		handler := factory(id, ctx)
		if id != ScreenManage {
			return handler
		}
		return &startupProbeScreen{ScreenHandler: handler, initialize: func() tea.Cmd {
			initialized++
			actual := handler.Init()
			if actual == nil {
				return nil
			}
			// Replace external observation only; exercise the real Manage Init and
			// shared preload guard, then return a coherent terminal observation.
			result := coherentCacheResult(a.installationSnapshotGeneration, cacheSnapshot(t, a.installationSnapshotGeneration), nil)
			return func() tea.Msg { executed++; return result }
		}}
	}
	a.SetStartScreen(ScreenManage)
	a.SetStartScreen(ScreenHotkeys)
	a.SetStartScreen(ScreenManage)
	_ = a.View()
	_ = a.View()
	if initialized != 0 || a.installationSnapshotGeneration != 0 {
		t.Fatal("retarget/first render initialized an abandoned screen")
	}
	cmd := a.Init()
	if cmd == nil || initialized != 1 || a.installationSnapshotGeneration != 1 || !a.installCacheLoading {
		t.Fatal("startup failed to retain exactly one Manage loader")
	}
	if next := a.Init(); next != nil || initialized != 1 {
		t.Fatal("repeated Init duplicated work")
	}
	done := cmd()
	if _, ok := done.(installationSnapshotDoneMsg); !ok {
		t.Fatalf("startup emitted duplicate/unexpected work: %T", done)
	}
	a.Update(done)
	if executed != 1 || a.installCacheLoading || !a.manageInstalledReady || !a.installationSnapshotReady {
		t.Fatal("retained loader did not complete")
	}
	a.screenMgr.factory = factory
	if next := a.screenMgr.Navigate(ScreenManage); next != nil || a.installationSnapshotGeneration != 1 {
		t.Fatal("completed startup query repeated on reentry")
	}
}

func TestStartScreenRealReadLoadersComplete(t *testing.T) {
	for _, screen := range []Screen{ScreenBackups, ScreenUsers, ScreenUpdate} {
		t.Run(fmt.Sprint(screen), func(t *testing.T) {
			withTempHome(t)
			a := NewApp(true)
			a.animationsEnabled = false
			a.syncSharedSettings()
			seedTypedReadyInstallCache(t, a, map[string]bool{})
			a.SetStartScreen(screen)
			if a.backupsLoading || a.usersLoaded || a.updateChecking {
				t.Fatal("route setup started a read")
			}
			cmd := a.Init()
			if cmd == nil {
				t.Fatal("startup read missing")
			}
			if screen == ScreenUpdate {
				// Package queries stay outside this handler-level test; validate that
				// its real initializer bound one generation and accept that result.
				a.Update(appAsyncResult{channel: asyncUpdates, generation: a.asyncRequests[asyncUpdates].generation, payload: updateCheckDoneMsg{}})
				if !a.updateCheckDone || a.updateChecking {
					t.Fatal("startup update result lost")
				}
			} else {
				msg := cmd()
				if _, ok := msg.(appAsyncResult); !ok {
					t.Fatalf("startup loader=%T", msg)
				}
				a.Update(msg)
				if screen == ScreenBackups && (!a.backupsLoaded || a.backupsLoading) {
					t.Fatal("startup backup result lost")
				}
				if screen == ScreenUsers && !a.usersLoaded {
					t.Fatal("startup user result lost")
				}
			}
			if next := a.Init(); next != nil {
				t.Fatal("completed read restarted")
			}
		})
	}
}

func TestStartScreenIntroRoutingAndInitialization(t *testing.T) {
	for _, skip := range []bool{false, true} {
		for _, motion := range []bool{false, true} {
			for _, target := range []Screen{ScreenManage, ScreenAnimation} {
				t.Run(fmt.Sprintf("skip=%v/motion=%v/target=%v", skip, motion, target), func(t *testing.T) {
					withTempHome(t)
					cfg := config.DefaultGlobalConfig()
					cfg.DisableAnimations = !motion
					if err := config.SaveGlobalConfig(cfg); err != nil {
						t.Fatal(err)
					}
					a := NewApp(skip)
					seedTypedReadyInstallCache(t, a, map[string]bool{})
					a.SetStartScreen(target)
					want := target
					if target == ScreenAnimation {
						want = ScreenWelcome
					}
					intro := !skip && motion
					if intro {
						want = ScreenAnimation
					}
					if a.screenMgr.Current().ID() != want {
						t.Fatalf("initial route=%v want%v", a.screenMgr.Current().ID(), want)
					}
					if a.screenMgr.currentInitialized {
						t.Fatal("intro initialized during setup")
					}
					initialized := 0
					factory := a.screenMgr.factory
					a.screenMgr.factory = func(id Screen, ctx *ScreenContext) ScreenHandler {
						handler := factory(id, ctx)
						return &startupProbeScreen{ScreenHandler: handler, initialize: func() tea.Cmd { initialized++; return handler.Init() }}
					}
					// Re-prepare the same route before initialization to install the probe.
					a.SetStartScreen(target)
					_ = a.Init()
					if initialized != 1 {
						t.Fatalf("initializer count=%d", initialized)
					}
					if cmd := a.Init(); cmd != nil {
						t.Fatal("intro initialized twice")
					}
					if intro {
						_, cmd := a.Update(animationDoneMsg{})
						if cmd == nil {
							t.Fatal("intro completion lost destination")
						}
						msg := cmd()
						nav, ok := msg.(NavigateMsg)
						if !ok {
							t.Fatalf("unexpected completion command %T", msg)
						}
						expected := target
						if target == ScreenAnimation {
							expected = ScreenWelcome
						}
						if nav.To != expected {
							t.Fatalf("post-intro=%v want%v", nav.To, expected)
						}
						a.Update(nav)
						if a.screenMgr.Current().ID() != expected || initialized != 2 {
							t.Fatal("destination did not initialize once after intro")
						}
					}
				})
			}
		}
	}
}

func TestStartScreenViewFallbackDoesNotDiscardInit(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	a.animationsEnabled = false
	a.SetStartScreen(ScreenBackups)
	seedTypedReadyInstallCache(t, a, map[string]bool{})
	a.screenMgr.current = nil
	_ = a.View()
	if a.backupsLoading || a.screenMgr.currentInitialized {
		t.Fatal("View discarded initialization work")
	}
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("fallback lost startup loader")
	}
	a.Update(cmd())
	if !a.backupsLoaded || a.backupsLoading {
		t.Fatal("fallback startup did not complete")
	}
}

func TestStartScreenConstructorOptionAndRetargetResetMode(t *testing.T) {
	withTempHome(t)
	a := NewApp(true, func(a *App) { a.SetStartScreen(ScreenThemePicker) })
	if a.screenMgr.Current().ID() != ScreenThemePicker || !a.themeStandalone {
		t.Fatal("constructor option lost requested route")
	}
	a.SetStartScreen(ScreenManage)
	if a.themeStandalone || a.configStandalone || a.screenMgr.Current().ID() != ScreenManage || a.installCacheLoading {
		t.Fatal("retarget kept abandoned screen mode or work")
	}
}
