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
	"strconv"
	"strings"

	"github.com/tekierz/dotfiles/internal/safefile"
)

// ManifestName is the file inside each backup directory that records the
// original relative path (and mode) of every backed-up file.
const ManifestName = "manifest.txt"

// defaultFileMode is used when a backup does not record the original file
// mode (e.g. legacy backups created before modes were stored). 0600 matches
// the project's documented permission policy for config/dotfiles.
const defaultFileMode os.FileMode = 0o600

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

	var entries []Entry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	home, _ := os.UserHomeDir()
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
	backupName := filepath.Base(cleanBackupDir)
	backupRoot := filepath.Dir(cleanBackupDir)
	backupRootName := filepath.Base(backupRoot)
	anchor := filepath.Dir(backupRoot)
	if backupName == "." || backupName == string(os.PathSeparator) ||
		backupRootName == "." || backupRootName == string(os.PathSeparator) ||
		backupRoot == anchor {
		return "", "", fmt.Errorf("invalid backup directory %q", backupDir)
	}
	anchoredRel := filepath.ToSlash(filepath.Join(backupRootName, backupName, rel))
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
	return restoreWithOperations(backupDir, home, defaultRestoreOperations())
}

func defaultRestoreOperations() restoreOperations {
	return restoreOperations{
		replaceFile:       safefile.ReplaceWithin,
		snapshotDirectory: safefile.SnapshotDirectoryWithin,
		restoreDirectory:  safefile.RestoreDirectoryWithin,
		removeFile:        safefile.RemoveWithin,
		removeDirectory:   safefile.RemoveDirectoryWithin,
	}
}

type restoreReplaceFunc func(root, rel string, data []byte, mode os.FileMode) error

type restoreOperations struct {
	replaceFile       restoreReplaceFunc
	snapshotDirectory func(root, rel string) (*safefile.DirectorySnapshot, error)
	restoreDirectory  func(root, rel string, snapshot *safefile.DirectorySnapshot) error
	removeFile        func(root, rel string) error
	removeDirectory   func(root, rel string) error
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
	result := RestoreResult{Skipped: map[string]string{}, Warnings: map[string]string{}}

	items, err := restoreItems(backupDir, home)
	if err != nil {
		return result, err
	}

	for _, it := range items {
		if it.skipReason != "" {
			result.Skipped[it.key()] = it.skipReason
			continue
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
			remove := operations.removeFile
			if it.isDir {
				remove = operations.removeDirectory
			}
			err := remove(home, filepath.ToSlash(it.relPath))
			if err == nil || errors.Is(err, os.ErrNotExist) {
				result.Removed = append(result.Removed, it.relPath)
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
			anchor, sourceRel, err := backupDescendantAnchor(backupDir, it.srcRel)
			if err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("resolve backup directory: %v", err)
				continue
			}
			snapshot, err := operations.snapshotDirectory(anchor, sourceRel)
			if err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("snapshot backup directory: %v", err)
				continue
			}
			if err := operations.restoreDirectory(home, filepath.ToSlash(it.relPath), snapshot); err != nil {
				var committed *safefile.CommittedError
				if errors.As(err, &committed) {
					result.Restored = append(result.Restored, it.relPath)
					result.Warnings[it.key()] = fmt.Sprintf("directory restore committed with a durability/cleanup warning: %v", err)
					continue
				}
				result.Skipped[it.key()] = fmt.Sprintf("restore directory: %v", err)
				continue
			}
			result.Restored = append(result.Restored, it.relPath)
			continue
		}

		if it.srcRel == "" {
			result.Skipped[it.key()] = "backup source is outside the selected backup directory"
			continue
		}
		data, revision, err := readBackupDescendant(backupDir, it.srcRel)
		if err != nil {
			result.Skipped[it.key()] = fmt.Sprintf("read backup file: %v", err)
			continue
		}
		if !revision.Exists() {
			result.Skipped[it.key()] = "read backup file: source does not exist"
			continue
		}

		// Restore ordinary files through the same descriptor-anchored atomic
		// replacement kernel used by generated configs. Every descendant below
		// the trusted HOME anchor is traversed with O_NOFOLLOW, missing parents
		// are created owner-only, the recorded mode is set before commit, and a
		// failed precommit write leaves the old destination intact.
		if err := operations.replaceFile(home, filepath.ToSlash(it.relPath), data, it.mode.Perm()); err != nil {
			var committed *safefile.CommittedError
			if errors.As(err, &committed) {
				result.Restored = append(result.Restored, it.relPath)
				result.Warnings[it.key()] = fmt.Sprintf("restore committed with a durability/verification warning: %v", err)
				continue
			}
			result.Skipped[it.key()] = fmt.Sprintf("write: %v", err)
			continue
		}

		result.Restored = append(result.Restored, it.relPath)
	}

	return result, nil
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
