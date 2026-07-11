//go:build darwin || linux

package safefile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	directoryMode = 0o700
	tempAttempts  = 32
)

type fileIdentity struct {
	device uint64
	inode  uint64
}

type stagedFile struct {
	name     string
	fd       int
	identity fileIdentity
}

// replaceHooks is intentionally package-private. Tests can place actions at
// security-sensitive boundaries without expanding the production API.
type replaceHooks struct {
	randomSuffix  func() (string, error)
	afterMkdir    func(parentFD int, name string) error
	afterReadOpen func(parentFD, fileFD int, name string) error
	afterLock     func(parentFD, lockFD int, name string) error
	beforeRemove  func(parentFD, targetFD int, name string) error
	afterRemove   func(parentFD, targetFD int, name string) error
	closeRemoved  func(fd int) error
	beforeCommit  func(parentFD, stagedFD int, temporary, target string) error
	afterCommit   func(parentFD, stagedFD int, target string) error
	fsyncStaged   func(fd int) error
	closeStaged   func(fd int) error
	fsyncDir      func(fd int, operation string) error
	unlinkTemp    func(parentFD int, name string) error
}

var replaceTestHooks replaceHooks

// EnsureDirectoryWithin creates rel, including any missing parents, below the
// trusted root without following symlinks in rel. Newly created directories
// have exact mode 0700 and are made durable before the function returns;
// existing directory modes are left unchanged.
//
// The mode argument is intentionally restricted to 0700 because openParent is
// also the directory-creation primitive used by ReplaceWithin. Keeping one
// private-directory policy prevents callers from accidentally establishing a
// writable trusted-root descendant.
func EnsureDirectoryWithin(root, rel string, mode fs.FileMode) error {
	if mode != directoryMode {
		return fmt.Errorf("%w: directory mode must be %04o, got %v", ErrInvalidMode, directoryMode, mode)
	}
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return err
	}
	components := append(append([]string(nil), directories...), target)

	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }() // Closing a read-only directory cannot change the result.

	directoryFD, err := openParent(rootFD, components, true)
	if err != nil {
		return err
	}
	return unix.Close(directoryFD)
}

// ReplaceWithin atomically replaces rel below root without following symlinks
// in rel. Root is a trusted anchor and is opened once, so root itself may be a
// legitimate symlink. Missing descendant directories are created with mode
// 0700; existing directory modes are not changed.
//
// The staged descriptor remains open across rename. Its inode is compared with
// the temporary namespace entry immediately before rename and with the target
// entry immediately afterward. Those checks make substitution observable, but
// portable Darwin/Linux APIs cannot make the identity check and rename one
// indivisible operation. A malicious process with the same UID can still race
// the final check/rename boundary or modify the same inode; callers should keep
// the trusted root private (normally mode 0700).
//
// If a failure is found after rename, the returned error is a *CommittedError.
func ReplaceWithin(root, rel string, data []byte, mode fs.FileMode) (returnErr error) {
	return replaceWithinRevision(root, rel, data, mode, nil, nil, true, nil)
}

// ReplaceWithinTracked returns the exact descriptor revision committed by the
// replacement. If the committed target no longer matches the requested bytes
// and mode at the return boundary, it returns a CommittedError and no evidence.
func ReplaceWithinTracked(root, rel string, data []byte, mode fs.FileMode) (Revision, error) {
	var revision Revision
	err := replaceWithinRevision(root, rel, data, mode, nil, &revision, true, nil)
	return revision, err
}

// ReplaceWithinRevision atomically replaces rel only when its exact current
// descriptor revision still matches expected. A tracked missing revision is a
// valid expectation and requires the target to remain absent through the
// staging commit boundary.
func ReplaceWithinRevision(root, rel string, expected Revision, data []byte, mode fs.FileMode) error {
	if !expected.Tracked() {
		return fmt.Errorf("%w: expected replacement revision is untracked", ErrRevisionChanged)
	}
	return replaceWithinRevision(root, rel, data, mode, &expected, nil, true, nil)
}

func ReplaceWithinRevisionTracked(root, rel string, expected Revision, data []byte, mode fs.FileMode) (Revision, error) {
	if !expected.Tracked() {
		return Revision{}, fmt.Errorf("%w: expected replacement revision is untracked", ErrRevisionChanged)
	}
	var revision Revision
	err := replaceWithinRevision(root, rel, data, mode, &expected, &revision, true, nil)
	return revision, err
}

// ReplaceWithinRevisionNoCreateTracked has the same exact revision-CAS and
// evidence semantics as ReplaceWithinRevisionTracked, but refuses to create a
// missing target parent. Reviewed operation plans must use the Authorized
// variant below; this compatibility surface does not bind ancestor identity.
func ReplaceWithinRevisionNoCreateTracked(root, rel string, expected Revision, data []byte, mode fs.FileMode) (Revision, error) {
	if !expected.Tracked() {
		return Revision{}, fmt.Errorf("%w: expected replacement revision is untracked", ErrRevisionChanged)
	}
	var revision Revision
	err := replaceWithinRevision(root, rel, data, mode, &expected, &revision, false, nil)
	return revision, err
}

// ReplaceWithinRevisionNoCreateAuthorizedTracked additionally requires the
// complete root-to-parent identity chain accepted by the operation plan. The
// chain is re-opened and checked immediately before and after commit.
func ReplaceWithinRevisionNoCreateAuthorizedTracked(root, rel string, expected Revision, parents *ParentChain, data []byte, mode fs.FileMode) (Revision, error) {
	if !expected.Tracked() {
		return Revision{}, fmt.Errorf("%w: expected replacement revision is untracked", ErrRevisionChanged)
	}
	if !parents.Tracked() {
		return Revision{}, fmt.Errorf("%w: expected parent chain is untracked", ErrParentChanged)
	}
	var revision Revision
	err := replaceWithinRevision(root, rel, data, mode, &expected, &revision, false, parents)
	return revision, err
}

func replaceWithinRevision(root, rel string, data []byte, mode fs.FileMode, expected *Revision, evidence *Revision, createParents bool, parents *ParentChain) (returnErr error) {
	if mode != mode.Perm() {
		return fmt.Errorf("%w: %v", ErrInvalidMode, mode)
	}
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return err
	}

	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }() // Closing a read-only directory cannot change the result.

	var parentFD int
	if parents != nil {
		parentFD, err = openAuthorizedParent(rootFD, directories, parents)
	} else {
		parentFD, err = openParent(rootFD, directories, createParents)
	}
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(parentFD) }() // See root descriptor close above.

	wantedParent, err := identityOf(parentFD)
	if err != nil {
		return fmt.Errorf("identify target parent: %w", err)
	}
	if err := inspectExistingLeaf(parentFD, target); err != nil {
		return err
	}

	staged, err := stage(parentFD, data, mode)
	if err != nil {
		return err
	}
	temporaryLive := true
	stagedClosed := false
	committed := false
	defer func() {
		var deferredErrs []error
		var operations []string
		if !stagedClosed {
			stagedClosed = true
			if err := closeStaged(staged.fd); err != nil {
				deferredErrs = append(deferredErrs, fmt.Errorf("close staged file: %w", err))
				operations = append(operations, "staged file close")
			}
		}
		if temporaryLive {
			if err := unlinkTemporary(parentFD, staged.name); err != nil {
				deferredErrs = append(deferredErrs, fmt.Errorf("cleanup temporary file %q: %w", staged.name, err))
				operations = append(operations, "temporary cleanup")
			}
		}
		if len(deferredErrs) == 0 {
			return
		}
		joined := errors.Join(append([]error{returnErr}, deferredErrs...)...)
		if committed {
			returnErr = &CommittedError{Operation: strings.Join(operations, "; "), Err: joined}
			return
		}
		returnErr = joined
	}()

	if hook := replaceTestHooks.beforeCommit; hook != nil {
		if err := hook(parentFD, staged.fd, staged.name, target); err != nil {
			return fmt.Errorf("before commit: %w", err)
		}
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		return fmt.Errorf("pre-commit parent verification: %w", err)
	}
	if err := verifyStagedEntry(parentFD, staged.name, staged.identity); err != nil {
		return fmt.Errorf("pre-commit staging verification: %w", err)
	}
	if expected != nil {
		if err := verifyTargetRevisionAtCommit(parentFD, target, *expected); err != nil {
			return fmt.Errorf("replacement revision check: %w", err)
		}
	}

	if err := unix.Renameat(parentFD, staged.name, parentFD, target); err != nil {
		return fmt.Errorf("commit replacement: %w", err)
	}
	temporaryLive = false
	committed = true

	var postCommit []error
	var operations []string
	if hook := replaceTestHooks.afterCommit; hook != nil {
		if err := hook(parentFD, staged.fd, target); err != nil {
			postCommit = append(postCommit, fmt.Errorf("post-commit boundary: %w", err))
			operations = append(operations, "post-commit hook")
		}
	}
	if err := verifyStagedEntry(parentFD, target, staged.identity); err != nil {
		postCommit = append(postCommit, err)
		operations = append(operations, "committed leaf verification")
	}
	stagedClosed = true
	if err := closeStaged(staged.fd); err != nil {
		postCommit = append(postCommit, fmt.Errorf("close staged file: %w", err))
		operations = append(operations, "staged file close")
	}
	if err := syncDirectory(parentFD, "commit parent"); err != nil {
		postCommit = append(postCommit, err)
		operations = append(operations, "parent fsync")
	}
	if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
		postCommit = append(postCommit, err)
		operations = append(operations, "post-commit parent verification")
	}
	if len(postCommit) != 0 {
		return &CommittedError{
			Operation: strings.Join(operations, "; "),
			Err:       errors.Join(postCommit...),
		}
	}
	if evidence != nil {
		committedData, revision, err := ReadWithin(root, rel)
		wantDigest := sha256.Sum256(data)
		if err != nil || !revision.Exists() || revision.device != staged.identity.device || revision.inode != staged.identity.inode || revision.Permissions() != mode.Perm() || revision.Digest() != wantDigest || !bytes.Equal(committedData, data) {
			if err == nil {
				err = ErrRevisionChanged
			}
			return &CommittedError{Operation: "capture committed replacement revision", Err: err}
		}
		if err := verifyMutationParent(root, rootFD, directories, wantedParent, parents); err != nil {
			return &CommittedError{Operation: "post-evidence parent-chain verification", Err: err}
		}
		*evidence = revision
	}
	return nil
}

func verifyTargetRevisionAtCommit(parentFD int, target string, expected Revision) error {
	identity, fileType, err := identityAt(parentFD, target)
	if !expected.Exists() {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: inspect expected-absent target: %w", ErrRevisionChanged, err)
		}
		return fmt.Errorf("%w: expected target %q to remain absent", ErrRevisionChanged, target)
	}
	if err != nil {
		return fmt.Errorf("%w: inspect expected target: %w", ErrRevisionChanged, err)
	}
	if fileType != unix.S_IFREG || identity != (fileIdentity{device: expected.device, inode: expected.inode}) {
		return fmt.Errorf("%w: target %q identity changed", ErrRevisionChanged, target)
	}
	fd, err := unix.Openat(parentFD, target, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("%w: open expected target: %w", ErrRevisionChanged, err)
	}
	defer func() { _ = unix.Close(fd) }()
	if err := verifyDescriptorRevision(fd, expected); err != nil {
		return err
	}
	actual, actualType, err := identityAt(parentFD, target)
	if err != nil {
		return fmt.Errorf("%w: reinspect target %q namespace: %w", ErrRevisionChanged, target, err)
	}
	if actualType != unix.S_IFREG || actual != identity {
		return fmt.Errorf("%w: target %q namespace changed at commit", ErrRevisionChanged, target)
	}
	return nil
}

func splitRelativePath(rel string) ([]string, string, error) {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.ContainsRune(rel, '\x00') {
		return nil, "", fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, "", fmt.Errorf("%w: %q", ErrInvalidPath, rel)
		}
	}
	return parts[:len(parts)-1], parts[len(parts)-1], nil
}

func openParent(rootFD int, directories []string, create bool) (int, error) {
	current, err := unix.Dup(rootFD)
	if err != nil {
		return -1, fmt.Errorf("duplicate trusted root descriptor: %w", err)
	}
	unix.CloseOnExec(current)

	for _, directory := range directories {
		next, err := openDirectoryAt(current, directory)
		created := false
		var createdIdentity fileIdentity
		if errors.Is(err, unix.ENOENT) && create {
			err = unix.Mkdirat(current, directory, directoryMode)
			switch {
			case err == nil:
				created = true
				createdIdentity, _, err = identityAt(current, directory)
				if err != nil {
					_ = unix.Close(current)
					return -1, fmt.Errorf("identify created directory %q: %w", directory, err)
				}
				if hook := replaceTestHooks.afterMkdir; hook != nil {
					if err := hook(current, directory); err != nil {
						_ = unix.Close(current)
						return -1, fmt.Errorf("after creating directory %q: %w", directory, err)
					}
				}
			case errors.Is(err, unix.EEXIST):
				// Another actor created the entry; treat it as existing and
				// never alter its mode.
			default:
				_ = unix.Close(current)
				return -1, fmt.Errorf("create directory %q: %w", directory, err)
			}
			next, err = openDirectoryAt(current, directory)
		}
		if err != nil {
			_ = unix.Close(current)
			return -1, fmt.Errorf("open directory %q: %w", directory, err)
		}
		if created {
			actual, err := identityOf(next)
			if err != nil {
				_ = unix.Close(next)
				_ = unix.Close(current)
				return -1, fmt.Errorf("identify opened directory %q: %w", directory, err)
			}
			if actual != createdIdentity {
				_ = unix.Close(next)
				_ = unix.Close(current)
				return -1, fmt.Errorf("%w: created directory %q was replaced before open", ErrParentChanged, directory)
			}
			var stat unix.Stat_t
			if err := unix.Fstat(next, &stat); err != nil {
				_ = unix.Close(next)
				_ = unix.Close(current)
				return -1, fmt.Errorf("inspect created directory %q: %w", directory, err)
			}
			if stat.Mode&0o7777 != directoryMode {
				_ = unix.Close(next)
				_ = unix.Close(current)
				return -1, fmt.Errorf("created directory %q has mode %04o, want %04o", directory, stat.Mode&0o7777, directoryMode)
			}
			if err := syncDirectory(next, "created directory"); err != nil {
				_ = unix.Close(next)
				_ = unix.Close(current)
				return -1, fmt.Errorf("fsync created directory %q: %w", directory, err)
			}
			if err := syncDirectory(current, "created directory parent"); err != nil {
				_ = unix.Close(next)
				_ = unix.Close(current)
				return -1, fmt.Errorf("fsync parent of created directory %q: %w", directory, err)
			}
		}
		_ = unix.Close(current)
		current = next
	}
	return current, nil
}

func openDirectoryAt(parentFD int, name string) (int, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err == nil {
		return fd, nil
	}
	var stat unix.Stat_t
	if statErr := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); statErr == nil && stat.Mode&unix.S_IFMT == unix.S_IFLNK {
		return -1, fmt.Errorf("%w: %q", ErrSymlink, name)
	}
	return -1, err
}

func inspectExistingLeaf(parentFD int, target string) error {
	_, fileType, err := identityAt(parentFD, target)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect target %q: %w", target, err)
	}
	switch fileType {
	case unix.S_IFREG:
		return nil
	case unix.S_IFLNK:
		return fmt.Errorf("%w: target %q", ErrSymlink, target)
	case unix.S_IFDIR:
		return fmt.Errorf("%w: target %q is a directory", ErrNonRegular, target)
	default:
		return fmt.Errorf("%w: target %q has file type %#o", ErrNonRegular, target, fileType)
	}
}

func stage(parentFD int, data []byte, mode fs.FileMode) (*stagedFile, error) {
	for attempt := 0; attempt < tempAttempts; attempt++ {
		suffix, err := temporarySuffix()
		if err != nil {
			return nil, fmt.Errorf("generate temporary name: %w", err)
		}
		name := ".safefile-" + suffix
		fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create temporary file: %w", err)
		}

		identity, err := identityOf(fd)
		if err != nil {
			return nil, cleanupFailedStage(parentFD, name, fd, fmt.Errorf("identify temporary file: %w", err))
		}
		if err := prepareStagedFile(fd, data, uint32(mode.Perm())); err != nil {
			return nil, cleanupFailedStage(parentFD, name, fd, err)
		}
		return &stagedFile{name: name, fd: fd, identity: identity}, nil
	}
	return nil, fmt.Errorf("create temporary file: exhausted %d randomized names", tempAttempts)
}

func cleanupFailedStage(parentFD int, name string, fd int, primary error) error {
	var errs = []error{primary}
	if err := closeStaged(fd); err != nil {
		errs = append(errs, fmt.Errorf("close staged file: %w", err))
	}
	if err := unlinkTemporary(parentFD, name); err != nil {
		errs = append(errs, fmt.Errorf("cleanup temporary file %q: %w", name, err))
	}
	return errors.Join(errs...)
}

func temporarySuffix() (string, error) {
	if hook := replaceTestHooks.randomSuffix; hook != nil {
		return hook()
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

func prepareStagedFile(fd int, data []byte, mode uint32) error {
	if err := unix.Fchmod(fd, mode); err != nil {
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	for len(data) != 0 {
		written, err := unix.Write(fd, data)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return fmt.Errorf("write temporary file: %w", err)
		}
		if written == 0 {
			return fmt.Errorf("write temporary file: %w", unix.EIO)
		}
		data = data[written:]
	}
	if err := syncStaged(fd); err != nil {
		return fmt.Errorf("fsync temporary file: %w", err)
	}
	return nil
}

func identityOf(fd int) (fileIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fileIdentity{}, err
	}
	return identityFromStat(&stat), nil
}

func identityAt(parentFD int, name string) (fileIdentity, uint32, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fileIdentity{}, 0, err
	}
	return identityFromStat(&stat), uint32(stat.Mode) & unix.S_IFMT, nil
}

func identityFromStat(stat *unix.Stat_t) fileIdentity {
	return fileIdentity{device: uint64(stat.Dev), inode: stat.Ino}
}

func verifyStagedEntry(parentFD int, name string, expected fileIdentity) error {
	actual, fileType, err := identityAt(parentFD, name)
	if err != nil {
		return fmt.Errorf("%w: inspect %q: %w", ErrStagedChanged, name, err)
	}
	if fileType != unix.S_IFREG {
		return fmt.Errorf("%w: %q is no longer a regular file", ErrStagedChanged, name)
	}
	if actual != expected {
		return fmt.Errorf("%w: %q expected device/inode %d/%d, got %d/%d", ErrStagedChanged, name, expected.device, expected.inode, actual.device, actual.inode)
	}
	return nil
}

func verifyParent(rootFD int, directories []string, expected fileIdentity) error {
	current, err := openParent(rootFD, directories, false)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrParentChanged, err)
	}
	defer func() { _ = unix.Close(current) }()
	actual, err := identityOf(current)
	if err != nil {
		return fmt.Errorf("%w: identify current parent: %w", ErrParentChanged, err)
	}
	if actual != expected {
		return fmt.Errorf("%w: expected device/inode %d/%d, got %d/%d", ErrParentChanged, expected.device, expected.inode, actual.device, actual.inode)
	}
	return nil
}

func syncStaged(fd int) error {
	if hook := replaceTestHooks.fsyncStaged; hook != nil {
		return hook(fd)
	}
	return unix.Fsync(fd)
}

func closeStaged(fd int) error {
	if hook := replaceTestHooks.closeStaged; hook != nil {
		return hook(fd)
	}
	return unix.Close(fd)
}

func syncDirectory(fd int, operation string) error {
	if hook := replaceTestHooks.fsyncDir; hook != nil {
		return hook(fd, operation)
	}
	return unix.Fsync(fd)
}

func unlinkTemporary(parentFD int, name string) error {
	if hook := replaceTestHooks.unlinkTemp; hook != nil {
		return hook(parentFD, name)
	}
	return unix.Unlinkat(parentFD, name, 0)
}
