//go:build linux

package safefile

import "golang.org/x/sys/unix"

// O_PATH obtains an identity-only descriptor without requiring leaf read
// permission or opening device/FIFO content paths.
func openRemovalTarget(parentFD int, target string) (int, error) {
	return unix.Openat(parentFD, target, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}
