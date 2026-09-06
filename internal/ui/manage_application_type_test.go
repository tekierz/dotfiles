package ui

import (
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/tekierz/dotfiles/internal/health"
	"github.com/tekierz/dotfiles/internal/pkg"
	"github.com/tekierz/dotfiles/internal/tools"
)

type applicationTypeOverrideTool struct {
	tools.Tool
	typeValue tools.ApplicationType
}

func (tool applicationTypeOverrideTool) ApplicationType() tools.ApplicationType {
	return tool.typeValue
}

func stripANSIAndPUA(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Co, r) {
			return -1
		}
		return r
	}, stripANSITest(value))
}

func TestManageApplicationTypesSortCanonicallyFromScrambledSourceWithUnknownLast(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	unknown := applicationTypeOverrideTool{Tool: tools.NewFzfTool(), typeValue: tools.ApplicationType("future-type")}
	source := []tools.Tool{
		unknown,
		tools.NewIINATool(),
		tools.NewTailscaleTool(),
		tools.NewPiTool(),
		tools.NewLazyGitTool(),
		tools.NewYaziTool(),
		tools.NewNeovimTool(),
		tools.NewGhosttyTool(),
		tools.NewZshTool(),
	}
	app.manageToolSource = func() []tools.Tool { return source }
	observations := make([]health.InstallationObservation, 0, len(source))
	for _, tool := range source {
		observations = append(observations, manageTruthObservation(t, tool.ID(), health.PresencePresent))
	}
	setManageTruthSnapshot(t, app, 41, pkg.PlatformMacOS, "brew", observations...)

	items := app.manageItems()
	wantIDs := []string{"global", "zsh", "ghostty", "neovim", "yazi", "lazygit", "pi", "tailscale", "iina", "fzf"}
	if len(items) != len(wantIDs) {
		t.Fatalf("items=%d, want %d", len(items), len(wantIDs))
	}
	for index, want := range wantIDs {
		if items[index].id != want {
			t.Fatalf("items[%d]=%q, want %q; order=%v", index, items[index].id, want, manageItemIDs(items))
		}
	}
	if got := items[len(items)-1].applicationType; got != tools.ApplicationTypeUnknown {
		t.Fatalf("unknown-last item type=%q", got)
	}
	for index := 1; index < len(items); index++ {
		if items[index].id == "global" {
			t.Fatalf("global duplicated at row %d", index)
		}
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if seen[item.id] {
			t.Fatalf("duplicate selectable row for %q", item.id)
		}
		seen[item.id] = true
	}
}

func manageItemIDs(items []manageItem) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		ids[index] = item.id
	}
	return ids
}

func TestManageApplicationTypeTextSurvivesANSIAndGlyphRemovalAtSupportedSizes(t *testing.T) {
	for _, size := range []struct{ width, height int }{{60, 18}, {80, 24}, {120, 40}} {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			ctx := newGoldenContext(t)
			app := ctx.app
			app.manageToolSource = func() []tools.Tool { return []tools.Tool{tools.NewPiTool()} }
			setManageTruthSnapshot(t, app, 42, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "pi", health.PresenceMissing))
			app.manageIndex = 1
			app.managePane = managePaneTools
			visible := strings.ToUpper(stripANSIAndPUA(NewManageScreen(ctx).View(size.width, size.height)))
			var row string
			for _, line := range manageSelectorLines(app, visible) {
				if strings.Contains(line, "PI") && strings.Contains(line, "NOT INSTALLED") {
					row = strings.Join(strings.Fields(line), " ")
					break
				}
			}
			if row == "" {
				t.Fatalf("Pi row with complete health state missing:\n%s", visible)
			}
			if !strings.Contains(row, "[AI]") || !strings.Contains(row, "PI") || !strings.Contains(row, "NOT INSTALLED") {
				t.Fatalf("row=%q, want ASCII type + name + health", row)
			}
		})
	}
}

func manageSelectorLines(app *App, visible string) []string {
	lines := strings.Split(visible, "\n")
	if app.width <= 80 {
		return lines
	}
	leftWidth := app.manageLayout().leftW
	for index, line := range lines {
		runes := []rune(line)
		if len(runes) > leftWidth {
			runes = runes[:leftWidth]
		}
		lines[index] = string(runes)
	}
	return lines
}

func TestManageApplicationTypeRepresentativeWideSelectorRows(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	source := []tools.Tool{
		tools.NewIINATool(), tools.NewFzfTool(), tools.NewTailscaleTool(), tools.NewPiTool(),
		tools.NewLazyGitTool(), tools.NewYaziTool(), tools.NewNeovimTool(), tools.NewGhosttyTool(), tools.NewZshTool(),
	}
	app.manageToolSource = func() []tools.Tool { return source }
	observations := make([]health.InstallationObservation, 0, len(source))
	for _, tool := range source {
		observations = append(observations, manageTruthObservation(t, tool.ID(), health.PresencePresent))
	}
	setManageTruthSnapshot(t, app, 44, pkg.PlatformMacOS, "brew", observations...)
	app.managePane = managePaneTools
	visible := strings.ToUpper(stripANSIAndPUA(NewManageScreen(ctx).View(120, 40)))
	selector := strings.Join(manageSelectorLines(app, visible), "\n")
	wants := []string{
		"[SYS] ZSH", "[TERM] GHOSTTY", "[EDIT] NEOVIM", "[FILE] YAZI", "[DEV] LAZYGIT",
		"[AI] PI", "[SVC] TAILSCALE", "[UTIL] FZF", "[APP] IINA",
	}
	for _, want := range wants {
		if !strings.Contains(selector, want) {
			t.Errorf("wide selector missing textual type row %q:\n%s", want, selector)
		}
	}
}

func TestManageApplicationTypeSortPreservesKeyboardAndMouseRowMapping(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	source := []tools.Tool{tools.NewPiTool(), tools.NewNeovimTool(), tools.NewGhosttyTool(), tools.NewZshTool()}
	app.manageToolSource = func() []tools.Tool { return source }
	observations := make([]health.InstallationObservation, 0, len(source))
	for _, tool := range source {
		observations = append(observations, manageTruthObservation(t, tool.ID(), health.PresencePresent))
	}
	setManageTruthSnapshot(t, app, 45, pkg.PlatformMacOS, "brew", observations...)
	app.width, app.height = 120, 40
	ctx.Width, ctx.Height = 120, 40
	app.managePane = managePaneTools
	screen := NewManageScreen(ctx)
	items := app.manageItems()
	if got := manageItemIDs(items); strings.Join(got, ",") != "global,zsh,ghostty,neovim,pi" {
		t.Fatalf("unexpected sorted rows: %v", got)
	}
	app.manageIndex = 1
	_ = screen.handleKey(keyMsg("down"))
	if items[app.manageIndex].id != "ghostty" {
		t.Fatalf("down selected %q, want ghostty", items[app.manageIndex].id)
	}
	_ = screen.handleKey(keyMsg("up"))
	if items[app.manageIndex].id != "zsh" {
		t.Fatalf("up selected %q, want zsh", items[app.manageIndex].id)
	}
	_ = screen.View(120, 40)
	layout := app.manageLayout()
	target := 4
	_ = screen.handleMouse(clickAt(layout.leftX+1, layout.leftListY+(target-app.manageToolsScroll)))
	if app.manageIndex != target || items[app.manageIndex].id != "pi" {
		t.Fatalf("mouse selected index=%d id=%q, want row %d pi", app.manageIndex, items[app.manageIndex].id, target)
	}
}

func TestManageApplicationTypeRenderingPerformsNoOperationalProbes(t *testing.T) {
	ctx := newGoldenContext(t)
	app := ctx.app
	probe := &manageProbeTool{Tool: tools.NewPiTool()}
	app.manageToolSource = func() []tools.Tool { return []tools.Tool{probe} }
	setManageTruthSnapshot(t, app, 43, pkg.PlatformMacOS, "brew", manageTruthObservation(t, "pi", health.PresenceMissing))
	_ = NewManageScreen(ctx).View(120, 40)
	if probe.packageCalls != 0 || probe.isInstalledCalls != 0 || probe.installCalls != 0 || probe.directCalls != 0 || probe.recipeCalls != 0 {
		t.Fatalf("type rendering performed probes: %#v", probe)
	}
}
