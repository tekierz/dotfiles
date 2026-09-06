package ui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

const hermesPendingReason = "verified artifact and architecture support pending"

func hermesManageContext(t *testing.T) (*ScreenContext, *App) {
	t.Helper()
	ctx := cursorAgentBareContext(t)
	app := ctx.app
	tool := tools.NewHermesAgentTool()
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	snapshot, err := tools.ObserveInstallationHealth(context.Background(), []tools.Tool{tool}, manager, pkg.PlatformMacOS, 131)
	if err != nil {
		t.Fatal(err)
	}
	observation, ok := snapshot.Tool("hermes")
	if !ok || observation.Presence() != health.PresenceUnknown || observation.Installability() != health.InstallabilityUnsupported {
		t.Fatalf("Hermes health=(found=%v,presence=%q,installability=%q)", ok, observation.Presence(), observation.Installability())
	}
	app.manageToolSource = func() []tools.Tool { return []tools.Tool{tool} }
	setManageTruthSnapshot(t, app, 131, pkg.PlatformMacOS, "brew", observation)
	selectManageTruthItem(t, app, "hermes")
	app.managePane = managePaneTools
	return ctx, app
}

func hermesSelectorRow(app *App, view string) string {
	for _, line := range manageSelectorLines(app, strings.ToUpper(stripANSITest(view))) {
		normalized := strings.Join(strings.Fields(line), " ")
		if strings.Contains(normalized, "▸") && strings.Contains(normalized, "[AI] HERMES AGENT") && strings.Contains(normalized, "STATUS UNKNOWN") {
			return normalized
		}
	}
	return ""
}

func TestHermesManageDiscoveryIsResponsiveAndFailClosed(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			ctx, app := hermesManageContext(t)
			ctx.Width, ctx.Height = size.width, size.height
			app.width, app.height = size.width, size.height
			beforeInventory := cursorAgentHomeInventory(t)
			beforeDigest := app.installationSnapshot.Digest()
			view := NewManageScreen(ctx).View(size.width, size.height)
			if row := hermesSelectorRow(app, view); row == "" {
				t.Fatalf("Hermes Manage selector omitted scoped [AI]/unknown row:\n%s", stripANSITest(view))
			}
			if exactVisibleLine(view, hermesPendingReason) == "" {
				t.Fatalf("Hermes Manage omitted exact pending reason line:\n%s", stripANSITest(view))
			}
			if !slices.Equal(beforeInventory, cursorAgentHomeInventory(t)) || app.installationSnapshot.Digest() != beforeDigest || app.pendingInstallPlan != nil {
				t.Fatal("Hermes Manage render mutated HOME, snapshot authority, or pending plan")
			}
		})
	}

	for _, pane := range []int{managePaneTools, managePaneSettings} {
		for _, key := range []string{"i", "I"} {
			t.Run(key+map[bool]string{true: "/tools", false: "/settings"}[pane == managePaneTools], func(t *testing.T) {
				ctx, app := hermesManageContext(t)
				app.managePane = pane
				beforeInventory := cursorAgentHomeInventory(t)
				beforeDigest := app.installationSnapshot.Digest()
				if cmd := NewManageScreen(ctx).handleKey(keyMsg(key)); cmd != nil || app.pendingInstallPlan != nil || app.manageStatus != hermesPendingReason {
					t.Fatalf("Hermes blocked install=(cmd=%v,plan=%v,status=%q)", cmd != nil, app.pendingInstallPlan != nil, app.manageStatus)
				}
				if !slices.Equal(beforeInventory, cursorAgentHomeInventory(t)) || app.installationSnapshot.Digest() != beforeDigest {
					t.Fatal("Hermes blocked action mutated HOME or snapshot authority")
				}
			})
		}
	}
}

func TestHermesCLIRegistryParityAndMouseGeometry(t *testing.T) {
	registry := tools.NewRegistry()
	var want, got []string
	hermesCount := 0
	for _, tool := range registry.All() {
		if tool.UIGroup() == tools.UIGroupCLITools {
			want = append(want, tool.ID())
		}
		if tool.ID() == "hermes" {
			hermesCount++
		}
	}
	for _, item := range cliToolItems {
		got = append(got, item.id)
	}
	slices.Sort(want)
	slices.Sort(got)
	if hermesCount != 1 || !slices.Equal(got, want) {
		t.Fatalf("Hermes/CLI parity count=%d rows=%v registry=%v", hermesCount, got, want)
	}

	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			ctx := cursorAgentBareContext(t)
			ctx.Width, ctx.Height = size.width, size.height
			ctx.app.width, ctx.app.height = size.width, size.height
			ctx.app.installCacheLoading = false
			ctx.app.manageInstalled = make(map[string]bool)
			ctx.app.deepDiveConfig.CLITools = make(map[string]bool)
			setManageTruthSnapshot(t, ctx.app, 131, pkg.PlatformMacOS, "brew", planningCacheObservation(t, tools.NewHermesAgentTool(), false))
			hermesIndex := -1
			for index, item := range cliToolItems {
				if item.id == "hermes" {
					hermesIndex = index
				}
			}
			if hermesIndex <= 0 {
				t.Fatal("Hermes has no adjacent CLI row")
			}
			adjacentIndex := hermesIndex - 1
			ctx.app.cliToolIndex = hermesIndex
			screen := NewConfigCLIToolsScreen(ctx)
			view := screen.View(size.width, size.height)
			x := (ctx.app.configFieldLayout.boxLeft + ctx.app.configFieldLayout.boxRight) / 2
			hermesY := labelLineY(t, view, "Hermes Agent")
			adjacentY := labelLineY(t, view, cliToolItems[adjacentIndex].name)
			if hermesY < 0 || adjacentY < 0 {
				t.Fatalf("Hermes/adjacent visible rows=%d/%d", hermesY, adjacentY)
			}
			screen.Update(clickAt(x, hermesY))
			if ctx.app.cliToolIndex != hermesIndex {
				t.Fatalf("Hermes click selected %d, want %d", ctx.app.cliToolIndex, hermesIndex)
			}
			screen.Update(keyMsg(" "))
			if ctx.app.deepDiveConfig.CLITools["hermes"] {
				t.Fatal("Hermes click/Space toggled unavailable row")
			}
			screen.Update(clickAt(x, adjacentY))
			if ctx.app.cliToolIndex != adjacentIndex {
				t.Fatalf("adjacent click selected %d, want %d", ctx.app.cliToolIndex, adjacentIndex)
			}
			screen.Update(keyMsg(" "))
			if ctx.app.deepDiveConfig.CLITools["hermes"] {
				t.Fatal("adjacent action mutated Hermes selection")
			}
		})
	}
}

func TestHermesDocumentationAndDeferredProfileContract(t *testing.T) {
	docs, err := os.ReadFile(filepath.Join("..", "..", "docs", "tools.md"))
	if err != nil {
		t.Fatal(err)
	}
	docsText := strings.ToLower(string(docs))
	hermesParagraph := scopedParagraph(docsText, "hermes agent")
	if hermesParagraph == "" {
		t.Fatal("docs/tools.md has no Hermes Agent paragraph")
	}
	for _, want := range []string{"36 tools", "hermes agent", "observation-only", hermesPendingReason, "no automatic install", "does not claim installed state, configuration, credentials, platform support, or architecture support"} {
		target := hermesParagraph
		if want == "36 tools" {
			target = docsText
		}
		if !strings.Contains(target, want) {
			t.Errorf("docs/tools.md Hermes paragraph omits contract %q: %q", want, hermesParagraph)
		}
	}
	todo, err := os.ReadFile(filepath.Join("..", "..", "tasks", "todo.md"))
	if err != nil {
		t.Fatal(err)
	}
	todoText := strings.ToLower(string(todo))
	checkpoint := scopedSection(todoText, "### current product-gap checkpoint", "\n## ")
	if checkpoint == "" {
		t.Fatal("tasks/todo.md has no current product-gap checkpoint")
	}
	for _, want := range []string{"hermes discovery-only", "unknown/unsupported", "installation-profile", "deferred"} {
		if !strings.Contains(checkpoint, want) {
			t.Errorf("tasks/todo.md current checkpoint omits deferred Hermes/profile contract %q", want)
		}
	}
	foundUncheckedProfile := false
	for _, line := range strings.Split(checkpoint, "\n") {
		if strings.Contains(line, "installation-profile") || strings.Contains(line, "installation profiles") {
			if strings.Contains(line, "- [x]") {
				t.Fatalf("installation-profile implementation was marked complete: %q", line)
			}
			if strings.Contains(line, "- [ ]") {
				foundUncheckedProfile = true
			}
		}
	}
	if !foundUncheckedProfile {
		t.Fatal("current checkpoint has no unchecked installation-profile item")
	}
}

func scopedParagraph(text, prefix string) string {
	for _, paragraph := range strings.Split(text, "\n\n") {
		trimmed := strings.TrimSpace(paragraph)
		if strings.HasPrefix(trimmed, prefix) {
			return trimmed
		}
	}
	return ""
}

func scopedSection(text, startMarker, endMarker string) string {
	start := strings.Index(text, startMarker)
	if start < 0 {
		return ""
	}
	remainder := text[start:]
	if end := strings.Index(remainder, endMarker); end > 0 {
		return remainder[:end]
	}
	return remainder
}

func TestHermesCLIInspectionRowIsOrderedDisabledAndResponsive(t *testing.T) {
	hermesIndex, cursorIndex, claudeIndex := -1, -1, -1
	for index, item := range cliToolItems {
		switch item.id {
		case "hermes":
			hermesIndex = index
			if item.name != "Hermes Agent" || item.desc != hermesPendingReason {
				t.Fatalf("Hermes row metadata=%q/%q", item.name, item.desc)
			}
		case "cursor-agent":
			cursorIndex = index
		case "claude-code":
			claudeIndex = index
		}
	}
	if hermesIndex != cursorIndex+1 || claudeIndex != hermesIndex+1 || navigableCLIToolCount != 9 || hermesIndex >= navigableCLIToolCount {
		t.Fatalf("Hermes order cursor/hermes/claude=%d/%d/%d navigable=%d", cursorIndex, hermesIndex, claudeIndex, navigableCLIToolCount)
	}
	keyboardCtx := cursorAgentBareContext(t)
	keyboardCtx.app.installCacheLoading = false
	keyboardCtx.app.manageInstalled = make(map[string]bool)
	keyboardCtx.app.deepDiveConfig.CLITools = make(map[string]bool)
	setManageTruthSnapshot(t, keyboardCtx.app, 131, pkg.PlatformMacOS, "brew", planningCacheObservation(t, tools.NewHermesAgentTool(), false))
	keyboardCtx.app.cliToolIndex = cursorIndex
	keyboardScreen := NewConfigCLIToolsScreen(keyboardCtx)
	keyboardScreen.Update(keyMsg("down"))
	if keyboardCtx.app.cliToolIndex != hermesIndex {
		t.Fatalf("Down from Cursor Agent selected %d, want Hermes %d", keyboardCtx.app.cliToolIndex, hermesIndex)
	}
	keyboardScreen.Update(keyMsg(" "))
	if keyboardCtx.app.deepDiveConfig.CLITools["hermes"] {
		t.Fatal("keyboard Space toggled unavailable Hermes row")
	}
	keyboardScreen.Update(keyMsg("down"))
	if keyboardCtx.app.cliToolIndex != hermesIndex {
		t.Fatalf("Down from Hermes entered nonnavigable Claude row: got %d want %d", keyboardCtx.app.cliToolIndex, hermesIndex)
	}
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			ctx := cursorAgentBareContext(t)
			ctx.Width, ctx.Height = size.width, size.height
			ctx.app.width, ctx.app.height = size.width, size.height
			ctx.app.installCacheLoading = false
			ctx.app.manageInstalled = make(map[string]bool)
			ctx.app.deepDiveConfig.CLITools = make(map[string]bool)
			setManageTruthSnapshot(t, ctx.app, 131, pkg.PlatformMacOS, "brew", planningCacheObservation(t, tools.NewHermesAgentTool(), false))
			ctx.app.cliToolIndex = hermesIndex
			screen := NewConfigCLIToolsScreen(ctx)
			view := screen.View(size.width, size.height)
			row := ""
			for _, line := range strings.Split(stripANSITest(view), "\n") {
				normalized := strings.Join(strings.Fields(line), " ")
				if strings.Contains(normalized, "Hermes Agent") && strings.Contains(strings.ToLower(normalized), "unavailable") {
					row = normalized
				}
			}
			if row == "" || exactVisibleLine(view, hermesPendingReason) == "" {
				t.Fatalf("Hermes CLI row/detail missing: row=%q\n%s", row, stripANSITest(view))
			}
			screen.Update(keyMsg(" "))
			if ctx.app.deepDiveConfig.CLITools["hermes"] {
				t.Fatal("Hermes unavailable row toggled")
			}
		})
	}
}
