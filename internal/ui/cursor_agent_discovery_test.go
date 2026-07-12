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

const cursorAgentPendingReason = "verified artifact support pending"

func cursorAgentManageContext(t *testing.T) (*ScreenContext, *App) {
	t.Helper()
	ctx := cursorAgentBareContext(t)
	app := ctx.app
	agent := tools.NewCursorAgentTool()
	desktop := tools.NewCursorTool()
	manager := pkg.NewMockPackageManager()
	manager.ManagerName = "brew"
	snapshot, err := tools.ObserveInstallationHealth(context.Background(), []tools.Tool{agent}, manager, pkg.PlatformMacOS, 111)
	if err != nil {
		t.Fatal(err)
	}
	observation, ok := snapshot.Tool("cursor-agent")
	if !ok || observation.Presence() != health.PresenceUnknown || observation.Installability() != health.InstallabilityUnsupported {
		t.Fatalf("Cursor Agent health=(present=%v,presence=%q,installability=%q)", ok, observation.Presence(), observation.Installability())
	}
	app.manageToolSource = func() []tools.Tool { return []tools.Tool{desktop, agent} }
	setManageTruthSnapshot(t, app, 111, pkg.PlatformMacOS, "brew", observation)
	selectManageTruthItem(t, app, "cursor-agent")
	app.managePane = managePaneTools
	return ctx, app
}

func cursorAgentBareContext(t *testing.T) *ScreenContext {
	t.Helper()
	withTempHome(t)
	app := NewApp(true)
	return &ScreenContext{app: app, Theme: "neon-seapunk", NavStyle: "emacs", AnimationsEnabled: false, Width: 80, Height: 24}
}

func exactVisibleLine(view, exact string) string {
	for _, line := range strings.Split(stripANSITest(view), "\n") {
		if normalized := strings.Join(strings.Fields(line), " "); normalized == exact {
			return normalized
		}
	}
	return ""
}

func cursorAgentSelectorRow(app *App, view string) string {
	for _, line := range manageSelectorLines(app, strings.ToUpper(stripANSITest(view))) {
		normalized := strings.Join(strings.Fields(line), " ")
		if strings.Contains(normalized, "▸") && strings.Contains(normalized, "[AI] CURSOR AGENT") && strings.Contains(normalized, "STATUS UNKNOWN") {
			return normalized
		}
	}
	return ""
}

func cursorAgentDeepDiveRow(view string) string {
	for _, line := range strings.Split(stripANSITest(view), "\n") {
		normalized := strings.Join(strings.Fields(line), " ")
		if strings.Contains(normalized, "Cursor Agent") && strings.Contains(strings.ToLower(normalized), "unavailable") {
			return normalized
		}
	}
	return ""
}

func cursorAgentHomeInventory(t *testing.T) []string {
	t.Helper()
	home := os.Getenv("HOME")
	var inventory []string
	if err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(home, path)
		if err != nil {
			return err
		}
		inventory = append(inventory, relative)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(inventory)
	return inventory
}

func TestCursorAgentManageSelectorIsDistinctTruthfulAndResponsive(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			ctx, app := cursorAgentManageContext(t)
			app.width, app.height = size.width, size.height
			ctx.Width, ctx.Height = size.width, size.height
			beforeInventory := cursorAgentHomeInventory(t)
			beforeDigest := app.installationSnapshot.Digest()
			view := NewManageScreen(ctx).View(size.width, size.height)
			row := cursorAgentSelectorRow(app, view)
			if row == "" {
				t.Fatalf("Cursor Agent selector row=%q", row)
			}
			if exactVisibleLine(view, cursorAgentPendingReason) == "" {
				t.Fatalf("Cursor Agent view omitted exact pending reason line:\n%s", stripANSITest(view))
			}
			items := app.manageItems()
			counts := map[string]int{}
			for _, item := range items {
				counts[item.id]++
			}
			if counts["cursor-agent"] != 1 || counts["cursor"] != 1 {
				t.Fatalf("Cursor identities are not distinct: %v", counts)
			}
			if !slices.Equal(beforeInventory, cursorAgentHomeInventory(t)) || app.installationSnapshot.Digest() != beforeDigest || app.pendingInstallPlan != nil {
				t.Fatal("Cursor Agent render mutated HOME, installation authority, or pending plan")
			}
		})
	}
}

func TestCursorAgentWideSelectorPreservesRawIconPendingMarkAndTruth(t *testing.T) {
	ctx, app := cursorAgentManageContext(t)
	app.width, app.height = 120, 40
	ctx.Width, ctx.Height = 120, 40
	items := app.manageItems()
	layout := app.manageLayout()
	if layout.leftW != 48 || layout.rightW != 71 {
		t.Fatalf("120-column Manage split=(left=%d,right=%d), want 48/71", layout.leftW, layout.rightW)
	}
	rawPanel := app.renderManageToolsPanel(layout, items)
	var rawRow string
	for _, line := range strings.Split(rawPanel, "\n") {
		if strings.Contains(stripANSITest(line), "Cursor Agent") {
			rawRow = line
			break
		}
	}
	if rawRow == "" {
		t.Fatal("120-column raw selector omitted Cursor Agent row")
	}
	plain := strings.ToUpper(strings.Join(strings.Fields(stripANSITest(rawRow)), " "))
	if !strings.Contains(plain, "[AI] CURSOR AGENT") || !strings.Contains(plain, "STATUS UNKNOWN") {
		t.Fatalf("120-column Cursor Agent row=%q, want type/name/full health", plain)
	}
	agent := tools.NewCursorAgentTool()
	if agent.Icon() == "" || !strings.Contains(rawRow, agent.Icon()) {
		t.Fatalf("120-column Cursor Agent row omitted exact nonempty icon %q", agent.Icon())
	}
	if pending := StatusDot("pending"); pending == "" || !strings.Contains(rawRow, pending) {
		t.Fatalf("120-column Cursor Agent row omitted semantic pending mark %q", stripANSITest(pending))
	}
}

func TestCursorAgentManageInstallKeysFailClosedFromEitherPane(t *testing.T) {
	for _, pane := range []int{managePaneTools, managePaneSettings} {
		for _, key := range []string{"i", "I"} {
			t.Run(key+map[bool]string{true: "/tools", false: "/settings"}[pane == managePaneTools], func(t *testing.T) {
				ctx, app := cursorAgentManageContext(t)
				app.managePane = pane
				beforeInventory := cursorAgentHomeInventory(t)
				beforeDigest := app.installationSnapshot.Digest()
				if cmd := NewManageScreen(ctx).handleKey(keyMsg(key)); cmd != nil || app.pendingInstallPlan != nil || app.manageStatus != cursorAgentPendingReason {
					t.Fatalf("Cursor Agent blocked install=(cmd=%v,plan=%v,status=%q)", cmd != nil, app.pendingInstallPlan != nil, app.manageStatus)
				}
				if !slices.Equal(beforeInventory, cursorAgentHomeInventory(t)) || app.installationSnapshot.Digest() != beforeDigest {
					t.Fatal("Cursor Agent blocked action mutated HOME or installation authority")
				}
			})
		}
	}
}

func TestCursorAgentDeepDiveRowIsUniqueInspectableDisabledAndPreservesIntegrationParity(t *testing.T) {
	registry := tools.NewRegistry()
	for _, id := range []string{"cursor", "cursor-agent", "codex", "opencode", "pi", "t3-code"} {
		if _, ok := registry.Get(id); !ok {
			t.Fatalf("registry missing %q", id)
		}
	}
	count := 0
	agentIndex := -1
	for index, item := range cliToolItems {
		if item.id == "cursor" {
			t.Fatal("Cursor desktop was conflated with the CLI Tools selector")
		}
		if item.id == "cursor-agent" {
			count++
			agentIndex = index
			if item.name != "Cursor Agent" || item.desc != cursorAgentPendingReason {
				t.Fatalf("Cursor Agent deep-dive metadata=%q/%q", item.name, item.desc)
			}
		}
	}
	if count != 1 || agentIndex < 0 || agentIndex >= navigableCLIToolCount {
		t.Fatalf("Cursor Agent deep-dive row count=%d index=%d navigable=%d", count, agentIndex, navigableCLIToolCount)
	}
	var wantCLI []string
	for _, tool := range registry.All() {
		if tool.UIGroup() == tools.UIGroupCLITools {
			wantCLI = append(wantCLI, tool.ID())
		}
	}
	gotCLI := make([]string, 0, len(cliToolItems))
	for _, item := range cliToolItems {
		gotCLI = append(gotCLI, item.id)
	}
	slices.Sort(wantCLI)
	slices.Sort(gotCLI)
	if !slices.Equal(gotCLI, wantCLI) {
		t.Fatalf("CLI Tools selector/registry parity mismatch: rows=%v registry=%v", gotCLI, wantCLI)
	}
	ctx := cursorAgentBareContext(t)
	ctx.app.installCacheLoading = false
	ctx.app.manageInstalled = make(map[string]bool)
	ctx.app.deepDiveConfig.CLITools = make(map[string]bool)
	ctx.app.manageInstalled["cursor-agent"] = false
	ctx.app.deepDiveConfig.CLITools["cursor-agent"] = false
	ctx.app.cliToolIndex = agentIndex
	screen := NewConfigCLIToolsScreen(ctx)
	_, _ = screen.Update(keyMsg(" "))
	if ctx.app.deepDiveConfig.CLITools["cursor-agent"] {
		t.Fatal("unavailable Cursor Agent row toggled into the install selection")
	}
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		ctx.Width, ctx.Height = size.width, size.height
		ctx.app.width, ctx.app.height = size.width, size.height
		ctx.app.cliToolIndex = agentIndex
		view := screen.View(size.width, size.height)
		if row := cursorAgentDeepDiveRow(view); row == "" {
			t.Errorf("%dx%d CLI Tools omitted scoped Cursor Agent unavailable row", size.width, size.height)
		}
		if exactVisibleLine(view, cursorAgentPendingReason) == "" {
			t.Errorf("%dx%d CLI Tools omitted exact focused pending detail", size.width, size.height)
		}
	}
}

func TestCursorAgentDeepDiveMouseGeometryRemainsExactWithFocusedReason(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			ctx := cursorAgentBareContext(t)
			ctx.Width, ctx.Height = size.width, size.height
			ctx.app.width, ctx.app.height = size.width, size.height
			ctx.app.installCacheLoading = false
			ctx.app.manageInstalled = make(map[string]bool)
			ctx.app.deepDiveConfig.CLITools = make(map[string]bool)
			agentIndex, adjacentIndex := -1, -1
			for index, item := range cliToolItems {
				if item.id == "cursor-agent" {
					agentIndex = index
					adjacentIndex = index - 1
				}
			}
			if agentIndex <= 0 || adjacentIndex < 0 {
				t.Fatal("Cursor Agent has no navigable adjacent fixture row")
			}
			ctx.app.cliToolIndex = agentIndex
			screen := NewConfigCLIToolsScreen(ctx)
			view := screen.View(size.width, size.height)
			layout := ctx.app.configFieldLayout
			x := (layout.boxLeft + layout.boxRight) / 2
			agentY := labelLineY(t, view, "Cursor Agent")
			adjacentY := labelLineY(t, view, cliToolItems[adjacentIndex].name)
			if agentY < 0 || adjacentY < 0 {
				t.Fatalf("visible geometry missing agent/adjacent rows: %d/%d", agentY, adjacentY)
			}
			screen.Update(clickAt(x, agentY))
			if ctx.app.cliToolIndex != agentIndex {
				t.Fatalf("Cursor Agent click selected %d, want %d", ctx.app.cliToolIndex, agentIndex)
			}
			screen.Update(keyMsg(" "))
			if ctx.app.deepDiveConfig.CLITools["cursor-agent"] {
				t.Fatal("Cursor Agent click/Space toggled unavailable row")
			}
			screen.Update(clickAt(x, adjacentY))
			if ctx.app.cliToolIndex != adjacentIndex {
				t.Fatalf("adjacent click selected %d, want %d", ctx.app.cliToolIndex, adjacentIndex)
			}
			cursorBefore := ctx.app.deepDiveConfig.CLITools["cursor-agent"]
			screen.Update(keyMsg(" "))
			if ctx.app.deepDiveConfig.CLITools["cursor-agent"] != cursorBefore {
				t.Fatal("adjacent row action mutated Cursor Agent selection")
			}
		})
	}
}

func TestCursorAgentDocumentationIsObservationOnlyAndCurrent(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "tools.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(content))
	for _, want := range []string{"35 tools", "cursor agent", "observation-only", cursorAgentPendingReason, "no automatic install"} {
		if !strings.Contains(text, want) {
			t.Errorf("docs/tools.md omits Cursor Agent contract %q", want)
		}
	}
}
