//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
	"io/fs"

	"golang.org/x/sys/unix"
)

// EnsureShallowDirectoryWithinSnapshotTracked creates exactly the leaf
// directory named by rel when expected is nil, or proves that the existing
// leaf still matches expected. Descendant parents must already exist and are
// never created. A newly created leaf is private (0700), durable, empty, and
// returned as opaque identity-bound evidence.
func EnsureShallowDirectoryWithinSnapshotTracked(root, rel string, expected *DirectorySnapshot, mode fs.FileMode) (*DirectorySnapshot, error) {
	return ensureShallowDirectoryWithinSnapshotTracked(root, rel, expected, nil, mode)
}

// EnsureShallowDirectoryWithinParentChainTracked creates or verifies one leaf
// only while the complete accepted root-to-parent identity chain still holds.
func EnsureShallowDirectoryWithinParentChainTracked(root, rel string, expected *DirectorySnapshot, parents *ParentChain, mode fs.FileMode) (*DirectorySnapshot, error) {
	if !parents.Tracked() {
		return nil, fmt.Errorf("%w: expected parent chain is untracked", ErrParentChanged)
	}
	return ensureShallowDirectoryWithinSnapshotTracked(root, rel, expected, parents, mode)
}

func ensureShallowDirectoryWithinSnapshotTracked(root, rel string, expected *DirectorySnapshot, parents *ParentChain, mode fs.FileMode) (*DirectorySnapshot, error) {
	if mode != directoryMode {
		return nil, fmt.Errorf("%w: directory mode must be %04o, got %v", ErrInvalidMode, directoryMode, mode)
	}
	if expected != nil && !recursiveDirectorySnapshot(expected) {
		return nil, fmt.Errorf("%w: expected directory snapshot is not recursive", ErrDirectoryChanged)
	}
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return nil, err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()
	var parentFD int
	if parents != nil {
		parentFD, err = openAuthorizedParent(rootFD, directories, parents)
	} else {
		parentFD, err = openParent(rootFD, directories, false)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(parentFD) }()
	wantedParent, err := identityOf(parentFD)
	if err != nil {
		return nil, fmt.Errorf("identify shallow directory parent: %w", err)
	}

	if expected != nil {
		current, identity, err := snapshotDirectoryEntryAt(parentFD, target)
		if err != nil {
			return nil, directoryChangedError("accepted shallow directory changed", err)
		}
		if identity != (fileIdentity{device: expected.device, inode: expected.inode}) || !sameDirectorySnapshotTree(current, expected) {
			return nil, fmt.Errorf("%w: shallow directory no longer matches accepted snapshot", ErrDirectoryChanged)
		}
		if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
			return nil, fmt.Errorf("shallow directory parent verification: %w", err)
		}
		verified, verifiedIdentity, err := snapshotDirectoryEntryAt(parentFD, target)
		if err != nil || verifiedIdentity != identity || !sameDirectorySnapshotTree(verified, current) {
			return nil, directoryChangedError("accepted shallow directory changed during verification", err)
		}
		return verified, nil
	}

	if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
		if err == nil {
			return nil, fmt.Errorf("%w: expected shallow directory %q to remain absent", ErrDirectoryChanged, target)
		}
		return nil, fmt.Errorf("inspect shallow directory target %q: %w", target, err)
	}
	if hook := directoryTestHooks.beforeShallowMkdir; hook != nil {
		if err := hook(parentFD, target); err != nil {
			return nil, fmt.Errorf("before shallow directory creation: %w", err)
		}
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		return nil, fmt.Errorf("pre-create shallow directory parent verification: %w", err)
	}
	if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
		if err == nil {
			return nil, fmt.Errorf("%w: shallow directory %q appeared before creation", ErrDirectoryChanged, target)
		}
		return nil, fmt.Errorf("reinspect shallow directory target %q: %w", target, err)
	}
	if err := unix.Mkdirat(parentFD, target, uint32(mode.Perm())); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil, fmt.Errorf("%w: shallow directory %q appeared at creation boundary", ErrDirectoryChanged, target)
		}
		return nil, fmt.Errorf("create shallow directory %q: %w", target, err)
	}

	evidence, evidenceErr := captureCreatedShallowDirectory(root, rootFD, parentFD, directories, target, wantedParent, parents, mode)
	if evidenceErr != nil {
		return nil, &CommittedError{Operation: "verify created shallow directory", Err: evidenceErr}
	}
	return evidence, nil
}

func captureCreatedShallowDirectory(root string, rootFD, parentFD int, directories []string, target string, wantedParent fileIdentity, parents *ParentChain, mode fs.FileMode) (*DirectorySnapshot, error) {
	directoryFD, err := openDirectoryAt(parentFD, target)
	if err != nil {
		return nil, fmt.Errorf("open created shallow directory: %w", err)
	}
	defer func() { _ = unix.Close(directoryFD) }()
	identity, fileType, permissions, err := directoryDescriptorState(directoryFD)
	if err != nil || fileType != unix.S_IFDIR || permissions.Perm() != mode.Perm() {
		if err == nil {
			err = ErrDirectoryChanged
		}
		return nil, fmt.Errorf("%w: inspect created shallow directory: %v", ErrDirectoryChanged, err)
	}
	if hook := directoryTestHooks.afterShallowMkdir; hook != nil {
		if err := hook(parentFD, directoryFD, target); err != nil {
			return nil, fmt.Errorf("after shallow directory creation: %w", err)
		}
	}
	if err := syncDirectory(directoryFD, "created shallow directory"); err != nil {
		return nil, err
	}
	if err := syncDirectory(parentFD, "created shallow directory parent"); err != nil {
		return nil, err
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		return nil, err
	}
	evidence, actualIdentity, err := snapshotDirectoryEntryAt(parentFD, target)
	if err != nil || actualIdentity != identity || evidence.Permissions() != mode.Perm() || len(evidence.root.entries) != 0 {
		return nil, directoryChangedError("created shallow directory changed before evidence capture", err)
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		return nil, fmt.Errorf("post-evidence parent-chain verification: %w", err)
	}
	return evidence, nil
}

// RemoveEmptyDirectoryWithinSnapshot removes exactly one empty directory leaf
// while it still matches expected. It never removes children recursively. A
// concurrent child, replacement, or metadata change is preserved and reported
// as ErrDirectoryChanged.
func RemoveEmptyDirectoryWithinSnapshot(root, rel string, expected *DirectorySnapshot) error {
	return removeEmptyDirectoryWithinSnapshot(root, rel, expected, nil)
}

// RemoveEmptyDirectoryWithinSnapshotAuthorized removes one exact empty leaf
// only while its bound root-to-parent namespace chain still matches.
func RemoveEmptyDirectoryWithinSnapshotAuthorized(root, rel string, expected *DirectorySnapshot, parents *ParentChain) error {
	if !parents.Tracked() {
		return fmt.Errorf("%w: expected parent chain is untracked", ErrParentChanged)
	}
	return removeEmptyDirectoryWithinSnapshot(root, rel, expected, parents)
}

func removeEmptyDirectoryWithinSnapshot(root, rel string, expected *DirectorySnapshot, parents *ParentChain) error {
	if !recursiveDirectorySnapshot(expected) || len(expected.root.entries) != 0 {
		return fmt.Errorf("%w: expected snapshot must describe an empty directory", ErrDirectoryChanged)
	}
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()
	var parentFD int
	if parents != nil {
		parentFD, err = openAuthorizedParent(rootFD, directories, parents)
	} else {
		parentFD, err = openParent(rootFD, directories, false)
	}
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(parentFD) }()
	wantedParent, err := identityOf(parentFD)
	if err != nil {
		return fmt.Errorf("identify empty-directory parent: %w", err)
	}
	current, identity, err := snapshotDirectoryEntryAt(parentFD, target)
	if err != nil || identity != (fileIdentity{device: expected.device, inode: expected.inode}) || !sameDirectorySnapshotTree(current, expected) {
		return directoryChangedError("empty-directory target no longer matches expected snapshot", err)
	}
	directoryFD, err := openDirectoryAt(parentFD, target)
	if err != nil {
		return fmt.Errorf("open empty-directory target: %w", err)
	}
	defer func() { _ = unix.Close(directoryFD) }()
	if hook := directoryTestHooks.beforeRemoveEmpty; hook != nil {
		if err := hook(parentFD, directoryFD, target); err != nil {
			return fmt.Errorf("before empty-directory removal: %w", err)
		}
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		return fmt.Errorf("pre-remove empty-directory parent verification: %w", err)
	}
	final, finalIdentity, err := snapshotDirectoryEntryAt(parentFD, target)
	if err != nil || finalIdentity != identity || !sameDirectorySnapshotTree(final, current) {
		return directoryChangedError("empty-directory target changed before removal", err)
	}
	if err := unix.Unlinkat(parentFD, target, unix.AT_REMOVEDIR); err != nil {
		if errors.Is(err, unix.ENOTEMPTY) || errors.Is(err, unix.EEXIST) || errors.Is(err, unix.ENOENT) {
			return fmt.Errorf("%w: empty-directory target changed at removal boundary: %v", ErrDirectoryChanged, err)
		}
		return fmt.Errorf("remove empty directory %q: %w", target, err)
	}
	var postCommit []error
	if err := syncDirectory(parentFD, "empty-directory removal parent"); err != nil {
		postCommit = append(postCommit, err)
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		postCommit = append(postCommit, err)
	}
	if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
		if err == nil {
			err = fmt.Errorf("target was recreated")
		}
		postCommit = append(postCommit, fmt.Errorf("%w: verify removed empty directory: %v", ErrDirectoryChanged, err))
	}
	if len(postCommit) != 0 {
		return &CommittedError{Operation: "complete empty-directory removal", Err: errors.Join(postCommit...)}
	}
	return nil
}
