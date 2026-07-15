package backup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tekierz/dotfiles/internal/operation"
	"github.com/tekierz/dotfiles/internal/safefile"
)

// CatalogEntry is one manifest-backed backup directory with opaque exact
// authority retained for later validation or deletion.
type CatalogEntry struct {
	Name      string
	Path      string
	Timestamp time.Time
	FileCount int
	Size      int64
	authority *catalogAuthority
}

type catalogAuthority struct {
	anchor   string
	rel      string
	snapshot *safefile.DirectorySnapshot
	parents  *safefile.ParentChain
	restore  *catalogRestoreSnapshot
}

// catalogRestoreSnapshot is the parse-only catalog authority consumed by the
// later restore join. Its source and items are private so mutable CatalogEntry
// display fields cannot redirect a retained source or target.
type catalogRestoreSnapshot struct {
	source *safefile.DirectorySnapshot
	items  []catalogRestoreItem
}

type catalogRestoreItem struct {
	source      string
	target      string
	kind        TargetKind
	existed     bool
	desiredMode os.FileMode
}

// ListCatalog returns only exact real directories containing a readable valid
// manifest. Interrupted partial backup roots and symlinked/replaced entries are
// omitted rather than presented as restorable sessions.
func ListCatalog(backupsDir string) ([]CatalogEntry, error) {
	if info, err := os.Lstat(filepath.Clean(backupsDir)); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("backup catalog is not a real directory: %s", backupsDir)
	}
	anchor, rootRel, err := catalogAnchor(backupsDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(backupsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	result := make([]CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !validCatalogName(name) {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(filepath.FromSlash(rootRel), name))
		snapshot, parents, observeErr := safefile.ObserveDirectoryWithin(anchor, rel)
		if observeErr != nil || snapshot == nil {
			continue
		}
		manifest, _, readErr := safefile.ReadDirectorySnapshotFile(snapshot, ManifestName)
		if readErr != nil {
			continue
		}
		restore, parseErr := parseCatalogRestoreSnapshot(manifest, home, snapshot)
		if parseErr != nil {
			continue
		}
		if bind, bindErr := safefile.BindParentChainWithin(anchor, rel, parents, nil); bindErr != nil || !safefile.SameParentChain(bind, parents) {
			continue
		}
		current, snapshotErr := safefile.SnapshotDirectoryWithin(anchor, rel)
		if snapshotErr != nil || !safefile.SameDirectoryRootState(current, snapshot) || current.Digest() != snapshot.Digest() {
			continue
		}
		_, size := snapshot.RecursiveFileStats(ManifestName)
		timestamp := time.Time{}
		if info, infoErr := entry.Info(); infoErr == nil {
			timestamp = info.ModTime()
		}
		result = append(result, CatalogEntry{
			Name: name, Path: filepath.Join(backupsDir, name), Timestamp: timestamp,
			FileCount: len(restore.items), Size: size,
			authority: &catalogAuthority{anchor: anchor, rel: rel, snapshot: snapshot, parents: parents, restore: restore},
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Timestamp.After(result[j].Timestamp) })
	return result, nil
}

func parseCatalogRestoreSnapshot(data []byte, home string, source *safefile.DirectorySnapshot) (*catalogRestoreSnapshot, error) {
	if source == nil || source.Digest() == ([32]byte{}) {
		return nil, fmt.Errorf("catalog restore source authority is incomplete")
	}
	items := make([]catalogRestoreItem, 0)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		item, err := parseCatalogRestoreItem(line, home)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[item.target]; duplicate {
			return nil, fmt.Errorf("catalog restore manifest repeats target %q", item.target)
		}
		seen[item.target] = struct{}{}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &catalogRestoreSnapshot{source: source, items: items}, nil
}

func parseCatalogRestoreItem(line, home string) (catalogRestoreItem, error) {
	if strings.Contains(line, "|") {
		original, backupPath, existed, itemType, mode, err := parseBashManifestFields(line)
		if err != nil {
			return catalogRestoreItem{}, err
		}
		if err := validateRawCatalogRestoreTarget(original, true); err != nil {
			return catalogRestoreItem{}, err
		}
		target, ok := relPathFromAbsHome(home, original)
		if !ok {
			return catalogRestoreItem{}, fmt.Errorf("catalog restore target %q is outside home", original)
		}
		target, err = normalizeCatalogRestoreTarget(home, target)
		if err != nil {
			return catalogRestoreItem{}, err
		}
		kind := TargetFile
		if itemType == "directory" {
			kind = TargetDirectory
		}
		item := catalogRestoreItem{target: target, kind: kind, existed: existed == "yes", desiredMode: mode}
		if item.existed {
			if backupPath == "" {
				return catalogRestoreItem{}, fmt.Errorf("catalog restore source is missing for %q", target)
			}
			item.source = target
		} else if backupPath != "" {
			return catalogRestoreItem{}, fmt.Errorf("absent catalog restore target %q has a source", target)
		}
		return item, nil
	}

	target := line
	mode := defaultFileMode
	if tab := strings.LastIndexByte(line, '\t'); tab >= 0 {
		target = line[:tab]
		parsed, err := parseOctalMode(line[tab+1:])
		if err != nil {
			return catalogRestoreItem{}, fmt.Errorf("invalid explicit mode for %q: %w", target, err)
		}
		mode = parsed
	}
	normalized, err := normalizeCatalogRestoreTarget(home, target)
	if err != nil {
		return catalogRestoreItem{}, err
	}
	return catalogRestoreItem{
		source: EncodeName(normalized), target: normalized, kind: TargetFile,
		existed: true, desiredMode: mode,
	}, nil
}

func normalizeCatalogRestoreTarget(home, target string) (string, error) {
	if err := validateRawCatalogRestoreTarget(target, false); err != nil {
		return "", err
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(target)))
	if !IsRestorePathSafe(home, filepath.FromSlash(normalized)) {
		return "", fmt.Errorf("invalid catalog restore target %q", target)
	}
	return normalized, nil
}

func validateRawCatalogRestoreTarget(target string, absolute bool) error {
	if target == "" || filepath.IsAbs(target) != absolute || strings.ContainsAny(target, "|\r\n\t\x00") {
		return fmt.Errorf("invalid catalog restore target %q", target)
	}
	slashTarget := filepath.ToSlash(target)
	components := strings.Split(slashTarget, "/")
	for index, component := range components {
		if component == "." || component == ".." || component == "" && !(absolute && index == 0) {
			return fmt.Errorf("catalog restore target %q is not canonical", target)
		}
	}
	if filepath.ToSlash(filepath.Clean(filepath.FromSlash(slashTarget))) != slashTarget {
		return fmt.Errorf("catalog restore target %q is not canonical", target)
	}
	return nil
}

func ValidateCatalogEntry(entry CatalogEntry) error {
	if entry.authority == nil || entry.authority.snapshot == nil || !entry.authority.parents.Tracked() ||
		entry.authority.restore == nil || entry.authority.restore.source != entry.authority.snapshot || entry.authority.restore.items == nil {
		return fmt.Errorf("%w: backup catalog authority is incomplete", safefile.ErrDirectoryChanged)
	}
	current, err := safefile.SnapshotDirectoryWithin(entry.authority.anchor, entry.authority.rel)
	if err != nil || !safefile.SameDirectoryRootState(current, entry.authority.snapshot) || current.Digest() != entry.authority.snapshot.Digest() {
		return fmt.Errorf("selected backup changed after listing: %w", errors.Join(safefile.ErrDirectoryChanged, err))
	}
	bound, err := safefile.BindParentChainWithin(entry.authority.anchor, entry.authority.rel, entry.authority.parents, nil)
	if err != nil || !safefile.SameParentChain(bound, entry.authority.parents) {
		return fmt.Errorf("selected backup parent changed after listing: %w", errors.Join(safefile.ErrParentChanged, err))
	}
	return nil
}

// RestoreCatalogEntry restores only the immutable source, target, kind, and
// mode accepted by ListCatalog. Public CatalogEntry fields are presentation
// metadata and are deliberately not consulted for restore authority.
//
// A catalog or restore-root authority failure is fatal. Individual target
// failures remain partial-restore results so callers can report every item
// that could not be applied without discarding successful work.
func RestoreCatalogEntry(entry CatalogEntry, home string) (RestoreResult, error) {
	result := RestoreResult{Skipped: map[string]string{}, Warnings: map[string]string{}}
	operation.Trace(operation.TraceRestore, operation.TraceRunning, operation.TraceCounts{})
	if err := ValidateCatalogEntry(entry); err != nil {
		traceCatalogRestoreResult(result, err)
		return result, err
	}
	session, err := safefile.NewRestoreSession(home)
	if err != nil {
		traceCatalogRestoreResult(result, err)
		return result, err
	}

	restore := entry.authority.restore
	for _, item := range restore.items {
		restoreCatalogItem(&result, session, home, restore.source, item)
	}
	traceCatalogRestoreResult(result, nil)
	return result, nil
}

func traceCatalogRestoreResult(result RestoreResult, fatal error) {
	outcome := operation.TraceSucceeded
	if fatal != nil {
		outcome = operation.TraceFailed
	} else if len(result.Skipped) != 0 || len(result.Warnings) != 0 {
		outcome = operation.TracePartial
	}
	failed := 0
	if fatal != nil {
		failed = 1
	}
	operation.Trace(operation.TraceRestore, outcome, operation.TraceCounts{
		Attempted: result.Count() + len(result.Removed) + len(result.Skipped),
		Succeeded: result.Count() + len(result.Removed),
		Failed:    failed,
		Skipped:   len(result.Skipped),
		Warnings:  len(result.Warnings),
	})
}

func restoreCatalogItem(result *RestoreResult, session *safefile.RestoreSession, home string, source *safefile.DirectorySnapshot, item catalogRestoreItem) {
	if !item.existed {
		removeCatalogTarget(result, session, home, item)
		return
	}

	switch item.kind {
	case TargetFile:
		_, expected, parents, err := safefile.ObserveFileWithin(home, item.target)
		if err == nil {
			_, err = session.RestoreFileWithMode(item.target, parents, expected, source, item.source, item.desiredMode)
		}
		recordCatalogRestore(result, item.target, "write", "restore committed with a durability/verification warning", err)
	case TargetDirectory:
		expected, parents, err := safefile.ObserveDirectoryWithin(home, item.target)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		if err == nil {
			_, err = session.RestoreDirectory(item.target, parents, expected, source, item.source)
		}
		recordCatalogRestore(result, item.target, "restore directory", "directory restore committed with a durability/cleanup warning", err)
	default:
		result.Skipped[item.target] = fmt.Sprintf("invalid accepted target kind %q", item.kind)
	}
}

func removeCatalogTarget(result *RestoreResult, session *safefile.RestoreSession, home string, item catalogRestoreItem) {
	var err error
	switch item.kind {
	case TargetFile:
		_, expected, parents, observeErr := safefile.ObserveFileWithin(home, item.target)
		err = observeErr
		if err == nil && expected.Exists() {
			err = session.RemoveFile(item.target, parents, expected)
		}
	case TargetDirectory:
		expected, parents, observeErr := safefile.ObserveDirectoryWithin(home, item.target)
		err = observeErr
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		if err == nil && expected != nil {
			err = session.RemoveDirectory(item.target, parents, expected)
		}
	default:
		result.Skipped[item.target] = fmt.Sprintf("invalid accepted target kind %q", item.kind)
		return
	}

	if err == nil || errors.Is(err, os.ErrNotExist) {
		result.Removed = append(result.Removed, item.target)
		return
	}
	var committed *safefile.CommittedError
	if errors.As(err, &committed) {
		result.Removed = append(result.Removed, item.target)
		result.Warnings[item.target] = fmt.Sprintf("removal committed with a durability/cleanup warning: %v", err)
		return
	}
	result.Skipped[item.target] = fmt.Sprintf("remove: %v", err)
}

func recordCatalogRestore(result *RestoreResult, target, operation, committedWarning string, err error) {
	if err == nil {
		result.Restored = append(result.Restored, target)
		return
	}
	var committed *safefile.CommittedError
	if errors.As(err, &committed) {
		result.Restored = append(result.Restored, target)
		result.Warnings[target] = fmt.Sprintf("%s: %v", committedWarning, err)
		return
	}
	result.Skipped[target] = fmt.Sprintf("%s: %v", operation, err)
}

func RemoveCatalogEntry(entry CatalogEntry) error {
	if err := ValidateCatalogEntry(entry); err != nil {
		return err
	}
	return safefile.RemoveDirectoryWithinSnapshotAuthorized(entry.authority.anchor, entry.authority.rel, entry.authority.snapshot, entry.authority.parents)
}

func catalogAnchor(backupsDir string) (string, string, error) {
	clean := filepath.Clean(backupsDir)
	if !filepath.IsAbs(clean) {
		return "", "", fmt.Errorf("backup catalog path must be absolute: %s", backupsDir)
	}
	candidates := []string{os.Getenv("HOME"), os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_STATE_HOME")}
	for _, candidate := range candidates {
		if candidate == "" || !filepath.IsAbs(candidate) {
			continue
		}
		candidate = filepath.Clean(candidate)
		if info, err := os.Lstat(candidate); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if rel, ok := catalogRelative(candidate, clean); ok {
			return candidate, rel, nil
		}
	}
	anchor := filepath.Dir(clean)
	info, err := os.Lstat(anchor)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("backup catalog has no trusted real parent: %s", anchor)
	}
	return anchor, filepath.Base(clean), nil
}

func catalogRelative(anchor, path string) (string, bool) {
	rel, err := filepath.Rel(anchor, path)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func validCatalogName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\\x00\r\n\t")
}
