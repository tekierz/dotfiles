package safefile

import "io/fs"

// DirectoryAuthority binds one complete recursive snapshot without retaining
// its payloads or tree. It cannot itself restore or delete anything: callers
// must capture a fresh snapshot, match it, and pass that snapshot to the
// existing mutation APIs with their separately accepted parent authority.
type DirectoryAuthority struct {
	device uint64
	inode  uint64
	uid    uint32
	gid    uint32
	mode   fs.FileMode
	digest [32]byte
}

// CompactDirectoryAuthority accepts only a complete, validated recursive
// snapshot. Root-only namespace captures cannot authorize recursive contents.
func CompactDirectoryAuthority(snapshot *DirectorySnapshot) (DirectoryAuthority, error) {
	if err := validateDirectorySnapshotExtractionSource(snapshot); err != nil {
		return DirectoryAuthority{}, err
	}
	return DirectoryAuthority{
		device: snapshot.device, inode: snapshot.inode, uid: snapshot.uid,
		gid: snapshot.gid, mode: snapshot.root.mode, digest: snapshot.digest,
	}, nil
}

// Tracked reports whether this token contains complete captured authority.
func (authority DirectoryAuthority) Tracked() bool {
	return authority.device != 0 && authority.inode != 0 && authority.digest != ([32]byte{}) && authority.mode == authority.mode.Perm()
}

// Matches requires the same root identity, ownership, permissions, and exact
// recursive names, types, modes and bytes. Invalid or root-only inputs fail.
func (authority DirectoryAuthority) Matches(snapshot *DirectorySnapshot) bool {
	if !authority.Tracked() {
		return false
	}
	current, err := CompactDirectoryAuthority(snapshot)
	return err == nil && current == authority
}
