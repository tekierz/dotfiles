package backup

import (
	"bufio"
	"bytes"
	"fmt"

	"github.com/tekierz/dotfiles/internal/safefile"
)

const (
	maxBackupManifestBytes     int64 = 1 << 20
	maxBackupManifestEntries         = 4096
	maxBackupManifestLineBytes       = 4096
	maxBackupSnapshotFiles           = 16384
	maxBackupFileBytes         int64 = 16 << 20
	maxBackupTotalBytes        int64 = 256 << 20
	maxCatalogRestoreItems           = 16384
	maxCatalogManifestBytes    int64 = 8 << 20
)

var backupSnapshotBudget = safefile.SnapshotBudget{
	MaxFiles:      maxBackupSnapshotFiles,
	MaxFileBytes:  maxBackupFileBytes,
	MaxTotalBytes: maxBackupTotalBytes,
}

func boundedManifestScanner(data []byte) (*bufio.Scanner, error) {
	if int64(len(data)) > maxBackupManifestBytes {
		return nil, fmt.Errorf("backup manifest exceeds %d bytes", maxBackupManifestBytes)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), maxBackupManifestLineBytes+1)
	return scanner, nil
}

func scanBackupManifest(data []byte, visit func(string) error) error {
	scanner, err := boundedManifestScanner(data)
	if err != nil {
		return err
	}
	entries := 0
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > maxBackupManifestLineBytes {
			return fmt.Errorf("backup manifest line exceeds %d bytes", maxBackupManifestLineBytes)
		}
		trimmed := bytes.TrimSpace([]byte(line))
		if len(trimmed) != 0 && trimmed[0] != '#' {
			entries++
			if entries > maxBackupManifestEntries {
				return fmt.Errorf("backup manifest exceeds %d entries", maxBackupManifestEntries)
			}
		}
		if err := visit(line); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("backup manifest line exceeds %d bytes", maxBackupManifestLineBytes)
	}
	return nil
}

func remainingBackupBytes(used int64) (int64, error) {
	if used < 0 || used > maxBackupTotalBytes {
		return 0, fmt.Errorf("backup payload exceeds %d total bytes", maxBackupTotalBytes)
	}
	return maxBackupTotalBytes - used, nil
}

func boundedBackupFileLimit(used int64) (int64, error) {
	remaining, err := remainingBackupBytes(used)
	if err != nil {
		return 0, err
	}
	if remaining < maxBackupFileBytes {
		return remaining, nil
	}
	return maxBackupFileBytes, nil
}

func generatedManifestSize(lines []string, fixedBytes int) (int64, error) {
	if len(lines) > maxBackupManifestEntries {
		return 0, fmt.Errorf("backup manifest exceeds %d entries", maxBackupManifestEntries)
	}
	total := int64(fixedBytes + 1)
	for _, line := range lines {
		if len(line) > maxBackupManifestLineBytes {
			return 0, fmt.Errorf("backup manifest line exceeds %d bytes", maxBackupManifestLineBytes)
		}
		total += int64(len(line) + 1)
		if total > maxBackupManifestBytes {
			return 0, fmt.Errorf("backup manifest exceeds %d bytes", maxBackupManifestBytes)
		}
	}
	return total, nil
}
