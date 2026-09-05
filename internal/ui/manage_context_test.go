package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestManageSettingsSynchronizeContext(t *testing.T) {
	for _, input := range []string{"keyboard", "mouse"} {
		for field := 0; field < 3; field++ {
			t.Run(input+string(rune('0'+field)), func(t *testing.T) {
				withTempHome(t)
				a := NewApp(true)
				a.manageInstalledReady = true
				a.screenMgr.Navigate(ScreenManage)
				a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
				found := false
				for i, item := range a.manageItems() {
					if item.id == "global" {
						a.manageIndex = i
						found = true
						break
					}
				}
				if !found {
					t.Fatal("global Manage settings missing")
				}
				a.pendingInstallPlan = &installPlan{}
				a.deepDiveContinuation = &deepDiveContinuation{}
				a.pendingManageSavePlan = &manageSavePlan{}
				a.managePane = managePaneSettings
				a.configFieldIndex = field
				if input == "keyboard" {
					a.Update(tea.KeyMsg{Type: tea.KeyEnter})
				} else {
					layout := a.manageLayout()
					a.Update(tea.MouseMsg{X: layout.rightX + layout.rightW - 2, Y: layout.rightListY + field, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				}
				if a.pendingInstallPlan != nil || a.deepDiveContinuation != nil || a.pendingManageSavePlan != nil {
					t.Fatal("Manage settings retained old review authority")
				}
				ctx := a.screenMgr.Context()
				if ctx.Theme != a.theme || ctx.NavStyle != a.navStyle || ctx.AnimationsEnabled != a.animationsEnabled {
					t.Fatal("Manage input left shared screen context stale")
				}
			})
		}
	}
}
