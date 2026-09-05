package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tekierz/dotfiles/internal/backup"
)

func TestBackupConfirmationExecutesCapturedCatalog(t *testing.T) {
	for _, action := range []string{"restore", "delete"} {
		t.Run(action, func(t *testing.T) {
			home := withTempHome(t)
			root := filepath.Join(home, ".config", "dotfiles", "backups")
			target := filepath.Join(home, ".zshrc")
			entries := make(map[string]BackupEntry)
			for _, name := range []string{"A", "B"} {
				if err := os.WriteFile(target, []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := backup.Create(home, filepath.Join(root, name), []string{".zshrc"}); err != nil {
					t.Fatal(err)
				}
			}
			catalog, err := backup.ListCatalog(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range catalog {
				entries[entry.Name] = BackupEntry{Name: entry.Name, Catalog: entry}
			}
			if len(entries) != 2 {
				t.Fatalf("catalog: %+v", catalog)
			}
			a := NewApp(true)
			a.backups = []BackupEntry{entries["A"], entries["B"]}
			screen := NewBackupsScreen(a.screenMgr.Context())
			key := tea.KeyMsg{Type: tea.KeyEnter}
			if action == "delete" {
				key = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}
			}
			screen.handleKey(key)
			// Simulate refreshed list and moved selection while the modal is open.
			a.backups = []BackupEntry{entries["B"]}
			a.backupIndex = 0
			cmd := screen.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
			if cmd == nil {
				t.Fatal("confirmation returned no command")
			}
			result := cmd().(appAsyncResult)
			if action == "restore" {
				done := result.payload.(backupRestoreDoneMsg)
				if done.err != nil || done.name != "A" {
					t.Fatalf("restore result: %+v", done)
				}
				data, readErr := os.ReadFile(target)
				if readErr != nil || string(data) != "A" {
					t.Fatalf("restored %q, %v", data, readErr)
				}
			} else {
				done := result.payload.(backupDeleteDoneMsg)
				if done.err != nil || done.name != "A" {
					t.Fatalf("delete result: %+v", done)
				}
				if _, statErr := os.Stat(filepath.Join(root, "A")); !os.IsNotExist(statErr) {
					t.Fatalf("A still exists: %v", statErr)
				}
			}
			if _, statErr := os.Stat(filepath.Join(root, "B", backup.ManifestName)); statErr != nil {
				t.Fatalf("B changed: %v", statErr)
			}
		})
	}
}
