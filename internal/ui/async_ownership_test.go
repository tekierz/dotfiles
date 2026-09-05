package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/pkg"
)

func TestAsyncOwnershipResultsSurviveNavigation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		deliver func(*App)
		verify  func(*testing.T, *App)
	}{
		{"updates", func(a *App) {
			a.updateChecking = true
			a.Update(updateCheckDoneMsg{updates: []pkg.Package{{Name: "tmux"}}})
		}, func(t *testing.T, a *App) {
			if a.updateChecking || !a.updateCheckDone || len(a.updateResults) != 1 {
				t.Fatal("update completion lost away from Updates")
			}
		}},
		{"backups", func(a *App) {
			a.backupsLoading = true
			a.Update(backupsLoadedMsg{backups: []BackupEntry{{Name: "saved"}}})
		}, func(t *testing.T, a *App) {
			if a.backupsLoading || !a.backupsLoaded || len(a.backups) != 1 {
				t.Fatal("backup list completion lost away from Backups")
			}
		}},
		{"users", func(a *App) { a.usersLoaded = true; a.Update(userLoadedMsg{users: []userItem{{name: "Alice"}}}) }, func(t *testing.T, a *App) {
			if len(a.usersItems) != 1 {
				t.Fatal("user completion lost away from Users")
			}
		}},
		{"restore", func(a *App) { a.backupRunning = true; a.Update(backupRestoreDoneMsg{name: "saved", count: 1}) }, func(t *testing.T, a *App) {
			if a.backupRunning || !strings.Contains(a.backupStatus, "Restored") {
				t.Fatal("restore completion lost away from Backups")
			}
		}},
		{"delete_error", func(a *App) { a.backupRunning = true; a.Update(backupDeleteDoneMsg{err: errors.New("denied")}) }, func(t *testing.T, a *App) {
			if a.backupRunning || !strings.Contains(a.backupStatus, "denied") {
				t.Fatal("delete failure lost away from Backups")
			}
		}},
		{"user_save_error", func(a *App) { a.Update(userSavedMsg{err: errors.New("denied")}) }, func(t *testing.T, a *App) {
			if !strings.Contains(a.usersStatus, "denied") {
				t.Fatal("user save failure lost away from Users")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withTempHome(t)
			a := NewApp(true)
			a.screenMgr.Navigate(ScreenMainMenu)
			tc.deliver(a)
			tc.verify(t, a)
			if a.screenMgr.Current().ID() != ScreenMainMenu {
				t.Fatal("completion navigated away")
			}
		})
	}
}

func TestAsyncOwnershipReadGenerations(t *testing.T) {
	for _, channel := range []asyncChannel{asyncUpdates, asyncBackups, asyncUsers} {
		t.Run(fmt.Sprint(channel), func(t *testing.T) {
			withTempHome(t)
			a := NewApp(true)
			a.screenMgr.Navigate(ScreenMainMenu)
			payload := func(name string) tea.Msg {
				switch channel { //nolint:exhaustive // This table exercises only read channels.
				case asyncUpdates:
					return updateCheckDoneMsg{updates: []pkg.Package{{Name: name}}}
				case asyncBackups:
					return backupsLoadedMsg{backups: []BackupEntry{{Name: name}}}
				default:
					return userLoadedMsg{users: []userItem{{name: name}}}
				}
			}
			result := func(name string) tea.Cmd { return func() tea.Msg { return payload(name) } }
			old := a.startAsync(channel, result("old"))()
			fresh := a.startAsync(channel, result("fresh"))()
			a.Update(old)
			if !a.asyncRequests[channel].pending {
				t.Fatal("stale result cleared fresh request")
			}
			a.Update(fresh)
			a.Update(old)
			a.Update(fresh) // A duplicate is consumed without replaying the reducer.
			a.Update(payload("untagged"))
			var got string
			switch channel { //nolint:exhaustive // This table exercises only read channels.
			case asyncUpdates:
				got = a.updateResults[0].Name
			case asyncBackups:
				got = a.backups[0].Name
			case asyncUsers:
				got = a.usersItems[0].name
			}
			if got != "fresh" || a.asyncRequests[channel].pending {
				t.Fatalf("latest result=%q pending=%v", got, a.asyncRequests[channel].pending)
			}
		})
	}
}

func TestAsyncOwnershipBackupOperations(t *testing.T) {
	for _, kind := range []string{"create", "delete", "restore"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%v", kind, fail), func(t *testing.T) {
				withTempHome(t)
				a := NewApp(true)
				a.screenMgr.Navigate(ScreenMainMenu)
				var err error
				if fail {
					err = errors.New("operation denied")
				}
				var payload tea.Msg
				switch kind {
				case "create":
					payload = backupCreateDoneMsg{name: "saved", warning: "partial", err: err}
				case "delete":
					payload = backupDeleteDoneMsg{name: "saved", err: err}
				case "restore":
					payload = backupRestoreDoneMsg{name: "saved", count: 1, warnings: 1, err: err}
				}
				a.backupRunning = true
				done := a.startAsync(asyncBackupOperation, func() tea.Msg { return payload })()
				_, follow := a.Update(done)
				if a.backupRunning || a.backupStatus == "" {
					t.Fatal("operation remained stranded")
				}
				if fail && !strings.Contains(a.backupStatus, "denied") {
					t.Fatal("operation error lost")
				}
				if !fail && kind != "delete" && !a.backupStatusWarning {
					t.Fatal("operation warning lost")
				}
				if !fail && kind != "restore" {
					if follow == nil {
						t.Fatal("refresh lost away from Backups")
					}
					if _, ok := follow().(appAsyncResult); !ok {
						t.Fatal("refresh lacks identity")
					}
				}
				status := a.backupStatus
				a.backupRunning = true
				next := a.startAsync(asyncBackupOperation, func() tea.Msg { return backupRestoreDoneMsg{name: "next", count: 1} })
				if next == nil {
					t.Fatal("next operation blocked after completion")
				}
				a.Update(done)
				if !a.backupRunning || a.backupStatus != status {
					t.Fatal("duplicate completed newer operation")
				}
				a.Update(next())
			})
		}
	}
}

func TestAsyncOwnershipUserOperationsAndReadInvalidation(t *testing.T) {
	for _, payload := range []tea.Msg{userSavedMsg{name: "Alice"}, userDeletedMsg{name: "Alice"}, userSwitchedMsg{name: "Alice"}, userSavedMsg{err: errors.New("denied")}, userDeletedMsg{err: errors.New("denied")}, userSwitchedMsg{err: errors.New("denied")}} {
		t.Run(fmt.Sprintf("%T/%v", payload, payload), func(t *testing.T) {
			withTempHome(t)
			a := NewApp(true)
			a.screenMgr.Navigate(ScreenMainMenu)
			stale := a.startAsync(asyncUsers, func() tea.Msg { return userLoadedMsg{users: []userItem{{name: "stale"}}} })()
			op := a.startAsync(asyncUserOperation, func() tea.Msg { return payload })
			a.Update(stale)
			if len(a.usersItems) != 0 {
				t.Fatal("pre-operation read accepted")
			}
			if next := a.startAsync(asyncUserOperation, func() tea.Msg { return userSavedMsg{} }); next != nil {
				t.Fatal("overlapping mutation admitted")
			}
			during := a.startAsync(asyncUsers, func() tea.Msg { return userLoadedMsg{users: []userItem{{name: "during"}}} })()
			done := op()
			_, follow := a.Update(done)
			a.Update(during)
			if len(a.usersItems) != 0 {
				t.Fatal("mid-operation read accepted after completion")
			}
			if a.usersStatus == "" || a.asyncRequests[asyncUserOperation].pending {
				t.Fatal("user operation result lost")
			}
			if !strings.Contains(a.usersStatus, "denied") {
				if follow == nil {
					t.Fatal("user refresh lost")
				}
				if _, ok := follow().(appAsyncResult); !ok {
					t.Fatal("user refresh lacks identity")
				}
			}
			_, duplicate := a.Update(done)
			if duplicate != nil {
				t.Fatal("duplicate operation restarted refresh")
			}
		})
	}
}

func TestAsyncOwnershipBackupModalMouse(t *testing.T) {
	for _, running := range []bool{false, true} {
		withTempHome(t)
		a := NewApp(true)
		a.backupsLoaded = true
		a.backups = []BackupEntry{{Name: "A"}, {Name: "B"}}
		a.screenMgr.Navigate(ScreenBackups)
		a.backupRunning = running
		a.backupConfirmMode = !running
		a.backupConfirmName = "A"
		for _, msg := range []tea.MouseMsg{{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}, {Button: tea.MouseButtonWheelDown}} {
			_, cmd := a.Update(msg)
			if cmd != nil || a.backupIndex != 0 || a.backupConfirmName != "A" {
				t.Fatal("modal mouse changed accepted selection/navigation")
			}
		}
	}
}

func TestAsyncOwnershipReadsFailAwayAndRetry(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	a.screenMgr.Navigate(ScreenMainMenu)
	for _, tc := range []struct {
		channel asyncChannel
		screen  Screen
		payload tea.Msg
	}{
		{asyncUpdates, ScreenUpdate, updateCheckDoneMsg{err: errors.New("query failed")}},
		{asyncBackups, ScreenBackups, backupsLoadedMsg{err: errors.New("query failed")}},
		{asyncUsers, ScreenUsers, userLoadedMsg{err: errors.New("query failed")}},
	} {
		done := a.startAsync(tc.channel, func() tea.Msg { return tc.payload })()
		a.Update(done)
		a.screenMgr.Navigate(tc.screen)
		// Updates and Backups expose explicit refresh; Users automatically retries.
		_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		if cmd == nil {
			t.Fatalf("failed read %v cannot refresh", tc.channel)
		}
		if !a.asyncRequests[tc.channel].pending {
			t.Fatalf("retry %v lacks generation", tc.channel)
		}
		a.screenMgr.Navigate(ScreenMainMenu)
	}
}

func TestAsyncOwnershipEntryArrivalOrder(t *testing.T) {
	for _, tc := range []struct {
		channel asyncChannel
		screen  Screen
		payload tea.Msg
	}{
		{asyncUpdates, ScreenUpdate, updateCheckDoneMsg{updates: []pkg.Package{{Name: "tmux"}}}},
		{asyncBackups, ScreenBackups, backupsLoadedMsg{backups: []BackupEntry{{Name: "saved"}}}},
		{asyncUsers, ScreenUsers, userLoadedMsg{users: []userItem{{name: "Alice"}}}},
	} {
		for _, before := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/before=%v", tc.channel, before), func(t *testing.T) {
				withTempHome(t)
				a := NewApp(true)
				a.screenMgr.Navigate(ScreenMainMenu)
				if cmd := startTabTargetLoad(a, tc.screen); cmd == nil {
					t.Fatal("entry did not start query")
				}
				generation := a.asyncRequests[tc.channel].generation
				if generation == 0 {
					t.Fatal("entry query unbound")
				}
				done := appAsyncResult{channel: tc.channel, generation: generation, payload: tc.payload}
				if before {
					a.Update(done)
				}
				if cmd := a.screenMgr.Navigate(tc.screen); cmd != nil {
					t.Fatal("navigation duplicated started/completed query")
				}
				a.screenMgr.Navigate(ScreenMainMenu)
				if !before {
					a.Update(done)
				}
				if cmd := a.screenMgr.Navigate(tc.screen); cmd != nil {
					t.Fatal("completed query lost on reentry")
				}
				if a.asyncRequests[tc.channel].pending {
					t.Fatal("entry query stranded")
				}
			})
		}
	}
}

func TestAsyncOwnershipUserMutationAllowsTabNavigation(t *testing.T) {
	withTempHome(t)
	a := NewApp(true)
	a.usersLoaded = true
	a.screenMgr.Navigate(ScreenUsers)
	a.startAsync(asyncUserOperation, func() tea.Msg { return userSavedMsg{name: "Alice"} })
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if cmd == nil {
		t.Fatal("pending user save blocked tab navigation")
	}
	_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd != nil || a.usersCreating {
		t.Fatal("pending user save admitted another mutation")
	}
}
