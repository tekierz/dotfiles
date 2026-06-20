package ui

import (
	"strings"
	"testing"
)

// TestFieldNavFooterAccurate verifies that configFieldNav.footer() returns the
// centralized footer string and that it:
//   - does NOT contain "enter select" (the old misleading wording, C29)
//   - DOES contain "enter" (so the user knows enter is handled)
//   - DOES contain "esc" (back)
//   - DOES contain arrows for navigation
//
// Also checks that the method's output matches the canonical text constant.
func TestFieldNavFooterAccurate(t *testing.T) {
	nav := &configFieldNav{}
	got := stripANSITest(nav.footer())

	if strings.Contains(got, "enter select") {
		t.Errorf("fieldNavFooter must not claim 'enter select'; enter==back in handleMsg. Got: %q", got)
	}
	if !strings.Contains(got, "enter") {
		t.Errorf("fieldNavFooter must mention 'enter' (it is handled as back). Got: %q", got)
	}
	if !strings.Contains(got, "esc") {
		t.Errorf("fieldNavFooter must mention 'esc'. Got: %q", got)
	}
	if !strings.Contains(got, "↑") || !strings.Contains(got, "↓") {
		t.Errorf("fieldNavFooter must mention up/down navigation. Got: %q", got)
	}
	if !strings.Contains(got, fieldNavFooterText) {
		t.Errorf("fieldNavFooter output must contain canonical text %q. Got: %q", fieldNavFooterText, got)
	}
}

// TestListNavFooterAccurate verifies that configListNav.footer() returns the
// centralized footer string and that it:
//   - does NOT contain "save & back" (mode-dependent, misleading in wizard mode)
//   - DOES contain "enter" and "esc"
//   - DOES contain "space toggle"
func TestListNavFooterAccurate(t *testing.T) {
	nav := &configListNav{}
	got := stripANSITest(nav.footer())

	if strings.Contains(got, "save & back") {
		t.Errorf("listNavFooter must not claim 'save & back' (only true in standalone mode). Got: %q", got)
	}
	if !strings.Contains(got, "enter") {
		t.Errorf("listNavFooter must mention 'enter' (it is handled as back). Got: %q", got)
	}
	if !strings.Contains(got, "esc") {
		t.Errorf("listNavFooter must mention 'esc'. Got: %q", got)
	}
	if !strings.Contains(got, "space") {
		t.Errorf("listNavFooter must mention 'space toggle'. Got: %q", got)
	}
	if !strings.Contains(got, listNavFooterText) {
		t.Errorf("listNavFooter output must contain canonical text %q. Got: %q", listNavFooterText, got)
	}
}

// TestListNavInstalledFooterAccurate verifies the installed-hint variant used
// by the install-state checkbox screens (macapps, clitools, etc.).
func TestListNavInstalledFooterAccurate(t *testing.T) {
	nav := &configListNav{}
	got := stripANSITest(nav.footerInstalled())

	if strings.Contains(got, "save & back") {
		t.Errorf("listNavInstalledFooter must not claim 'save & back'. Got: %q", got)
	}
	if !strings.Contains(got, "yellow") {
		t.Errorf("listNavInstalledFooter must include 'yellow = installed' hint. Got: %q", got)
	}
	if !strings.Contains(got, listNavInstalledFooterText) {
		t.Errorf("listNavInstalledFooter output must contain canonical text %q. Got: %q", listNavInstalledFooterText, got)
	}
}

// fieldNavFooterText is the canonical visible text of the shared field-nav
// footer, used by footer tests to check that screens render the correct content.
// It is the raw string before HelpStyle renders it (ANSI + padding are stripped
// when testing against View output).
const fieldNavFooterText = "↑↓ navigate • ←→/space change • enter/esc back"

// listNavFooterText is the canonical text for list-nav screens without install hints.
const listNavFooterText = "↑↓ navigate • space toggle • enter/esc back"

// listNavInstalledFooterText is the canonical text for list-nav screens with install hints.
const listNavInstalledFooterText = "↑↓ navigate • space toggle • enter/esc back • yellow = installed"

// TestFieldNavScreensUseSharedFooter verifies that every field-nav config
// screen's rendered View contains the shared footer text (no per-screen drift).
// It checks a representative set: Neovim, Zsh, Ghostty, Fzf, Glow, Btop,
// Git, LazyGit, Yazi, Tmux, LazyDocker.
func TestFieldNavScreensUseSharedFooter(t *testing.T) {
	const w, h = 80, 50

	cases := []struct {
		name   string
		screen ScreenHandler
	}{
		{"neovim", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigNeovimScreen(ctx)
		}()},
		{"zsh", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigZshScreen(ctx)
		}()},
		{"ghostty", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigGhosttyScreen(ctx)
		}()},
		{"fzf", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigFzfScreen(ctx)
		}()},
		{"glow", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigGlowScreen(ctx)
		}()},
		{"btop", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigBtopScreen(ctx)
		}()},
		{"git", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigGitScreen(ctx)
		}()},
		{"lazygit", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigLazyGitScreen(ctx)
		}()},
		{"yazi", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigYaziScreen(ctx)
		}()},
		{"tmux", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigTmuxScreen(ctx)
		}()},
		{"lazydocker", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigLazyDockerScreen(ctx)
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.screen.View(w, h)
			visible := stripANSITest(out)
			if !strings.Contains(visible, fieldNavFooterText) {
				t.Errorf("%s: View() does not contain shared footer %q\nScreen output (visible):\n%s",
					tc.name, fieldNavFooterText, visible)
			}
			// Explicitly check the bug that was filed: "enter select" must not appear.
			if strings.Contains(visible, "enter select") {
				t.Errorf("%s: View() contains misleading 'enter select'. Got: %s", tc.name, visible)
			}
		})
	}
}

// TestListNavScreensUseSharedFooter verifies that list-nav checkbox screens
// render the shared footer text (no per-screen drift).
func TestListNavScreensUseSharedFooter(t *testing.T) {
	const w, h = 80, 50

	installedScreens := []struct {
		name   string
		screen ScreenHandler
	}{
		{"macapps", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigMacAppsScreen(ctx)
		}()},
		{"clitools", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigCLIToolsScreen(ctx)
		}()},
		{"cliutilities", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigCLIUtilitiesScreen(ctx)
		}()},
		{"guiapps", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigGUIAppsScreen(ctx)
		}()},
		{"utilities", func() ScreenHandler {
			ctx := newDeepDiveContext(t)
			ctx.app.width, ctx.app.height = w, h
			ctx.Width, ctx.Height = w, h
			return NewConfigUtilitiesScreen(ctx)
		}()},
	}

	for _, tc := range installedScreens {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.screen.View(w, h)
			visible := stripANSITest(out)
			if !strings.Contains(visible, listNavInstalledFooterText) {
				t.Errorf("%s: View() does not contain shared installed footer %q\nVisible:\n%s",
					tc.name, listNavInstalledFooterText, visible)
			}
			if strings.Contains(visible, "save & back") {
				t.Errorf("%s: View() contains misleading 'save & back'. Got: %s", tc.name, visible)
			}
		})
	}

	// claudecode uses a plain list footer (no "yellow = installed").
	t.Run("claudecode", func(t *testing.T) {
		ctx := newDeepDiveContext(t)
		ctx.app.width, ctx.app.height = w, h
		ctx.Width, ctx.Height = w, h
		screen := NewConfigClaudeCodeScreen(ctx)
		out := screen.View(w, h)
		visible := stripANSITest(out)
		if !strings.Contains(visible, listNavFooterText) {
			t.Errorf("claudecode: View() does not contain shared footer %q\nVisible:\n%s",
				listNavFooterText, visible)
		}
		if strings.Contains(visible, "save & back") {
			t.Errorf("claudecode: View() contains misleading 'save & back'")
		}
	})
}
