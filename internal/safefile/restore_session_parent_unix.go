//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ObserveDirectoryStateWithin captures either exact root-only directory state
// or tracked absence together with one stable root-to-parent chain.
func ObserveDirectoryStateWithin(root, rel string) (DirectoryState, *ParentChain, error) {
	before, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return DirectoryState{}, nil, err
	}
	snapshot, observed, observeErr := CaptureDirectoryRootWithin(root, rel)
	if observeErr != nil && !errors.Is(observeErr, os.ErrNotExist) {
		return DirectoryState{}, nil, observeErr
	}
	if observeErr == nil {
		if !SameParentChain(before, observed) {
			return DirectoryState{}, nil, fmt.Errorf("%w: directory parent chain changed during observation", ErrParentChanged)
		}
		return DirectoryState{tracked: true, exists: true, snapshot: snapshot}, observed, nil
	}
	after, err := CaptureParentChainWithin(root, rel)
	if err != nil {
		return DirectoryState{}, nil, err
	}
	if !SameParentChain(before, after) {
		return DirectoryState{}, nil, fmt.Errorf("%w: absent directory parent chain changed during observation", ErrParentChanged)
	}
	return DirectoryState{tracked: true}, after, nil
}

// NewRestoreSession accepts only an absolute, clean, existing real directory
// and captures its exact namespace identity for every later session call.
func NewRestoreSession(root string) (*RestoreSession, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, fmt.Errorf("%w: restore root must be absolute and clean", ErrInvalidPath)
	}
	snapshot, err := captureRestoreSessionRoot(root)
	if err != nil {
		return nil, fmt.Errorf("capture restore root authority: %w", err)
	}
	return &RestoreSession{
		root: root, rootSnapshot: snapshot,
		created: make(map[string]*DirectorySnapshot),
	}, nil
}

func captureRestoreSessionRoot(root string) (*DirectorySnapshot, error) {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(fd) }()
	entry, err := parentChainEntryForFD(fd, "")
	if err != nil {
		return nil, err
	}
	return &DirectorySnapshot{
		tracked: true, rootOnly: true,
		device: entry.device, inode: entry.inode, uid: entry.uid, gid: entry.gid,
		root: directorySnapshotNode{mode: os.FileMode(entry.mode).Perm()},
	}, nil
}

func (s *RestoreSession) bindRestoreParents(rel string, accepted *ParentChain, create bool) (restoreParentBinding, error) {
	if s == nil {
		return restoreParentBinding{}, fmt.Errorf("%w: restore session is nil", ErrInvalidAuthority)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateLocked(); err != nil {
		return restoreParentBinding{}, err
	}
	if !accepted.Tracked() {
		return restoreParentBinding{}, fmt.Errorf("%w: accepted restore parent chain is untracked", ErrParentChanged)
	}
	directories, target, err := splitRelativePath(rel)
	if err != nil {
		return restoreParentBinding{}, err
	}
	rel = strings.Join(append(append([]string(nil), directories...), target), "/")
	if !create {
		parents, err := BindParentChainWithin(s.root, rel, accepted, s.created)
		if err != nil {
			return restoreParentBinding{}, err
		}
		return restoreParentBinding{session: s, tracked: true, parents: parents}, nil
	}
	if _, err := ValidateParentChainWithin(s.root, rel, accepted, s.created); err != nil {
		return restoreParentBinding{}, err
	}
	binding := restoreParentBinding{session: s, tracked: true}
	for index := range directories {
		prefix := strings.Join(directories[:index+1], "/")
		acceptedEntry := accepted.entries[index+1]
		if acceptedEntry.exists || s.created[prefix] != nil {
			continue
		}
		prefixParents, err := BindParentChainPrefixWithin(s.root, rel, prefix, accepted, s.created)
		if err != nil {
			return restoreParentBinding{}, s.failBindLocked(err, binding)
		}
		evidence, err := EnsureShallowDirectoryWithinParentChainTracked(s.root, prefix, nil, prefixParents, directoryMode)
		if err != nil {
			return restoreParentBinding{}, s.failBindLocked(err, binding)
		}
		s.created[prefix] = evidence
		binding.created = append(binding.created, restoreCreatedParent{rel: prefix, snapshot: evidence, parents: prefixParents})
	}
	parents, err := BindParentChainWithin(s.root, rel, accepted, s.created)
	if err != nil {
		return restoreParentBinding{}, s.failBindLocked(err, binding)
	}
	binding.parents = parents
	return binding, nil
}

func (s *RestoreSession) rollbackRestoreParents(binding restoreParentBinding) error {
	if s == nil {
		return fmt.Errorf("%w: restore session is nil", ErrInvalidAuthority)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !binding.tracked || binding.session != s {
		return fmt.Errorf("%w: restore parent binding belongs to another session", ErrInvalidAuthority)
	}
	if err := s.validateLocked(); err != nil {
		if len(binding.created) != 0 {
			return &RecoveryError{Operation: "reach original restore root for parent cleanup", Err: err}
		}
		return err
	}
	if err := s.rollbackLocked(binding); err != nil {
		return &RecoveryError{Operation: "remove restore-created parent directories", Err: err}
	}
	return nil
}

func (s *RestoreSession) validateLocked() error {
	if s.root == "" || s.rootSnapshot == nil || s.created == nil {
		return fmt.Errorf("%w: restore session authority is incomplete", ErrInvalidAuthority)
	}
	current, err := captureRestoreSessionRoot(s.root)
	if err != nil || !SameDirectoryRootState(s.rootSnapshot, current) {
		return fmt.Errorf("restore session root changed: %w", errors.Join(ErrDirectoryChanged, err))
	}
	return nil
}

func (s *RestoreSession) failBindLocked(primary error, binding restoreParentBinding) error {
	if rollbackErr := s.rollbackLocked(binding); rollbackErr != nil {
		return &RecoveryError{Operation: "rollback restore parents after bind failure", Err: errors.Join(primary, rollbackErr)}
	}
	return primary
}

func (s *RestoreSession) rollbackLocked(binding restoreParentBinding) error {
	for index := len(binding.created) - 1; index >= 0; index-- {
		created := binding.created[index]
		if s.created[created.rel] != created.snapshot {
			return fmt.Errorf("%w: restore-created parent evidence changed", ErrDirectoryChanged)
		}
		if err := RemoveEmptyDirectoryWithinSnapshotAuthorized(s.root, created.rel, created.snapshot, created.parents); err != nil {
			return err
		}
		delete(s.created, created.rel)
	}
	return nil
}
