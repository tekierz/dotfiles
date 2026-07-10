package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tekierz/dotfiles/internal/safefile"
)

type TargetKind string

const (
	TargetFile      TargetKind = "file"
	TargetDirectory TargetKind = "directory"
)

// Target is one exact mutation destination represented in a plan-derived
// rollback point. Missing targets are recorded explicitly so restore can
// remove paths created by the operation.
type Target struct {
	RelPath string
	Kind    TargetKind
}

// CreatePlan creates a fail-closed rollback point for an exact action-plan
// scope. Sources are read descriptor-relatively below HOME; files keep their
// exact modes, directories use opaque recursive snapshots, and absent targets
// are written to the manifest as existed=no. The manifest is committed last,
// so an interrupted partial directory is never accepted as restorable.
func CreatePlan(home, backupDir string, targets []Target) (int, error) {
	if !filepath.IsAbs(home) || !filepath.IsAbs(backupDir) {
		return 0, fmt.Errorf("home and backup directory must be absolute")
	}
	if len(targets) == 0 {
		return 0, fmt.Errorf("plan backup scope is empty")
	}
	cleanBackup := filepath.Clean(backupDir)
	backupParent := filepath.Dir(cleanBackup)
	backupName := filepath.Base(cleanBackup)
	if backupName == "." || backupParent == cleanBackup {
		return 0, fmt.Errorf("invalid backup directory %q", backupDir)
	}
	if _, err := os.Lstat(cleanBackup); err == nil {
		return 0, fmt.Errorf("backup directory already exists: %s", cleanBackup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, fmt.Errorf("inspect backup directory: %w", err)
	}
	anchor := backupParent
	backupRel := backupName
	if rel, relErr := filepath.Rel(home, cleanBackup); relErr == nil && IsRestorePathSafe(home, rel) {
		// HOME is the trusted descriptor anchor for the production backup path.
		// Never promote an existing ~/.config descendant to an anchor: it could
		// itself be a symlink redirecting rollback data outside HOME.
		anchor = home
		backupRel = filepath.ToSlash(rel)
	} else {
		// Tests and explicit callers may choose a separate absolute backup root.
		// In that case the nearest existing ancestor supplied by the caller is
		// the trust boundary; descendants remain descriptor-relative/no-follow.
		for {
			info, statErr := os.Stat(anchor)
			if statErr == nil {
				if !info.IsDir() {
					return 0, fmt.Errorf("backup ancestor is not a directory: %s", anchor)
				}
				break
			}
			if !errors.Is(statErr, os.ErrNotExist) {
				return 0, fmt.Errorf("inspect backup ancestor %s: %w", anchor, statErr)
			}
			parent := filepath.Dir(anchor)
			if parent == anchor {
				return 0, fmt.Errorf("no existing backup ancestor for %s", backupDir)
			}
			backupRel = filepath.ToSlash(filepath.Join(filepath.Base(anchor), filepath.FromSlash(backupRel)))
			anchor = parent
		}
	}
	if err := safefile.EnsureDirectoryWithin(anchor, backupRel, 0o700); err != nil {
		return 0, fmt.Errorf("create plan backup directory: %w", err)
	}

	seen := make(map[string]struct{}, len(targets))
	manifest := make([]string, 0, len(targets))
	captured := 0
	for _, target := range targets {
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(target.RelPath)))
		if rel == "." || rel == "" || filepath.IsAbs(rel) || !IsRestorePathSafe(home, filepath.FromSlash(rel)) || strings.ContainsAny(rel, "|\r\n\x00") {
			return captured, fmt.Errorf("invalid plan backup target %q", target.RelPath)
		}
		if target.Kind != TargetFile && target.Kind != TargetDirectory {
			return captured, fmt.Errorf("invalid plan backup target kind %q for %s", target.Kind, rel)
		}
		if _, duplicate := seen[rel]; duplicate {
			return captured, fmt.Errorf("duplicate plan backup target %q", rel)
		}
		seen[rel] = struct{}{}

		original := filepath.Join(home, filepath.FromSlash(rel))
		backupPath := filepath.Join(cleanBackup, filepath.FromSlash(rel))
		if strings.ContainsRune(original, '|') || strings.ContainsRune(backupPath, '|') {
			return captured, fmt.Errorf("backup paths containing '|' are unsupported")
		}
		switch target.Kind {
		case TargetFile:
			data, revision, err := safefile.ReadWithin(home, rel)
			if err != nil {
				return captured, fmt.Errorf("read plan backup target %s: %w", rel, err)
			}
			if !revision.Exists() {
				manifest = append(manifest, fmt.Sprintf("%s||no|file|600", original))
				continue
			}
			if err := safefile.ReplaceWithin(cleanBackup, rel, data, revision.Permissions()); err != nil {
				return captured, fmt.Errorf("store plan backup file %s: %w", rel, err)
			}
			manifest = append(manifest, fmt.Sprintf("%s|%s|yes|file|%o", original, backupPath, revision.Permissions()))
			captured++
		case TargetDirectory:
			snapshot, err := safefile.SnapshotDirectoryWithin(home, rel)
			if errors.Is(err, os.ErrNotExist) {
				manifest = append(manifest, fmt.Sprintf("%s||no|directory|700", original))
				continue
			}
			if err != nil {
				return captured, fmt.Errorf("snapshot plan backup directory %s: %w", rel, err)
			}
			if err := safefile.RestoreDirectoryWithin(cleanBackup, rel, snapshot); err != nil {
				return captured, fmt.Errorf("store plan backup directory %s: %w", rel, err)
			}
			manifest = append(manifest, fmt.Sprintf("%s|%s|yes|directory|%o", original, backupPath, snapshot.Permissions()))
			captured++
		}
	}

	data := []byte(strings.Join(manifest, "\n") + "\n")
	if err := safefile.ReplaceWithin(cleanBackup, ManifestName, data, 0o600); err != nil {
		return captured, fmt.Errorf("commit plan backup manifest: %w", err)
	}
	return captured, nil
}

// Create backs up each of the given files (paths relative to home) into
// backupDir, writing a manifest that records the original path and mode of
// every captured file. backupDir is created if necessary.
//
// It returns the number of files actually captured. Unlike the old inline
// loops, it does NOT silently report success when nothing was backed up:
//   - if zero files were captured (none existed / all unreadable) it returns
//     an error and writes no manifest, so callers cannot claim a rollback
//     point that does not exist (C4/C5);
//   - if the manifest write fails it returns that error.
//
// Per-file stat/read/write errors are skipped (the file may simply not exist),
// matching the previous behaviour, but the aggregate result is now honest.
func Create(home, backupDir string, files []string) (int, error) {
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return 0, err
	}

	var manifest []string
	count := 0
	for _, relPath := range files {
		srcPath := filepath.Join(home, relPath)
		info, err := os.Stat(srcPath)
		if err != nil {
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			continue
		}

		// Flat storage name (human-readable); the manifest is the
		// authoritative source for the original path on restore.
		dstPath := filepath.Join(backupDir, EncodeName(relPath))
		if err := os.WriteFile(dstPath, data, 0o600); err != nil {
			continue
		}

		// Record the original path and mode so restore can reconstruct both
		// exactly (the underscore encoding is lossy).
		manifest = append(manifest, ManifestLine(relPath, info.Mode()))
		count++
	}

	if count == 0 {
		return 0, fmt.Errorf("no files were backed up (none of %d candidate files were present)", len(files))
	}

	manifestPath := filepath.Join(backupDir, ManifestName)
	if err := os.WriteFile(manifestPath, []byte(strings.Join(manifest, "\n")), 0o600); err != nil {
		return count, fmt.Errorf("write backup manifest: %w", err)
	}

	return count, nil
}
