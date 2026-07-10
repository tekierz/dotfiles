//go:build darwin || linux

package safefile

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

type descriptorSnapshot struct {
	identity   fileIdentity
	mode       uint32
	size       int64
	modifiedNS int64
}

// ReadWithin reads rel below one trusted root descriptor without following any
// symlink in rel. An absent leaf (including an absent descendant parent) returns
// nil bytes, a tracked non-existing Revision, and nil error. Existing leaves
// must be regular files.
//
// File identity and bytes are collected through one held descriptor. If its
// observable metadata changes during the read, ErrRevisionChanged is returned
// instead of a revision for potentially torn data.
func ReadWithin(root, rel string) ([]byte, Revision, error) {
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return nil, Revision{}, err
	}

	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, Revision{}, fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()

	parentFD, err := openParent(rootFD, directories, false)
	if errors.Is(err, unix.ENOENT) {
		return nil, missingRevision(), nil
	}
	if err != nil {
		return nil, Revision{}, err
	}
	defer func() { _ = unix.Close(parentFD) }()

	_, fileType, err := identityAt(parentFD, target)
	if errors.Is(err, unix.ENOENT) {
		return nil, missingRevision(), nil
	}
	if err != nil {
		return nil, Revision{}, fmt.Errorf("inspect target %q: %w", target, err)
	}
	if fileType == unix.S_IFLNK {
		return nil, Revision{}, fmt.Errorf("%w: target %q", ErrSymlink, target)
	}
	if fileType != unix.S_IFREG {
		return nil, Revision{}, fmt.Errorf("%w: target %q has file type %#o", ErrNonRegular, target, fileType)
	}

	fd, err := unix.Openat(parentFD, target, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil, missingRevision(), nil
	}
	if err != nil {
		return nil, Revision{}, classifyLeafOpenError(parentFD, target, err)
	}
	file := os.NewFile(uintptr(fd), target)
	if file == nil {
		_ = unix.Close(fd)
		return nil, Revision{}, fmt.Errorf("wrap target descriptor")
	}

	before, err := snapshotDescriptor(fd, file)
	if err != nil {
		_ = file.Close()
		return nil, Revision{}, fmt.Errorf("inspect opened target %q: %w", target, err)
	}
	if fs.FileMode(before.mode).Type() != 0 {
		_ = file.Close()
		return nil, Revision{}, fmt.Errorf("%w: opened target %q is not regular", ErrNonRegular, target)
	}
	if hook := replaceTestHooks.afterReadOpen; hook != nil {
		if err := hook(parentFD, fd, target); err != nil {
			_ = file.Close()
			return nil, Revision{}, fmt.Errorf("after opening target: %w", err)
		}
	}

	data, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		return nil, Revision{}, fmt.Errorf("read target %q: %w", target, err)
	}
	after, err := snapshotDescriptor(fd, file)
	if err != nil {
		_ = file.Close()
		return nil, Revision{}, fmt.Errorf("inspect read target %q: %w", target, err)
	}
	if before != after || int64(len(data)) != after.size {
		_ = file.Close()
		return nil, Revision{}, fmt.Errorf("%w: target %q metadata changed", ErrRevisionChanged, target)
	}
	if err := file.Close(); err != nil {
		return nil, Revision{}, fmt.Errorf("close target %q: %w", target, err)
	}

	return data, Revision{
		tracked:    true,
		exists:     true,
		device:     after.identity.device,
		inode:      after.identity.inode,
		mode:       after.mode,
		size:       after.size,
		modifiedNS: after.modifiedNS,
		digest:     sha256.Sum256(data),
	}, nil
}

func missingRevision() Revision {
	return Revision{tracked: true, exists: false}
}

func snapshotDescriptor(fd int, file *os.File) (descriptorSnapshot, error) {
	identity, err := identityOf(fd)
	if err != nil {
		return descriptorSnapshot{}, err
	}
	info, err := file.Stat()
	if err != nil {
		return descriptorSnapshot{}, err
	}
	return descriptorSnapshot{
		identity:   identity,
		mode:       uint32(info.Mode()),
		size:       info.Size(),
		modifiedNS: info.ModTime().UnixNano(),
	}, nil
}

func classifyLeafOpenError(parentFD int, target string, openErr error) error {
	_, fileType, statErr := identityAt(parentFD, target)
	if statErr == nil {
		switch fileType {
		case unix.S_IFLNK:
			return fmt.Errorf("%w: target %q", ErrSymlink, target)
		case unix.S_IFREG:
			// Preserve the useful syscall error for a regular file.
		default:
			return fmt.Errorf("%w: target %q has file type %#o", ErrNonRegular, target, fileType)
		}
	}
	return fmt.Errorf("open target %q: %w", target, openErr)
}

// RemoveWithin removes one regular file at rel below a trusted root without
// following symlinks in rel. Descendant parents and the leaf must already
// exist. Directories, symlinks, devices, sockets, and FIFOs are refused.
//
// When permissions allow, the target is held open while its parent and namespace
// identity are checked immediately before unlinkat. Darwin cannot obtain an
// identity-only descriptor for every unreadable regular file, so that case uses
// two descriptor-relative namespace identity checks instead. Portable APIs cannot
// make the final identity check and unlink indivisible, so callers must keep the
// trusted root private from malicious same-UID mutation. Failures discovered
// after unlink are returned as *CommittedError because the requested name was
// removed.
func RemoveWithin(root, rel string) (returnErr error) {
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
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(parentFD) }()
	wantedParent, err := identityOf(parentFD)
	if err != nil {
		return fmt.Errorf("identify removal parent: %w", err)
	}

	inspectedTarget, inspectedType, err := identityAt(parentFD, target)
	if err != nil {
		return fmt.Errorf("inspect removal target %q: %w", target, err)
	}
	switch inspectedType {
	case unix.S_IFLNK:
		return fmt.Errorf("%w: removal target %q", ErrSymlink, target)
	case unix.S_IFREG:
	default:
		return fmt.Errorf("%w: removal target %q has file type %#o", ErrNonRegular, target, inspectedType)
	}

	fd, err := openRemovalTarget(parentFD, target)
	descriptorHeld := err == nil
	if err != nil && !errors.Is(err, unix.EACCES) && !errors.Is(err, unix.EPERM) {
		return classifyRemoveOpenError(parentFD, target, err)
	}
	if !descriptorHeld {
		fd = -1
	}
	closed := !descriptorHeld
	defer func() {
		if !closed {
			_ = unix.Close(fd)
		}
	}()

	wantedTarget := inspectedTarget
	if descriptorHeld {
		openedTarget, fileType, err := descriptorIdentityAndType(fd)
		if err != nil {
			return fmt.Errorf("inspect removal target %q: %w", target, err)
		}
		if fileType != unix.S_IFREG || openedTarget != inspectedTarget {
			return fmt.Errorf("%w: removal target %q changed between inspection and open", ErrTargetChanged, target)
		}
		wantedTarget = openedTarget
	}
	if hook := replaceTestHooks.beforeRemove; hook != nil {
		if err := hook(parentFD, fd, target); err != nil {
			return fmt.Errorf("before removing %q: %w", target, err)
		}
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		return fmt.Errorf("pre-remove parent verification: %w", err)
	}
	if err := verifyRemovalEntry(parentFD, target, wantedTarget); err != nil {
		return err
	}

	if err := unix.Unlinkat(parentFD, target, 0); err != nil {
		return fmt.Errorf("remove target %q: %w", target, err)
	}
	var postRemove []error
	var operations []string
	if hook := replaceTestHooks.afterRemove; hook != nil {
		if err := hook(parentFD, fd, target); err != nil {
			postRemove = append(postRemove, fmt.Errorf("post-remove boundary: %w", err))
			operations = append(operations, "post-remove hook")
		}
	}
	if _, _, err := identityAt(parentFD, target); err == nil {
		postRemove = append(postRemove, fmt.Errorf("%w: target %q was recreated after removal", ErrTargetChanged, target))
		operations = append(operations, "removed entry verification")
	} else if !errors.Is(err, unix.ENOENT) {
		postRemove = append(postRemove, fmt.Errorf("verify removed target %q: %w", target, err))
		operations = append(operations, "removed entry verification")
	}
	if descriptorHeld {
		closed = true
		if err := closeRemovedTarget(fd); err != nil {
			postRemove = append(postRemove, fmt.Errorf("close removed target %q: %w", target, err))
			operations = append(operations, "removed target close")
		}
	}
	if err := syncDirectory(parentFD, "remove parent"); err != nil {
		postRemove = append(postRemove, err)
		operations = append(operations, "parent fsync")
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		postRemove = append(postRemove, err)
		operations = append(operations, "post-remove parent verification")
	}
	if len(postRemove) != 0 {
		return &CommittedError{Operation: strings.Join(operations, "; "), Err: errors.Join(postRemove...)}
	}
	return nil
}

func closeRemovedTarget(fd int) error {
	if hook := replaceTestHooks.closeRemoved; hook != nil {
		return hook(fd)
	}
	return unix.Close(fd)
}

func classifyRemoveOpenError(parentFD int, target string, openErr error) error {
	_, fileType, statErr := identityAt(parentFD, target)
	if statErr == nil {
		switch fileType {
		case unix.S_IFLNK:
			return fmt.Errorf("%w: removal target %q", ErrSymlink, target)
		case unix.S_IFREG:
		default:
			return fmt.Errorf("%w: removal target %q has file type %#o", ErrNonRegular, target, fileType)
		}
	}
	return fmt.Errorf("open removal target %q: %w", target, openErr)
}

func descriptorIdentityAndType(fd int) (fileIdentity, uint32, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fileIdentity{}, 0, err
	}
	return identityFromStat(&stat), uint32(stat.Mode) & unix.S_IFMT, nil
}

func verifyRemovalEntry(parentFD int, target string, expected fileIdentity) error {
	actual, fileType, err := identityAt(parentFD, target)
	if err != nil {
		return fmt.Errorf("%w: inspect removal target %q: %v", ErrTargetChanged, target, err)
	}
	if fileType != unix.S_IFREG || actual != expected {
		return fmt.Errorf("%w: removal target %q no longer names opened regular file", ErrTargetChanged, target)
	}
	return nil
}

// AcquireLockWithin opens or creates a regular, single-link lock file at rel
// below root and takes an exclusive advisory flock. Descendant parents must
// already exist. Root is a trusted anchor and may itself be a symlink.
//
// The returned release function is safe for concurrent/repeated calls. It
// performs unlock and close at most once and returns that same result forever.
// Parent and lock namespace identities are revalidated after flock. As with any
// path-based advisory lock, a malicious same-UID process can replace the lock
// entry after AcquireLockWithin returns; keep the trusted root private.
func AcquireLockWithin(root, rel string, mode fs.FileMode) (func() error, error) {
	if mode != mode.Perm() {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMode, mode)
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

	parentFD, err := openParent(rootFD, directories, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(parentFD) }()
	wantedParent, err := identityOf(parentFD)
	if err != nil {
		return nil, fmt.Errorf("identify lock parent: %w", err)
	}

	fd, created, err := openLockAt(parentFD, target, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	closeLock := true
	defer func() {
		if closeLock {
			_ = unix.Close(fd)
		}
	}()

	wantedLock, err := prepareLockDescriptor(fd, target, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	if err := unix.Fsync(fd); err != nil {
		return nil, fmt.Errorf("fsync lock file %q: %w", target, err)
	}
	if created {
		if err := syncDirectory(parentFD, "lock parent"); err != nil {
			return nil, fmt.Errorf("fsync lock parent: %w", err)
		}
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		return nil, fmt.Errorf("flock %q: %w", target, err)
	}
	locked := true
	defer func() {
		if locked {
			_ = unix.Flock(fd, unix.LOCK_UN)
		}
	}()

	if hook := replaceTestHooks.afterLock; hook != nil {
		if err := hook(parentFD, fd, target); err != nil {
			return nil, fmt.Errorf("after locking %q: %w", target, err)
		}
	}
	if err := verifyParent(rootFD, directories, wantedParent); err != nil {
		return nil, fmt.Errorf("lock parent verification: %w", err)
	}
	if err := verifyLockDescriptor(fd, wantedLock, uint32(mode.Perm())); err != nil {
		return nil, err
	}
	if err := verifyLockEntry(parentFD, target, wantedLock, uint32(mode.Perm())); err != nil {
		return nil, err
	}

	locked = false
	closeLock = false
	var once sync.Once
	var releaseErr error
	release := func() error {
		once.Do(func() {
			unlockErr := unix.Flock(fd, unix.LOCK_UN)
			closeErr := unix.Close(fd)
			if unlockErr != nil {
				unlockErr = fmt.Errorf("unlock %q: %w", target, unlockErr)
			}
			if closeErr != nil {
				closeErr = fmt.Errorf("close lock %q: %w", target, closeErr)
			}
			releaseErr = errors.Join(unlockErr, closeErr)
		})
		return releaseErr
	}
	return release, nil
}

func openLockAt(parentFD int, target string, mode uint32) (int, bool, error) {
	flags := unix.O_RDWR | unix.O_NONBLOCK | unix.O_NOFOLLOW | unix.O_CLOEXEC
	fd, err := unix.Openat(parentFD, target, flags|unix.O_CREAT|unix.O_EXCL, mode)
	if err == nil {
		return fd, true, nil
	}
	if !errors.Is(err, unix.EEXIST) {
		return -1, false, classifyLockOpenError(parentFD, target, err)
	}
	fd, err = unix.Openat(parentFD, target, flags, 0)
	if err != nil {
		return -1, false, classifyLockOpenError(parentFD, target, err)
	}
	return fd, false, nil
}

func classifyLockOpenError(parentFD int, target string, openErr error) error {
	_, fileType, statErr := identityAt(parentFD, target)
	if statErr == nil {
		switch fileType {
		case unix.S_IFLNK:
			return fmt.Errorf("%w: lock %q", ErrSymlink, target)
		case unix.S_IFREG:
		default:
			return fmt.Errorf("%w: lock %q has file type %#o", ErrNonRegular, target, fileType)
		}
	}
	return fmt.Errorf("open lock %q: %w", target, openErr)
}

func prepareLockDescriptor(fd int, target string, mode uint32) (fileIdentity, error) {
	identity, fileType, links, _, err := lockDescriptorState(fd)
	if err != nil {
		return fileIdentity{}, fmt.Errorf("inspect lock %q: %w", target, err)
	}
	if fileType != unix.S_IFREG {
		return fileIdentity{}, fmt.Errorf("%w: lock %q is not regular", ErrNonRegular, target)
	}
	if links == 0 {
		return fileIdentity{}, fmt.Errorf("%w: lock %q was unlinked before acquisition", ErrLockChanged, target)
	}
	if links > 1 {
		return fileIdentity{}, fmt.Errorf("%w: lock %q has %d links", ErrHardlink, target, links)
	}
	if err := unix.Fchmod(fd, mode); err != nil {
		return fileIdentity{}, fmt.Errorf("chmod lock %q: %w", target, err)
	}
	if err := verifyLockDescriptor(fd, identity, mode); err != nil {
		return fileIdentity{}, err
	}
	return identity, nil
}

func verifyLockDescriptor(fd int, expected fileIdentity, mode uint32) error {
	identity, fileType, links, permissions, err := lockDescriptorState(fd)
	if err != nil {
		return fmt.Errorf("inspect locked descriptor: %w", err)
	}
	if fileType != unix.S_IFREG || identity != expected {
		return fmt.Errorf("%w: locked descriptor identity/type changed", ErrLockChanged)
	}
	if links == 0 {
		return fmt.Errorf("%w: locked descriptor was unlinked", ErrLockChanged)
	}
	if links > 1 {
		return fmt.Errorf("%w: locked descriptor has %d links", ErrHardlink, links)
	}
	if permissions != mode {
		return fmt.Errorf("%w: lock mode is %04o, want %04o", ErrLockChanged, permissions, mode)
	}
	return nil
}

func verifyLockEntry(parentFD int, target string, expected fileIdentity, mode uint32) error {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, target, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("%w: inspect lock entry %q: %v", ErrLockChanged, target, err)
	}
	identity := identityFromStat(&stat)
	if uint32(stat.Mode)&unix.S_IFMT != unix.S_IFREG || identity != expected {
		return fmt.Errorf("%w: lock entry %q no longer names locked inode", ErrLockChanged, target)
	}
	if uint64(stat.Nlink) != 1 {
		return fmt.Errorf("%w: lock entry %q has %d links", ErrHardlink, target, stat.Nlink)
	}
	if uint32(stat.Mode)&0o7777 != mode {
		return fmt.Errorf("%w: lock entry %q mode is %04o, want %04o", ErrLockChanged, target, uint32(stat.Mode)&0o7777, mode)
	}
	return nil
}

func lockDescriptorState(fd int) (fileIdentity, uint32, uint64, uint32, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fileIdentity{}, 0, 0, 0, err
	}
	return identityFromStat(&stat), uint32(stat.Mode) & unix.S_IFMT, uint64(stat.Nlink), uint32(stat.Mode) & 0o7777, nil
}
