package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
		manifestRel := filepath.ToSlash(filepath.Join(filepath.FromSlash(rel), ManifestName))
		manifestParents, authorityErr := safefile.ExtendParentChainWithinDirectory(anchor, manifestRel, rel, parents, snapshot)
		if authorityErr != nil {
			continue
		}
		manifest, revision, readErr := safefile.ReadWithinAuthorized(anchor, manifestRel, manifestParents)
		if readErr != nil || !revision.Exists() {
			continue
		}
		manifestEntries, parseErr := parseManifestData(manifest, home)
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
			FileCount: len(manifestEntries), Size: size,
			authority: &catalogAuthority{anchor: anchor, rel: rel, snapshot: snapshot, parents: parents},
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Timestamp.After(result[j].Timestamp) })
	return result, nil
}

func ValidateCatalogEntry(entry CatalogEntry) error {
	if entry.authority == nil || entry.authority.snapshot == nil || !entry.authority.parents.Tracked() {
		return fmt.Errorf("%w: backup catalog authority is incomplete", safefile.ErrDirectoryChanged)
	}
	current, err := safefile.SnapshotDirectoryWithin(entry.authority.anchor, entry.authority.rel)
	if err != nil || !safefile.SameDirectoryRootState(current, entry.authority.snapshot) || current.Digest() != entry.authority.snapshot.Digest() {
		return fmt.Errorf("%w: selected backup changed after listing: %v", safefile.ErrDirectoryChanged, err)
	}
	bound, err := safefile.BindParentChainWithin(entry.authority.anchor, entry.authority.rel, entry.authority.parents, nil)
	if err != nil || !safefile.SameParentChain(bound, entry.authority.parents) {
		return fmt.Errorf("%w: selected backup parent changed after listing: %v", safefile.ErrParentChanged, err)
	}
	return nil
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
