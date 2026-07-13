//go:build darwin || linux

package operation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadJournalSummariesDoesNotCreateMissingState(t *testing.T) {
	for _, test := range []struct {
		name        string
		createState bool
	}{
		{name: "state missing"},
		{name: "operations missing", createState: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("XDG_STATE_HOME", root)
			if test.createState {
				mustJournalSummaryMkdir(t, filepath.Join(root, "dotfiles"), 0o700)
			}
			before := journalSummaryInventory(t, root)

			set, err := ReadJournalSummaries(context.Background())
			if !errors.Is(err, ErrJournalSummariesNotPresent) || set.Truncated || set.Records == nil || len(set.Records) != 0 {
				t.Fatalf("missing journal = %+v, %v", set, err)
			}
			after := journalSummaryInventory(t, root)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("read-only collection changed filesystem\nbefore: %#v\nafter:  %#v", before, after)
			}
		})
	}
}

func TestReadJournalSummariesEmptyAndUnavailableShapesAreClosed(t *testing.T) {
	t.Run("empty journal", func(t *testing.T) {
		_, _ = journalSummaryFixture(t)
		set, err := ReadJournalSummaries(context.Background())
		if err != nil || set.Truncated || set.Records == nil || len(set.Records) != 0 {
			t.Fatalf("empty journal = %+v, %v", set, err)
		}
	})

	t.Run("unavailable anchor", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "relative-state")
		set, err := ReadJournalSummaries(context.Background())
		if !errors.Is(err, ErrJournalSummariesUnavailable) || set.Truncated || set.Records == nil || len(set.Records) != 0 ||
			err.Error() != ErrJournalSummariesUnavailable.Error() {
			t.Fatalf("unavailable journal = %+v, %v", set, err)
		}
	})
}

func TestReadJournalSummariesProjectsSortsAndDefensivelyCopies(t *testing.T) {
	root, operations := journalSummaryFixture(t)
	base := time.Date(2026, 7, 12, 12, 0, 0, 123456789, time.UTC)
	old := journalSummaryRecord(base, "0000000000000001", StatusRunning)
	newerLow := journalSummaryRecord(base.Add(time.Second), "0000000000000002", StatusSucceeded)
	newerHigh := journalSummaryRecord(base.Add(time.Second), "ffffffffffffffff", StatusFailed)
	newerHigh.Backup = filepath.Join(root, "private", "backup")
	newerHigh.Actions = []ActionResult{
		{ActionID: "private-pending", Status: ActionPending, Summary: "token=private"},
		{ActionID: "private-success", Status: ActionSucceeded},
		{ActionID: "private-failure", Status: ActionFailed},
		{ActionID: "private-skip", Status: ActionSkipped},
	}
	newerHigh.Rollback = &RollbackResult{Status: RollbackIncomplete, Restored: 1, Removed: 2, Skipped: 3, Warnings: 4, Summary: "private"}
	for _, record := range []Record{old, newerLow, newerHigh} {
		writeJournalSummaryRecord(t, operations, record)
	}
	before := journalSummaryInventory(t, root)

	set, err := ReadJournalSummaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if set.Truncated || len(set.Records) != 3 {
		t.Fatalf("summary set = %+v", set)
	}
	if set.Records[0].Ordinal != 0 || set.Records[0].Status != StatusFailed || !set.Records[0].BackupRecorded ||
		set.Records[0].Actions != (JournalActionCounts{Pending: 1, Succeeded: 1, Failed: 1, Skipped: 1}) ||
		set.Records[0].Rollback == nil || *set.Records[0].Rollback != (JournalRollbackSummary{Status: RollbackIncomplete, Restored: 1, Removed: 2, Skipped: 3, Warnings: 4}) ||
		set.Records[0].DurationMilliseconds == nil || *set.Records[0].DurationMilliseconds != 1250 {
		t.Fatalf("newest summary = %+v", set.Records[0])
	}
	if set.Records[1].Status != StatusSucceeded || set.Records[2].Status != StatusRunning || set.Records[2].DurationMilliseconds != nil {
		t.Fatalf("summary ordering = %+v", set.Records)
	}
	after := journalSummaryInventory(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("collection changed filesystem\nbefore: %#v\nafter:  %#v", before, after)
	}

	set.Records[0].Status = StatusRunning
	set.Records[0].Rollback.Status = RollbackFailed
	*set.Records[0].DurationMilliseconds = 99
	again, err := ReadJournalSummaries(context.Background())
	if err != nil || again.Records[0].Status != StatusFailed || again.Records[0].Rollback.Status != RollbackIncomplete || *again.Records[0].DurationMilliseconds != 1250 {
		t.Fatalf("returned data aliases accepted state: %+v, %v", again, err)
	}
}

func TestReadJournalSummariesValidatesAllCandidatesBeforeTruncation(t *testing.T) {
	_, operations := journalSummaryFixture(t)
	base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 21; index++ {
		record := journalSummaryRecord(base.Add(time.Duration(index)*time.Second), fmt.Sprintf("%016x", index+1), StatusSucceeded)
		writeJournalSummaryRecord(t, operations, record)
	}
	oldest := journalSummaryRecord(base.Add(-time.Second), "ffffffffffffffff", StatusSucceeded)
	data := marshalJournalSummaryRecord(t, oldest)
	data = bytesReplaceOnce(t, data, `"status":"succeeded"`, `"status":"unknown"`)
	writeJournalSummaryBytes(t, operations, oldest.OperationID+".json", data, 0o600)

	if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
		t.Fatalf("invalid truncated candidate error = %v", err)
	}
	os.Remove(filepath.Join(operations, oldest.OperationID+".json"))
	set, err := ReadJournalSummaries(context.Background())
	if err != nil || !set.Truncated || len(set.Records) != journalSummaryRecordLimit {
		t.Fatalf("truncated set = %+v, %v", set, err)
	}
}

func TestReadJournalSummariesRejectsEntryTypeAndExactMode(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, string, Record)
	}{
		{name: "unexpected name", mutate: func(t *testing.T, _, operations string, _ Record) {
			writeJournalSummaryBytes(t, operations, ".hidden", []byte("x"), 0o600)
		}},
		{name: "record directory", mutate: func(t *testing.T, _, operations string, record Record) {
			mustJournalSummaryMkdir(t, filepath.Join(operations, record.OperationID+".json"), 0o600)
		}},
		{name: "record symlink", mutate: func(t *testing.T, root, operations string, record Record) {
			outside := filepath.Join(root, "outside")
			writeJournalSummaryBytes(t, root, "outside", marshalJournalSummaryRecord(t, record), 0o600)
			if err := os.Symlink(outside, filepath.Join(operations, record.OperationID+".json")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "record broad mode", mutate: func(t *testing.T, _, operations string, record Record) {
			writeJournalSummaryBytes(t, operations, record.OperationID+".json", marshalJournalSummaryRecord(t, record), 0o644)
		}},
		{name: "record special mode", mutate: func(t *testing.T, _, operations string, record Record) {
			path := filepath.Join(operations, record.OperationID+".json")
			writeJournalSummaryBytes(t, operations, record.OperationID+".json", marshalJournalSummaryRecord(t, record), 0o600)
			if err := unix.Chmod(path, 0o1600); err != nil {
				t.Fatal(err)
			}
			if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSticky == 0 {
				t.Fatalf("special-mode fixture = %v, %v", info, err)
			}
		}},
		{name: "state broad mode", mutate: func(t *testing.T, root, _ string, _ Record) {
			if err := os.Chmod(filepath.Join(root, "dotfiles"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "operations special mode", mutate: func(t *testing.T, _, operations string, _ Record) {
			if err := unix.Chmod(operations, 0o1700); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, operations := journalSummaryFixture(t)
			record := journalSummaryRecord(time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded)
			test.mutate(t, root, operations, record)
			if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
				t.Fatalf("entry error = %v", err)
			}
		})
	}
}

func TestReadJournalSummariesStrictJSONAndRecordValidation(t *testing.T) {
	base := time.Date(2026, 7, 12, 12, 0, 0, 123456789, time.UTC)
	baseRecord := journalSummaryRecord(base, "0000000000000001", StatusSucceeded)
	valid := string(marshalJournalSummaryRecord(t, baseRecord))
	tests := []struct {
		name string
		data string
	}{
		{name: "duplicate top key", data: strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1)},
		{name: "duplicate nested key", data: strings.Replace(valid, `"action_id":"private-action"`, `"action_id":"private-action","action_id":"again"`, 1)},
		{name: "unknown top key", data: strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"private_path":"/secret"`, 1)},
		{name: "unknown nested key", data: strings.Replace(valid, `"action_id":"private-action"`, `"action_id":"private-action","private":"secret"`, 1)},
		{name: "trailing value", data: valid + `{}`},
		{name: "trailing garbage", data: valid + `garbage`},
		{name: "wrong schema", data: strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1)},
		{name: "offset timestamp", data: strings.Replace(valid, `2026-07-12T12:00:00.123456789Z`, `2026-07-12T05:00:00.123456789-07:00`, 1)},
		{name: "noncanonical timestamp", data: strings.Replace(valid, `2026-07-12T12:00:00.123456789Z`, `2026-07-12T12:00:00.1234567890Z`, 1)},
		{name: "escaped timestamp", data: strings.Replace(valid, `2026-07-12T12:00:00.123456789Z`, `\u0032026-07-12T12:00:00.123456789Z`, 1)},
		{name: "null finish", data: strings.Replace(valid, `"finished_at":"2026-07-12T12:00:01.373456789Z"`, `"finished_at":null`, 1)},
		{name: "finish before start", data: strings.Replace(valid, `2026-07-12T12:00:01.373456789Z`, `2026-07-12T11:59:59.123456789Z`, 1)},
		{name: "operation id mismatch", data: strings.Replace(valid, baseRecord.OperationID, strings.TrimSuffix(baseRecord.OperationID, "1")+"2", 1)},
		{name: "timestamp prefix mismatch", data: strings.Replace(valid, `2026-07-12T12:00:00.123456789Z`, `2026-07-12T12:00:01.123456789Z`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, operations := journalSummaryFixture(t)
			writeJournalSummaryBytes(t, operations, baseRecord.OperationID+".json", []byte(test.data), 0o600)
			if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
				t.Fatalf("strict record error = %v", err)
			}
		})
	}
}

func TestReadJournalSummariesLimits(t *testing.T) {
	t.Run("name sentinel precedes validation", func(t *testing.T) {
		_, operations := journalSummaryFixture(t)
		for index := 0; index < journalSummaryNameLimit+1; index++ {
			writeJournalSummaryBytes(t, operations, fmt.Sprintf("invalid-%04d", index), nil, 0o600)
		}
		if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesLimit) {
			t.Fatalf("name limit error = %v", err)
		}
	})

	t.Run("per record bytes", func(t *testing.T) {
		_, operations := journalSummaryFixture(t)
		record := journalSummaryRecord(time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded)
		writeJournalSummaryBytes(t, operations, record.OperationID+".json", make([]byte, journalSummaryFileByteLimit+1), 0o600)
		if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesLimit) {
			t.Fatalf("record limit error = %v", err)
		}
	})

	t.Run("aggregate includes revalidation", func(t *testing.T) {
		_, operations := journalSummaryFixture(t)
		base := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
		for index := 0; index < 17; index++ {
			record := journalSummaryRecord(base.Add(time.Duration(index)*time.Second), fmt.Sprintf("%016x", index+1), StatusSucceeded)
			record.Warnings = []string{strings.Repeat("x", int(journalSummaryFileByteLimit)-2048)}
			data := marshalJournalSummaryRecord(t, record)
			if int64(len(data)) >= journalSummaryFileByteLimit {
				t.Fatalf("fixture record too large: %d", len(data))
			}
			writeJournalSummaryBytes(t, operations, record.OperationID+".json", data, 0o600)
		}
		if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesLimit) {
			t.Fatalf("aggregate limit error = %v", err)
		}
	})
}

func TestReadJournalSummariesRejectsConcurrentDrift(t *testing.T) {
	tests := []struct {
		name string
		hook func(*journalSummaryHooks, string, string, Record)
	}{
		{name: "name set", hook: func(h *journalSummaryHooks, _, operations string, _ Record) {
			h.afterInitialReads = func() { writeJournalSummaryBytesNoTest(operations, ".appeared", []byte("x"), 0o600) }
		}},
		{name: "record replacement", hook: func(h *journalSummaryHooks, _, operations string, record Record) {
			h.afterInitialReads = func() {
				path := filepath.Join(operations, record.OperationID+".json")
				temporary := path + ".replacement"
				writeJournalSummaryBytesNoTest(operations, filepath.Base(temporary), marshalJournalSummaryRecordNoTest(record), 0o600)
				_ = os.Rename(temporary, path)
			}
		}},
		{name: "directory graft", hook: func(h *journalSummaryHooks, root, operations string, record Record) {
			h.beforeRevalidate = func() {
				_ = os.Rename(operations, filepath.Join(root, "accepted-operations"))
				_ = os.Mkdir(operations, 0o700)
				writeJournalSummaryBytesNoTest(operations, record.OperationID+".json", marshalJournalSummaryRecordNoTest(record), 0o600)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, operations := journalSummaryFixture(t)
			record := journalSummaryRecord(time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded)
			writeJournalSummaryRecord(t, operations, record)
			setJournalSummaryHooks(t, func(hooks *journalSummaryHooks) { test.hook(hooks, root, operations, record) })
			if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
				t.Fatalf("drift error = %v", err)
			}
		})
	}
}

func TestReadJournalSummariesRejectsStateSwapBetweenAuthorityCaptures(t *testing.T) {
	root, operations := journalSummaryFixture(t)
	state := filepath.Dir(operations)
	acceptedState := filepath.Join(root, "accepted-state")
	setJournalSummaryHooks(t, func(hooks *journalSummaryHooks) {
		hooks.afterStateCapture = func() {
			if err := os.Rename(state, acceptedState); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(state, "operations"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	})

	if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
		t.Fatalf("state authority swap error = %v", err)
	}
}

func TestReadJournalSummariesRejectsStateSwapToMissingOperationsAfterParentDerivation(t *testing.T) {
	root, operations := journalSummaryFixture(t)
	state := filepath.Dir(operations)
	acceptedState := filepath.Join(root, "accepted-state")
	setJournalSummaryHooks(t, func(hooks *journalSummaryHooks) {
		hooks.beforeOperationsCapture = func() {
			if err := os.Rename(state, acceptedState); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
		}
	})

	if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) || errors.Is(err, ErrJournalSummariesNotPresent) {
		t.Fatalf("missing operations below replacement state error = %v", err)
	}
}

func TestReadJournalSummariesRejectsStateSpecialModeBeforeOperationsCapture(t *testing.T) {
	root, _ := journalSummaryFixture(t)
	state := filepath.Join(root, "dotfiles")
	setJournalSummaryHooks(t, func(hooks *journalSummaryHooks) {
		hooks.beforeOperationsCapture = func() {
			if err := unix.Chmod(state, 0o1700); err != nil {
				t.Fatal(err)
			}
		}
	})

	if _, err := ReadJournalSummaries(context.Background()); !errors.Is(err, ErrJournalSummariesInvalid) {
		t.Fatalf("state special-mode error = %v", err)
	}
}

func TestReadJournalSummariesAppliesAggregateLimitBeforeRecordRead(t *testing.T) {
	_, operations := journalSummaryFixture(t)
	record := journalSummaryRecord(time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded)
	writeJournalSummaryRecord(t, operations, record)
	authority, err := captureJournalSummaryAuthority()
	if err != nil {
		t.Fatal(err)
	}

	readCalls := 0
	observedLimit := int64(-1)
	setJournalSummaryHooks(t, func(hooks *journalSummaryHooks) {
		hooks.beforeRecordRead = func(revalidation bool, name string, limit int64) error {
			readCalls++
			observedLimit = limit
			if !revalidation || name != record.OperationID+".json" {
				t.Fatalf("bounded read hook = revalidation:%t name:%q", revalidation, name)
			}
			return nil
		}
	})

	total := journalSummaryTotalByteLimit - 64
	if _, _, err := readJournalSummaryRecordBounded(authority, record.OperationID+".json", &total, true); !errors.Is(err, ErrJournalSummariesLimit) {
		t.Fatalf("remaining-budget read error = %v", err)
	}
	if readCalls != 1 || observedLimit != 64 || total != journalSummaryTotalByteLimit-64 {
		t.Fatalf("remaining-budget read = calls:%d limit:%d total:%d", readCalls, observedLimit, total)
	}

	total = journalSummaryTotalByteLimit
	if _, _, err := readJournalSummaryRecordBounded(authority, record.OperationID+".json", &total, true); !errors.Is(err, ErrJournalSummariesLimit) {
		t.Fatalf("zero-budget read error = %v", err)
	}
	if readCalls != 1 {
		t.Fatalf("zero remaining budget attempted a read: calls=%d", readCalls)
	}
}

func TestReadJournalSummariesKeepsFinalGenericReadFailureUnavailable(t *testing.T) {
	_, operations := journalSummaryFixture(t)
	record := journalSummaryRecord(time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded)
	writeJournalSummaryRecord(t, operations, record)
	setJournalSummaryHooks(t, func(hooks *journalSummaryHooks) {
		hooks.beforeRecordRead = func(revalidation bool, _ string, _ int64) error {
			if revalidation {
				return fs.ErrPermission
			}
			return nil
		}
	})

	set, err := ReadJournalSummaries(context.Background())
	if !errors.Is(err, ErrJournalSummariesUnavailable) || errors.Is(err, ErrJournalSummariesInvalid) ||
		err.Error() != ErrJournalSummariesUnavailable.Error() || set.Records == nil || len(set.Records) != 0 {
		t.Fatalf("final generic read failure = %+v, %v", set, err)
	}
}

func TestReadJournalSummariesHonorsContext(t *testing.T) {
	root, operations := journalSummaryFixture(t)
	record := journalSummaryRecord(time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC), "0000000000000001", StatusSucceeded)
	writeJournalSummaryRecord(t, operations, record)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	before := journalSummaryInventory(t, root)
	if _, err := ReadJournalSummaries(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("entry cancellation error = %v", err)
	}
	if !reflect.DeepEqual(before, journalSummaryInventory(t, root)) {
		t.Fatal("entry cancellation changed filesystem")
	}

	during, cancelDuring := context.WithCancel(context.Background())
	setJournalSummaryHooks(t, func(hooks *journalSummaryHooks) {
		hooks.afterNames = cancelDuring
	})
	if _, err := ReadJournalSummaries(during); !errors.Is(err, context.Canceled) {
		t.Fatalf("during cancellation error = %v", err)
	}
}

type journalSummaryInventoryEntry struct {
	Mode fs.FileMode
	Data string
}

func journalSummaryFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	state := filepath.Join(root, "dotfiles")
	operations := filepath.Join(state, "operations")
	mustJournalSummaryMkdir(t, state, 0o700)
	mustJournalSummaryMkdir(t, operations, 0o700)
	return root, operations
}

func journalSummaryRecord(start time.Time, suffix string, status Status) Record {
	finish := start.Add(1250 * time.Millisecond)
	record := Record{
		SchemaVersion: CurrentJournalSchemaVersion,
		OperationID:   start.UTC().Format("20060102T150405.000000000Z") + "-" + suffix,
		PlanHash:      strings.Repeat("a", 64), StartedAt: start.UTC(), Status: status,
		Actions: []ActionResult{{ActionID: "private-action", Status: ActionSucceeded, Summary: "private summary"}},
	}
	if status != StatusRunning {
		record.FinishedAt = &finish
	}
	return record
}

func writeJournalSummaryRecord(t *testing.T, operations string, record Record) {
	t.Helper()
	writeJournalSummaryBytes(t, operations, record.OperationID+".json", marshalJournalSummaryRecord(t, record), 0o600)
}

func marshalJournalSummaryRecord(t *testing.T, record Record) []byte {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func marshalJournalSummaryRecordNoTest(record Record) []byte {
	data, _ := json.Marshal(record)
	return append(data, '\n')
}

func writeJournalSummaryBytes(t *testing.T, directory, name string, data []byte, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), data, mode); err != nil {
		t.Fatal(err)
	}
}

func writeJournalSummaryBytesNoTest(directory, name string, data []byte, mode fs.FileMode) {
	_ = os.WriteFile(filepath.Join(directory, name), data, mode)
}

func mustJournalSummaryMkdir(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, mode); err != nil {
		t.Fatal(err)
	}
}

func setJournalSummaryHooks(t *testing.T, configure func(*journalSummaryHooks)) {
	t.Helper()
	journalSummaryTestHooks = journalSummaryHooks{}
	configure(&journalSummaryTestHooks)
	t.Cleanup(func() { journalSummaryTestHooks = journalSummaryHooks{} })
}

func journalSummaryInventory(t *testing.T, root string) map[string]journalSummaryInventoryEntry {
	t.Helper()
	inventory := make(map[string]journalSummaryInventoryEntry)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		item := journalSummaryInventoryEntry{Mode: info.Mode()}
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			item.Data = string(data)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			item.Data = target
		}
		inventory[filepath.ToSlash(rel)] = item
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

func bytesReplaceOnce(t *testing.T, data []byte, old, replacement string) []byte {
	t.Helper()
	result := strings.Replace(string(data), old, replacement, 1)
	if result == string(data) {
		t.Fatalf("fixture does not contain %q", old)
	}
	return []byte(result)
}
