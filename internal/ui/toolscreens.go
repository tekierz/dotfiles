package ui

import (
	"fmt"

	"github.com/tekierz/dotfiles/internal/tools"
)

// toolConfigScreens is the authoritative mapping from a tool's ID to its
// dedicated config Screen constant.
//
// The tools package stores ConfigScreen() as a raw int (it cannot import the
// ui package without creating an import cycle), and those ints are coupled to
// this package's Screen iota. This map names the Screen constants symbolically
// so the coupling lives in exactly one place: if the iota is ever reordered,
// verifyToolConfigScreens() (called from NewApp and the tests) panics, catching
// the drift immediately instead of silently routing to the wrong screen.
//
// Only tools with a dedicated config screen are listed. Tools that are part of
// a group screen report ConfigScreen() == 0 and are intentionally omitted.
var toolConfigScreens = map[string]Screen{
	"ghostty":     ScreenConfigGhostty,
	"tmux":        ScreenConfigTmux,
	"zsh":         ScreenConfigZsh,
	"neovim":      ScreenConfigNeovim,
	"git":         ScreenConfigGit,
	"yazi":        ScreenConfigYazi,
	"fzf":         ScreenConfigFzf,
	"lazygit":     ScreenConfigLazyGit,
	"btop":        ScreenConfigBtop,
	"glow":        ScreenConfigGlow,
	"claude-code": ScreenConfigClaudeCode,
}

// screenIsToolConfig reports whether the given screen is a dedicated per-tool
// config screen (i.e. one reachable via `dotfiles config <tool>`). Used to mark
// standalone config sessions so they persist edits on exit (C27).
func screenIsToolConfig(screen Screen) bool {
	for _, s := range toolConfigScreens {
		if s == screen {
			return true
		}
	}
	return false
}

// toolIDForScreen returns the tool ID whose dedicated config screen is the given
// screen, and true, or "" and false when the screen is not a per-tool config
// screen. It is the inverse of toolConfigScreens, used by the standalone
// `dotfiles config <tool>` exit to scope the write to exactly the opened tool.
func toolIDForScreen(screen Screen) (string, bool) {
	for id, s := range toolConfigScreens {
		if s == screen {
			return id, true
		}
	}
	return "", false
}

// verifyToolConfigScreens asserts that the authoritative toolConfigScreens map
// agrees with the raw ConfigScreen() ints declared in the tools registry. It
// panics on any disagreement so an iota reorder or a stale tool definition is
// caught at startup (and in tests) rather than producing a silent misroute.
func verifyToolConfigScreens() {
	for _, t := range tools.GetRegistry().All() {
		raw := t.ConfigScreen()
		want, mapped := toolConfigScreens[t.ID()]

		if raw == 0 {
			// Tool is part of a group screen; it must not be in the map.
			if mapped {
				panic(fmt.Sprintf(
					"toolConfigScreens: tool %q has ConfigScreen()==0 (group screen) but is listed in the map as %d",
					t.ID(), want,
				))
			}
			continue
		}

		// Tool has a dedicated screen: it must be in the map and the int must match.
		if !mapped {
			panic(fmt.Sprintf(
				"toolConfigScreens: tool %q has a dedicated ConfigScreen()==%d but is missing from the map",
				t.ID(), raw,
			))
		}
		if int(want) != raw {
			panic(fmt.Sprintf(
				"toolConfigScreens: tool %q ConfigScreen()==%d disagrees with map entry (%d); the Screen iota likely changed",
				t.ID(), raw, int(want),
			))
		}
	}
}
