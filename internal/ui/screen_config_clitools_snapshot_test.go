package ui

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

func TestCLIToolsSelectionUsesTypedSnapshotTruth(t *testing.T) {
	tests := []struct {
		name           string
		presence       health.Presence
		installability health.Installability
		legacy         bool
		wantLabel      string
		wantToggle     bool
	}{
		{name: "present", presence: health.PresencePresent, installability: health.InstallabilitySupported, wantLabel: "installed"},
		{name: "missing", presence: health.PresenceMissing, installability: health.InstallabilitySupported, legacy: true, wantLabel: "missing — install", wantToggle: true},
		{name: "partial", presence: health.PresencePartial, installability: health.InstallabilitySupported, wantLabel: "partial — repair", wantToggle: true},
		{name: "unknown", presence: health.PresenceUnknown, installability: health.InstallabilitySupported, wantLabel: "status unknown"},
		{name: "unsupported", presence: health.PresenceMissing, installability: health.InstallabilityUnsupported, wantLabel: "installation unavailable on macos"},
		{name: "availability unknown", presence: health.PresenceMissing, installability: health.InstallabilityUnknown, wantLabel: "installation availability unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := cursorAgentBareContext(t)
			app := ctx.app
			app.manageInstalled = map[string]bool{"codex": test.legacy}
			app.deepDiveConfig.CLITools = map[string]bool{"codex": false}
			app.cliToolIndex = cliToolIndexForTest(t, "codex")
			observation := manageTruthObservationWithInstallability(t, "codex", test.presence, test.installability)
			setManageTruthSnapshot(t, app, 201, pkg.PlatformMacOS, "brew", observation)

			screen := NewConfigCLIToolsScreen(ctx)
			beforeView := maps.Clone(app.deepDiveConfig.CLITools)
			view := strings.ToLower(stripANSITest(screen.View(80, 24)))
			if !reflect.DeepEqual(app.deepDiveConfig.CLITools, beforeView) {
				t.Fatalf("%s View mutated selections: before=%v after=%v", test.name, beforeView, app.deepDiveConfig.CLITools)
			}
			if !strings.Contains(view, test.wantLabel) {
				t.Fatalf("%s view missing %q:\n%s", test.name, test.wantLabel, view)
			}

			_, _ = screen.Update(keyMsg(" "))
			if got := app.deepDiveConfig.CLITools["codex"]; got != test.wantToggle {
				t.Fatalf("%s toggle=%t, want %t", test.name, got, test.wantToggle)
			}
		})
	}
}

func TestCLIToolsSelectionFailsClosedWithoutFreshSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*App)
		wantLabel string
	}{
		{name: "loading", configure: func(app *App) { app.installationSnapshotLoading = true; app.installCacheLoading = true }, wantLabel: "installation status loading"},
		{name: "stale", configure: func(app *App) { app.installationSnapshotStale = true }, wantLabel: "installation status stale"},
		{name: "error", configure: func(app *App) { app.installationSnapshotError = "secret token abc123 from /Users/private" }, wantLabel: installationSnapshotUnavailable},
		{name: "unready", configure: func(app *App) { app.installationSnapshotReady = false }, wantLabel: "installation status unknown"},
		{name: "generation mismatch", configure: func(app *App) { app.installationSnapshotGeneration++ }, wantLabel: installationSnapshotUnavailable},
		{name: "absent observation", configure: func(app *App) {
			setManageTruthSnapshot(t, app, 202, pkg.PlatformMacOS, "brew")
			app.manageInstalled["codex"] = true
		}, wantLabel: "installation status unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := cursorAgentBareContext(t)
			app := ctx.app
			app.manageInstalled = map[string]bool{"codex": false}
			app.deepDiveConfig.CLITools = map[string]bool{"codex": false}
			app.cliToolIndex = cliToolIndexForTest(t, "codex")
			setManageTruthSnapshot(t, app, 202, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "codex", health.PresenceMissing))
			test.configure(app)

			screen := NewConfigCLIToolsScreen(ctx)
			beforeView := maps.Clone(app.deepDiveConfig.CLITools)
			view := strings.ToLower(stripANSITest(screen.View(80, 24)))
			if !strings.Contains(view, test.wantLabel) {
				t.Fatalf("%s view missing %q:\n%s", test.name, test.wantLabel, view)
			}
			if strings.Contains(view, "secret token") || strings.Contains(view, "/users/private") {
				t.Fatalf("%s leaked raw cache error:\n%s", test.name, view)
			}
			if !reflect.DeepEqual(app.deepDiveConfig.CLITools, beforeView) {
				t.Fatalf("%s View mutated selections: before=%v after=%v", test.name, beforeView, app.deepDiveConfig.CLITools)
			}
			_, _ = screen.Update(keyMsg(" "))
			if app.deepDiveConfig.CLITools["codex"] {
				t.Fatalf("%s toggled without fresh authoritative evidence", test.name)
			}
		})
	}
}

func TestCLIToolsSnapshotCacheStatesRemainBoundedAndInert(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*App)
		want      string
	}{
		{
			name: "loading",
			configure: func(app *App) {
				app.installationSnapshotLoading = true
				app.installCacheLoading = true
			},
			want: "Installation status loading...",
		},
		{
			name: "stale",
			configure: func(app *App) {
				app.installationSnapshotStale = true
			},
			want: "Installation status stale",
		},
		{
			name: "error",
			configure: func(app *App) {
				app.installationSnapshotError = "secret token abc123 from /Users/private"
			},
			want: installationSnapshotUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
				t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
					ctx := cursorAgentBareContext(t)
					setManageTruthSnapshot(t, ctx.app, 208, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "codex", health.PresenceMissing))
					ctx.app.cliToolIndex = cliToolIndexForTest(t, "codex")
					ctx.app.deepDiveConfig.CLITools = map[string]bool{"codex": false}
					ctx.app.width, ctx.app.height = 200-size.width, 70-size.height
					test.configure(ctx.app)
					screen := NewConfigCLIToolsScreen(ctx)
					before := maps.Clone(ctx.app.deepDiveConfig.CLITools)
					view := screen.View(size.width, size.height)
					assertCLIToolsSnapshotFits(t, view, size.width, size.height)
					plain := stripANSITest(view)
					if !strings.Contains(plain, test.want) {
						t.Fatalf("%s at %dx%d missing sanitized status %q:\n%s", test.name, size.width, size.height, test.want, plain)
					}
					if strings.Contains(strings.ToLower(plain), "secret token") || strings.Contains(strings.ToLower(plain), "/users/private") {
						t.Fatalf("%s at %dx%d leaked raw cache error:\n%s", test.name, size.width, size.height, plain)
					}
					if !reflect.DeepEqual(ctx.app.deepDiveConfig.CLITools, before) {
						t.Fatalf("%s View mutated selections at %dx%d", test.name, size.width, size.height)
					}
					screen.Update(keyMsg(" "))
					if !reflect.DeepEqual(ctx.app.deepDiveConfig.CLITools, before) {
						t.Fatalf("%s accepted input at %dx%d", test.name, size.width, size.height)
					}
				})
			}
		})
	}
}

func TestCLIToolsSnapshotTruthRemainsResponsive(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		ctx := cursorAgentBareContext(t)
		ctx.Width, ctx.Height = size.width, size.height
		// Deliberately contradict stored App dimensions. View arguments are the
		// sole layout authority for deterministic embedding and resize handling.
		ctx.app.width, ctx.app.height = 200-size.width, 70-size.height
		ctx.app.deepDiveConfig.CLITools = map[string]bool{"pi": false}
		ctx.app.cliToolIndex = cliToolIndexForTest(t, "pi")
		setManageTruthSnapshot(t, ctx.app, 203, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "pi", health.PresencePartial))

		screen := NewConfigCLIToolsScreen(ctx)
		view := screen.View(size.width, size.height)
		if !strings.Contains(strings.ToLower(stripANSITest(view)), "partial — repair") {
			t.Fatalf("%dx%d omitted partial repair truth:\n%s", size.width, size.height, stripANSITest(view))
		}
		assertRenderedWidth(t, view, size.width)
		if got := len(strings.Split(view, "\n")); got > size.height {
			t.Fatalf("%dx%d rendered %d lines", size.width, size.height, got)
		}
		rowY := labelLineY(t, view, "Pi")
		if rowY < 0 || rowY >= size.height {
			t.Fatalf("%dx%d focused Pi row y=%d is outside the rendered viewport", size.width, size.height, rowY)
		}
		screen.Update(clickAt(size.width/2, rowY))
		screen.Update(keyMsg(" "))
		if !ctx.app.deepDiveConfig.CLITools["pi"] {
			t.Fatalf("%dx%d visible partial/repair row was not actionable", size.width, size.height)
		}
	}
}

func TestCLIToolsSnapshotLoadingRemainsResponsive(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			ctx := cursorAgentBareContext(t)
			ctx.app.width, ctx.app.height = 200-size.width, 70-size.height
			ctx.app.installationSnapshotReady = false
			ctx.app.installationSnapshotLoading = true
			ctx.app.installCacheLoading = true

			view := NewConfigCLIToolsScreen(ctx).View(size.width, size.height)
			if !strings.Contains(strings.ToLower(stripANSITest(view)), "installation status loading") {
				t.Fatalf("%dx%d loading view omitted truthful status:\n%s", size.width, size.height, stripANSITest(view))
			}
			assertCLIToolsSnapshotFits(t, view, size.width, size.height)
		})
	}
}

func TestCLIToolsViewAndInputContainNoLiveHostProbes(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(current), "screen_config_clitools.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	for _, forbidden := range []string{"pkg.DetectPlatform(", "manageInstalled[", "tools.GetRegistry()", "cliToolAvailableForPlatform(", ".IsInstalled(", "ObserveInstallationHealth(", "ObserveInstallations("} {
		if strings.Contains(source, forbidden) {
			t.Errorf("CLI-tools screen still contains live/lossy probe %q", forbidden)
		}
	}
}

func TestCLIToolsSnapshotKeyboardAndMouseActionParity(t *testing.T) {
	tests := []struct {
		name           string
		presence       health.Presence
		installability health.Installability
		legacy         bool
		wantToggle     bool
	}{
		{name: "present blocked", presence: health.PresencePresent, installability: health.InstallabilitySupported},
		{name: "partial repair", presence: health.PresencePartial, installability: health.InstallabilitySupported, legacy: true, wantToggle: true},
		{name: "missing install", presence: health.PresenceMissing, installability: health.InstallabilitySupported, legacy: true, wantToggle: true},
		{name: "unknown blocked", presence: health.PresenceUnknown, installability: health.InstallabilitySupported},
		{name: "unsupported blocked", presence: health.PresenceMissing, installability: health.InstallabilityUnsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := manageTruthObservationWithInstallability(t, "codex", test.presence, test.installability)
			newScreen := func(t *testing.T) (*App, *configCLIToolsScreen) {
				t.Helper()
				ctx := cursorAgentBareContext(t)
				ctx.app.manageInstalled = map[string]bool{"codex": test.legacy}
				ctx.app.deepDiveConfig.CLITools = map[string]bool{"codex": false}
				setManageTruthSnapshot(t, ctx.app, 204, pkg.PlatformMacOS, "brew", observation)
				// Reintroduce the contradictory legacy value after the typed snapshot
				// fixture updates compatibility readiness.
				ctx.app.manageInstalled["codex"] = test.legacy
				return ctx.app, NewConfigCLIToolsScreen(ctx)
			}

			keyboardApp, keyboardScreen := newScreen(t)
			keyboardApp.cliToolIndex = cliToolIndexForTest(t, "codex")
			keyboardScreen.Update(keyMsg(" "))

			mouseApp, mouseScreen := newScreen(t)
			mouseApp.cliToolIndex = cliToolIndexForTest(t, "glow")
			view := mouseScreen.View(80, 24)
			rowY := labelLineY(t, view, "Codex")
			if rowY < 0 || rowY >= 24 {
				t.Fatalf("Codex row y=%d outside mouse viewport:\n%s", rowY, stripANSITest(view))
			}
			mouseScreen.Update(clickAt(40, rowY))
			if mouseApp.cliToolIndex != cliToolIndexForTest(t, "codex") {
				t.Fatalf("mouse focused index=%d, want Codex", mouseApp.cliToolIndex)
			}
			mouseScreen.Update(keyMsg(" "))

			if !reflect.DeepEqual(mouseApp.deepDiveConfig.CLITools, keyboardApp.deepDiveConfig.CLITools) {
				t.Fatalf("mouse selections=%v differ from keyboard=%v", mouseApp.deepDiveConfig.CLITools, keyboardApp.deepDiveConfig.CLITools)
			}
			if got := mouseApp.deepDiveConfig.CLITools["codex"]; got != test.wantToggle {
				t.Fatalf("parity result=%t, want %t", got, test.wantToggle)
			}
		})
	}
}

func TestCLIToolsSnapshotPendingAgentsAreTruthfulAndInert(t *testing.T) {
	for _, id := range []string{"cursor-agent", "hermes"} {
		t.Run(id, func(t *testing.T) {
			registered, ok := tools.GetRegistry().Get(id)
			if !ok {
				t.Fatalf("registry tool %q missing", id)
			}
			reason := strings.TrimSpace(installationUnavailableReason(registered))
			if reason == "" {
				t.Fatalf("registry tool %q missing pending-support reason", id)
			}
			// Use the real typed discovery observation so the exact pending-support
			// reason is carried by the snapshot rather than recovered from registry
			// metadata inside View/Update.
			observation := planningCacheObservation(t, registered, false)

			for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
				t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
					ctx := cursorAgentBareContext(t)
					ctx.app.deepDiveConfig.CLITools = map[string]bool{id: true}
					setManageTruthSnapshot(t, ctx.app, 205, pkg.PlatformMacOS, "brew", observation)
					ctx.app.manageInstalled = map[string]bool{id: true}
					ctx.app.cliToolIndex = cliToolIndexForTest(t, id)
					ctx.app.width, ctx.app.height = 200-size.width, 70-size.height
					screen := NewConfigCLIToolsScreen(ctx)
					view := screen.View(size.width, size.height)
					assertCLIToolsSnapshotFits(t, view, size.width, size.height)
					plain := strings.ToLower(strings.Join(strings.Fields(stripANSITest(view)), " "))
					row := cliToolsSnapshotRow(view, registered.Name())
					if !strings.Contains(row, "status unknown") || !strings.Contains(row, "unavailable") {
						t.Fatalf("%s row=%q, want status unknown + unavailable", id, row)
					}
					if !strings.Contains(plain, strings.ToLower(strings.Join(strings.Fields(reason), " "))) {
						t.Fatalf("%s pending reason %q missing at %dx%d:\n%s", id, reason, size.width, size.height, stripANSITest(view))
					}
					if strings.Contains(row, "[x]") || strings.Contains(row, "[✓]") {
						t.Fatalf("%s contradictory selected value rendered actionable: %q", id, row)
					}

					before := maps.Clone(ctx.app.deepDiveConfig.CLITools)
					screen.Update(keyMsg(" "))
					rowY := labelLineY(t, view, registered.Name())
					if rowY < 0 || rowY >= size.height {
						t.Fatalf("%s row y=%d outside %dx%d", id, rowY, size.width, size.height)
					}
					screen.Update(clickAt(size.width/2, rowY))
					screen.Update(keyMsg(" "))
					if !reflect.DeepEqual(ctx.app.deepDiveConfig.CLITools, before) {
						t.Fatalf("%s unavailable row mutated selections: before=%v after=%v", id, before, ctx.app.deepDiveConfig.CLITools)
					}
				})
			}
		})
	}
}

func TestCLIToolsSnapshotClaudeCodeIsTruthfulNonNavigableContext(t *testing.T) {
	ctx := cursorAgentBareContext(t)
	ctx.app.deepDiveConfig.CLITools = map[string]bool{"claude-code": true}
	setManageTruthSnapshot(t, ctx.app, 206, pkg.PlatformMacOS, "brew",
		planningCacheObservation(t, tools.NewHermesAgentTool(), false),
		manageTruthObservation(t, "claude-code", health.PresencePresent),
	)
	ctx.app.manageInstalled = map[string]bool{"claude-code": false}
	ctx.app.cliToolIndex = navigableCLIToolCount - 1
	screen := NewConfigCLIToolsScreen(ctx)

	compact := screen.View(60, 18)
	assertCLIToolsSnapshotFits(t, compact, 60, 18)
	if row := cliToolsSnapshotRow(compact, "Claude Code"); !strings.Contains(row, "installed") {
		t.Fatalf("60x18 Hermes-focused viewport omitted truthful Claude context row: %q\n%s", row, stripANSITest(compact))
	}
	if !strings.Contains(strings.ToLower(stripANSITest(compact)), "configure from the dedicated claude code screen") {
		t.Fatalf("60x18 Hermes-focused viewport omitted Claude direction:\n%s", stripANSITest(compact))
	}
	compactClaudeY := labelLineY(t, compact, "Claude Code")
	beforeCompactIndex := ctx.app.cliToolIndex
	screen.Update(clickAt(30, compactClaudeY))
	if ctx.app.cliToolIndex != beforeCompactIndex {
		t.Fatalf("60x18 Claude context click changed index=%d, want=%d", ctx.app.cliToolIndex, beforeCompactIndex)
	}

	view := screen.View(80, 24)
	row := cliToolsSnapshotRow(view, "Claude Code")
	if !strings.Contains(row, "installed") {
		t.Fatalf("Claude context row %q does not reflect typed installed truth", row)
	}
	if !strings.Contains(strings.ToLower(stripANSITest(view)), "configure from the dedicated claude code screen") {
		t.Fatalf("Claude context row does not direct users to its dedicated screen:\n%s", stripANSITest(view))
	}

	beforeIndex := ctx.app.cliToolIndex
	beforeSelections := maps.Clone(ctx.app.deepDiveConfig.CLITools)
	rowY := labelLineY(t, view, "Claude Code")
	if rowY < 0 || rowY >= 24 {
		t.Fatalf("Claude context row y=%d outside viewport", rowY)
	}
	screen.Update(clickAt(40, rowY))
	screen.Update(keyMsg("down"))
	if ctx.app.cliToolIndex != beforeIndex {
		t.Fatalf("Claude context row became navigable: index=%d want=%d", ctx.app.cliToolIndex, beforeIndex)
	}
	if !reflect.DeepEqual(ctx.app.deepDiveConfig.CLITools, beforeSelections) {
		t.Fatalf("Claude context interaction mutated selections: before=%v after=%v", beforeSelections, ctx.app.deepDiveConfig.CLITools)
	}
}

func TestCLIToolsSnapshotStatusMeaningSurvivesANSIRemoval(t *testing.T) {
	ctx := cursorAgentBareContext(t)
	setManageTruthSnapshot(t, ctx.app, 207, pkg.PlatformMacOS, "brew",
		manageTruthObservation(t, "codex", health.PresencePresent),
		manageTruthObservation(t, "opencode", health.PresencePartial),
		manageTruthObservation(t, "pi", health.PresenceMissing),
	)
	ctx.app.cliToolIndex = cliToolIndexForTest(t, "opencode")
	view := NewConfigCLIToolsScreen(ctx).View(120, 40)
	plain := strings.ToLower(stripANSITest(view))
	for _, text := range []string{"installed", "partial — repair", "missing — install"} {
		if !strings.Contains(plain, text) {
			t.Errorf("ANSI-stripped view missing text status %q:\n%s", text, stripANSITest(view))
		}
	}
	for _, line := range strings.Split(plain, "\n") {
		if !strings.Contains(line, "navigate") {
			continue
		}
		if strings.Contains(line, "yellow") || strings.Contains(line, "cyan") || strings.Contains(line, "magenta") {
			t.Fatalf("footer relies on color for status meaning: %q", strings.TrimSpace(line))
		}
		if !strings.Contains(line, "status") && !strings.Contains(line, "installed") {
			t.Fatalf("footer omits textual status guidance: %q", strings.TrimSpace(line))
		}
		return
	}
	t.Fatal("CLI tools footer missing")
}

func cliToolsSnapshotRow(view, name string) string {
	for _, line := range strings.Split(stripANSITest(view), "\n") {
		if strings.Contains(strings.ToLower(line), strings.ToLower(name)) {
			return strings.ToLower(strings.Join(strings.Fields(line), " "))
		}
	}
	return ""
}

func assertCLIToolsSnapshotFits(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("rendered %d lines at %dx%d, want at most %d:\n%s", len(lines), width, height, height, stripANSITest(view))
	}
	for lineNumber, line := range lines {
		if got := lipgloss.Width(stripANSITest(line)); got > width {
			t.Fatalf("line %d width=%d exceeds %d at %dx%d: %q", lineNumber+1, got, width, width, height, stripANSITest(line))
		}
	}
}

func cliToolIndexForTest(t *testing.T, id string) int {
	t.Helper()
	for index, item := range cliToolItems {
		if item.id == id && index < navigableCLIToolCount {
			return index
		}
	}
	t.Fatalf("navigable CLI tool %q not found", id)
	return -1
}
