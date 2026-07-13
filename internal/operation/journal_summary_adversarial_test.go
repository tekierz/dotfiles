package operation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdversarialJournalSummariesMissingXDGRootCreatesNothing(t *testing.T) {
	container := t.TempDir()
	missingRoot := filepath.Join(container, "does", "not", "exist")
	t.Setenv("XDG_STATE_HOME", missingRoot)

	set, err := ReadJournalSummaries(context.Background())
	if !errors.Is(err, ErrJournalSummariesNotPresent) {
		t.Fatalf("missing XDG state error = %v, want ErrJournalSummariesNotPresent", err)
	}
	if set.Truncated || set.Records == nil || len(set.Records) != 0 {
		t.Fatalf("missing XDG state result = %#v, want non-nil empty records", set)
	}
	if _, statErr := os.Lstat(filepath.Join(container, "does")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("read-only collection created an XDG ancestor: %v", statErr)
	}
}

func TestAdversarialJournalSummariesRejectDirectoryAuthorityTypes(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, root, state, operations string)
	}{
		{
			name: "state symlink",
			mutate: func(t *testing.T, root, state, operations string) {
				if err := os.Remove(operations); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(state); err != nil {
					t.Fatal(err)
				}
				external := filepath.Join(root, "external-state")
				adversarialMkdir(t, external, 0o700)
				if err := os.Symlink(external, state); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "operations symlink",
			mutate: func(t *testing.T, root, _, operations string) {
				if err := os.Remove(operations); err != nil {
					t.Fatal(err)
				}
				external := filepath.Join(root, "external-operations")
				adversarialMkdir(t, external, 0o700)
				if err := os.Symlink(external, operations); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "operations regular file",
			mutate: func(t *testing.T, _, _, operations string) {
				if err := os.Remove(operations); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(operations, []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, state, operations := adversarialJournalFixture(t)
			test.mutate(t, root, state, operations)
			if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
				t.Fatalf("authority type error = %v, want ErrJournalSummariesInvalid", err)
			}
		})
	}
}

func TestAdversarialJournalSummariesNameSentinelBoundary(t *testing.T) {
	t.Run("exactly 1024 invalid names reaches validation", func(t *testing.T) {
		_, _, operations := adversarialJournalFixture(t)
		for index := 0; index < journalSummaryNameLimit; index++ {
			adversarialWrite(t, operations, fmt.Sprintf("invalid-%04d", index), nil, 0o600)
		}
		if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
			t.Fatalf("1024-name error = %v, want ErrJournalSummariesInvalid", err)
		}
	})

	t.Run("1025th name wins before validation", func(t *testing.T) {
		_, _, operations := adversarialJournalFixture(t)
		for index := 0; index < journalSummaryNameLimit+1; index++ {
			adversarialWrite(t, operations, fmt.Sprintf("invalid-%04d", index), nil, 0o600)
		}
		if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesLimit) {
			t.Fatalf("1025-name error = %v, want ErrJournalSummariesLimit", err)
		}
	})
}

func TestAdversarialJournalSummariesStrictNestedJSON(t *testing.T) {
	started := time.Date(2026, 7, 12, 15, 0, 0, 120000000, time.UTC)
	record := adversarialRecord(started, "abcdefabcdefabcd", StatusFailed, 1)
	record.Rollback = &RollbackResult{Status: RollbackIncomplete, Restored: 1, Removed: 2, Skipped: 3, Warnings: 4}
	valid := string(adversarialMarshal(t, record))

	for _, test := range []struct {
		name string
		raw  string
	}{
		{
			name: "duplicate rollback member",
			raw:  strings.Replace(valid, `"restored":1`, `"restored":1,"restored":1`, 1),
		},
		{
			name: "unknown rollback member",
			raw:  strings.Replace(valid, `"restored":1`, `"restored":1,"private_path":"/secret"`, 1),
		},
		{
			name: "duplicate member inside action array",
			raw: strings.Replace(valid,
				`"action_id":"private-action-0","status":"succeeded"`,
				`"action_id":"private-action-0","status":"succeeded","status":"succeeded"`, 1),
		},
		{
			name: "redundant timestamp precision",
			raw:  strings.Replace(valid, `2026-07-12T15:00:00.12Z`, `2026-07-12T15:00:00.1200Z`, 1),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, operations := adversarialJournalFixture(t)
			adversarialWrite(t, operations, record.OperationID+".json", []byte(test.raw), 0o600)
			if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
				t.Fatalf("strict JSON error = %v, want ErrJournalSummariesInvalid", err)
			}
		})
	}

	t.Run("trailing whitespace remains valid", func(t *testing.T) {
		_, _, operations := adversarialJournalFixture(t)
		adversarialWrite(t, operations, record.OperationID+".json", append(adversarialMarshal(t, record), []byte(" \n\t\r\n")...), 0o600)
		if _, err := ReadJournalSummaries(context.Background()); err != nil {
			t.Fatalf("trailing whitespace error = %v", err)
		}
	})

	t.Run("uppercase filename suffix is invalid", func(t *testing.T) {
		_, _, operations := adversarialJournalFixture(t)
		upperName := strings.ToUpper(record.OperationID[len(record.OperationID)-16:])
		name := record.OperationID[:len(record.OperationID)-16] + upperName + ".json"
		adversarialWrite(t, operations, name, adversarialMarshal(t, record), 0o600)
		if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
			t.Fatalf("uppercase filename error = %v, want ErrJournalSummariesInvalid", err)
		}
	})
}

func TestAdversarialJournalSummariesExactByteLimits(t *testing.T) {
	t.Run("one record exactly one MiB is accepted", func(t *testing.T) {
		_, _, operations := adversarialJournalFixture(t)
		record := adversarialRecord(time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded, 1)
		data := adversarialPadRecord(t, record, int(journalSummaryFileByteLimit))
		adversarialWrite(t, operations, record.OperationID+".json", data, 0o600)
		set, err := ReadJournalSummaries(context.Background())
		if err != nil || len(set.Records) != 1 {
			t.Fatalf("exact per-file limit = %#v, %v", set, err)
		}
	})

	t.Run("two passes totaling exactly 32 MiB are accepted", func(t *testing.T) {
		_, _, operations := adversarialJournalFixture(t)
		base := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
		for index := 0; index < 16; index++ {
			record := adversarialRecord(base.Add(time.Duration(index)*time.Second), fmt.Sprintf("%016x", index+1), StatusSucceeded, 1)
			adversarialWrite(t, operations, record.OperationID+".json", adversarialPadRecord(t, record, int(journalSummaryFileByteLimit)), 0o600)
		}
		set, err := ReadJournalSummaries(context.Background())
		if err != nil || set.Truncated || len(set.Records) != 16 {
			t.Fatalf("exact aggregate limit = %#v, %v", set, err)
		}
	})
}

func TestAdversarialJournalSummariesOrderingIsObservable(t *testing.T) {
	_, _, operations := adversarialJournalFixture(t)
	started := time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC)
	for suffix := 1; suffix <= 21; suffix++ {
		record := adversarialRecord(started, fmt.Sprintf("%016x", suffix), StatusSucceeded, suffix)
		adversarialWrite(t, operations, record.OperationID+".json", adversarialMarshal(t, record), 0o600)
	}

	set, err := ReadJournalSummaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !set.Truncated || len(set.Records) != 20 {
		t.Fatalf("ordered result = %#v", set)
	}
	for index, summary := range set.Records {
		wantSucceeded := 21 - index
		if summary.Ordinal != index || summary.Actions.Succeeded != wantSucceeded {
			t.Fatalf("record %d = %#v, want ordinal %d and %d succeeded actions", index, summary, index, wantSucceeded)
		}
	}
}

func TestAdversarialJournalSummariesRejectRemovalAndRenameDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(path string)
	}{
		{name: "remove", mutate: func(path string) { _ = os.Remove(path) }},
		{name: "rename", mutate: func(path string) { _ = os.Rename(path, strings.TrimSuffix(path, ".json")+"f.json") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, operations := adversarialJournalFixture(t)
			record := adversarialRecord(time.Date(2026, 7, 12, 19, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded, 1)
			path := filepath.Join(operations, record.OperationID+".json")
			adversarialWrite(t, operations, filepath.Base(path), adversarialMarshal(t, record), 0o600)
			adversarialSetJournalHooks(t, func(hooks *journalSummaryHooks) {
				hooks.beforeRevalidate = func() { test.mutate(path) }
			})
			if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
				t.Fatalf("namespace drift error = %v, want ErrJournalSummariesInvalid", err)
			}
		})
	}
}

func TestAdversarialJournalSummariesCancellationAtEachPass(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*journalSummaryHooks, context.CancelFunc)
	}{
		{name: "after names", configure: func(h *journalSummaryHooks, cancel context.CancelFunc) { h.afterNames = cancel }},
		{name: "after initial reads", configure: func(h *journalSummaryHooks, cancel context.CancelFunc) { h.afterInitialReads = cancel }},
		{name: "before revalidation", configure: func(h *journalSummaryHooks, cancel context.CancelFunc) { h.beforeRevalidate = cancel }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, operations := adversarialJournalFixture(t)
			record := adversarialRecord(time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded, 1)
			adversarialWrite(t, operations, record.OperationID+".json", adversarialMarshal(t, record), 0o600)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			adversarialSetJournalHooks(t, func(hooks *journalSummaryHooks) { test.configure(hooks, cancel) })
			if _, err := ReadJournalSummaries(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestAdversarialJournalSummariesReturnedPointersDoNotAlias(t *testing.T) {
	_, _, operations := adversarialJournalFixture(t)
	started := time.Date(2026, 7, 12, 21, 0, 0, 0, time.UTC)
	for index := 0; index < 2; index++ {
		record := adversarialRecord(started.Add(time.Duration(index)*time.Second), fmt.Sprintf("%016x", index+1), StatusFailed, 1)
		record.Rollback = &RollbackResult{Status: RollbackIncomplete, Restored: 1}
		adversarialWrite(t, operations, record.OperationID+".json", adversarialMarshal(t, record), 0o600)
	}

	set, err := ReadJournalSummaries(context.Background())
	if err != nil || len(set.Records) != 2 {
		t.Fatalf("summary set = %#v, %v", set, err)
	}
	*set.Records[0].DurationMilliseconds = 99
	set.Records[0].Rollback.Restored = 99
	if *set.Records[1].DurationMilliseconds == 99 || set.Records[1].Rollback.Restored == 99 {
		t.Fatalf("returned records share nested pointers: %#v", set.Records)
	}
}

func adversarialJournalFixture(t *testing.T) (root, state, operations string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	state = filepath.Join(root, "dotfiles")
	operations = filepath.Join(state, "operations")
	adversarialMkdir(t, state, 0o700)
	adversarialMkdir(t, operations, 0o700)
	return root, state, operations
}

func adversarialRecord(started time.Time, suffix string, status Status, actionCount int) Record {
	actions := make([]ActionResult, actionCount)
	for index := range actions {
		actions[index] = ActionResult{ActionID: fmt.Sprintf("private-action-%d", index), Status: ActionSucceeded}
	}
	record := Record{
		SchemaVersion: CurrentJournalSchemaVersion,
		OperationID:   started.UTC().Format("20060102T150405.000000000Z") + "-" + suffix,
		PlanHash:      strings.Repeat("a", 64),
		StartedAt:     started.UTC(),
		Status:        status,
		Actions:       actions,
	}
	if status != StatusRunning {
		finished := started.Add(1250 * time.Millisecond).UTC()
		record.FinishedAt = &finished
	}
	return record
}

func adversarialMarshal(t *testing.T, record Record) []byte {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func adversarialPadRecord(t *testing.T, record Record, size int) []byte {
	t.Helper()
	data := adversarialMarshal(t, record)
	if len(data) > size {
		t.Fatalf("record size %d exceeds requested padded size %d", len(data), size)
	}
	return append(data, []byte(strings.Repeat(" ", size-len(data)))...)
}

func adversarialWrite(t *testing.T, directory, name string, data []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), data, mode); err != nil {
		t.Fatal(err)
	}
}

func adversarialMkdir(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
}

func adversarialSetJournalHooks(t *testing.T, configure func(*journalSummaryHooks)) {
	t.Helper()
	journalSummaryTestHooks = journalSummaryHooks{}
	configure(&journalSummaryTestHooks)
	t.Cleanup(func() { journalSummaryTestHooks = journalSummaryHooks{} })
}
