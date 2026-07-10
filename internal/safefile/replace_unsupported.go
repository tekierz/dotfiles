//go:build !darwin && !linux

package safefile

import "io/fs"

// EnsureDirectoryWithin is unavailable on platforms without the required
// descriptor-relative directory operations.
func EnsureDirectoryWithin(_ string, _ string, _ fs.FileMode) error {
	return ErrUnsupported
}

// ReplaceWithin is unavailable on platforms without the required descriptor-
// relative filesystem operations.
func ReplaceWithin(_ string, _ string, _ []byte, _ fs.FileMode) error {
	return ErrUnsupported
}

// ReadWithin is unavailable on platforms without descriptor-relative open and
// no-follow operations.
func ReadWithin(_ string, _ string) ([]byte, Revision, error) {
	return nil, Revision{}, ErrUnsupported
}

// AcquireLockWithin is unavailable on platforms without descriptor-relative
// no-follow operations and flock.
func AcquireLockWithin(_ string, _ string, _ fs.FileMode) (func() error, error) {
	return nil, ErrUnsupported
}

// RemoveWithin is unavailable on platforms without descriptor-relative open,
// no-follow, unlink, and directory durability operations.
func RemoveWithin(_ string, _ string) error {
	return ErrUnsupported
}
