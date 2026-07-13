package ui

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

const installationSnapshotUnavailable = "installation status unavailable"

type installationSnapshotCacheRuntime struct {
	allTools         func() []tools.Tool
	detectPlatform   func() pkg.Platform
	detectManager    func() pkg.PackageManager
	observeHealth    func(context.Context, []tools.Tool, pkg.PackageManager, pkg.Platform, uint64) (health.InstallationSnapshot, error)
	observeUtilities func(context.Context, uint64) (map[string]bool, error)
}

type installationSnapshotDoneMsg struct {
	Generation      uint64
	Platform        pkg.Platform
	Manager         string
	ManagerIdentity pkg.ExecutableIdentity
	Snapshot        health.InstallationSnapshot
	Utilities       map[string]bool
	Err             error
	UtilityErr      error
}

type installationSnapshotCacheView struct {
	Generation        uint64
	Snapshot          health.InstallationSnapshot
	Ready             bool
	Loading           bool
	Stale             bool
	Error             string
	Utilities         map[string]bool
	CosmeticInstalled map[string]bool
}

func defaultInstallationSnapshotCacheRuntime() installationSnapshotCacheRuntime {
	return installationSnapshotCacheRuntime{
		allTools:       func() []tools.Tool { return tools.GetRegistry().All() },
		detectPlatform: pkg.DetectPlatform,
		detectManager:  pkg.DetectManager,
		observeHealth:  tools.ObserveInstallationHealth,
		observeUtilities: func(_ context.Context, _ uint64) (map[string]bool, error) {
			return observeUtilityInstallations()
		},
	}
}

func observeUtilityInstallations() (map[string]bool, error) {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, errors.New(installationSnapshotUnavailable)
		}
	}
	installed := make(map[string]bool, 3)
	for _, id := range []string{"hk", "caff", "sshh"} {
		_, err := os.Stat(filepath.Join(home, ".local", "bin", id))
		installed[id] = err == nil
	}
	return installed, nil
}

func (a *App) beginInstallationSnapshotLoad(runtime installationSnapshotCacheRuntime) tea.Cmd {
	a.installationSnapshotGeneration++
	generation := a.installationSnapshotGeneration
	a.installationSnapshotTerminal = false
	a.installationSnapshotReady = false
	a.installationSnapshotLoading = true
	a.installationSnapshotStale = a.installationSnapshot.Digest() != ""
	a.installationSnapshotError = ""
	a.installCacheLoading = true
	a.manageInstalledReady = false
	a.pendingInstallPlan = nil
	a.installPlanError = nil

	return func() tea.Msg {
		var platform pkg.Platform
		if runtime.detectPlatform != nil {
			platform = runtime.detectPlatform()
		}
		var manager pkg.PackageManager
		if runtime.detectManager != nil {
			manager = runtime.detectManager()
		}
		managerName := ""
		managerIdentity := pkg.ExecutableIdentity{}
		if manager != nil {
			managerName = manager.Name()
			if provider, ok := manager.(pkg.ExecutableIdentityProvider); ok {
				if observed, available := provider.ExecutableIdentity(); available && validUIManagerExecutableIdentity(observed) {
					managerIdentity = observed
				}
			}
		}
		var snapshot health.InstallationSnapshot
		var healthErr error
		if runtime.observeHealth == nil {
			healthErr = errors.New(installationSnapshotUnavailable)
		} else {
			all := []tools.Tool(nil)
			if runtime.allTools != nil {
				all = runtime.allTools()
			}
			snapshot, healthErr = runtime.observeHealth(context.Background(), all, manager, platform, generation)
		}
		var utilities map[string]bool
		var utilityErr error
		if runtime.observeUtilities != nil {
			utilities, utilityErr = runtime.observeUtilities(context.Background(), generation)
		}
		return installationSnapshotDoneMsg{Generation: generation, Platform: platform, Manager: managerName, ManagerIdentity: managerIdentity, Snapshot: snapshot, Utilities: utilities, Err: healthErr, UtilityErr: utilityErr}
	}
}

func (a *App) applyInstallationSnapshotDone(message installationSnapshotDoneMsg) {
	if message.Generation != a.installationSnapshotGeneration || a.installationSnapshotTerminal || !a.installationSnapshotLoading {
		return
	}
	a.installationSnapshotTerminal = true
	valid := message.Err == nil && message.Snapshot.Digest() != "" && message.Snapshot.Generation() == message.Generation && message.Snapshot.Platform() == string(message.Platform) && message.Snapshot.Manager() == message.Manager
	if !valid {
		a.installationSnapshotLoading = false
		a.installationSnapshotReady = false
		a.installationSnapshotStale = a.installationSnapshot.Digest() != ""
		a.installationSnapshotError = installationSnapshotUnavailable
		a.installCacheLoading = false
		a.manageInstalledReady = false
		return
	}

	cosmetic := make(map[string]bool, len(message.Snapshot.Tools()))
	for _, observation := range message.Snapshot.Tools() {
		cosmetic[observation.ToolID()] = observation.Presence() == health.PresencePresent
	}
	a.installationSnapshot = message.Snapshot
	a.installationSnapshotManagerIdentity = message.ManagerIdentity
	if message.UtilityErr == nil {
		a.installationSnapshotUtilities = maps.Clone(message.Utilities)
	}
	a.installationSnapshotCosmetic = cosmetic
	a.installationSnapshotLoading = false
	a.installationSnapshotReady = true
	a.installationSnapshotStale = false
	a.installationSnapshotError = ""
	a.installCacheLoading = false
	a.manageInstalled = maps.Clone(cosmetic)
	a.manageInstalledReady = true
	if a.currentScreenIs(ScreenFileTree) {
		a.refreshPendingInstallPlan()
	}
}

// currentScreenIs uses the ScreenManager as the live navigation authority. The
// legacy App.screen field is only a safe fallback for small unit-test/embedding
// contexts that do not have a current managed handler.
func (a *App) currentScreenIs(screen Screen) bool {
	if a == nil {
		return false
	}
	if a.screenMgr != nil && a.screenMgr.Current() != nil {
		return a.screenMgr.Current().ID() == screen
	}
	return a.screen == screen
}

func (a *App) installationSnapshotCacheView() installationSnapshotCacheView {
	return installationSnapshotCacheView{
		Generation: a.installationSnapshotGeneration, Snapshot: a.installationSnapshot,
		Ready: a.installationSnapshotReady, Loading: a.installationSnapshotLoading,
		Stale: a.installationSnapshotStale, Error: a.installationSnapshotError,
		Utilities: maps.Clone(a.installationSnapshotUtilities), CosmeticInstalled: maps.Clone(a.installationSnapshotCosmetic),
	}
}

func (a *App) installationHealthObservation(toolID string) (health.InstallationObservation, bool) {
	return a.installationSnapshot.Tool(toolID)
}

func (a *App) installationUtilityResults() map[string]bool {
	return maps.Clone(a.installationSnapshotUtilities)
}

func (a *App) installationUtilityInstalled(id string) (bool, bool) {
	installed, observed := a.installationSnapshotUtilities[id]
	return installed, observed
}

func (a *App) installationSnapshotPlanningReady() bool {
	return a.installationSnapshotReady && !a.installationSnapshotLoading && !a.installationSnapshotStale && a.installationSnapshotError == ""
}
