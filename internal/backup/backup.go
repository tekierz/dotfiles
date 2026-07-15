// Package backup provides shared logic for creating and restoring dotfile
// backups. Both the CLI (cmd/dotfiles) and the TUI (internal/ui) use these
// helpers so the encode/decode and restore paths cannot diverge.
package backup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

// ManifestName is the file inside each backup directory that records the
// original relative path (and mode) of every backed-up file.
const ManifestName = "manifest.txt"

// defaultFileMode is used when a backup does not record the original file
// mode (e.g. legacy backups created before modes were stored). 0600 matches
// the project's documented permission policy for config/dotfiles.
const defaultFileMode os.FileMode = 0o600

var restorePlanTestHooks struct {
	afterRootValidation func(PlanResult) error
}

// Entry describes a single file in a backup: its original path relative to
// the user's home directory and the mode it should be restored with.
type Entry struct {
	RelPath string
	Mode    os.FileMode
}

// EncodeName converts a relative path into the flat filename used to store a
// backed-up file. Path separators become underscores.
//
// NOTE: this encoding is intentionally lossy (an underscore in a component is
// indistinguishable from a separator). It is kept only so that flat backup
// files have stable, human-readable names; the manifest is the authoritative
// source for the original path on restore. See ReadManifest / Restore.
func EncodeName(relPath string) string {
	return strings.ReplaceAll(relPath, string(os.PathSeparator), "_")
}

// ManifestLine renders one manifest entry as "relpath\tmode" (octal). Restore
// parses this back into an Entry. The tab separator is safe because manifest
// relative paths never contain tabs.
func ManifestLine(relPath string, mode os.FileMode) string {
	return fmt.Sprintf("%s\t%o", relPath, mode.Perm())
}

// ReadManifest reads and parses the manifest inside backupDir. It returns the
// list of entries (path + mode) recorded at backup time. Lines without a mode
// field (legacy manifests) are parsed with defaultFileMode. Bash installer
// manifests are pipe-separated absolute paths; those are converted back to
// home-relative paths for existed=yes entries. A missing manifest returns
// (nil, nil) so callers can decide how to handle pre-manifest backups.
func ReadManifest(backupDir string) ([]Entry, error) {
	data, revision, err := readBackupDescendant(backupDir, ManifestName)
	if err != nil {
		return nil, err
	}
	if !revision.Exists() {
		return nil, nil
	}
	home, _ := os.UserHomeDir()
	return parseManifestData(data, home)
}

func parseManifestData(data []byte, home string) ([]Entry, error) {
	var entries []Entry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(line, "|") {
			entry, ok, err := readBashManifestEntry(line, home)
			if err != nil {
				return nil, err
			}
			if ok {
				entries = append(entries, entry)
			}
			continue
		}
		// Format: "relpath" (legacy) or "relpath\tmode" (current).
		relPath := line
		mode := defaultFileMode
		if tab := strings.LastIndexByte(line, '\t'); tab >= 0 {
			relPath = line[:tab]
			parsed, perr := parseOctalMode(line[tab+1:])
			if perr != nil {
				return nil, fmt.Errorf("invalid explicit mode for %q: %w", relPath, perr)
			}
			mode = parsed
		}
		entries = append(entries, Entry{RelPath: relPath, Mode: mode})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// readBackupDescendant treats both the backup-root basename and the selected
// backup basename as untrusted descendants of their shared parent anchor. This
// refuses a symlinked `backups` directory, a symlinked selected backup, and
// every symlink/non-directory below it. Trusting either of those two directory
// names as the safefile root would intentionally allow that root to be a
// symlink. Restore callers always pass an absolute <root>/<selected> layout;
// degenerate paths without both components are rejected.
func readBackupDescendant(backupDir, rel string) ([]byte, safefile.Revision, error) {
	anchor, anchoredRel, err := backupDescendantAnchor(backupDir, rel)
	if err != nil {
		return nil, safefile.Revision{}, err
	}
	return safefile.ReadWithin(anchor, anchoredRel)
}

func backupDescendantAnchor(backupDir, rel string) (string, string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("invalid backup descendant %q", rel)
	}
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		if component == "" || component == "." || component == ".." {
			return "", "", fmt.Errorf("invalid backup descendant %q", rel)
		}
	}
	cleanBackupDir := filepath.Clean(backupDir)
	if !filepath.IsAbs(cleanBackupDir) {
		return "", "", fmt.Errorf("invalid backup directory %q: path must be absolute", backupDir)
	}
	if strings.HasPrefix(cleanBackupDir, string(os.PathSeparator)+"var"+string(os.PathSeparator)) {
		if target, linkErr := os.Readlink(string(os.PathSeparator) + "var"); linkErr == nil {
			if target == "private/var" || target == "/private/var" {
				cleanBackupDir = filepath.Join(string(os.PathSeparator)+"private"+string(os.PathSeparator)+"var", strings.TrimPrefix(cleanBackupDir, string(os.PathSeparator)+"var"+string(os.PathSeparator)))
			}
		}
	}
	volume := filepath.VolumeName(cleanBackupDir)
	anchor := volume + string(os.PathSeparator)
	baseRel := strings.TrimPrefix(cleanBackupDir, anchor)
	if baseRel == "" || baseRel == "." || hasParentTraversal(baseRel) || !strings.Contains(baseRel, string(os.PathSeparator)) {
		return "", "", fmt.Errorf("invalid backup directory %q", backupDir)
	}
	// Anchor at the filesystem root so every untrusted component, including
	// .config/dotfiles or an external XDG state ancestor, is traversed with
	// O_NOFOLLOW. Promoting a nearby parent to a trusted root would follow a
	// symlink before safefile gets a chance to reject it.
	anchoredRel := filepath.ToSlash(filepath.Join(baseRel, rel))
	return anchor, anchoredRel, nil
}

// parseOctalMode parses an octal permission string (e.g. "600") into a mode.
func parseOctalMode(s string) (os.FileMode, error) {
	if s == "" {
		return 0, fmt.Errorf("mode is empty")
	}
	for _, digit := range s {
		if digit < '0' || digit > '7' {
			return 0, fmt.Errorf("mode %q is not a full octal permission string", s)
		}
	}
	parsed, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("parse mode %q: %w", s, err)
	}
	if parsed > 0o777 {
		return 0, fmt.Errorf("mode %q exceeds 0777", s)
	}
	return os.FileMode(parsed), nil
}

// safeJoin resolves relPath against home and verifies the result stays inside
// home. It rejects absolute paths and any path containing a ".." component
// before joining, so the guard cannot be bypassed by encoding order, and
// re-checks the cleaned result as defense-in-depth. It returns the cleaned
// destination path and true if the path is safe.
func safeJoin(home, relPath string) (string, bool) {
	// Reject absolute paths outright; they would escape home via Join.
	if filepath.IsAbs(relPath) {
		return "", false
	}
	// Reject explicit parent-directory traversal in any component.
	for _, part := range strings.Split(relPath, string(os.PathSeparator)) {
		if part == ".." {
			return "", false
		}
	}

	dstPath := filepath.Clean(filepath.Join(home, relPath))
	cleanHome := filepath.Clean(home)
	if dstPath != cleanHome && !strings.HasPrefix(dstPath, cleanHome+string(os.PathSeparator)) {
		return "", false
	}
	return dstPath, true
}

// IsRestorePathSafe reports whether restoring relPath under home stays within
// home. Exposed for tests and callers that want to validate without writing.
func IsRestorePathSafe(home, relPath string) bool {
	_, ok := safeJoin(home, relPath)
	return ok
}

// RestoreResult reports the outcome of a restore for a single backup.
type RestoreResult struct {
	// Restored is the relative path of each file successfully restored.
	Restored []string
	// Removed is the relative path of each file/directory removed because the
	// backup manifest recorded that it did not exist before installation.
	Removed []string
	// Skipped maps a backup item (relative path or filename) to the reason it
	// was not restored (path traversal, read/write error, etc.).
	Skipped map[string]string
	// Warnings maps a restored item to a post-commit durability/verification
	// warning. These entries ARE counted as restored because the replacement was
	// committed, but callers must surface the warning rather than claiming a
	// clean restore.
	Warnings map[string]string
}

// ExpectedState is the exact post-mutation state a rollback is permitted to
// replace or remove. Missing captures are intentionally not representable as a
// zero-value permission grant: callers must set Captured after a successful
// descriptor-anchored observation.
type ExpectedState struct {
	Attempted         bool
	Captured          bool
	Kind              TargetKind
	Exists            bool
	FileRevision      safefile.Revision
	DirectorySnapshot *safefile.DirectorySnapshot
	Parents           *safefile.ParentChain
	OriginalExists    bool
	OriginalCaptured  bool
	OriginalData      []byte
	OriginalMode      os.FileMode
	OriginalDirectory *safefile.DirectorySnapshot
	// EmptyOnly marks a parent directory created solely to make an accepted
	// leaf reachable. Rollback may remove it only after child targets have been
	// restored and while its exact identity remains empty.
	EmptyOnly bool
}

// CaptureExpectedState captures one exact post-write state for conditional
// rollback. It never follows symlinks or creates missing parents.
func CaptureExpectedState(home string, target Target) (ExpectedState, error) {
	rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(target.RelPath)))
	switch target.Kind {
	case TargetFile:
		_, revision, parents, err := safefile.ObserveFileWithin(home, rel)
		if err != nil {
			return ExpectedState{}, err
		}
		return ExpectedState{Attempted: true, Captured: true, Kind: TargetFile, Exists: revision.Exists(), FileRevision: revision, Parents: parents}, nil
	case TargetDirectory:
		snapshot, parents, err := safefile.ObserveDirectoryWithin(home, rel)
		if errors.Is(err, os.ErrNotExist) {
			return ExpectedState{Attempted: true, Captured: true, Kind: TargetDirectory, Parents: parents}, nil
		}
		if err != nil {
			return ExpectedState{}, err
		}
		return ExpectedState{Attempted: true, Captured: true, Kind: TargetDirectory, Exists: true, DirectorySnapshot: snapshot, Parents: parents}, nil
	default:
		return ExpectedState{}, fmt.Errorf("invalid expected-state target kind %q", target.Kind)
	}
}

// Count returns the number of files successfully restored.
func (r RestoreResult) Count() int { return len(r.Restored) }

// Restore restores ordinary files recorded in backupDir into the user's home
// directory. It drives the path mapping from the manifest when present
// (authoritative, lossless) and refuses lossy manifestless decoding. Each file
// is restored with its recorded mode (or 0600 by default), path traversal is
// rejected, and neither source nor destination descendants may be symlinks.
// Directory restores are recursively snapshotted from the selected backup and
// transactionally installed through descriptor-anchored no-follow operations.
// Paths recorded as not previously existing are removed through the matching
// descriptor-anchored file or recursive-directory transaction.
//
// It returns a RestoreResult plus a fatal error only for failures that prevent
// any restore (e.g. unreadable backup dir / unknown home). Per-file failures
// are recorded in Skipped and do not abort the whole restore.
func Restore(backupDir, home string) (RestoreResult, error) {
	result, err := restoreWithOperations(backupDir, home, defaultRestoreOperations())
	outcome := operation.TraceSucceeded
	switch {
	case err != nil:
		outcome = operation.TraceFailed
	case len(result.Skipped) != 0 || len(result.Warnings) != 0:
		outcome = operation.TracePartial
	}
	failed := 0
	if err != nil {
		failed = 1
	}
	operation.Trace(operation.TraceRestore, outcome, operation.TraceCounts{
		Attempted: result.Count() + len(result.Removed) + len(result.Skipped),
		Succeeded: result.Count() + len(result.Removed),
		Failed:    failed, Skipped: len(result.Skipped), Warnings: len(result.Warnings),
	})
	return result, err
}

// RestoreExpected restores only targets whose live state still exactly matches
// a captured post-mutation state. Changed or uncaptured targets are skipped and
// therefore make the rollback incomplete instead of overwriting external work.
func RestoreExpected(backupDir, home string, expected map[string]ExpectedState) (RestoreResult, error) {
	return restoreWithExpectedOperations(backupDir, home, defaultRestoreOperations(), expected)
}

// RestoreExpectedPlan restores from immutable in-memory originals captured by
// the accepted plan. The durable backup root is still validated as recovery
// evidence, but its mutable pathname contents are never used as automatic
// rollback input.
func RestoreExpectedPlan(plan PlanResult, home string, expected map[string]ExpectedState) (RestoreResult, error) {
	if err := ValidatePlanRoot(plan); err != nil {
		return RestoreResult{Skipped: map[string]string{}, Warnings: map[string]string{}}, err
	}
	if hook := restorePlanTestHooks.afterRootValidation; hook != nil {
		if err := hook(plan); err != nil {
			return RestoreResult{Skipped: map[string]string{}, Warnings: map[string]string{}}, err
		}
	}
	exactExpected, err := loadExactPlanOriginals(plan, expected)
	if err != nil {
		return RestoreResult{Skipped: map[string]string{}, Warnings: map[string]string{}}, err
	}
	items := make([]restoreItem, 0, len(plan.targets))
	for _, target := range plan.targets {
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(target.RelPath)))
		state := exactExpected[rel]
		items = append(items, restoreItem{
			relPath: rel,
			existed: state.OriginalCaptured && state.OriginalExists,
			isDir:   target.Kind == TargetDirectory,
			mode:    state.OriginalMode,
			srcRel:  rel,
		})
	}
	result, err := restoreWithExpectedItems("", home, defaultRestoreOperations(), exactExpected, items)
	if validationErr := ValidatePlanRoot(plan); validationErr != nil {
		err = errors.Join(err, validationErr)
	}
	return result, err
}

func loadExactPlanOriginals(plan PlanResult, expected map[string]ExpectedState) (map[string]ExpectedState, error) {
	if plan.sources == nil || plan.targets == nil {
		return nil, fmt.Errorf("%w: plan backup source authority is unavailable", safefile.ErrDirectoryChanged)
	}
	manifest, ok := plan.sources[ManifestName]
	if !ok || !manifest.file.Tracked() || !manifest.file.Exists() || !manifest.parents.Tracked() {
		return nil, fmt.Errorf("%w: plan manifest authority is unavailable", safefile.ErrRevisionChanged)
	}
	manifestRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(plan.rel), ManifestName))
	_, currentManifest, err := safefile.ReadWithinAuthorized(plan.anchor, manifestRel, manifest.parents)
	if err != nil || currentManifest != manifest.file {
		return nil, fmt.Errorf("plan manifest changed before rollback: %w", errors.Join(safefile.ErrRevisionChanged, err))
	}

	result := make(map[string]ExpectedState, len(expected))
	for rel, state := range expected {
		state.OriginalData = append([]byte(nil), state.OriginalData...)
		result[filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))] = state
	}
	for _, target := range plan.targets {
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(target.RelPath)))
		state := result[rel]
		source, exists := plan.sources[rel]
		if !exists {
			if state.OriginalCaptured && state.OriginalExists {
				return nil, fmt.Errorf("accepted original state for %s disagrees with its absent backup source", rel)
			}
			state.OriginalCaptured = true
			state.OriginalExists = false
			state.OriginalData = nil
			state.OriginalMode = 0
			state.OriginalDirectory = nil
			result[rel] = state
			continue
		}
		if source.target.Kind != target.Kind || filepath.ToSlash(filepath.Clean(filepath.FromSlash(source.target.RelPath))) != rel || !source.parents.Tracked() {
			return nil, fmt.Errorf("%w: plan source authority for %s has the wrong target", safefile.ErrDirectoryChanged, rel)
		}
		fullRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(plan.rel), filepath.FromSlash(rel)))
		switch target.Kind {
		case TargetFile:
			data, revision, readErr := safefile.ReadWithinAuthorized(plan.anchor, fullRel, source.parents)
			if readErr != nil || revision != source.file || !revision.Exists() {
				return nil, fmt.Errorf("plan backup file %s changed before rollback: %w", rel, errors.Join(safefile.ErrRevisionChanged, readErr))
			}
			if state.OriginalCaptured && (!state.OriginalExists || state.OriginalMode.Perm() != revision.Permissions() || !bytes.Equal(state.OriginalData, data)) {
				return nil, fmt.Errorf("accepted original file state for %s disagrees with its exact backup source", rel)
			}
			state.OriginalCaptured = true
			state.OriginalExists = true
			state.OriginalData = append([]byte(nil), data...)
			state.OriginalMode = revision.Permissions()
			state.OriginalDirectory = nil
		case TargetDirectory:
			if source.directory == nil {
				return nil, fmt.Errorf("%w: plan backup directory %s has no snapshot", safefile.ErrDirectoryChanged, rel)
			}
			before, bindErr := safefile.BindParentChainWithin(plan.anchor, fullRel, source.parents, nil)
			if bindErr != nil {
				return nil, bindErr
			}
			snapshot, snapshotErr := safefile.SnapshotDirectoryWithin(plan.anchor, fullRel)
			after, afterErr := safefile.BindParentChainWithin(plan.anchor, fullRel, source.parents, nil)
			if snapshotErr != nil || afterErr != nil || !safefile.SameParentChain(before, after) ||
				!safefile.SameDirectoryRootState(snapshot, source.directory) || snapshot.Digest() != source.directory.Digest() {
				return nil, fmt.Errorf("plan backup directory %s changed before rollback: %w", rel, errors.Join(safefile.ErrDirectoryChanged, snapshotErr, afterErr))
			}
			if state.OriginalCaptured && (!state.OriginalExists || state.OriginalDirectory == nil ||
				state.OriginalDirectory.Permissions() != snapshot.Permissions() || state.OriginalDirectory.Digest() != snapshot.Digest()) {
				return nil, fmt.Errorf("accepted original directory state for %s disagrees with its exact backup source", rel)
			}
			state.OriginalCaptured = true
			state.OriginalExists = true
			state.OriginalDirectory = snapshot
			state.OriginalData = nil
			state.OriginalMode = 0
		default:
			return nil, fmt.Errorf("invalid plan source target kind %q", target.Kind)
		}
		result[rel] = state
	}
	return result, nil
}

func defaultRestoreOperations() restoreOperations {
	return restoreOperations{
		replaceFile:              safefile.ReplaceWithin,
		snapshotDirectory:        safefile.SnapshotDirectoryWithin,
		restoreDirectory:         safefile.RestoreDirectoryWithin,
		removeFile:               safefile.RemoveWithin,
		removeDirectory:          safefile.RemoveDirectoryWithin,
		replaceFileRevision:      safefile.ReplaceWithinRevision,
		removeFileRevision:       safefile.RemoveWithinRevision,
		restoreDirectorySnapshot: safefile.RestoreDirectoryWithinSnapshot,
		removeDirectorySnapshot:  safefile.RemoveDirectoryWithinSnapshot,
		removeEmptyDirectory:     safefile.RemoveEmptyDirectoryWithinSnapshot,
		replaceFileAuthorized: func(root, rel string, expected safefile.Revision, parents *safefile.ParentChain, data []byte, mode os.FileMode) error {
			_, err := safefile.ReplaceWithinRevisionNoCreateAuthorizedTracked(root, rel, expected, parents, data, mode)
			return err
		},
		removeFileAuthorized: safefile.RemoveWithinRevisionAuthorized,
		restoreDirectoryAuthorized: func(root, rel string, snapshot, expected *safefile.DirectorySnapshot, parents *safefile.ParentChain) error {
			_, err := safefile.RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(root, rel, snapshot, expected, parents)
			return err
		},
		removeDirectoryAuthorized: safefile.RemoveDirectoryWithinSnapshotAuthorized,
		removeEmptyAuthorized:     safefile.RemoveEmptyDirectoryWithinSnapshotAuthorized,
	}
}

type restoreReplaceFunc func(root, rel string, data []byte, mode os.FileMode) error

type restoreOperations struct {
	replaceFile                restoreReplaceFunc
	snapshotDirectory          func(root, rel string) (*safefile.DirectorySnapshot, error)
	restoreDirectory           func(root, rel string, snapshot *safefile.DirectorySnapshot) error
	removeFile                 func(root, rel string) error
	removeDirectory            func(root, rel string) error
	replaceFileRevision        func(root, rel string, expected safefile.Revision, data []byte, mode os.FileMode) error
	removeFileRevision         func(root, rel string, expected safefile.Revision) error
	restoreDirectorySnapshot   func(root, rel string, snapshot, expected *safefile.DirectorySnapshot) error
	removeDirectorySnapshot    func(root, rel string, expected *safefile.DirectorySnapshot) error
	removeEmptyDirectory       func(root, rel string, expected *safefile.DirectorySnapshot) error
	replaceFileAuthorized      func(root, rel string, expected safefile.Revision, parents *safefile.ParentChain, data []byte, mode os.FileMode) error
	removeFileAuthorized       func(root, rel string, expected safefile.Revision, parents *safefile.ParentChain) error
	restoreDirectoryAuthorized func(root, rel string, snapshot, expected *safefile.DirectorySnapshot, parents *safefile.ParentChain) error
	removeDirectoryAuthorized  func(root, rel string, expected *safefile.DirectorySnapshot, parents *safefile.ParentChain) error
	removeEmptyAuthorized      func(root, rel string, expected *safefile.DirectorySnapshot, parents *safefile.ParentChain) error
}

func restoreWithReplace(backupDir, home string, replace restoreReplaceFunc) (RestoreResult, error) {
	return restoreWithOperations(backupDir, home, restoreOperations{
		replaceFile:       replace,
		snapshotDirectory: safefile.SnapshotDirectoryWithin,
		restoreDirectory:  safefile.RestoreDirectoryWithin,
		removeFile:        safefile.RemoveWithin,
		removeDirectory:   safefile.RemoveDirectoryWithin,
	})
}

func restoreWithOperations(backupDir, home string, operations restoreOperations) (RestoreResult, error) {
	return restoreWithExpectedOperations(backupDir, home, operations, nil)
}

func restoreWithExpectedOperations(backupDir, home string, operations restoreOperations, expected map[string]ExpectedState) (RestoreResult, error) {
	return restoreWithExpectedItems(backupDir, home, operations, expected, nil)
}

func restoreWithExpectedItems(backupDir, home string, operations restoreOperations, expected map[string]ExpectedState, provided []restoreItem) (RestoreResult, error) {
	result := RestoreResult{Skipped: map[string]string{}, Warnings: map[string]string{}}

	items := provided
	if items == nil {
		var err error
		items, err = restoreItems(backupDir, home)
		if err != nil {
			return result, err
		}
	}
	if expected != nil {
		if err := validateConditionalManifest(items, expected); err != nil {
			return result, err
		}
		// Child leaves must be restored/removed before the empty parent
		// directories that made them reachable. Descending depth also handles
		// nested created parents deterministically; lexical order breaks ties.
		sort.SliceStable(items, func(i, j int) bool {
			left := filepath.ToSlash(filepath.Clean(filepath.FromSlash(items[i].relPath)))
			right := filepath.ToSlash(filepath.Clean(filepath.FromSlash(items[j].relPath)))
			leftDepth := strings.Count(left, "/")
			rightDepth := strings.Count(right, "/")
			if leftDepth != rightDepth {
				return leftDepth > rightDepth
			}
			return left < right
		})
	}

	for _, it := range items {
		if it.skipReason != "" {
			result.Skipped[it.key()] = it.skipReason
			continue
		}
		conditional := expected != nil
		relKey := filepath.ToSlash(filepath.Clean(filepath.FromSlash(it.relPath)))
		postState := ExpectedState{}
		if conditional {
			var ok bool
			postState, ok = expected[relKey]
			if !ok || !postState.Attempted {
				// The action never ran, so rollback has no authority or work here.
				continue
			}
			if !postState.Captured {
				result.Skipped[it.key()] = "conditional rollback has no proven post-write state"
				continue
			}
			if !postState.Parents.Tracked() {
				result.Skipped[it.key()] = "conditional rollback has no bound parent-chain authority"
				continue
			}
			wantKind := TargetFile
			if it.isDir {
				wantKind = TargetDirectory
			}
			if postState.Kind != wantKind {
				result.Skipped[it.key()] = "conditional rollback post-write kind mismatch"
				continue
			}
		}

		dstPath := it.dstPath
		if dstPath == "" {
			// Go-format entries store relative destinations.
			var ok bool
			dstPath, ok = safeJoin(home, it.relPath)
			if !ok {
				result.Skipped[it.key()] = "path traversal detected"
				continue
			}
		}

		if !absPathWithinHome(home, dstPath) {
			result.Skipped[it.key()] = "path traversal detected"
			continue
		}

		if !it.existed {
			var err error
			if conditional {
				err = removeExpectedTarget(home, relKey, postState, operations)
			} else if it.isDir {
				err = operations.removeDirectory(home, relKey)
			} else {
				err = operations.removeFile(home, relKey)
			}
			if err == nil || errors.Is(err, os.ErrNotExist) {
				if !conditional || postState.Exists {
					result.Removed = append(result.Removed, it.relPath)
				}
				continue
			}
			var committed *safefile.CommittedError
			if errors.As(err, &committed) {
				result.Removed = append(result.Removed, it.relPath)
				result.Warnings[it.key()] = fmt.Sprintf("removal committed with a durability/cleanup warning: %v", err)
				continue
			}
			result.Skipped[it.key()] = fmt.Sprintf("remove: %v", err)
			continue
		}

		if it.isDir {
			if it.srcRel == "" {
				result.Skipped[it.key()] = "backup source is outside the selected backup directory"
				continue
			}
			var snapshot *safefile.DirectorySnapshot
			if conditional && postState.OriginalCaptured {
				snapshot = postState.OriginalDirectory
				if snapshot == nil {
					result.Skipped[it.key()] = "immutable original directory snapshot is unavailable"
					continue
				}
			} else {
				anchor, sourceRel, err := backupDescendantAnchor(backupDir, it.srcRel)
				if err != nil {
					result.Skipped[it.key()] = fmt.Sprintf("resolve backup directory: %v", err)
					continue
				}
				snapshot, err = operations.snapshotDirectory(anchor, sourceRel)
				if err != nil {
					result.Skipped[it.key()] = fmt.Sprintf("snapshot backup directory: %v", err)
					continue
				}
			}
			var restoreErr error
			if conditional {
				restoreErr = operations.restoreDirectoryAuthorized(home, relKey, snapshot, postState.DirectorySnapshot, postState.Parents)
			} else {
				restoreErr = operations.restoreDirectory(home, relKey, snapshot)
			}
			if restoreErr != nil {
				var committed *safefile.CommittedError
				if errors.As(restoreErr, &committed) {
					result.Restored = append(result.Restored, it.relPath)
					result.Warnings[it.key()] = fmt.Sprintf("directory restore committed with a durability/cleanup warning: %v", restoreErr)
					continue
				}
				result.Skipped[it.key()] = fmt.Sprintf("restore directory: %v", restoreErr)
				continue
			}
			result.Restored = append(result.Restored, it.relPath)
			continue
		}

		if it.srcRel == "" {
			result.Skipped[it.key()] = "backup source is outside the selected backup directory"
			continue
		}
		var data []byte
		mode := it.mode.Perm()
		if conditional && postState.OriginalCaptured {
			data = append([]byte(nil), postState.OriginalData...)
			mode = postState.OriginalMode.Perm()
		} else {
			var revision safefile.Revision
			var err error
			data, revision, err = readBackupDescendant(backupDir, it.srcRel)
			if err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("read backup file: %v", err)
				continue
			}
			if !revision.Exists() {
				result.Skipped[it.key()] = "read backup file: source does not exist"
				continue
			}
		}

		// Restore ordinary files through the same descriptor-anchored atomic
		// replacement kernel used by generated configs. Every descendant below
		// the trusted HOME anchor is traversed with O_NOFOLLOW, missing parents
		// are created owner-only, the recorded mode is set before commit, and a
		// failed precommit write leaves the old destination intact.
		var replaceErr error
		if conditional {
			replaceErr = operations.replaceFileAuthorized(home, relKey, postState.FileRevision, postState.Parents, data, mode)
		} else {
			replaceErr = operations.replaceFile(home, relKey, data, it.mode.Perm())
		}
		if replaceErr != nil {
			var committed *safefile.CommittedError
			if errors.As(replaceErr, &committed) {
				result.Restored = append(result.Restored, it.relPath)
				result.Warnings[it.key()] = fmt.Sprintf("restore committed with a durability/verification warning: %v", replaceErr)
				continue
			}
			result.Skipped[it.key()] = fmt.Sprintf("write: %v", replaceErr)
			continue
		}

		result.Restored = append(result.Restored, it.relPath)
	}

	return result, nil
}

func validateConditionalManifest(items []restoreItem, expected map[string]ExpectedState) error {
	type manifestState struct {
		kind    TargetKind
		existed bool
	}
	manifest := make(map[string]manifestState, len(items))
	for _, item := range items {
		if item.skipReason != "" {
			continue
		}
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(item.relPath)))
		if rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("conditional rollback manifest has invalid target %q", item.relPath)
		}
		if _, duplicate := manifest[rel]; duplicate {
			return fmt.Errorf("conditional rollback manifest repeats normalized target %s", rel)
		}
		kind := TargetFile
		if item.isDir {
			kind = TargetDirectory
		}
		manifest[rel] = manifestState{kind: kind, existed: item.existed}
	}
	for rawRel, state := range expected {
		if !state.Attempted {
			continue
		}
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rawRel)))
		entry, ok := manifest[rel]
		if !ok {
			return fmt.Errorf("conditional rollback manifest omits attempted target %s", rel)
		}
		if state.Captured && entry.kind != state.Kind {
			return fmt.Errorf("conditional rollback manifest kind for %s is %s, want %s", rel, entry.kind, state.Kind)
		}
		if state.OriginalCaptured && entry.existed != state.OriginalExists {
			return fmt.Errorf("conditional rollback manifest existence for %s does not match immutable original", rel)
		}
	}
	return nil
}

func removeExpectedTarget(home, rel string, expected ExpectedState, operations restoreOperations) error {
	if !expected.Exists {
		switch expected.Kind {
		case TargetFile:
			_, current, err := safefile.ReadWithinAuthorized(home, rel, expected.Parents)
			if err != nil {
				return err
			}
			if current != expected.FileRevision {
				return safefile.ErrRevisionChanged
			}
			return nil
		case TargetDirectory:
			_, err := safefile.BindParentChainWithin(home, rel, expected.Parents, nil)
			if err != nil {
				return err
			}
			_, err = safefile.SnapshotDirectoryWithin(home, rel)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			return safefile.ErrDirectoryChanged
		}
	}
	if expected.Kind == TargetDirectory {
		if expected.EmptyOnly {
			return operations.removeEmptyAuthorized(home, rel, expected.DirectorySnapshot, expected.Parents)
		}
		return operations.removeDirectoryAuthorized(home, rel, expected.DirectorySnapshot, expected.Parents)
	}
	return operations.removeFileAuthorized(home, rel, expected.FileRevision, expected.Parents)
}

type restoreItem struct {
	srcPath    string
	srcRel     string
	dstPath    string
	relPath    string
	mode       os.FileMode
	existed    bool
	isDir      bool
	skipReason string
}

func (it restoreItem) key() string {
	if it.relPath != "" {
		return it.relPath
	}
	if it.dstPath != "" {
		return it.dstPath
	}
	return it.srcPath
}

func restoreItems(backupDir, home string) ([]restoreItem, error) {
	data, revision, err := readBackupDescendant(backupDir, ManifestName)
	if err != nil {
		return nil, err
	}
	if !revision.Exists() {
		return nil, fmt.Errorf("backup has no %s; refusing lossy underscore path decode", ManifestName)
	}

	var items []restoreItem
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.Contains(line, "|") {
			item, ok, err := parseBashManifestLine(line, backupDir, home)
			if err != nil {
				items = append(items, restoreItem{
					relPath:    line,
					skipReason: fmt.Sprintf("invalid manifest line: %v", err),
				})
				continue
			}
			if ok {
				items = append(items, item)
			}
			continue
		}

		relPath := line
		mode := defaultFileMode
		if tab := strings.LastIndexByte(line, '\t'); tab >= 0 {
			relPath = line[:tab]
			parsed, perr := parseOctalMode(line[tab+1:])
			if perr != nil {
				items = append(items, restoreItem{
					relPath:    relPath,
					skipReason: fmt.Sprintf("invalid explicit mode: %v", perr),
				})
				continue
			}
			mode = parsed
		}
		items = append(items, restoreItem{
			srcPath: filepath.Join(backupDir, EncodeName(relPath)),
			srcRel:  EncodeName(relPath),
			relPath: relPath,
			mode:    mode,
			existed: true,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func readBashManifestEntry(line, home string) (Entry, bool, error) {
	original, _, existed, _, mode, err := parseBashManifestFields(line)
	if err != nil {
		return Entry{}, false, err
	}
	if existed == "no" {
		return Entry{}, false, nil
	}
	relPath, ok := relPathFromAbsHome(home, original)
	if !ok {
		return Entry{}, false, fmt.Errorf("bash backup manifest path %q is outside home", original)
	}
	return Entry{RelPath: relPath, Mode: mode}, true, nil
}

func parseBashManifestLine(line, backupDir, home string) (restoreItem, bool, error) {
	original, backupPath, existed, itemType, mode, err := parseBashManifestFields(line)
	if err != nil {
		return restoreItem{}, false, err
	}

	relPath, ok := relPathFromAbsHome(home, original)
	if !ok {
		return restoreItem{
			dstPath: original,
			relPath: original,
			existed: existed == "yes",
			isDir:   itemType == "directory",
		}, true, nil
	}

	item := restoreItem{
		dstPath: filepath.Clean(original),
		relPath: relPath,
		mode:    mode,
		existed: existed == "yes",
		isDir:   itemType == "directory",
	}
	if item.existed {
		if backupPath == "" {
			return restoreItem{}, false, fmt.Errorf("invalid bash backup manifest line for %q: missing backup path", original)
		}
		item.srcPath = filepath.Clean(filepath.Join(backupDir, relPath))
		item.srcRel = relPath
	}
	return item, true, nil
}

func parseBashManifestFields(line string) (original, backupPath, existed, itemType string, mode os.FileMode, err error) {
	parts := strings.Split(line, "|")
	mode = defaultFileMode
	switch len(parts) {
	case 3:
		itemType = "file"
	case 4:
		itemType = parts[3]
	case 5:
		itemType = parts[3]
		mode, err = parseOctalMode(parts[4])
		if err != nil {
			return "", "", "", "", 0, fmt.Errorf("invalid bash backup manifest mode: %w", err)
		}
	default:
		return "", "", "", "", 0, fmt.Errorf("invalid bash backup manifest line %q: expected original|backup|existed[|type[|mode]]", line)
	}
	if len(parts) >= 4 {
		if itemType != "file" && itemType != "directory" {
			return "", "", "", "", 0, fmt.Errorf("invalid bash backup manifest line %q: type must be file or directory", line)
		}
	}
	original, backupPath, existed = parts[0], parts[1], parts[2]
	if existed != "yes" && existed != "no" {
		return "", "", "", "", 0, fmt.Errorf("invalid bash backup manifest line %q: existed field must be yes or no", line)
	}
	return original, backupPath, existed, itemType, mode, nil
}

func relPathFromAbsHome(home, path string) (string, bool) {
	if path == "" || !filepath.IsAbs(path) || hasParentTraversal(path) {
		return "", false
	}
	cleanPath := filepath.Clean(path)
	for _, candidate := range homePathCandidates(home) {
		if cleanPath == candidate {
			continue
		}
		if !strings.HasPrefix(cleanPath, candidate+string(os.PathSeparator)) {
			continue
		}
		rel, err := filepath.Rel(candidate, cleanPath)
		if err != nil || rel == "." || rel == "" || hasParentTraversal(rel) {
			continue
		}
		return rel, true
	}
	return "", false
}

func absPathWithinHome(home, path string) bool {
	_, ok := relPathFromAbsHome(home, path)
	return ok
}

func hasParentTraversal(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(os.PathSeparator)) {
		if part == ".." {
			return true
		}
	}
	return false
}

func homePathCandidates(home string) []string {
	cleanHome := filepath.Clean(home)
	candidates := []string{cleanHome}
	if realHome, err := filepath.EvalSymlinks(cleanHome); err == nil {
		realHome = filepath.Clean(realHome)
		if realHome != cleanHome {
			candidates = append(candidates, realHome)
		}
	}
	return candidates
}
