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
	links      uint64
	uid        uint32
	gid        uint32
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
	return readWithin(root, rel, nil)
}

// ReadWithinLimit is ReadWithin with an explicit allocation and byte limit.
// Stable oversized files return ErrSizeLimit without source bytes or revision.
func ReadWithinLimit(root, rel string, limit int64) ([]byte, Revision, error) {
	if limit < 0 {
		return nil, Revision{}, fmt.Errorf("%w: negative limit %d", ErrSizeLimit, limit)
	}
	return readWithin(root, rel, &limit)
}

func readWithin(root, rel string, limit *int64) ([]byte, Revision, error) {
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
	return readLeafWithinParent(parentFD, target, limit)
}

func readLeafWithinParent(parentFD int, target string, limit *int64) ([]byte, Revision, error) {
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
	file := os.NewFile(uintptr(fd), target) // #nosec G115 -- Successful open/openat returns a nonnegative native file descriptor.
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
	if !restorableOwner(before.uid, before.gid, os.Geteuid(), os.Getegid()) {
		_ = file.Close()
		return nil, Revision{}, fmt.Errorf("%w: target %q owner %d:%d cannot be restored by %d:%d", ErrRevisionChanged, target, before.uid, before.gid, os.Geteuid(), os.Getegid())
	}
	if hook := replaceTestHooks.afterReadOpen; hook != nil {
		if err := hook(parentFD, fd, target); err != nil {
			_ = file.Close()
			return nil, Revision{}, fmt.Errorf("after opening target: %w", err)
		}
	}
	if limit != nil && before.size > *limit {
		after, inspectErr := snapshotDescriptor(fd, file)
		if inspectErr != nil {
			_ = file.Close()
			return nil, Revision{}, fmt.Errorf("inspect oversized target %q: %w", target, inspectErr)
		}
		if before != after {
			_ = file.Close()
			return nil, Revision{}, fmt.Errorf("%w: target %q metadata changed", ErrRevisionChanged, target)
		}
		closeErr := file.Close()
		sizeErr := fmt.Errorf("%w: target %q is %d bytes, limit %d", ErrSizeLimit, target, before.size, *limit)
		if closeErr != nil {
			return nil, Revision{}, errors.Join(sizeErr, fmt.Errorf("close target %q: %w", target, closeErr))
		}
		return nil, Revision{}, sizeErr
	}

	var reader io.Reader = file
	if limit != nil && *limit < int64(^uint64(0)>>1) {
		reader = io.LimitReader(file, *limit+1)
	}
	data, err := io.ReadAll(reader)
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
	if limit != nil && int64(len(data)) > *limit {
		_ = file.Close()
		return nil, Revision{}, fmt.Errorf("%w: target %q exceeds limit %d", ErrSizeLimit, target, *limit)
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
		links:      after.links,
		uid:        after.uid,
		gid:        after.gid,
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
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return descriptorSnapshot{}, err
	}
	return descriptorSnapshot{
		identity:   identity,
		mode:       uint32(info.Mode()),
		links:      uint64(stat.Nlink),
		uid:        stat.Uid,
		gid:        stat.Gid,
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
	return removeWithin(root, rel, nil, nil)
}

// RemoveWithinRevision removes rel only if the opened regular file still has
// the exact descriptor-anchored Revision observed by ReadWithin. The expected
// inode, link count, permissions, size, modification time, and content digest
// are revalidated inside the removal operation immediately before unlink.
func RemoveWithinRevision(root, rel string, expected Revision) error {
	if !expected.Tracked() || !expected.Exists() {
		return fmt.Errorf("%w: expected removal revision must describe an existing file", ErrRevisionChanged)
	}
	return removeWithin(root, rel, &expected, nil)
}

// RemoveWithinRevisionAuthorized removes the exact accepted file only while
// the bound root-to-parent namespace chain still matches.
func RemoveWithinRevisionAuthorized(root, rel string, expected Revision, parents *ParentChain) error {
	if !expected.Tracked() || !expected.Exists() {
		return fmt.Errorf("%w: expected removal revision must describe an existing file", ErrRevisionChanged)
	}
	if !parents.Tracked() {
		return fmt.Errorf("%w: expected parent chain is untracked", ErrParentChanged)
	}
	return removeWithin(root, rel, &expected, parents)
}

func removeWithin(root, rel string, expected *Revision, parents *ParentChain) (returnErr error) {
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

	var fd int
	if expected != nil {
		fd, err = unix.Openat(parentFD, target, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	} else {
		fd, err = openRemovalTarget(parentFD, target)
	}
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
	if expected != nil {
		if !descriptorHeld {
			return fmt.Errorf("%w: removal target %q could not be held for revision verification", ErrRevisionChanged, target)
		}
		if err := verifyDescriptorRevision(fd, *expected); err != nil {
			return fmt.Errorf("verify removal revision for %q: %w", target, err)
		}
	}
	if hook := replaceTestHooks.beforeRemove; hook != nil {
		if err := hook(parentFD, fd, target); err != nil {
			return fmt.Errorf("before removing %q: %w", target, err)
		}
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		return fmt.Errorf("pre-remove parent verification: %w", err)
	}
	if err := verifyRemovalEntry(parentFD, target, wantedTarget); err != nil {
		return err
	}
	if expected != nil {
		if err := verifyDescriptorRevision(fd, *expected); err != nil {
			return fmt.Errorf("final removal revision for %q: %w", target, err)
		}
		// Recheck the namespace after reading the held descriptor so a swap at
		// the revision-check boundary cannot redirect unlink to a replacement.
		if err := verifyRemovalEntry(parentFD, target, wantedTarget); err != nil {
			return err
		}
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
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		postRemove = append(postRemove, err)
		operations = append(operations, "post-remove parent verification")
	}
	if len(postRemove) != 0 {
		return &CommittedError{Operation: strings.Join(operations, "; "), Err: errors.Join(postRemove...)}
	}
	return nil
}

func verifyDescriptorRevision(fd int, expected Revision) error {
	if !restorableOwner(expected.uid, expected.gid, os.Geteuid(), os.Getegid()) {
		return fmt.Errorf("%w: expected file owner is not restorable", ErrRevisionChanged)
	}
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return fmt.Errorf("%w: inspect held descriptor: %w", ErrRevisionChanged, err)
	}
	if uint32(before.Mode)&unix.S_IFMT != unix.S_IFREG ||
		identityFromStat(&before) != (fileIdentity{device: expected.device, inode: expected.inode}) ||
		uint64(before.Nlink) != expected.links ||
		before.Uid != expected.uid || before.Gid != expected.gid ||
		uint32(before.Mode)&0o777 != expected.mode&0o777 ||
		before.Size != expected.size || statModifiedNanoseconds(&before) != expected.modifiedNS {
		return ErrRevisionChanged
	}

	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	var offset int64
	for offset < expected.size {
		want := int64(len(buffer))
		if remaining := expected.size - offset; remaining < want {
			want = remaining
		}
		read, err := unix.Pread(fd, buffer[:int(want)], offset)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: read held descriptor: %w", ErrRevisionChanged, err)
		}
		if read == 0 {
			return ErrRevisionChanged
		}
		_, _ = hash.Write(buffer[:read])
		offset += int64(read)
	}
	var extra [1]byte
	if read, err := unix.Pread(fd, extra[:], expected.size); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: verify held descriptor length: %w", ErrRevisionChanged, err)
	} else if read != 0 {
		return ErrRevisionChanged
	}
	var actualDigest [sha256.Size]byte
	copy(actualDigest[:], hash.Sum(nil))
	if actualDigest != expected.digest {
		return ErrRevisionChanged
	}

	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return fmt.Errorf("%w: reinspect held descriptor: %w", ErrRevisionChanged, err)
	}
	if identityFromStat(&after) != identityFromStat(&before) || after.Nlink != before.Nlink ||
		after.Mode != before.Mode || after.Uid != before.Uid || after.Gid != before.Gid || after.Size != before.Size ||
		statModifiedNanoseconds(&after) != statModifiedNanoseconds(&before) {
		return ErrRevisionChanged
	}
	return nil
}

func statModifiedNanoseconds(stat *unix.Stat_t) int64 {
	return stat.Mtim.Sec*1_000_000_000 + stat.Mtim.Nsec
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
		return fmt.Errorf("%w: inspect removal target %q: %w", ErrTargetChanged, target, err)
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
	return acquireLockWithin(root, rel, mode, nil)
}

// AcquireLockWithinAuthorized binds the complete lock namespace chain both at
// acquisition and release, preventing a replaced private lock directory from
// splitting cooperating writers across different lock inodes.
func AcquireLockWithinAuthorized(root, rel string, mode fs.FileMode, parents *ParentChain) (func() error, error) {
	if !parents.Tracked() {
		return nil, fmt.Errorf("%w: lock parent authority is untracked", ErrParentChanged)
	}
	return acquireLockWithin(root, rel, mode, parents)
}

func acquireLockWithin(root, rel string, mode fs.FileMode, parents *ParentChain) (func() error, error) {
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
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
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
			var namespaceErr error
			releaseRootFD, openErr := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
			if openErr != nil {
				namespaceErr = fmt.Errorf("open lock root before release: %w", openErr)
			} else {
				var releaseParentFD int
				if parents != nil {
					releaseParentFD, openErr = openAuthorizedParent(releaseRootFD, directories, parents)
				} else {
					releaseParentFD, openErr = openParent(releaseRootFD, directories, false)
				}
				if openErr != nil {
					namespaceErr = fmt.Errorf("open lock parent before release: %w", openErr)
				} else {
					namespaceErr = errors.Join(
						verifyLockDescriptor(fd, wantedLock, uint32(mode.Perm())),
						verifyLockEntry(releaseParentFD, target, wantedLock, uint32(mode.Perm())),
					)
					_ = unix.Close(releaseParentFD)
				}
				_ = unix.Close(releaseRootFD)
			}
			unlockErr := unix.Flock(fd, unix.LOCK_UN)
			closeErr := unix.Close(fd)
			if unlockErr != nil {
				unlockErr = fmt.Errorf("unlock %q: %w", target, unlockErr)
			}
			if closeErr != nil {
				closeErr = fmt.Errorf("close lock %q: %w", target, closeErr)
			}
			releaseErr = errors.Join(namespaceErr, unlockErr, closeErr)
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
		return fmt.Errorf("%w: inspect lock entry %q: %w", ErrLockChanged, target, err)
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
