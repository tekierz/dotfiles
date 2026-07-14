//go:build darwin || linux

package safefile

import (
	"errors"
	"fmt"
)

// RestoreFile installs one regular file extracted from immutable recursive
// snapshot authority, subject to the accepted target revision and parent chain.
func (s *RestoreSession) RestoreFile(rel string, parents *ParentChain, expected Revision, source *DirectorySnapshot, sourceRel string) (Revision, error) {
	data, mode, err := ReadDirectorySnapshotFile(source, sourceRel)
	if err != nil {
		return Revision{}, err
	}
	if !expected.Tracked() {
		return Revision{}, fmt.Errorf("%w: expected restore revision is untracked", ErrRevisionChanged)
	}
	binding, err := s.bindRestoreParents(rel, parents, true)
	if err != nil {
		return Revision{}, err
	}
	revision, err := ReplaceWithinRevisionNoCreateAuthorizedTracked(s.root, rel, expected, binding.parents, data, mode)
	if err == nil {
		return revision, nil
	}
	return Revision{}, s.rollbackRestoreFailure("restore file", binding, err)
}

// RemoveFile removes only an existing regular file named by exact accepted
// revision and parent authority. It never creates missing parents.
func (s *RestoreSession) RemoveFile(rel string, parents *ParentChain, expected Revision) error {
	if s == nil {
		return fmt.Errorf("%w: restore session is nil", ErrInvalidAuthority)
	}
	if !expected.Tracked() || !expected.Exists() {
		return fmt.Errorf("%w: expected removal revision must describe an existing file", ErrRevisionChanged)
	}
	binding, err := s.bindRestoreParents(rel, parents, false)
	if err != nil {
		return err
	}
	return RemoveWithinRevisionAuthorized(s.root, rel, expected, binding.parents)
}

func (s *RestoreSession) rollbackRestoreFailure(operation string, binding restoreParentBinding, primary error) error {
	var committed *CommittedError
	if errors.As(primary, &committed) || len(binding.created) == 0 {
		return primary
	}
	if err := s.rollbackRestoreParents(binding); err != nil {
		return &RecoveryError{Operation: operation, Err: errors.Join(primary, err)}
	}
	return primary
}
