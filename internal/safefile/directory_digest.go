package safefile

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
)

func newDirectorySnapshot(root directorySnapshotNode) *DirectorySnapshot {
	snapshot := &DirectorySnapshot{tracked: true, root: root}
	h := sha256.New()
	_, _ = h.Write([]byte("dotfiles/safefile-directory-snapshot/v1\x00"))
	hashDirectorySnapshotNode(h, &root)
	copy(snapshot.digest[:], h.Sum(nil))
	return snapshot
}

func hashDirectorySnapshotNode(h hash.Hash, node *directorySnapshotNode) {
	_, _ = h.Write([]byte{'D'})
	hashSnapshotUint64(h, uint64(node.mode.Perm()))
	hashSnapshotUint64(h, uint64(len(node.entries)))
	for _, entry := range node.entries {
		hashSnapshotBytes(h, []byte(entry.name))
		if entry.dir != nil {
			hashDirectorySnapshotNode(h, entry.dir)
			continue
		}
		_, _ = h.Write([]byte{'F'})
		hashSnapshotUint64(h, uint64(entry.mode.Perm()))
		hashSnapshotBytes(h, entry.data)
	}
}

func hashSnapshotBytes(h hash.Hash, value []byte) {
	hashSnapshotUint64(h, uint64(len(value)))
	_, _ = h.Write(value)
}

func hashSnapshotUint64(h hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = h.Write(encoded[:])
}
