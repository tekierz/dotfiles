package safefile

import "sync"

// DirectoryState is an opaque observation of one directory leaf. The zero
// value is deliberately untracked and grants no mutation authority.
type DirectoryState struct {
	tracked  bool
	exists   bool
	snapshot *DirectorySnapshot
}

// Exists reports whether a tracked observation captured an existing directory.
func (s DirectoryState) Exists() bool { return s.tracked && s.exists }

// RestoreSession binds restore-parent work to one exact real root and owns the
// evidence for private parent directories created during that session.
type RestoreSession struct {
	mu           sync.Mutex
	root         string
	rootSnapshot *DirectorySnapshot
	created      map[string]*DirectorySnapshot
}

type restoreCreatedParent struct {
	rel      string
	snapshot *DirectorySnapshot
	parents  *ParentChain
}

type restoreParentBinding struct {
	session *RestoreSession
	tracked bool
	parents *ParentChain
	created []restoreCreatedParent
}
