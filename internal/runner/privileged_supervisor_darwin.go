//go:build darwin

package runner

func enablePrivilegedDescendantReaping() error { return errPrivilegedSupervisorUnavailable }
func reapPrivilegedDescendants() error         { return nil }
