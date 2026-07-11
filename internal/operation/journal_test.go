package operation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tekierz/dotfiles/internal/safefile"
)

func testPlan(t *testing.T, now time.Time) Plan {
	t.Helper()
	action := validConfigAction()
	plan, err := NewPlan(now, []Action{action})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestJournalWritesAndUpdatesAtomicPrivateRecord(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	now := time.Date(2026, 7, 10, 19, 0, 0, 0, time.UTC)
	record, err := StartRecord(testPlan(t, now), now)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Write(record); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, ".local", "state", "dotfiles", "operations", record.OperationID+".json")
	assertJournalMode(t, path, 0o600)
	assertJournalMode(t, filepath.Dir(path), 0o700)
	loaded, err := journal.Read(record.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != StatusRunning || loaded.PlanHash != record.PlanHash {
		t.Fatalf("loaded running record = %+v", loaded)
	}

	results := []ActionResult{{ActionID: record.Actions[0].ActionID, Status: ActionSucceeded, Summary: "done\nwithout\x1b[31m controls"}}
	if err := record.Finish(StatusSucceeded, now.Add(time.Second), results, []string{"warning\ttext"}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Write(record); err != nil {
		t.Fatal(err)
	}
	loaded, err = journal.Read(record.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != StatusSucceeded || loaded.FinishedAt == nil {
		t.Fatalf("loaded terminal record = %+v", loaded)
	}
	if strings.ContainsAny(loaded.Actions[0].Summary, "\n\x1b") || loaded.Actions[0].Summary != "done without[31m controls" {
		t.Fatalf("summary was not sanitized: %q", loaded.Actions[0].Summary)
	}
	assertJournalMode(t, path, 0o600)
}

func TestJournalPersistsSanitizedRollbackOutcome(t *testing.T) {
	now := time.Now()
	record, err := StartRecord(testPlan(t, now), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.SetRollback(RollbackResult{Status: RollbackIncomplete, Restored: 2, Removed: 1, Skipped: 1, Warnings: 1, Summary: "manual\nreview\x1b[31m"}); err != nil {
		t.Fatal(err)
	}
	if record.Rollback == nil || record.Rollback.Summary != "manual review[31m" {
		t.Fatalf("rollback outcome = %+v", record.Rollback)
	}
	bad := *record.Rollback
	bad.Status = "unknown"
	if err := record.SetRollback(bad); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("invalid rollback status error = %v", err)
	}
}

func TestTerminalBackupPathsExcludesRunningOperations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	journal, err := DefaultJournal()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	running, err := StartRecord(testPlan(t, now), now)
	if err != nil {
		t.Fatal(err)
	}
	running.Backup = filepath.Join(home, ".local", "state", "dotfiles", "backups", "running")
	if err := journal.Write(running); err != nil {
		t.Fatal(err)
	}
	terminal, err := StartRecord(testPlan(t, now.Add(time.Second)), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	terminal.Backup = filepath.Join(home, ".local", "state", "dotfiles", "backups", "terminal")
	if err := terminal.Finish(StatusFailed, now.Add(2*time.Second), nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := journal.Write(terminal); err != nil {
		t.Fatal(err)
	}
	paths, err := journal.TerminalBackupPaths()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := paths[terminal.Backup]; !ok {
		t.Fatalf("terminal backup missing from %v", paths)
	}
	if _, ok := paths[running.Backup]; ok {
		t.Fatalf("running backup was retention-eligible: %v", paths)
	}
}

func TestJournalRefusesSymlinkedStateDescendant(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	stateParent := filepath.Join(home, ".local", "state")
	if err := os.MkdirAll(stateParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(stateParent, "dotfiles")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	now := time.Now()
	record, err := StartRecord(testPlan(t, now), now)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := DefaultJournal()
	if err == nil {
		err = journal.Write(record)
	}
	if !errors.Is(err, safefile.ErrSymlink) {
		t.Fatalf("journal symlink error = %v, want ErrSymlink", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("journal wrote outside trusted HOME: %v", entries)
	}
}

func TestJournalRefusesSymlinkedMissingXDGAncestor(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "redirect")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "redirect", "state"))
	_, err := DefaultJournal()
	if err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("DefaultJournal error = %v, want symlinked ancestor refusal", err)
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Fatalf("symlink target changed: entries=%v err=%v", entries, readErr)
	}
}

func TestRecordValidationAndSummaryBounds(t *testing.T) {
	now := time.Now()
	record, err := StartRecord(testPlan(t, now), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Finish(StatusRunning, now.Add(time.Second), nil, nil); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("Finish running error = %v, want ErrInvalidRecord", err)
	}

	long := strings.Repeat("界", 1000)
	sanitized := sanitizeSummary(long)
	if len(sanitized) > 2051 || !strings.HasSuffix(sanitized, "…") {
		t.Fatalf("bounded summary has %d bytes and suffix %q", len(sanitized), sanitized[len(sanitized)-3:])
	}
	if !utf8.ValidString(sanitized) {
		t.Fatal("bounded summary split a UTF-8 rune")
	}
}

func TestJournalReadRejectsTraversal(t *testing.T) {
	journal := Journal{root: t.TempDir(), rel: "operations"}
	for _, id := range []string{"", "../escape", "nested/id", "bad id"} {
		if _, err := journal.Read(id); !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("Read(%q) error = %v, want ErrInvalidRecord", id, err)
		}
	}
}

func assertJournalMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}
