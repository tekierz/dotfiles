// Package backup provides shared logic for creating and restoring dotfile
// backups. Both the CLI (cmd/dotfiles) and the TUI (internal/ui) use these
// helpers so the encode/decode and restore paths cannot diverge.
package backup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// field (legacy manifests) are parsed with defaultFileMode. A missing manifest
// returns (nil, nil) so callers can fall back to filename decoding.
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
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
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

// noFollowWrite writes data to dstPath without following an existing symlink
// at dstPath. If dstPath is currently a symlink the write is refused, so a
// pre-existing symlink cannot redirect the write to a target outside home.
func noFollowWrite(dstPath string, data []byte, mode os.FileMode) error {
	if fi, err := os.Lstat(dstPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write through symlink: %s", dstPath)
	}
	return os.WriteFile(dstPath, data, mode)
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

	manifest, err := ReadManifest(backupDir)
	if err != nil {
		return result, err
	}

	// Build the list of (sourceFile, relPath, mode) to restore. Prefer the
	// manifest; fall back to scanning the directory for legacy backups.
	type item struct {
		srcName string
		relPath string
		mode    os.FileMode
	}
	var items []item

	if len(manifest) > 0 {
		for _, e := range manifest {
			items = append(items, item{
				srcName: EncodeName(e.RelPath),
				relPath: e.RelPath,
				mode:    e.Mode,
			})
		}
	} else {
		entries, derr := os.ReadDir(backupDir)
		if derr != nil {
			return result, derr
		}
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == ManifestName {
				continue
			}
			// Legacy decode: underscores back to separators. Lossy, but the
			// only option for manifest-less backups.
			items = append(items, item{
				srcName: entry.Name(),
				relPath: strings.ReplaceAll(entry.Name(), "_", string(os.PathSeparator)),
				mode:    defaultFileMode,
			})
		}
	}

	for _, it := range items {
		srcPath := filepath.Join(backupDir, it.srcName)

		// IsRestorePathSafe is the exported, production guard against traversal
		// (absolute paths and ".." components). safeJoin returns the cleaned
		// destination once the path is known safe.
		if !IsRestorePathSafe(home, it.relPath) {
			result.Skipped[it.relPath] = "path traversal detected"
			continue
		}
		dstPath, ok := safeJoin(home, it.relPath)
		if !ok {
			result.Skipped[it.relPath] = "path traversal detected"
			continue
		}

		// Lexical safety (safeJoin) does not cover symlinked INTERMEDIATE
		// directories. Resolve the deepest existing ancestor and confirm it is
		// still inside home before creating directories or writing, so a
		// symlinked parent cannot redirect the write outside home.
		withinHome, perr := resolvedParentWithinHome(dstPath, home)
		if perr != nil {
			result.Skipped[it.relPath] = fmt.Sprintf("resolve parent: %v", perr)
			continue
		}
		if !withinHome {
			result.Skipped[it.relPath] = "refusing to write through symlinked parent outside home"
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
			result.Skipped[it.relPath] = fmt.Sprintf("create directory: %v", err)
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			result.Skipped[it.relPath] = fmt.Sprintf("read backup file: %v", err)
			continue
		}

		if err := noFollowWrite(dstPath, data, it.mode); err != nil {
			result.Skipped[it.relPath] = fmt.Sprintf("write: %v", err)
			continue
		}

		result.Restored = append(result.Restored, it.relPath)
	}

	return result, nil
}
