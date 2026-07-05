// Package backup provides shared logic for creating and restoring dotfile
// backups. Both the CLI (cmd/dotfiles) and the TUI (internal/ui) use these
// helpers so the encode/decode and restore paths cannot diverge.
package backup

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	f, err := os.Open(filepath.Join(backupDir, ManifestName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)
	home, _ := os.UserHomeDir()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
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
			if parsed, perr := parseOctalMode(line[tab+1:]); perr == nil {
				mode = parsed
			}
		}
		entries = append(entries, Entry{RelPath: relPath, Mode: mode})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// parseOctalMode parses an octal permission string (e.g. "600") into a mode.
func parseOctalMode(s string) (os.FileMode, error) {
	var mode os.FileMode
	if _, err := fmt.Sscanf(s, "%o", &mode); err != nil {
		return 0, err
	}
	return mode.Perm(), nil
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

// noFollowWrite writes data to dstPath without following a symlink at the final
// path component. The open is made atomic with respect to symlinks via
// syscall.O_NOFOLLOW: the kernel refuses (ELOOP) to follow a final-component
// symlink at open time. This closes the TOCTOU window that an Lstat-then-write
// approach left open, where a symlink swapped in between the check and the write
// could redirect the write to a target outside home. It mirrors os.WriteFile's
// O_WRONLY|O_CREATE|O_TRUNC semantics for the normal (non-symlink) path.
func noFollowWrite(dstPath string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return fmt.Errorf("refusing to write through symlink: %s", dstPath)
		}
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// resolvedParentWithinHome verifies that the real (symlink-resolved) parent
// directory of dstPath still lives inside home. safeJoin's lexical check and
// noFollowWrite's final-component check do not catch a symlinked INTERMEDIATE
// directory (e.g. ~/.config/evil -> /outside): the leaf does not exist yet, so
// only resolving the deepest existing ancestor reveals the escape.
//
// EvalSymlinks is applied to the deepest EXISTING ancestor of dstPath (the leaf
// and freshly-created directories normally do not exist yet at restore time, so
// resolving the literal parent would fail and wrongly reject legitimate
// restores into new directories under the real home). The resolved ancestor
// must equal home or sit beneath it.
func resolvedParentWithinHome(dstPath, home string) (bool, error) {
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return false, err
	}

	// Walk up from the parent until we hit a directory that exists, then resolve
	// its symlinks. Everything below it does not exist yet, so it cannot itself
	// be a symlink redirecting the write.
	ancestor := filepath.Dir(dstPath)
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			clean := filepath.Clean(resolved)
			return clean == realHome || strings.HasPrefix(clean, realHome+string(os.PathSeparator)), nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			// Reached the filesystem root without finding an existing ancestor.
			return false, nil
		}
		ancestor = parent
	}
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
}

// Count returns the number of files successfully restored.
func (r RestoreResult) Count() int { return len(r.Restored) }

// Restore restores every file recorded in backupDir into the user's home
// directory. It drives the path mapping from the manifest when present
// (authoritative, lossless), falling back to filename decoding for legacy
// backups that have no manifest. Each file is restored with its recorded mode
// (or 0600 by default), parent directories are created, path traversal is
// rejected, and writes never follow a pre-existing symlink.
//
// It returns a RestoreResult plus a fatal error only for failures that prevent
// any restore (e.g. unreadable backup dir / unknown home). Per-file failures
// are recorded in Skipped and do not abort the whole restore.
func Restore(backupDir, home string) (RestoreResult, error) {
	result := RestoreResult{Skipped: map[string]string{}}

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

		// Lexical safety (safeJoin) does not cover symlinked INTERMEDIATE
		// directories. Resolve the deepest existing ancestor and confirm it is
		// still inside home before creating directories or writing, so a
		// symlinked parent cannot redirect the write outside home.
		withinHome, perr := resolvedParentWithinHome(dstPath, home)
		if perr != nil {
			result.Skipped[it.key()] = fmt.Sprintf("resolve parent: %v", perr)
			continue
		}
		if !withinHome {
			result.Skipped[it.key()] = "refusing to write through symlinked parent outside home"
			continue
		}

		if !it.existed {
			removed, err := removeIfPresent(dstPath, it.isDir)
			if err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("remove created path: %v", err)
				continue
			}
			if removed {
				result.Removed = append(result.Removed, it.relPath)
			}
			continue
		}

		if it.srcPath == "" || !pathWithinBase(backupDir, it.srcPath) {
			result.Skipped[it.key()] = "backup source is outside the selected backup directory"
			continue
		}

		if it.isDir {
			if err := verifyReadableDir(it.srcPath); err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("read backup directory: %v", err)
				continue
			}
			if err := os.RemoveAll(dstPath); err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("replace directory: %v", err)
				continue
			}
			if err := copyTree(it.srcPath, dstPath); err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("restore directory: %v", err)
				continue
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("create directory: %v", err)
				continue
			}

			data, err := os.ReadFile(it.srcPath)
			if err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("read backup file: %v", err)
				continue
			}

			if err := noFollowWrite(dstPath, data, it.mode); err != nil {
				result.Skipped[it.key()] = fmt.Sprintf("write: %v", err)
				continue
			}
		}

		result.Restored = append(result.Restored, it.relPath)
	}

	return result, nil
}

type restoreItem struct {
	srcPath    string
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
	manifestPath := filepath.Join(backupDir, ManifestName)
	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return manifestlessItems(backupDir)
		}
		return nil, err
	}
	defer f.Close()

	var items []restoreItem
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
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
			if parsed, perr := parseOctalMode(line[tab+1:]); perr == nil {
				mode = parsed
			}
		}
		items = append(items, restoreItem{
			srcPath: filepath.Join(backupDir, EncodeName(relPath)),
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

func manifestlessItems(backupDir string) ([]restoreItem, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Name() == ManifestName {
			continue
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("backup contains directory %q but no %s manifest; refusing lossy restore", entry.Name(), ManifestName)
		}
		return nil, fmt.Errorf("backup contains file %q but no %s manifest; refusing lossy underscore path decode", entry.Name(), ManifestName)
	}
	return nil, nil
}

func readBashManifestEntry(line, home string) (Entry, bool, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return Entry{}, false, fmt.Errorf("invalid bash backup manifest line %q: expected original|backup|existed", line)
	}
	original := parts[0]
	existed := parts[2]
	if existed != "yes" && existed != "no" {
		return Entry{}, false, fmt.Errorf("invalid bash backup manifest line %q: existed field must be yes or no", line)
	}
	if existed == "no" {
		return Entry{}, false, nil
	}
	relPath, ok := relPathFromAbsHome(home, original)
	if !ok {
		return Entry{}, false, fmt.Errorf("bash backup manifest path %q is outside home", original)
	}
	return Entry{RelPath: relPath, Mode: defaultFileMode}, true, nil
}

func parseBashManifestLine(line, backupDir, home string) (restoreItem, bool, error) {
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return restoreItem{}, false, fmt.Errorf("invalid bash backup manifest line %q: expected original|backup|existed", line)
	}

	original := parts[0]
	backupPath := parts[1]
	existed := parts[2]
	itemType := ""
	if len(parts) >= 4 {
		itemType = parts[3]
	}

	if existed != "yes" && existed != "no" {
		return restoreItem{}, false, fmt.Errorf("invalid bash backup manifest line %q: existed field must be yes or no", line)
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
		mode:    defaultFileMode,
		existed: existed == "yes",
		isDir:   itemType == "directory",
	}
	if item.existed {
		if backupPath == "" {
			return restoreItem{}, false, fmt.Errorf("invalid bash backup manifest line for %q: missing backup path", original)
		}
		item.srcPath = filepath.Clean(filepath.Join(backupDir, relPath))
	}
	return item, true, nil
}

func verifyReadableDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}

	return filepath.WalkDir(path, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		switch {
		case d.Type()&os.ModeSymlink != 0:
			_, err := os.Readlink(path)
			return err
		case d.Type().IsRegular():
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			return f.Close()
		default:
			return nil
		}
	})
}

func removeIfPresent(path string, isDir bool) (bool, error) {
	if isDir {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return false, nil
		}
		return true, os.RemoveAll(path)
	}
	info, statErr := os.Lstat(path)
	if os.IsNotExist(statErr) {
		return false, nil
	}
	if statErr != nil {
		return false, statErr
	}
	if info.IsDir() {
		return false, nil
	}
	err := os.Remove(path)
	return err == nil, err
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}

		switch {
		case d.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.IsDir():
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		case d.Type().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			return noFollowWrite(target, data, info.Mode().Perm())
		default:
			return nil
		}
	})
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

func pathWithinBase(base, path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	cleanBase := filepath.Clean(base)
	cleanPath := filepath.Clean(path)
	return cleanPath == cleanBase || strings.HasPrefix(cleanPath, cleanBase+string(os.PathSeparator))
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
