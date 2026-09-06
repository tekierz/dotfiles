//go:build darwin

package safefile

import "golang.org/x/sys/unix"

// O_EVTONLY obtains a descriptor suitable for identity checks without requiring
// read permission or triggering regular-file content access.
func openRemovalTarget(parentFD int, target string) (int, error) {
	return unix.Openat(parentFD, target, unix.O_EVTONLY|unix.O_SYMLINK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}
