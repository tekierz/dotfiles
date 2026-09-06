package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/config"
)

func TestThemeSaveFailureStaysVisibleAndCanRetry(t *testing.T) {
	for _, returnTo := range []Screen{ScreenMainMenu, ScreenWelcome} {
		for _, failure := range []string{"malformed", "unwritable"} {
			t.Run(fmt.Sprintf("%s/return=%d", failure, returnTo), func(t *testing.T) {
				withTempHome(t)
				initial := config.DefaultGlobalConfig()
				initial.Theme = "catppuccin-mocha"
				if err := config.SaveGlobalConfig(initial); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(config.ConfigDir(), "global.json")
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				a := NewApp(true)
				a.themeStandalone = true
				a.themeReturn = returnTo
				a.screenMgr.Navigate(ScreenThemePicker)
				a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
				a.Update(tea.KeyMsg{Type: tea.KeyDown})
				selected, index := a.theme, a.themeIndex
				if selected == initial.Theme {
					t.Fatal("fixture did not preview a new theme")
				}
				before := original
				switch failure {
				case "malformed":
					before = []byte("{invalid")
					if err := os.WriteFile(path, before, 0600); err != nil {
						t.Fatal(err)
					}
				case "unwritable":
					if os.Geteuid() == 0 {
						t.Skip("permission fixture requires non-root process")
					}
					if err := os.Chmod(config.ConfigDir(), 0500); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = os.Chmod(config.ConfigDir(), 0700) })
				}
				plan := &installPlan{}
				a.pendingInstallPlan = plan
				_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
				// A nil command correctly means the picker remains open.
				if cmd != nil {
					t.Fatalf("failed save returned navigation/quit command (%T)", cmd())
				}
				if a.screenMgr.Current().ID() != ScreenThemePicker || !strings.Contains(a.View(), "Failed to save theme") {
					t.Fatal("save error not visible in picker")
				}
				if a.theme != selected || a.themeIndex != index || a.screenMgr.Context().Theme != selected {
					t.Fatal("failed save discarded selected preview")
				}
				if a.pendingInstallPlan != plan {
					t.Fatal("failed save applied success-only invalidation")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("failed save changed persisted bytes")
				}
				if err := os.Chmod(config.ConfigDir(), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
				if cmd == nil {
					t.Fatalf("retry failed: %s", a.themeStatus)
				}
				saved, err := config.LoadGlobalConfig()
				if err != nil || saved.Theme != selected {
					t.Fatalf("retry did not save selected theme: %v", err)
				}
				if a.themeStatus != "" || a.pendingInstallPlan != nil {
					t.Fatal("successful save did not complete shared-state transition")
				}
				if returnTo == ScreenMainMenu {
					if nav, ok := cmd().(NavigateMsg); !ok || nav.To != ScreenMainMenu {
						t.Fatal("retry did not return to menu")
					}
				} else if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("CLI retry did not quit")
				}
			})
		}
	}
}
