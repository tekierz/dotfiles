package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
// matching the previous behavior, but the aggregate result is now honest.
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
