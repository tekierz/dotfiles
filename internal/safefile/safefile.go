// Package safefile provides descriptor-anchored directory snapshot,
// replacement and removal, plus file reads, replacement, removal, and advisory
// locking below a trusted root.
package safefile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// restorableOwner is the portable policy kernel for ownership observations.
// The unprivileged restore path can reproduce only the process effective uid
// and gid; unknown/negative process identities and foreign owners fail closed.
func restorableOwner(uid, gid uint32, euid, egid int) bool {
	// #nosec G115 -- non-negative IDs are range-checked before narrowing.
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
	// ErrSizeLimit reports that a stable regular file exceeds the caller's
	// explicit ReadWithinLimit byte budget.
	ErrSizeLimit = errors.New("file exceeds read size limit")
	// ErrSnapshotLimit reports that a recursive directory capture exceeded an
	// explicit file-count, per-file, or aggregate byte budget.
	ErrSnapshotLimit = errors.New("directory snapshot exceeds budget")
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
	// ErrInvalidAuthority reports an opaque authority value that was not
	// produced by a complete tracked capture or whose private shape is invalid.
	ErrInvalidAuthority = errors.New("invalid private filesystem authority")
)

// DirectorySnapshot is an immutable, opaque capture produced by this package.
// SnapshotDirectoryWithin returns a recursive capture that binds root
// identity/owner plus recursive names, node types, modes, and file bytes.
// CaptureDirectoryRootWithin returns a root-only namespace token that cannot
// be used by recursive verify, restore, or removal APIs. Nested uid/gid values
// are not serialized; recursive capture fails closed unless every node is
// owned by the process euid/egid, the only ownership the unprivileged restore
// path can reproduce. Its unexported representation prevents callers from
// injecting snapshot data.
type DirectorySnapshot struct {
	tracked  bool
	rootOnly bool
	device   uint64
	inode    uint64
	uid      uint32
	gid      uint32
	root     directorySnapshotNode
	digest   [32]byte
}

// SnapshotBudget bounds recursive snapshot allocation. All members must be
// positive. MaxFiles also caps directory entries so a directory-only tree
// cannot bypass the allocation bound.
type SnapshotBudget struct {
	MaxFiles      int
	MaxFileBytes  int64
	MaxTotalBytes int64
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

// ParentChainAuthorityDigest returns a domain-separated private fingerprint of
// the complete tracked root-to-parent authority. Only the digest leaves this
// package; relative names and filesystem identities remain opaque.
func ParentChainAuthorityDigest(chain *ParentChain) (string, error) {
	if chain == nil || !chain.tracked || len(chain.entries) == 0 {
		return "", ErrInvalidAuthority
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("dotfiles/safefile-parent-chain-authority/v1\x00"))
	hashSnapshotUint64(hash, uint64(len(chain.entries)))
	previousRel := ""
	missing := false
	for index, entry := range chain.entries {
		if index == 0 {
			if entry.rel != "" || !entry.exists {
				return "", ErrInvalidAuthority
			}
		} else {
			if entry.rel == "" || strings.ContainsRune(entry.rel, '\x00') || path.Clean(entry.rel) != entry.rel || path.IsAbs(entry.rel) || path.Dir(entry.rel) != normalizedAuthorityParent(previousRel) {
				return "", ErrInvalidAuthority
			}
			previousRel = entry.rel
		}
		if entry.exists {
			if missing || entry.device == 0 || entry.inode == 0 || entry.mode&^uint32(0o7777) != 0 || entry.mode&0o022 != 0 {
				return "", ErrInvalidAuthority
			}
		} else {
			missing = true
			if entry.device != 0 || entry.inode != 0 || entry.mode != 0 || entry.uid != 0 || entry.gid != 0 {
				return "", ErrInvalidAuthority
			}
		}
		hashSnapshotBytes(hash, []byte(entry.rel))
		if entry.exists {
			_, _ = hash.Write([]byte{1})
		} else {
			_, _ = hash.Write([]byte{0})
		}
		hashSnapshotUint64(hash, entry.device)
		hashSnapshotUint64(hash, entry.inode)
		hashSnapshotUint64(hash, uint64(entry.mode))
		hashSnapshotUint64(hash, uint64(entry.uid))
		hashSnapshotUint64(hash, uint64(entry.gid))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func normalizedAuthorityParent(previous string) string {
	if previous == "" {
		return "."
	}
	return previous
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
// secret handling. A nil, untracked, or root-only snapshot returns the zero
// digest; root-only tokens intentionally do not describe recursive contents.
func (s *DirectorySnapshot) Digest() [32]byte {
	if s == nil || !s.tracked || s.rootOnly {
		return [32]byte{}
	}
	return s.digest
}

// DirectorySnapshotAuthorityDigest returns a domain-separated private
// fingerprint of root identity, ownership, permissions, capture kind, and—if
// recursive—the already-validated recursive content digest. It intentionally
// does not change Digest's root-only zero semantics.
func DirectorySnapshotAuthorityDigest(snapshot *DirectorySnapshot) (string, error) {
	if snapshot == nil || !snapshot.tracked || snapshot.device == 0 || snapshot.inode == 0 || snapshot.root.mode != snapshot.root.mode.Perm() {
		return "", ErrInvalidAuthority
	}
	if snapshot.rootOnly {
		if snapshot.digest != ([32]byte{}) || len(snapshot.root.entries) != 0 {
			return "", ErrInvalidAuthority
		}
	} else {
		if snapshot.digest == ([32]byte{}) || newDirectorySnapshot(snapshot.root).digest != snapshot.digest {
			return "", ErrInvalidAuthority
		}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("dotfiles/safefile-directory-authority/v1\x00"))
	if snapshot.rootOnly {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	hashSnapshotUint64(hash, snapshot.device)
	hashSnapshotUint64(hash, snapshot.inode)
	hashSnapshotUint64(hash, uint64(snapshot.uid))
	hashSnapshotUint64(hash, uint64(snapshot.gid))
	hashSnapshotUint64(hash, uint64(snapshot.root.mode.Perm()))
	hashSnapshotBytes(hash, snapshot.digest[:])
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func recursiveDirectorySnapshot(snapshot *DirectorySnapshot) bool {
	return snapshot != nil && snapshot.tracked && !snapshot.rootOnly
}

// ReadDirectorySnapshotFile returns one cloned regular file below a complete recursive authority at an already-clean rel path.
func ReadDirectorySnapshotFile(snapshot *DirectorySnapshot, rel string) ([]byte, fs.FileMode, error) {
	return ReadDirectorySnapshotFileLimit(snapshot, rel, int64(^uint64(0)>>1))
}

// ReadDirectorySnapshotFileLimit clones one accepted snapshot file only when
// its already-captured size is within limit.
func ReadDirectorySnapshotFileLimit(snapshot *DirectorySnapshot, rel string, limit int64) ([]byte, fs.FileMode, error) {
	if limit < 0 {
		return nil, 0, fmt.Errorf("%w: negative limit %d", ErrSizeLimit, limit)
	}
	entry, err := directorySnapshotDescendant(snapshot, rel)
	if err != nil {
		return nil, 0, err
	}
	if entry.dir != nil {
		return nil, 0, fmt.Errorf("%w: snapshot descendant %q is a directory", ErrInvalidPath, rel)
	}
	if int64(len(entry.data)) > limit {
		return nil, 0, fmt.Errorf("%w: snapshot descendant exceeds %d bytes", ErrSizeLimit, limit)
	}
	return cloneDirectorySnapshotBytes(entry.data), entry.mode.Perm(), nil
}

// SubdirectorySnapshot returns a deep, immutable data-only copy of one
// captured subtree. The result retains recursive restore data and its digest,
// but deliberately carries no live namespace identity authority.
func SubdirectorySnapshot(snapshot *DirectorySnapshot, rel string) (*DirectorySnapshot, error) {
	entry, err := directorySnapshotDescendant(snapshot, rel)
	if err != nil {
		return nil, err
	}
	if entry.dir == nil {
		return nil, fmt.Errorf("%w: snapshot descendant %q is not a directory", ErrInvalidPath, rel)
	}
	result := newDirectorySnapshot(cloneDirectorySnapshotNode(*entry.dir))
	result.uid, result.gid = snapshot.uid, snapshot.gid
	return result, nil
}

func directorySnapshotDescendant(snapshot *DirectorySnapshot, rel string) (*directorySnapshotEntry, error) {
	if err := validateDirectorySnapshotExtractionSource(snapshot); err != nil {
		return nil, err
	}
	parts, err := cleanDirectorySnapshotRelativePath(rel)
	if err != nil {
		return nil, err
	}
	node := &snapshot.root
	for index, name := range parts {
		var found *directorySnapshotEntry
		for entryIndex := range node.entries {
			if node.entries[entryIndex].name == name {
				found = &node.entries[entryIndex]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("%w: snapshot descendant %q", fs.ErrNotExist, rel)
		}
		if index == len(parts)-1 {
			return found, nil
		}
		if found.dir == nil {
			return nil, fmt.Errorf("%w: snapshot descendant %q crosses a regular file", ErrInvalidPath, rel)
		}
		node = found.dir
	}
	return nil, ErrInvalidPath
}

func cleanDirectorySnapshotRelativePath(rel string) ([]string, error) {
	if rel == "" || rel == "." || path.IsAbs(rel) || strings.ContainsRune(rel, '\x00') || path.Clean(rel) != rel {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("%w: %q", ErrInvalidPath, rel)
		}
	}
	return parts, nil
}

func validateDirectorySnapshotExtractionSource(snapshot *DirectorySnapshot) error {
	if !recursiveDirectorySnapshot(snapshot) || !validDirectorySnapshotExtractionNode(&snapshot.root) {
		return ErrInvalidAuthority
	}
	if _, err := DirectorySnapshotAuthorityDigest(snapshot); err != nil {
		return ErrInvalidAuthority
	}
	return nil
}

func validDirectorySnapshotExtractionNode(node *directorySnapshotNode) bool {
	if node == nil || node.mode != node.mode.Perm() {
		return false
	}
	previous := ""
	for index := range node.entries {
		entry := &node.entries[index]
		if entry.name == "" || entry.name == "." || entry.name == ".." || strings.ContainsAny(entry.name, "/\x00") ||
			(previous != "" && entry.name <= previous) || entry.mode != entry.mode.Perm() {
			return false
		}
		previous = entry.name
		if entry.dir != nil && (len(entry.data) != 0 || entry.mode != entry.dir.mode || !validDirectorySnapshotExtractionNode(entry.dir)) {
			return false
		}
	}
	return true
}

func cloneDirectorySnapshotNode(source directorySnapshotNode) directorySnapshotNode {
	clone := directorySnapshotNode{mode: source.mode, entries: make([]directorySnapshotEntry, len(source.entries))}
	for index, entry := range source.entries {
		clone.entries[index] = directorySnapshotEntry{name: entry.name, mode: entry.mode, data: cloneDirectorySnapshotBytes(entry.data)}
		if entry.dir != nil {
			child := cloneDirectorySnapshotNode(*entry.dir)
			clone.entries[index].dir = &child
		}
	}
	return clone
}

func cloneDirectorySnapshotBytes(data []byte) []byte { return append(data[:0:0], data...) }

// RecursiveFileStats returns the number and total byte size of regular files
// captured recursively, excluding any named direct children of the snapshot
// root. Root-only namespace tokens report zero values.
func (s *DirectorySnapshot) RecursiveFileStats(excludeRootNames ...string) (int, int64) {
	if !recursiveDirectorySnapshot(s) {
		return 0, 0
	}
	excluded := make(map[string]struct{}, len(excludeRootNames))
	for _, name := range excludeRootNames {
		excluded[name] = struct{}{}
	}
	var walk func(directorySnapshotNode) (int, int64)
	walk = func(node directorySnapshotNode) (int, int64) {
		var count int
		var size int64
		for _, entry := range node.entries {
			if entry.dir != nil {
				nestedCount, nestedSize := walk(*entry.dir)
				count += nestedCount
				size += nestedSize
				continue
			}
			count++
			size += int64(len(entry.data))
		}
		return count, size
	}
	var count int
	var size int64
	for _, entry := range s.root.entries {
		if _, skip := excluded[entry.name]; skip {
			continue
		}
		if entry.dir != nil {
			nestedCount, nestedSize := walk(*entry.dir)
			count += nestedCount
			size += nestedSize
		} else {
			count++
			size += int64(len(entry.data))
		}
	}
	return count, size
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
		left.device != 0 && left.inode != 0 && left.device == right.device && left.inode == right.inode
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

// Mode returns the complete stable os.FileInfo mode captured from the held
// descriptor. Callers that require an exact private-file mode can therefore
// reject setuid, setgid, sticky, and other non-permission bits as well as
// ordinary permission mismatches.
func (r Revision) Mode() fs.FileMode { return fs.FileMode(r.mode) }

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
