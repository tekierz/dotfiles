package ui

import (
	"testing"

	"github.com/tekierz/dotfiles/internal/tools"
)

// TestToolConfigScreensAgreeWithRegistry verifies the authoritative
// toolConfigScreens map agrees with the raw ConfigScreen() ints declared in the
// tools registry. This is the regression guard for the Screen iota / tool int
// coupling: if the iota is reordered (or a tool's int goes stale), this fails.
func TestToolConfigScreensAgreeWithRegistry(t *testing.T) {
	// verifyToolConfigScreens panics on any disagreement.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("toolConfigScreens disagrees with registry: %v", r)
		}
	}()
	verifyToolConfigScreens()
}

// TestGetToolConfigScreenAgreesWithMap verifies the CLI-facing
// GetToolConfigScreen delegates to the authoritative toolConfigScreens map
// rather than keeping a divergent private copy (C28). Before the fix the CLI
// advertised tools it could not open (e.g. "apps") and omitted real ones
// (lazygit, btop, glow, claude-code).
func TestGetToolConfigScreenAgreesWithMap(t *testing.T) {
	// Every authoritative entry must resolve to the same screen via the CLI path.
	for id, want := range toolConfigScreens {
		got, ok := GetToolConfigScreen(id)
		if !ok {
			t.Errorf("GetToolConfigScreen(%q) returned not-ok, but it is in toolConfigScreens", id)
			continue
		}
		if got != want {
			t.Errorf("GetToolConfigScreen(%q) = %d, want %d", id, got, want)
		}
	}

	// The CLI must not open tools that are not in the authoritative map. The old
	// private map exposed "apps" and "utilities" which are not real config
	// screens; assert they are rejected now.
	for _, bad := range []string{"apps", "utilities", "nonexistent"} {
		if _, ok := GetToolConfigScreen(bad); ok {
			t.Errorf("GetToolConfigScreen(%q) unexpectedly returned ok", bad)
		}
	}

	// ConfigurableToolIDs must exactly cover the authoritative map.
	ids := ConfigurableToolIDs()
	if len(ids) != len(toolConfigScreens) {
		t.Errorf("ConfigurableToolIDs() has %d entries, map has %d", len(ids), len(toolConfigScreens))
	}
	for _, id := range ids {
		if _, ok := toolConfigScreens[id]; !ok {
			t.Errorf("ConfigurableToolIDs() includes %q which is not in toolConfigScreens", id)
		}
	}
}

// TestToolConfigScreensExactMatch checks each direction explicitly so a failure
// reports the specific tool rather than only that the map drifted.
func TestToolConfigScreensExactMatch(t *testing.T) {
	reg := tools.GetRegistry()

	// Every dedicated-screen tool must have a matching map entry.
	for _, tool := range reg.All() {
		raw := tool.ConfigScreen()
		want, ok := toolConfigScreens[tool.ID()]
		if raw == 0 {
			if ok {
				t.Errorf("tool %q is a group tool (ConfigScreen()==0) but is in toolConfigScreens", tool.ID())
			}
			continue
		}
		if !ok {
			t.Errorf("tool %q has dedicated ConfigScreen()==%d but is missing from toolConfigScreens", tool.ID(), raw)
			continue
		}
		if int(want) != raw {
			t.Errorf("tool %q: ConfigScreen()==%d but map says %d", tool.ID(), raw, int(want))
		}
	}

	// Every map entry must correspond to a real tool with that dedicated screen.
	for id, screen := range toolConfigScreens {
		tool, ok := reg.Get(id)
		if !ok {
			t.Errorf("toolConfigScreens has entry %q that is not a registered tool", id)
			continue
		}
		if int(screen) != tool.ConfigScreen() {
			t.Errorf("toolConfigScreens[%q]==%d but tool reports ConfigScreen()==%d", id, int(screen), tool.ConfigScreen())
		}
	}
}
