//go:build !darwin && !linux

package safefile

import "fmt"

func ObserveDirectoryStateWithin(_ string, _ string) (DirectoryState, *ParentChain, error) {
	return DirectoryState{}, nil, ErrUnsupported
}

func NewRestoreSession(_ string) (*RestoreSession, error) { return nil, ErrUnsupported }

func (s *RestoreSession) bindRestoreParents(_ string, _ *ParentChain, _ bool) (restoreParentBinding, error) {
	return restoreParentBinding{}, fmt.Errorf("bind restore parents: %w", ErrUnsupported)
}

func (s *RestoreSession) rollbackRestoreParents(_ restoreParentBinding) error {
	return fmt.Errorf("rollback restore parents: %w", ErrUnsupported)
}
