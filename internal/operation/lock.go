package operation

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tekierz/dotfiles/internal/safefile"
)

// AcquireStateLock acquires a per-target advisory lock below dotfiles' private
// operational-state root. Product writers use this instead of persistent lock
// files beside user configuration, keeping the reviewed config action scope
// free of hidden side effects. scope separates independent lock protocols;
// target should be the canonical absolute mutation destination.
func AcquireStateLock(scope, target string) (func() error, error) {
	if scope == "" || target == "" {
		return nil, fmt.Errorf("operation lock scope and target are required")
	}
	root, stateRel, err := stateAnchor()
	if err != nil {
		return nil, err
	}
	locksRel := filepath.ToSlash(filepath.Join(stateRel, "locks"))
	if err := safefile.EnsureDirectoryWithin(root, locksRel, 0o700); err != nil {
		return nil, fmt.Errorf("create private operation lock directory: %w", err)
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(locksRel)))
	if err != nil {
		return nil, fmt.Errorf("verify private operation lock directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("operation lock path is not a real directory")
	}
	if info.Mode().Perm() != 0o700 {
		return nil, fmt.Errorf("operation lock directory must be private (mode 0700), got %04o", info.Mode().Perm())
	}

	canonical, err := canonicalLockTarget(target)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(scope + "\x00" + canonical))
	lockRel := filepath.ToSlash(filepath.Join(locksRel, fmt.Sprintf("%x.lock", digest)))
	release, err := safefile.AcquireLockWithin(root, lockRel, 0o600)
	if err != nil {
		if errors.Is(err, safefile.ErrUnsupported) {
			return nil, fmt.Errorf("operation locking is unsupported: %w", err)
		}
		return nil, fmt.Errorf("acquire private operation lock: %w", err)
	}
	return release, nil
}

func canonicalLockTarget(target string) (string, error) {
	clean := filepath.Clean(target)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("operation lock target must be absolute: %s", target)
	}
	ancestor := filepath.Dir(clean)
	suffix := []string{filepath.Base(clean)}
	for {
		info, err := os.Lstat(ancestor)
		if err == nil {
			if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				return "", fmt.Errorf("operation lock target ancestor is not a directory: %s", ancestor)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect operation lock target ancestor: %w", err)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", fmt.Errorf("resolve operation lock target ancestor: %w", err)
	}
	parts := append([]string{resolved}, suffix...)
	return filepath.Join(parts...), nil
}
