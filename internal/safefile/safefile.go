// Package safefile provides descriptor-anchored directory snapshot,
// replacement and removal, plus file reads, replacement, removal, and advisory
// locking below a trusted root.
package safefile

import (
	"errors"
	"fmt"
	"io/fs"
)

// restorableOwner is the portable policy kernel for ownership observations.
// The unprivileged restore path can reproduce only the process effective uid
// and gid; unknown/negative process identities and foreign owners fail closed.
func restorableOwner(uid, gid uint32, euid, egid int) bool {
	return euid >= 0 && egid >= 0 && uint64(euid) <= uint64(^uint32(0)) && uint64(egid) <= uint64(^uint32(0)) &&
		uid == uint32(euid) && gid == uint32(egid)
}

var (
	// ErrInvalidPath reports a target path that cannot be safely resolved below
	// the trusted root.
	ErrInvalidPath = errors.New("invalid relative path")
	// ErrSymlink reports a refused symlink in the untrusted portion of a path.
	ErrSymlink = errors.New("symlink refused")
	// ErrParentChanged reports that the target parent is no longer reachable at
	// the identity originally opened below the trusted root.
	ErrParentChanged = errors.New("target parent changed")
	// ErrNonRegular reports an existing leaf that is not a regular file. Device
	// nodes, sockets, FIFOs, and directories are never replaced or removed.
	ErrNonRegular = errors.New("non-regular target refused")
	// ErrStagedChanged reports that the randomized staging name or committed leaf
	// no longer names the inode held open by ReplaceWithin.
	ErrStagedChanged = errors.New("staged file identity changed")
	// ErrRevisionChanged reports that a file changed while ReadWithin held and
	// read its descriptor, so no stable source revision can be returned.
	ErrRevisionChanged = errors.New("file changed while reading")
	// ErrHardlink reports a lock path with more than one directory entry. Such a
	// file is refused before chmod or flock can affect another name.
	ErrHardlink = errors.New("hardlinked lock file refused")
	// ErrLockChanged reports that the locked descriptor and the current lock
	// namespace entry no longer identify the same inode.
	ErrLockChanged = errors.New("lock path changed while acquiring lock")
	// ErrTargetChanged reports that a descriptor-anchored removal target no
	// longer names the regular file that was opened for deletion.
	ErrTargetChanged = errors.New("removal target identity changed")
	// ErrDirectoryChanged reports that a directory tree changed while it was
	// being captured or while a transactional namespace mutation was prepared.
	ErrDirectoryChanged = errors.New("directory tree changed during safe operation")
	// ErrInvalidMode reports a mode containing non-permission bits.
	ErrInvalidMode = errors.New("invalid permission mode")
	// ErrUnsupported reports that descriptor-anchored file operations are
	// unavailable on the current operating system.
	ErrUnsupported = errors.New("safe file operation is unsupported on this platform")
)

// DirectorySnapshot is an immutable, opaque recursive capture produced by
// SnapshotDirectoryWithin. It binds root identity/owner plus recursive names,
// node types, modes, and file bytes. Nested uid/gid values are not serialized;
// capture fails closed unless every node is owned by the process euid/egid, the
// only ownership the unprivileged restore path can reproduce. Its unexported
// representation prevents callers from injecting snapshot data.
type DirectorySnapshot struct {
	tracked bool
	device  uint64
	inode   uint64
	uid     uint32
	gid     uint32
	root    directorySnapshotNode
	digest  [32]byte
}

// ParentChain is an immutable, opaque, non-recursive identity capture from a
// trusted root through the parent of one mutation target. Missing descendant
// parents are represented explicitly. Plan execution binds those missing
// entries only to exact shallow-directory creation evidence before a leaf
// writer may commit.
type ParentChain struct {
	tracked bool
	entries []parentChainEntry
}

type parentChainEntry struct {
	rel    string
	exists bool
	device uint64
	inode  uint64
	mode   uint32
	uid    uint32
	gid    uint32
}

// Tracked reports whether the chain was captured by CaptureParentChainWithin.
func (c *ParentChain) Tracked() bool { return c != nil && c.tracked }

// SameParentChain reports exact equality of two tracked root-to-parent
// namespace observations.
func SameParentChain(left, right *ParentChain) bool {
	if !left.Tracked() || !right.Tracked() || len(left.entries) != len(right.entries) {
		return false
	}
	for index := range left.entries {
		if left.entries[index] != right.entries[index] {
			return false
		}
	}
	return true
}

type directorySnapshotNode struct {
	mode    fs.FileMode
	entries []directorySnapshotEntry
}

type directorySnapshotEntry struct {
	name string
	mode fs.FileMode
	data []byte
	dir  *directorySnapshotNode
}

// Digest returns a deterministic SHA-256 observation token for the complete
// recursive snapshot: names, node types, permission modes, and file bytes. Root
// uid/gid participates in exact CAS comparisons but not this public digest;
// nested nodes are accepted only under the restorable-owner policy described
// above. The digest is intended for change detection, not authentication or
// secret handling. A nil or untracked snapshot returns the zero digest.
func (s *DirectorySnapshot) Digest() [32]byte {
	if s == nil || !s.tracked {
		return [32]byte{}
	}
	return s.digest
}

// Permissions returns the captured root directory permission bits. A nil or
// untracked snapshot returns zero.
func (s *DirectorySnapshot) Permissions() fs.FileMode {
	if s == nil || !s.tracked {
		return 0
	}
	return s.root.mode.Perm()
}

// SameDirectoryIdentity reports whether two tracked snapshots describe the
// same root directory inode. Recursive contents may differ. This is useful for
// private staging areas whose contents are intentionally populated between an
// empty creation snapshot and a final cleanup snapshot.
func SameDirectoryIdentity(left, right *DirectorySnapshot) bool {
	return left != nil && right != nil && left.tracked && right.tracked &&
		left.device == right.device && left.inode == right.inode
}

// SameDirectoryRootState reports whether two tracked snapshots bind the same
// root inode, uid, gid, and permission mode while deliberately ignoring
// recursive contents.
func SameDirectoryRootState(left, right *DirectorySnapshot) bool {
	return SameDirectoryIdentity(left, right) && left.uid == right.uid && left.gid == right.gid && left.root.mode == right.root.mode
}

// RecoveryError reports that a directory transaction could not restore the
// pre-operation namespace after a failure before the requested replacement or
// removal committed. It deliberately does not implement Committed(): the
// requested new state is not known to be applied, and callers must surface the
// manual-recovery requirement rather than report a successful restore.
type RecoveryError struct {
	Operation string
	Err       error
}

func (e *RecoveryError) Error() string {
	return fmt.Sprintf("safe directory transaction recovery failed; %s: %v", e.Operation, e.Err)
}

func (e *RecoveryError) Unwrap() error { return e.Err }

// Revision is an opaque, comparable description of one ReadWithin result. It
// contains only value fields: tracked/existence state, file identity, mode,
// size, modification time, and a digest of the bytes read from the same held
// descriptor. Callers can compare revisions with == for optimistic concurrency.
// The zero value is untracked and cannot represent a completed ReadWithin call.
type Revision struct {
	tracked    bool
	exists     bool
	device     uint64
	inode      uint64
	mode       uint32
	links      uint64
	uid        uint32
	gid        uint32
	size       int64
	modifiedNS int64
	digest     [32]byte
}

// Tracked reports whether this revision came from ReadWithin.
func (r Revision) Tracked() bool { return r.tracked }

// Exists reports whether the file existed for this revision.
func (r Revision) Exists() bool { return r.tracked && r.exists }

// Permissions returns the permission bits observed on the held descriptor.
// Missing and untracked revisions return zero.
func (r Revision) Permissions() fs.FileMode { return fs.FileMode(r.mode).Perm() }

// LinkCount reports the number of directory entries observed for the opened
// inode. Missing and untracked revisions return zero. Mutation workflows can
// require exactly one link before removing a user-local file, avoiding changes
// to another package manager or user path that happens to share the inode.
func (r Revision) LinkCount() uint64 { return r.links }

// Digest returns the content hash captured by ReadWithin. Missing/untracked
// revisions return the zero digest.
func (r Revision) Digest() [32]byte { return r.digest }

// CommittedError reports a failure discovered after a mutation reached its
// commit point: a staged file was renamed over its target or a named file was
// unlinked from its held parent directory. Operation identifies the subsequent
// durability, identity, close, or cleanup check that failed.
//
// Callers can distinguish this state with errors.As. Retrying blindly is unsafe
// because the requested mutation may already have been applied.
type CommittedError struct {
	Operation string
	Err       error
}

func (e *CommittedError) Error() string {
	return fmt.Sprintf("safe file operation committed; %s failed: %v", e.Operation, e.Err)
}

// Unwrap exposes the underlying post-commit failure.
func (e *CommittedError) Unwrap() error { return e.Err }

// Committed always returns true and allows state-oriented error handling
// without coupling callers to the concrete error type.
func (e *CommittedError) Committed() bool { return true }
