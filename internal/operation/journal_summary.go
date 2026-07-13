package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/safefile"
)

const (
	journalSummaryNameLimit      = 1024
	journalSummaryRecordLimit    = 20
	journalSummaryFileByteLimit  = int64(1 << 20)
	journalSummaryTotalByteLimit = int64(32 << 20)
)

var (
	ErrJournalSummariesNotPresent  = errors.New("operation journal summaries are not present")
	ErrJournalSummariesInvalid     = errors.New("operation journal summaries are invalid")
	ErrJournalSummariesUnavailable = errors.New("operation journal summaries are unavailable")
	ErrJournalSummariesLimit       = errors.New("operation journal summary limit exceeded")
)

// JournalSummarySet contains only facts approved for public support output.
// It deliberately omits record identities, paths, hashes, timestamps, and text.
type JournalSummarySet struct {
	Truncated bool
	Records   []JournalSummary
}

type JournalSummary struct {
	Ordinal              int
	Status               Status
	Actions              JournalActionCounts
	BackupRecorded       bool
	Rollback             *JournalRollbackSummary
	DurationMilliseconds *int64
}

type JournalActionCounts struct {
	Pending   int
	Succeeded int
	Failed    int
	Skipped   int
}

type JournalRollbackSummary struct {
	Status   RollbackStatus
	Restored int
	Removed  int
	Skipped  int
	Warnings int
}

type journalSummaryAuthority struct {
	root       string
	operations string
	parents    *safefile.ParentChain
	directory  *safefile.DirectorySnapshot
}

type journalSummaryCandidate struct {
	name     string
	record   Record
	revision safefile.Revision
	summary  JournalSummary
}

type journalSummaryHooks struct {
	afterStateCapture       func()
	beforeOperationsCapture func()
	afterNames              func()
	afterInitialReads       func()
	beforeRevalidate        func()
	beforeRecordRead        func(revalidation bool, name string, limit int64) error
}

var journalSummaryTestHooks journalSummaryHooks

// ReadJournalSummaries reads the existing private journal without creating,
// locking, or mutating operational state. Returned data is safe to project
// directly into the allowlisted support-document operation section.
func ReadJournalSummaries(ctx context.Context) (JournalSummarySet, error) {
	empty := JournalSummarySet{Records: []JournalSummary{}}
	if err := contextError(ctx); err != nil {
		return empty, err
	}
	authority, err := captureJournalSummaryAuthority()
	if err != nil {
		return empty, err
	}
	if err := contextError(ctx); err != nil {
		return empty, err
	}

	names, err := readJournalSummaryNames(ctx, authority)
	if err != nil {
		return empty, err
	}
	if hook := journalSummaryTestHooks.afterNames; hook != nil {
		hook()
	}
	if err := contextError(ctx); err != nil {
		return empty, err
	}

	candidates := make([]journalSummaryCandidate, 0, len(names))
	var totalBytes int64
	for _, name := range names {
		if err := contextError(ctx); err != nil {
			return empty, err
		}
		data, revision, readErr := readJournalSummaryRecordBounded(authority, name, &totalBytes, false)
		if readErr != nil {
			return empty, readErr
		}
		if !revision.Exists() || revision.Mode() != 0o600 {
			return empty, ErrJournalSummariesInvalid
		}
		record, decodeErr := decodeJournalSummaryRecord(data, name)
		if decodeErr != nil {
			return empty, ErrJournalSummariesInvalid
		}
		summary, summaryErr := summarizeJournalRecord(record)
		if summaryErr != nil {
			return empty, ErrJournalSummariesInvalid
		}
		candidates = append(candidates, journalSummaryCandidate{name: name, record: record, revision: revision, summary: summary})
	}
	if hook := journalSummaryTestHooks.afterInitialReads; hook != nil {
		hook()
	}
	if err := contextError(ctx); err != nil {
		return empty, err
	}

	sort.Slice(candidates, func(left, right int) bool {
		if !candidates[left].record.StartedAt.Equal(candidates[right].record.StartedAt) {
			return candidates[left].record.StartedAt.After(candidates[right].record.StartedAt)
		}
		if candidates[left].record.OperationID != candidates[right].record.OperationID {
			return candidates[left].record.OperationID > candidates[right].record.OperationID
		}
		return candidates[left].name > candidates[right].name
	})

	if hook := journalSummaryTestHooks.beforeRevalidate; hook != nil {
		hook()
	}
	if err := revalidateJournalSummaries(ctx, authority, names, candidates, &totalBytes); err != nil {
		return empty, err
	}

	count := len(candidates)
	if count > journalSummaryRecordLimit {
		count = journalSummaryRecordLimit
		empty.Truncated = true
	}
	empty.Records = make([]JournalSummary, count)
	for index := 0; index < count; index++ {
		summary := candidates[index].summary
		summary.Ordinal = index
		empty.Records[index] = cloneJournalSummary(summary)
	}
	return empty, nil
}

func captureJournalSummaryAuthority() (*journalSummaryAuthority, error) {
	root, stateRel, err := stateAnchor()
	if err != nil {
		return nil, ErrJournalSummariesUnavailable
	}
	stateDirectory, stateParents, err := safefile.CaptureDirectoryRootWithin(root, stateRel)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrJournalSummariesNotPresent
	}
	if err != nil {
		return nil, classifyJournalSummaryDirectoryCaptureError(root, stateRel, err)
	}
	if stateDirectory.Permissions() != 0o700 {
		return nil, ErrJournalSummariesInvalid
	}
	state, err := safefile.OpenDirectoryWithinAuthorized(root, stateRel, stateParents, stateDirectory)
	if err != nil {
		return nil, classifyJournalSummaryAuthorityError(err)
	}
	if !journalSummaryDirectoryHasExactMode(state) {
		_ = state.Close()
		return nil, ErrJournalSummariesInvalid
	}
	if err := state.Close(); err != nil {
		return nil, ErrJournalSummariesUnavailable
	}
	if hook := journalSummaryTestHooks.afterStateCapture; hook != nil {
		hook()
	}

	operationsRel := filepath.ToSlash(filepath.Join(stateRel, "operations"))
	if hook := journalSummaryTestHooks.beforeOperationsCapture; hook != nil {
		hook()
	}
	operationsDirectory, operationsParents, err := safefile.CaptureChildDirectoryWithinAuthorized(root, stateRel, "operations", stateParents, stateDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrJournalSummariesNotPresent
	}
	if err != nil {
		return nil, classifyJournalSummaryDirectoryCaptureError(root, operationsRel, err)
	}
	if operationsDirectory.Permissions() != 0o700 {
		return nil, ErrJournalSummariesInvalid
	}
	return &journalSummaryAuthority{
		root: root, operations: operationsRel,
		parents: operationsParents, directory: operationsDirectory,
	}, nil
}

func readJournalSummaryNames(ctx context.Context, authority *journalSummaryAuthority) ([]string, error) {
	directory, err := safefile.OpenDirectoryWithinAuthorized(authority.root, authority.operations, authority.parents, authority.directory)
	if err != nil {
		return nil, classifyJournalSummaryAuthorityError(err)
	}
	if !journalSummaryDirectoryHasExactMode(directory) {
		_ = directory.Close()
		return nil, ErrJournalSummariesInvalid
	}
	names, readErr := directory.Readdirnames(journalSummaryNameLimit + 1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, ErrJournalSummariesUnavailable
	}
	if closeErr != nil {
		return nil, ErrJournalSummariesUnavailable
	}
	if len(names) > journalSummaryNameLimit {
		return nil, ErrJournalSummariesLimit
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !validJournalSummaryFilename(name) {
			return nil, ErrJournalSummariesInvalid
		}
		if _, exists := seen[name]; exists {
			return nil, ErrJournalSummariesInvalid
		}
		seen[name] = struct{}{}
	}
	sort.Strings(names)
	return names, nil
}

func readJournalSummaryRecord(authority *journalSummaryAuthority, name string, limit int64) ([]byte, safefile.Revision, error) {
	rel := filepath.ToSlash(filepath.Join(authority.operations, name))
	parents, err := safefile.ExtendParentChainWithinDirectory(authority.root, rel, authority.operations, authority.parents, authority.directory)
	if err != nil {
		return nil, safefile.Revision{}, err
	}
	return safefile.ReadWithinAuthorizedLimit(authority.root, rel, parents, limit)
}

func readJournalSummaryRecordBounded(authority *journalSummaryAuthority, name string, totalBytes *int64, revalidation bool) ([]byte, safefile.Revision, error) {
	if totalBytes == nil || *totalBytes < 0 || *totalBytes >= journalSummaryTotalByteLimit {
		return nil, safefile.Revision{}, ErrJournalSummariesLimit
	}
	remaining := journalSummaryTotalByteLimit - *totalBytes
	limit := journalSummaryFileByteLimit
	if remaining < limit {
		limit = remaining
	}
	if limit <= 0 {
		return nil, safefile.Revision{}, ErrJournalSummariesLimit
	}
	if hook := journalSummaryTestHooks.beforeRecordRead; hook != nil {
		if err := hook(revalidation, name, limit); err != nil {
			return nil, safefile.Revision{}, classifyJournalSummaryReadError(err)
		}
	}
	data, revision, err := readJournalSummaryRecord(authority, name, limit)
	if err != nil {
		return nil, safefile.Revision{}, classifyJournalSummaryReadError(err)
	}
	if !addJournalSummaryBytes(totalBytes, len(data)) {
		return nil, safefile.Revision{}, ErrJournalSummariesLimit
	}
	return data, revision, nil
}

func revalidateJournalSummaries(ctx context.Context, authority *journalSummaryAuthority, names []string, candidates []journalSummaryCandidate, totalBytes *int64) error {
	currentNames, err := readJournalSummaryNames(ctx, authority)
	if err != nil {
		return err
	}
	if !equalJournalSummaryNames(names, currentNames) {
		return ErrJournalSummariesInvalid
	}
	byName := make(map[string]safefile.Revision, len(candidates))
	for _, candidate := range candidates {
		byName[candidate.name] = candidate.revision
	}
	for _, name := range currentNames {
		if err := contextError(ctx); err != nil {
			return err
		}
		_, revision, readErr := readJournalSummaryRecordBounded(authority, name, totalBytes, true)
		if readErr != nil {
			return readErr
		}
		if !revision.Exists() || revision.Mode() != 0o600 || revision != byName[name] {
			return ErrJournalSummariesInvalid
		}
	}
	return nil
}

func decodeJournalSummaryRecord(data []byte, name string) (Record, error) {
	if err := rejectDuplicateJournalKeys(data); err != nil {
		return Record{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire journalRecordWire
	if err := decoder.Decode(&wire); err != nil {
		return Record{}, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Record{}, err
	}
	record := wire.record()
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	stem := strings.TrimSuffix(name, ".json")
	if record.OperationID != stem || record.StartedAt.UTC().Format("20060102T150405.000000000Z") != stem[:26] {
		return Record{}, ErrJournalSummariesInvalid
	}
	return record, nil
}

type journalRecordWire struct {
	SchemaVersion int                       `json:"schema_version"`
	OperationID   string                    `json:"operation_id"`
	PlanHash      string                    `json:"plan_hash"`
	StartedAt     canonicalJournalTimestamp `json:"started_at"`
	FinishedAt    optionalJournalTimestamp  `json:"finished_at,omitempty"`
	Status        Status                    `json:"status"`
	Backup        string                    `json:"backup,omitempty"`
	Rollback      *RollbackResult           `json:"rollback,omitempty"`
	Actions       []ActionResult            `json:"actions"`
	Warnings      []string                  `json:"warnings,omitempty"`
}

func (wire journalRecordWire) record() Record {
	record := Record{
		SchemaVersion: wire.SchemaVersion, OperationID: wire.OperationID, PlanHash: wire.PlanHash,
		StartedAt: wire.StartedAt.value, Status: wire.Status, Backup: wire.Backup, Rollback: wire.Rollback,
		Actions: wire.Actions, Warnings: wire.Warnings,
	}
	if wire.FinishedAt.present {
		finished := wire.FinishedAt.value
		record.FinishedAt = &finished
	}
	return record
}

type canonicalJournalTimestamp struct {
	value time.Time
}

func (timestamp *canonicalJournalTimestamp) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil || !strings.HasSuffix(raw, "Z") {
		return ErrJournalSummariesInvalid
	}
	canonicalJSON, err := json.Marshal(raw)
	if err != nil || !bytes.Equal(data, canonicalJSON) {
		return ErrJournalSummariesInvalid
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || parsed.Format(time.RFC3339Nano) != raw {
		return ErrJournalSummariesInvalid
	}
	timestamp.value = parsed
	return nil
}

type optionalJournalTimestamp struct {
	present bool
	value   time.Time
}

func (timestamp *optionalJournalTimestamp) UnmarshalJSON(data []byte) error {
	timestamp.present = true
	if bytes.Equal(data, []byte("null")) {
		return ErrJournalSummariesInvalid
	}
	var canonical canonicalJournalTimestamp
	if err := canonical.UnmarshalJSON(data); err != nil {
		return err
	}
	timestamp.value = canonical.value
	return nil
}

func rejectDuplicateJournalKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return ErrJournalSummariesInvalid
	}
	if err := consumeJournalObject(decoder); err != nil {
		return err
	}
	return requireJSONTokenEOF(decoder)
}

func consumeJournalObject(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok {
			return ErrJournalSummariesInvalid
		}
		if _, duplicate := seen[name]; duplicate {
			return ErrJournalSummariesInvalid
		}
		seen[name] = struct{}{}
		if err := consumeJournalValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return ErrJournalSummariesInvalid
	}
	return nil
}

func consumeJournalValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return consumeJournalObject(decoder)
	case '[':
		for decoder.More() {
			if err := consumeJournalValue(decoder); err != nil {
				return err
			}
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim(']') {
			return ErrJournalSummariesInvalid
		}
		return nil
	default:
		return ErrJournalSummariesInvalid
	}
}

func requireJSONTokenEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return ErrJournalSummariesInvalid
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrJournalSummariesInvalid
	}
	return nil
}

func summarizeJournalRecord(record Record) (JournalSummary, error) {
	summary := JournalSummary{Status: record.Status, BackupRecorded: record.Backup != ""}
	for _, action := range record.Actions {
		switch action.Status {
		case ActionPending:
			summary.Actions.Pending++
		case ActionSucceeded:
			summary.Actions.Succeeded++
		case ActionFailed:
			summary.Actions.Failed++
		case ActionSkipped:
			summary.Actions.Skipped++
		default:
			return JournalSummary{}, ErrJournalSummariesInvalid
		}
	}
	if record.Rollback != nil {
		summary.Rollback = &JournalRollbackSummary{
			Status: record.Rollback.Status, Restored: record.Rollback.Restored, Removed: record.Rollback.Removed,
			Skipped: record.Rollback.Skipped, Warnings: record.Rollback.Warnings,
		}
	}
	if record.Status != StatusRunning {
		if record.FinishedAt == nil {
			return JournalSummary{}, ErrJournalSummariesInvalid
		}
		milliseconds, ok := journalDurationMilliseconds(record.StartedAt, *record.FinishedAt)
		if !ok {
			return JournalSummary{}, ErrJournalSummariesInvalid
		}
		summary.DurationMilliseconds = &milliseconds
	}
	return summary, nil
}

func journalDurationMilliseconds(started, finished time.Time) (int64, bool) {
	if finished.Before(started) {
		return 0, false
	}
	seconds := finished.Unix() - started.Unix()
	nanoseconds := int64(finished.Nanosecond()) - int64(started.Nanosecond())
	if nanoseconds < 0 {
		seconds--
		nanoseconds += int64(time.Second)
	}
	if seconds < 0 || seconds > math.MaxInt64/1000 {
		return 0, false
	}
	milliseconds := seconds*1000 + nanoseconds/int64(time.Millisecond)
	if milliseconds < 0 {
		return 0, false
	}
	return milliseconds, true
}

func validJournalSummaryFilename(name string) bool {
	if len(name) != 48 || name[8] != 'T' || name[15] != '.' || name[25] != 'Z' || name[26] != '-' || name[43:] != ".json" {
		return false
	}
	for index := 0; index < 43; index++ {
		switch index {
		case 8, 15, 25, 26:
			continue
		}
		character := name[index]
		if index >= 27 {
			if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
				return false
			}
			continue
		}
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func addJournalSummaryBytes(total *int64, size int) bool {
	if size < 0 || int64(size) > journalSummaryTotalByteLimit-*total {
		return false
	}
	*total += int64(size)
	return true
}

func equalJournalSummaryNames(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneJournalSummary(summary JournalSummary) JournalSummary {
	if summary.Rollback != nil {
		rollback := *summary.Rollback
		summary.Rollback = &rollback
	}
	if summary.DurationMilliseconds != nil {
		duration := *summary.DurationMilliseconds
		summary.DurationMilliseconds = &duration
	}
	return summary
}

func journalSummaryDirectoryHasExactMode(directory *os.File) bool {
	info, err := directory.Stat()
	return err == nil && info.Mode() == os.ModeDir|0o700
}

func classifyJournalSummaryAuthorityError(err error) error {
	if errors.Is(err, safefile.ErrParentChanged) || errors.Is(err, safefile.ErrDirectoryChanged) || errors.Is(err, safefile.ErrSymlink) ||
		errors.Is(err, safefile.ErrNonRegular) || errors.Is(err, safefile.ErrInvalidPath) || errors.Is(err, safefile.ErrInvalidAuthority) {
		return ErrJournalSummariesInvalid
	}
	return ErrJournalSummariesUnavailable
}

func classifyJournalSummaryDirectoryCaptureError(root, rel string, err error) error {
	info, inspectErr := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if inspectErr == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return ErrJournalSummariesInvalid
	}
	return classifyJournalSummaryAuthorityError(err)
}

func classifyJournalSummaryReadError(err error) error {
	if errors.Is(err, safefile.ErrSizeLimit) {
		return ErrJournalSummariesLimit
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, safefile.ErrParentChanged) || errors.Is(err, safefile.ErrDirectoryChanged) ||
		errors.Is(err, safefile.ErrRevisionChanged) || errors.Is(err, safefile.ErrSymlink) || errors.Is(err, safefile.ErrNonRegular) ||
		errors.Is(err, safefile.ErrInvalidPath) || errors.Is(err, safefile.ErrInvalidAuthority) {
		return ErrJournalSummariesInvalid
	}
	return ErrJournalSummariesUnavailable
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
