//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type directoryHooks struct {
	afterSnapshotOpen     func(parentFD, directoryFD int, name string) error
	afterSnapshotFileOpen func(parentFD, fileFD int, name string) error
	afterMoveAside        func(parentFD int, target, recovery string) error
	beforeInstall         func(parentFD int, stagedFD int, staged, target string) error
	afterInstall          func(parentFD int, stagedFD int, target string) error
	beforeRemove          func(parentFD int, target string) error
	afterRemoveMove       func(parentFD int, target, recovery string) error
	afterRemoveCommit     func(parentFD int, recovery string) error
	beforeRemoveEntry     func(parentFD int, name string) error
}

// directoryTestHooks is package-private so hostile-boundary and recovery tests
// can inject mutations without expanding the production API.
var directoryTestHooks directoryHooks

type directoryEntryState struct {
	name     string
	identity fileIdentity
	fileType uint32
	mode     fs.FileMode
}

// SnapshotDirectoryWithin captures one directory tree below root through held,
// descriptor-relative, no-follow opens. Every node must be a directory or a
// regular file; symlinks, sockets, FIFOs and devices are rejected. The tree is
// captured twice through the same held root descriptor and must compare equal,
// making concurrent namespace/content changes observable before a snapshot is
// returned.
func SnapshotDirectoryWithin(root, rel string) (*DirectorySnapshot, error) {
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return nil, err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()
	parentFD, err := openParent(rootFD, directories, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(parentFD) }()

	snapshot, _, err := snapshotDirectoryEntryAt(parentFD, target)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func snapshotDirectoryEntryAt(parentFD int, target string) (*DirectorySnapshot, fileIdentity, error) {
	state, err := statDirectoryEntryAt(parentFD, target)
	if err != nil {
		return nil, fileIdentity{}, fmt.Errorf("inspect directory target %q: %w", target, err)
	}
	switch state.fileType {
	case unix.S_IFLNK:
		return nil, fileIdentity{}, fmt.Errorf("%w: directory target %q", ErrSymlink, target)
	case unix.S_IFDIR:
	default:
		return nil, fileIdentity{}, fmt.Errorf("%w: directory target %q has file type %#o", ErrNonRegular, target, state.fileType)
	}

	directoryFD, err := openDirectoryAt(parentFD, target)
	if err != nil {
		return nil, fileIdentity{}, fmt.Errorf("open directory target %q: %w", target, err)
	}
	defer func() { _ = unix.Close(directoryFD) }()
	openedIdentity, openedType, openedMode, err := directoryDescriptorState(directoryFD)
	if err != nil {
		return nil, fileIdentity{}, fmt.Errorf("inspect opened directory target %q: %w", target, err)
	}
	if openedType != unix.S_IFDIR || openedIdentity != state.identity || openedMode != state.mode {
		return nil, fileIdentity{}, fmt.Errorf("%w: directory target %q changed between inspection and open", ErrDirectoryChanged, target)
	}
	if hook := directoryTestHooks.afterSnapshotOpen; hook != nil {
		if err := hook(parentFD, directoryFD, target); err != nil {
			return nil, fileIdentity{}, fmt.Errorf("after opening directory snapshot target: %w", err)
		}
	}

	first, err := captureDirectoryNode(directoryFD)
	if err != nil {
		return nil, fileIdentity{}, err
	}
	second, err := captureDirectoryNode(directoryFD)
	if err != nil {
		return nil, fileIdentity{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return nil, fileIdentity{}, fmt.Errorf("%w: directory target %q produced different recursive captures", ErrDirectoryChanged, target)
	}
	actual, fileType, err := identityAt(parentFD, target)
	if err != nil || fileType != unix.S_IFDIR || actual != state.identity {
		return nil, fileIdentity{}, fmt.Errorf("%w: directory target %q namespace entry changed", ErrDirectoryChanged, target)
	}
	return newDirectorySnapshot(first), state.identity, nil
}

func captureDirectoryNode(directoryFD int) (directorySnapshotNode, error) {
	identityBefore, fileTypeBefore, modeBefore, err := directoryDescriptorState(directoryFD)
	if err != nil {
		return directorySnapshotNode{}, fmt.Errorf("inspect directory snapshot descriptor: %w", err)
	}
	if fileTypeBefore != unix.S_IFDIR {
		return directorySnapshotNode{}, fmt.Errorf("%w: snapshot descriptor is not a directory", ErrNonRegular)
	}
	states, err := listDirectoryStates(directoryFD)
	if err != nil {
		return directorySnapshotNode{}, err
	}

	node := directorySnapshotNode{mode: modeBefore, entries: make([]directorySnapshotEntry, 0, len(states))}
	for _, state := range states {
		switch state.fileType {
		case unix.S_IFLNK:
			return directorySnapshotNode{}, fmt.Errorf("%w: directory snapshot entry %q", ErrSymlink, state.name)
		case unix.S_IFREG:
			data, mode, err := captureRegularFileAt(directoryFD, state)
			if err != nil {
				return directorySnapshotNode{}, err
			}
			node.entries = append(node.entries, directorySnapshotEntry{name: state.name, mode: mode, data: data})
		case unix.S_IFDIR:
			childFD, err := openDirectoryAt(directoryFD, state.name)
			if err != nil {
				return directorySnapshotNode{}, fmt.Errorf("open snapshot directory %q: %w", state.name, err)
			}
			openedIdentity, openedType, openedMode, inspectErr := directoryDescriptorState(childFD)
			if inspectErr != nil || openedType != unix.S_IFDIR || openedIdentity != state.identity || openedMode != state.mode {
				_ = unix.Close(childFD)
				if inspectErr != nil {
					return directorySnapshotNode{}, fmt.Errorf("inspect snapshot directory %q: %w", state.name, inspectErr)
				}
				return directorySnapshotNode{}, fmt.Errorf("%w: snapshot directory %q changed before recursion", ErrDirectoryChanged, state.name)
			}
			child, err := captureDirectoryNode(childFD)
			closeErr := unix.Close(childFD)
			if err != nil {
				return directorySnapshotNode{}, err
			}
			if closeErr != nil {
				return directorySnapshotNode{}, fmt.Errorf("close snapshot directory %q: %w", state.name, closeErr)
			}
			node.entries = append(node.entries, directorySnapshotEntry{name: state.name, mode: child.mode, dir: &child})
		default:
			return directorySnapshotNode{}, fmt.Errorf("%w: directory snapshot entry %q has file type %#o", ErrNonRegular, state.name, state.fileType)
		}
	}

	statesAfter, err := listDirectoryStates(directoryFD)
	if err != nil {
		return directorySnapshotNode{}, err
	}
	identityAfter, fileTypeAfter, modeAfter, err := directoryDescriptorState(directoryFD)
	if err != nil {
		return directorySnapshotNode{}, fmt.Errorf("reinspect directory snapshot descriptor: %w", err)
	}
	if identityBefore != identityAfter || fileTypeAfter != unix.S_IFDIR || modeBefore != modeAfter || !reflect.DeepEqual(states, statesAfter) {
		return directorySnapshotNode{}, fmt.Errorf("%w: directory changed during recursive capture", ErrDirectoryChanged)
	}
	return node, nil
}

func captureRegularFileAt(parentFD int, expected directoryEntryState) ([]byte, fs.FileMode, error) {
	fd, err := unix.Openat(parentFD, expected.name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, 0, classifyLeafOpenError(parentFD, expected.name, err)
	}
	file := os.NewFile(uintptr(fd), expected.name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, 0, fmt.Errorf("wrap snapshot file descriptor %q", expected.name)
	}
	before, err := snapshotDescriptor(fd, file)
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("inspect snapshot file %q: %w", expected.name, err)
	}
	if before.identity != expected.identity || fs.FileMode(before.mode).Type() != 0 || fs.FileMode(before.mode).Perm() != expected.mode {
		_ = file.Close()
		return nil, 0, fmt.Errorf("%w: snapshot file %q changed before read", ErrDirectoryChanged, expected.name)
	}
	if hook := directoryTestHooks.afterSnapshotFileOpen; hook != nil {
		if err := hook(parentFD, fd, expected.name); err != nil {
			_ = file.Close()
			return nil, 0, fmt.Errorf("after opening snapshot file %q: %w", expected.name, err)
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("read snapshot file %q: %w", expected.name, err)
	}
	after, err := snapshotDescriptor(fd, file)
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("reinspect snapshot file %q: %w", expected.name, err)
	}
	if before != after || int64(len(data)) != after.size {
		_ = file.Close()
		return nil, 0, fmt.Errorf("%w: snapshot file %q changed during read", ErrDirectoryChanged, expected.name)
	}
	if err := file.Close(); err != nil {
		return nil, 0, fmt.Errorf("close snapshot file %q: %w", expected.name, err)
	}
	return data, fs.FileMode(after.mode).Perm(), nil
}

func listDirectoryStates(directoryFD int) ([]directoryEntryState, error) {
	scanFD, err := unix.Openat(directoryFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open directory snapshot scan: %w", err)
	}
	file := os.NewFile(uintptr(scanFD), "directory-snapshot")
	if file == nil {
		_ = unix.Close(scanFD)
		return nil, fmt.Errorf("wrap directory snapshot scan descriptor")
	}
	entries, err := file.ReadDir(-1)
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("read directory snapshot entries: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close directory snapshot scan: %w", closeErr)
	}
	states := make([]directoryEntryState, 0, len(entries))
	for _, entry := range entries {
		state, err := statDirectoryEntryAt(directoryFD, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("inspect directory snapshot entry %q: %w", entry.Name(), err)
		}
		states = append(states, state)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].name < states[j].name })
	return states, nil
}

func statDirectoryEntryAt(parentFD int, name string) (directoryEntryState, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return directoryEntryState{}, err
	}
	return directoryEntryState{
		name:     name,
		identity: identityFromStat(&stat),
		fileType: uint32(stat.Mode) & unix.S_IFMT,
		mode:     fs.FileMode(uint32(stat.Mode) & 0o777),
	}, nil
}

func directoryDescriptorState(fd int) (fileIdentity, uint32, fs.FileMode, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fileIdentity{}, 0, 0, err
	}
	return identityFromStat(&stat), uint32(stat.Mode) & unix.S_IFMT, fs.FileMode(uint32(stat.Mode) & 0o777), nil
}

// RestoreDirectoryWithin transactionally replaces a directory at rel with an
// immutable snapshot. A complete, fsynced sibling tree is staged first. An
// existing symlink-free directory is moved to a randomized recovery name only
// after both trees and the held parent identity are verified. Any failure before
// installing the staged tree rolls the original name back; failures after the
// staged rename return *CommittedError and leave the restored tree in place.
func RestoreDirectoryWithin(root, rel string, snapshot *DirectorySnapshot) (returnErr error) {
	return restoreDirectoryWithinSnapshot(root, rel, snapshot, nil, false)
}

// RestoreDirectoryWithinSnapshot replaces rel only if its exact recursive
// post-write snapshot still matches expected. A nil expected snapshot requires
// the target to remain absent through the commit boundary.
func RestoreDirectoryWithinSnapshot(root, rel string, snapshot, expected *DirectorySnapshot) error {
	if expected != nil && !expected.tracked {
		return fmt.Errorf("%w: expected directory snapshot is untracked", ErrDirectoryChanged)
	}
	return restoreDirectoryWithinSnapshot(root, rel, snapshot, expected, true)
}

func restoreDirectoryWithinSnapshot(root, rel string, snapshot, expected *DirectorySnapshot, conditional bool) (returnErr error) {
	if snapshot == nil || !snapshot.tracked {
		return fmt.Errorf("directory snapshot is nil or untracked")
	}
	if err := validateDirectorySnapshotNode(&snapshot.root); err != nil {
		return err
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
	parentFD, err := openParent(rootFD, directories, true)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(parentFD) }()
	wantedParent, err := identityOf(parentFD)
	if err != nil {
		return fmt.Errorf("identify directory restore parent: %w", err)
	}

	var existingSnapshot *DirectorySnapshot
	var existingIdentity fileIdentity
	existing := false
	if _, _, inspectErr := identityAt(parentFD, target); inspectErr == nil {
		existingSnapshot, existingIdentity, err = snapshotDirectoryEntryAt(parentFD, target)
		if err != nil {
			return fmt.Errorf("validate live directory before restore: %w", err)
		}
		existing = true
	} else if !errors.Is(inspectErr, unix.ENOENT) {
		return fmt.Errorf("inspect live directory before restore: %w", inspectErr)
	}
	if conditional {
		switch {
		case expected == nil && existing:
			return fmt.Errorf("%w: expected restore target %q to remain absent", ErrDirectoryChanged, target)
		case expected != nil && !existing:
			return fmt.Errorf("%w: expected restore target %q disappeared", ErrDirectoryChanged, target)
		case expected != nil && !reflect.DeepEqual(existingSnapshot.root, expected.root):
			return fmt.Errorf("%w: restore target %q no longer matches expected post-write tree", ErrDirectoryChanged, target)
		}
	}

	stagedName, stagedFD, stagedIdentity, err := stageDirectorySnapshot(parentFD, &snapshot.root)
	if err != nil {
		return err
	}
	stagedNamed := true
	stagedOpen := true
	defer func() {
		var cleanupErr error
		if stagedNamed && stagedOpen {
			if err := makeStagedDirectoryRemovable(stagedFD); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("prepare staged directory cleanup: %w", err))
			}
		}
		if stagedOpen {
			stagedOpen = false
			if err := unix.Close(stagedFD); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("close staged directory: %w", err))
			}
		}
		if stagedNamed {
			if err := removeDirectoryEntryAt(parentFD, stagedName, stagedIdentity); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cleanup staged directory %q: %w", stagedName, err))
			}
		}
		if cleanupErr != nil {
			returnErr = errors.Join(returnErr, cleanupErr)
		}
	}()

	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		return fmt.Errorf("pre-restore parent verification: %w", err)
	}
	if err := verifyDirectoryEntry(parentFD, stagedName, stagedIdentity); err != nil {
		return fmt.Errorf("pre-restore staging verification: %w", err)
	}
	if err := verifyStagedDirectorySnapshot(stagedFD, &snapshot.root); err != nil {
		return fmt.Errorf("pre-restore staged tree verification: %w", err)
	}
	if existing {
		current, currentIdentity, err := snapshotDirectoryEntryAt(parentFD, target)
		if err != nil || currentIdentity != existingIdentity || !reflect.DeepEqual(current.root, existingSnapshot.root) {
			return directoryChangedError("live directory changed while restore was staged", err)
		}
	} else if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
		if err == nil {
			return fmt.Errorf("%w: restore target %q appeared before commit", ErrDirectoryChanged, target)
		}
		return fmt.Errorf("reinspect absent restore target %q: %w", target, err)
	}

	recoveryName := ""
	recoveryLive := false
	if existing {
		recoveryName, err = unusedDirectoryName(parentFD, ".safefile-recovery-")
		if err != nil {
			return err
		}
		if err := unix.Renameat(parentFD, target, parentFD, recoveryName); err != nil {
			return fmt.Errorf("move live directory to recovery name: %w", err)
		}
		recoveryLive = true
		rollback := func(primary error) error {
			restored, rollbackErr := rollbackDirectoryName(parentFD, target, recoveryName, existingIdentity, rootFD, directories, wantedParent)
			if restored {
				recoveryLive = false
			}
			if rollbackErr != nil {
				return &RecoveryError{Operation: "restore original directory name", Err: errors.Join(primary, rollbackErr)}
			}
			return primary
		}
		if hook := directoryTestHooks.afterMoveAside; hook != nil {
			if err := hook(parentFD, target, recoveryName); err != nil {
				return rollback(fmt.Errorf("after moving live directory aside: %w", err))
			}
		}
		if err := verifyParent(rootFD, directories, wantedParent); err != nil {
			return rollback(fmt.Errorf("moved-aside parent verification: %w", err))
		}
		if err := verifyDirectoryEntry(parentFD, recoveryName, existingIdentity); err != nil {
			return rollback(fmt.Errorf("moved-aside directory verification: %w", err))
		}
		if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
			if err == nil {
				err = fmt.Errorf("target was recreated")
			}
			return rollback(directoryChangedError(fmt.Sprintf("restore target %q is not absent after move-aside", target), err))
		}
		if hook := directoryTestHooks.beforeInstall; hook != nil {
			if err := hook(parentFD, stagedFD, stagedName, target); err != nil {
				return rollback(fmt.Errorf("before installing staged directory: %w", err))
			}
		}
		if err := verifyParent(rootFD, directories, wantedParent); err != nil {
			return rollback(fmt.Errorf("install-boundary parent verification: %w", err))
		}
		if err := verifyDirectoryEntry(parentFD, recoveryName, existingIdentity); err != nil {
			return rollback(fmt.Errorf("install-boundary recovery verification: %w", err))
		}
		recoverySnapshot, recoveryIdentity, snapshotErr := snapshotDirectoryEntryAt(parentFD, recoveryName)
		if snapshotErr != nil || recoveryIdentity != existingIdentity || !reflect.DeepEqual(recoverySnapshot.root, existingSnapshot.root) {
			return rollback(directoryChangedError("moved-aside directory changed before restore commit", snapshotErr))
		}
		if err := verifyDirectoryEntry(parentFD, stagedName, stagedIdentity); err != nil {
			return rollback(fmt.Errorf("install-boundary staging verification: %w", err))
		}
		if err := verifyStagedDirectorySnapshot(stagedFD, &snapshot.root); err != nil {
			return rollback(fmt.Errorf("install-boundary staged tree verification: %w", err))
		}
		if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
			if err == nil {
				err = fmt.Errorf("target was recreated")
			}
			return rollback(directoryChangedError(fmt.Sprintf("restore target %q changed at install boundary", target), err))
		}
		if err := unix.Renameat(parentFD, stagedName, parentFD, target); err != nil {
			return rollback(fmt.Errorf("install staged directory: %w", err))
		}
	} else {
		if hook := directoryTestHooks.beforeInstall; hook != nil {
			if err := hook(parentFD, stagedFD, stagedName, target); err != nil {
				return fmt.Errorf("before installing staged directory: %w", err)
			}
		}
		if err := verifyParent(rootFD, directories, wantedParent); err != nil {
			return fmt.Errorf("install-boundary parent verification: %w", err)
		}
		if err := verifyDirectoryEntry(parentFD, stagedName, stagedIdentity); err != nil {
			return fmt.Errorf("install-boundary staging verification: %w", err)
		}
		if err := verifyStagedDirectorySnapshot(stagedFD, &snapshot.root); err != nil {
			return fmt.Errorf("install-boundary staged tree verification: %w", err)
		}
		if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
			if err == nil {
				return fmt.Errorf("%w: restore target %q appeared before install", ErrDirectoryChanged, target)
			}
			return fmt.Errorf("reinspect restore target %q: %w", target, err)
		}
		if err := unix.Renameat(parentFD, stagedName, parentFD, target); err != nil {
			return fmt.Errorf("install staged directory: %w", err)
		}
	}
	stagedNamed = false

	var postCommit []error
	var operations []string
	if hook := directoryTestHooks.afterInstall; hook != nil {
		if err := hook(parentFD, stagedFD, target); err != nil {
			postCommit = append(postCommit, fmt.Errorf("post-install boundary: %w", err))
			operations = append(operations, "post-install hook")
		}
	}
	if err := verifyDirectoryEntry(parentFD, target, stagedIdentity); err != nil {
		postCommit = append(postCommit, err)
		operations = append(operations, "installed directory verification")
	}
	stagedOpen = false
	if err := unix.Close(stagedFD); err != nil {
		postCommit = append(postCommit, fmt.Errorf("close installed directory: %w", err))
		operations = append(operations, "installed directory close")
	}
	if err := syncDirectory(parentFD, "directory restore parent"); err != nil {
		postCommit = append(postCommit, err)
		operations = append(operations, "restore parent fsync")
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		postCommit = append(postCommit, err)
		operations = append(operations, "post-restore parent verification")
	}
	// Preserve the original recovery tree whenever install verification or
	// durability is uncertain. Deleting it in that state would turn a detectable
	// committed warning into irreversible data loss.
	if recoveryLive && len(postCommit) == 0 {
		if err := removeDirectoryEntryAt(parentFD, recoveryName, existingIdentity); err != nil {
			postCommit = append(postCommit, fmt.Errorf("cleanup recovery directory %q: %w", recoveryName, err))
			operations = append(operations, "old directory cleanup")
		} else {
			recoveryLive = false
		}
	}
	if len(postCommit) != 0 {
		return &CommittedError{Operation: strings.Join(operations, "; "), Err: errors.Join(postCommit...)}
	}
	return nil
}

// RemoveDirectoryWithin transactionally removes a symlink-free directory tree.
// The validated tree is first renamed to a randomized recovery name and the
// parent is fsynced. Precommit failures roll that rename back. Recursive cleanup
// happens only after the removal name is durably committed; cleanup or later
// durability failures therefore return *CommittedError.
func RemoveDirectoryWithin(root, rel string) error {
	return removeDirectoryWithinSnapshot(root, rel, nil)
}

// RemoveDirectoryWithinSnapshot removes rel only while its exact recursive
// snapshot still matches expected at the move-aside commit boundary.
func RemoveDirectoryWithinSnapshot(root, rel string, expected *DirectorySnapshot) error {
	if expected == nil || !expected.tracked {
		return fmt.Errorf("%w: expected removal snapshot is nil or untracked", ErrDirectoryChanged)
	}
	return removeDirectoryWithinSnapshot(root, rel, expected)
}

func removeDirectoryWithinSnapshot(root, rel string, expected *DirectorySnapshot) error {
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()
	parentFD, err := openParent(rootFD, directories, false)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(parentFD) }()
	wantedParent, err := identityOf(parentFD)
	if err != nil {
		return fmt.Errorf("identify directory removal parent: %w", err)
	}
	if _, _, err := identityAt(parentFD, target); errors.Is(err, unix.ENOENT) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect directory removal target: %w", err)
	}
	targetSnapshot, targetIdentity, err := snapshotDirectoryEntryAt(parentFD, target)
	if err != nil {
		return fmt.Errorf("validate directory before removal: %w", err)
	}
	if expected != nil && !reflect.DeepEqual(targetSnapshot.root, expected.root) {
		return fmt.Errorf("%w: removal target no longer matches expected post-write tree", ErrDirectoryChanged)
	}
	if hook := directoryTestHooks.beforeRemove; hook != nil {
		if err := hook(parentFD, target); err != nil {
			return fmt.Errorf("before directory removal: %w", err)
		}
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		return fmt.Errorf("pre-remove parent verification: %w", err)
	}
	current, currentIdentity, err := snapshotDirectoryEntryAt(parentFD, target)
	if err != nil || currentIdentity != targetIdentity || !reflect.DeepEqual(current.root, targetSnapshot.root) {
		return directoryChangedError("directory changed while removal was prepared", err)
	}
	recoveryName, err := unusedDirectoryName(parentFD, ".safefile-removed-")
	if err != nil {
		return err
	}
	if err := unix.Renameat(parentFD, target, parentFD, recoveryName); err != nil {
		return fmt.Errorf("move removed directory to recovery name: %w", err)
	}
	rollback := func(primary error) error {
		restored, rollbackErr := rollbackDirectoryName(parentFD, target, recoveryName, targetIdentity, rootFD, directories, wantedParent)
		if rollbackErr != nil {
			return &RecoveryError{Operation: "restore directory after failed removal", Err: errors.Join(primary, rollbackErr)}
		}
		if !restored {
			return &RecoveryError{Operation: "restore directory after failed removal", Err: primary}
		}
		return primary
	}
	if hook := directoryTestHooks.afterRemoveMove; hook != nil {
		if err := hook(parentFD, target, recoveryName); err != nil {
			return rollback(fmt.Errorf("after moving removed directory aside: %w", err))
		}
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		return rollback(fmt.Errorf("removal commit-boundary parent verification: %w", err))
	}
	if err := verifyDirectoryEntry(parentFD, recoveryName, targetIdentity); err != nil {
		return rollback(fmt.Errorf("removal commit-boundary recovery verification: %w", err))
	}
	recoverySnapshot, recoveryIdentity, snapshotErr := snapshotDirectoryEntryAt(parentFD, recoveryName)
	if snapshotErr != nil || recoveryIdentity != targetIdentity || !reflect.DeepEqual(recoverySnapshot.root, targetSnapshot.root) {
		return rollback(directoryChangedError("moved-aside directory changed before removal commit", snapshotErr))
	}
	if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
		if err == nil {
			err = fmt.Errorf("target was recreated")
		}
		return rollback(directoryChangedError(fmt.Sprintf("removal target %q changed at commit boundary", target), err))
	}
	if err := syncDirectory(parentFD, "directory removal parent"); err != nil {
		return rollback(err)
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		return &CommittedError{Operation: "post-removal parent verification", Err: err}
	}
	if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
		if err == nil {
			err = fmt.Errorf("target was recreated")
		}
		return &CommittedError{Operation: "removed directory verification", Err: directoryChangedError("removed directory changed after commit", err)}
	}
	if err := verifyDirectoryEntry(parentFD, recoveryName, targetIdentity); err != nil {
		return &CommittedError{Operation: "removal recovery entry verification", Err: err}
	}
	if hook := directoryTestHooks.afterRemoveCommit; hook != nil {
		if err := hook(parentFD, recoveryName); err != nil {
			return &CommittedError{Operation: "post-removal hook", Err: err}
		}
	}
	if err := removeDirectoryEntryAt(parentFD, recoveryName, targetIdentity); err != nil {
		return &CommittedError{Operation: "removed directory cleanup", Err: err}
	}
	return nil
}

func validateDirectorySnapshotNode(node *directorySnapshotNode) error {
	if node == nil || node.mode != node.mode.Perm() {
		return fmt.Errorf("%w: invalid directory snapshot mode", ErrInvalidMode)
	}
	previous := ""
	for _, entry := range node.entries {
		if entry.name == "" || entry.name == "." || entry.name == ".." || strings.Contains(entry.name, "/") || strings.ContainsRune(entry.name, '\x00') {
			return fmt.Errorf("%w: invalid directory snapshot entry %q", ErrInvalidPath, entry.name)
		}
		if previous != "" && entry.name <= previous {
			return fmt.Errorf("%w: duplicate or unsorted directory snapshot entry %q", ErrInvalidPath, entry.name)
		}
		previous = entry.name
		if entry.dir != nil {
			if entry.mode != entry.dir.mode || len(entry.data) != 0 {
				return fmt.Errorf("invalid directory snapshot entry %q", entry.name)
			}
			if err := validateDirectorySnapshotNode(entry.dir); err != nil {
				return err
			}
			continue
		}
		if entry.mode != entry.mode.Perm() {
			return fmt.Errorf("%w: invalid file mode for snapshot entry %q", ErrInvalidMode, entry.name)
		}
	}
	return nil
}

func verifyStagedDirectorySnapshot(directoryFD int, expected *directorySnapshotNode) error {
	first, err := captureDirectoryNode(directoryFD)
	if err != nil {
		return err
	}
	second, err := captureDirectoryNode(directoryFD)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(first, *expected) || !reflect.DeepEqual(second, *expected) {
		return fmt.Errorf("%w: staged directory no longer matches source snapshot", ErrStagedChanged)
	}
	return nil
}

func stageDirectorySnapshot(parentFD int, node *directorySnapshotNode) (string, int, fileIdentity, error) {
	for attempt := 0; attempt < tempAttempts; attempt++ {
		suffix, err := temporarySuffix()
		if err != nil {
			return "", -1, fileIdentity{}, fmt.Errorf("generate staged directory name: %w", err)
		}
		name := ".safefile-directory-" + suffix
		if err := unix.Mkdirat(parentFD, name, directoryMode); errors.Is(err, unix.EEXIST) {
			continue
		} else if err != nil {
			return "", -1, fileIdentity{}, fmt.Errorf("create staged directory: %w", err)
		}
		state, err := statDirectoryEntryAt(parentFD, name)
		if err != nil {
			cleanupErr := removeUnknownDirectoryEntryAt(parentFD, name)
			return "", -1, fileIdentity{}, errors.Join(fmt.Errorf("inspect staged directory: %w", err), cleanupErr)
		}
		fd, err := openDirectoryAt(parentFD, name)
		if err != nil {
			cleanupErr := removeDirectoryEntryAt(parentFD, name, state.identity)
			return "", -1, fileIdentity{}, errors.Join(fmt.Errorf("open staged directory: %w", err), cleanupErr)
		}
		openedIdentity, openedType, _, inspectErr := directoryDescriptorState(fd)
		if inspectErr != nil || openedType != unix.S_IFDIR || openedIdentity != state.identity {
			_ = unix.Close(fd)
			cleanupErr := removeDirectoryEntryAt(parentFD, name, state.identity)
			if inspectErr != nil {
				return "", -1, fileIdentity{}, errors.Join(fmt.Errorf("inspect opened staged directory: %w", inspectErr), cleanupErr)
			}
			return "", -1, fileIdentity{}, errors.Join(fmt.Errorf("%w: staged directory changed before open", ErrStagedChanged), cleanupErr)
		}
		if err := populateDirectorySnapshot(fd, node); err != nil {
			prepareErr := makeStagedDirectoryRemovable(fd)
			_ = unix.Close(fd)
			cleanupErr := removeDirectoryEntryAt(parentFD, name, state.identity)
			return "", -1, fileIdentity{}, errors.Join(err, prepareErr, cleanupErr)
		}
		return name, fd, state.identity, nil
	}
	return "", -1, fileIdentity{}, fmt.Errorf("create staged directory: exhausted %d randomized names", tempAttempts)
}

func removeUnknownDirectoryEntryAt(parentFD int, name string) error {
	state, err := statDirectoryEntryAt(parentFD, name)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect staged directory during cleanup: %w", err)
	}
	if state.fileType != unix.S_IFDIR {
		return fmt.Errorf("%w: staged cleanup entry %q is no longer a directory", ErrStagedChanged, name)
	}
	return removeDirectoryEntryAt(parentFD, name, state.identity)
}

// makeStagedDirectoryRemovable resets only the private, newly constructed
// staging tree to owner-only directory modes so precommit cleanup remains
// possible even when the captured final modes are read-only. Every descendant
// is inspected without following symlinks before chmod/open.
func makeStagedDirectoryRemovable(directoryFD int) error {
	if err := unix.Fchmod(directoryFD, directoryMode); err != nil {
		return fmt.Errorf("reset staged directory mode: %w", err)
	}
	states, err := listDirectoryStates(directoryFD)
	if err != nil {
		return err
	}
	for _, state := range states {
		switch state.fileType {
		case unix.S_IFLNK:
			return fmt.Errorf("%w: staged cleanup entry %q", ErrSymlink, state.name)
		case unix.S_IFREG:
			continue
		case unix.S_IFDIR:
			if err := unix.Fchmodat(directoryFD, state.name, directoryMode, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return fmt.Errorf("reset staged child directory mode %q: %w", state.name, err)
			}
			childFD, err := openDirectoryAt(directoryFD, state.name)
			if err != nil {
				return fmt.Errorf("open staged child directory %q for cleanup: %w", state.name, err)
			}
			openedIdentity, openedType, _, inspectErr := directoryDescriptorState(childFD)
			if inspectErr != nil || openedType != unix.S_IFDIR || openedIdentity != state.identity {
				_ = unix.Close(childFD)
				if inspectErr != nil {
					return fmt.Errorf("inspect staged child directory %q for cleanup: %w", state.name, inspectErr)
				}
				return fmt.Errorf("%w: staged child directory %q changed before cleanup", ErrDirectoryChanged, state.name)
			}
			if err := makeStagedDirectoryRemovable(childFD); err != nil {
				_ = unix.Close(childFD)
				return err
			}
			if err := unix.Close(childFD); err != nil {
				return fmt.Errorf("close staged child directory %q after cleanup preparation: %w", state.name, err)
			}
		default:
			return fmt.Errorf("%w: staged cleanup entry %q has file type %#o", ErrNonRegular, state.name, state.fileType)
		}
	}
	return nil
}

func populateDirectorySnapshot(directoryFD int, node *directorySnapshotNode) error {
	for _, entry := range node.entries {
		if entry.dir != nil {
			if err := unix.Mkdirat(directoryFD, entry.name, directoryMode); err != nil {
				return fmt.Errorf("create staged child directory %q: %w", entry.name, err)
			}
			state, err := statDirectoryEntryAt(directoryFD, entry.name)
			if err != nil {
				return fmt.Errorf("inspect staged child directory %q: %w", entry.name, err)
			}
			childFD, err := openDirectoryAt(directoryFD, entry.name)
			if err != nil {
				return fmt.Errorf("open staged child directory %q: %w", entry.name, err)
			}
			openedIdentity, openedType, _, inspectErr := directoryDescriptorState(childFD)
			if inspectErr != nil || openedType != unix.S_IFDIR || openedIdentity != state.identity {
				_ = unix.Close(childFD)
				if inspectErr != nil {
					return fmt.Errorf("inspect opened staged child directory %q: %w", entry.name, inspectErr)
				}
				return fmt.Errorf("%w: staged child directory %q changed before open", ErrStagedChanged, entry.name)
			}
			if err := populateDirectorySnapshot(childFD, entry.dir); err != nil {
				_ = unix.Close(childFD)
				return err
			}
			if err := unix.Close(childFD); err != nil {
				return fmt.Errorf("close staged child directory %q: %w", entry.name, err)
			}
			continue
		}

		fd, err := unix.Openat(directoryFD, entry.name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if err != nil {
			return fmt.Errorf("create staged file %q: %w", entry.name, err)
		}
		identity, err := identityOf(fd)
		if err == nil {
			err = prepareStagedFile(fd, entry.data, uint32(entry.mode.Perm()))
		}
		closeErr := closeStaged(fd)
		if err != nil {
			return fmt.Errorf("prepare staged file %q: %w", entry.name, errors.Join(err, closeErr))
		}
		if closeErr != nil {
			return fmt.Errorf("close staged file %q: %w", entry.name, closeErr)
		}
		if err := verifyStagedEntry(directoryFD, entry.name, identity); err != nil {
			return err
		}
	}
	if err := unix.Fchmod(directoryFD, uint32(node.mode.Perm())); err != nil {
		return fmt.Errorf("set staged directory mode: %w", err)
	}
	if err := syncDirectory(directoryFD, "staged directory"); err != nil {
		return err
	}
	return nil
}

func verifyDirectoryEntry(parentFD int, name string, expected fileIdentity) error {
	actual, fileType, err := identityAt(parentFD, name)
	if err != nil {
		return fmt.Errorf("%w: inspect directory entry %q: %w", ErrDirectoryChanged, name, err)
	}
	if fileType != unix.S_IFDIR || actual != expected {
		return fmt.Errorf("%w: directory entry %q no longer names the expected directory", ErrDirectoryChanged, name)
	}
	return nil
}

func directoryChangedError(detail string, cause error) error {
	if cause != nil {
		return fmt.Errorf("%w: %s: %w", ErrDirectoryChanged, detail, cause)
	}
	return fmt.Errorf("%w: %s", ErrDirectoryChanged, detail)
}

func unusedDirectoryName(parentFD int, prefix string) (string, error) {
	for attempt := 0; attempt < tempAttempts; attempt++ {
		suffix, err := temporarySuffix()
		if err != nil {
			return "", fmt.Errorf("generate directory recovery name: %w", err)
		}
		name := prefix + suffix
		if _, _, err := identityAt(parentFD, name); errors.Is(err, unix.ENOENT) {
			return name, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect directory recovery name %q: %w", name, err)
		}
	}
	return "", fmt.Errorf("find unused directory recovery name: exhausted %d randomized names", tempAttempts)
}

func rollbackDirectoryName(parentFD int, target, recovery string, expected fileIdentity, rootFD int, directories []string, wantedParent fileIdentity) (bool, error) {
	if err := verifyDirectoryEntry(parentFD, recovery, expected); err != nil {
		return false, err
	}
	if _, _, err := identityAt(parentFD, target); !errors.Is(err, unix.ENOENT) {
		if err == nil {
			err = fmt.Errorf("target exists")
		}
		return false, fmt.Errorf("cannot recover %q while target is occupied: %w", target, err)
	}
	if err := unix.Renameat(parentFD, recovery, parentFD, target); err != nil {
		return false, fmt.Errorf("restore recovery directory name: %w", err)
	}
	restored := true
	var errs []error
	if err := syncDirectory(parentFD, "directory rollback parent"); err != nil {
		errs = append(errs, err)
	}
	if err := verifyDirectoryEntry(parentFD, target, expected); err != nil {
		errs = append(errs, err)
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		errs = append(errs, err)
	}
	return restored, errors.Join(errs...)
}

func removeDirectoryEntryAt(parentFD int, name string, expected fileIdentity) error {
	if err := verifyDirectoryEntry(parentFD, name, expected); err != nil {
		return err
	}
	fd, err := openDirectoryAt(parentFD, name)
	if err != nil {
		return fmt.Errorf("open directory %q for recursive removal: %w", name, err)
	}
	openedIdentity, openedType, _, inspectErr := directoryDescriptorState(fd)
	if inspectErr != nil || openedType != unix.S_IFDIR || openedIdentity != expected {
		_ = unix.Close(fd)
		if inspectErr != nil {
			return fmt.Errorf("inspect directory %q for recursive removal: %w", name, inspectErr)
		}
		return fmt.Errorf("%w: directory %q changed before recursive removal", ErrDirectoryChanged, name)
	}
	if err := removeDirectoryContents(fd); err != nil {
		_ = unix.Close(fd)
		return err
	}
	if err := unix.Close(fd); err != nil {
		return fmt.Errorf("close directory %q before removal: %w", name, err)
	}
	if err := verifyDirectoryEntry(parentFD, name, expected); err != nil {
		return err
	}
	if hook := directoryTestHooks.beforeRemoveEntry; hook != nil {
		if err := hook(parentFD, name); err != nil {
			return fmt.Errorf("before removing directory entry %q: %w", name, err)
		}
	}
	if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil {
		return fmt.Errorf("remove directory entry %q: %w", name, err)
	}
	if err := syncDirectory(parentFD, "recursive directory parent"); err != nil {
		return err
	}
	return nil
}

func removeDirectoryContents(directoryFD int) error {
	states, err := listDirectoryStates(directoryFD)
	if err != nil {
		return err
	}
	for _, state := range states {
		switch state.fileType {
		case unix.S_IFLNK:
			return fmt.Errorf("%w: recursive removal entry %q", ErrSymlink, state.name)
		case unix.S_IFREG:
			actual, fileType, err := identityAt(directoryFD, state.name)
			if err != nil || fileType != unix.S_IFREG || actual != state.identity {
				return fmt.Errorf("%w: recursive removal file %q changed", ErrDirectoryChanged, state.name)
			}
			if hook := directoryTestHooks.beforeRemoveEntry; hook != nil {
				if err := hook(directoryFD, state.name); err != nil {
					return fmt.Errorf("before removing file entry %q: %w", state.name, err)
				}
			}
			if err := unix.Unlinkat(directoryFD, state.name, 0); err != nil {
				return fmt.Errorf("remove file entry %q: %w", state.name, err)
			}
		case unix.S_IFDIR:
			if err := removeDirectoryEntryAt(directoryFD, state.name, state.identity); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: recursive removal entry %q has file type %#o", ErrNonRegular, state.name, state.fileType)
		}
	}
	if err := syncDirectory(directoryFD, "recursive directory contents"); err != nil {
		return err
	}
	return nil
}
