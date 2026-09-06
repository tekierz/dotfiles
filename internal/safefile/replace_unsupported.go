//go:build !darwin && !linux

package safefile

import (
	"fmt"
	"io/fs"
	"os"
)

// SnapshotDirectoryWithin is unavailable on platforms without the required
// descriptor-relative directory operations.
func SnapshotDirectoryWithin(_ string, _ string) (*DirectorySnapshot, error) {
	return nil, ErrUnsupported
}

func SnapshotDirectoryWithinBudget(_ string, _ string, _ SnapshotBudget) (*DirectorySnapshot, error) {
	return nil, ErrUnsupported
}

func CaptureParentChainWithin(_ string, _ string) (*ParentChain, error) { return nil, ErrUnsupported }

func BindParentChainWithin(_ string, _ string, _ *ParentChain, _ map[string]*DirectorySnapshot) (*ParentChain, error) {
	return nil, ErrUnsupported
}

func ExtendParentChainWithinDirectory(_ string, _, _ string, _ *ParentChain, _ *DirectorySnapshot) (*ParentChain, error) {
	return nil, ErrUnsupported
}

func ReadWithinAuthorized(_ string, _ string, _ *ParentChain) ([]byte, Revision, error) {
	return nil, Revision{}, ErrUnsupported
}

func ReadWithinAuthorizedLimit(_ string, _ string, _ *ParentChain, limit int64) ([]byte, Revision, error) {
	if limit < 0 {
		return nil, Revision{}, fmt.Errorf("%w: negative limit %d", ErrSizeLimit, limit)
	}
	return nil, Revision{}, ErrUnsupported
}

func ObserveFileWithin(_ string, _ string) ([]byte, Revision, *ParentChain, error) {
	return nil, Revision{}, nil, ErrUnsupported
}

func ObserveFileWithinLimit(_ string, _ string, _ int64) ([]byte, Revision, *ParentChain, error) {
	return nil, Revision{}, nil, ErrUnsupported
}

func ObserveDirectoryWithin(_ string, _ string) (*DirectorySnapshot, *ParentChain, error) {
	return nil, nil, ErrUnsupported
}

func ObserveDirectoryWithinBudget(_ string, _ string, _ SnapshotBudget) (*DirectorySnapshot, *ParentChain, error) {
	return nil, nil, ErrUnsupported
}

func VerifyDirectoryWithinSnapshot(_ string, _ string, _ *DirectorySnapshot) error {
	return ErrUnsupported
}

// RestoreDirectoryWithin is unavailable on platforms without the required
// descriptor-relative directory and durability operations.
func RestoreDirectoryWithin(_ string, _ string, _ *DirectorySnapshot) error {
	return ErrUnsupported
}

func RestoreDirectoryWithinSnapshot(_ string, _ string, _, _ *DirectorySnapshot) error {
	return ErrUnsupported
}

func RestoreDirectoryWithinSnapshotTracked(_ string, _ string, _, _ *DirectorySnapshot) (*DirectorySnapshot, error) {
	return nil, ErrUnsupported
}

func RestoreDirectoryWithinSnapshotNoCreateTracked(_ string, _ string, _, _ *DirectorySnapshot) (*DirectorySnapshot, error) {
	return nil, ErrUnsupported
}

func RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(_ string, _ string, _, _ *DirectorySnapshot, _ *ParentChain) (*DirectorySnapshot, error) {
	return nil, ErrUnsupported
}

// RemoveDirectoryWithin is unavailable on platforms without the required
// descriptor-relative recursive removal and durability operations.
func RemoveDirectoryWithin(_ string, _ string) error {
	return ErrUnsupported
}

func RemoveDirectoryWithinSnapshot(_ string, _ string, _ *DirectorySnapshot) error {
	return ErrUnsupported
}

func RemoveDirectoryWithinSnapshotAuthorized(_ string, _ string, _ *DirectorySnapshot, _ *ParentChain) error {
	return ErrUnsupported
}

func EnsureShallowDirectoryWithinSnapshotTracked(_ string, _ string, _ *DirectorySnapshot, _ fs.FileMode) (*DirectorySnapshot, error) {
	return nil, ErrUnsupported
}

func EnsureShallowDirectoryWithinParentChainTracked(_ string, _ string, _ *DirectorySnapshot, _ *ParentChain, _ fs.FileMode) (*DirectorySnapshot, error) {
	return nil, ErrUnsupported
}

func RemoveEmptyDirectoryWithinSnapshot(_ string, _ string, _ *DirectorySnapshot) error {
	return ErrUnsupported
}

func RemoveEmptyDirectoryWithinSnapshotAuthorized(_ string, _ string, _ *DirectorySnapshot, _ *ParentChain) error {
	return ErrUnsupported
}

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

func ReplaceWithinTracked(_ string, _ string, _ []byte, _ fs.FileMode) (Revision, error) {
	return Revision{}, ErrUnsupported
}

func ReplaceWithinRevision(_ string, _ string, _ Revision, _ []byte, _ fs.FileMode) error {
	return ErrUnsupported
}

func ReplaceWithinRevisionTracked(_ string, _ string, _ Revision, _ []byte, _ fs.FileMode) (Revision, error) {
	return Revision{}, ErrUnsupported
}

func ReplaceWithinRevisionNoCreateTracked(_ string, _ string, _ Revision, _ []byte, _ fs.FileMode) (Revision, error) {
	return Revision{}, ErrUnsupported
}

func ReplaceWithinRevisionNoCreateAuthorizedTracked(_ string, _ string, _ Revision, _ *ParentChain, _ []byte, _ fs.FileMode) (Revision, error) {
	return Revision{}, ErrUnsupported
}

// ReadWithin is unavailable on platforms without descriptor-relative open and
// no-follow operations.
func ReadWithin(_ string, _ string) ([]byte, Revision, error) {
	return nil, Revision{}, ErrUnsupported
}

// ReadWithinLimit is unavailable on platforms without descriptor-relative
// open and no-follow operations.
func ReadWithinLimit(_ string, _ string, _ int64) ([]byte, Revision, error) {
	return nil, Revision{}, ErrUnsupported
}

// AcquireLockWithin is unavailable on platforms without descriptor-relative
// no-follow operations and flock.
func AcquireLockWithin(_ string, _ string, _ fs.FileMode) (func() error, error) {
	return nil, ErrUnsupported
}

func AcquireLockWithinAuthorized(_ string, _ string, _ fs.FileMode, _ *ParentChain) (func() error, error) {
	return nil, ErrUnsupported
}

func CaptureDirectoryRootWithin(_ string, _ string) (*DirectorySnapshot, *ParentChain, error) {
	return nil, nil, ErrUnsupported
}

func BindParentChainPrefixWithin(_ string, _, _ string, _ *ParentChain, _ map[string]*DirectorySnapshot) (*ParentChain, error) {
	return nil, ErrUnsupported
}

func ValidateParentChainWithin(_ string, _ string, _ *ParentChain, _ map[string]*DirectorySnapshot) (*ParentChain, error) {
	return nil, ErrUnsupported
}

func OpenDirectoryWithinAuthorized(_ string, _ string, _ *ParentChain, _ *DirectorySnapshot) (*os.File, error) {
	return nil, ErrUnsupported
}

func CaptureChildDirectoryWithinAuthorized(_ string, _, _ string, _ *ParentChain, _ *DirectorySnapshot) (*DirectorySnapshot, *ParentChain, error) {
	return nil, nil, ErrUnsupported
}

// RemoveWithin is unavailable on platforms without descriptor-relative open,
// no-follow, unlink, and directory durability operations.
func RemoveWithin(_ string, _ string) error {
	return ErrUnsupported
}

func RemoveWithinRevision(_ string, _ string, _ Revision) error {
	return ErrUnsupported
}

func RemoveWithinRevisionAuthorized(_ string, _ string, _ Revision, _ *ParentChain) error {
	return ErrUnsupported
}
