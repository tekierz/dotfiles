//go:build darwin || linux

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tekierz/dotfiles/internal/backup"
	"github.com/tekierz/dotfiles/internal/config"
	"github.com/tekierz/dotfiles/internal/operation"
)

func TestPlanRetentionPrunesOnlyTerminalOperationBackups(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", "")
	if err := os.MkdirAll(config.ConfigDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultGlobalConfig()
	cfg.BackupMaxCount = 1
	cfg.BackupMaxAgeDays = 0
	if err := config.SaveGlobalConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePlan, err := operation.CaptureStatePlan()
	if err != nil {
		t.Fatal(err)
	}
	state, err := operation.BootstrapStateNamespaceTracked(statePlan)
	if err != nil {
		t.Fatal(err)
	}
	backupsDir, err := operation.StateSubdirectory("backups")
	if err != nil {
		t.Fatal(err)
	}
	create := func(name string, modified time.Time) string {
		t.Helper()
		path := filepath.Join(backupsDir, name)
		if _, err := backup.CreatePlanTrackedWithState(home, path, []backup.Target{{RelPath: ".zshrc", Kind: backup.TargetFile}}, state); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
		return path
	}
	now := time.Now()
	oldTerminal := create("old-terminal", now.Add(-3*time.Hour))
	newTerminal := create("new-terminal", now.Add(-time.Hour))
	running := create("running", now.Add(-4*time.Hour))

	plan, err := operation.NewPlan(now, []operation.Action{{
		ID: "retention-test", Kind: operation.KindInstallTool, Target: "test",
		Description: "retention test", Disposition: operation.DispositionApply,
		DesiredDigest: strings.Repeat("0", 64), Ownership: operation.OwnershipPackageManager,
		Reversibility: operation.ReversibilityBestEffort,
	}})
	if err != nil {
		t.Fatal(err)
	}
	journal, err := operation.DefaultJournalWithAuthority(state)
	if err != nil {
		t.Fatal(err)
	}
	writeRecord := func(path string, terminal bool, offset time.Duration) {
		t.Helper()
		started := now.Add(offset)
		record, err := operation.StartRecord(plan, started)
		if err != nil {
			t.Fatal(err)
		}
		record.Backup = path
		if terminal {
			if err := record.Finish(operation.StatusSucceeded, started.Add(time.Second), nil, nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := journal.Write(record); err != nil {
			t.Fatal(err)
		}
	}
	writeRecord(oldTerminal, true, 0)
	writeRecord(newTerminal, true, time.Second)
	writeRecord(running, false, 2*time.Second)

	if err := cleanupBackupsWithState(state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(oldTerminal); !os.IsNotExist(err) {
		t.Fatalf("old terminal backup was not pruned: %v", err)
	}
	for _, path := range []string{newTerminal, running} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected backup %s was pruned: %v", path, err)
		}
	}
}
