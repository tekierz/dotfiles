//go:build darwin || linux

package safefile

import "fmt"

// RestoreDirectory installs one directory subtree extracted from immutable
// recursive snapshot authority, subject to accepted target and parent state.
func (s *RestoreSession) RestoreDirectory(rel string, parents *ParentChain, expected, source *DirectorySnapshot, sourceRel string) (*DirectorySnapshot, error) {
	snapshot, err := SubdirectorySnapshot(source, sourceRel)
	if err != nil {
		return nil, err
	}
	if expected != nil && !recursiveDirectorySnapshot(expected) {
		return nil, fmt.Errorf("%w: expected restore directory snapshot is not recursive", ErrDirectoryChanged)
	}
	binding, err := s.bindRestoreParents(rel, parents, true)
	if err != nil {
		return nil, err
	}
	evidence, err := RestoreDirectoryWithinSnapshotNoCreateAuthorizedTracked(s.root, rel, snapshot, expected, binding.parents)
	if err == nil {
		return evidence, nil
	}
	return nil, s.rollbackRestoreFailure("restore directory", binding, err)
}

// RemoveDirectory removes only an existing directory subtree named by exact
// recursive snapshot and parent authority. It never creates missing parents.
func (s *RestoreSession) RemoveDirectory(rel string, parents *ParentChain, expected *DirectorySnapshot) error {
	if s == nil {
		return fmt.Errorf("%w: restore session is nil", ErrInvalidAuthority)
	}
	if !recursiveDirectorySnapshot(expected) {
		return fmt.Errorf("%w: expected removal snapshot is nil, untracked, or root-only", ErrDirectoryChanged)
	}
	binding, err := s.bindRestoreParents(rel, parents, false)
	if err != nil {
		return err
	}
	if err := RemoveDirectoryWithinSnapshotAuthorized(s.root, rel, expected, binding.parents); err != nil {
		return err
	}
	if err := VerifyDirectoryWithinSnapshot(s.root, rel, nil); err != nil {
		return &CommittedError{Operation: "post-removal directory absence verification", Err: err}
	}
	return nil
}
