//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// CaptureDirectoryRootWithin captures only one directory leaf's root identity,
// ownership, and mode. It deliberately does not walk recursive contents and is
// used for private namespace authority where unrelated descendants may be
// large or contain symlinks.
func CaptureDirectoryRootWithin(root, rel string) (*DirectorySnapshot, *ParentChain, error) {
	parents, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return nil, nil, err
	}
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return nil, nil, err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = unix.Close(rootFD) }()
	parentFD, err := openAuthorizedParent(rootFD, directories, parents)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = unix.Close(parentFD) }()
	directoryFD, err := openDirectoryAt(parentFD, target)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = unix.Close(directoryFD) }()
	var stat unix.Stat_t
	if err := unix.Fstat(directoryFD, &stat); err != nil {
		return nil, nil, err
	}
	wanted := identityFromStat(&stat)
	if !restorableOwner(stat.Uid, stat.Gid, os.Geteuid(), os.Getegid()) {
		return nil, nil, fmt.Errorf("%w: directory owner is not the process owner", ErrDirectoryChanged)
	}
	snapshot := &DirectorySnapshot{
		tracked:  true,
		rootOnly: true,
		device:   uint64(stat.Dev),
		inode:    uint64(stat.Ino),
		uid:      stat.Uid,
		gid:      stat.Gid,
		root:     directorySnapshotNode{mode: fs.FileMode(uint32(stat.Mode) & 0o777)},
	}
	verifyLeaf := func() error {
		entry, err := statDirectoryEntryAt(parentFD, target)
		if err != nil || entry.identity != wanted || entry.fileType != unix.S_IFDIR || entry.uid != stat.Uid || entry.gid != stat.Gid || entry.mode.Perm() != snapshot.Permissions() {
			return fmt.Errorf("%w: directory root namespace entry changed", ErrDirectoryChanged)
		}
		return nil
	}
	if err := verifyLeaf(); err != nil {
		return nil, nil, err
	}
	if _, err := BindParentChainWithin(root, rel, parents, nil); err != nil {
		return nil, nil, err
	}
	if err := verifyLeaf(); err != nil {
		return nil, nil, err
	}
	return snapshot, parents, nil
}

// BindParentChainPrefixWithin derives and binds authority for one directory
// prefix from a single accepted full-target chain. It never recaptures the
// namespace; plan-created prefixes are accepted only through exact created
// snapshots supplied by the executor.
func BindParentChainPrefixWithin(root, fullTargetRel, prefixTargetRel string, acceptedFull *ParentChain, created map[string]*DirectorySnapshot) (*ParentChain, error) {
	if !acceptedFull.Tracked() {
		return nil, fmt.Errorf("%w: accepted full parent chain is untracked", ErrParentChanged)
	}
	fullTargetRel = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(fullTargetRel))), "/")
	prefixTargetRel = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(prefixTargetRel))), "/")
	if fullTargetRel != prefixTargetRel && !strings.HasPrefix(fullTargetRel, prefixTargetRel+"/") {
		return nil, fmt.Errorf("%w: prefix %s is outside full target %s", ErrInvalidPath, prefixTargetRel, fullTargetRel)
	}
	directories, _, err := splitRelativePath(prefixTargetRel)
	if err != nil {
		return nil, err
	}
	wantEntries := len(directories) + 1
	if len(acceptedFull.entries) < wantEntries {
		return nil, fmt.Errorf("%w: accepted full chain is shorter than prefix", ErrParentChanged)
	}
	prefix := &ParentChain{tracked: true, entries: append([]parentChainEntry(nil), acceptedFull.entries[:wantEntries]...)}
	return BindParentChainWithin(root, prefixTargetRel, prefix, created)
}

// OpenDirectoryWithinAuthorized returns a held descriptor for one exact
// directory leaf after validating both its complete parent chain and expected
// root identity/state. The caller owns the returned file.
func OpenDirectoryWithinAuthorized(root, rel string, parents *ParentChain, expected *DirectorySnapshot) (*os.File, error) {
	if !parents.Tracked() || expected == nil || !expected.tracked {
		return nil, fmt.Errorf("%w: directory open authority is incomplete", ErrParentChanged)
	}
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return nil, err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(rootFD) }()
	parentFD, err := openAuthorizedParent(rootFD, directories, parents)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(parentFD) }()
	fd, err := openDirectoryAt(parentFD, target)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if uint64(stat.Dev) != expected.device || uint64(stat.Ino) != expected.inode || stat.Uid != expected.uid || stat.Gid != expected.gid || fs.FileMode(uint32(stat.Mode)&0o777).Perm() != expected.Permissions() {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("%w: authorized directory leaf changed", ErrDirectoryChanged)
	}
	if _, err := BindParentChainWithin(root, rel, parents, nil); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	entry, err := statDirectoryEntryAt(parentFD, target)
	if err != nil || entry.identity != (fileIdentity{device: expected.device, inode: expected.inode}) || entry.fileType != unix.S_IFDIR ||
		entry.uid != expected.uid || entry.gid != expected.gid || entry.mode.Perm() != expected.Permissions() {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("%w: authorized directory leaf changed during validation", ErrDirectoryChanged)
	}
	if _, err := BindParentChainWithin(root, rel, parents, nil); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), target), nil
}

// CaptureParentChainWithin captures root plus every target-parent component.
// Once a component is absent, every deeper component is recorded absent
// without being created.
func CaptureParentChainWithin(root, rel string) (*ParentChain, error) {
	directories, _, err := splitRelativePath(rel)
	if err != nil {
		return nil, err
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open trusted root: %w", err)
	}
	defer func() { _ = unix.Close(rootFD) }()
	return captureParentChainAt(rootFD, directories)
}

// ObserveFileWithin returns one leaf revision paired with a stable exact
// parent-chain observation. A namespace change across the descriptor-stable
// leaf read is rejected.
func ObserveFileWithin(root, rel string) ([]byte, Revision, *ParentChain, error) {
	before, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return nil, Revision{}, nil, err
	}
	data, revision, err := ReadWithin(root, rel)
	if err != nil {
		return nil, Revision{}, nil, err
	}
	after, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return nil, Revision{}, nil, err
	}
	if !SameParentChain(before, after) {
		return nil, Revision{}, nil, fmt.Errorf("%w: file parent chain changed during observation", ErrParentChanged)
	}
	return data, revision, after, nil
}

// ObserveDirectoryWithin pairs a recursive directory snapshot (or an absent
// result) with a stable exact parent-chain observation.
func ObserveDirectoryWithin(root, rel string) (*DirectorySnapshot, *ParentChain, error) {
	before, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return nil, nil, err
	}
	snapshot, snapshotErr := SnapshotDirectoryWithin(root, rel)
	if snapshotErr != nil && !errors.Is(snapshotErr, os.ErrNotExist) {
		return nil, nil, snapshotErr
	}
	after, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return nil, nil, err
	}
	if !SameParentChain(before, after) {
		return nil, nil, fmt.Errorf("%w: directory parent chain changed during observation", ErrParentChanged)
	}
	return snapshot, after, snapshotErr
}

// ExtendParentChainWithinDirectory derives authority for a descendant target
// from an already-bound parent chain and exact directory-leaf identity. Every
// deeper target parent must already exist and is captured non-recursively.
func ExtendParentChainWithinDirectory(root, targetRel, directoryRel string, parents *ParentChain, directory *DirectorySnapshot) (*ParentChain, error) {
	if !parents.Tracked() || directory == nil || !directory.tracked {
		return nil, fmt.Errorf("%w: base directory authority is untracked", ErrParentChanged)
	}
	directoryRel = strings.TrimSuffix(directoryRel, "/")
	if targetRel == directoryRel || !strings.HasPrefix(targetRel, directoryRel+"/") {
		return nil, fmt.Errorf("%w: target is not below authorized directory", ErrInvalidPath)
	}
	current, err := CaptureParentChainWithin(root, targetRel)
	if err != nil {
		return nil, err
	}
	if len(current.entries) < len(parents.entries)+1 {
		return nil, fmt.Errorf("%w: descendant parent-chain shape is too short", ErrParentChanged)
	}
	for index, expected := range parents.entries {
		actual := current.entries[index]
		if !expected.exists || expected != actual {
			return nil, fmt.Errorf("%w: base parent-chain component %q changed", ErrParentChanged, expected.rel)
		}
	}
	directoryEntry := current.entries[len(parents.entries)]
	if !directoryEntry.exists || directoryEntry.rel != directoryRel || directoryEntry.device != directory.device || directoryEntry.inode != directory.inode ||
		directoryEntry.uid != directory.uid || directoryEntry.gid != directory.gid || directoryEntry.mode != uint32(directory.root.mode)&0o7777 {
		return nil, fmt.Errorf("%w: authorized base directory changed", ErrParentChanged)
	}
	for _, entry := range current.entries[len(parents.entries)+1:] {
		if !entry.exists {
			return nil, fmt.Errorf("%w: descendant parent %q is missing", ErrParentChanged, entry.rel)
		}
	}
	return current, nil
}

// ReadWithinAuthorized reads a file only while the complete bound parent chain
// matches before and after the descriptor-stable read.
func ReadWithinAuthorized(root, rel string, parents *ParentChain) ([]byte, Revision, error) {
	if _, err := BindParentChainWithin(root, rel, parents, nil); err != nil {
		return nil, Revision{}, err
	}
	data, revision, err := ReadWithin(root, rel)
	if err != nil {
		return nil, Revision{}, err
	}
	if _, err := BindParentChainWithin(root, rel, parents, nil); err != nil {
		return nil, Revision{}, err
	}
	return data, revision, nil
}

func captureParentChainAt(rootFD int, directories []string) (*ParentChain, error) {
	rootEntry, err := parentChainEntryForFD(rootFD, "")
	if err != nil {
		return nil, fmt.Errorf("identify parent-chain root: %w", err)
	}
	chain := &ParentChain{tracked: true, entries: []parentChainEntry{rootEntry}}
	current, err := unix.Dup(rootFD)
	if err != nil {
		return nil, fmt.Errorf("duplicate parent-chain root: %w", err)
	}
	defer func() { _ = unix.Close(current) }()
	missing := false
	currentRel := ""
	for _, name := range directories {
		if currentRel == "" {
			currentRel = name
		} else {
			currentRel += "/" + name
		}
		if missing {
			chain.entries = append(chain.entries, parentChainEntry{rel: currentRel})
			continue
		}
		next, openErr := openDirectoryAt(current, name)
		if errors.Is(openErr, unix.ENOENT) {
			missing = true
			chain.entries = append(chain.entries, parentChainEntry{rel: currentRel})
			continue
		}
		if openErr != nil {
			return nil, fmt.Errorf("open parent-chain directory %q: %w", currentRel, openErr)
		}
		entry, identityErr := parentChainEntryForFD(next, currentRel)
		if identityErr != nil {
			_ = unix.Close(next)
			return nil, fmt.Errorf("identify parent-chain directory %q: %w", currentRel, identityErr)
		}
		_ = unix.Close(current)
		current = next
		chain.entries = append(chain.entries, entry)
	}
	return chain, nil
}

func parentChainEntryForFD(fd int, rel string) (parentChainEntry, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return parentChainEntry{}, err
	}
	if uint32(stat.Mode)&unix.S_IFMT != unix.S_IFDIR {
		return parentChainEntry{}, ErrNonRegular
	}
	mode := uint32(stat.Mode) & 0o7777
	if mode&0o022 != 0 {
		return parentChainEntry{}, fmt.Errorf("%w: parent-chain directory %q is group/world writable", ErrParentChanged, rel)
	}
	if !restorableOwner(stat.Uid, stat.Gid, os.Geteuid(), os.Getegid()) {
		return parentChainEntry{}, fmt.Errorf("%w: parent-chain directory %q owner %d:%d is not the process owner %d:%d", ErrParentChanged, rel, stat.Uid, stat.Gid, os.Geteuid(), os.Getegid())
	}
	identity := identityFromStat(&stat)
	return parentChainEntry{rel: rel, exists: true, device: identity.device, inode: identity.inode, mode: mode, uid: stat.Uid, gid: stat.Gid}, nil
}

// BindParentChainWithin verifies every plan-time existing identity and binds
// each newly present component only when its exact creation snapshot appears
// in created. The returned chain requires every target parent to exist.
func BindParentChainWithin(root, rel string, accepted *ParentChain, created map[string]*DirectorySnapshot) (*ParentChain, error) {
	current, err := validateParentChainWithin(root, rel, accepted, created, true)
	return current, err
}

// ValidateParentChainWithin validates plan-time existing identities and exact
// created evidence while permitting other plan-accepted missing descendants to
// remain absent. It is used before reviewed product-parent creation.
func ValidateParentChainWithin(root, rel string, accepted *ParentChain, created map[string]*DirectorySnapshot) (*ParentChain, error) {
	return validateParentChainWithin(root, rel, accepted, created, false)
}

func validateParentChainWithin(root, rel string, accepted *ParentChain, created map[string]*DirectorySnapshot, requireAll bool) (*ParentChain, error) {
	if !accepted.Tracked() {
		return nil, fmt.Errorf("%w: accepted parent chain is untracked", ErrParentChanged)
	}
	current, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return nil, err
	}
	if len(current.entries) != len(accepted.entries) {
		return nil, fmt.Errorf("%w: parent-chain shape changed", ErrParentChanged)
	}
	for index, expected := range accepted.entries {
		actual := current.entries[index]
		if expected.rel != actual.rel {
			return nil, fmt.Errorf("%w: parent-chain component %q is missing", ErrParentChanged, expected.rel)
		}
		if expected.exists {
			if !actual.exists || expected != actual {
				return nil, fmt.Errorf("%w: parent-chain component %q was replaced", ErrParentChanged, expected.rel)
			}
			continue
		}
		createdSnapshot := created[expected.rel]
		if createdSnapshot == nil && !requireAll {
			if actual.exists {
				return nil, fmt.Errorf("%w: parent-chain component %q appeared without accepted creation evidence", ErrParentChanged, expected.rel)
			}
			continue
		}
		if !actual.exists {
			return nil, fmt.Errorf("%w: parent-chain component %q is missing", ErrParentChanged, expected.rel)
		}
		if !recursiveDirectorySnapshot(createdSnapshot) || createdSnapshot.device != actual.device || createdSnapshot.inode != actual.inode ||
			createdSnapshot.uid != actual.uid || createdSnapshot.gid != actual.gid || uint32(createdSnapshot.Permissions()) != actual.mode {
			return nil, fmt.Errorf("%w: parent-chain component %q lacks exact creation evidence", ErrParentChanged, expected.rel)
		}
	}
	return current, nil
}

func openAuthorizedParent(rootFD int, directories []string, chain *ParentChain) (int, error) {
	if !chain.Tracked() || len(chain.entries) != len(directories)+1 {
		return -1, fmt.Errorf("%w: parent-chain authority does not match target", ErrParentChanged)
	}
	rootActual, err := parentChainEntryForFD(rootFD, "")
	if err != nil {
		return -1, fmt.Errorf("identify authorized root: %w", err)
	}
	rootExpected := chain.entries[0]
	if rootExpected != rootActual {
		return -1, fmt.Errorf("%w: trusted root identity changed", ErrParentChanged)
	}
	current, err := unix.Dup(rootFD)
	if err != nil {
		return -1, fmt.Errorf("duplicate authorized root: %w", err)
	}
	for index, name := range directories {
		next, openErr := openDirectoryAt(current, name)
		if openErr != nil {
			_ = unix.Close(current)
			return -1, fmt.Errorf("open authorized parent %q: %w", name, openErr)
		}
		expected := chain.entries[index+1]
		actual, identityErr := parentChainEntryForFD(next, expected.rel)
		if identityErr != nil || expected != actual {
			_ = unix.Close(next)
			_ = unix.Close(current)
			return -1, fmt.Errorf("%w: authorized parent %q changed", ErrParentChanged, expected.rel)
		}
		_ = unix.Close(current)
		current = next
	}
	return current, nil
}

func verifyAuthorizedParentChain(rootFD int, directories []string, chain *ParentChain) error {
	parentFD, err := openAuthorizedParent(rootFD, directories, chain)
	if err != nil {
		return err
	}
	return unix.Close(parentFD)
}

func verifyMutationParent(root string, rootFD int, directories []string, wanted fileIdentity, chain *ParentChain) error {
	if chain != nil {
		pathFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			return fmt.Errorf("%w: reopen trusted root path: %v", ErrParentChanged, err)
		}
		pathEntry, identityErr := parentChainEntryForFD(pathFD, "")
		_ = unix.Close(pathFD)
		expected := chain.entries[0]
		if identityErr != nil || expected != pathEntry {
			return fmt.Errorf("%w: trusted root path identity changed", ErrParentChanged)
		}
	}
	if err := verifyParent(rootFD, directories, wanted); err != nil {
		return err
	}
	if chain != nil {
		return verifyAuthorizedParentChain(rootFD, directories, chain)
	}
	return nil
}
